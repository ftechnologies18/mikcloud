# RUNBOOK — Architecture DB hybride mikcloud (N°274)

Outil d'exécution : workflow **`ops-db-hybrid.yml`** (dispatch manuel, 7 modes).
Références : CHANGELOG N°274 (récit + leçons), Task 5/6 (analyse + architecture),
`backup-neon.sh` (pièges N°243 repris), `docs/RUNBOOK-SECRETS.md`.

## 0. Architecture (4 étages de durabilité)

| Tier | Stockage | État après N°274 | RPO |
|------|----------|------------------|-----|
| 0 | Mémoire `model.DB` (backend) | **vivant, inchangé** — source de vérité runtime, flush asynchrone ≤1 tx/3 s | ~0 |
| 1 | **PostgreSQL 18.6 local Ftechci** (127.0.0.1:5432, DB `mikcloud`, rôle `mikcloud`) | **PRIMAIRE** depuis 03:03:54 UTC le 09/10 | flush ≤3 s |
| 1.5 | WAL archivé `/var/backups/mikcloud/wal` (`archive_timeout=60s`) | actif (local) — upload OCI Object Storage en N°275 | ≤60 s |
| 2 | **Supabase (pooler session :5432)** | fallback chaud **FIGÉ** au cutover — re-sync nocturne en N°275 | figé |
| 3 | Artefact GitHub `supabase-dr-*` chiffré `BACKUP_KEY` (AES-256-CBC/PBKDF2, rétention 30 j) | actif (mode=backup) | au tir |

- **Neon est figé aussi** : le timer `mikcloud-backup` (Supabase→Neon,
  03:00 UTC) est désactivé au cutover — sinon il écraserait le coffre Neon
  avec Supabase figé. Chaîne cible N°275 : local→Supabase (nuit)→Neon (nuit).
- Le DSN local vit dans `/etc/mikcloud/mikcloud.env` (`DATABASE_URL`) ;
  le mot de passe du rôle dans `/etc/mikcloud/localpg.env` (root:root 600).
- `sslmode=disable` : légitime (socket local, aucun port public — Security
  List OCI = 80/443/22 ; échappatoire documentée dans `pg.go`, N°75).

## 1. Configuration PG locale (Ftechci 4/24)

`/etc/postgresql/18/main/conf.d/mikcloud.conf` : listen localhost ·
max_connections 60 · shared_buffers 4GB · effective_cache_size 12GB ·
work_mem 16MB · maintenance_work_mem 256MB · archive_mode on ·
archive_command → copie WAL locale · archive_timeout 60s ·
shared_preload pg_stat_statements · log_checkpoints ·
log_autovacuum_min_duration 1s · autovacuum 6 workers / naptime 30s.

Autovacuum chirurgical (posé au restore, tables chaudes :
`health_checkpoint, hotspot_users, routers, settings, commands`) :
`autovacuum_vacuum_scale_factor=0.02, autovacuum_analyze_scale_factor=0.01,
autovacuum_vacuum_cost_delay=1`.

## 2. Les 7 modes de `ops-db-hybrid`

| Mode | Effet | Downtime | Notes |
|------|-------|----------|-------|
| `audit` | inventaire lecture seule (système, services, env keys, PG, PGDG, hosts DB, comptages Supabase) | 0 | sûr à relancer |
| `backup` | pg_dump -Fc Supabase → `/var/backups/mikcloud/db/supabase-*.dump` + artefact chiffré | 0 | dump en 2 phases, TOC ≥30 tables |
| `setup-pg` | installe/configure PG serveur + rôle + DB + localpg.env | 0 | idempotent (localpg.env préservé) |
| `restore` | DROP/CREATE DB + pg_restore TOC-filtré + ANALYZE + asserts (≥30 tables, >0 lignes) | 0 (backend pas dessus) | verdict drift INFORMATIF (Supabase vit) |
| `cutover` | **bascule primaire** : delta dump/restore → garde comptages STRICT (whitelist append-only) → env backup `.pre-hybrid-*` → flip DSN → restart → preuves (journalctl + tup_written) → gel timer. Rollback AUTO si preuve absente | ~10 s (restart) | `confirm=MIK-DB-HYBRID` requis |
| `rollback` | dump local de sécurité → restore `.pre-hybrid-*` → restart → réactive timer | ~10 s | `confirm` + `accept_stale_data=YES` |
| `status` | état complet + **protection : gel auto du timer si primaire local** | 0 | sûr à relancer |

## 3. Pièges consignés (à ne JAMAIS redécouvrir)

1. **pg_dump → pooler SESSION Supabase (:5432)**, jamais :6543 (transactionnel).
2. **`--schema=public`** obligatoire (schémas plateforme Supabase non
   restaurables hors Supabase, clauses SUSET).
3. **Dump en 2 phases** (fichier, jamais de pipe entre deux bases — SSL
   SYSCALL N°243-d).
4. **Le dump embarque `CREATE SCHEMA public;`** (sans IF NOT EXISTS) →
   toujours restaurer sur base FRAÎCHE + `--use-list` excluant ` SCHEMA `.
5. **`pg_stat_statements` n'est PAS restaurée par pg_restore**
   (CREATE EXTENSION réservé au superuser) → recréer en `sudo -u postgres`.
6. **Répertoires de dumps : `-o "$USER"`** (sinon root → Permission denied).
7. **`sudo . fichier` invalide** → `. <(sudo cat fichier)`.
8. **`/etc/mikcloud` non traversable pour le user SSH** → `sudo test -f`.
9. **Supabase VIT en continu** (insertions + purges) → toute comparaison
   stricte hors fenêtre gelée est fallacieuse.
10. **Au cutover, le timer Supabase→Neon doit être GELÉ** (sinon Neon
    écrasé par des données figées) — protection idempotente au status.
11. **`deploy-oracle.yml` préserve le DSN local** au re-déploiement
    (anti-retour-silencieux, N°274).
12. Mode dégradé du backend : **le service démarre même sans DB** — le
    healthcheck ne prouve RIEN ; preuves = journalctl
    « persistance PostgreSQL active » + `tup_inserted+tup_updated`.

## 4. Utilisation courante

```bash
# État (à faire après chaque geste) :
gh workflow run ops-db-hybrid.yml -f mode=status
# Backup DR chiffré (rétention 30 j) :
gh workflow run ops-db-hybrid.yml -f mode=backup
# ROLLBACK d'urgence (Supabase figé — accepter la péremption) :
gh workflow run ops-db-hybrid.yml -f mode=rollback \
  -f confirm=MIK-DB-HYBRID -f accept_stale_data=YES
```

Rollback manuel complet (sans workflow, sur la VM) :
```bash
sudo cp /etc/mikcloud/mikcloud.env.pre-hybrid-<STAMP> /etc/mikcloud/mikcloud.env
sudo systemctl restart mikcloud
sudo systemctl enable --now mikcloud-backup.timer
```

## 5. Prochaines étapes (N°275)

1. **Reverse-sync nocturne** : script VM (systemd timer 03:00 UTC)
   `pg_dump local → pg_restore --clean --if-exists vers Supabase` (pooler
   session), puis le même flux vers Neon (coffre re-chaîné) ;
2. **WAL → OCI Object Storage** (PITR distant ~1-5 min) — prérequis :
   credentials OCI sur la VM (instance principal ou Customer Secret Keys) ;
3. **Monitoring** : cron `status` + alertes Telegram (drift, croissance,
   WAL, dernier backup) ;
4. **Drill de restauration** complet (démo : détruire le local, restaurer
   depuis dump/WAL, re-flip) ;
5. Rétention `web_vitals` (append-only, 5 888 lignes/jour — purge à décider).
