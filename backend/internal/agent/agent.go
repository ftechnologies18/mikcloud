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
	"strconv"
	"strings"

	"mikcloud/hotspot-api/internal/model"
)

// ScriptFilename — nom du fichier de commandes sur le routeur.
const ScriptFilename = "mikcloud-cmd.rsc"

// SchedulerName — nom du scheduler créé sur le routeur.
const SchedulerName = "mikcloud-agent"

// WatcherName — N°77 — nom du scheduler VEILLEUR d'invités créé sur le routeur.
const WatcherName = "mikcloud-watch"

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
const WatcherIntervalSec = 20

// WalledGardenMarker — commentaire des règles walled-garden posées par
// MikCloud (N°29 — runbook N°27-D automatisé). L'idempotence s'appuie dessus :
// seules les règles portant ce marqueur sont remplacées — les règles
// personnelles du gérant sont préservées.
const WalledGardenMarker = "mikcloud-wg"

// SafeWifiMarker — commentaire des règles NAT posées par SafeWiFi (N°80 —
// protection DNS du WiFi public). Même contrat d'idempotence : seules les
// règles portant ce marqueur sont retirées puis recréées — les règles du
// gérant sont préservées.
const SafeWifiMarker = "mikcloud-safewifi"

// Résolveurs filtrants publics (anycast, gratuits) par niveau N°80 :
//
//	threats : Quad9 — malwares, phishing, arnaques ;
//	family  : AdGuard Family — + contenus adultes, publicités.
const (
	SafeWifiDNSQuad9         = "9.9.9.9"
	SafeWifiDNSAdGuardFamily = "94.140.14.15"
)

// SafeWifiResolver — adresse du résolveur filtrant d'un niveau. Toute valeur
// inconnue retombe sur Quad9 : en cas de doute, le filtrage minimal protège.
func SafeWifiResolver(level string) string {
	if level == model.SafeWifiFamily {
		return SafeWifiDNSAdGuardFamily
	}
	return SafeWifiDNSQuad9
}

// SafeWifiLevelFromPayload — niveau d'une commande safewifi, normalisé
// ("" ou valeur inconnue → off : le script ne pose alors aucune règle).
func SafeWifiLevelFromPayload(p map[string]any) string {
	if s, _ := p["level"].(string); model.ValidSafeWifiLevel(s) {
		return s
	}
	return model.SafeWifiOff
}

// ShieldMarker — commentaire des règles FILTER posées par Shield (N°81 —
// bouclier réseau du WiFi public). Même contrat d'idempotence : seules
// les règles portant ce marqueur sont retirées puis recréées — les
// règles du gérant sont préservées.
const ShieldMarker = "mikcloud-shield"

// N°81 — ports bloqués pour les clients du WiFi public :
//   - administration du routeur (tcp) : ftp, ssh, telnet, winbox, api ;
//   - api en udp (8728/8729 répondent aussi en udp) ;
//   - propagation de malwares (SMB/NetBIOS, tcp et udp) — les vecteurs
//     de mouvement latéral classiques des réseaux partagés.
const (
	ShieldAdminTCPPorts   = "21,22,23,8291,8728,8729"
	ShieldAdminUDPPorts   = "8728,8729"
	ShieldMalwareTCPPorts = "135,137,138,139,445"
	ShieldMalwareUDPPorts = "137,138,139"
)

// ShieldLevelFromPayload — niveau d'une commande shield, normalisé
// ("" ou valeur inconnue → off : le script ne pose alors aucune règle).
func ShieldLevelFromPayload(p map[string]any) string {
	if s, _ := p["level"].(string); model.ValidShieldLevel(s) {
		return s
	}
	return model.ShieldOff
}

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
func NopScript() string { return "# mikcloud nop\n" }

// ImportChunkSize — nombre d'utilisateurs hotspot lus par commande
// import_hotspot. L'import est paginé : si le routeur a plus d'utilisateurs,
// le résultat du chunk en file le suivant (voir applyImportHotspot). La taille
// garde le corps POST (http-data) loin de la limite RouterOS (~64 Ko).
const ImportChunkSize = 300

// ImportProfilesMax — nombre de profils hotspot lus par commande (les profils
// sont peu nombreux en pratique ; 60 couvre très largement).
const ImportProfilesMax = 60

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
		`" http-method=post http-data=("` + data + `") output=none`
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

// onLoginLockScript — script on-login du verrou « 1er appareil » (v2,
// anti-partage). À la première connexion d'un utilisateur du profil, l'adresse
// MAC de l'appareil ($"caller-id") est mémorisée dans le commentaire routeur
// sous la marque « mikcloud_lock: » (append : le commentaire de traçabilité
// MikCloud, ex. lot de vouchers, est préservé). Aux connexions suivantes, tout
// autre appareil est déconnecté immédiatement. Le commentaire routeur n'est
// JAMAIS relu par le cloud (import = name|profile|disabled) : la liaison MAC
// reste un état local au routeur. Une seule ligne : les valeurs de propriété
// RouterOS sont embarquées entre guillemets (voir rosScriptValue).
const onLoginLockScript = `:do {:local m $"caller-id";:local u $user;:if ([:len $m] > 0) do={:local e [/ip hotspot user find name=$u];:if ([:len $e] > 0) do={:local c [:tostr [/ip hotspot user get $e comment]];:local i [:find $c "mikcloud_lock:"];:if ([:typeof $i] = "nil") do={:if ([:len $c] = 0) do={/ip hotspot user set $e comment=("mikcloud_lock:" . $m)} else={/ip hotspot user set $e comment=($c . " mikcloud_lock:" . $m)}} else={:local lm [:pick $c ($i + 14) [:len $c]];:if ($lm != $m) do={/ip hotspot active remove [find user=$u]}}}}} on-error={ :log info "mikcloud: liaison mac ignoree" }`

// rosScriptValue — échappe un SCRIPT RouterOS pour l'embarquer dans une valeur
// de propriété entre guillemets (ex. on-login) : le script interne doit
// survivre au parsing de la ligne externe ($, " et \ protégés).
func rosScriptValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `$`, `\$`)
	return s
}

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
	if p.LockFirstDevice {
		s += ` on-login="` + rosScriptValue(onLoginLockScript) + `"`
	}
	return s
}

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
	if p.LockFirstDevice {
		s += ` on-login="` + rosScriptValue(onLoginLockScript) + `"`
	} else {
		s += ` on-login=""`
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
	}
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(profileSetLine(name, p))
	sb.WriteString(profileUserLimitLine(p))
	sb.WriteString(b.reportLine(cmd.ID, true, nil) + "\n")
	return sb.String()
}

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
const ReadChunkSize = 500

// MaxReadChunks — borne ABSOLUE de chunks par cycle read_state (20 × 500 =
// 10 000 users). Au-delà, le cloud n'enchaîne plus : le rapport ne sera jamais
// complet, il reste honnête (aucune déduction sur les absents — comportement
// trunc N°75) plutôt que de mentir sur un parc hors d'atteinte du protocole.
const MaxReadChunks = 20

// watcherOnEvent — N°77 — corps UNE LIGNE (séparateurs « ; », formes valides
// en import .rsc — même sérialisation que buildSchedulerAdd) du on-event du
// veilleur d'invités. Reçoit l'URL et le token DÉJÀ échappés pour le niveau de
// citation INTERNE (le corps contient ses propres chaînes quotées) ; l'appelant
// ré-échappe le corps entier pour le niveau on-event="…" (double échappement
// assumé et correct : chaque niveau de citation décode le sien).
//
// Sémantique : à chaque tick (20 s), compter les hôtes hotspot NON autorisés —
// un hôte non autorisé = un appareil connecté qui n'a PAS encore de session :
// c'est exactement la fenêtre « invité sur le portail, claim imminent ou en
// cours ». Si > 0 → check-in complet (fichier PROPRE au veilleur, jamais le
// dst-path du scheduler principal : deux fetchs concurrents ne peuvent pas
// s'écraser le fichier). Si 0 → RIEN (aucun octet émis — la veille N°75 garde
// ses 6 Mo/mois). Exclusions : bypassed (binding MAC permanent — sinon le
// veilleur tirerait 24 h/24 pour un appareil du gérant) et blocked (banni du
// login : aucun claim ne viendra de lui).
func watcherOnEvent(urlEsc, tokEsc string) string {
	return ":do { " +
		":local mkgw 0; " +
		":do { :set mkgw [/ip hotspot host print count-only where !authorized && !bypassed && !blocked] } on-error={ :set mkgw 0 }; " +
		":if ($mkgw > 0) do={ " +
		":local mkwf \"yes\"; " +
		":do { /tool fetch url=\"" + urlEsc + "/agent/cmd?token=" + tokEsc + "\" dst-path=\"" + WatcherFilename + "\" } on-error={ :set mkwf \"no\" }; " +
		":if ($mkwf = \"yes\") do={ :delay 2s; /import file-name=\"" + WatcherFilename + "\" } " +
		"} " +
		"} on-error={}"
}

// buildWatcherEnsure — N°77 — déploie (ou redéploie) le veilleur d'invités sur
// un routeur agent : remove-then-add idempotent, rapporté comme toute commande.
// Servi aux routeurs dont WatcherOK est faux (ensureWatcherLocked au check-in —
// pattern walled_garden : convergence automatique du parc existant en UN
// check-in, re-file tant que le retour « ok » n'est pas arrivé).
func (b Builder) buildWatcherEnsure(cmd model.Command) string {
	urlEsc := rosEscape(strings.TrimRight(b.BaseURL, "/"))
	tokEsc := rosEscape(b.Token)
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do {\n  /system scheduler remove [find name=\"" + WatcherName + "\"]\n} on-error={}\n")
	sb.WriteString(":do {\n  /system scheduler add name=\"" + WatcherName + "\" interval=" + strconv.Itoa(WatcherIntervalSec) + "s start-time=startup on-event=\"" +
		rosEscape(watcherOnEvent(urlEsc, tokEsc)) + "\"\n} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, nil))
	return sb.String()
}

// buildSafeWifi — N°80 : protection DNS du WiFi public (SafeWiFi).
//
// Principe : deux règles NAT dst-nat réécrivent TOUT le port 53 transitant
// par le routeur (requêtes des clients vers n'importe quel résolveur, y
// compris celles adressées au routeur lui-même) vers le résolveur filtrant
// du niveau. Le DNS PROPRE du routeur n'est pas touché (son trafic part en
// chain=output, hors dstnat) : le check-in agent et la résolution locale
// restent intacts quel que soit l'état du résolveur filtrant — la
// disponibilité du site prime sur la stricteté du filtrage, et /ip dns
// n'est JAMAIS modifié.
//
//	off      : retire les règles marquées (retour à l'état antérieur) ;
//	threats  : Quad9 (malwares, phishing, arnaques) ;
//	family   : AdGuard Family (+ contenus adultes, publicités).
//
// Idempotent : seules les règles marquées "mikcloud-safewifi" sont
// remplacées, celles du gérant sont conservées. Le rapport échoe le nombre
// de règles marquées présentes APRÈS application (vérité routeur : le cloud
// ne pose la signature que si ce compte est exact — 2 en filtrage actif,
// 0 sinon).
//
// Limite documentée (MVP) : DoH (DNS over HTTPS, port 443) contourne la
// redirection — un filtrage par requête nécessiterait un DPI hors de portée
// d'un routeur 128 Mo. L'immense majorité des appareils en salon utilise le
// DNS du DHCP : la redirection couvre le besoin réel.
func (b Builder) buildSafeWifi(cmd model.Command) string {
	level := SafeWifiLevelFromPayload(cmd.Payload)
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do {\n  /ip firewall nat remove [find comment=\"" + SafeWifiMarker + "\"]\n} on-error={}\n")
	if level != model.SafeWifiOff {
		dns := SafeWifiResolver(level)
		sb.WriteString(":do {\n  /ip firewall nat add chain=dstnat action=dst-nat to-addresses=" + dns +
			" to-ports=53 protocol=udp dst-port=53 comment=\"" + SafeWifiMarker + "\"\n} on-error={ :set " + okVar + " false }\n")
		sb.WriteString(":do {\n  /ip firewall nat add chain=dstnat action=dst-nat to-addresses=" + dns +
			" to-ports=53 protocol=tcp dst-port=53 comment=\"" + SafeWifiMarker + "\"\n} on-error={ :set " + okVar + " false }\n")
	}
	// Rapport — vérité routeur : le compte de règles marquées présentes
	// après application (valeur DYNAMIQUE calculée côté routeur, pattern
	// fetchResultData : le cloud ne croit que ce que le routeur rapporte).
	sb.WriteString(":local swr [:len [/ip firewall nat find comment=\"" + SafeWifiMarker + "\"]]\n")
	ok := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + urlEscape(cmd.ID) + `&status=ok&rules=". $swr) output=none`
	ko := b.reportLine(cmd.ID, false, map[string]string{"message": "echec des regles de protection sur le routeur"})
	sb.WriteString(":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n")
	return sb.String()
}

// buildShield — N°81 : bouclier réseau du WiFi public (Shield).
//
// Principe : cinq règles FILTER par serveur hotspot du routeur, posées
// en TÊTE de chaîne (place-before=0) et ciblées sur l'INTERFACE du
// hotspot — le réseau du gérant (LAN) et le trafic propre du routeur
// (chain=output : check-in agent, DNS sortant) ne sont JAMAIS touchés :
//
//	input   ×2 : ports d'administration (winbox, ssh, telnet, api, ftp)
//	            inaccessibles DEPUIS le WiFi public ;
//	forward ×3 : connexions invalides, SMB et NetBIOS bloqués pour les
//	            appareils des clients (vecteurs de propagation des
//	            malwares sur réseau partagé) et vers le réseau du gérant.
//
//	L'interface du hotspot est lue SUR le routeur (foreach /ip hotspot
//	find) : le script s'adapte à toute topologie (wlan1,
//	bridge-hotspot…) et suit un renommage d'interface à la prochaine
//	réparation 6 h (vérité routeur, pattern fetchResultData).
//
//	off : retire les règles marquées (retour à l'état antérieur).
//
// Idempotent : seules les règles marquées "mikcloud-shield" sont
// remplacées, celles du gérant sont conservées. Le rapport échoe le
// nombre de règles marquées présentes APRÈS application ET le nombre
// de serveurs hotspots trouvés — le cloud ne pose la signature que si
// rules == 5 × hotspots (2 input + 3 forward par hotspot).
//
// Limites documentées (MVP) : le blindage s'applique au trafic IPv4
// traversant le routeur — l'isolation L2 de deux appareils d'un même
// pont (client-à-client sans traverser le routeur) relève du réglage
// du pont (use-ip-firewall, coûteux sur MIPS), hors de portée d'un MVP
// non intrusif ; l'administration reste possible depuis MikCloud
// (agent, connexions sortantes) et depuis le réseau local du gérant.
func (b Builder) buildShield(cmd model.Command) string {
	level := ShieldLevelFromPayload(cmd.Payload)
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do {\n  /ip firewall filter remove [find comment=\"" + ShieldMarker + "\"]\n} on-error={}\n")
	sb.WriteString(":local shn 0\n")
	if level != model.ShieldOff {
		sb.WriteString(":foreach h in=[/ip hotspot find] do={\n")
		sb.WriteString("  :set shn ($shn + 1)\n")
		sb.WriteString("  :local shi [/ip hotspot get $h interface]\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=input place-before=0 in-interface=$shi action=drop protocol=tcp dst-port=" + ShieldAdminTCPPorts +
			" comment=\"" + ShieldMarker + "\"\n  } on-error={ :set " + okVar + " false }\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=input place-before=0 in-interface=$shi action=drop protocol=udp dst-port=" + ShieldAdminUDPPorts +
			" comment=\"" + ShieldMarker + "\"\n  } on-error={ :set " + okVar + " false }\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=forward place-before=0 in-interface=$shi action=drop connection-state=invalid comment=\"" + ShieldMarker + "\"\n  } on-error={ :set " + okVar + " false }\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=forward place-before=0 in-interface=$shi action=drop protocol=tcp dst-port=" + ShieldMalwareTCPPorts +
			" comment=\"" + ShieldMarker + "\"\n  } on-error={ :set " + okVar + " false }\n")
		sb.WriteString("  :do {\n    /ip firewall filter add chain=forward place-before=0 in-interface=$shi action=drop protocol=udp dst-port=" + ShieldMalwareUDPPorts +
			" comment=\"" + ShieldMarker + "\"\n  } on-error={ :set " + okVar + " false }\n")
		sb.WriteString("}\n")
	}
	// Rapport — vérité routeur : le compte de règles marquées présentes
	// après application ET le nombre de serveurs hotspots trouvés (valeurs
	// DYNAMIQUES calculées côté routeur, pattern fetchResultData : le
	// cloud ne croit que ce que le routeur rapporte).
	sb.WriteString(":local shr [:len [/ip firewall filter find comment=\"" + ShieldMarker + "\"]]\n")
	ok := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + urlEscape(cmd.ID) + `&status=ok&rules=". $shr . "&hs=". $shn) output=none`
	ko := b.reportLine(cmd.ID, false, map[string]string{"message": "echec des regles du bouclier sur le routeur"})
	sb.WriteString(":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n")
	return sb.String()
}

// buildReadState — v5 (N°76) : télémétrie + rapport PAGINÉ des utilisateurs.
// Motivation : v4 (N°75) bornait le rapport à 500 users et gelait TOUTE
// déduction au-delà (badge « absent du routeur » ni posé ni levé) — les faux
// badges posés avant N°75 sur un parc de 3 478 users (cap v2 : 150) restaient
// prisonniers à vie : le rapport était tronqué en PERMANENCE, la
// réconciliation ne tournait plus jamais. v5 découpe le parc en fenêtres :
//   - total=<nb total d'users sur le routeur> : compteur EXACT rapporté par
//     chaque chunk — le cloud connaît le vrai parc dès le 1er ;
//   - start/count : fenêtre [start, start+count) du parc, inlinés par Go
//     depuis le payload de la commande ; out=<entrées émises> ;
//   - sessions : rapportées UNIQUEMENT par le chunk final (start+count >=
//     total) — les chunks intermédiaires n'alourdissent pas leur POST pour
//     rien ; stotal (total de sessions actives) est rapporté par tous ;
//   - trunc=true|false : il RESTE des chunks (start+count < total) —
//     informatif (le cloud décide sur total/start/count, pas sur le drapeau).
//
// v4 (N°75) : board/freehdd/totalhdd en Mo côté script ; ifaces (8 running,
// compteurs cumulés → débits par diff côté cloud). v2 : le gel honnête des
// déductions sur rapport incomplet reste LA règle (chunk perdu = cycle
// abandonné sans déduction, la complétude est vérifiée avant d'appliquer).
func (b Builder) buildReadState(cmd model.Command) string {
	start := int(plInt64(cmd.Payload, "start"))
	count := int(plInt64(cmd.Payload, "count"))
	if start < 0 {
		start = 0
	}
	if count <= 0 || count > ReadChunkSize {
		count = ReadChunkSize
	}
	end := start + count
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
:local mikIds [/ip hotspot user find]
:local mikTotal [:len $mikIds]
:local rusr ""
:local rout 0
:local n 0
:foreach u in=$mikIds do={
  :if ($n >= @@START@@ && $n < @@END@@) do={
    :set rusr ($rusr . [:tostr [/ip hotspot user get $u name]] . "|" . [:tostr [/ip hotspot user get $u profile]] . "|" . [:tostr [/ip hotspot user get $u disabled]] . ";")
    :set rout ($rout + 1)
  }
  :set n ($n + 1)
}
:local rtrunc "false"
:if (@@END@@ < $mikTotal) do={ :set rtrunc "true" }
:local rstotal [:len [/ip hotspot active find]]
:local rsess ""
:local rsn 0
:if (@@END@@ >= $mikTotal) do={
  :foreach a in=[/ip hotspot active find] do={
    :if ($rsn < 250) do={
      :set rsess ($rsess . [:tostr [/ip hotspot active get $a user]] . "|" . [:tostr [/ip hotspot active get $a address]] . "|" . [:tostr [/ip hotspot active get $a uptime]] . "|" . [:tostr [/ip hotspot active get $a bytes-in]] . "|" . [:tostr [/ip hotspot active get $a bytes-out]] . ";")
      :set rsn ($rsn + 1)
    }
  }
}
:local rsesspart ("&stotal=". $rstotal)
:if (@@END@@ >= $mikTotal) do={
  :set rsesspart ("&stotal=". $rstotal ."&sessions=". $rsess)
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
		` ."&board=". $rboard ."&freehdd=". $rfreehdd ."&totalhdd=". $rtotalhdd` +
		` ."&total=". $mikTotal ."&start=@@START@@&count=@@COUNT@@&out=". $rout` +
		` ."&users=". $rusr . $rsesspart ."&ifaces=". $rif ."&trunc=". $rtrunc) output=none` + "\n")
	// Placeholders substitués en dernier : une seule chaîne brute lisible,
	// aucune concaténation au milieu du script (le pattern @@VAR@@ ne peut
	// pas apparaître par accident dans une commande RouterOS).
	out := strings.NewReplacer("@@START@@", strconv.Itoa(start), "@@END@@", strconv.Itoa(end), "@@COUNT@@", strconv.Itoa(count)).Replace(sb.String())
	return out
}

func (b Builder) buildUserAdd(cmd model.Command) string {
	name := SanitizeName(plStr(cmd.Payload, "name"))
	pass := plStr(cmd.Payload, "password")
	prof := plProfile(cmd.Payload, "profile")
	comment := plStr(cmd.Payload, "comment")
	quota := plInt64(cmd.Payload, "limitBytesTotal")
	// Parité Mikhmon : limit-uptime du voucher (minutes) — le payload prime,
	// sinon héritage du session-timeout du profil (0 = illimité).
	uptime := plInt64(cmd.Payload, "limitUptimeMin")
	if uptime <= 0 && prof.HasTimeout && prof.SessionTimeoutMin > 0 {
		uptime = int64(prof.SessionTimeoutMin)
	}
	server := plStr(cmd.Payload, "server")
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(profileEnsureLine(prof))
	line := `/ip hotspot user add name="` + rosEscape(name) + `"`
	if pass != "" {
		line += ` password="` + rosEscape(pass) + `"`
	}
	line += ` profile="` + rosEscape(prof.Name) + `"`
	// Quota de temps TOTAL du ticket (parité Mikhmon) : limit-uptime du
	// voucher (override par lot) ou, à défaut, la durée du profil. Une fois
	// le cumul épuisé, le routeur refuse la reconnexion (« no more time »).
	if uptime > 0 {
		line += " limit-uptime=" + rosMinutes(int(uptime))
	}
	// Parité Mikhmon : serveur hotspot RouterOS visé ("all" ou nom précis).
	if server != "" {
		line += ` server="` + rosEscape(server) + `"`
	}
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
	// Parité Mikhmon : limit-uptime par lot (minutes) — le payload prime,
	// sinon héritage du session-timeout du profil (0 = illimité).
	uptime := plInt64(cmd.Payload, "limitUptimeMin")
	if uptime <= 0 && prof.HasTimeout && prof.SessionTimeoutMin > 0 {
		uptime = int64(prof.SessionTimeoutMin)
	}
	server := plStr(cmd.Payload, "server")
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
	sb.WriteString(profileEnsureLine(prof))
	for _, u := range users {
		line := `/ip hotspot user add name="` + rosEscape(SanitizeName(u.Name)) + `" password="` + rosEscape(u.Password) +
			`" profile="` + rosEscape(prof.Name) + `"`
		// Quota de temps TOTAL du ticket (parité Mikhmon, cf. buildUserAdd) :
		// limit-uptime du lot (override) ou du profil ; cumul épuisé = refus.
		if uptime > 0 {
			line += " limit-uptime=" + rosMinutes(int(uptime))
		}
		// Parité Mikhmon : serveur hotspot RouterOS visé ("all" ou nom précis).
		if server != "" {
			line += ` server="` + rosEscape(server) + `"`
		}
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
		`" http-method=post http-data=("cmd=` + cmdID + `&status=ok&data=". $rdata) output=none`
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
		`&status=ok&sent=". $psent ."&received=". $precv ."&minMs=". $pmin ."&avgMs=". $pavg ."&maxMs=". $pmax) output=none`
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

// buildReadResources — Parité Mikhmon : noms des ressources RouterOS utiles
// aux formulaires — pools d'adresses (/ip pool), files parent (/queue simple,
// hors files dynamiques) et serveurs hotspot (/ip hotspot). Chaque entrée est
// rapportée « kind|name; » (pool|queue|server), relu par parseResourcesRows.
func (b Builder) buildReadResources(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n:local rdata \"\"\n")
	sb.WriteString(":do {\n  :local rn 0\n")
	sb.WriteString("  :foreach pe in=[/ip pool find] do={\n")
	sb.WriteString("    :if ($rn < 60) do={\n")
	sb.WriteString(`      :set rdata ($rdata . "pool|" . [:tostr [/ip pool get $pe name]] . ";")` + "\n")
	sb.WriteString("      :set rn ($rn + 1)\n")
	sb.WriteString("    }\n  }\n")
	sb.WriteString("  :local qn 0\n")
	sb.WriteString("  :foreach qe in=[/queue simple find] do={\n")
	sb.WriteString("    :if ($qn < 60) do={\n")
	sb.WriteString("      :if ([:tostr [/queue simple get $qe dynamic]] = \"false\") do={\n")
	sb.WriteString(`        :set rdata ($rdata . "queue|" . [:tostr [/queue simple get $qe name]] . ";")` + "\n")
	sb.WriteString("      }\n")
	sb.WriteString("      :set qn ($qn + 1)\n")
	sb.WriteString("    }\n  }\n")
	sb.WriteString("  :local hn 0\n")
	sb.WriteString("  :foreach he in=[/ip hotspot find] do={\n")
	sb.WriteString("    :if ($hn < 20) do={\n")
	sb.WriteString(`      :set rdata ($rdata . "server|" . [:tostr [/ip hotspot get $he name]] . ";")` + "\n")
	sb.WriteString("      :set hn ($hn + 1)\n")
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
// N°75 — veille adaptative : le payload peut porter intervalSec (borné
// [45, 900] par défense en profondeur — le cloud est le seul émetteur mais
// un payload corrompu ne doit jamais espacer le scheduler au-delà du
// raisonnable) : la commande devient la bascule de pas du scheduler MikCloud
// (45 s actif ↔ 240 s veille), pilotée par le cloud à chaque check-in.
// L'outil F10 (disabled) garde son comportement historique.
func (b Builder) buildSchedulerSet(cmd model.Command) string {
	name := SanitizeName(plStr(cmd.Payload, "name"))
	set := `/system scheduler set [find name="` + rosEscape(name) + `"]`
	iv := 0
	if raw := int(plInt64(cmd.Payload, "intervalSec")); raw > 0 {
		iv = raw
		if iv < 45 {
			iv = 45
		}
		if iv > 900 {
			iv = 900
		}
		set += " interval=" + strconv.Itoa(iv) + "s"
	}
	if plBool(cmd.Payload, "disabled") {
		set += " disabled=yes"
	} else {
		set += " disabled=no"
	}
	// N°75 — le RAPPORT échoe l'intervalle réellement appliqué : le cloud
	// pose Router.SchedulerSec depuis cette valeur (vérité routeur, pas
	// le payload émis — une commande en file peut être périmée).
	extra := map[string]string{}
	if iv > 0 {
		extra["intervalSec"] = strconv.Itoa(iv)
	}
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":do { " + set + " } on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, extra))
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
// N°29 — walled-garden d'inscription publique (runbook N°27-D automatisé)
// ---------------------------------------------------------------------------

// SanitizeWGDomain — hôte walled-garden sûr : minuscules, [a-z0-9._-] plus un
// suffixe de port NUMÉRIQUE (déploiements non standard), 253 caractères max.
// Tout le reste est refusé : ces valeurs sont injectées dans un script
// RouterOS (défense en profondeur, rosEscape reste appliqué à l'écriture).
func SanitizeWGDomain(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || len(s) > 253 {
		return ""
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == ':') {
			return ""
		}
	}
	if i := strings.IndexByte(s, ':'); i >= 0 { // port numérique uniquement
		port := s[i+1:]
		if port == "" || len(port) > 5 {
			return ""
		}
		for j := 0; j < len(port); j++ {
			if port[j] < '0' || port[j] > '9' {
				return ""
			}
		}
	}
	return s
}

// WalledGardenDomainsFromPayload — les domaines d'une commande walled_garden
// ([]any JSON ou []string mémoire, cf. plStrList), assainis.
func WalledGardenDomainsFromPayload(p map[string]any) []string {
	raw := plStrList(p, "domains")
	out := make([]string, 0, len(raw))
	for _, d := range raw {
		if d = SanitizeWGDomain(d); d != "" {
			out = append(out, d)
		}
	}
	return out
}

// walledGardenInstallBlock — bloc walled-garden du script d'INSTALLATION
// (N°29) : la page d'inscription publique /join/{token} et l'API qu'elle
// appelle restent joignables AVANT authentification depuis le WiFi du hotspot
// (le scan du QR fonctionne sur place, cf. docs/RUNBOOK-WALLED-GARDEN.md).
// Idempotent : seules les règles marquées "mikcloud-wg" sont remplacées.
// Corps multi-lignes (règle du parseur console — cf. en-tête InstallScript).
// Vide si aucun domaine annoncé par le déploiement.
func walledGardenInstallBlock(domains []string) string {
	if len(domains) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n    # Walled-garden d'inscription publique (N°27/N°29) : la page /join et\n")
	sb.WriteString("    # son API restent joignables AVANT authentification — le scan du QR\n")
	sb.WriteString("    # fonctionne depuis le WiFi du hotspot. Seules les règles marquées\n")
	sb.WriteString("    # \"" + WalledGardenMarker + "\" sont remplacées, les vôtres sont conservées.\n")
	sb.WriteString("    :do {\n      /ip hotspot walled-garden remove [find comment=\"" + WalledGardenMarker + " page\"]\n    } on-error={}\n")
	sb.WriteString("    :do {\n      /ip hotspot walled-garden ip remove [find comment=\"" + WalledGardenMarker + " page\"]\n    } on-error={}\n")
	sb.WriteString("    :do {\n      /ip hotspot walled-garden remove [find comment=\"" + WalledGardenMarker + " dns\"]\n    } on-error={}\n")
	for _, d := range domains {
		sb.WriteString("    :do {\n      :if ([:len [/ip hotspot walled-garden find comment=\"" + WalledGardenMarker + " page\" dst-host=\"" + rosEscape(d) + "\"]] = 0) do={ /ip hotspot walled-garden add action=allow dst-host=\"" + rosEscape(d) + "\" comment=\"" + WalledGardenMarker + " page\" }\n    } on-error={}\n")
		// N°48 — miroir ip (action=accept, cf. N°31-d) : couverture HTTPS.
		sb.WriteString("    :do {\n      :if ([:len [/ip hotspot walled-garden ip find comment=\"" + WalledGardenMarker + " page\" dst-host=\"" + rosEscape(d) + "\"]] = 0) do={ /ip hotspot walled-garden ip add action=accept dst-host=\"" + rosEscape(d) + "\" comment=\"" + WalledGardenMarker + " page\" }\n    } on-error={}\n")
	}
	// N°48 — règles « api » (variante ip, action=accept, dst-host) : la
	// variante proxy ci-dessus ne voit que le HTTP pur (port 80) — or l'API
	// (claim, /portal, /join) est en HTTPS (Render/Vercel). Sans règles ip,
	// le TLS 443 pré-auth restait bloqué par le hotspot → le fetch du claim
	// échouait côté client (« Service WiFi offert momentanément indisponible
	// »). Pas de restriction de port : couvre TCP 80/443 ET UDP 443 (QUIC).
	sb.WriteString("    :do {\n      /ip hotspot walled-garden ip remove [find comment=\"" + WalledGardenMarker + " api\"]\n    } on-error={}\n")
	for _, d := range domains {
		sb.WriteString("    :do {\n      :if ([:len [/ip hotspot walled-garden ip find comment=\"" + WalledGardenMarker + " api\" dst-host=\"" + rosEscape(d) + "\"]] = 0) do={ /ip hotspot walled-garden ip add action=accept dst-host=\"" + rosEscape(d) + "\" comment=\"" + WalledGardenMarker + " api\" }\n    } on-error={}\n")
	}
	sb.WriteString("    :do {\n      /ip hotspot walled-garden ip remove [find comment=\"" + WalledGardenMarker + " dns\"]\n    } on-error={}\n")
	sb.WriteString("    :do {\n      :if ([:len [/ip hotspot walled-garden ip find comment=\"" + WalledGardenMarker + " dns\" protocol=udp]] = 0) do={ /ip hotspot walled-garden ip add action=accept protocol=udp dst-port=53 comment=\"" + WalledGardenMarker + " dns\" }\n    } on-error={}\n")
	sb.WriteString("    :do {\n      :if ([:len [/ip hotspot walled-garden ip find comment=\"" + WalledGardenMarker + " dns\" protocol=tcp]] = 0) do={ /ip hotspot walled-garden ip add action=accept protocol=tcp dst-port=53 comment=\"" + WalledGardenMarker + " dns\" }\n    } on-error={}\n")
	return sb.String()
}

// buildWalledGarden — N°29 : applique le walled-garden d'inscription publique
// sur un routeur AGENT déjà en ligne (le script d'installation le fait pour
// les routeurs neufs). Idempotent : les règles marquées sont remplacées, les
// autres préservées. Les 2 règles DNS (udp/tcp 53) garantissent que la
// résolution traverse le routeur même pour les clients avec DNS codé en dur —
// le matching par domaine du walled-garden s'appuie sur le reniflement DNS.
// Rapport : domains = nombre de règles page/api réellement posées.
// N°32 — TRAÇAGE : une variable RouterOS « step » est posée avant chaque bloc
// à risque et embarquée dans le rapport d'erreur (concaténation console
// « . $step », construct d'expression identique au « (“…”) » prouvé) — le
// constat prod du 05/09 (3× echec_sur_le_routeur sur le script « propre »,
// alors que le même script à 5 domaines passe) exige de savoir QUELLE ligne
// échoue sans accès console au routeur client.
func (b Builder) buildWalledGarden(cmd model.Command) string {
	domains := WalledGardenDomainsFromPayload(cmd.Payload)
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	// N°31-c — BATTEMENT DE CŒUR : le script confirme SA LIVRAISON avant
	// d'attaquer les lignes à risque (construct « status=started », identique
	// aux fetch de rapport — prouvé 849+ fois). Si l'import meurt ensuite sur
	// une ligne que ce RouterOS rejette, le cloud sait au moins que le
	// fichier est ARRIVÉ — l'ancien silence total rendait tout diagnostic
	// impossible (constat prod : 2 livraisons sans AUCUN signal).
	sb.WriteString(`/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + urlEscape(cmd.ID) + `&status=started") output=none` + "\n")
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(`:local step "start"` + "\n")
	step := func(s string) { sb.WriteString(`:set step "` + s + `"` + "\n") }
	// N°31-c — find EXACT (même classe syntaxique que `find name="..."` des
	// user_remove — prouvé 4×) au lieu du regex `comment~"..."` — suspect
	// n°1 du blocage d'import constaté en prod (chunk muet 2×/2×, commandes
	// du même check-in tuées avec lui). Nos règles portent EXACTEMENT les
	// commentaires ci-dessous : la suppression exacte est complète.
	// N°31-e — removes SILENCIEUX (best-effort) : une règle « en usage » par
	// les clients du hotspot (flux DNS permanents sur les règles DNS — constat
	// prod 18:26→18:30 : 6 re-filés error d’affilée) ne doit PAS faire
	// échouer la mise à jour : le service prime sur le ménage.
	step("rm-page")
	sb.WriteString(":do { /ip hotspot walled-garden remove [find comment=\"" + WalledGardenMarker + " page\"] } on-error={}\n")
	step("rm-api-ip")
	sb.WriteString(":do { /ip hotspot walled-garden ip remove [find comment=\"" + WalledGardenMarker + " api\"] } on-error={}\n")
	step("rm-dns-ip")
	sb.WriteString(":do { /ip hotspot walled-garden ip remove [find comment=\"" + WalledGardenMarker + " dns\"] } on-error={}\n")
	// N°31-e — adds CONDITIONNELS à l’absence : si le remove vient d’échouer,
	// la règle existe DÉJÀ (service assuré) → skip — PAS de doublon, PAS
	// d’erreur. Seule une vraie erreur d’add met okVar à false.
	// N°32 — chaque bloc à risque est précédé de :set step : le rapport
	// d’erreur embarque la ligne fautive (« &step=" . $step ») — diagnostic
	// sans accès console (les removes étant best-effort, seuls les adds
	// peuvent porter okVar à false).
	// N°31-d — action=ACCEPT (et NON allow) sur walled-garden ip : la table
	// n’accepte que accept|drop|reject (doc officielle HotSpot) — « allow »
	// est une erreur de validation console qui rejetait TOUT le fichier
	// d’import (constat prod : 4 livraisons muettes, rien ne s’exécutait).
	for i, d := range domains {
		step("add-page-" + strconv.Itoa(i+1))
		sb.WriteString(":do { :if ([:len [/ip hotspot walled-garden find comment=\"" + WalledGardenMarker + " page\" dst-host=\"" + rosEscape(d) + "\"]] = 0) do={ /ip hotspot walled-garden add action=allow dst-host=\"" + rosEscape(d) + "\" comment=\"" + WalledGardenMarker + " page\" } } on-error={ :set " + okVar + " false }\n")
	}
	// N°48 — règles « api » (variante ip) : le HTTPS pré-auth passe ICI, pas
	// dans la variante proxy (page) qui ne voit que le HTTP pur. La variante
	// ip n'accepte que accept|drop|reject (N°31-d) → action=accept ; le
	// matching dst-host s'appuie sur le reniflement DNS (les règles DNS
	// ci-dessous garantissent que la résolution transite par le routeur,
	// même pour les clients avec DNS codé en dur).
	for i, d := range domains {
		step("add-api-" + strconv.Itoa(i+1))
		sb.WriteString(":do { :if ([:len [/ip hotspot walled-garden ip find comment=\"" + WalledGardenMarker + " api\" dst-host=\"" + rosEscape(d) + "\"]] = 0) do={ /ip hotspot walled-garden ip add action=accept dst-host=\"" + rosEscape(d) + "\" comment=\"" + WalledGardenMarker + " api\" } } on-error={ :set " + okVar + " false }\n")
	}
	step("add-dns-udp")
	sb.WriteString(":do { :if ([:len [/ip hotspot walled-garden ip find comment=\"" + WalledGardenMarker + " dns\" protocol=udp]] = 0) do={ /ip hotspot walled-garden ip add action=accept protocol=udp dst-port=53 comment=\"" + WalledGardenMarker + " dns\" } } on-error={ :set " + okVar + " false }\n")
	step("add-dns-tcp")
	sb.WriteString(":do { :if ([:len [/ip hotspot walled-garden ip find comment=\"" + WalledGardenMarker + " dns\" protocol=tcp]] = 0) do={ /ip hotspot walled-garden ip add action=accept protocol=tcp dst-port=53 comment=\"" + WalledGardenMarker + " dns\" } } on-error={ :set " + okVar + " false }\n")
	ok := b.reportLine(cmd.ID, true, map[string]string{"domains": strconv.Itoa(len(domains))})
	ko := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + urlEscape(cmd.ID) + `&status=error&message=echec_sur_le_routeur&step=" . $step) output=none`
	sb.WriteString(":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n")
	return sb.String()
}

// HotspotFilesFromPayload — la liste ordonnée des fichiers à déployer pour
// une commande hotspot_files. Chaque entrée porte le chemin RELATIF
// (login.html, status.html, css/bootstrap.min.css, …) qui sert à la fois de
// clé de signature et de dst-path côté routeur (sous le dossier hotspot/).
func HotspotFilesFromPayload(p map[string]any) []string {
	raw := plStrList(p, "files")
	out := make([]string, 0, len(raw))
	for _, f := range raw {
		if f = sanitizePortalPath(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// sanitizePortalPath — n'accepte qu'un chemin RELATIF sans remontée (pas de
// « .. », pas de « / » initial) et un charset sûr : la cible côté routeur est
// toujours hotspot/<path>, et le fetch ne doit JAMAIS sortir de ce dossier
// (sécurité défense en profondeur même si la source est déjà validée cloud).
func sanitizePortalPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || strings.HasPrefix(p, "/") {
		return ""
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return ""
		}
		for i := 0; i < len(seg); i++ {
			c := seg[i]
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
				c == '-' || c == '_' || c == '.') {
				return ""
			}
		}
	}
	return p
}

// buildHotspotFiles — N°35 : déploiement automatique du portail captif sur un
// routeur AGENT déjà en ligne. Le cloud sert les fichiers personnalisés par
// compte (branding, offres, textes, slug WiFi, lien join) via
// GET /portal/{token}/{path} (mêmes hôtes que l'agent, TLS strict hérité,
// déjà walled-gardenés N°29). L'agent fait un /tool fetch par fichier (RouterOS
// n'a PAS d'extracteur ZIP natif — architecture un-fetch-par-fichier, ~30
// fetchs par déploiement). Ordre séquentiel : assets (css/js/img/webfonts)
// d'abord, status.html et pages auxiliaires ensuite, login.html EN DERNIER
// pour l'atomicité — un client qui ouvre login.html pendant le déploiement
// chargera la nouvelle page qui référence des assets déjà en place.
//
// Pattern calqué sur buildWalledGarden (N°29-N°32) :
//   - battement de cœur status=started AVANT les fetchs (preuve de livraison) ;
//   - variable step posée avant chaque bloc à risque (diagnostic sans console) ;
//   - on-error={} par fichier (un fetch échoué ne tue pas les autres) ;
//   - okVar global, rapport ok si tous les fetchs ont réussi ;
//   - reprise zombie 10 min via staleSentReadKinds (idempotent par surcharge) ;
//   - signature posée au retour « ok » uniquement (handleAgentResult).
//
// Le dossier hotspot/ DOIT exister côté routeur (pré-requis manuel one-shot :
// /ip hotspot profile set html-directory=hotspot). Si absent, le 1er fetch
// échoue → okVar false → rapport step="mkdir" ou "fetch-<path>" → re-file au
// check-in suivant (max 3 re-tries puis error avec activity log).
func (b Builder) buildHotspotFiles(cmd model.Command) string {
	files := HotspotFilesFromPayload(cmd.Payload)
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	// Battement de cœur N°31-c : prouve la livraison du .rsc avant les fetchs.
	sb.WriteString(`/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + urlEscape(cmd.ID) + `&status=started") output=none` + "\n")
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(`:local step "start"` + "\n")
	step := func(s string) { sb.WriteString(`:set step "` + s + `"` + "\n") }

	// Pré-requis : s'assurer que le dossier hotspot/ existe. /file mkdir est
	// idempotent sous RouterOS 7+ (renvoie une erreur bénigne si déjà là → on-error={}).
	// Côté < 7.x, la commande n'existe pas : le fetch créera lui-même le
	// sous-dossier à la première écriture (comportement RouterOS) — si mkdir
	// échoue, on continue quand même : le fetch décide.
	step("mkdir")
	sb.WriteString(`:do { /file mkdir "hotspot" } on-error={}` + "\n")

	base := strings.TrimRight(b.BaseURL, "/") + "/portal/" + urlEscape(b.Token) + "/"
	// N°35 — ordre séquentiel pour atomicité : assets d'abord, login.html en
	// dernier. L'appelant (ensureHotspotFilesLocked) a déjà trié la liste dans
	// cet ordre ; on dépile simplement.
	// urlPathEscape échappe chaque segment du path SÉPARÉMENT (pour préserver
	// les « / » qui séparent css/, js/, img/, webfonts/ — urlEscape encode
	// « / » en %2F, ce qui casserait l'URL côté routeur).
	for i, f := range files {
		step("fetch-" + strconv.Itoa(i+1))
		// Remove best-effort du fichier précédent (surcharge) puis fetch du
		// nouveau. Si remove échoue (fichier absent la 1re fois), on
		// continue. Si fetch échoue → okVar false, mais les autres fetchs
		// continuent : un seul fichier cassé ne doit pas tout bloquer —
		// le routeur garde l'ancienne version des autres, et la reprise
		// zombie 10 min re-tentera le fichier fauteur.
		dst := "hotspot/" + f
		sb.WriteString(`:do { /file remove "` + rosEscape(dst) + `" } on-error={}` + "\n")
		sb.WriteString(`:do { /tool fetch url="` + base + urlPathEscape(f) +
			`" dst-path="` + rosEscape(dst) + `" } on-error={ :set ` + okVar + " false }\n")
	}
	step("done")
	ok := b.reportLine(cmd.ID, true, map[string]string{"files": strconv.Itoa(len(files))})
	ko := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + urlEscape(cmd.ID) + `&status=error&message=echec_sur_le_routeur&step=" . $step) output=none`
	sb.WriteString(":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n")
	return sb.String()
}

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
