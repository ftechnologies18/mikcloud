#!/usr/bin/env bash
# mikcloud — coffre-fort Neon : snapshot nocturne de la base Supabase.
# Installé par bootstrap.sh en /usr/local/sbin/mikcloud-backup, appelé par
# mikcloud-backup.timer (03:00 UTC, docs/MIGRATION-ORACLE.md §7).
#
# PIÈGE consigné : pg_dump exige le pooler SESSION de Supabase (port 5432)
# — le pooler TRANSACTION (6543, celui de l'application) ne supporte pas
# pg_dump (verrous advisory + prepared statements interdits).
# PIÈGE consigné (N°243) : pg_dump du pont doit être ≥ la version du serveur
# Supabase (17.6) → postgresql-client-17 du dépôt PGDG (celui d'Ubuntu 24.04
# est en 16.x et refuse : « server version mismatch »).
# PIÈGE consigné (N°243-c) : le dump Supabase contenait des lignes EXTENSION
# supabase_vault, refusées par Neon (« not in the allowed extensions list »)
# → le flux est filtré (sed) sur TOUTE ligne EXTENSION supabase_vault.
# Comparatif mesuré : Supabase installe {pg_stat_statements, pgcrypto,
# plpgsql, supabase_vault, uuid-ossp} — Neon propose les 4 autres ; le
# coffre-fort ne perd RIEN d'applicatif (le vault Supabase stocke des
# secrets d'infra, aucun objet MikCloud n'y vit).
# PIÈGE consigné (N°243-d, run 3 — 04/10 21:12 UTC) : le dump BASE ENTIÈRE
# embarque les schémas de la PLATEFORME Supabase (auth, storage, realtime,
# vault, graphql, graphql_public, neon_auth, pgbouncer, extensions) ; la
# fonction realtime.list_changes porte une clause « SET log_min_messages
# TO 'fatal' » (paramètre SUSET, réservé aux superutilisateurs) → Neon
# refuse au rôle non-superuser neondb_owner : « permission denied to set
# parameter "log_min_messages" » et ON_ERROR_STOP arrête tout. Correctif
# structurel : --schema=public ci-dessous — le coffre-fort ne snapshot QUE
# les données applicatives MikCloud, seules restaurables hors Supabase.
# Vérifié sur dump réel : public est auto-suffisant (gen_random_uuid()
# natif PG13+, zéro référence croisée vers auth/storage/…, zéro clause
# SUSET restante, zéro ligne EXTENSION — le filtre supabase_vault est
# conservé en défense en profondeur, il ne matche plus rien).
# PIÈGE consigné (N°243-d, tests 04/10 21:30-22:00 UTC) : le pipeline direct
# `pg_dump | psql` est FAILLIBLE — 4 échecs / 4 à ~61 s, toujours au milieu
# de la COPY de la première grosse table (commands : « SSL SYSCALL error:
# EOF detected », ~3,8 Mo passés sur 7,6, connexion source coupée).
# Réfutations par tests ciblés : ni l'inactivité simple (stall 75 s : OK),
# ni le consommateur lent (drain 64 Ko/s pendant 139 s : OK), ni la double
# connexion longue (Neon actif + dump throttlé, 141 s : OK), ni le dump
# fichier (4/4 OK, même avec un psql Neon actif en parallèle) → le tueur
# est le COUPLAGE PIPE entre les deux bases. Découplage en deux phases :
# 1) dump vers fichier temporaire (la connexion source n'est jamais mise
# en attente par un consommateur), 2) restore DEPUIS le fichier (le filtre
# sed s'applique alors sur fichier). Reprise jusqu'à 3 tentatives espacées
# de 30 s pour les aléas résiduels ; chaque tentative repart de zéro
# (--clean --if-exists = idempotent) ; pipefail et [ -s ] garantissent
# qu'un dump tronqué ou vide n'est JAMAIS un succès.
# Connexions dans /etc/mikcloud/backup.env :
#   DATABASE_URL_SESSION=postgresql://...pooler.supabase.com:5432/postgres
#   NEON_DATABASE_URL=postgresql://...neon.tech/neondb?sslmode=require
#           (endpoint DIRECT, non-poolé, pour la restauration)
set -euo pipefail

ENV_FILE=/etc/mikcloud/backup.env
if [ ! -r "$ENV_FILE" ]; then
        echo "mikcloud-backup: $ENV_FILE absent — coffre-fort non configuré, rien à faire."
        exit 0
fi
set -a
. "$ENV_FILE"
set +a
: "${DATABASE_URL_SESSION:?DATABASE_URL_SESSION requis dans backup.env}"
: "${NEON_DATABASE_URL:?NEON_DATABASE_URL requis dans backup.env}"

STAMP=$(date -u +%Y-%m-%dT%H%M%SZ)
echo "mikcloud-backup[$STAMP]: départ du snapshot Supabase -> Neon"
# --schema=public : coffre-fort = données applicatives MikCloud uniquement
# (PIÈGE N°243-d ci-dessus — les schémas plateforme Supabase ne sont pas
# restaurables hors Supabase et bloquent le restore).
# --clean --if-exists : le coffre-fort reflète le DERNIER état connu
# (remplace les tables existantes) ; ON_ERROR_STOP : échec net plutôt
# qu'une sauvegarde silencieusement partielle.
DUMP_FILE=$(mktemp /tmp/mikcloud-backup.XXXXXX.sql)
trap 'rm -f "$DUMP_FILE"' EXIT
MAX_ATTEMPTS=3
ATTEMPT=1
while :; do
        echo "mikcloud-backup[$STAMP]: tentative $ATTEMPT/$MAX_ATTEMPTS — dump Supabase (schéma public)…"
        if pg_dump --schema=public --no-owner --no-privileges --clean --if-exists \
                        "$DATABASE_URL_SESSION" > "$DUMP_FILE" 2> /tmp/mikcloud-backup.err \
                && [ -s "$DUMP_FILE" ]; then
                DUMP_BYTES=$(wc -c < "$DUMP_FILE")
                echo "mikcloud-backup[$STAMP]: dump OK ($DUMP_BYTES octets) — restore vers Neon…"
                if sed -E '/(CREATE|DROP|COMMENT ON|ALTER) EXTENSION (IF (NOT )?EXISTS )?"?supabase_vault"?/d' "$DUMP_FILE" \
                        | psql "$NEON_DATABASE_URL" -v ON_ERROR_STOP=1 \
                                > /tmp/mikcloud-backup.out 2>> /tmp/mikcloud-backup.err; then
                        echo "mikcloud-backup[$STAMP]: succès (tentative $ATTEMPT, dump $DUMP_BYTES octets)"
                        exit 0
                fi
        fi
        if [ "$ATTEMPT" -ge "$MAX_ATTEMPTS" ]; then
                echo "mikcloud-backup[$STAMP]: ÉCHEC après $ATTEMPT tentatives — $(tail -n 3 /tmp/mikcloud-backup.err 2>/dev/null | tr '\n' ' ')"
                exit 1
        fi
        echo "mikcloud-backup[$STAMP]: tentative $ATTEMPT échouée — relance dans 30 s"
        ATTEMPT=$((ATTEMPT + 1))
        sleep 30
done
