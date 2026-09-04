// Package api — application des résultats remontés par l'agent sur l'état
// (POST /agent/result) : télémétrie système et diff de sessions/logs
// (applyReadState), trafic (applyAgentTraffic, parseIfaceCounters,
// addUserBytes), usage vouchers (markVoucherUsed), uptime, et utilitaires de
// parsing des rapports agent (splitAgentList, parseInt64,
// normalizePingResult pour GET /api/commands/{id}).
package api

import (
	"fmt"
	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// applyReadState — applique la télémétrie + sync users/sessions d'un routeur agent.
//
// P1 (audit Mikhmon) :
//   - F8 : board / freehdd / totalhdd (Mo, divisés côté script) →
//     Router.BoardName / FreeHddMb / TotalHddMb ;
//   - F6 : ifaces=name:rx:tx;… → diff des compteurs cumulés avec l'état
//     précédent (db.Traffic) → débits par interface + point d'historique ;
//   - F3 : diff des sessions avant/après remplacement → UserLogs login/logout
//     (un utilisateur déjà présent n'est pas re-journalisé) ;
//   - Suivi data : bytes-in/out des sessions actives cumulés par deltas dans
//     les compteurs du user (BytesIn/BytesOut — miroir du limit-bytes-total).
func (a *API) applyReadState(db *model.DB, router *model.Router, vals url.Values) {
	if v := strings.TrimSpace(vals.Get("version")); v != "" {
		if len(v) > 32 {
			v = v[:32]
		}
		router.Version = v
	}
	if up := parseRosUptime(vals.Get("uptime")); up > 0 {
		router.UptimeSec = up
	}
	if cpu, err := strconv.Atoi(vals.Get("cpu")); err == nil && cpu >= 0 && cpu <= 100 {
		router.CPULoad = cpu
	}
	// F8 — carte + disque (Mo ; division octets→Mo faite côté script).
	if v := strings.TrimSpace(vals.Get("board")); v != "" && len(v) <= 64 {
		router.BoardName = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(vals.Get("freehdd"))); err == nil && v >= 0 && v <= 1<<20 {
		router.FreeHddMb = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(vals.Get("totalhdd"))); err == nil && v >= 0 && v <= 1<<20 {
		router.TotalHddMb = v
	}

	// Users : "name|profile|disabled;…"
	userEntries := splitAgentList(vals.Get("users"))
	router.HotspotUsers = len(userEntries)

	// Sync des utilisateurs créés hors MikCloud (inconnus du cloud)
	known := map[string]bool{}
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].RouterID == router.ID {
			known[strings.ToLower(db.HotspotUsers[i].Username)] = true
		}
	}
	onRouter := map[string]bool{} // usernames rapportés par CE read_state
	for _, e := range userEntries {
		name := agent.SanitizeName(e[0])
		if name == "" || name == "-" || known[strings.ToLower(name)] {
			if name != "" && name != "-" {
				onRouter[strings.ToLower(name)] = true
			}
			continue
		}
		profName := ""
		disabled := false
		if len(e) > 1 {
			profName = e[1]
		}
		if len(e) > 2 && e[2] == "true" {
			disabled = true
		}
		db.HotspotUsers = append(db.HotspotUsers, model.HotspotUser{
			ID: model.NewID("u-"), AccountID: router.AccountID, Kind: "regular", Username: name,
			ProfileName: profName, RouterID: router.ID, RouterName: router.Name,
			Status:    map[bool]string{true: "disabled", false: "active"}[disabled],
			CreatedAt: model.NowISO(),
		})
		onRouter[strings.ToLower(name)] = true
	}

	// N (rapprochement doux) — utilisateurs du cloud absents de CE read_state :
	// marqués « absent du routeur » (badge + action de resynchronisation).
	// Le cloud reste le registre durable : RIEN n'est supprimé automatiquement.
	// Garde-fous anti-faux-positifs :
	//   - statut actif ou disabled uniquement (used/expired = absence attendue) ;
	//   - grâce de 2 minutes après création (commande user_add encore en file) ;
	//   - le badge se lève tout seul au read_state suivant si l'utilisateur
	//     réapparaît (recréation manuelle dans Winbox, par exemple).
	grace := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.RouterID != router.ID {
			continue
		}
		if u.Status != "active" && u.Status != "disabled" {
			continue
		}
		if onRouter[strings.ToLower(u.Username)] {
			if u.MissingOnRouter {
				u.MissingOnRouter = false // réapparu — badge levé
			}
			continue
		}
		if u.CreatedAt > grace {
			continue // trop récent : la commande d'ajout peut être en file
		}
		if !u.MissingOnRouter {
			u.MissingOnRouter = true
		}
	}

	// Sessions actives : "user|ip|uptime|bytes-in|bytes-out;…" (script v3).
	sessEntries := splitAgentList(vals.Get("sessions"))
	router.ActiveSessions = len(sessEntries)
	userIDs := map[string]string{}
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].RouterID == router.ID {
			userIDs[strings.ToLower(db.HotspotUsers[i].Username)] = db.HotspotUsers[i].ID
		}
	}

	// F3 — capture des sessions présentes AVANT remplacement (diff), par
	// username : un profil shared-users > 1 autorise PLUSIEURS sessions
	// simultanées du même utilisateur — chacune doit conserver son identité
	// (sinon la 2e session change d'ID à chaque poll : bruit de journal).
	prevByUser := map[string][]model.Session{}
	for _, s := range db.Sessions {
		if s.RouterID == router.ID {
			prevByUser[s.Username] = append(prevByUser[s.Username], s)
		}
	}

	live := []model.Session{}
	matched := []bool{} // appariée à une session précédente ? (aligné avec live)
	for _, e := range sessEntries {
		s := model.Session{ID: model.NewID("s-"), AccountID: router.AccountID, Username: e[0], RouterID: router.ID, RouterName: router.Name}
		if len(e) > 1 {
			s.IP = e[1]
		}
		if len(e) > 2 {
			s.UptimeSec = parseRosUptime(e[2])
		}
		// P — bytes-in/bytes-out rapportés par le script read_state v3.
		if len(e) > 3 {
			s.BytesIn = parseInt64(e[3])
		}
		if len(e) > 4 {
			s.BytesOut = parseInt64(e[4])
		}
		if id, ok := userIDs[strings.ToLower(e[0])]; ok {
			s.UserID = id
			for i := range db.HotspotUsers {
				if db.HotspotUsers[i].ID == id {
					s.ProfileName = db.HotspotUsers[i].ProfileName
					break
				}
			}
		}
		// P (fix session flip) — appariement FIFO : chaque session live
		// reprend l'identité (ID + StartedAt) d'une session précédente du
		// même username ; le read_state met à jour uptime et bytes mais ne
		// reset PAS l'identité de la session. C'est ce qui empêche le
		// clignotement « connexion → déconnexion » à chaque poll.
		if lst := prevByUser[s.Username]; len(lst) > 0 {
			prev := lst[0]
			prevByUser[s.Username] = lst[1:]
			s.ID = prev.ID
			s.StartedAt = prev.StartedAt
			matched = append(matched, true)
			// Suivi data user-level — cumul par DELTAS entre read_state successifs
			// (bytes-in/bytes-out d'une session active RouterOS
			// sont cumulatifs depuis le login). Les octets gagnent le
			// compteur du user EN LIVE (session de plusieurs jours suivie
			// côté cloud), en miroir du limit-bytes-total appliqué par le
			// routeur. Garde anti-régression : compteur décroissant
			// (reset-counters) → delta 0.
			if s.UserID != "" && (s.BytesIn > prev.BytesIn || s.BytesOut > prev.BytesOut) {
				addUserBytes(db, s.UserID,
					max(int64(0), s.BytesIn-prev.BytesIn),
					max(int64(0), s.BytesOut-prev.BytesOut))
			}
		} else {
			s.StartedAt = model.NowISO()
			matched = append(matched, false)
			// Nouvelle session : ses octets déjà accumulés à la 1re
			// observation (RouterOS compte depuis le login) entrent
			// intégralement au compteur du user — les deltas des polls
			// suivants complètent le cumul jusqu'à la déconnexion.
			if s.UserID != "" && (s.BytesIn > 0 || s.BytesOut > 0) {
				addUserBytes(db, s.UserID, s.BytesIn, s.BytesOut)
			}
		}
		live = append(live, s)
	}

	// F3 — nouvelles sessions (non appariées) → login ; sessions précédentes
	// non retrouvées → logout.
	now := time.Now().UTC()
	for i := range live {
		if !matched[i] {
			logRouterUserEvent(db, router, live[i], "login", now)
			a.markVoucherUsed(db, router, live[i], now) // statut dynamique : 1re connexion = utilisé (+ vente auto)
		}
	}
	for _, lst := range prevByUser {
		for _, s := range lst {
			logRouterUserEvent(db, router, s, "logout", now)
			accumulateUptime(db, s) // quota temps : cumul de la session terminée
			// NB — pas de cumul de BYTES ici : les compteurs data user-level
			// sont déjà alimentés EN LIVE par deltas (addUserBytes) ; un
			// cumul au logout doublerait le comptage.
		}
	}

	kept := db.Sessions[:0]
	for _, s := range db.Sessions {
		if s.RouterID != router.ID {
			kept = append(kept, s)
		}
	}
	db.Sessions = append(kept, live...)

	// F6 — trafic : diff des compteurs cumulés par interface.
	applyAgentTraffic(db, router, vals.Get("ifaces"), now)
}

// logRouterUserEvent — journalise un login/logout détecté par diff de sessions
// (F3, applyReadState — mode agent). MAC inconnue du rapport : laissée vide.
func logRouterUserEvent(db *model.DB, router *model.Router, s model.Session, action string, now time.Time) {
	if db.UserLogs == nil {
		db.UserLogs = []model.UserLog{}
	}
	db.UserLogs = append(db.UserLogs, model.UserLog{
		ID:         model.NewID("ul-"),
		AccountID:  router.AccountID,
		UserID:     s.UserID,
		Username:   s.Username,
		Action:     action,
		RouterID:   router.ID,
		RouterName: router.Name,
		IP:         s.IP,
		MAC:        s.MAC,
		At:         now.UTC().Format(time.RFC3339),
	})
}

// markVoucherUsed — statut dynamique : au PREMIER login détecté (diff de
// sessions du read_state), le voucher est marqué « utilisé » (UsedAt horodaté
// + statut stocké used). Les utilisateurs réguliers (kind regular, miroir du
// routeur) ne sont pas marqués. À appeler sous verrou.
//
// UX R4 — vente AUTOMATIQUE à la connexion : le revendeur remet le ticket au
// client (papier imprimé ou code dicté) sans repasser par l'app ; quand le
// client SE CONNECTE pour la première fois, la vente se confirme toute
// seule — décompte du stock (SoldAt), trace DISTINCTE d'une vente tactile
// (SoldVia=auto_connect), créance dépôt-vente au prix gros (règle N°19 :
// elle naît à la remise, la connexion en est la preuve), rapport de journée.
// Idempotent par SoldAt : un ticket déjà vendu (tactile ou papier) n'est
// jamais recompté, et un voucher hors stock revendeur n'est pas une vente.
func (a *API) markVoucherUsed(db *model.DB, router *model.Router, s model.Session, now time.Time) {
	if s.UserID == "" {
		return
	}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.ID == s.UserID && u.Kind == "voucher" {
			if u.UsedAt == "" {
				u.UsedAt = now.UTC().Format(time.RFC3339)
				model.AnchorVoucherValidity(db, u, now) // validité ancrée au 1er login
			}
			if u.Status == "active" {
				u.Status = "used"
			}
			// Vente auto — réservée au stock revendeur, jamais recomptée.
			if u.ResellerID != "" && u.SoldAt == "" && u.Status != "disabled" {
				u.SoldAt = now.UTC().Format(time.RFC3339)
				u.SoldVia = "auto_connect"
				if u.CreditSale {
					db.Transactions = append([]model.Transaction{{
						ID: model.NewID("tx-"), AccountID: router.AccountID, Type: "debt",
						ResellerID: u.ResellerID, ResellerName: u.ResellerName,
						Amount: u.Price,
						Note:   fmt.Sprintf("Créance : voucher %s connecté par le client (vente auto)", u.Username),
						At:     u.SoldAt,
					}}, db.Transactions...)
				}
				a.logActivity(db, router.AccountID, "voucher",
					"Voucher "+u.Username+" connecté par le client — vente automatique (stock revendeur décompté)")
			}
			return
		}
	}
}

// accumulateUptime — cumule l'uptime de la session terminée dans
// uptimeUsedSec du user (quota temps « limit-uptime » : le routeur applique
// la coupure, le cloud reflète le cumul pour l'état « expiré — temps épuisé »).
// À appeler sous verrou, une seule fois par session terminée (diff logout).
func accumulateUptime(db *model.DB, s model.Session) {
	if s.UserID == "" || s.UptimeSec <= 0 {
		return
	}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.ID != s.UserID {
			continue
		}
		total := u.UptimeUsedSec + s.UptimeSec
		// Parité limit-uptime : le routeur coupe EXACTEMENT à la limite, mais
		// le dernier échantillon lu avant la disparition de la session peut
		// lui manquer de ≤ 1 intervalle de lecture (45 s). Dans cette fenêtre,
		// la déconnexion observée EST l'épuisement du quota — on aligne le
		// cumul sur la limite, sinon le voucher resterait « utilisé » pour
		// toujours (cf. TimeLimitParityGraceSec).
		if u.Kind == "voucher" && u.TimeLimitMin > 0 {
			if limit := u.TimeLimitMin * 60; total >= limit-model.TimeLimitParityGraceSec {
				total = limit
			}
		}
		u.UptimeUsedSec = total
		return
	}
}

// addUserBytes — cumule les octets observés sur une session (deltas entre
// read_state successifs, mode agent) dans les compteurs data du user.
// Miroir cloud du limit-bytes-total appliqué par le routeur (DataQuotaMb) :
// le suivi data reste lisible côté cloud même quand la session dure plusieurs
// jours. À appeler sous verrou, par deltas uniquement (jamais un cumul
// absolu — une session déjà comptée ne doit pas l'être deux fois).
func addUserBytes(db *model.DB, userID string, dIn, dOut int64) {
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].ID == userID {
			db.HotspotUsers[i].BytesIn += dIn
			db.HotspotUsers[i].BytesOut += dOut
			return
		}
	}
}

// parseIfaceCounters — décode « name:rx:tx;… » (compteurs cumulés en octets,
// rapport read_state v2). Les entrées malformées sont ignorées.
func parseIfaceCounters(raw string) []model.IfaceTraffic {
	out := []model.IfaceTraffic{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	for _, item := range strings.Split(raw, ";") {
		if strings.TrimSpace(item) == "" {
			continue
		}
		parts := strings.Split(item, ":")
		if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" {
			continue
		}
		rx, err1 := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		tx, err2 := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
		if err1 != nil || err2 != nil || rx < 0 || tx < 0 {
			continue
		}
		out = append(out, model.IfaceTraffic{Name: strings.TrimSpace(parts[0]), RxBytes: rx, TxBytes: tx})
	}
	return out
}

// applyAgentTraffic — F6 : calcule les débits par interface à partir du diff
// des compteurs cumulés (« delta octets × 8 / delta temps », RxBps/TxBps sont
// en bits/s — aligné sur le seed et formatBitsPerSec du front) :
//   - première mesure  → compteurs stockés, débits 0 (pas de référence) ;
//   - interface inconnue ou compteur décroissant (reboot) → débit 0 ;
//   - sinon            → RxBps/TxBps + TrafficPoint (somme des interfaces)
//     ajouté à History (cap 60).
//
// UpdatedAt est stocké en RFC3339Nano : deux read_state peuvent être très
// rapprochés (test/poll rapide) — la précision sub-seconde évite un delta nul.
func applyAgentTraffic(db *model.DB, router *model.Router, raw string, now time.Time) {
	ifaces := parseIfaceCounters(raw)
	if len(ifaces) == 0 {
		return
	}
	var tr *model.RouterTraffic
	for i := range db.Traffic {
		if db.Traffic[i].RouterID == router.ID {
			tr = &db.Traffic[i]
			break
		}
	}
	if tr == nil {
		if db.Traffic == nil {
			db.Traffic = []model.RouterTraffic{}
		}
		db.Traffic = append(db.Traffic, model.RouterTraffic{
			ID: router.ID, RouterID: router.ID, AccountID: router.AccountID,
			UpdatedAt:  now.UTC().Format(time.RFC3339Nano),
			Interfaces: ifaces,
			History:    []model.TrafficPoint{},
		})
		return // première mesure : référence posée, débits à 0
	}

	dt := 0.0
	if prev, err := time.Parse(time.RFC3339, tr.UpdatedAt); err == nil {
		if d := now.Sub(prev).Seconds(); d > 0 {
			dt = d
		}
	}
	prevIface := make(map[string]model.IfaceTraffic, len(tr.Interfaces))
	for _, it := range tr.Interfaces {
		prevIface[it.Name] = it
	}
	out := make([]model.IfaceTraffic, 0, len(ifaces))
	var sumRx, sumTx int64
	for _, it := range ifaces {
		rxBps, txBps := int64(0), int64(0)
		if dt > 0 {
			if p, ok := prevIface[it.Name]; ok {
				if it.RxBytes > p.RxBytes {
					rxBps = int64(float64(it.RxBytes-p.RxBytes) * 8 / dt)
				}
				if it.TxBytes > p.TxBytes {
					txBps = int64(float64(it.TxBytes-p.TxBytes) * 8 / dt)
				}
			}
		}
		out = append(out, model.IfaceTraffic{
			Name: it.Name, RxBytes: it.RxBytes, TxBytes: it.TxBytes, RxBps: rxBps, TxBps: txBps,
		})
		sumRx += rxBps
		sumTx += txBps
	}
	tr.Interfaces = out
	tr.History = append(tr.History, model.TrafficPoint{
		T: now.UTC().Format(time.RFC3339), RxBps: sumRx, TxBps: sumTx,
	})
	if len(tr.History) > 60 {
		tr.History = tr.History[len(tr.History)-60:]
	}
	tr.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
}

// normalizePingResult — F8 : le rapport agent arrive en valeurs formulaire
// (chaînes « sent=4&received=4&minMs=12… ») ; le front (toPingStats) attend
// des NOMBRES + lossPct. Conversion ici, à l'arrivée du rapport, pour que
// GET /api/commands/{id} serve un result directement consommable.
func normalizePingResult(res map[string]any) {
	if res == nil {
		return
	}
	for _, k := range []string{"sent", "received", "minMs", "avgMs", "maxMs"} {
		switch v := res[k].(type) {
		case string:
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				res[k] = n
			}
		case float64:
			res[k] = int64(v)
		}
	}
	sent, okSent := res["sent"].(int64)
	recv, okRecv := res["received"].(int64)
	if okSent && okRecv && sent > 0 {
		loss := (sent - recv) * 100 / sent
		if loss < 0 {
			loss = 0
		}
		res["lossPct"] = loss
	}
}

// splitAgentList — découpe "a|b|c;d|e|f;…" en entrées.
func splitAgentList(raw string) [][]string {
	out := [][]string{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	for _, item := range strings.Split(raw, ";") {
		if strings.TrimSpace(item) == "" {
			continue
		}
		parts := strings.Split(item, "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		out = append(out, parts)
	}
	return out
}

// parseInt64 — parse tolérant d'un entier 64 bits (bytes-in/bytes-out des sessions).
func parseInt64(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}
	var n int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int64(c-'0')
		}
	}
	return n
}
