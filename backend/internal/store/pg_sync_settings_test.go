package store

// Tests N°210 — différentiel settings (correctif de la fuite « Service-
// Initiated » Render) : l'empreinte couvre la projection EXACTE de la ligne
// (un blob de branding qui change la change ; deux comptes ne la partagent
// jamais), la taille approximative suit les blobs, et rebuildHashes pose les
// empreintes au boot (le premier flush ne réécrit pas toutes les lignes pour
// rien — parité N°133).

import (
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

// TestSettingsRowHashDeterministeEtScope — l'empreinte est stable pour une
// même ligne et scoped par compte (l'id fait partie de la projection : une
// même config sur deux comptes reste deux lignes distinctes).
func TestSettingsRowHashDeterministeEtScope(t *testing.T) {
	s := model.Settings{}
	s.Tenant.Name = "ProMax WIFI"
	s.Tenant.LogoURL = "data:image/png;base64,AAAA"
	h1 := settingsRowHash("acc-1", s)
	if h1 != settingsRowHash("acc-1", s) {
		t.Fatal("l'empreinte d'une même ligne doit être déterministe")
	}
	if settingsRowHash("acc-2", s) == h1 {
		t.Fatal("deux comptes ne doivent pas partager la même empreinte (l'id est dans la projection)")
	}
}

// TestSettingsRowHashChangeAvecBranding — chaque champ de branding qui
// voyage dans l'upsert doit faire changer l'empreinte : sinon la ligne ne
// serait jamais réécrite (dérive silencieuse de la base).
func TestSettingsRowHashChangeAvecBranding(t *testing.T) {
	base := model.Settings{}
	base.Tenant.Name = "Cyber"
	base.Tenant.LogoURL = "data:image/png;base64,AAAA"
	base.Tenant.PortalSlides = `["https://media.ftci.fr/media/a/1.webp"]`
	h := settingsRowHash("acc-x", base)

	logo := base
	logo.Tenant.LogoURL = "data:image/png;base64,BBBB"
	if settingsRowHash("acc-x", logo) == h {
		t.Fatal("un changement de logo doit changer l'empreinte (la ligne doit voyager)")
	}
	slides := base
	slides.Tenant.PortalSlides = `["https://media.ftci.fr/media/a/2.webp"]`
	if settingsRowHash("acc-x", slides) == h {
		t.Fatal("un changement de slides doit changer l'empreinte")
	}
	wa := base
	wa.Tenant.PortalWhatsapp = `{"number":"2250700000000"}`
	if settingsRowHash("acc-x", wa) == h {
		t.Fatal("un changement de WhatsApp support doit changer l'empreinte")
	}
	nom := base
	nom.Tenant.Name = "Cyber 2"
	if settingsRowHash("acc-x", nom) == h {
		t.Fatal("un changement de nom doit changer l'empreinte")
	}
}

// TestSettingsRowBytesSuitLesBlobs — la volumétrie approximative suit les
// blobs (garde-fou egress exposé par sync-status) avec un plancher.
func TestSettingsRowBytesSuitLesBlobs(t *testing.T) {
	petit := model.Settings{}
	petit.Tenant.LogoURL = "data:x"
	gros := petit
	gros.Tenant.LogoURL = "data:image/png;base64," + strings.Repeat("A", 4096)
	if settingsRowBytes(gros) <= settingsRowBytes(petit) {
		t.Fatal("la taille approximative doit suivre les blobs (garde-fou egress)")
	}
	if settingsRowBytes(petit) < 256 {
		t.Fatal("plancher 256 octets attendu pour une ligne sans blobs")
	}
}

// TestRebuildHashesPoseLesEmpreintesSettings — parité boot : les empreintes
// settings sont posées au chargement (sinon le premier flush réécrirait
// toutes les lignes, blobs compris — exactement le bug corrigé).
func TestRebuildHashesPoseLesEmpreintesSettings(t *testing.T) {
	a := model.Settings{}
	a.Tenant.Name = "A"
	b := model.Settings{}
	b.Tenant.Name = "B"
	db := &model.DB{}
	db.SettingsByAccount = map[string]model.Settings{"acc-a": a, "acc-b": b}

	p := &PG{}
	p.rebuildHashes(db)
	if len(p.settingsHashes) != 2 {
		t.Fatalf("2 empreintes settings attendues au boot, %d trouvée(s)", len(p.settingsHashes))
	}
	if p.settingsHashes["acc-a"] != settingsRowHash("acc-a", db.SettingsByAccount["acc-a"]) {
		t.Fatal("l'empreinte boot doit être celle de la projection courante")
	}
}
