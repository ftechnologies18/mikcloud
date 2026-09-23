#!/usr/bin/env bash
# MIKCLOUD — Rotation du jeton Cloudflare R2 (canal d'images des portails).
#
# Contexte N°183 (23/09/2026) : le jeton R2_API_TOKEN de Render est mort un
# soir (expiré/révoqué côté Cloudflare — premiers 401 le 20/09 23:44Z) :
# chaque slide/bannière téléversée répondait 502 et disparaissait des
# portails, SANS aucun signal console. Ce script fait la rotation complète
# en < 5 minutes une fois le nouveau jeton créé côté Cloudflare :
#
#   1. VÉRIFIE le nouveau jeton contre l'API Cloudflare (verify) AVANT de
#      toucher à quoi que ce soit — un jeton faux ne doit jamais entrer en
#      production ;
#   2. VÉRIFIE la permission R2 (listage des compartiments du compte lu
#      dans l'env Render) ;
#   3. PUT env-vars Render : liste COMPLÈTE (discipline N°172), seule
#      R2_API_TOKEN est remplacée — garde anti-perte sur les CLÉS et sur
#      les VALEURS illisibles ;
#   4. Déclenche le redéploiement Render (l'env ne s'applique qu'au boot) ;
#   5. Fume-test : une URL média témoin doit répondre 200.
#
# CRÉATION DU JETON (à faire dans Cloudflare avant le script) :
#   Dashboard Cloudflare → My Profile (en haut à droite) → API Tokens →
#   Create Token → Custom Token :
#     - Permissions : Account → R2 → Edit
#     - Account Resources : Include → <le compte MikCloud>
#     - TTL : de préférence AUCUNE expiration (sinon : rappel calendaire
#       AVANT l'expiration — cf. RUNBOOK-SECRETS §2.7 : c'est une
#       expiration silencieuse qui a causé l'incident de septembre)
#   → Continue to summary → Create Token → copier le jeton (cfat_…).
#
# Usage :
#   R2_NEW_TOKEN="cfat_…" ops/media/r2-rotate-token.sh            # DRY-RUN
#   R2_NEW_TOKEN="cfat_…" ops/media/r2-rotate-token.sh --exec     # APPLIQUE
#   MEDIA_WITNESS_URL="https://…/api/media/media/acc…/….jpg" en env
#   optionnel pour le fume-test final (défaut : une slide du compte témoin).
#
# Secrets (priorité : variable d'environnement > coffre /home/z/.secrets) :
#   RENDER_API_KEY    clé API Render (rnd_…)   [coffre : render-api-key.txt]
#   R2_NEW_TOKEN      le NOUVEAU jeton cfat_…   [coffre : r2-new-token.txt]
set -euo pipefail

SERVICE_ID="srv-da974o142hec73euul60"
API="https://api.render.com/v1"
CF="https://api.cloudflare.com/client/v4"
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

R2_NEW_TOKEN="${R2_NEW_TOKEN:-}"
if [ -z "$R2_NEW_TOKEN" ] && [ -f "$COFFRE/r2-new-token.txt" ]; then
  R2_NEW_TOKEN="$(grep -E '^cfat_' "$COFFRE/r2-new-token.txt" | head -1)"
fi
if [ -z "$R2_NEW_TOKEN" ]; then
  die "R2_NEW_TOKEN absent — créez le jeton côté Cloudflare (étapes en tête de ce script) puis :
  R2_NEW_TOKEN=\"cfat_…\" $0 ${1:-}"
fi
[ "${#R2_NEW_TOKEN}" -ge 40 ] || die "Le jeton paraît trop court (${#R2_NEW_TOKEN} car.) — jeton Cloudflare complet attendu (cfat_…)"

WITNESS="${MEDIA_WITNESS_URL:-https://mikcloud.onrender.com/api/media/media/acc-6e2e34a620c9/2026/188c56c5b5990e623c291d89b15eef01.jpg}"

say "═══ Rotation du jeton R2 (N°183) ═══"
[ "$EXEC" -eq 0 ] && say "MODE DRY-RUN (aucune écriture — ajouter --exec pour appliquer)"
say ""

# ── 1. Le nouveau jeton est-il VALIDE côté Cloudflare ? ───────────────────
say "→ 1/5 Vérification du NOUVEAU jeton auprès de Cloudflare…"
# Pas de -f ici : un jeton refusé renvoie 401 avec un corps JSON {success:false}
# — c'est le VERDICT qu'on veut lire, pas une erreur réseau.
VERIFY="$(curl -sS --max-time 20 -H "Authorization: Bearer $R2_NEW_TOKEN" "$CF/user/tokens/verify")" \
  || die "API Cloudflare injoignable (réseau)"
VERIFY_OK="$(printf '%s' "$VERIFY" | python3 -c 'import json,sys
try: print(json.load(sys.stdin).get("success") is True)
except Exception: print("False")')"
[ "$VERIFY_OK" = "True" ] || die "Cloudflare REFUSE le nouveau jeton (verify ≠ success) — recréer le jeton, ne rien pousser"
say "   ✓ jeton accepté par Cloudflare"

# ── 2. Lecture de l'env Render + contrôle de permission R2 ────────────────
say "→ 2/5 Lecture de l'environnement Render…"
ENV_JSON="$(curl -fsS --max-time 20 -H "Authorization: Bearer $RENDER_API_KEY" "$API/services/$SERVICE_ID/env-vars")" \
  || die "Lecture des variables Render impossible"
ACCOUNT="$(printf '%s' "$ENV_JSON" | python3 -c '
import json,sys
for e in json.load(sys.stdin):
    if e["envVar"]["key"] == "R2_ACCOUNT_ID": print(e["envVar"].get("value") or ""); break')"
[ -n "$ACCOUNT" ] || die "R2_ACCOUNT_ID absent de l'env Render — le poser d'abord (voir RUNBOOK-SECRETS §1)"
BUCKET="$(printf '%s' "$ENV_JSON" | python3 -c '
import json,sys
for e in json.load(sys.stdin):
    if e["envVar"]["key"] == "R2_BUCKET": print(e["envVar"].get("value") or ""); break')"
[ -n "$BUCKET" ] || BUCKET="mikcloud-media"
say "   compte Cloudflare : $ACCOUNT · compartiment : $BUCKET"

BUCKETS="$(curl -fsS --max-time 20 -H "Authorization: Bearer $R2_NEW_TOKEN" \
  "$CF/accounts/$ACCOUNT/r2/buckets" || true)"
if printf '%s' "$BUCKETS" | python3 -c 'import json,sys; sys.exit(0 if json.load(sys.stdin).get("success") else 1)' 2>/dev/null; then
  HAS_BUCKET="$(printf '%s' "$BUCKETS" | python3 -c "
import json,sys
d = json.load(sys.stdin)
names = [b['name'] for b in d.get('result',{}).get('buckets',[])]
print('vue' if '$BUCKET' in names else ('NON vue (compartiments vus : %s)' % ', '.join(names[:5]) if names else 'vue (aucun compartiment listé)'))")"
  say "   ✓ permission R2 effective — compartiment cible : $HAS_BUCKET"
else
  die "Le nouveau jeton n'arrive pas à lister les compartiments R2 (permission Account → R2 : Edit requise)"
fi

# ── 3. Construction du PUT (liste complète, discipline N°172) ─────────────
# Garde anti-perte AMÉLIORÉE vs N°172 : une variable dont la valeur est
# ILLISIBLE (null) en GET serait ré-émise VIDE par le PUT — refus net,
# l'opérateur inspecte (les valeurs volontairement vides, ex. REGISTER_KEY
# « », sont ré-émises à l'identique, elles, sans danger).
UNREADABLE="$(printf '%s' "$ENV_JSON" | python3 -c '
import json,sys
out = [e["envVar"]["key"] for e in json.load(sys.stdin) if e["envVar"].get("value") is None]
print(", ".join(out))')"
[ -z "$UNREADABLE" ] || die "Variables à valeur ILLISIBLE en lecture (seraient vidées par le PUT) : $UNREADABLE — inspecter le dashboard Render d'abord"

HAS_TOKEN="$(printf '%s' "$ENV_JSON" | python3 -c '
import json,sys
print("oui" if any(e["envVar"]["key"] == "R2_API_TOKEN" for e in json.load(sys.stdin)) else "non")')"
say "   R2_API_TOKEN déjà présente sur le service : $HAS_TOKEN"

PUT_PAYLOAD="$(python3 -c '
import json, sys
envs = json.loads(sys.argv[1])
new_token = sys.argv[2]
lst, seen = [], False
for e in envs:
    k = e["envVar"]["key"]
    v = e["envVar"].get("value")
    if v is None:
        continue  # déjà refusé plus haut
    if k == "R2_API_TOKEN":
        v, seen = new_token, True
    lst.append({"key": k, "value": v})
if not seen:
    lst.append({"key": "R2_API_TOKEN", "value": new_token})
print(json.dumps(lst))' "$ENV_JSON" "$R2_NEW_TOKEN")"
NPUT="$(printf '%s' "$PUT_PAYLOAD" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
say "→ 3/5 PUT préparé : $NPUT variable(s), seule R2_API_TOKEN change (+ ajout si absente)"
say ""
if [ "$EXEC" -eq 0 ]; then
  say "DRY-RUN terminé — rien n'a été modifié. Relancer avec --exec pour appliquer."
  exit 0
fi

# ── 4. PUT + redéploiement ────────────────────────────────────────────────
say "→ 4/5 Application (PUT env-vars)…"
curl -fsS --max-time 30 -X PUT \
  -H "Authorization: Bearer $RENDER_API_KEY" \
  -H "Content-Type: application/json" \
  -d "$PUT_PAYLOAD" \
  "$API/services/$SERVICE_ID/env-vars" > /dev/null \
  || die "ÉCHEC du PUT env-vars"
sleep 2
# Garde anti-perte : clés conservées + R2_API_TOKEN remplacée (longueur).
ENV_AFTER="$(curl -fsS --max-time 20 -H "Authorization: Bearer $RENDER_API_KEY" "$API/services/$SERVICE_ID/env-vars")"
CHECK="$(python3 -c '
import json, sys
before = json.loads(sys.argv[1]); after = json.loads(sys.argv[2])
bk = {e["envVar"]["key"] for e in before}; ak = {e["envVar"]["key"] for e in after}
lost = bk - ak
av = {e["envVar"]["key"]: (e["envVar"].get("value") or "") for e in after}
print("OK" if not lost and len(av.get("R2_API_TOKEN","")) >= 40 else "KO:" + ",".join(sorted(lost)))' \
  "$ENV_JSON" "$ENV_AFTER")"
case "$CHECK" in
  OK) say "   ✓ variables posées, aucune clé perdue, R2_API_TOKEN remplacée" ;;
  *)  die "Anomalie post-PUT : $CHECK" ;;
esac

say "   → Déclenchement du redéploiement (l'env ne s'applique qu'au boot)…"
DEPLOY_ID="$(curl -fsS --max-time 30 -X POST \
  -H "Authorization: Bearer $RENDER_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{}' "$API/services/$SERVICE_ID/deploys" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')" \
  || die "ÉCHEC du déclenchement de déploiement"
say "   déploiement $DEPLOY_ID en cours — attente du live (≤ 8 min)…"
LIVE=""
for i in $(seq 1 48); do
  sleep 10
  STATUS="$(curl -fsS --max-time 20 -H "Authorization: Bearer $RENDER_API_KEY" \
    "$API/services/$SERVICE_ID/deploys?limit=10" \
    | python3 -c "
import json,sys
for d in json.load(sys.stdin):
    if d['id'] == '$DEPLOY_ID': print(d['status']); break" || echo "?")"
  say "      [$((i*10))s] $STATUS"
  case "$STATUS" in
    live) LIVE="yes"; break ;;
    build_failed|update_failed|canceled|deactivated) die "déploiement $STATUS — inspecter le dashboard Render" ;;
  esac
done
[ "$LIVE" = "yes" ] || die "Le déploiement n'est pas passé au live en 8 min — inspecter Render"

# ── 5. Fume-test : une URL média témoin doit revenir ──────────────────────
say "→ 5/5 Fume-test de l'URL témoin…"
sleep 5
CODE="$(curl -s -o /dev/null -w '%{http_code}' --max-time 30 "$WITNESS")"
if [ "$CODE" = "200" ]; then
  say "   ✓ HTTP 200 — les images sont de retour"
else
  say "   ⚠ HTTP $CODE sur le témoin : si 502, attendre ~30 s (boot frais) et retenter ;"
  say "     si 404 le compartiment/l'historique des clés est à vérifier côté Cloudflare."
fi
say ""
say "═══ ROTATION TERMINÉE ═══"
say "Dernière vérification conseillée : console → Paramètres → Santé →"
say "« Stockage d'images (portail) » doit lire Opérationnel (sonde N°183)."
say "Puis ranger le jeton au coffre et révoquer l'ancien côté Cloudflare"
say "(il est déjà mort, mais pour la traçabilité)."
