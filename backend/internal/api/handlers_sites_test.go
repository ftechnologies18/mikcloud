// Package api — tests N°182 : sites physiques + personnalisation du portail
// captif par site et par routeur (chaîne ROUTEUR → SITE → COMPTE).
//
// Familles :
//   - CRUD /api/sites (cycle de vie, validations, plafond, isolation) ;
//   - assignation PUT /api/routers/{id}/site (attache/détache, cross-compte) ;
//   - surcharge PUT /api/routers/{id}/portal (canonique, reset) ;
//   - résolution resolvePortalBranding (héritage champ par champ 3 niveaux) ;
//   - miroir de signature hotspotFilesSig (rétrocompat v2 + parts site/rovr) ;
//   - servage effectif : buildPortalConfig (waveLink effectif des offres) et
//     l'aperçu console portal-preview reflète la chaîne.
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

// seedSiteInAccount — injecte un site directement dans le store (bypass des
// endpoints pour semer des états précis : surcharge canonique posée à la main).
func seedSiteInAccount(t *testing.T, st interface {
	Lock()
	Unlock()
	Save()
	Data() *model.DB
}, accID, id, name, override string) {
	t.Helper()
	st.Lock()
	defer st.Unlock()
	st.Data().Sites = append(st.Data().Sites, model.Site{
		ID: id, AccountID: accID, Name: name, PortalOverride: override,
		CreatedAt: model.NowISO(), UpdatedAt: model.NowISO(),
	})
	st.Save()
}

// overrideBody — corps de surcharge JSON prêt pour les endpoints.
func overrideBody(displayName string, extra map[string]any) map[string]any {
	body := map[string]any{}
	if displayName != "" {
		body["displayName"] = displayName
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

// doGetArray — requête GET authentifiée dont la réponse est un TABLEAU JSON
// (doJSON décode en map : inutilisable pour les listes).
func doGetArray(t *testing.T, ts *httptest.Server, path, token string) []any {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s : %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s : statut %d", path, resp.StatusCode)
	}
	var out []any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("GET %s : réponse non tableau : %v", path, err)
	}
	return out
}

// TestSitesCRUDLifecycle — POST → GET → PUT → DELETE : cycle complet d'un
// site, compteurs de routeurs compris (2 assignés, 1 détaché à la suppression).
func TestSitesCRUDLifecycle(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "gerant-sites-crud", "")
	routerID := seedRouterInAccount(t, st, accID, "CRUD Router")

	// POST — création avec surcharge complète (nom affiché + ticker).
	status, out := doJSON(t, ts, "POST", "/api/sites", token, map[string]any{
		"name":        "Site Riviera",
		"description": "Bâtiment principal",
		"location":    "Abidjan, Riviera",
		"portalOverride": map[string]any{
			"displayName":  "WiFi Riviera",
			"portalTicker": []string{"Bienvenue à la Riviera !", "Wifi gratuit 30 min"},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("POST /api/sites : statut %d, corps %v", status, out)
	}
	siteID, _ := out["id"].(string)
	if siteID == "" {
		t.Fatal("la création doit renvoyer l'id du site")
	}
	if got, _ := out["hasOverride"].(bool); !got {
		t.Fatal("hasOverride attendu true (ticker + displayName posés)")
	}

	// PUT /api/routers/{id}/site — assignation du routeur.
	status, _ = doJSON(t, ts, "PUT", "/api/routers/"+routerID+"/site", token, map[string]any{"siteId": siteID})
	if status != http.StatusOK {
		t.Fatalf("assignation : statut %d", status)
	}

	// GET — le site porte 1 routeur.
	sites := doGetArray(t, ts, "/api/sites", token)
	if len(sites) != 1 {
		t.Fatalf("1 site attendu, obtenu %d", len(sites))
	}
	site := sites[0].(map[string]any)
	if got, _ := site["routerCount"].(float64); got != 1 {
		t.Fatalf("routerCount attendu 1, obtenu %v", site["routerCount"])
	}

	// PUT — édition : renommage + surcharge réinitialisée (override vide).
	status, _ = doJSON(t, ts, "PUT", "/api/sites/"+siteID, token, map[string]any{
		"name":           "Site Riviera 2",
		"portalOverride": map[string]any{},
	})
	if status != http.StatusOK {
		t.Fatalf("PUT /api/sites/{id} : statut %d", status)
	}
	st.Lock()
	saved := model.FindSiteScoped(st.Data(), siteID, accID)
	if saved == nil || saved.Name != "Site Riviera 2" {
		st.Unlock()
		t.Fatalf("renommage non persisté : %+v", saved)
	}
	if saved.PortalOverride != "" {
		st.Unlock()
		t.Fatalf("override vidé attendu (\"\"), obtenu %q", saved.PortalOverride)
	}
	st.Unlock()

	// DELETE — le routeur assigné est détaché proprement.
	status, out = doJSON(t, ts, "DELETE", "/api/sites/"+siteID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("DELETE : statut %d", status)
	}
	if got, _ := out["detached"].(float64); got != 1 {
		t.Fatalf("detached attendu 1, obtenu %v", out["detached"])
	}
	st.Lock()
	router := findRouterScoped(st.Data(), routerID, accID)
	st.Unlock()
	if router == nil || router.SiteID != "" {
		t.Fatalf("le routeur doit être détaché (SiteID vide), obtenu %+v", router)
	}

	// GET — plus aucun site.
	if sites = doGetArray(t, ts, "/api/sites", token); len(sites) != 0 {
		t.Fatalf("0 site attendu après suppression, obtenu %d", len(sites))
	}
}

// TestSiteCreateValidation — bornes : nom vide/refus, nom trop long, doublon
// par compte, ticker au-delà de 5 messages, WhatsApp mal formé, plafond 20.
func TestSiteCreateValidation(t *testing.T) {
	_, ts := newTestServerWithStore(t)
	token, _, _ := registerAccount(t, ts, "gerant-sites-valid", "")

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"nom manquant", map[string]any{"description": "x"}, http.StatusBadRequest},
		{"nom vide", map[string]any{"name": "   "}, http.StatusBadRequest},
		{"nom trop long", map[string]any{"name": strings.Repeat("s", 61)}, http.StatusBadRequest},
		{"description trop longue", map[string]any{"name": "OK", "description": strings.Repeat("d", 201)}, http.StatusBadRequest},
		{"location trop longue", map[string]any{"name": "OK", "location": strings.Repeat("l", 121)}, http.StatusBadRequest},
		{"ticker > 5", map[string]any{"name": "OK", "portalOverride": map[string]any{
			"portalTicker": []string{"a", "b", "c", "d", "e", "f"},
		}}, http.StatusBadRequest},
		{"whatsapp invalide", map[string]any{"name": "OK", "portalOverride": map[string]any{
			"portalWhatsapp": map[string]any{"number": "abc"},
		}}, http.StatusBadRequest},
		{"displayName trop long", map[string]any{"name": "OK", "portalOverride": map[string]any{
			"displayName": strings.Repeat("n", 81),
		}}, http.StatusBadRequest},
		{"logo non data-URL", map[string]any{"name": "OK", "portalOverride": map[string]any{
			"logoUrl": "https://example.ci/logo.png",
		}}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		status, _ := doJSON(t, ts, "POST", "/api/sites", token, tc.body)
		if status != tc.want {
			t.Errorf("%s : statut %d, attendu %d", tc.name, status, tc.want)
		}
	}

	// Nom valide + logo data URL = accepté.
	status, _ := doJSON(t, ts, "POST", "/api/sites", token, map[string]any{
		"name": "Site Valide",
		"portalOverride": map[string]any{
			"logoUrl": "data:image/png;base64,iVBORw0KGgo=",
		},
	})
	if status != http.StatusOK {
		t.Errorf("site valide refusé : statut %d", status)
	}

	// Doublon de nom (casse ignorée) → 400.
	status, _ = doJSON(t, ts, "POST", "/api/sites", token, map[string]any{"name": "SITE VALIDE"})
	if status != http.StatusBadRequest {
		t.Errorf("doublon de nom : statut %d, attendu 400", status)
	}
}

// TestSitesMaxPerAccount — plafond de 20 sites par compte.
func TestSitesMaxPerAccount(t *testing.T) {
	_, ts := newTestServerWithStore(t)
	token, _, _ := registerAccount(t, ts, "gerant-sites-max", "")
	for i := 0; i < model.MaxSitesPerAccount; i++ {
		status, _ := doJSON(t, ts, "POST", "/api/sites", token, map[string]any{
			"name": "Site " + string(rune('A'+i)),
		})
		if status != http.StatusOK {
			t.Fatalf("site %d : statut %d", i, status)
		}
	}
	status, out := doJSON(t, ts, "POST", "/api/sites", token, map[string]any{"name": "Site En Trop"})
	if status != http.StatusBadRequest {
		t.Fatalf("21e site : statut %d, attendu 400, corps %v", status, out)
	}
}

// TestSitesIsolationCrossAccount — les sites d'un compte sont invisibles et
// immuables pour un autre compte (liste vide, PUT/DELETE 404, assignation
// d'un site étranger refusée).
func TestSitesIsolationCrossAccount(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accA, _ := registerAccount(t, ts, "gerant-iso-a", "")
	// Compte B : inscription directe avec un téléphone DISTINCT (le registre
	// refuse deux comptes sur un même numéro — garde anti-abus N°50).
	tokenB, accB, _ := func() (string, string, string) {
		status, out := doJSON(t, ts, "POST", "/api/auth/register", "", map[string]string{
			"name": "Gérant Iso B", "username": "gerant-iso-b", "password": "mot-de-passe-8+",
			"email": "gerant-iso-b@example.ci", "phone": "0707070708", "country": "CI", "city": "Abidjan",
		})
		if status != http.StatusCreated {
			t.Fatalf("inscription B : statut %d, corps %v", status, out)
		}
		tok, _ := out["token"].(string)
		user, _ := out["user"].(map[string]any)
		acc, _ := user["accountId"].(string)
		usr, _ := user["id"].(string)
		return tok, acc, usr
	}()
	routerB := seedRouterInAccount(t, st, accB, "Iso Router B")

	seedSiteInAccount(t, st, accA, "st-iso-a", "Site A", "")

	// B ne voit pas les sites de A.
	if sites := doGetArray(t, ts, "/api/sites", tokenB); len(sites) != 0 {
		t.Fatalf("B ne doit voir aucun site, obtenu %d", len(sites))
	}
	// B ne peut ni éditer ni supprimer le site de A.
	if status, _ := doJSON(t, ts, "PUT", "/api/sites/st-iso-a", tokenB, map[string]any{"name": "Pirate"}); status != http.StatusNotFound {
		t.Errorf("PUT site d'un autre compte : statut %d, attendu 404", status)
	}
	if status, _ := doJSON(t, ts, "DELETE", "/api/sites/st-iso-a", tokenB, nil); status != http.StatusNotFound {
		t.Errorf("DELETE site d'un autre compte : statut %d, attendu 404", status)
	}
	// B ne peut pas assigner son routeur au site de A.
	if status, _ := doJSON(t, ts, "PUT", "/api/routers/"+routerB+"/site", tokenB, map[string]any{"siteId": "st-iso-a"}); status != http.StatusBadRequest {
		t.Errorf("assignation d'un site étranger : statut %d, attendu 400", status)
	}
	// La résolution du portail ignore un SiteID pointant hors du compte.
	st.Lock()
	router := findRouterScoped(st.Data(), routerB, accB)
	router.SiteID = "st-iso-a" // corruption directe : l'isolation se révérifie à la lecture
	st.Save()
	st.Unlock()
	st.Lock()
	db := st.Data()
	b := resolvePortalBranding(db, accB, router)
	st.Unlock()
	if b.DisplayName == "" {
		t.Fatal("branding du compte attendu en repli")
	}
}

// TestRouterSiteAssignFlows — assignation puis détachement, site inconnu,
// routeur inconnu, message d'activité.
func TestRouterSiteAssignFlows(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "gerant-assign", "")
	routerID := seedRouterInAccount(t, st, accID, "Assign Router")
	seedSiteInAccount(t, st, accID, "st-assign", "Site Assign", "")

	if status, _ := doJSON(t, ts, "PUT", "/api/routers/"+routerID+"/site", token, map[string]any{"siteId": "st-assign"}); status != http.StatusOK {
		t.Fatalf("assignation : statut inattendu")
	}
	st.Lock()
	router := findRouterScoped(st.Data(), routerID, accID)
	st.Unlock()
	if router == nil || router.SiteID != "st-assign" {
		t.Fatalf("SiteID attendu st-assign, obtenu %+v", router)
	}

	// Site inconnu → 400.
	if status, _ := doJSON(t, ts, "PUT", "/api/routers/"+routerID+"/site", token, map[string]any{"siteId": "st-ghost"}); status != http.StatusBadRequest {
		t.Errorf("site inconnu : statut %d, attendu 400", status)
	}
	// Routeur inconnu → 404.
	if status, _ := doJSON(t, ts, "PUT", "/api/routers/r-ghost/site", token, map[string]any{"siteId": "st-assign"}); status != http.StatusNotFound {
		t.Errorf("routeur inconnu : statut %d, attendu 404", status)
	}
	// Détachement ("") → SiteID vide.
	if status, _ := doJSON(t, ts, "PUT", "/api/routers/"+routerID+"/site", token, map[string]any{"siteId": ""}); status != http.StatusOK {
		t.Fatalf("détachement : statut inattendu")
	}
	st.Lock()
	router = findRouterScoped(st.Data(), routerID, accID)
	st.Unlock()
	if router == nil || router.SiteID != "" {
		t.Fatalf("SiteID attendu vide après détachement, obtenu %+v", router)
	}
}

// TestRouterPortalOverridePut — la surcharge individuelle du routeur est
// canonique (champs vides omis, "" si vide) et réinitialisable.
func TestRouterPortalOverridePut(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "gerant-portal-put", "")
	routerID := seedRouterInAccount(t, st, accID, "Override Router")

	status, out := doJSON(t, ts, "PUT", "/api/routers/"+routerID+"/portal", token, map[string]any{
		"displayName":  "Portail Individuel",
		"portalTicker": []string{"Bienvenue !"},
	})
	if status != http.StatusOK {
		t.Fatalf("PUT portal : statut %d, corps %v", status, out)
	}
	st.Lock()
	router := findRouterScoped(st.Data(), routerID, accID)
	st.Unlock()
	if router == nil || router.PortalOverride == "" {
		t.Fatal("surcharge canonique attendue non vide")
	}
	var ov model.PortalOverride
	if err := json.Unmarshal([]byte(router.PortalOverride), &ov); err != nil {
		t.Fatalf("surcharge non décodable : %v", err)
	}
	if ov.DisplayName != "Portail Individuel" || len(ov.Ticker) != 1 || ov.Ticker[0] != "Bienvenue !" {
		t.Fatalf("surcharge inattendue : %+v", ov)
	}
	if ov.LogoURL != "" {
		t.Fatalf("champ vide doit être omis du canonique, obtenu %q", ov.LogoURL)
	}

	// Réinitialisation : corps entièrement vide → "" (hérite site puis compte).
	status, _ = doJSON(t, ts, "PUT", "/api/routers/"+routerID+"/portal", token, map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("reset portal : statut %d", status)
	}
	st.Lock()
	router = findRouterScoped(st.Data(), routerID, accID)
	st.Unlock()
	if router == nil || router.PortalOverride != "" {
		t.Fatalf("reset attendu (\"\"), obtenu %q", router.PortalOverride)
	}

	// Validation partagée : ticker > 5 → 400.
	status, _ = doJSON(t, ts, "PUT", "/api/routers/"+routerID+"/portal", token, map[string]any{
		"portalTicker": []string{"1", "2", "3", "4", "5", "6"},
	})
	if status != http.StatusBadRequest {
		t.Errorf("ticker > 5 sur routeur : statut %d, attendu 400", status)
	}
}

// TestPortalBrandingInheritanceChain — LA chaîne N°182 : le branding effectif
// part du compte, la surcharge du SITE écrase champ par champ, la surcharge du
// ROUTEUR écrase en dernier. Champ vide = hérite (jamais de masquage).
func TestPortalBrandingInheritanceChain(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accID, _ := registerAccount(t, ts, "gerant-chaine", "")

	// Compte : nom + logo + waveLink + ticker.
	st.Lock()
	db := st.Data()
	settings := ensureSettings(db, accID)
	settings.Tenant.Name = "Compte Principal"
	settings.Tenant.LogoURL = "data:image/png;base64,COMPTE"
	settings.Tenant.WaveLink = "https://pay.wave.com/m/compte/c/ci/"
	settings.Tenant.PortalTicker = `["message compte"]`
	db.SettingsByAccount[accID] = settings
	st.Save()
	st.Unlock()

	// Site : nom affiché + logo (waveLink et ticker hérités).
	siteOverride := model.CanonicalPortalOverride(model.PortalOverride{
		DisplayName: "Site Cocody",
		LogoURL:     "data:image/png;base64,SITE",
	})
	seedSiteInAccount(t, st, accID, "st-chaine", "Cocody", siteOverride)

	// Trois routeurs : hors site / dans le site / dans le site + surcharge individuelle.
	st.Lock()
	db = st.Data()
	db.Routers = append(db.Routers,
		model.Router{ID: "r-hors-site", AccountID: accID, Name: "Hors Site", Mode: "agent"},
		model.Router{ID: "r-dans-site", AccountID: accID, Name: "Dans Site", Mode: "agent", SiteID: "st-chaine"},
		model.Router{ID: "r-indiv", AccountID: accID, Name: "Individuel", Mode: "agent", SiteID: "st-chaine",
			PortalOverride: model.CanonicalPortalOverride(model.PortalOverride{
				DisplayName: "Accueil Gaming",
				WaveLink:    "https://pay.wave.com/m/gaming/c/ci/",
			})},
	)
	st.Save()
	st.Unlock()

	st.Lock()
	db = st.Data()
	defer st.Unlock()

	// Hors site : branding du compte intégral.
	b := resolvePortalBranding(db, accID, &db.Routers[len(db.Routers)-3])
	if b.DisplayName != "Compte Principal" || b.LogoURL != "data:image/png;base64,COMPTE" ||
		b.WaveLink != "https://pay.wave.com/m/compte/c/ci/" || b.TickerJSON != `["message compte"]` {
		t.Fatalf("hors site : branding du compte attendu, obtenu %+v", b)
	}

	// Dans le site : nom + logo du site, waveLink + ticker hérités du compte.
	b = resolvePortalBranding(db, accID, &db.Routers[len(db.Routers)-2])
	if b.DisplayName != "Site Cocody" || b.LogoURL != "data:image/png;base64,SITE" {
		t.Fatalf("dans site : surcharge site attendue, obtenu %+v", b)
	}
	if b.WaveLink != "https://pay.wave.com/m/compte/c/ci/" {
		t.Fatalf("dans site : waveLink hérité du compte attendu, obtenu %q", b.WaveLink)
	}

	// Individuel : DisplayName du routeur, logo du SITE, waveLink du routeur.
	b = resolvePortalBranding(db, accID, &db.Routers[len(db.Routers)-1])
	if b.DisplayName != "Accueil Gaming" {
		t.Fatalf("individuel : DisplayName routeur attendu, obtenu %q", b.DisplayName)
	}
	if b.LogoURL != "data:image/png;base64,SITE" {
		t.Fatalf("individuel : logo hérité du site attendu, obtenu %q", b.LogoURL)
	}
	if b.WaveLink != "https://pay.wave.com/m/gaming/c/ci/" {
		t.Fatalf("individuel : waveLink routeur attendu, obtenu %q", b.WaveLink)
	}
	if b.TickerJSON != `["message compte"]` {
		t.Fatalf("individuel : ticker hérité du compte attendu, obtenu %q", b.TickerJSON)
	}
}

// TestHotspotFilesSigChain — miroir de signature : (1) un site inconnu du
// compte n'altère PAS la sig (repli compte — parts absentes) ; (2) l'assignation
// d'un site du compte change la sig (part site posée) ; (3) une surcharge
// routeur change la sig ; (4) détachement + reset → retour à la sig de base.
func TestHotspotFilesSigChain(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accID, _ := registerAccount(t, ts, "gerant-sig", "")
	routerID := seedRouterInAccount(t, st, accID, "Sig Router")

	st.Lock()
	db := st.Data()
	router := findRouterScoped(db, routerID, accID)
	files := []string{"login.html"}
	baseSig := hotspotFilesSig(files, db, router)
	ghostSig := ""
	router.SiteID = "st-etranger" // site d'un autre compte : repli compte
	ghostSig = hotspotFilesSig(files, db, router)
	router.SiteID = ""
	st.Save()
	st.Unlock()

	if baseSig != ghostSig {
		t.Fatalf("un SiteID hors du compte ne doit PAS changer la sig : %q vs %q", baseSig, ghostSig)
	}

	seedSiteInAccount(t, st, accID, "st-sig", "Site Sig", model.CanonicalPortalOverride(model.PortalOverride{
		DisplayName: "Site Sig",
	}))

	st.Lock()
	db = st.Data()
	router = findRouterScoped(db, routerID, accID)
	router.SiteID = "st-sig"
	siteSig := hotspotFilesSig(files, db, router)
	if siteSig == baseSig {
		st.Unlock()
		t.Fatal("l'assignation d'un site du compte DOIT changer la sig (re-déploiement attendu)")
	}
	router.PortalOverride = model.CanonicalPortalOverride(model.PortalOverride{DisplayName: "Indiv"})
	indivSig := hotspotFilesSig(files, db, router)
	if indivSig == siteSig {
		st.Unlock()
		t.Fatal("la surcharge routeur DOIT changer la sig")
	}
	// Retour à l'état vierge : la sig revient EXACTEMENT à la base (parts
	// absentes) — pas de dérive cumulative.
	router.SiteID = ""
	router.PortalOverride = ""
	resetSig := hotspotFilesSig(files, db, router)
	st.Unlock()
	if resetSig != baseSig {
		t.Fatalf("retour à l'état vierge : sig attendue %q, obtenue %q", baseSig, resetSig)
	}
}

// TestBuildPortalConfigEffectiveWaveLink — les offres payantes portent le
// waveUrl du lien marchand EFFECTIF : celui du compte hors site, celui du
// site une fois assigné.
func TestBuildPortalConfigEffectiveWaveLink(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	_, accID, _ := registerAccount(t, ts, "gerant-wave", "")

	st.Lock()
	db := st.Data()
	settings := ensureSettings(db, accID)
	settings.Tenant.Name = "Wave Compte"
	settings.Tenant.WaveLink = "https://pay.wave.com/m/compte/c/ci/"
	db.SettingsByAccount[accID] = settings
	db.Profiles = append(db.Profiles, model.Profile{ID: "p-wave", AccountID: accID, Name: "1 Heure", Price: 200})
	db.Routers = append(db.Routers, model.Router{ID: "r-wave", AccountID: accID, Name: "Wave Router", Mode: "agent"})
	st.Save()
	st.Unlock()

	waveSiteOverride := model.CanonicalPortalOverride(model.PortalOverride{
		WaveLink: "https://pay.wave.com/m/monsite/c/ci/",
	})
	seedSiteInAccount(t, st, accID, "st-wave", "Site Wave", waveSiteOverride)

	st.Lock()
	db = st.Data()
	router := findRouterScoped(db, routerIDOf(db, accID, "Wave Router"), accID)
	req, _ := http.NewRequest("GET", "/", nil) // buildPortalConfig lit l'origine (agentBaseURL)

	cfg := buildPortalConfig(db, router, req)
	if len(cfg.Offers) != 1 || cfg.Offers[0].WaveURL != "https://pay.wave.com/m/compte/c/ci/amount/200/" {
		st.Unlock()
		t.Fatalf("offre hors site : waveUrl du compte attendu, obtenu %+v", cfg.Offers)
	}

	router.SiteID = "st-wave"
	cfg = buildPortalConfig(db, router, req)
	if len(cfg.Offers) != 1 || cfg.Offers[0].WaveURL != "https://pay.wave.com/m/monsite/c/ci/amount/200/" {
		st.Unlock()
		t.Fatalf("offre dans site : waveUrl du site attendu, obtenu %+v", cfg.Offers)
	}
	st.Unlock()
}

// routerIDOf — retrouve l'ID du routeur par nom dans le compte (helper de test).
func routerIDOf(db *model.DB, accID, name string) string {
	for i := range db.Routers {
		if db.Routers[i].AccountID == accID && db.Routers[i].Name == name {
			return db.Routers[i].ID
		}
	}
	return ""
}

// TestPortalPreviewResolvesChain — l'aperçu console reflète la chaîne : le
// HTML servi contient le DisplayName du SITE (pas le nom du compte).
func TestPortalPreviewResolvesChain(t *testing.T) {
	st, ts := newTestServerWithStore(t)
	token, accID, _ := registerAccount(t, ts, "gerant-preview", "")
	routerID := seedRouterInAccount(t, st, accID, "Preview Router")

	st.Lock()
	db := st.Data()
	settings := ensureSettings(db, accID)
	settings.Tenant.Name = "Nom Du Compte"
	db.SettingsByAccount[accID] = settings
	st.Save()
	st.Unlock()

	seedSiteInAccount(t, st, accID, "st-preview", "Site Preview", model.CanonicalPortalOverride(model.PortalOverride{
		DisplayName: "Hotel Palm Beach",
	}))
	doJSON(t, ts, "PUT", "/api/routers/"+routerID+"/site", token, map[string]any{"siteId": "st-preview"})

	req, _ := http.NewRequest("GET", ts.URL+"/api/routers/"+routerID+"/portal-preview", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("preview : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview : statut %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Hotel Palm Beach") {
		t.Fatal("l'aperçu doit porter le DisplayName du site (chaîne N°182)")
	}
}
