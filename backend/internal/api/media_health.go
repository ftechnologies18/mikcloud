// media_health.go — N°183 : observabilité du stockage d'images Cloudflare R2.
//
// Contexte incident (20/09/2026) : les images du portail captif (slides du
// carrousel, bannières téléversées via POST /api/media) vivent dans un
// compartiment R2 et sont SERVIES par CE backend via l'API REST Cloudflare
// (jeton R2_API_TOKEN, handlers_media.go N°53). Le jeton est mort un soir de
// septembre (expiré/révoqué côté Cloudflare) : chaque GET /api/media/{key}
// répondait 502 et les slides des portails concernés disparaissaient — SANS
// AUCUN signal côté console (les images sont « optionnelles » côté client :
// onerror les retire discrètement). Le diagnostic n'a été possible qu'en
// corrélant les logs Render ([media] get … statut 401 en rafale) avec l'API
// Cloudflare (token verify → Invalid). Vingt heures AVANT la bascule Supabase
// — la migration était hors de cause, mais rien ne le disait dans la console.
//
// Produit : une SONDE volontairement minimaliste, exposée dans
// GET /api/admin/sync-status (bloc « media », purement additif — surveille.sh
// N°179 et les consommateurs existants ignorent les blocs inconnus) :
//
//	configured  les variables R2_ACCOUNT_ID + R2_API_TOKEN sont posées ;
//	tokenStatus verdict de l'API Cloudflare /user/tokens/verify —
//	            "valid"   jeton actif (upload et lecture peuvent passer) ;
//	            "invalid" jeton refusé (expiré ou révoqué) → rotation
//	                      nécessaire (RUNBOOK-SECRETS §2.7) ;
//	            "unknown" Cloudflare injoignable/timeout — sonde sans
//	                      conclusion, on ne crie pas au loup ;
//	bucket      compartiment lu dans la configuration (contexte opérateur) ;
//	checkedAt   horodatage RFC3339 du verdict.
//
// La sonde est en cache mémoire 5 minutes (au plus un appel Cloudflare par
// fenêtre, même avec une console qui poll toutes les 15 s), timeout borné
// 4 s, AUCUNE écriture — elle ne peut pas perturber le canal qu'elle observe.
// Un concurrent qui trouverait le cache périmé refait simplement l'appel : le
// verrou n'est JAMAIS tenu pendant le réseau.
package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

// r2ProbeTTL — durée de validité du verdict en cache.
const r2ProbeTTL = 5 * time.Minute

// r2ProbeTimeout — borne stricte de l'appel Cloudflare (la carte Santé ne
// doit pas accrocher sur une lenteur externe).
const r2ProbeTimeout = 4 * time.Second

// r2VerifyEndpoint — vérification de jeton côté Cloudflare. La réponse ne
// divulgue RIEN au-delà du statut du jeton fourni (pas de listing, pas
// d'identifiants) — c'est l'endpoint de diagnostic officiel de Cloudflare.
const r2VerifyEndpoint = "https://api.cloudflare.com/client/v4/user/tokens/verify"

// mediaHealth — photographie du canal média (sérialisée telle quelle dans le
// bloc « media » de sync-status).
type mediaHealth struct {
	Configured  bool   `json:"configured"`
	Bucket      string `json:"bucket,omitempty"`
	TokenStatus string `json:"tokenStatus"`         // valid | invalid | unknown | "" (non configuré)
	CheckedAt   string `json:"checkedAt,omitempty"` // RFC3339
}

// r2ProbeState — cache mémoire monoposte.
type r2ProbeState struct {
	health mediaHealth
	at     time.Time
}

var (
	r2ProbeMu    sync.Mutex
	r2ProbeCache *r2ProbeState
)

// r2VerifyCall — appel réseau réel (injectable en tests : remplacé par un
// compteur/stub, cf. media_health_test.go). Renvoie le corps de la réponse
// Cloudflare (borné 64 Ko) ou l'erreur réseau.
var r2VerifyCall = func(token string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r2ProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r2VerifyEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := mediaHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(io.LimitReader(resp.Body, 64<<10))
}

// r2MediaHealthSnapshot — verdict du canal R2, cache 5 min. Ne renvoie
// JAMAIS d'erreur : une sonde indisponible vaut "unknown" (la carte Santé
// reste servie), une absence de configuration vaut un bloc explicite —
// c'est précisément ce qui manquait pendant l'incident (le 502 des images
// n'était visible nulle part dans la console).
func r2MediaHealthSnapshot() mediaHealth {
	mc := mediaConfig()
	if mc == nil {
		// Non configuré : verdict stable, aucun appel réseau.
		return mediaHealth{Configured: false, TokenStatus: ""}
	}
	now := time.Now().UTC()

	r2ProbeMu.Lock()
	if r2ProbeCache != nil && now.Sub(r2ProbeCache.at) < r2ProbeTTL {
		h := r2ProbeCache.health
		r2ProbeMu.Unlock()
		return h
	}
	r2ProbeMu.Unlock()

	status := "unknown"
	if body, err := r2VerifyCall(mc.token); err == nil {
		var v struct {
			Success bool `json:"success"`
		}
		if json.Unmarshal(body, &v) == nil {
			if v.Success {
				status = "valid"
			} else {
				status = "invalid"
			}
		}
	}
	h := mediaHealth{
		Configured:  true,
		Bucket:      mc.bucket,
		TokenStatus: status,
		CheckedAt:   now.Format(time.RFC3339),
	}
	r2ProbeMu.Lock()
	r2ProbeCache = &r2ProbeState{health: h, at: now}
	r2ProbeMu.Unlock()
	return h
}
