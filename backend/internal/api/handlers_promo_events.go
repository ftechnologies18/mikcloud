// handlers_promo_events.go — N°56 : analytics du portail captif en mode
// hospitalité (impressions / clics par ligne de vitrine).
//
// Principe commercial : le gérant d'un hôtel/maquis/salon n'achète pas des
// « statistiques serveur », il veut pouvoir dire « mon menu a été vu 480 fois
// cette semaine ». Deux endpoints suffisent :
//
//   - POST /api/portal/track  PUBLIC (pré-auth du hotspot, cf. middleware) —
//     la page du portail y dépose ses événements au chargement des cartes
//     (impression) et à l'ouverture de leur lien (click). Résolution du
//     compte par la clé publique du portail (Tenant.PortalKey — non secrète
//     par design : elle ne donne AUCUN droit de lecture).
//   - GET  /api/promos/stats  console gérant (JWT, manager et plus) —
//     agrégats par promo (jour / 7 jours glissants / total) + totaux.
//
// Garde-fous anti-gonflement (dans l'ordre) :
//  1. DÉDUPLICATION : l'ID de l'événement est un hash déterministe de
//     (compte, promo, type, appareil, jour UTC) — un même appareil ne compte
//     qu'UNE fois par promo et par jour ; re-POSTer est un no-op.
//  2. EXISTENCE : seuls les IDs de promo réellement présents dans la vitrine
//     du compte sont acceptés (pas d'invention d'IDs).
//  3. QUOTA IP : limiter dédié (NAT-friendly, cf. N°50) sur le endpoint public.
//  4. PLAFOND : 3 000 événements/compte/jour, rétention 90 jours, plafond
//     mémoire (prunePromoEvents — la diff syncTable répercute les suppressions
//     en Neon au Save suivant).
package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// promoEventKind — types d'événements admis (le reste est ignoré, 204).
const (
	promoKindImpression = "impression"
	promoKindClick      = "click"
)

// Bornes du journal analytics (voir prunePromoEvents).
const (
	promoRetentionDays = 90     // fenêtre de rétention (les stats vivent dans cette fenêtre)
	promoEventsMaxRows = 12_000 // plafond mémoire du journal (tous comptes)
	promoDayCap        = 3_000  // plafond d'événements par compte et par jour (anti-spam)
)

// promoKeyRe — Tenant.PortalKey : 16 hex minuscules (model.NewID-like généré
// par ensureSettings). Toute autre forme est ignorée sans erreur (204) : le
// endpoint public ne révèle rien, même à un sondage automatisé.
var promoKeyRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

// promoIDRe — id de promo bien formé : préfixe + hex (id aléatoire « p… »
// posé par encodePromos, ou id déterministe « h… » des lignes héritées).
var promoIDRe = regexp.MustCompile(`^[ph][0-9a-f]{7,12}$`)

// promoIDValid — true si l'id respecte le format attendu.
func promoIDValid(id string) bool { return promoIDRe.MatchString(id) }

// promoFallbackID — id DÉTERMINISTE des lignes de vitrine héritées d'avant
// N°56 (jamais ré-enregistrées depuis) : dérivé du contenu, il est stable
// d'une page à l'autre et d'un déploiement à l'autre. Préfixe « h » pour le
// distinguer des ids aléatoires « p » posés à l'enregistrement console.
// Limite assumée : deux promos au contenu strictement identique partagent
// leur compteur (et éditer le contenu d'une ligne héritée change son id —
// un enregistrement console lui donne ensuite un id stable).
func promoFallbackID(title, desc, img, price string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{title, desc, img, price}, "\x1f")))
	return "h" + hex.EncodeToString(sum[:4])
}

// promoEventID — ID déterministe de l'événement (garde-fou n°1) : le couple
// (appareil, jour) NE PEUT produire qu'une seule ligne par promo et par type.
func promoEventID(acc, promoID, kind, clientKey, day string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{acc, promoID, kind, clientKey, day}, "\x1f")))
	return "e" + hex.EncodeToString(sum[:10])
}

// promoIDsOf — l'ensemble des ids de promo ADMIS pour ce compte : ids posés
// par la console, à défaut ids déterministes (cf. portalHospitality, même
// règle — les deux côtés doivent désigner les mêmes lignes).
func promoIDsOf(t model.Tenant) map[string]struct{} {
	ids := map[string]struct{}{}
	if t.PortalPromos == "" {
		return ids
	}
	var raw []struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		Desc       string `json:"desc"`
		ImageURL   string `json:"imageUrl"`
		PriceLabel string `json:"priceLabel"`
	}
	if json.Unmarshal([]byte(t.PortalPromos), &raw) != nil {
		return ids
	}
	for _, it := range raw {
		id := it.ID
		if id == "" {
			id = promoFallbackID(it.Title, it.Desc, it.ImageURL, it.PriceLabel)
		}
		ids[id] = struct{}{}
	}
	return ids
}

// handlePromoTrack — POST /api/portal/track (PUBLIC, cf. authMiddleware) :
// dépôt d'un événement impression/click. Réponse 204 dans TOUS les cas
// (succès, doublon, clé inconnue, promo absente, quota) : le portail n'a
// rien à faire d'une erreur, et l'endpoint ne doit fournir aucune oracle à
// un sondage.
func (a *API) handlePromoTrack(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key       string `json:"key"`
		PromoID   string `json:"promoId"`
		Kind      string `json:"kind"`
		ClientKey string `json:"clientKey"`
	}
	if err := decodeBody(r, &req); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	if kind != promoKindImpression && kind != promoKindClick {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	key := strings.ToLower(strings.TrimSpace(req.Key))
	if !promoKeyRe.MatchString(key) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	promoID := strings.ToLower(strings.TrimSpace(req.PromoID))
	if !promoIDValid(promoID) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Garde-fou n°3 — quota IP NAT-friendly (tout l'établissement partage une
	// IP publique ; un invité honnête émet ≤ 12 événements par page). Le refus
	// reste un 204 : pas d'oracle, pas de Retry-After exploitable.
	if ok, _ := a.portalTrack.allow(clientIP(r)); !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Appareil : MAC normalisée du portail (clientMac — pattern N°50) si
	// fournie, sinon l'IP du client (dernier recours : derrière le NAT du
	// hotspot, tous les invités partagent la même IP — la dédup par jour
	// reste correcte, juste plus grossière).
	clientKey := normalizeJoinMac(req.ClientKey)
	if clientKey == "" {
		clientKey = "ip:" + clientIP(r)
	}
	day := time.Now().UTC().Format("2006-01-02")

	a.store.Lock()
	db := a.store.Data()
	// Résolution du compte par la clé publique du portail (garde-fou n°2 :
	// l'événement n'est compté que si la promo existe dans la vitrine).
	acc := ""
	for accID, s := range db.SettingsByAccount {
		if s.Tenant.PortalKey == key {
			acc = accID
			break
		}
	}
	if acc == "" {
		a.store.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	settings := ensureSettings(db, acc)
	if _, known := promoIDsOf(settings.Tenant)[promoID]; !known {
		a.store.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	id := promoEventID(acc, promoID, kind, clientKey, day)
	exists, todayCount := false, 0
	for i := range db.PromoEvents {
		ev := &db.PromoEvents[i]
		if ev.AccountID != acc {
			continue
		}
		if ev.Day == day {
			todayCount++
		}
		if ev.ID == id {
			exists = true
		}
	}
	// Garde-fou n°4 — plafond journalier par compte (tous les compteurs du
	// jour sont déjà sous plafond si on insère encore ; sinon on drop : les
	// impressions réelles continuent de compter demain).
	if exists || todayCount >= promoDayCap {
		a.store.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	db.PromoEvents = append(db.PromoEvents, model.PromoEvent{
		ID:        id,
		AccountID: acc,
		PromoID:   promoID,
		Kind:      kind,
		ClientKey: clientKey,
		Day:       day,
		CreatedAt: model.NowISO(),
	})
	prunePromoEvents(db)
	a.store.Save()
	a.store.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// promoStatsBucket — compteurs d'une ligne de vitrine (ou des totaux).
type promoStatsBucket struct {
	Day   int `json:"day"`
	Week  int `json:"week"`
	Total int `json:"total"`
}

// handlePromoStats — GET /api/promos/stats (console, manager et plus) :
// agrégats analytics par promo + totaux, pour vendre « votre menu vu 480
// fois cette semaine ». Tout est lu sous un seul passage du journal (mémoire)
// : aucune requête Neon sur le chemin chaud du tableau de bord.
func (a *API) handlePromoStats(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	a.store.Lock()
	db := a.store.Data()
	settings := ensureSettings(db, acc)

	// Ligne de vitrine courante (ordre du JSON du tenant — l'ordre d'affichage
	// du portail), avec les mêmes ids que le portail (posé ou déterministe).
	type promoRow struct {
		ID    string           `json:"id"`
		Title string           `json:"title"`
		Impr  promoStatsBucket `json:"impressions"`
		Click promoStatsBucket `json:"clicks"`
	}
	var rows []promoRow
	rowByID := map[string]*promoRow{}
	{
		var raw []struct {
			ID         string `json:"id"`
			Title      string `json:"title"`
			Desc       string `json:"desc"`
			ImageURL   string `json:"imageUrl"`
			PriceLabel string `json:"priceLabel"`
		}
		if settings.Tenant.PortalPromos != "" && json.Unmarshal([]byte(settings.Tenant.PortalPromos), &raw) == nil {
			for _, it := range raw {
				id := it.ID
				if id == "" {
					id = promoFallbackID(it.Title, it.Desc, it.ImageURL, it.PriceLabel)
				}
				if _, dup := rowByID[id]; dup {
					continue
				}
				row := promoRow{ID: id, Title: it.Title}
				rows = append(rows, row)
				rowByID[id] = &rows[len(rows)-1]
			}
		}
	}

	today := time.Now().UTC().Format("2006-01-02")
	weekStart := time.Now().UTC().AddDate(0, 0, -6).Format("2006-01-02") // 7 jours glissants, aujourd'hui inclus

	// Compteurs par ligne : index dans rows (des pointeurs sur une tranche
	// qui grandit deviendraient obsolètes après ré-allocation — on agrège
	// par index, jamais par adresse).
	type counters struct {
		impr, click promoStatsBucket
	}
	perRow := make(map[string]*counters, len(rows))
	for i := range rows {
		perRow[rows[i].ID] = &counters{}
	}
	totals := map[string]*promoStatsBucket{
		promoKindImpression: {},
		promoKindClick:      {},
	}
	for i := range db.PromoEvents {
		ev := &db.PromoEvents[i]
		if ev.AccountID != acc {
			continue
		}
		cnt, ok := perRow[ev.PromoID]
		if !ok {
			continue // promo retirée ou id orphelin (contenu édité) : hors périmètre
		}
		var rowBucket *promoStatsBucket
		var totalBucket *promoStatsBucket
		if ev.Kind == promoKindClick {
			rowBucket, totalBucket = &cnt.click, totals[promoKindClick]
		} else if ev.Kind == promoKindImpression {
			rowBucket, totalBucket = &cnt.impr, totals[promoKindImpression]
		} else {
			continue
		}
		rowBucket.Total++
		totalBucket.Total++
		if ev.Day >= weekStart {
			rowBucket.Week++
			totalBucket.Week++
			if ev.Day == today {
				rowBucket.Day++
				totalBucket.Day++
			}
		}
	}
	for i := range rows {
		cnt := perRow[rows[i].ID]
		rows[i].Impr = cnt.impr
		rows[i].Click = cnt.click
	}
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"today":    today,
		"weekFrom": weekStart,
		"promos":   rows,
		"totals": map[string]promoStatsBucket{
			promoKindImpression: *totals[promoKindImpression],
			promoKindClick:      *totals[promoKindClick],
		},
	})
}

// prunePromoEvents — rétention du journal (garde-fou n°4, mémoire) : les
// événements hors fenêtre 90 jours sortent de l'état, puis le plafond dur
// 12 000 lignes évince les plus anciens (tri par Day). La diff syncTable
// supprime les lignes correspondantes en Neon au Save suivant. O(n) à chaque
// insertion : le journal est borné, le coût reste négligeable.
func prunePromoEvents(db *model.DB) {
	if len(db.PromoEvents) == 0 {
		return
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -promoRetentionDays).Format("2006-01-02")
	kept := db.PromoEvents[:0]
	for _, ev := range db.PromoEvents {
		if ev.Day >= cutoff {
			kept = append(kept, ev)
		}
	}
	db.PromoEvents = kept
	if len(db.PromoEvents) > promoEventsMaxRows {
		sort.Slice(db.PromoEvents, func(i, j int) bool { return db.PromoEvents[i].Day < db.PromoEvents[j].Day })
		db.PromoEvents = append(db.PromoEvents[:0], db.PromoEvents[len(db.PromoEvents)-promoEventsMaxRows:]...)
	}
}
