# RUNBOOK — Keep-alive Render (anti-hibernation)

> Contexte : le backend Go tourne sur le plan **FREE de Render**, qui suspend
> le service après **~15 min sans trafic**. Le redémarrage à froid qui suit
> coûte **30 à 90 s** au premier visiteur (boot conteneur + rechargement
> complet de l'état depuis Neon).

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

## 2. Option A — Monitor externe UptimeRobot (gratuit, recommandé)

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

### Après activation

Désactiver le workflow GitHub (devenu redondant) :

```bash
gh workflow disable keepalive.yml
# ou : onglet GitHub → Actions → Keep-alive Render → ⋯ → Disable workflow
```

Ne pas le supprimer du dépôt : il sert de repli si le compte UptimeRobot est
abandonné (il suffit de le réactiver).

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

## 4. Table de décision

| Critère | A — UptimeRobot Free | B — Render Starter |
|---|---|---|
| Coût | 0 $ | ≈ 7 $/mois |
| Cold boots | Éliminés (ping 5 min) | Éliminés (always-on) |
| Dépendance externe | 1 compte tiers | Aucune |
| Sonde/alertes de disponibilité | Inclus (bonus) | Dashboard Render seul |
| Effort | ~5 min (créer le compte) | ~1 min + suppression workflow |

**Recommandation : Option A immédiatement** (gratuit, 5 min), Option B au
premier signe de trafic réel.

## 5. Vérification du bon fonctionnement

```bash
# Le service répond-il sans cold boot ?
time curl -s -o /dev/null -w '%{http_code}\n' https://mikcloud.onrender.com/
# → HTTP 200 en < 1 s : service éveillé (un cold boot donnerait 30-90 s).

# Historique du monitor UptimeRobot : onglet Response Time / Logs.
# Historique GitHub : onglet Actions → Keep-alive Render
#   (résumé de run : statut + latence mesurée depuis N°84).
```
