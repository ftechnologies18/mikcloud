#!/bin/bash
# mikcloud-hunt-dispatch.sh — v3 JNB (N°268, 08/10/2026).
#
# Historique : v1 (N°236→237) dispatchait hunt-a1.yml toutes les 5 min →
# en pénurie de runners, chaque run EN ATTENTE remplaçait le précédent →
# notifications « All jobs were cancelled » répétées. v2 (N°250-c) :
# AVANT tout dispatch, vérifier le dernier run — s'il n'est pas terminé,
# PASSER le créneau (aucune annulation possible).
#
# v3 (N°268) : GO exploitant « chasse JNB double moteur » — la boucle du
# pont est REPOINTÉE de hunt-a1.yml (Marseille, éteinte N°265) vers
# hunt-a1-jnb.yml (tenancy Johannesburg « autres fins »). Elle constitue
# le MOTEUR 1 « instance » du double moteur : dispatch 24/7 toutes les N
# min tant que le pont E5 vit — l'épuisement des crédits (31/10/2026)
# réclamera la VM et éteindra la boucle avec elle (moteur 2 « GitHub » :
# hunt-parallel-jnb.yml cron 2-59/5 + crons nocturnes hunt-a1-jnb.yml
# prennent le relais et survivent, doctrine N°267).
#
# Discipline conservée (v2, N°250-c) :
#   1) le workflow cible est-il désactivé ? (victoire JNB détectée par la
#      garde, ou extinction manuelle → le timer s'éteint avec lui) ;
#   2) un run est-il encore vivant ? (queued / in_progress → créneau
#      passé — exactement un chasseur principal vivant à la fois) ;
#   3) dispatch ponctuel (voie fiable : crons GitHub en retard, N°234).
#
# Jeton : /root/.config/mikcloud-hunt/token (root, mode 600) — droits
# lecture workflows + dispatch uniquement.

TOKEN_FILE=/root/.config/mikcloud-hunt/token
TOKEN="$(cat "$TOKEN_FILE" 2>/dev/null)" || {
  echo "ERREUR : jeton absent ($TOKEN_FILE)."
  exit 1
}

API="https://api.github.com/repos/ftechnologies18/mikcloud/actions"
WF="hunt-a1-jnb.yml"

# 1) Le chasseur JNB est-il désactivé ? (victoire → extinction du moteur)
STATE="$(curl -s --max-time 20 -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/vnd.github+json" \
  "$API/workflows/$WF" | grep -o '"state":"[a-z_]*"' | head -1 | cut -d'"' -f4)"
if [ "$STATE" = "disabled_manually" ] || [ "$STATE" = "disabled_inactivity" ]; then
  echo "hunt-a1-jnb désactivé ($STATE) — victoire JNB ou extinction : arrêt du timer."
  systemctl disable --now mikcloud-hunt-dispatch.timer
  exit 0
fi

# 2) Un run est-il encore vivant ? (queued / in_progress → passer)
LAST="$(curl -s --max-time 20 -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/vnd.github+json" \
  "$API/workflows/$WF/runs?per_page=1" | grep -o '"status":"[a-z_]*"' | head -1 | cut -d'"' -f4)"
if [ "$LAST" = "queued" ] || [ "$LAST" = "in_progress" ]; then
  echo "run précédent encore $LAST — créneau passé (aucune annulation possible)."
  exit 0
fi

# 3) Dispatch ponctuel (voie fiable : crons GitHub en retard, N°234)
HTTP="$(curl -s -o /dev/null -w '%{http_code}' --max-time 30 -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/vnd.github+json" \
  "$API/workflows/$WF/dispatches" \
  -d '{"ref":"main"}')"
if [ "$HTTP" = "204" ]; then
  exit 0
fi
echo "dispatch HTTP $HTTP — le prochain créneau retentera."
exit 0
