#!/usr/bin/env bash
# MIKCLOUD — Étape 5 du 1er octobre (RUNBOOK-POSTGRES.md §11) :
# bascule de l'ENVIRONNEMENT Render vers Supabase, AVANT la fusion.
#
#   1. DATABASE_URL   ← DSN Supabase SESSION pooler :5432 (SANS paramètre
#      sslmode : l'app ajoute verify-full elle-même, pg.go N°75, et la
#      racine TLS privée Supabase est dans l'image depuis N°166) ;
#   2. NEON_KEEPALIVE ← off (le keep-alive n'a plus d'objet sur Supabase —
#      et il réveillerait le compute Neon de secours en continu) ;
#   3. autoDeploy     ← yes (ré-arme le déploiement automatique, §3).
#
# N°172 — MÉTHODE CORRIGÉE : le point d'entrée env-vars de l'API Render
#   n'accepte QUE GET et PUT (mesuré au premier --exec réel, 21/09 20:08Z :
#   PATCH → 405, Allow: GET/PUT) — le PUT envoie la liste COMPLÈTE des
#   variables (lues à l'instant : les autres sont re-émises à l'identique,
#   DATABASE_URL remplacée, NEON_KEEPALIVE ajoutée si absente). La
#   vérification post-bascule contrôle aussi qu'AUCUNE variable n'a été
#   perdue. PATCH /v1/services/{id} (autoDeploy) reste valable (200 mesuré).
#
# N°172 — MÉTHODE CORRIGÉE : le point d'entrée env-vars de l'API Render
#   n'accepte QUE GET et PUT (mesuré au premier --exec réel, 21/09 20:08Z :
#   PATCH → 405, Allow: GET/PUT) — le PUT envoie la liste COMPLÈTE des
#   variables (lues à l'instant : les autres sont re-émises à l'identique,
#   DATABASE_URL remplacée, NEON_KEEPALIVE ajoutée si absente). La
#   vérification post-bascule contrôle aussi qu'AUCUNE variable n'a été
#   perdue. PATCH /v1/services/{id} (autoDeploy) reste valable (200 mesuré).
#
# N°172 — MÉTHODE CORRIGÉE : le point d'entrée env-vars de l'API Render
#   n'accepte QUE GET et PUT (mesuré au premier --exec réel, 21/09 20:08Z :
#   PATCH → 405, Allow: GET, PUT) — le PUT envoie la liste COMPLÈTE des
#   variables (lues à l'instant : les autres sont re-émises à l'identique,
#   DATABASE_URL remplacée, NEON_KEEPALIVE ajoutée si absente). La
#   vérification post-bascule contrôle aussi qu'AUCUNE variable n'a été
#   perdue. PATCH /v1/services/{id} (autoDeploy) reste valable (200 mesuré).
#
# SANS EFFET IMMÉDIAT : les variables Render s'appliquent au prochain
# démarrage du service. Le process tourne toujours sur son état mémoire ;
# le redémarrage unique vient avec la fusion n163+n164 (étape 6).
#
# Usage :
#   ops/oct1/step5-render-flip.sh            # DRY-RUN : montre, ne touche rien
#   ops/oct1/step5-render-flip.sh --exec     # APPLIQUE (API Render en écriture)
#
# Secrets (priorité : variable d'environnement > coffre /home/z/.secrets) :
#   RENDER_API_KEY          clé API Render (rnd_…)
#   SUPABASE_DATABASE_URL   DSN session pooler :5432, SANS sslmode/pgbouncer
#   (coffre : render-api-key.txt ; supabase-dsn.txt → ligne :5432)
#
# Pré-requis documentés §11 : migration exécutée (étape 4) AVANT ce script.
# Rollback : le snapshot des variables d'avant-bascule est écrit dans le
# coffre (render-env-before-flip-<date>.json) — rePATCHer DATABASE_URL avec
# l'ancienne valeur + autoDeploy=no suffit à revenir en arrière.
set -euo pipefail

SERVICE_ID="srv-da974o142hec73euul60"
API="https://api.render.com/v1"
COFFRE="${MIKCLOUD_VAULT:-/home/z/.secrets}"
EXEC=0
[ "${1:-}" = "--exec" ] && EXEC=1

say()  { printf '%s\n' "$*"; }
die()  { printf '✗ %s\n' "$*" >&2; exit 1; }

# ── Résolution des secrets : env > coffre ─────────────────────────────────
RENDER_API_KEY="${RENDER_API_KEY:-}"
if [ -z "$RENDER_API_KEY" ] && [ -f "$COFFRE/render-api-key.txt" ]; then
  RENDER_API_KEY="$(grep -E '^(rnd_|rctl_)' "$COFFRE/render-api-key.txt" | head -1)"
fi
[ -n "$RENDER_API_KEY" ] || die "RENDER_API_KEY absent (env ou $COFFRE/render-api-key.txt)"

SUPABASE_DATABASE_URL="${SUPABASE_DATABASE_URL:-}"
if [ -z "$SUPABASE_DATABASE_URL" ] && [ -f "$COFFRE/supabase-dsn.txt" ]; then
  # Ligne DSN du session pooler :5432 (celle qui porte « :5432 »)
  SUPABASE_DATABASE_URL="$(grep -E '^postgresql://.*:5432/' "$COFFRE/supabase-dsn.txt" | head -1)"
fi
[ -n "$SUPABASE_DATABASE_URL" ] || die "SUPABASE_DATABASE_URL absent (env ou $COFFRE/supabase-dsn.txt, ligne :5432)"

# ── Validations DURES du DSN (les pièges documentés §10/§12) ──────────────
case "$SUPABASE_DATABASE_URL" in
  postgresql://*) : ;;
  *) die "Le DSN doit commencer par postgresql://" ;;
esac
echo "$SUPABASE_DATABASE_URL" | grep -q '\.pooler\.supabase\.com:5432/' \
  || die "Le DSN doit pointer le SESSION pooler (.pooler.supabase.com:5432) — refusé"
echo "$SUPABASE_DATABASE_URL" | grep -q 'sslmode=' \
  && die "Le DSN ne doit PAS contenir sslmode= (l'app ajoute verify-full elle-même)"
echo "$SUPABASE_DATABASE_URL" | grep -qE '(:6543|pgbouncer=true)' \
  && die "Le DSN ressemble au pooler TRANSACTIONNEL :6543 — cassé pour pgx (42P05), refusé"
echo "$SUPABASE_DATABASE_URL" | grep -q 'channel_binding=' \
  && die "channel_binding= inutile ici — retirer le paramètre"

# ── État AVANT (lecture seule) ─────────────────────────────────────────────
say "═══ Étape 5 — bascule env Render → Supabase (runbook §11) ═══"
[ "$EXEC" -eq 0 ] && say "MODE DRY-RUN (aucune écriture — ajouter --exec pour appliquer)"
say ""

say "→ Lecture de l'état courant du service…"
SERVICE_JSON="$(curl -fsS --max-time 20 -H "Authorization: Bearer $RENDER_API_KEY" "$API/services/$SERVICE_ID")" \
  || die "API Render injoignable / clé refusée"
AUTO="$(printf '%s' "$SERVICE_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["autoDeploy"])')"
say "   autoDeploy actuel : $AUTO"

ENV_JSON="$(curl -fsS --max-time 20 -H "Authorization: Bearer $RENDER_API_KEY" "$API/services/$SERVICE_ID/env-vars")" \
  || die "Lecture des variables impossible"
CURRENT_DB="$(printf '%s' "$ENV_JSON" | python3 -c '
import json,sys
for e in json.load(sys.stdin):
    if e["envVar"]["key"] == "DATABASE_URL":
        print(e["envVar"].get("value") or ""); break')"

# Hôte de l'actuel (affiché SANS mot de passe)
if [ -n "$CURRENT_DB" ]; then
  CURRENT_HOST="$(printf '%s' "$CURRENT_DB" | sed -E 's|postgresql://[^@]+@([^/?:]+).*|\1|')"
  say "   DATABASE_URL actuel : host = $CURRENT_HOST"
  case "$CURRENT_DB" in
    *pooler.supabase.com*) say "   ⚠ DATABASE_URL pointe DÉJÀ Supabase — bascule déjà faite ?" ;;
  esac
else
  say "   ⚠ aucune variable DATABASE_URL posée sur le service ?!"
fi

# Snapshot AVANT (matière à rollback) — écrit dans le coffre, hors dépôt
if [ "$EXEC" -eq 1 ] && [ -n "$CURRENT_DB" ]; then
  STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
  SNAP="$COFFRE/render-env-before-flip-$STAMP.json"
  printf '%s\n' "$ENV_JSON" > "$SNAP" && chmod 600 "$SNAP"
  say "   snapshot avant-bascule : $SNAP"
fi
say ""

# ── Plan ───────────────────────────────────────────────────────────────────
say "Plan d'exécution :"
say "  1. PUT  /services/$SERVICE_ID/env-vars   (liste COMPLÈTE — N°172)"
say "       DATABASE_URL   = postgresql://…@<session pooler :5432 masqué>…"
say "       NEON_KEEPALIVE = off"
say "       + les autres variables lues ci-dessus, re-émises à l'identique"
say "  2. PATCH /services/$SERVICE_ID   autoDeploy=yes"
say ""
say "Rappel : AUCUN effet sur le process en cours — les variables ne"
say "s'appliquent qu'au prochain démarrage (la fusion de l'étape 6)."
say ""

if [ "$EXEC" -eq 0 ]; then
  say "DRY-RUN terminé — rien n'a été modifié. Relancer avec --exec le moment venu."
  exit 0
fi

# ── Exécution ──────────────────────────────────────────────────────────────
say "→ Construction de la liste complète (état lu + bascule DATABASE_URL + NEON_KEEPALIVE)…"
PUT_PAYLOAD="$(python3 -c '
import json, sys
envs = json.loads(sys.argv[1])
supabase_dsn = sys.argv[2]
lst = []
seen_ka = False
for e in envs:
    k = e["envVar"]["key"]
    v = e["envVar"].get("value") or ""
    if k == "DATABASE_URL":
        v = supabase_dsn
    elif k == "NEON_KEEPALIVE":
        v = "off"
        seen_ka = True
    lst.append({"key": k, "value": v})
if not seen_ka:
    lst.append({"key": "NEON_KEEPALIVE", "value": "off"})
print(json.dumps(lst))' "$ENV_JSON" "$SUPABASE_DATABASE_URL")"
NPUT="$(printf '%s' "$PUT_PAYLOAD" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
say "   $NPUT variable(s) dans le PUT — aucune perte : état complet + NEON_KEEPALIVE"
say "→ PUT des variables d'environnement…"
RESP="$(curl -fsS --max-time 30 -X PUT \
  -H "Authorization: Bearer $RENDER_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d "$PUT_PAYLOAD" \
  "$API/services/$SERVICE_ID/env-vars")" \
  || die "ÉCHEC du PUT env-vars — vérifier la réponse Render ci-dessus"
say "   ✓ variables posées (DATABASE_URL → Supabase, NEON_KEEPALIVE=off, $((NPUT-2)) conservées)"

say "→ PATCH autoDeploy=yes…"
curl -fsS --max-time 30 -X PATCH \
  -H "Authorization: Bearer $RENDER_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{"autoDeploy":"yes"}' \
  "$API/services/$SERVICE_ID" > /dev/null \
  || die "ÉCHEC du PATCH autoDeploy"
say "   ✓ autoDeploy ré-activé"
say ""

# ── Vérification post-bascule ──────────────────────────────────────────────
say "→ Vérification (relecture)…"
sleep 2
ENV_AFTER="$(curl -fsS --max-time 20 -H "Authorization: Bearer $RENDER_API_KEY" "$API/services/$SERVICE_ID/env-vars")"
CHECK="$(printf '%s' "$ENV_AFTER" | python3 -c '
import json, sys
vars = {e["envVar"]["key"]: (e["envVar"].get("value") or "") for e in json.load(sys.stdin)}
db, ka = vars.get("DATABASE_URL",""), vars.get("NEON_KEEPALIVE","")
print("OK" if (".pooler.supabase.com:5432/" in db and ka == "off") else "KO")
print("host=" + (db.split("@")[-1] if "@" in db else "?"))')"
VERDICT="$(printf '%s' "$CHECK" | head -1)"
[ "$VERDICT" = "OK" ] || die "La relecture ne confirme PAS la bascule — inspecter manuellement"
say "   ✓ DATABASE_URL → $(printf '%s' "$CHECK" | sed -n '2p')"
say "   ✓ NEON_KEEPALIVE=off"
# N°172 — garde anti-perte : toutes les clés de l'état AVANT doivent survivre
LOST="$(python3 -c '
import json, sys
before = {e["envVar"]["key"] for e in json.loads(sys.argv[1])}
after = {e["envVar"]["key"] for e in json.loads(sys.argv[2])}
print(" ".join(sorted(before - after)))' "$ENV_JSON" "$ENV_AFTER")"
[ -z "$LOST" ] || die "VARIABLES PERDUES au PUT : $LOST — restaurer depuis le snapshot du coffre"
say "   ✓ aucune variable perdue"

AUTO_AFTER="$(curl -fsS --max-time 20 -H "Authorization: Bearer $RENDER_API_KEY" "$API/services/$SERVICE_ID" | python3 -c 'import json,sys; print(json.load(sys.stdin)["autoDeploy"])')"
[ "$AUTO_AFTER" = "yes" ] || die "autoDeploy n'est pas passé à yes"
say "   ✓ autoDeploy=yes"
say ""
say "═══ Étape 5 TERMINÉE ═══"
say "Étape suivante (§11-6) : fusionner n163-zikisso-repair puis"
say "n164-persistence-safety dans main, retirer le sentinel"
say "RENDER-DEPLOY-FROZEN DANS le commit de fusion, pousser — CI verte →"
say "l'UNIQUE redémarrage s'exécute sur la base Supabase migrée."
