// pinlock_test.go — tests du verrouillage PIN revendeur par compte (S2) :
// unitaires sur le limiter (horloge injectable) + intégration HTTP bout en
// bout sur la surface publique (handleResellerLogin, sans le limiteur IP de
// main.go qui vit en dehors du mux).

package api

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestPinLockoutUnit — cycle de vie du verrou : seuil, expiration, remise à
// zéro par un succès, étanchéité entre revendeurs.
func TestPinLockoutUnit(t *testing.T) {
	base := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	cur := base
	p := newPinLimiter()
	p.now = func() time.Time { return cur }

	// 4 échecs : pas encore de verrou.
	for i := 0; i < pinMaxFailures-1; i++ {
		if locked, _ := p.fail("res-1"); locked {
			t.Fatalf("échec %d : verrou prématuré", i+1)
		}
		if ok, _ := p.check("res-1"); ok {
			t.Fatalf("échec %d : check signale un verrou", i+1)
		}
	}
	// 5e échec : verrou de 15 minutes déclenché.
	locked, remaining := p.fail("res-1")
	if !locked || remaining != pinLockoutDuration {
		t.Fatalf("5e échec : verrou 15 min attendu, obtenu %v/%v", locked, remaining)
	}
	// t+14:59 : toujours verrouillé.
	cur = base.Add(14*time.Minute + 59*time.Second)
	if ok, reste := p.check("res-1"); !ok || reste <= 0 {
		t.Fatalf("verrou inactif à t+14:59 (ok=%v remaining=%v)", ok, reste)
	}
	// t+15:01 : verrou expiré.
	cur = base.Add(pinLockoutDuration + time.Second)
	if ok, _ := p.check("res-1"); ok {
		t.Fatal("verrou toujours actif après expiration")
	}
	// Premier échec post-expiration : compteur neuf (pas de verrou immédiat).
	if locked, _ := p.fail("res-1"); locked {
		t.Fatal("verrou immédiat après expiration : compteur non réinitialisé")
	}
	// Succès : historique effacé — il faut 5 nouveaux échecs pour verrouiller.
	p.reset("res-1")
	for i := 0; i < pinMaxFailures-1; i++ {
		if locked, _ := p.fail("res-1"); locked {
			t.Fatalf("post-reset échec %d : verrou prématuré", i+1)
		}
	}
	// Étanchéité : verrouiller res-2 n'affecte pas res-1 (et inversement).
	for i := 0; i < pinMaxFailures; i++ {
		p.fail("res-2")
	}
	if ok, _ := p.check("res-1"); ok {
		t.Fatal("le verrou de res-2 a fuité sur res-1")
	}
	if ok, _ := p.check("res-2"); !ok {
		t.Fatal("verrou de res-2 attendu")
	}
	if ok, _ := p.check("res-inconnu"); ok {
		t.Fatal("un revendeur jamais ciblé ne peut pas être verrouillé")
	}
}

// TestResellerPinLockoutE2E — intégration complète : 5 mauvais PIN sur un
// revendeur → la 6e tentative refuse même le BON PIN (429 + Retry-After) ;
// l'autre revendeur du même compte passe normalement ; le compteur est
// remis à zéro par un succès.
func TestResellerPinLockoutE2E(t *testing.T) {
	ts := newTestServer(t)
	token, _, _ := registerAccount(t, ts, "gerant-pin", "")

	for i, nom := range []string{"vendeur1", "vendeur2"} {
		status, out := doJSON(t, ts, "POST", "/api/resellers", token, map[string]any{
			"name": "Vendeur " + nom, "username": nom, "pin": map[bool]string{true: "1234", false: "5678"}[i == 0],
		})
		if status != http.StatusOK {
			t.Fatalf("création %s : statut %d %v", nom, status, out)
		}
	}

	// 5 échecs de PIN sur vendeur1 (réponse inchangée : 400 générique).
	for i := 0; i < 5; i++ {
		status, out := doJSON(t, ts, "POST", "/api/reseller/login", "", map[string]string{
			"username": "vendeur1", "pin": "0000",
		})
		if status != http.StatusBadRequest {
			t.Fatalf("échec PIN %d : statut attendu 400, obtenu %d %v", i+1, status, out)
		}
	}

	// 6e tentative AVEC le bon PIN : verrou actif → 429 + Retry-After.
	req, err := http.NewRequest("POST", ts.URL+"/api/reseller/login",
		strings.NewReader(`{"username":"vendeur1","pin":"1234"}`))
	if err != nil {
		t.Fatalf("requête impossible : %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("requête verrou : %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("verrou attendu 429 même avec le bon PIN, obtenu %d", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("en-tête Retry-After attendu sous verrou")
	}

	// Le revendeur NON ciblé du même compte passe (bon PIN → 200 + token).
	status, out := doJSON(t, ts, "POST", "/api/reseller/login", "", map[string]string{
		"username": "vendeur2", "pin": "5678",
	})
	if status != http.StatusOK {
		t.Fatalf("revendeur non ciblé doit passer (200), obtenu %d %v", status, out)
	}
	if tk, _ := out["token"].(string); tk == "" {
		t.Fatal("login revendeur2 doit renvoyer un token")
	}

	// Message anti-énumération inchangé pour un revendeur inconnu.
	status, out = doJSON(t, ts, "POST", "/api/reseller/login", "", map[string]string{
		"username": "inconnu-total", "pin": "9999",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("revendeur inconnu : 400 attendu, obtenu %d %v", status, out)
	}
}
