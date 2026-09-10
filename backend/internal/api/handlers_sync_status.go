// handlers_sync_status.go — N°71 : santé de la persistance + agents.
//
// GET /api/admin/sync-status (console plateforme, onglet Maintenance) est un
// diagnostic READ-ONLY de deux flux silencieux :
//   - la synchro différentielle FNV-1a mémoire → PostgreSQL/Neon (pg.go) :
//     tentatives/succès/échecs, durée et volumétrie du dernier delta, dernier
//     contact confirmé Neon, dérive éventuelle lignes mémoire/répliquées ;
//   - les agents routeur (HTTP-poll 45 s) : fraîcheur des check-ins, file de
//     commandes (en attente / envoyées / zombies > staleSentLimit), conflits
//     d'identité S6.
//
// Aucune écriture : l'endpoint ne peut pas perturber le service qu'il observe.
// Les compteurs de synchro vivent dans le package store (syncstats.go) —
// zéro métrique n'existait avant N°71 (un échec de synchro n'était visible
// que d'une ligne de journal Render).
package api

import (
	"net/http"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// agentSnapshot — photographie VOLATILE des agents routeurs (calculée sous
// verrou par handleSyncStatus ; les constantes de fraîcheur OnlineWindow et
// staleSentLimit appartiennent à ce package, agent_handlers.go).
type agentSnapshot struct {
	Routers          int    `json:"routers"`               // tous modes confondus
	RoutersAgent     int    `json:"routersAgent"`          // mode agent (HTTP-poll sortant)
	RoutersReal      int    `json:"routersReal"`           // mode réel (API binaire 8728)
	RoutersSimulated int    `json:"routersSimulated"`      // mode simulé
	RoutersOnline    int    `json:"routersOnline"`         // agents avec check-in < OnlineWindow
	RoutersConflict  int    `json:"routersConflict"`       // S6 : conflit d'identité actif
	CommandsQueued   int    `json:"commandsQueued"`        // en file, pas encore servies
	CommandsSent     int    `json:"commandsSent"`          // servies, résultat pas encore revenu
	CommandsStale    int    `json:"commandsStale"`         // servies depuis > staleSentLimit (zombies)
	LastCheckIn      string `json:"lastCheckIn,omitempty"` // LastSeen le plus récent (RFC3339)
}

// agentSnapshotLocked — parcourt routeurs et commandes sous le verrou global.
// Un agent « en ligne » = un check-in plus récent que OnlineWindow : la
// vérité est la FRAÎCHEUR du LastSeen (touchAgent pose « online » au passage,
// mais rien ne le repasse à « offline » si l'agent se tait — d'où la fenêtre).
func agentSnapshotLocked(db *model.DB, now time.Time) agentSnapshot {
	snap := agentSnapshot{}
	var lastSeen time.Time
	for i := range db.Routers {
		r := &db.Routers[i]
		snap.Routers++
		switch r.Mode {
		case "agent":
			snap.RoutersAgent++
		case "real":
			snap.RoutersReal++
		default:
			snap.RoutersSimulated++
		}
		if r.IdentityConflict {
			snap.RoutersConflict++
		}
		if seen, err := time.Parse(time.RFC3339, r.LastSeen); err == nil {
			// N°75 — fenêtre élargie au régime de veille adaptative :
			// 3 × le pas du scheduler du routeur (un agent endormi à
			// 180 s reste « en ligne » ; le plafond historique de 3 min
			// s'applique aux routeurs actifs à 45 s).
			if r.Mode == "agent" && now.Sub(seen) < agentOnlineWindow(r) {
				snap.RoutersOnline++
			}
			if seen.After(lastSeen) {
				lastSeen = seen
			}
		}
	}
	for i := range db.Commands {
		c := &db.Commands[i]
		switch c.Status {
		case "queued":
			snap.CommandsQueued++
		case "sent":
			snap.CommandsSent++
			if sent, err := time.Parse(time.RFC3339, c.SentAt); err == nil && now.Sub(sent) > staleSentLimit {
				snap.CommandsStale++
			}
		}
	}
	if !lastSeen.IsZero() {
		snap.LastCheckIn = lastSeen.UTC().Format(time.RFC3339)
	}
	return snap
}

// handleSyncStatus — GET /api/admin/sync-status : double garde (rang 3 via
// requireRole à l'enregistrement, puis rôle plateforme — cf. overview).
func (a *API) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}
	// Photographie persistance : verrou pris/rendu par Store.SyncHealth.
	h := a.store.SyncHealth()
	// Photographie agents : verrou global explicite (style overview), les
	// constantes de fraîcheur vivent dans ce package.
	a.store.Lock()
	agents := agentSnapshotLocked(a.store.Data(), time.Now().UTC())
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"mode":      h.Mode,                              // postgresql (production) | json (développement)
		"sync":      h.Sync,                              // compteurs différentiels — null en mode JSON
		"neon":      h.Neon,                              // contact + keep-alive — null en mode JSON
		"tables":    h.Tables,                            // lignes mémoire vs répliquées par table
		"agents":    agents,                              // fraîcheur check-ins + file de commandes
		"bandwidth": a.egress.snapshot(time.Now().UTC()), // N°72 — octets sortis du jour, par catégorie (borne basse : corps uniquement)
	})
}
