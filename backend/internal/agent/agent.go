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
# ============================================================
:global mikcloudToken "` + rosEscape(token) + `"
:global mikcloudUrl   "` + strings.TrimRight(baseURL, "/") + `"

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
/system scheduler remove [find name="` + SchedulerName + `"]

# --- 3) L'agent permanent : check-in toutes les 45 s (survit aux reboots) ---
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

:log info "MikCloud: agent installe, check-in dans 45s"
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

func (b Builder) buildReadState(cmd model.Command) string {
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(`:local rsres [/system resource get]
:local rver [:tostr ($rsres->"version")]
:local rup [:tostr ($rsres->"uptime")]
:local rcpu [:tostr ($rsres->"cpu-load")]
:local rmem [:tostr ($rsres->"free-memory")]
:local rmemb [:tostr ($rsres->"total-memory")]
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
`)
	sb.WriteString(`/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + cmd.ID +
		`&status=ok&version=". $rver ."&uptime=". $rup ."&cpu=". $rcpu ."&freemem=". $rmem ."&totalmem=". $rmemb ."&users=". $rusr ."&sessions=". $rsess) check-certificate=no output=none` + "\n")
	return sb.String()
}

func (b Builder) buildUserAdd(cmd model.Command) string {
	name := SanitizeName(plStr(cmd.Payload, "name"))
	pass := plStr(cmd.Payload, "password")
	prof := plProfile(cmd.Payload, "profile")
	comment := plStr(cmd.Payload, "comment")
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
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { " + profileEnsureLine(prof) + " } on-error={ :log info \"mikcloud: profil deja present\" }\n")
	for _, u := range users {
		line := `/ip hotspot user add name="` + rosEscape(SanitizeName(u.Name)) + `" password="` + rosEscape(u.Password) +
			`" profile="` + rosEscape(prof.Name) + `"`
		if batch != "" {
			line += ` comment="mikcloud:` + rosEscape(batch) + `"`
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
