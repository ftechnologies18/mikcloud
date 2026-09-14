// QoS Manager N°104 — plafond agrégat du hotspot, automatisé de bout en bout.
//
// PARCOURS : le gérant ouvre la carte QoS d'un routeur → le cloud calcule la
// recommandation (capacité déclarée N°103, sinon enveloppe mesurée, jamais
// inventée) → « Appliquer » pose l'état désiré (cible, limites) → la file
// mikcloud-qos CONVERGE au check-in (pattern walled_garden : create-or-set
// idempotent, relecture de vérification, signature posée seulement si la
// vérité routeur correspond bit à bit) → les files dynamiques des utilisateurs
// deviennent SES enfants via parent-queue (machinerie profile_set existante —
// la mécanique RouterOS propre : l'arbitrage HTB remplace l'ordre fragile de
// la liste des files) → auto-réparation 6 h + monitoring queue_read 30 min
// (dérive détectée = signature vidée = re-file au check-in suivant).
//
// Chaque site a son FAI (N°103) : les limites sont PAR ROUTEUR, la
// recommandation n'est JAMAIS globale.
package api

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// qosRefresh — cadence d'auto-réparation (pattern N°49/80/81) : à
// configuration identique, la file est re-posée au plus tard 6 h après une
// suppression locale (ménage Winbox, restauration de backup).
const qosRefresh = 6 * time.Hour

// qosQueueReadInterval — cadence du monitoring (lecture queue_read) : 30 min
// quand la QoS est active — dérive détectée même sans visite console, stats
// fraîches pour la carte.
const qosQueueReadInterval = 30 * time.Minute

// Bornes des limites (bits/s) : 1 Mbps (une QoS de hotspot plus basse est un
// cap ADSL d'un autre âge) à 10 Gbps (borne cohérente avec la capacité
// déclarée N°103).
const (
	qosMinLimitBps = 1_000_000
	qosMaxLimitBps = 10_000_000_000
)

// ---------------------------------------------------------------------------
// État désiré, signature, fraîcheur
// ---------------------------------------------------------------------------

// qosSig — signature courte et stable d'une config QoS désirée (sel de
// version du script + cible + limites). La signature posée au retour « ok »
// VÉRIFIÉ est comparée à celle-ci à chaque check-in : changer la cible ou les
// limites dans la console re-file la convergence, zéro orchestration.
func qosSig(router *model.Router) string {
	return agent.HashToken(agent.QoSScriptVersion + "|" + router.QoSTarget + "|" +
		strconv.FormatInt(router.QoSMaxUpBps, 10) + "|" + strconv.FormatInt(router.QoSMaxDownBps, 10))[:16]
}

// qosFresh — vrai si la config courante a été CONFIRMÉE appliquée récemment
// (pattern shieldFresh N°81). Vide/illisible → re-file prudent.
func qosFresh(router *model.Router) bool {
	if router.QoSAppliedAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, router.QoSAppliedAt)
	return err == nil && time.Since(t) < qosRefresh
}

// cmdInt64 — entier d'un payload de commande, tolérant aux aller-retours de
// persistance (int64 en mémoire, float64 après un cycle PostgreSQL, chaîne
// après un re-décodage JSON — miroir de agent.plInt64, côté api).
func cmdInt64(payload map[string]any, key string) int64 {
	switch v := payload[key].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	}
	return 0
}

// ---------------------------------------------------------------------------
// Recommandation (capacité déclarée N°103 > enveloppe mesurée > rien)
// ---------------------------------------------------------------------------

// qosRecommendation — la valeur calculée proposée au gérant. Source :
// "declared" (étiquette du forfait, saisie N°103), "measured" (enveloppe
// observée — seulement si `confident`, 3 jours qualifiés), "none" (aucune
// base honnête : rien n'est proposé, jamais inventé).
type qosRecommendation struct {
	Source          string `json:"source"`
	CapacityDownBps int64  `json:"capacityDownBps"`
	CapacityUpBps   int64  `json:"capacityUpBps"`
	MaxDownBps      int64  `json:"maxDownBps"`
	MaxUpBps        int64  `json:"maxUpBps"`
	BurstDownBps    int64  `json:"burstDownBps"`
	BurstUpBps      int64  `json:"burstUpBps"`
	ThrDownBps      int64  `json:"thrDownBps"`
	ThrUpBps        int64  `json:"thrUpBps"`
	MeasuredDays    int    `json:"measuredDays"`
}

// qosRecommended — calcule la recommandation d'un routeur : capacité = débit
// déclarée si présente, sinon enveloppe mesurée (confidente). Règle d'expert
// anti-bufferbloat : max-limit = 95 % de la capacité (le shaper routeur doit
// être LE goulot, jamais la file du FAI), burst = capacité, seuil = 80 % du
// max. Déclarée incomplète (une seule direction) : la direction manquée
// retombe sur la mesure — les deux sources se complètent, la base reste honnête.
func qosRecommended(db *model.DB, router *model.Router, now time.Time) qosRecommendation {
	measDown, measUp, measDays := lineQualityEnvelope(db, router.ID, router.WanIface, now)
	capDown, capUp := router.LineDownBps, router.LineUpBps
	source := "declared"
	if capDown == 0 {
		capDown = measDown
		source = "measured"
	}
	if capUp == 0 {
		capUp = measUp
		source = "measured"
	}
	if capDown < qosMinLimitBps || capUp < qosMinLimitBps {
		return qosRecommendation{Source: "none", MeasuredDays: measDays}
	}
	if capDown > qosMaxLimitBps {
		capDown = qosMaxLimitBps
	}
	if capUp > qosMaxLimitBps {
		capUp = qosMaxLimitBps
	}
	// max = 95 % (entier) ; burst = capacité ; seuil = 80 % du max — les
	// mêmes ratios que QoSDerived côté agent : la recommandation affichée et
	// la file posée ne peuvent pas diverger.
	maxDown, maxUp := capDown*95/100, capUp*95/100
	burstUp, burstDown, thrUp, thrDown := agent.QoSDerived(maxUp, maxDown)
	return qosRecommendation{
		Source: source, CapacityDownBps: capDown, CapacityUpBps: capUp,
		MaxDownBps: maxDown, MaxUpBps: maxUp,
		BurstDownBps: burstDown, BurstUpBps: burstUp,
		ThrDownBps: thrDown, ThrUpBps: thrUp,
		MeasuredDays: measDays,
	}
}

// lineQualityEnvelope — enveloppe mesurée de l'interface WAN (max des max des
// jours ÉCLOS qualifiés : samples ≥ 50, jour courant exclu — mêmes règles que
// GET /api/routers/{id}/line-quality, la source unique de vérité N°103).
// Renvoie aussi le nombre de jours qualifiés (base de `confident` ≥ 3).
func lineQualityEnvelope(db *model.DB, routerID, wan string, now time.Time) (int64, int64, int) {
	if wan == "" {
		return 0, 0, 0
	}
	today := model.LineQualityDayKey(now)
	oldest := now.AddDate(0, 0, -lineQualityWindow).Format("2006-01-02")
	var down, up int64
	days := 0
	for i := range db.LineQuality {
		lq := &db.LineQuality[i]
		if lq.RouterID != routerID || lq.Iface != wan || lq.Day < oldest || lq.Day == today {
			continue
		}
		if lq.Samples < lineQualityMinDaySamples {
			continue
		}
		days++
		if lq.RxMaxBps > down {
			down = lq.RxMaxBps
		}
		if lq.TxMaxBps > up {
			up = lq.TxMaxBps
		}
	}
	return down, up, days
}

// ---------------------------------------------------------------------------
// Rapports agent : lignes de files, vérification
// ---------------------------------------------------------------------------

// qosQueueRow — une file simple RouterOS telle que rapportée (queue_read) ou
// relue après application (queue_ensure). Les limites/débits sont normalisés
// en bps côté cloud (le formatage « 17M » vs « 17000000 » ne compte plus) ;
// les formes brutes restent servies pour l'affichage.
type qosQueueRow struct {
	Name        string `json:"name"`
	Target      string `json:"target"`
	MaxLimit    string `json:"maxLimit"`
	Queue       string `json:"queue"`
	Disabled    bool   `json:"disabled"`
	BytesUp     int64  `json:"bytesUp"`
	BytesDown   int64  `json:"bytesDown"`
	RateUpBps   int64  `json:"rateUpBps"`
	RateDownBps int64  `json:"rateDownBps"`
	MaxUpBps    int64  `json:"maxUpBps"`
	MaxDownBps  int64  `json:"maxDownBps"`
	Dynamic     bool   `json:"dynamic"`
}

// parseQueueRows — rapport « queue|name|target|max-limit|queue|disabled[|bytes|rate[|dynamic]];… ».
// Entrées malformées ignorées (tolérance aux versions d'agent futures).
func parseQueueRows(rows [][]string) []qosQueueRow {
	out := make([]qosQueueRow, 0, len(rows))
	for _, e := range rows {
		if len(e) < 6 || field(e, 0) != "queue" || field(e, 1) == "" {
			continue
		}
		maxUp, maxDown := agent.RosRateBpsDual(field(e, 3))
		row := qosQueueRow{
			Name: field(e, 1), Target: field(e, 2), MaxLimit: field(e, 3),
			Queue: field(e, 4), Disabled: field(e, 5) == "true",
			MaxUpBps: maxUp, MaxDownBps: maxDown,
		}
		if len(e) >= 8 {
			row.BytesUp, row.BytesDown = parseDualInt(field(e, 6))
			row.RateUpBps, row.RateDownBps = agent.RosRateBpsDual(field(e, 7))
		}
		// N°106 — drapeau dynamique rapporté (9e colonne) ; ceinture et
		// bretelles : les files dynamiques RouterOS portent un nom entre
		// chevrons (« <user> ») — un rapport ancien sans la colonne garde
		// ainsi son étiquette honnête (et le ménage ne les propose jamais).
		row.Dynamic = len(e) >= 9 && field(e, 8) == "true"
		if !row.Dynamic && strings.HasPrefix(row.Name, "<") && strings.HasSuffix(row.Name, ">") {
			row.Dynamic = true
		}
		out = append(out, row)
	}
	return out
}

// parseDualInt — paire « a/b » d'entiers (compteurs de bytes des stats).
func parseDualInt(s string) (int64, int64) {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		up, _ := strconv.ParseInt(strings.TrimSpace(s[:i]), 10, 64)
		down, _ := strconv.ParseInt(strings.TrimSpace(s[i+1:]), 10, 64)
		return up, down
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n, 0
}

// qosRowMatches — la file relue correspond-elle EXACTEMENT à l'état désiré
// (cible, limites en bps, types PCQ par défaut, active) ?
func qosRowMatches(row qosQueueRow, target string, maxUp, maxDown int64) bool {
	return row.Name == agent.QoSQueueName &&
		row.Target == target &&
		row.MaxUpBps == maxUp && row.MaxDownBps == maxDown &&
		row.Queue == agent.QoSQueueTypes &&
		!row.Disabled
}

// qosEnsureVerified — le retour « ok » d'un queue_ensure porte la RELECTURE
// routeur : la signature ne sera posée que si cette vérité correspond au
// payload émis (pattern SafeWiFi N°80 : la foi n'est pas une preuve).
func qosEnsureVerified(cmd *model.Command, data string) bool {
	target, _ := cmd.Payload["target"].(string)
	maxUp, maxDown := cmdInt64(cmd.Payload, "maxUpBps"), cmdInt64(cmd.Payload, "maxDownBps")
	for _, row := range parseQueueRows(splitAgentList(data)) {
		if qosRowMatches(row, target, maxUp, maxDown) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Convergence au check-in (pattern walled_garden / shield / familyguard)
// ---------------------------------------------------------------------------

// ensureQoSLocked — sous verrou, depuis handleAgentCmd :
//   - QoS active : sig différente (jamais appliquée, config changée, ou
//     auto-réparation 6 h expirée) → queue_ensure en file (une seule en vol) ;
//     sig fraîche → monitoring : queue_read si la dernière lecture date de
//     plus de 30 min (dérive + stats, même sans console ouverte) ;
//   - QoS désactivée mais déjà APPLIQUÉE (QoSAppliedAt posé) → queue_remove
//     en file jusqu'au retour confirmant la disparition ;
//   - jamais utilisée et désactivée : RIEN (économie de veille N°75 — un
//     routeur dont le gérant n'ouvre jamais la carte QoS ne consomme rien).
func (a *API) ensureQoSLocked(db *model.DB, router *model.Router) {
	if router.Mode != "agent" {
		return
	}
	if !router.QoSEnabled {
		// Retrait convergent : une file posée doit disparaître, même si le
		// DELETE est arrivé pendant un check-in manqué.
		if router.QoSAppliedAt == "" {
			return
		}
		if qosCommandInFlight(db, router.ID, model.CmdQueueRemove) {
			return
		}
		queueCommandLocked(db, router.AccountID, router.ID, model.CmdQueueRemove, map[string]any{})
		return
	}
	sig := qosSig(router)
	if router.QoSSig != sig || !qosFresh(router) {
		if qosCommandInFlight(db, router.ID, model.CmdQueueEnsure) {
			return
		}
		burstUp, burstDown, thrUp, thrDown := agent.QoSDerived(router.QoSMaxUpBps, router.QoSMaxDownBps)
		queueCommandLocked(db, router.AccountID, router.ID, model.CmdQueueEnsure, map[string]any{
			"target": router.QoSTarget, "sig": sig,
			"maxUpBps": router.QoSMaxUpBps, "maxDownBps": router.QoSMaxDownBps,
			"burstUpBps": burstUp, "burstDownBps": burstDown,
			"thrUpBps": thrUp, "thrDownBps": thrDown,
		})
		return
	}
	// Appliqué et frais : monitoring périodique (lecture seule, idempotente —
	// la reprise zombie N°73 peut la re-jouer sans dommage).
	if qosQueueReadStale(db, router.ID, time.Now().UTC()) &&
		!qosCommandInFlight(db, router.ID, model.CmdQueueRead) {
		queueCommandLocked(db, router.AccountID, router.ID, model.CmdQueueRead, map[string]any{})
	}
}

// qosCommandInFlight — une commande du kind déjà en file/en vol pour ce
// routeur (pas d'accumulation pendant que le rapport est en chemin).
func qosCommandInFlight(db *model.DB, routerID, kind string) bool {
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == routerID && c.Kind == kind && (c.Status == "queued" || c.Status == "sent") {
			return true
		}
	}
	return false
}

// qosQueueReadStale — dernière queue_read APPLIQUÉE (done) plus vieille que
// la cadence de monitoring (aucune → stale ; le check-in suivant en pose une).
func qosQueueReadStale(db *model.DB, routerID string, now time.Time) bool {
	var best string
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID != routerID || c.Kind != model.CmdQueueRead || c.Status != "done" {
			continue
		}
		if c.DoneAt > best {
			best = c.DoneAt
		}
	}
	if best == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, best)
	return err != nil || now.Sub(t) >= qosQueueReadInterval
}

// qosAttachProfilesLocked — rattachement des files dynamiques des
// utilisateurs : ParentQueue de CHAQUE profil du compte passe à mikcloud-qos
// (une seule valeur de file parent par profil, parité Mikhmon) et la
// machinerie profile_set existante converge l'état complet vers les routeurs.
// appelée au retour VÉRIFIÉ du queue_ensure — la file existe AVANT que les
// commandes profile_set ne partent (l'ordre est garanti par la causalité,
// pas par la chance).
//
// ARBITRAGE (documenté) : le champ Profile.ParentQueue est ACCOUNT-level
// (parité Mikhmon, poussé à tous les routeurs agents du compte). Le
// rattachement automatique n'a lieu que si le compte n'a QU'UN SEUL routeur
// agent — sur un parc multi-box, la file parent reste un choix du gérant dans
// le formulaire profil (le nom mikcloud-qos apparaît dans le sélecteur dès
// que la file existe : read_resources). Un parent-queue pointant une file
// inexistante sur une AUTRE box serait du bruit silencieux, jamais une
// décision prise à son insu.
func (a *API) qosAttachProfilesLocked(db *model.DB, router *model.Router, queue string) {
	agentRouters := 0
	for i := range db.Routers {
		if db.Routers[i].AccountID == router.AccountID && db.Routers[i].Mode == "agent" {
			agentRouters++
		}
	}
	if agentRouters != 1 {
		return
	}
	rewired := 0
	for i := range db.Profiles {
		p := &db.Profiles[i]
		if p.AccountID == router.AccountID && p.ParentQueue != queue {
			p.ParentQueue = queue
			a.queueProfileSetLocked(db, router.AccountID, *p)
			rewired++
		}
	}
	if rewired > 0 {
		a.logActivity(db, router.AccountID, "router", "QoS : "+strconv.Itoa(rewired)+
			" profil(s) rattaché(s) à la file "+queue+" (files utilisateurs filles du plafond agrégat)")
	}
}

// qosDetachProfilesLocked — miroir du retrait : la file parent des profils
// redevient vide (parent-queue=none côté routeur via profile_set), mêmes
// gardes (compte mono-routeur agent uniquement).
func (a *API) qosDetachProfilesLocked(db *model.DB, router *model.Router, queue string) {
	agentRouters := 0
	for i := range db.Routers {
		if db.Routers[i].AccountID == router.AccountID && db.Routers[i].Mode == "agent" {
			agentRouters++
		}
	}
	if agentRouters != 1 {
		return
	}
	for i := range db.Profiles {
		p := &db.Profiles[i]
		if p.AccountID == router.AccountID && p.ParentQueue == queue {
			p.ParentQueue = ""
			a.queueProfileSetLocked(db, router.AccountID, *p)
		}
	}
}

// ---------------------------------------------------------------------------
// Routes console
// ---------------------------------------------------------------------------

// handleRouterQoSGet — GET /api/routers/{id}/qos : l'état complet de la carte
// QoS — config courante + signature, recommandation (N°103), files du routeur
// (cache queue_read ≤ 120 s, lecture à la demande sinon ; simulé → lignes
// déterministes). real → 400 (contrat des lectures télémétriques).
func (a *API) handleRouterQoSGet(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	now := time.Now().UTC()

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	routerCopy := *rr
	rec := qosRecommended(db, rr, now)
	burstUp, burstDown, thrUp, thrDown := agent.QoSDerived(routerCopy.QoSMaxUpBps, routerCopy.QoSMaxDownBps)

	// Files : simulé → déterministes ; agent → cache/à-la-demande (outils F9).
	queues := map[string]any{"queued": false, "data": []qosQueueRow{}, "updatedAt": ""}
	switch routerCopy.Mode {
	case "simulated":
		queues["data"] = qosSimulatedRows(&routerCopy)
		queues["updatedAt"] = model.NowISO()
	case "agent":
		// N°106 — un retrait de file (agrégat OU ménage legacy) terminé
		// APRÈS la dernière lecture rend le cache obsolète : la table doit
		// refléter la vérité routeur, pas un cliché antérieur au geste du
		// gérant → re-lecture posée, la table revient au check-in suivant.
		cutoff := lastQueueRemovalDoneAt(db, id)
		if fresh := freshToolCommand(db, id, model.CmdQueueRead, now); fresh != nil && (cutoff == "" || fresh.DoneAt > cutoff) {
			raw, _ := fresh.Result["data"].(string)
			queues["data"] = parseQueueRows(splitAgentList(raw))
			queues["updatedAt"] = fresh.DoneAt
		} else if !pendingToolCommand(db, id, model.CmdQueueRead) {
			queueCommandLocked(db, acc, id, model.CmdQueueRead, map[string]any{})
			a.store.Save()
			queues["queued"] = true
		} else {
			queues["queued"] = true
		}
	}
	a.store.Save()
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"routerId":   id,
		"queueName":  agent.QoSQueueName,
		"queueTypes": agent.QoSQueueTypes,
		"burstTime":  agent.QoSBurstTime,
		"status": map[string]any{
			"enabled":      routerCopy.QoSEnabled,
			"target":       routerCopy.QoSTarget,
			"maxUpBps":     routerCopy.QoSMaxUpBps,
			"maxDownBps":   routerCopy.QoSMaxDownBps,
			"burstUpBps":   burstUp,
			"burstDownBps": burstDown,
			"thrUpBps":     thrUp,
			"thrDownBps":   thrDown,
			"applied":      routerCopy.QoSSig != "",
			"appliedAt":    routerCopy.QoSAppliedAt,
			// Désactivée mais file encore présente : le retrait converge.
			"removalPending": !routerCopy.QoSEnabled && routerCopy.QoSAppliedAt != "",
		},
		"recommendation": rec,
		"queues":         queues,
	})
}

// qosSimulatedRows — files déterministes d'un routeur simulé : la file
// agrégat telle que configurée (si active) + deux files dynamiques
// d'utilisateurs (le rendu de la carte en démo, sans matériel).
func qosSimulatedRows(rr *model.Router) []qosQueueRow {
	rows := []qosQueueRow{}
	if rr.QoSEnabled {
		maxUp, maxDown := rr.QoSMaxUpBps, rr.QoSMaxDownBps
		if maxUp < qosMinLimitBps || maxDown < qosMinLimitBps {
			maxUp, maxDown = 5_000_000, 50_000_000
		}
		rows = append(rows, qosQueueRow{
			Name: agent.QoSQueueName, Target: rr.QoSTarget,
			MaxLimit: strconv.FormatInt(maxUp, 10) + "/" + strconv.FormatInt(maxDown, 10),
			Queue:    agent.QoSQueueTypes,
			MaxUpBps: maxUp, MaxDownBps: maxDown,
			// Débit sous le plafond (la démo ne « sature » pas en continu).
			RateUpBps: maxUp / 3, RateDownBps: maxDown * 7 / 10,
			BytesUp: 18_442_133_000, BytesDown: 341_006_918_000,
		})
		rows = append(rows, qosQueueRow{
			Name: "<D-SC-A21B>", Target: rr.QoSTarget, Queue: "default-small/default",
			Dynamic: true, RateUpBps: maxUp / 8, RateDownBps: maxDown / 5,
			MaxUpBps: maxUp, MaxDownBps: maxDown,
		}, qosQueueRow{
			Name: "<D-SC-F44E>", Target: rr.QoSTarget, Queue: "default-small/default",
			Dynamic: true, RateUpBps: maxUp / 12, RateDownBps: maxDown / 6,
			MaxUpBps: maxUp, MaxDownBps: maxDown,
		})
	}
	return rows
}

// handleRouterQoSPut — PUT /api/routers/{id}/qos {enabled, target, maxUpBps,
// maxDownBps} : pose l'état désiré. La convergence (file, signature,
// rattachement des profils) suit au check-in — ≤ 45 s console ouverte, la
// commande est AUSSI enfilée immédiatement (geste explicite du gérant).
func (a *API) handleRouterQoSPut(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Enabled    *bool   `json:"enabled"`
		Target     *string `json:"target"`
		MaxUpBps   *int64  `json:"maxUpBps"`
		MaxDownBps *int64  `json:"maxDownBps"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	// Champs optionnels (PATCH sémantique) : absents = inchangés.
	target := rr.QoSTarget
	if req.Target != nil {
		t, ok := normalizeCIDR(*req.Target)
		if !ok {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Cible invalide (CIDR IPv4 attendu, ex. 192.168.10.0/24)")
			return
		}
		target = t
	}
	maxUp, maxDown := rr.QoSMaxUpBps, rr.QoSMaxDownBps
	if req.MaxUpBps != nil {
		maxUp = *req.MaxUpBps
	}
	if req.MaxDownBps != nil {
		maxDown = *req.MaxDownBps
	}
	enabled := rr.QoSEnabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if enabled {
		if target == "" {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Cible requise (sous-réseau hotspot, ex. 192.168.10.0/24)")
			return
		}
		if maxUp < qosMinLimitBps || maxUp > qosMaxLimitBps || maxDown < qosMinLimitBps || maxDown > qosMaxLimitBps {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Limites invalides (1 Mbps à 10 Gbps par direction, en bits/s)")
			return
		}
	}
	rr.QoSEnabled = enabled
	rr.QoSTarget = target
	rr.QoSMaxUpBps = maxUp
	rr.QoSMaxDownBps = maxDown
	rr.QoSSig = "" // l'état désiré change : la convergence re-file
	if !enabled {
		// Retrait immédiat des rattachements de profils (mono-box) + file.
		a.qosDetachProfilesLocked(db, rr, agent.QoSQueueName)
		if rr.Mode == "agent" && rr.QoSAppliedAt != "" && !qosCommandInFlight(db, rr.ID, model.CmdQueueRemove) {
			queueCommandLocked(db, acc, rr.ID, model.CmdQueueRemove, map[string]any{})
		}
		a.logActivityBy(r, db, acc, "router", "QoS désactivée sur «"+rr.Name+"» — retrait de la file en cours")
	} else {
		if rr.Mode == "agent" && !qosCommandInFlight(db, rr.ID, model.CmdQueueEnsure) {
			burstUp, burstDown, thrUp, thrDown := agent.QoSDerived(maxUp, maxDown)
			queueCommandLocked(db, acc, rr.ID, model.CmdQueueEnsure, map[string]any{
				"target": target, "sig": qosSig(rr),
				"maxUpBps": maxUp, "maxDownBps": maxDown,
				"burstUpBps": burstUp, "burstDownBps": burstDown,
				"thrUpBps": thrUp, "thrDownBps": thrDown,
			})
		}
		a.logActivityBy(r, db, acc, "router", "QoS activée sur «"+rr.Name+"» — file "+agent.QoSQueueName+
			" ("+strconv.FormatInt(maxDown/1_000_000, 10)+"M/"+strconv.FormatInt(maxUp/1_000_000, 10)+"M max)"+qosConvergenceHint(rr))
	}
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": enabled})
}

// handleRouterQoSDelete — DELETE /api/routers/{id}/qos : désactivation
// (la config est conservée — un ré-allumage retrouve ses valeurs).
func (a *API) handleRouterQoSDelete(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	rr.QoSEnabled = false
	rr.QoSSig = ""
	a.qosDetachProfilesLocked(db, rr, agent.QoSQueueName)
	if rr.Mode == "agent" && rr.QoSAppliedAt != "" && !qosCommandInFlight(db, rr.ID, model.CmdQueueRemove) {
		queueCommandLocked(db, acc, rr.ID, model.CmdQueueRemove, map[string]any{})
	}
	a.logActivityBy(r, db, acc, "router", "QoS désactivée sur «"+rr.Name+"» — retrait de la file en cours")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// normalizeCIDR — CIDR IPv4 canonique (« 192.168.10.0/24 »). Refuse IPv6,
// les réseaux d'hôte étranges restent acceptés (le gérant sait ce qu'il
// bride — la file cible CE qu'on lui donne).
func normalizeCIDR(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	ip, ipnet, err := net.ParseCIDR(s)
	if err != nil || ip.To4() == nil {
		return "", false
	}
	return ipnet.String(), true
}

// qosConvergenceHint — libellé du délai de convergence pour le journal.
func qosConvergenceHint(rr *model.Router) string {
	if rr.Mode != "agent" {
		return ""
	}
	return " — convergence au prochain check-in (≤ 45 s)"
}

// ---------------------------------------------------------------------------
// N°106 — Ménage à distance des files legacy
// ---------------------------------------------------------------------------

// queueRemoveNameInFlight — un queue_remove visant CE nom déjà en file/en
// vol (payload « name », absent = mikcloud-qos) : pas d'accumulation quand
// le gérant re-clique pendant la convergence. Contrairement au garde du
// QoS Manager (kind seul), deux retraits de noms DIFFÉRENTS coexistent —
// chacun est un geste ponctuel, tous convergent.
func queueRemoveNameInFlight(db *model.DB, routerID, name string) bool {
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID != routerID || c.Kind != model.CmdQueueRemove || (c.Status != "queued" && c.Status != "sent") {
			continue
		}
		if agent.QueueRemoveTarget(c.Payload) == name {
			return true
		}
	}
	return false
}

// lastQueueRemovalDoneAt — DoneAt du queue_remove le plus récent pour ce
// routeur (mikcloud-qos OU ménage legacy N°106) : "" si aucun. Sert de
// coupe d'obsolescence au cache queue_read de la carte (un retrait terminé
// depuis la dernière lecture = table périmée).
func lastQueueRemovalDoneAt(db *model.DB, routerID string) string {
	var best string
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID != routerID || c.Kind != model.CmdQueueRemove || c.Status != "done" {
			continue
		}
		if c.DoneAt > best {
			best = c.DoneAt
		}
	}
	return best
}

// handleRouterQueueDelete — N°106 — DELETE /api/routers/{id}/queues/{name}
// (rang 2) : ménage à distance d'une file statique LEGACY (posée à la main
// avant le QoS Manager : HOTSPOT-Total, GLOBAL-Internet…). Le gérant n'est
// pas sur site et ses clients non plus : la table des files de l'onglet QoS
// lui montre la vérité RouterOS, CE geste retire celle qu'il désigne —
// détachement des profils qui la référencent puis retrait au check-in
// (≤ 45 s), disparition prouvée par le compte restant rapporté.
//
// Gardes-fous : mikcloud-qos refusé (son retrait passe par « Désactiver la
// QoS », qui pilote AUSSI l'état cloud — sig, appliedAt, profils) ; noms
// dynamiques refusés (files des utilisateurs, recréées par les rate-limits
// — les supprimer n'a aucun sens) ; charset strict partagé avec le builder
// (un payload corrompu échoue, ne vise jamais une autre file).
func (a *API) handleRouterQueueDelete(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	name := r.PathValue("name")
	if !agent.ValidQueueName(name) {
		writeErr(w, http.StatusBadRequest,
			"Nom de file invalide (1 à 64 caractères : lettres, chiffres, espaces, - _ . ; les files dynamiques des utilisateurs ne se suppriment pas)")
		return
	}
	if name == agent.QoSQueueName {
		writeErr(w, http.StatusBadRequest,
			"La file mikcloud-qos se retire par « Désactiver la QoS » (le cloud pilote aussi son état)")
		return
	}
	// N°106 quota (fusion) — les files mikthrottle-<user> sont posées et
	// retirées par le scheduler mikcloud-quota DU ROUTEUR (tick 20 s) :
	// les supprimer à la main débriderait un utilisateur dont le quota
	// est épuisé jusqu'au tick suivant — pour rien (elle revient). Le
	// ménage ne touche qu'aux files LEGACY posées à la main.
	if strings.HasPrefix(name, agent.QuotaThrottlePrefix) {
		writeErr(w, http.StatusBadRequest,
			"Les files mikthrottle- sont pilotées par le bridage quota du routeur (scheduler mikcloud-quota) : changez le mode du profil ou attendez la fin de session")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode == "real" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, realModeUnsupported)
		return
	}
	if rr.Mode == "agent" {
		// Détacher d'abord les profils du compte qui référencent la file
		// (miroir du retrait QoS — mono-routeur agent : sur un parc
		// multi-box, le champ account-level reste un choix du gérant dans
		// le formulaire profil, jamais une décision prise à son insu).
		a.qosDetachProfilesLocked(db, rr, name)
		if !queueRemoveNameInFlight(db, rr.ID, name) {
			queueCommandLocked(db, acc, rr.ID, model.CmdQueueRemove, map[string]any{"name": name})
		}
		a.logActivityBy(r, db, acc, "router", "File «"+name+"» retirée de «"+rr.Name+
			"» — ménage à distance (disparition au check-in, ≤ 45 s)")
	}
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
