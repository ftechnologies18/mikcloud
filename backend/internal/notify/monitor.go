// Moniteur MikCloud — goroutine de surveillance multi-tenant.
//
// Toutes les 30 s, sous verrou du store :
//   - routeurs agents dont le dernier check-in dépasse la fenêtre
//     (OfflineAfterSec, défaut 135 s = 3 × 45 s) → bascule « offline » +
//     notification (rappel toutes les 30 min tant que la panne dure) ;
//   - retour de check-in après une panne notifiée → notification « de retour
//     en ligne » (la transition online est faite par touchAgent) ;
//   - stock de vouchers vendables par routeur < seuil → alerte (anti-spam :
//     une notification par passage d'état low/empty, mémorisé en base) ;
//   - rapport journalier à l'heure configurée (UTC = Abidjan GMT+0).
//
// Les envois réseau ne sont JAMAIS faits sous verrou : les notifications sont
// collectées puis délivrées après déverrouillage (les logs sont réécrits sous
// verrou ensuite). Les comptes désactivés ne reçoivent rien.
package notify

import (
	"log"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

const (
	pollEvery       = 30 * time.Second // cadence du moniteur
	reminderEvery   = 30 * time.Minute // rappel d'une panne en cours
	keepLogsPerAcc  = 200              // entrées d'historique conservées par compte
	defaultCurrency = "FCFA"
)

// Service — état interne du moniteur (transitoire : reconstruit au démarrage).
type Service struct {
	st *store.Store
	// notifiedOffline — routerID → dernier envoi de notification de panne.
	notifiedOffline map[string]time.Time
	// N°99 — autoPoolMarked — routerID → dernier état de pression pool pour
	// lequel l'auto-réparation a déjà été marquée (anti-boucle : une marque
	// par TRANSITION, indépendante de PoolAlertState qui n'est suivi que si
	// les notifications du compte sont actives). Transitoire : après un
	// redémarrage du cloud, une pression toujours haute re-marque UNE fois —
	// borné, jamais de file inondée (leçon N°97-ter).
	autoPoolMarked map[string]string
}

// NewService crée le moniteur attaché au store.
func NewService(st *store.Store) *Service {
	return &Service{st: st, notifiedOffline: map[string]time.Time{}, autoPoolMarked: map[string]string{}}
}

// Run lance la boucle de surveillance (à appeler dans une goroutine).
// N°74 — chaque tick est protégé : une panique de surveillance est journalisée
// et la boucle REPART au tick suivant (avant : mort du moniteur à vie, et le
// verrou du store restait pris si la panique était entre Lock et Unlock).
func (s *Service) Run() {
	time.Sleep(5 * time.Second) // laisser le serveur HTTP démarrer proprement
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("moniteur : panique récupérée (reprise au tick suivant) : %v\n%s", r, debug.Stack())
				}
			}()
			s.tick()
		}()
		time.Sleep(pollEvery)
	}
}

// outboxItem — notification collectée sous verrou, délivrée hors verrou.
// N°74 — avant, l'envoi réseau (Deliver) était appelé PENDANT la collecte sous
// verrou : un canal injoignable (SMTP muet ~2 min, Telegram en timeout 12 s)
// gelait TOUTE l'API (check-ins agents, claims WiFi publics, consoles) le
// temps de la tentative. La structure ne portait que les logs déjà écrits ;
// elle porte désormais le message À envoyer, la délivrance a lieu après le
// déverrouillage.
type outboxItem struct {
	cfg   model.NotificationSettings
	kind  string
	title string
	body  string
}

// tick — un passage complet de surveillance : collecte sous verrou (avec
// libération garantie par defer), puis délivrance réseau HORS verrou.
func (s *Service) tick() {
	outbox, platformEmail := s.collect(time.Now().UTC())
	// Délivrance hors verrou (réseau), puis écriture de l'historique.
	// N°150 : platformEmail = relais du compte principal, résolu sous verrou
	// pendant la collecte — l'envoi peut durer, il part APRÈS déverrouillage.
	for i := range outbox {
		logs := DeliverWithPlatform(&outbox[i].cfg, platformEmail, outbox[i].kind, outbox[i].title, outbox[i].body, "")
		s.appendLogs(logs)
	}
}

// collect — phase sous verrou du passage de surveillance : décisions
// (offline/rappels/stock/rapport) et constitution de l'outbox. Le verrou est
// libéré par un defer — il survit même à une panique interne (N°74).
// N°150 : retourne aussi les réglages e-mail du compte principal (relais
// plateforme) pour la délivrance — un compte sans SMTP/Resend propre mais
// avec l'alerte e-mail active reçoit désormais ses envois via la plateforme.
func (s *Service) collect(now time.Time) ([]outboxItem, *model.NotificationSettings) {
	var outbox []outboxItem

	s.st.Lock()
	defer s.st.Unlock()
	db := s.st.Data()
	changed := false
	platformEmail := platformEmailLocked(db)

	// Statut des comptes SaaS (les comptes désactivés ne reçoivent rien).
	accDisabled := map[string]bool{}
	for _, a := range db.Accounts {
		if a.Status == "disabled" {
			accDisabled[a.ID] = true
		}
	}

	// 1) Routeurs agents : auto-mark offline / rappel / retour en ligne.
	for i := range db.Routers {
		r := &db.Routers[i]
		if r.Mode == "simulated" || r.LastSeen == "" {
			continue // simulated : piloté par le moteur de démo ; jamais vu : pas d'alerte
		}
		seen, err := time.Parse(time.RFC3339, r.LastSeen)
		if err != nil {
			continue
		}
		if accDisabled[r.AccountID] {
			continue
		}
		cfg := store.GetOrCreateNotifSettings(db, r.AccountID)
		// N°75 — seuil élargi au régime de veille adaptative : 3 × le pas
		// du scheduler (un routeur endormi à 180 s serait sinon marqué hors
		// ligne entre deux check-ins — fausse alerte à chaque sieste).
		expired := now.Sub(seen) > r.EffectiveOfflineAfter(cfg.OfflineAfterSec)

		if r.Status == "online" {
			if expired {
				// Transition en ligne → hors ligne (valeur affichée par la console).
				r.Status = "offline"
				changed = true
				s.notifiedOffline[r.ID] = now
				// N°148 — la TRANSITION est journalisée dans le
				// compte d'activité (la cloche) : c'est un événement
				// réel, contrairement aux synchros de routine qui
				// n'y écrivent plus. Les comptes désactivés ne
				// polluent pas leur propre journal pour autant.
				if !accDisabled[r.AccountID] {
					away0 := now.Sub(seen).Round(time.Minute)
					model.AppendActivity(db, model.Activity{
						AccountID: r.AccountID, Type: "router",
						Message: "Routeur «" + r.Name + "» hors ligne — sans check-in depuis " + formatDuration(away0),
					})
				}
				// N°150 — les identifiants plateforme (bot FTCI, relais e-mail du
				// compte principal) comptent pour la décision d'alerte.
				if cfg.Enabled && HasAnyChannelWithPlatform(&cfg, platformEmail) {
					away := now.Sub(seen).Round(time.Minute)
					outbox = append(outbox, outboxItem{cfg: cfg, kind: KindRouterOffline,
						title: "🔴 Routeur hors ligne — " + r.Name,
						body: "Le routeur «" + r.Name + "» (" + routerSiteLabel(r) + ") ne répond plus depuis " +
							formatDuration(away) + ".\nDernier contact : " + formatTimeAbidjan(seen) + ".\n" +
							"Les clients ne peuvent plus se connecter : vérifiez l'alimentation, la liaison Internet ou le scheduler mikcloud."})
				}
			}
			continue
		}

		// Statut « offline » : rappel périodique tant que la panne dure.
		if last, ok := s.notifiedOffline[r.ID]; ok && now.Sub(last) >= reminderEvery {
			s.notifiedOffline[r.ID] = now
			if cfg.Enabled && HasAnyChannelWithPlatform(&cfg, platformEmail) {
				away := now.Sub(seen).Round(time.Minute)
				outbox = append(outbox, outboxItem{cfg: cfg, kind: KindRouterOffline,
					title: "⏳ Toujours hors ligne — " + r.Name,
					body: "Le routeur «" + r.Name + "» est toujours injoignable depuis " +
						formatDuration(away) + ". Pensez à prévenir les revendeurs du site."})
			}
		}
	}

	// 2) Retours en ligne (touchAgent a repassé le statut à « online »).
	if len(s.notifiedOffline) > 0 {
		for i := range db.Routers {
			r := &db.Routers[i]
			if _, wasNotified := s.notifiedOffline[r.ID]; !wasNotified {
				continue
			}
			if r.Status != "online" {
				continue
			}
			delete(s.notifiedOffline, r.ID)
			if accDisabled[r.AccountID] {
				continue
			}
			// N°148 — retour en ligne : la transition symétrique est
			// journalisée aussi (le gérant voit la panne ET la fin de
			// panne dans sa cloche, comme dans ses canaux).
			model.AppendActivity(db, model.Activity{
				AccountID: r.AccountID, Type: "router",
				Message: "Routeur «" + r.Name + "» de retour en ligne (check-in reçu)",
			})
			changed = true // l'entrée d'activité doit être persistée
			cfg := store.GetOrCreateNotifSettings(db, r.AccountID)
			if cfg.Enabled && HasAnyChannelWithPlatform(&cfg, platformEmail) {
				outbox = append(outbox, outboxItem{cfg: cfg, kind: KindRouterBack,
					title: "🟢 Routeur de retour en ligne — " + r.Name,
					body: "Le routeur «" + r.Name + "» (« " + routerSiteLabel(r) + " ») répond de nouveau " +
						"(check-in reçu à " + formatTimeAbidjan(now) + "). Tout est normal."})
			}
		}
	}

	// 3) Stock de vouchers vendables par routeur (sites ayant déjà eu des vouchers).
	stock := map[string][2]int{} // routerID → [vendables, déjà-approvisionné (0/1)]
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.Kind != "voucher" {
			continue
		}
		var cur [2]int
		if v, ok := stock[u.RouterID]; ok {
			cur = v
		}
		cur[1] = 1 // au moins un voucher existe sur ce site
		if u.Status == "active" && (u.ExpiresAt == "" || func() bool {
			exp, err := time.Parse(time.RFC3339, u.ExpiresAt)
			return err != nil || exp.After(now)
		}()) {
			cur[0]++
		}
		stock[u.RouterID] = cur
	}
	setStockState := func(acc, routerID, state string) {
		if db.NotifSettings == nil {
			db.NotifSettings = map[string]model.NotificationSettings{}
		}
		st0 := db.NotifSettings[acc]
		if st0.StockAlertState == nil {
			st0.StockAlertState = map[string]string{}
		}
		if state == "" {
			delete(st0.StockAlertState, routerID)
		} else {
			st0.StockAlertState[routerID] = state
		}
		db.NotifSettings[acc] = st0
	}
	for i := range db.Routers {
		r := &db.Routers[i]
		if accDisabled[r.AccountID] {
			continue
		}
		info, ok := stock[r.ID]
		if !ok || info[1] == 0 {
			continue // aucun voucher jamais généré sur ce site : pas d'alerte
		}
		cfg := store.GetOrCreateNotifSettings(db, r.AccountID)
		if !cfg.Enabled {
			continue
		}
		state := "ok"
		if info[0] == 0 {
			state = "empty"
		} else if info[0] < cfg.LowStockThreshold {
			state = "low"
		}
		prev := cfg.StockAlertState[r.ID]
		if state == prev || (state != "low" && state != "empty") {
			if state == "ok" && prev != "" {
				setStockState(r.AccountID, r.ID, "")
				changed = true
			}
			continue
		}
		setStockState(r.AccountID, r.ID, state)
		changed = true
		if !cfg.Enabled || !HasAnyChannelWithPlatform(&cfg, platformEmail) {
			continue
		}
		title, body := stockMessage(r, info[0], state, cfg.LowStockThreshold)
		outbox = append(outbox, outboxItem{cfg: cfg, kind: KindLowStock, title: title, body: body})
	}

	// 3-b) N°97 — occupation du pool d'adresses IP du hotspot : l'épuisement
	// (« cannot assign ip address - no more free addresses from pool ») frappe
	// les clients PAYANTS aux heures de pointe. PoolCap (posé par le docteur
	// pool, auto-diagnostiqué au check-in) + PoolHosts (rafraîchi par chaque
	// read_state) → état ok / high (≥ 80 %) / full (≥ 95 %), même mécanique
	// anti-spam que le stock : une notification par transition, mémorisée en
	// base. Capacité inconnue (PoolCap=0 : routeur jamais diagnostiqué) →
	// aucune alerte (jamais de pourcentage inventé).
	setPoolState := func(acc, routerID, state string) {
		if db.NotifSettings == nil {
			db.NotifSettings = map[string]model.NotificationSettings{}
		}
		st0 := db.NotifSettings[acc]
		if st0.PoolAlertState == nil {
			st0.PoolAlertState = map[string]string{}
		}
		if state == "" {
			delete(st0.PoolAlertState, routerID)
		} else {
			st0.PoolAlertState[routerID] = state
		}
		db.NotifSettings[acc] = st0
	}
	for i := range db.Routers {
		r := &db.Routers[i]
		if accDisabled[r.AccountID] || r.PoolCap <= 0 {
			continue
		}
		cfg := store.GetOrCreateNotifSettings(db, r.AccountID)
		usage := 0
		if r.PoolHosts > 0 {
			usage = r.PoolHosts * 100 / r.PoolCap
			if usage > 100 {
				usage = 100
			}
		}
		state := ""
		switch {
		case usage >= 95:
			state = "full"
		case usage >= 80:
			state = "high"
		}

		// N°99 — auto-réparation opt-in du pool : quand la pression FRANCHIT un
		// seuil (transition, pas état) sur un routeur agent dont le gérant a
		// activé le switch, le recyclage des IP zombies part SANS geste humain.
		// Marquage (PoolAutoPending) consommé au check-in par
		// ensurePoolDoctorLocked — jamais l'extension (geste topologique).
		// Indépendant des canaux de notification : l'auto-réparation est un
		// réglage DÉDIÉ du routeur ; sans canal, l'action a lieu et reste
		// tracée (journal d'activité au filage). Anti-boucle : une marque par
		// transition (mémoire propre, la file N°97-ter reste protégée).
		if state != "" && r.PoolAuto && r.Mode == "agent" {
			if s.autoPoolMarked[r.ID] != state {
				s.autoPoolMarked[r.ID] = state
				if !r.PoolAutoPending {
					r.PoolAutoPending = true
					changed = true
				}
				if cfg.Enabled && HasAnyChannelWithPlatform(&cfg, platformEmail) {
					title, body := poolAutoMessage(r, usage, r.PoolHosts, r.PoolCap, state)
					outbox = append(outbox, outboxItem{cfg: cfg, kind: KindPoolAuto, title: title, body: body})
				}
			}
		} else if state == "" {
			delete(s.autoPoolMarked, r.ID) // retour au calme : la prochaine montée re-marquera
		}

		if !cfg.Enabled {
			continue // N°97 : le suivi d'alerte suit le réglage des notifications
		}
		prev := cfg.PoolAlertState[r.ID]
		if state == prev {
			continue
		}
		setPoolState(r.AccountID, r.ID, state)
		changed = true
		if state == "" || !HasAnyChannelWithPlatform(&cfg, platformEmail) {
			continue
		}
		title, body := poolMessage(r, usage, r.PoolHosts, r.PoolCap, state)
		outbox = append(outbox, outboxItem{cfg: cfg, kind: KindPoolAlert, title: title, body: body})
	}

	// 4) Rapport journalier (heure UTC = heure d'Abidjan, GMT+0 sans DST).
	today := now.Format("2006-01-02")
	for i := range db.Accounts {
		acc := &db.Accounts[i]
		if accDisabled[acc.ID] {
			continue
		}
		// N°154 — le compte principal n'est pas un client SaaS : ni routeurs,
		// ni ventes, ni stock de vouchers — son rapport quotidien serait vide
		// (bruit quotidien dans la boîte du propriétaire). Même discipline que
		// les annonces N°152 : le principal parle, il n'est pas destinataire.
		if acc.ID == model.AccountMainID {
			continue
		}
		cfg := store.GetOrCreateNotifSettings(db, acc.ID)
		if !cfg.Enabled || !cfg.DailyReport || cfg.LastReportDate == today {
			continue
		}
		if now.Hour() < cfg.ReportHour {
			continue
		}
		title, body := buildDailyReport(db, acc.ID, now, cfg)
		cfg.LastReportDate = today
		store.SetNotifSettings(db, cfg)
		changed = true
		if HasAnyChannelWithPlatform(&cfg, platformEmail) {
			outbox = append(outbox, outboxItem{cfg: cfg, kind: KindDailyReport, title: title, body: body})
		}
	}

	if changed {
		s.st.Save()
	}
	return outbox, platformEmail
}

// platformEmailLocked — N°150 : réglages e-mail du compte principal (relais
// plateforme), à appeler SOUS VERROU du store. Nil si le compte principal
// n'a pas d'identifiants exploitables (ni Resend ni SMTP) — le comportement
// BYO historique s'applique alors partout.
func platformEmailLocked(db *model.DB) *model.NotificationSettings {
	if cfg, ok := db.NotifSettings[model.AccountMainID]; ok && EmailCredentialsOK(&cfg) {
		return &cfg
	}
	return nil
}

// appendLogs — écrit l'historique sous verrou (persisté + purgé par compte).
func (s *Service) appendLogs(logs []model.NotificationLog) {
	if len(logs) == 0 {
		return
	}
	s.st.Lock()
	db := s.st.Data()
	for _, l := range logs {
		l.ID = model.NewID("n-")
		l.At = model.NowISO()
		db.NotifLog = append(db.NotifLog, l)
	}
	trimNotifLog(db)
	s.st.Save()
	s.st.Unlock()
}

// trimNotifLog — garde les keepLogsPerAcc dernières entrées par compte.
func trimNotifLog(db *model.DB) {
	if len(db.NotifLog) == 0 {
		return
	}
	// tri ascendant par date (append chronologique déjà garanti, par sécurité).
	sort.SliceStable(db.NotifLog, func(i, j int) bool {
		return db.NotifLog[i].At < db.NotifLog[j].At
	})
	total := map[string]int{}
	for _, l := range db.NotifLog {
		total[l.AccountID]++
	}
	seen := map[string]int{}
	kept := db.NotifLog[:0]
	for _, l := range db.NotifLog {
		seen[l.AccountID]++
		if total[l.AccountID]-seen[l.AccountID] < keepLogsPerAcc { // il reste < keep après → garder
			kept = append(kept, l)
		}
	}
	db.NotifLog = kept
}

// ---------------------------------------------------------------------------
// Helpers de mise en forme des messages (français, ton gérant ivoirien)
// ---------------------------------------------------------------------------

// currencyLabel — XOF s'affiche FCFA (usage courant en Côte d'Ivoire).
func currencyLabel(cur string) string {
	if strings.EqualFold(cur, "XOF") || cur == "" {
		return defaultCurrency
	}
	return cur
}

func routerSiteLabel(r *model.Router) string {
	if r.Host != "" && r.Host != "simulé" {
		return r.Host
	}
	return "site " + r.Name
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "< 1 min"
	}
	if d < time.Hour {
		return strconvI(int(d.Minutes())) + " min"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h < 24 {
		res := strconvI(h) + " h"
		if m > 0 {
			res += " " + strconvI(m) + " min"
		}
		return res
	}
	return strconvI(h/24) + " j " + strconvI(h%24) + " h"
}

func strconvI(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func formatTimeAbidjan(t time.Time) string {
	return t.Format("02/01/2006 15:04")
}

func stockMessage(r *model.Router, available int, state string, threshold int) (string, string) {
	if state == "empty" {
		return "⚠️ Stock épuisé — " + r.Name,
			"Plus aucun voucher disponible sur «" + r.Name + "».\n" +
				"Générez un nouveau lot maintenant pour ne pas bloquer les ventes des revendeurs."
	}
	return "📦 Stock de vouchers bas — " + r.Name,
		"Il ne reste que " + strconvI(available) + " voucher(s) sur «" + r.Name +
			"» (seuil d'alerte : " + strconvI(threshold) + ").\nPrévoyez un nouveau lot avant la rupture."
}

// poolAutoMessage — N°99 — confirmation d'auto-réparation : le gérant sait
// que le cloud a agi seul (parce qu'il l'y a autorisé) et quoi faire si la
// pression ne retombe pas (l'extension reste un geste humain).
func poolAutoMessage(r *model.Router, usage, hosts, capacity int, state string) (string, string) {
	if state == "full" {
		return "🤖 Pool IP plein — auto-réparation lancée — " + r.Name,
			"Le pool de «" + r.Name + "» est saturé (" + strconvI(hosts) + "/" + strconvI(capacity) +
				" — " + strconvI(usage) + " %). L'auto-réparation est lancée : le recyclage des IP zombies sera appliqué " +
				"au prochain point de contact du routeur (≤ 45 s), sans action de votre part.\n" +
				"Si des clients voient encore « no more free addresses from pool », étendez le pool : Outils routeur → " +
				"Système → Étendre le pool (+ ~2 000 IP)."
	}
	return "🤖 Pool IP à " + strconvI(usage) + " % — auto-réparation lancée — " + r.Name,
		"L'occupation du pool de «" + r.Name + "» a atteint " + strconvI(usage) + " % (" + strconvI(hosts) + "/" + strconvI(capacity) +
			"), l'auto-réparation est lancée : le recyclage des IP zombies sera appliqué au prochain point de " +
			"contact du routeur (≤ 45 s), sans action de votre part.\n" +
			"Si la pression reste haute après le recyclage, étendez le pool : Outils routeur → Système → Étendre le pool."
}

// poolMessage — N°97 — message d'alerte d'occupation du pool IP. Le gérant
// est prévenu AVANT l'épuisement (80 %) : l'action est nommée (Outils
// routeur → Système → docteur pool : recycler/étendre).
func poolMessage(r *model.Router, usage, hosts, capacity int, state string) (string, string) {
	if state == "full" {
		return "🚨 Pool IP plein — " + r.Name,
			"Le pool d'adresses IP de «" + r.Name + "» est saturé (" + strconvI(hosts) + "/" + strconvI(capacity) +
				" — " + strconvI(usage) + " %).\n" +
				"Les nouveaux clients ne peuvent plus obtenir d'adresse IP (erreur « no more free addresses from pool »).\n" +
				"Outils routeur → Système → Docteur pool : étendez le pool (+ ~2 000 IP) maintenant."
	}
	return "📶 Pool IP presque plein — " + r.Name,
		"Le pool d'adresses IP de «" + r.Name + "» atteint " + strconvI(usage) + " % (" +
			strconvI(hosts) + "/" + strconvI(capacity) + ").\n" +
			"Aux heures de pointe il risque de saturer : Outils routeur → Système → Docteur pool (recycler les IP zombies, étendre si besoin)."
}

// buildDailyReport — rapport quotidien d'un compte (ventes, utilisateurs,
// routeurs, stock). now : instant UTC courant (= heure d'Abidjan).
func buildDailyReport(db *model.DB, acc string, now time.Time, cfg model.NotificationSettings) (string, string) {
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	dayISO := dayStart.Format(time.RFC3339)

	currency := defaultCurrency
	if s, ok := db.SettingsByAccount[acc]; ok && s.Tenant.Name != "" {
		currency = currencyLabel(s.Tenant.Currency)
	}
	tenantName := "MikCloud"
	if s, ok := db.SettingsByAccount[acc]; ok && s.Tenant.Name != "" {
		tenantName = s.Tenant.Name
	}

	salesCount, salesAmount := 0, 0
	for _, s := range db.Sales {
		if s.AccountID == acc && s.At >= dayISO {
			salesCount++
			salesAmount += s.Amount
		}
	}
	newUsers := 0
	for _, u := range db.HotspotUsers {
		if u.AccountID == acc && u.CreatedAt >= dayISO {
			newUsers++
		}
	}
	activeSessions := 0
	for _, s := range db.Sessions {
		if s.AccountID == acc {
			activeSessions++
		}
	}
	routersOnline, routersTotal := 0, 0
	for _, r := range db.Routers {
		if r.AccountID != acc {
			continue
		}
		routersTotal++
		if r.Status == "online" {
			routersOnline++
		}
	}
	activeVouchers := 0
	for _, u := range db.HotspotUsers {
		if u.AccountID != acc || u.Kind != "voucher" || u.Status != "active" {
			continue
		}
		if u.ExpiresAt == "" {
			activeVouchers++
			continue
		}
		if exp, err := time.Parse(time.RFC3339, u.ExpiresAt); err == nil && exp.After(now) {
			activeVouchers++
		}
	}

	title := "📊 Rapport MikCloud — " + dayStart.Format("02/01/2006")
	body := tenantName + " — " + dayStart.Format("02/01/2006") + "\n" +
		"────────────────────\n" +
		"💰 Ventes du jour : " + strconvI(salesCount) + " ticket(s) — " + strconvI(salesAmount) + " " + currency + "\n" +
		"👤 Nouveaux utilisateurs : " + strconvI(newUsers) + "\n" +
		"📶 Sessions actives : " + strconvI(activeSessions) + "\n" +
		"📡 Routeurs en ligne : " + strconvI(routersOnline) + "/" + strconvI(routersTotal) + "\n" +
		"🎟️ Vouchers disponibles : " + strconvI(activeVouchers) + "\n" +
		"────────────────────\n" +
		"Rapport quotidien automatique MikCloud."
	return title, body
}
