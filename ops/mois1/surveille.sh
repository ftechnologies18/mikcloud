#!/usr/bin/env bash
# MIKCLOUD — Surveillance du premier mois post-migration Supabase
# (RUNBOOK-POSTGRES.md §8) : taille, egress, carte Santé, synchro/check-ins.
# LECTURE SEULE : aucune écriture, aucun déploiement, aucun secret modifié.
#
# Mesure, dans l'ordre :
#   1. Taille de la base (SQL Supabase) — seuil d'alerte 350 Mo (70 % des
#      500 Mo gratuits) : taille totale, lignes du schéma public, top
#      tables, croissance commands sur 24 h ;
#   2. Egress — deux angles :
#      a. applicatif backend (carte Santé /api/admin/sync-status → bloc
#         « bandwidth » N°72 : octets sortis du jour, par catégorie) ;
#      b. proxies Supabase : taille du dernier artefact de backup (le dump
#         quotidien participe à l'egress) — la figure EXACTE du quota
#         Supabase (5 Go/mois) se lit sur le dashboard → Reports/Usage,
#         aucun token d'API Management (sbp_) n'étant au coffre ;
#   3. Carte Santé (login admin) — mode, succès/échecs/consécutifs, âge de
#      la dernière synchro (le seuil §8 est « tout échec > 5 min ») ;
#   4. Synchro / check-ins (API logs Render) — occurrences de « store:
#      synchro PostgreSQL différée échouée » sur la fenêtre, entrées
#      level=error, présence du trafic agents (/agent/cmd, /agent/result).
#
# Usage : ops/mois1/surveille.sh
# Env :   MIKCLOUD_LOG_WINDOW_HOURS (défaut 6) — fenêtre des logs Render ;
#         MIKCLOUD_SIZE_ALERT_MB (défaut 350) — seuil taille (Mo).
# Secrets (env > coffre /home/z/.secrets) : SUPABASE_DATABASE_URL,
#   GITHUB_TOKEN, RENDER_API_KEY, ADMIN_PASSWORD.
# Dépendances : curl, python3 ; psycopg pour le SQL (auto-install via uv
#   si absent, sinon instruction affichée).
#
# Syntaxe API logs Render découverte le 21/09 au soir (N°179) :
#   GET /v1/logs?ownerId=…&resource={serviceId}&startTime=…&endTime=…&limit=…
#   (les paramètres plats owner/name/service renvoient « invalid path »).
#
# NOTE d'implémentation : aucune fonction ne porte le nom d'une commande
# standard (leçon du 21/09 : une fonction head() éclipse /usr/bin/head).
set -uo pipefail   # pas de -e : on veut le verdict COMPLET, check par check

COFFRE="${MIKCLOUD_VAULT:-/home/z/.secrets}"
REPO="ftechnologies18/mikcloud"
SERVICE_ID="srv-da974o142hec73euul60"
TEAM_ID="tea-da95ttajnfac73cpmnf0"
BACKEND_URL="https://mikcloud.onrender.com"
LOG_WINDOW_H="${MIKCLOUD_LOG_WINDOW_HOURS:-6}"
SIZE_ALERT_MB="${MIKCLOUD_SIZE_ALERT_MB:-350}"
EGRESS_ALERT_MB="3500"   # 3,5 Go = 70 % des 5 Go gratuits Supabase

OK=0; WARN=0; FAIL=0
ok()   { OK=$((OK+1));   printf '  ✓ %s\n' "$*"; }
warn() { WARN=$((WARN+1)); printf '  ⚠ %s\n' "$*"; }
fail() { FAIL=$((FAIL+1)); printf '  ✗ %s\n' "$*"; }
section() { printf '\n── %s ──\n' "$*"; }
say() { printf '%s\n' "$*"; }
fcmp_lt() { # fcmp_lt A B — vrai si A < B (décimaux)
  python3 -c "import sys; sys.exit(0 if float('$1') < float('$2') else 1)"
}

pick() { # pick NOM_VAR fichier regex
  local v="${!1:-}"
  if [ -z "$v" ] && [ -f "$COFFRE/$2" ]; then
    v="$(grep -E "$3" "$COFFRE/$2" | /usr/bin/head -1)"
  fi
  printf '%s' "$v"
}
SUPABASE_DATABASE_URL="$(pick SUPABASE_DATABASE_URL supabase-dsn.txt '^postgresql://.*:5432/')"
GITHUB_TOKEN="$(pick GITHUB_TOKEN github-token.txt '^ghp_|^github_pat_')"
RENDER_API_KEY="$(pick RENDER_API_KEY render-api-key.txt '^rnd_|^rctl_')"
ADMIN_PASSWORD="$(pick ADMIN_PASSWORD admin-credentials.txt '^ADMIN_PASSWORD=')"
ADMIN_PASSWORD="${ADMIN_PASSWORD#ADMIN_PASSWORD=}"

say "════════════════════════════════════════════════════════════════"
say " MIKCLOUD — surveillance mois 1 (runbook §8)"
say " $(date -u '+%Y-%m-%d %H:%M:%S UTC') · fenêtre logs ${LOG_WINDOW_H} h"
say "════════════════════════════════════════════════════════════════"

python3 -c "import psycopg" >/dev/null 2>&1 || {
  command -v uv >/dev/null 2>&1 \
    && uv pip install --quiet "psycopg[binary]" >/dev/null 2>&1 || true
}
if ! python3 -c "import psycopg" >/dev/null 2>&1; then
  say "psycopg absent — installer : uv pip install \"psycopg[binary]\""
  exit 2
fi

# ── 1. Taille de la base (SQL Supabase) ───────────────────────────────────
section "1. Taille de la base (seuil d'alerte : ${SIZE_ALERT_MB} Mo = 70 % de 500 Mo)"
SIZE_MB=""; LINES_PUBLIC=""; COMMANDS_24H=""; TOP=""; PCT_ALERT=""; PCT_CEIL=""
if [ -n "$SUPABASE_DATABASE_URL" ]; then
# Les valeurs multi-mots (top tables, catégories) sont émises entre quotes
# pour que l'eval ne les découpe pas sur « ; » ou les espaces.
  MEAS="$(python3 - "$SUPABASE_DATABASE_URL" "$SIZE_ALERT_MB" <<'PYEOF'
import sys
from urllib.parse import urlparse
import socket
import psycopg

q = lambda s: "'" + str(s).replace("'", "") + "'"

dsn, alert_mb = sys.argv[1], float(sys.argv[2])
u = urlparse(dsn)
ip = sorted({a[4][0] for a in socket.getaddrinfo(u.hostname, u.port or 5432,
                                                  socket.AF_INET, socket.SOCK_STREAM)})[0]
conninfo = (f"host={u.hostname} hostaddr={ip} port={u.port} dbname={u.path.lstrip('/')} "
            f"user={u.username} password={u.password} sslmode=require connect_timeout=15")
with psycopg.connect(conninfo) as c:
    size = c.execute("SELECT pg_database_size('postgres')").fetchone()[0]
    mo = size / 1048576.0
    print(f"SIZE_MB={mo:.2f}")
    print(f"PCT_ALERT={100.0 * mo / alert_mb:.1f}")
    print(f"PCT_CEIL={100.0 * mo / 500.0:.1f}")
    print("LINES_PUBLIC=" + q(c.execute(
        "SELECT coalesce(sum(n_live_tup),0) FROM pg_stat_user_tables WHERE schemaname='public'"
    ).fetchone()[0]))
    try:
        cmd24 = c.execute(
            "SELECT count(*) FROM public.commands "
            "WHERE created_at::timestamptz >= now() - interval '24 hours'"
        ).fetchone()[0]
    except Exception:
        cmd24 = "?"
    print("COMMANDS_24H=" + q(cmd24))
    print("TOP=" + q(";".join(
        f"{r[0]}:{r[1]/1048576.0:.2f}" for r in c.execute(
            "SELECT relname, pg_total_relation_size(relid) FROM pg_stat_user_tables "
            "WHERE schemaname='public' ORDER BY 2 DESC LIMIT 5").fetchall())))
PYEOF
)" 2>/tmp/mikcloud-surveille-sql.err
  if [ -n "$MEAS" ]; then
    eval "$MEAS"
    if fcmp_lt "$SIZE_MB" "$SIZE_ALERT_MB"; then
      ok "base = ${SIZE_MB} Mo (${PCT_ALERT} % du seuil, ${PCT_CEIL} % du plafond 500 Mo)"
    else
      fail "base = ${SIZE_MB} Mo — SEUIL ${SIZE_ALERT_MB} Mo FRANCHI (§8 : Render PostgreSQL Starter, §4-D)"
    fi
    say "  · lignes schéma public : ${LINES_PUBLIC} · commands 24 h : ${COMMANDS_24H}"
    say "  · top tables (Mo) : ${TOP}"
  else
    fail "taille de base illisible — $(cat /tmp/mikcloud-surveille-sql.err | tr '\n' ' ' | cut -c1-120)"
  fi
else
  warn "SUPABASE_DATABASE_URL absent — check taille sauté"
fi

# ── 2. Egress ──────────────────────────────────────────────────────────────
section "2. Egress (seuil Supabase : 3,5 Go/mois = 70 % de 5 Go)"
SYNC_JSON="$(mktemp)"
CARTE_OK=0
if [ -n "$ADMIN_PASSWORD" ]; then
  TOKEN="$(curl -fsS --max-time 15 -X POST "$BACKEND_URL/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PASSWORD\"}" 2>/dev/null \
    | python3 -c 'import json,sys; print(json.load(sys.stdin).get("token",""))' 2>/dev/null)" || TOKEN=""
  if [ -n "$TOKEN" ] \
     && curl -fsS --max-time 15 "$BACKEND_URL/api/admin/sync-status" \
          -H "Authorization: Bearer $TOKEN" -o "$SYNC_JSON" 2>/dev/null; then
    CARTE_OK=1
    BW="$(python3 - "$SYNC_JSON" <<'PYEOF'
import json, sys
d = json.load(open(sys.argv[1]))
bw = d.get("bandwidth") or {}
total = bw.get("totalBytes") or 0
mo = total / 1048576.0
cats = ", ".join(f"{c.get('name')}:{(c.get('bytes') or 0)/1024.0:.0f}K"
                 for c in (bw.get("categories") or []))
print(f"BW_DAY={bw.get('day','?')}")
print(f"BW_TOTAL_MB={mo:.2f}")
print("BW_CATS=" + "'" + cats.replace("'", "") + "'")
PYEOF
)"
    if [ -n "$BW" ]; then
      eval "$BW"
      # Ce compteur repart de zéro chaque jour (N°72) — le risque §8 porte
      # sur le CUMUL mensuel : on extrapole à 31 jours, garde-fou grossier.
      PROJ="$(python3 -c "print('%.1f' % (float('$BW_TOTAL_MB') * 31))" 2>/dev/null)"
      if fcmp_lt "$PROJ" "$EGRESS_ALERT_MB"; then
        ok "egress applicatif du jour = ${BW_TOTAL_MB} Mo → projection 31 j ≈ ${PROJ} Mo (< ${EGRESS_ALERT_MB} Mo)"
      else
        fail "egress applicatif du jour = ${BW_TOTAL_MB} Mo → PROJECTION ${PROJ} Mo ≥ ${EGRESS_ALERT_MB} Mo"
      fi
      say "  · date compteur : ${BW_DAY} · catégories : ${BW_CATS}"
    fi
  else
    warn "carte Santé injoignable — egress applicatif non mesuré"
  fi
else
  warn "ADMIN_PASSWORD absent — egress applicatif sauté"
fi

# Proxy Supabase : taille du dernier artefact de backup (dump quotidien).
if [ -n "$GITHUB_TOKEN" ]; then
  ART_MB="$(curl -fsS --max-time 20 -H "Authorization: token $GITHUB_TOKEN" \
    "https://api.github.com/repos/$REPO/actions/artifacts?per_page=10" 2>/dev/null \
    | python3 -c "
import json, sys
d = json.load(sys.stdin)
arts = [a for a in d.get('artifacts', []) if a.get('name') == 'mikcloud-backup' and not a.get('expired')]
print('%.2f' % (arts[0]['size_in_bytes']/1048576.0) if arts else '')" 2>/dev/null)"
  if [ -n "$ART_MB" ] && [ "$ART_MB" != "0.00" ]; then
    say "  · proxy egress Supabase : dernier dump chiffré = ${ART_MB} Mo/jour"
    ok "proxy dump mesuré (${ART_MB} Mo/j) — figure exacte : dashboard Supabase → Reports/Usage"
  else
    warn "aucun artefact mikcloud-backup actif (backup expiré/désactivé ?)"
  fi
else
  warn "GITHUB_TOKEN absent — proxy dump sauté"
fi

# ── 3. Carte Santé ─────────────────────────────────────────────────────────
section "3. Carte « Santé de la persistance » (seuil : échec > 5 min)"
if [ "$CARTE_OK" -eq 1 ]; then
  CARTE="$(python3 - "$SYNC_JSON" <<'PYEOF'
import json, sys, datetime
d = json.load(open(sys.argv[1]))
sync = d.get("sync") or {}
neon = d.get("neon") or {}
agents = d.get("agents") or {}
print(f"MODE={d.get('mode')}")
print(f"SUCC={sync.get('successes')} FAIL={sync.get('failures')} CONS={sync.get('consecutiveFailures')}")
print(f"DEGRADED={d.get('degraded')}")
print(f"PG_CONTACT={neon.get('lastContactAt')} KEEPALIVE={neon.get('keepAliveMode')}")
print(f"ROUTERS_ON={agents.get('routersOnline')} LAST_CHECKIN={agents.get('lastCheckIn')}")
ls = sync.get("lastSuccessAt")
age = ""
if ls:
    try:
        delta = (datetime.datetime.now(datetime.timezone.utc)
                 - datetime.datetime.fromisoformat(ls.replace("Z", "+00:00"))).total_seconds()
        age = int(delta)
    except Exception:
        pass
print(f"AGE_S={age if age != '' else -1}")
PYEOF
)"
  eval "$CARTE"
  if [ "${CONS:-x}" = "0" ] && [ "${AGE_S:--1}" -ge 0 ] && [ "${AGE_S:--1}" -le 300 ]; then
    ok "mode=${MODE}, 0 échec consécutif, dernière synchro OK il y a ${AGE_S} s"
  elif [ "${CONS:-x}" = "0" ]; then
    warn "0 échec consécutif MAIS dernière synchro OK vieille de ${AGE_S} s (> 5 min)"
  else
    fail "${CONS} échecs consécutifs — seuil §8 « échec > 5 min » atteint ou dépassé"
  fi
  if [ "$DEGRADED" = "None" ]; then
    ok "aucun état dégradé affiché (degraded=null)"
  else
    warn "état dégradé : $DEGRADED"
  fi
  say "  · succès=${SUCC} échecs=${FAIL} · contact PG : ${PG_CONTACT} · keep-alive : ${KEEPALIVE}"
  say "  · routeurs online : ${ROUTERS_ON} · dernier check-in : ${LAST_CHECKIN}"
else
  fail "carte Santé illisible — §8 non vérifiable (login admin refusé ?)"
fi

# ── 4. Synchro / check-ins (logs Render) ───────────────────────────────────
section "4. Synchro / check-ins — logs Render (${LOG_WINDOW_H} dernières heures)"
if [ -n "$RENDER_API_KEY" ]; then
  T1="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  T0="$(date -u -d "-${LOG_WINDOW_H} hours" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || printf '%s' "$T1")"
  PAGE="$(mktemp)"; ALL="$(mktemp)"
  CURSOR="$T1"; PAGES=0
  while [ $PAGES -lt 10 ]; do
    PAGES=$((PAGES+1))
    CODE="$(curl -sgS --max-time 30 -o "$PAGE" -w '%{http_code}' \
      -H "Authorization: Bearer $RENDER_API_KEY" \
      "https://api.render.com/v1/logs?ownerId=$TEAM_ID&resource=$SERVICE_ID&startTime=$T0&endTime=$CURSOR&limit=100" \
      2>/dev/null)"
    [ "$CODE" = "200" ] || break
    HASMORE="$(python3 -c "import json; print(json.load(open('$PAGE')).get('hasMore'))" 2>/dev/null)"
    python3 - "$PAGE" >> "$ALL" <<'PYEOF'
import json, sys
rows = json.load(open(sys.argv[1])).get("logs", [])
for l in rows:
    print(json.dumps(l))
PYEOF
    [ "$HASMORE" = "True" ] || [ "$HASMORE" = "true" ] || break
    CURSOR="$(python3 -c "import json; print(json.load(open('$PAGE'))['logs'][-1]['timestamp'])" 2>/dev/null)"
    [ -n "$CURSOR" ] || break
  done
  STATS="$(python3 - "$ALL" <<'PYEOF'
import json, sys
logs = [json.loads(x) for x in open(sys.argv[1]) if x.strip()]
msg = lambda l: str(l.get("message", ""))
echecs = sum(1 for l in logs if "synchro PostgreSQL différée échouée" in msg(l))
errors = sum(1 for l in logs if str(l.get("level", "")).lower() in ("error", "fatal"))
agents = sum(1 for l in logs if "/agent/cmd" in msg(l) or "/agent/result" in msg(l))
print(f"N={len(logs)} SYNC_FAIL={echecs} ERRORS={errors} AGENT_REQ={agents}")
PYEOF
)" 2>/dev/null
  if [ -n "$STATS" ]; then
    say "  · $STATS"
    SYNC_FAIL="$(printf '%s' "$STATS" | sed -n 's/.*SYNC_FAIL=\([0-9]*\).*/\1/p')"
    ERRORS="$(printf '%s' "$STATS" | sed -n 's/.*ERRORS=\([0-9]*\).*/\1/p')"
    AGENT_REQ="$(printf '%s' "$STATS" | sed -n 's/.*AGENT_REQ=\([0-9]*\).*/\1/p')"
    if [ "${SYNC_FAIL:-1}" = "0" ]; then
      ok "0 « store: synchro PostgreSQL différée échouée » sur la fenêtre"
    else
      fail "${SYNC_FAIL} « synchro PostgreSQL différée échouée » — le signal d'alerte §8 EST PRÉSENT"
    fi
    if [ "${ERRORS:-1}" = "0" ]; then
      ok "0 entrée level=error/fatal"
    else
      warn "${ERRORS} entrées error/fatal (à lire)"
    fi
    if [ "${AGENT_REQ:-0}" -gt 0 ]; then
      ok "check-ins agents vivants (${AGENT_REQ} requêtes /agent/* sur la fenêtre)"
    else
      warn "aucune requête /agent/* sur la fenêtre — parc connecté ?"
    fi
  else
    fail "logs Render illisibles (API logs : syntaxe ownerId+resource, cf. en-tête)"
  fi
  rm -f "$PAGE" "$ALL"
else
  warn "RENDER_API_KEY absent — check logs sauté"
fi
rm -f "$SYNC_JSON"

# ── Verdict ────────────────────────────────────────────────────────────────
say ""
say "════════════════════════════════════════════════════════════════"
say " VERDICT §8 : $OK OK · $WARN avertissements · $FAIL échecs"
say " Mesure à journaler dans docs/RUNBOOK-POSTGRES.md §8 (tableau de suivi)"
say "════════════════════════════════════════════════════════════════"
[ $FAIL -eq 0 ] && exit 0 || exit 1
