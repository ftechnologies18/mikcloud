// Tâches planifiées routeur (scheduler) et actions d'alimentation.
// Extrait du monolithe agent.go (N°88) — même package, contenu inchangé.
package agent

import (
	"mikcloud/hotspot-api/internal/model"
	"regexp"
	"strconv"
	"strings"
)

// intervalPattern — durée RouterOS simple (« 45s », « 5m », « 1h », « 2d », « 1w »).
var intervalPattern = regexp.MustCompile(`^[0-9]+[smhdw]$`)

// buildSchedulerAdd — F10 : /system scheduler add name/interval/on-event.
// Le handler a validé le format ; le builder reste défensif (interval hors
// format → 45s, nom assaini via SanitizeName).

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
