package api

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// N°53 — Stockage média Cloudflare R2 (images du gérant : bannière du portail
// N°45, promos hospitalité à venir — mode hospitalité N°54+).
//
// Canal retenu : l'API REST Cloudflare (Bearer R2_API_TOKEN), PAS l'API S3 :
//   - zéro SDK Go (le go.mod reste minimal : pgx + crypto),
//   - un seul secret à configurer (le jeton API) — pas de paire S3
//     access-key/secret à gérer,
//   - les volumes sont minuscules (images ≤ 2 Mo) : PUT/GET objet par clé
//     suffisent largement.
//
// Env attendues (Render) :
//   R2_ACCOUNT_ID — identifiant de compte Cloudflare
//   R2_API_TOKEN  — jeton API avec permission R2:Edit sur le compartiment
//   R2_BUCKET     — compartiment (défaut mikcloud-media)
// Non configuré ⇒ upload → 503 media_unconfigured (le frontend retombe sur la
// data URL intégrée ≤ 500 Ko, contrat N°45 inchangé) ; lecture → 404.
//
// Walled-garden (portail captif) : les images sont servies par CE backend
// (même hôte que apiBase — mikcloud.onrender.com), déjà joignable pré-auth par
// le claim et le fetch live (N°47/48). Aucune entrée walled-garden nouvelle,
// aucun domaine externe à autoriser.
// ---------------------------------------------------------------------------

const mediaMaxBytes = 2 << 20 // 2 Mo — bannières/promos compressées

// mediaHTTPClient — client dédié : timeout serré (Render ne doit pas accrocher
// sur une lenteur Cloudflare), pas de cookies, TLS par défaut.
var mediaHTTPClient = &http.Client{Timeout: 20 * time.Second}

// mediaAllowedTypes — types MIME acceptés, SNIFFÉS dans le contenu (512
// premiers octets) et non dans le Content-Type déclaré : un exécutable
// renommé .jpg est rejeté. La valeur est l'extension canonique de stockage.
var mediaAllowedTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// mediaExtToType — inverse strict (lecture) : on ne sert QUE ce que l'upload
// a pu déposer — le Content-Type de réponse est déduit de l'extension de la
// clé, jamais des en-têtes du stockage.
var mediaExtToType = map[string]string{
	".jpg":  "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
	".gif":  "image/gif",
}

// mediaKeyRe — clés DÉLIVRÉES par handleMediaUpload uniquement : préfixe
// media/, compte/année/racine hexadécimale 128 bits, extension whitelistée.
// Toute autre clé est refusée en lecture (pas de traversal, pas d'objet
// étranger servi par l'endpoint public).
var mediaKeyRe = regexp.MustCompile(`^media/[A-Za-z0-9._-]+/[0-9]{4}/[a-f0-9]{32}\.(jpg|png|webp|gif)$`)

// mediaClient — paramètres du canal REST Cloudflare.
type mediaClient struct {
	account string
	token   string
	bucket  string
}

// mediaConfig — lit la configuration R2 ; nil si incomplète (fonctionnalité
// hors service mais API propre : le reste du produit ne dépend pas de R2).
func mediaConfig() *mediaClient {
	account := strings.TrimSpace(getEnv("R2_ACCOUNT_ID"))
	token := strings.TrimSpace(getEnv("R2_API_TOKEN"))
	bucket := strings.TrimSpace(getEnv("R2_BUCKET"))
	if bucket == "" {
		bucket = "mikcloud-media"
	}
	if account == "" || token == "" {
		return nil
	}
	return &mediaClient{account: account, token: token, bucket: bucket}
}

// ---------------------------------------------------------------------------
// N°205 — sortie des médias du tuyau Render (bande passante facturable).
//
// Constat facturation (septembre 2026) : Render Hobby 2026 n'inclut que
// 5 Go de bande passante sortante par mois (0,15 $/Go au-delà) ; chaque
// bannière/logo servi par le proxy /api/media traversait le backend —
// ~300 Ko × des milliers de chargements de portail captif par jour ont
// produit la facture de septembre (~13 $). R2, lui, ne facture PAS la
// sortie : les images migrent sur le domaine public du bucket.
//
// R2_PUBLIC_BASE (ex. https://media.ftci.fr — domaine public du bucket R2,
// proxifié Cloudflare) déplace la lecture des images sur R2 en direct :
//   - l'upload RENVOIE l'URL publique (plus le proxy) ;
//   - les URL proxy DÉJÀ STOCKÉES (bannières des comptes existants) sont
//     réécrites à la volée au service (resolvePortalBranding, wifi site
//     info) — zéro migration, zéro geste du gérant ;
//   - le proxy GET /api/media/{key} RESTE (pages portail déjà déployées
//     sur les routeurs avec l'ancienne URL cuite, repli si le fetch live
//     échoue) mais il n'est plus LA voie de service ;
//   - le domaine public rejoint le walled-garden (walledGardenDomains) :
//     les invités pré-auth chargent la bannière depuis R2.
//
// Vide/non configuré ⇒ comportement inchangé (proxy historique) — déploiement
// rétrocompatible : le code peut partir AVANT la variable d'env.
// ---------------------------------------------------------------------------

// mediaPublicBase — base publique du bucket R2 (https://media.ftci.fr),
// sans slash final. Vide = fonctionnalité inactive (proxy historique).
func mediaPublicBase() string {
	return strings.TrimRight(strings.TrimSpace(getEnv("R2_PUBLIC_BASE")), "/")
}

// mediaRewriteURL — réécrit une URL média servie par le proxy (n'importe
// quelle origine + /api/media/media/{compte}/{année}/{hex}.{ext}) vers la
// base publique R2. Les URL externes (https://… hors MikCloud), vides ou
// au format inattendu restent intactes — défense en profondeur : le
// suffixe doit matcher mediaKeyRe (clé délivrée par l'upload, hex 128 bits)
// avant toute réécriture.
func mediaRewriteURL(u string) string {
	base := mediaPublicBase()
	if base == "" || u == "" {
		return u
	}
	i := strings.Index(u, "/api/media/media/")
	if i < 0 {
		return u
	}
	key := u[i+len("/api/media/"):] // "media/{compte}/{année}/{hex}.{ext}"
	if !mediaKeyRe.MatchString(key) {
		return u
	}
	return base + "/" + key
}

// mediaServeURL — URL de service d'une clé média fraîchement posée : base
// publique R2 si configurée, sinon le proxy (même hôte que apiBase —
// joignable pré-auth, walled-garden N°48). L'URL publique est ce que la
// console PERSISTE en BannerURL/LogoURL : elle façonne l'empreinte du
// portail (portalBrandingFingerprint) → re-déploiement automatique.
func mediaServeURL(r *http.Request, key string) string {
	if pub := mediaPublicBase(); pub != "" {
		return pub + "/" + key
	}
	return agentBaseURL(r) + "/api/media/" + key
}

// handleMediaUpload — POST /api/media (auth gérant, multipart "file").
// Dépose l'image dans R2 et renvoie son URL publique permanente :
// {R2_PUBLIC_BASE}/{key} (N°205) ou {base}/api/media/{key} (proxy historique).
func (a *API) handleMediaUpload(w http.ResponseWriter, r *http.Request) {
	mc := mediaConfig()
	if mc == nil {
		writeErrCode(w, http.StatusServiceUnavailable, "media_unconfigured",
			"Stockage d'images non configuré — collez une URL https ou utilisez une image intégrée", nil)
		return
	}
	acc := accountScope(r)
	if acc == "" {
		writeErr(w, http.StatusUnauthorized, "Session invalide")
		return
	}
	// Limite dure AVANT lecture : un corps géant n'est jamais bufferisé.
	r.Body = http.MaxBytesReader(w, r.Body, mediaMaxBytes+(1<<20)) // marge encodage multipart
	if err := r.ParseMultipartForm(mediaMaxBytes); err != nil {
		writeErr(w, http.StatusBadRequest, "Image trop volumineuse (2 Mo max) ou formulaire invalide")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Champ « file » manquant (multipart)")
		return
	}
	defer func() { _ = file.Close() }()

	// Sniff du type dans le CONTENU (512 premiers octets).
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	ctype := http.DetectContentType(head[:n])
	ext, allowed := mediaAllowedTypes[ctype]
	if !allowed {
		writeErr(w, http.StatusBadRequest, "Format d'image non pris en charge (jpg, png, webp, gif)")
		return
	}
	rest, _ := io.ReadAll(io.LimitReader(file, mediaMaxBytes+1))
	data := append(append([]byte{}, head[:n]...), rest...)
	if len(data) > mediaMaxBytes {
		writeErr(w, http.StatusBadRequest, "Image trop volumineuse (2 Mo max)")
		return
	}
	if len(data) < 32 {
		writeErr(w, http.StatusBadRequest, "Image vide ou corrompue")
		return
	}

	// Clé : media/{compte}/{année}/{hex32}{ext} — collision quasi impossible
	// (128 bits), immutabilité de fait (Cache-Control immuable en lecture).
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		writeErr(w, http.StatusInternalServerError, "Erreur interne (aléa)")
		return
	}
	key := fmt.Sprintf("media/%s/%d/%s%s", acc, time.Now().UTC().Year(), hex.EncodeToString(buf), ext)

	if err := mediaPutObject(mc, key, ctype, data); err != nil {
		log.Printf("[media] upload %s : %v", key, err)
		writeErr(w, http.StatusBadGateway, "Stockage média indisponible — réessayez")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"url":  mediaServeURL(r, key), // N°205 — R2 public si configuré, proxy sinon
		"key":  key,
		"size": len(data),
		"type": ctype,
	})
}

// handleMediaGet — GET /api/media/{key} (PUBLIC, cache immutable) : rejoue
// l'objet depuis R2. Les images du portail captif passent par ici (pré-auth,
// même hôte que le claim) — d'où l'importance du Cache-Control : un navigateur
// ne re-télécharge jamais une image déjà vue.
//
// N°209 — REDIRECTION vers le domaine public R2 quand R2_PUBLIC_BASE est
// configuré : le proxy ne STREAM PLUS l'objet (chaque octet traversait le
// backend = bande passante Render facturable, 0,15 $/Go au-delà de 5 Go —
// constat production 02/10/2026 : la catégorie « medias » du compteur N°72
// comptait 83,7 Mo en 8,5 h POST-fix N°205, portée par les slides/promos
// non réécrits et les pages déjà cuites). Une redirection coûte ~300 octets
// au lieu de ~230 Ko : le proxy devient un aiguilleur, plus un tuyau. Les
// consommateurs restants (pages portail pas encore re-déployées, caches
// navigateur périmées, console) suivent la redirection naturellement ; le
// cache d'une heure borne le re-contrôle. Base vide ⇒ proxy historique
// (streaming) — rétrocompatibilité du déploiement.
func (a *API) handleMediaGet(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if !mediaKeyRe.MatchString(key) {
		writeErr(w, http.StatusNotFound, "Image introuvable")
		return
	}
	// N°209 — aiguillage vers R2 public : la clé est DÉJÀ validée (aucune
	// redirection ouverte — on ne redirige que nos clés hex 128 bits vers
	// NOTRE base), et l'objet vit sur le domaine public indépendamment de
	// notre jeton API R2 (le GET public ne l'exige pas).
	if pub := mediaPublicBase(); pub != "" {
		w.Header().Set("Location", pub+"/"+key)
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.WriteHeader(http.StatusFound)
		return
	}
	mc := mediaConfig()
	if mc == nil {
		// N°183 — « pas configuré » n'est PAS « introuvable » : le 404
		// générique d'avant rendait l'absence de configuration impossible
		// à distinguer d'une vraie image manquante dans les logs. Le code
		// media_unconfigured (miroir du 503 d'upload) rend l'état
		// diagnostiquable ; la sonde de la carte Santé (media_health.go)
		// l'affiche en continu.
		writeErrCode(w, http.StatusServiceUnavailable, "media_unconfigured",
			"Stockage d'images non configuré (R2)", nil)
		return
	}
	ext := key[strings.LastIndex(key, "."):]
	ctype := mediaExtToType[ext]
	if ctype == "" {
		writeErr(w, http.StatusNotFound, "Image introuvable")
		return
	}
	body, err := mediaGetObject(mc, key)
	if err != nil {
		if _, ok := err.(errMediaNotFound); ok {
			writeErr(w, http.StatusNotFound, "Image introuvable")
			return
		}
		log.Printf("[media] get %s : %v", key, err)
		writeErr(w, http.StatusBadGateway, "Stockage média indisponible — réessayez")
		return
	}
	defer func() { _ = body.Close() }()
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Clé unique à jamais (hex 128 bits) → cache agressif sûr.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

// mediaPutObject — PUT /accounts/{a}/r2/buckets/{b}/objects/{key} (REST).
func mediaPutObject(mc *mediaClient, key, ctype string, data []byte) error {
	endpoint := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/accounts/%s/r2/buckets/%s/objects/%s",
		url.PathEscape(mc.account), url.PathEscape(mc.bucket), url.PathEscape(key),
	)
	req, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+mc.token)
	req.Header.Set("Content-Type", ctype)
	// N°205 — cache immuable stocké DANS les métadonnées de l'objet : le
	// domaine public R2 le renvoie tel quel → CDN Cloudflare + navigateurs
	// ne re-téléchargent jamais une clé déjà vue (les clés sont uniques à
	// jamais : hex 128 bits). Vérifié au feu : l'API REST stocke cet
	// en-tête dans httpMetadata.cacheControl.
	req.Header.Set("Cache-Control", "public, max-age=31536000, immutable")
	resp, err := mediaHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("r2 put: statut %d", resp.StatusCode)
	}
	return nil
}

// mediaGetObject — GET objet (REST) : body streamé + erreur typée 404.
func mediaGetObject(mc *mediaClient, key string) (io.ReadCloser, error) {
	endpoint := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/accounts/%s/r2/buckets/%s/objects/%s",
		url.PathEscape(mc.account), url.PathEscape(mc.bucket), url.PathEscape(key),
	)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+mc.token)
	resp, err := mediaHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return nil, errMediaNotFound{}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("r2 get: statut %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// errMediaNotFound — 404 côté R2 (objet absent).
type errMediaNotFound struct{}

func (errMediaNotFound) Error() string { return "objet absent" }
