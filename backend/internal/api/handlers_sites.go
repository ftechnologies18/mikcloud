// handlers_sites.go — N°182 — sites physiques et personnalisation du portail
// captif par site et par routeur (chaîne ROUTEUR → SITE → COMPTE).
//
// Le « site » est un regroupement de routeurs du compte (hôtel
// multi-bâtiments, chaîne de cybercafés, campus) qui peut porter sa propre
// identité de portail (surcharge de branding). Trois régimes cohabitent,
// AU CHOIX du gérant, SANS migration :
//   - portail UNIFIÉ (statu quo historique) : routeurs hors site, sans
//     surcharge — ils servent le portail du compte ;
//   - portail PAR SITE : les routeurs assignés à un site surchargé servent
//     l'identité du site ;
//   - portail INDIVIDUEL : une surcharge posée directement sur le routeur
//     prime sur son site puis sur le compte.
//
// Tout changement (assignation, surcharge) rejoint l'empreinte de signature
// hotspot_files → re-déploiement automatique au check-in suivant (≤ 45 s),
// exactement comme un changement de branding du compte (N°135).
package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"mikcloud/hotspot-api/internal/model"
)

// siteResponse — un site vu par la console : l'entité + les compteurs de
// routeurs assignés (l'UI affiche le régime de portail de chaque routeur).
type siteResponse struct {
	model.Site
	RouterCount int      `json:"routerCount"`
	RouterIDs   []string `json:"routerIds"`
	HasOverride bool     `json:"hasOverride"`
}

// siteResponseLocked — construit la réponse d'un site sous verrou (compte les
// routeurs assignés du MÊME compte — l'isolation se vérifie à chaque lecture).
func siteResponseLocked(db *model.DB, s model.Site) siteResponse {
	out := siteResponse{Site: s, RouterIDs: []string{}}
	for i := range db.Routers {
		if db.Routers[i].AccountID == s.AccountID && db.Routers[i].SiteID == s.ID {
			out.RouterCount++
			out.RouterIDs = append(out.RouterIDs, db.Routers[i].ID)
		}
	}
	out.HasOverride = s.PortalOverride != ""
	return out
}

// handleSitesList — GET /api/sites : les sites du compte, triés par création
// décroissante (même discipline que la liste des routeurs).
func (a *API) handleSitesList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	db := a.store.Data()
	out := []siteResponse{}
	for i := range db.Sites {
		if db.Sites[i].AccountID == acc {
			out = append(out, siteResponseLocked(db, db.Sites[i]))
		}
	}
	a.store.Unlock()
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	writeJSON(w, http.StatusOK, out)
}

// sitePutReq — corps de création/édition d'un site. L'override est TOUJOURS
// remplacé en entier quand il est fourni (non-nil) : la console renvoie
// l'état complet du formulaire (même contrat que la carte Expérience du
// compte) ; nil = inchangé (PUT partiel défensif).
type sitePutReq struct {
	Name           *string            `json:"name"`
	Description    *string            `json:"description"`
	Location       *string            `json:"location"`
	PortalOverride *portalOverrideReq `json:"portalOverride"`
}

// handleSiteCreate — POST /api/sites.
func (a *API) handleSiteCreate(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	var req sitePutReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Name == nil {
		writeErr(w, http.StatusBadRequest, "Nom du site requis")
		return
	}
	name := strings.TrimSpace(*req.Name)
	description := strings.TrimSpace(strOr(req.Description, ""))
	location := strings.TrimSpace(strOr(req.Location, ""))
	if name == "" {
		writeErr(w, http.StatusBadRequest, "Nom du site requis (1-60 caractères)")
		return
	}
	if len(name) > 60 {
		writeErr(w, http.StatusBadRequest, "Nom du site trop long (60 caractères max)")
		return
	}
	if len(description) > 200 {
		writeErr(w, http.StatusBadRequest, "Description trop longue (200 caractères max)")
		return
	}
	if len(location) > 120 {
		writeErr(w, http.StatusBadRequest, "Localisation trop longue (120 caractères max)")
		return
	}
	overrideJSON := ""
	if req.PortalOverride != nil {
		v, err := assemblePortalOverride(req.PortalOverride)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Personnalisation du portail invalide : "+err.Error())
			return
		}
		overrideJSON = v
	}
	a.store.Lock()
	db := a.store.Data()
	// Unicité du nom PAR COMPTE (même discipline que les routeurs).
	for i := range db.Sites {
		if db.Sites[i].AccountID == acc && strings.EqualFold(db.Sites[i].Name, name) {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Ce nom de site existe déjà")
			return
		}
	}
	// Plafond : le site est un regroupement, pas une collection ouverte.
	count := 0
	for i := range db.Sites {
		if db.Sites[i].AccountID == acc {
			count++
		}
	}
	if count >= model.MaxSitesPerAccount {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Nombre maximum de sites atteint (20)")
		return
	}
	now := model.NowISO()
	site := model.Site{
		ID: model.NewID("st-"), AccountID: acc,
		Name: name, Description: description, Location: location,
		PortalOverride: overrideJSON,
		CreatedAt:      now, UpdatedAt: now,
	}
	db.Sites = append(db.Sites, site)
	msg := "Site " + site.Name + " ajouté"
	if overrideJSON != "" {
		msg += " (portail personnalisé)"
	}
	a.logActivityBy(r, db, acc, "site", msg)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, siteResponseLocked(db, site))
}

// handleSiteUpdate — PUT /api/sites/{id} : champs descriptifs en pointeurs
// (nil = inchangé), override remplacé en entier quand fourni.
func (a *API) handleSiteUpdate(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	var req sitePutReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	overrideJSON := ""
	if req.PortalOverride != nil {
		v, err := assemblePortalOverride(req.PortalOverride)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Personnalisation du portail invalide : "+err.Error())
			return
		}
		overrideJSON = v
	}
	a.store.Lock()
	db := a.store.Data()
	cur := model.FindSiteScoped(db, id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Site introuvable")
		return
	}
	updated := *cur
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Nom du site requis (1-60 caractères)")
			return
		}
		if len(name) > 60 {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Nom du site trop long (60 caractères max)")
			return
		}
		updated.Name = name
	}
	if req.Description != nil {
		description := strings.TrimSpace(*req.Description)
		if len(description) > 200 {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Description trop longue (200 caractères max)")
			return
		}
		updated.Description = description
	}
	if req.Location != nil {
		location := strings.TrimSpace(*req.Location)
		if len(location) > 120 {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Localisation trop longue (120 caractères max)")
			return
		}
		updated.Location = location
	}
	// Unicité du nom (hors le site lui-même).
	for i := range db.Sites {
		if db.Sites[i].AccountID == acc && db.Sites[i].ID != updated.ID && strings.EqualFold(db.Sites[i].Name, updated.Name) {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Ce nom de site existe déjà")
			return
		}
	}
	if req.PortalOverride != nil {
		updated.PortalOverride = overrideJSON
	}
	updated.UpdatedAt = model.NowISO()
	*cur = updated
	msg := "Site " + updated.Name + " modifié"
	a.logActivityBy(r, db, acc, "site", msg)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, siteResponseLocked(db, updated))
}

// handleSiteDelete — DELETE /api/sites/{id} : les routeurs encore assignés
// sont DÉTACHÉS automatiquement (ils redeviennent hors site et servent le
// portail du compte ; leur sig change → re-déploiement automatique ≤ 45 s).
// La réponse porte le nombre de routeurs détachés — la console l'annonce
// AVANT dans son AlertDialog (aucune surprise silencieuse).
func (a *API) handleSiteDelete(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	idx := -1
	for i := range db.Sites {
		if db.Sites[i].ID == id && db.Sites[i].AccountID == acc {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Site introuvable")
		return
	}
	name := db.Sites[idx].Name
	detached := 0
	routers := db.Routers[:0:0]
	for i := range db.Routers {
		rr := db.Routers[i]
		if rr.AccountID == acc && rr.SiteID == id {
			rr.SiteID = ""
			detached++
		}
		routers = append(routers, rr)
	}
	db.Routers = routers
	db.Sites = append(db.Sites[:idx], db.Sites[idx+1:]...)
	msg := "Site " + name + " supprimé"
	if detached > 0 {
		msg += " (" + strconv.Itoa(detached) + " routeur(s) détaché(s) — portail du compte réappliqué)"
	}
	a.logActivityBy(r, db, acc, "site", msg)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"message": msg, "detached": detached})
}

// handleRouterSiteAssign — PUT /api/routers/{id}/site {siteId} : assignation
// (ou détachement quand siteId est vide). Le site doit appartenir au MÊME
// compte (isolation multi-tenant vérifiée serveur, à CHAQUE assignation).
// La signature hotspot_files change au check-in suivant → re-déploiement
// automatique du portail du routeur (≤ 45 s).
func (a *API) handleRouterSiteAssign(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		SiteID string `json:"siteId"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	siteID := strings.TrimSpace(req.SiteID)
	a.store.Lock()
	db := a.store.Data()
	router := findRouterScoped(db, id, acc)
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	siteName := ""
	if siteID != "" {
		site := model.FindSiteScoped(db, siteID, acc)
		if site == nil {
			a.store.Unlock()
			writeErr(w, http.StatusBadRequest, "Site introuvable")
			return
		}
		siteName = site.Name
	}
	prevSiteID := router.SiteID
	router.SiteID = siteID
	msg := ""
	if siteID == "" {
		if prevSiteID != "" {
			msg = "Routeur " + router.Name + " retiré de son site — portail du compte réappliqué"
		} else {
			msg = "Routeur " + router.Name + " hors site (inchangé)"
		}
	} else {
		msg = "Routeur " + router.Name + " assigné au site " + siteName
	}
	a.logActivityBy(r, db, acc, "router", msg)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"message": msg, "siteId": siteID})
}

// handleRouterPortalPut — PUT /api/routers/{id}/portal : surcharge
// INDIVIDUELLE du portail du routeur (le niveau le plus fin de la chaîne
// N°182). Le corps est la surcharge COMPLÈTE (portailOverrideReq) : tous les
// champs vides = le routeur hérite de son site puis du compte (retour au
// régime unifié/par site). La signature change au check-in suivant →
// re-déploiement automatique (≤ 45 s).
func (a *API) handleRouterPortalPut(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	var req portalOverrideReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	overrideJSON, err := assemblePortalOverride(&req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Personnalisation du portail invalide : "+err.Error())
		return
	}
	a.store.Lock()
	db := a.store.Data()
	router := findRouterScoped(db, id, acc)
	if router == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	router.PortalOverride = overrideJSON
	msg := "Portail du routeur " + router.Name + " personnalisé"
	if overrideJSON == "" {
		msg = "Portail du routeur " + router.Name + " réinitialisé (hérite du site puis du compte)"
	}
	a.logActivityBy(r, db, acc, "router", msg)
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"message": msg, "portalOverride": overrideJSON})
}

// strOr — valeur du pointeur ou repli chaîne vide (saisies descriptives).
func strOr(s *string, def string) string {
	if s == nil {
		return def
	}
	return *s
}
