// N°115 — Mise à jour RouterOS depuis MikCloud : builders des commandes
// routeros_check (lecture de l'état des mises à jour) et routeros_update
// (installation). Le gérant garde son parc à jour sans Winbox : la vérification
// interroge les serveurs MikroTik DEPUIS le routeur (le canal de mise à jour
// du routeur — stable par défaut — fait foi), l'installation télécharge puis
// redémarre. Parc concerné : agents RouterOS ≥ 7.19 (garde TLS à
// l'installation, cf. InstallScript) — sémantique v7 de /system package update
// (champs status/latest-version/installed-version/channel, install =
// download + install + reboot).
//
// N°125 — firmware RouterBOARD : le bootloader est un monde SÉPARÉ du RouterOS
// (il ne s'applique qu'au REDÉMARRAGE, et seulement si auto-upgrade=yes —
// désactivé par défaut) : un parc à jour côté RouterOS peut donc porter un
// firmware en attente, révélé par le check et appliqué par la commande
// routerboard_firmware.
package agent

import (
	"strconv"
	"strings"

	"mikcloud/hotspot-api/internal/model"
)

// RouterOSCheckPollMax — bornes de la boucle d'attente de la vérification :
// check-for-updates est ASYNCHRONE (le status passe par « Checking… » le
// temps que le routeur interroge les serveurs MikroTik). Le script attend
// 2 s par itération, ≤ RouterOSCheckPollMax itérations (~30 s pire cas —
// un lien lent) puis rapporte le status TEL QUEL : le cloud normalise et
// tolère un état inconnu (« unknown », affiché brut au gérant).
const RouterOSCheckPollMax = 15

// buildRouterOSCheck — N°115 — /system package update check-for-updates puis
// lecture de status / latest-version / installed-version / channel. Chaque
// lecture est isolée dans son :do on-error (un champ absent sur une version
// exotique ne tue pas la commande). Le rapport est DYNAMIQUE (valeurs
// calculées côté routeur, concaténées — pattern ping F8) : la clé rosStatus
// transporte le status RouterOS BRUT (la clé status est déjà celle du
// protocole ok/error du rapport).
//
// N°125 — le check lit AUSSI l'état du firmware RouterBOARD (bootloader) :
// current-firmware (en place) vs upgrade-firmware (livré avec le dernier
// paquet RouterOS, appliqué au redémarrage si auto-upgrade=yes — désactivé
// par défaut : c'est ainsi qu'un parc à jour côté RouterOS se retrouve avec
// un firmware en attente). Lectures isolées pareillement : un CHR ou un vieux
// build sans /system routerboard ne tue pas la commande — les champs restent
// vides et le cloud n'expose simplement pas la ligne firmware.
func (b Builder) buildRouterOSCheck(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":local rosStat \"\"\n:local rosLatest \"\"\n:local rosInst \"\"\n:local rosChan \"\"\n")
	sb.WriteString(":local fwCur \"\"\n:local fwStg \"\"\n:local fwAuto \"\"\n")
	sb.WriteString(":do {\n")
	// Lancement de la vérification — asynchrone, le status passe par
	// « Checking for updates… » le temps de la réponse MikroTik.
	sb.WriteString("  /system package update check-for-updates\n")
	// Poll de fin : tant que le status contient « heck » (Checking/checking,
	// la casse varie selon les builds), on attend. La garde [:typeof …] = "num"
	// est le pattern maison sûr de find-qui-ne-trouve-pas (nil, jamais comparé
	// nu — cf. buildPing) ; un status final (up to date / New version / Error)
	// sort immédiatement de la boucle.
	sb.WriteString("  :local i 0\n")
	sb.WriteString("  :while ($i < " + strconv.Itoa(RouterOSCheckPollMax) + ") do={\n")
	sb.WriteString("    :delay 2s\n")
	sb.WriteString("    :set rosStat [/system package update get status]\n")
	sb.WriteString("    :local p [:find $rosStat \"heck\"]\n")
	sb.WriteString("    :if ([:typeof $p] = \"num\") do={ :set i ($i + 1) } else={ :set i 99 }\n")
	sb.WriteString("  }\n")
	// Relecture finale + champs périphériques, chacun isolé (défense vieux ROS).
	sb.WriteString("  :do { :set rosStat [/system package update get status] } on-error={ }\n")
	sb.WriteString("  :do { :set rosLatest [/system package update get latest-version] } on-error={ }\n")
	sb.WriteString("  :do { :set rosInst [/system package update get installed-version] } on-error={ }\n")
	sb.WriteString("  :do { :set rosChan [/system package update get channel] } on-error={ }\n")
	// N°125 — firmware RouterBOARD (bootloader), isolé pareillement.
	// N°132 — auto-upgrade vit sous /system routerboard SETTINGS : le
	// chemin N°125 échouait en silence sur le vrai matériel (fwAuto
	// toujours vide, indicateur jamais affiché).
	sb.WriteString("  :do { :set fwCur [/system routerboard get current-firmware] } on-error={ }\n")
	sb.WriteString("  :do { :set fwStg [/system routerboard get upgrade-firmware] } on-error={ }\n")
	sb.WriteString("  :do { :set fwAuto [/system routerboard settings get auto-upgrade] } on-error={ }\n")
	sb.WriteString("} on-error={ :set " + okVar + " false }\n")
	// Rapport dynamique (valeurs calculées côté routeur, pas d'escape possible —
	// RouterOS les concatène à l'exécution, elles ne passent jamais par le
	// générateur Go ; le cloud borne/rosEscape à la relecture).
	ok := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + cmd.ID +
		`&status=ok&rosStatus=". $rosStat ."&latest=". $rosLatest ."&installed=". $rosInst ."&channel=". $rosChan .` +
		`"&fwCurrent=". $fwCur ."&fwStaged=". $fwStg ."&fwAuto=". $fwAuto) output=none`
	ko := b.reportLine(cmd.ID, false, map[string]string{"message": "verification de mise a jour impossible sur le routeur"})
	sb.WriteString(":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n")
	return sb.String()
}

// buildRouterOSUpdate — N°115 — /system package update install : télécharge
// les paquets puis les installe et REDÉMARRE le routeur (sémantique v7 ;
// Mikhmon enchaîne exactement check → install). Pattern reboot F10 (buildPower) :
// le rapport ok part AVANT l'exécution — le /tool fetch est bloquant, il
// termine avant que le téléchargement (minutes) et le redémarrage ne coupent
// le script. Si l'installation ÉCHOUE (téléchargement impossible : pas de
// sortie internet vers MikroTik, serveurs injoignables, stockage plein), le
// routeur ne redémarre PAS : le :do on-error rapporte l'échec après coup —
// le poll du front voit la commande basculer done → error (handleAgentResult
// ré-applique le rapport, la commande est retrouvée par ID).
//
// Le routeur ne re-check-in plus pendant l'installation (script bloqué) puis
// redémarre : la commande « done » reste la trace du lancement, la VERSION
// nouvelle revient d'elle-même via le read_state post-redémarrage (enfilé par
// handleAgentResult, fraîcheur post-écriture) et le journal N°115 en trace la
// confirmation.
//
// N°125 — le firmware RouterBOARD suit le RouterOS : auto-upgrade posé AVANT
// l'install pour que le MÊME redémarrage applique les deux (sinon le firmware
// livré avec le paquet reste en attente — comportement RouterOS par défaut,
// auto-upgrade=no).
func (b Builder) buildRouterOSUpdate(cmd model.Command) string {
	var sb strings.Builder
	sb.WriteString(header(cmd))
	// Rapport de lancement AVANT l'exécution (pattern reboot) : extra « latest »
	//purement informatif (validé par le handler, échappé par urlEscape).
	sb.WriteString(b.reportLine(cmd.ID, true, map[string]string{
		"action": "install",
		"latest": sanitizeRouterOSVersion(plStr(cmd.Payload, "latest")),
	}) + "\n")
	sb.WriteString(":delay 1s\n")
	// N°125 — firmware RouterBOARD synchronisé au même redémarrage. Isolé : un
	// CHR ou un vieux build sans /system routerboard ne doit pas bloquer
	// l'installation RouterOS.
	sb.WriteString(":do { /system routerboard settings set auto-upgrade=yes } on-error={ }\n")
	sb.WriteString(":do {\n")
	sb.WriteString("  /system package update install\n")
	sb.WriteString("} on-error={\n")
	sb.WriteString("  :log warning \"mikcloud: mise a jour RouterOS impossible\"\n")
	sb.WriteString("  " + b.reportLine(cmd.ID, false, map[string]string{
		"message": "installation impossible sur le routeur (telechargement echoue ?)",
	}) + "\n")
	sb.WriteString("}\n")
	return sb.String()
}

// buildRouterboardFirmware — N°125 — applique le firmware RouterBOARD EN
// ATTENTE : le firmware livré avec le dernier paquet RouterOS ne s'applique
// qu'au REDÉMARRAGE et seulement si auto-upgrade=yes (désactivé par défaut —
// d'où les parcs « RouterOS à jour, firmware en attente » que le check N°125
// révèle). Garde côté routeur (le cloud ne se fie jamais au front) : relire
// current-firmware / upgrade-firmware et ne redémarrer QUE si un firmware
// attend réellement — une coupure de 2 à 5 min doit avoir une raison. Sinon :
// rapport ok SANS redémarrage (état déjà synchronisé). Quand il y a quelque
// chose à appliquer : auto-upgrade posé (les prochaines mises à jour RouterOS
// appliqueront le firmware d'elles-mêmes), staging explicite, rapport ok
// AVANT le redémarrage (pattern reboot F10 — le fetch bloquant termine
// premier), reboot. L'échec du staging est rapporté en erreur (le routeur ne
// redémarre pas dans ce cas).
func (b Builder) buildRouterboardFirmware(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(":local fwCur \"\"\n:local fwStg \"\"\n:local fwNeed false\n")
	sb.WriteString(":do { :set fwCur [/system routerboard get current-firmware] } on-error={ }\n")
	sb.WriteString(":do { :set fwStg [/system routerboard get upgrade-firmware] } on-error={ }\n")
	// Garde : ne redémarrer que s'il y a un firmware en attente RÉEL (le
	// cloud a pu montrer un état périmé — le routeur fait foi).
	sb.WriteString(":if ($fwStg != \"\" && $fwStg != $fwCur) do={ :set fwNeed true }\n")
	sb.WriteString(":if ($fwNeed) do={\n")
	sb.WriteString("  :do {\n")
	sb.WriteString("    /system routerboard settings set auto-upgrade=yes\n")
	sb.WriteString("    /system routerboard upgrade\n")
	sb.WriteString("  } on-error={ :set " + okVar + " false }\n")
	sb.WriteString("}\n")
	// Rapport dynamique (pattern ping F8 — les valeurs sont lues côté routeur
	// et concaténées à l'exécution) : « applied » distingue un vrai appliquage
	// (redémarrage en cours) d'un état déjà synchronisé.
	ok := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + cmd.ID +
		`&status=ok&action=firmware&fwCurrent=". $fwCur ."&fwStaged=". $fwStg ."&applied=". $fwNeed) output=none`
	ko := b.reportLine(cmd.ID, false, map[string]string{"message": "firmware routeurboard impossible sur le routeur"})
	sb.WriteString(":if ($" + okVar + ") do={\n")
	sb.WriteString("  " + ok + "\n")
	// Le redémarrage suit le rapport ok, UNIQUEMENT s'il y a quelque chose à
	// appliquer (sinon la commande se termine sans couper le hotspot).
	sb.WriteString("  :if ($fwNeed) do={\n")
	sb.WriteString("    :delay 2s\n")
	sb.WriteString("    /system reboot\n")
	sb.WriteString("  }\n")
	sb.WriteString("} else={\n")
	sb.WriteString("  :log warning \"mikcloud: firmware routeurboard impossible\"\n")
	sb.WriteString("  " + ko + "\n")
	sb.WriteString("}\n")
	return sb.String()
}

// sanitizeRouterOSVersion — version RouterOS sûre pour l'embarquement dans un
// script/une URL : [0-9A-Za-z.-] jusqu'à 32 caractères, PREMIER caractère
// obligatoirement un chiffre (miroir de la regex du handler — les versions
// RouterOS réelles : « 7.18.3 », « 7.19beta5 », « 6.49.18 (long-term) » — les
// parenthèses/suffixes d'édition sont coupés, seul le noyau numérique compte
// pour l'affichage). Défense en profondeur : le handler valide déjà la forme,
// urlEscape neutralise le reste.
func sanitizeRouterOSVersion(v string) string {
	v = strings.TrimSpace(v)
	cut := 0
	for i := 0; i < len(v) && cut < 32; i++ {
		c := v[i]
		switch {
		case cut == 0 && !(c >= '0' && c <= '9'):
			// Pas un chiffre en tête : ce n'est pas une version, rien à garder.
			return ""
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '.', c == '-':
			cut++
		default:
			return v[:cut]
		}
	}
	return v[:cut]
}
