#!/usr/bin/env bash
# MIKCLOUD — Étape 8 du 1er octobre (RUNBOOK-POSTGRES.md §11) : armer le
# secours quotidien et l'archive froide, APRÈS la fusion (étape 6) et le
# flip du secret DATABASE_URL (étape 8a = step8-flip-secret.py).
#
#   1. RÉ-ACTIVER le workflow backup (désactivé le 21/09 N°167 — sinon son
#      cron du dimanche 03:17 UTC exporterait vers un Neon quota-bloqué) ;
#   2. déclencher UNE FOIS backup (validation du premier export chiffré
#      réel vers les artefacts mikcloud-backup) ;
#   3. déclencher UNE FOIS standby-restore (validation du premier restore
#      Supabase → Neon ; le cron quotidien 02:43 UTC prend le relais).
#
# Usage :
#   ops/oct1/step8-arm-standby.sh            # DRY-RUN
#   ops/oct1/step8-arm-standby.sh --exec     # APPLIQUE
#
# Pré-requis : fusion effectuée (standby-restore.yml vit sur main depuis
# l'étape 6) ; secret DATABASE_URL déjà retourné vers Supabase (8a).
# Secret (env > coffre) : GITHUB_TOKEN (PAT droits repo).
set -euo pipefail

REPO="ftechnologies18/mikcloud"
COFFRE="${MIKCLOUD_VAULT:-/home/z/.secrets}"
EXEC=0
[ "${1:-}" = "--exec" ] && EXEC=1

say() { printf '%s\n' "$*"; }
die() { printf '✗ %s\n' "$*" >&2; exit 1; }

GITHUB_TOKEN="${GITHUB_TOKEN:-}"
if [ -z "$GITHUB_TOKEN" ] && [ -f "$COFFRE/github-token.txt" ]; then
  GITHUB_TOKEN="$(grep -E '^(ghp_|github_pat_)' "$COFFRE/github-token.txt" | head -1)"
fi
[ -n "$GITHUB_TOKEN" ] || die "GITHUB_TOKEN absent (env ou $COFFRE/github-token.txt)"

GHAPI="https://api.github.com/repos/$REPO"

say "═══ Étape 8b — armement secours + archive froide (runbook §11) ═══"
[ "$EXEC" -eq 0 ] && say "MODE DRY-RUN (aucune écriture — ajouter --exec pour appliquer)"
say ""

# Garde : le secret DATABASE_URL doit DÉJÀ pointer Supabase (étape 8a).
# Indirect mais sûr : on exige la confirmation explicite de l'opérateur.
say "AVANT de continuer : step8-flip-secret.py --exec doit avoir été exécuté"
say "(secret DATABASE_URL = Supabase). Sinon backup exporterait l'ANCIENNE base."
if [ "$EXEC" -eq 0 ]; then
  say ""
  say "DRY-RUN : le plan serait —"
fi
say "  1. PUT  /actions/workflows/backup.yml/enable          (ré-active l'archive)"
say "  2. POST /actions/workflows/backup.yml/dispatches       (premier export réel)"
say "  3. POST /actions/workflows/standby-restore.yml/dispatches (premier restore)"
say ""

if [ "$EXEC" -eq 0 ]; then
  say "DRY-RUN terminé — rien n'a été modifié."
  exit 0
fi

# ── 1. Ré-activer backup.yml ───────────────────────────────────────────────
say "→ Ré-activation du workflow backup…"
CODE="$(curl -s -o /dev/null -w '%{http_code}' --max-time 20 -X PUT \
  -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/vnd.github+json" \
  "$GHAPI/actions/workflows/backup.yml/enable")"
[ "$CODE" = "204" ] || die "activation backup.yml : HTTP $CODE"
say "   ✓ backup.yml ré-activé"

# ── 2. Premier export (validation) ─────────────────────────────────────────
say "→ Dispatch backup (premier export chiffré réel)…"
CODE="$(curl -s -o /dev/null -w '%{http_code}' --max-time 20 -X POST \
  -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/vnd.github+json" \
  -H "Content-Type: application/json" \
  -d '{"ref":"main"}' \
  "$GHAPI/actions/workflows/backup.yml/dispatches")"
[ "$CODE" = "204" ] || die "dispatch backup.yml : HTTP $CODE"
say "   ✓ dispatché — suivre le run dans l'onglet Actions (export + vérification"
say "     de réinsertion intégrale, cf. backup.yml)"

# ── 3. Premier restore (validation) ────────────────────────────────────────
say "→ Dispatch standby-restore (premier restore Supabase → Neon)…"
CODE="$(curl -s -o /dev/null -w '%{http_code}' --max-time 20 -X POST \
  -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/vnd.github+json" \
  -H "Content-Type: application/json" \
  -d '{"ref":"main"}' \
  "$GHAPI/actions/workflows/standby-restore.yml/dispatches")"
[ "$CODE" = "204" ] || die "dispatch standby-restore.yml : HTTP $CODE (la fusion a-t-elle eu lieu ?)"
say "   ✓ dispatché — suivre le run (dump Supabase → comptages → restore Neon)"

say ""
say "═══ Étape 8b TERMINÉE ═══"
say "Le modèle de cohabitation est ARMÉ (§10) : Supabase production,"
say "Neon secours quotidien (cron 02:43 UTC), archive chiffrée hebdo"
say "(dimanche 03:17 UTC, artefacts mikcloud-backup, 90 j de rétention)."
say "Surveillance premier mois : §8 (taille, egress, carte Santé)."
