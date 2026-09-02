// Package api — provisionning agent côté console (auth JWT) :
//
//	GET  /api/routers/{id}/provision     → état du provisionning (token stocké haché)
//	POST /api/routers/{id}/rotate-token  → nouveau token + script complet
//	POST /api/routers/{id}/refresh       → file une commande read_state
//	POST /api/routers/{id}/import        → file un import complet (profils + utilisateurs)
package api

import (
	"fmt"
	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Console : provisionning / rotation / refresh
// ---------------------------------------------------------------------------

// handleRouterProvision — état du provisionning agent d'un routeur.
// Le token n'est pas stocké en clair : après fermeture de la fenêtre de création,
// il faut "Régénérer le script" (rotation) pour en obtenir un nouveau.
func (a *API) handleRouterProvision(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	cur := findRouterScoped(a.store.Data(), id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	rc := *cur
	a.store.Unlock()

	if rc.Mode != "agent" {
		writeErr(w, http.StatusBadRequest, "Ce routeur n'est pas en mode agent")
		return
	}
	online := false
	if t, err := time.Parse(time.RFC3339, rc.LastSeen); err == nil {
		online = time.Since(t) < OnlineWindow
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":         "agent",
		"tokenPreview": rc.TokenPreview,
		"provisioned":  rc.AgentTokenHash != "",
		"lastSeen":     rc.LastSeen,
		"online":       online,
		"scheduler":    agent.SchedulerName,
		"scriptFile":   agent.ScriptFilename,
		"note":         "Token stocké haché : utilisez rotate-token pour obtenir un nouveau script",
	})
}

// handleRouterRotateToken — régénère le token et renvoie le script complet.
func (a *API) handleRouterRotateToken(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	token, err := agent.NewToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Génération de token impossible")
		return
	}
	a.store.Lock()
	cur := findRouterScoped(a.store.Data(), id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if cur.Mode != "agent" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Ce routeur n'est pas en mode agent")
		return
	}
	cur.AgentTokenHash = agent.HashToken(token)
	cur.TokenPreview = agent.Preview(token)
	cur.LastSeen = ""
	cur.Status = "offline" // l'ancien agent (token périmé) ne check-inera plus
	a.logActivityBy(r, a.store.Data(), cur.AccountID, "router", "Token agent régénéré pour «"+cur.Name+"»")
	a.store.Save()
	name := cur.Name
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"agentToken":    token,
		"installScript": agent.InstallScript(agentBaseURL(r), token, name),
	})
}

// handleRouterRefresh — file une commande read_state pour un routeur agent.
func (a *API) handleRouterRefresh(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	cur := findRouterScoped(db, id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if cur.Mode != "agent" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Le rafraîchissement agent ne s'applique qu'au mode agent")
		return
	}
	cmd := queueCommandLocked(db, cur.AccountID, id, model.CmdReadState, map[string]any{})
	a.store.Save()
	cmdID := cmd.ID
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"queued":    true,
		"commandId": cmdID,
		"message":   "Synchronisation en attente du prochain check-in du routeur (≤ 45 s)",
	})
}

// handleRouterImport — file un import complet (profils + utilisateurs) d'un
// routeur agent vers le cloud. Utilisé après la première connexion d'un
// routeur qui gérait déjà son hotspot (Mikhmon, configuration manuelle…).
func (a *API) handleRouterImport(w http.ResponseWriter, r *http.Request) {
	// P3 — compte expiré : écritures métier refusées (l'import crée des
	// utilisateurs hotspot — lecture seule).
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	cur := findRouterScoped(db, id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if cur.Mode != "agent" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "L'import ne s'applique qu'aux routeurs en mode agent")
		return
	}
	if cur.Status != "online" {
		a.store.Unlock()
		writeErr(w, http.StatusConflict, "Le routeur doit être en ligne pour importer (le check-in de l'agent alimente l'import)")
		return
	}
	cmd := queueCommandLocked(db, cur.AccountID, id, model.CmdImportHotspot,
		map[string]any{"start": 0, "count": agent.ImportChunkSize})
	a.store.Save()
	cmdID := cmd.ID
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"queued":    true,
		"commandId": cmdID,
		"message":   "Import en attente du prochain check-in du routeur (≤ 45 s)",
	})
}

// parseRosDurationMin — parse une durée RouterOS en minutes :
// "08:00:00" → 480, "1d" → 1440, "1d02:00:00" → 1560, "1h30m" → 90, "45s" → 1.
// Retourne 0 si la valeur est vide/illisible.
func parseRosDurationMin(s string) int {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0
	}
	// Format horloge RouterOS : [Nd]H:MM:SS (ou MM:SS).
	if strings.Contains(s, ":") {
		days := 0
		if idx := strings.Index(s, "d"); idx >= 0 {
			if d, err := strconv.Atoi(strings.TrimSpace(s[:idx])); err == nil {
				days = d
			}
			s = s[idx+1:]
		}
		parts := strings.Split(s, ":")
		h, m, sec := 0, 0, 0
		if len(parts) >= 3 {
			h, _ = strconv.Atoi(parts[0])
			m, _ = strconv.Atoi(parts[1])
			sec, _ = strconv.Atoi(parts[2])
		} else if len(parts) == 2 {
			m, _ = strconv.Atoi(parts[0])
			sec, _ = strconv.Atoi(parts[1])
		}
		total := days*1440 + h*60 + m
		if total == 0 && sec > 0 {
			total = 1
		}
		return total
	}
	// Format suffixé : 1d2h3m4s.
	total := 0
	num := ""
	for _, c := range s {
		if c >= '0' && c <= '9' {
			num += string(c)
			continue
		}
		n := 0
		if num != "" {
			n, _ = strconv.Atoi(num)
			num = ""
		}
		switch c {
		case 'd':
			total += n * 1440
		case 'h':
			total += n * 60
		case 'm':
			total += n
		case 's':
			if n > 0 && total == 0 {
				total = 1 // arrondi : moins d'une minute → 1 min
			}
		}
	}
	return total
}

// applyImportHotspot — applique le résultat d'un chunk d'import (profils +
// utilisateurs existants sur le routeur).
//
// Règles :
//   - le cloud reste MAÎTRE : une entrée déjà connue n'est jamais écrasée
//     (les profils matchent par AccountID+nom ; les utilisateurs par
//     username DU COMPTE — voir usernameTaken) ;
//   - l'import est paginé : si le chunk n'était pas le dernier, le suivant
//     est enfilé ici (même mécanique que read_state) ;
//   - les utilisateurs importés sont "regular", statut actif/disabled, avec
//     leur profil (nom) et leur commentaire (tracé Mikhmon éventuel).
func (a *API) applyImportHotspot(db *model.DB, router *model.Router, cmd model.Command, vals url.Values) (summary string, moreQueued bool) {
	now := model.NowISO()
	acc := router.AccountID

	// 1) Profils : name|rate-limit|shared-users|session-timeout;…
	profAdded := 0
	for _, e := range splitAgentList(vals.Get("profiles")) {
		name := agent.SanitizeName(e[0])
		if name == "" || name == "-" {
			continue
		}
		exists := false
		for i := range db.Profiles {
			if db.Profiles[i].AccountID == acc && strings.EqualFold(db.Profiles[i].Name, name) {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		rate := "1M/1M"
		if len(e) > 1 && strings.TrimSpace(e[1]) != "" {
			rate = strings.TrimSpace(e[1])
		}
		shared := 1
		if len(e) > 2 {
			if n, err := strconv.Atoi(e[2]); err == nil && n > 0 {
				shared = n
			}
		}
		st := 60
		if len(e) > 3 {
			if m := parseRosDurationMin(e[3]); m > 0 {
				st = m
			}
		}
		db.Profiles = append(db.Profiles, model.Profile{
			ID: model.NewID("p-"), AccountID: acc, Name: name, RateLimit: rate,
			SessionTimeoutMin: st, SharedUsers: shared, ValidityDays: 1,
			Price: 0, DataQuotaMb: 0, CreatedAt: now,
			ExpMode: "notify",
		})
		profAdded++
	}

	// 2) Utilisateurs : name|profile|disabled|comment|limit-bytes-total;…
	userAdded := 0
	knownUser := map[string]bool{}
	for i := range db.HotspotUsers {
		if db.HotspotUsers[i].AccountID == acc {
			knownUser[strings.ToLower(db.HotspotUsers[i].Username)] = true
		}
	}
	for _, e := range splitAgentList(vals.Get("users")) {
		name := agent.SanitizeName(e[0])
		if name == "" || name == "-" || knownUser[strings.ToLower(name)] {
			continue
		}
		profName := ""
		if len(e) > 1 {
			profName = agent.SanitizeName(e[1])
		}
		disabled := len(e) > 2 && (e[2] == "true" || e[2] == "yes")
		comment := ""
		if len(e) > 3 && e[3] != "-" {
			comment = e[3]
		}
		db.HotspotUsers = append(db.HotspotUsers, model.HotspotUser{
			ID: model.NewID("u-"), AccountID: acc, Kind: "regular",
			Username: name, ProfileName: profName,
			RouterID: router.ID, RouterName: router.Name,
			Status:    map[bool]string{true: "disabled", false: "active"}[disabled],
			Comment:   comment,
			CreatedAt: now,
		})
		knownUser[strings.ToLower(name)] = true
		userAdded++
	}

	// 3) Pagination : si le routeur a plus d'utilisateurs que ce chunk,
	//    file le chunk suivant (dédupliqué par queueCommandLocked).
	if total, err := strconv.Atoi(vals.Get("total")); err == nil && total > 0 {
		start := int(plPayloadInt(cmd.Payload, "start"))
		count := int(plPayloadInt(cmd.Payload, "count"))
		if count <= 0 {
			count = agent.ImportChunkSize
		}
		if start+count < total {
			queueCommandLocked(db, acc, router.ID, model.CmdImportHotspot,
				map[string]any{"start": start + count, "count": count})
			moreQueued = true
		}
	}

	summary = fmt.Sprintf("%d nouveau(x) profil(s), %d utilisateur(s) importé(s)", profAdded, userAdded)
	return summary, moreQueued
}

// plPayloadInt — lit un entier du payload d'une commande (JSON reloadé →
// float64, créé en mémoire → int).
func plPayloadInt(payload map[string]any, key string) int64 {
	if payload == nil {
		return 0
	}
	switch v := payload[key].(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return int64(n)
	default:
		return 0
	}
}
