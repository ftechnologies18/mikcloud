package api

// Tests N°101 — Phase 3 Hotspot/HomeNet « les features maison » :
//   - garde d'usage : GET /api/devices n'existe que pour les comptes
//     homenet (404 côté hotspot — miroir des vues produit hotspot) ;
//   - inventaire end-to-end : le check-in d'un routeur agent d'un foyer
//     enfile read_dhcp (cadenceur), le rapport des bails nourrit le
//     registre (upsert par MAC, déduction « gone » sur rapport complet
//     uniquement), les comptes hotspot n'ont AUCUN cycle — et un rapport
//     F9 d'une box hotspot n'alimente JAMAIS le registre ;
//   - nom affecté : PUT /api/devices/{id} (borne 48 runes, vide légitime,
//     isolation multi-tenant) ;
//   - pause dîner : POST pause/unpause → état désiré + commande
//     device_pause en file, servie au check-in, signée au rapport VÉRIFIÉ
//     (rules == len(macs)), silence ensuite ;
//   - honnêteté du rapport : compte menteur (rules faux) et version
//     périmée (le parent change d'avis pendant le vol) ne signent PAS ;
//   - expiration : l'ensemble désiré change tout seul, le convergeur
//     re-file la levée ;
//   - le registre suit la box : suppression du routeur = purge.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// seedAgentRouter — routeur agent en ligne, version TLS-compatible, rattaché
// au compte donné (pattern agent_watch_test.go).
func seedAgentRouter(t *testing.T, st *store.Store, accID, id, name, tok string) {
	t.Helper()
	st.Lock()
	st.Data().Routers = append(st.Data().Routers, model.Router{
		ID: id, AccountID: accID, Name: name, Mode: "agent", Status: "online",
		Version: "7.20 (stable)", AgentTokenHash: agent.HashToken(tok), SchedulerSec: agentSleepSec,
		LastSeen: model.NowISO(),
	})
	st.Save()
	st.Unlock()
}

// deviceCmdID — relit l'ID d'une commande d'un kind donné depuis le
// commentaire d'audit du script du check-in (« # mikcloud cmd {id} {kind} »).
func deviceCmdID(t *testing.T, script, kind string) string {
	t.Helper()
	for _, line := range strings.Split(script, "\n") {
		if strings.Contains(line, "mikcloud cmd ") {
			parts := strings.Fields(strings.TrimSpace(line))
			if len(parts) >= 5 && parts[4] == kind {
				return parts[3]
			}
		}
	}
	return ""
}

// devicesOf — lignes du registre d'un routeur, directement dans le store
// (vérité interne, complémentaire de l'API).
func devicesOf(st *store.Store, routerID string) []model.Device {
	st.Lock()
	defer st.Unlock()
	out := []model.Device{}
	for _, d := range st.Data().Devices {
		if d.RouterID == routerID {
			out = append(out, d)
		}
	}
	return out
}

// routerPauseSig — signature de pause courante d'un routeur (store).
func routerPauseSig(st *store.Store, routerID string) string {
	st.Lock()
	defer st.Unlock()
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == routerID {
			return st.Data().Routers[i].PauseSig
		}
	}
	return ""
}

// queueReadDhcp — dépose une lecture read_dhcp à la demande (le geste de
// l'outil F9), servie au check-in suivant.
func queueReadDhcp(st *store.Store, accID, routerID, cmdID string) {
	st.Lock()
	st.Data().Commands = append(st.Data().Commands, model.Command{
		ID: cmdID, RouterID: routerID, AccountID: accID, Kind: model.CmdReadDhcp,
		Status: "queued", CreatedAt: model.NowISO(),
	})
	st.Save()
	st.Unlock()
}

// deviceCmdIDs — tous les IDs de commandes d'un kind servis dans le script
// d'un check-in (l'ordre du script, commentaires d'audit).
func deviceCmdIDs(t *testing.T, script, kind string) []string {
	t.Helper()
	ids := []string{}
	for _, line := range strings.Split(script, "\n") {
		if strings.Contains(line, "mikcloud cmd ") {
			parts := strings.Fields(strings.TrimSpace(line))
			if len(parts) >= 5 && parts[4] == kind {
				ids = append(ids, parts[3])
			}
		}
	}
	return ids
}

// reportServedDevicePauses — simule un routeur HONNÊTE : chaque commande
// device_pause servie dans le script rapporte rules = nombre de MAC de SON
// payload (un routeur réel applique ce qu'on lui envoie, ni plus ni moins).
// Renvoie le nombre de commandes rapportées.
func reportServedDevicePauses(t *testing.T, ts *httptest.Server, st *store.Store, tok, script string) int {
	t.Helper()
	n := 0
	for _, id := range deviceCmdIDs(t, script, model.CmdDevicePause) {
		st.Lock()
		macs := 0
		for _, c := range st.Data().Commands {
			if c.ID == id {
				macs = len(agent.DevicePauseMacsFromPayload(c.Payload))
			}
		}
		st.Unlock()
		agentReport(t, ts, tok, id, url.Values{"status": {"ok"}, "rules": {strconv.Itoa(macs)}})
		n++
	}
	return n
}

// TestDevicePauseScriptShape — le script device_pause est idempotent
// (remove-then-add des seules règles marquées), coupe par MAC en tête de
// chaîne (place-before=0 : au-dessus du fasttrack), pose le miroir IPv6
// best-effort et rapporte le compte de règles marquées IPv4 (vérité routeur).
func TestDevicePauseScriptShape(t *testing.T) {
	b := agent.Builder{BaseURL: "https://cloud.exemple", Token: "p4use-t0ken-abcdefghijkl"}

	script, err := b.ScriptFor(model.Command{ID: "c-dp1", Kind: model.CmdDevicePause,
		Payload: map[string]any{"macs": "AA:BB:CC:DD:EE:01;aa:bb:cc:dd:ee:02;", "sig": "dp-v1-abcd"}})
	if err != nil {
		t.Fatalf("ScriptFor(device_pause) : %v", err)
	}
	for _, want := range []string{
		`/ip firewall filter remove [find comment="` + agent.DevicePauseMarker + `"]`,
		`/ipv6 firewall filter remove [find comment="` + agent.DevicePauseMarker + `"]`,
		`src-mac-address="AA:BB:CC:DD:EE:01"`,
		`src-mac-address="AA:BB:CC:DD:EE:02"`, // normalisé majuscules
		`place-before=0`,
		`action=drop`,
		`&rules=". $dpr`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script device_pause doit contenir %q :\n%s", want, preview(script, 700))
		}
	}
	// Deux MAC = exactement 2 règles IPv4 (une par appareil), 2 miroirs IPv6.
	if got := strings.Count(script, "/ip firewall filter add"); got != 2 {
		t.Fatalf("2 règles IPv4 attendues, %d trouvées", got)
	}
	if got := strings.Count(script, "/ipv6 firewall filter add"); got != 2 {
		t.Fatalf("2 miroirs IPv6 attendus, %d trouvés", got)
	}

	// Ensemble vide : retrait seul (toutes les pauses levées), aucun add.
	script, _ = b.ScriptFor(model.Command{ID: "c-dp0", Kind: model.CmdDevicePause,
		Payload: map[string]any{"macs": ";", "sig": "dp-v1-vide"}})
	if strings.Contains(script, "filter add") {
		t.Fatalf("ensemble vide : aucune règle ne doit être posée :\n%s", preview(script, 400))
	}

	// Parse payload : séparateurs, casse, vides et MAC invalides filtrés.
	macs := agent.DevicePauseMacsFromPayload(map[string]any{"macs": "AA:BB:CC:DD:EE:01;;aa:bb:cc:dd:ee:02;XX:BB:CC:DD:EE:03;"})
	if len(macs) != 2 || macs[0] != "AA:BB:CC:DD:EE:01" || macs[1] != "AA:BB:CC:DD:EE:02" {
		t.Fatalf("DevicePauseMacsFromPayload = %v, attendu les 2 MAC valides normalisées", macs)
	}
}

// TestDevicesGuardedByUsage — la famille « devices » n'existe que pour les
// foyers : 404 pour un compte hotspot, 200 pour un compte homenet fraîchement
// inscrit publiquement (l'inscription HomeNet est OUVERTE depuis N°101).
func TestDevicesGuardedByUsage(t *testing.T) {
	ts := newTestServer(t)

	hsToken, _, _ := registerAccount(t, ts, "devices-hs-gerant", "")
	if status, out := doJSON(t, ts, "GET", "/api/devices", hsToken, nil); status != http.StatusNotFound {
		t.Fatalf("compte hotspot : GET /api/devices doit répondre 404, statut %d corps %v", status, out)
	}

	status, out := doJSON(t, ts, "POST", "/api/auth/register", "", registerUsageBody("devices-hn-famille", model.AccountUsageHomeNet))
	if status != http.StatusCreated {
		t.Fatalf("inscription homenet : statut %d, corps %v (ouverte depuis N°101)", status, out)
	}
	hnToken, _ := out["token"].(string)
	if status, _ := doJSON(t, ts, "GET", "/api/devices", hnToken, nil); status != http.StatusOK {
		t.Fatalf("compte homenet : GET /api/devices doit répondre 200, statut %d", status)
	}
	// (le corps est un tableau JSON — le registre vide se vérifie au store
	// et au navigateur ; ici le contrat de garde suffit)
}

// TestHomeDeviceInventoryEndToEnd — le flux complet de l'inventaire :
// check-in (read_dhcp enfilé par le cadenceur foyer) → rapport de bails →
// registre (upsert MAC, IP rafraîchie) → déduction « gone » sur rapport
// COMPLET (ligne conservée) → cadence (pas de re-file immédiat) → silence
// côté hotspot (aucun cycle cadencé, aucun registre alimenté par l'outil F9).
func TestHomeDeviceInventoryEndToEnd(t *testing.T) {
	st, ts := newTestServerWithStore(t)

	// Foyer inscrit publiquement (N°101) + box agent.
	status, out := doJSON(t, ts, "POST", "/api/auth/register", "", registerUsageBody("invent-famille", model.AccountUsageHomeNet))
	if status != http.StatusCreated {
		t.Fatalf("inscription foyer : %d %v", status, out)
	}
	hnToken, _ := out["token"].(string)
	user, _ := out["user"].(map[string]any)
	accID, _ := user["accountId"].(string)
	_ = hnToken
	const tok = "inv3nt-t0ken-abcdefghijkl"
	seedAgentRouter(t, st, accID, "r-inv", "MAISON TEST", tok)

	// 1) Check-in : le cadenceur foyer enfile read_dhcp dans LE MÊME check-in.
	body := agentCheckIn(t, ts, tok)
	cmdID := deviceCmdID(t, body, model.CmdReadDhcp)
	if cmdID == "" {
		t.Fatalf("le check-in d'une box de foyer doit servir read_dhcp :\n%s", preview(body, 500))
	}

	// 2) Rapport : 2 baux (une TV, un téléphone) — et rapport des éventuelles
	// device_pause servies (ensemble vide du convergeur : un routeur honnête
	// rapporte tout, aucune commande ne reste en vol).
	agentReport(t, ts, tok, cmdID, url.Values{"status": {"ok"},
		"data": {"AA:BB:CC:DD:EE:01|192.168.88.10|tv-salon|30m|bound;AA:BB:CC:DD:EE:02|192.168.88.11|tel-mama|25m|bound;"}})
	reportServedDevicePauses(t, ts, st, tok, body)
	devs := devicesOf(st, "r-inv")
	if len(devs) != 2 {
		t.Fatalf("2 appareils attendus après rapport, %d", len(devs))
	}
	var tv model.Device
	for _, d := range devs {
		if d.MAC == "AA:BB:CC:DD:EE:01" {
			tv = d
		}
	}
	if tv.IP != "192.168.88.10" || tv.Hostname != "tv-salon" || tv.Status != "bound" || tv.Expires != "30m" {
		t.Fatalf("champs du bail mal appliqués : %+v", tv)
	}
	if tv.AccountID != accID || tv.RouterName != "MAISON TEST" || tv.CreatedAt == "" || tv.LeaseAt == "" {
		t.Fatalf("scoping du registre : %+v", tv)
	}

	// 3) Cadence : le check-in suivant ne re-file PAS (devicesDone frais).
	body2 := agentCheckIn(t, ts, tok)
	if id := deviceCmdID(t, body2, model.CmdReadDhcp); id != "" {
		t.Fatalf("rapport fraîchement appliqué : aucun read_dhcp ne doit être re-servi : %q", preview(body2, 300))
	}

	// 4) Rapport complet SANS la TV (bail retiré côté box) + IP du tel
	// changée : la TV passe « gone » (ligne conservée pour le nom), le tel
	// est rafraîchi. Lecture déposée à la demande (geste outil F9).
	queueReadDhcp(st, accID, "r-inv", "c-inv2")
	served := ""
	for i := 0; i < 3 && served == ""; i++ {
		body3 := agentCheckIn(t, ts, tok)
		if id := deviceCmdID(t, body3, model.CmdReadDhcp); id == "c-inv2" {
			served = id
		}
	}
	if served == "" {
		t.Fatal("la lecture read_dhcp déposée n'a jamais été servie")
	}
	agentReport(t, ts, tok, served, url.Values{"status": {"ok"},
		"data": {"AA:BB:CC:DD:EE:02|192.168.88.99|tel-mama|24m|bound;"}})
	devs = devicesOf(st, "r-inv")
	if len(devs) != 2 {
		t.Fatalf("la ligne « gone » doit être CONSERVÉE (le nom y survit), %d lignes", len(devs))
	}
	for _, d := range devs {
		if d.MAC == "AA:BB:CC:DD:EE:01" && d.Status != model.DeviceLeaseGone {
			t.Fatalf("bail absent d'un rapport complet : statut %q, attendu gone", d.Status)
		}
		if d.MAC == "AA:BB:CC:DD:EE:02" && d.IP != "192.168.88.99" {
			t.Fatalf("IP non rafraîchie : %s", d.IP)
		}
	}

	// 5) Côté hotspot : le check-in d'une box de compte hotspot ne dépose
	// AUCUN read_dhcp cadencé — et un rapport F9 (clic outil) n'alimente
	// PAS le registre (le registre est familial, jamais un parc public).
	_, hsAccID, _ := registerAccount(t, ts, "invent-hs-gerant", "")
	const htok = "inv3nt-t0ken-hotspot-abc"
	seedAgentRouter(t, st, hsAccID, "r-inv-hs", "CYBER TEST", htok)
	bodyHs := agentCheckIn(t, ts, htok)
	if id := deviceCmdID(t, bodyHs, model.CmdReadDhcp); id != "" {
		t.Fatalf("une box de compte hotspot ne doit recevoir AUCUN read_dhcp cadencé :\n%s", preview(bodyHs, 400))
	}
	queueReadDhcp(st, hsAccID, "r-inv-hs", "c-inv-hs")
	bodyHs2 := agentCheckIn(t, ts, htok)
	idHS := deviceCmdID(t, bodyHs2, model.CmdReadDhcp)
	if idHS == "" {
		t.Fatal("la commande read_dhcp déposée pour la box hotspot doit être servie (outil F9)")
	}
	agentReport(t, ts, htok, idHS, url.Values{"status": {"ok"},
		"data": {"11:22:33:44:55:66|10.5.50.9|client-wifi|10m|bound;"}})
	if devs := devicesOf(st, "r-inv-hs"); len(devs) != 0 {
		t.Fatalf("un compte hotspot ne doit alimenter AUCUN appareil : %d lignes", len(devs))
	}
}

// TestDeviceRenameAndPauseFlow — nom affecté + pause dîner + convergence :
// renommage (borne, vide légitime, isolation), pause 30 min (état désiré +
// commande en file, servie au check-in, signée au rapport vérifié, silence
// ensuite), rapport menteur (aucune signature), reprise (ensemble vide,
// signature vide), expiration (levée re-filée), purge à la suppression de
// la box.
func TestDeviceRenameAndPauseFlow(t *testing.T) {
	st, ts := newTestServerWithStore(t)

	status, out := doJSON(t, ts, "POST", "/api/auth/register", "", registerUsageBody("pause-famille", model.AccountUsageHomeNet))
	if status != http.StatusCreated {
		t.Fatalf("inscription foyer : %d", status)
	}
	hnToken, _ := out["token"].(string)
	user, _ := out["user"].(map[string]any)
	accID, _ := user["accountId"].(string)
	const tok = "p4use-t0ken-flot-abcdef"
	seedAgentRouter(t, st, accID, "r-pause", "MAISON PAUSE", tok)

	// Appareil découvert par le chemin réel : check-in (cadenceur) + rapport.
	// Le check-in initial sert AUSSI la device_pause à ensemble vide (le
	// convergeur signe l'état « aucune pause » et nettoie d'éventuelles
	// règles résiduelles — un routeur honnête rapporte TOUT ce qu'il exécute).
	body := agentCheckIn(t, ts, tok)
	cmdID := deviceCmdID(t, body, model.CmdReadDhcp)
	if cmdID == "" {
		t.Fatalf("le cadenceur doit servir read_dhcp :\n%s", preview(body, 400))
	}
	agentReport(t, ts, tok, cmdID, url.Values{"status": {"ok"},
		"data": {"AA:BB:CC:DD:EE:10|192.168.88.10|tel-enfant|30m|bound;"}})
	reportServedDevicePauses(t, ts, st, tok, body)
	devs := devicesOf(st, "r-pause")
	if len(devs) != 1 {
		t.Fatalf("1 appareil attendu, %d", len(devs))
	}
	devID := devs[0].ID

	// — Renommage : borne 48 runes, vide légitime, persistance —
	if status, out := doJSON(t, ts, "PUT", "/api/devices/"+devID, hnToken, map[string]string{"name": "Télé de la petite"}); status != http.StatusOK {
		t.Fatalf("renommage : %d %v", status, out)
	}
	long := strings.Repeat("é", 60)
	if status, _ := doJSON(t, ts, "PUT", "/api/devices/"+devID, hnToken, map[string]string{"name": long}); status != http.StatusBadRequest {
		t.Fatalf("nom > 48 runes : attendu 400, statut %d", status)
	}
	if devicesOf(st, "r-pause")[0].Name != "Télé de la petite" {
		t.Fatalf("nom non persisté : %q", devicesOf(st, "r-pause")[0].Name)
	}
	// Appareil d'un autre compte (hotspot ET homenet) : 404.
	other, _, _ := registerAccount(t, ts, "pause-autre", "")
	if status, _ := doJSON(t, ts, "PUT", "/api/devices/"+devID, other, map[string]string{"name": "vol"}); status != http.StatusNotFound {
		t.Fatalf("appareil d'un autre compte : attendu 404, statut %d", status)
	}

	// — Pause dîner 30 min : état désiré + device_pause en file —
	status, out = doJSON(t, ts, "POST", "/api/devices/"+devID+"/pause", hnToken, map[string]any{"paused": true, "minutes": 30})
	if status != http.StatusOK {
		t.Fatalf("pause : %d %v", status, out)
	}
	if paused, _ := out["paused"].(bool); !paused {
		t.Fatal("pause : la réponse doit porter paused=true")
	}
	until, _ := out["pausedUntil"].(string)
	if until == "" {
		t.Fatal("pause bornée : pausedUntil doit être posé (RFC3339)")
	}
	if u, err := time.Parse(time.RFC3339, until); err != nil {
		t.Fatalf("pausedUntil illisible : %q", until)
	} else if m := time.Until(u).Minutes(); m <= 29 || m > 31 {
		t.Fatalf("pause 30 min : échéance à %.1f min", m)
	}

	// La commande device_pause est en file avec la MAC + la signature courante.
	st.Lock()
	var queued *model.Command
	for i := range st.Data().Commands {
		c := &st.Data().Commands[i]
		if c.Kind == model.CmdDevicePause && c.RouterID == "r-pause" && c.Status == "queued" {
			queued = c
		}
	}
	st.Unlock()
	if queued == nil {
		t.Fatal("pause posée : une commande device_pause doit être en file")
	}
	if macs := agent.DevicePauseMacsFromPayload(queued.Payload); len(macs) != 1 || macs[0] != "AA:BB:CC:DD:EE:10" {
		t.Fatalf("payload device_pause = %v, attendu la MAC de l'appareil", macs)
	}
	wantSig := devicePauseSig([]string{"AA:BB:CC:DD:EE:10"})

	// Servie au check-in, signée au rapport VÉRIFIÉ (rules == 1). NOTE : le
	// premier check-in d'une box homenet sert AUSSI la device_pause à
	// ensemble VIDE du convergeur (PauseSig vierge ≠ signature vide : elle
	// nettoie d'éventuelles règles mikcloud-pause résiduelles d'avant
	// MikCloud — même contrat que safewifi off) ; le routeur honnête
	// rapporte chacune.
	body = agentCheckIn(t, ts, tok)
	if ids := deviceCmdIDs(t, body, model.CmdDevicePause); len(ids) == 0 {
		t.Fatalf("device_pause doit être servie au check-in :\n%s", preview(body, 400))
	}
	reportServedDevicePauses(t, ts, st, tok, body)
	if sig := routerPauseSig(st, "r-pause"); sig != wantSig {
		t.Fatalf("signature de pause = %q, attendu %q (rapport vérifié)", sig, wantSig)
	}

	// Convergence : le check-in suivant ne re-file PAS (même signature).
	body = agentCheckIn(t, ts, tok)
	if len(deviceCmdIDs(t, body, model.CmdDevicePause)) != 0 {
		t.Fatal("pause convergée : aucune nouvelle device_pause ne doit être servie")
	}

	// — Rapport menteur (rules=7 alors que 1 MAC envoyée) : PAS de
	// signature — la fossilisation force le re-file, le compte faux ne
	// signe pas (post-mortem N°93 : le compte exact n'est pas une théorie).
	st.Lock()
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-pause" {
			st.Data().Routers[i].PauseSig = "dp-v1-fossile"
		}
	}
	st.Save()
	st.Unlock()
	body = agentCheckIn(t, ts, tok)
	ids := deviceCmdIDs(t, body, model.CmdDevicePause)
	if len(ids) == 0 {
		t.Fatal("signature divergente : device_pause doit être re-filée")
	}
	for _, id := range ids {
		agentReport(t, ts, tok, id, url.Values{"status": {"ok"}, "rules": {"7"}}) // menteur : rules faux
	}
	if sig := routerPauseSig(st, "r-pause"); sig != "dp-v1-fossile" {
		t.Fatalf("un compte de règles menteur ne doit RIEN signer : %q", sig)
	}

	// — Reprise : ensemble vide, rapport rules=0, signature vide-sig —
	if status, _ := doJSON(t, ts, "POST", "/api/devices/"+devID+"/pause", hnToken, map[string]any{"paused": false}); status != http.StatusOK {
		t.Fatalf("reprise : statut %d", status)
	}
	body = agentCheckIn(t, ts, tok)
	if ids := deviceCmdIDs(t, body, model.CmdDevicePause); len(ids) == 0 {
		t.Fatal("reprise : la levée doit être servie (ensemble vide)")
	}
	reportServedDevicePauses(t, ts, st, tok, body)
	if sig := routerPauseSig(st, "r-pause"); sig != devicePauseSig(nil) {
		t.Fatalf("levée convergée : signature = %q, attendu la signature vide", sig)
	}

	// — Expiration : l'état EFFECTIF se lève tout seul (PauseActiveAt) et
	// le convergeur re-file la levée (l'ensemble désiré a changé sans
	// aucun geste humain — miroir FamilyGuard aux frontières de fenêtre).
	st.Lock()
	for i := range st.Data().Devices {
		if st.Data().Devices[i].ID == devID {
			st.Data().Devices[i].Paused = true
			st.Data().Devices[i].PausedUntil = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
		}
	}
	for i := range st.Data().Routers {
		if st.Data().Routers[i].ID == "r-pause" {
			st.Data().Routers[i].PauseSig = wantSig // l'ancienne pause est encore « appliquée »
		}
	}
	st.Save()
	st.Unlock()
	if devs := devicesOf(st, "r-pause"); len(devs) != 1 || devs[0].PauseActiveAt(time.Now().UTC()) {
		t.Fatal("pause expirée : l'état effectif doit déjà être « rétabli »")
	}
	body = agentCheckIn(t, ts, tok)
	if len(deviceCmdIDs(t, body, model.CmdDevicePause)) == 0 {
		t.Fatal("pause expirée : la levée doit être re-filée par le convergeur (ensemble désiré changé)")
	}
	reportServedDevicePauses(t, ts, st, tok, body)

	// — Le registre suit la box : suppression du routeur = purge —
	if status, out := doJSON(t, ts, "DELETE", "/api/routers/r-pause", hnToken, nil); status != http.StatusOK {
		t.Fatalf("suppression du routeur : %d %v", status, out)
	}
	if devs := devicesOf(st, "r-pause"); len(devs) != 0 {
		t.Fatalf("le registre doit suivre la box (purge à la suppression), %d lignes restantes", len(devs))
	}
}

// TestDevicePauseStaleVersionNotSigned — le parent change d'avis PENDANT le
// vol : la version envoyée n'est plus désirée au rapport → aucune signature
// (l'état périmé ne se fige jamais — pattern SafeWiFi N°80), puis le
// check-in suivant re-file la version courante.
func TestDevicePauseStaleVersionNotSigned(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	status, out := doJSON(t, ts, "POST", "/api/auth/register", "", registerUsageBody("stale-famille", model.AccountUsageHomeNet))
	if status != http.StatusCreated {
		t.Fatalf("inscription : %d", status)
	}
	user, _ := out["user"].(map[string]any)
	accID, _ := user["accountId"].(string)
	const tok = "st4le-t0ken-abcdef"
	seedAgentRouter(t, st, accID, "r-stale", "MAISON STALE", tok)

	// Deux appareils en pause, commande EN VOL avec sig(2 MAC).
	st.Lock()
	st.Data().Devices = append(st.Data().Devices,
		model.Device{ID: "dev-a", AccountID: accID, RouterID: "r-stale", MAC: "AA:BB:CC:DD:EE:A1", Paused: true},
		model.Device{ID: "dev-b", AccountID: accID, RouterID: "r-stale", MAC: "AA:BB:CC:DD:EE:B2", Paused: true})
	want2 := devicePauseSig([]string{"AA:BB:CC:DD:EE:A1", "AA:BB:CC:DD:EE:B2"})
	st.Data().Commands = append(st.Data().Commands, model.Command{
		ID: "c-stale", RouterID: "r-stale", AccountID: accID, Kind: model.CmdDevicePause,
		Status: "sent", CreatedAt: model.NowISO(),
		Payload: map[string]any{"macs": "AA:BB:CC:DD:EE:A1;AA:BB:CC:DD:EE:B2;", "sig": want2},
	})
	st.Save()
	st.Unlock()

	// Le rapport arrive avec rules=2 (honnête)… mais entre-temps le parent
	// a levé la pause d'un appareil : la version envoyée est périmée.
	st.Lock()
	st.Data().Devices[0].Paused = false
	st.Save()
	st.Unlock()
	agentReport(t, ts, tok, "c-stale", url.Values{"status": {"ok"}, "rules": {"2"}})
	if sig := routerPauseSig(st, "r-stale"); sig == want2 {
		t.Fatal("version périmée : la signature ne doit PAS être posée (re-file au check-in)")
	}

	// Le check-in suivant re-file la version courante (1 MAC) et la signe.
	body := agentCheckIn(t, ts, tok)
	if id := deviceCmdID(t, body, model.CmdDevicePause); id == "" {
		t.Fatal("après péremption : device_pause doit être re-filée avec l'ensemble courant")
	} else {
		agentReport(t, ts, tok, id, url.Values{"status": {"ok"}, "rules": {"1"}})
	}
	if want1 := devicePauseSig([]string{"AA:BB:CC:DD:EE:B2"}); routerPauseSig(st, "r-stale") != want1 {
		t.Fatalf("signature courante = %q, attendu %q", routerPauseSig(st, "r-stale"), want1)
	}
}
