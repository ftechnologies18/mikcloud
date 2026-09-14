// N°97 → N°108 — application du rapport pool_doctor + calcul de capacité.
//
// Le script (agent/pooldoctor.go) rapporte APRÈS application :
//
//	recycled=yes|no          — timeouts + addresses-per-mac + lease DHCP posés
//	extended=yes|no          — range dédié ajouté au vrai fournisseur d'adresses
//	hosts=<n>                — hôtes tenant une IP (authentifiés + zombies)
//	active=<n>               — sessions authentifiées
//	pools=nom|r1,r2;…        — pools RouterOS (ranges vérité routeur)
//	servers=n|prof|if|loginTO|idleTO|keepTO|pool|perMAC;…
//	dhcp=n|if|pool|lease;…
//
// N°108 : la liste « profiles » du N°97 est retirée du rapport (address-pool
// et addresses-per-mac vivent sur /ip hotspot — le N°97 les lisait sur le
// profil, propriétés INEXISTANTES : champs toujours vides) ; le pool du
// SERVEUR hotspot arrive en 7e champ de la liste serveurs.
//
// Le cloud en tire : PoolCap (capacité des pools RÉFÉRENCÉS — pool du serveur
// hotspot OU pool du DHCP de son interface — pas les pools orphelins),
// PoolRanges (affichage), PoolHosts, PoolDoctorAt. Les listes brutes restent
// dans cmd.Result (console « Outils routeur »). Calcul partagé avec le
// simulateur (handler POST pool-doctor en mode simulated) : ParsePoolCapacity.
package api

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

// ParsePoolCapacity — capacité totale (adresses) des pools référencés.
//
// Entrée : map nom de pool → ranges (« a.b.c.d-e.f.g.h » et/ou « a.b.c.d/nn »
// séparés par des virgules), et liste des noms de pools référencés par les
// profils (payload profiles du docteur). Renvoie (capacité, ranges joins).
// Robustesse : un range illisible est ignoré (jamais d'invention) ; un pool
// référencé introuvable est ignoré pareil (profile → pool supprimé à la
// main : le diagnostic suivant le dira par ses propres listes).
func ParsePoolCapacity(pools map[string]string, referenced []string) (int, string) {
	seen := map[string]bool{}
	capTotal := 0
	var rangeParts []string
	for _, name := range referenced {
		name = strings.TrimSpace(name)
		if name == "" || name == "none" || name == "(unknown)" || seen[name] {
			continue
		}
		ranges, ok := pools[name]
		if !ok {
			continue
		}
		seen[name] = true
		for _, rg := range strings.Split(ranges, ",") {
			rg = strings.TrimSpace(rg)
			if rg == "" {
				continue
			}
			if n := rangeCapacity(rg); n > 0 {
				capTotal += n
				if len(rangeParts) < 8 { // borne d'affichage
					rangeParts = append(rangeParts, rg)
				}
			}
		}
	}
	return capTotal, strings.Join(rangeParts, ", ")
}

// rangeCapacity — nombre d'adresses d'un range RouterOS.
// « 10.5.50.2-10.5.50.254 » → 253 ; « 10.77.0.0/21 » → 2046.
// Formats mixtes « ip-prefix » (10.5.50.0-10.5.50.255) gérés par la
// soustraction entière — aucun CIDR malformé n'est accepté.
func rangeCapacity(rg string) int {
	if strings.Contains(rg, "/") {
		ip, ipnet, err := net.ParseCIDR(rg)
		if err != nil || ip.To4() == nil || ipnet == nil {
			return 0
		}
		ones, bits := ipnet.Mask.Size()
		if bits != 32 || ones < 8 || ones > 30 {
			return 0 // /31 /32 (sans usage DHCP) et sous-réseaux absurdes rejetés
		}
		return 1 << uint(32-ones)
	}
	parts := strings.SplitN(rg, "-", 2)
	if len(parts) != 2 {
		return 0
	}
	lo := net.ParseIP(strings.TrimSpace(parts[0])).To4()
	hi := net.ParseIP(strings.TrimSpace(parts[1])).To4()
	if lo == nil || hi == nil {
		return 0
	}
	l := uint32(lo[0])<<24 | uint32(lo[1])<<16 | uint32(lo[2])<<8 | uint32(lo[3])
	h := uint32(hi[0])<<24 | uint32(hi[1])<<16 | uint32(hi[2])<<8 | uint32(hi[3])
	if h < l || h-l > 1<<24 {
		return 0
	}
	return int(h - l + 1)
}

// parseDoctorPools — « nom|ranges;… » → map nom → ranges.
func parseDoctorPools(raw string) map[string]string {
	out := map[string]string{}
	for _, entry := range strings.Split(raw, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		fields := strings.SplitN(entry, "|", 2)
		name := strings.TrimSpace(fields[0])
		if name == "" {
			continue
		}
		if len(fields) == 2 {
			out[name] = strings.TrimSpace(fields[1])
		} else {
			out[name] = ""
		}
	}
	return out
}

// parseDoctorReferenced — noms de pools référencés par les configs HOTSPOT
// du routeur : (1) address-pool de chaque SERVEUR hotspot (7e champ de
// « n|prof|if|loginTO|idleTO|keepTO|pool|perMAC;… » — N°108 : la propriété
// vit sur /ip hotspot, la réallocation d'IP du hotspot tire de là) ; (2) pool
// du serveur DHCP posé sur une INTERFACE de serveur hotspot
// (« n|if|pool|lease;… ») — un serveur sans address-pool s'appuie sur le DHCP
// du bridge (cas réel ProMax WIFI : serveur sans pool, dhcp-server
// Hotspot-Pool sur Bridge-Hotspot) : sa capacité vient de là. Tolérant aux
// rapports antérieurs au N°108 (serveurs à 6 champs : le pool serveur
// n'y figure pas, la branche DHCP suffit — un rapport en vol au moment du
// déploiement reste lisible).
func parseDoctorReferenced(serversRaw, dhcpRaw string) []string {
	var names []string
	// Pool du serveur hotspot (7e champ) + interfaces portant un serveur
	// hotspot (3e champ) — une seule passe sur la liste.
	hotspotIfaces := map[string]bool{}
	for _, entry := range strings.Split(serversRaw, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		fields := strings.Split(entry, "|")
		if len(fields) >= 3 {
			if ifc := strings.TrimSpace(fields[2]); ifc != "" {
				hotspotIfaces[ifc] = true
			}
		}
		if len(fields) >= 7 {
			if pool := strings.TrimSpace(fields[6]); pool != "" && pool != "none" && pool != "(unknown)" {
				names = append(names, pool)
			}
		}
	}
	// Pools des serveurs DHCP posés sur CES interfaces (4e champ = lease,
	// pas encore lu ici — la branche ne regarde que l'interface et le pool).
	for _, entry := range strings.Split(dhcpRaw, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		fields := strings.Split(entry, "|")
		if len(fields) >= 3 {
			ifc, pool := strings.TrimSpace(fields[1]), strings.TrimSpace(fields[2])
			if hotspotIfaces[ifc] && pool != "" && pool != "static" {
				names = append(names, pool)
			}
		}
	}
	return names
}

// applyPoolDoctor — consomme le rapport du docteur : pose PoolCap/
// PoolRanges/PoolHosts/PoolDoctorAt sur le routeur, journalise
// l'activité (résumé lisible gérant).
func (a *API) applyPoolDoctor(db *model.DB, router *model.Router, vals url.Values) string {
	pools := parseDoctorPools(vals.Get("pools"))
	referenced := parseDoctorReferenced(vals.Get("servers"), vals.Get("dhcp"))
	cap, ranges := ParsePoolCapacity(pools, referenced)

	router.PoolCap = cap
	router.PoolRanges = ranges
	router.PoolDoctorAt = model.NowISO()
	if h, ok := parseReportInt(vals.Get("hosts")); ok {
		router.PoolHosts = h
	}
	// NB : ActiveSessions n'est PAS posé ici — la liste /api/routers le
	// recalcule depuis la table Sessions (source de vérité unique, tenue
	// par read_state) ; l'écart PoolHosts - ActiveSessions (IP zombies)
	// reste ainsi cohérent avec le reste de la console.

	summary := fmt.Sprintf("pool IP : %d adresse(s) disponibles", cap)
	if cap > 0 {
		summary = fmt.Sprintf("Docteur pool : %d/%d IP occupées (%d%%) — %s",
			router.PoolHosts, cap, poolUsagePct(router), ranges)
	}
	recycled := vals.Get("recycled") == "yes"
	extended := vals.Get("extended") == "yes"
	switch {
	case extended && recycled:
		summary += " — recyclage + extension appliqués"
	case extended:
		summary += " — extension appliquée"
	case recycled:
		summary += " — recyclage appliqué"
	}
	return summary
}

// poolUsagePct — occupation du pool en pourcentage entier (0 si capacité
// inconnue — jamais de division par zéro).
func poolUsagePct(r *model.Router) int {
	if r.PoolCap <= 0 || r.PoolHosts <= 0 {
		return 0
	}
	pct := r.PoolHosts * 100 / r.PoolCap
	if pct > 100 {
		return 100
	}
	return pct
}

// ensurePoolDoctorLocked — N°97 — auto-diagnostiqueur du check-in (pattern
// ensureWatcherLocked) : enfile un pool_doctor en DIAGNOSTIC PUR (aucun
// écrit sur le routeur : recycle=false, extend=false) quand aucun diagnostic
// n'a jamais abouti (PoolDoctorAt vide) ou que le dernier dépasse
// agent.PoolDoctorRefresh. Les corrections (recyclage/extension) restent
// des gestes EXPLICITES du gérant (bouton « Outils routeur → Système ») —
// le check-in ne modifie jamais la configuration de son propre chef.
//
// N°97-ter — la fraîcheur se juge sur PoolDoctorAt SEUL (et non plus sur
// « PoolCap > 0 ET frais ») : un diagnostic ABOUTI dont la capacité reste
// nulle (hotspot sans pool identifiable) doit reposer 7 jours comme un
// autre — l'ancienne condition re-filait la commande à CHAQUE check-in,
// soit toutes les 20 s avec le veilleur d'invités (constat production
// ProMax WIFI : file + journal inondés de ~180 commandes/heure).
//
// N°99 — EXCEPTION consentie : PoolAutoPending (posé par le moniteur quand
// la pression du pool franchit 80/95 % sur un routeur dont le gérant a
// activé le switch « Auto-réparation ») file le RECYCLAGE des IP zombies
// sans geste humain. Le nuage n'agit toujours pas de son propre chef : le
// gérant l'y a autorisé explicitement, par routeur, et l'action reste
// bornée (recyclage seul, JAMAIS l'extension — geste topologique à
// confirmation explicite ; une marque par transition d'alerte, l'anti-boucle
// vit dans le moniteur). Journalisée dans l'activité au filage.
//
// À appeler sous le verrou du store.
func (a *API) ensurePoolDoctorLocked(db *model.DB, router *model.Router) {
	if router.Mode != "agent" {
		return
	}
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == router.ID && c.Kind == model.CmdPoolDoctor &&
			(c.Status == "queued" || c.Status == "sent") {
			return // déjà en file ou en vol : l'auto-réparation attendra le prochain check-in
		}
	}

	// N°99 — auto-réparation en attente : recyclage AVANT le diagnostic de
	// fraîcheur (une pression high/full est un signal fort, même avec un
	// diagnostic d'hier). Le flag est consommé ICI : si la commande échoue,
	// la pression restera haute mais la mémoire de transition (moniteur)
	// interdit tout re-filage en boucle — la prochaine remontée de pression
	// après un retour au calme re-marquera.
	if router.PoolAutoPending {
		router.PoolAutoPending = false
		queueCommandLocked(db, router.AccountID, router.ID, model.CmdPoolDoctor,
			map[string]any{"recycle": true, "extend": false})
		pct := poolUsagePct(router)
		db.Activity = append([]model.Activity{{
			ID: model.NewID("act-"), AccountID: router.AccountID, Type: "router",
			Message: "Auto-réparation pool IP : recyclage des IP zombies envoyé sur «" + router.Name +
				"» (occupation " + strconv.Itoa(pct) + " % — switch Auto-réparation actif)",
			At: model.NowISO(), // acteur vide = moteur interne (cf. model.Activity)
		}}, db.Activity...)
		if len(db.Activity) > 500 {
			db.Activity = db.Activity[:500]
		}
		return
	}

	if router.PoolDoctorAt != "" {
		if at, err := time.Parse(time.RFC3339, router.PoolDoctorAt); err == nil {
			if time.Since(at) < agent.PoolDoctorRefresh {
				return // diagnostic frais : rien à faire
			}
		}
	}
	queueCommandLocked(db, router.AccountID, router.ID, model.CmdPoolDoctor,
		map[string]any{"recycle": false, "extend": false})
}
