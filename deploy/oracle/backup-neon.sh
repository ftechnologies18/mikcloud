#!/usr/bin/env bash
# mikcloud — coffre-fort Neon : snapshot nocturne de la base Supabase.
# Installé par bootstrap.sh en /usr/local/sbin/mikcloud-backup, appelé par
# mikcloud-backup.timer (03:00 UTC, docs/MIGRATION-ORACLE.md §7).
#
# PIÈGE consigné : pg_dump exige le pooler SESSION de Supabase (port 5432)
# — le pooler TRANSACTION (6543, celui de l'application) ne supporte pas
# pg_dump (verrous advisory + prepared statements interdits).
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
# --clean --if-exists : le coffre-fort reflète le DERNIER état connu
# (remplace les tables existantes) ; ON_ERROR_STOP : échec net plutôt
# qu'une sauvegarde silencieusement partielle.
if pg_dump --no-owner --no-privileges --clean --if-exists "$DATABASE_URL_SESSION" \
	| psql "$NEON_DATABASE_URL" -v ON_ERROR_STOP=1 > /tmp/mikcloud-backup.out 2> /tmp/mikcloud-backup.err; then
	echo "mikcloud-backup[$STAMP]: succès"
else
	echo "mikcloud-backup[$STAMP]: ÉCHEC — $(tail -n 3 /tmp/mikcloud-backup.err 2>/dev/null | tr '\n' ' ')"
	exit 1
fi
