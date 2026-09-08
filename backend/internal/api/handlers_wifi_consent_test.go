// Tests N°69 « consentement marketing explicite » : interrupteur du claim
// (OFF par défaut, jamais pré-coché), preuve horodatée (OptInAt), héritage
// (un consentement posé reste acquis — pas de retrait silencieux par
// omission), upgrade au re-claim idempotent, endpoint public /consent
// (retrait « Ne plus recevoir » symétrique, gardes rate-limit + honeypot +
// réduction par site.MarketingOptIn) et export CSV opt_in_since.
package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// guestRows — lignes du registre d'un téléphone sur un compte (sous verrou).
func guestRows(st *store.Store, acc, phone string) []model.WifiGuest {
	var rows []model.WifiGuest
	for _, g := range st.Data().WifiGuests {
		if g.AccountID == acc && g.Phone == phone {
			rows = append(rows, g)
		}
	}
	return rows
}

// TestWifiClaimOptInSwitch — le claim porte l'état de l'interrupteur :
// OFF (défaut, refus sans pénalité) ⇒ OptIn=false, OptInAt vide ;
// ON (geste affirmatif) ⇒ OptIn=true + PREUVE horodatée. La réponse du
// claim expose l'état effectif (la carte code l'affiche).
func TestWifiClaimOptInSwitch(t *testing.T) {
	ts, st := newWifiTestServer(t)
	_, accID, _ := registerAccount(t, ts, "gerant-wifi-switch", "")
	routerID, profileID := seedWifiEnv(t, st, accID)
	seedWifiSite(t, st, accID, "salon-switch", routerID, profileID, true, 5, 100)

	// Refus par omission : interrupteur non posé (optIn=false, défaut UI).
	status, out := doJSON(t, ts, "POST", "/api/wifi/site/salon-switch/claim", "", map[string]any{
		"phone": "2250707080001", "optIn": false,
	})
	if status != http.StatusOK {
		t.Fatalf("claim off : statut %d (corps %v)", status, out)
	}
	if respOpt, _ := out["optIn"].(bool); respOpt {
		t.Fatal("réponse claim : optIn attendu false (interrupteur non posé)")
	}
	st.Lock()
	rows := guestRows(st, accID, "2250707080001")
	st.Unlock()
	if len(rows) != 1 || rows[0].OptIn || rows[0].OptInAt != "" {
		t.Fatalf("registre off : %+v (voulu 1 ligne, OptIn=false, OptInAt vide)", rows)
	}

	// Consentement affirmatif : interrupteur posé (optIn=true).
	status, out = doJSON(t, ts, "POST", "/api/wifi/site/salon-switch/claim", "", map[string]any{
		"phone": "2250707080002", "optIn": true,
	})
	if status != http.StatusOK {
		t.Fatalf("claim on : statut %d (corps %v)", status, out)
	}
	if respOpt, _ := out["optIn"].(bool); !respOpt {
		t.Fatal("réponse claim : optIn attendu true (interrupteur posé)")
	}
	st.Lock()
	rows = guestRows(st, accID, "2250707080002")
	st.Unlock()
	if len(rows) != 1 || !rows[0].OptIn {
		t.Fatalf("registre on : %+v (voulu 1 ligne, OptIn=true)", rows)
	}
	if rows[0].OptInAt == "" {
		t.Fatal("preuve OptInAt vide — le consentement doit être horodaté (loi 2013-450)")
	}
	if _, err := time.Parse(time.RFC3339, rows[0].OptInAt); err != nil {
		t.Fatalf("OptInAt %q n'est pas un RFC3339 : %v", rows[0].OptInAt, err)
	}
}

// TestWifiClaimOptInMarketingReduction — un opt-in sauvage (interrupteur
// impossible côté UI : le site a le marketing éteint) est réduit à false —
// symétrie exacte avec l'ancien comportement de la case (optIn && site).
func TestWifiClaimOptInMarketingReduction(t *testing.T) {
	ts, st := newWifiTestServer(t)
	_, accID, _ := registerAccount(t, ts, "gerant-wifi-reduc", "")
	routerID, profileID := seedWifiEnv(t, st, accID)
	site := seedWifiSite(t, st, accID, "quiet-place", routerID, profileID, true, 5, 100)

	// Le gérant éteint le marketing sur le site.
	st.Lock()
	for i := range st.Data().WifiSites {
		if st.Data().WifiSites[i].ID == site.ID {
			st.Data().WifiSites[i].MarketingOptIn = false
		}
	}
	st.Unlock()

	status, out := doJSON(t, ts, "POST", "/api/wifi/site/quiet-place/claim", "", map[string]any{
		"phone": "2250707080003", "optIn": true,
	})
	if status != http.StatusOK {
		t.Fatalf("claim : statut %d (corps %v)", status, out)
	}
	if respOpt, _ := out["optIn"].(bool); respOpt {
		t.Fatal("optIn=true doit être réduit à false quand le site a le marketing éteint")
	}
	st.Lock()
	rows := guestRows(st, accID, "2250707080003")
	st.Unlock()
	if len(rows) != 1 || rows[0].OptIn || rows[0].OptInAt != "" {
		t.Fatalf("registre : %+v (voulu OptIn=false, OptInAt vide)", rows)
	}
}

// TestWifiClaimDuplicateUpgrade — au re-claim idempotent (même téléphone,
// même jour, même code), l'interrupteur posé à la re-scan ENREGISTRE le
// consentement immédiatement (pas d'attente au lendemain). Le sens inverse
// n'existe pas : ne pas toucher l'interrupteur n'est pas un retrait.
func TestWifiClaimDuplicateUpgrade(t *testing.T) {
	ts, st := newWifiTestServer(t)
	_, accID, _ := registerAccount(t, ts, "gerant-wifi-dup", "")
	routerID, profileID := seedWifiEnv(t, st, accID)
	seedWifiSite(t, st, accID, "maquis-dup", routerID, profileID, true, 5, 100)

	// 1er claim sans interrupteur.
	status, out := doJSON(t, ts, "POST", "/api/wifi/site/maquis-dup/claim", "", map[string]any{
		"phone": "2250707080004", "optIn": false,
	})
	if status != http.StatusOK {
		t.Fatalf("claim 1 : statut %d (corps %v)", status, out)
	}
	// Re-scan le même jour, interrupteur posé ⇒ MÊME code (duplicate) mais
	// consentement enregistré + horodaté.
	status, out = doJSON(t, ts, "POST", "/api/wifi/site/maquis-dup/claim", "", map[string]any{
		"phone": "2250707080004", "optIn": true,
	})
	if status != http.StatusOK {
		t.Fatalf("claim 2 (duplicate) : statut %d (corps %v)", status, out)
	}
	if dup, _ := out["duplicate"].(bool); !dup {
		t.Fatal("le re-claim du même téléphone doit rester idempotent (duplicate=true)")
	}
	if respOpt, _ := out["optIn"].(bool); !respOpt {
		t.Fatal("duplicate : optIn attendu true (upgrade au re-scan)")
	}
	st.Lock()
	rows := guestRows(st, accID, "2250707080004")
	st.Unlock()
	if len(rows) != 1 || !rows[0].OptIn || rows[0].OptInAt == "" {
		t.Fatalf("registre après upgrade : %+v (voulu 1 ligne OptIn=true + preuve)", rows)
	}
}

// TestWifiClaimConsentInheritance — un numéro déjà consenti garde son
// consentement : claim du lendemain interrupteur non posé ⇒ la nouvelle
// ligne HÉRITE (le retrait est explicite via /consent, jamais une omission).
func TestWifiClaimConsentInheritance(t *testing.T) {
	ts, st := newWifiTestServer(t)
	_, accID, _ := registerAccount(t, ts, "gerant-wifi-inherit", "")
	routerID, profileID := seedWifiEnv(t, st, accID)
	site := seedWifiSite(t, st, accID, "hotel-inherit", routerID, profileID, true, 5, 100)

	// Une ligne D'HIER, consentie (OptIn=true + preuve) pour ce numéro.
	st.Lock()
	yesterday := model.WifiDayKey("Africa/Abidjan", time.Now().UTC().AddDate(0, 0, -1))
	st.Data().WifiGuests = append(st.Data().WifiGuests, model.WifiGuest{
		ID: "wg-old", AccountID: accID, SiteID: site.ID, SiteName: site.Name,
		Phone: "2250707080005", OptIn: true, OptInAt: model.NowISO(),
		Code: "OLD01", Day: yesterday, CreatedAt: model.NowISO(),
	})
	st.Unlock()

	// Claim d'aujourd'hui, interrupteur NON posé (optIn=false) : héritage.
	status, out := doJSON(t, ts, "POST", "/api/wifi/site/hotel-inherit/claim", "", map[string]any{
		"phone": "2250707080005", "optIn": false,
	})
	if status != http.StatusOK {
		t.Fatalf("claim : statut %d (corps %v)", status, out)
	}
	if respOpt, _ := out["optIn"].(bool); !respOpt {
		t.Fatal("l'état effectif doit rester true (héritage du consentement, pas de retrait silencieux)")
	}
	st.Lock()
	rows := guestRows(st, accID, "2250707080005")
	st.Unlock()
	today := 0
	for _, g := range rows {
		if g.ID != "wg-old" {
			today++
			if !g.OptIn || g.OptInAt == "" {
				t.Fatalf("ligne du jour : %+v (voulu OptIn=true hérité + preuve)", g)
			}
		}
	}
	if today != 1 {
		t.Fatalf("lignes du jour : %d, voulu 1", today)
	}
}

// TestWifiConsentWithdrawAndOptIn — l'endpoint public /consent : retrait
// « Ne plus recevoir » (TOUTES les lignes du numéro passent à false, preuve
// effacée), puis opt-in re-posable (preuve horodatée). /status expose
// l'état courant (suit le numéro, pas le jour).
func TestWifiConsentWithdrawAndOptIn(t *testing.T) {
	ts, st := newWifiTestServer(t)
	_, accID, _ := registerAccount(t, ts, "gerant-wifi-consent", "")
	routerID, profileID := seedWifiEnv(t, st, accID)
	seedWifiSite(t, st, accID, "cafe-consent", routerID, profileID, true, 5, 100)

	// Claim consenti (une ligne aujourd'hui).
	doJSON(t, ts, "POST", "/api/wifi/site/cafe-consent/claim", "", map[string]any{
		"phone": "2250707080006", "optIn": true,
	})

	// Retrait : 1 geste, toutes les lignes, preuve effacée.
	status, out := doJSON(t, ts, "POST", "/api/wifi/site/cafe-consent/consent", "", map[string]any{
		"phone": "2250707080006", "optIn": false,
	})
	if status != http.StatusOK {
		t.Fatalf("consent off : statut %d (corps %v)", status, out)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("consent off : ok attendu true, corps %v", out)
	}
	if respOpt, _ := out["optIn"].(bool); respOpt {
		t.Fatal("consent off : optIn attendu false")
	}
	st.Lock()
	rows := guestRows(st, accID, "2250707080006")
	st.Unlock()
	for _, g := range rows {
		if g.OptIn || g.OptInAt != "" {
			t.Fatalf("après retrait : %+v (voulu OptIn=false, preuve effacée)", g)
		}
	}
	// /status reflète l'état du numéro.
	stCode, sOut := doJSON(t, ts, "GET", "/api/wifi/site/cafe-consent/status?phone=2250707080006", "", nil)
	if stCode != http.StatusOK {
		t.Fatalf("status : statut %d", stCode)
	}
	if respOpt, _ := sOut["optIn"].(bool); respOpt {
		t.Fatal("status après retrait : optIn attendu false")
	}

	// Re-activation : preuve horodatée posée à nouveau.
	status, out = doJSON(t, ts, "POST", "/api/wifi/site/cafe-consent/consent", "", map[string]any{
		"phone": "2250707080006", "optIn": true,
	})
	if status != http.StatusOK {
		t.Fatalf("consent on : statut %d (corps %v)", status, out)
	}
	st.Lock()
	rows = guestRows(st, accID, "2250707080006")
	st.Unlock()
	for _, g := range rows {
		if !g.OptIn || g.OptInAt == "" {
			t.Fatalf("après ré-activation : %+v (voulu OptIn=true + preuve)", g)
		}
	}
}

// TestWifiConsentGuards — mêmes garde-fous que le claim : téléphone invalide
// 400, slug inconnu 404, honeypot « website » = succès FACTICE sans aucune
// écriture, marketing éteint ⇒ un opt-in sauvage reste false.
func TestWifiConsentGuards(t *testing.T) {
	ts, st := newWifiTestServer(t)
	_, accID, _ := registerAccount(t, ts, "gerant-wifi-guards", "")
	routerID, profileID := seedWifiEnv(t, st, accID)
	site := seedWifiSite(t, st, accID, "secure-cafe", routerID, profileID, true, 5, 100)

	if status, _ := doJSON(t, ts, "POST", "/api/wifi/site/secure-cafe/consent", "", map[string]any{
		"phone": "12", "optIn": true,
	}); status != http.StatusBadRequest {
		t.Fatalf("téléphone invalide : statut %d, voulu 400", status)
	}
	if status, _ := doJSON(t, ts, "POST", "/api/wifi/site/inconnu/consent", "", map[string]any{
		"phone": "2250707080007", "optIn": true,
	}); status != http.StatusNotFound {
		t.Fatalf("slug inconnu : statut %d, voulu 404", status)
	}

	// Claim consenti, puis tentative de retrait PAR UN BOT (honeypot rempli)
	// : succès factice, l'état réel ne bouge pas.
	doJSON(t, ts, "POST", "/api/wifi/site/secure-cafe/claim", "", map[string]any{
		"phone": "2250707080008", "optIn": true,
	})
	status, out := doJSON(t, ts, "POST", "/api/wifi/site/secure-cafe/consent", "", map[string]any{
		"phone": "2250707080008", "optIn": false, "website": "http://spam.example",
	})
	if status != http.StatusOK {
		t.Fatalf("honeypot : statut %d, voulu 200 (succès factice)", status)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("honeypot : ok factice attendu true, corps %v", out)
	}
	st.Lock()
	rows := guestRows(st, accID, "2250707080008")
	st.Unlock()
	if len(rows) != 1 || !rows[0].OptIn {
		t.Fatalf("honeypot ne doit RIEN écrire : %+v", rows)
	}

	// Marketing éteint : un opt-in sauvage via /consent reste false.
	st.Lock()
	for i := range st.Data().WifiSites {
		if st.Data().WifiSites[i].ID == site.ID {
			st.Data().WifiSites[i].MarketingOptIn = false
		}
	}
	st.Unlock()
	status, out = doJSON(t, ts, "POST", "/api/wifi/site/secure-cafe/consent", "", map[string]any{
		"phone": "2250707080008", "optIn": true,
	})
	if status != http.StatusOK {
		t.Fatalf("consent : statut %d (corps %v)", status, out)
	}
	if respOpt, _ := out["optIn"].(bool); respOpt {
		t.Fatal("opt-in sauvage (marketing éteint) doit être réduit à false")
	}
}

// TestWifiGuestsCSVOptInSince — l'export CSV porte la colonne opt_in_since
// (preuve horodatée du consentement, « - » sinon) et le registre JSON
// expose optInAt.
func TestWifiGuestsCSVOptInSince(t *testing.T) {
	ts, st := newWifiTestServer(t)
	token, accID, _ := registerAccount(t, ts, "gerant-wifi-csv", "")
	routerID, profileID := seedWifiEnv(t, st, accID)
	seedWifiSite(t, st, accID, "csv-cafe", routerID, profileID, true, 5, 100)

	doJSON(t, ts, "POST", "/api/wifi/site/csv-cafe/claim", "", map[string]any{
		"phone": "2250707080009", "optIn": true,
	})
	doJSON(t, ts, "POST", "/api/wifi/site/csv-cafe/claim", "", map[string]any{
		"phone": "2250707080010", "optIn": false,
	})

	// Registre JSON : optInAt présent sur la ligne consentie.
	status, out := doJSON(t, ts, "GET", "/api/wifi/guests", token, nil)
	if status != http.StatusOK {
		t.Fatalf("registre : statut %d", status)
	}
	guests, _ := out["guests"].([]any)
	foundOpt, foundNo := false, false
	for _, raw := range guests {
		g, _ := raw.(map[string]any)
		phone, _ := g["phone"].(string)
		if phone == "2250707080009" {
			if at, _ := g["optInAt"].(string); at != "" {
				foundOpt = true
			}
		}
		if phone == "2250707080010" {
			if _, has := g["optInAt"]; !has {
				foundNo = true // omitempty : absent quand vide
			}
		}
	}
	if !foundOpt {
		t.Fatal("optInAt absent de la ligne consentie du registre JSON")
	}
	if !foundNo {
		t.Fatal("optInAt doit être omis (omitempty) pour la ligne non consentie")
	}

	// Export CSV : colonne opt_in_since + valeurs.
	req, _ := http.NewRequest("GET", ts.URL+"/api/wifi/guests?export=csv", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("export CSV : %v", err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	csv := string(buf[:n])
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export CSV : statut %d", resp.StatusCode)
	}
	if !strings.Contains(csv, "opt_in;opt_in_since") {
		t.Fatalf("en-tête CSV sans opt_in_since : %q", csv)
	}
	for _, line := range strings.Split(csv, "\r\n") {
		switch {
		case strings.Contains(line, "2250707080009"):
			if !strings.Contains(line, ";oui;2") && !strings.Contains(line, ";oui;20") {
				t.Fatalf("ligne consentie sans preuve opt_in_since : %q", line)
			}
		case strings.Contains(line, "2250707080010"):
			if !strings.Contains(line, ";non;-;") {
				t.Fatalf("ligne non consentie sans « - » : %q", line)
			}
		}
	}
}
