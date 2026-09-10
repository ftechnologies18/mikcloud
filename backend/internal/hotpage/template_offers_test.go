package hotpage

// Tests N°75 — le template du portail ne doit JAMAIS porter de lien marchand
// hardcodé : le bug historique facturait les invités des comptes sans waveLink
// au marchand de l'opérateur (M_5Mg9EG61ZHDF), avec en prime des prix affichés
// faux (le libellé du template restait « 100 F » pendant que le href menait au
// montant réel du profil). Les cartes statiques sont des placeholders
// data-mik-offer pilotés par renderOffers (offres LIVE du compte).

import (
	"strings"
	"testing"
)

// TestLoginTemplateHasNoHardcodedWaveMerchant — AUCUNE URL marchand Wave ne
// doit apparaître en dur dans le HTML statique (ni login.html ni aucune autre
// page) : seules les href posées dynamiquement par renderOffers (depuis le
// waveLink DU COMPTE) peuvent pointer vers pay.wave.com.
func TestLoginTemplateHasNoHardcodedWaveMerchant(t *testing.T) {
	for _, page := range []string{"login.html", "status.html", "alogin.html", "logout.html", "error.html", "redirect.html", "radvert.html", "rlogin.html"} {
		body := RawFile(page)
		if body == "" {
			continue // page absente du template : rien à vérifier
		}
		// Motif marchand : pay.wave.com/m/{id} — la mention du domaine dans le
		// JS de délégation (détection href.indexOf('pay.wave.com')) est légitime,
		// un LIEN marchand ne l'est jamais.
		if i := strings.Index(body, "pay.wave.com/m/"); i >= 0 {
			ctx := body[i:]
			if len(ctx) > 80 {
				ctx = ctx[:80]
			}
			t.Fatalf("%s contient une URL marchand Wave hardcodée (%q…) : les liens marchands ne peuvent venir que de la config du compte (renderOffers)", page, ctx)
		}
		if strings.Contains(body, `href="https://pay.wave.com`) {
			t.Fatalf("%s contient un href Wave hardcodé : les liens marchands ne peuvent venir que de la config du compte (renderOffers)", page)
		}
	}
}

// TestLoginTemplateOfferPlaceholders — les 7 cartes statiques sont bien des
// placeholders : marqueurs data-mik-offer présents, sans href marchand, et le
// moteur de rendu (renderOffers + mikOffersVisible + masquage des colonnes
// orphelines) est en place.
func TestLoginTemplateOfferPlaceholders(t *testing.T) {
	body := RawFile("login.html")
	if body == "" {
		t.Fatal("login.html absent du template")
	}
	if got := strings.Count(body, `data-mik-offer="1"`); got != 7 {
		t.Fatalf("7 cartes placeholder attendues (6 régulières + 1 vedette), obtenu %d", got)
	}
	for _, marker := range []string{
		`data-mik-offers-grid="1"`, `data-mik-offers-title="1"`, `data-mik-offers-sub="1"`,
		"mikOffersVisible(payable.length > 0)",
		"mikValidityLabel", "mikColumnOf",
		"renderOffers(cfg);", // appel inconditionnel : c'est lui qui retire la vitrine sans offre payable
	} {
		if !strings.Contains(body, marker) {
			t.Fatalf("fragment %q absent de login.html — le moteur d'offres N°75 est incomplet", marker)
		}
	}
	// L'appel inconditionnel ne doit plus être gardé par un « offers.length > 0 »
	// (l'ancienne garde laissait les cartes hardcodées visibles sans offres).
	if strings.Contains(body, "cfg.offers && cfg.offers.length > 0") {
		t.Fatal("ancienne garde « if (cfg.offers…) renderOffers » encore présente : sans offre, les cartes statiques resteraient visibles")
	}
	// La délégation Android doit couvrir les href dynamiques (pas de liaison
	// au seul chargement).
	if !strings.Contains(body, "document.addEventListener('click'") {
		t.Fatal("délégation du deep-link Wave absente : les href posés dynamiquement n'ouvriraient pas l'app Wave sur Android")
	}
}
