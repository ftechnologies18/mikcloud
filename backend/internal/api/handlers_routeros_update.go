// Package api — N°115 — mise à jour RouterOS depuis MikCloud : vérification
// (check-for-updates DEPUIS le routeur, la vérité du canal du routeur) et
// installation (download + install + redémarrage). Parité Mikhmon « Update
// RouterOS », sans Winbox.
//
//	POST /api/routers/{id}/routeros-check  → simulated : réponse immédiate ;
//	                                        agent : commande routeros_check en
//	                                        file → {queued, commandId} — le
//	                                        front poll /api/commands/{id}
//	                                        (pattern ping F8) ;
//	POST /api/routers/{id}/routeros-update → simulated : application
//	                                        immédiate (version, uptime,
//	                                        sessions coupées) ; agent :
//	                                        commande routeros_update en file
//	                                        (dédupliquée — un lancement à la
//	                                        fois par routeur).
//
// Aucune nouvelle colonne : la version courante vit déjà dans Router.Version
// (read_state), la confirmation de la mise à jour revient par le journal de
// télémétrie N°115 (applyReadState trace les changements de version).
package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// simRouterOSLatest — dernière version « stable » jouée en mode simulé (la
// démo sans matériel). Cosmétique : en mode agent, la vérité vient des
// serveurs MikroTik via le routeur lui-même.
const simRouterOSLatest = "7.19.4"

// routerOSVersionRe — forme d'une version RouterOS acceptée dans un payload
// (défense en profondeur : la valeur est embarquée dans le script .rsc du
// rapport de lancement et journalisée).
var routerOSVersionRe = regexp.MustCompile(`^[0-9][0-9A-Za-z.\-]{0,31}$`)

// routerOSStateLatest / … — états normalisés du contrôle de mise à jour,
// côté front comme côté rapport agent.
const (
	routerOSStateLatest    = "latest"    // « System is already up to date »
	routerOSStateAvailable = "available" // « New version is available: X »
	routerOSStateError     = "error"     // le routeur rapporte une erreur
	routerOSStateUnknown   = "unknown"   // status illisible — affiché brut
)

// pendingCommandOfKind — commande queued|sent du kind pour ce routeur, nil
// sinon (dédup des lancements lourds). Sous verrou.
func pendingCommandOfKind(db *model.DB, routerID, kind string) *model.Command {
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == routerID && c.Kind == kind && (c.Status == "queued" || c.Status == "sent") {
			return c
		}
	}
	return nil
}

// handleRouterOSCheck — POST /api/routers/{id}/routeros-check.
func (a *API) handleRouterOSCheck(w http.ResponseWriter, r *http.Request) {
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
	if rr.Mode == "agent" {
		// Dédup : une vérification à la fois — le second clic récupère la
		// commande EN COURS (le front repoll la même).
		if pend := pendingCommandOfKind(db, id, model.CmdRouterOSCheck); pend != nil {
			cmdID := pend.ID
			a.store.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{
				"queued": true, "commandId": cmdID,
				"message": "Vérification déjà en attente du check-in du routeur (≤ 45 s)",
			})
			return
		}
		cmd := queueCommandLocked(db, acc, id, model.CmdRouterOSCheck, map[string]any{})
		a.store.Save()
		cmdID := cmd.ID
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"queued": true, "commandId": cmdID,
			"message": "Vérification envoyée — réponse au prochain check-in du routeur (≤ 45 s)",
		})
		return
	}

	// simulated — réponse immédiate, déterministe : la version posée à la
	// création (7.12–7.15) est toujours en retard sur la dernière stable ; un
	// routeur déjà mis à jour (== simRouterOSLatest) est à jour.
	a.store.Unlock()
	state := routerOSStateAvailable
	status := "New version is available: " + simRouterOSLatest
	if rr.Version == simRouterOSLatest {
		state = routerOSStateLatest
		status = "System is already up to date"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"queued": false, "ok": true,
		"state":            state,
		"status":           status,
		"latestVersion":    simRouterOSLatest,
		"installedVersion": rr.Version,
		"channel":          "stable",
	})
}

// decodeBodyTolerant — decodeBody qui tolère l'ABSENCE de corps (EOF →
// structure zéro, les champs optionnels gardent leurs valeurs nulles : un
// POST nu — curl -X POST sans -d, le bouton du front sans cible — ne doit
// pas échouer) ; un corps PRÉSENT mais invalide reste une erreur.
func decodeBodyTolerant(r *http.Request, v any) error {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}

// handleRouterOSUpdate — POST /api/routers/{id}/routeros-update {latest?}.
func (a *API) handleRouterOSUpdate(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Latest string `json:"latest"`
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
	if rr.Mode == "agent" {
		// Dédup stricte : l'installation télécharge puis REDÉMARRE le
		// routeur — jamais deux lancements en parallèle. Le second clic
		// récupère la commande en vol.
		if pend := pendingCommandOfKind(db, id, model.CmdRouterOSUpdate); pend != nil {
			cmdID := pend.ID
			a.store.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{
				"queued": true, "commandId": cmdID, "already": true,
				"message": "Une mise à jour RouterOS est déjà en cours sur ce routeur",
			})
			return
		}
		payload := map[string]any{}
		if latest != "" {
			payload["latest"] = latest
		}
		cmd := queueCommandLocked(db, acc, id, model.CmdRouterOSUpdate, payload)
		a.logActivityBy(r, db, acc, "router", "Mise à jour RouterOS demandée sur «"+rr.Name+"»"+versionSuffix(latest))
		a.store.Save()
		cmdID := cmd.ID
		a.store.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"queued": true, "commandId": cmdID,
			"message": "Mise à jour envoyée — installation au prochain check-in (≤ 45 s), puis téléchargement et redémarrage (2 à 5 min)",
		})
		return
	}

	// simulated — application immédiate : la version passe à la cible (à
	// défaut la dernière stable simulée), le routeur « redémarre » (uptime à
	// zéro, sessions coupées et journalisées logout — miroir exact du reboot
	// simulé handleRouterPower).
	next := latest
	if next == "" {
		next = simRouterOSLatest
	}
	prev := rr.Version
	rr.Version = next
	rr.UptimeSec = 0
	rr.ActiveSessions = 0
	now := time.Now().UTC()
	kept := db.Sessions[:0]
	for _, s := range db.Sessions {
		if s.RouterID == id {
			logRouterUserEvent(db, rr, s, "logout", now)
			continue
		}
		kept = append(kept, s)
	}
	db.Sessions = kept
	a.logActivityBy(r, db, acc, "router", "RouterOS de «"+rr.Name+"» mis à jour : "+prev+" → "+next+" (routeur redémarré)")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": next})
}

// versionSuffix — «  vers X » si une version cible est fournie.
func versionSuffix(latest string) string {
	if latest == "" {
		return ""
	}
	return " vers " + latest
}

// normalizeRouterOSCheck — N°115 — le rapport routeros_check arrive en
// valeurs formulaire (chaînes) : le front attend state + status + latestVersion
// + installedVersion + channel. L'état est dérivé du status RouterOS BRUT
// (« System is already up to date », « New version is available: X », « Error:
// … ») avec repli sur la comparaison installed != latest (un status de build
// inconnue ne doit pas masquer une mise à jour évidente) ; l'état « unknown »
// préserve le status brut pour affichage honnête.
func normalizeRouterOSCheck(res map[string]any) {
	if res == nil {
		return
	}
	str := func(k string) string {
		if v, ok := res[k].(string); ok {
			return strings.TrimSpace(v)
		}
		return ""
	}
	raw := str("rosStatus")
	latest, installed, channel := str("latest"), str("installed"), str("channel")

	// Borne les chaînes libres rapportées (défense : le status RouterOS est
	// court, mais un firmware exotique ne doit jamais gonfler l'historique).
	if len(raw) > 160 {
		raw = raw[:160]
	}

	state := routerOSStateUnknown
	low := strings.ToLower(raw)
	switch {
	case strings.Contains(low, "up to date"):
		state = routerOSStateLatest
	case strings.Contains(low, "new version"):
		state = routerOSStateAvailable
	case strings.Contains(low, "error"):
		state = routerOSStateError
	case latest != "" && installed != "" && latest != installed:
		// Repli robuste : le champ latest-version tranchera même si le
		// libellé du status change de forme entre builds RouterOS.
		state = routerOSStateAvailable
	case raw != "":
		state = routerOSStateUnknown
	}
	if state == routerOSStateUnknown && raw == "" && latest != "" && installed != "" && latest == installed {
		state = routerOSStateLatest
	}

	res["state"] = state
	res["status"] = raw
	res["latestVersion"] = latest
	res["installedVersion"] = installed
	res["channel"] = channel
}
