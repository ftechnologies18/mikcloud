// Persistance PostgreSQL (Neon) — schéma relationnel + synchronisation différentielle.
//
// Architecture : la mémoire reste le moteur de calcul (les handlers continuent
// de travailler sur *model.DB sous verrou), PostgreSQL devient la source de
// vérité durable entre redémarrages. À chaque Save(), seules les lignes
// ajoutées, modifiées ou supprimées sont écrites (une transaction par sauvegarde),
// ce qui reste léger même avec le polling 5 s du tableau de bord.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // pilote « pgx » pour database/sql

	"mikcloud/hotspot-api/internal/model"
)

// maxRowsPerStatement — bornes d'insertion multi-lignes (limite PostgreSQL : 65535 paramètres).
const maxRowsPerStatement = 200

// PG — backend PostgreSQL : pool de connexions + empreintes (hash FNV-1a) de la
// dernière synchronisation réussie, par table, pour calculer les différences.
type PG struct {
	db     *sql.DB
	hashes map[string]map[string]uint64
}

// OpenPG ouvre le pool, attend que la base réponde (cold start Neon) et crée le
// schéma si nécessaire (DDL idempotent).
func OpenPG(databaseURL string) (*PG, error) {
	// Neon exige TLS ; on ajoute sslmode=require si l'URL n'en précise pas.
	if !strings.Contains(databaseURL, "sslmode=") {
		sep := "?"
		if strings.Contains(databaseURL, "?") {
			sep = "&"
		}
		databaseURL += sep + "sslmode=require"
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("ouverture PostgreSQL : %w", err)
	}
	// Réglages adaptés à Neon (compute suspendu après inactivité) :
	// peu de connexions, recyclées avant que Neon ne les coupe.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(3 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)

	// Ping avec retry : le premier réveil d'une base Neon peut prendre quelques secondes.
	var lastErr error
	for attempt := 1; attempt <= 10; attempt++ {
		if lastErr = db.Ping(); lastErr == nil {
			break
		}
		log.Printf("pg: tentative %d/10 — base pas encore prête (%v), nouvelle tentative dans 1 s…", attempt, lastErr)
		time.Sleep(time.Second)
	}
	if lastErr != nil {
		db.Close()
		return nil, fmt.Errorf("PostgreSQL injoignable : %w", lastErr)
	}

	p := &PG{db: db, hashes: map[string]map[string]uint64{}}
	if err := p.ensureSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return p, nil
}

// Close ferme le pool.
func (p *PG) Close() error { return p.db.Close() }

// ---------------------------------------------------------------------------
// Schéma (idempotent)
// ---------------------------------------------------------------------------

// ensureSchema crée les tables et index si absents. Les horodatages sont
// conservés en TEXT (RFC3339) pour garantir un aller-retour strictement
// identique avec le modèle Go ; last_tick seul est en TIMESTAMPTZ (time.Time).
func (p *PG) ensureSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS admin_users (
                        id            TEXT PRIMARY KEY,
                        name          TEXT NOT NULL,
                        username      TEXT NOT NULL,
                        role          TEXT NOT NULL,
                        password_hash TEXT NOT NULL,
                        salt          TEXT NOT NULL,
                        created_at    TEXT NOT NULL
                )`,
		`CREATE TABLE IF NOT EXISTS routers (
                        id              TEXT PRIMARY KEY,
                        name            TEXT NOT NULL,
                        host            TEXT NOT NULL,
                        port            INTEGER NOT NULL,
                        username        TEXT NOT NULL,
                        password        TEXT NOT NULL,
                        mode            TEXT NOT NULL,
                        status          TEXT NOT NULL,
                        version         TEXT NOT NULL,
                        uptime_sec      BIGINT NOT NULL,
                        cpu_load        INTEGER NOT NULL,
                        hotspot_users   INTEGER NOT NULL,
                        active_sessions INTEGER NOT NULL,
                        created_at      TEXT NOT NULL
                )`,
		`CREATE TABLE IF NOT EXISTS profiles (
                        id                  TEXT PRIMARY KEY,
                        name                TEXT NOT NULL,
                        rate_limit          TEXT NOT NULL,
                        session_timeout_min INTEGER NOT NULL,
                        shared_users        INTEGER NOT NULL,
                        validity_days       INTEGER NOT NULL,
                        price               INTEGER NOT NULL,
                        data_quota_mb       INTEGER NOT NULL,
                        created_at          TEXT NOT NULL
                )`,
		`CREATE TABLE IF NOT EXISTS hotspot_users (
                        id              TEXT PRIMARY KEY,
                        kind            TEXT NOT NULL,
                        username        TEXT NOT NULL,
                        password        TEXT NOT NULL,
                        profile_id      TEXT NOT NULL,
                        profile_name    TEXT NOT NULL,
                        router_id       TEXT NOT NULL,
                        router_name     TEXT NOT NULL,
                        status          TEXT NOT NULL,
                        batch_id        TEXT NOT NULL,
                        reseller_id     TEXT NOT NULL,
                        reseller_name   TEXT NOT NULL,
                        comment         TEXT NOT NULL,
                        bytes_in        BIGINT NOT NULL,
                        bytes_out       BIGINT NOT NULL,
                        uptime_used_sec BIGINT NOT NULL,
                        created_at      TEXT NOT NULL,
                        expires_at      TEXT NOT NULL,
                        used_at         TEXT NOT NULL,
                        price           INTEGER NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_hotspot_users_batch    ON hotspot_users (batch_id)`,
		`CREATE INDEX IF NOT EXISTS idx_hotspot_users_router   ON hotspot_users (router_id)`,
		`CREATE INDEX IF NOT EXISTS idx_hotspot_users_reseller ON hotspot_users (reseller_id)`,
		`CREATE INDEX IF NOT EXISTS idx_hotspot_users_status   ON hotspot_users (status)`,
		`CREATE INDEX IF NOT EXISTS idx_hotspot_users_created  ON hotspot_users (created_at)`,
		`CREATE TABLE IF NOT EXISTS batches (
                        id            TEXT PRIMARY KEY,
                        profile_id    TEXT NOT NULL,
                        profile_name  TEXT NOT NULL,
                        router_id     TEXT NOT NULL,
                        router_name   TEXT NOT NULL,
                        count         INTEGER NOT NULL,
                        unit_price    INTEGER NOT NULL,
                        total_cost    INTEGER NOT NULL,
                        channel       TEXT NOT NULL,
                        reseller_id   TEXT NOT NULL,
                        reseller_name TEXT NOT NULL,
                        created_at    TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_batches_router  ON batches (router_id)`,
		`CREATE INDEX IF NOT EXISTS idx_batches_created ON batches (created_at)`,
		`CREATE TABLE IF NOT EXISTS resellers (
                        id            TEXT PRIMARY KEY,
                        name          TEXT NOT NULL,
                        username      TEXT NOT NULL,
                        phone         TEXT NOT NULL,
                        credit        INTEGER NOT NULL,
                        vouchers_sold INTEGER NOT NULL,
                        revenue       INTEGER NOT NULL,
                        status        TEXT NOT NULL,
                        created_at    TEXT NOT NULL
                )`,
		`CREATE TABLE IF NOT EXISTS transactions (
                        id            TEXT PRIMARY KEY,
                        type          TEXT NOT NULL,
                        reseller_id   TEXT NOT NULL,
                        reseller_name TEXT NOT NULL,
                        amount        INTEGER NOT NULL,
                        note          TEXT NOT NULL,
                        at            TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_transactions_reseller ON transactions (reseller_id)`,
		`CREATE INDEX IF NOT EXISTS idx_transactions_at       ON transactions (at)`,
		`CREATE TABLE IF NOT EXISTS sessions (
                        id           TEXT PRIMARY KEY,
                        user_id      TEXT NOT NULL,
                        username     TEXT NOT NULL,
                        profile_name TEXT NOT NULL,
                        router_id    TEXT NOT NULL,
                        router_name  TEXT NOT NULL,
                        ip           TEXT NOT NULL,
                        mac          TEXT NOT NULL,
                        started_at   TEXT NOT NULL,
                        uptime_sec   BIGINT NOT NULL,
                        bytes_in     BIGINT NOT NULL,
                        bytes_out    BIGINT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_router ON sessions (router_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user   ON sessions (user_id)`,
		`CREATE TABLE IF NOT EXISTS activity (
                        id      TEXT PRIMARY KEY,
                        type    TEXT NOT NULL,
                        message TEXT NOT NULL,
                        at      TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_activity_at ON activity (at)`,
		`CREATE TABLE IF NOT EXISTS sales (
                        id            TEXT PRIMARY KEY,
                        amount        INTEGER NOT NULL,
                        profile_name  TEXT NOT NULL,
                        count         INTEGER NOT NULL,
                        channel       TEXT NOT NULL,
                        reseller_name TEXT NOT NULL,
                        router_id     TEXT NOT NULL,
                        router_name   TEXT NOT NULL,
                        batch_id      TEXT NOT NULL,
                        at            TEXT NOT NULL
                )`,
		`CREATE INDEX IF NOT EXISTS idx_sales_at     ON sales (at)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_router ON sales (router_id)`,
		`CREATE TABLE IF NOT EXISTS settings (
                        id               INTEGER PRIMARY KEY, -- toujours 1 (singleton)
                        tenant_name      TEXT NOT NULL,
                        tenant_currency  TEXT NOT NULL,
                        tenant_timezone  TEXT NOT NULL,
                        plan_name        TEXT NOT NULL,
                        plan_max_routers TEXT NOT NULL,
                        plan_max_users   TEXT NOT NULL,
                        last_tick        TIMESTAMPTZ
                )`,
	}
	for _, q := range stmts {
		if _, err := p.db.Exec(q); err != nil {
			return fmt.Errorf("création du schéma : %w (requête : %.80s)", err, q)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Chargement au démarrage
// ---------------------------------------------------------------------------

// Load relit toute la base et reconstruit *model.DB. Retourne found=false si la
// base est vide (l'appelant déclenchera le seed démo + synchronisation initiale).
func (p *PG) Load() (db *model.DB, found bool, err error) {
	db = &model.DB{
		Users:        []model.AdminUser{},
		Routers:      []model.Router{},
		Profiles:     []model.Profile{},
		HotspotUsers: []model.HotspotUser{},
		Batches:      []model.Batch{},
		Resellers:    []model.Reseller{},
		Transactions: []model.Transaction{},
		Sessions:     []model.Session{},
		Activity:     []model.Activity{},
		Sales:        []model.Sale{},
	}

	steps := []struct {
		name  string
		query func() error
	}{
		{"admin_users", func() error { return loadInto(p, &db.Users, adminSpec) }},
		{"routers", func() error { return loadInto(p, &db.Routers, routerSpec) }},
		{"profiles", func() error { return loadInto(p, &db.Profiles, profileSpec) }},
		{"hotspot_users", func() error { return loadInto(p, &db.HotspotUsers, hotspotUserSpec) }},
		{"batches", func() error { return loadInto(p, &db.Batches, batchSpec) }},
		{"resellers", func() error { return loadInto(p, &db.Resellers, resellerSpec) }},
		{"transactions", func() error { return loadInto(p, &db.Transactions, transactionSpec) }},
		{"sessions", func() error { return loadInto(p, &db.Sessions, sessionSpec) }},
		{"activity", func() error { return loadInto(p, &db.Activity, activitySpec) }},
		{"sales", func() error { return loadInto(p, &db.Sales, saleSpec) }},
		{"settings", func() error { return p.loadSettings(db) }},
	}
	for _, st := range steps {
		if err := st.query(); err != nil {
			return nil, false, fmt.Errorf("lecture %s : %w", st.name, err)
		}
	}

	// Les tris applicatifs sont faits en Go (sort.Slice dans les handlers),
	// l'ordre de lecture n'a donc aucune importance.

	found = len(db.Users) > 0 || len(db.Routers) > 0 || len(db.Profiles) > 0 ||
		len(db.HotspotUsers) > 0 || len(db.Resellers) > 0 || len(db.Sales) > 0
	if !found {
		return nil, false, nil
	}

	// Le cache d'empreintes reflète l'état chargé.
	p.rebuildHashes(db)
	return db, true, nil
}

// loadSettings lit la ligne singleton settings (id=1).
func (p *PG) loadSettings(db *model.DB) error {
	var (
		tenantName, tenantCurrency, tenantTimezone string
		planName, planMaxRouters, planMaxUsers     string
		lastTick                                   sql.NullTime
	)
	err := p.db.QueryRow(
		`SELECT tenant_name, tenant_currency, tenant_timezone, plan_name, plan_max_routers, plan_max_users, last_tick
                 FROM settings WHERE id = 1`).Scan(
		&tenantName, &tenantCurrency, &tenantTimezone,
		&planName, &planMaxRouters, &planMaxUsers, &lastTick)
	if err == sql.ErrNoRows {
		return nil // pas encore de réglages → valeurs par défaut du modèle
	}
	if err != nil {
		return err
	}
	db.Tenant = model.Tenant{Name: tenantName, Currency: tenantCurrency, Timezone: tenantTimezone}
	db.Settings = model.Settings{
		Tenant: db.Tenant,
		Plan:   model.Plan{Name: planName, MaxRouters: planMaxRouters, MaxUsers: planMaxUsers},
	}
	if lastTick.Valid {
		db.LastTick = lastTick.Time
	}
	return nil
}

// ---------------------------------------------------------------------------
// Synchronisation différentielle (appelée par Store.Save, sous verrou)
// ---------------------------------------------------------------------------

// Sync compare l'état mémoire aux empreintes de la dernière synchronisation
// réussie et applique les différences en une transaction :
// upserts des lignes nouvelles/modifiées, suppressions des disparues.
func (p *PG) Sync(db *model.DB) error {
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("pg sync (begin) : %w", err)
	}
	defer tx.Rollback() // no-op si Commit réussit

	if err := syncTable(tx, p.hashes, adminSpec, db.Users); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, routerSpec, db.Routers); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, profileSpec, db.Profiles); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, hotspotUserSpec, db.HotspotUsers); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, batchSpec, db.Batches); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, resellerSpec, db.Resellers); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, transactionSpec, db.Transactions); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, sessionSpec, db.Sessions); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, activitySpec, db.Activity); err != nil {
		return err
	}
	if err := syncTable(tx, p.hashes, saleSpec, db.Sales); err != nil {
		return err
	}
	if err := p.syncSettings(tx, db); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("pg sync (commit) : %w", err)
	}
	return nil
}

// syncSettings écrit le singleton (upsert id=1).
func (p *PG) syncSettings(tx *sql.Tx, db *model.DB) error {
	lastTick := sql.NullTime{Time: db.LastTick, Valid: !db.LastTick.IsZero()}
	_, err := tx.Exec(
		`INSERT INTO settings (id, tenant_name, tenant_currency, tenant_timezone, plan_name, plan_max_routers, plan_max_users, last_tick)
                 VALUES (1, $1, $2, $3, $4, $5, $6, $7)
                 ON CONFLICT (id) DO UPDATE SET
                   tenant_name      = EXCLUDED.tenant_name,
                   tenant_currency  = EXCLUDED.tenant_currency,
                   tenant_timezone  = EXCLUDED.tenant_timezone,
                   plan_name        = EXCLUDED.plan_name,
                   plan_max_routers = EXCLUDED.plan_max_routers,
                   plan_max_users   = EXCLUDED.plan_max_users,
                   last_tick        = EXCLUDED.last_tick`,
		db.Tenant.Name, db.Tenant.Currency, db.Tenant.Timezone,
		db.Settings.Plan.Name, db.Settings.Plan.MaxRouters, db.Settings.Plan.MaxUsers,
		lastTick)
	if err != nil {
		return fmt.Errorf("pg sync settings : %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Outillage générique : specs de tables, diff, upserts multi-lignes
// (fonctions libres : les méthodes Go ne peuvent pas introduire de paramètres
// de type — c'est une restriction du langage)
// ---------------------------------------------------------------------------

// entitySpec — description d'une table : colonnes (cols[0] est TOUJOURS la
// clé primaire « id »), extraction d'id, lecture et écriture d'une ligne.
type entitySpec[T any] struct {
	table  string
	cols   []string
	idOf   func(*T) string
	scan   func(*sql.Rows) (T, error)
	args   func(*T) []any
	hashOf func(*T) uint64
}

// loadInto — SELECT des colonnes explicites → tranche typée.
func loadInto[T any](p *PG, out *[]T, spec entitySpec[T]) error {
	rows, err := p.db.Query(`SELECT ` + strings.Join(spec.cols, ", ") + ` FROM ` + spec.table)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		v, err := spec.scan(rows)
		if err != nil {
			return err
		}
		*out = append(*out, v)
	}
	return rows.Err()
}

// syncTable — différentiel : détecte ajouts/modifications (comparaison
// d'empreintes) et disparitions (id absents), applique le tout, puis rafraîchit
// le cache UNIQUEMENT en cas de succès (un échec sera retenté au Save suivant).
func syncTable[T any](tx *sql.Tx, hashes map[string]map[string]uint64, spec entitySpec[T], rows []T) error {
	cached := hashes[spec.table]
	if cached == nil {
		cached = map[string]uint64{}
		hashes[spec.table] = cached
	}

	seen := make(map[string]struct{}, len(rows))
	var changed []T
	for i := range rows {
		id := spec.idOf(&rows[i])
		seen[id] = struct{}{}
		h := spec.hashOf(&rows[i])
		if old, ok := cached[id]; !ok || old != h {
			changed = append(changed, rows[i])
		}
	}
	var removed []string
	for id := range cached {
		if _, ok := seen[id]; !ok {
			removed = append(removed, id)
		}
	}

	if len(changed) > 0 {
		if err := upsertRows(tx, spec, changed); err != nil {
			return err
		}
	}
	if len(removed) > 0 {
		if err := deleteRows(tx, spec.table, removed); err != nil {
			return err
		}
	}

	// Cache rafraîchi uniquement après succès des écritures.
	for id := range cached {
		delete(cached, id)
	}
	for i := range rows {
		cached[spec.idOf(&rows[i])] = spec.hashOf(&rows[i])
	}
	return nil
}

// hashEntity — empreinte FNV-1a de la sérialisation JSON (l'ordre des champs
// d'une struct est stable en Go → déterministe).
func hashEntity[T any](v *T) uint64 {
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	h := fnv.New64a()
	h.Write(b)
	return h.Sum64()
}

// deleteRows — DELETE ... WHERE id IN (…) par blocs de 500.
func deleteRows(tx *sql.Tx, table string, ids []string) error {
	for start := 0; start < len(ids); start += 500 {
		end := min(start+500, len(ids))
		chunk := ids[start:end]
		ph := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, id := range chunk {
			ph[i] = "$" + strconv.Itoa(i+1)
			args[i] = id
		}
		q := `DELETE FROM ` + table + ` WHERE id IN (` + strings.Join(ph, ",") + `)`
		if _, err := tx.Exec(q, args...); err != nil {
			return fmt.Errorf("suppression %s : %w", table, err)
		}
	}
	return nil
}

// upsertRows — INSERT ... ON CONFLICT (id) DO UPDATE par blocs multi-lignes
// (une seule requête pour jusqu'à 200 lignes → un seul aller-retour réseau).
func upsertRows[T any](tx *sql.Tx, spec entitySpec[T], rows []T) error {
	n := len(spec.cols)
	// Clause SET de l'upsert (toutes les colonnes sauf la clé).
	sets := make([]string, 0, n-1)
	for _, c := range spec.cols[1:] {
		sets = append(sets, c+` = EXCLUDED.`+c)
	}
	setClause := strings.Join(sets, ", ")

	for start := 0; start < len(rows); start += maxRowsPerStatement {
		end := min(start+maxRowsPerStatement, len(rows))
		chunk := rows[start:end]

		var sb strings.Builder
		args := make([]any, 0, len(chunk)*n)
		k := 0
		sb.WriteString(`INSERT INTO ` + spec.table + ` (` + strings.Join(spec.cols, ", ") + `) VALUES `)
		for i := range chunk {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteByte('(')
			for j := 0; j < n; j++ {
				if j > 0 {
					sb.WriteByte(',')
				}
				k++
				sb.WriteString("$" + strconv.Itoa(k))
			}
			sb.WriteByte(')')
			args = append(args, spec.args(&chunk[i])...)
		}
		sb.WriteString(` ON CONFLICT (id) DO UPDATE SET ` + setClause)

		if _, err := tx.Exec(sb.String(), args...); err != nil {
			return fmt.Errorf("upsert %s : %w", spec.table, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Specs concrètes des 10 tables entité.
// IMPORTANT : l'ordre des colonnes doit rester strictement identique entre
// cols, scan et args.
// ---------------------------------------------------------------------------

var adminSpec = entitySpec[model.AdminUser]{
	table: "admin_users",
	cols:  []string{"id", "name", "username", "role", "password_hash", "salt", "created_at"},
	idOf:  func(u *model.AdminUser) string { return u.ID },
	scan: func(r *sql.Rows) (model.AdminUser, error) {
		var u model.AdminUser
		err := r.Scan(&u.ID, &u.Name, &u.Username, &u.Role, &u.PasswordHash, &u.Salt, &u.CreatedAt)
		return u, err
	},
	args: func(u *model.AdminUser) []any {
		return []any{u.ID, u.Name, u.Username, u.Role, u.PasswordHash, u.Salt, u.CreatedAt}
	},
	hashOf: hashEntity[model.AdminUser],
}

var routerSpec = entitySpec[model.Router]{
	table: "routers",
	cols: []string{"id", "name", "host", "port", "username", "password", "mode", "status",
		"version", "uptime_sec", "cpu_load", "hotspot_users", "active_sessions", "created_at"},
	idOf: func(x *model.Router) string { return x.ID },
	scan: func(r *sql.Rows) (model.Router, error) {
		var x model.Router
		err := r.Scan(&x.ID, &x.Name, &x.Host, &x.Port, &x.Username, &x.Password, &x.Mode, &x.Status,
			&x.Version, &x.UptimeSec, &x.CPULoad, &x.HotspotUsers, &x.ActiveSessions, &x.CreatedAt)
		return x, err
	},
	args: func(x *model.Router) []any {
		return []any{x.ID, x.Name, x.Host, x.Port, x.Username, x.Password, x.Mode, x.Status,
			x.Version, x.UptimeSec, x.CPULoad, x.HotspotUsers, x.ActiveSessions, x.CreatedAt}
	},
	hashOf: hashEntity[model.Router],
}

var profileSpec = entitySpec[model.Profile]{
	table: "profiles",
	cols:  []string{"id", "name", "rate_limit", "session_timeout_min", "shared_users", "validity_days", "price", "data_quota_mb", "created_at"},
	idOf:  func(x *model.Profile) string { return x.ID },
	scan: func(r *sql.Rows) (model.Profile, error) {
		var x model.Profile
		err := r.Scan(&x.ID, &x.Name, &x.RateLimit, &x.SessionTimeoutMin, &x.SharedUsers,
			&x.ValidityDays, &x.Price, &x.DataQuotaMb, &x.CreatedAt)
		return x, err
	},
	args: func(x *model.Profile) []any {
		return []any{x.ID, x.Name, x.RateLimit, x.SessionTimeoutMin, x.SharedUsers,
			x.ValidityDays, x.Price, x.DataQuotaMb, x.CreatedAt}
	},
	hashOf: hashEntity[model.Profile],
}

var hotspotUserSpec = entitySpec[model.HotspotUser]{
	table: "hotspot_users",
	cols: []string{"id", "kind", "username", "password", "profile_id", "profile_name",
		"router_id", "router_name", "status", "batch_id", "reseller_id", "reseller_name",
		"comment", "bytes_in", "bytes_out", "uptime_used_sec", "created_at", "expires_at", "used_at", "price"},
	idOf: func(x *model.HotspotUser) string { return x.ID },
	scan: func(r *sql.Rows) (model.HotspotUser, error) {
		var x model.HotspotUser
		err := r.Scan(&x.ID, &x.Kind, &x.Username, &x.Password, &x.ProfileID, &x.ProfileName,
			&x.RouterID, &x.RouterName, &x.Status, &x.BatchID, &x.ResellerID, &x.ResellerName,
			&x.Comment, &x.BytesIn, &x.BytesOut, &x.UptimeUsedSec, &x.CreatedAt, &x.ExpiresAt, &x.UsedAt, &x.Price)
		return x, err
	},
	args: func(x *model.HotspotUser) []any {
		return []any{x.ID, x.Kind, x.Username, x.Password, x.ProfileID, x.ProfileName,
			x.RouterID, x.RouterName, x.Status, x.BatchID, x.ResellerID, x.ResellerName,
			x.Comment, x.BytesIn, x.BytesOut, x.UptimeUsedSec, x.CreatedAt, x.ExpiresAt, x.UsedAt, x.Price}
	},
	hashOf: hashEntity[model.HotspotUser],
}

var batchSpec = entitySpec[model.Batch]{
	table: "batches",
	cols:  []string{"id", "profile_id", "profile_name", "router_id", "router_name", "count", "unit_price", "total_cost", "channel", "reseller_id", "reseller_name", "created_at"},
	idOf:  func(x *model.Batch) string { return x.ID },
	scan: func(r *sql.Rows) (model.Batch, error) {
		var x model.Batch
		err := r.Scan(&x.ID, &x.ProfileID, &x.ProfileName, &x.RouterID, &x.RouterName,
			&x.Count, &x.UnitPrice, &x.TotalCost, &x.Channel, &x.ResellerID, &x.ResellerName, &x.CreatedAt)
		return x, err
	},
	args: func(x *model.Batch) []any {
		return []any{x.ID, x.ProfileID, x.ProfileName, x.RouterID, x.RouterName,
			x.Count, x.UnitPrice, x.TotalCost, x.Channel, x.ResellerID, x.ResellerName, x.CreatedAt}
	},
	hashOf: hashEntity[model.Batch],
}

var resellerSpec = entitySpec[model.Reseller]{
	table: "resellers",
	cols:  []string{"id", "name", "username", "phone", "credit", "vouchers_sold", "revenue", "status", "created_at"},
	idOf:  func(x *model.Reseller) string { return x.ID },
	scan: func(r *sql.Rows) (model.Reseller, error) {
		var x model.Reseller
		err := r.Scan(&x.ID, &x.Name, &x.Username, &x.Phone, &x.Credit, &x.VouchersSold, &x.Revenue, &x.Status, &x.CreatedAt)
		return x, err
	},
	args: func(x *model.Reseller) []any {
		return []any{x.ID, x.Name, x.Username, x.Phone, x.Credit, x.VouchersSold, x.Revenue, x.Status, x.CreatedAt}
	},
	hashOf: hashEntity[model.Reseller],
}

var transactionSpec = entitySpec[model.Transaction]{
	table: "transactions",
	cols:  []string{"id", "type", "reseller_id", "reseller_name", "amount", "note", "at"},
	idOf:  func(x *model.Transaction) string { return x.ID },
	scan: func(r *sql.Rows) (model.Transaction, error) {
		var x model.Transaction
		err := r.Scan(&x.ID, &x.Type, &x.ResellerID, &x.ResellerName, &x.Amount, &x.Note, &x.At)
		return x, err
	},
	args: func(x *model.Transaction) []any {
		return []any{x.ID, x.Type, x.ResellerID, x.ResellerName, x.Amount, x.Note, x.At}
	},
	hashOf: hashEntity[model.Transaction],
}

var sessionSpec = entitySpec[model.Session]{
	table: "sessions",
	cols:  []string{"id", "user_id", "username", "profile_name", "router_id", "router_name", "ip", "mac", "started_at", "uptime_sec", "bytes_in", "bytes_out"},
	idOf:  func(x *model.Session) string { return x.ID },
	scan: func(r *sql.Rows) (model.Session, error) {
		var x model.Session
		err := r.Scan(&x.ID, &x.UserID, &x.Username, &x.ProfileName, &x.RouterID, &x.RouterName,
			&x.IP, &x.MAC, &x.StartedAt, &x.UptimeSec, &x.BytesIn, &x.BytesOut)
		return x, err
	},
	args: func(x *model.Session) []any {
		return []any{x.ID, x.UserID, x.Username, x.ProfileName, x.RouterID, x.RouterName,
			x.IP, x.MAC, x.StartedAt, x.UptimeSec, x.BytesIn, x.BytesOut}
	},
	hashOf: hashEntity[model.Session],
}

var activitySpec = entitySpec[model.Activity]{
	table: "activity",
	cols:  []string{"id", "type", "message", "at"},
	idOf:  func(x *model.Activity) string { return x.ID },
	scan: func(r *sql.Rows) (model.Activity, error) {
		var x model.Activity
		err := r.Scan(&x.ID, &x.Type, &x.Message, &x.At)
		return x, err
	},
	args: func(x *model.Activity) []any {
		return []any{x.ID, x.Type, x.Message, x.At}
	},
	hashOf: hashEntity[model.Activity],
}

var saleSpec = entitySpec[model.Sale]{
	table: "sales",
	cols:  []string{"id", "amount", "profile_name", "count", "channel", "reseller_name", "router_id", "router_name", "batch_id", "at"},
	idOf:  func(x *model.Sale) string { return x.ID },
	scan: func(r *sql.Rows) (model.Sale, error) {
		var x model.Sale
		err := r.Scan(&x.ID, &x.Amount, &x.ProfileName, &x.Count, &x.Channel, &x.ResellerName,
			&x.RouterID, &x.RouterName, &x.BatchID, &x.At)
		return x, err
	},
	args: func(x *model.Sale) []any {
		return []any{x.ID, x.Amount, x.ProfileName, x.Count, x.Channel, x.ResellerName,
			x.RouterID, x.RouterName, x.BatchID, x.At}
	},
	hashOf: hashEntity[model.Sale],
}

// rebuildHashes — reconstruit le cache d'empreintes à partir d'un état mémoire
// (après un Load ou un seed initial).
func (p *PG) rebuildHashes(db *model.DB) {
	p.hashes = map[string]map[string]uint64{
		adminSpec.table:       hashRows(db.Users, adminSpec),
		routerSpec.table:      hashRows(db.Routers, routerSpec),
		profileSpec.table:     hashRows(db.Profiles, profileSpec),
		hotspotUserSpec.table: hashRows(db.HotspotUsers, hotspotUserSpec),
		batchSpec.table:       hashRows(db.Batches, batchSpec),
		resellerSpec.table:    hashRows(db.Resellers, resellerSpec),
		transactionSpec.table: hashRows(db.Transactions, transactionSpec),
		sessionSpec.table:     hashRows(db.Sessions, sessionSpec),
		activitySpec.table:    hashRows(db.Activity, activitySpec),
		saleSpec.table:        hashRows(db.Sales, saleSpec),
	}
}

// hashRows — empreintes indexées par id.
func hashRows[T any](rows []T, spec entitySpec[T]) map[string]uint64 {
	m := make(map[string]uint64, len(rows))
	for i := range rows {
		m[spec.idOf(&rows[i])] = spec.hashOf(&rows[i])
	}
	return m
}
