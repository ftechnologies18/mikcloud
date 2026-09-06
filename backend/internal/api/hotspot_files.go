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
// la mise en file — la sig reste stable tant que l'ensemble des fichiers
// n'a pas changé de forme, pas tant que le branding n'a pas changé.
//
// NOTE sur la signature : la sig est actuellement calculée sur la LISTE des
// chemins (path1|path2|…), pas sur le CONTENU personnalisé. Conséquence :
// un changement de branding seul ne déclenche PAS un re-déploiement (les
// chemins sont les mêmes). C'est volontaire pour la phase initiale : le
// portail hybride (login.html) récupère la config live au chargement via
// fetch('/api/wifi/site/{slug}/portal') — pas besoin de re-déployer les
// fichiers pour un branding. Le re-déploiement automatique se déclenche
// uniquement quand l'ENSEMBLE des fichiers change (ajout/suppression d'un
// asset, refonte du template). Un bouton « Re-déployer maintenant » dans
// la console permet le forçage manuel (vidage de la sig).
package api

import (
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
	sig := hotpage.Sig(files)
	if router.HotspotFilesSig == sig {
		return // déjà déployé avec cet ensemble exact de fichiers
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

// hotspotFilesSigFromPayload — extrait la sig d'une commande hotspot_files
// (posée au retour ok par handleAgentResult). Vide si absente.
func hotspotFilesSigFromPayload(p map[string]any) string {
	if v, ok := p["sig"].(string); ok {
		return v
	}
	return ""
}
