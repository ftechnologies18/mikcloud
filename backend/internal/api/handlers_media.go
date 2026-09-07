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

// handleMediaUpload — POST /api/media (auth gérant, multipart "file").
// Dépose l'image dans R2 et renvoie son URL publique permanente :
// {base}/api/media/media/{compte}/{année}/{hex}.{ext}
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
	base := agentBaseURL(r) // même hôte que apiBase : joignable pré-auth (walled-garden N°48)
	writeJSON(w, http.StatusCreated, map[string]any{
		"url":  base + "/api/media/" + key,
		"key":  key,
		"size": len(data),
		"type": ctype,
	})
}

// handleMediaGet — GET /api/media/{key} (PUBLIC, cache immutable) : rejoue
// l'objet depuis R2. Les images du portail captif passent par ici (pré-auth,
// même hôte que le claim) — d'où l'importance du Cache-Control : un navigateur
// ne re-télécharge jamais une image déjà vue.
func (a *API) handleMediaGet(w http.ResponseWriter, r *http.Request) {
	mc := mediaConfig()
	key := r.PathValue("key")
	if mc == nil || !mediaKeyRe.MatchString(key) {
		writeErr(w, http.StatusNotFound, "Image introuvable")
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
