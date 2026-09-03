// signup_abuse_test.go — tests du quota d'inscription par IP (S3) :
// unitaires sur le limiter (horloge injectable, cycle burst + quotidien) +
// intégration HTTP bout en bout sur la surface publique (handleRegister,
// sans le limiteur IP de main.go qui vit en dehors du mux).

package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestSignupQuotaUnitBurst — anti-burst : 5 tentatives en rafale passent,
// la 6e est refusée avec un Retry-After positif, une IP voisine n'est pas
// touchée, et la place se libère à l'expiration de la fenêtre glissante.
func TestSignupQuotaUnitBurst(t *testing.T) {
	base := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	cur := base
	s := newSignupLimiter()
	s.now = func() time.Time { return cur }

	// 5 tentatives immédiates : admises.
	for i := 0; i < signupBurstMax; i++ {
		if ok, retry := s.allow("1.2.3.4"); !ok {
			t.Fatalf("tentative %d refusée (retry %v)", i+1, retry)
		}
	}
	// 6e tentative dans la fenêtre burst : refusée.
	ok, retry := s.allow("1.2.3.4")
	if ok || retry <= 0 {
		t.Fatalf("la 6e tentative doit être refusée avec Retry-After positif (ok=%v retry=%v)", ok, retry)
	}
	// Étanchéité : une autre IP n'est pas affectée.
	if ok, _ := s.allow("5.6.7.8"); !ok {
		t.Fatal("le quota d'une IP voisine ne doit pas être touché")
	}
	// Après la fenêtre burst (11 min), la place est libérée — le compteur
	// quotidien n'est pas en cause (6 tentatives < signupDailyMax).
	cur = base.Add(signupBurstWindow + time.Minute)
	if ok, retry := s.allow("1.2.3.4"); !ok {
		t.Fatalf("après la fenêtre burst, la tentative doit passer (retry %v)", retry)
	}
}

// TestSignupQuotaUnitDaily — anti-farm : 20 tentatives espacées (sous le
// seuil burst) passent, la 21e est refusée jusqu'à vidage de la fenêtre
// glissante de 24 h.
func TestSignupQuotaUnitDaily(t *testing.T) {
	base := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	cur := base
	s := newSignupLimiter()
	s.now = func() time.Time { return cur }

	// 20 tentatives espacées de 3 min : au plus 4 dans toute fenêtre de
	// 10 min (sous signupBurstMax), donc seul le quota quotidien compte.
	for i := 0; i < signupDailyMax; i++ {
		if ok, retry := s.allow("1.2.3.4"); !ok {
			t.Fatalf("tentative %d refusée (retry %v)", i+1, retry)
		}
		cur = cur.Add(3 * time.Minute)
	}
	// 21e tentative : quota quotidien atteint → refus.
	ok, retry := s.allow("1.2.3.4")
	if ok || retry <= 0 {
		t.Fatalf("la 21e tentative doit être refusée avec Retry-After positif (ok=%v retry=%v)", ok, retry)
	}
	// La fenêtre glissante se vide : après 24 h, le quota repart de zéro.
	cur = cur.Add(signupDailyWindow + time.Minute)
	if ok, retry := s.allow("1.2.3.4"); !ok {
		t.Fatalf("après 24 h, le quota doit repartir de zéro (retry %v)", retry)
	}
}

// TestSignupQuotaE2E — surface HTTP : 5 inscriptions valides (201) puis la
// 6e tentative est coupée (429 + Retry-After), même avec un corps valide —
// toute tentative consomme le quota.
func TestSignupQuotaE2E(t *testing.T) {
	ts := newTestServer(t)

	for i := 0; i < signupBurstMax; i++ {
		status, out := doJSON(t, ts, "POST", "/api/auth/register", "", map[string]string{
			"name":     "Gérant quota " + fmt.Sprint(i),
			"username": fmt.Sprintf("quota-e2e-%d", i),
			"password": "mot-de-passe-8+",
			"email":    fmt.Sprintf("quota%d@example.ci", i),
			"phone":    "0707070707",
			"country":  "CI",
			"city":     "Abidjan",
		})
		if status != http.StatusCreated {
			t.Fatalf("inscription %d : statut attendu 201, obtenu %d %v", i+1, status, out)
		}
	}

	// 6e tentative (corps valide) : quota burst → 429 + Retry-After.
	req, err := http.NewRequest("POST", ts.URL+"/api/auth/register",
		strings.NewReader(`{"name":"Fermier","username":"quota-e2e-farm","password":"mot-de-passe-8+","email":"farm@example.ci","phone":"0707070707","country":"CI","city":"Abidjan"}`))
	if err != nil {
		t.Fatalf("requête impossible : %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("requête quota : %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("quota attendu 429, obtenu %d", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("en-tête Retry-After attendu sous quota")
	}
}
