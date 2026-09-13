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
	"mikcloud/hotspot-api/internal/model"
	"regexp"
	"strconv"
	"strings"
)

// ScriptFilename — nom du fichier de commandes sur le routeur.
const ScriptFilename = "mikcloud-cmd.rsc"

// SchedulerName — nom du scheduler créé sur le routeur.

// SchedulerName — nom du scheduler créé sur le routeur.
const SchedulerName = "mikcloud-agent"

// WatcherName — N°77 — nom du scheduler VEILLEUR d'invités créé sur le routeur.

// WatcherName — N°77 — nom du scheduler VEILLEUR d'invités créé sur le routeur.
const WatcherName = "mikcloud-watch"

// WatcherFilename — N°77 — fichier de commandes propre au veilleur (jamais le
// même dst-path que le scheduler principal : deux check-ins concurrents ne
// peuvent pas s'écraser mutuellement le fichier — le veilleur tire toutes les
// 20 s pendant qu'un invité est NON autorisé, le principal suit son pas 45 s /
// 180 s ; leurs fenêtres d'exécution se chevaucheront forcément).

// WatcherFilename — N°77 — fichier de commandes propre au veilleur (jamais le
// même dst-path que le scheduler principal : deux check-ins concurrents ne
// peuvent pas s'écraser mutuellement le fichier — le veilleur tire toutes les
// 20 s pendant qu'un invité est NON autorisé, le principal suit son pas 45 s /
// 180 s ; leurs fenêtres d'exécution se chevaucheront forcément).
const WatcherFilename = "mikcloud-watch.rsc"

// WatcherIntervalSec — N°77 — pas du veilleur. 20 s : un invité qui vient de
// se connecter (hôte hotspot non autorisé = l'appareil est SUR le portail)
// obtient son check-in en ≤ 20 s au lieu d'attendre le réveil du scheduler
// principal (jusqu'à 180 s de veille N°75 — constat production : claim gratuit
// passé de ~45 s à 1-2 min, découragement des invités en restaurant/maquis).
// Coût : STRICTEMENT nul à l'arrêt (le tick compte les hôtes non autorisés et
// s'arrête là — aucun octet émis), ~400 o par tick SEULEMENT pendant qu'un
// invité est réellement sur le portail (~1,2 Ko/min d'attention). La capacité
// « plan gratuit » gagnée par la veille N°75 est préservée intégralement.

// WatcherIntervalSec — N°77 — pas du veilleur. 20 s : un invité qui vient de
// se connecter (hôte hotspot non autorisé = l'appareil est SUR le portail)
// obtient son check-in en ≤ 20 s au lieu d'attendre le réveil du scheduler
// principal (jusqu'à 180 s de veille N°75 — constat production : claim gratuit
// passé de ~45 s à 1-2 min, découragement des invités en restaurant/maquis).
// Coût : STRICTEMENT nul à l'arrêt (le tick compte les hôtes non autorisés et
// s'arrête là — aucun octet émis), ~400 o par tick SEULEMENT pendant qu'un
// invité est réellement sur le portail (~1,2 Ko/min d'attention). La capacité
// « plan gratuit » gagnée par la veille N°75 est préservée intégralement.
const WatcherIntervalSec = 20

// WalledGardenMarker — commentaire des règles walled-garden posées par
// MikCloud (N°29 — runbook N°27-D automatisé). L'idempotence s'appuie dessus :
// seules les règles portant ce marqueur sont remplacées — les règles
// personnelles du gérant sont préservées.

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

// HashToken — SHA-256 hexadécimal du token (seul stockage côté cloud).
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// Preview retourne les 4 premiers caractères du token pour l'affichage.

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
// Sécurité (audit P0 #14) : le « $ » est échappé lui aussi. Sans lui, une
// valeur utilisateur contenant « $… » (mot de passe, commentaire, nom…)
// déclenchait l'interpolation de variables RouterOS à l'exécution du script —
// injection d'expression. rosScriptValue (valeurs de propriété) la traite
// déjà ; strings.NewReplacer fait une passe unique, donc les remplacements
// ne se ré-échappent pas entre eux.

// ---------------------------------------------------------------------------
// Échappement / assainissement RouterOS
// ---------------------------------------------------------------------------

// rosEscape échappe une valeur pour une chaîne RouterOS entre guillemets.
// Sécurité (audit P0 #14) : le « $ » est échappé lui aussi. Sans lui, une
// valeur utilisateur contenant « $… » (mot de passe, commentaire, nom…)
// déclenchait l'interpolation de variables RouterOS à l'exécution du script —
// injection d'expression. rosScriptValue (valeurs de propriété) la traite
// déjà ; strings.NewReplacer fait une passe unique, donc les remplacements
// ne se ré-échappent pas entre eux.
func rosEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "\n", " ", "\r", " ", "\t", " ")
	return r.Replace(s)
}

// VersionAtLeast — true si la version RouterOS déclarée est ≥ major.minor.
// Sécurité (audit P0 #5) : les agents valident strictement le TLS du cloud,
// ce qui exige RouterOS ≥ 7.19 (première version embarquant les certificats
// racine nécessaires à la validation Let's Encrypt). Tolère les suffixes
// d'édition (« 7.19.6 (stable) », « 7.20beta4 », « 7.19rc1 ») : seuls les
// deux premiers nombres sont comparés. Une version non analysable (vide,
// illisible) renvoie true — on ne bloque jamais un parc legacy dont la
// version n'a pas encore été remontée ; c'est le garde du REGISTER (la
// version y est toujours envoyée) qui fait le tri à l'installation.

// VersionAtLeast — true si la version RouterOS déclarée est ≥ major.minor.
// Sécurité (audit P0 #5) : les agents valident strictement le TLS du cloud,
// ce qui exige RouterOS ≥ 7.19 (première version embarquant les certificats
// racine nécessaires à la validation Let's Encrypt). Tolère les suffixes
// d'édition (« 7.19.6 (stable) », « 7.20beta4 », « 7.19rc1 ») : seuls les
// deux premiers nombres sont comparés. Une version non analysable (vide,
// illisible) renvoie true — on ne bloque jamais un parc legacy dont la
// version n'a pas encore été remontée ; c'est le garde du REGISTER (la
// version y est toujours envoyée) qui fait le tri à l'installation.
func VersionAtLeast(v string, major, minor int) bool {
	m := versionNumRe.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return true
	}
	M, _ := strconv.Atoi(m[1])
	N, _ := strconv.Atoi(m[2])
	return M > major || (M == major && N >= minor)
}

var versionNumRe = regexp.MustCompile(`^(\d+)\.(\d+)`)

// SanitizeName assainit un nom (utilisateur ou profil) pour le routeur :
// caractères [A-Za-z0-9._-] conservés, le reste devient "-", 48 caractères max.

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
//
// Le script est collé dans la console (Terminal Winbox), pas importé comme
// fichier .rsc : le parseur console y est nettement plus fragile. Règles
// respectées ici (issues d'incidents réels) :
//   - les :local top-level meurent entre deux commandes collées → TOUT le
//     corps est enveloppé dans UN SEUL bloc :do { … } exécuté comme une
//     commande unique (les :local y survivent) ;
//   - les corps de bloc one-line « do={ :set x y } » sont rejetés par le
//     parseur console de certaines versions (« syntax error ») → tous les
//     corps sont multi-lignes ;
//   - « :set v false » (booléen nu) est fragile → drapeaux en "yes"/"no" ;
//   - [/system device-mode get …] renvoie un booléen OU une chaîne
//     ("yes"/"no") selon la version : comparer explicitement à false/"no"/
//     "false" (un « ! » sur "no" — chaîne non vide — serait faux) ;
//   - « output=none » ne sauvegarde PAS le fichier téléchargé (doc MikroTik
//     : « none - do not store downloaded data ») → le check-in utilise
//     dst-path seul (output par défaut = file), sinon l'import échoue ;
//   - les :global RouterOS ne survivent pas à coup sûr à un reboot → URL et
//     token sont INLINÉS dans l'on-event : le scheduler est auto-suffisant.

// ---------------------------------------------------------------------------
// Scripts
// ---------------------------------------------------------------------------

// InstallScript — le script de provisionning complet (1 collage dans Winbox).
//
// Le script est collé dans la console (Terminal Winbox), pas importé comme
// fichier .rsc : le parseur console y est nettement plus fragile. Règles
// respectées ici (issues d'incidents réels) :
//   - les :local top-level meurent entre deux commandes collées → TOUT le
//     corps est enveloppé dans UN SEUL bloc :do { … } exécuté comme une
//     commande unique (les :local y survivent) ;
//   - les corps de bloc one-line « do={ :set x y } » sont rejetés par le
//     parseur console de certaines versions (« syntax error ») → tous les
//     corps sont multi-lignes ;
//   - « :set v false » (booléen nu) est fragile → drapeaux en "yes"/"no" ;
//   - [/system device-mode get …] renvoie un booléen OU une chaîne
//     ("yes"/"no") selon la version : comparer explicitement à false/"no"/
//     "false" (un « ! » sur "no" — chaîne non vide — serait faux) ;
//   - « output=none » ne sauvegarde PAS le fichier téléchargé (doc MikroTik
//     : « none - do not store downloaded data ») → le check-in utilise
//     dst-path seul (output par défaut = file), sinon l'import échoue ;
//   - les :global RouterOS ne survivent pas à coup sûr à un reboot → URL et
//     token sont INLINÉS dans l'on-event : le scheduler est auto-suffisant.
func InstallScript(baseURL, token, routerName string, wgDomains ...string) string {
	// Le nom n'apparaît que dans un commentaire .rsc : interdire tout retour à
	// la ligne ou caractère de contrôle (anti-injection dans le fichier).
	safeName := strings.Map(func(c rune) rune {
		if c == '\n' || c == '\r' || c == '\t' || c < 0x20 {
			return ' '
		}
		return c
	}, routerName)
	urlEsc := rosEscape(strings.TrimRight(baseURL, "/"))
	tokEsc := rosEscape(token)
	return `# ============================================================
# MikCloud — Installation agent  (routeur: ` + safeName + `)
# Coller CE fichier ENTIER dans Terminal (Winbox) — 1 seule fois.
# Le script s'exécute comme UN SEUL bloc : ne pas le lancer ligne par ligne.
# ============================================================
# NB : les fetch valident STRICTEMENT le certificat TLS du cloud (audit P0
# #5 : aucun repli check-certificate=no — un tel repli autorisait un MITM
# à délivrer un script de commandes arbitraire au routeur). RouterOS 7.19
# ou plus récent est REQUIS : les versions antérieures n'embarquent pas les
# certificats racine nécessaires à la validation Let's Encrypt. Le cloud
# refuse d'inscrire un agent qui déclare une version inférieure.
#
# ROUTEURS NEUFS (RouterOS 7.17+) : d'usine, fetch / scheduler / hotspot
# sont DÉSACTIVÉS par le « device-mode » restreint (protection anti-malware).
# Le pré-vol ci-dessous le détecte et affiche la marche à suivre au lieu
# d'échouer avec « not allowed by device-mode ».
# État vérifiable à tout moment : /system device-mode print
# ============================================================
:global mikcloudToken "` + tokEsc + `"
:global mikcloudUrl   "` + urlEsc + `"

:do {

  :local mikReady "yes"

  :do {
    :local dmS [/system device-mode get scheduler]
    :local dmF [/system device-mode get fetch]
    :if ($dmS = false || $dmS = "no" || $dmS = "false") do={
      :set mikReady "no"
    }
    :if ($dmF = false || $dmF = "no" || $dmF = "false") do={
      :set mikReady "no"
    }
    :do {
      :local dmH [/system device-mode get hotspot]
      :if ($dmH = false || $dmH = "no" || $dmH = "false") do={
        :set mikReady "no"
      }
    } on-error={}
  } on-error={}

  :if ($mikReady = "no") do={
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

    :local ident [/system identity get name]
    :local mod [/system resource get board-name]
    :local ver [/system resource get version]
    :local up [/system resource get uptime]

    :do {
      /tool fetch url="` + urlEsc + `/agent/register?token=` + tokEsc + `" http-method=post http-data=("identity=" . [:tostr $ident] . "&model=" . [:tostr $mod] . "&version=" . [:tostr $ver] . "&uptime=" . [:tostr $up]) output=none
    } on-error={
      :log error "MikCloud: inscription impossible (reseau ou TLS ? RouterOS 7.19+ requis)"
    }

    :do {
      /system scheduler remove [find name="` + SchedulerName + `"]
    } on-error={}

    :local mikAdded "no"
    :do {
      /system scheduler add name="` + SchedulerName + `" interval=45s start-time=startup on-event={
        :local fetched "yes"
        :do {
          /tool fetch url="` + urlEsc + `/agent/cmd?token=` + tokEsc + `" dst-path="` + ScriptFilename + `"
        } on-error={
          :set fetched "no"
          :log warning "MikCloud agent: check-in echoue (reseau ou TLS ? RouterOS 7.19+ requis)"
        }
        :if ($fetched = "yes") do={
          :delay 2s
          /import file-name="` + ScriptFilename + `"
        }
      }
      :set mikAdded "yes"
    } on-error={
      :put "MIKCLOUD : echec de la creation du scheduler."
      :do {
        :put (" scheduler flag lu a l'instant : " . [/system device-mode get scheduler])
      } on-error={}
      :put "Si l'erreur ci-dessus est 'not allowed by device-mode', executez :"
      :put "  /system/device-mode/update scheduler=yes fetch=yes hotspot=yes"
      :put "puis confirmez physiquement (bouton reset ou coupure d'alimentation)."
      :log error "MikCloud: creation scheduler echouee (device-mode ?)"
    }

    :if ($mikAdded = "yes") do={
      :put "MIKCLOUD : agent installe. Prochaine connexion au cloud dans 45 s max."
      :log info "MikCloud: agent installe, check-in dans 45s"
    }

    :do {
      /system scheduler remove [find name="` + WatcherName + `"]
    } on-error={}

    :do {
      /system scheduler add name="` + WatcherName + `" interval=` + strconv.Itoa(WatcherIntervalSec) + `s start-time=startup on-event="` + rosEscape(watcherOnEvent(urlEsc, tokEsc)) + `"
      :put "MIKCLOUD : veilleur d'invites installe (check-in 20 s quand un invite est sur le portail)."
      :log info "MikCloud: veilleur d'invites installe"
    } on-error={
      :put "MIKCLOUD : veilleur d'invites non installe (le claim reste servi au pas du scheduler principal)."
    }

` + walledGardenInstallBlock(wgDomains) + `  }
} on-error={
  :log error "MikCloud: erreur pendant l'installation de l'agent"
}
`
}

// NopScript — réponse quand il n'y a rien à faire (l'import ne fait rien).

// NopScript — réponse quand il n'y a rien à faire (l'import ne fait rien).
func NopScript() string { return "# mikcloud nop\n" }

// ImportChunkSize — nombre d'utilisateurs hotspot lus par commande
// import_hotspot. L'import est paginé : si le routeur a plus d'utilisateurs,
// le résultat du chunk en file le suivant (voir applyImportHotspot). La taille
// garde le corps POST (http-data) loin de la limite RouterOS (~64 Ko).

// Builder construit les scripts de commandes pour un routeur donné.
type Builder struct {
	BaseURL string
	Token   string
}

// ScriptFor retourne le script .rsc d'une commande (erreur si kind inconnu).

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
	case model.CmdReadResources:
		return b.buildReadResources(cmd), nil
	case model.CmdSchedulerAdd:
		return b.buildSchedulerAdd(cmd), nil
	case model.CmdSchedulerSet:
		return b.buildSchedulerSet(cmd), nil
	case model.CmdSchedulerRemove:
		return b.buildSchedulerRemove(cmd), nil
	case model.CmdWatcherEnsure:
		return b.buildWatcherEnsure(cmd), nil
	case model.CmdSafeWifi:
		return b.buildSafeWifi(cmd), nil
	case model.CmdShield:
		return b.buildShield(cmd), nil
	case model.CmdFamilyGuard:
		return b.buildFamilyGuard(cmd), nil
	case model.CmdAntiVpn:
		return b.buildAntiVpn(cmd), nil
	case model.CmdReboot:
		return b.buildPower(cmd, "reboot"), nil
	case model.CmdShutdown:
		return b.buildPower(cmd, "shutdown"), nil
	case model.CmdImportHotspot:
		return b.buildImportHotspot(cmd), nil
	case model.CmdProfileSet:
		return b.buildProfileSet(cmd), nil
	case model.CmdWalledGarden:
		return b.buildWalledGarden(cmd), nil
	case model.CmdHotspotFiles:
		return b.buildHotspotFiles(cmd), nil
	default:
		return "", fmt.Errorf("kind de commande inconnu : %s", cmd.Kind)
	}
}

// resultLines — les deux branches de rapport (ok / error) d'une commande.

// resultLines — les deux branches de rapport (ok / error) d'une commande.
func (b Builder) resultLines(cmdID string, okVar string, extraOK map[string]string) string {
	ok := b.reportLine(cmdID, true, extraOK)
	ko := b.reportLine(cmdID, false, map[string]string{"message": "echec sur le routeur"})
	return ":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n"
}

// reportLine — le /tool fetch qui rapporte le résultat d'une commande.

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
		`" http-method=post http-data=("` + data + `") output=none`
}

// urlEscape — encodage minimal sûr pour les valeurs d'URL (http-data).

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

// onLoginLockScript — script on-login du verrou « 1er appareil » (v2,
// anti-partage). À la première connexion d'un utilisateur du profil, l'adresse
// MAC de l'appareil ($"caller-id") est mémorisée dans le commentaire routeur
// sous la marque « mikcloud_lock: » (append : le commentaire de traçabilité
// MikCloud, ex. lot de vouchers, est préservé). Aux connexions suivantes, tout
// autre appareil est déconnecté immédiatement. Le commentaire routeur n'est
// JAMAIS relu par le cloud (import = name|profile|disabled) : la liaison MAC
// reste un état local au routeur. Une seule ligne : les valeurs de propriété
// RouterOS sont embarquées entre guillemets (voir rosScriptValue).

// header — commentaire d'audit d'une commande (parsé aussi par le simulateur).
func header(cmd model.Command) string {
	return "# mikcloud cmd " + cmd.ID + " " + cmd.Kind + "\n"
}

// ---------------------------------------------------------------------------
// Builders par kind
// ---------------------------------------------------------------------------

// ReadChunkSize — N°76 — nombre d'utilisateurs hotspot rapportés par commande
// read_state. La réconciliation des grands parcs est PAGINÉE : le script
// rapporte une fenêtre [start, start+count) du parc + le total exact, et le
// cloud enchaîne les chunks jusqu'au rapport complet (pattern import_hotspot,
// éprouvé en production). La taille garde le corps POST (http-data) loin de la
// limite RouterOS (~64 Ko) : 500 entrées « name|profile|disabled; » ≈ 20 Ko.

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
		`" http-method=post http-data=("cmd=` + cmdID + `&status=ok&data=". $rdata) output=none`
	ko := b.reportLine(cmdID, false, map[string]string{"message": "lecture impossible sur le routeur"})
	return ":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n"
}

// buildPing — F8 : /ping count=4 as-value → sent/received/minMs/avgMs/maxMs.
// Robustesse vieux ROS : la conversion time→ms essaie la division arithmétique
// (ROS7) puis retombe sur l'analyse de la chaîne « Nms » ; chaque étape est
// isolée dans un :do on-error. Une perte (timeout) ne compte pas comme reçue.

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
		`&status=ok&sent=". $psent ."&received=". $precv ."&minMs=". $pmin ."&avgMs=". $pavg ."&maxMs=". $pmax) output=none`
	ko := b.reportLine(cmd.ID, false, map[string]string{"message": "ping impossible sur le routeur"})
	sb.WriteString(":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n")
	return sb.String()
}

// buildIpbindingAdd — F7 : /ip hotspot ip-binding add (mac requise, type
// bypassed|blocked, address/comment optionnels).

// urlPathEscape — échappe un path composé de segments séparés par « / » en
// préservant les « / » (contrairement à urlEscape qui les encode en %2F).
// Chaque segment est échappé via urlEscape ; les « / » sont remis après.
// Sécurité : le path a déjà été validé par sanitizePortalPath (pas de « .. »,
// pas de « / » initial, charset sûr) — cette fonction n'est qu'un encodage
// d'URL, pas une re-validation.
func urlPathEscape(p string) string {
	if p == "" {
		return ""
	}
	segs := strings.Split(p, "/")
	out := make([]string, len(segs))
	for i, s := range segs {
		out[i] = urlEscape(s)
	}
	return strings.Join(out, "/")
}

// ---------------------------------------------------------------------------
// Accès typés au payload (map[string]any)
// ---------------------------------------------------------------------------

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
