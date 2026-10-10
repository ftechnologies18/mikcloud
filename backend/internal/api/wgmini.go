// wgmini.go — N°291 — client du mini-service hôte wg-mini (décision D3-a de
// N°289) : la SEULE porte du backend vers wg-peer.sh sur la VM de Marseille.
//
// Pourquoi un mini-service : le backend tourne dans un conteneur distroless
// — il ne peut ni sudo ni toucher /etc/wireguard. wg-mini (unité systemd
// root, même machine, bind 127.0.0.1:4020 — port à réserver au registre,
// conventions §8) expose add/remove/list/conf de wg-peer.sh. Aucune
// traversée réseau publique : le trafic reste dans la boucle locale.
//
// Authentification : HMAC-SHA256("<timestamp>.<body>", WG_MINI_SECRET) — le
// MÊME schéma que les webhooks GeniusPay entrants (X-Webhook-Signature,
// handlers_geniuspay.go), inversé côté émetteur ; wg-mini vérifie en temps
// constant avec une fenêtre d'horloge de ±120 s. Le secret vit dans l'env
// du backend (/etc/mikcloud/env 600 root, conventions §8 — jamais logué,
// jamais commité).
//
// Discipline de tests : wgMiniCallFn est un point d'indirection (les tests
// api le remplacent — AUCUN réseau réel en CI, même discipline que
// sendAccountEmail / deliverNotif / telegramSendFn).
package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// wgMiniTimeout — les gestes wg-peer.sh sont locaux (wg syncconf ≈ ms, la
// génération de clés ≈ ms) ; 12 s couvre largement, aligné sur notify.
var wgMiniHTTP = &http.Client{Timeout: 12 * time.Second}

// wgMiniCallFn — point d'indirection pour les tests.
var wgMiniCallFn = wgMiniCall

// wgMiniTarget — URL + secret du mini-service. WG_MINI_URL est optionnel
// (défaut http://127.0.0.1:4020 — le port du registre, décision D3-a) ;
// WG_MINI_SECRET est OBLIGATOIRE (absence = produit explicitement non
// configuré, message orienté exploitant).
func wgMiniTarget() (url, secret string, err error) {
	secret = strings.TrimSpace(os.Getenv("WG_MINI_SECRET"))
	if secret == "" {
		return "", "", errors.New("mini-service wg-mini non configuré (WG_MINI_SECRET absent) — installez-le sur la VM cf. deploy/oracle/wg-mini/README.md")
	}
	url = strings.TrimSpace(os.Getenv("WG_MINI_URL"))
	if url == "" {
		url = "http://127.0.0.1:4020"
	}
	return url, secret, nil
}

// wgMiniCall — un appel signé. Réponse attendue : enveloppe JSON
// {"ok":true,...} ou {"ok":false,"error":"..."} (message humain francophone
// de wg-mini / wg-peer.sh, sans secret).
func wgMiniCall(method, path string, payload any) ([]byte, error) {
	base, secret, err := wgMiniTarget()
	if err != nil {
		return nil, err
	}
	var body []byte
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	ts := time.Now().UTC().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", ts)
	mac.Write(body)
	req, err := http.NewRequest(method, strings.TrimRight(base, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-WGMini-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-WGMini-Signature", hex.EncodeToString(mac.Sum(nil)))
	resp, err := wgMiniHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wg-mini injoignable : %w — vérifiez le service systemd wg-mini sur la VM (systemctl status wg-mini)", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("wg-mini : réponse illisible : %w", err)
	}
	var env struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	_ = json.Unmarshal(raw, &env)
	if resp.StatusCode != http.StatusOK || !env.OK {
		if env.Error != "" {
			return nil, errors.New(env.Error)
		}
		return nil, fmt.Errorf("wg-mini a répondu %s", resp.Status)
	}
	return raw, nil
}

// wgMiniPeer — une ligne de wg-peer.sh list (réconciliée, D4).
type wgMiniPeer struct {
	Name      string `json:"name"`
	IP        string `json:"ip"`
	Handshake string `json:"handshake"`
	Transfer  string `json:"transfer"`
}

// wgMiniAdd — wg-peer.sh add <name> : génère clés + conf client full-tunnel
// CÔTÉ VM et renvoie la conf (une seule traversée cloud ; la clé privée
// n'a jamais existé ailleurs).
func wgMiniAdd(name string) (conf, ip string, err error) {
	raw, err := wgMiniCallFn(http.MethodPost, "/v1/add", map[string]string{"name": name})
	if err != nil {
		return "", "", err
	}
	var env struct {
		Conf string `json:"conf"`
		IP   string `json:"ip"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", "", fmt.Errorf("wg-mini : réponse add illisible : %w", err)
	}
	if strings.TrimSpace(env.Conf) == "" {
		return "", "", errors.New("wg-mini : conf client vide — vérifiez wg-peer.sh sur la VM")
	}
	return env.Conf, env.IP, nil
}

// wgMiniRemove — wg-peer.sh remove <name> : retire le bloc [Peer] du
// wg0.conf + syncconf à chaud (les autres peers restent intacts) + supprime
// les livrables.
func wgMiniRemove(name string) error {
	_, err := wgMiniCallFn(http.MethodPost, "/v1/remove", map[string]string{"name": name})
	return err
}

// wgMiniConf — relit la conf d'un peer existant (re-livraison D5-a si la
// copie cloud a été perdue — self-healing au premier reveal).
func wgMiniConf(name string) (string, error) {
	raw, err := wgMiniCallFn(http.MethodGet, "/v1/conf?name="+url.QueryEscape(name), nil)
	if err != nil {
		return "", err
	}
	var env struct {
		Conf string `json:"conf"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", fmt.Errorf("wg-mini : réponse conf illisible : %w", err)
	}
	return env.Conf, nil
}

// wgMiniList — wg-peer.sh list (réconciliation cloud ↔ VM, check D4).
func wgMiniList() ([]wgMiniPeer, error) {
	raw, err := wgMiniCallFn(http.MethodGet, "/v1/list", nil)
	if err != nil {
		return nil, err
	}
	var env struct {
		Peers []wgMiniPeer `json:"peers"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("wg-mini : réponse list illisible : %w", err)
	}
	return env.Peers, nil
}

// wgMiniPing — health check (status de la console : dot VM joignable).
func wgMiniPing() error {
	_, err := wgMiniCallFn(http.MethodGet, "/v1/ping", nil)
	return err
}

// wgMiniPingFn — indirection pour les tests (même discipline).
var wgMiniPingFn = wgMiniPing

// wgMiniListFn — indirection pour les tests.
var wgMiniListFn = wgMiniList

// wgMiniAddFn / wgMiniRemoveFn / wgMiniConfFn — indirections pour les tests.
var (
	wgMiniAddFn    = wgMiniAdd
	wgMiniRemoveFn = wgMiniRemove
	wgMiniConfFn   = wgMiniConf
)
