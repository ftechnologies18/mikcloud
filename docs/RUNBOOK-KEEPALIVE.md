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
>
> **STATUT (2026-09-29, N°194) : le mur des 750 heures — voir §6.** Alerte
> e-mail Render reçue le 29/09 (627/750 h d'instance du mois). Verdict :
> AUCUNE interruption avant le reset du 1er octobre (marge ~80 h), mais
> chaque mois de 31 jours jouera le ras du plafond (~744 h nécessaires,
> 0,8 % de marge). Option B (Starter) : la réponse structurelle, à prendre
> au premier client payant — ou dès que la règle du 29/10 (§6.5) la
> déclenche.

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
>
> NB (2026-09-29, N°194) : l'alerte « 750 heures d'instance » du plan free
> (§6) donne un NOUVEAU déclencheur à cette option : au-delà des cold boots,
> c'est le seul moyen d'éliminer le PLAFOND D'HEURES — un backend de
> supervision 24/7 consomme ~744 h sur tout mois de 31 jours, soit 99,2 %
> du budget gratuit du workspace.

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

## 6. Le mur des 750 heures d'instance — alerte Render du 29/09/2026 (N°194)

### 6.1 Le mail (données mesurées)

> « You're approaching the monthly usage limit for free web services —
> Your workspace has used **627 of its 750 free instance hours** this
> month. If you run out of free instance hours, your free web services
> will be suspended for the rest of the month: **mikcloud**. These
> services will resume automatically at the start of the next calendar
> month when your free usage resets. To make sure a service remains
> active, upgrade it to any paid instance type from its Settings page in
> the Render Dashboard. »

Le plan FREE de Render plafonne le **workspace** (pas le service) à
**750 heures d'instance par mois calendaire** — reset le 1er à 00:00 UTC
(= minuit local Abidjan, même fuseau). `mikcloud` est l'unique service free
du workspace (le mail ne liste que lui ; le scheduler `mikcloud-quota` vit
sur les ROUTEURS en RouterOS — il ne consomme rien chez Render).

### 6.2 Verdict immédiat : AUCUNE interruption avant le 1er octobre

Mesuré le 29/09 05:42 UTC :

- Reste à couvrir : **42,5 h** (29/09 05:42 → 01/10 00:00 UTC) ;
- Budget restant : 750 − 627 = **123 h** ;
- **Marge : ~80 h (≈ 2× le temps restant)** → le service tiendra jusqu'au
  reset SANS RIEN FAIRE. Le mail est le warning automatique de Render
  (déclenché vers 84 % du quota), pas une suspension.

### 6.3 Le mur structurel : chaque mois de 31 jours joue le ras du plafond

Un backend de supervision hotspot doit rester joignable 24/7 : les
check-ins agents (45-180 s, N°75) ne laissent JAMAIS 15 min sans trafic →
jamais d'hibernation (l'Option A verrouille ce comportement, mais les
agents le garantiraient seuls — retirer UptimeRobot ne gagnerait ~rien).

| Mois | Heures nécessaires (24/7) | Plafond | Marge |
|---|---|---|---|
| 30 jours (novembre) | 720 h | 750 h | 30 h (4 %) |
| 31 jours (octobre, décembre…) | **744 h** | 750 h | **6 h (0,8 %)** |

Septembre n'a consommé que 627 h parce que le service hibernait par
fenêtres avant le monitor UptimeRobot (13/09) — le ping GitHub réel
mesuré ~2 h 50 (§1) laissait le service dormir (écart ≈ 50 h vs le 24/7
intégral, cohérent). **Octobre sera le premier mois PLEIN sans aucune
hibernation : la consommation convergera vers 744 h.**

Le plafond facture la **PRÉSENCE**, pas le travail : même famille de mur
que le Neon N°162 (temps d'éveil vs volume). Les optimisations
N°72-77 + N°157/N°159 (bande passante, lignes, CPU) et la migration
Supabase (persistance) n'ont AUCUN levier sur des heures d'instance — et
leurs promesses restent tenues par ailleurs (base 8,6 % du quota Supabase,
egress applicatif projeté ~45 Mo/31 j vs 5 Go, mesurés N°179).

### 6.4 Ce que signifie une suspension (si le mur tombait un jour)

- **Frontend Vercel** : reste UP (statique) mais la console est inutilisable
  (API injoignable) ;
- **Portail captif** : la page de login est servie PAR LE ROUTEUR (repli
  local N°75) → elle s'affiche toujours, mais la soumission du login
  valide côté cloud → **aucun NOUVEAU client ne peut se connecter**
  pendant la fenêtre. Les sessions déjà établies continuent (elles vivent
  sur RouterOS) ;
- **Agents** : check-ins en échec, retry continu — sans perte (comportement
  prouvé par l'incident N°163 : les deltas repartent à la reprise). À la
  reprise, le boot recharge l'état complet depuis Supabase (chemin de boot
  normal, sain depuis la migration) — c'est LA différence avec l'ère Neon :
  une suspension est aujourd'hui **survivable**, pas fatale ;
- **Comptoir / ventes / e-mails transactionnels** : morts pendant la
  fenêtre ;
- Reprise AUTOMATIQUE au 1er du mois suivant 00:00 UTC.

### 6.5 Décision recommandée (règle simple)

1. **Maintenant → 1er octobre : ne rien faire** (§6.2). Ne PAS retirer le
   keep-alive : les agents maintiennent l'éveil de toute façon (gain ~0)
   et le retrait rouvrirait la porte aux cold boots si le parc agent se
   vidait un jour.
2. **Octobre : laisser filer, avec une règle de sortie datée.** Le compteur
   se lit sur le dashboard Render (heures d'instance du workspace) ; le
   mail de warning reviendra vers ~84 % (≈ 25-27/10). **Règle : au matin
   du 29/10, projeter `heures consommées + heures restantes jusqu'au
   reset` — si la projection ≥ 745 h, upgrader immédiatement en Starter**
   (facturé à l'usage : ~1-2 $ pour les dernières heures) OU assumer une
   fenêtre de suspension le 31/10 en soirée — le 31/10/2026 est un
   SAMEDI (soirée Halloween : potentiellement le pire moment pour couper
   des hotspots).
3. **Réponse structurelle (au premier client payant) : Option B — Render
   Starter ~7 $/mois** (§3) : élimine le plafond d'heures ET le spin-down ;
   le monitor UptimeRobot devient alors inutile (à retirer). Avec Render
   PostgreSQL Starter (~6-7 $) pour la base au même moment
   (RUNBOOK-POSTGRES §4-D) : **~14 $/mois** le tout — la trajectoire
   documentée depuis le 13/09.

NB : une « pause nocturne planifiée » (suspendre le service la nuit pour
économiser des heures) est envisageable sur le papier mais INACCEPTABLE
dès qu'un gérant vend des forfaits 24 h (un ticket acheté à 20 h doit
connecter à 2 h du matin) — à réserver à un parc strictement diurne, et
le comportement exact du compteur sur suspend/resume manuel n'est pas
mesuré.
