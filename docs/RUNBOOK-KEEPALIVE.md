# RUNBOOK — Keep-alive Render (anti-hibernation)

> Contexte : le backend Go tourne sur le plan **FREE de Render**, qui suspend
> le service après **~15 min sans trafic**. Le redémarrage à froid qui suit
> coûte **30 à 90 s** au premier visiteur (boot conteneur + rechargement
> complet de l'état depuis Neon).

> **STATUT (2026-09-13) : Option A ACTIVE** — monitor UptimeRobot Free
> (HTTP, toutes les 5 min) posé sur `https://mikcloud.onrender.com/` par le
> gérant ; le workflow GitHub est **désactivé** (§2, conservé comme repli).
> Défense en profondeur : (1) UptimeRobot élimine l'hibernation, (2) la
> couche frontend N°84 (§1-bis) rend tout cold boot résiduel (p.ex. pendant
> un déploiement) quasi invisible, (3) le workflow GitHub dort en repli
> (réactivable en une commande). Option B (Render Starter) reste le
> correctif de fond au premier trafic réel payant.

## 1. Constat mesuré (septembre 2026)

Le workflow GitHub Actions `.github/workflows/keepalive.yml` pings
`https://mikcloud.onrender.com/` avec un cron nominal `*/10` (toutes les
10 min). **Cadence réelle observée** (API Actions, 15 derniers runs) :

| Métrique | Valeur |
|---|---|
| Intervalle nominal | 10 min |
| Intervalle réel min / max | 99 min / 288 min |
| Intervalle réel moyen | ≈ 2 h 50 |

**Conclusion : le planificateur GitHub Actions ne tient pas ses engagements
de cadence** (retards plateforme non contractuels, files d'attente
variables). Le service Render hiberne donc entre deux pings : le keep-alive
GitHub ne peut pas, à lui seul, empêcher les cold boots.

Atténuations embarquées dans le workflow (N°40) : deux lignes de cron
décalées **hors des minutes rondes** (pic de congestion du planificateur) —
utile, mais insuffisant seul.

## 1-bis. Couche frontend — résilience cold boot (N°84)

Même quand un cold boot se produit (entre deux pings retardés), l'utilisateur
ne doit plus le subir comme un échec. Deux garde-fous embarqués dans
l'écran de connexion (`frontend/src/components/hotspot/login-screen.tsx`) :

1. **Réveil proactif** — au premier montage de l'écran de connexion, un ping
   silencieux `GET /` (`wakeBackend()` dans `lib/hotspot/api.ts`, idempotent
   par garde module, échec ignoré) part vers le backend : le serveur
   Render démarre **pendant que l'utilisateur tape ses identifiants**.
2. **Rejeu patient du login** — si la soumission échoue au niveau RÉSEAU
   (timeout, connexion — pas une réponse HTTP d'erreur), une unique
   seconde tentative part avec un délai de 75 s et un message explicite
   (« Réveil du serveur cloud en cours… », i18n FR/EN). Un cold boot de
   30-90 s passe donc inaperçu. Le login est le seul POST autorisé à se
   rejouer : rejouer une génération de vouchers ou un e-mail serait un
   double effet de bord (traitement serveur réussi masqué par le timeout).

Ces deux garde-fous rendent les cold boots résiduels quasi invisibles ;
les options A/B ci-dessous restent les correctifs de fond (availability
continue, pas juste tolérance à l'éveil).

## 2. Option A — Monitor externe UptimeRobot (gratuit) — **ACTIVÉE le 2026-09-13**

UptimeRobot Free : **50 monitors, checks HTTP toutes les 5 min, 0 $, sans
carte bancaire**. Indépendant de GitHub → fiabilité réelle de 5 min, ce qui
garde le service Render éveillé en permanence (15 min d'inactivité jamais
atteintes).

### Mise en place (≈ 5 minutes)

1. Créer un compte sur <https://uptimerobot.com> (plan Free).
2. **Add New Monitor** :
   - *Monitor Type* : `HTTP(s)`
   - *Friendly Name* : `MIKCLOUD backend (Render)`
   - *URL* : `https://mikcloud.onrender.com/`
   - *Monitoring Interval* : `5 minutes`
3. (Optionnel) *Advanced* → cocher une notification email pour les pannes —
   double avantage : le monitor devient aussi une **sonde de disponibilité**
   avec historique et alertes.
4. Sauvegarder. Après 2-3 checks (10-15 min), le service Render passe
   `running` et **ne se resuspend plus**.

### Activation effective (2026-09-13)

Le gérant a créé le compte et posé le monitor exactement comme prescrit
ci-dessus (≈ 5 min de manipulation, 0 $). Preuve d'efficacité mesurée le
jour même :

- dernier ping GitHub Actions à **11:06 UTC** (retards plateforme
  habituels — 5 h 17 entre les runs 114 et 115) ;
- à **13:18 UTC**, `GET /` répond **HTTP 200 en 0,25 s** sans cold boot —
  le service est resté éveillé sans l'aide du cron GitHub ;
- structurellement : une cadence de 5 min < seuil d'hibernation de 15 min,
  le service ne peut plus s'endormir.

### Workflow GitHub — désactivé le 2026-09-13 (fait)

Conformément au plan initial, le cron GitHub redondant a été désactivé
(le workflow reste DANS le dépôt comme repli) :

```bash
gh workflow disable keepalive.yml
# ou l'API : PUT /repos/ftechnologies18/mikcloud/actions/workflows/keepalive.yml/disable
# → HTTP 204, état vérifié ensuite : disabled_manually
# (ci.yml et backup.yml restent actifs)
```

Réactivation (si le compte UptimeRobot est un jour abandonné — vérifier
alors l'historique de pannes du monitor avant de le supprimer) :

```bash
gh workflow enable keepalive.yml
```

NB : pousser une modification du fichier `keepalive.yml` ne le réactive
PAS — l'état disabled vit côté GitHub, pas dans le dépôt.

## 3. Option B — Upgrade Render Starter (~7 $/mois)

Le plan **Starter** (`render.com/pricing`) rend le service web **always-on** :
plus de spin-down, plus de cold boot, latence constante — et le keep-alive
devient inutile.

### Mise en place

1. Dashboard Render → service `mikcloud` → **Change Plan → Starter**.
2. Supprimer le workflow GitHub :

```bash
git rm .github/workflows/keepalive.yml
git commit -m "N°XX — Render Starter : suppression du keep-alive devenu inutile"
```

### Quand choisir cette option ?

- Le projet a des utilisateurs réels sensibles à la latence du premier accès
  (cold boot 30-90 s à chaque visite espacée).
- Ou dès qu'un domaine de production série est mis en avant.

> NB (2026-09-13) : l'Option A couvre désormais ce besoin à 0 $ — l'Option B
> redevient pertinente au premier trafic réel payant ou si le gérant veut
> se passer de tout compte tiers.

## 4. Table de décision

| Critère | A — UptimeRobot Free | B — Render Starter |
|---|---|---|
| Coût | 0 $ | ≈ 7 $/mois |
| Cold boots | Éliminés (ping 5 min) | Éliminés (always-on) |
| Dépendance externe | 1 compte tiers | Aucune |
| Sonde/alertes de disponibilité | Inclus (bonus) | Dashboard Render seul |
| Effort | ~5 min (créer le compte) | ~1 min + suppression workflow |

**Recommandation : Option A immédiatement** (gratuit, 5 min) — **faite le
2026-09-13** ; Option B au premier signe de trafic réel payant.

## 5. Vérification du bon fonctionnement

```bash
# Le service répond-il sans cold boot ?
time curl -s -o /dev/null -w '%{http_code}\n' https://mikcloud.onrender.com/
# → HTTP 200 en < 1 s : service éveillé (un cold boot donnerait 30-90 s).

# Historique du monitor UptimeRobot : onglet Response Time / Logs
#   (PRIMAIRE depuis le 2026-09-13 — c'est LA sonde de disponibilité).
# Historique GitHub : onglet Actions → Keep-alive Render
#   (historique jusqu'au 13/09/2026 11:06 UTC — workflow désormais désactivé).
```
