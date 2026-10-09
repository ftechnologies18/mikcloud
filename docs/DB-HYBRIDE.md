# RUNBOOK — Architecture DB hybride mikcloud (N°274, consolidée N°275/N°276)

Outil d'exécution : workflow **`ops-db-hybrid.yml`** (dispatch + cron
quotidien 04:23 UTC → `health`, 11 modes). Diagnostic/remise en route
VM : **`ops-vm-diag.yml`** (état + serial console history + START/RESET).
Références : CHANGELOG N°274/N°275 (récits + leçons), Task 5/6 (analyse
+ architecture), `backup-neon.sh` (pièges N°243 repris),
`docs/RUNBOOK-SECRETS.md`.

## 0. Architecture (4 étages de durabilité)

| Tier | Stockage | État après N°275 | RPO |
|------|----------|------------------|-----|
| 0 | Mémoire `model.DB` (backend) | **vivant, inchangé** — source de vérité runtime, flush asynchrone ≤1 tx/3 s | ~0 |
| 1 | **PostgreSQL 18.6 local Ftechci** (127.0.0.1:5432, DB `mikcloud`, rôle `mikcloud`) | **PRIMAIRE** depuis 03:03:54 UTC le 09/10 | flush ≤3 s |
| 1.5 | **WAL gz → OCI Object Storage** (`mikcloud-wal`, upload */5 min) + `pg_basebackup` quotidien — archive_command **gzip** (le brut creusait ~17 Go/j) ; **CHIFFREMENT CLIENT AES-256-CBC/PBKDF2 avant upload (N°277, `.enc` illisible sans la clé, refus d'envoyer en clair)** ; prune locale 48 h ; **lifecycle 21 j ACTIF (N°276)** ; PITR distant ≈ base + WAL ≤ 6 min | **ACTIF** | ≤6 min |
| 2 | **Supabase (pooler session :5432)** — re-synchronisé CHAQUE NUIT 03:00 UTC par `mikcloud-reverse-sync.timer` (restore `--clean` + RLS ré-armé) — preuve : 40/40 tables, `last_success=05:08:59Z` | **ACTIF (réplique nocturne)** | ≤26 h |
| 2bis | **Neon** — même flux nocturne que Supabase (endpoint direct) | **ACTIF (réplique nocturne)** | ≤26 h |
| 3 | Artefact GitHub `mikcloud-dr-*` chiffré `BACKUP_KEY` (AES-256-CBC/PBKDF2, rétention 30 j) — **dump du PRIMAIRE** depuis N°275 | actif (mode=backup) | au tir |

- **Chaîne nocturne RÉ-ARMÉE (N°275)** : `mikcloud-reverse-sync.timer`
  (03:00 UTC) fait local→Supabase→Neon ; l'ancien `mikcloud-backup.timer`
  (Supabase→Neon) reste GELÉ pour toujours (superseded).
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

## 2. Les 11 modes de `ops-db-hybrid`

| Mode | Effet | Downtime | Notes |
|------|-------|----------|-------|
| `audit` | inventaire lecture seule (système, services, env keys, PG, PGDG, hosts DB, comptages Supabase) | 0 | sûr à relancer |
| `backup` | pg_dump du **PRIMAIRE local** `-Fc` → `/var/backups/mikcloud/db/primary-*.dump` + artefact chiffré `mikcloud-dr-*` | 0 | dump en 2 phases, TOC ≥30 tables |
| `setup-pg` | installe/configure PG serveur + rôle + DB + localpg.env | 0 | idempotent (localpg.env préservé) |
| `restore` | DROP/CREATE DB + pg_restore TOC-filtré + ANALYZE + asserts (≥30 tables, >0 lignes) | 0 (backend pas dessus) | verdict drift INFORMATIF ; admet primary-* et supabase-* |
| `cutover` | **bascule primaire** : delta dump/restore → garde comptages STRICT (whitelist append-only) → env backup `.pre-hybrid-*` → flip DSN → restart → preuves (journalctl + tup_written) → gel timer. Rollback AUTO si preuve absente | ~10 s (restart) | `confirm=MIK-DB-HYBRID` requis |
| `rollback` | dump local de sécurité → restore `.pre-hybrid-*` → restart → réactive timer | ~10 s | `confirm` + `accept_stale_data=YES` |
| `status` | état complet + **protection : gel auto du timer si primaire local** | 0 | sûr à relancer |
| `arm-dr` | **arme la chaîne DR** (idempotent) : archive_command gzip + reload PG, OCI CLI system-wide (pip sous sudo + test import root), `/etc/oci` (clé API coffre), bucket `mikcloud-wal` + **policy IAM service principal + lifecycle 21 j (N°276)** + **chiffrement client WAL/basebackup (N°277, WAL_ENC_KEY par stdin)**, scripts DR + 3 timers systemd | 0 | `confirm=MIK-DR-ARM` requis |
| `reverse-sync` | déclenche le flux nocturne MAINTENANT (service oneshot synchrone) : rétention web_vitals → basebackup → dump → Supabase → Neon | 0 | affiche log + statut + comptages cibles |
| `health` | lance le monitor une fois + API publique depuis le runner — **mode du cron GitHub 04:23 UTC** | 0 | sûr à relancer |
| `drill` | restaurabilité : dump primaire → restore STRICT `--exit-on-error` sur base jetable `mikcloud_drill` → asserts (40 tables, comptages identiques) → drop + **WAL distant téléchargé/DÉCHIFFRÉ/décompressé (N°277, fallback héritage `.gz`)** + listing basebackup | 0 | primaire jamais touché |

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
13. **Le dump est un ARGUMENT de pg_restore** (N°275-terdecies) : sans
    lui, pg_restore lit stdin (= /dev/null sous systemd) → « input file
    is too short (read 0, expected 5) ».
14. **TOUTE invocation `oci` : `</dev/null`** — le CLI (python) lit/
    bufférise stdin et dévore les lignes suivantes d'un script `bash -s`
    (« syntax error near unexpected token `then` », N°275-decies).
15. **`/etc/oci` en 700 root** → le user SSH deploy ne peut même pas stat
    le config (« Could not find config file ») → `oci` sous `sudo` côté
    workflow ; les scripts DR tournent en root (ok).
16. **oci-cli : pip sous SUDO + purge du symlink préalable + test décisif
    `sudo oci --version`** — un install `--user` (modules dans
    `~/.local/lib`) est invisible du root ; pip écrit À TRAVERS un
    symlink existant au lieu de le remplacer (N°275 runs 16-19).
    `python3-oci-cli` n'existe PAS dans les dépôts Ubuntu 26.04.
17. **DSN de drill** : couper la query (`?…`) puis remplacer le DERNIER
    segment de chemin — `${DSN%%/mikcloud*}` attrape le `://mikcloud:PW@`
    et fabrique un utilisateur fantôme (N°275 run 31).
18. **Fichiers téléchargés par root dans /tmp (sticky bit)** → `sudo rm`
    depuis le user deploy (N°275 run 41).
19. **Paramètre `schedule:` SOUS `on:`** — sinon 422 GitHub (N°275-bis).
20. **Script VM ≠ workflow** : les scripts DR vivent sur la VM — après
    TOUTE modification dans arm-dr, RELANCER `arm-dr` avant les modes
    qui déclenchent ces scripts (leçon run 28).
21. **PUT lifecycle → `InsufficientServicePermissions`** : le service
    principal Object Storage doit pouvoir gérer object-family — la
    policy se pose PAR API (`iam policy create`) si le user du coffre a
    `manage policies` ; recréer le bucket ne sert à RIEN (N°276).
22. **`openssl -pass env:VAR` exige une variable EXPORTÉE** — sourcer un
    fichier d'env sans `set -a` ne suffit pas (« No environment
    variable ») → `export WAL_ENC_KEY` après chaque source (scripts VM
    ET drill) ; sinon échec chiffrement → refus d'upload en clair
    (comportement voulu) mais PITR dégradé (N°277-bis).

## 4. Utilisation courante

```bash
# État (à faire après chaque geste) :
gh workflow run ops-db-hybrid.yml -f mode=status
# Backup DR chiffré DU PRIMAIRE (rétention 30 j) :
gh workflow run ops-db-hybrid.yml -f mode=backup
# Reverse-sync manuel (sinon automatique 03:00 UTC) :
gh workflow run ops-db-hybrid.yml -f mode=reverse-sync
# Santé complète (sinon automatique 04:23 UTC) :
gh workflow run ops-db-hybrid.yml -f mode=health
# Drill de restaurabilité (primaire jamais touché) :
gh workflow run ops-db-hybrid.yml -f mode=drill
# Diagnostic VM / remise en route (incident) :
gh workflow run ops-vm-diag.yml -f action=diag
gh workflow run ops-vm-diag.yml -f action=reboot -f confirm=MIK-VM-REBOOT       # soft
gh workflow run ops-vm-diag.yml -f action=reboot -f confirm=MIK-VM-REBOOT-HARD  # dur (hung ignore l'ACPI)
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

**Fichiers DR sur la VM** (N°275) : `/usr/local/sbin/mikcloud-{wal-upload,
reverse-sync,monitor}` · `/etc/mikcloud/{walupload.env,monitor.env}` ·
`/etc/oci/{config,oci_api_key.pem}` (700/600 root) · timers
`mikcloud-{wal-upload,monitor,reverse-sync}.timer` · état dans
`/var/lib/mikcloud/*.status`. PITR (objets **CHIFFRÉS `.enc` depuis N°277**,
clé = `WAL_ENC_KEY` dans `/etc/mikcloud/walupload.env`) : dernier
`basebackup/base-*.tar.gz.enc` + WAL `*.enc` du bucket →
`openssl enc -d -aes-256-cbc -pbkdf2 -iter 200000 -in X.enc -out X.gz
-pass env:WAL_ENC_KEY` (variable EXPORTÉE, piège 22) → `.gz` dégzippés →
`pg_wal` → `recovery.signal`. (héritage : les `.gz` en clair d'avant
N°277 vieillissent ≤ 21 j puis partent au lifecycle).

## 5. Réalisé en N°275/276/277 + points ouverts

### Réalisé (GO « GO N°275 », 47 runs, + N°276/277)
1. **Reverse-sync nocturne** ✓ — `mikcloud-reverse-sync.timer` 03:00 UTC,
   testé en direct : Supabase 40/40 + Neon 40/40, RLS ré-armé,
   rétention `web_vitals` 90 j + VACUUM, base backup PITR 7,4 Mo/j ;
2. **WAL → OCI Object Storage** ✓ — bucket `mikcloud-wal`, upload */5 min,
   archive_command gzip (−50× disque), prune locale 48 h, garde 3 dumps ;
3. **Monitoring Telegram** ✓ — `mikcloud-monitor.timer` */15 min, 10
   familles de contrôles (docker ajouté N°279), anti-spam 4 h, heartbeat 06:00 UTC — chat d'alerte
   **APPAIRÉ par l'exploitant** (console admin, confirmé 09/10) et relu
   par `arm-dr` #53 : chat `7026277370` posé dans
   `/etc/mikcloud/monitor.env` (règle : plus ancien compte
   `notif_settings` Telegram activé, reprise par arm-dr/reverse-sync) ;
   alertes DR délivrables (heartbeat 06:00 UTC ou premier incident).
   Les 4 comptes utilisateurs peuvent aussi activer Telegram depuis
   leur compte pour recevoir les alertes liées à leurs propres
   opérations ;
4. **Drill de restauration** ✓ — mode `drill` automatisé (base jetable +
   WAL distant), comptages identiques vérifiés ;
5. **Rétention web_vitals** ✓ — 90 jours (réglable via
   `WEB_VITALS_RETENTION_DAYS` dans `/etc/mikcloud/monitor.env`).
6. **Lifecycle 21 j du bucket** ✓ (N°276) — policy IAM
   `mikcloud-objectstorage-lifecycle` posée par API par `arm-dr`
   (`Allow service objectstorage-<region> to manage object-family in
   tenancy` — piège 21), règle `expire-21d` ACTIVE (relue depuis le
   bucket) : PITR borné 21 j, plateau ~0,3-1,2 Go (plafond gratuit
   10 Go).
7. **Chiffrement client WAL + basebackups** ✓ (N°277/277-bis) — AES-256-CBC
   PBKDF2 iter 200000 (`WAL_ENC_KEY` = `BACKUP_KEY`, transférée par
   stdin) AVANT upload → bucket 100 % `.enc` illisible sans la clé,
   refus strict d'upload en clair ; preuves : timer `new=25 fail=0`,
   base backup « CHIFFRÉ uploadé 7.3M », drill « CHIFFRÉ, déchiffré +
   décompressé 16M » ; héritage `.gz` en clair purge par lifecycle ≤ 21 j.
8. **Appairage Telegram armé** ✓ (N°278) — voir réalisé n°3 ; monitor
   assaini au même passage : `printf '%b'` (×4, warning « invalid
   format character » éradiqué) + écho arm-dr fidèle.

### DETTE TECHNIQUE (reportée à la FIN du développement produit)
- **User OCI moindre privilège** : la clé API sur la VM (`/etc/oci`) a
  `manage policies` + droits compute (héritage resize N°272). À faire :
  user dédié `mikcloud-dr` limité à `manage object-family` sur le(s)
  compartment(s) DR + rotation de clé — la VM compromise ne pourrait
  plus toucher au compute/IAM. **Reporté : fin du développement
  produit** (décision exploitant, N°277).

### Points ouverts N°277+
- **Aucun** — le dernier point (cause du « hang » du 09/10, 05:14-06:31
  UTC) est **CLÔT en N°282** : poweroff ACPI **volontaire de
  l'exploitant** (power key console 05:13:23, pendant l'installation du
  second back-end), guest éteint proprement, OCI resté RUNNING fantôme
  ~77 min jusqu'au RESET dur ; zéro perte, DR intacte. Leçon : ne pas
  éteindre le guest depuis l'intérieur (préférer `sudo reboot` ou
  Stop/Start console OCI) — cf. RUNBOOK-HEBERGEMENT §8. `ops-vm-diag`
  conserve la capture console PATIENTE (polling) pour tout futur
  incident.
