// handlers_devices.go — N°101, Phase 3 Hotspot/HomeNet : les appareils du
// foyer (registre des bails DHCP + noms affectés + pause dîner).
//
// La vue Appareils de la Phase 2 vivait du PONT de données des sessions
// hotspot (GET /api/sessions) — un pis-aller : un foyer n'a PAS de portail
// captif, ses appareils rejoignent le WiFi et reçoivent un bail DHCP du
// routeur. La Phase 3 leur donne LEUR source de vérité :
//
//   - l'agent rapporte /ip dhcp-server lease (commande read_dhcp, déjà
//     construite pour l'outil F9) sur un cadenceur dédié aux routeurs agent
//     de comptes HOMENET (ensureHomeDevicesLocked — les parcs hotspot ne
//     paient pas ce cycle : leurs clics DHCP restent du cache outil F9) ;
//   - chaque bail devient une ligne « device » (upsert par MAC — l'identité
//     stable), la famille lui affecte un nom (PUT), et peut couper son
//     internet pour un temps donné (POST pause — la pause dîner) ;
//   - la pause est un ÉTAT DÉSIRÉ cloud (pattern FamilyGuard N°82) : la
//     commande agent device_pause fait converger le routeur, l'expiration
//     est recalculée à chaque check-in (aucune horloge routeur consultée).
package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// devicesMinInterval — N°101 — cadence du cycle read_dhcp d'un foyer : même
// pas que read_state (2 min). Un inventaire domestique est petit (≤ 100
// bails côté script), le coût est une commande de lecture légère (~1 Ko)
// toutes les ~2,7 check-ins — le régime egress N°74/N°75 reste la règle.
const devicesMinInterval = 2 * time.Minute

// deviceReadCap — le script read_dhcp rapporte AU PLUS 100 baux (borne F9).
// Un rapport de TAILLE EXACTEMENT 100 est potentiellement tronqué : aucune
// déduction « absent = parti » ne s'applique alors (honnêteté v2/v4 — on ne
// badgue jamais sur un rapport incomplet).
const deviceReadCap = 100

// ---------------------------------------------------------------------------
// GET /api/devices — le registre du foyer
// ---------------------------------------------------------------------------

func (a *API) handleDevicesList(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	now := time.Now().UTC()
	a.store.Lock()
	db := a.store.Data()
	devices := make([]model.Device, 0, len(db.Devices))
	for i := range db.Devices {
		d := db.Devices[i]
		if d.AccountID != acc {
			continue
		}
		// Le nom du routeur est rafraîchi à la volée (un renommage de la box
		// ne laisse jamais d'étiquette périmée dans le registre).
		if rr := findRouterScoped(db, d.RouterID, acc); rr != nil {
			d.RouterName = rr.Name
		}
		// État EFFECTIF de la pause à l'instant de la lecture (une pause
		// expirée s'affiche déjà levée — le check-in suivant la lèvera côté
		// routeur, l'UI ne ment jamais dans l'autre sens).
		d.Paused = d.PauseActiveAt(now)
		devices = append(devices, d)
	}
	a.store.Unlock()

	// Ordre de lecture familiale : en ligne d'abord, puis en pause, puis le
	// reste ; au sein de chaque groupe, par nom affiché (nom affecté, sinon
	// host-name, sinon MAC — même repli que l'UI).
	deviceDisplayName := func(d model.Device) string {
		if d.Name != "" {
			return d.Name
		}
		if d.Hostname != "" {
			return d.Hostname
		}
		return d.MAC
	}
	sort.Slice(devices, func(i, j int) bool {
		pi, pj := devicePauseRank(devices[i]), devicePauseRank(devices[j])
		if pi != pj {
			return pi < pj
		}
		return strings.ToLower(deviceDisplayName(devices[i])) < strings.ToLower(deviceDisplayName(devices[j]))
	})
	// N°75 — ETag/304 : la vue Appareils poll tant qu'elle est ouverte ; le
	// corps ne change qu'au cycle read_dhcp suivant (~2 min) ou à une action.
	writeJSONCacheable(w, r, http.StatusOK, devices)
}

// devicePauseRank — ordre d'affichage : en ligne (0) < en pause (1) < hors
// ligne (2). Un appareil en pause MAIS hors ligne reste visible en groupe
// pause (l'action attend le retour de l'appareil).
func devicePauseRank(d model.Device) int {
	if model.DeviceOnline(d.Status) {
		return 0
	}
	if d.Paused {
		return 1
	}
	return 2
}

// ---------------------------------------------------------------------------
// PUT /api/devices/{id} — le nom affecté (« TV du salon », « Tel de mama »)
// ---------------------------------------------------------------------------

func (a *API) handleDeviceRename(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	name := strings.TrimSpace(req.Name)
	// Un nom vide est LÉGITIME (retour à l'anonymat : host-name/MAC) — seule
	// la BORNE est rejetée (48 caractères après trim, runes : les accents
	// comptent pour un, un nom de famille s'écrit avec).
	if len([]rune(name)) > model.DeviceNameMax {
		writeErr(w, http.StatusBadRequest, "Nom trop long (48 caractères maximum)")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	idx := findDeviceScoped(db, id, acc)
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Appareil introuvable")
		return
	}
	d := &db.Devices[idx]
	old := d.Name
	d.Name = name
	if old != name {
		label := name
		if label == "" {
			label = d.MAC
		}
		a.logActivityBy(r, db, acc, "device", "Appareil renommé : "+label)
	}
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name})
}

// ---------------------------------------------------------------------------
// POST /api/devices/{id}/pause — la pause dîner
// ---------------------------------------------------------------------------

// Durées proposées par l'UI (minutes). 0 = illimité (« jusqu'à réactivation »).
// Le serveur accepte toute valeur 0..1440 (une journée) : le contrat reste
// simple, l'UI curate les raccourcis.
const (
	devicePauseMaxMinutes = 1440
)

func (a *API) handleDevicePause(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	id := r.PathValue("id")
	var req struct {
		Paused  bool `json:"paused"`
		Minutes int  `json:"minutes"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	if req.Minutes < 0 || req.Minutes > devicePauseMaxMinutes {
		writeErr(w, http.StatusBadRequest, "Durée de pause invalide (0 à 1440 minutes)")
		return
	}
	a.store.Lock()
	db := a.store.Data()
	idx := findDeviceScoped(db, id, acc)
	if idx < 0 {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Appareil introuvable")
		return
	}
	d := &db.Devices[idx]
	rr := findRouterScoped(db, d.RouterID, acc)
	if rr == nil {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Routeur de l'appareil introuvable")
		return
	}
	now := time.Now().UTC()
	until := ""
	if req.Paused {
		d.Paused = true
		if req.Minutes > 0 {
			until = now.Add(time.Duration(req.Minutes) * time.Minute).Format(time.RFC3339)
		}
		d.PausedUntil = until // "" = illimité
	} else {
		d.Paused = false
		d.PausedUntil = ""
	}
	// Convergence immédiate : la commande device_pause est (re)posée dans la
	// file avec l'ensemble désiré COURANT — servie au prochain check-in de la
	// box (≤ 45 s console ouverte, ≤ 180 s en veille — l'appel console vient
	// de marquer le routeur « sous attention », N°75).
	a.queueDevicePauseLocked(db, rr)
	label := d.Name
	if label == "" {
		if d.Hostname != "" {
			label = d.Hostname
		} else {
			label = d.MAC
		}
	}
	if req.Paused {
		if req.Minutes > 0 {
			a.logActivityBy(r, db, acc, "device", "Pause dîner : internet de "+label+" coupé ("+strconv.Itoa(req.Minutes)+" min, en attente de la box)")
		} else {
			a.logActivityBy(r, db, acc, "device", "Pause dîner : internet de "+label+" coupé (jusqu'à réactivation, en attente de la box)")
		}
	} else {
		a.logActivityBy(r, db, acc, "device", "Internet de "+label+" rétabli (en attente de la box)")
	}
	// Instantanés AVANT déverrouillage : d pointe dans db.Devices (le verrou
	// du store est la seule protection de ces champs — jamais lus après).
	paused, pausedUntil := d.Paused, d.PausedUntil
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "paused": paused, "pausedUntil": pausedUntil})
}

// ---------------------------------------------------------------------------
// Convergence device_pause (pattern FamilyGuard : le cloud calcule, le
// routeur converge — jamais d'horloge routeur consultée)
// ---------------------------------------------------------------------------

// desiredPauseMacsLocked — MAC des appareils de CE routeur dont la pause est
// effective à `now` (demandée ET non expirée), triées (signature stable).
// À appeler sous le verrou du store.
func desiredPauseMacsLocked(db *model.DB, routerID string, now time.Time) []string {
	macs := []string{}
	for i := range db.Devices {
		d := &db.Devices[i]
		if d.RouterID == routerID && d.PauseActiveAt(now) {
			macs = append(macs, d.MAC)
		}
	}
	sort.Strings(macs)
	return macs
}

// DevicePauseVersion — sel de version de la forme des règles (pattern
// safeWifiRulesVersion) : toute évolution du script (nouveau marquage,
// paramètre supplémentaire) bump ce sel → chaque routeur en ligne reçoit la
// nouvelle forme au check-in suivant.
const DevicePauseVersion = "dp-v1"

// devicePauseSig — signature de l'ensemble pause : version + hash de la
// liste triée. Deux foyers aux mêmes pauses partagent la même signature —
// elle ne contient AUCUNE donnée compte/routeur.
func devicePauseSig(macs []string) string {
	h := sha256.Sum256([]byte(DevicePauseVersion + "|" + strings.Join(macs, ";")))
	return DevicePauseVersion + "-" + hex.EncodeToString(h[:8])
}

// queueDevicePauseLocked — dépose (ou met à jour) la commande device_pause
// pour CE routeur avec l'ensemble désiré COURANT. Une commande déjà « queued »
// est RÉÉCRITE (payload frais : deux pauses posées à la suite partagent le
// prochain check-in au lieu de le doubler) ; une commande « sent » est laissée
// (son rapport sera vérifié contre la signature désirée — périmée = re-file,
// pattern SafeWiFi « le gérant change d'avis pendant le vol »).
// À appeler sous le verrou ; Save à charge de l'appelant.
func (a *API) queueDevicePauseLocked(db *model.DB, router *model.Router) {
	now := time.Now().UTC()
	macs := desiredPauseMacsLocked(db, router.ID, now)
	payload := map[string]any{
		"macs": strings.Join(macs, ";") + ";",
		"sig":  devicePauseSig(macs),
	}
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == router.ID && c.Kind == model.CmdDevicePause && c.Status == "queued" {
			c.Payload = payload
			return
		}
	}
	queueCommandLocked(db, router.AccountID, router.ID, model.CmdDevicePause, payload)
}

// ensureDevicePauseLocked — N°101 — convergeur check-in de la pause dîner :
// la signature désirée est recalculée à CHAQUE check-in (une pause qui
// expire change l'ensemble → re-file, la coupure se lève d'elle-même) et
// comparée à celle APPLIQUÉE (Router.PauseSig) ; divergence → device_pause
// en file (idempotent, servie dans CE check-in). HOMENET uniquement : les
// routeurs hotspot n'ont ni appareil ni pause — aucun bruit de file.
// À appeler sous le verrou depuis handleAgentCmd.
func (a *API) ensureDevicePauseLocked(db *model.DB, router *model.Router) {
	if router.Mode != "agent" || accountUsageLocked(db, router.AccountID) != model.AccountUsageHomeNet {
		return
	}
	now := time.Now().UTC()
	sig := devicePauseSig(desiredPauseMacsLocked(db, router.ID, now))
	if router.PauseSig == sig {
		return // convergé (y compris l'ensemble vide : rien à couper)
	}
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == router.ID && c.Kind == model.CmdDevicePause && (c.Status == "queued" || c.Status == "sent") {
			if s, _ := c.Payload["sig"].(string); s == sig {
				return // la bonne version est déjà en vol
			}
			break // version périmée en vol : file la fraîche (l'idempotence la fera gagner)
		}
	}
	a.queueDevicePauseLocked(db, router)
}

// ensureHomeDevicesLocked — N°101 — cadenceur de l'inventaire : enfile un
// read_dhcp pour CE routeur si (1) c'est un routeur agent d'un compte
// HOMENET (les parcs hotspot ne paient pas ce cycle — leurs données DHCP
// restent de l'outil F9 à la demande), (2) aucun n'est déjà en file/vol,
// (3) le dernier rapport appliqué date de plus de devicesMinInterval.
// À appeler sous le verrou depuis handleAgentCmd.
func (a *API) ensureHomeDevicesLocked(db *model.DB, router *model.Router) {
	if router.Mode != "agent" || accountUsageLocked(db, router.AccountID) != model.AccountUsageHomeNet {
		return
	}
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == router.ID && c.Kind == model.CmdReadDhcp && (c.Status == "queued" || c.Status == "sent") {
			return // déjà en file ou en vol (clic F9 compris : jamais deux lectures)
		}
	}
	if t, ok := a.devicesDone[router.ID]; ok && time.Since(t) < devicesMinInterval {
		return // pas encore dû
	}
	queueCommandLocked(db, router.AccountID, router.ID, model.CmdReadDhcp, map[string]any{})
}

// ---------------------------------------------------------------------------
// Application du rapport read_dhcp → registre des appareils
// ---------------------------------------------------------------------------

// applyDeviceLeases — N°101 — upsert des bails rapportés dans le registre :
//   - MAC inconnue → création (le nom affecté naît vide : la famille nomme) ;
//   - MAC connue → IP/host-name/statut/bail rafraîchis (host-name vide
//     conservé : certains appareils ne le rapportent qu'une fois) ;
//   - rapport COMPLET (moins de baux que la borne du script) → les appareils
//     du routeur ABSENTS du rapport passent « gone » (bail retiré côté box) —
//     la ligne est conservée : le nom de famille y survit, l'appareil qui
//     revient retrouve son identité au bail suivant.
//
// Renvoie le nombre d'appareils découverts. À appeler sous le verrou ;
// Save à charge de l'appelant.
func (a *API) applyDeviceLeases(db *model.DB, router *model.Router, vals map[string][]string) int {
	raw := ""
	if vs := vals["data"]; len(vs) > 0 {
		raw = vs[0]
	}
	rows := splitAgentList(raw)
	now := model.NowISO()
	seen := map[string]bool{}
	created := 0
	for _, e := range rows {
		if len(e) < 5 {
			continue
		}
		mac := model.NormalizeMAC(e[0])
		if mac == "" {
			continue // rapport corrompu : jamais de ligne fantôme
		}
		seen[mac] = true
		ip := bounded(e[1], 45)
		host := bounded(e[2], 64)
		expires := bounded(e[3], 24)
		status := bounded(e[4], 16)
		idx := -1
		for i := range db.Devices {
			if db.Devices[i].RouterID == router.ID && db.Devices[i].MAC == mac {
				idx = i
				break
			}
		}
		if idx < 0 {
			db.Devices = append(db.Devices, model.Device{
				ID: model.NewID("dev-"), AccountID: router.AccountID, RouterID: router.ID,
				RouterName: router.Name, MAC: mac, Hostname: host, IP: ip, Status: status,
				Expires: expires, LeaseAt: now, CreatedAt: now,
			})
			created++
		} else {
			d := &db.Devices[idx]
			d.RouterName = router.Name
			if ip != "" {
				d.IP = ip
			}
			if host != "" {
				d.Hostname = host
			}
			d.Status = status
			d.Expires = expires
			d.LeaseAt = now
		}
	}
	// Déduction « gone » UNIQUEMENT sur rapport complet (borne F9 = 100) :
	// un rapport tronqué ne badgue jamais (honnêteté, pattern read_state).
	if len(rows) < deviceReadCap {
		for i := range db.Devices {
			d := &db.Devices[i]
			if d.RouterID == router.ID && !seen[d.MAC] && d.Status != model.DeviceLeaseGone {
				d.Status = model.DeviceLeaseGone
			}
		}
	}
	return created
}

// findDeviceScoped — appareil par ID dans le périmètre du compte (404 sinon).
// Sous verrou.
func findDeviceScoped(db *model.DB, id, acc string) int {
	for i := range db.Devices {
		if db.Devices[i].ID == id && db.Devices[i].AccountID == acc {
			return i
		}
	}
	return -1
}

// bounded — trim + borne défensive d'un champ de rapport agent.
func bounded(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		s = s[:n]
	}
	return s
}

// applyDevicePauseResult — N°101 — vérifie et signe le retour « ok » de la
// commande device_pause (vérité routeur : le compte de règles marquées
// IPv4 APRÈS application doit égaler le nombre de MAC du payload) et pose
// Router.PauseSig si la version envoyée est TOUJOURS la version désirée.
// Sous verrou ; l'appelant journalise.
func (a *API) applyDevicePauseResult(db *model.DB, router *model.Router, cmd *model.Command, vals map[string][]string) (applied int) {
	macs := agent.DevicePauseMacsFromPayload(cmd.Payload)
	sentSig := agent.DevicePauseSigFromPayload(cmd.Payload)
	rules := 0
	if vs := vals["rules"]; len(vs) > 0 {
		if n, err := strconv.Atoi(strings.TrimSpace(vs[0])); err == nil && n >= 0 {
			rules = n
		}
	}
	wantSig := devicePauseSig(desiredPauseMacsLocked(db, router.ID, time.Now().UTC()))
	if rules != len(macs) || sentSig == "" || sentSig != wantSig {
		return 0 // compte infidèle ou version périmée : pas de signature, re-file au check-in
	}
	router.PauseSig = sentSig
	return len(macs)
}
