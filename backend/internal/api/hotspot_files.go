// Package api — déploiement automatique du portail captif (N°35).
//
// ensureHotspotFilesLocked est l'homologue de ensureWalledGardenLocked pour
// le portail captif (login.html, status.html, assets) : si la configuration
// du portail (branding, offres, textes, slug WiFi, lien join) a changé ou
// n'a jamais été déployée sur ce routeur, la commande hotspot_files rejoint
// la file FIFO ci-dessous — servie dans CE check-in. C'est le point N°35 :
// aucun upload Winbox/FTP, chaque agent se met à niveau tout seul à son
// premier check-in (≤ 45 s) puis à chaque changement de config côté console.
// La signature n'est posée qu'au retour « ok » (handleAgentResult) : un échec
// est retenté au check-in suivant, un changement de config re-file
// automatiquement — pattern identique à walled_garden (N°29).
//
// hotpageFilesForRouter calcule la liste des fichiers à déployer pour un
// routeur : tous les fichiers du template hotpage.DefaultFiles(), dans
// l'ordre d'atomicité (assets → pages → login.html en dernier). Cette liste
// est la même pour tous les routeurs d'un compte ; la personnalisation par
// compte se fait au moment du SERVE (GET /portal/{token}/{path}), pas à
// la mise en file.
//
// NOTE sur la signature (N°135 — sig v2) : la sig couvre le CONTENU des
// fichiers du template (N°48-b) ET l'empreinte du BRANDING du compte
// (portalBrandingFingerprint : logo, nom, bannière, Wave, hospitalité,
// offres payantes, site WiFi lié, lien join). Un changement de branding en
// console — un logo posé dans la carte Vouchers par exemple — change la
// sig → le portail est re-déployé au check-in suivant (≤ 45 s), comme la
// doc du package hotpage l'a toujours promis. Avant N°135, la sig ne
// couvrait que les fichiers : un logo posé en console n'atteignait JAMAIS
// le portail déployé sans un « Re-déployer » manuel — et le fetch live ne
// le rattrapait que pour les routeurs liés à un site WiFi actif (les
// cybercafés pure-vouchers restaient sur le branding périmé, ou pire sur
// le logo par défaut du template — celui d'un AUTRE client). Le bouton
// « Re-déployer maintenant » reste pour le forçage manuel (vidage de sig).
package api

import (
	"strconv"
	"strings"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/hotpage"
	"mikcloud/hotspot-api/internal/model"
)

// ensureHotspotFilesLocked — sous verrou : si la liste des fichiers du
// portail à déployer sur ce routeur diffère de celle déjà déployée (et
// qu'aucune commande hotspot_files n'est en vol), file la commande — elle
// est servie dans CE check-in. Pattern identique à ensureWalledGardenLocked.
func ensureHotspotFilesLocked(db *model.DB, router *model.Router) {
	if router == nil || router.Mode != "agent" {
		return // le mode simulé/réel ne déploie rien (pas de check-in agent)
	}
	files := hotpage.DefaultFiles()
	if len(files) == 0 {
		return // template vide (build cassé) — on ne file rien
	}
	sig := hotspotFilesSig(files, db, router) // N°135 — fichiers + branding du compte
	if router.HotspotFilesSig == sig {
		return // déjà déployé avec cet ensemble exact de fichiers + branding
	}
	for i := range db.Commands {
		c := &db.Commands[i]
		if c.RouterID == router.ID && c.Kind == model.CmdHotspotFiles && (c.Status == "queued" || c.Status == "sent") {
			return // un déploiement est déjà en vol
		}
	}
	queueCommandLocked(db, router.AccountID, router.ID, model.CmdHotspotFiles, map[string]any{
		"files": files,
		"sig":   sig,
	})
}

// hotspotFilesSig — N°135 — signature du portail POUR CE ROUTEUR :
// empreinte des fichiers du template (hotpage.Sig) + empreinte du BRANDING
// du compte (portalBrandingFingerprint), condensées en 16 caractères. La
// sig v2 tient enfin la promesse documentée du package hotpage : « un
// changement de config (branding, offres…) change la sig → re-déploiement
// automatique au check-in suivant (≤ 45 s) ».
func hotspotFilesSig(files []string, db *model.DB, router *model.Router) string {
	return agent.HashToken(hotpage.Sig(files) + "\x1f" + portalBrandingFingerprint(db, router))[:16]
}

// portalBrandingFingerprint — empreinte stable de TOUT ce qui atteint le
// fallback inliné du portail (bloc mikcloud-config + marqueurs du
// template). Miroir volontairement compact de buildPortalConfig : mêmes
// résolutions (1er site WiFi actif lié au routeur, 1er lien join actif,
// profils payables max 8), mêmes helpers (wifiQuotaResp, joinLinkActive,
// JoinButtonEnabled, LogRetentionDaysEffective) — si un champ rejoint la
// config du portail sans rejoindre cette empreinte, le portail déployé
// garderait une valeur périmée sans jamais se re-déployer.
// Les URL dérivées de la REQUÊTE (apiBase) n'y figurent pas (constantes
// par déploiement) ; APP_PUBLIC_URL y figure car il façonne wifiUrl/joinUrl
// cuits au déploiement.
func portalBrandingFingerprint(db *model.DB, router *model.Router) string {
	acc := router.AccountID
	settings := ensureSettings(db, acc)
	t := settings.Tenant
	parts := []string{
		"v2",
		t.Name,
		t.LogoURL,
		t.BannerURL,
		t.WaveLink,
		strconv.FormatBool(t.JoinButtonEnabled()),
		t.PortalStyle,
		t.PortalWelcome,
		t.PortalPromos,
		t.PortalSocials,
		t.PortalSlides,   // N°136 — slides du carrousel : servies dans la config, donc dans la sig
		t.PortalServices, // N°137 — services « Nos Services » : servis dans la config, donc dans la sig
		t.PortalKey,
		strconv.Itoa(t.LogRetentionDaysEffective()),
		getEnv("APP_PUBLIC_URL"),
	}
	// 1er site WiFi actif lié au routeur (même résolution que buildPortalConfig).
	for i := range db.WifiSites {
		s := &db.WifiSites[i]
		if s.AccountID == acc && s.RouterID == router.ID && s.Active {
			parts = append(parts, "wifi:"+s.Slug, strconv.FormatBool(s.MarketingOptIn))
			if profile := findProfileScoped(db, s.ProfileID, acc); profile != nil {
				ft, fd := wifiQuotaResp(s, profile)
				parts = append(parts, "quota:"+strconv.FormatInt(ft, 10)+":"+strconv.FormatInt(fd, 10))
			}
			break
		}
	}
	// 1er lien d'inscription actif lié : le TOKEN (l'URL cuite au déploiement
	// en dépend — une révocation/création doit re-déployer pour l'ôter/le poser).
	joinTok := ""
	for i := range db.JoinLinks {
		l := &db.JoinLinks[i]
		if l.AccountID == acc && l.RouterID == router.ID && !l.Revoked && joinLinkActive(l) {
			joinTok = l.Token
			break
		}
	}
	parts = append(parts, "join:"+joinTok)
	// Offres payables du compte (max 8, même plafond que la config servie :
	// une 9e offre ne change NI la config NI la sig — pas de re-déploiement vain).
	offers := 0
	for i := range db.Profiles {
		p := &db.Profiles[i]
		if p.AccountID != acc || p.Price <= 0 {
			continue
		}
		offers++
		if offers > 8 {
			break
		}
		parts = append(parts, "offer:"+p.Name+":"+strconv.Itoa(p.Price)+":"+strconv.Itoa(p.ValidityMinutes())+":"+strconv.Itoa(p.DataQuotaMb))
	}
	return strings.Join(parts, "\x1f")
}

// hotspotFilesSigFromPayload — extrait la sig d'une commande hotspot_files
// (posée au retour ok par handleAgentResult). Vide si absente.
func hotspotFilesSigFromPayload(p map[string]any) string {
	if v, ok := p["sig"].(string); ok {
		return v
	}
	return ""
}
