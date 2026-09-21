#!/usr/bin/env bash
# MIKCLOUD — Pré-vol du 1er octobre (RUNBOOK-POSTGRES.md §11, étapes 1 à 3).
# LECTURE SEULE : aucune écriture, aucun déploiement, aucun secret modifié.
#
# Vérifie, dans l'ordre :
#   0. horloge (le reset quota Neon est attendu à 2026-10-01T00:00:00Z) ;
#   1. API Neon : période de consommation, CU consommés, état du compute ;
#   2. SQL Neon : le quota 53000 est-il levé ? (GO/NO-GO de l'étape 1) ;
#   3. SQL Supabase : joignable + état du schéma public (0 = livraison,
#      ~35 tables = déjà migrée) ;
#   4. API Render : service vivant, autoDeploy, hôte DATABASE_URL ;
#   5. GitHub : secrets attendus, états des workflows, sentinel, dernière CI ;
#   6. production : HTTP backend + frontend ;
#   7. (optionnel, ADMIN_PASSWORD au coffre) carte Santé via l'API admin :
#      mode, compteurs de synchro (rattrapage de l'étape 2), contact PG.
#
# Usage : ops/oct1/preflight.sh
# Secrets (env > coffre /home/z/.secrets) : NEON_API_KEY, GITHUB_TOKEN,
#   RENDER_API_KEY, SUPABASE_DATABASE_URL, NEON_DATABASE_URL, ADMIN_PASSWORD.
# Dépendances : curl, python3 ; psycopg pour les checks SQL (auto-install
#   via uv si absent, sinon instruction affichée).
#
# NOTE d'implémentation : aucune fonction ne porte le nom d'une commande
# standard (leçon du 21/09 : une fonction head() éclipse /usr/bin/head
# dans les substitutions et corrompt silencieusement les valeurs lues).
set -uo pipefail   # pas de -e : on veut le verdict COMPLET, check par check

COFFRE="${MIKCLOUD_VAULT:-/home/z/.secrets}"
REPO="ftechnologies18/mikcloud"
SERVICE_ID="srv-da974o142hec73euul60"
NEON_ORG="org-blue-forest-04016555"
NEON_PROJECT="long-feather-75906741"
BACKEND_URL="https://mikcloud.onrender.com"
FRONTEND_URL="https://mikcloud.ftci.fr"

OK=0; WARN=0; FAIL=0
ok()   { OK=$((OK+1));   printf '  ✓ %s\n' "$*"; }
warn() { WARN=$((WARN+1)); printf '  ⚠ %s\n' "$*"; }
fail() { FAIL=$((FAIL+1)); printf '  ✗ %s\n' "$*"; }
section() { printf '\n── %s ──\n' "$*"; }
say() { printf '%s\n' "$*"; }

# ── Secrets : env > coffre ────────────────────────────────────────────────
pick() { # pick NOM_VAR fichier regex
  local v="${!1:-}"
  if [ -z "$v" ] && [ -f "$COFFRE/$2" ]; then
    v="$(grep -E "$3" "$COFFRE/$2" | /usr/bin/head -1)"
  fi
  printf '%s' "$v"
}
NEON_API_KEY="$(pick NEON_API_KEY neon-credentials.txt '^napi_')"
GITHUB_TOKEN="$(pick GITHUB_TOKEN github-token.txt '^ghp_|^github_pat_')"
RENDER_API_KEY="$(pick RENDER_API_KEY render-api-key.txt '^rnd_|^rctl_')"
SUPABASE_DATABASE_URL="$(pick SUPABASE_DATABASE_URL supabase-dsn.txt '^postgresql://.*:5432/')"
NEON_DATABASE_URL="$(pick NEON_DATABASE_URL neon-credentials.txt '^postgresql://.*neon\.tech')"
ADMIN_PASSWORD="$(pick ADMIN_PASSWORD admin-credentials.txt '^ADMIN_PASSWORD=')"
ADMIN_PASSWORD="${ADMIN_PASSWORD#ADMIN_PASSWORD=}"

say "════════════════════════════════════════════════════════════════"
say " MIKCLOUD — pré-vol du 1er octobre (runbook §11 étapes 1-3)"
say " $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
say "════════════════════════════════════════════════════════════════"

# ── 0. Horloge ─────────────────────────────────────────────────────────────
section "0. Horloge"
NOW_EPOCH="$(date -u +%s)"
RESET_EPOCH="$(date -u -d '2026-10-01T00:00:00Z' +%s 2>/dev/null || echo 0)"
if [ "$RESET_EPOCH" -gt 0 ]; then
  DIFF=$(( NOW_EPOCH - RESET_EPOCH ))
  if [ "$DIFF" -ge 0 ]; then
    ok "reset quota Neon PASSÉ depuis $(( DIFF / 3600 )) h $(( (DIFF % 3600) / 60 )) min"
  else
    warn "reset quota Neon dans encore $(( (-DIFF) / 3600 )) h $(( (-DIFF % 3600) / 60 )) min — NE RIEN BASCULER"
  fi
else
  warn "impossible de calculer l'échéance du reset"
fi

# ── 1. API Neon ────────────────────────────────────────────────────────────
section "1. API Neon (console.neon.tech/api/v2)"
if [ -n "$NEON_API_KEY" ]; then
  PROJ="$(curl -fsS --max-time 20 -H "Authorization: Bearer $NEON_API_KEY" \
    "https://console.neon.tech/api/v2/projects/$NEON_PROJECT" 2>/dev/null)" \
    && ok "projet joignable" || { fail "API Neon injoignable (clé napi_ ou DNS ?)"; PROJ=""; }
  if [ -n "$PROJ" ]; then
    printf '%s' "$PROJ" | python3 -c '
import json, sys
p = json.load(sys.stdin)
p = p.get("project", p)
print("  · période : → " + str(p.get("consumption_period_end", "?")))
cpu = p.get("cpu_used_sec", 0)
print("  · CU consommés : %.2f CU-h (%d s)" % (cpu / 3600, cpu))' || true
    case "$PROJ" in
      *'"consumption_period_end":"2026-11-01'*) ok "NOUVELLE période ouverte (reset effectif)" ;;
      *'"consumption_period_end":"2026-10-01'*) warn "période SEPTEMBRE encore affichée (reset pas encore vu par l'API)" ;;
    esac
  fi
  EP="$(curl -fsS --max-time 20 -H "Authorization: Bearer $NEON_API_KEY" \
    "https://console.neon.tech/api/v2/projects/$NEON_PROJECT/endpoints" 2>/dev/null)" || EP=""
  if [ -n "$EP" ]; then
    STATE="$(printf '%s' "$EP" | python3 -c 'import json,sys; print(json.load(sys.stdin)["endpoints"][0].get("current_state","?"))')"
    case "$STATE" in
      idle) warn "compute Neon : idle (normal — le syncreur Render le réveillera)" ;;
      running) ok "compute Neon : running" ;;
      *) warn "compute Neon : $STATE" ;;
    esac
  fi
else
  warn "NEON_API_KEY absent — check API sauté"
fi

# ── Client SQL (psql > psycopg) ────────────────────────────────────────────
# IPv4 forcé : le sandbox n'a pas d'IPv6 (mesuré le 21/09 — le DNS du pooler
# expose 3×AAAA que libpq/psycopg poursuivent en vain avant l'IPv4).
sql_one() { # sql_one DSN — imprime le résultat de SELECT 1 sur stdout ;
            # le message d'erreur éventuel va sur stderr (capturé par l'appelant)
  local dsn="$1"
  if command -v psql >/dev/null 2>&1; then
    psql -t -A --quiet --set=ON_ERROR_STOP=1 "$dsn" -c 'SELECT 1' 2>&1
  else
    python3 - "$dsn" <<'PYEOF'
import sys
try:
    import psycopg
except ImportError:
    sys.exit(3)
from urllib.parse import urlparse
import socket
dsn = sys.argv[1]
u = urlparse(dsn)
host, port = u.hostname, u.port or 5432
try:
    ip = sorted({a[4][0] for a in socket.getaddrinfo(host, port, socket.AF_INET, socket.SOCK_STREAM)})[0]
except Exception:
    sys.exit(2)
conninfo = (f"host={host} hostaddr={ip} port={port} dbname={u.path.lstrip('/')} "
            f"user={u.username} password={u.password} sslmode=require connect_timeout=10")
try:
    with psycopg.connect(conninfo) as c:
        print(c.execute("SELECT 1").fetchone()[0])
except Exception as e:
    print(str(e).strip()[:400], file=sys.stderr)
    sys.exit(1)
PYEOF
  fi
}
sql_count_tables() { # sql_count_tables DSN — compte les tables du schéma public
  python3 - "$1" <<'PYEOF'
import sys
from urllib.parse import urlparse
import socket
import psycopg
dsn = sys.argv[1]; u = urlparse(dsn)
ip = sorted({a[4][0] for a in socket.getaddrinfo(u.hostname, u.port or 5432, socket.AF_INET, socket.SOCK_STREAM)})[0]
conninfo = (f"host={u.hostname} hostaddr={ip} port={u.port} dbname={u.path.lstrip('/')} "
            f"user={u.username} password={u.password} sslmode=require connect_timeout=10")
with psycopg.connect(conninfo) as c:
    print(c.execute("SELECT count(*) FROM information_schema.tables WHERE table_schema='public'").fetchone()[0])
PYEOF
}

python3 -c "import psycopg" >/dev/null 2>&1 || {
  command -v uv >/dev/null 2>&1 \
    && uv pip install --quiet "psycopg[binary]" >/dev/null 2>&1 || true
}

# ── 2. SQL Neon (GO/NO-GO étape 1) ─────────────────────────────────────────
section "2. SQL Neon (quota 53000 levé ?)"
if [ -n "$NEON_DATABASE_URL" ]; then
  OUT="$(sql_one "$NEON_DATABASE_URL" 2>&1)"; RC=$?
  if [ $RC -eq 0 ] && [ "$OUT" = "1" ]; then
    ok "connexion SQL Neon OK — QUOTA LEVÉ : l'étape 1 du §11 est satisfaite"
  elif echo "$OUT" | grep -qi "exceeded the quota"; then
    fail "erreur 53000 : quota TOUJOURS ACTIF — attendre le reset, NE RIEN BASCULER"
  elif [ $RC -eq 3 ]; then
    warn "aucun client SQL (psql/psycopg) — installer : uv pip install \"psycopg[binary]\""
  else
    warn "connexion Neon échouée (${OUT:-réseau/DNS}) — réessayer (cold start)"
  fi
else
  warn "NEON_DATABASE_URL absent — check SQL sauté"
fi

# ── 3. SQL Supabase ────────────────────────────────────────────────────────
section "3. SQL Supabase (cible de migration)"
if [ -n "$SUPABASE_DATABASE_URL" ]; then
  OUT="$(sql_one "$SUPABASE_DATABASE_URL" 2>&1)"; RC=$?
  if [ $RC -eq 0 ] && [ "$OUT" = "1" ]; then
    ok "connexion SQL Supabase OK (session pooler :5432)"
    NTABLES="$(sql_count_tables "$SUPABASE_DATABASE_URL" 2>/dev/null || true)"
    if [ -n "$NTABLES" ]; then
      if [ "$NTABLES" = "0" ]; then
        ok "schéma public vide — état de LIVRAISON (migration à faire, étape 4)"
      else
        warn "schéma public : $NTABLES table(s) — migration déjà passée ? (étape 4 déjà exécutée)"
      fi
    fi
  elif [ $RC -eq 3 ]; then
    warn "aucun client SQL — installer psycopg"
  else
    fail "connexion Supabase ÉCHOUÉE (${OUT:-réseau}) — bloquant pour l'étape 4"
  fi
else
  fail "SUPABASE_DATABASE_URL absent — bloquant"
fi

# ── 4. API Render ──────────────────────────────────────────────────────────
section "4. API Render"
if [ -n "$RENDER_API_KEY" ]; then
  S="$(curl -fsS --max-time 20 -H "Authorization: Bearer $RENDER_API_KEY" \
    "https://api.render.com/v1/services/$SERVICE_ID" 2>/dev/null)" \
    && ok "service mikcloud joignable" || fail "API Render injoignable / clé refusée"
  if [ -n "${S:-}" ]; then
    AUTO="$(printf '%s' "$S" | python3 -c 'import json,sys; print(json.load(sys.stdin)["autoDeploy"])')"
    [ "$AUTO" = "no" ] && ok "autoDeploy=no (gel N°162 en place — normal avant l'étape 5)" \
                        || warn "autoDeploy=$AUTO (attendu no avant l'étape 5)"
  fi
  E="$(curl -fsS --max-time 20 -H "Authorization: Bearer $RENDER_API_KEY" \
    "https://api.render.com/v1/services/$SERVICE_ID/env-vars" 2>/dev/null)" || E=""
  if [ -n "$E" ]; then
    DBHOST="$(printf '%s' "$E" | python3 -c '
import json,sys
for e in json.load(sys.stdin):
    if e["envVar"]["key"]=="DATABASE_URL":
        v=e["envVar"].get("value") or ""
        print(v.split("@")[-1].split("/")[0] if "@" in v else "?"); break')"
    case "$DBHOST" in
      *neon.tech*) ok "DATABASE_URL → $DBHOST (Neon — état pré-bascule normal)" ;;
      *pooler.supabase.com*) warn "DATABASE_URL → $DBHOST (Supabase — étape 5 déjà exécutée ?)" ;;
      *) warn "DATABASE_URL → $DBHOST (?)" ;;
    esac
  fi
else
  warn "RENDER_API_KEY absent — check Render sauté"
fi

# ── 5. GitHub ──────────────────────────────────────────────────────────────
section "5. GitHub (secrets, workflows, sentinel, CI)"
GHAPI="https://api.github.com/repos/$REPO"
if [ -n "$GITHUB_TOKEN" ]; then
  SECRETS="$(curl -fsS --max-time 20 -H "Authorization: Bearer $GITHUB_TOKEN" \
    "$GHAPI/actions/secrets" 2>/dev/null)" || SECRETS=""
  if [ -n "$SECRETS" ]; then
    MISSING="$(printf '%s' "$SECRETS" | python3 -c '
import json, sys
names = {s["name"] for s in json.load(sys.stdin).get("secrets", [])}
expected = ["BACKUP_KEY", "DATABASE_URL", "NEON_STANDBY_DATABASE_URL",
            "RENDER_API_KEY", "SUPABASE_DATABASE_URL"]
print(" ".join(n for n in expected if n not in names))')"
    if [ -z "$MISSING" ]; then
      ok "les 5 secrets attendus sont présents"
    else
      fail "secrets ABSENTS : $MISSING"
    fi
  else
    fail "liste des secrets illisible (token ?)"
  fi
  WF="$(curl -fsS --max-time 20 -H "Authorization: Bearer $GITHUB_TOKEN" \
    "$GHAPI/actions/workflows" 2>/dev/null)" || WF=""
  if [ -n "$WF" ]; then
    WF_STATES="$(printf '%s' "$WF" | python3 -c '
import json, sys
for w in json.load(sys.stdin).get("workflows", []):
    print(w["name"] + "|" + w["path"] + "|" + w["state"])')"
    for PAIR in "CI|active" "migrate-neon-supabase|active" "backup|disabled_manually" "Keep-alive Render|disabled_manually"; do
      W_NAME="${PAIR%%|*}"; W_STATE="${PAIR##*|}"
      FOUND="$(printf '%s\n' "$WF_STATES" | grep -F "$W_NAME" | /usr/bin/head -1 | cut -d'|' -f3)"
      [ "$FOUND" = "$W_STATE" ] && ok "workflow « $W_NAME » = $W_STATE" \
        || warn "workflow « $W_NAME » : ${FOUND:-introuvable} (attendu $W_STATE)"
    done
  fi
  MAINSHA="$(curl -fsS --max-time 20 -H "Authorization: Bearer $GITHUB_TOKEN" \
    "$GHAPI/branches/main" 2>/dev/null | python3 -c 'import json,sys; print(json.load(sys.stdin)["commit"]["sha"])' 2>/dev/null)" || MAINSHA=""
  if [ -n "$MAINSHA" ]; then
    SENT="$(curl -fsS --max-time 20 -H "Authorization: Bearer $GITHUB_TOKEN" \
      "$GHAPI/contents/RENDER-DEPLOY-FROZEN?ref=$MAINSHA" 2>/dev/null)"
    if [ -n "$SENT" ]; then ok "sentinel RENDER-DEPLOY-FROZEN présent sur main (gel en place)"
    else warn "sentinel ABSENT sur main — déploiements décongelés ?!"; fi
  fi
  LAST="$(curl -fsS --max-time 20 -H "Authorization: Bearer $GITHUB_TOKEN" \
    "$GHAPI/actions/runs?branch=main&per_page=1" 2>/dev/null | python3 -c '
import json, sys
r = json.load(sys.stdin)["workflow_runs"][0]
print(r["head_sha"][:7] + " " + r["status"] + "/" + str(r["conclusion"]) + " " + r["created_at"])' 2>/dev/null)" || LAST=""
  [ -n "$LAST" ] && say "  · dernière CI main : $LAST"
else
  warn "GITHUB_TOKEN absent — check GitHub sauté"
fi

# ── 6. Production ──────────────────────────────────────────────────────────
section "6. Production (HTTP)"
CODE="$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 "$BACKEND_URL/" || echo 000)"
[ "$CODE" = "200" ] && ok "backend Render : HTTP 200" || fail "backend Render : HTTP $CODE"
CODE="$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 "$FRONTEND_URL/" || echo 000)"
[ "$CODE" = "200" ] && ok "frontend Vercel : HTTP 200" || fail "frontend Vercel : HTTP $CODE"

# ── 7. Carte Santé (admin) — rattrapage de l'étape 2 ───────────────────────
section "7. Carte Santé admin (mode + rattrapage)"
if [ -n "$ADMIN_PASSWORD" ]; then
  TOKEN="$(curl -fsS --max-time 15 -X POST "$BACKEND_URL/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PASSWORD\"}" 2>/dev/null \
    | python3 -c 'import json,sys; print(json.load(sys.stdin).get("token",""))' 2>/dev/null)" || TOKEN=""
  if [ -n "$TOKEN" ]; then
    ok "login admin OK"
    curl -fsS --max-time 15 "$BACKEND_URL/api/admin/sync-status" \
      -H "Authorization: Bearer $TOKEN" 2>/dev/null | python3 -c '
import json, sys
d = json.load(sys.stdin)
sync = d.get("sync") or {}
neon = d.get("neon") or {}
print("  · mode : " + str(d.get("mode")))
print("  · synchro : succès=" + str(sync.get("successes")) + " échecs=" + str(sync.get("failures")) + " consécutifs=" + str(sync.get("consecutiveFailures")))
print("  · dernière synchro OK : " + str(sync.get("lastSuccessAt") or "JAMAIS"))
if sync.get("lastError"):
    print("  · dernière erreur : " + str(sync.get("lastError"))[:120])
if neon:
    print("  · contact PG : " + str(neon.get("lastContactAt") or "jamais") + " | keep-alive : " + str(neon.get("keepAliveMode")))' \
      || warn "sync-status illisible"
  else
    warn "login admin refusé (mot de passe changé ?)"
  fi
else
  warn "ADMIN_PASSWORD absent (coffre admin-credentials.txt) — check sauté"
fi

# ── Verdict ────────────────────────────────────────────────────────────────
say ""
say "════════════════════════════════════════════════════════════════"
say " VERDICT : $OK OK · $WARN avertissements · $FAIL échecs"
say "════════════════════════════════════════════════════════════════"
if [ $FAIL -gt 0 ]; then
  say "Des échecs bloquants — consulter le runbook §11 avant toute action."
  exit 1
fi
say "Rappel séquence : (1-2) attendre rattrapage syncreur (~1-2 h après reset,"
say "carte Santé synchro ok) → (4) dispatch migrate-neon-supabase → (5)"
say "step5-render-flip.sh --exec → (6) fusion n163+n164 + levée sentinel →"
say "(7) vérifications → (8) step8-flip-secret.py + step8-arm-standby.sh"
