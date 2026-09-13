// Vérification cloud des modules sécurité : walled garden, SafeWiFi, Shield, FamilyGuard (ensure + signatures).
// Extrait du monolithe agent_handlers.go (N°88) — même package, contenu inchangé.
package api

import (
	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// N°29 — walled-garden d'inscription publique (runbook N°27-D automatisé)
// ---------------------------------------------------------------------------

// walledGardenDomains — les noms d'hôtes à rendre joignables SANS
// authentification depuis le WiFi du hotspot : l'origine PAGE (console —
// héberge /join/{token}, encodée dans le QR) et l'hôte API (appelé par le
// navigateur du visiteur depuis la page). Sources : MICKLOUD_BASE_URL,
// APP_PUBLIC_URL, ALLOWED_ORIGIN (CORS = exactement les origines navigateur)
// et l'hôte de la requête agent courante (l'API telle que ce déploiement
// l'expose). Dé-dupliqués, triés (signature stable), 10 max — un déploiement
// standard en produit 2 (page + API).
func walledGardenDomains(r *http.Request) []string {
	hosts := make([]string, 0, 4)
	add := func(raw string) {
		if h := agent.SanitizeWGDomain(normalizeWGHost(raw)); h != "" && wgHostUsable(h) {
			hosts = append(hosts, h)
		}
	}
	add(os.Getenv("MIKCLOUD_BASE_URL"))
	add(os.Getenv("APP_PUBLIC_URL"))
	for _, o := range strings.Split(os.Getenv("ALLOWED_ORIGIN"), ",") {
		add(strings.TrimSpace(o))
	}
	if r != nil {
		add(r.Host)
	}
	seen := make(map[string]bool, len(hosts))
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	sort.Strings(out)
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}

// wgHostUsable — filtre les hôtes inutiles voire nuisibles dans un
// walled-garden hotspot (complément N°31-b) : boucle locale, RFC1918,
// link-local, mDNS et 0.0.0.0 ne sont PAS joignables depuis un client du
// WiFi — les autoriser ne protège aucun flux réel et pollue la table
// walled-garden du gérant (constat prod : « localhost:3000 » issu des
// origines de dev de ALLOWED_ORIGIN). Les hôtes publics restent éligibles,
// port numérique compris (le Host HTTP l'inclut sur les ports non standard).

// wgHostUsable — filtre les hôtes inutiles voire nuisibles dans un
// walled-garden hotspot (complément N°31-b) : boucle locale, RFC1918,
// link-local, mDNS et 0.0.0.0 ne sont PAS joignables depuis un client du
// WiFi — les autoriser ne protège aucun flux réel et pollue la table
// walled-garden du gérant (constat prod : « localhost:3000 » issu des
// origines de dev de ALLOWED_ORIGIN). Les hôtes publics restent éligibles,
// port numérique compris (le Host HTTP l'inclut sur les ports non standard).
func wgHostUsable(h string) bool {
	if h == "" {
		return false
	}
	host := h
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i] // port retiré pour l'évaluation (un FQDN ne porte pas « : »)
	}
	if host == "" {
		return false // IPv6 abrégée (« ::1 ») — non exprimable en dst-host
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return false
	}
	if o := strings.Split(host, "."); len(o) == 4 { // candidat IPv4
		nums := make([]int, 4)
		ok := true
		for i, p := range o {
			n, err := strconv.Atoi(p)
			if err != nil || p == "" || len(p) > 3 {
				ok = false
				break
			}
			nums[i] = n
		}
		if ok {
			a, b := nums[0], nums[1]
			if a == 0 || a == 10 || a == 127 || (a == 169 && b == 254) || (a == 172 && b >= 16 && b <= 31) || (a == 192 && b == 168) {
				return false
			}
		}
	}
	return true
}

// normalizeWGHost — extrait l'hôte brut d'une origine/URL/hôte : préfixe de
// schéma, userinfo et chemin retirés, ports par défaut (80/443) retirés.
// La validation fine du jeu de caractères est faite par agent.SanitizeWGDomain.

// normalizeWGHost — extrait l'hôte brut d'une origine/URL/hôte : préfixe de
// schéma, userinfo et chemin retirés, ports par défaut (80/443) retirés.
// La validation fine du jeu de caractères est faite par agent.SanitizeWGDomain.
func normalizeWGHost(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.LastIndexByte(s, '@'); i >= 0 { // userinfo parasites
		s = s[i+1:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSuffix(s, ":443")
	s = strings.TrimSuffix(s, ":80")
	return s
}

// walledGardenSig — signature courte et stable d'une configuration de
// domaines (hash du join trié + SEL DE VERSION des règles) : elle distingue
// « déjà appliqué sur ce routeur » d'« à (re)appliquer » sans table
// supplémentaire.
// N°48 — sel wg-v2-api : la v1 ne posait que des règles « page » (variante
// proxy du hotspot = HTTP pur, port 80) + DNS. Or l'API MikCloud (claim,
// /portal, /join) est intégralement en HTTPS (Render/Vercel) : le TLS 443
// pré-auth restait bloqué et le fetch du claim échouait côté client
// (« Service WiFi offert momentanément indisponible »). La v2 ajoute les
// règles « api » (variante ip, dst-host, action=accept). Le sel change la
// signature SANS changer la liste des domaines → chaque routeur déjà en
// ligne (sig v1 stockée) reçoit la mise à niveau automatiquement à son
// premier check-in (ensureWalledGardenLocked voit un mismatch → re-file).

// walledGardenSig — signature courte et stable d'une configuration de
// domaines (hash du join trié + SEL DE VERSION des règles) : elle distingue
// « déjà appliqué sur ce routeur » d'« à (re)appliquer » sans table
// supplémentaire.
// N°48 — sel wg-v2-api : la v1 ne posait que des règles « page » (variante
// proxy du hotspot = HTTP pur, port 80) + DNS. Or l'API MikCloud (claim,
// /portal, /join) est intégralement en HTTPS (Render/Vercel) : le TLS 443
// pré-auth restait bloqué et le fetch du claim échouait côté client
// (« Service WiFi offert momentanément indisponible »). La v2 ajoute les
// règles « api » (variante ip, dst-host, action=accept). Le sel change la
// signature SANS changer la liste des domaines → chaque routeur déjà en
// ligne (sig v1 stockée) reçoit la mise à niveau automatiquement à son
// premier check-in (ensureWalledGardenLocked voit un mismatch → re-file).
const walledGardenRulesVersion = "wg-v2-api"

func walledGardenSig(domains []string) string {
	return agent.HashToken(walledGardenRulesVersion + "|" + strings.Join(domains, "|"))[:16]
}

// N°49 — auto-réparation : même à configuration IDENTIQUE, le bloc
// walled-garden est re-filé périodiquement (walledGardenRefresh). Le bloc
// étant idempotent (remove+add des seules règles marquées mikcloud-wg), ce
// re-file répare silencieusement une liste vidée ou amputée LOCALEMENT sur
// le routeur : ménage Mikhmon, restauration de backup, ajout manuel
// partiel (constat prod CyberSC 2026-09-06 : règles DNS mikcloud-wg posées
// mais règles page absentes — le bouton « S'inscrire » aboutissait à une
// page injoignable, la sig côté cloud croyant le contraire).

// N°49 — auto-réparation : même à configuration IDENTIQUE, le bloc
// walled-garden est re-filé périodiquement (walledGardenRefresh). Le bloc
// étant idempotent (remove+add des seules règles marquées mikcloud-wg), ce
// re-file répare silencieusement une liste vidée ou amputée LOCALEMENT sur
// le routeur : ménage Mikhmon, restauration de backup, ajout manuel
// partiel (constat prod CyberSC 2026-09-06 : règles DNS mikcloud-wg posées
// mais règles page absentes — le bouton « S'inscrire » aboutissait à une
// page injoignable, la sig côté cloud croyant le contraire).
const walledGardenRefresh = 6 * time.Hour

// walledGardenFresh — vrai si la configuration actuelle a été CONFIRMÉE
// appliquée récemment. Deux cas forcant le re-file :
//   - WalledGardenAppliedAt vide alors que la sig est posée : routeur
//     configuré AVANT le N°49 (l'horodatage n'existait pas) — re-file au
//     premier check-in suivant, ce qui répare aussi le constat prod.
//   - horodatage présent mais plus vieux que walledGardenRefresh.

// walledGardenFresh — vrai si la configuration actuelle a été CONFIRMÉE
// appliquée récemment. Deux cas forcant le re-file :
//   - WalledGardenAppliedAt vide alors que la sig est posée : routeur
//     configuré AVANT le N°49 (l'horodatage n'existait pas) — re-file au
//     premier check-in suivant, ce qui répare aussi le constat prod.
//   - horodatage présent mais plus vieux que walledGardenRefresh.
func walledGardenFresh(router *model.Router) bool {
	if router.WalledGardenAppliedAt == "" {
		return false // sig posée avant le N°49 → réparer une fois, puis cadence
	}
	t, err := time.Parse(time.RFC3339, router.WalledGardenAppliedAt)
	if err != nil {
		return false // horodatage illisible → prudent : re-filer (idempotent)
	}
	return time.Since(t) < walledGardenRefresh
}

// ensureWalledGardenLocked — sous verrou : si la configuration walled-garden
// courante diffère de celle déjà appliquée sur le routeur (et qu'aucune
// commande n'est en vol), file la mise à jour — elle est servie dans CE
// check-in. C'est le point N°29 pour les routeurs DÉJÀ EN LIGNE lors du
// déploiement : aucun recollage manuel, chaque agent se met à niveau tout
// seul à son premier check-in (≤ 45 s). La signature n'est posée qu'au
// retour « ok » (handleAgentResult) : un échec est retenté au check-in
// suivant, un changement de config re-file automatiquement.

// ensureWalledGardenLocked — sous verrou : si la configuration walled-garden
// courante diffère de celle déjà appliquée sur le routeur (et qu'aucune
// commande n'est en vol), file la mise à jour — elle est servie dans CE
// check-in. C'est le point N°29 pour les routeurs DÉJÀ EN LIGNE lors du
// déploiement : aucun recollage manuel, chaque agent se met à niveau tout
// seul à son premier check-in (≤ 45 s). La signature n'est posée qu'au
// retour « ok » (handleAgentResult) : un échec est retenté au check-in
// suivant, un changement de config re-file automatiquement.
func ensureWalledGardenLocked(db *model.DB, router *model.Router, domains []string) {
	if len(domains) == 0 {
		return
	}
	sig := walledGardenSig(domains)
	if router.WalledGardenSig == sig && walledGardenFresh(router) {
		return // déjà appliqué avec cette configuration exacte, et récemment
	}
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == router.ID && c.Kind == model.CmdWalledGarden && (c.Status == "queued" || c.Status == "sent") {
			return // une mise à jour est déjà en vol
		}
	}
	queueCommandLocked(db, router.AccountID, router.ID, model.CmdWalledGarden, map[string]any{
		"domains": domains,
		"sig":     sig,
	})
}

// parseAgentForm — parse tolérant : query URL + corps brut (RouterOS n'envoie pas
// toujours un Content-Type form-urlencoded).

// safeWifiRulesVersion — sel de version des règles SafeWiFi : toute
// évolution de la FORME des règles (changement de résolveur, nouveau
// marquage, champs supplémentaires) change ce sel → chaque routeur en
// ligne reçoit la mise à niveau automatiquement à son premier check-in
// (ensureSafeWifiLocked voit un mismatch → re-file). Pattern walled-garden
// N°48. sw-v2 (N°85) : règles NAT en tête de table (place-before=0, une
// règle dstnat antérieure ne peut plus passer devant), blocage DoT
// (tcp/853) et DoH (tcp/443 vers liste mikcloud-safewifi-doh) par
// serveur hotspot, coupure DNS/DoT/DoH IPv6 (best-effort).
const safeWifiRulesVersion = "sw-v2"

// safeWifiRefresh — cadence d'auto-réparation (pattern N°49) : à
// configuration IDENTIQUE, le bloc safewifi est re-filé périodiquement. Le
// bloc étant idempotent (remove-then-add des seules règles marquées), ce
// re-file répare une règle effacée localement (ménage Mikhmon, restauration
// de backup) au plus tard 6 h après, sans intervention.

// safeWifiRefresh — cadence d'auto-réparation (pattern N°49) : à
// configuration IDENTIQUE, le bloc safewifi est re-filé périodiquement. Le
// bloc étant idempotent (remove-then-add des seules règles marquées), ce
// re-file répare une règle effacée localement (ménage Mikhmon, restauration
// de backup) au plus tard 6 h après, sans intervention.
const safeWifiRefresh = 6 * time.Hour

// safeWifiSig — signature courte et stable d'un niveau SafeWiFi (hash du
// niveau + sel de version des règles) : elle distingue « déjà appliqué sur
// ce routeur » d'« à (re)appliquer » sans table supplémentaire.

// safeWifiSig — signature courte et stable d'un niveau SafeWiFi (hash du
// niveau + sel de version des règles) : elle distingue « déjà appliqué sur
// ce routeur » d'« à (re)appliquer » sans table supplémentaire.
func safeWifiSig(level string) string {
	return agent.HashToken(safeWifiRulesVersion + "|" + level)[:16]
}

// safeWifiFresh — vrai si la config actuelle a été CONFIRMÉE appliquée
// récemment. Vide ou illisible → re-file prudent (pattern walledGardenFresh).

// safeWifiFresh — vrai si la config actuelle a été CONFIRMÉE appliquée
// récemment. Vide ou illisible → re-file prudent (pattern walledGardenFresh).
func safeWifiFresh(router *model.Router) bool {
	if router.SafeWifiAppliedAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, router.SafeWifiAppliedAt)
	if err != nil {
		return false
	}
	return time.Since(t) < safeWifiRefresh
}

// safeWifiLevelLabel — libellé court d'un niveau pour le journal d'activité.

// safeWifiLevelLabel — libellé court d'un niveau pour le journal d'activité.
func safeWifiLevelLabel(level string) string {
	switch level {
	case model.SafeWifiThreats:
		return "menaces bloquées"
	case model.SafeWifiFamily:
		return "filtrage famille"
	}
	return "protection désactivée"
}

// ensureSafeWifiLocked — N°80 : converge la protection DNS du WiFi public,
// sous verrou, depuis handleAgentCmd. Trois cas de figure :
//   - niveau "off" JAMAIS utilisé (sig vide) : RIEN — un routeur antérieur
//     au N°80 dont le gérant n'ouvre jamais la carte ne consomme aucun
//     octet (l'économie de veille N°75 reste entière) ;
//   - niveau actif ou déjà utilisé : si la signature courante diffère de la
//     config appliquée (changement de niveau, évolution du sel de version)
//     ou si l'application n'est plus fraîche, la commande safewifi rejoint
//     la file — servie dans CE check-in (deferred bucket, fermeture) ;
//   - une commande en file/en vol suffit — le rapport tranchera (ok → sig
//     posée après vérification du compte de règles, error → re-file au
//     check-in suivant).

// ensureSafeWifiLocked — N°80 : converge la protection DNS du WiFi public,
// sous verrou, depuis handleAgentCmd. Trois cas de figure :
//   - niveau "off" JAMAIS utilisé (sig vide) : RIEN — un routeur antérieur
//     au N°80 dont le gérant n'ouvre jamais la carte ne consomme aucun
//     octet (l'économie de veille N°75 reste entière) ;
//   - niveau actif ou déjà utilisé : si la signature courante diffère de la
//     config appliquée (changement de niveau, évolution du sel de version)
//     ou si l'application n'est plus fraîche, la commande safewifi rejoint
//     la file — servie dans CE check-in (deferred bucket, fermeture) ;
//   - une commande en file/en vol suffit — le rapport tranchera (ok → sig
//     posée après vérification du compte de règles, error → re-file au
//     check-in suivant).
func (a *API) ensureSafeWifiLocked(db *model.DB, router *model.Router) {
	if router.Mode != "agent" {
		return
	}
	level := router.SafeWifiLevelEffective()
	if level == model.SafeWifiOff && router.SafeWifiSig == "" {
		return // jamais utilisé : aucun filtrage, aucune commande
	}
	sig := safeWifiSig(level)
	if router.SafeWifiSig == sig && safeWifiFresh(router) {
		return // déjà appliqué avec ce niveau exact, et récemment
	}
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == router.ID && c.Kind == model.CmdSafeWifi &&
			(c.Status == "queued" || c.Status == "sent") {
			return // une mise à jour est déjà en vol
		}
	}
	queueCommandLocked(db, router.AccountID, router.ID, model.CmdSafeWifi, map[string]any{
		"level": level,
		"sig":   sig,
	})
}

// shieldRulesVersion — sel de version des règles FILTER Shield (pattern
// safeWifiRulesVersion N°80) : toute évolution de la FORME des règles
// (nouveau port, nouveau marquage, règle supplémentaire par hotspot)
// change ce sel → chaque routeur en ligne reçoit la mise à niveau
// automatiquement à son premier check-in.

// shieldRulesVersion — sel de version des règles FILTER Shield (pattern
// safeWifiRulesVersion N°80) : toute évolution de la FORME des règles
// (nouveau port, nouveau marquage, règle supplémentaire par hotspot)
// change ce sel → chaque routeur en ligne reçoit la mise à niveau
// automatiquement à son premier check-in.
const shieldRulesVersion = "sh-v1"

// shieldRefresh — cadence d'auto-réparation (pattern N°49) : à
// configuration IDENTIQUE, le bloc shield est re-filé périodiquement.
// Idempotent (remove-then-add des seules règles marquées) : répare une
// règle effacée localement (ménage, restauration de backup) au plus
// tard 6 h après, et suit un renommage d'interface du hotspot.

// shieldRefresh — cadence d'auto-réparation (pattern N°49) : à
// configuration IDENTIQUE, le bloc shield est re-filé périodiquement.
// Idempotent (remove-then-add des seules règles marquées) : répare une
// règle effacée localement (ménage, restauration de backup) au plus
// tard 6 h après, et suit un renommage d'interface du hotspot.
const shieldRefresh = 6 * time.Hour

// shieldRulesPerHotspot — règles posées par serveur hotspot quand le
// bouclier est actif (2 input + 3 forward). La vérification du retour
// l'utilise avec le compte de hotspots RAPPORTÉ par le routeur.

// shieldRulesPerHotspot — règles posées par serveur hotspot quand le
// bouclier est actif (2 input + 3 forward). La vérification du retour
// l'utilise avec le compte de hotspots RAPPORTÉ par le routeur.
const shieldRulesPerHotspot = 5

// shieldSig — signature courte et stable d'un niveau Shield (hash du
// niveau + sel de version des règles) : elle distingue « déjà appliqué
// sur ce routeur » d'« à (re)appliquer » sans table supplémentaire.

// shieldSig — signature courte et stable d'un niveau Shield (hash du
// niveau + sel de version des règles) : elle distingue « déjà appliqué
// sur ce routeur » d'« à (re)appliquer » sans table supplémentaire.
func shieldSig(level string) string {
	return agent.HashToken(shieldRulesVersion + "|" + level)[:16]
}

// shieldFresh — vrai si la config actuelle a été CONFIRMÉE appliquée
// récemment. Vide ou illisible → re-file prudent (pattern shieldFresh ≈
// safeWifiFresh).

// shieldFresh — vrai si la config actuelle a été CONFIRMÉE appliquée
// récemment. Vide ou illisible → re-file prudent (pattern shieldFresh ≈
// safeWifiFresh).
func shieldFresh(router *model.Router) bool {
	if router.ShieldAppliedAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, router.ShieldAppliedAt)
	if err != nil {
		return false
	}
	return time.Since(t) < shieldRefresh
}

// shieldLevelLabel — libellé court d'un niveau pour le journal d'activité.

// shieldLevelLabel — libellé court d'un niveau pour le journal d'activité.
func shieldLevelLabel(level string) string {
	if level == model.ShieldOn {
		return "bouclier actif"
	}
	return "bouclier désactivé"
}

// ensureShieldLocked — N°81 : converge le bouclier réseau du WiFi public,
// sous verrou, depuis handleAgentCmd (contrat exact de ensureSafeWifiLocked
// N°80) :
//   - niveau "off" JAMAIS utilisé (sig vide) : RIEN — un routeur
//     antérieur au N°81 dont le gérant n'ouvre jamais la carte ne
//     consomme aucun octet (économie de veille N°75 entière) ;
//   - niveau actif ou déjà utilisé : si la signature courante diffère
//     de la config appliquée ou si l'application n'est plus fraîche,
//     la commande shield rejoint la file (deferred bucket, fermeture) ;
//   - une commande en file/en vol suffit — le rapport tranchera (ok →
//     sig posée après vérification 5 règles × hotspots, error → re-file
//     au check-in suivant).

// ensureShieldLocked — N°81 : converge le bouclier réseau du WiFi public,
// sous verrou, depuis handleAgentCmd (contrat exact de ensureSafeWifiLocked
// N°80) :
//   - niveau "off" JAMAIS utilisé (sig vide) : RIEN — un routeur
//     antérieur au N°81 dont le gérant n'ouvre jamais la carte ne
//     consomme aucun octet (économie de veille N°75 entière) ;
//   - niveau actif ou déjà utilisé : si la signature courante diffère
//     de la config appliquée ou si l'application n'est plus fraîche,
//     la commande shield rejoint la file (deferred bucket, fermeture) ;
//   - une commande en file/en vol suffit — le rapport tranchera (ok →
//     sig posée après vérification 5 règles × hotspots, error → re-file
//     au check-in suivant).
func (a *API) ensureShieldLocked(db *model.DB, router *model.Router) {
	if router.Mode != "agent" {
		return
	}
	level := router.ShieldLevelEffective()
	if level == model.ShieldOff && router.ShieldSig == "" {
		return // jamais utilisé : aucune commande
	}
	sig := shieldSig(level)
	if router.ShieldSig == sig && shieldFresh(router) {
		return // déjà appliqué avec ce niveau exact, et récemment
	}
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == router.ID && c.Kind == model.CmdShield &&
			(c.Status == "queued" || c.Status == "sent") {
			return // une mise à jour est déjà en vol
		}
	}
	queueCommandLocked(db, router.AccountID, router.ID, model.CmdShield, map[string]any{
		"level": level,
		"sig":   sig,
	})
}

// familyGuardRulesVersion — sel de version des règles FILTER FamilyGuard
// (pattern shieldRulesVersion N°81) : toute évolution de la FORME des
// règles (action, marquage, règle supplémentaire par hotspot) change ce
// sel → chaque routeur en ligne reçoit la mise à niveau automatiquement à
// son premier check-in.

// familyGuardRulesVersion — sel de version des règles FILTER FamilyGuard
// (pattern shieldRulesVersion N°81) : toute évolution de la FORME des
// règles (action, marquage, règle supplémentaire par hotspot) change ce
// sel → chaque routeur en ligne reçoit la mise à niveau automatiquement à
// son premier check-in.
const familyGuardRulesVersion = "fg-v1"

// familyGuardRefresh — cadence d'auto-réparation (pattern N°49) : à
// configuration ET ÉTAT identiques (même position dans la fenêtre), le
// bloc est re-filé périodiquement. Idempotent : répare une règle effacée
// localement (ménage, restauration de backup) au plus tard 6 h après, et
// suit un renommage d'interface du hotspot.

// familyGuardRefresh — cadence d'auto-réparation (pattern N°49) : à
// configuration ET ÉTAT identiques (même position dans la fenêtre), le
// bloc est re-filé périodiquement. Idempotent : répare une règle effacée
// localement (ménage, restauration de backup) au plus tard 6 h après, et
// suit un renommage d'interface du hotspot.
const familyGuardRefresh = 6 * time.Hour

// familyGuardRulesPerHotspot — règle posée par serveur hotspot quand le
// couvre-feu est en cours (1 forward reject). La vérification du retour
// l'utilise avec le compte de hotspots RAPPORTÉ par le routeur.

// familyGuardRulesPerHotspot — règle posée par serveur hotspot quand le
// couvre-feu est en cours (1 forward reject). La vérification du retour
// l'utilise avec le compte de hotspots RAPPORTÉ par le routeur.
const familyGuardRulesPerHotspot = 1

// familyGuardSig — signature courte et stable d'une config FamilyGuard
// APPLIQUÉE : hash du spec + sel de version + ÉTAT désiré au moment de
// l'application (couvre-feu en cours ou non). L'état fait partie de la
// signature car il BASCULE à chaque frontière de fenêtre : le check-in
// suivant voit une signature différente et re-file la bascule — c'est
// ainsi que le couvre-feu se lève le matin sans autre orchestration.

// familyGuardSig — signature courte et stable d'une config FamilyGuard
// APPLIQUÉE : hash du spec + sel de version + ÉTAT désiré au moment de
// l'application (couvre-feu en cours ou non). L'état fait partie de la
// signature car il BASCULE à chaque frontière de fenêtre : le check-in
// suivant voit une signature différente et re-file la bascule — c'est
// ainsi que le couvre-feu se lève le matin sans autre orchestration.
func familyGuardSig(spec string, active bool) string {
	state := "0"
	if active {
		state = "1"
	}
	return agent.HashToken(familyGuardRulesVersion + "|" + spec + "|" + state)[:16]
}

// familyGuardFresh — vrai si la config actuelle a été CONFIRMÉE appliquée
// récemment. Vide ou illisible → re-file prudent (pattern N°80/N°81).

// familyGuardFresh — vrai si la config actuelle a été CONFIRMÉE appliquée
// récemment. Vide ou illisible → re-file prudent (pattern N°80/N°81).
func familyGuardFresh(router *model.Router) bool {
	if router.FamilyGuardAppliedAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, router.FamilyGuardAppliedAt)
	if err != nil {
		return false
	}
	return time.Since(t) < familyGuardRefresh
}

// familyGuardSpecSummary — libellé court d'une fenêtre pour le journal
// d'activité (ex. « 22:00 → 06:00, tous les jours »).

// familyGuardSpecSummary — libellé court d'une fenêtre pour le journal
// d'activité (ex. « 22:00 → 06:00, tous les jours »).
func familyGuardSpecSummary(c model.FamilyGuardConfig) string {
	days := "tous les jours"
	all := true
	any := false
	for i := 0; i < 7; i++ {
		if c.Days[i] == '1' {
			any = true
		} else {
			all = false
		}
	}
	if !any {
		days = "aucun jour"
	} else if !all {
		names := []string{"lun", "mar", "mer", "jeu", "ven", "sam", "dim"}
		picked := ""
		for i := 0; i < 7; i++ {
			if c.Days[i] == '1' {
				if picked != "" {
					picked += ","
				}
				picked += names[i]
			}
		}
		days = picked
	}
	return c.Start + " → " + c.End + ", " + days
}

// ensureFamilyGuardLocked — N°82 : converge le couvre-feu internet du WiFi
// public, sous verrou, depuis handleAgentCmd (contrat exact de
// ensureSafeWifiLocked N°80 / ensureShieldLocked N°81) :
//   - spec vide = JAMAIS utilisé : RIEN — un routeur dont le gérant
//     n'ouvre jamais la carte ne consomme aucun octet (économie de veille
//     N°75 entière) ;
//   - l'ÉTAT désiré (dans la fenêtre ou non) est recalculé à CHAQUE
//     check-in en UTC (heure d'Abidjan — l'horloge routeur n'est jamais
//     consultée) : à chaque frontière de fenêtre la signature change et
//     la bascule rejoint la file, servie dans CE check-in (deferred
//     bucket, fermeture) ;
//   - une commande en file/en vol suffit — le rapport tranchera (ok →
//     sig posée après vérification 1 règle × hotspots, error → re-file
//     au check-in suivant) ;
//   - un routeur hors-ligne pendant une bascule converge vers l'état
//     « maintenant » à son retour : aucune commande périmée ne s'accumule.

// ensureFamilyGuardLocked — N°82 : converge le couvre-feu internet du WiFi
// public, sous verrou, depuis handleAgentCmd (contrat exact de
// ensureSafeWifiLocked N°80 / ensureShieldLocked N°81) :
//   - spec vide = JAMAIS utilisé : RIEN — un routeur dont le gérant
//     n'ouvre jamais la carte ne consomme aucun octet (économie de veille
//     N°75 entière) ;
//   - l'ÉTAT désiré (dans la fenêtre ou non) est recalculé à CHAQUE
//     check-in en UTC (heure d'Abidjan — l'horloge routeur n'est jamais
//     consultée) : à chaque frontière de fenêtre la signature change et
//     la bascule rejoint la file, servie dans CE check-in (deferred
//     bucket, fermeture) ;
//   - une commande en file/en vol suffit — le rapport tranchera (ok →
//     sig posée après vérification 1 règle × hotspots, error → re-file
//     au check-in suivant) ;
//   - un routeur hors-ligne pendant une bascule converge vers l'état
//     « maintenant » à son retour : aucune commande périmée ne s'accumule.
func (a *API) ensureFamilyGuardLocked(db *model.DB, router *model.Router) {
	if router.Mode != "agent" || router.FamilyGuardSpec == "" {
		return
	}
	cfg, ok := model.ParseFamilyGuardSpec(router.FamilyGuardSpec)
	if !ok {
		return // spec inviolable (jamais posée par le handler) : silence prudent
	}
	active := cfg.ActiveAt(time.Now().UTC())
	sig := familyGuardSig(router.FamilyGuardSpec, active)
	if router.FamilyGuardSig == sig && familyGuardFresh(router) {
		return // déjà appliqué avec ce spec et cet état, et récemment
	}
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == router.ID && c.Kind == model.CmdFamilyGuard &&
			(c.Status == "queued" || c.Status == "sent") {
			return // une mise à jour est déjà en vol
		}
	}
	queueCommandLocked(db, router.AccountID, router.ID, model.CmdFamilyGuard, map[string]any{
		"spec":   router.FamilyGuardSpec,
		"active": active,
		"sig":    sig,
	})
}

// antiVpnRulesVersion — sel de version des règles FILTER AntiVPN
// (pattern safeWifiRulesVersion N°80) : toute évolution de la FORME des
// règles (nouveau port, règle supplémentaire par hotspot) change ce
// sel → chaque routeur en ligne reçoit la mise à niveau automatiquement
// à son premier check-in.

// antiVpnRulesVersion — sel de version des règles FILTER AntiVPN
// (pattern safeWifiRulesVersion N°80) : toute évolution de la FORME des
// règles (nouveau port, règle supplémentaire par hotspot) change ce
// sel → chaque routeur en ligne reçoit la mise à niveau automatiquement
// à son premier check-in.
const antiVpnRulesVersion = "av-v1"

// antiVpnRefresh — cadence d'auto-réparation (pattern N°49) : à
// configuration IDENTIQUE, le bloc antivpn est re-filé périodiquement.
// Idempotent (remove-then-add des seules règles marquées) : répare une
// règle effacée localement (ménage, restauration de backup) au plus
// tard 6 h après, et suit un renommage d'interface du hotspot.

// antiVpnRefresh — cadence d'auto-réparation (pattern N°49) : à
// configuration IDENTIQUE, le bloc antivpn est re-filé périodiquement.
// Idempotent (remove-then-add des seules règles marquées) : répare une
// règle effacée localement (ménage, restauration de backup) au plus
// tard 6 h après, et suit un renommage d'interface du hotspot.
const antiVpnRefresh = 6 * time.Hour

// antiVpnSig — signature courte et stable d'un niveau AntiVPN (hash du
// niveau + sel de version des règles) : elle distingue « déjà appliqué
// sur ce routeur » d'« à (re)appliquer » sans table supplémentaire.

// antiVpnSig — signature courte et stable d'un niveau AntiVPN (hash du
// niveau + sel de version des règles) : elle distingue « déjà appliqué
// sur ce routeur » d'« à (re)appliquer » sans table supplémentaire.
func antiVpnSig(level string) string {
	return agent.HashToken(antiVpnRulesVersion + "|" + level)[:16]
}

// antiVpnFresh — vrai si la config actuelle a été CONFIRMÉE appliquée
// récemment. Vide ou illisible → re-file prudent (pattern N°80/N°81).

// antiVpnFresh — vrai si la config actuelle a été CONFIRMÉE appliquée
// récemment. Vide ou illisible → re-file prudent (pattern N°80/N°81).
func antiVpnFresh(router *model.Router) bool {
	if router.AntiVpnAppliedAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, router.AntiVpnAppliedAt)
	if err != nil {
		return false
	}
	return time.Since(t) < antiVpnRefresh
}

// antiVpnLevelLabel — libellé court d'un niveau pour le journal d'activité.

// antiVpnLevelLabel — libellé court d'un niveau pour le journal d'activité.
func antiVpnLevelLabel(level string) string {
	if level == model.AntiVpnOn {
		return "bloque-VPN actif"
	}
	return "bloque-VPN désactivé"
}

// ensureAntiVpnLocked — N°88 : converge le bloque-VPN du WiFi public,
// sous verrou, depuis handleAgentCmd (contrat exact de ensureShieldLocked
// N°81) :
//   - niveau "off" JAMAIS utilisé (sig vide) : RIEN — un routeur
//     antérieur au N°88 dont le gérant n'ouvre jamais la carte ne
//     consomme aucun octet (économie de veille N°75 entière) ;
//   - niveau actif ou déjà utilisé : si la signature courante diffère
//     de la config appliquée ou si l'application n'est plus fraîche,
//     la commande antivpn rejoint la file (deferred bucket, fermeture) ;
//   - une commande en file/en vol suffit — le rapport tranchera (ok →
//     sig posée après vérification 4 règles × hotspots, error → re-file
//     au check-in suivant).

// ensureAntiVpnLocked — N°88 : converge le bloque-VPN du WiFi public,
// sous verrou, depuis handleAgentCmd (contrat exact de ensureShieldLocked
// N°81) :
//   - niveau "off" JAMAIS utilisé (sig vide) : RIEN — un routeur
//     antérieur au N°88 dont le gérant n'ouvre jamais la carte ne
//     consomme aucun octet (économie de veille N°75 entière) ;
//   - niveau actif ou déjà utilisé : si la signature courante diffère
//     de la config appliquée ou si l'application n'est plus fraîche,
//     la commande antivpn rejoint la file (deferred bucket, fermeture) ;
//   - une commande en file/en vol suffit — le rapport tranchera (ok →
//     sig posée après vérification 4 règles × hotspots, error → re-file
//     au check-in suivant).
func (a *API) ensureAntiVpnLocked(db *model.DB, router *model.Router) {
	if router.Mode != "agent" {
		return
	}
	level := router.AntiVpnLevelEffective()
	if level == model.AntiVpnOff && router.AntiVpnSig == "" {
		return // jamais utilisé : aucune commande
	}
	sig := antiVpnSig(level)
	if router.AntiVpnSig == sig && antiVpnFresh(router) {
		return // déjà appliqué avec ce niveau exact, et récemment
	}
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == router.ID && c.Kind == model.CmdAntiVpn &&
			(c.Status == "queued" || c.Status == "sent") {
			return // une mise à jour est déjà en vol
		}
	}
	queueCommandLocked(db, router.AccountID, router.ID, model.CmdAntiVpn, map[string]any{
		"level": level,
		"sig":   sig,
	})
}

// ensureReadStateDue — N°74 — cadenceur de la télémétrie read_state. Enfile
// un read_state pour CE routeur si : (1) aucun n'est déjà en file ou en vol
// (statuts queued/sent), ET (2) le dernier appliqué date de plus de
// readStateMinInterval (ou aucun depuis le boot : premier check-in). À
// appeler sous le verrou du store, depuis handleAgentCmd — la commande est
// servie dans LE MÊME check-in (elle rejoint la FIFO juste avant le service).
