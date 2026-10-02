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
	"net/http"
	"net/http/httptest"
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
