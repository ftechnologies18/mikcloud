// media_health.go — N°183/N°185 : observabilité du stockage d'images Cloudflare R2.
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
// Cloudflare. Vingt heures AVANT la bascule Supabase — la migration était
// hors de cause, mais rien ne le disait dans la console.
//
// N°185 (24/09/2026) — la sonde N°183 interrogeait /user/tokens/verify, et
// la rotation réelle a révélé un FAUX NÉGATIF : les jetons R2 créés depuis
// la console R2 (« Manage R2 API Tokens », format cfat_…, qui délivrent
// AUSSI une paire d'identifiants S3) sont REFUSÉS par cet endpoint
// (« Invalid API Token », code 1000) alors qu'ils fonctionnent
// PARFAITEMENT sur l'API R2 elle-même (prouvé en direct : listage des
// compartiments + téléchargement de l'objet témoin, tous deux en 200 avec
// le jeton cfat_ refusé par verify). La sonde interroge désormais l'API R2
// DIRECTEMENT — listage des compartiments du compte — ce qui est de toute
// façon un test STRICTEMENT meilleur : il valide le jeton ET la portée R2
// (la permission exacte que le servage exige) en un seul appel, chose que
// verify ne pouvait pas faire. Un jeton mort y répond 401 « Authentication
// error » (code 10000 — constaté sur l'ancien jeton Render de l'incident) :
// la régression de l'incident reste attrapée.
//
// Produit : une SONDE volontairement minimaliste, exposée dans
// GET /api/admin/sync-status (bloc « media », purement additif — surveille.sh
// N°179 et les consommateurs existants ignorent les blocs inconnus) :
//
//	configured  les variables R2_ACCOUNT_ID + R2_API_TOKEN sont posées ;
//	tokenStatus verdict du sondage R2 (listage des compartiments) —
//	            "valid"   jeton accepté par l'API R2 (le servage peut passer) ;
//	            "invalid" jeton refusé (401/403 : expiré, révoqué ou sans
//	                      portée R2) → rotation nécessaire (RUNBOOK-SECRETS §2.7) ;
//	            "unknown" Cloudflare injoignable, timeout ou réponse ambiguë
//	                      (5xx, 200 sans succès) — sonde sans conclusion, on
//	                      ne crie pas au loup ;
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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// r2ProbeTTL — durée de validité du verdict en cache.
const r2ProbeTTL = 5 * time.Minute

// r2ProbeTimeout — borne stricte de l'appel Cloudflare (la carte Santé ne
// doit pas accrocher sur une lenteur externe).
const r2ProbeTimeout = 4 * time.Second

// r2BucketsListFmt — listage des compartiments du compte : LE sondage de
// référence (N°185). Un jeton qui passe cet appel possède exactement ce que
// le servage exige (authentification + portée R2 du compte) ; un jeton mort
// y reçoit 401 « Authentication error » (code 10000 — constaté sur l'ancien
// jeton Render de l'incident). Ne JAMAIS remplacer par /user/tokens/verify :
// cet endpoint refuse les jetons R2 de type cfat_ (console R2) alors qu'ils
// fonctionnent parfaitement ici — faux négatif prouvé le 24/09/2026.
const r2BucketsListFmt = "https://api.cloudflare.com/client/v4/accounts/%s/r2/buckets"

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

// r2ProbeCall — appel réseau réel (injectable en tests : remplacé par un
// compteur/stub, cf. media_health_test.go). Renvoie le code HTTP et le corps
// de la réponse Cloudflare (borné 64 Ko), ou l'erreur réseau.
var r2ProbeCall = func(account, token string) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r2ProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf(r2BucketsListFmt, url.PathEscape(account)), nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := mediaHTTPClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
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
	if code, body, err := r2ProbeCall(mc.account, mc.token); err == nil {
		var v struct {
			Success bool `json:"success"`
		}
		switch {
		case code == http.StatusOK && json.Unmarshal(body, &v) == nil && v.Success:
			status = "valid"
		case code == http.StatusUnauthorized || code == http.StatusForbidden:
			// Refus explicite de Cloudflare : jeton mort, révoqué ou sans
			// portée R2 — les trois exigent la même action opérateur
			// (rotation, RUNBOOK-SECRETS §2.7).
			status = "invalid"
		}
		// Tout le reste (5xx, corps illisible, 200 sans succès) : réponse
		// ambiguë — la sonde ne conclut pas, "unknown".
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
