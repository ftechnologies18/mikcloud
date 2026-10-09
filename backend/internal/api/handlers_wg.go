// N°285 — renfort WireGuard des routeurs agents : endpoints console.
//
//	POST /api/routers/{id}/wg-enable   → file wg_keygen (ou passe « ready » si clé connue)
//	GET  /api/routers/{id}/wg          → état du tunnel (JAMAIS la PSK ni la clé serveur)
//	PUT  /api/routers/{id}/wg-params   → pose les valeurs du peer serveur (livret wg-peer.sh) + file wg_setup
//	POST /api/routers/{id}/wg-test     → dial direct 10.8.0.N:8728 (preuve de joignabilité du tunnel)
//	POST /api/routers/{id}/wg-disable  → file wg_teardown (démontage routeur) + rappel de révocation serveur
//
// DOCTRINE (N°285) : le mode agent reste le SOCLE — le tunnel n'est JAMAIS
// sur le chemin critique du check-in (aucune réécriture de scheduler, aucun
// DNS static) : un tunnel mort dégrade le RENFORT, jamais le contrôle.
package api

import (
	"encoding/json"
	"fmt"
	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// wgResult — réponse générique des endpoints WG (l'état complet est lu via
// GET /api/routers/{id}/wg — les handlers de mutation répondent court).
type wgResult struct {
	OK      bool   `json:"ok"`
	State   string `json:"state"`
	Message string `json:"message"`
}

// wgRouterLocked — retrouve le routeur (périmètre compte) et impose le mode
// agent : le renfort WireGuard ne s'applique qu'au mode agent (le mode real
// a déjà son chemin direct, le simulé n'a pas de réseau réel).
func (a *API) wgRouterLocked(id, acc string) (*model.Router, *model.DB) {
	db := a.store.Data()
	cur := findRouterScoped(db, id, acc)
	if cur == nil || cur.Mode != "agent" {
		return nil, db
	}
	return cur, db
}

// parseEndpointWG — découpe "IP:port" du livret wg-peer.sh (IPv4 v4-only :
// JoinHostPort ajouterait des crochets inutiles ; un hôte nu est accepté
// avec le port par défaut 51820).
func parseEndpointWG(raw string) (host string, port int, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0, fmt.Errorf("endpoint vide")
	}
	if i := strings.LastIndexByte(raw, ':'); i >= 0 {
		host = strings.TrimSpace(raw[:i])
		port, err = strconv.Atoi(strings.TrimSpace(raw[i+1:]))
		if err != nil {
			return "", 0, fmt.Errorf("port d'endpoint invalide")
		}
	} else {
		host = raw
		port = 51820
	}
	if !agent.ValidWgHost(host) {
		return "", 0, fmt.Errorf("hôte d'endpoint invalide")
	}
	if !agent.ValidWgPort(port) {
		return "", 0, fmt.Errorf("port d'endpoint invalide (1-65535)")
	}
	return host, port, nil
}

// handleRouterWgEnable — POST /api/routers/{id}/wg-enable : lance le cycle
// du renfort. Si la clé publique du routeur est déjà connue (re-activation),
// pas de commande : état « ready » immédiat (le routeur ne re-génère pas sa
// clé — l'interface mikcloud-wg est idempotente côté routeur).
func (a *API) handleRouterWgEnable(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")

	a.store.Lock()
	cur, db := a.wgRouterLocked(id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur agent introuvable")
		return
	}
	// Dédup : une keygen en file/vol ne se re-file pas (pattern N°115).
	if cur.WgState == "pending_keygen" {
		a.store.Unlock()
		writeJSON(w, http.StatusOK, wgResult{OK: true, State: cur.WgState,
			Message: "Génération de clé déjà en attente du prochain check-in (≤ 45 s)"})
		return
	}
	suggestion := agent.WgPeerNameSuggestion(cur.Name, cur.ID)
	if cur.WgPub != "" {
		cur.WgState = "ready"
		cur.WgError = ""
		a.logActivityBy(r, db, cur.AccountID, "router", "Renfort WireGuard ré-armé sur «"+cur.Name+"» (clé déjà connue)")
		a.store.Save()
		a.store.Unlock()
		writeJSON(w, http.StatusOK, wgResult{OK: true, State: "ready",
			Message: "Clé du routeur déjà connue — posez le peer côté serveur puis livrez les paramètres"})
		return
	}
	cmd := queueCommandLocked(db, cur.AccountID, cur.ID, model.CmdWgKeygen, map[string]any{})
	cur.WgState = "pending_keygen"
	cur.WgError = ""
	cur.WgPeerName = suggestion
	a.logActivityBy(r, db, cur.AccountID, "router", "Renfort WireGuard demandé sur «"+cur.Name+"» — génération de clé au prochain check-in (≤ 45 s)")
	a.store.Save()
	cmdID := cmd.ID
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "state": "pending_keygen", "commandId": cmdID,
		"peerName": suggestion,
		"message":  "Le routeur va générer sa clé WireGuard au prochain check-in (≤ 45 s) — sa clé privée ne quitte jamais l'appareil",
	})
}

// handleRouterWgStatus — GET /api/routers/{id}/wg : état du renfort.
// Les secrets (PSK, clé publique serveur) ne sortent JAMAIS — seuls leur
// présence (hasParams) et les valeurs non sensibles sont retournées.
func (a *API) handleRouterWgStatus(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	cur, _ := a.wgRouterLocked(id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur agent introuvable")
		return
	}
	rc := *cur
	a.store.Unlock()

	online := false
	if t, err := time.Parse(time.RFC3339, rc.LastSeen); err == nil {
		online = time.Since(t) < OnlineWindow
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"state":        rc.WgState,
		"wgPub":        rc.WgPub,
		"peerName":     agent.WgPeerNameSuggestion(rc.Name, rc.ID),
		"wgPeerName":   rc.WgPeerName,
		"ipv4":         rc.WgIPv4,
		"endpoint":     rc.WgEndpoint,
		"appliedAt":    rc.WgAppliedAt,
		"error":        rc.WgError,
		"online":       online,
		"mode":         "agent",
		"hasParams":    rc.WgIPv4 != "" && rc.WgServerPub != "" && rc.WgPSK != "",
		"hasKeygen":    rc.WgPub != "",
		"serverTunnel": agent.WgServerTunnelIP,
		"note":         "PSK et clé serveur jamais exposés — livrez-les via PUT wg-params depuis le livret wg-peer.sh",
	})
}

// handleRouterWgParams — PUT /api/routers/{id}/wg-params : pose les valeurs
// du peer serveur (issues du livret /opt/wireguard/peers/<nom>.router.txt
// produit par wg-peer.sh add-router) et file wg_setup. Validations strictes :
// une valeur mal formée ne part jamais au routeur.
func (a *API) handleRouterWgParams(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		PeerName  string `json:"peerName"`
		Address   string `json:"address"`
		ServerPub string `json:"serverPub"`
		PSK       string `json:"psk"`
		Endpoint  string `json:"endpoint"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	req.PeerName = strings.TrimSpace(req.PeerName)
	req.Address = strings.TrimSpace(req.Address)
	req.ServerPub = strings.TrimSpace(req.ServerPub)
	req.PSK = strings.TrimSpace(req.PSK)

	if !agent.ValidWgPeerName(req.PeerName) {
		writeErr(w, http.StatusBadRequest, "Nom de peer invalide (attendu [a-z0-9-], 1-32 — celui du dispatch ops-wg)")
		return
	}
	if !agent.ValidWgIPv4(req.Address) {
		writeErr(w, http.StatusBadRequest, "Adresse tunnel invalide (attendu 10.8.0.N, N entre 2 et 254)")
		return
	}
	if !agent.ValidWgKey(req.ServerPub) {
		writeErr(w, http.StatusBadRequest, "Clé publique serveur invalide (44 caractères base64 terminés par « = »)")
		return
	}
	if !agent.ValidWgKey(req.PSK) {
		writeErr(w, http.StatusBadRequest, "PresharedKey invalide (44 caractères base64 terminés par « = »)")
		return
	}
	host, port, err := parseEndpointWG(req.Endpoint)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Endpoint invalide : "+err.Error())
		return
	}

	a.store.Lock()
	cur, db := a.wgRouterLocked(id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur agent introuvable")
		return
	}
	if cur.WgPub == "" {
		a.store.Unlock()
		writeErr(w, http.StatusConflict, "Générez d'abord la clé WireGuard du routeur (Activer le tunnel) — le livret serveur en dépend")
		return
	}
	// Anti-collision : une adresse tunnel = UN routeur (pool global à la VM).
	for i := range db.Routers {
		other := &db.Routers[i]
		if other.ID != cur.ID && other.WgIPv4 == req.Address {
			a.store.Unlock()
			writeErr(w, http.StatusConflict, "Adresse tunnel "+req.Address+" déjà attribuée à un autre routeur («"+other.Name+"»)")
			return
		}
	}
	payload := agent.WgSetupPayloadFrom(req.ServerPub, req.PSK, agent.WgIPv4WithPrefix(req.Address), host, port)
	cmd := queueCommandLocked(db, cur.AccountID, cur.ID, model.CmdWgSetup, payload)
	cur.WgPeerName = req.PeerName
	cur.WgIPv4 = req.Address
	cur.WgServerPub = req.ServerPub
	cur.WgPSK = req.PSK
	cur.WgEndpoint = host + ":" + strconv.Itoa(port)
	cur.WgState = "pending_setup"
	cur.WgError = ""
	a.logActivityBy(r, db, cur.AccountID, "router", "Tunnel WireGuard provisionné sur «"+cur.Name+"» (peer "+req.PeerName+", "+req.Address+") — application au prochain check-in (≤ 45 s)")
	a.store.Save()
	cmdID := cmd.ID
	a.store.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "state": "pending_setup", "commandId": cmdID,
		"message": "Paramètres livrés — le routeur configure son tunnel au prochain check-in (≤ 45 s), puis teste-le (Bouton Tester)",
	})
}

// dialWGPort — dial TCP direct à travers le tunnel (le backend tourne sur
// l'hôte : wg0 est joignable nativement). Retourne la latence en ms.
func dialWGPort(ipv4 string, port int) (int64, bool) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ipv4, strconv.Itoa(port)), 3*time.Second)
	if err != nil {
		return 0, false
	}
	latency := time.Since(start).Milliseconds()
	_ = conn.Close()
	return latency, true
}

// handleRouterWgTest — POST /api/routers/{id}/wg-test : preuve de
// joignabilité directe du tunnel depuis la VM. Diagnostic en deux ports :
// 8728 (API RouterOS, activée par défaut) puis 8291 (Winbox) — un tunnel UP
// avec 8728 fermé est rapporté « joignable mais API désactivée », jamais
// « tunnel mort » (honnêteté du diagnostic).
func (a *API) handleRouterWgTest(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	cur, _ := a.wgRouterLocked(id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur agent introuvable")
		return
	}
	rc := *cur
	a.store.Unlock()

	if rc.WgIPv4 == "" {
		writeErr(w, http.StatusBadRequest, "Aucune adresse tunnel connue pour ce routeur — livrez d'abord les paramètres (wg-params)")
		return
	}
	if latency, ok := dialWGPort(rc.WgIPv4, 8728); ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "reachable": true, "port": 8728, "latencyMs": latency,
			"message": fmt.Sprintf("Tunnel joignable — API RouterOS répond sur %s:8728 (%d ms)", rc.WgIPv4, latency),
		})
		return
	}
	if latency, ok := dialWGPort(rc.WgIPv4, 8291); ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "reachable": true, "port": 8291, "latencyMs": latency,
			"message": "Tunnel joignable (Winbox répond) — mais l'API RouterOS est fermée : /ip service enable api pour le pilotage direct",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": false, "reachable": false, "port": 0, "latencyMs": 0,
		"message": "Tunnel non joignable (" + rc.WgIPv4 + ") — vérifiez : handshake côté VM (ops-wg status), peer posé, Security List UDP 51820",
	})
}

// handleRouterWgDisable — POST /api/routers/{id}/wg-disable : file le
// démontage côté routeur (wg_teardown) ; la révocation serveur reste un
// geste séparé (ops-wg peer-remove) — le message le rappelle explicitement.
func (a *API) handleRouterWgDisable(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("id")
	a.store.Lock()
	cur, db := a.wgRouterLocked(id, acc)
	if cur == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur agent introuvable")
		return
	}
	if cur.WgPub == "" && cur.WgIPv4 == "" {
		a.store.Unlock()
		writeJSON(w, http.StatusOK, wgResult{OK: true, State: "", Message: "Tunnel déjà absent de ce routeur"})
		return
	}
	if pend := pendingCommandOfKind(db, cur.ID, model.CmdWgTeardown); pend == nil {
		queueCommandLocked(db, cur.AccountID, cur.ID, model.CmdWgTeardown, map[string]any{})
	}
	msg := "Démontage demandé — application au prochain check-in (≤ 45 s)"
	if cur.WgPeerName != "" {
		msg += ". Pensez à révoquer le peer côté serveur : dispatch ops-wg mode=peer-remove peer_name=" + cur.WgPeerName
	}
	a.logActivityBy(r, db, cur.AccountID, "router", "Démontage du tunnel WireGuard demandé sur «"+cur.Name+"»")
	a.store.Save()
	a.store.Unlock()

	writeJSON(w, http.StatusOK, wgResult{OK: true, State: "active", Message: msg})
}
