# CHASSE JNB — « Double moteur » (N°268)

> Terrain de chasse : **tenancy Johannesburg** (le compte OCI de
> l'exploitant lui-même) — objectif : capturer une **A1.Flex** (≤ 2 OCPU /
> 12 Go, cap Always Free N°267) et un filet **E2.1.Micro**, « à d'autres
> fins » que la production. La production Marseille
> (**Ftechci** — ex-`mikcloud-backend` —, autre tenancy) est **invisible et intouchable** par
> construction (gardes par nom + OCID de tenancy distincts).

## 1. Les deux moteurs

| | Moteur 1 « instance » (pont E5) | Moteur 2 « GitHub » |
|---|---|---|
| Support | VM.Standard.E5.Flex 84.12.85.241 (crédits jusqu'au **31/10**) | Runners GitHub (dépôt public — gratuit illimité) |
| Mécanique | boucle `mikcloud-hunt-dispatch` **v3** (systemd timer, 2-5 min, non-superposante N°250-c) → dispatch `hunt-a1-jnb.yml` | `hunt-parallel-jnb.yml` cron `2-59/5` 24/7 + crons nocturnes `hunt-a1-jnb.yml` (`11,36 20-23,0-3 * * *`) |
| Installation / pilotage | `hunt-e5-arm.yml` (workflow_dispatch) | fichiers au dépôt — activés dès le push |
| Fin de vie | 31/10 : Oracle réclame l'E5 → la boucle meurt avec la VM (si capture avant : auto-extinction à la victoire) | **permanent** — survit au 31/10 (doctrine N°267) |

Doctrine de tir inchangée (N°222) : **sonder d'abord** (capacity-report —
gratuit), **ne tirer que si un pool est AVAILABLE** ; tir en **IP publique
éphémère** (l'IP réservée 84.12.85.241 du pont et l'IP prod
84.235.228.160 ne sont jamais touchées). Filet micro N°241 : propriété du
seul chasseur principal, cible #5.

> **N°270** : la sonde micro tolère les réponses **asynchrones** de l'API
> capacité (observé en tenancy JNB, runs #62/#63 — réponse sans
> `shape-availabilities`) : GET du rapport si son OCID est présent, puis
> UN SEUL retry de create, sinon `PROBE_ERROR` gracieux. Sans boucle —
> budget anti-429 intact. L'A1 (priorité) n'est pas concernée.

## 2. Les captures

- Noms : `mikcloud-jnb-a1` / `mikcloud-jnb-micro` (namespace dédié JNB).
- **Cloud-init volontairement INERT** : aucun `bootstrap.sh`, aucun secret —
  hostname + marqueur `/root/.mikcloud-jnb-capture` + bannière motd.
  L'usage « autres fins » de chaque capture est une décision de l'exploitant.
- Victoire = issue `[JNB] A1 capturée…` + désactivation croisée des DEUX
  chasseurs JNB + auto-extinction de la boucle E5 (v3 constate
  `hunt-a1-jnb.yml` désactivé).
- Survie : Always Free — les captures ≤ 2/12 + micros survivent à la fin du
  trial SANS PAYG (N°267).

## 3. Veille armée — les deux gestes console restants

Tant que le coffre `OCI_*_JNB` est incomplet, les chasseurs tournent à vide
**en sortant VERT** (`::notice::[JNB] VEILLE ARMÉE…`) — aucune notification
d'échec, aucun tir. Dès que les deux gestes ci-dessous sont faits, la
chasse devient réelle **sans nouvelle intervention** :

1. **Geste 1 — clé API** : console OCI (tenancy **JNB**) → profil →
   *Clés API* → *Ajouter une clé API* → *Coller une clé publique* → source
   canonique : `https://raw.githubusercontent.com/ftechnologies18/mikcloud/main/deploy/oracle/pilot-api-public-key-jnb-v2.pem`
   → l'empreinte affichée doit être
   `80:d6:18:71:82:26:dc:59:95:ba:66:e1:75:34:54:9c`.
2. **Geste 2 — config** : transmettre le bloc « Informations de
   configuration » (la ligne `user=` est la seule valeur manquante du
   coffre) — même procédure que PILOTAGE-ORACLE.md §2.

Alternative sans console : si un `~/.oci` existe encore sur le pont E5
(poste de pilotage historique), `hunt-e5-arm.yml` avec
`recover_oci_key=true` imprime les identifiants + la clé privée
**chiffrée** (sealed box) — jamais de PEM en clair dans les logs.

## 4. Coffre de secrets (GitHub)

| Secret | Valeur | État |
|---|---|---|
| `OCI_CLI_TENANCY_JNB` | OCID tenancy JNB (restauré du commit 892ebb4, N°256) | posé N°268 |
| `OCI_CLI_REGION_JNB` | `af-johannesburg-1` | posé N°268 |
| `OCI_CLI_FINGERPRINT_JNB` | empreinte de la clé v2 | posé N°268 |
| `OCI_API_KEY_JNB` | clé privée v2 (RSA 2048) | posé N°268 |
| `OCI_CLI_USER_JNB` | OCID utilisateur JNB | **Geste 2** |

Les secrets `OCI_*` (sans suffixe) restent la propriété **Marseille** —
jamais mélangés, jamais modifiés par le kit JNB.

## 5. Arrêt d'urgence

- Boucle E5 : `hunt-e5-arm.yml` avec `stop_loop=true` (ou directement
  `systemctl disable --now mikcloud-hunt-dispatch.timer` sur le pont).
- Chasseurs GitHub : croix console ou `gh workflow disable hunt-a1-jnb.yml hunt-parallel-jnb.yml`.
- Clé API : console → Clés API → Delete (2 clics, effet immédiat).
- Les chasseurs **Marseille** (`hunt-a1.yml` / `hunt-parallel.yml`) restent
  `disabled_manually` (N°265) — le kit JNB ne les touche jamais.
