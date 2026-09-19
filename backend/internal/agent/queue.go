// QoS Manager N°104 — file agrégat du hotspot (/queue simple), builders RouterOS.
//
// PRINCIPE (analyse d'expert router) : une SEULE file statique
// « mikcloud-qos » cible le sous-réseau hotspot, max-limit upload/download,
// types PCQ PAR DÉFAUT de RouterOS (pcq-upload-default classifier
// src-address / pcq-download-default classifier dst-address, pcq-rate=0 :
// partage équitable des sous-flux). Les files DYNAMIQUES des utilisateurs
// (posées par les rate-limit des user profiles) deviennent SES ENFANTS via
// parent-queue : l'arbitrage HTB natif remplace l'ordre fragile de la liste
// — le plafond agrégat s'applique VRAIMENT, les limites par utilisateur
// vivent dedans. Paramètres duaux orientés UPLOAD d'abord (convention
// RouterOS, colonnes Winbox : max-limit=up/down).
//
// VALEURS EN BPS BRUTS : max-limit/burst acceptent des entiers (17000000) —
// aucun formatage « 17M » à l'émission, donc aucune ambiguïté de suffixe ;
// la RELECTURE routeur, elle, revient formatée (« 17M », « 12.5M »…) : le
// cloud normalise chaque côté en bps (RosRateBps) avant de comparer —
// la vérification est bit à bit, pas textuelle.
package agent

import (
	"fmt"
	"strconv"
	"strings"

	"mikcloud/hotspot-api/internal/model"
)

// QoSQueueName — nom canonique de la file agrégat MikCloud (marquage
// mikcloud- des objets posés sur le routeur, miroir safewifi/shield/pool).
const QoSQueueName = "mikcloud-qos"

// QoSQueueTypes — types PCQ par défaut de RouterOS, upload d'abord.
const QoSQueueTypes = "pcq-upload-default/pcq-download-default"

// QoSScriptVersion — sel de version de la FORME du bloc QoS (pattern
// familyGuardRulesVersion N°82) : toute évolution du script (paramètre
// supplémentaire, disposition différente) change ce sel → tout le parc
// équipé reçoit la nouvelle forme au check-in suivant.
const QoSScriptVersion = "qos-v1"

// QoSBurstTime — fenêtre de burst (les deux côtés, upload d'abord).
const QoSBurstTime = "10s/10s"

// QoSComment — commentaire de la file (lisible dans Winbox).
const QoSComment = "mikcloud: agregat hotspot (QoS Manager)"

// ValidQueueName — N°106 — nom de file statique acceptable pour le ménage
// à distance : 1 à 64 caractères parmi lettres, chiffres, espace, « - »,
// « _ », « . » ; pas d'espace en tête ni en queue. Les noms dynamiques
// RouterOS (« <user> ») sont refusés par construction (charset sans
// chevrons) : ces files appartiennent aux utilisateurs hotspot et sont
// recréées par les rate-limits — les supprimer n'a aucun sens. Partagé
// handler (rejet en 400) et builder (rejet en erreur) : les deux côtés
// valident le même contrat.
func ValidQueueName(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	if s[0] == ' ' || s[len(s)-1] == ' ' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == ' ', c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}

// QueueRemoveTarget — nom visé par un queue_remove (payload « name »,
// absent = file agrégat mikcloud-qos). Partagé builder et traitement des
// rapports : les deux côtés parlent du même nom, jamais de constantes
// dupliquées.
func QueueRemoveTarget(payload map[string]any) string {
	if n, _ := payload["name"].(string); n != "" {
		return n
	}
	return QoSQueueName
}

// QoSDerived — burst et seuil dérivés des limites (mêmes ratios que la
// recommandation manuelle du runbook : max-limit = 95 % de la capacité →
// capacité = max×20/19 = burst ; seuil = 80 % du max — la file devient le
// goult déterministe, jamais la file du FAI : anti-bufferbloat).
//
// En entiers purs (pas de float) : déterministe au filage, dans les tests et
// à la re-vérification.
func QoSDerived(maxUpBps, maxDownBps int64) (burstUpBps, burstDownBps, thrUpBps, thrDownBps int64) {
	burstUpBps = maxUpBps * 20 / 19
	burstDownBps = maxDownBps * 20 / 19
	thrUpBps = maxUpBps * 4 / 5
	thrDownBps = maxDownBps * 4 / 5
	return
}

// RosRateBps — valeur de débit RouterOS → bits/s. Accepte les formes
// formatées (« 17M », « 12.5M », « 512k », « 1.5G », suffixe « bps » ou
// « B ») comme les entiers bruts (« 17000000 ») ; « unlimited » et les
// formes vides/inconnues → 0 (jamais d'erreur : une relecture muette reste
// une absence de preuve, pas un mensonge).
func RosRateBps(s string) int64 {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimSuffix(s, "bps")
	s = strings.TrimSpace(s)
	if s == "" || s == "unlimited" || s == "0" {
		return 0
	}
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "g"):
		mult, s = 1_000_000_000, strings.TrimSuffix(s, "g")
	case strings.HasSuffix(s, "m"):
		mult, s = 1_000_000, strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "k"):
		mult, s = 1_000, strings.TrimSuffix(s, "k")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Décimal (« 12.5M ») : fraction bornée à 3 chiffres (RouterOS n'affiche
	// jamais plus — la valeur reste exacte, le calcul reste en entiers).
	intPart, fracPart := s, ""
	if i := strings.IndexAny(s, ".,"); i >= 0 {
		intPart, fracPart = s[:i], s[i+1:]
		if len(fracPart) > 3 {
			fracPart = fracPart[:3]
		}
	}
	parseUint := func(str string) int64 {
		if str == "" {
			return 0
		}
		var n int64
		for _, c := range str {
			if c < '0' || c > '9' {
				return -1
			}
			n = n*10 + int64(c-'0')
		}
		return n
	}
	base := parseUint(intPart)
	frac := parseUint(fracPart)
	if base < 0 || frac < 0 {
		return 0
	}
	// (base.frac) × mult : la fraction est décalée d'abord, le suffixe ensuite —
	// l'ordre évite toute division entière intermédiaire.
	shift := int64(1)
	for range fracPart {
		shift *= 10
	}
	return (base*shift + frac) * mult / shift
}

// RosRateBpsDual — paire « a/b » (upload/download) → (up, down). Une seule
// valeur → elle vaut pour l'upload, download 0.
func RosRateBpsDual(s string) (int64, int64) {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return RosRateBps(s[:i]), RosRateBps(s[i+1:])
	}
	return RosRateBps(s), 0
}

// buildQueueEnsure — N°104 — create-or-set idempotent de la file agrégat,
// puis RELECTURE de vérification (vérité routeur : le script ne rapporte
// pas ce qu'il a envoyé, mais ce que /queue simple contient APRÈS —
// target|max-limit|queue|disabled, cf. queueReadLine).
func (b Builder) buildQueueEnsure(cmd model.Command) string {
	target := plStr(cmd.Payload, "target")
	maxUp := plInt64(cmd.Payload, "maxUpBps")
	maxDown := plInt64(cmd.Payload, "maxDownBps")
	burstUp := plInt64(cmd.Payload, "burstUpBps")
	burstDown := plInt64(cmd.Payload, "burstDownBps")
	thrUp := plInt64(cmd.Payload, "thrUpBps")
	thrDown := plInt64(cmd.Payload, "thrDownBps")
	// Défense en profondeur : le handler valide, le builder borne (des
	// limites absurdes ne doivent jamais atteindre le routeur, même via un
	// payload corrompu).
	if maxUp < 1_000_000 || maxUp > 10_000_000_000 {
		maxUp = 1_000_000
	}
	if maxDown < 1_000_000 || maxDown > 10_000_000_000 {
		maxDown = 1_000_000
	}
	if burstUp < maxUp || burstUp > 10_000_000_000 {
		burstUp, _, _, _ = QoSDerived(maxUp, maxDown)
	}
	if burstDown < maxDown || burstDown > 10_000_000_000 {
		_, burstDown, _, _ = QoSDerived(maxUp, maxDown)
	}
	if thrUp < 1_000_000 || thrUp > maxUp {
		thrUp = maxUp * 4 / 5
	}
	if thrDown < 1_000_000 || thrDown > maxDown {
		thrDown = maxDown * 4 / 5
	}
	target = sanitizeCIDRList(target)
	limits := strconv.FormatInt(maxUp, 10) + "/" + strconv.FormatInt(maxDown, 10)
	burst := strconv.FormatInt(burstUp, 10) + "/" + strconv.FormatInt(burstDown, 10)
	thr := strconv.FormatInt(thrUp, 10) + "/" + strconv.FormatInt(thrDown, 10)

	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n:local rdata \"\"\n")
	sb.WriteString(":do {\n")
	sb.WriteString("  :local mkq [/queue simple find where name=\"" + QoSQueueName + "\"]\n")
	sb.WriteString("  :if ([:len $mkq] > 0) do={\n")
	sb.WriteString("    /queue simple set $mkq target=" + target +
		" max-limit=" + limits + " queue=" + QoSQueueTypes +
		" burst-limit=" + burst + " burst-threshold=" + thr +
		" burst-time=" + QoSBurstTime + " disabled=no\n")
	sb.WriteString("  } else={\n")
	sb.WriteString("    /queue simple add name=\"" + QoSQueueName + "\" target=" + target +
		" max-limit=" + limits + " queue=" + QoSQueueTypes +
		" burst-limit=" + burst + " burst-threshold=" + thr +
		" burst-time=" + QoSBurstTime + " comment=\"" + rosEscape(QoSComment) + "\"\n")
	sb.WriteString("  }\n")
	// Relecture : ce que le routeur a VRAIMENT (target, max-limit, types,
	// disabled) — la vérification cloud normalise en bps avant comparaison.
	sb.WriteString("  :do {\n")
	sb.WriteString("    :set mkq [/queue simple find where name=\"" + QoSQueueName + "\"]\n")
	sb.WriteString("    :if ([:len $mkq] > 0) do={\n")
	sb.WriteString(queueReadLine("mkq"))
	sb.WriteString("    }\n")
	sb.WriteString("  } on-error={}\n")
	sb.WriteString("} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.fetchResultData(cmd.ID, okVar))
	return sb.String()
}

// rosQueueTargetCSV — N°159 — lit la cible d'une file et la rend en liste
// À VIRGULES dans la variable $qtc : RouterOS imprime une cible MULTIPLE en
// liste à points-virgules (« 11.11.11.0/24;10.77.0.0/21 ») — or « ; » est
// LE séparateur d'entrées du protocole de rapport : la ligne
// « queue|mikcloud-qos|11.11.11.0/24;10.77.0.0/21|… » se scindait en deux
// fragments malformés que parseQueueRows rejetait (len<6 / tag absent) →
// la relecture de vérification n'aboutissait JAMAIS pour une file
// multi-cibles : signature jamais posée, re-file à chaque check-in
// (production 19/09 : Benie wifi et ProMax WIFI, ~1 630 queue_ensure
// « done » non vérifiés en 12 h — 27 % du volume agent — pendant que le
// mono-cible CYBER S.C convergeait). Le get renvoie un TABLEAU pour une
// cible multiple (typeof array) : il est rejoint à virgules ; une cible
// simple reste la chaîne telle quelle.
func rosQueueTargetCSV(varName string) string {
	return ":local qtg [/queue simple get $" + varName + " target]\n" +
		":local qtc [:tostr $qtg]\n" +
		":if ([:typeof $qtg] = \"array\") do={\n" +
		"  :set qtc \"\"\n" +
		"  :foreach t in=$qtg do={\n" +
		"    :if ([:len $qtc] > 0) do={ :set qtc ($qtc . \",\") }\n" +
		"    :set qtc ($qtc . [:tostr $t])\n" +
		"  }\n" +
		"}\n"
}

// queueReadLine — concatène dans $rdata la ligne de relecture d'une file :
// « queue|name|target|max-limit|queue|disabled; ». La cible est normalisée
// EN VIRGULES (rosQueueTargetCSV, N°159) : le « ; » séparateur de RouterOS
// scindait la ligne pour une cible multiple. Les stats (bytes/rate)
// suivent le même format via buildQueueRead (7 colonnes : +bytes|rate) —
// le parseur cloud tolère les deux profondeurs.
func queueReadLine(varName string) string {
	return rosQueueTargetCSV(varName) +
		"      :set rdata ($rdata . \"queue|\" . \"" + QoSQueueName + "\" . \"|\" ." +
		" $qtc . \"|\" ." +
		" [:tostr [/queue simple get $" + varName + " max-limit]] . \"|\" ." +
		" [:tostr [/queue simple get $" + varName + " queue]] . \"|\" ." +
		" [:tostr [/queue simple get $" + varName + " disabled]] . \";\")\n"
}

// buildQueueRead — N°104 — lecture des files simples du routeur : identité
// (nom, cible, limites, types, disabled) + stats (bytes, rate — formats
// RouterOS, normalisés côté cloud) + drapeau dynamique (N°106 — étiquette
// honnête des files des utilisateurs et garde-fou du ménage : jamais
// proposées à la suppression). Cap 60 files (le read_resources déjà
// en pose 60 : mêmes bornes), les DYNAMIQUES incluses — c'est leur
// agrégat dans le parent qui intéresse la carte QoS.
func (b Builder) buildQueueRead(cmd model.Command) string {
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n:local rdata \"\"\n")
	sb.WriteString(":do {\n  :local qn 0\n")
	sb.WriteString("  :foreach qe in=[/queue simple find] do={\n")
	sb.WriteString("    :if ($qn < 60) do={\n")
	sb.WriteString("      :local qnm [:tostr [/queue simple get $qe name]]\n")
	// N°159 — la cible est lue PUIS normalisée en virgules : une cible
	// multiple scindait la ligne de rapport (cf. rosQueueTargetCSV) —
	// le monitoring queue_read vidait alors la signature d'une file
	// pourtant conforme (aucune ligne ne matchait).
	sb.WriteString("      " + rosQueueTargetCSV("qe"))
	sb.WriteString("      :local qml [:tostr [/queue simple get $qe max-limit]]\n")
	sb.WriteString("      :local qtp [:tostr [/queue simple get $qe queue]]\n")
	sb.WriteString("      :local qds [:tostr [/queue simple get $qe disabled]]\n")
	sb.WriteString("      :local qdy \"false\"\n      :do { :set qdy [:tostr [/queue simple get $qe dynamic]] } on-error={ :set qdy \"false\" }\n")
	sb.WriteString("      :local qbs \"\"\n      :do { :set qbs [:tostr [/queue simple get $qe bytes]] } on-error={ :set qbs \"\" }\n")
	sb.WriteString("      :local qrt \"\"\n      :do { :set qrt [:tostr [/queue simple get $qe rate]] } on-error={ :set qrt \"\" }\n")
	// Précédent read_resources (parité Mikhmon) : les noms de files RouterOS
	// sont lus tels quels — le protocole d'agent du projet fait confiance à
	// cette source depuis la vague F9 (aucun assainissement des noms non plus
	// côté read_resources).
	sb.WriteString(`      :set rdata ($rdata . "queue|" . $qnm . "|" . $qtc . "|" . $qml . "|" . $qtp . "|" . $qds . "|" . $qbs . "|" . $qrt . "|" . $qdy . ";")` + "\n")
	sb.WriteString("      :set qn ($qn + 1)\n")
	sb.WriteString("    }\n  }\n")
	sb.WriteString("} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.fetchResultData(cmd.ID, okVar))
	return sb.String()
}

// buildQueueRemove — N°104/N°106 — retrait propre : détacher d'abord les
// profils hotspot qui référencent la file (parent-queue=none), puis retirer
// la file. Rapporte le nombre de files du même nom RESTANTES (0 attendu —
// vérité routeur, pas la foi en un remove silencieux).
//
// N°106 « ménage à distance » : payload « name » optionnel — sans lui (et
// pour la convergence QoS), la cible reste la file agrégat mikcloud-qos ;
// avec lui, c'est une file LEGACY posée à la main (ex. HOTSPOT-Total avant
// le QoS Manager) que le gérant retire DEPUIS MikCloud, sans être sur site.
// Le nom est validé strictement : un payload corrompu fait ÉCHOUER la
// commande (statut error, message clair) plutôt que de viser une autre
// file — jamais de repli vers mikcloud-qos (ce serait supprimer la
// MAUVAISE file).
func (b Builder) buildQueueRemove(cmd model.Command) (string, error) {
	name := QueueRemoveTarget(cmd.Payload)
	if !ValidQueueName(name) {
		return "", fmt.Errorf("nom de file invalide : %q", name)
	}
	esc := rosEscape(name)
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	sb.WriteString(":local " + okVar + " true\n:local rdata \"\"\n")
	sb.WriteString(":do {\n")
	sb.WriteString("  :do { /ip hotspot user profile set [find where parent-queue=\"" + esc + "\"] parent-queue=none } on-error={}\n")
	sb.WriteString("  :do { /queue simple remove [find where name=\"" + esc + "\"] } on-error={ :set " + okVar + " false }\n")
	sb.WriteString("  :local mkleft 0\n")
	sb.WriteString("  :do { :set mkleft [/queue simple print count-only where name=\"" + esc + "\"] } on-error={ :set mkleft 0 }\n")
	sb.WriteString(`  :set rdata ($rdata . "removed|" . [:tostr $mkleft] . ";")` + "\n")
	sb.WriteString("} on-error={ :set " + okVar + " false }\n")
	sb.WriteString(b.fetchResultData(cmd.ID, okVar))
	return sb.String(), nil
}

// sanitizeCIDRPart — N°110 — UN préfixe d'une liste de cibles : charset
// CIDR strict (chiffres, « . », « / »), ≤ 18 caractères, slash requis.
// "" si l'élément ne s'y prête pas (distinguer « vide » de « défaut » :
// la liste doit savoir qu'un élément a été REJETÉ, pas le remplacer
// silencieusement par un autre sous-réseau — ce serait brider le MAUVAIS).
func sanitizeCIDRPart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 18 || !strings.Contains(s, "/") {
		return ""
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c == '.' || c == '/') {
			return ""
		}
	}
	return s
}

// sanitizeCIDRList — N°110 — cible simple OU multiple (« 192.168.10.0/24,
// 10.77.0.0/21 ») : un hotspot dont le pool a été étendu (docteur N°108,
// range dédié 10.77.0.0/21) vit sur des sous-réseaux DISJOINTS — aucun
// préfixe unique ne les couvre, RouterOS accepte une liste de cibles dans
// UNE seule file. 1 à 4 éléments, séparateur virgule (jamais un caractère
// d'injection : le split précède le filtre charset). Un seul élément
// invalide fait retomber TOUTE la cible sur le défaut historique — la
// défense en profondeur reste un repli franc, jamais un demi-bridage.
func sanitizeCIDRList(s string) string {
	parts := strings.Split(s, ",")
	if len(parts) < 1 || len(parts) > 4 {
		return "192.168.88.0/24"
	}
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		c := sanitizeCIDRPart(p)
		if c == "" {
			return "192.168.88.0/24"
		}
		clean = append(clean, c)
	}
	return strings.Join(clean, ",")
}
