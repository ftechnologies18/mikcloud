// Tests N°29 — walled-garden d'inscription publique (runbook N°27-D
// automatisé) : normalisation des domaines, calcul des domaines du
// déploiement, et mise en file idempotente aux check-in des routeurs agents
// (le point « routeurs déjà en ligne » : aucune commande en double, re-file
// automatique si la configuration change).
package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/agent"
	"mikcloud/hotspot-api/internal/model"
)

func TestNormalizeWGHost(t *testing.T) {
	cas := []struct{ in, want string }{
		{"https://Mikcloud.Ftci.fr/", "mikcloud.ftci.fr"}, // origine complète
		{"https://x.example:443/p?q=1", "x.example"},      // port par défaut retiré
		{"http://a.example:8080", "a.example:8080"},       // port non standard conservé
		{"https://u:p@host.example/x", "host.example"},    // userinfo retiré
		{"  Host.Example  ", "host.example"},              // trim + casse
		{"", ""},                                          // vide → refus
		{"https://", ""},                                  // schéma seul → refus
		{"bad host.example", ""},                          // espace → refus (sanitize)
		{"evil\"; /system reboot", ""},                    // injection → refus (sanitize)
	}
	for _, c := range cas {
		if got := agent.SanitizeWGDomain(normalizeWGHost(c.in)); got != c.want {
			t.Fatalf("normalizeWGHost(%q) → %q, attendu %q", c.in, got, c.want)
		}
	}
}

// TestWalledGardenSigVersioned — N°48 : la signature intègre un sel de
// VERSION des règles, pas seulement les domaines. Garde-fou du bug prod : la
// v1 calculait la sig sur les seuls domaines — corriger le SCRIPT (ajout des
// règles ip HTTPS) sans changer la liste laissait la sig identique →
// ensureWalledGardenLocked ne re-filait rien → les routeurs restaient en
// règles v1 (HTTPS pré-auth bloqué, claim « indisponible » côté client). Si
// ce test échoue, le sel de version a disparu de walledGardenSig.
func TestWalledGardenSigVersioned(t *testing.T) {
	domains := []string{"mikcloud.onrender.com", "mikcloud.ftci.fr"}
	s := walledGardenSig(domains)
	legacy := agent.HashToken(strings.Join(domains, "|"))[:16] // formule v1 (sans sel)
	if s == legacy {
		t.Fatal("walledGardenSig sans sel de version : un correctif de règles ne serait jamais re-poussé aux routeurs (régression N°48)")
	}
	// Stabilité : même entrée → même sig (contrat « déjà appliqué » vs « à
	// (re)appliquer »). NB : le tri des domaines est le fait de
	// walledGardenDomains (amont), pas de walledGardenSig.
	if s != walledGardenSig([]string{"mikcloud.onrender.com", "mikcloud.ftci.fr"}) {
		t.Fatal("walledGardenSig doit être déterministe")
	}
}

func TestWalledGardenDomains(t *testing.T) {
	t.Setenv("MIKCLOUD_BASE_URL", "")
	t.Setenv("APP_PUBLIC_URL", "https://app.example.com/")
	t.Setenv("ALLOWED_ORIGIN", "https://app.example.com, https://other.example.org")
	req := httptest.NewRequest("GET", "http://api.example:4000/agent/cmd?token=x", nil)
	got := walledGardenDomains(req)
	want := []string{"api.example:4000", "app.example.com", "other.example.org"}
	if len(got) != len(want) {
		t.Fatalf("walledGardenDomains = %v, attendu %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("walledGardenDomains[%d] = %q, attendu %q (tri stable exigé)", i, got[i], want[i])
		}
	}

	// Dé-dup : l'origine API déjà présente via ALLOWED_ORIGIN ne compte qu'une fois.
	t.Setenv("ALLOWED_ORIGIN", "https://app.example.com, https://api.example:4000")
	req2 := httptest.NewRequest("GET", "http://api.example:4000/agent/cmd?token=x", nil)
	if got := walledGardenDomains(req2); len(got) != 2 || got[0] != "api.example:4000" || got[1] != "app.example.com" {
		t.Fatalf("walledGardenDomains (dé-dup) = %v", got)
	}
}

func TestEnsureWalledGardenLocked(t *testing.T) {
	db := &model.DB{}
	router := &model.Router{ID: "r-wg", AccountID: "acc-wg"}
	domains := []string{"a.example", "b.example"}

	// 1er check-in : rien d'appliqué → UNE commande walled_garden en file.
	ensureWalledGardenLocked(db, router, domains)
	if len(db.Commands) != 1 {
		t.Fatalf("après 1er ensure : %d commande(s), attendu 1", len(db.Commands))
	}
	cmd := db.Commands[0]
	if cmd.Kind != model.CmdWalledGarden || cmd.Status != "queued" || cmd.RouterID != "r-wg" {
		t.Fatalf("commande inattendue : %+v", cmd)
	}
	if sig, _ := cmd.Payload["sig"].(string); sig != walledGardenSig(domains) {
		t.Fatalf("sig du payload = %q, attendu %q", sig, walledGardenSig(domains))
	}

	// Check-in suivant, commande toujours en vol → AUCUN doublon.
	ensureWalledGardenLocked(db, router, domains)
	if len(db.Commands) != 1 {
		t.Fatalf("commande en vol dupliquée : %d commande(s)", len(db.Commands))
	}

	// La commande est rapportée « ok » (handleAgentResult pose la signature).
	router.WalledGardenSig = walledGardenSig(domains)
	db.Commands[0].Status = "done"

	// Check-in suivant, config inchangée → toujours rien de neuf.
	ensureWalledGardenLocked(db, router, domains)
	if len(db.Commands) != 1 {
		t.Fatalf("re-file injustifiée : %d commande(s)", len(db.Commands))
	}

	// Changement de configuration (ex. nouveau domaine) → re-file automatique.
	domains2 := []string{"a.example", "b.example", "c.example"}
	ensureWalledGardenLocked(db, router, domains2)
	if len(db.Commands) != 2 {
		t.Fatalf("changement de config non détecté : %d commande(s), attendu 2", len(db.Commands))
	}

	// Échec simulé (sig jamais posé) → retenté au check-in suivant.
	db.Commands[1].Status = "error"
	ensureWalledGardenLocked(db, router, domains2)
	if len(db.Commands) != 3 {
		t.Fatalf("échec non retenté : %d commande(s), attendu 3", len(db.Commands))
	}

	// Aucun domaine annoncé (déploiement sans origine exploitable) → aucune file.
	before := len(db.Commands)
	ensureWalledGardenLocked(db, router, nil)
	if len(db.Commands) != before {
		t.Fatal("aucun domaine → aucune commande attendue")
	}
}

// TestRequeueStaleWalledGarden — audit N°31 : un walled_garden « sent » sans
// rapport depuis plus de staleSentLimit (rapport perdu : blip réseau, reboot
// en cours de check-in…) doit repartir en file — sinon il restait « sent » à
// jamais, ensureWalledGardenLocked le croyant « en vol », et le walled-garden
// n'était JAMAIS appliqué sur le routeur. La re-exécution est sûre : le bloc
// est idempotent (remove+add des seules règles marquées mikcloud-wg).
func TestRequeueStaleWalledGarden(t *testing.T) {
	old := time.Now().Add(-staleSentLimit - time.Minute).Format(time.RFC3339)
	fresh := time.Now().Format(time.RFC3339)
	db := &model.DB{Commands: []model.Command{
		{ID: "c-old", RouterID: "r-z", Kind: model.CmdWalledGarden, Status: "sent", SentAt: old},
		{ID: "c-new", RouterID: "r-z", Kind: model.CmdWalledGarden, Status: "sent", SentAt: fresh},
	}}

	requeueStaleReadsLocked(db, "r-z")

	if db.Commands[0].Status != "queued" || db.Commands[0].SentAt != "" {
		t.Fatalf("walled_garden périmée non reprise : %+v", db.Commands[0])
	}
	if db.Commands[1].Status != "sent" {
		t.Fatalf("walled_garden fraîche (en vol) reprise à tort : %+v", db.Commands[1])
	}

	// Boucle complète : après reprise, ensureWalledGardenLocked ne crée
	// PAS de doublon (la commande reprise est « en vol » à nouveau) et le
	// FIFO la sert telle quelle au check-in courant.
	router := &model.Router{ID: "r-z", AccountID: "acc-z"}
	ensureWalledGardenLocked(db, router, []string{"a.example"})
	if len(db.Commands) != 2 {
		t.Fatalf("doublon après reprise : %d commande(s), attendu 2", len(db.Commands))
	}
}

// TestWGHostUsable — complément N°31-b : les hôtes non joignables depuis un
// client du WiFi (boucle locale, RFC1918, link-local, mDNS) sont exclus de
// la configuration walled-garden ; les hôtes publics (port numérique
// compris) restent éligibles. Constat prod : « localhost:3000 » issu des
// origines de dev de ALLOWED_ORIGIN polluait la configuration.
func TestWGHostUsable(t *testing.T) {
	cas := []struct {
		in   string
		want bool
	}{
		{"localhost:3000", false},              // dev local (constat prod)
		{"localhost", false},                   // boucle
		{"api.localhost", false},               // sous-domaine boucle
		{"127.0.0.1", false},                   // loopback IPv4
		{"10.0.0.5", false},                    // RFC1918
		{"192.168.1.10", false},                // RFC1918
		{"172.20.0.1", false},                  // RFC1918 (172.16–31)
		{"172.32.0.1", true},                   // hors RFC1918 (172.32+)
		{"169.254.9.9", false},                 // link-local
		{"0.0.0.0", false},                     // this-network
		{"atelier.local", false},               // mDNS
		{"mikcloud.ftci.fr", true},             // page publique
		{"mikcloud.onrender.com", true},        // API
		{"api.example:4000", true},             // port non standard conservé
		{"mikcloud-ftech-ci.vercel.app", true}, // preview Vercel
	}
	for _, c := range cas {
		if got := wgHostUsable(c.in); got != c.want {
			t.Fatalf("wgHostUsable(%q) = %v, attendu %v", c.in, got, c.want)
		}
	}
}
