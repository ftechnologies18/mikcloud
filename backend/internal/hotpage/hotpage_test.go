// Package hotpage — tests du template embarqué et de la signature (N°35).
package hotpage

import (
	"sort"
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/agent"
)

// TestDefaultFilesNonEmpty — le template doit contenir au minimum login.html,
// status.html et md5.js (les fichiers critiques du portail MikroTik). Ce test
// échoue si le dossier template/ est vide au build (CI détecterait).
func TestDefaultFilesNonEmpty(t *testing.T) {
	files := DefaultFiles()
	if len(files) < 3 {
		t.Fatalf("DefaultFiles() = %d fichiers, attendu ≥ 3 (login.html, status.html, md5.js au minimum)", len(files))
	}
}

// TestDefaultFilesOrder — login.html est TOUJOURS en dernier (atomicité du
// déploiement : la page critique pré-auth n'est écrasée qu'une fois les
// assets en place). md5.js précède les pages HTML qui le référencent.
func TestDefaultFilesOrder(t *testing.T) {
	files := DefaultFiles()
	if len(files) == 0 {
		t.Fatal("DefaultFiles() vide")
	}
	last := files[len(files)-1]
	if last != "login.html" {
		t.Errorf("login.html doit fermer la marche (dernier fichier), ordre actuel : dernier=%q", last)
	}
	// md5.js doit apparaître avant login.html.
	md5Idx, loginIdx := -1, -1
	for i, f := range files {
		if f == "md5.js" {
			md5Idx = i
		}
		if f == "login.html" {
			loginIdx = i
		}
	}
	if md5Idx < 0 {
		t.Error("md5.js absent du template — requis par login.html pour CHAP-MD5")
	}
	if loginIdx < 0 {
		t.Error("login.html absent du template — page critique du portail")
	}
	if md5Idx >= 0 && loginIdx >= 0 && md5Idx > loginIdx {
		t.Errorf("md5.js (idx %d) doit précéder login.html (idx %d)", md5Idx, loginIdx)
	}
}

// TestDefaultFilesNoDup — un fichier ne doit pas apparaître deux fois (la sig
// serait instable et le déploiement gaspillerait un fetch).
func TestDefaultFilesNoDup(t *testing.T) {
	files := DefaultFiles()
	seen := map[string]bool{}
	for _, f := range files {
		if seen[f] {
			t.Errorf("fichier en double : %q", f)
		}
		seen[f] = true
	}
}

// TestRawFile — le fichier login.html existe et contient le marqueur CHAP
// (preuve que le template de référence est bien embarqué).
func TestRawFile(t *testing.T) {
	login := RawFile("login.html")
	if login == "" {
		t.Fatal("RawFile(login.html) vide — template non embarqué")
	}
	if !strings.Contains(login, "chap-id") && !strings.Contains(login, "doLogin") {
		t.Error("login.html ne contient ni 'chap-id' ni 'doLogin' — template incorrect ?")
	}
}

// TestRawFileSecurity — pas de remontée de dossier (sécurité défense en
// profondeur : l'appelant passe déjà un path relatif, mais le package doit
// refuser toute tentative de ../).
func TestRawFileSecurity(t *testing.T) {
	if RawFile("../go.mod") != "" {
		t.Error("RawFile(../go.mod) doit retourner '' (remontée refusée)")
	}
	if RawFile("/etc/passwd") != "" {
		t.Error("RawFile(/etc/passwd) doit retourner '' (chemin absolu refusé)")
	}
	if RawFile("") != "" {
		t.Error("RawFile('') doit retourner '' (chemin vide refusé)")
	}
}

// TestFileBinary — les assets binaires (jpg, woff2) sont servis via File([]byte).
// N°135 — img/logo.png (le logo du site pilote, 288 Ko) a été RETIRÉ du
// template : le logo du portail est celui DU CLIENT (marqueur LOGO_BLOCK)
// ou l'initiale du tenant — pub1.jpg est l'asset binaire de référence.
func TestFileBinary(t *testing.T) {
	b, ok := File("img/pub1.jpg")
	if !ok {
		t.Fatal("File(img/pub1.jpg) = nil, attendu []byte — asset binaire non servi")
	}
	if len(b) < 1000 {
		t.Errorf("File(img/pub1.jpg) = %d octets, attendu > 1000", len(b))
	}
	// Signature JPEG : FF D8 FF
	if len(b) < 3 || b[0] != 0xFF || b[1] != 0xD8 || b[2] != 0xFF {
		t.Error("File(img/pub1.jpg) — en-tête JPEG invalide")
	}
	// N°135 — le logo d'un autre client ne doit plus être embarqué.
	if _, still := File("img/logo.png"); still {
		t.Error("File(img/logo.png) existe encore — le logo du site pilote doit rester hors du template (N°135)")
	}
}

// TestLoginTemplateLogoBlock — N°135 : login.html porte le marqueur du bloc
// logo ({{MIKCLOUD_LOGO_BLOCK}}) et ne référence PLUS l'asset img/logo.png
// — le logo par défaut historique était celui du site pilote de l'audit
// (CYBER ESPACE SC) : tout portail sans logo configuré affichait le logo
// d'un AUTRE client. Garde-fou anti-régression.
func TestLoginTemplateLogoBlock(t *testing.T) {
	body := RawFile("login.html")
	if body == "" {
		t.Fatal("login.html absent du template")
	}
	if !strings.Contains(body, "{{MIKCLOUD_LOGO_BLOCK}}") {
		t.Fatal("marqueur {{MIKCLOUD_LOGO_BLOCK}} absent de login.html — le bloc logo n'est plus templatisé")
	}
	if strings.Contains(body, "img/logo.png") {
		t.Fatal("login.html référence encore img/logo.png — le logo d'un autre client ne doit plus être servi par défaut")
	}
	if strings.Contains(body, "CYBER ESPACE SC") {
		t.Fatal("login.html mentionne encore « CYBER ESPACE SC » — un autre client ne doit pas être nommé sur les portails")
	}
	// Le JS du fetch live doit savoir RETIRER le logo (repli initale) —
	// un logo retiré en console ne doit pas survivre au fetch live.
	if !strings.Contains(body, "logoImg.parentNode.removeChild(logoImg)") {
		t.Fatal("le retrait dynamique du logo (fetch live sans logoUrl) absent de login.html")
	}
	// object-fit contain : le logo du client ne se rogne pas.
	if !strings.Contains(body, "object-fit: contain") {
		t.Fatal("object-fit: contain absent — le logo du client serait rogné par le cover historique")
	}
}

// TestLoginTemplateServicesBlock — N°137 : la section « Nos Services » est
// templatisée (marqueurs ATTR/BLOCK) et ne porte PLUS les 4 services codés
// en dur du site pilote — les services affichés sont ceux DU TENANT, posés
// en console ; sans services configurés, la section est masquée (ATTR).
func TestLoginTemplateServicesBlock(t *testing.T) {
	body := RawFile("login.html")
	if body == "" {
		t.Fatal("login.html absent du template")
	}
	if !strings.Contains(body, "{{MIKCLOUD_SERVICES_ATTR}}") {
		t.Fatal("marqueur {{MIKCLOUD_SERVICES_ATTR}} absent de login.html — le wrap Nos Services n'est plus templatisé")
	}
	if !strings.Contains(body, "{{MIKCLOUD_SERVICES_BLOCK}}") {
		t.Fatal("marqueur {{MIKCLOUD_SERVICES_BLOCK}} absent de login.html — les <li> des services ne sont plus templatisés")
	}
	for _, banned := range []string{
		"Cyber Espace & Internet",
		"Maintenance Informatique",
		"Développement Web & Applications",
		"Services Monétiques",
	} {
		if strings.Contains(body, banned) {
			t.Fatalf("login.html porte encore le service codé en dur du site pilote : %q", banned)
		}
	}
	// Le JS du fetch live doit piloter la section (sans re-déploiement).
	if !strings.Contains(body, "mikcloud-services-wrap") {
		t.Fatal("le pilotage JS de la section Nos Services (fetch live) absent de login.html")
	}
}

// TestHasFile — cohérence entre HasFile et File/RawFile.
func TestHasFile(t *testing.T) {
	cases := map[string]bool{
		"login.html":            true,
		"status.html":           true,
		"md5.js":                true,
		"css/bootstrap.min.css": true,
		"img/pub1.jpg":          true,
		"img/logo.png":          false, // N°135 — logo du site pilote retiré
		"nonexistent.html":      false,
		"../go.mod":             false,
		"":                      false,
	}
	for path, want := range cases {
		if got := HasFile(path); got != want {
			t.Errorf("HasFile(%q) = %v, attendu %v", path, got, want)
		}
	}
}

// TestSigStable — la sig est stable pour deux listes identiques (ordres
// différents), différente pour deux listes différentes.
func TestSigStable(t *testing.T) {
	a := []string{"login.html", "status.html", "md5.js"}
	b := []string{"md5.js", "login.html", "status.html"} // même ensemble, ordre différent
	c := []string{"login.html", "status.html"}           // ensemble différent
	sa, sb, sc := Sig(a), Sig(b), Sig(c)
	if sa != sb {
		t.Errorf("Sig instable : a=%q b=%q (doivent être égaux — même ensemble)", sa, sb)
	}
	if sa == sc {
		t.Errorf("Sig collision : a=%q c=%q (doivent différer — ensembles différents)", sa, sc)
	}
	if len(sa) != 16 {
		t.Errorf("Sig longueur = %d, attendu 16 (cf. walledGardenSig)", len(sa))
	}
}

// TestSigContentSensitive — N°48-b : la sig couvre le CONTENU des fichiers,
// pas seulement la liste des chemins. Garde-fou du bug coulissant du N°48 :
// avec la formule v1 (chemins seuls), le login.html corrigé n'était jamais
// re-poussé aux routeurs — le portail restait servie périmé jusqu'au clic
// manuel « Re-déployer maintenant ». Si ce test échoue, le contenu a de
// nouveau disparu de la signature.
func TestSigContentSensitive(t *testing.T) {
	files := DefaultFiles()
	if len(files) == 0 {
		t.Fatal("template embarqué vide — DefaultFiles doit lister les fichiers")
	}
	s := Sig(files)
	if len(s) != 16 {
		t.Fatalf("Sig longueur = %d, attendu 16", len(s))
	}
	// Formule v1 (chemins seuls, triés) — si Sig la reproduit, le contenu ne
	// fait plus partie de la signature.
	sorted := make([]string, len(files))
	copy(sorted, files)
	sort.Strings(sorted)
	legacy := agent.HashToken(strings.Join(sorted, "|"))[:16]
	if s == legacy {
		t.Fatal("Sig basée chemins seuls : une édition du template ne serait jamais re-poussée (régression N°48-b)")
	}
	if s != Sig(files) {
		t.Fatal("Sig doit être déterministe (mêmes fichiers → même sig)")
	}
}

// TestSigEmpty — la sig d'une liste vide ne panique pas.
func TestSigEmpty(t *testing.T) {
	s := Sig(nil)
	if len(s) != 16 {
		t.Errorf("Sig(nil) longueur = %d, attendu 16", len(s))
	}
}

// TestLoginTemplateTicker — N°138 : le bandeau animé sous le logo est
// templatisé (marqueur TICKER_JSON dans l'init Typed.js) et ne porte PLUS
// les 3 messages codés en dur — ils vivent dans le repli serveur
// (tickerJSON) ; le JS expose l'instance (window.mikTyped) pour le
// pilotage live (bloc 10 de applyConfig).
func TestLoginTemplateTicker(t *testing.T) {
	body := RawFile("login.html")
	if body == "" {
		t.Fatal("login.html absent du template")
	}
	if !strings.Contains(body, "{{MIKCLOUD_TICKER_JSON}}") {
		t.Fatal("marqueur {{MIKCLOUD_TICKER_JSON}} absent de login.html — les messages du bandeau ne sont plus templatisés")
	}
	if strings.Contains(body, "strings: ['Wifi haut débit !'") {
		t.Fatal("login.html porte encore les messages codés en dur — ils doivent venir du marqueur (repli serveur)")
	}
	if !strings.Contains(body, "window.mikTyped") {
		t.Fatal("l'exposition de l'instance Typed (window.mikTyped) absente de login.html — pas de pilotage live du bandeau")
	}
	if !strings.Contains(body, "mikTyped.reset()") {
		t.Fatal("le rafraîchissement live de l'animation (reset()) absent de login.html")
	}
}

// TestTemplatesWhatsapp — N°139 : le lien support WhatsApp est templatisé
// (marqueurs WHATSAPP_HREF/LABEL) sur les TROIS pages qui le portent
// (login, logout, error) et ne porte PLUS le numéro codé en dur — il vit
// dans le repli serveur (whatsappHref/whatsappLabel) ; le login expose
// l'ancre + le span pour le pilotage live (bloc 11 de applyConfig).
func TestTemplatesWhatsapp(t *testing.T) {
	for _, page := range []string{"login.html", "logout.html", "error.html"} {
		body := RawFile(page)
		if body == "" {
			t.Fatalf("%s absent du template", page)
		}
		if !strings.Contains(body, "{{MIKCLOUD_WHATSAPP_HREF}}") || !strings.Contains(body, "{{MIKCLOUD_WHATSAPP_LABEL}}") {
			t.Fatalf("%s : marqueurs {{MIKCLOUD_WHATSAPP_*}} absents — le numéro support n'est plus templatisé", page)
		}
		if strings.Contains(body, "wa.me/2250150491807") || strings.Contains(body, "01 5049 1807") {
			t.Fatalf("%s : porte encore le numéro support codé en dur — il doit venir des marqueurs (repli serveur)", page)
		}
	}
	// Le login porte l'infrastructure du pilotage live (bloc 11).
	login := RawFile("login.html")
	if !strings.Contains(login, `id="mikcloud-wa-link"`) || !strings.Contains(login, `id="mikcloud-wa-label"`) {
		t.Fatal("l'ancre/span du pilotage live WhatsApp absents de login.html (bloc 11 sans cible)")
	}
	if !strings.Contains(login, "cfg.portalWhatsapp") {
		t.Fatal("le bloc 11 de applyConfig (cfg.portalWhatsapp) absent de login.html")
	}
}
