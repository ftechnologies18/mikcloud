// Package api — modèles (templates) de vouchers (F2, audit Mikhmon) : CRUD
// scopé au compte, formats a4/58mm/80mm, sanitize du HTML et unicité du
// modèle par défaut.
package api

import (
	"mikcloud/hotspot-api/internal/model"
	"net/http"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// F2 — Modèles (templates) de vouchers
// ---------------------------------------------------------------------------

// templateFormats — formats d'impression supportés (contrat F2).
var templateFormats = map[string]bool{"a4": true, "58mm": true, "80mm": true}

func (a *API) handleTemplatesList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	templates := []model.VoucherTemplate{}
	for _, t := range a.store.Data().Templates {
		if t.AccountID == acc {
			templates = append(templates, t)
		}
	}
	a.store.Unlock()

	// Tri : modèle par défaut d'abord, puis plus récent.
	sort.Slice(templates, func(i, j int) bool {
		if templates[i].IsDefault != templates[j].IsDefault {
			return templates[i].IsDefault
		}
		return templates[i].CreatedAt > templates[j].CreatedAt
	})
	writeJSON(w, http.StatusOK, templates)
}

func (a *API) handleTemplateCreate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	var req struct {
		Name      string `json:"name"`
		Format    string `json:"format"`
		BodyHTML  string `json:"bodyHtml"`
		IsDefault bool   `json:"isDefault"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	if len(name) < 1 || len(name) > 60 {
		writeErr(w, http.StatusBadRequest, "Le nom du modèle doit faire entre 1 et 60 caractères")
		return
	}
	format := strings.TrimSpace(req.Format)
	if !templateFormats[format] {
		writeErr(w, http.StatusBadRequest, "Format invalide (a4, 58mm ou 80mm)")
		return
	}
	if len(req.BodyHTML) > 20000 {
		writeErr(w, http.StatusBadRequest, "Le corps du modèle doit faire au plus 20 000 caractères")
		return
	}
	body := model.SanitizeTemplateHTML(req.BodyHTML)

	a.store.Lock()
	db := a.store.Data()
	if req.IsDefault {
		for i := range db.Templates {
			if db.Templates[i].AccountID == acc {
				db.Templates[i].IsDefault = false
			}
		}
	}
	tpl := model.VoucherTemplate{
		ID: model.NewID("tpl-"), AccountID: acc, Name: name, Format: format,
		BodyHTML: body, IsDefault: req.IsDefault, CreatedAt: model.NowISO(),
	}
	db.Templates = append(db.Templates, tpl)
	a.logActivityBy(r, db, acc, "voucher", "Modèle de voucher «"+name+"» créé")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusCreated, tpl)
}

func (a *API) handleTemplateUpdate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Name      *string `json:"name"`
		Format    *string `json:"format"`
		BodyHTML  *string `json:"bodyHtml"`
		IsDefault *bool   `json:"isDefault"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Name != nil && (len(strings.TrimSpace(*req.Name)) < 1 || len(strings.TrimSpace(*req.Name)) > 60) {
		writeErr(w, http.StatusBadRequest, "Le nom du modèle doit faire entre 1 et 60 caractères")
		return
	}
	if req.Format != nil && !templateFormats[strings.TrimSpace(*req.Format)] {
		writeErr(w, http.StatusBadRequest, "Format invalide (a4, 58mm ou 80mm)")
		return
	}
	if req.BodyHTML != nil && len(*req.BodyHTML) > 20000 {
		writeErr(w, http.StatusBadRequest, "Le corps du modèle doit faire au plus 20 000 caractères")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	var tpl *model.VoucherTemplate
	for i := range db.Templates {
		if db.Templates[i].ID == id && db.Templates[i].AccountID == acc {
			tpl = &db.Templates[i]
			break
		}
	}
	if tpl == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Modèle introuvable")
		return
	}
	if req.Name != nil {
		tpl.Name = strings.TrimSpace(*req.Name)
	}
	if req.Format != nil {
		tpl.Format = strings.TrimSpace(*req.Format)
	}
	if req.BodyHTML != nil {
		tpl.BodyHTML = model.SanitizeTemplateHTML(*req.BodyHTML)
	}
	if req.IsDefault != nil {
		if *req.IsDefault {
			for i := range db.Templates {
				if db.Templates[i].AccountID == acc && db.Templates[i].ID != id {
					db.Templates[i].IsDefault = false
				}
			}
		}
		tpl.IsDefault = *req.IsDefault
	}
	updated := *tpl
	a.logActivityBy(r, db, acc, "voucher", "Modèle de voucher «"+updated.Name+"» modifié")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, updated)
}

func (a *API) handleTemplateDelete(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	accTemplates := 0
	idx := -1
	for i := range db.Templates {
		if db.Templates[i].AccountID != acc {
			continue
		}
		accTemplates++
		if db.Templates[i].ID == id {
			idx = i
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Modèle introuvable")
		return
	}
	if accTemplates <= 1 {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Impossible de supprimer le dernier modèle du compte")
		return
	}
	name := db.Templates[idx].Name
	db.Templates = append(db.Templates[:idx], db.Templates[idx+1:]...)
	a.logActivityBy(r, db, acc, "voucher", "Modèle de voucher «"+name+"» supprimé")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
