// N°230 — migration d'URL des schedulers mikcloud (runbook
// MIGRATION-ORACLE.md §5.1-6 : bascule de domaine SANS Winbox).
//
// Le canal de commande d'un routeur est un scheduler (/system scheduler
// « mikcloud-agent », 45 s) dont l'on-event porte l'URL du cloud EN DUR
// (gravée à l'installation) ; le veilleur d'invités (« mikcloud-watch »,
// 20 s) porte la même URL dans son on-event. Le cadenceur de quota
// (« mikcloud-quota ») est 100 % local — aucune URL, rien à migrer.
// Cette commande réécrit les DEUX schedulers porteurs avec l'URL courante
// du Builder (MIKCLOUD_BASE_URL), sous trois garanties :
//
//  1. PRE-FLIGHT — la nouvelle URL est testée (/agent/register en heartbeat
//     pur : corps vide, aucun marqueur consommé, aucune télémétrie
//     modifiée) AVANT tout retrait : DNS + TLS + token validés, sinon la
//     migration avorte et l'ancien scheduler reste en place ;
//  2. PONT ANTI-ORPHELIN — le scheduler principal n'est JAMAIS retiré sans
//     filet : un scheduler pont « mikcloud-agent-b » (nouvelle URL) est
//     posé et vérifié (présence + hôte dans l'on-event) AVANT que
//     l'ancien ne parte ; si la repose du canonique échoue, le pont reste
//     vivant — le routeur garde un canal de commande opérationnel ;
//  3. MÉNAGE CONDITIONNEL — le pont n'est retiré QUE si le scheduler
//     canonique est vérifié présent AVEC la nouvelle URL dans son
//     on-event (un remove qui échouerait en silence ne saurait pas se
//     faire passer pour une réussite).
//
// Idempotent de bout en bout (re-file zombie N°73, reprise N°159) : un
// pont déjà posé est réutilisé, les vérifications par nom + hôte
// tranchent, le ménage final ne part qu'à l'état convergé.
package agent

import (
	"mikcloud/hotspot-api/internal/model"
	"strconv"
	"strings"
)

// AgentBridgeName — nom du scheduler PONT (bascule anti-orphelin N°230).
// Distinct du canonique : remove [find name=…] retire TOUTES les entrées
// du nom — un pont homonyme du canonique serait fauché par le ménage.
const AgentBridgeName = "mikcloud-agent-b"

// agentMainOnEvent — corps UNE LIGNE du on-event du scheduler principal
// (miroir sémantique du bloc de l'installateur, sérialisation
// watcherOnEvent : séparateurs « ; », reçoit l'URL et le token DÉJÀ
// échappés pour le niveau de citation interne, l'appelant ré-échappe le
// corps entier pour on-event="…"). Fetch /agent/cmd → import : c'est le
// battement de cœur du canal de commande.
func agentMainOnEvent(urlEsc, tokEsc string) string {
	return ":local mkf \"yes\"; " +
		":do { /tool fetch url=\"" + urlEsc + "/agent/cmd?token=" + tokEsc + "\" dst-path=\"" + ScriptFilename + "\" } on-error={ :set mkf \"no\"; :log warning \"MikCloud agent: check-in echoue (reseau ou TLS ? RouterOS 7.19+ requis)\" }; " +
		":if ($mkf = \"yes\") do={ :delay 2s; /import file-name=\"" + ScriptFilename + "\" }"
}

// agentHost — hôte nu d'une URL de base (marqueur de vérification du
// on-event : le hôte de la NOUVELLE URL doit y figurer littéralement).
func agentHost(base string) string {
	s := strings.TrimRight(strings.TrimSpace(base), "/")
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndexByte(s, '@'); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// buildAgentMigrate — .rsc de la commande agent_migrate (N°230).
func (b Builder) buildAgentMigrate(cmd model.Command) string {
	urlEsc := rosEscape(strings.TrimRight(b.BaseURL, "/"))
	tokEsc := rosEscape(b.Token)
	host := agentHost(b.BaseURL)
	okVar := "ok" + idSafe(cmd.ID)
	p := "m" + idSafe(cmd.ID) // préfixe des variables locales (unicité inter-commandes dans un même import)
	mainOE := rosEscape(agentMainOnEvent(urlEsc, tokEsc))
	watchOE := rosEscape(watcherOnEvent(urlEsc, tokEsc))
	hasHost := func(varName, schedName string) string {
		// :local X false ; :foreach s in=[find where name=…] do={ :if ([:typeof [:find $oe HOST]] != "nil") do={ :set X true } }
		return ":local " + varName + " false\n" +
			":do { :foreach " + p + "s in=[/system scheduler find where name=\"" + schedName + "\"] do={ :local " + p + "oe [:tostr [/system scheduler get $" + p + "s on-event]]; :if ([:typeof [:find $" + p + "oe \"" + host + "\"]] != \"nil\") do={ :set " + varName + " true } } } on-error={}\n"
	}

	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " false\n")

	// [1/4] PRE-FLIGHT — la nouvelle URL répond-elle (DNS + TLS + token) ?
	// /agent/register sans corps = heartbeat pur (touchAgent, rien d'écrit).
	sb.WriteString(":local " + p + "pre \"yes\"\n")
	sb.WriteString(":do { /tool fetch url=\"" + urlEsc + "/agent/register?token=" + tokEsc + "\" http-method=post output=none } on-error={ :set " + p + "pre \"no\"; :log warning \"MikCloud: migration URL avortee (pre-flight echoue)\" }\n")

	sb.WriteString(":if ($" + p + "pre = \"yes\") do={\n")

	// [2/4] PONT — poser (ou retrouver) mikcloud-agent-b avec la NOUVELLE
	// URL. Un reste d'une exécution antérieure est retiré d'abord (son URL
	// pourrait être une ancienne cible morte : le filet doit être VIVANT).
	sb.WriteString("  :do { /system scheduler remove [find name=\"" + AgentBridgeName + "\"] } on-error={}\n")
	sb.WriteString("  :do { /system scheduler add name=\"" + AgentBridgeName + "\" interval=45s start-time=startup on-event=\"" + mainOE + "\" } on-error={}\n")
	sb.WriteString("  " + hasHost(p+"b", AgentBridgeName))

	sb.WriteString("  :if ($" + p + "b) do={\n")

	// [3/4] CANONIQUE — retirer l'ancien, reposer le canonique (nouvelle
	// URL). Le script s'exécute SOUS l'ancien scheduler : RouterOS poursuit
	// l'exécution après le retrait (pattern self-removing), et si un lien
	// cassait le pont reste vivant — jamais d'orphelin.
	sb.WriteString("    :do { /system scheduler remove [find name=\"" + SchedulerName + "\"] } on-error={}\n")
	sb.WriteString("    :delay 1s\n")
	sb.WriteString("    :local " + p + "o false\n")
	sb.WriteString("    :do { :foreach " + p + "s in=[/system scheduler find where name=\"" + SchedulerName + "\"] do={ :set " + p + "o true } } on-error={}\n")
	sb.WriteString("    :if (!$" + p + "o) do={\n")
	sb.WriteString("      :do { /system scheduler add name=\"" + SchedulerName + "\" interval=45s start-time=startup on-event=\"" + mainOE + "\" } on-error={}\n")
	sb.WriteString("    }\n")

	// Veilleur d'invités — remove-then-add à la nouvelle URL (pattern
	// watcher_ensure N°77 : il n'est PAS le canal de commande, un simple
	// remove-then-add suffit).
	sb.WriteString("    :do { /system scheduler remove [find name=\"" + WatcherName + "\"] } on-error={}\n")
	sb.WriteString("    :do { /system scheduler add name=\"" + WatcherName + "\" interval=" + strconv.Itoa(WatcherIntervalSec) + "s start-time=startup on-event=\"" + watchOE + "\" } on-error={}\n")

	// [4/4] MÉNAGE CONDITIONNEL — le canonique doit être présent AVEC la
	// nouvelle URL (le hôte dans l'on-event) : seulement alors le pont est
	// retiré ; sinon il reste en place et l'échec est rapporté.
	sb.WriteString("    " + hasHost(p+"c", SchedulerName))
	sb.WriteString("    :if ($" + p + "c) do={ :set " + okVar + " true; :do { /system scheduler remove [find name=\"" + AgentBridgeName + "\"] } on-error={} } else={ :log warning \"MikCloud: migration URL partielle (pont " + AgentBridgeName + " actif)\" }\n")

	sb.WriteString("  }\n")
	sb.WriteString("}\n")
	sb.WriteString(b.resultLines(cmd.ID, okVar, map[string]string{"baseUrl": strings.TrimRight(b.BaseURL, "/")}))
	return sb.String()
}
