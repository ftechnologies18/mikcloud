// Tests N°205 « sortie des médias du tuyau Render » : réécriture à la volée
// des URL média proxy (…/api/media/media/{compte}/{année}/{hex}.{ext}) vers
// le domaine public R2 (R2_PUBLIC_BASE), upload servant l'URL publique, et
// propagation du domaine dans le walled-garden + l'empreinte du portail.
//
// Contexte facturation (septembre 2026) : Render Hobby n'inclut que 5 Go de
// bande passante sortante par mois (0,15 $/Go au-delà) ; les bannières du
// portail, servies par le proxy backend à CHAQUE chargement de page captive,
// ont produit la facture de septembre (~13 $). R2 ne facture pas la sortie :
// les images migrent sur le domaine public du bucket, le proxy reste en
// repli. La base publique VIDE doit laisser tout inchangé (rétrocompat).
package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

// hex32 — clé valide de mediaKeyRe (hex minuscules, 128 bits).
const testHex32 = "0123456789abcdef0123456789abcdef"

// TestMediaRewriteURL — la réécriture ne touche QUE nos clés proxy valides :
// URL absolue (n'importe quelle origine), URL relative, clé hex 128 bits et
// extension whitelistée. Tout le reste (URL externe, data:image/, clé
// invalide, vide) et la base publique vide laissent la valeur intacte.
func TestMediaRewriteURL(t *testing.T) {
	proxy := "https://mikcloud.onrender.com/api/media/media/acc-x/2026/" + testHex32 + ".webp"
	proxyOtherOrigin := "https://autredomaine.ftci.fr/api/media/media/acc-y/2026/ffffffffffffffffffffffffffffffff.jpg"
	external := "https://cdn.exemple.net/img/banniere.jpg"
	dataURL := "data:image/png;base64,iVBORw0KGgo="

	// Base publique vide ⇒ identité stricte (comportement historique).
	t.Setenv("R2_PUBLIC_BASE", "")
	for _, u := range []string{proxy, proxyOtherOrigin, external, dataURL, ""} {
		if got := mediaRewriteURL(u); got != u {
			t.Fatalf("base vide : mediaRewriteURL(%q) = %q, voulu identité", u, got)
		}
	}

	// Base configurée (slash final toléré et normalisé).
	t.Setenv("R2_PUBLIC_BASE", "https://media.ftci.fr/")
	cases := []struct{ name, in, want string }{
		{"proxy absolu", proxy, "https://media.ftci.fr/media/acc-x/2026/" + testHex32 + ".webp"},
		{"proxy autre origine", proxyOtherOrigin, "https://media.ftci.fr/media/acc-y/2026/ffffffffffffffffffffffffffffffff.jpg"},
		{"externe intact", external, external},
		{"data URL intacte", dataURL, dataURL},
		{"vide intact", "", ""},
		{"chemin sans clé hex", "https://mikcloud.onrender.com/api/media/media/acc-x/2026/pas-un-hex.jpg", "https://mikcloud.onrender.com/api/media/media/acc-x/2026/pas-un-hex.jpg"},
		{"chemin autre", "https://mikcloud.onrender.com/api/autres/chose.jpg", "https://mikcloud.onrender.com/api/autres/chose.jpg"},
		{"extension hors liste", "https://mikcloud.onrender.com/api/media/media/acc-x/2026/" + testHex32 + ".txt", "https://mikcloud.onrender.com/api/media/media/acc-x/2026/" + testHex32 + ".txt"},
	}
	for _, c := range cases {
		if got := mediaRewriteURL(c.in); got != c.want {
			t.Fatalf("%s : mediaRewriteURL(%q) = %q, voulu %q", c.name, c.in, got, c.want)
		}
	}
}

// TestMediaServeURL — l'URL servie pour une clé fraîche : base publique si
// configurée, sinon le proxy sur l'origine de la requête (même hôte que
// apiBase — joignable pré-auth, walled-garden N°48).
func TestMediaServeURL(t *testing.T) {
	key := "media/acc-z/2026/" + testHex32 + ".png"
	r := httptest.NewRequest(http.MethodPost, "https://mikcloud.onrender.com/api/media", nil)

	t.Setenv("R2_PUBLIC_BASE", "https://media.ftci.fr")
	if got := mediaServeURL(r, key); got != "https://media.ftci.fr/"+key {
		t.Fatalf("base publique : mediaServeURL = %q, voulu l'URL R2", got)
	}

	t.Setenv("R2_PUBLIC_BASE", "")
	if got := mediaServeURL(r, key); got != "https://mikcloud.onrender.com/api/media/"+key {
		t.Fatalf("repli proxy : mediaServeURL = %q, voulu l'URL proxy", got)
	}
}

// TestWalledGardenDomainsIncludeMediaHost — R2_PUBLIC_BASE configuré ⇒ son
// hôte rejoint la liste blanche pré-auth (l'invité charge la bannière depuis
// R2, pas à travers le backend) ; non configuré ⇒ liste inchangée.
func TestWalledGardenDomainsIncludeMediaHost(t *testing.T) {
	t.Setenv("MIKCLOUD_BASE_URL", "https://mikcloud.onrender.com")
	t.Setenv("APP_PUBLIC_URL", "")
	t.Setenv("ALLOWED_ORIGIN", "")

	t.Setenv("R2_PUBLIC_BASE", "https://media.ftci.fr")
	found := false
	for _, h := range walledGardenDomains(nil) {
		if h == "media.ftci.fr" {
			found = true
		}
	}
	if !found {
		t.Fatal("R2_PUBLIC_BASE configuré : media.ftci.fr absent du walled-garden")
	}

	t.Setenv("R2_PUBLIC_BASE", "")
	for _, h := range walledGardenDomains(nil) {
		if h == "media.ftci.fr" {
			t.Fatal("R2_PUBLIC_BASE vide : media.ftci.fr ne doit PAS être au walled-garden")
		}
	}
}

// TestResolvePortalBrandingRewritesMedia — la chaîne de branding (compte seul
// puis compte→site→routeur) réécrit les URL proxy en R2 public ; les URL
// externes et les data:image/ passent intactes.
func TestResolvePortalBrandingRewritesMedia(t *testing.T) {
	t.Setenv("R2_PUBLIC_BASE", "https://media.ftci.fr")
	st, _ := newTestServerWithStore(t)
	st.Lock()
	db := st.Data()
	db.Accounts = append(db.Accounts, model.Account{ID: "acc-media-rw", Name: "Media RW"})
	s := ensureSettings(db, "acc-media-rw")
	s.Tenant.LogoURL = "https://mikcloud.onrender.com/api/media/media/acc-media-rw/2026/" + testHex32 + ".png"
	s.Tenant.BannerURL = "https://mikcloud.onrender.com/api/media/media/acc-media-rw/2026/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.webp"
	db.SettingsByAccount["acc-media-rw"] = s

	// Compte seul (router nil — fetch live d'un site sans routeur).
	b := resolvePortalBranding(db, "acc-media-rw", nil)
	st.Unlock()
	if want := "https://media.ftci.fr/media/acc-media-rw/2026/" + testHex32 + ".png"; b.LogoURL != want {
		t.Fatalf("logo réécrit : %q, voulu %q", b.LogoURL, want)
	}
	if want := "https://media.ftci.fr/media/acc-media-rw/2026/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.webp"; b.BannerURL != want {
		t.Fatalf("bannière réécrite : %q, voulu %q", b.BannerURL, want)
	}
}

// TestResolvePortalBrandingExternalMediaUntouched — un logo externe (autre
// hébergeur) et une bannière intégrée (data:image/) ne sont JAMAIS réécrits.
func TestResolvePortalBrandingExternalMediaUntouched(t *testing.T) {
	t.Setenv("R2_PUBLIC_BASE", "https://media.ftci.fr")
	st, _ := newTestServerWithStore(t)
	st.Lock()
	db := st.Data()
	db.Accounts = append(db.Accounts, model.Account{ID: "acc-media-ext", Name: "Media Ext"})
	s := ensureSettings(db, "acc-media-ext")
	s.Tenant.LogoURL = "https://cdn.exemple.net/logo.png"
	s.Tenant.BannerURL = "data:image/png;base64,iVBORw0KGgoAAAANS"
	db.SettingsByAccount["acc-media-ext"] = s
	b := resolvePortalBranding(db, "acc-media-ext", nil)
	st.Unlock()
	if b.LogoURL != "https://cdn.exemple.net/logo.png" || b.BannerURL != "data:image/png;base64,iVBORw0KGgoAAAANS" {
		t.Fatalf("URL externes réécrites à tort : logo=%q banner=%q", b.LogoURL, b.BannerURL)
	}
}

// TestWifiSiteInfoBannerRewritten — l'endpoint PUBLIC /api/wifi/site/{slug}
// sert la bannière réécrite : le navigateur de l'invité charge l'image depuis
// R2, plus à travers le proxy Render (le plus gros poste de la facture).
func TestWifiSiteInfoBannerRewritten(t *testing.T) {
	t.Setenv("R2_PUBLIC_BASE", "https://media.ftci.fr")
	ts, st := newWifiTestServer(t)
	_, accID, _ := registerAccount(t, ts, "gerant-media-rw", "")
	routerID, profileID := seedWifiEnv(t, st, accID)
	seedWifiSite(t, st, accID, "media-rw", routerID, profileID, true, 1, 100)

	st.Lock()
	db := st.Data()
	s := ensureSettings(db, accID)
	s.Tenant.BannerURL = "https://mikcloud.onrender.com/api/media/media/" + accID + "/2026/" + testHex32 + ".webp"
	s.Tenant.LogoURL = "https://mikcloud.onrender.com/api/media/media/" + accID + "/2026/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.jpg"
	db.SettingsByAccount[accID] = s
	st.Unlock()

	status, out := doJSON(t, ts, "GET", "/api/wifi/site/media-rw", "", nil)
	if status != http.StatusOK {
		t.Fatalf("info site : statut %d, corps %v", status, out)
	}
	if got, _ := out["bannerUrl"].(string); got != "https://media.ftci.fr/media/"+accID+"/2026/"+testHex32+".webp" {
		t.Fatalf("bannerUrl servi : %q, voulu l'URL R2 publique", got)
	}
	if got, _ := out["logoUrl"].(string); got != "https://media.ftci.fr/media/"+accID+"/2026/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.jpg" {
		t.Fatalf("logoUrl servi : %q, voulu l'URL R2 publique", got)
	}
}

// TestPortalBrandingFingerprintChangesWithMediaBase — R2_PUBLIC_BASE façonne
// les logoUrl/bannerUrl cuits au déploiement : changer la base change la sig
// → la commande hotspot_files est re-filée et le portail re-déployé (les
// pages déjà sur les routeurs portent l'ancienne URL cuite — le fetch live
// prime N°35-c, mais le fallback doit converger aussi).
func TestPortalBrandingFingerprintChangesWithMediaBase(t *testing.T) {
	st, _ := newTestServerWithStore(t)
	st.Lock()
	db := st.Data()
	db.Accounts = append(db.Accounts, model.Account{ID: "acc-fp-media", Name: "FP Media"})
	db.Routers = append(db.Routers, model.Router{
		ID: "r-fp-media", AccountID: "acc-fp-media", Name: "FPRouter",
		Mode: "agent", Status: "online",
	})
	router := &db.Routers[len(db.Routers)-1]
	sig1 := portalBrandingFingerprint(db, router)
	st.Unlock()

	t.Setenv("R2_PUBLIC_BASE", "https://media.ftci.fr")
	st.Lock()
	sig2 := portalBrandingFingerprint(db, router)
	st.Unlock()
	if sig1 == "" {
		t.Fatalf("empreinte vide inattendue")
	}
	if sig1 == sig2 {
		t.Fatal("poser R2_PUBLIC_BASE doit changer l'empreinte branding (re-déploiement du portail attendu)")
	}
}

// ---------------------------------------------------------------------------
// N°209 — la fuite résiduelle du 02/10/2026 : les SLIDES (N°136) et les
// IMAGES DE PROMOS (N°54) n'étaient PAS réécrits vers R2 public (N°205 ne
// couvrait que logo+bannière) — la catégorie « medias » du compteur N°72
// comptait 83,7 Mo en 8,5 h post-fix, portée par le carrousel servi à chaque
// chargement de portail. Le proxy devient par ailleurs un AIGUILLEUR (302
// vers R2 public, ~300 o au lieu de ~230 Ko par requête résiduelle).
// ---------------------------------------------------------------------------

// TestRewriteSlidesMedia — réécriture du JSON de slides : chaque URL proxy
// devient R2 public, les URL externes/data: passent intactes, et une string
// sans URL proxy ressort BINAIRE IDENTIQUE (aucun reformatage parasite).
func TestRewriteSlidesMedia(t *testing.T) {
	t.Setenv("R2_PUBLIC_BASE", "https://media.ftci.fr")
	proxy := "https://mikcloud.onrender.com/api/media/media/acc-s/2026/" + testHex32 + ".jpg"
	external := "https://cdn.exemple.net/slide.jpg"

	in := `["` + proxy + `","` + external + `"]`
	want := `["https://media.ftci.fr/media/acc-s/2026/` + testHex32 + `.jpg","` + external + `"]`
	if got := rewriteSlidesMedia(in); got != want {
		t.Fatalf("slides réécrites : %q, voulu %q", got, want)
	}

	// Identité stricte : rien à réécrire.
	for name, s := range map[string]string{
		"vide":            "",
		"externes seules": `["` + external + `","` + external + `"]`,
		"sans api/media":  `["https://a.fr/x.jpg"]`,
		"JSON invalide":   `{pas du json`,
		"data URLs":       `["data:image/png;base64,iVBOR"]`,
		"structure objet": `{"slides":[]}`,
	} {
		if got := rewriteSlidesMedia(s); got != s {
			t.Fatalf("%s : identité attendue, got %q", name, got)
		}
	}

	// Base vide ⇒ identité même avec URL proxy (rétrocompat).
	t.Setenv("R2_PUBLIC_BASE", "")
	if got := rewriteSlidesMedia(in); got != in {
		t.Fatalf("base vide : identité attendue, got %q", got)
	}
}

// TestRewritePromosMedia — réécriture du champ imageUrl des promos
// hospitalité : les AUTRES clés et lignes sont préservées mot pour mot au
// re-encodage, et une string sans URL proxy ressort identique.
func TestRewritePromosMedia(t *testing.T) {
	t.Setenv("R2_PUBLIC_BASE", "https://media.ftci.fr")
	proxy := "https://mikcloud.onrender.com/api/media/media/acc-p/2026/" + testHex32 + ".webp"

	in := `[{"title":"Jus","desc":"Froid","imageUrl":"` + proxy + `","priceLabel":"500 XOF"},{"title":"Sans image","desc":"ok"}]`
	got := rewritePromosMedia(in)
	if !strings.Contains(got, `"imageUrl":"https://media.ftci.fr/media/acc-p/2026/`+testHex32+`.webp"`) {
		t.Fatalf("imageUrl promo non réécrit : %q", got)
	}
	if !strings.Contains(got, `"title":"Jus"`) || !strings.Contains(got, `"title":"Sans image"`) || !strings.Contains(got, `"priceLabel":"500 XOF"`) {
		t.Fatalf("champs voisins perdus au re-encodage : %q", got)
	}
	if strings.Contains(got, "/api/media/") {
		t.Fatalf("URL proxy résiduelle : %q", got)
	}

	// Identité stricte : rien à réécrire.
	for name, s := range map[string]string{
		"vide":             "",
		"promo sans img":   `[{"title":"X"}]`,
		"img externe":      `[{"imageUrl":"https://cdn.fr/a.jpg"}]`,
		"JSON invalide":    `[pas du json`,
		"imageUrl non str": `[{"imageUrl":42}]`,
	} {
		if g := rewritePromosMedia(s); g != s {
			t.Fatalf("%s : identité attendue, got %q", name, g)
		}
	}
}

// TestResolvePortalBrandingRewritesSlidesAndPromos — bout en bout branding :
// un compte avec carrousel + promos proxy voit ses SlidesJSON/PromosJSON
// réécrits à la résolution (la chaîne compte→site→routeur du N°205 s'applique
// désormais à TOUTES les images du portail).
func TestResolvePortalBrandingRewritesSlidesAndPromos(t *testing.T) {
	t.Setenv("R2_PUBLIC_BASE", "https://media.ftci.fr")
	st, _ := newTestServerWithStore(t)
	st.Lock()
	db := st.Data()
	db.Accounts = append(db.Accounts, model.Account{ID: "acc-media-sp", Name: "Media SP"})
	s := ensureSettings(db, "acc-media-sp")
	s.Tenant.PortalSlides = `[` +
		`"https://mikcloud.onrender.com/api/media/media/acc-media-sp/2026/` + testHex32 + `.jpg",` +
		`"https://cdn.exemple.net/externe.jpg"]`
	s.Tenant.PortalPromos = `[{"title":"Menu","imageUrl":"https://mikcloud.onrender.com/api/media/media/acc-media-sp/2026/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.png","priceLabel":"1 000 XOF"}]`
	db.SettingsByAccount["acc-media-sp"] = s
	b := resolvePortalBranding(db, "acc-media-sp", nil)
	st.Unlock()

	wantSlides := `["https://media.ftci.fr/media/acc-media-sp/2026/` + testHex32 + `.jpg","https://cdn.exemple.net/externe.jpg"]`
	if b.SlidesJSON != wantSlides {
		t.Fatalf("slides réécrits : %q, voulu %q", b.SlidesJSON, wantSlides)
	}
	if !strings.Contains(b.PromosJSON, `"imageUrl":"https://media.ftci.fr/media/acc-media-sp/2026/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.png"`) {
		t.Fatalf("imageUrl promo non réécrit : %q", b.PromosJSON)
	}
	// La config aval (portalSlidesList) décode les URLs réécrites telles quelles.
	slides := portalSlidesList(b.SlidesJSON)
	if len(slides) != 2 || slides[0] != "https://media.ftci.fr/media/acc-media-sp/2026/"+testHex32+".jpg" || slides[1] != "https://cdn.exemple.net/externe.jpg" {
		t.Fatalf("portalSlidesList sur slides réécrits : %v", slides)
	}
}

// TestMediaGetRedirectsToPublicBase — N°209 : avec R2_PUBLIC_BASE posée, le
// proxy ne stream plus — il aiguille en 302 vers R2 public (clé validée
// AVANT la redirection : aucune redirection ouverte) avec un cache borné ;
// base vide ⇒ comportement historique (503 media_unconfigured sans config
// R2, la clé invalide reste 404).
func TestMediaGetRedirectsToPublicBase(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_ = st
	key := "media/acc-redir/2026/" + testHex32 + ".webp"
	// NB : ts.Client() + CheckRedirect=ErrUseLastResponse — le client Go
	// SUIT les redirections par défaut (il irait chercher la vraie page R2
	// sur internet) ; le test observe la réponse 302 ELLE-MÊME, pas sa cible.
	cl := ts.Client()
	cl.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	get := func(path string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		if err != nil {
			t.Fatalf("requête impossible : %v", err)
		}
		resp, err := cl.Do(req)
		if err != nil {
			t.Fatalf("GET %s : %v", path, err)
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return resp
	}

	t.Setenv("R2_PUBLIC_BASE", "https://media.ftci.fr")
	resp := get("/api/media/" + key)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("statut = %d, voulu 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "https://media.ftci.fr/"+key {
		t.Fatalf("Location = %q, voulu l'URL R2 publique", loc)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Fatalf("Cache-Control = %q, voulu public, max-age=3600", cc)
	}

	// Clé invalide : 404 AVANT toute redirection (pas de redirection ouverte).
	resp = get("/api/media/media/acc-redir/2026/pas-un-hex.jpg")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("clé invalide : statut %d, voulu 404 (jamais une redirection)", resp.StatusCode)
	}

	// Base vide ⇒ proxy historique : sans config R2, 503 media_unconfigured.
	t.Setenv("R2_PUBLIC_BASE", "")
	resp = get("/api/media/" + key)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("base vide sans R2 : statut %d, voulu 503 media_unconfigured (comportement historique)", resp.StatusCode)
	}
}
