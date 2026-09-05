package api

// N°26 — Lots éteints : un lot dont TOUS les tickets sont expirés disparaît
// du système, automatiquement (demande gérant : « un lot dont tous les
// tickets ont expiré doit disparaître »).
//
// Ce que la disparition couvre :
//   - les TICKETS du lot (HotspotUsers expirés, deteneur direct) ;
//   - la LIGNE DE LOT (db.Batches) — plus de lot « Expiré » zombie dans
//     l'onglet Lots, même sous le filtre « Tous » ;
//   - les SESSIONS des tickets supprimés (abandonnées, comme le nettoyage
//     manuel F5) ;
//   - le ROUTEUR : commande user_remove par paquets de 50 pour chaque
//     routeur AGENT (parité handleUsersCleanup) — RouterOS reste propre.
//
// Garde-fous (délibérés) :
//   - au moins 1 ticket : les lots déjà vides relèvent de la purge manuelle
//     (trace « Purgé » conservée) ;
//   - AUCUN ticket revendeur (N°23/W1) : le stock confié reste la trace de
//     ce qui a été remis au revendeur — même expiré, il se retrouve via la
//     reprise N°23 puis se supprime individuellement ;
//   - TOMBSTONES de purge posés pour chaque username supprimé : les tickets
//     restent physiquement sur le routeur réel (disabled via enforceExpired)
//     et, sans tombstone, le read_state agent (< 45 s) les réimporterait en
//     utilisateurs réguliers fantômes (résurrection — cf. audit Task 8/9).
//
// Point d'appel : fin de enforceExpired — le passage commun de TOUTES les
// entrées du système (listes console, dashboard, ops, check-in agent 45 s),
// sous verrou, avec Save par l'appelant (persistance des tombstones et de la
// suppression en Neon au même cycle).

import (
	"fmt"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// sweepDeadBatches — supprime les lots dont tous les tickets sont expirés.
// À appeler sous verrou, APRÈS applyExpiry (statuts à jour) et APRÈS
// enforceExpired (les nouveaux expirés sont déjà marqués côté routeur).
// Retourne le nombre de lots supprimés (tests/journal).
func sweepDeadBatches(db *model.DB) int {
	now := time.Now().UTC()

	// 1. Index des tickets par lot (vouchers uniquement).
	type lotState struct {
		accID    string
		total    int
		expired  int
		reseller int      // tickets confiés — N°23/W1 : bloquent la disparition
		ids      []string // IDs de tickets à supprimer
		names    []string // usernames bruts (routeur + sessions)
		lowers   []string // usernames minuscules (tombstones)
		routers  map[string]bool
	}
	lots := map[string]*lotState{}
	for i := range db.HotspotUsers {
		u := &db.HotspotUsers[i]
		if u.Kind != "voucher" || u.BatchID == "" {
			continue
		}
		st := lots[u.BatchID]
		if st == nil {
			st = &lotState{accID: u.AccountID, routers: map[string]bool{}}
			lots[u.BatchID] = st
		}
		st.total++
		if u.ResellerID != "" {
			st.reseller++
		}
		if u.Status == "expired" {
			st.expired++
			st.ids = append(st.ids, u.ID)
			st.names = append(st.names, agent.SanitizeName(u.Username))
			st.lowers = append(st.lowers, strings.ToLower(u.Username))
			st.routers[u.RouterID] = true
		}
	}
	if len(lots) == 0 {
		return 0
	}

	// 2. Lots éteints : ≥ 1 ticket, TOUS expirés, aucun ticket revendeur.
	dead := map[string]*lotState{}
	for id, st := range lots {
		if st.total > 0 && st.total == st.expired && st.reseller == 0 {
			dead[id] = st
		}
	}
	if len(dead) == 0 {
		return 0
	}

	// 3. Suppression des tickets du lot (par ID).
	deadIDs := map[string]bool{}
	for _, st := range dead {
		for _, id := range st.ids {
			deadIDs[id] = true
		}
	}
	keptUsers := db.HotspotUsers[:0]
	for _, u := range db.HotspotUsers {
		if deadIDs[u.ID] {
			continue
		}
		keptUsers = append(keptUsers, u)
	}
	db.HotspotUsers = keptUsers

	// 4. Suppression des lignes de lot correspondantes.
	keptBatches := db.Batches[:0]
	for _, b := range db.Batches {
		if _, ok := dead[b.ID]; ok {
			continue
		}
		keptBatches = append(keptBatches, b)
	}
	db.Batches = keptBatches

	// 5. Tombstones anti-résurrection (usernames minuscules — clés attendues).
	tombs := map[string]map[string]bool{}
	for _, st := range dead {
		set := tombs[st.accID]
		if set == nil {
			set = map[string]bool{}
			tombs[st.accID] = set
		}
		for _, low := range st.lowers {
			set[low] = true
		}
	}
	writePurgeTombstones(db, tombs, now)

	// 6. Sessions des tickets supprimés : abandonnées (parité F5).
	deadNames := map[string]bool{}
	for _, st := range dead {
		for _, name := range st.names {
			deadNames[name] = true
		}
	}
	keptSessions := db.Sessions[:0]
	for _, s := range db.Sessions {
		if deadNames[s.Username] {
			continue
		}
		keptSessions = append(keptSessions, s)
	}
	db.Sessions = keptSessions

	// 7. Nettoyage RouterOS : user_remove par paquets de 50, par routeur AGENT.
	for _, st := range dead {
		for routerID := range st.routers {
			rr := findRouterScoped(db, routerID, st.accID)
			if rr == nil || rr.Mode != "agent" {
				continue // simulé/réel passerelle : le cloud suffit
			}
			for start := 0; start < len(st.names); start += 50 {
				end := start + 50
				if end > len(st.names) {
					end = len(st.names)
				}
				queueCommandLocked(db, rr.AccountID, rr.ID, model.CmdUserRemove, map[string]any{
					"names": st.names[start:end],
				})
			}
		}
	}

	// 8. Journal d'activité par compte (trace de la disparition automatique).
	perAcc := map[string]int{}
	for _, st := range dead {
		perAcc[st.accID]++
	}
	for accID, n := range perAcc {
		db.Activity = append([]model.Activity{{
			ID:        model.NewID("act-"),
			AccountID: accID,
			Type:      "voucher",
			Message:   fmt.Sprintf("Lots éteints supprimés automatiquement : %d lot(s) dont tous les tickets ont expiré (tombstones anti-résurrection posés)", n),
			At:        model.NowISO(),
		}}, db.Activity...)
		if len(db.Activity) > 500 {
			db.Activity = db.Activity[:500]
		}
	}
	return len(dead)
}
