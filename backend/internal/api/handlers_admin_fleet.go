// Package api — N°117 — mise à jour RouterOS de FLOTTE : le super-admin
// pilote le parc de TOUS les comptes clients depuis la console plateforme.
// Suite directe du N°115 (le gérant met à jour SON routeur) : la vue
// « Parc routeurs » liste chaque routeur de chaque compte avec sa version
// installée, la dernière version détectée et son état (à jour / mise à jour
// disponible / vérification en cours / installation en cours / erreur),
// puis deux gestes :
//
//	POST /api/admin/fleet/routeros-check  → enfile un routeros_check sur
//	                                       chaque routeur agent (lecture
//	                                       seule, sans risque) — les
//	                                       résultats reviennent au rythme
//	                                       des check-ins ;
//	POST /api/admin/fleet/routeros-update → installe la mise à jour sur les
//	                                       routeurs ciblés (corps optionnel
//	                                       {routerIds} — absent : TOUS les
//	                                       routeurs où une mise à jour a
//	                                       été DÉTECTÉE, jamais à l'aveugle).
//
// SÉCURITÉ — la cible par défaut n'est PAS « tous les routeurs » mais
// « tous les routeurs avec une mise à jour connue disponible » : un update
// RouterOS REDÉMARRE le routeur et coupe le hotspot du client ; ne cibler
// que le retard connu (dernier check abouti) évite de faire redémarrer un
// parc déjà à jour pour rien. La confirmation du front montre le compte
// exact avant le geste.
//
// ZÉRO NOUVEAU SCHÉMA — l'état de flotte dérive de l'existant :
//   - Router.Version (télémétrie read_state) = version installée ;
//   - la dernière commande routeros_check aboutie porte l'état normalisé
//     (normalizeRouterOSCheck, N°115) dans son Result ;
//   - les commandes en vol portent checking/updating (dédup maison :
//     pendingCommandOfKind).
//
// Les routeurs SIMULÉS n'ont pas d'agent : leur état est calculé à la
// volée (version posée vs dernière stable simulée — miroir du chemin
// simulated de N°115) et l'update de flotte leur applique la mise à jour
// immédiatement (démo sans matériel).
//
// Les commandes sont enfilées sous le COMPTE CLIENT du routeur
// (queueCommandLocked acc = rr.AccountID) : le gérant concerné voit
// l'opération dans SON journal d'activité, et le rapport agent remonte
// par le chemin standard N°115.
package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// fleetRouterOSState — état de mise à jour RouterOS d'un routeur, dérivé
// des commandes N°115 (voir l'en-tête du fichier). Champs vides = inconnu.
type fleetRouterOSState struct {
	state       string // latest | available | error | unknown | "" (jamais vérifié)
	latest      string // version disponible (dernier check abouti)
	status      string // status RouterOS brut (borné), affiché honnêtement
	checkedAt   string // DoneAt du dernier check abouti
	checking    bool   // un routeros_check queued|sent est en vol
	updating    bool   // un routeros_update queued|sent est en vol
	updateError string // message de la dernière installation échouée
	fwCurrent   string // N°125 — firmware RouterBOARD en place (dernier check)
	fwStaged    string // N°125 — firmware RouterBOARD en attente (livré avec le paquet RouterOS)
}

// fleetRouterOSStateOf — parcourt l'historique des commandes du routeur et
// dérive l'état : dernières abouties par kind (l'ordre FIFO du slice fait
// que le dernier vu gagne), commandes en vol par kind. Sous verrou.
func fleetRouterOSStateOf(db *model.DB, rr *model.Router) fleetRouterOSState {
	var st fleetRouterOSState
	lastCheck, lastUpdateErr := "", ""
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID != rr.ID {
			continue
		}
		switch c.Kind {
		case model.CmdRouterOSCheck:
			if c.Status == "queued" || c.Status == "sent" {
				st.checking = true
				continue
			}
			if c.DoneAt == "" || c.DoneAt < lastCheck {
				continue
			}
			lastCheck = c.DoneAt
			st.checkedAt = c.DoneAt
			st.state, st.latest, st.status = "", "", ""
			if c.Status == "error" {
				st.state = routerOSStateError
				st.status = boundedString(c.Result["message"], 160)
				continue
			}
			st.state = boundedString(c.Result["state"], 32)
			if st.state == "" {
				st.state = routerOSStateUnknown
			}
			st.latest = boundedString(c.Result["latestVersion"], 32)
			st.status = boundedString(c.Result["status"], 160)
		case model.CmdRouterOSUpdate:
			if c.Status == "queued" || c.Status == "sent" {
				st.updating = true
				continue
			}
			if c.Status == "error" && c.DoneAt != "" && c.DoneAt >= lastUpdateErr {
				lastUpdateErr = c.DoneAt
				st.updateError = boundedString(c.Result["message"], 160)
			}
		}
	}
	// Simulé : pas d'agent, donc aucune commande — l'état est la vérité
	// calculée (version posée vs dernière stable simulée, miroir N°115).
	if rr.Mode == "simulated" && rr.Version != "" && st.state == "" {
		if rr.Version == simRouterOSLatest {
			st.state = routerOSStateLatest
		} else {
			st.state = routerOSStateAvailable
		}
		st.latest = simRouterOSLatest
		// N°125 — le firmware simulé suit toujours le RouterOS (auto-upgrade
		// simulé) : jamais en attente en mode démo.
		st.fwCurrent = rr.Version
		st.fwStaged = rr.Version
	}
	return st
}

// boundedString — chaîne bornée d'une valeur de rapport (défense : un
// firmware exotique ne gonfle ni le JSON de flotte ni le journal).
func boundedString(v any, max int) string {
	s, _ := v.(string)
	s = strings.TrimSpace(s)
	if len(s) > max {
		s = s[:max]
	}
	return s
}

// fleetRouterOut — ligne de la vue « Parc routeurs » (super-admin).
type fleetRouterOut struct {
	ID          string `json:"id"`
	AccountID   string `json:"accountId"`
	AccountName string `json:"accountName"`
	Name        string `json:"name"`
	Mode        string `json:"mode"`   // simulated | real | agent
	Status      string `json:"status"` // online | offline
	Version     string `json:"version"`
	LastSeen    string `json:"lastSeen,omitempty"`

	// État RouterOS N°115/N°117 (dérivé, cf. fleetRouterOSStateOf).
	RosState    string `json:"rosState,omitempty"`    // latest|available|error|unknown (absent = jamais vérifié)
	RosLatest   string `json:"rosLatest,omitempty"`   // version disponible détectée
	RosStatus   string `json:"rosStatus,omitempty"`   // status brut (borné 160)
	CheckedAt   string `json:"checkedAt,omitempty"`   // dernier check abouti
	Checking    bool   `json:"checking"`              // vérification en vol
	Updating    bool   `json:"updating"`              // installation en vol
	UpdateError string `json:"updateError,omitempty"` // dernière installation échouée

	// N°125 — firmware RouterBOARD (dérivé du dernier check abouti).
	FwCurrent string `json:"fwCurrent,omitempty"` // firmware en place
	FwStaged  string `json:"fwStaged,omitempty"`  // firmware en attente (≠ fwCurrent)
}

// handleAdminFleetRouters — GET /api/admin/fleet/routers : le parc complet,
// tous comptes confondus, avec l'état de mise à jour RouterOS de chacun.
func (a *API) handleAdminFleetRouters(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	a.store.Lock()
	db := a.store.Data()

	accName := make(map[string]string, len(db.Accounts))
	for i := range db.Accounts {
		accName[db.Accounts[i].ID] = db.Accounts[i].Name
	}

	out := make([]fleetRouterOut, 0, len(db.Routers))
	summary := map[string]int{
		"total": 0, "agent": 0, "simulated": 0, "real": 0,
		"online": 0, "checking": 0, "updating": 0,
		"available": 0, "latest": 0,
	}
	for i := range db.Routers {
		rr := &db.Routers[i]
		st := fleetRouterOSStateOf(db, rr)
		summary["total"]++
		summary[rr.Mode]++
		if rr.Status == "online" {
			summary["online"]++
		}
		if st.checking {
			summary["checking"]++
		}
		if st.updating {
			summary["updating"]++
		}
		if st.state == routerOSStateAvailable {
			summary["available"]++
		}
		if st.state == routerOSStateLatest {
			summary["latest"]++
		}
		name := accName[rr.AccountID]
		if name == "" {
			name = rr.AccountID
		}
		out = append(out, fleetRouterOut{
			ID: rr.ID, AccountID: rr.AccountID, AccountName: name,
			Name: rr.Name, Mode: rr.Mode, Status: rr.Status, Version: rr.Version, LastSeen: rr.LastSeen,
			RosState: st.state, RosLatest: st.latest, RosStatus: st.status, CheckedAt: st.checkedAt,
			Checking: st.checking, Updating: st.updating, UpdateError: st.updateError,
			FwCurrent: st.fwCurrent, FwStaged: st.fwStaged,
		})
	}
	a.store.Unlock()

	sort.Slice(out, func(i, j int) bool {
		if out[i].AccountName != out[j].AccountName {
			return out[i].AccountName < out[j].AccountName
		}
		return out[i].Name < out[j].Name
	})
	writeJSON(w, http.StatusOK, map[string]any{"routers": out, "summary": summary})
}

// fleetResolveTargets — résout les routeurs ciblés par un geste de flotte :
// liste explicite d'IDs (bouton par routeur) ou, à défaut, TOUS les routeurs
// du parc (le filtre available/non-agent est fait par l'APPELANT — le check
// vise tous les agents, l'update filtre sur le retard connu). Sous verrou.
func fleetResolveTargets(db *model.DB, ids []string) []*model.Router {
	if len(ids) == 0 {
		targets := make([]*model.Router, 0, len(db.Routers))
		for i := range db.Routers {
			targets = append(targets, &db.Routers[i])
		}
		return targets
	}
	var targets []*model.Router
	for _, id := range ids {
		for i := range db.Routers {
			if db.Routers[i].ID == id {
				targets = append(targets, &db.Routers[i])
				break
			}
		}
	}
	return targets
}

// handleAdminFleetRouterOSCheck — POST /api/admin/fleet/routeros-check
// {routerIds?} — enfile un routeros_check sur chaque routeur AGENT ciblé
// (liste explicite ou tout le parc). Lecture seule : aucun risque. Les
// routeurs simulés n'ont rien à enfiler (état calculé à la volée au GET),
// le mode réel n'est pas supporté (matrice §0). Dédup par routeur : un
// check déjà en vol n'est pas re-enfilé.
func (a *API) handleAdminFleetRouterOSCheck(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	var req struct {
		RouterIDs []string `json:"routerIds"`
	}
	if err := decodeBodyTolerant(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	queued, skipped := 0, 0
	for _, rr := range fleetResolveTargets(db, req.RouterIDs) {
		if rr.Mode != "agent" {
			skipped++ // simulated : état live au GET ; real : non supporté
			continue
		}
		if pendingCommandOfKind(db, rr.ID, model.CmdRouterOSCheck) != nil {
			skipped++ // déjà en vol — la réponse arrivera au check-in
			continue
		}
		queueCommandLocked(db, rr.AccountID, rr.ID, model.CmdRouterOSCheck, map[string]any{})
		queued++
	}
	if queued > 0 {
		a.store.Save()
	}
	a.store.Unlock()

	msg := "Vérification envoyée — les réponses arrivent au rythme des check-ins des routeurs (≤ 45 s par routeur en ligne)"
	if queued == 0 {
		msg = "Aucune vérification à lancer (déjà en cours ou parc sans routeur agent)"
	}
	writeJSON(w, http.StatusOK, map[string]any{"queued": queued, "skipped": skipped, "message": msg})
}

// handleAdminFleetRouterOSUpdate — POST /api/admin/fleet/routeros-update
// {routerIds?, latest?} — installe la mise à jour RouterOS sur les routeurs
// ciblés. Sans routerIds : TOUS les routeurs avec une mise à jour DÉTECTÉE
// (état available du dernier check — jamais à l'aveugle : un update
// redémarre le routeur et coupe le hotspot du client). Agents : commande
// routeros_update (dédup stricte — jamais deux installations en parallèle
// sur un même routeur) avec la cible du dernier check ; simulés :
// application immédiate (miroir du chemin simulated de N°115 — version,
// uptime à zéro, sessions coupées et journalisées logout).
func (a *API) handleAdminFleetRouterOSUpdate(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	var req struct {
		RouterIDs []string `json:"routerIds"`
		Latest    string   `json:"latest"`
	}
	if err := decodeBodyTolerant(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	latest := strings.TrimSpace(req.Latest)
	if latest != "" && !routerOSVersionRe.MatchString(latest) {
		writeErr(w, http.StatusBadRequest, "Version cible invalide")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	queued, applied, skipped := 0, 0, 0
	// Résumé par compte client : UNE entrée de journal par compte (un geste
	// de flotte ne doit pas inonder le journal de 50 lignes identiques).
	type accSummary struct {
		queuedNames, appliedNames []string
	}
	perAcc := map[string]*accSummary{}
	now := time.Now().UTC()

	for _, rr := range fleetResolveTargets(db, req.RouterIDs) {
		st := fleetRouterOSStateOf(db, rr)
		if rr.Mode == "real" {
			skipped++
			continue
		}
		// La barrière « available » s'applique TOUJOURS, en ciblage explicite
		// comme en flotte : un routeur à jour (ou jamais vérifié) n'est pas
		// re-redémarré pour rien — le front ne propose le bouton d'un routeur
		// que sur retard détecté, le serveur ne fait pas confiance au front.
		if st.state != routerOSStateAvailable {
			skipped++
			continue
		}
		target := st.latest
		if target == "" {
			target = latest
		}
		if rr.Mode == "agent" {
			if pendingCommandOfKind(db, rr.ID, model.CmdRouterOSUpdate) != nil {
				skipped++ // installation déjà en vol
				continue
			}
			payload := map[string]any{}
			if target != "" {
				payload["latest"] = target
			}
			queueCommandLocked(db, rr.AccountID, rr.ID, model.CmdRouterOSUpdate, payload)
			queued++
			s, ok := perAcc[rr.AccountID]
			if !ok {
				s = &accSummary{}
				perAcc[rr.AccountID] = s
			}
			s.queuedNames = append(s.queuedNames, "«"+rr.Name+"»")
			continue
		}
		// Simulated — application immédiate (miroir exact du chemin
		// simulated de N°115 : la version passe à la cible, le routeur
		// « redémarre », les sessions sont coupées et journalisées logout).
		next := target
		if next == "" {
			next = simRouterOSLatest
		}
		prev := rr.Version
		rr.Version = next
		rr.UptimeSec = 0
		rr.ActiveSessions = 0
		kept := db.Sessions[:0]
		for _, s := range db.Sessions {
			if s.RouterID == rr.ID {
				logRouterUserEvent(db, rr, s, "logout", now)
				continue
			}
			kept = append(kept, s)
		}
		db.Sessions = kept
		applied++
		s, ok := perAcc[rr.AccountID]
		if !ok {
			s = &accSummary{}
			perAcc[rr.AccountID] = s
		}
		s.appliedNames = append(s.appliedNames, "«"+rr.Name+"» ("+prev+" → "+next+")")
	}

	// Journal : une entrée par compte client concerné — le gérant voit dans
	// SON journal que la plateforme a agi sur son parc (actor = super-admin).
	for acc, s := range perAcc {
		var parts []string
		if n := len(s.queuedNames); n > 0 {
			parts = append(parts, fleetNamesLabel(n, s.queuedNames)+" en file d'installation (téléchargement puis redémarrage, 2 à 5 min par routeur)")
		}
		if n := len(s.appliedNames); n > 0 {
			parts = append(parts, fleetNamesLabel(n, s.appliedNames)+" mis à jour immédiatement (routeurs redémarrés)")
		}
		a.logActivityBy(r, db, acc, "router", "Mise à jour RouterOS de flotte lancée par la plateforme : "+strings.Join(parts, " ; "))
	}
	if queued > 0 || applied > 0 {
		a.store.Save()
	}
	a.store.Unlock()

	total := queued + applied
	msg := "Aucune mise à jour à lancer — vérifiez d'abord le parc (les routeurs à jour ou déjà en installation sont ignorés)"
	if total > 0 {
		msg = "Mise à jour de flotte lancée : " +
			fleetCountLabel(queued, "routeur en file d'installation", "routeurs en file d'installation") +
			", " + fleetCountLabel(applied, "routeur mis à jour immédiatement", "routeurs mis à jour immédiatement") +
			" — chaque routeur redémarre à son tour, le portail coupe pendant l'opération"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"queued": queued, "applied": applied, "skipped": skipped, "message": msg,
	})
}

// fleetNamesLabel — « A », « B » + 3 autres — les noms sont bornés à 3 dans
// le journal (un parc de 50 routeurs ne doit pas produire une ligne illisible).
func fleetNamesLabel(n int, names []string) string {
	shown := names
	if len(shown) > 3 {
		shown = shown[:3]
	}
	label := strings.Join(shown, ", ")
	if n > 3 {
		label += " + " + fleetCountLabel(n-3, "autre", "autres")
	}
	return label
}

// fleetCountLabel — « N <singulier> » / « N <pluriel> » avec pluriel honnête
// (le pluriel français n'est pas un simple « s » appendu : « en file
// d'installation » → « en file d'installation » ne change pas, « autre » →
// « autres » change).
func fleetCountLabel(n int, singular, plural string) string {
	if n > 1 {
		return strconv.Itoa(n) + " " + plural
	}
	return strconv.Itoa(n) + " " + singular
}
