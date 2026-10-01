package store

// Test N°199 — le moteur de simulation verse sa progression de volume dans
// les agrégats journaliers (le KPI « Volume de données » de l'aperçu de
// période lit ces lignes : la démo doit se comporter comme un vrai site) ;
// un parc AGENT n'y touche JAMAIS (seul le read_state accumule).

import (
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

func TestTickSimulatedVolumeAccumulates(t *testing.T) {
	now := time.Now().UTC()

	db := BuildEmptyState()
	db.LastTick = now.Add(-1 * time.Hour)
	db.Routers = []model.Router{{ID: "rt-sim", AccountID: "acc-sim", Name: "Démo", Mode: "simulated"}}
	db.HotspotUsers = []model.HotspotUser{{
		ID: "u-sim", AccountID: "acc-sim", Username: "demo", Status: "active", RouterID: "rt-sim",
	}}
	db.Sessions = []model.Session{{
		ID: "s-sim", AccountID: "acc-sim", UserID: "u-sim", Username: "demo", RouterID: "rt-sim",
	}}
	touched := NewTableSet()
	Tick(db, now, touched)

	if len(db.VolumeDays) != 1 {
		t.Fatalf("1 ligne de volume attendue après un tick simulé, obtenu %d", len(db.VolumeDays))
	}
	row := db.VolumeDays[0]
	if row.AccountID != "acc-sim" || row.RouterID != "rt-sim" {
		t.Fatalf("clé de ligne = (%s, %s), attendu (acc-sim, rt-sim)", row.AccountID, row.RouterID)
	}
	if row.Day != now.Format("2006-01-02") {
		t.Fatalf("Day = %q, attendu le jour courant (compte sans réglage → UTC)", row.Day)
	}
	if row.ID != model.VolumeDayID("acc-sim", "rt-sim", row.Day) {
		t.Fatalf("ID = %q, attendu la clé naturelle %q", row.ID, model.VolumeDayID("acc-sim", "rt-sim", row.Day))
	}
	if row.BytesIn <= 0 || row.BytesOut <= 0 {
		t.Fatalf("la progression simulée doit verser des octets : %d/%d", row.BytesIn, row.BytesOut)
	}
	// La ligne reflète EXACTEMENT les compteurs du user : même versement,
	// mêmes montants (les sessions créées/terminées par le même tick ne
	// touchent pas aux octets).
	u := db.HotspotUsers[0]
	if u.BytesIn != row.BytesIn || u.BytesOut != row.BytesOut {
		t.Fatalf("compteurs user %d/%d ≠ ligne %d/%d", u.BytesIn, u.BytesOut, row.BytesIn, row.BytesOut)
	}
	// Tout le versement de CE tick est attribué à l'heure courante.
	if got := model.VolumeHoursSumThrough(row.Hours, now.Hour()); got != row.BytesIn+row.BytesOut {
		t.Fatalf("histogramme 0..%dh = %d, attendu %d (tout le tick à l'heure courante)", now.Hour(), got, row.BytesIn+row.BytesOut)
	}
	if !touched.Has(TableVolumeDays) {
		t.Fatal("le tick simulé doit marquer volume_days (sauvegarde ciblée)")
	}

	// Second tick : la MÊME ligne s'enrichit (pas de doublon par tick).
	Tick(db, now.Add(90*time.Minute), NewTableSet())
	if len(db.VolumeDays) != 1 {
		t.Fatalf("toujours 1 ligne après le second tick, obtenu %d", len(db.VolumeDays))
	}
	if db.VolumeDays[0].BytesIn < row.BytesIn {
		t.Fatal("la ligne doit rester monotone (croissante)")
	}

	// Parc AGENT : les octets viennent du read_state (addUserBytes), JAMAIS
	// du tick — aucune ligne, aucun marquage.
	agentDB := BuildEmptyState()
	agentDB.LastTick = now.Add(-1 * time.Hour)
	agentDB.Routers = []model.Router{{ID: "rt-ag", AccountID: "acc-ag", Name: "Agent", Mode: "agent"}}
	agentDB.HotspotUsers = []model.HotspotUser{{
		ID: "u-ag", AccountID: "acc-ag", Username: "client", Status: "active", RouterID: "rt-ag",
	}}
	agentDB.Sessions = []model.Session{{
		ID: "s-ag", AccountID: "acc-ag", UserID: "u-ag", Username: "client", RouterID: "rt-ag",
		BytesIn: 5000, BytesOut: 5000,
	}}
	touched2 := NewTableSet()
	Tick(agentDB, now, touched2)
	if len(agentDB.VolumeDays) != 0 {
		t.Fatalf("parc agent : AUCUNE ligne de volume attendue, obtenu %d", len(agentDB.VolumeDays))
	}
	if touched2.Has(TableVolumeDays) {
		t.Fatal("parc agent : volume_days ne doit PAS être marqué par le tick")
	}
}
