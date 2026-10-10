// handlers_ppp.go — N°293 — PPPoE Phase A (chantier ⑥ de N°289, Option V
// lot 3) : console des abonnés PPPoE d'un WISP.
//
// Décisions appliquées (docs/ANALYSE-P1-PPP-WG-CYBER.md §2, arbitrées) :
//   - D1 — le transport est le canal AGENT (commandes ppp_* — zéro cred API
//     stockée, zéro port public, CGNAT-proof). Les gestes d'écriture
//     exigent un routeur MODE AGENT : sur un routeur simulé ou réel, aucune
//     commande ne peut converger (refus clair plutôt que commande fantôme).
//   - D2 — v1 gère les secrets d'un pppoe-server EXISTANT : Profile est le
//     nom d'un profil PPP déjà présent côté routeur (le cloud ne provisionne
//     ni interface, ni profils — phase B).
//   - D6 — PPPoE est un segment ISP TRANSVERSE (hotspot/homenet) : AUCUN
//     garde requireUsage (contrairement aux routes cyber/vpn) — pas de
//     nouvel usage, pas de flag d'activation.
//
// Machine à états (gabarit handlers_wg.go / discipline N°291) :
//
//	pending → active   à la confirmation agent (ou parité read_state) ;
//	pending → error    à l'échec rapporté (ErrorMsg, jamais de secret) ;
//	pending (suppression) → retrait de la ligne UNIQUEMENT à la
//	                   confirmation agent — jamais de retrait avant.
//
// Le kick est de la télémétrie (aucun état cloud).
package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// pppPasswordMax / pppCommentMax — bornes de saisie console (le mot de passe
// PPP est interfacé tel quel, protégé par rosEscape côté builder ; le
// commentaire routeur est préfixé du marqueur mikcloud-ppp).
const (
	pppPasswordMax = 64
	pppCommentMax  = 120
)

// ---------------------------------------------------------------------------
// Résolution + helpers (sous verrou)
// ---------------------------------------------------------------------------

// pppSecretOfCommandLocked — résout le secret visé par une commande ppp_*
// (payload secretId), scoping routeur + compte (isolation multi-tenant à
// chaque résolution — même discipline que findRouterScoped). nil si absent
// ou hors périmètre.
func pppSecretOfCommandLocked(db *model.DB, router *model.Router, cmd *model.Command) *model.PppSecret {
	sid, _ := cmd.Payload["secretId"].(string)
	if sid == "" {
		return nil
	}
	s := model.FindPppSecretScoped(db, sid, router.AccountID)
	if s == nil || s.RouterID != router.ID {
		return nil
	}
	return s
}

// applyPppSetDeltas — réapplique au modèle les deltas d'une commande
// ppp_secret_set : SEULEMENT les clés présentes dans le payload (le set
// routeur est partiel par construction — la ligne est dynamique).
func applyPppSetDeltas(s *model.PppSecret, payload map[string]any) {
	if v, ok := payload["password"].(string); ok {
		s.Password = v
	}
	if v, ok := payload["profile"].(string); ok {
		s.Profile = v
	}
	if v, ok := payload["comment"].(string); ok {
		s.Comment = v
	}
	if v, ok := payload["disabled"].(bool); ok {
		s.Disabled = v
	}
}

// pendingPppCommandLocked — une commande du kind visant ce secret est-elle
// déjà en file/vol ? (dédup des clics répétés — pattern pendingToolCommand
// affiné par secretId : plusieurs secrets du même routeur portent
// simultanément des commandes du même kind.)
func pendingPppCommandLocked(db *model.DB, routerID, kind, secretID string) bool {
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID != routerID || c.Kind != kind || (c.Status != "queued" && c.Status != "sent") {
			continue
		}
		if sid, _ := c.Payload["secretId"].(string); sid == secretID {
			return true
		}
	}
	return false
}

// pppAgentOnly — PPPoE v1 est piloté PAR L'AGENT (D1) : sur un routeur
// simulé (aucun pppoe-server réel derrière) ou réel (creds API hors
// doctrine), un geste d'écriture ne peut pas converger. Renvoie le message
// d'erreur si le mode ne convient pas, "" sinon.
func pppAgentOnly(rr *model.Router) string {
	if rr.Mode == "agent" {
		return ""
	}
	return "PPPoE est piloté par l'agent MikCloud (mode agent requis) — basculez ce routeur en mode agent pour gérer les abonnés"
}

// pppSecretCountLocked — secrets du ROUTER (plafond MaxPppSecretsPerRouter).
func pppSecretCountLocked(db *model.DB, routerID string) int {
	n := 0
	for i := range db.PppSecrets {
		if db.PppSecrets[i].RouterID == routerID {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Registre : liste, création, modification, suppression, kick
// ---------------------------------------------------------------------------

// handlePppSecretsList — GET /api/routers/{routerID}/ppp/secrets : les
// abonnés PPPoE du compte+routeur (JAMAIS les lignes des autres routeurs),
// triés par création décroissante, avec l'état de parité
// (LastSeenOnRouter — « jamais vu » = vide).
func (a *API) handlePppSecretsList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	routerID := r.PathValue("routerID")
	a.store.Lock()
	db := a.store.Data()
	if findRouterScoped(db, routerID, acc) == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	out := []model.PppSecret{}
	for i := range db.PppSecrets {
		if db.PppSecrets[i].AccountID == acc && db.PppSecrets[i].RouterID == routerID {
			out = append(out, db.PppSecrets[i])
		}
	}
	a.store.Unlock()
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	writeJSON(w, http.StatusOK, out)
}

// handlePppSecretCreate — POST /api/routers/{routerID}/ppp/secrets
// {name,password,profile,comment} : création cloud (State=pending) +
// commande ppp_secret_add en file (201). Validations model (regex large
// sans caractères dangereux), unicité PAR ROUTEUR, plafond 400/routeur.
func (a *API) handlePppSecretCreate(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	routerID := r.PathValue("routerID")
	var req struct {
		Name     string `json:"name"`
		Password string `json:"password"`
		Profile  string `json:"profile"`
		Comment  string `json:"comment"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := model.NormalizePppName(req.Name)
	if !model.ValidPppName(name) {
		writeErr(w, http.StatusBadRequest, "Nom d'abonné invalide (1-64 : lettres minuscules, chiffres, . _ - @ — pas d'espace ni de guillemet)")
		return
	}
	profile := strings.TrimSpace(req.Profile)
	if !model.ValidPppProfileName(profile) {
		writeErr(w, http.StatusBadRequest, "Nom de profil PPP invalide (1-64 : lettres, chiffres, . _ - @)")
		return
	}
	password := req.Password
	if len(password) > pppPasswordMax {
		writeErr(w, http.StatusBadRequest, "Mot de passe trop long ("+strconv.Itoa(pppPasswordMax)+" caractères max)")
		return
	}
	comment := strings.TrimSpace(req.Comment)
	if len(comment) > pppCommentMax {
		writeErr(w, http.StatusBadRequest, "Commentaire trop long ("+strconv.Itoa(pppCommentMax)+" caractères max)")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	router := findRouterScoped(db, routerID, acc)
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if msg := pppAgentOnly(router); msg != "" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if model.PppSecretNameTaken(db, router.ID, name) {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Un abonné de ce nom existe déjà sur ce routeur")
		return
	}
	if pppSecretCountLocked(db, router.ID) >= model.MaxPppSecretsPerRouter {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Nombre maximum d'abonnés PPPoE atteint pour ce routeur ("+strconv.Itoa(model.MaxPppSecretsPerRouter)+")")
		return
	}
	now := model.NowISO()
	s := model.PppSecret{
		ID: model.NewID("pp-"), AccountID: acc, RouterID: router.ID,
		Name: name, Password: password, Profile: profile, Comment: comment,
		Service: model.PppService, State: model.PppStatePending,
		CreatedAt: now, UpdatedAt: now,
	}
	db.PppSecrets = append(db.PppSecrets, s)
	payload := agent.PppSecretAddPayloadFrom(s.Name, s.Password, s.Profile, s.Comment, s.Disabled, false)
	payload["secretId"] = s.ID
	cmd := queueCommandLocked(db, acc, router.ID, model.CmdPppSecretAdd, payload)
	a.logActivityBy(r, db, acc, "ppp", "Abonné PPPoE "+name+" créé (en attente du routeur, commande "+cmd.ID+")")
	a.store.Save()
	cmdID := cmd.ID
	a.store.Unlock()

	writeJSON(w, http.StatusCreated, map[string]any{
		"secret": s, "commandId": cmdID,
		"message": "Abonné enregistré — confirmation au prochain check-in du routeur (≤ 45 s)",
	})
}

// handlePppSecretUpdate — PATCH /api/ppp/secrets/{secretID}
// {password?,profile?,disabled?,comment?} (pointeurs : présents seulement) :
// mise à jour cloud IMMÉDIATE + commande ppp_secret_set (set partiel —
// seules les propriétés présentes sont envoyées au routeur).
func (a *API) handlePppSecretUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("secretID")
	var req struct {
		Password *string `json:"password"`
		Profile  *string `json:"profile"`
		Disabled *bool   `json:"disabled"`
		Comment  *string `json:"comment"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Password != nil && len(*req.Password) > pppPasswordMax {
		writeErr(w, http.StatusBadRequest, "Mot de passe trop long ("+strconv.Itoa(pppPasswordMax)+" caractères max)")
		return
	}
	if req.Profile != nil {
		*req.Profile = strings.TrimSpace(*req.Profile)
		if !model.ValidPppProfileName(*req.Profile) {
			writeErr(w, http.StatusBadRequest, "Nom de profil PPP invalide (1-64 : lettres, chiffres, . _ - @)")
			return
		}
	}
	if req.Comment != nil {
		*req.Comment = strings.TrimSpace(*req.Comment)
		if len(*req.Comment) > pppCommentMax {
			writeErr(w, http.StatusBadRequest, "Commentaire trop long ("+strconv.Itoa(pppCommentMax)+" caractères max)")
			return
		}
	}

	a.store.Lock()
	db := a.store.Data()
	s := model.FindPppSecretScoped(db, id, acc)
	if s == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Abonné introuvable")
		return
	}
	router := findRouterScoped(db, s.RouterID, acc)
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if msg := pppAgentOnly(router); msg != "" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	// Mise à jour cloud immédiate (l'état désiré), la commande porte les
	// MÊMES deltas — la confirmation agent repasse pending→active.
	if req.Password != nil {
		s.Password = *req.Password
	}
	if req.Profile != nil {
		s.Profile = *req.Profile
	}
	if req.Disabled != nil {
		s.Disabled = *req.Disabled
	}
	if req.Comment != nil {
		s.Comment = *req.Comment
	}
	payload := agent.PppSecretSetPayloadFrom(req.Password, req.Profile, req.Comment, req.Disabled)
	payload["secretId"] = s.ID
	payload["name"] = s.Name
	cmd := queueCommandLocked(db, acc, router.ID, model.CmdPppSecretSet, payload)
	s.State = model.PppStatePending
	s.ErrorMsg = ""
	s.UpdatedAt = model.NowISO()
	a.logActivityBy(r, db, acc, "ppp", "Abonné PPPoE "+s.Name+" modifié (en attente du routeur, commande "+cmd.ID+")")
	a.store.Save()
	cmdID := cmd.ID
	name := s.Name
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "name": name, "state": "pending", "commandId": cmdID,
		"message": "Modification enregistrée — confirmation au prochain check-in du routeur (≤ 45 s)",
	})
}

// handlePppSecretDelete — DELETE /api/ppp/secrets/{secretID} : State=pending
// suppression + commande ppp_secret_remove en file. La confirmation agent
// retire la LIGNE du registre (agent_handlers.go) ; un échec repasse
// State=error + ErrorMsg (jamais de retrait registre avant confirmation,
// discipline N°291 — contrairement aux vouchers hotspot dont la suppression
// est immédiate : un abonné PAYANT réapparaissant après un échec serait une
// trahison de registre).
func (a *API) handlePppSecretDelete(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("secretID")
	a.store.Lock()
	db := a.store.Data()
	s := model.FindPppSecretScoped(db, id, acc)
	if s == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Abonné introuvable")
		return
	}
	router := findRouterScoped(db, s.RouterID, acc)
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if msg := pppAgentOnly(router); msg != "" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	s.State = model.PppStatePending
	s.ErrorMsg = ""
	s.UpdatedAt = model.NowISO()
	var cmdID string
	if pendingPppCommandLocked(db, router.ID, model.CmdPppSecretRemove, s.ID) {
		cmdID = "déjà en file"
	} else {
		cmd := queueCommandLocked(db, acc, router.ID, model.CmdPppSecretRemove,
			map[string]any{"secretId": s.ID, "name": s.Name})
		cmdID = cmd.ID
	}
	a.logActivityBy(r, db, acc, "ppp", "Suppression de l'abonné PPPoE "+s.Name+" demandée (retrait du registre à la confirmation du routeur)")
	a.store.Save()
	name := s.Name
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "name": name, "state": "pending", "commandId": cmdID,
		"message": "Suppression demandée — le registre sera mis à jour à la confirmation du routeur (≤ 45 s)",
	})
}

// handlePppSecretKick — POST /api/ppp/secrets/{secretID}/kick : déconnexion
// de la session PPPoE active de l'abonné (ppp_kick → 202). Le SECRET reste
// (l'abonné se reconnecte) — télémétrie, aucun état cloud (pattern ping F8 :
// honnête, la confirmation est sans effet observable côté registre).
func (a *API) handlePppSecretKick(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("secretID")
	a.store.Lock()
	db := a.store.Data()
	s := model.FindPppSecretScoped(db, id, acc)
	if s == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Abonné introuvable")
		return
	}
	router := findRouterScoped(db, s.RouterID, acc)
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if msg := pppAgentOnly(router); msg != "" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	cmd := queueCommandLocked(db, acc, router.ID, model.CmdPppKick,
		map[string]any{"secretId": s.ID, "name": s.Name})
	a.logActivityBy(r, db, acc, "ppp", "Déconnexion de l'abonné PPPoE "+s.Name+" demandée")
	a.store.Save()
	cmdID := cmd.ID
	name := s.Name
	a.store.Unlock()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"queued": true, "commandId": cmdID, "name": name,
		"message": "Déconnexion en attente du prochain check-in du routeur (≤ 45 s) — l'abonné peut se reconnecter",
	})
}

// ---------------------------------------------------------------------------
// Lectures d'outils (cache agent ≤ 120 s — pattern read_dhcp / ping F8)
// ---------------------------------------------------------------------------

// pppDiscoverRow — une ligne de /ppp/secret/print rapportée par
// ppp_read_secrets (découverte des secrets EXISTANTS du pppoe-server —
// créés hors MikCloud : MikTik/Winbox/Mikhmon — D2 « gérer l'existant »).
type pppDiscoverRow struct {
	Name     string `json:"name"`
	Profile  string `json:"profile"`
	Disabled bool   `json:"disabled"`
	Service  string `json:"service"`
	Comment  string `json:"comment"`
}

// pppActiveRow — une ligne de /ppp/active/print (sessions PPPoE actives).
type pppActiveRow struct {
	Name     string `json:"name"`
	Service  string `json:"service"`
	CallerID string `json:"callerId"`
	Address  string `json:"address"`
	Uptime   string `json:"uptime"` // forme RouterOS (ex. « 3d21h14m56s »)
}

func parsePppDiscoverRows(rows [][]string) []pppDiscoverRow {
	out := make([]pppDiscoverRow, 0, len(rows))
	for _, e := range rows {
		if len(e) == 0 || e[0] == "" {
			continue
		}
		out = append(out, pppDiscoverRow{
			Name: e[0],
			// Tolérance aux champs absents/résiduels (contrat F9 — les
			// commentaires libres peuvent porter des « | » résiduels : le
			// champ tronqué reste affichable, jamais interprété).
			Profile: field(e, 1), Disabled: field(e, 2) == "true",
			Service: field(e, 3), Comment: field(e, 4),
		})
	}
	return out
}

func parsePppActiveRows(rows [][]string) []pppActiveRow {
	out := make([]pppActiveRow, 0, len(rows))
	for _, e := range rows {
		if len(e) == 0 || e[0] == "" {
			continue
		}
		out = append(out, pppActiveRow{
			Name: e[0], Service: field(e, 1), CallerID: field(e, 2),
			Address: field(e, 3), Uptime: field(e, 4),
		})
	}
	return out
}

// handlePppDiscover — GET /api/routers/{routerID}/ppp/discover : cache du
// rapport ppp_read_secrets (TTL 120 s, pattern outils F9) — les secrets du
// routeur vus par le routeur lui-même, y compris ceux créés hors MikCloud.
// Frais → 200 {queued:false, data, updatedAt} ; sinon → 202 {queued:true}
// (une commande part, dédupliquée — le front re-poll).
func (a *API) handlePppDiscover(w http.ResponseWriter, r *http.Request) {
	a.servePppTool(w, r, model.CmdPppReadSecrets, parsePppDiscoverRows)
}

// handlePppActiveList — GET /api/routers/{routerID}/ppp/active : cache du
// rapport ppp_read_active (TTL 120 s) — sessions PPPoE actives du routeur.
func (a *API) handlePppActiveList(w http.ResponseWriter, r *http.Request) {
	a.servePppTool(w, r, model.CmdPppReadActive, parsePppActiveRows)
}

// servePppTool — mécanique commune des lectures PPP (miroir serveRouterTool
// sans la génération simulée : un routeur simulé n'a PAS de pppoe-server,
// la liste vide est la réponse honnête) :
//
//	agent     → commande done depuis < 120 s → Result["data"] resplit
//	            → 200 {queued:false, data, updatedAt} ;
//	            sinon commande en file (dédupliquée) ou enfilement
//	            → 202 {queued:true, data:[], updatedAt:""} ;
//	simulated → 200 {queued:false, data:[], updatedAt:now} ;
//	real      → 400 realModeUnsupported.
func (a *API) servePppTool[T any](w http.ResponseWriter, r *http.Request, kind string, parseRows func([][]string) []T) {
	acc := accountScope(r)
	id := r.PathValue("routerID")
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
	if rr.Mode == "simulated" {
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"queued": false, "data": []T{}, "updatedAt": model.NowISO()})
		return
	}
	now := time.Now().UTC()
	if fresh := freshToolCommand(db, id, kind, now); fresh != nil {
		raw, _ := fresh.Result["data"].(string)
		rows := parseRows(splitAgentList(raw))
		doneAt := fresh.DoneAt
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"queued": false, "data": rows, "updatedAt": doneAt})
		return
	}
	if !pendingToolCommand(db, id, kind) {
		queueCommandLocked(db, acc, id, kind, map[string]any{})
		a.store.Save()
	}
	a.store.Unlock()
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": true, "data": []T{}, "updatedAt": ""})
}
