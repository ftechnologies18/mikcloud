// handlers_wifi.go — N°27 « WiFi Jetable » : mode d'accès offert adapté aux
// restaurants & cafés, salons de coiffure, maquis et buvettes.
//
// Flux public (SANS authentification, rate-limité) :
//  1. le client scanne le QR code affiché sur sa table → page publique
//     /wifi/{slug} (frontend) → GET  /api/wifi/site/{slug}          (branding) ;
//  2. il laisse son numéro (marketing opt-in) → POST /api/wifi/site/{slug}/claim
//     → un ticket à code UNIQUE (user = mdp, verrou N°25) est émis à la volée
//     dans un lot-jour par site (canal direct, trace « wifi-jetable »), avec
//     quotas gratuits limit-uptime / limit-bytes-total poussés au routeur ;
//  3. le code s'affiche sur la page (copie 1 clic) → bouton de connexion vers
//     le lien DNS du hotspot ({loginUrl}?username=CODE&password=CODE) ;
//  4. au-delà du quota, le ROUTEUR coupe (pas de code à écrire) ; le client
//     re-scanne → GET /api/wifi/site/{slug}/status → état « épuisé » + offres
//     payantes (bascule 1 clic : achat comptoir ou lien Wave du gérant).
//
// Garde-fous anti-abus : rate-limit IP (cf. main.go), plafond par
// téléphone/jour (idempotence : re-claim ⇒ MÊME code), plafond site/jour
// (budget gratuit du gérant), garde abonnement (compte expiré → émission
// refusée 402) — N°50 : honeypot « website » (succès factice), quota
// anti-fermage par IP (20/10 min + 100/24 h) et plafond par appareil (MAC)
// journalier, empreintes MAC/IP tracées dans le registre. Registre marketing : WifiGuest (opt-in explicite, export CSV
// console, loi ivoirienne n°2013-450 / ARTCI-CIL : consentement et finalité
// affichés, suppression à la demande via DELETE du site).
package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// findWifiSiteBySlug — slug GLOBALEMENT unique (tous comptes) : l'URL
// publique /wifi/{slug} ne porte pas d'identifiant de compte.
func findWifiSiteBySlug(db *model.DB, slug string) *model.WifiSite {
	for i := range db.WifiSites {
		if db.WifiSites[i].Slug == slug {
			return &db.WifiSites[i]
		}
	}
	return nil
}

// findWifiSiteByID — site scopé au compte (handlers console).
func findWifiSiteByID(db *model.DB, id, acc string) *model.WifiSite {
	for i := range db.WifiSites {
		if db.WifiSites[i].ID == id && db.WifiSites[i].AccountID == acc {
			return &db.WifiSites[i]
		}
	}
	return nil
}

// wifiLoginBase — base de l'URL de login du hotspot du site : priorité à la
// URL saisie sur le routeur (Router.HotspotLoginUrl), sinon le DNS name du
// tenant (http://{dns}/login), sinon "" (le frontend masquera le bouton).
func wifiLoginBase(db *model.DB, site *model.WifiSite) string {
	if rr := findRouterScoped(db, site.RouterID, site.AccountID); rr != nil {
		if url := strings.TrimSpace(rr.HotspotLoginUrl); url != "" {
			return url
		}
	}
	if dns := strings.TrimSpace(ensureSettings(db, site.AccountID).Tenant.DNSName); dns != "" {
		return "http://" + dns + "/login"
	}
	return ""
}

// wifiLoginURL — deep-link d'auto-connexion (mécanique QR vouchers, parité
// Mikhmon) : {base}?username=CODE&password=CODE.
func wifiLoginURL(base, code string) string {
	if base == "" {
		return ""
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + "username=" + code + "&password=" + code
}

// wifiResolveVoucher — ticket lié à une ligne de registre visiteur.
func wifiResolveVoucher(db *model.DB, g *model.WifiGuest) *model.HotspotUser {
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].ID == g.VoucherID {
			return &db.HotspotUsers[i]
		}
	}
	return nil
}

// wifiVoucherUsable — le ticket délivré permet-il encore de se connecter ?
// (statut effectif ni expiré ni désactivé ; « used » = déjà connecté mais
// quota restant éventuel).
func wifiVoucherUsable(u *model.HotspotUser) bool {
	if u == nil || u.Kind != "voucher" {
		return false
	}
	eff := model.EffectiveStatus(u, time.Now().UTC())
	return eff != "expired" && eff != "disabled"
}

// wifiQuotaResponse — quota gratuit effectif du site (0 = hériter du profil).
func wifiQuotaResp(site *model.WifiSite, profile *model.Profile) (timeMin, dataMb int64) {
	timeMin = site.FreeTimeMin
	if timeMin <= 0 {
		timeMin = int64(profile.SessionTimeoutMin)
	}
	dataMb = site.FreeDataMb
	if dataMb < 0 {
		dataMb = 0
	}
	return timeMin, dataMb
}

// wifiOffers — offres payantes proposées à la bascule : profils du compte
// dont le prix est > 0 (les profils sont périmètre-compte, pas périmètre-
// routeur), max 8.
func wifiOffers(db *model.DB, site *model.WifiSite) []map[string]any {
	offers := []map[string]any{}
	addOffer := func(p model.Profile) {
		price := p.Price
		if p.SellingPrice > 0 {
			price = p.SellingPrice
		}
		if price <= 0 || len(offers) >= 8 {
			return
		}
		offers = append(offers, map[string]any{
			"id": p.ID, "name": p.Name, "price": price,
			"validityMinutes": p.ValidityMinutes(),
			"dataQuotaMb":     p.DataQuotaMb,
			"timeLimitMin":    p.SessionTimeoutMin,
		})
	}
	for _, p := range db.Profiles {
		if p.AccountID == site.AccountID {
			addOffer(p)
		}
	}
	return offers
}

// ---------------------------------------------------------------------------
// Endpoints PUBLICS (whitelist middleware.go, rate-limit « wifi-claim » /
// « wifi-read » dans main.go) — le compte est résolu depuis le slug.
// ---------------------------------------------------------------------------

// handleWifiSiteInfo — branding + quotas affichables du site (aucune donnée
// sensible : pas d'IDs routeur, pas de codes).
func (a *API) handleWifiSiteInfo(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	a.store.Lock()
	db := a.store.Data()
	site := findWifiSiteBySlug(db, slug)
	if site == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Site WiFi introuvable")
		return
	}
	settings := ensureSettings(db, site.AccountID)
	guard := a.subscriptionGuardStateLocked(site.AccountID)
	siteCopy := *site
	resp := map[string]any{
		"slug":           siteCopy.Slug,
		"name":           siteCopy.Name,
		"tenantName":     settings.Tenant.Name,
		"logoUrl":        settings.Tenant.LogoURL,
		"bannerUrl":      settings.Tenant.BannerURL, // N°45 — bannière portail/WiFi
		"freeTimeMin":    siteCopy.FreeTimeMin,
		"freeDataMb":     siteCopy.FreeDataMb,
		"marketingOptIn": siteCopy.MarketingOptIn,
		"active":         siteCopy.Active,
		"suspended":      guard.Status == "expired",
	}
	if profile := findProfileScoped(db, siteCopy.ProfileID, siteCopy.AccountID); profile != nil {
		timeMin, dataMb := wifiQuotaResp(&siteCopy, profile)
		resp["freeTimeMin"] = timeMin
		resp["freeDataMb"] = dataMb
		resp["profileName"] = profile.Name
	}
	if siteCopy.Active && guard.Status != "expired" {
		resp["offers"] = wifiOffers(db, &siteCopy)
	}
	a.store.Unlock()
	writeJSONCacheable(w, r, http.StatusOK, resp) // N°74 — ETag/304 : branding stable re-téléchargé pour rien à chaque page
}

// wifiPhoneConsented — N°69 : état de consentement courant d'un numéro sur
// un compte. L'état suit le NUMÉRO (pas la ligne du jour) : une bascule via
// /consent met à jour TOUTES les lignes du téléphone, donc « une ligne au
// true » = consentement actif. À appeler sous verrou.
func wifiPhoneConsented(db *model.DB, acc, phone string) bool {
	for i := range db.WifiGuests {
		if db.WifiGuests[i].AccountID == acc && db.WifiGuests[i].Phone == phone && db.WifiGuests[i].OptIn {
			return true
		}
	}
	return false
}

// wifiSetPhoneConsent — N°69 : pose l'état de consentement sur TOUTES les
// lignes d'un numéro du compte (l'état suit le numéro — cf. type WifiGuest).
// Retourne le nombre de lignes mises à jour. À appeler sous verrou ;
// l'appellant décide du Save.
func wifiSetPhoneConsent(db *model.DB, acc, phone string, optIn bool) int {
	n := 0
	for i := range db.WifiGuests {
		g := &db.WifiGuests[i]
		if g.AccountID != acc || g.Phone != phone {
			continue
		}
		g.OptIn = optIn
		g.OptInAt = ""
		if optIn {
			g.OptInAt = model.NowISO()
		}
		n++
	}
	return n
}

// handleWifiClaim — cœur du mode : valide le téléphone, applique les plafonds
// (idempotence téléphone/jour, budget site/jour), émet UN ticket gratuit et
// enregistre le visiteur (registre marketing). Renvoie le code + le lien de
// connexion du hotspot.
func (a *API) handleWifiClaim(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	var req struct {
		Phone string `json:"phone"`
		OptIn bool   `json:"optIn"`
		// N°50 — durcissement anti-abus : honeypot « website » (champ
		// invisible du formulaire, jamais rendu visible) et MAC de
		// l'appareil (injectée par le portail via $(mac-esc) ; la page
		// /wifi scannée hors portail ne peut pas la fournir — vide OK).
		Website string `json:"website"`
		Mac     string `json:"mac"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	// N°50 — quota anti-fermage par IP (20/10 min, 100/24 h) : consommé par
	// TOUTE tentative (succès comme échec de validation, mimant le contrat
	// join/signup). Borne le volume depuis une même IP avant toute création
	// de voucher — les plafonds métier journaliers (téléphone/MAC/site)
	// restent la seconde ligne de défense.
	if ok, retry := a.wifiClaim.allow(clientIP(r)); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeErr(w, http.StatusTooManyRequests, "Trop de tentatives — réessayez plus tard")
		return
	}
	phone := model.NormalizeWifiPhone(req.Phone)
	if phone == "" {
		writeErr(w, http.StatusBadRequest, "Numéro de téléphone invalide (8 à 15 chiffres, indicatif inclus)")
		return
	}
	// N°50 — MAC de l'appareil (claim portail : $(mac-esc) injecté par le
	// routeur) normalisée comme au formulaire d'inscription ; IP client du
	// claim (déjà consommée par le limiteur) tracée pour l'audit du gérant.
	mac := normalizeJoinMac(req.Mac)
	ip := clientIP(r)

	a.store.Lock()
	db := a.store.Data()
	site := findWifiSiteBySlug(db, slug)
	if site == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Site WiFi introuvable")
		return
	}
	if !site.Active {
		a.store.Unlock()
		writeErrCode(w, http.StatusForbidden, "site_inactive", "WiFi offert momentanément désactivé — appelez le personnel", nil)
		return
	}
	// Garde abonnement : un compte expiré ne distribue plus de WiFi gratuit
	// (même verrou économique que les écritures métier, cf. guards.go).
	if guard := a.subscriptionGuardStateLocked(site.AccountID); guard.Status == "expired" {
		a.store.Unlock()
		writeErrCode(w, http.StatusPaymentRequired, "subscription_expired", "Service WiFi offert suspendu — contactez l'établissement", nil)
		return
	}
	profile := findProfileScoped(db, site.ProfileID, site.AccountID)
	if profile == nil {
		a.store.Unlock()
		writeErr(w, http.StatusInternalServerError, "Profil du site WiFi introuvable — contactez le gérant")
		return
	}
	router := findRouterScoped(db, site.RouterID, site.AccountID)
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusInternalServerError, "Routeur du site WiFi introuvable — contactez le gérant")
		return
	}
	// N°75 — veille adaptative : un invité est SUR le portail de ce site —
	// son routeur repasse en mode rapide (le claim qui suit est servi en
	// 45 s, pas au prochain cycle de veille).
	a.markAttention("rt:" + router.ID)
	routerCopy := *router
	profileCopy := *profile
	settings := ensureSettings(db, site.AccountID)
	// N°50 — honeypot : un bot qui a rempli le champ caché reçoit un succès
	// FACTICE (même forme JSON qu'une vraie émission, code aléatoire jamais
	// créé côté routeur) — rien n'est émis, rien n'est enregistré, et le bot
	// ne reçoit AUCUN indice sur le filtre (même contrat que le formulaire
	// d'inscription). Placé APRÈS la résolution site/profil pour refléter
	// des quotas plausibles, AVANT toute écriture.
	if strings.TrimSpace(req.Website) != "" {
		// Tout est lu SOUS verrou (jamais de lecture de db après Unlock) ;
		// seul l'écrit de la réponse est relâché.
		fake := model.RandomCodeFrom(5, "")
		fakeTime, fakeData := wifiQuotaResp(site, profile)
		fakeURL := wifiLoginURL(wifiLoginBase(db, site), fake)
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"duplicate":     false,
			"waitForRouter": false,
			"code":          fake,
			"loginUrl":      fakeURL,
			"timeLimitMin":  fakeTime,
			"dataQuotaMb":   fakeData,
			"profileName":   profileCopy.Name,
			"siteName":      site.Name,
		})
		return
	}
	now := time.Now().UTC()
	dayKey := model.WifiDayKey(settings.Tenant.Timezone, now)

	// Registre du jour : (compte, site, téléphone) — base de l'idempotence
	// et des plafonds.
	today := []model.WifiGuest{}
	siteToday := 0
	macToday := 0 // N°50 — claims du même appareil (MAC) aujourd'hui, ce site
	for _, g := range db.WifiGuests {
		if g.AccountID != site.AccountID || g.Day != dayKey {
			continue
		}
		if g.SiteID == site.ID {
			siteToday++
			if g.Phone == phone {
				today = append(today, g)
			}
			if mac != "" && g.Mac == mac {
				macToday++
			}
		}
	}

	// Idempotence : re-claim du même téléphone ⇒ le MÊME code (tant qu'il est
	// utilisable) — rafraîchissement de page, mobile qui ré-ouvre, pas de
	// double émission, pas de farm de codes.
	for i := len(today) - 1; i >= 0; i-- {
		v := wifiResolveVoucher(db, &today[i])
		if wifiVoucherUsable(v) {
			// N°69 — UPGRADE au re-claim idempotent : le visiteur qui pose
			// l'interrupteur au re-scan (même jour) voit son consentement
			// enregistré immédiatement, sans attendre un nouveau ticket au
			// lendemain. Le sens inverse (retrait) reste EXPLICITE via
			// POST /consent : ne pas toucher l'interrupteur n'est pas un
			// retrait — un consentement posé reste acquis (loi 2013-450 : le
			// retrait doit être aussi aisé que le consentement, tous deux
			// sont des actions dédiées, jamais une omission).
			guestOptIn := wifiPhoneConsented(db, site.AccountID, phone)
			if req.OptIn && site.MarketingOptIn && !guestOptIn {
				wifiSetPhoneConsent(db, site.AccountID, phone, true)
				guestOptIn = true
				a.logActivityBy(r, db, site.AccountID, "wifi",
					fmt.Sprintf("WiFi jetable «%s» : consentement marketing activé par %s", site.Name, maskPhone(phone)))
				a.store.Save()
			}
			a.store.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{
				"duplicate":     true,
				"waitForRouter": routerCopy.Mode == "agent",
				"code":          v.Username,
				"loginUrl":      wifiLoginURL(wifiLoginBase(db, site), v.Username),
				"timeLimitMin":  v.TimeLimitMin,
				"dataQuotaMb":   v.DataQuotaMb,
				"profileName":   v.ProfileName,
				"siteName":      site.Name,
				"optIn":         guestOptIn, // N°69 — état marketing du numéro (carte code)
			})
			return
		}
	}
	// N°52 — Ton du refus selon le MODE de l'établissement. MikCloud sert
	// deux usages : la vente de tickets (commercial/hybride) ET l'offre
	// gratuite de fidélisation (hôtel, maquis, café-glacier, salon…). Avec
	// des profils payants au catalogue on propose l'upsell ; sans catalogue
	// payant, pousser une « offre payante » inexistante décrédibilise l'écran
	// et l'établissement → message neutre. Même signal que l'écran « épuisé »
	// de la page /wifi, qui liste ces mêmes offres (wifiOffers).
	// Calculé sous verrou, une seule fois, avant les trois gardes.
	capSuffix := " — passez à une offre payante"
	if len(wifiOffers(db, site)) == 0 {
		capSuffix = " — demandez au personnel ou revenez demain"
	}
	// N°50 — Plafond par appareil (MAC) / jour : derrière le NAT du hotspot
	// tous les clients partagent la MÊME IP publique — la MAC est la seule
	// clé qui isole réellement un appareil (même leçon que le quota MAC N°33
	// de l'inscription). Elle n'est disponible que pour les claims du
	// portail ; la page /wifi scannée hors portail ne la fournit pas (les
	// plafonds téléphone/site + le quota IP anti-fermage restent actifs).
	// Placé APRÈS l'idempotence téléphone : le re-claim du MÊME téléphone
	// renvoie toujours le même code, plafond atteint ou non.
	if mac != "" {
		perMac := site.DailyPerMac
		if perMac < 1 {
			perMac = 1
		}
		if macToday >= perMac {
			a.store.Unlock()
			writeErrCode(w, http.StatusTooManyRequests, "device_cap", "Votre WiFi offert du jour est déjà consommé sur cet appareil"+capSuffix, nil) // N°52 — ton contextuel
			return
		}
	}
	// Plafond par téléphone/jour.
	perPhone := site.DailyPerPhone
	if perPhone < 1 {
		perPhone = 1
	}
	if len(today) >= perPhone {
		a.store.Unlock()
		writeErrCode(w, http.StatusTooManyRequests, "phone_cap", "Votre WiFi offert du jour est déjà consommé"+capSuffix, nil) // N°52 — ton contextuel
		return
	}
	// Budget gratuit du site (plafond journalier posé par le gérant).
	cap := site.DailyCap
	if cap < 1 {
		cap = 1
	}
	if siteToday >= cap {
		a.store.Unlock()
		writeErrCode(w, http.StatusTooManyRequests, "site_cap", "Le WiFi offert de l'établissement est épuisé pour aujourd'hui", nil)
		return
	}

	// Quotas gratuits effectifs (site surcharge le profil).
	timeMin, dataMb := wifiQuotaResp(site, profile)
	if dataMb > 1_073_741_824 {
		dataMb = 1_073_741_824
	}

	// Émission du ticket : code unique user = mdp (verrou N°25), validité
	// ancrée au premier login (ExpiresAt vide → AnchorVoucherValidity).
	code := model.RandomCodeFrom(5, "")
	for j := 0; j < 50 && usernameTaken(db, site.AccountID, code); j++ {
		code = model.RandomCodeFrom(5, "")
	}
	// Lot-jour par site (préfixe W) : grouping propre, comptabilité honnête
	// (prix du profil), et nettoyage AUTOMATIQUE par le sweep N°26 quand tous
	// les tickets du lot ont expiré.
	batchID := fmt.Sprintf("W%s-%s", strings.ReplaceAll(dayKey, "-", ""), site.ID)
	comment := sanitizeVoucherComment("WiFi offert · " + site.Name)
	voucher := model.HotspotUser{
		ID: model.NewID("v-"), AccountID: site.AccountID, Kind: "voucher",
		Username: code, Password: code,
		ProfileID: profileCopy.ID, ProfileName: profileCopy.Name,
		RouterID: routerCopy.ID, RouterName: routerCopy.Name,
		Status: "active", BatchID: batchID,
		CreatedAt: model.NowISO(), ExpiresAt: "", UsedAt: "",
		Price: profileCopy.Price, SellingPrice: profileCopy.SellingPrice,
		DataQuotaMb: dataMb, TimeLimitMin: timeMin,
	}
	// N°69 — consentement marketing explicite : l'interrupteur posé par le
	// visiteur (portail login.html OU page /wifi). Aucune case pré-cochée
	// n'existe plus — false est l'état par défaut, l'action affirmative
	// (interrupteur posé) crée le consentement + sa preuve horodatée.
	// Héritage : un numéro déjà consenti garde son consentement (ne pas
	// toucher l'interrupteur ≠ se désinscrire — le retrait est explicite via
	// POST /consent, cf. handleWifiConsent).
	optIn := req.OptIn && site.MarketingOptIn
	if !optIn && wifiPhoneConsented(db, site.AccountID, phone) {
		optIn = true
	}
	guest := model.WifiGuest{
		ID: model.NewID("wg-"), AccountID: site.AccountID,
		SiteID: site.ID, SiteName: site.Name, Phone: phone,
		OptIn:     optIn,
		VoucherID: voucher.ID, Code: code, Day: dayKey, CreatedAt: model.NowISO(),
		Mac: mac, IP: ip, // N°50 — empreintes anti-abus (audit gérant)
	}
	if optIn {
		guest.OptInAt = model.NowISO() // N°69 — preuve horodatée du consentement
	}
	isAgent := routerCopy.Mode == "agent"
	if isAgent {
		voucher.Username = agent.SanitizeName(voucher.Username)
		voucher.ProfileName = agent.SanitizeName(voucher.ProfileName)
	}
	loginBase := wifiLoginBase(db, site)
	// Relâche le verrou des validations : la phase d'application le reprend
	// (le mutex du store n'est PAS réentrant — cf. pattern du générateur).
	a.store.Unlock()

	// bookkeeping — écritures persistantes du claim (registre visiteur,
	// lot-jour, comptabilité, audit). Appelé sous verrou, UNE fois, APRÈS
	// l'application au routeur (pattern du générateur de vouchers).
	bookkeeping := func(db *model.DB) {
		var cmdID string
		if isAgent {
			payload := map[string]any{
				"profile": profileRef(profileCopy),
				"users":   []map[string]any{{"name": voucher.Username, "password": voucher.Password}},
				"batch":   batchID,
			}
			if dataMb > 0 {
				payload["limitBytesTotal"] = dataMb * 1048576
			}
			if timeMin > 0 {
				payload["limitUptimeMin"] = timeMin
			}
			if comment != "" {
				payload["comment"] = comment
			}
			cmd := queueCommandLocked(db, routerCopy.AccountID, routerCopy.ID, model.CmdVoucherBatch, payload)
			cmdID = cmd.ID
		}
		// N°47 — anti-course : la commande est tracée sur le registre du jour
		// AVANT l'append. /status en expose l'état (« provisioned ») au
		// portail, qui n'auto-logue le visiteur qu'une fois l'utilisateur
		// réellement appliqué au routeur par l'agent (check-in ≤ 45 s).
		guest.ClaimCmdID = cmdID
		db.WifiGuests = append(db.WifiGuests, guest)
		// Lot-jour : création ou incrément (Count / TotalCost).
		batchFound := false
		for i := range db.Batches {
			if db.Batches[i].ID == batchID {
				db.Batches[i].Count++
				db.Batches[i].TotalCost += profileCopy.Price
				batchFound = true
				break
			}
		}
		if !batchFound {
			db.Batches = append([]model.Batch{{
				ID: batchID, AccountID: site.AccountID,
				ProfileID: profileCopy.ID, ProfileName: profileCopy.Name,
				RouterID: routerCopy.ID, RouterName: routerCopy.Name,
				Count: 1, UnitPrice: profileCopy.Price, TotalCost: profileCopy.Price,
				DataQuotaMb: dataMb, TimeLimitMin: timeMin,
				Channel: "direct", CreatedAt: model.NowISO(),
			}}, db.Batches...)
		}
		// Comptabilité du coût gratuit : une vente n'est tracée que si le profil
		// choisi a un prix (profil « WiFi Offert » à 0 F ⇒ zéro bruit comptable).
		if profileCopy.Price > 0 {
			selling := profileCopy.Price
			if profileCopy.SellingPrice > 0 {
				selling = profileCopy.SellingPrice
			}
			db.Sales = append(db.Sales, model.Sale{
				ID: model.NewID("sale-"), AccountID: site.AccountID,
				Amount: profileCopy.Price, ProfileName: profileCopy.Name, Count: 1,
				Channel: "direct", RouterID: routerCopy.ID, RouterName: routerCopy.Name,
				BatchID: batchID, At: model.NowISO(),
				Cost: profileCopy.Price, SellingTotal: selling,
			})
		}
		a.logActivityBy(r, db, site.AccountID, "wifi",
			fmt.Sprintf("WiFi jetable «%s» : code délivré au %s (lot %s%s)", site.Name, maskPhone(phone), batchID, cmdNote(cmdID)))
	}

	if isAgent {
		// Mode agent (production) : miroir cloud + commande voucher_batch en
		// file — le routeur créera l'utilisateur au prochain check-in (≤ 45 s).
		a.store.Lock()
		db = a.store.Data()
		db.HotspotUsers = append(db.HotspotUsers, voucher)
		bookkeeping(db)
		a.store.Save()
		a.store.Unlock()
	} else {
		// Routeurs simulé/réel : push HORS verrou (le simulateur insère
		// lui-même le voucher dans le store), puis bookkeeping.
		gw := a.gatewayFor(routerCopy)
		v := voucher
		if err := gw.AddUser(&v); err != nil {
			writeErr(w, http.StatusBadGateway, "Création du ticket impossible : "+err.Error())
			return
		}
		a.store.Lock()
		db = a.store.Data()
		bookkeeping(db)
		a.store.Save()
		a.store.Unlock()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"duplicate":     false,
		"waitForRouter": isAgent,
		"code":          voucher.Username,
		"loginUrl":      wifiLoginURL(loginBase, voucher.Username),
		"timeLimitMin":  timeMin,
		"dataQuotaMb":   dataMb,
		"profileName":   voucher.ProfileName,
		"siteName":      site.Name,
		"optIn":         guest.OptIn, // N°69 — état marketing du numéro (carte code)
	})
}

// cmdNote — suffixe de traçabilité de la commande agent (vide si non-agent).
func cmdNote(cmdID string) string {
	if cmdID == "" {
		return ""
	}
	return ", commande " + cmdID
}

// maskPhone — téléphone partiellement masqué dans les traces d'activité
// (page publique = pas d'acteur authentifié ; le numéro complet reste dans
// le registre WifiGuest, accessible au gérant uniquement).
func maskPhone(phone string) string {
	if len(phone) <= 4 {
		return phone
	}
	return strings.Repeat("*", len(phone)-4) + phone[len(phone)-4:]
}

// wifiProvisioned — N°47 : le code du visiteur est-il déjà appliqué au
// routeur ? En mode agent, le claim met le voucher en file (voucher_batch) ;
// l'utilisateur n'existe sur le MikroTik qu'après le check-in de l'agent
// (≤ 45 s). Le portail lit ce champ via /status et n'auto-logue qu'une fois
// « provisioned » vrai — sinon le CHAP échoue (« invalid username or
// password ») et le visiteur croit le service cassé. Modes simulated/real :
// le push est synchrone dans le claim ⇒ toujours true. Commande inconnue
// (registre antérieur à N°47) : true — on ne bloque jamais par défaut.
// À appeler sous verrou.
func wifiProvisioned(db *model.DB, g *model.WifiGuest, site *model.WifiSite) bool {
	if g == nil || g.ClaimCmdID == "" {
		return true
	}
	var router *model.Router
	for i := range db.Routers {
		if db.Routers[i].ID == site.RouterID {
			router = &db.Routers[i]
			break
		}
	}
	if router == nil || router.Mode != "agent" {
		return true
	}
	for i := range db.Commands {
		cmd := &db.Commands[i]
		if cmd.ID == g.ClaimCmdID {
			return cmd.Status == "done" || cmd.DoneAt != ""
		}
	}
	return true
}

// handleWifiStatus — état du ticket du jour pour un téléphone : none | active
// | exhausted + offres payantes (bascule 1 clic côté page publique).
func (a *API) handleWifiStatus(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	phone := model.NormalizeWifiPhone(r.URL.Query().Get("phone"))
	if phone == "" {
		writeErr(w, http.StatusBadRequest, "Numéro de téléphone invalide")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	site := findWifiSiteBySlug(db, slug)
	if site == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Site WiFi introuvable")
		return
	}
	// N°75 — veille adaptative : la page de claim POLL ce statut toutes les 5 s
	// — chaque poll marque le routeur du site (l'attente de provisioning garde
	// le mode rapide, l'auto-login n'attend pas un cycle de veille).
	a.markAttention("rt:" + site.RouterID)
	settings := ensureSettings(db, site.AccountID)
	dayKey := model.WifiDayKey(settings.Tenant.Timezone, time.Now().UTC())
	resp := map[string]any{"state": "none", "active": site.Active}
	// N°69 — l'état marketing suit le NUMÉRO (toutes lignes confondues, pas
	// seulement le jour courant) : la carte code de la page /wifi l'affiche
	// (lien « Ne plus recevoir ») même à un re-scan sans nouveau claim.
	resp["optIn"] = wifiPhoneConsented(db, site.AccountID, phone)
	if site.Active && a.subscriptionGuardStateLocked(site.AccountID).Status != "expired" {
		resp["offers"] = wifiOffers(db, site)
	}
	var latest *model.WifiGuest
	for i := range db.WifiGuests {
		g := &db.WifiGuests[i]
		if g.AccountID == site.AccountID && g.SiteID == site.ID && g.Phone == phone && g.Day == dayKey {
			if latest == nil || g.CreatedAt > latest.CreatedAt {
				latest = g
			}
		}
	}
	if latest != nil {
		v := wifiResolveVoucher(db, latest)
		if wifiVoucherUsable(v) {
			resp["state"] = "active"
			resp["code"] = v.Username
			resp["loginUrl"] = wifiLoginURL(wifiLoginBase(db, site), v.Username)
			resp["timeLimitMin"] = v.TimeLimitMin
			resp["dataQuotaMb"] = v.DataQuotaMb
			// N°47 — le code est-il déjà appliqué au routeur ? (anti-course)
			resp["provisioned"] = wifiProvisioned(db, latest, site)
		} else {
			resp["state"] = "exhausted"
		}
	}
	a.store.Unlock()
	writeJSON(w, http.StatusOK, resp)
}

// handleWifiConsent — N°69 : bascule du consentement marketing d'un numéro
// (le retrait « Ne plus recevoir » de la carte code /wifi, un éventuel
// opt-in post-claim). Endpoint PUBLIC — mêmes gardes que le claim :
//
//   - rate-limit du bucket wifiClaim (20/10 min + 100/24 h par IP) ;
//   - honeypot « website » (champ invisible ; un bot rempli reçoit un
//     succès FACTICE, rien n'est écrit) ;
//   - validation stricte du téléphone (8-15 chiffres) ;
//   - optIn=true réduit par site.MarketingOptIn (le marketing éteint sur
//     le site ⇒ l'interrupteur n'est pas proposé, un opt-in sauvage ne
//     s'enregistre pas — symétrie exacte avec le claim).
//
// Sémantique : l'état suit le NUMÉRO — TOUTES les lignes du registre du
// téléphone passent à l'état demandé (le retrait est immédiat et complet,
// l'opt-in pose sa preuve horodatée OptInAt). Le retrait reste possible
// même site en pause ou compte expiré : un droit de retrait ne se suspend
// jamais. Réponse : {"ok":true,"optIn":<état effectif>,"updated":<lignes>}.
func (a *API) handleWifiConsent(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	var req struct {
		Phone string `json:"phone"`
		OptIn bool   `json:"optIn"`
		// N°50/N°69 — honeypot « website » (champ invisible ; un bot rempli
		// reçoit un succès FACTICE, rien n'est écrit) — même contrat que le claim.
		Website string `json:"website"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if ok, retry := a.wifiClaim.allow(clientIP(r)); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeErr(w, http.StatusTooManyRequests, "Trop de tentatives — réessayez plus tard")
		return
	}
	phone := model.NormalizeWifiPhone(req.Phone)
	if phone == "" {
		writeErr(w, http.StatusBadRequest, "Numéro de téléphone invalide (8 à 15 chiffres, indicatif inclus)")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	site := findWifiSiteBySlug(db, slug)
	if site == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Site WiFi introuvable")
		return
	}
	// Honeypot (même contrat que le claim) : succès FACTICE — même forme
	// JSON, aucune écriture, aucun indice sur le filtre.
	if strings.TrimSpace(req.Website) != "" {
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "optIn": req.OptIn, "updated": 1})
		return
	}
	optIn := req.OptIn && site.MarketingOptIn
	updated := wifiSetPhoneConsent(db, site.AccountID, phone, optIn)
	if updated > 0 {
		state := "retiré"
		if optIn {
			state = "activé"
		}
		a.logActivityBy(r, db, site.AccountID, "wifi",
			fmt.Sprintf("WiFi jetable «%s» : consentement marketing %s par %s (%d ligne(s))",
				site.Name, state, maskPhone(phone), updated))
		a.store.Save()
	}
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "optIn": optIn, "updated": updated})
}

// handleWifiPortal — N°35-c : config LIVE du portail captif pour le site
// WiFi jetable {slug}. Endpoint PUBLIC (pas de JWT, pas de token agent) —
// appelé par la page login.html SERVIE PAR LE ROUTEUR au chargement, pour
// récupérer une config FRAICHE qui prime sur le fallback inliné au déploiement.
//
// Parcours :
//  1. Le routeur déploie login.html via l'agent (hotspot_files). Le fichier
//     contient un bloc <script id="mikcloud-config"> avec la config au moment
//     du déploiement (fallback si le cloud est injoignable).
//  2. Au chargement côté client, login.html lit ce bloc PUIS fetch ce
//     endpoint /api/wifi/site/{slug}/portal pour récupérer la config LIVE.
//  3. Si le fetch réussit (200), la config live prime sur le fallback —
//     le gérant peut changer branding/offres/textes sans re-déployer le
//     portail. Si le fetch échoue (cloud injoignable, slug introuvable,
//     site inactif), le fallback inliné est utilisé — le portail reste
//     fonctionnel même hors connexion cloud.
//
// Rate-limit : `wifi-read` (30/min/IP) — cf. main.go authRateLimit. La page
// charge une fois par visite, le plafond est largement suffisant.
//
// CORS : ouverte à toute origine (cf. corsMiddleware — le portail routeur a
// une origine imprévisible). Pas de cookie, pas de JWT → ouverture cohérente
// avec la nature publique de l'endpoint.
//
// Réponse 404 si le slug n'existe pas. Sinon 200 avec un PortalConfig qui
// transporte N°51 l'état réel : site actif → sa config (active:true) ; site
// en pause → la config FRAÎCHE du routeur (wifiSlug vide + active:false, ou
// le 1er AUTRE site actif du routeur) — la page retire la carte claim du
// fallback inliné périmé sans attendre un re-déploiement.
func (a *API) handleWifiPortal(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	a.store.Lock()
	db := a.store.Data()
	site := findWifiSiteBySlug(db, slug)
	if site == nil {
		a.store.Unlock()
		writeErrCode(w, http.StatusNotFound, "site_unknown", "Site WiFi introuvable", nil)
		return
	}
	// Le routeur du site WiFi est le point d'ancrage de la config. On retrouve
	// le routeur pour résoudre le lien join lié (1er JoinLink actif du routeur)
	// et le branding (settings du compte). Si le routeur a été supprimé mais
	// pas le site WiFi (cas théorique — la suppression du routeur ne cascade
	// pas vers les sites WiFi), on continue avec un Router vide — le join_url
	// et le wave_link peuvent rester vides, le fallback inliné prendra le
	// relais côté page.
	var router *model.Router
	for i := range db.Routers {
		if db.Routers[i].ID == site.RouterID && db.Routers[i].AccountID == site.AccountID {
			router = &db.Routers[i]
			break
		}
	}
	// N°75 — veille adaptative : chaque chargement du portail (invité qui
	// vient de se connecter au WiFi) marque le routeur du site — il sera
	// en mode rapide à son prochain check-in, AVANT le claim éventuel.
	a.markAttention("rt:" + site.RouterID)
	cfg := buildPortalConfigForSite(db, site, router, r)
	if !site.Active && router != nil {
		// N°51 — site en pause : config fraîche du ROUTEUR (1er site actif
		// lié — éventuellement un autre — sinon wifiSlug vide + active:false).
		// Auto-réparation sans re-déploiement : la page retire la carte du
		// fallback périmé, ou la rebranche sur le bon slug.
		cfg = buildPortalConfig(db, router, r)
	}
	a.store.Unlock()
	writeJSONCacheable(w, r, http.StatusOK, cfg) // N°74 — ETag/304 : la config live est stable des heures durant
}

// ---------------------------------------------------------------------------
// Endpoints CONSOLE (rôle gérant+)
// ---------------------------------------------------------------------------

// wifiSitePayload — corps commun create/update.
type wifiSitePayload struct {
	Name           string `json:"name"`
	RouterID       string `json:"routerId"`
	ProfileID      string `json:"profileId"`
	FreeTimeMin    int64  `json:"freeTimeMin"`
	FreeDataMb     int64  `json:"freeDataMb"`
	MarketingOptIn bool   `json:"marketingOptIn"`
	DailyPerPhone  int    `json:"dailyPerPhone"`
	DailyPerMac    int    `json:"dailyPerMac"` // N°50 — tickets max / appareil (MAC) / jour
	DailyCap       int    `json:"dailyCap"`
	WifiSSID       string `json:"wifiSsid"`     // N°49 — QR de connexion de l'affiche (vide = pas de QR WiFi)
	WifiPassword   string `json:"wifiPassword"` // N°49 — mot de passe WPA si le réseau est protégé (vide = ouvert)
	Active         bool   `json:"active"`
}

// validateWifiSitePayload — validations partagées ; renvoie (payload, ok).
// Les quotas par défaut sont AJUSTABLES par le gérant (ajustement demandé) :
// temps/data libres (0 = hériter du profil), plafonds bornés.
func validateWifiSitePayload(w http.ResponseWriter, r *http.Request, a *API, acc string) (*wifiSitePayload, bool) {
	var req wifiSitePayload
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return nil, false
	}
	req.Name = strings.TrimSpace(req.Name)
	if len(req.Name) < 2 || len(req.Name) > 60 {
		writeErr(w, http.StatusBadRequest, "Le nom de l'établissement doit contenir entre 2 et 60 caractères")
		return nil, false
	}
	if req.FreeTimeMin < 0 || req.FreeTimeMin > 2628000 {
		writeErr(w, http.StatusBadRequest, "Le quota de temps offert doit être compris entre 0 et 2628000 minutes")
		return nil, false
	}
	if req.FreeDataMb < 0 || req.FreeDataMb > 1_073_741_824 {
		writeErr(w, http.StatusBadRequest, "Le quota de données offert doit être compris entre 0 et 1073741824 Mo")
		return nil, false
	}
	if req.DailyPerPhone == 0 {
		req.DailyPerPhone = 1
	}
	if req.DailyPerPhone < 1 || req.DailyPerPhone > 10 {
		writeErr(w, http.StatusBadRequest, "Le plafond par téléphone doit être compris entre 1 et 10 tickets par jour")
		return nil, false
	}
	if req.DailyPerMac == 0 {
		req.DailyPerMac = 1
	}
	if req.DailyPerMac < 1 || req.DailyPerMac > 10 {
		writeErr(w, http.StatusBadRequest, "Le plafond par appareil doit être compris entre 1 et 10 tickets par jour")
		return nil, false
	}
	if req.DailyCap == 0 {
		req.DailyCap = 100
	}
	if req.DailyCap < 1 || req.DailyCap > 1000 {
		writeErr(w, http.StatusBadRequest, "Le budget journalier doit être compris entre 1 et 1000 tickets")
		return nil, false
	}
	// NB : la cohérence routeur/profil (scopée au compte) est vérifiée dans
	// les handlers APRÈS résolution du site ciblé — un site d'un AUTRE compte
	// doit répondre 404, pas 400 (isolation multi-tenant).
	req.RouterID = strings.TrimSpace(req.RouterID)
	req.ProfileID = strings.TrimSpace(req.ProfileID)
	// N°49 — QR de connexion WiFi : bornes des normes radio (SSID 32 octets
	// 802.11, phrase secrète WPA 63 caractères). Le mot de passe est stocké
	// en clair VOLONTAIREMENT : sa seule utilité est d'être encodé dans le
	// QR imprimé, donc destiné aux clients de l'établissement.
	req.WifiSSID = strings.TrimSpace(req.WifiSSID)
	if len(req.WifiSSID) > 32 {
		writeErr(w, http.StatusBadRequest, "Le SSID du réseau ne peut pas dépasser 32 caractères (norme 802.11)")
		return nil, false
	}
	req.WifiPassword = strings.TrimSpace(req.WifiPassword)
	if len(req.WifiPassword) > 63 {
		writeErr(w, http.StatusBadRequest, "Le mot de passe WiFi ne peut pas dépasser 63 caractères (norme WPA)")
		return nil, false
	}
	return &req, true
}

// handleWifiSitesList — sites du compte + statistiques du jour (visiteurs,
// codes délivrés, opt-ins cumulés).
func (a *API) handleWifiSitesList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	db := a.store.Data()
	settings := ensureSettings(db, acc)
	dayKey := model.WifiDayKey(settings.Tenant.Timezone, time.Now().UTC())
	sites := make([]model.WifiSite, 0, len(db.WifiSites))
	for _, s := range db.WifiSites {
		if s.AccountID == acc {
			sites = append(sites, s)
		}
	}
	type siteStats struct {
		GuestsToday int `json:"guestsToday"`
		OptInTotal  int `json:"optInTotal"`
	}
	stats := map[string]siteStats{}
	for _, s := range sites {
		st := siteStats{}
		for _, g := range db.WifiGuests {
			if g.AccountID != acc || g.SiteID != s.ID {
				continue
			}
			if g.Day == dayKey {
				st.GuestsToday++
			}
			if g.OptIn {
				st.OptInTotal++
			}
		}
		stats[s.ID] = st
	}
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"sites": sites, "day": dayKey, "stats": stats})
}

// handleWifiSiteCreate — création d'un site : slug unique GLOBALEMENT
// (l'URL publique ne porte pas le compte), dérivé du nom.
func (a *API) handleWifiSiteCreate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	if !a.guardAccountWrite(w, r) {
		return
	}
	req, ok := validateWifiSitePayload(w, r, a, acc)
	if !ok {
		return
	}
	a.store.Lock()
	db := a.store.Data()
	// Routeur + profil scopés au compte (profils créables par le gérant via
	// POST /api/profiles, dont la création inline « nouveau quota » vue WiFi).
	router0 := findRouterScoped(db, req.RouterID, acc)
	if router0 == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Routeur introuvable")
		return
	}
	profile0 := findProfileScoped(db, req.ProfileID, acc)
	if profile0 == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Profil introuvable — créez le quota « WiFi Offert » voulu")
		return
	}
	// Slug unique global : dérivé du nom + suffixe numérique si collision.
	base := model.NormalizeWifiSlug(req.Name)
	if base == "" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Nom d'établissement inexploitable en URL (lettres/chiffres requis)")
		return
	}
	slug := base
	for i := 2; findWifiSiteBySlug(db, slug) != nil && i < 100; i++ {
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	if findWifiSiteBySlug(db, slug) != nil {
		slug = base + "-" + strings.TrimPrefix(model.NewID(""), "0")
	}
	site := model.WifiSite{
		ID: model.NewID("ws-"), AccountID: acc,
		Name: req.Name, Slug: slug,
		RouterID: router0.ID, RouterName: router0.Name,
		ProfileID: profile0.ID, ProfileName: profile0.Name,
		FreeTimeMin: req.FreeTimeMin, FreeDataMb: req.FreeDataMb,
		MarketingOptIn: req.MarketingOptIn,
		DailyPerPhone:  req.DailyPerPhone, DailyPerMac: req.DailyPerMac, DailyCap: req.DailyCap,
		WifiSSID: req.WifiSSID, WifiPassword: req.WifiPassword,
		Active: req.Active, CreatedAt: model.NowISO(),
	}
	db.WifiSites = append(db.WifiSites, site)
	// N°51 — création d'un site ACTIF : la carte claim doit apparaître sur le
	// portail du routeur. Tant qu'aucun site actif n'existait au déploiement, le
	// fallback inliné n'a pas de slug et la page ne fetch JAMAIS la config live
	// → re-déploiement forcé (sig vidée, re-filée au prochain check-in ≤ 45 s).
	// Inerte hors mode agent (aucun check-in, ensureHotspotFilesLocked no-op).
	if req.Active {
		router0.HotspotFilesSig = ""
	}
	a.logActivityBy(r, db, acc, "wifi", fmt.Sprintf("Site WiFi jetable «%s» créé (/wifi/%s, routeur %s)", site.Name, site.Slug, site.RouterName))
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, site)
}

// handleWifiSiteUpdate — mise à jour complète (ajustement des quotas,
// plafonds, profil, routeur) + BASCULE 1 CLIC : champ « active ».
// Le slug est VOLONTAIREMENT immuable (les QR codes sont imprimés).
func (a *API) handleWifiSiteUpdate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	if !a.guardAccountWrite(w, r) {
		return
	}
	id := r.PathValue("id")
	req, ok := validateWifiSitePayload(w, r, a, acc)
	if !ok {
		return
	}
	a.store.Lock()
	db := a.store.Data()
	// Site d'abord : un site d'un autre compte répond 404 (isolation), avant
	// toute validation scopée au compte du demandeur.
	site := findWifiSiteByID(db, id, acc)
	if site == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Site WiFi introuvable")
		return
	}
	oldRouterID := site.RouterID // N°51 — pour le re-déploiement si le site change de routeur
	router := findRouterScoped(db, req.RouterID, acc)
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Routeur introuvable")
		return
	}
	profile := findProfileScoped(db, req.ProfileID, acc)
	if profile == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Profil introuvable — créez le quota « WiFi Offert » voulu")
		return
	}
	toggled := ""
	if site.Active != req.Active {
		if req.Active {
			toggled = "activé"
		} else {
			toggled = "désactivé"
		}
	}
	site.Name = req.Name
	site.RouterID = router.ID
	site.RouterName = router.Name
	site.ProfileID = profile.ID
	site.ProfileName = profile.Name
	site.FreeTimeMin = req.FreeTimeMin
	site.FreeDataMb = req.FreeDataMb
	site.MarketingOptIn = req.MarketingOptIn
	site.DailyPerPhone = req.DailyPerPhone
	site.DailyPerMac = req.DailyPerMac
	site.DailyCap = req.DailyCap
	site.WifiSSID = req.WifiSSID
	site.WifiPassword = req.WifiPassword
	site.Active = req.Active
	// N°51 — bascule active (1 clic) ou changement de routeur : la présence de
	// la carte claim sur le portail change → re-déploiement forcé du portail
	// (sig vidée, re-filée au prochain check-in ≤ 45 s). La config live
	// (active:false) retire déjà la carte dès la visite suivante ; ce
	// re-déploiement resynchronise le fallback inliné — indispensable dans le
	// sens « activé après coup » (fallback sans slug = pas de fetch live).
	if toggled != "" || oldRouterID != router.ID {
		router.HotspotFilesSig = ""
		if old := findRouterScoped(db, oldRouterID, acc); old != nil && old.ID != router.ID {
			old.HotspotFilesSig = ""
		}
	}
	msg := fmt.Sprintf("Site WiFi jetable «%s» mis à jour (quotas : %d min / %d Mo, plafonds : %d/tél, %d/appareil, %d/site)", site.Name, site.FreeTimeMin, site.FreeDataMb, site.DailyPerPhone, site.DailyPerMac, site.DailyCap)
	if toggled != "" {
		msg = fmt.Sprintf("WiFi jetable «%s» %s (bascule 1 clic)", site.Name, toggled)
	}
	a.logActivityBy(r, db, acc, "wifi", msg)
	a.store.Save()
	updated := *site
	a.store.Unlock()
	writeJSON(w, http.StatusOK, updated)
}

// handleWifiSiteDelete — suppression du site + de son registre visiteurs
// (droit à l'effacement) ; les tickets déjà émis restent soumis à leurs
// quotas et au cycle d'expiration / sweep N°26.
func (a *API) handleWifiSiteDelete(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	if !a.guardAccountWrite(w, r) {
		return
	}
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	site := findWifiSiteByID(db, id, acc)
	if site == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Site WiFi introuvable")
		return
	}
	removed := 0
	guests := db.WifiGuests[:0]
	for _, g := range db.WifiGuests {
		if g.SiteID == site.ID {
			removed++
			continue
		}
		guests = append(guests, g)
	}
	db.WifiGuests = guests
	sites := db.WifiSites[:0]
	for _, s := range db.WifiSites {
		if s.ID == site.ID {
			continue
		}
		sites = append(sites, s)
	}
	db.WifiSites = sites
	// N°51 — le site supprimé ne doit plus proposer sa carte claim : sig
	// hotspot_files vidée → re-déploiement au prochain check-in (≤ 45 s).
	// D'ici là, la config live (buildPortalConfig — plus aucun site actif lié)
	// fait déjà retirer la carte par la page.
	if router := findRouterScoped(db, site.RouterID, acc); router != nil {
		router.HotspotFilesSig = ""
	}
	a.logActivityBy(r, db, acc, "wifi", fmt.Sprintf("Site WiFi jetable «%s» supprimé (%d entrées de registre effacées)", site.Name, removed))
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removedGuests": removed})
}

// handleWifiGuests — registre marketing (gérant uniquement) : liste filtrable
// + export CSV (opt-in ou tout) — la base « votre WiFi devient votre base
// clients ».
func (a *API) handleWifiGuests(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	siteID := r.URL.Query().Get("siteId")
	optInFilter := r.URL.Query().Get("optIn")
	export := r.URL.Query().Get("export")
	a.store.Lock()
	db := a.store.Data()
	rows := []model.WifiGuest{}
	for _, g := range db.WifiGuests {
		if g.AccountID != acc {
			continue
		}
		if siteID != "" && g.SiteID != siteID {
			continue
		}
		switch optInFilter {
		case "true":
			if !g.OptIn {
				continue
			}
		case "false":
			if g.OptIn {
				continue
			}
		}
		rows = append(rows, g)
	}
	// Plus récents d'abord, plafond de réponse.
	for i := 0; i < len(rows)/2; i++ {
		j := len(rows) - 1 - i
		rows[i], rows[j] = rows[j], rows[i]
	}
	if len(rows) > 2000 {
		rows = rows[:2000]
	}
	a.store.Unlock()

	if export == "csv" {
		var sb strings.Builder
		// N°69 — opt_in_since : preuve horodatée du consentement
		// (colonne vide « - » = jamais consenti / retiré). La base
		// marketing légale du gérant = filtre optIn + cette colonne.
		sb.WriteString("date;telephone;appareil;ip;opt_in;opt_in_since;code;site\r\n") // N°50 — appareil (MAC) + ip pour l'audit anti-abus
		for _, g := range rows {
			sb.WriteString(g.CreatedAt)
			sb.WriteByte(';')
			sb.WriteString(g.Phone)
			sb.WriteByte(';')
			sb.WriteString(g.Mac)
			sb.WriteByte(';')
			sb.WriteString(g.IP)
			sb.WriteByte(';')
			if g.OptIn {
				sb.WriteString("oui")
			} else {
				sb.WriteString("non")
			}
			sb.WriteByte(';')
			if g.OptInAt != "" {
				sb.WriteString(g.OptInAt)
			} else {
				sb.WriteString("-")
			}
			sb.WriteByte(';')
			sb.WriteString(g.Code)
			sb.WriteByte(';')
			sb.WriteString(g.SiteName)
			sb.WriteString("\r\n")
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="wifi-guests.csv"`)
		_, _ = w.Write([]byte(sb.String()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"guests": rows, "count": len(rows)})
}
