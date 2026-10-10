// N°293 — PPPoE Phase A (chantier ⑥ de N°289, Option V lot 3) : gestion des
// secrets d'un pppoe-server EXISTANT via le canal agent (décision D1 —
// commandes ppp_* canoniques, zéro cred API stockée, zéro port public,
// CGNAT-proof ; le renfort API directe à travers wg0 est la phase B).
//
// Périmètre v1 (décision D2) : le cloud GÈRE des secrets PPP sur un serveur
// PPPoE déjà configuré chez le WISP — il ne provisionne NI l'interface
// pppoe-server, NI les profils PPP (Profile référence un nom de profil
// EXISTANT côté routeur). Le provisioning assisté est la phase B.
//
// Conventions des builders (vérifiées N°285/N°290, gabarits wireguard.go +
// users.go) : header(cmd), :local ok<ID> true, :do { … } on-error={
// :set ok<ID> false }, rapport b.resultLines ou /tool fetch dynamique quand
// la valeur est lue côté routeur (pattern buildWgSetup / buildReadState) ;
// variables locales préfixées « q » + idSafe(cmd.ID) (unicité
// inter-commandes d'un même import) ; rosEscape() sur TOUTE valeur
// interpolée — exec sans shell côté routeur, la défense est aux DEUX bords
// (le handler valide model.ValidPppName/ValidPppProfileName AVANT
// l'enfilement, le builder re-sanitise : aucune valeur ne franchit les deux
// frontières non vérifiée).
//
// HONNÊTETÉ v1 (parité) : le read_state du chunk final rapporte la liste
// des secrets présents (cf. readstate.go) et rafraîchit LastSeenOnRouter —
// mais une ligne cloud ABSENTE du routeur n'est NI détruite NI re-créée
// automatiquement (contrairement aux vouchers N°162) : un abonné payant
// supprimé à la main en Winbox ne doit pas ressusciter en silence. La
// réparation reste un geste EXPLICITE : re-enregistrer le secret (le marqueur
// repair de ppp_secret_add rend l'opération idempotente — présent = ok sans
// retouche, pattern voucher_batch N°162).
package agent

import (
	"strconv"
	"strings"

	"mikcloud/hotspot-api/internal/model"
)

// PppActiveReportCap — plafond d'entrées du rapport ppp_read_active (pattern
// sessions du read_state : au-delà de 250 sessions actives, la liste reste
// bornée pour garder le corps POST loin de la limite RouterOS ~64 Ko).
const PppActiveReportCap = 250

// pppSafeName — re-sanitisation défensive du NOM d'un secret (le handler a
// déjà validé model.ValidPppName ; ici on réduit AUSSI : tout caractère hors
// alphabet sûr disparaît, jamais d'injection même sur un payload corrompu).
func pppSafeName(s string) string {
	var sb strings.Builder
	for _, c := range strings.TrimSpace(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '.', c == '_', c == '-', c == '@':
			sb.WriteRune(c)
		}
	}
	out := sb.String()
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

// pppSafeProfile — idem pour le profil (majuscules admises).
func pppSafeProfile(s string) string {
	var sb strings.Builder
	for _, c := range strings.TrimSpace(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '.', c == '_', c == '-', c == '@':
			sb.WriteRune(c)
		}
	}
	out := sb.String()
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

// pppSecretLine — la ligne /ppp/secret/add d'un secret (service=pppoe
// constant v1, commentaire routeur = marqueur mikcloud-ppp + commentaire
// libre du gérant — pattern mikcloud-wg : seuls les objets marqués nous
// appartiennent). Toutes les valeurs sont échappées (rosEscape).
func pppSecretLine(name, password, profile, comment string, disabled bool) string {
	line := `/ppp/secret/add name="` + rosEscape(pppSafeName(name)) + `"`
	if password != "" {
		line += ` password="` + rosEscape(password) + `"`
	}
	line += ` profile="` + rosEscape(pppSafeProfile(profile)) + `"`
	line += " service=" + model.PppService
	line += ` comment="` + rosEscape(model.PppMarker+" "+comment) + `"`
	if disabled {
		line += " disabled=yes"
	}
	return line
}

// buildPppSecretAdd — ppp_secret_add : création du secret sur le routeur.
// Payload : name, password, profile, comment, disabled, repair.
//
// repair (parité voucher_batch N°162) : l'autoréparation n'AJOUTE que si le
// nom est absent ([:len [/ppp/secret find name=…]] = 0) — un secret déjà
// présent n'est NI recompté NI retouché (verrou MAC, marqueurs et
// commentaires posés à la main préservés) et le rapport est ok : idempotent
// ET véridique. Sans repair (création console), un doublon serait refusé par
// RouterOS → rapport error (l'unicité PAR ROUTEUR est déjà garantie côté
// cloud, le doublon ne peut venir que d'une création manuelle concurrente).
func (b Builder) buildPppSecretAdd(cmd model.Command) string {
	name := pppSafeName(plStr(cmd.Payload, "name"))
	password := plStr(cmd.Payload, "password")
	profile := pppSafeProfile(plStr(cmd.Payload, "profile"))
	comment := plStr(cmd.Payload, "comment")
	disabled := plBool(cmd.Payload, "disabled")
	repair := plBool(cmd.Payload, "repair")
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	line := pppSecretLine(name, password, profile, comment, disabled)
	if repair {
		// Idempotence de la réparation : déjà présent = état voulu, ok
		// sans retouche (aucun set, aucun recomptage).
		sb.WriteString(`:if ([:len [/ppp/secret find name="` + rosEscape(name) + `"]] = 0) do={ :do { ` +
			line + ` } on-error={ :set ` + okVar + ` false; :log warning "mikcloud: add secret ppp echoue" } }` + "\n")
	} else {
		sb.WriteString(":do { " + line + " } on-error={ :set " + okVar + " false }\n")
	}
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// buildPppSecretSet — ppp_secret_set : /ppp/secret/set [find name=…] avec
// SEULEMENT les propriétés présentes dans le payload (password, profile,
// disabled, comment) — la ligne est construite dynamiquement (pattern
// buildUserSet) : un set partiel ne touche jamais les autres propriétés.
func (b Builder) buildPppSecretSet(cmd model.Command) string {
	name := pppSafeName(plStr(cmd.Payload, "name"))
	okVar := "ok" + idSafe(cmd.ID)
	set := `/ppp/secret/set [find name="` + rosEscape(name) + `"]`
	if plHas(cmd.Payload, "password") {
		set += ` password="` + rosEscape(plStr(cmd.Payload, "password")) + `"`
	}
	if plHas(cmd.Payload, "profile") {
		set += ` profile="` + rosEscape(pppSafeProfile(plStr(cmd.Payload, "profile"))) + `"`
	}
	if plHas(cmd.Payload, "comment") {
		set += ` comment="` + rosEscape(model.PppMarker+" "+plStr(cmd.Payload, "comment")) + `"`
	}
	if plBool(cmd.Payload, "disabled") {
		set += " disabled=yes"
	} else if plHas(cmd.Payload, "disabled") {
		set += " disabled=no"
	}
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { " + set + " } on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// buildPppSecretRemove — ppp_secret_remove : retrait du secret. Absent =
// état convergé (ok, pas d'erreur) — le remove RouterOS sur un find vide
// est un no-op silencieux ; le rapport reste VERIDIQUE (une vraie erreur
// routeur porte ok=false). Le retrait du REGISTRE cloud n'a lieu qu'à la
// confirmation (discipline N°291 : jamais de retrait avant confirmation).
func (b Builder) buildPppSecretRemove(cmd model.Command) string {
	name := pppSafeName(plStr(cmd.Payload, "name"))
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(`:do { /ppp/secret/remove [find name="` + rosEscape(name) + `"] } on-error={ :set ` + okVar + ` false }` + "\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// buildPppKick — ppp_kick : déconnexion de la session PPPoE d'un abonné
// (/ppp/active/remove). Absent = session déjà fermée (ok, convergé —
// pattern buildKick). Le kick n'est PAS la suppression du secret : le
// service peut être rétabli par l'abonné (reconnexion immédiate).
func (b Builder) buildPppKick(cmd model.Command) string {
	name := rosEscape(pppSafeName(plStr(cmd.Payload, "name")))
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(`:do { /ppp/active/remove [find name="` + name + `"] } on-error={ :log info "mikcloud: session ppp deja fermee" }` + "\n")
	sb.WriteString(b.reportLine(cmd.ID, true, nil) + "\n")
	return sb.String()
}

// buildPppReadSecrets — ppp_read_secrets : /ppp/secret/print PAGINÉ (pattern
// buildReadState v5 — placeholders @@START@@/@@END@@/@@COUNT@@ substitués en
// dernier sur la chaîne brute). Format « name|profile|disabled|service|comment; »
// (séparateurs | et ; — convention read_* F9), rapporté via /tool fetch
// http-data dynamique sous le paramètre « data » (pattern read_dhcp : la
// ligne brute atterrit dans Command.Result["data"], cache outil ≤ 120 s).
// Le total EXACT est rapporté par chaque chunk (le cloud connaît le parc dès
// le premier) ; trunc informatif.
func (b Builder) buildPppReadSecrets(cmd model.Command) string {
	start := int(plInt64(cmd.Payload, "start"))
	count := int(plInt64(cmd.Payload, "count"))
	if start < 0 {
		start = 0
	}
	if count <= 0 || count > ReadChunkSize {
		count = ReadChunkSize
	}
	end := start + count
	p := "q" + idSafe(cmd.ID) // préfixe des variables locales (unicité inter-commandes d'un même import)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(`:local ` + p + `ids [/ppp/secret find]
:local ` + p + `tot [:len $` + p + `ids]
:local ` + p + `out ""
:local ` + p + `emitted 0
:local ` + p + `n 0
:foreach s in=$` + p + `ids do={
  :if ($` + p + `n >= @@START@@ && $` + p + `n < @@END@@) do={
    :set ` + p + `out ($` + p + `out . [:tostr [/ppp/secret get $s name]] . "|" . [:tostr [/ppp/secret get $s profile]] . "|" . [:tostr [/ppp/secret get $s disabled]] . "|" . [:tostr [/ppp/secret get $s service]] . "|" . [:tostr [/ppp/secret get $s comment]] . ";")
    :set ` + p + `emitted ($` + p + `emitted + 1)
  }
  :set ` + p + `n ($` + p + `n + 1)
}
:local ` + p + `trunc "false"
:if (@@END@@ < $` + p + `tot) do={ :set ` + p + `trunc "true" }
`)
	sb.WriteString(`/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + cmd.ID +
		`&status=ok&total=". $` + p + `tot ."&start=@@START@@&count=@@COUNT@@&out=". $` + p + `emitted ."&data=". $` + p + `out ."&trunc=". $` + p + `trunc) output=none` + "\n")
	out := strings.NewReplacer("@@START@@", strconv.Itoa(start), "@@END@@", strconv.Itoa(end), "@@COUNT@@", strconv.Itoa(count)).Replace(sb.String())
	return out
}

// buildPppReadActive — ppp_read_active : /ppp/active/print (sessions PPPoE
// ACTIVES : name|service|caller-id|address|uptime), plafond
// PppActiveReportCap entrées (pattern sessions read_state — les compteurs
// exacts restent honnêtes : total rapporté même au-delà du plafond). Le
// rapport est lu côté routeur → fetch dynamique (pattern buildWgSetup),
// liste sous « data » (cache outil, pattern read_dhcp).
func (b Builder) buildPppReadActive(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	p := "q" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(`:local ` + p + `act ""
:local ` + p + `n 0
:do {
  :foreach a in=[/ppp/active find] do={
    :if ($` + p + `n < ` + strconv.Itoa(PppActiveReportCap) + `) do={
      :set ` + p + `act ($` + p + `act . [:tostr [/ppp/active get $a name]] . "|" . [:tostr [/ppp/active get $a service]] . "|" . [:tostr [/ppp/active get $a caller-id]] . "|" . [:tostr [/ppp/active get $a address]] . "|" . [:tostr [/ppp/active get $a uptime]] . ";")
      :set ` + p + `n ($` + p + `n + 1)
    }
  }
} on-error={ :set ` + okVar + ` false }
:local ` + p + `tot [:len [/ppp/active find]]
`)
	ok := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + cmd.ID + `&status=ok&total=". $` + p + `tot ."&data=". $` + p + `act) output=none`
	ko := b.reportLine(cmd.ID, false, map[string]string{"message": "lecture des sessions ppp impossible"})
	sb.WriteString(":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n")
	return sb.String()
}

// ---------------------------------------------------------------------------
// Payloads canoniques (pattern WgSetupPayloadFrom N°285)
// ---------------------------------------------------------------------------

// PppSecretAddPayloadFrom — payload canonique de ppp_secret_add (le handler
// l'appelle après validation ; le builder ne lit QUE ces clés). repair =
// autoréparation idempotente (parité : présent = ok sans retouche).
func PppSecretAddPayloadFrom(name, password, profile, comment string, disabled, repair bool) map[string]any {
	payload := map[string]any{
		"name":     name,
		"password": password,
		"profile":  profile,
		"comment":  comment,
		"disabled": disabled,
	}
	if repair {
		payload["repair"] = true
	}
	return payload
}

// PppSecretSetPayloadFrom — payload canonique de ppp_secret_set : seuls les
// champs POINTÉS sont présents dans le payload (la ligne set est construite
// dynamiquement d'après la présence — nil = propriété non touchée).
func PppSecretSetPayloadFrom(password, profile, comment *string, disabled *bool) map[string]any {
	payload := map[string]any{}
	if password != nil {
		payload["password"] = *password
	}
	if profile != nil {
		payload["profile"] = *profile
	}
	if comment != nil {
		payload["comment"] = *comment
	}
	if disabled != nil {
		payload["disabled"] = *disabled
	}
	return payload
}
