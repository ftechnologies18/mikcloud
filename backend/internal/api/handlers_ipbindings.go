// Package api — IP bindings (F7, audit Mikhmon) : CRUD cloud + commandes
// agent ipbinding_add/set/remove (CONTRACT-V2 §0 — simulated ✅, agent ✅,
// real → 400 realModeUnsupported).
package api

import (
	"encoding/json"
	"mikcloud/hotspot-api/internal/model"
	"net"
	"net/http"
	"regexp"
	"strings"
)

// macPattern — validation d'une adresse MAC (contrat F7).
var macPattern = regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)

// ---------------------------------------------------------------------------
// F7 — IP bindings
// ---------------------------------------------------------------------------

// handleIPBindingsList — GET /api/routers/{id}/ipbindings → IPBinding[].
func (a *API) handleIPBindingsList(w http.ResponseWriter, r *http.Request) {
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
	bindings := []model.IPBinding{}
	for _, b := range db.IPBindings {
		if b.AccountID == acc && b.RouterID == id {
			bindings = append(bindings, b)
		}
	}
	a.store.Unlock()
	writeJSON(w, http.StatusOK, bindings)
}

// handleIPBindingCreate — POST /api/routers/{id}/ipbindings
// {mac, address?, comment?, type?} : simulated → CRUD cloud (201) ; agent →
// cloud + commande ipbinding_add (201 + queued:true) ; real → 400.
func (a *API) handleIPBindingCreate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		MAC     string `json:"mac"`
		Address string `json:"address"`
		Comment string `json:"comment"`
		Type    string `json:"type"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	mac := strings.TrimSpace(req.MAC)
	if !macPattern.MatchString(mac) {
		writeErr(w, http.StatusBadRequest, "Adresse MAC invalide (format AA:BB:CC:DD:EE:FF)")
		return
	}
	typ := strings.TrimSpace(req.Type)
	if typ == "" {
		typ = "bypassed"
	}
	if typ != "bypassed" && typ != "blocked" {
		writeErr(w, http.StatusBadRequest, "Type invalide (bypassed ou blocked)")
		return
	}
	address := strings.TrimSpace(req.Address)
	if address != "" && net.ParseIP(address) == nil {
		writeErr(w, http.StatusBadRequest, "Adresse IP invalide")
		return
	}
	comment := strings.TrimSpace(req.Comment)
	if len(comment) > 200 {
		comment = comment[:200]
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
	for _, b := range db.IPBindings {
		if b.AccountID == acc && b.RouterID == id && strings.EqualFold(b.MAC, mac) {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Cette adresse MAC est déjà liée sur ce routeur")
			return
		}
	}
	binding := model.IPBinding{
		ID: model.NewID("ipb-"), AccountID: acc, RouterID: id,
		MAC: strings.ToUpper(mac), Address: address, Comment: comment,
		Type: typ, Disabled: false, CreatedAt: model.NowISO(),
	}
	db.IPBindings = append(db.IPBindings, binding)
	var cmdID string
	if rr.Mode == "agent" {
		cmd := queueCommandLocked(db, acc, id, model.CmdIpbindingAdd, map[string]any{
			"mac": binding.MAC, "address": address, "comment": comment, "type": typ,
		})
		cmdID = cmd.ID
	}
	a.logActivityBy(r, db, acc, "router", "IP binding "+binding.MAC+" ("+typ+") ajouté sur «"+rr.Name+"»")
	a.store.Save()
	a.store.Unlock()

	data, _ := json.Marshal(binding)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	if cmdID != "" {
		out["queued"] = true
		out["commandId"] = cmdID
		out["message"] = "Commande envoyée — création au prochain check-in du routeur (≤ 45 s)"
	}
	writeJSON(w, http.StatusCreated, out)
}

// findIPBindingScoped — binding scopé au compte (404 s'il appartient à un autre).
func findIPBindingScoped(db *model.DB, id, acc string) *model.IPBinding {
	for i := range db.IPBindings {
		if db.IPBindings[i].ID == id && db.IPBindings[i].AccountID == acc {
			return &db.IPBindings[i]
		}
	}
	return nil
}

// handleIPBindingUpdate — PUT /api/ipbindings/{id} {disabled?, comment?, address?} :
// simulated → cloud ; agent → cloud + commande ipbinding_set ; real → 400.
func (a *API) handleIPBindingUpdate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Disabled *bool   `json:"disabled"`
		Comment  *string `json:"comment"`
		Address  *string `json:"address"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Address != nil && strings.TrimSpace(*req.Address) != "" && net.ParseIP(strings.TrimSpace(*req.Address)) == nil {
		writeErr(w, http.StatusBadRequest, "Adresse IP invalide")
		return
	}
	if req.Comment != nil && len(*req.Comment) > 200 {
		writeErr(w, http.StatusBadRequest, "Le commentaire doit faire au plus 200 caractères")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	b := findIPBindingScoped(db, id, acc)
	if b == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "IP binding introuvable")
		return
	}
	rr := findRouterScoped(db, b.RouterID, acc)
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
	payload := map[string]any{"mac": b.MAC}
	if req.Disabled != nil {
		b.Disabled = *req.Disabled
		payload["disabled"] = *req.Disabled
	}
	if req.Comment != nil {
		b.Comment = strings.TrimSpace(*req.Comment)
	}
	if req.Address != nil {
		b.Address = strings.TrimSpace(*req.Address)
		if b.Address != "" {
			payload["address"] = b.Address
		}
	}
	if rr.Mode == "agent" {
		queueCommandLocked(db, acc, rr.ID, model.CmdIpbindingSet, payload)
	}
	updated := *b
	a.logActivityBy(r, db, acc, "router", "IP binding "+updated.MAC+" modifié sur «"+rr.Name+"»")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, updated)
}

// handleIPBindingDelete — DELETE /api/ipbindings/{id} : simulated → cloud ;
// agent → cloud + commande ipbinding_remove ; real → 400.
func (a *API) handleIPBindingDelete(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")

	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.IPBindings {
		if db.IPBindings[i].ID == id && db.IPBindings[i].AccountID == acc {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "IP binding introuvable")
		return
	}
	b := db.IPBindings[idx]
	rr := findRouterScoped(db, b.RouterID, acc)
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
	db.IPBindings = append(db.IPBindings[:idx], db.IPBindings[idx+1:]...)
	if rr.Mode == "agent" {
		queueCommandLocked(db, acc, rr.ID, model.CmdIpbindingRemove, map[string]any{"mac": b.MAC})
	}
	a.logActivityBy(r, db, acc, "router", "IP binding "+b.MAC+" supprimé de «"+rr.Name+"»")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
