// Package agent — tokens de provisionning + générateur de scripts RouterOS.
//
// Modèle « facilité déconcertante » : le routeur appelle le cloud (connexions
// 100 % sortantes, compatibles CGNAT/Starlink) toutes les 45 s via un scheduler
// natif. Le cloud répond soit "# mikcloud nop" (rien à faire), soit un script
// .rsc que le routeur exécute via /import. Chaque commande se rapporte elle-même
// via /tool fetch POST /agent/result.
//
// Ce fichier ne contient AUCUNE dépendance externe (stdlib uniquement).
package agent

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"mikcloud/hotspot-api/internal/model"
)

// ScriptFilename — nom du fichier de commandes sur le routeur.
const ScriptFilename = "mikcloud-cmd.rsc"

// SchedulerName — nom du scheduler créé sur le routeur.
const SchedulerName = "mikcloud-agent"

// ---------------------------------------------------------------------------
// Tokens
// ---------------------------------------------------------------------------

// NewToken génère un token d'agent (32 caractères base64url, 192 bits d'entropie).
func NewToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken — SHA-256 hexadécimal du token (seul stockage côté cloud).
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// Preview retourne les 4 premiers caractères du token pour l'affichage.
func Preview(token string) string {
	if len(token) > 4 {
		return token[:4] + "…"
	}
	return token
}

// ---------------------------------------------------------------------------
// Échappement / assainissement RouterOS
// ---------------------------------------------------------------------------

// rosEscape échappe une valeur pour une chaîne RouterOS entre guillemets.
func rosEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", " ", "\r", " ", "\t", " ")
	return r.Replace(s)
}

// SanitizeName assainit un nom (utilisateur ou profil) pour le routeur :
// caractères [A-Za-z0-9._-] conservés, le reste devient "-", 48 caractères max.
func SanitizeName(s string) string {
	var sb strings.Builder
	for _, c := range strings.TrimSpace(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '_', c == '-':
			sb.WriteRune(c)
		default:
			sb.WriteByte('-')
		}
	}
	out := sb.String()
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}

// idSafe retourne un suffixe de variable RouterOS valide depuis un id de commande.
func idSafe(id string) string {
	var sb strings.Builder
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			sb.WriteRune(c)
		}
	}
	return sb.String()
}

// rosMinutes convertit des minutes en durée RouterOS ("90m", "24h", "3d").
func rosMinutes(m int) string {
	if m <= 0 {
		return ""
	}
	switch {
	case m%1440 == 0:
		return fmt.Sprintf("%dd", m/1440)
	case m%60 == 0:
		return fmt.Sprintf("%dh", m/60)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

// ---------------------------------------------------------------------------
// Scripts
// ---------------------------------------------------------------------------

// InstallScript — le script de provisionning complet (1 collage dans Winbox).
func InstallScript(baseURL, token, routerName string) string {
	// Le nom n'apparaît que dans un commentaire .rsc : interdire tout retour à
	// la ligne ou caractère de contrôle (anti-injection dans le fichier).
	safeName := strings.Map(func(c rune) rune {
		if c == '\n' || c == '\r' || c == '\t' || c < 0x20 {
			return ' '
		}
		return c
	}, routerName)
	return `# ============================================================
# MikCloud — Installation agent  (routeur: ` + safeName + `)
# Coller CE fichier entier dans Terminal (Winbox) — 1 seule fois.
# ============================================================
# NB : les fetch essaient D'ABORD avec validation TLS, puis réessaient avec
# check-certificate=no. RouterOS < 7.19 n'embarque aucun certificat racine
# (la validation Let's Encrypt échoue) — le repli garantit la compatibilité
# 6.44 → 7.19+. Sur 7.19+ la validation réussit : trafic authentifié.
#
# ROUTEURS NEUFS (RouterOS 7.17+) : d'usine, fetch / scheduler / hotspot
# sont DÉSACTIVÉS par le « device-mode » restreint (protection anti-malware).
# Le pré-vol ci-dessous le détecte et affiche la marche à suivre au lieu
# d'échouer avec « not allowed by device-mode ».
# ============================================================
:global mikcloudToken "` + rosEscape(token) + `"
:global mikcloudUrl   "` + strings.TrimRight(baseURL, "/") + `"

# --- 0) Pre-vol : device-mode (RouterOS 7.13+ ; ignoré sur versions antérieures) ---
:local mikReady true
:do {
  :local dmS [/system device-mode get scheduler]
  :local dmF [/system device-mode get fetch]
  :if ((!$dmS) || (!$dmF)) do={ :set mikReady false }
  :do {
    :local dmH [/system device-mode get hotspot]
    :if (!$dmH) do={ :set mikReady false }
  } on-error={}
} on-error={}
:if (!$mikReady) do={
  :put ""
  :put "MIKCLOUD : installation bloquee par le device-mode de ce routeur."
  :put "1) Executez une seule fois :"
  :put "   /system/device-mode/update scheduler=yes fetch=yes hotspot=yes"
  :put "2) Confirmez PHYSIQUEMENT dans les 5 minutes : appuyez une fois sur"
  :put "   le bouton reset du routeur, OU debranchez puis rebranchez"
  :put "   l'alimentation (cold reboot). Le routeur redemarre alors."
  :put "3) Apres redemarrage, recollez CE script complet."
  :put ""
  :log error "MikCloud: device-mode restreint - /system/device-mode/update scheduler=yes fetch=yes hotspot=yes puis confirmation physique (bouton reset ou cold reboot)"
} else={

# --- 1) Inscription immediate : le cloud decouvre ce routeur ---
:local ident [/system identity get name]
:local mod [/system resource get board-name]
:local res [/system resource get]
:local ver [:tostr ($res->"version")]
:local up [:tostr ($res->"uptime")]
:do {
  /tool fetch url=("$mikcloudUrl/agent/register?token=$mikcloudToken") http-method=post \
    http-data=("identity=". [:tostr $ident] ."&model=". [:tostr $mod] ."&version=". $ver ."&uptime=". $up) output=none
} on-error={
  :do {
    /tool fetch url=("$mikcloudUrl/agent/register?token=$mikcloudToken") http-method=post \
      http-data=("identity=". [:tostr $ident] ."&model=". [:tostr $mod] ."&version=". $ver ."&uptime=". $up) check-certificate=no output=none
  } on-error={ :log warning "MikCloud: inscription impossible (reseau?)" }
}

# --- 2) Reinstallation propre : suppression d'un ancien agent ---
:do { /system scheduler remove [find name="` + SchedulerName + `"] } on-error={}

# --- 3) L'agent permanent : check-in toutes les 45 s (survit aux reboots) ---
:local mikAdded false
:do {
  /system scheduler add name="` + SchedulerName + `" interval=45s start-time=startup on-event={
    :local fetched true
    :do {
      /tool fetch url=($mikcloudUrl . "/agent/cmd?token=" . $mikcloudToken) \
        dst-path="` + ScriptFilename + `" keep-result=yes output=none
    } on-error={
      :do {
        /tool fetch url=($mikcloudUrl . "/agent/cmd?token=" . $mikcloudToken) check-certificate=no \
          dst-path="` + ScriptFilename + `" keep-result=yes output=none
      } on-error={ :set fetched false; :log warning "MikCloud agent: check-in echoue (reseau?)" }
    }
    :if ($fetched) do={
      :delay 1s
      /import file-name="` + ScriptFilename + `"
    }
  }
  :set mikAdded true
} on-error={
  :put "MIKCLOUD : echec de la creation du scheduler."
  :put "Si le message d'erreur est 'not allowed by device-mode', executez :"
  :put "  /system/device-mode/update scheduler=yes fetch=yes hotspot=yes"
  :put "puis confirmez physiquement (bouton reset ou coupure d'alimentation)."
  :log error "MikCloud: creation scheduler echouee (device-mode ?)"
}
:if ($mikAdded) do={ :log info "MikCloud: agent installe, check-in dans 45s" }

}
`
}

// NopScript — réponse quand il n'y a rien à faire (l'import ne fait rien).
func NopScript() string { return "# mikcloud nop\n" }

// Builder construit les scripts de commandes pour un routeur donné.
type Builder struct {
	BaseURL string
	Token   string
}

// ScriptFor retourne le script .rsc d'une commande (erreur si kind inconnu).
func (b Builder) ScriptFor(cmd model.Command) (string, error) {
	switch cmd.Kind {
	case model.CmdReadState:
		return b.buildReadState(cmd), nil
	case model.CmdUserAdd:
		return b.buildUserAdd(cmd), nil
	case model.CmdVoucherBatch:
		return b.buildVoucherBatch(cmd), nil
	case model.CmdUserRemove:
		return b.buildUserRemove(cmd), nil
	case model.CmdUserSet:
		return b.buildUserSet(cmd), nil
	case model.CmdKick:
		return b.buildKick(cmd), nil
	case model.CmdUserReset:
		return b.buildUserReset(cmd), nil
	// P1 (audit Mikhmon) — F7/F8/F9/F10
	case model.CmdPing:
		return b.buildPing(cmd), nil
	case model.CmdIpbindingAdd:
		return b.buildIpbindingAdd(cmd), nil
	case model.CmdIpbindingSet:
		return b.buildIpbindingSet(cmd), nil
	case model.CmdIpbindingRemove:
		return b.buildIpbindingRemove(cmd), nil
	case model.CmdReadDhcp:
		return b.buildReadDhcp(cmd), nil
	case model.CmdReadHosts:
		return b.buildReadHosts(cmd), nil
	case model.CmdReadCookies:
		return b.buildReadCookies(cmd), nil
	case model.CmdReadLog:
		return b.buildReadLog(cmd), nil
	case model.CmdReadScheduler:
		return b.buildReadScheduler(cmd), nil
	case model.CmdSchedulerAdd:
		return b.buildSchedulerAdd(cmd), nil
	case model.CmdSchedulerSet:
		return b.buildSchedulerSet(cmd), nil
	case model.CmdSchedulerRemove:
		return b.buildSchedulerRemove(cmd), nil
	case model.CmdReboot:
		return b.buildPower(cmd, "reboot"), nil
	case model.CmdShutdown:
		return b.buildPower(cmd, "shutdown"), nil
	default:
		return "", fmt.Errorf("kind de commande inconnu : %s", cmd.Kind)
	}
}

// resultLines — les deux branches de rapport (ok / error) d'une commande.
func (b Builder) resultLines(cmdID string, okVar string, extraOK map[string]string) string {
	ok := b.reportLine(cmdID, true, extraOK)
	ko := b.reportLine(cmdID, false, map[string]string{"message": "echec sur le routeur"})
	return ":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n"
}

// reportLine — le /tool fetch qui rapporte le résultat d'une commande.
func (b Builder) reportLine(cmdID string, ok bool, extra map[string]string) string {
	status := "ok"
	if !ok {
		status = "error"
	}
	data := "cmd=" + urlEscape(cmdID) + "&status=" + status
	for k, v := range extra {
		if v == "" {
			continue
		}
		data += "&" + k + "=" + urlEscape(v)
	}
	return `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("` + data + `") check-certificate=no output=none`
}

// urlEscape — encodage minimal sûr pour les valeurs d'URL (http-data).
func urlEscape(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			sb.WriteByte(c)
		case c == ' ':
			sb.WriteByte('_')
		default:
			sb.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return sb.String()
}

// profileEnsureLine — crée le profil sur le routeur s'il n'existe pas (idempotent).
func profileEnsureLine(p ProfileRef) string {
	line := `/ip hotspot user profile add name="` + rosEscape(p.Name) + `"`
	if p.RateLimit != "" {
		line += ` rate-limit="` + rosEscape(p.RateLimit) + `"`
	}
	if st := rosMinutes(p.SessionTimeoutMin); st != "" {
		line += " session-timeout=" + st
	}
	if p.SharedUsers > 0 {
		line += fmt.Sprintf(" shared-users=%d", p.SharedUsers)
	}
	return line
}

// header — commentaire d'audit d'une commande (parsé aussi par le simulateur).
func header(cmd model.Command) string {
	return "# mikcloud cmd " + cmd.ID + " " + cmd.Kind + "\n"
}

// ---------------------------------------------------------------------------
// Builders par kind
// ---------------------------------------------------------------------------

// buildReadState — v2 (P1, F6/F8) : en plus de la télémétrie, des utilisateurs
// et des sessions actives, le rapport contient :
//   - board / freehdd / totalhdd : nom de carte + disque en Mo. La division
//     octets → Mo se fait CÔTÉ SCRIPT (free-hdd-space/total-hdd-space sont des
//     octets ; [:tonum] peut échouer sur un appareil sans disque → fallback 0
//     via on-error) — le rapport reste compact et le cloud stocke des Mo tels
//     quels dans Router.BoardName/FreeHddMb/TotalHddMb ;
//   - ifaces=name:rx:tx;… : compteurs CUMULÉS rx-byte/tx-byte de /interface,
//     8 interfaces running maximum (le cloud calcule les débits par diff).
func (b Builder) buildReadState(cmd model.Command) string {
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(`:local rsres [/system resource get]
:local rver [:tostr ($rsres->"version")]
:local rup [:tostr ($rsres->"uptime")]
:local rcpu [:tostr ($rsres->"cpu-load")]
:local rmem [:tostr ($rsres->"free-memory")]
:local rmemb [:tostr ($rsres->"total-memory")]
:local rboard [:tostr ($rsres->"board-name")]
:local rfreehdd 0
:local rtotalhdd 0
:do {
  :set rfreehdd ([:tonum [:tostr ($rsres->"free-hdd-space")]] / 1048576)
  :set rtotalhdd ([:tonum [:tostr ($rsres->"total-hdd-space")]] / 1048576)
} on-error={ :set rfreehdd 0; :set rtotalhdd 0 }
:local rusr ""
:local rn 0
:foreach u in=[/ip hotspot user find] do={
  :if ($rn < 150) do={
    :set rusr ($rusr . [:tostr [/ip hotspot user get $u name]] . "|" . [:tostr [/ip hotspot user get $u profile]] . "|" . [:tostr [/ip hotspot user get $u disabled]] . ";")
    :set rn ($rn + 1)
  }
}
:local rsess ""
:local rsn 0
:foreach a in=[/ip hotspot active find] do={
  :if ($rsn < 100) do={
    :set rsess ($rsess . [:tostr [/ip hotspot active get $a user]] . "|" . [:tostr [/ip hotspot active get $a address]] . "|" . [:tostr [/ip hotspot active get $a uptime]] . ";")
    :set rsn ($rsn + 1)
  }
}
:local rif ""
:do {
  :local rin 0
  :foreach ifv in=[/interface find] do={
    :if ($rin < 8) do={
      :if ([:tostr [/interface get $ifv running]] = "true") do={
        :set rif ($rif . [:tostr [/interface get $ifv name]] . ":" . [:tostr [/interface get $ifv rx-byte]] . ":" . [:tostr [/interface get $ifv tx-byte]] . ";")
        :set rin ($rin + 1)
      }
    }
  }
} on-error={ :set rif "" }
`)
	sb.WriteString(`/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + cmd.ID +
		`&status=ok&version=". $rver ."&uptime=". $rup ."&cpu=". $rcpu ."&freemem=". $rmem ."&totalmem=". $rmemb` +
		` ."&board=". $rboard ."&freehdd=". $rfreehdd ."&totalhdd=". $rtotalhdd ."&users=". $rusr ."&sessions=". $rsess ."&ifaces=". $rif) check-certificate=no output=none` + "\n")
	return sb.String()
}

func (b Builder) buildUserAdd(cmd model.Command) string {
	name := SanitizeName(plStr(cmd.Payload, "name"))
	pass := plStr(cmd.Payload, "password")
	prof := plProfile(cmd.Payload, "profile")
	comment := plStr(cmd.Payload, "comment")
	quota := plInt64(cmd.Payload, "limitBytesTotal")
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { " + profileEnsureLine(prof) + " } on-error={ :log info \"mikcloud: profil deja present\" }\n")
	line := `/ip hotspot user add name="` + rosEscape(name) + `"`
	if pass != "" {
		line += ` password="` + rosEscape(pass) + `"`
	}
	line += ` profile="` + rosEscape(prof.Name) + `"`
	if quota > 0 {
		// Quota de données : limit-bytes-total (in + out cumulés, in/out laissés à 0).
		line += fmt.Sprintf(" limit-bytes-total=%d", quota)
	}
	if comment != "" {
		line += ` comment="` + rosEscape(comment) + `"`
	}
	sb.WriteString(":do { " + line + " } on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

func (b Builder) buildVoucherBatch(cmd model.Command) string {
	prof := plProfile(cmd.Payload, "profile")
	users := plUserList(cmd.Payload, "users")
	batch := plStr(cmd.Payload, "batch")
	custom := plStr(cmd.Payload, "comment")
	quota := plInt64(cmd.Payload, "limitBytesTotal")
	okVar := "ok" + idSafe(cmd.ID)
	// Commentaire router : la traçabilité MikCloud (lot) reste toujours présente ;
	// le commentaire libre du gérant est préfixé devant s'il existe.
	comment := ""
	switch {
	case custom != "" && batch != "":
		comment = custom + " · mikcloud:" + batch
	case custom != "":
		comment = custom
	case batch != "":
		comment = "mikcloud:" + batch
	}
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { " + profileEnsureLine(prof) + " } on-error={ :log info \"mikcloud: profil deja present\" }\n")
	for _, u := range users {
		line := `/ip hotspot user add name="` + rosEscape(SanitizeName(u.Name)) + `" password="` + rosEscape(u.Password) +
			`" profile="` + rosEscape(prof.Name) + `"`
		if quota > 0 {
			// Quota de données du lot (ex. « 5 Go = 500 F ») : limit-bytes-total
			// en octets — le routeur déconnecte le voucher une fois épuisé.
			line += fmt.Sprintf(" limit-bytes-total=%d", quota)
		}
		if comment != "" {
			line += ` comment="` + rosEscape(comment) + `"`
		}
		sb.WriteString(":do { " + line + " } on-error={ :log warning \"mikcloud: add voucher echoue\" }\n")
	}
	sb.WriteString(b.resultLines(cmd.ID, okVar, map[string]string{"created": fmt.Sprintf("%d", len(users))}))
	return sb.String()
}

func (b Builder) buildUserRemove(cmd model.Command) string {
	names := plStrList(cmd.Payload, "names")
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	for _, n := range names {
		sn := rosEscape(SanitizeName(n))
		sb.WriteString(":do { /ip hotspot user remove [find name=\"" + sn + "\"] } on-error={ :set " + okVar + " false }\n")
		sb.WriteString(":do { /ip hotspot active remove [find user=\"" + sn + "\"] } on-error={ :log info \"mikcloud: session deja fermee\" }\n")
	}
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

func (b Builder) buildUserSet(cmd model.Command) string {
	old := SanitizeName(plStr(cmd.Payload, "oldName"))
	name := SanitizeName(plStr(cmd.Payload, "name"))
	prof := plProfile(cmd.Payload, "profile")
	okVar := "ok" + idSafe(cmd.ID)
	if old == "" {
		old = name
	}
	set := `/ip hotspot user set [find name="` + rosEscape(old) + `"]`
	if name != "" && name != old {
		set += ` name="` + rosEscape(name) + `"`
	}
	if prof.Name != "" {
		set += ` profile="` + rosEscape(prof.Name) + `"`
	}
	if pw := plStr(cmd.Payload, "password"); pw != "" {
		set += ` password="` + rosEscape(pw) + `"`
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

func (b Builder) buildKick(cmd model.Command) string {
	user := rosEscape(SanitizeName(plStr(cmd.Payload, "user")))
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":do { /ip hotspot active remove [find user=\"" + user + "\"] } on-error={ :log info \"mikcloud: session introuvable\" }\n")
	sb.WriteString(b.reportLine(cmd.ID, true, nil) + "\n")
	return sb.String()
}

// buildUserReset — F4 : remet à zéro les compteurs (bytes/uptime) d'un
// utilisateur hotspot côté routeur (/ip hotspot user reset-counters).
func (b Builder) buildUserReset(cmd model.Command) string {
	name := rosEscape(SanitizeName(plStr(cmd.Payload, "name")))
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { /ip hotspot user reset-counters [find name=\"" + name + "\"] } on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// ---------------------------------------------------------------------------
// Builders P1 (audit Mikhmon) — F6/F7/F8/F9/F10
// ---------------------------------------------------------------------------
//
// Convention des commandes read_* (F9/F10) : les lignes sont collectées dans la
// variable $rdata au format « champ|champ|champ;champ|… » — champs séparés par
// « | » (les valeurs MAC/IP contiennent « : », un séparateur « : » serait
// ambigu) et entrées par « ; », comme users/sessions de read_state. Le cloud
// relit ce format avec splitAgentList.
//
// Les champs libres (messages de log, on-event) sont assainis CÔTÉ SCRIPT
// (substitution de « | » et « ; » — et « : » pour on-event — par des espaces,
// boucle caractère par caractère : RouterOS n'a pas de replace natif) ; le
// parseur cloud tolère de plus les caractères résiduels (contrat F9).

// fetchResultOK / fetchResultErr — les deux /tool fetch de rapport des
// commandes read_* (ok transporte $rdata, calculé côté routeur).
func (b Builder) fetchResultData(cmdID, okVar string) string {
	ok := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + cmdID + `&status=ok&data=". $rdata) check-certificate=no output=none`
	ko := b.reportLine(cmdID, false, map[string]string{"message": "lecture impossible sur le routeur"})
	return ":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n"
}

// buildPing — F8 : /ping count=4 as-value → sent/received/minMs/avgMs/maxMs.
// Robustesse vieux ROS : la conversion time→ms essaie la division arithmétique
// (ROS7) puis retombe sur l'analyse de la chaîne « Nms » ; chaque étape est
// isolée dans un :do on-error. Une perte (timeout) ne compte pas comme reçue.
func (b Builder) buildPing(cmd model.Command) string {
	// Défense en profondeur : la cible est validée par le handler (IP ou
	// hostname ≤ 253) ; on retire ici tout caractère de rupture de script.
	target := strings.Map(func(c rune) rune {
		switch c {
		case '"', '\\', '\n', '\r', '\t', ' ', ';', '|', '$', '%':
			return -1
		}
		return c
	}, strings.TrimSpace(plStr(cmd.Payload, "target")))
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":local psent 0\n:local precv 0\n:local pmin 0\n:local pavg 0\n:local pmax 0\n:local psum 0\n")
	sb.WriteString(":do {\n")
	sb.WriteString("  :local pres [/ping address=\"" + rosEscape(target) + "\" count=4 as-value]\n")
	sb.WriteString("  :set psent [:len $pres]\n")
	sb.WriteString("  :foreach pe in=$pres do={\n")
	sb.WriteString("    :if ([:typeof ($pe->\"time\")] != \"nothing\" && [:tostr ($pe->\"time\")] != \"timeout\") do={\n")
	sb.WriteString("      :local pms -1\n")
	sb.WriteString("      :do { :set pms (($pe->\"time\") / 1ms) } on-error={ }\n")
	sb.WriteString("      :if ($pms < 0) do={\n")
	sb.WriteString("        :do {\n")
	sb.WriteString("          :local pstr [:tostr ($pe->\"time\")]\n")
	sb.WriteString("          :local pcut [:find $pstr \"ms\"]\n")
	sb.WriteString("          :if ([:typeof $pcut] = \"num\") do={ :set pms [:tonum [:pick $pstr 0 $pcut]] }\n")
	sb.WriteString("        } on-error={ }\n")
	sb.WriteString("      }\n")
	sb.WriteString("      :if ($pms >= 0) do={\n")
	sb.WriteString("        :set precv ($precv + 1)\n")
	sb.WriteString("        :set psum ($psum + $pms)\n")
	sb.WriteString("        :if ($precv = 1) do={ :set pmin $pms; :set pmax $pms }\n")
	sb.WriteString("        :if ($pms < $pmin) do={ :set pmin $pms }\n")
	sb.WriteString("        :if ($pms > $pmax) do={ :set pmax $pms }\n")
	sb.WriteString("      }\n")
	sb.WriteString("    }\n")
	sb.WriteString("  }\n")
	sb.WriteString("  :if ($precv > 0) do={ :set pavg ($psum / $precv) }\n")
	sb.WriteString("} on-error={ :set " + okVar + " false }\n")
	// Rapport dynamique (valeurs calculées côté routeur, pas d'escape possible).
	ok := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + cmd.ID +
		`&status=ok&sent=". $psent ."&received=". $precv ."&minMs=". $pmin ."&avgMs=". $pavg ."&maxMs=". $pmax) check-certificate=no output=none`
	ko := b.reportLine(cmd.ID, false, map[string]string{"message": "ping impossible sur le routeur"})
	sb.WriteString(":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n")
	return sb.String()
}

// buildIpbindingAdd — F7 : /ip hotspot ip-binding add (mac requise, type
// bypassed|blocked, address/comment optionnels).
func (b Builder) buildIpbindingAdd(cmd model.Command) string {
	mac := strings.TrimSpace(plStr(cmd.Payload, "mac"))
	address := strings.TrimSpace(plStr(cmd.Payload, "address"))
	comment := plStr(cmd.Payload, "comment")
	typ := plStr(cmd.Payload, "type")
	if typ != "blocked" {
		typ = "bypassed"
	}
	line := `/ip hotspot ip-binding add mac-address="` + rosEscape(mac) + `" type=` + typ
	if address != "" {
		line += ` address="` + rosEscape(address) + `"`
	}
	if comment != "" {
		line += ` comment="` + rosEscape(comment) + `"`
	}
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { " + line + " } on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// buildIpbindingSet — F7 : set [find mac-address=…] disabled=yes|no (+address).
func (b Builder) buildIpbindingSet(cmd model.Command) string {
	mac := strings.TrimSpace(plStr(cmd.Payload, "mac"))
	set := `/ip hotspot ip-binding set [find mac-address="` + rosEscape(mac) + `"]`
	if plHas(cmd.Payload, "disabled") {
		if plBool(cmd.Payload, "disabled") {
			set += " disabled=yes"
		} else {
			set += " disabled=no"
		}
	}
	if address := strings.TrimSpace(plStr(cmd.Payload, "address")); address != "" {
		set += ` address="` + rosEscape(address) + `"`
	}
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { " + set + " } on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// buildIpbindingRemove — F7 : remove [find mac-address=…].
func (b Builder) buildIpbindingRemove(cmd model.Command) string {
	mac := strings.TrimSpace(plStr(cmd.Payload, "mac"))
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { /ip hotspot ip-binding remove [find mac-address=\"" + rosEscape(mac) + "\"] } on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// buildReadDhcp — F9 : /ip dhcp-server lease → mac|address|host|expires|status.
func (b Builder) buildReadDhcp(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n:local rdata \"\"\n")
	sb.WriteString(":do {\n  :local rn 0\n")
	sb.WriteString("  :foreach le in=[/ip dhcp-server lease find] do={\n")
	sb.WriteString("    :if ($rn < 100) do={\n")
	sb.WriteString(`      :set rdata ($rdata . [:tostr [/ip dhcp-server lease get $le mac-address]] . "|" . [:tostr [/ip dhcp-server lease get $le address]] . "|" . [:tostr [/ip dhcp-server lease get $le host-name]] . "|" . [:tostr [/ip dhcp-server lease get $le expires-after]] . "|" . [:tostr [/ip dhcp-server lease get $le status]] . ";")` + "\n")
	sb.WriteString("      :set rn ($rn + 1)\n")
	sb.WriteString("    }\n  }\n")
	sb.WriteString("} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.fetchResultData(cmd.ID, okVar))
	return sb.String()
}

// buildReadHosts — F9 : /ip hotspot host → mac|address|server|uptime|authorized
// (authorized=true si l'hôte est bypassed).
func (b Builder) buildReadHosts(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n:local rdata \"\"\n")
	sb.WriteString(":do {\n  :local rn 0\n")
	sb.WriteString("  :foreach he in=[/ip hotspot host find] do={\n")
	sb.WriteString("    :if ($rn < 100) do={\n")
	sb.WriteString("      :local hauth \"false\"\n")
	sb.WriteString("      :if ([:tostr [/ip hotspot host get $he bypassed]] = \"true\") do={ :set hauth \"true\" }\n")
	sb.WriteString(`      :set rdata ($rdata . [:tostr [/ip hotspot host get $he mac-address]] . "|" . [:tostr [/ip hotspot host get $he address]] . "|" . [:tostr [/ip hotspot host get $he server]] . "|" . [:tostr [/ip hotspot host get $he uptime]] . "|" . $hauth . ";")` + "\n")
	sb.WriteString("      :set rn ($rn + 1)\n")
	sb.WriteString("    }\n  }\n")
	sb.WriteString("} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.fetchResultData(cmd.ID, okVar))
	return sb.String()
}

// buildReadCookies — F9 : /ip hotspot cookie → user|mac|expires.
func (b Builder) buildReadCookies(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n:local rdata \"\"\n")
	sb.WriteString(":do {\n  :local rn 0\n")
	sb.WriteString("  :foreach ce in=[/ip hotspot cookie find] do={\n")
	sb.WriteString("    :if ($rn < 50) do={\n")
	sb.WriteString(`      :set rdata ($rdata . [:tostr [/ip hotspot cookie get $ce user]] . "|" . [:tostr [/ip hotspot cookie get $ce mac-address]] . "|" . [:tostr [/ip hotspot cookie get $ce expires-in]] . ";")` + "\n")
	sb.WriteString("      :set rn ($rn + 1)\n")
	sb.WriteString("    }\n  }\n")
	sb.WriteString("} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.fetchResultData(cmd.ID, okVar))
	return sb.String()
}

// buildReadLog — F9 : /log where topics~"hotspot" → time|topics|message,
// 50 DERNIÈRES lignes seulement (le /log find va du plus ancien au plus
// récent : on saute les ltotal-50 premières). Les « | » et « ; » des messages
// sont remplacés par des espaces avant concaténation.
func (b Builder) buildReadLog(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n:local rdata \"\"\n")
	sb.WriteString(":do {\n")
	sb.WriteString("  :local lall [/log find where topics~\"hotspot\"]\n")
	sb.WriteString("  :local ltotal [:len $lall]\n")
	sb.WriteString("  :local lstart 0\n")
	sb.WriteString("  :if ($ltotal > 50) do={ :set lstart ($ltotal - 50) }\n")
	sb.WriteString("  :local li 0\n")
	sb.WriteString("  :foreach le in=$lall do={\n")
	sb.WriteString("    :if ($li >= $lstart) do={\n")
	sb.WriteString("      :local lmsg [:tostr [/log get $le message]]\n")
	sb.WriteString("      :local lmsgc \"\"\n")
	sb.WriteString("      :if ([:len $lmsg] > 0) do={\n")
	sb.WriteString("        :for lx from=0 to=([:len $lmsg] - 1) do={\n")
	sb.WriteString("          :local lch [:pick $lmsg $lx ($lx + 1)]\n")
	sb.WriteString("          :if ($lch = \"|\" || $lch = \";\") do={ :set lch \" \" }\n")
	sb.WriteString("          :set lmsgc ($lmsgc . $lch)\n")
	sb.WriteString("        }\n")
	sb.WriteString("      }\n")
	sb.WriteString(`      :set rdata ($rdata . [:tostr [/log get $le time]] . "|" . [:tostr [/log get $le topics]] . "|" . $lmsgc . ";")` + "\n")
	sb.WriteString("    }\n")
	sb.WriteString("    :set li ($li + 1)\n")
	sb.WriteString("  }\n")
	sb.WriteString("} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.fetchResultData(cmd.ID, okVar))
	return sb.String()
}

// buildReadScheduler — F10 : /system scheduler → name|interval|disabled|onevent.
// Le on-event est assaini (« | », « ; », « : » et retours à la ligne → espaces).
func (b Builder) buildReadScheduler(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n:local rdata \"\"\n")
	sb.WriteString(":do {\n  :local rn 0\n")
	sb.WriteString("  :foreach se in=[/system scheduler find] do={\n")
	sb.WriteString("    :if ($rn < 100) do={\n")
	sb.WriteString("      :local sev [:tostr [/system scheduler get $se on-event]]\n")
	sb.WriteString("      :local sevc \"\"\n")
	sb.WriteString("      :if ([:len $sev] > 0) do={\n")
	sb.WriteString("        :for sx from=0 to=([:len $sev] - 1) do={\n")
	sb.WriteString("          :local sch [:pick $sev $sx ($sx + 1)]\n")
	sb.WriteString("          :if ($sch = \"|\" || $sch = \";\" || $sch = \":\") do={ :set sch \" \" }\n")
	sb.WriteString("          :set sevc ($sevc . $sch)\n")
	sb.WriteString("        }\n")
	sb.WriteString("      }\n")
	sb.WriteString(`      :set rdata ($rdata . [:tostr [/system scheduler get $se name]] . "|" . [:tostr [/system scheduler get $se interval]] . "|" . [:tostr [/system scheduler get $se disabled]] . "|" . $sevc . ";")` + "\n")
	sb.WriteString("      :set rn ($rn + 1)\n")
	sb.WriteString("    }\n  }\n")
	sb.WriteString("} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.fetchResultData(cmd.ID, okVar))
	return sb.String()
}

// intervalPattern — durée RouterOS simple (« 45s », « 5m », « 1h », « 2d », « 1w »).
var intervalPattern = regexp.MustCompile(`^[0-9]+[smhdw]$`)

// buildSchedulerAdd — F10 : /system scheduler add name/interval/on-event.
// Le handler a validé le format ; le builder reste défensif (interval hors
// format → 45s, nom assaini via SanitizeName).
func (b Builder) buildSchedulerAdd(cmd model.Command) string {
	name := SanitizeName(plStr(cmd.Payload, "name"))
	interval := strings.TrimSpace(plStr(cmd.Payload, "interval"))
	if !intervalPattern.MatchString(interval) {
		interval = "45s"
	}
	onEvent := plStr(cmd.Payload, "onEvent")
	okVar := "ok" + idSafe(cmd.ID)
	line := `/system scheduler add name="` + rosEscape(name) + `" interval="` + rosEscape(interval) +
		`" on-event="` + rosEscape(onEvent) + `"`
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { " + line + " } on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// buildSchedulerSet — F10 : set [find name=…] disabled=yes|no.
func (b Builder) buildSchedulerSet(cmd model.Command) string {
	name := SanitizeName(plStr(cmd.Payload, "name"))
	set := `/system scheduler set [find name="` + rosEscape(name) + `"]`
	if plBool(cmd.Payload, "disabled") {
		set += " disabled=yes"
	} else {
		set += " disabled=no"
	}
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { " + set + " } on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// buildSchedulerRemove — F10 : remove [find name=…].
func (b Builder) buildSchedulerRemove(cmd model.Command) string {
	name := SanitizeName(plStr(cmd.Payload, "name"))
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { /system scheduler remove [find name=\"" + rosEscape(name) + "\"] } on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// buildPower — F10 : rapport ok IMMÉDIATEMENT (le /tool fetch est bloquant :
// le résultat part AVANT l'exécution) puis /system reboot (ou shutdown).
// La commande est rapportée « done » au cloud, qui enfile un read_state : le
// routeur re-synchronisera son état au check-in suivant son redémarrage.
func (b Builder) buildPower(cmd model.Command, action string) string {
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(b.reportLine(cmd.ID, true, map[string]string{"action": action}) + "\n")
	sb.WriteString(":delay 1s\n")
	sb.WriteString(":do { /system " + action + " } on-error={ :log warning \"mikcloud: " + action + " impossible\" }\n")
	return sb.String()
}

// ---------------------------------------------------------------------------
// Accès typés au payload (map[string]any)
// ---------------------------------------------------------------------------

func plStr(p map[string]any, k string) string {
	if p == nil {
		return ""
	}
	if v, ok := p[k].(string); ok {
		return v
	}
	return ""
}

func plBool(p map[string]any, k string) bool {
	if v, ok := p[k].(bool); ok {
		return v
	}
	return false
}

func plHas(p map[string]any, k string) bool {
	_, ok := p[k]
	return ok
}

// plInt64 lit un entier du payload (JSON → float64, mémoire → int / int64).
func plInt64(p map[string]any, k string) int64 {
	switch v := p[k].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	}
	return 0
}

// ProfileRef — référence compacte d'un profil pour les scripts.
type ProfileRef struct {
	Name              string
	RateLimit         string
	SessionTimeoutMin int
	SharedUsers       int
}

func plProfile(p map[string]any, k string) ProfileRef {
	m, ok := p[k].(map[string]any)
	if !ok {
		return ProfileRef{}
	}
	ref := ProfileRef{Name: plStr(m, "name"), RateLimit: plStr(m, "rateLimit")}
	if v, ok := m["sessionTimeoutMin"].(float64); ok {
		ref.SessionTimeoutMin = int(v)
	}
	if v, ok := m["sharedUsers"].(float64); ok {
		ref.SharedUsers = int(v)
	}
	return ref
}

// VoucherRef — ligne de voucher pour un batch.
type VoucherRef struct {
	Name     string
	Password string
}

func plUserList(p map[string]any, k string) []VoucherRef {
	// Le payload peut venir de la mémoire (types concrets Go : []map[string]any)
	// ou d'une relecture JSON ([]any) — accepter les deux formes.
	collect := func(items []any) []VoucherRef {
		out := make([]VoucherRef, 0, len(items))
		for _, it := range items {
			if m, ok := it.(map[string]any); ok {
				out = append(out, VoucherRef{Name: plStr(m, "name"), Password: plStr(m, "password")})
			}
		}
		return out
	}
	if raw, ok := p[k].([]any); ok {
		return collect(raw)
	}
	if raw2, ok := p[k].([]map[string]any); ok {
		items := make([]any, 0, len(raw2))
		for _, it := range raw2 {
			items = append(items, it)
		}
		return collect(items)
	}
	return nil
}

func plStrList(p map[string]any, k string) []string {
	// Même tolérance que plUserList : []any (JSON) ou []string (mémoire).
	if raw, ok := p[k].([]any); ok {
		out := make([]string, 0, len(raw))
		for _, it := range raw {
			if s, ok := it.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	if raw2, ok := p[k].([]string); ok {
		out := make([]string, len(raw2))
		copy(out, raw2)
		return out
	}
	return nil
}
