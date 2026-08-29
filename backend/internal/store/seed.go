// Package store — seed démo complet, dates relatives à time.Now(), rand seedé
// pour un re-seed reproductible (POST /api/admin/reset).
package store

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
)

// seedRandomSource — source déterministe : la structure du seed est stable.
const seedRandomSource = 20240601

// BuildSeed construit la base de démonstration :
//   - tenant "SpotNet WiFi" (FCFA, UTC), admin admin/admin123
//   - 2 routeurs simulés online, 6 profils
//   - ~60 utilisateurs réguliers, ~380 vouchers sur 14 jours (lots + statuts variés)
//   - 5 revendeurs, ~30 transactions, ventes 14 jours (week-ends plus forts)
//   - 18-28 sessions actives, ~25 entrées d'activité
func BuildSeed() *model.DB {
	rnd := rand.New(rand.NewSource(seedRandomSource))
	now := time.Now().UTC()
	ago := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	code := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = model.CodeCharset[rnd.Intn(len(model.CodeCharset))]
		}
		return string(b)
	}
	mac := func() string {
		b := make([]byte, 6)
		for i := range b {
			b[i] = byte(rnd.Intn(256))
		}
		parts := make([]string, 6)
		for i, x := range b {
			parts[i] = strings.ToUpper(fmt.Sprintf("%02x", x))
		}
		return strings.Join(parts, ":")
	}

	tenant := model.Tenant{Name: "SpotNet WiFi", Currency: "FCFA", Timezone: "UTC"}
	db := &model.DB{
		Tenant:       tenant,
		Settings:     model.Settings{Tenant: tenant, Plan: model.Plan{Name: "Pro", MaxRouters: "illimité", MaxUsers: "illimité"}},
		LastTick:     now,
		Users:        []model.AdminUser{},
		Routers:      []model.Router{},
		Profiles:     []model.Profile{},
		HotspotUsers: []model.HotspotUser{},
		Batches:      []model.Batch{},
		Resellers:    []model.Reseller{},
		Transactions: []model.Transaction{},
		Sessions:     []model.Session{},
		Activity:     []model.Activity{},
		Sales:        []model.Sale{},
	}

	// --- Compte admin (admin / admin123) ---
	salt := auth.NewSalt()
	db.Users = append(db.Users, model.AdminUser{
		ID:           "admin-1",
		Name:         "Administrateur SpotNet",
		Username:     "admin",
		Role:         "admin",
		PasswordHash: auth.HashPassword("admin123", salt),
		Salt:         salt,
		CreatedAt:    ago(120 * 24 * time.Hour),
	})

	// --- Routeurs simulés ---
	r1 := model.Router{
		ID: "router-main", Name: "RB-Main-Centre", Host: "10.10.10.1", Port: 8728,
		Username: "admin", Mode: "simulated", Status: "online", Version: "7.14.3",
		UptimeSec: int64(45*24*3600 + rnd.Intn(36000)), CPULoad: 12 + rnd.Intn(18),
		CreatedAt: ago(90 * 24 * time.Hour),
	}
	r2 := model.Router{
		ID: "router-lounge", Name: "hAP-Lounge", Host: "10.10.20.1", Port: 8728,
		Username: "admin", Mode: "simulated", Status: "online", Version: "7.15.2",
		UptimeSec: int64(12*24*3600 + rnd.Intn(36000)), CPULoad: 20 + rnd.Intn(18),
		CreatedAt: ago(60 * 24 * time.Hour),
	}
	db.Routers = []model.Router{r1, r2}

	// --- Profils (du plus récent au plus ancien : "1 Heure" d'abord) ---
	profiles := []model.Profile{
		{ID: "p-1h", Name: "1 Heure", RateLimit: "1M/1M", SessionTimeoutMin: 60, SharedUsers: 1, ValidityDays: 1, Price: 500, DataQuotaMb: 0},
		{ID: "p-6h", Name: "6 Heures", RateLimit: "2M/2M", SessionTimeoutMin: 360, SharedUsers: 1, ValidityDays: 1, Price: 1000, DataQuotaMb: 0},
		{ID: "p-24h", Name: "24 Heures", RateLimit: "3M/3M", SessionTimeoutMin: 1440, SharedUsers: 1, ValidityDays: 1, Price: 1500, DataQuotaMb: 0},
		{ID: "p-7j", Name: "7 Jours", RateLimit: "4M/4M", SessionTimeoutMin: 1440, SharedUsers: 1, ValidityDays: 7, Price: 5000, DataQuotaMb: 0},
		{ID: "p-30j", Name: "30 Jours Illimité", RateLimit: "5M/5M", SessionTimeoutMin: 1440, SharedUsers: 2, ValidityDays: 30, Price: 10000, DataQuotaMb: 0},
		{ID: "p-essai", Name: "Essai Gratuit", RateLimit: "512k/512k", SessionTimeoutMin: 15, SharedUsers: 1, ValidityDays: 1, Price: 0, DataQuotaMb: 0},
	}
	for i := range profiles {
		profiles[i].CreatedAt = ago(time.Duration(115+i) * 24 * time.Hour)
	}
	db.Profiles = profiles

	// --- Revendeurs ---
	resellerDefs := []struct {
		name     string
		username string
		phone    string
		credit   int
	}{
		{"Awa Diallo", "awa.diallo", "+221 77 412 55 08", 45000},
		{"Moussa Traoré", "moussa.traore", "+221 76 830 22 47", 80000},
		{"Fatou Ndiaye", "fatou.ndiaye", "+221 78 655 90 13", 12500},
		{"Ibrahima Sow", "ibrahima.sow", "+221 70 233 41 96", 5000},
		{"Khadija Benali", "khadija.benali", "+212 6 61 78 42 30", 23000},
	}
	for i, rd := range resellerDefs {
		db.Resellers = append(db.Resellers, model.Reseller{
			ID: "res-" + strconv.Itoa(i+1), Name: rd.name, Username: rd.username, Phone: rd.phone,
			Credit: rd.credit, VouchersSold: 0, Revenue: 0, Status: "active",
			CreatedAt: ago(time.Duration(95-i*12) * 24 * time.Hour),
		})
	}

	// --- ~60 utilisateurs réguliers ---
	firstNames := []string{"Moussa", "Awa", "Fatou", "Ibrahima", "Khadija", "Mamadou", "Aminata", "Ousmane", "Mariama", "Cheikh", "Rokhaya", "Souleymane", "Aliou", "Bineta", "Modou", "Ndèye", "Assane", "Coumba", "Maguette", "Sérigne"}
	lastNames := []string{"Diop", "Ndiaye", "Diallo", "Traoré", "Sow", "Benali", "Fall", "Sy", "Ba", "Guèye", "Camara", "Kane", "Mbaye", "Faye", "Sarr", "Cissé", "Diatta", "Thiam", "Seck", "Sagna"}
	comments := []string{"", "", "", "Client fidèle", "Paiement Mobile Money", "Paiement espèces", "Abonnement mensuel", "Boutique voisine", "", ""}
	// pondération des profils pour les réguliers (30j et 24h populaires)
	regularProfiles := []int{4, 4, 4, 3, 3, 2, 2, 1, 1, 0}
	seenUsernames := map[string]bool{}
	for i := 0; i < 60; i++ {
		f := firstNames[rnd.Intn(len(firstNames))]
		l := strings.ReplaceAll(lastNames[rnd.Intn(len(lastNames))], "é", "e")
		base := strings.ToLower(f + "." + l)
		username := base
		if seenUsernames[username] {
			username = fmt.Sprintf("%s%d", base, 10+rnd.Intn(90))
		}
		seenUsernames[username] = true
		p := db.Profiles[regularProfiles[rnd.Intn(len(regularProfiles))]]
		router := r1
		if rnd.Float64() > 0.6 {
			router = r2
		}
		created := now.Add(-time.Duration(rnd.Intn(60)*24+rnd.Intn(24)) * time.Hour)
		status := "active"
		if rnd.Float64() < 0.15 {
			status = "disabled"
		}
		bytesIn := int64(rnd.Intn(40)) * 1_000_000_000
		bytesOut := bytesIn/6 + int64(rnd.Intn(1_500_000_000))
		db.HotspotUsers = append(db.HotspotUsers, model.HotspotUser{
			ID: model.NewID("u-"), Kind: "regular", Username: username, Password: code(8),
			ProfileID: p.ID, ProfileName: p.Name, RouterID: router.ID, RouterName: router.Name,
			Status: status, BatchID: "", ResellerID: "", ResellerName: "",
			Comment: comments[rnd.Intn(len(comments))],
			BytesIn: bytesIn, BytesOut: bytesOut, UptimeUsedSec: int64(rnd.Intn(400)) * 3600,
			CreatedAt: created.Format(time.RFC3339),
			ExpiresAt: created.Add(time.Duration(p.ValidityDays) * 24 * time.Hour).Format(time.RFC3339),
			UsedAt:    "", Price: p.Price,
		})
	}

	// --- Vouchers (~380) sur 14 jours, par lots, avec ventes et transactions ---
	voucherProfiles := []int{2, 2, 2, 2, 2, 0, 0, 0, 1, 1, 1, 3, 4, 5} // 24h dominante
	takenUsernames := map[string]bool{}
	seq := 0
	for d := 13; d >= 0; d-- {
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -d)
		base := 18 + rnd.Intn(15) // 18-32 vouchers vendus par jour
		if wd := dayStart.Weekday(); wd == time.Saturday || wd == time.Sunday {
			base = int(float64(base) * 1.4) // week-ends ~40 % plus forts
		}
		remaining := base
		for remaining > 0 {
			size := 5 + rnd.Intn(11) // lots de 5 à 15 (plusieurs lots/jour → mix crédible)
			if size > remaining {
				size = remaining
			}
			remaining -= size
			p := db.Profiles[voucherProfiles[rnd.Intn(len(voucherProfiles))]]

			channel := "direct"
			var resID, resName string
			if rnd.Float64() < 0.45 {
				channel = "reseller"
				res := db.Resellers[rnd.Intn(len(db.Resellers))]
				resID, resName = res.ID, res.Name
			}

			// horaire du lot : dans SA journée, jamais dans le futur
			maxMinutes := 1440
			if toNow := int(now.Sub(dayStart).Minutes()); toNow < maxMinutes {
				maxMinutes = toNow
			}
			if maxMinutes < 1 {
				maxMinutes = 1
			}
			at := dayStart.Add(time.Duration(rnd.Intn(maxMinutes)) * time.Minute)
			seq++
			batchID := fmt.Sprintf("B%s-%03d", at.Format("20060102"), seq)

			// un lot est émis par UN seul site (routeur) — site principal dominant
			router := r1
			if rnd.Float64() > 0.65 {
				router = r2
			}

			for j := 0; j < size; j++ {
				c := code(5)
				for takenUsernames["SC-"+c] {
					c = code(5)
				}
				takenUsernames["SC-"+c] = true
				pw := code(5)
				for pw == c {
					pw = code(5)
				}
				createdAt := at.Add(time.Duration(rnd.Intn(60)) * time.Second)
				db.HotspotUsers = append(db.HotspotUsers, model.HotspotUser{
					ID: model.NewID("v-"), Kind: "voucher", Username: "SC-" + c, Password: pw,
					ProfileID: p.ID, ProfileName: p.Name, RouterID: router.ID, RouterName: router.Name,
					Status: "active", BatchID: batchID, ResellerID: resID, ResellerName: resName,
					Comment: "", BytesIn: 0, BytesOut: 0, UptimeUsedSec: 0,
					CreatedAt: createdAt.Format(time.RFC3339),
					ExpiresAt: createdAt.Add(time.Duration(p.ValidityDays) * 24 * time.Hour).Format(time.RFC3339),
					UsedAt:    "", Price: p.Price,
				})
			}

			cost := size * p.Price
			db.Batches = append(db.Batches, model.Batch{
				ID: batchID, ProfileID: p.ID, ProfileName: p.Name,
				RouterID: router.ID, RouterName: router.Name,
				Count: size, UnitPrice: p.Price, TotalCost: cost,
				Channel: channel, ResellerID: resID, ResellerName: resName,
				CreatedAt: at.Format(time.RFC3339),
			})
			db.Sales = append(db.Sales, model.Sale{
				ID: model.NewID("sale-"), Amount: cost, ProfileName: p.Name, Count: size,
				Channel: channel, ResellerName: resName,
				RouterID: router.ID, RouterName: router.Name, BatchID: batchID,
				At: at.Format(time.RFC3339),
			})
			if channel == "reseller" {
				db.Transactions = append(db.Transactions, model.Transaction{
					ID: model.NewID("tx-"), Type: "sale", ResellerID: resID, ResellerName: resName,
					Amount: cost, Note: fmt.Sprintf("Achat de %d vouchers (%s)", size, p.Name),
					At: at.Format(time.RFC3339),
				})
			}
		}
	}

	// --- Historique comptable 12 mois : lots tracés + ventes, vouchers purgés ---
	// Au-delà de 14 jours, les vouchers consommés/expirés ont été purgés des
	// routeurs (pratique standard d'un hotspot), mais chaque lot reste tracé
	// dans les registres (comptabilité + traçabilité).
	eligibleResellers := func(dayStart time.Time) []model.Reseller {
		out := []model.Reseller{}
		for _, res := range db.Resellers {
			ct, err := time.Parse(time.RFC3339, res.CreatedAt)
			if err == nil && !ct.After(dayStart) {
				out = append(out, res)
			}
		}
		return out
	}
	for d := 364; d >= 14; d-- {
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -d)
		// croissance douce : +15 % sur l'année
		growth := 0.85 + 0.15*float64(364-d)/350.0
		base := int(float64(18+rnd.Intn(15)) * growth)
		if wd := dayStart.Weekday(); wd == time.Saturday || wd == time.Sunday {
			base = int(float64(base) * 1.35)
		}
		remaining := base
		for remaining > 0 {
			size := 8 + rnd.Intn(18) // lots de 8 à 25
			if size > remaining {
				size = remaining
			}
			remaining -= size
			p := db.Profiles[voucherProfiles[rnd.Intn(len(voucherProfiles))]]

			// site émetteur : r2 (hAP-Lounge) ouvert il y a ~60 jours
			router := r1
			if d <= 60 && rnd.Float64() > 0.65 {
				router = r2
			}

			channel := "direct"
			var resID, resName string
			if elig := eligibleResellers(dayStart); len(elig) > 0 && rnd.Float64() < 0.45 {
				channel = "reseller"
				res := elig[rnd.Intn(len(elig))]
				resID, resName = res.ID, res.Name
			}

			at := dayStart.Add(time.Duration(8+rnd.Intn(12)) * time.Hour)
			seq++
			batchID := fmt.Sprintf("B%s-%03d", at.Format("20060102"), seq)
			cost := size * p.Price
			db.Batches = append(db.Batches, model.Batch{
				ID: batchID, ProfileID: p.ID, ProfileName: p.Name,
				RouterID: router.ID, RouterName: router.Name,
				Count: size, UnitPrice: p.Price, TotalCost: cost,
				Channel: channel, ResellerID: resID, ResellerName: resName,
				CreatedAt: at.Format(time.RFC3339),
			})
			db.Sales = append(db.Sales, model.Sale{
				ID: model.NewID("sale-"), Amount: cost, ProfileName: p.Name, Count: size,
				Channel: channel, ResellerName: resName,
				RouterID: router.ID, RouterName: router.Name, BatchID: batchID,
				At: at.Format(time.RFC3339),
			})
		}
	}

	// --- Statuts des vouchers (~40 % active, ~40 % used, ~15 % expired, ~5 % disabled) ---
	voucherIdx := []int{}
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].Kind == "voucher" {
			voucherIdx = append(voucherIdx, i)
		}
	}
	rnd.Shuffle(len(voucherIdx), func(i, j int) { voucherIdx[i], voucherIdx[j] = voucherIdx[j], voucherIdx[i] })
	total := len(voucherIdx)
	nExpired := total * 15 / 100
	nDisabled := total * 5 / 100
	nUsed := total * 40 / 100

	validityElapsed := func(u *model.HotspotUser) bool {
		var validityDays int
		for _, p := range db.Profiles {
			if p.ID == u.ProfileID {
				validityDays = p.ValidityDays
				break
			}
		}
		created, err := time.Parse(time.RFC3339, u.CreatedAt)
		if err != nil {
			return false
		}
		return created.Add(time.Duration(validityDays) * 24 * time.Hour).Before(now)
	}
	marked := map[int]bool{}
	setExpired := func(i int) {
		db.HotspotUsers[i].Status = "expired"
		db.HotspotUsers[i].UsedAt = ""
	}
	setUsed := func(i int) {
		u := &db.HotspotUsers[i]
		u.Status = "used"
		created, _ := time.Parse(time.RFC3339, u.CreatedAt)
		window := now.Sub(created)
		if window > time.Minute {
			u.UsedAt = created.Add(time.Duration(rnd.Int63n(int64(window)))).Format(time.RFC3339)
		} else {
			u.UsedAt = u.CreatedAt
		}
		u.BytesIn = int64(rnd.Intn(800)) * 1_000_000
		u.BytesOut = u.BytesIn / 7
		u.UptimeUsedSec = int64(rnd.Intn(7200))
	}

	// 1) expired : en priorité les vouchers dont la validité est dépassée
	count := 0
	for _, i := range voucherIdx {
		if count >= nExpired {
			break
		}
		if marked[i] || !validityElapsed(&db.HotspotUsers[i]) {
			continue
		}
		setExpired(i)
		marked[i] = true
		count++
	}
	for _, i := range voucherIdx {
		if count >= nExpired {
			break
		}
		if marked[i] {
			continue
		}
		setExpired(i)
		marked[i] = true
		count++
	}
	// 2) disabled
	count = 0
	for _, i := range voucherIdx {
		if count >= nDisabled {
			break
		}
		if marked[i] {
			continue
		}
		db.HotspotUsers[i].Status = "disabled"
		marked[i] = true
		count++
	}
	// 3) used
	count = 0
	for _, i := range voucherIdx {
		if count >= nUsed {
			break
		}
		if marked[i] {
			continue
		}
		setUsed(i)
		marked[i] = true
		count++
	}

	// --- Sessions actives (18-28) ---
	activeVouchers := []int{}
	for _, i := range voucherIdx {
		if db.HotspotUsers[i].Status == "active" {
			activeVouchers = append(activeVouchers, i)
		}
	}
	rnd.Shuffle(len(activeVouchers), func(i, j int) { activeVouchers[i], activeVouchers[j] = activeVouchers[j], activeVouchers[i] })
	sessionUsernames := map[string]bool{}
	addSession := func(idx int) {
		u := &db.HotspotUsers[idx]
		router := r1
		if u.RouterID == r2.ID {
			router = r2
		}
		uptime := int64(180 + rnd.Intn(5*3600)) // 3 min à ~5 h
		started := now.Add(-time.Duration(uptime) * time.Second)
		ip := router.Host[:strings.LastIndexByte(router.Host, '.')+1] + strconv.Itoa(2+rnd.Intn(200))
		db.Sessions = append(db.Sessions, model.Session{
			ID: model.NewID("s-"), UserID: u.ID, Username: u.Username, ProfileName: u.ProfileName,
			RouterID: router.ID, RouterName: router.Name, IP: ip, MAC: mac(),
			StartedAt: started.Format(time.RFC3339),
			UptimeSec: uptime,
			BytesIn:   uptime * int64(1_000_000+rnd.Intn(2_000_000)),
			BytesOut:  uptime * int64(200_000+rnd.Intn(600_000)),
		})
		sessionUsernames[u.Username] = true
		if u.Kind == "voucher" {
			u.Status = "used"
			u.UsedAt = started.Format(time.RFC3339)
		}
	}
	nSessions := 18 + rnd.Intn(11)
	nVoucherSess := 6 + rnd.Intn(5)
	nRegularSess := nSessions - nVoucherSess
	for i := 0; i < nVoucherSess && i < len(activeVouchers); i++ {
		addSession(activeVouchers[i])
	}
	regularActive := []int{}
	for i := range db.HotspotUsers {
		u := db.HotspotUsers[i]
		if u.Kind == "regular" && u.Status == "active" && !sessionUsernames[u.Username] {
			regularActive = append(regularActive, i)
		}
	}
	rnd.Shuffle(len(regularActive), func(i, j int) { regularActive[i], regularActive[j] = regularActive[j], regularActive[i] })
	for i := 0; i < nRegularSess && i < len(regularActive); i++ {
		addSession(regularActive[i])
	}
	sort.Slice(db.Sessions, func(i, j int) bool { return db.Sessions[i].StartedAt > db.Sessions[j].StartedAt })

	// --- Statistiques revendeurs (cohérentes avec les vouchers utilisés) ---
	for i := range db.Resellers {
		sold, revenue := 0, 0
		for j := range db.HotspotUsers {
			u := &db.HotspotUsers[j]
			if u.Kind == "voucher" && u.ResellerID == db.Resellers[i].ID && u.Status == "used" {
				sold++
				revenue += u.Price
			}
		}
		db.Resellers[i].VouchersSold = sold
		db.Resellers[i].Revenue = revenue
	}

	// --- Transactions de rechargement (~15) ---
	creditNotes := []string{"Rechargement espèces", "Rechargement Mobile Money", "Rechargement Wave", "Rechargement virement"}
	for i := range db.Resellers {
		nb := 2 + rnd.Intn(2)
		for j := 0; j < nb; j++ {
			at := now.Add(-time.Duration(rnd.Intn(40*24)) * time.Hour)
			amount := (1 + rnd.Intn(5)) * 10000 // 10 000 à 50 000 FCFA
			db.Transactions = append(db.Transactions, model.Transaction{
				ID: model.NewID("tx-"), Type: "credit", ResellerID: db.Resellers[i].ID,
				ResellerName: db.Resellers[i].Name, Amount: amount,
				Note: creditNotes[rnd.Intn(len(creditNotes))], At: at.Format(time.RFC3339),
			})
		}
	}
	sort.Slice(db.Transactions, func(i, j int) bool { return db.Transactions[i].At > db.Transactions[j].At })

	// --- Journal d'activité (~26 entrées récentes) ---
	regNames := []string{}
	for _, u := range db.HotspotUsers {
		if u.Kind == "regular" {
			regNames = append(regNames, u.Username)
		}
	}
	pick := func(i int) string {
		if len(regNames) == 0 {
			return "client"
		}
		return regNames[i%len(regNames)]
	}
	var lastSale model.Sale
	if len(db.Sales) > 0 {
		lastSale = db.Sales[len(db.Sales)-1]
	}
	sampleBatch, sampleCode := "", "SC-XXXXX"
	for _, u := range db.HotspotUsers {
		if u.Kind != "voucher" {
			continue
		}
		if sampleBatch == "" {
			sampleBatch = u.BatchID
		}
		if u.Status == "used" {
			sampleCode = u.Username
			break
		}
	}
	acts := []model.Activity{}
	addAct := func(typ, msg string, minutesAgo int) {
		acts = append(acts, model.Activity{
			ID: model.NewID("act-"), Type: typ, Message: msg,
			At: now.Add(-time.Duration(minutesAgo) * time.Minute).Format(time.RFC3339),
		})
	}
	addAct("system", "Service MikCloud démarré", 2)
	addAct("session", "Nouvelle session : "+pick(0), 5+rnd.Intn(4))
	addAct("voucher", fmt.Sprintf("Génération de %d vouchers (%s)", lastSale.Count, lastSale.ProfileName), 22)
	addAct("session", "Session de "+pick(1)+" terminée", 38)
	addAct("user", "Utilisateur "+pick(2)+" activé", 61)
	addAct("reseller", "Crédit de 15 000 FCFA ajouté à "+db.Resellers[0].Name, 118)
	addAct("router", "Test de connexion réussi sur "+r1.Name+" (14 ms)", 176)
	addAct("voucher", fmt.Sprintf("Lot %s exporté pour impression (PDF)", sampleBatch), 240)
	addAct("user", "Utilisateur "+pick(3)+" créé", 305)
	addAct("session", "Session de "+pick(4)+" fermée (kick)", 366)
	addAct("reseller", db.Resellers[1].Name+" a acheté 20 vouchers (24 Heures)", 420)
	addAct("voucher", "Génération de 15 vouchers (Essai Gratuit)", 540)
	addAct("system", "Mise à jour des statistiques routeurs", 610)
	addAct("router", r2.Name+" : charge CPU 32 %", 725)
	addAct("user", "Utilisateur "+pick(5)+" désactivé", 840)
	addAct("session", "Nouvelle session : "+pick(6), 960)
	addAct("reseller", "Crédit de 25 000 FCFA ajouté à "+db.Resellers[1].Name, 1200)
	addAct("voucher", "Génération de 30 vouchers (1 Heure)", 1320)
	addAct("system", "Sauvegarde des données effectuée", 1440)
	addAct("router", "Routeur "+r2.Name+" reconnecté", 1560)
	addAct("user", "Utilisateur "+pick(7)+" activé", 1680)
	addAct("session", "Session de "+pick(8)+" terminée", 1800)
	addAct("voucher", "Voucher "+sampleCode+" utilisé", 1920)
	addAct("reseller", "Crédit de 10 000 FCFA ajouté à "+db.Resellers[4].Name, 2400)
	addAct("system", "Paramètres du tenant mis à jour", 2640)
	addAct("voucher", "Génération de 10 vouchers (7 Jours)", 2760)
	sort.Slice(acts, func(i, j int) bool { return acts[i].At > acts[j].At })
	db.Activity = acts

	// Compteurs (recalculés dynamiquement par l'API, seedés pour lisibilité du JSON)
	usersByRouter := map[string]int{}
	sessionsByRouter := map[string]int{}
	for _, u := range db.HotspotUsers {
		usersByRouter[u.RouterID]++
	}
	for _, s := range db.Sessions {
		sessionsByRouter[s.RouterID]++
	}
	for i := range db.Routers {
		db.Routers[i].HotspotUsers = usersByRouter[db.Routers[i].ID]
		db.Routers[i].ActiveSessions = sessionsByRouter[db.Routers[i].ID]
	}

	return db
}
