#!/usr/bin/env bash
# MIKCLOUD — Rotation du jeton Cloudflare R2 (canal d'images des portails).
#
# Contexte N°183 (23/09/2026) : le jeton R2_API_TOKEN de Render est mort un
# soir (expiré/révoqué côté Cloudflare — premiers 401 le 20/09 23:44Z) :
# chaque slide/bannière téléversée répondait 502 et disparaissait des
# portails, SANS aucun signal console. Ce script fait la rotation complète
# en < 5 minutes une fois le nouveau jeton créé côté Cloudflare :
#
#   1. LIT l'environnement Render (LECTURE SEULE — rien n'est écrit avant
#      que le nouveau jeton soit prouvé bon) : compte, compartiment, garde
#      anti-perte sur les valeurs illisibles ;
#   2. VALIDE le nouveau jeton sur l'API R2 ELLE-MÊME (listage des
#      compartiments) — validité + permission R2 en un seul appel ;
#   3. PUT env-vars Render : liste COMPLÈTE (discipline N°172), seule
#      R2_API_TOKEN est remplacée — garde anti-perte sur les CLÉS et sur
#      les VALEURS illisibles ;
#   4. Déclenche le redéploiement Render (l'env ne s'applique qu'au boot) ;
#   5. Fume-test : une URL média témoin doit répondre 200.
#
# N°185 (24/09/2026) — PIÈGE « cfat_ » : la version N°183 validait le jeton
# via /user/tokens/verify. Or les jetons créés depuis la CONSOLE R2 (« R2 →
# Manage R2 API Tokens », format cfat_…, qui délivrent AUSSI une paire
# d'identifiants S3) sont REFUSÉS par cet endpoint (« Invalid API Token »,
# code 1000) alors qu'ils fonctionnent PARFAITEMENT sur l'API R2. La
# rotation réelle du 24/09 est morte à l'étape 1 sur un jeton POURTANT
# VALIDE. Règle désormais gravée ici : NE JAMAIS utiliser /user/tokens/verify
# pour valider un jeton R2 — le sondage se fait sur l'API R2 elle-même.
#
# CRÉATION DU JETON (à faire dans Cloudflare avant le script) — DEUX voies :
#   Voie A — console R2 : R2 → Manage R2 API Tokens → Create API Token
#     (Object Read & Write, ou Admin Read & Write) → le jeton cfat_… est la
#     « Valeur du jeton » ; la paire S3 qui l'accompagne ne sert PAS au
#     backend (API REST Bearer) — la ranger au coffre pour les outils S3
#     (rclone, aws cli, inspections de secours).
#   Voie B — jeton classique : My Profile → API Tokens → Create Token →
#     Custom Token — Permissions : Account → R2 → Edit.
#   Dans les DEUX cas : TTL de préférence AUCUNE expiration (sinon : rappel
#   calendaire AVANT l'expiration — cf. RUNBOOK-SECRETS §2.7 : c'est une
#   expiration silencieuse qui a causé l'incident de septembre).
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

say "═══ Rotation du jeton R2 (N°183/N°185) ═══"
[ "$EXEC" -eq 0 ] && say "MODE DRY-RUN (aucune écriture — ajouter --exec pour appliquer)"
say ""

# ── 1. Lecture de l'environnement Render (LECTURE SEULE) ──────────────────
# Rien n'est écrit avant l'étape 4 : lire l'env Render est sans risque, et
# le compte extrait ici (R2_ACCOUNT_ID) sert à l'étape 2 pour valider le
# jeton sur la BONNE API (l'API R2 a besoin du compte, /user/tokens/verify
# n'en avait pas besoin — c'est le seul motif de cette inversion d'étapes).
say "→ 1/5 Lecture de l'environnement Render (lecture seule)…"
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

# ── 2. Le nouveau jeton est-il VALIDE et R2-capable ? ─────────────────────
# Sondage sur l'API R2 elle-même (N°185) : le listage des compartiments
# valide le jeton ET la permission en un seul appel, pour TOUS les types de
# jetons (cfat_ de la console R2 comme classiques). Un jeton mort y répond
# 401 « Authentication error » (code 10000). NE PAS revenir à
# /user/tokens/verify : il refuse les cfat_ valides (faux négatif prouvé).
say "→ 2/5 Validation du NOUVEAU jeton sur l'API R2 (listage des compartiments)…"
BODY_FILE="$(mktemp)"
trap 'rm -f "$BODY_FILE"' EXIT
CODE="$(curl -sS --max-time 20 -o "$BODY_FILE" -w '%{http_code}' \
  -H "Authorization: Bearer $R2_NEW_TOKEN" \
  "$CF/accounts/$ACCOUNT/r2/buckets" || echo 000)"
case "$CODE" in
  200) ;;
  401|403) die "Cloudflare REFUSE le nouveau jeton (HTTP $CODE) — recréer le jeton (voies A/B en tête de script), ne rien pousser" ;;
  *) die "Réponse inattendue de Cloudflare (HTTP $CODE) — ne rien pousser, réessayer" ;;
esac
if python3 -c 'import json,sys; sys.exit(0 if json.load(open(sys.argv[1])).get("success") else 1)' "$BODY_FILE" 2>/dev/null; then
  HAS_BUCKET="$(python3 -c '
import json,sys
d = json.load(open(sys.argv[1]))
target = sys.argv[2]
names = [b["name"] for b in d.get("result",{}).get("buckets",[])]
print("vue" if target in names else ("NON vue (compartiments vus : %s)" % ", ".join(names[:5]) if names else "vue (aucun compartiment listé)"))' "$BODY_FILE" "$BUCKET")"
  say "   ✓ jeton accepté par l'API R2 — compartiment cible : $HAS_BUCKET"
else
  die "Cloudflare a répondu 200 sans succès — corps inattendu, ne rien pousser"
fi

# ── 3. Construction du PUT (liste complète, discipline N°172) ─────────────
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
# NB : le POST /deploys renvoie l'objet à plat, mais la LISTE /deploys
# emballe chaque entrée dans {"deploy": {…}, "cursor": …} (pagination
# v1 Render) — la boucle de suivi ci-dessous tient compte des DEUX formes
# (bug latent N°183 corrigé N°185 : KeyError 'id' à chaque poll sinon).
DEPLOY_ID="$(curl -fsS --max-time 30 -X POST \
  -H "Authorization: Bearer $RENDER_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{}' "$API/services/$SERVICE_ID/deploys" | python3 -c '
import json,sys
d = json.load(sys.stdin)
d = d.get("deploy", d)
print(d["id"])')" \
  || die "ÉCHEC du déclenchement de déploiement"
say "   déploiement $DEPLOY_ID en cours — attente du live (≤ 8 min)…"
LIVE=""
for i in $(seq 1 48); do
  sleep 10
  STATUS="$(curl -fsS --max-time 20 -H "Authorization: Bearer $RENDER_API_KEY" \
    "$API/services/$SERVICE_ID/deploys?limit=10" \
    | python3 -c '
import json,sys
for e in json.load(sys.stdin):
    d = e.get("deploy") if isinstance(e, dict) else None
    if d and d.get("id") == sys.argv[1]: print(d.get("status", "?")); break' "$DEPLOY_ID" || echo "?")"
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
say "« Stockage d'images (portail) » doit lire Opérationnel (sonde N°185)."
say "Puis ranger le jeton au coffre et révoquer l'ancien côté Cloudflare"
say "(il est déjà mort, mais pour la traçabilité)."
