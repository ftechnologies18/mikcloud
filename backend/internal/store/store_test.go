// Tests du package store — MODE JSON UNIQUEMENT (aucune connexion PostgreSQL).
//
// Couverture :
//   - BuildEmptyState : invariants de mise en service (zéro donnée démo,
//     slices non nil) ;
//   - New en mode JSON : état vide + bootstrapAdmin (admin plateforme sans
//     identifiant connu), gardee hors PostgreSQL via DATABASE_URL vide ;
//   - Save → relecture : aller-retour des entités, chiffrement au repos des
//     mots de passe routeur dans db.json (secretbox) ;
//   - mutations différentielles (ajout/modification/suppression entre deux
//     Save) puis Reload : l'état rechargé reflète la dernière version ;
//   - migrateMultiTenant (backfill AccountID + création du compte principal,
//     idempotence), normalizeSettingsDefaults ;
//   - defaults.go : profil « Staff » + 3 gabarits de tickets ;
//   - model.RepairTimeLimitParity (réparation parité limit-uptime, pure) ;
//   - applyAdminOverride (variable ADMIN_PASSWORD, remplacement du compte
//     démo admin/admin123).
package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/secretbox"
)

// TestMain — active secretbox (comme au démarrage du service) pour que les
// mots de passe routeur soient réellement scellés dans db.json.
func TestMain(m *testing.M) {
	os.Unsetenv("CREDENTIALS_KEY")
	if err := secretbox.Init("jwt-secret-de-test-store"); err != nil {
		panic("secretbox.Init impossible : " + err.Error())
	}
	os.Exit(m.Run())
}

// newJSONStore — Store en mode JSON dans un répertoire temporaire, garanti
// SANS PostgreSQL (DATABASE_URL vide) ni override ADMIN_PASSWORD.
func newJSONStore(t *testing.T) *Store {
	t.Helper()
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ADMIN_PASSWORD", "")
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New impossible : %v", err)
	}
	return s
}

func TestBuildEmptyStateInvariants(t *testing.T) {
	db := BuildEmptyState()
	// Zéro entité métier : aucune donnée de démo ne doit ressusciter.
	if len(db.Accounts) != 0 || len(db.Routers) != 0 || len(db.Profiles) != 0 ||
		len(db.HotspotUsers) != 0 || len(db.Batches) != 0 || len(db.Resellers) != 0 ||
		len(db.Transactions) != 0 || len(db.Sessions) != 0 || len(db.Sales) != 0 ||
		len(db.Templates) != 0 || len(db.Users) != 0 {
		t.Fatal("l'état de mise en service doit être totalement vide (zéro démo)")
	}
	// Slices/maps non nil : l'API doit servir [] et la synchro n'insérer rien.
	if db.Accounts == nil || db.Routers == nil || db.Profiles == nil || db.HotspotUsers == nil ||
		db.SettingsByAccount == nil || db.Sales == nil || db.Templates == nil {
		t.Fatal("les collections doivent être non nil (slices vides, pas null)")
	}
	if db.LastTick.IsZero() {
		t.Fatal("LastTick doit être initialisé")
	}
}

func TestNewJSONModeBootstrapAdmin(t *testing.T) {
	s := newJSONStore(t)
	db := s.Data()
	if len(db.Users) != 1 {
		t.Fatalf("le démarrage local doit créer exactement 1 admin plateforme, obtenu %d", len(db.Users))
	}
	u := db.Users[0]
	if u.Role != "admin" || u.Username != "admin" {
		t.Fatalf("admin inattendu : %+v", u)
	}
	if u.AccountID != "" {
		t.Fatal("l'admin plateforme est un opérateur SaaS : AUCUN compte client")
	}
	if !strings.HasPrefix(u.PasswordHash, "$2") {
		t.Fatal("le mot de passe doit être haché bcrypt (jamais en clair, jamais un mot de passe connu)")
	}
}

func TestSaveReloadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ADMIN_PASSWORD", "")
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New impossible : %v", err)
	}

	acc := model.Account{ID: "acc-t1", Name: "Hotspot Cocody", Status: "active", CreatedAt: model.NowISO()}
	rt := model.Router{ID: "rt-1", AccountID: acc.ID, Name: "MikroTik 1", Host: "10.0.0.1",
		Password: "mot-de-passe-routeur-super-secret", Mode: "agent", Status: "online"}
	u := model.HotspotUser{ID: "hu-1", AccountID: acc.ID, Kind: "voucher", Username: "SC-0001",
		Password: "4321", ProfileName: "Staff", Status: "active", Price: 500, CreatedAt: model.NowISO()}

	s.Lock()
	db := s.Data()
	db.Accounts = append(db.Accounts, acc)
	db.Routers = append(db.Routers, rt)
	db.HotspotUsers = append(db.HotspotUsers, u)
	s.Save()
	s.Unlock()

	// Sur disque, le mot de passe routeur doit être CHIFFRÉ (jamais en clair).
	raw, err := os.ReadFile(filepath.Join(dir, "db.json"))
	if err != nil {
		t.Fatalf("db.json illisible : %v", err)
	}
	if strings.Contains(string(raw), rt.Password) {
		t.Fatal("le mot de passe routeur ne doit JAMAIS apparaître en clair dans db.json")
	}
	if !strings.Contains(string(raw), secretbox.Prefix) {
		t.Fatal("le mot de passe routeur doit être scellé (préfixe enc:v1:) dans db.json")
	}

	// Re-ouverture du même répertoire : les entités reviennent à l'identique,
	// mot de passe routeur déchiffré en mémoire.
	s2, err := New(dir)
	if err != nil {
		t.Fatalf("réouverture impossible : %v", err)
	}
	db2 := s2.Data()
	if len(db2.Accounts) != 1 || db2.Accounts[0].Name != acc.Name {
		t.Fatalf("compte non conservé : %+v", db2.Accounts)
	}
	if len(db2.Routers) != 1 || db2.Routers[0].Password != rt.Password {
		t.Fatalf("mot de passe routeur non restauré après déchiffrement : %+v", db2.Routers)
	}
	if len(db2.HotspotUsers) != 1 || db2.HotspotUsers[0].Username != u.Username {
		t.Fatalf("utilisateur hotspot non conservé : %+v", db2.HotspotUsers)
	}
}

func TestDifferentialMutationsThenReload(t *testing.T) {
	s := newJSONStore(t)

	// 1re sauvegarde : un compte + un utilisateur.
	s.Lock()
	db := s.Data()
	db.Accounts = append(db.Accounts, model.Account{ID: "acc-d1", Name: "Compte A", Status: "active"})
	db.HotspotUsers = append(db.HotspotUsers, model.HotspotUser{ID: "hu-a", AccountID: "acc-d1",
		Kind: "voucher", Username: "V-A1", Status: "active"})
	s.Save()
	s.Unlock()

	// 2e passe : modification (statut), ajout (2e utilisateur), suppression (3e).
	s.Lock()
	db = s.Data()
	db.HotspotUsers[0].Status = "used"
	db.HotspotUsers = append(db.HotspotUsers,
		model.HotspotUser{ID: "hu-b", AccountID: "acc-d1", Kind: "voucher", Username: "V-B1", Status: "active"},
		model.HotspotUser{ID: "hu-c", AccountID: "acc-d1", Kind: "voucher", Username: "V-C1", Status: "active"})
	s.Save()
	s.Unlock()

	s.Lock()
	db = s.Data()
	db.HotspotUsers = db.HotspotUsers[:len(db.HotspotUsers)-1] // V-C1 supprimé
	s.Save()
	s.Unlock()

	stats, err := s.Reload()
	if err != nil {
		t.Fatalf("Reload impossible : %v", err)
	}
	if !stats.OK || stats.HotspotUsers != 2 || stats.Accounts != 1 {
		t.Fatalf("statistiques de rechargement inattendues : %+v", stats)
	}
	db = s.Data()
	if len(db.HotspotUsers) != 2 {
		t.Fatalf("après Reload : %d utilisateurs, attendu 2", len(db.HotspotUsers))
	}
	seen := map[string]string{}
	for _, u := range db.HotspotUsers {
		seen[u.Username] = u.Status
	}
	if seen["V-A1"] != "used" || seen["V-B1"] != "active" {
		t.Fatalf("état différentiel non reflété après Reload : %v", seen)
	}
	if _, ok := seen["V-C1"]; ok {
		t.Fatal("l'utilisateur supprimé ne doit pas revenir après Reload")
	}
}

func TestMigrateMultiTenantBackfill(t *testing.T) {
	db := BuildEmptyState()
	// Ancien état mono-tenant : entités sans AccountID + un utilisateur client.
	db.Users = append(db.Users, model.AdminUser{ID: "usr-1", Username: "gerant", Role: "owner"})
	db.Routers = append(db.Routers, model.Router{ID: "rt-x", Name: "Routeur legacy"})
	db.HotspotUsers = append(db.HotspotUsers, model.HotspotUser{ID: "hu-x", Username: "V-X1"})

	if !migrateMultiTenant(db) {
		t.Fatal("la migration doit signaler une modification")
	}
	// Un compte principal doit naitre (il y a des utilisateurs clients).
	found := false
	for _, a := range db.Accounts {
		if a.ID == model.AccountMainID {
			found = true
		}
	}
	if !found {
		t.Fatal("le compte principal doit être créé lors de la migration")
	}
	for _, r := range db.Routers {
		if r.AccountID != model.AccountMainID {
			t.Fatalf("backfill AccountID manquant sur le routeur : %+v", r)
		}
	}
	for _, u := range db.HotspotUsers {
		if u.AccountID != model.AccountMainID {
			t.Fatalf("backfill AccountID manquant sur l'utilisateur : %+v", u)
		}
	}
	// Les réglages du compte principal doivent exister (défauts FCFA).
	st, ok := db.SettingsByAccount[model.AccountMainID]
	if !ok {
		t.Fatal("les réglages du compte principal doivent être initialisés")
	}
	if st.Tenant.Name == "" || st.Tenant.Currency == "" {
		t.Fatalf("réglages par défaut incomplets : %+v", st.Tenant)
	}
	// Idempotence : un second passage ne change RIEN.
	if migrateMultiTenant(db) {
		t.Fatal("la migration doit être idempotente")
	}
	// L'admin plateforme n'a PAS de compte client : jamais rattaché.
	db.Users = append(db.Users, model.AdminUser{ID: "usr-2", Username: "admin", Role: "admin"})
	migrateMultiTenant(db)
	for _, u := range db.Users {
		if u.Role == "admin" && u.AccountID != "" {
			t.Fatalf("l'admin plateforme doit rester sans compte : %+v", u)
		}
	}
}

func TestNormalizeSettingsDefaults(t *testing.T) {
	var s model.Settings // mode vide (données antérieures) → défauts P0
	if !normalizeSettingsDefaults(&s) {
		t.Fatal("un mode vide doit être normalisé")
	}
	if s.Tenant.ExpiryPolicyMode != "keep" || s.Tenant.ExpiryPolicyAfterDays != 30 {
		t.Fatalf("défauts attendus keep/30, obtenu %s/%d", s.Tenant.ExpiryPolicyMode, s.Tenant.ExpiryPolicyAfterDays)
	}
	// Un mode explicite (même avec 0 jour) est respecté tel quel.
	explicite := model.Settings{Tenant: model.Tenant{ExpiryPolicyMode: "remove", ExpiryPolicyAfterDays: 0}}
	if normalizeSettingsDefaults(&explicite) {
		t.Fatal("un mode explicite ne doit pas être modifié")
	}
	if explicite.Tenant.ExpiryPolicyAfterDays != 0 {
		t.Fatal("les jours d'un mode explicite doivent rester inchangés")
	}
}

func TestSeedProfilesAndTemplates(t *testing.T) {
	const acc = "acc-seed"
	profiles := SeedProfilesFor(acc)
	if len(profiles) != 1 {
		t.Fatalf("1 profil par défaut attendu, obtenu %d", len(profiles))
	}
	p := profiles[0]
	if p.Name != "Staff" || p.AccountID != acc {
		t.Fatalf("profil Staff inattendu : %+v", p)
	}
	if p.SessionTimeoutMin != 43200 || p.SharedUsers != 2 || p.Price != 0 {
		t.Fatalf("caractéristiques Staff incorrectes : %+v", p)
	}
	if p.ID == "" || p.CreatedAt == "" {
		t.Fatal("le profil doit porter un id et une date de création")
	}

	tpls := SeedTemplatesFor(acc)
	if len(tpls) != 3 {
		t.Fatalf("3 gabarits de tickets attendus, obtenu %d", len(tpls))
	}
	ids := map[string]bool{}
	for i, tp := range tpls {
		if tp.AccountID != acc {
			t.Fatalf("gabarit %d rattaché au mauvais compte : %s", i, tp.AccountID)
		}
		if tp.ID == "" || ids[tp.ID] {
			t.Fatalf("id de gabarit vide ou dupliqué : %q", tp.ID)
		}
		ids[tp.ID] = true
		if !strings.Contains(tp.BodyHTML, "{{username}}") || !strings.Contains(tp.BodyHTML, "{{password}}") {
			t.Fatalf("le gabarit %d doit substituer identifiant et mot de passe", i)
		}
	}
	if !tpls[0].IsDefault || tpls[1].IsDefault || tpls[2].IsDefault {
		t.Fatal("seul le gabarit Grille A4 doit être par défaut")
	}
}

func TestRepairTimeLimitParity(t *testing.T) {
	db := BuildEmptyState()
	// Voucher coupé par le routeur : cumul 30 s SOUS la limite (fenêtre de grâce).
	db.HotspotUsers = append(db.HotspotUsers, model.HotspotUser{
		ID: "hu-par", Kind: "voucher", Username: "V-P1", Status: "used",
		TimeLimitMin: 10, UptimeUsedSec: 10*60 - 30, // déficit 30 s ≤ 60 s de grâce
	})
	// Voucher avec un GROS déficit : pas de réparation.
	db.HotspotUsers = append(db.HotspotUsers, model.HotspotUser{
		ID: "hu-far", Kind: "voucher", Username: "V-P2", Status: "used",
		TimeLimitMin: 60, UptimeUsedSec: 30 * 60,
	})
	// Voucher avec session LIVE : le routeur n'a pas encore coupé — intouchable.
	db.HotspotUsers = append(db.HotspotUsers, model.HotspotUser{
		ID: "hu-live", Kind: "voucher", Username: "V-P3", Status: "active",
		TimeLimitMin: 5, UptimeUsedSec: 5*60 - 10,
	})
	db.Sessions = append(db.Sessions, model.Session{ID: "s-1", UserID: "hu-live", Username: "V-P3"})

	n := model.RepairTimeLimitParity(db)
	if n != 1 {
		t.Fatalf("1 seul voucher doit être realigné, obtenu %d", n)
	}
	if got := db.HotspotUsers[0].UptimeUsedSec; got != 10*60 {
		t.Fatalf("cumul aligné = %d, attendu %d", got, 10*60)
	}
	if db.HotspotUsers[1].UptimeUsedSec != 30*60 {
		t.Fatal("un déficit hors grâce ne doit pas être réparé")
	}
	if db.HotspotUsers[2].UptimeUsedSec != 5*60-10 {
		t.Fatal("un voucher avec session live ne doit pas être touché")
	}
	// Idempotence : rien à réparer au second passage.
	if n := model.RepairTimeLimitParity(db); n != 0 {
		t.Fatalf("la réparation doit être idempotente, %d au second passage", n)
	}
}

func TestApplyAdminOverride(t *testing.T) {
	db := BuildEmptyState()
	// Compte démo hérité (admin/admin123) encore intact.
	demo := model.AdminUser{ID: "adm-demo", Username: "admin", Name: "Démo", Role: "admin",
		PasswordHash: auth.HashPassword("admin123", "")}
	db.Users = append(db.Users, demo)

	t.Setenv("ADMIN_PASSWORD", "super-secret-2024")
	t.Setenv("ADMIN_USERNAME", "")
	t.Setenv("ADMIN_NAME", "")
	if !applyAdminOverride(db) {
		t.Fatal("l'override doit modifier l'état quand ADMIN_PASSWORD est définie")
	}
	// Le compte démo au mot de passe public est supprimé.
	for _, u := range db.Users {
		if u.ID == "adm-demo" {
			t.Fatal("le compte démo admin/admin123 doit être retiré")
		}
	}
	// Le compte ADMIN_USERNAME est créé et vérifiable.
	var admin *model.AdminUser
	for i := range db.Users {
		if db.Users[i].Username == "admin" {
			admin = &db.Users[i]
		}
	}
	if admin == nil {
		t.Fatal("le compte administrateur doit être créé depuis l'environnement")
	}
	if !strings.HasPrefix(admin.PasswordHash, "$2") {
		t.Fatal("le mot de passe de l'override doit être bcrypt")
	}
	if admin.EnvPasswordHash == "" {
		t.Fatal("EnvPasswordHash doit mémoriser l'intention de la variable d'environnement")
	}
	// Sans ADMIN_PASSWORD : aucun changement.
	t.Setenv("ADMIN_PASSWORD", "")
	if applyAdminOverride(db) {
		t.Fatal("sans ADMIN_PASSWORD, l'override ne doit rien faire")
	}
}

func TestGetOrCreateNotifSettings(t *testing.T) {
	db := BuildEmptyState()
	s := GetOrCreateNotifSettings(db, "acc-n1")
	if s.OfflineAfterSec != 135 || s.LowStockThreshold != 25 || s.ReportHour != 20 {
		t.Fatalf("défauts de notification inattendus : %+v", s)
	}
	if _, ok := db.NotifSettings["acc-n1"]; !ok {
		t.Fatal("les réglages créés doivent être écrits dans la base")
	}
	// Bornes appliquées par Normalize à l'écriture.
	s.OfflineAfterSec = 1
	s.ReportHour = 99
	SetNotifSettings(db, s)
	got := db.NotifSettings["acc-n1"]
	if got.OfflineAfterSec != 60 {
		t.Fatalf("OfflineAfterSec doit être borné à 60 minimum, obtenu %d", got.OfflineAfterSec)
	}
	if got.ReportHour < 0 || got.ReportHour > 23 {
		t.Fatalf("ReportHour doit être borné 0-23, obtenu %d", got.ReportHour)
	}
}

// TestTickNothingUnderTwoSeconds — Tick est un no-op rapproché : deux ticks
// espacés de moins de 2 s ne doivent rien faire (garde anti-sur-exécution).
func TestTickNothingUnderTwoSeconds(t *testing.T) {
	db := BuildEmptyState()
	db.LastTick = time.Now()
	db.Routers = append(db.Routers, model.Router{ID: "rt-t", Mode: "simulated", CPULoad: 10})
	before := db.Routers[0].UptimeSec
	Tick(db, time.Now().Add(time.Second)) // < 2 s : refusé
	if db.Routers[0].UptimeSec != before {
		t.Fatal("Tick rapproché (< 2 s) ne doit rien faire")
	}
}
