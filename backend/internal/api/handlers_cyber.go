// handlers_cyber.go — N°290 — module Cybercafé (overlay du mode hotspot).
//
// Un « poste » est un appareil du cybercafé identifié par sa MAC (même
// identité stable que les appareils HomeNet N°101). Le module ajoute :
//   - le registre des postes (saisie manuelle ou import des bails DHCP déjà
//     rapportés par read_dhcp — les comptes du module bénéficient de la même
//     cadence d'inventaire que les foyers, garde élargi de
//     ensureHomeDevicesLocked) ;
//   - l'attribution d'un CODE-TEMPS par poste (voucher limit-uptime créé sur
//     le gabarit de génération N°25/F13 : un code = un poste, mode
//     « utilisateur = mot de passe » verrouillé, validité ancrée au 1er
//     login, trace d'origine SoldVia = « cyber_poste » pattern N°8) ;
//   - la pause/reprise PAR POSTE (commande device_pause N°101 réutilisée —
//     l'ensemble désiré fusionne appareils foyers et postes cyber dans
//     desiredPauseMacsLocked, même marqueur mikcloud-pause, même idempotence) ;
//   - la CAISSE : Transaction (sale) + Sale enregistrées à l'attribution,
//     canal « direct » — visibles immédiatement dans rapports, compta et
//     journaux mensuels existants (aucun double compteur à maintenir).
//
// Le module est ACTIVABLE par compte (settings.Tenant.CyberEnabled, défaut
// OFF) — décision D6 de N°289 : un flag, PAS un nouvel usage (l'enum
// hotspot|homenet reste inchangé). Toutes les routes exigent un compte
// hotspot (requireUsage) ; les handlers vérifient le flag (403
// « cyber_disabled » — la console propose alors l'activation en un clic).
package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// cyberEnabledLocked — le module Cybercafé est-il actif pour CE compte ?
// (settings.Tenant.CyberEnabled — nil = OFF, module opt-in.) Sous verrou.
func cyberEnabledLocked(db *model.DB, acc string) bool {
	s, ok := db.SettingsByAccount[acc]
	if !ok {
		return false
	}
	return s.Tenant.CyberModuleEnabled()
}

// requireCyber — garde commune des handlers cyber : le module doit être
// activé. Appelée SOUS VERROU (lecture des settings) ; écrit la réponse
// d'erreur et renvoie false sinon.
func (a *API) requireCyber(w http.ResponseWriter, db *model.DB, acc string) bool {
	if cyberEnabledLocked(db, acc) {
		return true
	}
	writeErrCode(w, http.StatusForbidden, "cyber_disabled",
		"Module Cybercafé désactivé — activez-le pour continuer", nil)
	return false
}

// ---------------------------------------------------------------------------
// Réponses console
// ---------------------------------------------------------------------------

// cyberLinkedUser — le code-temps lié au poste, vu par la console (statut
// EFFECTIF calculé — expiré prime sur le statut stocké, même règle que la
// vue Utilisateurs) + le quota temps et son cumul pour l'affichage.
type cyberLinkedUser struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Status        string `json:"status"`
	TimeLimitMin  int64  `json:"timeLimitMin"`
	UptimeUsedSec int64  `json:"uptimeUsedSec"`
}

// cyberPosteResponse — un poste + son code-temps lié (nil si libre).
type cyberPosteResponse struct {
	model.CyberPoste
	LinkedUser *cyberLinkedUser `json:"linkedUser"`
}

// cyberPosteResponseLocked — construit la réponse sous verrou (l'isolation
// multi-tenant se vérifie à chaque lecture : le voucher lié doit appartenir
// au MÊME compte que le poste).
func cyberPosteResponseLocked(db *model.DB, p model.CyberPoste) cyberPosteResponse {
	out := cyberPosteResponse{CyberPoste: p}
	if p.ActiveUserID == "" {
		return out
	}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.ID == p.ActiveUserID && u.AccountID == p.AccountID {
			out.LinkedUser = &cyberLinkedUser{
				ID:            u.ID,
				Username:      u.Username,
				Status:        model.EffectiveStatus(u, time.Now().UTC()),
				TimeLimitMin:  u.TimeLimitMin,
				UptimeUsedSec: u.UptimeUsedSec,
			}
			break
		}
	}
	return out
}

// findCyberPosteIdx — index du poste scoping compte (miroir findDeviceScoped).
func findCyberPosteIdx(db *model.DB, id, acc string) int {
	for i := range db.CyberPostes {
		if db.CyberPostes[i].ID == id && db.CyberPostes[i].AccountID == acc {
			return i
		}
	}
	return -1
}

// cyberPosteTaken — un poste existe déjà pour cette MAC sur CE routeur ?
func cyberPosteTaken(db *model.DB, acc, routerID, mac string) bool {
	for i := range db.CyberPostes {
		if db.CyberPostes[i].AccountID == acc && db.CyberPostes[i].RouterID == routerID && db.CyberPostes[i].MAC == mac {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Registre : liste, création, import DHCP, renommage, suppression
// ---------------------------------------------------------------------------

// handleCyberPostesList — GET /api/cyber/postes : les postes du compte,
// triés par création décroissante (même discipline que la liste des sites).
func (a *API) handleCyberPostesList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	db := a.store.Data()
	if !a.requireCyber(w, db, acc) {
		a.store.Unlock()
		return
	}
	out := []cyberPosteResponse{}
	for i := range db.CyberPostes {
		if db.CyberPostes[i].AccountID == acc {
			out = append(out, cyberPosteResponseLocked(db, db.CyberPostes[i]))
		}
	}
	a.store.Unlock()
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	writeJSON(w, http.StatusOK, out)
}

// handleCyberPosteCreate — POST /api/cyber/postes {mac, name?, routerId} :
// saisie manuelle d'un poste (le gérant lit la MAC derrière le PC ou la
// reprend d'une étiquette). Unicité MAC par (compte, routeur), plafond
// MaxCyberPostesPerAccount — même discipline que les sites/appareils.
func (a *API) handleCyberPosteCreate(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	var req struct {
		MAC      string `json:"mac"`
		Name     string `json:"name"`
		RouterID string `json:"routerId"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	mac := model.NormalizeMAC(req.MAC)
	if mac == "" {
		writeErr(w, http.StatusBadRequest, "MAC invalide (format attendu AA:BB:CC:DD:EE:FF)")
		return
	}
	name := model.SanitizeDeviceName(req.Name)
	a.store.Lock()
	db := a.store.Data()
	if !a.requireCyber(w, db, acc) {
		a.store.Unlock()
		return
	}
	router := findRouterScoped(db, strings.TrimSpace(req.RouterID), acc)
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if cyberPosteTaken(db, acc, router.ID, mac) {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Un poste avec cette MAC existe déjà sur ce routeur")
		return
	}
	count := 0
	for i := range db.CyberPostes {
		if db.CyberPostes[i].AccountID == acc {
			count++
		}
	}
	if count >= model.MaxCyberPostesPerAccount {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Nombre maximum de postes atteint (100)")
		return
	}
	now := model.NowISO()
	p := model.CyberPoste{
		ID: model.NewID("cp-"), AccountID: acc,
		RouterID: router.ID, RouterName: router.Name,
		MAC: mac, Name: name,
		CreatedAt: now, UpdatedAt: now,
	}
	db.CyberPostes = append(db.CyberPostes, p)
	a.logActivityBy(r, db, acc, "cyber", "Poste ajouté : "+model.CyberPosteLabel(p))
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, cyberPosteResponse{CyberPoste: p})
}

// handleCyberPosteImport — POST /api/cyber/postes/import {deviceId} : crée
// un poste depuis une ligne du registre DHCP (la découverte read_dhcp
// alimente db.Devices pour les comptes du module — garde élargi N°290). La
// MAC et l'IP sont reprises ; le host-name DHCP suggère le nom du poste.
func (a *API) handleCyberPosteImport(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	var req struct {
		DeviceID string `json:"deviceId"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	if !a.requireCyber(w, db, acc) {
		a.store.Unlock()
		return
	}
	idx := findDeviceScoped(db, strings.TrimSpace(req.DeviceID), acc)
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Appareil découvert introuvable")
		return
	}
	d := &db.Devices[idx]
	if cyberPosteTaken(db, acc, d.RouterID, d.MAC) {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Cette MAC est déjà enregistrée comme poste")
		return
	}
	count := 0
	for i := range db.CyberPostes {
		if db.CyberPostes[i].AccountID == acc {
			count++
		}
	}
	if count >= model.MaxCyberPostesPerAccount {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Nombre maximum de postes atteint (100)")
		return
	}
	now := model.NowISO()
	p := model.CyberPoste{
		ID: model.NewID("cp-"), AccountID: acc,
		RouterID: d.RouterID, RouterName: d.RouterName,
		MAC: d.MAC, Name: model.SanitizeDeviceName(d.Hostname), IP: d.IP,
		CreatedAt: now, UpdatedAt: now,
	}
	db.CyberPostes = append(db.CyberPostes, p)
	a.logActivityBy(r, db, acc, "cyber", "Poste importé depuis le DHCP : "+model.CyberPosteLabel(p)+" ("+d.MAC+")")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, cyberPosteResponse{CyberPoste: p})
}

// handleCyberDiscover — GET /api/cyber/discover : les appareils connus du
// compte (rapports read_dhcp) avec l'indicateur « imported » — la console
// propose l'import en un clic des MAC pas encore enregistrées comme postes.
func (a *API) handleCyberDiscover(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	db := a.store.Data()
	if !a.requireCyber(w, db, acc) {
		a.store.Unlock()
		return
	}
	out := []map[string]any{}
	for i := range db.Devices {
		d := &db.Devices[i]
		if d.AccountID != acc {
			continue
		}
		out = append(out, map[string]any{
			"id":         d.ID,
			"mac":        d.MAC,
			"ip":         d.IP,
			"hostname":   d.Hostname,
			"status":     d.Status,
			"routerId":   d.RouterID,
			"routerName": d.RouterName,
			"imported":   cyberPosteTaken(db, acc, d.RouterID, d.MAC),
		})
	}
	a.store.Unlock()
	sort.SliceStable(out, func(i, j int) bool {
		ri, _ := out[i]["routerName"].(string)
		rj, _ := out[j]["routerName"].(string)
		if ri != rj {
			return ri < rj
		}
		mi, _ := out[i]["mac"].(string)
		mj, _ := out[j]["mac"].(string)
		return mi < mj
	})
	writeJSON(w, http.StatusOK, out)
}

// handleCyberPosteUpdate — PUT /api/cyber/postes/{id} {name} : renommage.
func (a *API) handleCyberPosteUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Name *string `json:"name"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	if !a.requireCyber(w, db, acc) {
		a.store.Unlock()
		return
	}
	p := model.FindCyberPosteScoped(db, id, acc)
	if p == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Poste introuvable")
		return
	}
	if req.Name != nil {
		p.Name = model.SanitizeDeviceName(*req.Name)
	}
	p.UpdatedAt = model.NowISO()
	resp := cyberPosteResponseLocked(db, *p)
	a.logActivityBy(r, db, acc, "cyber", "Poste renommé : "+model.CyberPosteLabel(*p))
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, resp)
}

// handleCyberPosteDelete — DELETE /api/cyber/postes/{id} : le poste quitte
// le registre (le code-temps lié, lui, vit sa vie dans Vouchers). Si le
// poste était en pause, l'ensemble désiré change → re-file device_pause
// (la coupure se lève au prochain check-in, pattern N°101).
func (a *API) handleCyberPosteDelete(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	if !a.requireCyber(w, db, acc) {
		a.store.Unlock()
		return
	}
	idx := findCyberPosteIdx(db, id, acc)
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Poste introuvable")
		return
	}
	p := db.CyberPostes[idx]
	wasPaused := p.Paused
	db.CyberPostes = append(db.CyberPostes[:idx], db.CyberPostes[idx+1:]...)
	if wasPaused {
		if rr := findRouterScoped(db, p.RouterID, acc); rr != nil {
			a.queueDevicePauseLocked(db, rr)
		}
	}
	a.logActivityBy(r, db, acc, "cyber", "Poste supprimé : "+model.CyberPosteLabel(p))
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Poste supprimé"})
}

// ---------------------------------------------------------------------------
// Attribution / libération du code-temps (un code = un poste)
// ---------------------------------------------------------------------------

// handleCyberPosteAssign — POST /api/cyber/postes/{id}/assign
// {profileId, timeLimitMin?} : crée UN voucher limit-uptime sur le gabarit de
// génération (mode « utilisateur = mot de passe » verrouillé N°25, validité
// ancrée au 1er login, SellingPrice F13) et le LIE au poste. La caisse est
// enregistrée dans la même écriture (Transaction + Sale, canal « direct »)
// — les rapports/compta/journaux existants comptent la vente sans double
// compteur. Le poste doit être libre (libération explicite d'abord : la
// traçabilité « quel code tourne sur quelle machine » reste lisible).
func (a *API) handleCyberPosteAssign(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		ProfileID    string `json:"profileId"`
		TimeLimitMin int    `json:"timeLimitMin"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()
	if !a.requireCyber(w, db, acc) {
		a.store.Unlock()
		return
	}
	poste := model.FindCyberPosteScoped(db, id, acc)
	if poste == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Poste introuvable")
		return
	}
	if poste.ActiveUserID != "" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Poste occupé — libérez-le avant d'attribuer un nouveau code")
		return
	}
	profile := findProfileScoped(db, strings.TrimSpace(req.ProfileID), acc)
	if profile == nil {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Profil introuvable")
		return
	}
	router := findRouterScoped(db, poste.RouterID, acc)
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur du poste introuvable")
		return
	}
	routerCopy := *router
	profileCopy := *profile
	posteCopy := *poste
	// Quota temps (même règle que la génération : 0 = hériter du
	// sessionTimeoutMin du profil, borne parité Mikhmon).
	if req.TimeLimitMin < 0 || req.TimeLimitMin > 2628000 {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Le quota de temps doit être compris entre 0 et 2628000 minutes")
		return
	}
	timeLimitMin := int64(req.TimeLimitMin)
	if timeLimitMin <= 0 {
		timeLimitMin = int64(profileCopy.SessionTimeoutMin)
	}
	// Code unique (gabarit génération, count=1, mode « same » N°25).
	code := model.RandomCodeFrom(5, "")
	for j := 0; j < 50 && usernameTaken(db, acc, code); j++ {
		code = model.RandomCodeFrom(5, "")
	}
	batchID := fmt.Sprintf("B%s-%04d", now.Format("20060102"), now.Nanosecond()%10000)
	cost := profileCopy.Price
	selling := profileCopy.Price
	if profileCopy.SellingPrice > 0 {
		selling = profileCopy.SellingPrice
	}
	u := model.HotspotUser{
		ID: model.NewID("v-"), AccountID: acc, Kind: "voucher",
		Username: code, Password: code,
		ProfileID: profileCopy.ID, ProfileName: profileCopy.Name,
		RouterID: routerCopy.ID, RouterName: routerCopy.Name,
		Status: "active", BatchID: batchID,
		CreatedAt: model.NowISO(), ExpiresAt: "", UsedAt: "",
		Price: profileCopy.Price, SellingPrice: profileCopy.SellingPrice,
		DataQuotaMb: int64(profileCopy.DataQuotaMb), TimeLimitMin: timeLimitMin,
		Comment: sanitizeVoucherComment("cyber " + model.CyberPosteLabel(posteCopy)),
		// Trace d'origine (pattern anti-vol N°8) : la vente naît du module
		// Cybercafé — visible dans le détail de connexion.
		SoldAt: model.NowISO(), SoldVia: "cyber_poste",
	}

	if routerCopy.Mode == "agent" {
		u.Username = agent.SanitizeName(u.Username)
		u.ProfileName = agent.SanitizeName(u.ProfileName)
		db.HotspotUsers = append(db.HotspotUsers, u)
		liftPurgeTombstone(db, acc, u.Username)
		payload := map[string]any{
			// N°201 — résolu pour LE routeur du poste (parent-queue managé
			// omis si la file n'existe pas sur cette box).
			"profile": profileRefFor(profileCopy, &routerCopy),
			"users":   []map[string]any{{"name": u.Username, "password": u.Password}},
			"batch":   batchID,
		}
		if u.DataQuotaMb > 0 {
			payload["limitBytesTotal"] = u.DataQuotaMb * 1048576
			if profileCopy.QuotaModeEffective() == model.QuotaModeThrottle && profileCopy.ThrottleRate != "" {
				payload["throttleRate"] = profileCopy.ThrottleRate
			}
		}
		if u.Comment != "" {
			payload["comment"] = u.Comment
		}
		if timeLimitMin > 0 {
			payload["limitUptimeMin"] = timeLimitMin
		}
		queueCommandLocked(db, acc, routerCopy.ID, model.CmdVoucherBatch, payload)
		a.cyberBookkeepingLocked(r, db, acc, poste, profileCopy, routerCopy, u, batchID, cost, selling)
		resp := cyberPosteResponseLocked(db, *poste)
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"poste": resp, "voucher": u, "totalCost": cost,
			"message": "Code attribué au poste — créé sur le routeur au prochain check-in (≤ 45 s)",
		})
		return
	}

	// Gateway (simulée/réelle) — même contrat que la génération : AddUser
	// HORS verrou (appel réseau réel), bookkeeping sous verrou ensuite.
	gw := a.gatewayFor(routerCopy)
	a.store.Unlock()
	if err := gw.AddUser(&u); err != nil {
		writeErr(w, http.StatusBadRequest, "Création impossible : "+err.Error())
		return
	}
	a.store.Lock()
	db = a.store.Data()
	poste = model.FindCyberPosteScoped(db, id, acc)
	if poste == nil || poste.ActiveUserID != "" {
		a.store.Unlock()
		writeErr(w, http.StatusConflict, "État du poste modifié entre-temps — réessayez")
		return
	}
	a.cyberBookkeepingLocked(r, db, acc, poste, profileCopy, routerCopy, u, batchID, cost, selling)
	resp := cyberPosteResponseLocked(db, *poste)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"poste": resp, "voucher": u, "totalCost": cost,
		"message": "Code attribué au poste",
	})
}

// cyberBookkeepingLocked — caisse de l'attribution (gabarit génération
// N°290/F13, canal « direct ») + liaison du poste. SOUS VERROU ; Save à
// charge de l'appelant.
func (a *API) cyberBookkeepingLocked(r *http.Request, db *model.DB, acc string, poste *model.CyberPoste, profile model.Profile, router model.Router, u model.HotspotUser, batchID string, cost, selling int) {
	db.Transactions = append([]model.Transaction{{
		ID: model.NewID("tx-"), AccountID: acc, Type: "sale",
		Amount: cost, Note: "Code-temps poste " + model.CyberPosteLabel(*poste) + " (" + profile.Name + ")",
		At: model.NowISO(),
	}}, db.Transactions...)
	db.Sales = append(db.Sales, model.Sale{
		ID: model.NewID("sale-"), AccountID: acc, Amount: cost, ProfileName: profile.Name, Count: 1,
		Channel:  "direct",
		RouterID: router.ID, RouterName: router.Name, BatchID: batchID,
		At: model.NowISO(), Cost: cost, SellingTotal: selling,
	})
	db.Batches = append([]model.Batch{{
		ID: batchID, AccountID: acc, ProfileID: profile.ID, ProfileName: profile.Name,
		RouterID: router.ID, RouterName: router.Name,
		Count: 1, UnitPrice: profile.Price, TotalCost: cost,
		DataQuotaMb: int64(profile.DataQuotaMb), TimeLimitMin: u.TimeLimitMin,
		Channel: "direct", CreatedAt: model.NowISO(),
	}}, db.Batches...)
	// Liaison du poste : un code = un poste (libération explicite pour délier).
	poste.ActiveUserID = u.ID
	poste.ActiveUsername = u.Username
	poste.ActiveProfileName = profile.Name
	poste.UpdatedAt = model.NowISO()
	a.logActivityBy(r, db, acc, "cyber", "Code-temps attribué au poste "+model.CyberPosteLabel(*poste)+" ("+profile.Name+")")
}

// handleCyberPosteRelease — POST /api/cyber/postes/{id}/release : délie le
// poste de son code-temps (le voucher, lui, reste gérable dans Utilisateurs/
// Vouchers : prolongation F4, expiration, purge — sa vie continue hors du
// poste). Le poste redevient attribuable.
func (a *API) handleCyberPosteRelease(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	if !a.requireCyber(w, db, acc) {
		a.store.Unlock()
		return
	}
	p := model.FindCyberPosteScoped(db, id, acc)
	if p == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Poste introuvable")
		return
	}
	previous := p.ActiveUsername
	p.ActiveUserID = ""
	p.ActiveUsername = ""
	p.ActiveProfileName = ""
	p.UpdatedAt = model.NowISO()
	if previous != "" {
		a.logActivityBy(r, db, acc, "cyber", "Poste "+model.CyberPosteLabel(*p)+" libéré (code "+previous+")")
	}
	resp := cyberPosteResponseLocked(db, *p)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Pause / reprise d'un poste (device_pause N°101, ensemble cyber fusionné)
// ---------------------------------------------------------------------------

// handleCyberPostePause — POST /api/cyber/postes/{id}/pause {paused, minutes}
// : même contrat que la pause dîner HomeNet (0..1440 min, 0 = illimité) ;
// la convergence réutilise la commande device_pause et la même signature —
// l'ensemble désiré (desiredPauseMacsLocked) fusionne postes et appareils.
func (a *API) handleCyberPostePause(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Paused  bool `json:"paused"`
		Minutes int  `json:"minutes"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Minutes < 0 || req.Minutes > devicePauseMaxMinutes {
		writeErr(w, http.StatusBadRequest, "Durée de pause invalide (0 à 1440 minutes)")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	if !a.requireCyber(w, db, acc) {
		a.store.Unlock()
		return
	}
	p := model.FindCyberPosteScoped(db, id, acc)
	if p == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Poste introuvable")
		return
	}
	rr := findRouterScoped(db, p.RouterID, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur du poste introuvable")
		return
	}
	now := time.Now().UTC()
	if req.Paused {
		p.Paused = true
		if req.Minutes > 0 {
			p.PausedUntil = now.Add(time.Duration(req.Minutes) * time.Minute).Format(time.RFC3339)
		} else {
			p.PausedUntil = "" // illimité
		}
	} else {
		p.Paused = false
		p.PausedUntil = ""
	}
	p.UpdatedAt = model.NowISO()
	// Convergence immédiate : l'ensemble désiré COURANT est (re)posé dans la
	// file — servi au prochain check-in de la box (≤ 45 s console ouverte).
	a.queueDevicePauseLocked(db, rr)
	if req.Paused {
		if req.Minutes > 0 {
			a.logActivityBy(r, db, acc, "cyber", "Poste "+model.CyberPosteLabel(*p)+" en pause ("+fmt.Sprint(req.Minutes)+" min, en attente de la box)")
		} else {
			a.logActivityBy(r, db, acc, "cyber", "Poste "+model.CyberPosteLabel(*p)+" en pause (jusqu'à réactivation, en attente de la box)")
		}
	} else {
		a.logActivityBy(r, db, acc, "cyber", "Poste "+model.CyberPosteLabel(*p)+" rétabli (en attente de la box)")
	}
	resp := cyberPosteResponseLocked(db, *p)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Activation du module (décision D6 de N°289 : un flag, pas un nouvel usage)
// ---------------------------------------------------------------------------

// handleCyberSettings — PUT /api/cyber/settings {enabled} (rang 3) : active
// ou désactive le module pour le compte. À la DÉSACTIVATION, les pauses de
// postes sont TOUTES levées et la convergence re-filée : aucun filtre
// mikcloud-pause orphelin ne survit à un module éteint (contrat d'état
// propre — le module réactivé repart d'un registre intact, mais libre).
func (a *API) handleCyberSettings(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	settings := ensureSettings(db, acc)
	settings.Tenant.CyberEnabled = &req.Enabled
	db.SettingsByAccount[acc] = settings
	if !req.Enabled {
		affected := map[string]*model.Router{}
		for i := range db.CyberPostes {
			p := &db.CyberPostes[i]
			if p.AccountID != acc || !p.Paused {
				continue
			}
			p.Paused = false
			p.PausedUntil = ""
			if rr := findRouterScoped(db, p.RouterID, acc); rr != nil && rr.Mode == "agent" {
				affected[rr.ID] = rr
			}
		}
		for _, rr := range affected {
			a.queueDevicePauseLocked(db, rr)
		}
	}
	if req.Enabled {
		a.logActivityBy(r, db, acc, "cyber", "Module Cybercafé activé")
	} else {
		a.logActivityBy(r, db, acc, "cyber", "Module Cybercafé désactivé (pauses de postes levées)")
	}
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, settings)
}
