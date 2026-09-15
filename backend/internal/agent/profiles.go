// Profils hotspot : import, ensure, paramètres et limites d'utilisateurs simultanés.
// Extrait du monolithe agent.go (N°88) — même package, contenu inchangé.
package agent

import (
	"fmt"
	"mikcloud/hotspot-api/internal/model"
	"strings"
)

// ImportChunkSize — nombre d'utilisateurs hotspot lus par commande
// import_hotspot. L'import est paginé : si le routeur a plus d'utilisateurs,
// le résultat du chunk en file le suivant (voir applyImportHotspot). La taille
// garde le corps POST (http-data) loin de la limite RouterOS (~64 Ko).
const ImportChunkSize = 300

// ImportProfilesMax — nombre de profils hotspot lus par commande (les profils
// sont peu nombreux en pratique ; 60 couvre très largement).

// ImportProfilesMax — nombre de profils hotspot lus par commande (les profils
// sont peu nombreux en pratique ; 60 couvre très largement).
const ImportProfilesMax = 60

// buildImportHotspot — lecture paginée des données EXISTANTES du routeur pour
// les importer dans le cloud (profils + utilisateurs hotspot).
//
// Format du rapport (POST /agent/result, form-encodé) :
//
//      profiles=name|rate-limit|shared-users|session-timeout;…
//      users=name|profile|disabled|comment|limit-bytes-total;…
//      total=<nb total d'utilisateurs sur le routeur>
//
// Garde-fous : les champs name/profile contenant un séparateur du protocole
// (| ; & = %) font sauter l'entrée ; le commentaire est tronqué à 60 caractères
// et neutralisé s'il contient un séparateur. La pagination (start/count inlinés
// par Go) découpe les utilisateurs par lots d'ImportChunkSize.

// buildImportHotspot — lecture paginée des données EXISTANTES du routeur pour
// les importer dans le cloud (profils + utilisateurs hotspot).
//
// Format du rapport (POST /agent/result, form-encodé) :
//
//	profiles=name|rate-limit|shared-users|session-timeout;…
//	users=name|profile|disabled|comment|limit-bytes-total;…
//	total=<nb total d'utilisateurs sur le routeur>
//
// Garde-fous : les champs name/profile contenant un séparateur du protocole
// (| ; & = %) font sauter l'entrée ; le commentaire est tronqué à 60 caractères
// et neutralisé s'il contient un séparateur. La pagination (start/count inlinés
// par Go) découpe les utilisateurs par lots d'ImportChunkSize.
func (b Builder) buildImportHotspot(cmd model.Command) string {
	start := int(plInt64(cmd.Payload, "start"))
	count := int(plInt64(cmd.Payload, "count"))
	if count <= 0 {
		count = ImportChunkSize
	}
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(`:local mikProf ""
:do {
  :local pn 0
  :foreach p in=[/ip hotspot user profile find] do={
    :if ($pn < ` + fmt.Sprintf("%d", ImportProfilesMax) + `) do={
      :local nm [:tostr [/ip hotspot user profile get $p name]]
      :local rl ""
      :do {
        :set rl [:tostr [/ip hotspot user profile get $p rate-limit]]
      } on-error={}
      :local sh "1"
      :do {
        :set sh [:tostr [/ip hotspot user profile get $p shared-users]]
      } on-error={}
      :local st ""
      :do {
        :set st [:tostr [/ip hotspot user profile get $p session-timeout]]
      } on-error={}
      :if ([:len $nm] > 0) do={
        :set mikProf ($mikProf . $nm . "|" . $rl . "|" . $sh . "|" . $st . ";")
      }
      :set pn ($pn + 1)
    }
  }
} on-error={}
:local mikIds [/ip hotspot user find]
:local mikTotal [:len $mikIds]
:local mikUsr ""
:local mikOut 0
:local n 0
:foreach u in=$mikIds do={
  :if ($n >= ` + fmt.Sprintf("%d", start) + ` && $n < (` + fmt.Sprintf("%d", start) + ` + ` + fmt.Sprintf("%d", count) + `)) do={
    :local nm [:tostr [/ip hotspot user get $u name]]
    :local pf ""
    :do {
      :set pf [:tostr [/ip hotspot user get $u profile]]
    } on-error={}
    :local ds "false"
    :do {
      :set ds [:tostr [/ip hotspot user get $u disabled]]
    } on-error={}
    :local cm ""
    :do {
      :set cm [:tostr [/ip hotspot user get $u comment]]
    } on-error={}
    :local lb "0"
    :do {
      :set lb [:tostr [/ip hotspot user get $u limit-bytes-total]]
    } on-error={}
    :local bad false
    :if ([:typeof [:find $nm "|"]] != "nil") do={ :set bad true }
    :if ([:typeof [:find $nm ";"]] != "nil") do={ :set bad true }
    :if ([:typeof [:find $nm "&"]] != "nil") do={ :set bad true }
    :if ([:typeof [:find $nm "="]] != "nil") do={ :set bad true }
    :if ([:typeof [:find $nm "%"]] != "nil") do={ :set bad true }
    :if ([:typeof [:find $pf "|"]] != "nil") do={ :set bad true }
    :if ([:typeof [:find $pf ";"]] != "nil") do={ :set bad true }
    :if ([:typeof [:find $pf "&"]] != "nil") do={ :set bad true }
    :if ([:typeof [:find $pf "="]] != "nil") do={ :set bad true }
    :if ([:typeof [:find $pf "%"]] != "nil") do={ :set bad true }
    :if ([:len $cm] > 60) do={ :set cm [:pick $cm 0 60] }
    :if ([:typeof [:find $cm "|"]] != "nil") do={ :set cm "-" }
    :if ([:typeof [:find $cm ";"]] != "nil") do={ :set cm "-" }
    :if ([:typeof [:find $cm "&"]] != "nil") do={ :set cm "-" }
    :if ([:typeof [:find $cm "="]] != "nil") do={ :set cm "-" }
    :if ([:typeof [:find $cm "%"]] != "nil") do={ :set cm "-" }
    :if ([:typeof [:find $cm "+"]] != "nil") do={ :set cm "-" }
    :if (!$bad && [:len $nm] > 0) do={
      :set mikUsr ($mikUsr . $nm . "|" . $pf . "|" . $ds . "|" . $cm . "|" . $lb . ";")
      :set mikOut ($mikOut + 1)
    }
  }
  :set n ($n + 1)
}
`)
	sb.WriteString(`/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + cmd.ID +
		`&status=ok&total=". $mikTotal ."&out=". $mikOut ."&profiles=". $mikProf ."&users=". $mikUsr) output=none` + "\n")
	return sb.String()
}

// Builder construit les scripts de commandes pour un routeur donné.

// profileEnsureLine — garantit que le profil du cloud existe sur le routeur
// avec ses paramètres EXACTS : add (création) puis set (alignement si le
// profil existait déjà — créé dans Winbox ou par un import). Sans ce set, un
// profil préexistant sans session-timeout restait sans timeout → les vouchers
// n'expiraient jamais. Le set est inconditionnel : sur un profil absent,
// `set [find …]` sans résultat est un no-op silencieux en RouterOS, jamais
// une erreur. Deux lignes indépendantes plutôt qu'un :do imbriqué : une
// erreur de l'add n'empêche jamais le set. Renvoie TROIS lignes terminées (add, set, quota rétroactif des utilisateurs existants).
func profileEnsureLine(p ProfileRef) string {
	add := `/ip hotspot user profile add name="` + rosEscape(p.Name) + `"` +
		profileAddParams(p)
	return ":do { " + add + " } on-error={ :log info \"mikcloud: profil " +
		rosEscape(p.Name) + " deja present, mise a jour\" }\n" +
		profileSetLine(p.Name, p) + profileUserLimitLine(p)
}

// profileAddParams — paramètres de CRÉATION d'un profil. Une clé absente du
// payload n'est pas écrite ; le verrou « 1er appareil » seulement s'il est actif.

// profileAddParams — paramètres de CRÉATION d'un profil. Une clé absente du
// payload n'est pas écrite ; le verrou « 1er appareil » seulement s'il est actif.
func profileAddParams(p ProfileRef) string {
	s := ""
	if p.HasRate && p.RateLimit != "" {
		s += ` rate-limit="` + rosEscape(p.RateLimit) + `"`
	}
	if p.HasTimeout && p.SessionTimeoutMin > 0 {
		s += " session-timeout=" + rosMinutes(p.SessionTimeoutMin)
	}
	if p.HasShared && p.SharedUsers > 0 {
		s += fmt.Sprintf(" shared-users=%d", p.SharedUsers)
	}
	if p.HasPool && p.AddressPool != "" {
		s += ` address-pool="` + rosEscape(p.AddressPool) + `"`
	}
	if p.HasQueue && p.ParentQueue != "" {
		s += ` parent-queue="` + rosEscape(p.ParentQueue) + `"`
	}
	// N°106 — on-login COMBINÉ : verrou « 1er appareil » et/ou bridage quota
	// (un seul champ on-login par profil — blocs :do indépendants concaténés).
	if onLogin := profileOnLoginScript(p); onLogin != "" {
		s += ` on-login="` + rosScriptValue(onLogin) + `"`
	}
	// N°106 — on-logout de bridage : retire la file mikthrottle-<user> quand
	// la dernière session de l'utilisateur se ferme (profil throttle uniquement).
	if p.QuotaThrottle {
		s += ` on-logout="` + rosScriptValue(onLogoutQuotaScript) + `"`
	}
	return s
}

// profileSetLine — ligne `set` qui aligne le profil routeur sur le cloud
// (session-timeout, rate-limit, shared-users, verrou « 1er appareil »).
// Une clé ABSENTE du payload (commande héritée) n'est JAMAIS effacée : sans
// ce garde-fou, un set partiel remettrait session-timeout à 0 et les vouchers
// liés n'expireraient plus.

// profileSetLine — ligne `set` qui aligne le profil routeur sur le cloud
// (session-timeout, rate-limit, shared-users, verrou « 1er appareil »).
// Une clé ABSENTE du payload (commande héritée) n'est JAMAIS effacée : sans
// ce garde-fou, un set partiel remettrait session-timeout à 0 et les vouchers
// liés n'expireraient plus.
func profileSetLine(name string, p ProfileRef) string {
	s := `/ip hotspot user profile set [find name="` + rosEscape(name) + `"]`
	if p.HasRate {
		s += ` rate-limit="` + rosEscape(p.RateLimit) + `"`
	}
	if p.HasTimeout {
		if st := rosMinutes(p.SessionTimeoutMin); st != "" {
			s += " session-timeout=" + st
		} else {
			s += ` session-timeout=0s`
		}
	}
	if p.HasShared && p.SharedUsers > 0 {
		s += fmt.Sprintf(" shared-users=%d", p.SharedUsers)
	}
	if p.HasPool {
		if p.AddressPool != "" {
			s += ` address-pool="` + rosEscape(p.AddressPool) + `"`
		} else {
			s += ` address-pool=none`
		}
	}
	if p.HasQueue {
		if p.ParentQueue != "" {
			s += ` parent-queue="` + rosEscape(p.ParentQueue) + `"`
		} else {
			s += ` parent-queue=none`
		}
	}
	// N°113 — le set aligne AUSSI les scripts de bridage : profileOnLoginScript
	// combine verrou « 1er appareil » ET quota (un seul champ on-login par
	// profil), et on-logout porte le retrait de la file pour les profils
	// throttle (vidé sinon : un profil repassé en « cut » perd ses scripts —
	// sémantique d'alignement complet du set).
	// AVANT N°113 (bug du terrain N°106) : le set écrasait on-login avec le
	// verrou SEUL (ou vide) — le on-login de bridage posé par le add de la
	// MÊME commande était effacé aussitôt, et on-logout n'était jamais
	// aligné : la fenêtre de re-login n'était couverte NULLE PART.
	if onLogin := profileOnLoginScript(p); onLogin != "" {
		s += ` on-login="` + rosScriptValue(onLogin) + `"`
	} else {
		s += ` on-login=""`
	}
	if p.QuotaThrottle {
		s += ` on-logout="` + rosScriptValue(onLogoutQuotaScript) + `"`
	} else {
		s += ` on-logout=""`
	}
	return ":do { " + s + " } on-error={ :log info \"mikcloud: profil " + rosEscape(name) + " inaccessible\" }\n"
}

// profileUserLimitLine — applique RÉTROACTIVEMENT le quota de temps du profil
// (limit-uptime) à tous les utilisateurs hotspot DÉJÀ PRÉSENTS sur le routeur
// sous ce profil : les vouchers créés avant l'introduction du quota n'en
// portaient pas et, après la coupe de session, le même code repartait pour une
// session complète à chaque reconnexion. `set [find profile=…]` sans résultat
// est un no-op silencieux ; un utilisateur dont le cumul dépasse déjà la limite
// est refusé dès l'application (le routeur compare uptime-used à limit-uptime
// à l'authentification, cookie MAC compris). Aucune ligne si la durée est
// absente du payload ou nulle : on n'efface JAMAIS un quota que le payload ne
// portait pas (cf. flags Has*).

// profileUserLimitLine — applique RÉTROACTIVEMENT le quota de temps du profil
// (limit-uptime) à tous les utilisateurs hotspot DÉJÀ PRÉSENTS sur le routeur
// sous ce profil : les vouchers créés avant l'introduction du quota n'en
// portaient pas et, après la coupe de session, le même code repartait pour une
// session complète à chaque reconnexion. `set [find profile=…]` sans résultat
// est un no-op silencieux ; un utilisateur dont le cumul dépasse déjà la limite
// est refusé dès l'application (le routeur compare uptime-used à limit-uptime
// à l'authentification, cookie MAC compris). Aucune ligne si la durée est
// absente du payload ou nulle : on n'efface JAMAIS un quota que le payload ne
// portait pas (cf. flags Has*).
func profileUserLimitLine(p ProfileRef) string {
	if !p.HasTimeout || p.SessionTimeoutMin <= 0 {
		return ""
	}
	// Cible uniquement les utilisateurs SANS quota (limit-uptime=0s) : les
	// vouchers portent désormais leur propre limit-uptime par lot (parité
	// Mikhmon) — un set global écraserait ces quotas individuels.
	s := `/ip hotspot user set [find where profile="` + rosEscape(p.Name) + `" && limit-uptime=0s] limit-uptime=` + rosMinutes(p.SessionTimeoutMin)
	return ":do { " + s + " } on-error={ :log info \"mikcloud: quota temps profil " + rosEscape(p.Name) + " inaccessible\" }\n"
}

// buildProfileSet — v2 : synchronise UN profil routeur sur l'état du cloud :
// verrou « 1er appareil » (on-login), rate-limit, session-timeout et
// shared-users. Profil absent = no-op silencieux (il sera créé AVEC les bons
// paramètres à la prochaine création d'utilisateur) : la commande reste un
// succès, l'état du cloud fait foi.

// buildProfileSet — v2 : synchronise UN profil routeur sur l'état du cloud :
// verrou « 1er appareil » (on-login), rate-limit, session-timeout et
// shared-users. Profil absent = no-op silencieux (il sera créé AVEC les bons
// paramètres à la prochaine création d'utilisateur) : la commande reste un
// succès, l'état du cloud fait foi.
func (b Builder) buildProfileSet(cmd model.Command) string {
	name := SanitizeName(plStr(cmd.Payload, "name"))
	p := ProfileRef{
		Name:              name,
		RateLimit:         plStr(cmd.Payload, "rateLimit"),
		SessionTimeoutMin: int(plInt64(cmd.Payload, "sessionTimeoutMin")),
		SharedUsers:       int(plInt64(cmd.Payload, "sharedUsers")),
		LockFirstDevice:   plBool(cmd.Payload, "lockFirstDevice"),
		AddressPool:       plStr(cmd.Payload, "addressPool"),
		ParentQueue:       plStr(cmd.Payload, "parentQueue"),
		HasRate:           plHas(cmd.Payload, "rateLimit"),
		HasTimeout:        plHas(cmd.Payload, "sessionTimeoutMin"),
		HasShared:         plHas(cmd.Payload, "sharedUsers"),
		HasPool:           plHas(cmd.Payload, "addressPool"),
		HasQueue:          plHas(cmd.Payload, "parentQueue"),
		// N°106 — mode bridage (scripts on-login/on-logout du profil).
		QuotaThrottle: plStr(cmd.Payload, "quotaMode") == model.QuotaModeThrottle,
	}
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(profileSetLine(name, p))
	sb.WriteString(profileUserLimitLine(p))
	sb.WriteString(b.reportLine(cmd.ID, true, nil) + "\n")
	return sb.String()
}

// header — commentaire d'audit d'une commande (parsé aussi par le simulateur).

// ProfileRef — référence compacte d'un profil pour les scripts.
type ProfileRef struct {
	Name              string
	RateLimit         string
	SessionTimeoutMin int
	SharedUsers       int
	LockFirstDevice   bool
	// Parité Mikhmon : address-pool / parent-queue RouterOS ("" = none).
	AddressPool string
	ParentQueue string
	// Présence des clés dans le payload : une commande héritée peut n'exporter
	// que le nom (voire le verrou) — dans ce cas le set ne doit PAS toucher au
	// rate-limit/session-timeout/shared-users du routeur (jamais d'effacement
	// accidentel d'un paramètre que le payload ne portait pas).
	HasRate    bool
	HasTimeout bool
	HasShared  bool
	HasPool    bool
	HasQueue   bool
	// N°106 — mode bridage : le profil porte les scripts génériques on-login/
	// on-logout qui posent/retirent la file mikthrottle-<user> (le quota et le
	// débit vivent dans le marqueur mikq: du commentaire de chaque utilisateur).
	QuotaThrottle bool
}

func plProfile(p map[string]any, k string) ProfileRef {
	m, ok := p[k].(map[string]any)
	if !ok {
		return ProfileRef{}
	}
	ref := ProfileRef{Name: plStr(m, "name"), RateLimit: plStr(m, "rateLimit")}
	// Tolérant aux DEUX formes : payload en mémoire (int Go) ou relu du JSON
	// (float64). L'assertion float64 seule perdait sessionTimeoutMin sur le
	// chemin live (payload construit par le handler avec des int) → profils
	// créés SANS session-timeout, vouchers sans expiration (incident 31/08).
	if _, ok := m["rateLimit"]; ok {
		ref.HasRate = true
	}
	if _, ok := m["sessionTimeoutMin"]; ok {
		ref.HasTimeout = true
		ref.SessionTimeoutMin = int(plInt64(m, "sessionTimeoutMin"))
	}
	if _, ok := m["sharedUsers"]; ok {
		ref.HasShared = true
		ref.SharedUsers = int(plInt64(m, "sharedUsers"))
	}
	ref.LockFirstDevice = plBool(m, "lockFirstDevice")
	if _, ok := m["addressPool"]; ok {
		ref.HasPool = true
		ref.AddressPool = plStr(m, "addressPool")
	}
	if _, ok := m["parentQueue"]; ok {
		ref.HasQueue = true
		ref.ParentQueue = plStr(m, "parentQueue")
	}
	// N°106 — mode bridage (scripts génériques on-login/on-logout du profil).
	ref.QuotaThrottle = plStr(m, "quotaMode") == model.QuotaModeThrottle
	return ref
}

// VoucherRef — ligne de voucher pour un batch.
