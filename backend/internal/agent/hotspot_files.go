// Fichiers du portail captif livrés au routeur (hotspot files).
// Extrait du monolithe agent.go (N°88) — même package, contenu inchangé.
package agent

import (
	"mikcloud/hotspot-api/internal/model"
	"strconv"
	"strings"
)

// HotspotFilesFromPayload — la liste ordonnée des fichiers à déployer pour
// une commande hotspot_files. Chaque entrée porte le chemin RELATIF
// (login.html, status.html, css/bootstrap.min.css, …) qui sert à la fois de
// clé de signature et de dst-path côté routeur (sous le dossier hotspot/).
func HotspotFilesFromPayload(p map[string]any) []string {
	raw := plStrList(p, "files")
	out := make([]string, 0, len(raw))
	for _, f := range raw {
		if f = sanitizePortalPath(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// sanitizePortalPath — n'accepte qu'un chemin RELATIF sans remontée (pas de
// « .. », pas de « / » initial) et un charset sûr : la cible côté routeur est
// toujours hotspot/<path>, et le fetch ne doit JAMAIS sortir de ce dossier
// (sécurité défense en profondeur même si la source est déjà validée cloud).

// sanitizePortalPath — n'accepte qu'un chemin RELATIF sans remontée (pas de
// « .. », pas de « / » initial) et un charset sûr : la cible côté routeur est
// toujours hotspot/<path>, et le fetch ne doit JAMAIS sortir de ce dossier
// (sécurité défense en profondeur même si la source est déjà validée cloud).
func sanitizePortalPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || strings.HasPrefix(p, "/") {
		return ""
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return ""
		}
		for i := 0; i < len(seg); i++ {
			c := seg[i]
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
				c == '-' || c == '_' || c == '.') {
				return ""
			}
		}
	}
	return p
}

// buildHotspotFiles — N°35 : déploiement automatique du portail captif sur un
// routeur AGENT déjà en ligne. Le cloud sert les fichiers personnalisés par
// compte (branding, offres, textes, slug WiFi, lien join) via
// GET /portal/{token}/{path} (mêmes hôtes que l'agent, TLS strict hérité,
// déjà walled-gardenés N°29). L'agent fait un /tool fetch par fichier (RouterOS
// n'a PAS d'extracteur ZIP natif — architecture un-fetch-par-fichier, ~30
// fetchs par déploiement). Ordre séquentiel : assets (css/js/img/webfonts)
// d'abord, status.html et pages auxiliaires ensuite, login.html EN DERNIER
// pour l'atomicité — un client qui ouvre login.html pendant le déploiement
// chargera la nouvelle page qui référence des assets déjà en place.
//
// Pattern calqué sur buildWalledGarden (N°29-N°32) :
//   - battement de cœur status=started AVANT les fetchs (preuve de livraison) ;
//   - variable step posée avant chaque bloc à risque (diagnostic sans console) ;
//   - on-error={} par fichier (un fetch échoué ne tue pas les autres) ;
//   - okVar global, rapport ok si tous les fetchs ont réussi ;
//   - reprise zombie 10 min via staleSentReadKinds (idempotent par surcharge) ;
//   - signature posée au retour « ok » uniquement (handleAgentResult).
//
// Le dossier hotspot/ DOIT exister côté routeur (pré-requis manuel one-shot :
// /ip hotspot profile set html-directory=hotspot). Si absent, le 1er fetch
// échoue → okVar false → rapport step="mkdir" ou "fetch-<path>" → re-file au
// check-in suivant (max 3 re-tries puis error avec activity log).

// buildHotspotFiles — N°35 : déploiement automatique du portail captif sur un
// routeur AGENT déjà en ligne. Le cloud sert les fichiers personnalisés par
// compte (branding, offres, textes, slug WiFi, lien join) via
// GET /portal/{token}/{path} (mêmes hôtes que l'agent, TLS strict hérité,
// déjà walled-gardenés N°29). L'agent fait un /tool fetch par fichier (RouterOS
// n'a PAS d'extracteur ZIP natif — architecture un-fetch-par-fichier, ~30
// fetchs par déploiement). Ordre séquentiel : assets (css/js/img/webfonts)
// d'abord, status.html et pages auxiliaires ensuite, login.html EN DERNIER
// pour l'atomicité — un client qui ouvre login.html pendant le déploiement
// chargera la nouvelle page qui référence des assets déjà en place.
//
// Pattern calqué sur buildWalledGarden (N°29-N°32) :
//   - battement de cœur status=started AVANT les fetchs (preuve de livraison) ;
//   - variable step posée avant chaque bloc à risque (diagnostic sans console) ;
//   - on-error={} par fichier (un fetch échoué ne tue pas les autres) ;
//   - okVar global, rapport ok si tous les fetchs ont réussi ;
//   - reprise zombie 10 min via staleSentReadKinds (idempotent par surcharge) ;
//   - signature posée au retour « ok » uniquement (handleAgentResult).
//
// Le dossier hotspot/ DOIT exister côté routeur (pré-requis manuel one-shot :
// /ip hotspot profile set html-directory=hotspot). Si absent, le 1er fetch
// échoue → okVar false → rapport step="mkdir" ou "fetch-<path>" → re-file au
// check-in suivant (max 3 re-tries puis error avec activity log).
func (b Builder) buildHotspotFiles(cmd model.Command) string {
	files := HotspotFilesFromPayload(cmd.Payload)
	okVar := "ok" + idSafe(cmd.ID)
	var sb strings.Builder
	sb.WriteString(header(cmd))
	// Battement de cœur N°31-c : prouve la livraison du .rsc avant les fetchs.
	sb.WriteString(`/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + urlEscape(cmd.ID) + `&status=started") output=none` + "\n")
	sb.WriteString(":local " + okVar + " true\n")
	sb.WriteString(`:local step "start"` + "\n")
	step := func(s string) { sb.WriteString(`:set step "` + s + `"` + "\n") }

	// Pré-requis : s'assurer que le dossier hotspot/ existe. /file mkdir est
	// idempotent sous RouterOS 7+ (renvoie une erreur bénigne si déjà là → on-error={}).
	// Côté < 7.x, la commande n'existe pas : le fetch créera lui-même le
	// sous-dossier à la première écriture (comportement RouterOS) — si mkdir
	// échoue, on continue quand même : le fetch décide.
	step("mkdir")
	sb.WriteString(`:do { /file mkdir "hotspot" } on-error={}` + "\n")

	base := strings.TrimRight(b.BaseURL, "/") + "/portal/" + urlEscape(b.Token) + "/"
	// N°35 — ordre séquentiel pour atomicité : assets d'abord, login.html en
	// dernier. L'appelant (ensureHotspotFilesLocked) a déjà trié la liste dans
	// cet ordre ; on dépile simplement.
	// urlPathEscape échappe chaque segment du path SÉPARÉMENT (pour préserver
	// les « / » qui séparent css/, js/, img/, webfonts/ — urlEscape encode
	// « / » en %2F, ce qui casserait l'URL côté routeur).
	for i, f := range files {
		step("fetch-" + strconv.Itoa(i+1))
		// Remove best-effort du fichier précédent (surcharge) puis fetch du
		// nouveau. Si remove échoue (fichier absent la 1re fois), on
		// continue. Si fetch échoue → okVar false, mais les autres fetchs
		// continuent : un seul fichier cassé ne doit pas tout bloquer —
		// le routeur garde l'ancienne version des autres, et la reprise
		// zombie 10 min re-tentera le fichier fauteur.
		dst := "hotspot/" + f
		sb.WriteString(`:do { /file remove "` + rosEscape(dst) + `" } on-error={}` + "\n")
		sb.WriteString(`:do { /tool fetch url="` + base + urlPathEscape(f) +
			`" dst-path="` + rosEscape(dst) + `" } on-error={ :set ` + okVar + " false }\n")
	}
	step("done")
	ok := b.reportLine(cmd.ID, true, map[string]string{"files": strconv.Itoa(len(files))})
	ko := `/tool fetch url="` + strings.TrimRight(b.BaseURL, "/") + `/agent/result?token=` + urlEscape(b.Token) +
		`" http-method=post http-data=("cmd=` + urlEscape(cmd.ID) + `&status=error&message=echec_sur_le_routeur&step=" . $step) output=none`
	sb.WriteString(":if ($" + okVar + ") do={\n  " + ok + "\n} else={\n  " + ko + "\n}\n")
	return sb.String()
}

// urlPathEscape — échappe un path composé de segments séparés par « / » en
// préservant les « / » (contrairement à urlEscape qui les encode en %2F).
// Chaque segment est échappé via urlEscape ; les « / » sont remis après.
// Sécurité : le path a déjà été validé par sanitizePortalPath (pas de « .. »,
// pas de « / » initial, charset sûr) — cette fonction n'est qu'un encodage
// d'URL, pas une re-validation.
