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
// refusée 402). Registre marketing : WifiGuest (opt-in explicite, export CSV
// console, loi ivoirienne n°2013-450 / ARTCI-CIL : consentement et finalité
// affichés, suppression à la demande via DELETE du site).
package api

import (
	"fmt"
	"net/http"
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
	writeJSON(w, http.StatusOK, resp)
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
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
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
	routerCopy := *router
	profileCopy := *profile
	settings := ensureSettings(db, site.AccountID)
	now := time.Now().UTC()
	dayKey := model.WifiDayKey(settings.Tenant.Timezone, now)

	// Registre du jour : (compte, site, téléphone) — base de l'idempotence
	// et des plafonds.
	today := []model.WifiGuest{}
	siteToday := 0
	for _, g := range db.WifiGuests {
		if g.AccountID != site.AccountID || g.Day != dayKey {
			continue
		}
		if g.SiteID == site.ID {
			siteToday++
			if g.Phone == phone {
				today = append(today, g)
			}
		}
	}

	// Idempotence : re-claim du même téléphone ⇒ le MÊME code (tant qu'il est
	// utilisable) — rafraîchissement de page, mobile qui ré-ouvre, pas de
	// double émission, pas de farm de codes.
	for i := len(today) - 1; i >= 0; i-- {
		v := wifiResolveVoucher(db, &today[i])
		if wifiVoucherUsable(v) {
			a.store.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{
				"duplicate":    true,
				"code":         v.Username,
				"loginUrl":     wifiLoginURL(wifiLoginBase(db, site), v.Username),
				"timeLimitMin": v.TimeLimitMin,
				"dataQuotaMb":  v.DataQuotaMb,
				"profileName":  v.ProfileName,
				"siteName":     site.Name,
			})
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
		writeErrCode(w, http.StatusTooManyRequests, "phone_cap", "Votre WiFi offert du jour est déjà consommé — passez à une offre payante", nil)
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
	guest := model.WifiGuest{
		ID: model.NewID("wg-"), AccountID: site.AccountID,
		SiteID: site.ID, SiteName: site.Name, Phone: phone,
		OptIn:     req.OptIn && site.MarketingOptIn, // opt-in tracé seulement si la case est proposée
		VoucherID: voucher.ID, Code: code, Day: dayKey, CreatedAt: model.NowISO(),
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
		db.WifiGuests = append(db.WifiGuests, guest)
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
		"duplicate":    false,
		"code":         voucher.Username,
		"loginUrl":     wifiLoginURL(loginBase, voucher.Username),
		"timeLimitMin": timeMin,
		"dataQuotaMb":  dataMb,
		"profileName":  voucher.ProfileName,
		"siteName":     site.Name,
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
	settings := ensureSettings(db, site.AccountID)
	dayKey := model.WifiDayKey(settings.Tenant.Timezone, time.Now().UTC())
	resp := map[string]any{"state": "none", "active": site.Active}
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
		} else {
			resp["state"] = "exhausted"
		}
	}
	a.store.Unlock()
	writeJSON(w, http.StatusOK, resp)
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
	DailyCap       int    `json:"dailyCap"`
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
		DailyPerPhone:  req.DailyPerPhone, DailyCap: req.DailyCap,
		Active: req.Active, CreatedAt: model.NowISO(),
	}
	db.WifiSites = append(db.WifiSites, site)
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
	site.DailyCap = req.DailyCap
	site.Active = req.Active
	msg := fmt.Sprintf("Site WiFi jetable «%s» mis à jour (quotas : %d min / %d Mo, plafonds : %d/tél, %d/site)", site.Name, site.FreeTimeMin, site.FreeDataMb, site.DailyPerPhone, site.DailyCap)
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
		sb.WriteString("date;telephone;opt_in;code;site\r\n")
		for _, g := range rows {
			sb.WriteString(g.CreatedAt)
			sb.WriteByte(';')
			sb.WriteString(g.Phone)
			sb.WriteByte(';')
			if g.OptIn {
				sb.WriteString("oui")
			} else {
				sb.WriteString("non")
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
