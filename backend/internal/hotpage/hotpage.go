// Package hotpage — portail captif MikCloud servi par le backend puis déployé
// automatiquement sur les routeurs via l'agent (N°35).
//
// Le dossier template/ (renommé depuis « Hotspot Page/ » à la racine du repo)
// contient la RÉFÉRENCE des fichiers HTML/CSS/JS/asset attendus par RouterOS
// pour un portail captif MikroTik (login.html, status.html, alogin.html,
// md5.js, css/, js/, img/, webfonts/). Ces fichiers sont embarqués dans le
// binaire backend via //go:embed (taille ~1.5 Mo — acceptable : un déploiement
// unique par redémarrage du service, lecture depuis la mémoire).
//
// Au déploiement, le cloud personnalise chaque fichier par COMPTE :
//   - branding (nom tenant, logo URL, services, WhatsApp) ;
//   - slug WiFi jetable (lien /wifi/{slug}) ;
//   - lien d'inscription publique avec MAC pré-injectée (/join/{token}?mac=$(mac-esc)) ;
//   - offres payantes (price + waveUrl par profil) ;
//   - URL de base du portail (pour les fetchs hybrides cloud/local) ;
//   - textes d'accueil et palette.
//
// La signature (HotspotFilesSig) est le hash stable de l'ensemble des fichiers
// PERSONNALISÉS : un changement de config (branding, offres…) change la sig →
// ensureHotspotFilesLocked re-file automatiquement la commande hotspot_files →
// l'agent re-déploie au prochain check-in (≤ 45 s). Zéro intervention humaine.
package hotpage

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"sort"
	"strings"

	"mikcloud/hotspot-api/internal/agent"
)

//go:embed all:template
var templateFS embed.FS

// DefaultFiles — la liste ordonnée des fichiers du template de référence,
// dans l'ordre de DÉPLOIEMENT voulu pour l'atomicité du portail :
//  1. assets (CSS, JS, webfonts, images) — jamais ouverts directement par
//     le client, mais référencés par login.html/status.html. Ils doivent
//     être en place AVANT que la nouvelle login.html ne s'y réfère.
//  2. pages auxiliaires (status, alogin, logout, error, radvert, rlogin,
//     redirect) — servies après authentification ou en cas d'erreur. Elles
//     ne cassent pas le login si elles sont brièvement en cours de update.
//  3. login.html EN DERNIER — la page critique pré-auth. Un client qui
//     l'ouvre pendant le déploiement chargera l'ANCIENNE version (encore
//     présente car pas encore écrasée) qui référence des assets déjà à jour
//     côté nouveau, ou la NOUVELLE (juste écrasée) qui référence des assets
//     déjà en place. Les deux cas sont fonctionnels.
//
// md5.js est déployé tôt : il est requis par login.html pour CHAP-MD5.
func DefaultFiles() []string {
	entries, err := fs.ReadDir(templateFS, "template")
	if err != nil {
		// Le embed ne peut pas échouer en production (compilé dans le binaire) ;
		// ce path n'est atteint qu'en cas de template/ vide au build, ce que
		// la CI détecterait (test TestDefaultFilesNonEmpty ci-dessous).
		return nil
	}
	assets := []string{}
	pages := []string{}
	login := ""
	for _, e := range entries {
		if e.IsDir() {
			sub, err := fs.ReadDir(templateFS, "template/"+e.Name())
			if err != nil {
				continue
			}
			for _, s := range sub {
				if s.IsDir() {
					continue // un seul niveau de sous-dossier suffisant pour css/js/img/webfonts
				}
				assets = append(assets, e.Name()+"/"+s.Name())
			}
			continue
		}
		name := e.Name()
		switch {
		case name == "login.html":
			login = name
		case strings.HasSuffix(name, ".html") || strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".txt"):
			pages = append(pages, name)
		case strings.HasSuffix(name, ".ico"):
			assets = append(assets, name)
		default:
			assets = append(assets, name)
		}
	}
	sort.Strings(assets)
	sort.Strings(pages)
	// md5.js doit précéder les pages HTML qui le référencent (login.html).
	// On le remonte en tête des pages auxiliaires.
	md5First := []string{}
	rest := []string{}
	for _, p := range pages {
		if p == "md5.js" {
			md5First = append(md5First, p)
		} else {
			rest = append(rest, p)
		}
	}
	out := append(append(assets, md5First...), rest...)
	if login != "" {
		out = append(out, login) // login.html en DERNIER (atomicité)
	}
	return out
}

// RawFile — contenu brut d'un fichier du template (non personnalisé), en
// texte. Préférable pour les fichiers HTML/CSS/JS/txt (lecture = string).
// Retourne "" si le chemin n'existe pas (l'appelant décide du fallback).
func RawFile(path string) string {
	if !pathAllowed(path) {
		return ""
	}
	b, err := templateFS.ReadFile("template/" + path)
	if err != nil {
		return ""
	}
	return string(b)
}

// File — contenu brut d'un fichier du template (texte OU binaire), en
// []byte. Utilisé par le serveur /portal/{token}/{path} pour servir aussi
// bien login.html que img/logo.png, webfonts/*.woff2, favicon.ico. Retourne
// (nil, false) si le chemin n'existe pas ou est refusé.
func File(path string) ([]byte, bool) {
	if !pathAllowed(path) {
		return nil, false
	}
	b, err := templateFS.ReadFile("template/" + path)
	if err != nil {
		return nil, false
	}
	return b, true
}

// pathAllowed — true si le path relatif est sûr (pas de remontée, pas de
// slash initial). Les fichiers binaires sont servis via File() ; les fichiers
// texte via RawFile() (string) ou File() ([]byte).
func pathAllowed(path string) bool {
	if strings.Contains(path, "..") || strings.HasPrefix(path, "/") || path == "" {
		return false
	}
	return true
}

// HasFile — true si le path relatif existe dans le template.
func HasFile(path string) bool {
	if strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		return false
	}
	f, err := templateFS.Open("template/" + path)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	return err == nil && !st.IsDir()
}

// Sig — signature stable d'un ensemble de fichiers du portail. Pattern
// identique à walledGardenSig : hash court, tronqué à 16 car.
// N°48-b — la signature couvre le CONTENU (chemin + hash du contenu de
// chaque fichier embarqué), pas seulement la liste des chemins : une édition
// du template (ex. N°48 : retrait du bandeau « WiFi offert » doublon) doit
// re-pousser le portail vers les routeurs agents automatiquement, sans
// action console (« Re-déployer maintenant »). Une liste identique avec des
// contenus inchangés → même sig → check-in no-op (aucun spam de commandes).
// Constat d'origine : la sig v1 (chemins seuls) laissait le routeur servir
// un login.html périmé après chaque correctif template.
func Sig(files []string) string {
	sorted := make([]string, len(files))
	copy(sorted, files)
	sort.Strings(sorted)
	parts := make([]string, 0, len(sorted))
	for _, f := range sorted {
		sum := sha256.Sum256([]byte(f)) // fallback stable si le fichier manque
		if b, ok := File(f); ok {
			sum = sha256.Sum256(b)
		}
		parts = append(parts, f+"="+hex.EncodeToString(sum[:8]))
	}
	return agent.HashToken(strings.Join(parts, "|"))[:16]
}
