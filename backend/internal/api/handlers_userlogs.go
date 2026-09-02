// Package api — journal utilisateurs (F3, audit Mikhmon) : liste paginée et
// export CSV (séparateur « ; », BOM UTF-8) des logs login/logout/expire/kick.
package api

import (
	"fmt"
	"mikcloud/hotspot-api/internal/model"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// F3 — Journal utilisateurs (login / logout / expire / kick)
// ---------------------------------------------------------------------------

// filterUserLogs — filtres partagés liste JSON + export CSV (sous verrou).
func filterUserLogs(db *model.DB, acc string, q url.Values) []model.UserLog {
	search := strings.ToLower(strings.TrimSpace(q.Get("search")))
	routerID := strings.TrimSpace(q.Get("routerId"))
	action := strings.TrimSpace(q.Get("action"))

	logs := []model.UserLog{}
	for _, l := range db.UserLogs {
		if l.AccountID != acc {
			continue
		}
		if action != "" && l.Action != action {
			continue
		}
		if routerID != "" && l.RouterID != routerID {
			continue
		}
		if search != "" {
			hay := strings.ToLower(l.Username + " " + l.IP)
			if !strings.Contains(hay, search) {
				continue
			}
		}
		logs = append(logs, l)
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].At > logs[j].At })
	return logs
}

func (a *API) handleUserLogsList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	page := queryInt(r, "page", 1, 1, 1_000_000)
	pageSize := queryInt(r, "pageSize", 20, 1, 100)

	a.store.Lock()
	logs := filterUserLogs(a.store.Data(), acc, r.URL.Query())
	a.store.Unlock()

	total := len(logs)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":     logs[start:end],
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// handleUserLogsExport — CSV du journal utilisateurs (séparateur « ; », BOM).
func (a *API) handleUserLogsExport(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	logs := filterUserLogs(a.store.Data(), acc, r.URL.Query())
	a.store.Unlock()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"mikcloud-journal-utilisateurs.csv\"")
	// BOM UTF-8 : Excel reconnaît l'encodage.
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	_, _ = w.Write([]byte("Date;Utilisateur;Action;Routeur;IP;MAC\r\n"))
	for _, l := range logs {
		_, _ = w.Write([]byte(fmt.Sprintf("%s;%s;%s;%s;%s;%s\r\n",
			csvField(l.At), csvField(l.Username), csvField(l.Action),
			csvField(l.RouterName), csvField(l.IP), csvField(l.MAC))))
	}
}
