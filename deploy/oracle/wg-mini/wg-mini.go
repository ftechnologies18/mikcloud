// wg-mini — N°291 — passerelle locale WireGuard pour MikCloud (chantier ⑦,
// décision D3-a de N°289).
//
// RÔLE : le backend MikCloud tourne dans un conteneur distroless — il ne
// peut ni sudo ni toucher /etc/wireguard. wg-mini est une unité systemd
// ROOT (wg-peer.sh exige root) sur la MÊME VM qui expose les gestes de
// wg-peer.sh au backend, en boucle locale exclusivement :
//
//	POST /v1/add    {"name":"vpn-…"}  → wg-peer.sh add    → {"ok":true,"conf":"…","ip":"10.8.0.N"}
//	POST /v1/remove {"name":"vpn-…"}  → wg-peer.sh remove → {"ok":true}
//	GET  /v1/conf?name=vpn-…          → lecture conf      → {"ok":true,"conf":"…"}
//	GET  /v1/list                     → wg-peer.sh list   → {"ok":true,"peers":[…]}
//	GET  /v1/ping                     → {"ok":true,"service":"wg-mini","version":"v1"}
//
// SÉCURITÉ (conventions §8 de docs/RUNBOOK-HEBERGEMENT.md) :
//   - bind 127.0.0.1:4020 UNIQUEMENT (port à réserver au registre) — aucune
//     traversée réseau publique ;
//   - TOUTE requête signée HMAC-SHA256("<ts>.<body>", WG_MINI_SECRET) — le
//     MÊME schéma que les webhooks GeniusPay du backend, inversé ;
//     fenêtre d'horloge ±120 s, comparaison à temps constant ;
//   - nom de peer validé par regex stricte côté mini ET côté cloud (aucun
//     caractère d'échappement ne peut atteindre exec — pas de shell) ;
//   - le secret et les confs ne sont JAMAIS logués (clés hors logs) ;
//   - timeouts partout (script 20 s, client 10 s), corps limité à 8 Ko.
//
// INSTALLATION : cf. README.md du répertoire (recette §8 complète).
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const version = "v1"

var (
	addr       = envOr("WG_MINI_ADDR", "127.0.0.1:4020")
	scriptPath = envOr("WG_PEER_SCRIPT", "/opt/wireguard/wg-peer.sh")
	peersDir   = envOr("WG_PEERS_DIR", "/opt/wireguard/peers")
	secret     = strings.TrimSpace(os.Getenv("WG_MINI_SECRET"))
)

// nameRe — mêmes règles que le cloud (model.vpnpeer.go) : préfixe « vpn- »
// obligatoire (les peers de GESTION routeurs ne passent PAS par wg-mini),
// minuscules/chiffres/tirets, aucun métacaractère.
var nameRe = regexp.MustCompile(`^vpn-[a-z0-9][a-z0-9-]{2,26}$`)

// scriptTimeout — wg-peer.sh add génère des clés + syncconf : quelques ms
// sur la VM ; 20 s couvre une VM fatiguée sans laisser pendre une requête.
const scriptTimeout = 20 * time.Second

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// ---------------------------------------------------------------------------
// Authentification HMAC (miroir exact du client backend wgmini.go)
// ---------------------------------------------------------------------------

func authorized(r *http.Request, body []byte) bool {
	tsRaw := r.Header.Get("X-WGMini-Timestamp")
	sig := r.Header.Get("X-WGMini-Signature")
	if tsRaw == "" || sig == "" {
		return false
	}
	ts, err := strconv.ParseInt(tsRaw, 10, 64)
	if err != nil {
		return false
	}
	if d := time.Since(time.Unix(ts, 0)); d > 120*time.Second || d < -120*time.Second {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", ts)
	mac.Write(body)
	given, err := hex.DecodeString(strings.ToLower(sig))
	if err != nil {
		return false
	}
	return hmac.Equal(given, mac.Sum(nil))
}

// ---------------------------------------------------------------------------
// Helpers réponse + exécution script
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	log.Printf("[refus] %s", msg) // messages techniques SEULEMENT — jamais de corps/conf
	writeJSON(w, status, map[string]any{"ok": false, "error": msg})
}

// runScript — exécute wg-peer.sh SANS shell (exec direct, arguments
// positionnels : l'injection est structurellement impossible).
func runScript(ctx context.Context, args ...string) (string, error) {
	c, cancel := context.WithTimeout(ctx, scriptTimeout)
	defer cancel()
	cmd := exec.CommandContext(c, scriptPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if c.Err() == context.DeadlineExceeded {
			return "", errors.New("wg-peer.sh a dépassé 20 s — vérifiez wg0 sur la VM")
		}
		// dernier message utile du script (« ❌ … ») pour la console cloud
		msg := lastLine(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(msg)
	}
	return string(out), nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	line := strings.TrimSpace(lines[len(lines)-1])
	if len(line) > 200 {
		line = line[:200]
	}
	return line
}

// confPath — chemin de la conf d'un peer (le nom est déjà validé par la
// regex : aucune traversée de chemin possible).
func confPath(name string) string {
	return filepath.Join(peersDir, name+".conf")
}

// parseIPFromConf — la ligne Address de la conf client (informationnel).
func parseIPFromConf(conf string) string {
	for _, line := range strings.Split(conf, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Address") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				ip := strings.TrimSpace(parts[1])
				if i := strings.Index(ip, ","); i > 0 {
					ip = strings.TrimSpace(ip[:i]) // v4 d'abord, l'ULA v6 suit
				}
				return ip
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func handleAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "corps invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	if !nameRe.MatchString(name) {
		fail(w, http.StatusBadRequest, "nom de peer invalide (attendu vpn-… minuscules/tirets)")
		return
	}
	start := time.Now()
	out, err := runScript(r.Context(), "add", name)
	if err != nil {
		log.Printf("[add] %s échec (%s) : %v", name, time.Since(start).Round(time.Millisecond), err)
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	raw, err := os.ReadFile(confPath(name))
	if err != nil {
		log.Printf("[add] %s créé mais conf illisible : %v", name, err)
		fail(w, http.StatusInternalServerError, "peer créé sur wg0 mais conf illisible — vérifiez "+peersDir)
		return
	}
	conf := string(raw)
	log.Printf("[add] %s ok (%s)", name, time.Since(start).Round(time.Millisecond))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"name": name,
		"ip":   parseIPFromConf(conf),
		"conf": conf,
		"_":    strings.TrimSpace(out), // trace wg-peer.sh (sans la conf)
	})
}

func handleRemove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "corps invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	if !nameRe.MatchString(name) {
		fail(w, http.StatusBadRequest, "nom de peer invalide")
		return
	}
	start := time.Now()
	if _, err := runScript(r.Context(), "remove", name); err != nil {
		log.Printf("[remove] %s échec : %v", name, err)
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	log.Printf("[remove] %s ok (%s)", name, time.Since(start).Round(time.Millisecond))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleConf(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if !nameRe.MatchString(name) {
		fail(w, http.StatusBadRequest, "nom de peer invalide")
		return
	}
	raw, err := os.ReadFile(confPath(name))
	if err != nil {
		fail(w, http.StatusNotFound, "conf introuvable pour "+name)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name, "conf": string(raw)})
}

// handleList — parse la sortie de wg-peer.sh list :
//
//	nom | IP tunnel | dernier handshake | reçu/envoyé
//	vpn-xxxx | 10.8.0.N/32[, fd00:8::N/128] | 12s|jamais | 1.2 MiB/3.4 MiB
func handleList(w http.ResponseWriter, r *http.Request) {
	out, err := runScript(r.Context(), "list")
	if err != nil {
		log.Printf("[list] échec : %v", err)
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	peers := []map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "nom |") || strings.Contains(line, "─") {
			continue // en-tête / séparateur
		}
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}
		ip := strings.TrimSpace(parts[1])
		if i := strings.Index(ip, ","); i > 0 {
			ip = strings.TrimSpace(ip[:i])
		}
		peers = append(peers, map[string]string{
			"name":      strings.TrimSpace(parts[0]),
			"ip":        ip,
			"handshake": strings.TrimSpace(parts[2]),
			"transfer":  strings.TrimSpace(parts[3]),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "peers": peers})
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "wg-mini", "version": version})
}

// ---------------------------------------------------------------------------
// Serveur
// ---------------------------------------------------------------------------

// signed — lit le corps (borné à 8 Ko), vérifie la HMAC puis remet le corps
// en place pour le handler.
func signed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<10))
		if err != nil {
			fail(w, http.StatusRequestEntityTooLarge, "corps trop volumineux ou illisible")
			return
		}
		if !authorized(r, body) {
			log.Printf("[refus] signature HMAC invalide depuis %s", r.RemoteAddr)
			fail(w, http.StatusUnauthorized, "signature invalide")
			return
		}
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		next(w, r)
	}
}

func main() {
	if secret == "" || len(secret) < 32 {
		log.Fatal("WG_MINI_SECRET absent ou trop court (32 caractères minimum) — cf. /etc/mikcloud/wg-mini.env")
	}
	if _, err := os.Stat(scriptPath); err != nil {
		log.Printf("[démarrage] attention : %s introuvable (%v) — les gestes échoueront", scriptPath, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/add", signed(handleAdd))
	mux.HandleFunc("POST /v1/remove", signed(handleRemove))
	mux.HandleFunc("GET /v1/conf", signed(handleConf))
	mux.HandleFunc("GET /v1/list", signed(handleList))
	mux.HandleFunc("GET /v1/ping", signed(handlePing))
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("écoute %s impossible : %v", addr, err)
	}
	log.Printf("wg-mini %s à l'écoute sur %s (script=%s peers=%s)", version, addr, scriptPath, peersDir)
	log.Fatal(srv.Serve(ln))
}
