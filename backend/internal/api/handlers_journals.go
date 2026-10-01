// handlers_journals.go — N°200 — journaux MENSUELS GELÉS (P2 de la refonte
// app/reports, décision D3 : « gel auto au 1ᵉʳ + bouton Clôturer le mois
// maintenant »).
//
// Réponse au constat C4 de l'audit (« les données affichées ne reflètent pas
// la réalité ») : les rapports recalculaient TOUT à la volée depuis des
// sources vivantes purgées selon la rétention (user_logs 30/60/90 j,
// vouchers selon la politique d'expiration) — les courbes « 12 derniers
// mois » pourrissaient avec le temps. Désormais, au bascule de mois, un
// journal est GELÉ : les cinq KPI de l'aperçu (doctrine N°198 inchangée)
// + la répartition par canal, figés pour toujours.
//
// TROIS ENDPOINTS :
//   - GET  /api/reports/journals       — archives du compte (mois descend.) ;
//   - GET  /api/reports/journals.csv   — export Excel FR (; + BOM + CRLF) ;
//   - POST /api/reports/journals/close — « Clôturer le mois maintenant ».
//
// Le GEL AUTOMATIQUE vit dans le balayage horaire (retention.go —
// RunRetentionSweep, rattrapage au boot Render inclus) : ensureMonthlyJournals
// couvre les comptes dormants, jamais consultés (même argument que la
// rétention N°64 — les sources décayent, attendre une visite console
// perdrait des données). La lecture des archives rejoue le même contrôle à
// moindre coût (idempotent) : la toute première consultation après le 1ᵉʳ
// voit déjà le mois clos, sans attendre le passage horaire.
//
// IMMUABLE PAR CONSTRUCTION : la clé naturelle « mj-<compte>:<YYYY-MM> »
// garantit qu'un mois n'est JAMAIS journalisé deux fois — la clôture
// manuelle d'un mois déjà gelé répond 409, et le bascule automatique
// saute silencieusement un mois déjà clos manuellement (gelé = gelé).

package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// journalExistsLocked — un journal existe-t-il déjà pour ce (compte, mois) ?
// L'appelant tient le verrou du store.
func journalExistsLocked(db *model.DB, accID, month string) bool {
	id := model.MonthlyJournalID(accID, month)
	for i := range db.MonthlyJournals {
		if db.MonthlyJournals[i].ID == id {
			return true
		}
	}
	return false
}

// appendMonthlyJournalLocked — ajoute le journal (la clé a été vérifiée
// inexistante par l'appelant) et garantit la slice non-nil.
func appendMonthlyJournalLocked(db *model.DB, j model.MonthlyJournal) {
	if db.MonthlyJournals == nil {
		db.MonthlyJournals = []model.MonthlyJournal{}
	}
	db.MonthlyJournals = append(db.MonthlyJournals, j)
}

// buildMonthlyJournal — fige le journal du mois calendaire de monthStart
// (1ᵉʳ du mois 00 h 00, AU FUSEAU DU COMPTE) :
//
//   - periodEnd  : borne EXCLUSIVE temporelle des événements (bascule auto :
//     le 1ᵉʳ du mois suivant ; clôture manuelle : l'instant du clic) ;
//   - endDayKey  : dernier JOUR (clé « 2006-01-02 » locale) des agrégats
//     volume_days à inclure — la ligne du jour de clôture manuelle est
//     PARTIELLE par nature (l'accumulation est live) et doit pourtant
//     compter : c'est un instantané, pas une mesure de minuit.
//
// Doctrine N°198 inchangée : Revenus = trésorerie réelle, Ventes = tickets
// écoulés, Panier = prix réellement payé, Connexions = logins journalisés,
// Volume = agrégats journaliers N°199. L'appelant tient le verrou.
func buildMonthlyJournal(db *model.DB, accID string, monthStart, periodEnd time.Time, endDayKey, source string, now time.Time) model.MonthlyJournal {
	loc := accountTimezone(db, accID)
	month := monthStart.In(loc).Format("2006-01")
	startDayKey := monthStart.In(loc).Format("2006-01-02")
	j := model.MonthlyJournal{
		ID:          model.MonthlyJournalID(accID, month),
		AccountID:   accID,
		Month:       month,
		PeriodStart: monthStart.Format(time.RFC3339),
		PeriodEnd:   periodEnd.Format(time.RFC3339),
		Source:      source,
		Timezone:    loc.String(),
		ClosedAt:    now.UTC().Format(time.RFC3339),
	}
	if s, ok := db.SettingsByAccount[accID]; ok {
		j.Currency = s.Tenant.Currency
	}

	// TRÉSORERIE RÉELLE + répartition par canal (direct vs revendeurs nets).
	for _, e := range collectSaleEvents(db, accID, monthStart) {
		if !e.At.Before(periodEnd) {
			continue
		}
		j.Revenue += e.Amount
		if e.Reseller == "" {
			j.DirectRevenue += e.Amount
		} else {
			j.ResellerRevenue += e.Amount
		}
	}

	// ÉCOULEMENTS + panier payé (moyenne des prix publics réellement payés).
	var publicPaid int
	for _, v := range collectSoldVouchers(db, accID, monthStart) {
		if !v.At.Before(periodEnd) {
			continue
		}
		j.Sales++
		publicPaid += v.Public
		if v.ResellerName == "" {
			j.DirectSales++
		} else {
			j.ResellerSales++
		}
	}
	if j.Sales > 0 {
		j.AvgTicket = publicPaid / j.Sales
	}

	// CONNEXIONS journalisées — la rétention du journal (30/60/90 j par
	// compte, plafond 5 000) borne naturellement ce qui SURVIT à la clôture :
	// le journal fige ce que le système mesurait à l'instant du gel.
	for i := range db.UserLogs {
		l := &db.UserLogs[i]
		if l.AccountID != accID || l.Action != "login" {
			continue
		}
		at, err := time.Parse(time.RFC3339, l.At)
		if err != nil || at.Before(monthStart) || !at.Before(periodEnd) {
			continue
		}
		j.Logins++
	}

	// VOLUME DE DONNÉES — agrégats journaliers N°199 (tous sites du compte :
	// le journal est le livre du COMPTE, pas d'un site). Les jours couverts
	// sont bornés par endDayKey (cf. signature).
	for i := range db.VolumeDays {
		v := &db.VolumeDays[i]
		if v.AccountID != accID || v.Day < startDayKey || v.Day > endDayKey {
			continue
		}
		j.DataIn += v.BytesIn
		j.DataOut += v.BytesOut
	}

	// Couverture : jours calendaires du mois tombés dans la fenêtre.
	if periodEnd.Before(monthStart.AddDate(0, 1, 0)) {
		j.Partial = true
		j.Days = periodEnd.In(loc).Day()
	} else {
		j.Days = int(monthStart.AddDate(0, 1, 0).Sub(monthStart).Hours() / 24)
	}
	return j
}

// ensureMonthlyJournals — GEL AUTOMATIQUE au bascule de mois : pour chaque
// compte, si le mois PRÉCÉDENT (au fuseau du compte) n'a pas de journal, il
// est créé pour le mois PLEIN. Idempotent (la clé naturelle filtre les
// recomptages) ; renvoie le nombre de journaux créés. L'appelant tient le
// verrou et ASSURE la sauvegarde.
//
// Seul le mois immédiatement précédent est couvert : un trou plus ancien
// reste un trou VISIBLE dans les archives — honnête — plutôt qu'un journal
// reconstruit de sources déjà purgées (rétention user_logs 30/60/90 j,
// vouchers selon la politique d'expiration). Le premier déploiement archive
// ainsi le mois précédent depuis les sources survivantes (décision D2 : le
// volume vaut 0 pour tout mois antérieur à l'accumulateur).
func (a *API) ensureMonthlyJournals(db *model.DB, now time.Time) int {
	created := 0
	for i := range db.Accounts {
		accID := db.Accounts[i].ID
		loc := accountTimezone(db, accID)
		nowLocal := now.In(loc)
		// Dérivation TOUJOURS depuis le 1ᵉʳ du mois courant : AddDate(0,-1,0)
		// sur un 29/30/31 déborderait sur le mois suivant (31 octobre − 1 mois
		// = 31 septembre =… 1ᵉʳ octobre) — depuis le 1ᵉʳ, le jour 1 ne déborde
		// jamais.
		curStart := time.Date(nowLocal.Year(), nowLocal.Month(), 1, 0, 0, 0, 0, loc)
		prevStart := curStart.AddDate(0, -1, 0)
		prevMonth := prevStart.Format("2006-01")
		if journalExistsLocked(db, accID, prevMonth) {
			continue
		}
		appendMonthlyJournalLocked(db, buildMonthlyJournal(
			db, accID, prevStart, curStart,
			curStart.AddDate(0, 0, -1).Format("2006-01-02"),
			"auto", now))
		created++
	}
	return created
}

// handleJournalsList — GET /api/reports/journals : archives du compte, mois
// descendants. La réponse porte l'état du mois COURANT (déjà clôturé ?) pour
// le bouton « Clôturer le mois maintenant » — les chiffres LIVE du mois en
// cours restent ceux de l'aperçu de période (GET /api/stats/overview
// ?period=month), jamais confondus avec les archives gelées.
func (a *API) handleJournalsList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	now := time.Now().UTC()

	a.store.Lock()
	db := a.store.Data()
	// Rattrapage paresseux (idempotent, une boucle par compte) : la toute
	// première consultation après le 1ᵉʳ voit déjà le mois clos — même moteur
	// que le balayage horaire.
	created := a.ensureMonthlyJournals(db, now)
	loc := accountTimezone(db, acc)
	currentMonth := now.In(loc).Format("2006-01")
	currentClosed := journalExistsLocked(db, acc, currentMonth)

	out := make([]model.MonthlyJournal, 0, len(db.MonthlyJournals))
	for i := range db.MonthlyJournals {
		if db.MonthlyJournals[i].AccountID == acc {
			out = append(out, db.MonthlyJournals[i])
		}
	}
	if created > 0 {
		a.store.Save() // une écriture par compte et par mois — le coût est sans objet
	}
	a.store.Unlock()

	sort.Slice(out, func(i, j int) bool { return out[i].Month > out[j].Month })
	writeJSON(w, http.StatusOK, map[string]any{
		"journals":      out,
		"currentMonth":  currentMonth,
		"currentClosed": currentClosed,
		"generatedAt":   now.Format(time.RFC3339),
	})
}

// handleJournalClose — POST /api/reports/journals/close : « Clôturer le mois
// maintenant » (décision D3). Acte comptable délibéré : le mois EN COURS est
// gelé à l'instant du clic (partial=true, fenêtre couverte explicite) —
// l'activité restante du mois ne sera PAS journalisée, le dialogue de
// confirmation côté console le dit. Un mois déjà gelé répond 409 : gelé =
// gelé, JAMAIS de réécriture d'un document figé.
func (a *API) handleJournalClose(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	now := time.Now().UTC()

	a.store.Lock()
	db := a.store.Data()
	// Filet de sûreté : rattraper d'abord le mois précédent si le balayage
	// horaire n'est pas encore passé (sinon ce mois-là resterait un trou —
	// ensure ne couvre que le mois immédiatement précédent).
	a.ensureMonthlyJournals(db, now)

	loc := accountTimezone(db, acc)
	nowLocal := now.In(loc)
	monthStart := time.Date(nowLocal.Year(), nowLocal.Month(), 1, 0, 0, 0, 0, loc)
	month := monthStart.Format("2006-01")
	if journalExistsLocked(db, acc, month) {
		a.store.Unlock()
		writeErr(w, http.StatusConflict, "Le mois en cours est déjà clôturé — un journal gelé ne se réécrit jamais.")
		return
	}
	j := buildMonthlyJournal(db, acc, monthStart, now,
		nowLocal.Format("2006-01-02"), "manual", now)
	appendMonthlyJournalLocked(db, j)
	a.store.Save()
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"journal":       j,
		"currentMonth":  month,
		"currentClosed": true,
	})
}

// handleJournalsCSV — GET /api/reports/journals.csv : export des archives
// (séparateur « ; », BOM UTF-8, CRLF — Excel FR, mêmes conventions que la
// comptabilité). Une ligne par mois gelé, plus ancien en premier (lecture
// chronologique du livre comptable).
func (a *API) handleJournalsCSV(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)

	a.store.Lock()
	db := a.store.Data()
	// Même rattrapage paresseux que la liste : l'export reflète l'état
	// complet au moment du téléchargement.
	a.ensureMonthlyJournals(db, time.Now().UTC())
	tenantName := ensureSettings(db, acc).Tenant.Name
	rows := make([]model.MonthlyJournal, 0, len(db.MonthlyJournals))
	for i := range db.MonthlyJournals {
		if db.MonthlyJournals[i].AccountID == acc {
			rows = append(rows, db.MonthlyJournals[i])
		}
	}
	a.store.Unlock()
	sort.Slice(rows, func(i, j int) bool { return rows[i].Month < rows[j].Month })

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"mikcloud-journaux-mensuels.csv\"")
	// BOM UTF-8 : Excel reconnaît l'encodage.
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	_, _ = w.Write([]byte(fmt.Sprintf("MikCloud ;Journaux mensuels ;%s\r\n\r\n", csvField(tenantName))))
	_, _ = w.Write([]byte("Mois ;Ventes ;Chiffre d'affaires ;Panier moyen ;Connexions ;Volume (Go) ;" +
		"Ventes directes ;CA direct ;Ventes réseau ;CA réseau ;Couverture ;Clôture ;Date de clôture\r\n"))
	for _, j := range rows {
		coverage := fmt.Sprintf("%d j", j.Days)
		if j.Partial {
			coverage += " (partiel)"
		}
		source := "Automatique"
		if j.Source == "manual" {
			source = "Manuelle"
		}
		_, _ = w.Write([]byte(fmt.Sprintf("%s ;%d ;%d ;%d ;%d ;%.1f ;%d ;%d ;%d ;%d ;%s ;%s ;%s\r\n",
			csvField(j.Month), j.Sales, j.Revenue, j.AvgTicket, j.Logins,
			float64(j.DataIn+j.DataOut)/(1024*1024*1024),
			j.DirectSales, j.DirectRevenue, j.ResellerSales, j.ResellerRevenue,
			coverage, source, strings.ReplaceAll(j.ClosedAt, "T", " "))))
	}
}
