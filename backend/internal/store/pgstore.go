// pgstore.go — persistance PostgreSQL (Neon) en snapshot JSONB.
//
// Objectif : le plan Render FREE a un système de fichiers ÉPHÉMÈRE — le
// db.json est perdu à chaque redéploiement (inacceptable : ventes, lots,
// revendeurs = de l'argent). Avec DATABASE_URL (Neon free 0,5 Go), le store
// persiste son état complet dans une table app_state (jsonb) et survit aux
// redéploiements, sans changer les 33 endpoints (même interface Lock/Data/Save).
//
// Stratégie d'écriture : coalescing asynchrone (au plus un flush ~400 ms,
// la dernière version gagne) — fluide sous charge, perte max ~0,5 s au crash.
// Sans DATABASE_URL, le store reste 100 % JSON local (dev, tests, offline).
package store

import (
        "database/sql"
        "encoding/json"
        "log"
        "os"
        "time"

        _ "github.com/lib/pq" // driver Postgres pur-Go (Neon exige sslmode=require)

        "mikcloud/hotspot-api/internal/model"
)

// pgSchema — une seule ligne d'état : tout model.DB sérialisé en jsonb.
const pgSchema = `CREATE TABLE IF NOT EXISTS app_state (
        id         int PRIMARY KEY CHECK (id = 1),
        data       jsonb NOT NULL,
        updated_at timestamptz NOT NULL DEFAULT now()
)`

// initPostgres active la persistance Neon : ouvre la connexion, crée la table,
// charge le snapshot existant (ou écrit le seed initial). Retourne l'état chargé.
func (s *Store) initPostgres(dsn string) (bool, error) {
        pg, err := sql.Open("postgres", dsn)
        if err != nil {
                return false, err
        }
        pg.SetMaxOpenConns(4)
        pg.SetMaxIdleConns(2)
        pg.SetConnMaxIdleTime(5 * time.Minute) // Neon suspend les compute inactifs
        if _, err := pg.Exec(pgSchema); err != nil {
                _ = pg.Close()
                return false, err
        }
        s.pg = pg

        // Charge le snapshot existant, s'il y en a un.
        var raw []byte
        err = pg.QueryRow(`SELECT data FROM app_state WHERE id = 1`).Scan(&raw)
        switch {
        case err == sql.ErrNoRows:
                return false, nil // premier lancement : le seed sera écrit par New()
        case err != nil:
                _ = pg.Close()
                s.pg = nil
                return false, err
        }
        var db model.DB
        if err := json.Unmarshal(raw, &db); err != nil {
                _ = pg.Close()
                s.pg = nil
                return false, err
        }
        s.db = &db
        s.ensureSlices()
        return true, nil
}

// startPgWorker lance le goroutine de flush coalescé (à appeler une fois).
func (s *Store) startPgWorker() {
        go func() {
                defer close(s.pgDone)
                for {
                        select {
                        case <-s.pgStop:
                                s.pgFlushOnce() // flush final avant arrêt (SIGTERM Render)
                                return
                        case <-s.pgDirty:
                                time.Sleep(400 * time.Millisecond) // groupe les écritures rapprochées
                                s.drainPgDirty()
                                s.pgFlushOnce()
                        }
                }
        }()
}

// drainPgDirty vide le channel (coalescing : une seule écriture pour N Save).
func (s *Store) drainPgDirty() {
        for {
                select {
                case <-s.pgDirty:
                default:
                        return
                }
        }
}

// pgFlushOnce sérialise l'état courant (sous verrou) et le pousse dans Neon.
func (s *Store) pgFlushOnce() {
        if s.pg == nil {
                return
        }
        s.mu.Lock()
        data, err := json.Marshal(s.db)
        s.mu.Unlock()
        if err != nil {
                log.Printf("store/pg: sérialisation impossible : %v", err)
                return
        }
        if _, err := s.pg.Exec(
                `INSERT INTO app_state (id, data) VALUES (1, $1::jsonb)
                 ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data, updated_at = now()`,
                string(data),
        ); err != nil {
                log.Printf("store/pg: écriture Neon impossible : %v", err)
        }
}

// Close termine proprement : flush final + fermeture de la connexion Neon.
func (s *Store) Close() {
        if s.pg == nil {
                return
        }
        select {
        case <-s.pgStop:
                return // déjà arrêté
        default:
        }
        close(s.pgStop)
        select {
        case <-s.pgDone:
        case <-time.After(5 * time.Second):
                log.Printf("store/pg: flush final trop lent, arrêt forcé")
        }
        _ = s.pg.Close()
}

// pgEnabled — DATABASE_URL est définie (et non vide).
func pgEnabled() bool { return os.Getenv("DATABASE_URL") != "" }
