// handlers_ppp_live.go — N°294 — PPPoE Phase B : RENFORT TUNNEL (opt-in) —
// le pilotage temps réel des abonnés via l'API RouterOS directe à travers le
// tunnel WireGuard (gabarit handlers_wg.go N°285, porte annoncée depuis
// agent/wireguard.go : « ouvre la porte au pilotage direct »).
//
// Contrats :
//   - creds API : RÉUTILISE les Router.Username/Password existants (scellés
//     secretbox — sealedSnapshot/unsealSecrets, sanitizeRouter efface le
//     mot de passe de TOUTE sérialisation). Aucune nouvelle credential, aucun
//     nouveau mécanisme de stockage : le gérant les saisit une fois par
//     routeur dans la console.
//   - transport : dial 10.8.0.N:8728 (client binaire routeros, stdlib) — le
//     backend tourne SUR la VM, wg0 est joignable nativement (même chemin que
//     dialWGPort / wg-test N°285). Le mode agent reste LE SOCLE : le renfort
//     n'est jamais sur le chemin critique, un tunnel mort dégrade le temps
//     réel, jamais le contrôle (convergence 45 s intacte).
//   - honnêteté des refus : tunnel absent / creds absentes / API fermée sont
//     des erreurs DISTINCTES et explicites (jamais « tunnel mort » pour une
//     API désactivée — même discipline que le diagnostic wg-test).
//
// Provisioning assisté (D2 phase B) : génération d'un script .rsc IDEMPOTENT
// (garde :if [:len …] = 0 sur chaque bloc) que le WISP colle dans son
// terminal — MikCloud ne touche JAMAIS au routeur ici : c'est un livrable de
// documentation exécutable, sans aucun secret.
package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/routeros"
)

// pppLivePort — port de l'API RouterOS à travers le tunnel (le diagnostic
// wg-test N°285 guide déjà le gérant vers /ip service enable api).
const pppLivePort = 8728

// pppLiveTimeout — budget du dial + commandes temps réel (l'API binaire est
// locale au tunnel : latence sub-milliseconde, 6 s est très large).
const pppLiveTimeout = 6 * time.Second

// pppLiveFetch — indirection de test : lecture directe /ppp/active/print via
// le tunnel (le dial binaire authentifié est encapsulé — les tests injectent
// des lignes factices, aucun réseau ; même discipline que telegramSendFn).
var pppLiveFetch = func(ipv4, username, password string) ([]map[string]string, error) {
	c, err := routeros.Dial(ipv4, pppLivePort, username, password, pppLiveTimeout)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	res, _, err := c.Run("/ppp/active/print")
	return res, err
}

// pppLiveKick — indirection de test : déconnexion directe (find par nom puis
// remove par .id — protocole binaire, pas de shell ni d'interpolation libre).
// Session déjà fermée = convergé (nil).
var pppLiveKick = func(ipv4, username, password, name string) error {
	c, err := routeros.Dial(ipv4, pppLivePort, username, password, pppLiveTimeout)
	if err != nil {
		return err
	}
	defer c.Close()
	res, _, err := c.Run("/ppp/active/print", "?name="+name)
	if err != nil {
		return err
	}
	if len(res) == 0 {
		return nil
	}
	rid := res[0][".id"]
	if rid == "" {
		return fmt.Errorf("entrée session sans identifiant")
	}
	_, _, err = c.Run("/ppp/active/remove", "=.id="+rid)
	return err
}

// pppLiveGate — prérequis du renfort sous verrou : routeur agent + tunnel
// provisionné (WgIPv4) + creds API posées. Retourne le message d'erreur
// console ("" = tout est prêt), l'IPv4 tunnel et les creds.
func pppLiveGateLocked(rr *model.Router) (msg, ipv4, user, pass string) {
	if rr.Mode != "agent" {
		return "Le renfort temps réel s'applique aux routeurs en mode agent", "", "", ""
	}
	if rr.WgIPv4 == "" {
		return "Aucun tunnel WireGuard provisionné pour ce routeur — activez d'abord le renfort WireGuard (wg-params)", "", "", ""
	}
	if rr.Username == "" || rr.Password == "" {
		return "Credentials API RouterOS absentes — enregistrez-les dans la console (Renfort temps réel)", "", "", ""
	}
	return "", rr.WgIPv4, rr.Username, rr.Password
}

// ---------------------------------------------------------------------------
// Credentials API du renfort (stockage sealed existant — réutilisation)
// ---------------------------------------------------------------------------

// handlePppApiCredsGet — GET /api/routers/{routerID}/ppp/api-creds : état du
// renfort temps réel (JAMAIS le mot de passe — sanitizeRouter le retire déjà
// de toute sérialisation, ici on ne renvoie que la présence).
func (a *API) handlePppApiCredsGet(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("routerID")
	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	out := map[string]any{
		"configured":  rr.Username != "" && rr.Password != "",
		"username":    rr.Username,
		"hasTunnel":   rr.WgIPv4 != "",
		"wgIpv4":      rr.WgIPv4,
		"port":        pppLivePort,
		"agentOnline": rr.Status == "online",
		"tunnelState": rr.WgState,
		"note":        "Le mot de passe n'est jamais retourné — seule sa présence est confirmée",
	}
	a.store.Unlock()
	writeJSON(w, http.StatusOK, out)
}

// handlePppApiCredsPut — PUT /api/routers/{routerID}/ppp/api-creds
// {username,password} : pose les creds API RouterOS du renfort temps réel
// (stockage sealed existant — secretbox via sealedSnapshot, jamais
// sérialisées). Idempotent : reposer les mêmes creds est sans effet.
func (a *API) handlePppApiCredsPut(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("routerID")
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	username := strings.TrimSpace(req.Username)
	if len(username) < 1 || len(username) > 64 {
		writeErr(w, http.StatusBadRequest, "Nom d'utilisateur API invalide (1-64 caractères)")
		return
	}
	if strings.ContainsAny(username, ` " $\`) {
		writeErr(w, http.StatusBadRequest, "Nom d'utilisateur API invalide (caractères interdits)")
		return
	}
	if len(req.Password) < 1 || len(req.Password) > 64 {
		writeErr(w, http.StatusBadRequest, "Mot de passe API invalide (1-64 caractères)")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	if rr.Mode != "agent" {
		a.store.Unlock()
		writeErr(w, http.StatusBadRequest, "Le renfort temps réel s'applique aux routeurs en mode agent")
		return
	}
	rr.Username = username
	rr.Password = req.Password // scellé au persist (sealedSnapshot), jamais sérialisé
	a.logActivityBy(r, db, acc, "ppp", "Credentials API temps réel enregistrées pour «"+rr.Name+"» (renfort PPPoE)")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Credentials enregistrées (chiffrées au repos) — le temps réel est disponible via le tunnel",
	})
}

// handlePppApiCredsDelete — DELETE /api/routers/{routerID}/ppp/api-creds :
// retrait des creds (le renfort retombe sur le canal agent seul — le SOCLE
// n'est jamais affecté).
func (a *API) handlePppApiCredsDelete(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("routerID")
	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	rr.Username = ""
	rr.Password = ""
	a.logActivityBy(r, db, acc, "ppp", "Credentials API temps réel retirées de «"+rr.Name+"»")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Credentials retirées — le pilotage redevient agent seul (≤ 45 s)"})
}

// ---------------------------------------------------------------------------
// Temps réel via tunnel : sessions actives + kick instantané
// ---------------------------------------------------------------------------

// handlePppLiveActive — POST /api/routers/{routerID}/ppp/live/active : les
// sessions PPPoE actives lues DIRECTEMENT sur le routeur à travers le tunnel
// (temps réel — pas de cache 120 s). POST (et non GET) : l'appel déclenche
// une connexion authentifiée, il n'est ni idempotent ni cachable.
func (a *API) handlePppLiveActive(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("routerID")
	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	msg, ipv4, user, pass := pppLiveGateLocked(rr)
	if msg != "" {
		a.store.Unlock()
		writeErr(w, http.StatusConflict, msg)
		return
	}
	// Copie VALEUR : aucun pointeur vers le store ne franchit le dial.
	ipv4C, userC, passC := ipv4, user, pass
	a.store.Unlock()
	start := time.Now()
	res, err := pppLiveFetch(ipv4C, userC, passC)
	latency := time.Since(start).Milliseconds()
	rows := []pppActiveRow{}
	if err == nil {
		for _, m := range res {
			rows = append(rows, pppActiveRow{
				Name:     m["name"],
				Service:  m["service"],
				CallerID: m["caller-id"],
				Address:  m["address"],
				Uptime:   m["uptime"],
			})
		}
	}
	if err != nil {
		// Honnêteté du diagnostic (gabarit wg-test) : les creds
		// n'apparaissent JAMAIS dans le message.
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"ok": false, "reachable": false, "error": err.Error(),
			"message": "API RouterOS injoignable à travers le tunnel (" + err.Error() + ") — vérifiez /ip service enable api et les credentials",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "reachable": true, "port": pppLivePort,
		"latencyMs": latency, "data": rows, "count": len(rows),
		"updatedAt": model.NowISO(),
	})
}

// handlePppSecretKickLive — POST /api/ppp/secrets/{secretID}/kick-live :
// déconnexion INSTANTANÉE de la session PPPoE active (direct tunnel, sans
// attendre le check-in 45 s). Le SECRET reste — l'abonné peut se reconnecter
// (télémétrie, aucun état cloud, même contrat que le kick agent).
func (a *API) handlePppSecretKickLive(w http.ResponseWriter, r *http.Request) {
	if !a.guardAccountWrite(w, r) {
		return
	}
	acc := accountScope(r)
	id := r.PathValue("secretID")
	a.store.Lock()
	db := a.store.Data()
	s := model.FindPppSecretScoped(db, id, acc)
	if s == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Abonné introuvable")
		return
	}
	rr := findRouterScoped(db, s.RouterID, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	msg, ipv4, user, pass := pppLiveGateLocked(rr)
	if msg != "" {
		a.store.Unlock()
		writeErr(w, http.StatusConflict, msg)
		return
	}
	name := s.Name
	ipv4C, userC, passC := ipv4, user, pass
	a.store.Unlock()
	if err := pppLiveKick(ipv4C, userC, passC, name); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"ok": false, "error": err.Error(),
			"message": "Déconnexion directe impossible (" + err.Error() + ") — la déconnexion agent (≤ 45 s) reste disponible",
		})
		return
	}
	a.store.Lock()
	a.logActivityBy(r, a.store.Data(), acc, "ppp", "Abonné PPPoE "+name+" déconnecté en temps réel (tunnel)")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "name": name,
		"message": "Session déconnectée (temps réel via tunnel)",
	})
}

// ---------------------------------------------------------------------------
// Provisioning assisté du serveur PPPoE (livrable .rsc idempotent)
// ---------------------------------------------------------------------------

// pppIfaceRe / pppServiceRe — noms d'interface et de service-name : RouterOS
// accepte large mais le script généré est collé TEL QUEL dans un terminal —
// alphabet strict sans guillemet/backslash/$/espace.
var (
	pppIfaceRe   = regexp.MustCompile(`^[A-Za-z0-9._@:-]{1,32}$`)
	pppServiceRe = regexp.MustCompile(`^[A-Za-z0-9._@:-]{1,32}$`)
)

// handlePppProvisioningScript — GET
// /api/routers/{routerID}/ppp/provisioning-script?interface=…&service=…&profile=…&poolStart=…&poolEnd=…&localAddress=…&dns=…
// : génère le script d'installation du serveur PPPoE (phase B — D2 :
// « provisioning assisté »). AUCUN secret, AUCUNE commande filée : le WISP
// colle le script lui-même. Idempotent (garde [:len …] = 0 par bloc).
func (a *API) handlePppProvisioningScript(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("routerID")
	q := r.URL.Query()
	iface := strings.TrimSpace(q.Get("interface"))
	service := strings.TrimSpace(q.Get("service"))
	profile := strings.TrimSpace(q.Get("profile"))
	poolStart := strings.TrimSpace(q.Get("poolStart"))
	poolEnd := strings.TrimSpace(q.Get("poolEnd"))
	localAddress := strings.TrimSpace(q.Get("localAddress"))
	dns := strings.TrimSpace(q.Get("dns"))

	if iface == "" {
		writeErr(w, http.StatusBadRequest, "Interface du LAN à bridge (ex. ether2) requise")
		return
	}
	if !pppIfaceRe.MatchString(iface) {
		writeErr(w, http.StatusBadRequest, "Nom d'interface invalide (1-32 : lettres, chiffres, . _ : -)")
		return
	}
	if service == "" {
		service = "mikcloud"
	}
	if !pppServiceRe.MatchString(service) {
		writeErr(w, http.StatusBadRequest, "Service-name invalide (1-32 : lettres, chiffres, . _ : -)")
		return
	}
	if profile == "" {
		profile = "mikcloud-ppp"
	}
	if !model.ValidPppProfileName(profile) {
		writeErr(w, http.StatusBadRequest, "Nom de profil invalide (1-64 : lettres, chiffres, . _ - @)")
		return
	}
	if !model.ValidPppStaticIP(poolStart) || !model.ValidPppStaticIP(poolEnd) {
		writeErr(w, http.StatusBadRequest, "Plage du pool invalide (poolStart/poolEnd IPv4 attendus)")
		return
	}
	// Ordre numérique du pool (début < fin) : un range inversé serait
	// silencieusement vide côté RouterOS.
	aToI := func(ip string) uint32 {
		var b [4]byte
		fmt.Sscanf(ip, "%d.%d.%d.%d", &b[0], &b[1], &b[2], &b[3])
		return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	}
	if aToI(poolStart) >= aToI(poolEnd) {
		writeErr(w, http.StatusBadRequest, "La plage du pool est vide : poolStart doit précéder poolEnd")
		return
	}
	if localAddress != "" && !model.ValidPppStaticIP(localAddress) {
		writeErr(w, http.StatusBadRequest, "Adresse locale invalide (IPv4 attendu)")
		return
	}
	dnsList := []string{}
	if dns != "" {
		for _, d := range strings.Split(dns, ",") {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			if !model.ValidPppStaticIP(d) {
				writeErr(w, http.StatusBadRequest, "Serveur DNS invalide («"+d+"» : IPv4 attendu)")
				return
			}
			dnsList = append(dnsList, d)
		}
	}

	a.store.Lock()
	db := a.store.Data()
	rr := findRouterScoped(db, id, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur introuvable")
		return
	}
	name := rr.Name
	a.store.Unlock()

	poolName := "mikcloud-ppp-pool"
	var sb strings.Builder
	sb.WriteString("# ============================================================\n")
	sb.WriteString("# MikCloud — provisioning assisté du serveur PPPoE (phase B)\n")
	if name != "" {
		sb.WriteString("# Routeur : " + name + "\n")
	}
	sb.WriteString("# Généré le " + model.NowISO() + "\n")
	sb.WriteString("# À coller dans Terminal (Winbox) ou SSH. Idempotent : chaque bloc\n")
	sb.WriteString("# ne crée son objet que s'il est absent — relancer ne duplique rien.\n")
	sb.WriteString("# Vérification après application :\n")
	sb.WriteString("#   /interface pppoe-server server print\n")
	sb.WriteString("# ============================================================\n\n")
	// 1. Pool d'adresses des abonnés.
	sb.WriteString(`/ip pool
:if ([:len [find name="` + poolName + `"]] = 0) do={ add name="` + poolName + `" ranges="` + poolStart + "-" + poolEnd + `" }
`)
	// 2. Profil de base (le cloud ne crée pas les profils PPP D2 — le script
	// assisté reste un geste EXPLICITE du WISP, hors du canal agent).
	sb.WriteString(`/ppp profile
:if ([:len [find name="` + profile + `"]] = 0) do={ add name="` + profile + `"`)
	if localAddress != "" {
		sb.WriteString(` local-address=` + localAddress)
	}
	sb.WriteString(` remote-address=` + poolName)
	if len(dnsList) > 0 {
		sb.WriteString(` dns-server=` + strings.Join(dnsList, ","))
	}
	sb.WriteString(` only-one=yes }
`)
	// 3. Serveur PPPoE sur l'interface choisie.
	sb.WriteString(`/interface pppoe-server server
:if ([:len [find service-name="` + service + `"]] = 0) do={ add service-name="` + service + `" interface=` + iface + ` default-profile="` + profile + `" disabled=no one-session-per-host=yes }
`)
	sb.WriteString("\n# Les abonnés se créent ensuite dans la console MikCloud (Abonnés PPPoE)\n# — le profil « " + profile + " » est alors référencé tel quel.\n")

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "script": sb.String(),
		"warnings": []string{
			"Vérifiez l'interface (" + iface + ") : c'est le LAN où les abonnés établissent leur session PPPoE",
			"Le pool " + poolStart + "-" + poolEnd + " ne doit pas chevaucher vos réseaux existants (DHCP, VPN, WAN)",
		},
	})
}
