# RUNBOOK — Portail captif automatique MikCloud (N°35)

> Document opérateur MikCloud. Objectif : déployer et maintenir le portail
> captif (login.html, status.html, assets) sur les routeurs MikroTik en mode
> **agent**, sans aucune intervention humaine après le pré-requis one-shot.
> Dernière mise à jour : N°35-d (2026-09-06).

> **⚙️ Automatisation (N°35)** : les routeurs en mode **AGENT** déploient le
> portail automatiquement — commande `hotspot_files` servie au check-in
> (≤ 45 s), idempotente par signature (`Router.HotspotFilesSig`), reprise
> zombie 10 min. Le gérant change sa config dans la console (branding, offres,
> textes) → le portail se rafraîchit en live via `fetch /api/wifi/site/{slug}/portal`
> côté page ; pour forcer un re-déploiement complet (changement de template),
> bouton « Re-déployer » dans la console → vue **Portail**.

## 0. Pré-requis one-shot (à faire une fois par routeur)

Le portail captif MikCloud est servi par le routeur depuis son dossier
`hotspot/`. MikroTik n'a pas de commande « créer un dossier » avant RouterOS 7 ;
l'agent tente `/file mkdir "hotspot"` automatiquement (idempotent), mais il
faut indiquer au profil hotspot que le HTML vit dans ce dossier.

### Étape 1 — Configurer le html-directory (Winbox)

1. Winbox → **IP → Hotspot → Hotspot Setup** (si pas déjà fait).
2. **IP → Hotspot → Profiles** → double-cliquer le profil hotspot (souvent
   `hsprof1`) → onglet **HTML** → champ **HTML Directory** : saisir `hotspot`.
3. **OK**. Le routeur cherchera maintenant `login.html`, `status.html`, etc.
   dans le dossier `hotspot/` au lieu de la racine.

> Sans cette étape, le routeur sert ses pages par défaut (moche, générique,
> parfois EN) et le portail MikCloud déployé par l'agent n'apparaît jamais —
> bien que les fichiers soient bien présents dans `hotspot/`.

### Étape 2 — Activer le mode agent (si pas déjà fait)

Le portail n'est déployé que par les routeurs en mode **agent** (HTTP-poll
sortant). Les modes « simulé » et « réel » ne déploient rien.

1. MikCloud → **Infrastructure → Routeurs → Ajouter** : IP, port 8728,
   identifiants, mode **Agent**.
2. Suivre le script d'installation (token 32 car, scheduler 45 s). Le routeur
   s'inscrit au premier check-in (≤ 45 s) → le walled-garden N°29 est posé
   automatiquement → le portail N°35 est déployé automatiquement.

### Étape 3 — Vérification (optionnelle, première fois)

1. Winbox → **Files** : vérifier la présence du dossier `hotspot/` avec les
   fichiers `login.html`, `status.html`, `md5.js`, `css/`, `js/`, `img/`,
   `webfonts/` (~30 fichiers).
2. MikCloud → **Infrastructure → Portail** : le routeur doit afficher le badge
   **« Portail à jour »** dans les 45 s suivant le check-in. Le journal doit
   contenir « Portail captif déployé sur «<name>» ».
3. Connecter un client de test au WiFi → le portail captif s'ouvre → la page
   MikCloud (branding + bandeau « WiFi offert » + claim inline) s'affiche.

## 1. Architecture — rappel

```
┌────────────────────────────────────────────────────────────┐
│ REPO GITHUB : backend/internal/hotpage/template/            │
│   login.html, status.html, md5.js, css/, js/, img/, webfonts │
│   → //go:embed dans le binaire backend au build              │
└────────────────────────────────────────────────────────────┘
              │ (build Dockerfile Render)
              ▼
┌────────────────────────────────────────────────────────────┐
│ BACKEND CLOUD (Render)                                       │
│   GET /portal/{tokenAgent}/{path} → HTML personnalisé par    │
│   compte (buildPortalConfig + hotpage.Personalize)           │
│   GET /api/wifi/site/{slug}/portal → config LIVE (public)    │
│   POST /api/routers/{id}/redeploy-portal → vide la sig       │
│   GET /api/routers/{id}/portal-preview → HTML pour iframe    │
└────────────────────────────────────────────────────────────┘
              │ (l'agent /tool fetch chaque fichier au check-in)
              ▼
┌────────────────────────────────────────────────────────────┐
│ AGENT MIKCLOUD (sur le routeur, polling 45 s)                │
│   CmdHotspotFiles : ~30 /tool fetch vers /portal/{token}/   │
│   Ordre : assets → status → login.html EN DERNIER          │
│   Idempotence via Router.HotspotFilesSig (16 car hash)      │
│   Reprise zombie 10 min si fetch perdu                      │
│   Trace step (mkdir, fetch-1, fetch-2...) dans le rapport   │
└────────────────────────────────────────────────────────────┘
              │ (fichiers écrits dans hotspot/)
              ▼
┌────────────────────────────────────────────────────────────┐
│ PAGE login.html (servie par le routeur, pré-auth)            │
│   Lit le bloc <script id="mikcloud-config"> (fallback)      │
│   Tente fetch /api/wifi/site/{slug}/portal (config live)    │
│   Live prime sur le fallback (Object.assign merge)          │
│   Claim inline → code → doLogin() CHAP auto → en ligne       │
│   Bouton « S'inscrire » → /join/{token}?mac=$(mac-esc)       │
└────────────────────────────────────────────────────────────┘
```

## 2. Console gérant — vue Portail

Accessible depuis la sidebar : **Infrastructure → Portail**.

### Liste des routeurs agents

Chaque routeur en mode agent affiche :

- **Nom** + mode + statut (online/offline).
- **Badge de signature** :
  - 🟢 **Portail à jour** — `HotspotFilesSig` non vide, déploiement confirmé.
  - 🟡 **Re-déploiement en attente** — sig vidée manuellement, re-déploiement au prochain check-in.
  - ⚪ **Jamais déployé** — sig absente, routeur neuf ou jamais déployé.
- **Boutons** :
  - **Aperçu** — ouvre un dialog avec iframe `srcDoc` du HTML personnalisé
    (fetch `/api/routers/{id}/portal-preview`). Aperçu statique — le fetch
    live n'est pas actif (origine backend ≠ origine routeur).
  - **Re-déployer** — AlertDialog de confirmation → POST
    `/api/routers/{id}/redeploy-portal` → vide la sig côté backend →
    `ensureHotspotFilesLocked` re-file automatiquement au prochain check-in
    (≤ 45 s). Trace l'acteur dans le journal d'activité.

### Journal des déploiements

Liste filtrée du journal d'activité (`/api/activity`), rafraîchie toutes
les 30 s, n'affichant que les événements de type `router` dont le message
contient :
- « Portail captif déployé sur » (déploiement automatique réussi, N°35-a) ;
- « Re-déploiement du portail demandé pour » (action manuelle, N°35-d).

## 3. Quand re-déployer manuellement ?

| Cas | Re-déploiement nécessaire ? |
|---|---|
| Changement de branding (nom, logo, services) | ❌ Non — le fetch live rafraîchit automatiquement |
| Changement d'offres (prix, profils) | ❌ Non — fetch live |
| Changement de textes d'accueil | ❌ Non — fetch live |
| Changement de lien Wave marchand | ❌ Non — fetch live |
| Refonte du template (ajout/suppression d'un asset, nouvelle page) | ✅ Oui — bouton Re-déployer |
| Routeur qui ne déploie pas (sig corrompue) | ✅ Oui — force le re-file |
| Migration de version MikCloud (nouveau template embarqué) | ✅ Oui — pousse le nouveau template |

**Règle** : la signature est calculée sur la LISTE des chemins (pas le
contenu). Un changement de branding seul ne change pas la sig → pas de
re-déploiement (le fetch live gère). Un changement de template (nouveau
fichier, fichier supprimé) change la sig → re-déploiement automatique au
prochain check-in. Le bouton « Re-déployer » force le re-déploiement même
si la sig correspond — utile pour pousser une mise à jour du template
sans attendre un changement de sig.

## 4. Troubleshooting

### 4.1 Le portail ne s'affiche pas sur le routeur

**Symptômes** : le client voit la page de login par défaut de MikroTik
(ou une page 404), pas le portail MikCloud brandé.

**Diagnostic** :
1. MikCloud → **Infrastructure → Portail** : le routeur est-il en mode
   agent et en ligne ?
   - ❌ Mode `simulated` ou `real` → le portail n'est jamais déployé.
     Passer en mode agent (recréer le routeur).
   - ❌ Statut `offline` → le routeur ne check-in plus. Vérifier la
     connectivité (script agent, DNS, firewall). Le portail ne peut pas
     se déployer sans check-in.
2. Si le routeur est agent + en ligne : le badge dit-il « Portail à jour » ?
   - ⚪ **Jamais déployé** → le 1er déploiement n'a pas encore eu lieu.
     Attendre 1 check-in (≤ 45 s). Si toujours « Jamais déployé » après
     5 min, voir §4.2.
   - 🟡 **Re-déploiement en attente** → normal après un re-déploiement
     manuel. Attendre le check-in.
3. Winbox → **Files** : le dossier `hotspot/` contient-il les fichiers ?
   - ❌ Dossier absent → l'agent n'a pas pu créer le dossier. Voir §4.3.
   - ❌ Fichiers présents mais portail par défaut → pré-requis
     `html-directory` non configuré. Voir §0 étape 1.
4. MikCloud → **Infrastructure → Portail → Journal** : cherche un événement
   « Portail captif déployé sur «<name>» ». Si absent, le déploiement
   n'a pas réussi — voir §4.2 pour le diagnostic du `step`.

### 4.2 Le déploiement échoue — lire le `step`

Chaque commande `hotspot_files` porte une variable RouterOS `step` mise à
jour avant chaque bloc à risque (pattern N°32). Le rapport d'erreur
embarque `&step=" . $step` → on sait QUELLE ligne a échoué sans accès
console au routeur.

Le journal d'activité (vue Portail) peut afficher :
- « Portail captif déployé sur «<name>» (N fichier(s)) » — succès.
- « Commande hotspot_files ÉCHOUÉE sur «<name>» (echec_sur_le_routeur) » —
  échec. Le `step` est dans le rapport brut (visible dans les logs Render,
  pas dans le journal d'activité console).

**Valeurs de `step`** :
| `step` | Cause probable | Action |
|---|---|---|
| `mkdir` | `/file mkdir "hotspot"` a échoué (RouterOS < 7 ou dossier read-only) | Vérifier la version RouterOS (≥ 7.19 requis pour TLS strict). Créer le dossier manuellement : Winbox → Files → New → Directory → `hotspot`. |
| `fetch-N` | Le `/tool fetch` du N-ième fichier a échoué (TLS, DNS, réseau) | Vérifier la connectivité du routeur vers le cloud. Vérifier le walled-garden N°29 (les hôtes cloud doivent être autorisés pré-auth). Vérifier la version RouterOS (≥ 7.19 pour les certificats Let's Encrypt). |
| `done` | Tous les fetchs ont réussi, le rapport final est en cours | Pas une erreur — attendre le rapport final. |

### 4.3 Le dossier `hotspot/` n'existe pas

L'agent tente `/file mkdir "hotspot"` automatiquement (idempotent). Si ça
échoue :

1. **RouterOS < 7** : `/file mkdir` n'existe pas. Mettre à jour le
   routeur (System → Packages) vers RouterOS ≥ 7.19 (requis aussi pour le
   TLS strict). Puis redéployer.
2. **RouterOS ≥ 7** mais dossier read-only (flash pleine, système de
   fichiers corrompu) : Winbox → Files → vérifier l'espace libre. Si
   < 5 Mo, supprimer les anciens logs (`/log print` → clear). Redéployer.
3. **Création manuelle** (dépannage) : Winbox → Files → New → Directory →
   saisir `hotspot`. L'agent reprendra au prochain check-in.

### 4.4 Le portail s'affiche mais sans branding

**Symptôme** : la page login.html est bien celle de MikCloud (présence du
formulaire claim, du bandeau « WiFi offert ») mais le tenant name est
« MikCloud » (défaut) au lieu du vrai nom, et le logo est absent.

**Cause** : le fetch live `/api/wifi/site/{slug}/portal` échoue (le
navigateur du client n'arrive pas à joindre le cloud). Le fallback inliné
est utilisé — il contient la config au moment du déploiement, qui peut
être périmée si le branding a changé depuis.

**Diagnostic** :
1. Vérifier le walled-garden N°29 — l'hôte `mikcloud.onrender.com`
   (ou le backend Render) doit être autorisé pré-auth. Si absent, le
   navigateur du client ne peut pas faire le fetch.
2. Vérifier le DNS côté routeur : `mikcloud.onrender.com` doit résoudre.
   Le walled-garden DNS (règles udp/tcp 53 comment `mikcloud-wg dns`)
   doit être en place.
3. Si le cloud est injoignable (panne Render, DNS cassé), le fallback
   inliné prend le relais — c'est le comportement attendu. Le portail
   reste fonctionnel, juste moins frais.

### 4.5 Le claim inline échoue

**Symptôme** : le client saisit son numéro, clique « Recevoir », voit
« Service WiFi offert momentanément indisponible — demandez votre code au
personnel ».

**Cause** : le `fetch` POST `/api/wifi/site/{slug}/claim` échoue. Le
script affiche ce message d'erreur spécifique quand le réseau est
injoignable (différent d'une erreur métier comme `phone_cap`).

**Diagnostic** :
1. Vérifier le walled-garden N°29 (hôte backend autorisé pré-auth).
2. Vérifier que le routeur est en ligne et que le site WiFi est actif.
3. Vérifier les rate-limits `wifi-claim` (6/min/IP) — si le client a
   spammer le bouton, il peut être temporairement bloqué (429).
4. Si le cloud est injoignable, le client doit demander son code au
   personnel (contournement manuel).

## 5. Maintenance — mettre à jour le template

Le template vit dans le repo : `backend/internal/hotpage/template/`.
Modifier un fichier (login.html, status.html, ajouter un asset) +
commit + push → CI → déploiement Render. Le nouveau template est
embarqué dans le binaire backend via `//go:embed`.

Pour que les routeurs récupèrent le nouveau template :
- Si la LISTE des chemins a changé (nouveau fichier, fichier supprimé) →
  la sig change automatiquement → re-déploiement au prochain check-in.
- Si seul le CONTENU a changé (même liste de chemins) → la sig ne change
  pas → utiliser le bouton « Re-déployer » dans la console pour forcer.

## 6. Sécurité — rappels

- **Token agent** 32 car (192 bits), haché SHA-256 côté cloud. Jamais
  en clair en DB. Jamais exposé après la création/rotation.
- **TLS strict** : `/tool fetch` hérite du TLS strict (RouterOS ≥ 7.19
  requis). Pas de `check-certificate=no`.
- **Idempotence** : la signature `HotspotFilesSig` empêche les
  re-déploiements inutiles. Un routeur à jour ne re-déploie pas.
- **Reprise zombie** : une commande `sent` sans rapport depuis > 10 min
  est re-filée automatiquement (N°31).
- **Anti-énumération** : `/portal/{token}/{path}` retourne 404 sans
  révéler la structure pour un token invalide.
- **CSP** : l'aperçu console porte `frame-ancestors 'self'` pour
  empêcher le clickjacking de l'iframe.
- **Pas de secret dans le PortalConfig** : le JSON embarqué dans
  login.html (et renvoyé par `/api/wifi/site/{slug}/portal`) ne contient
  que des infos publiques (tenant name, slug, offres, wave link, logo
  URL). Pas de token agent, pas de mots de passe.

## 7. Limites assumées

- **Pas d'extracteur ZIP natif RouterOS** : l'agent fait un
  `/tool fetch` par fichier (~30 fetchs par déploiement). Chaque fetch
  est un point de défaillance potentiel, mitigé par `step` + `on-error`
  + reprise zombie.
- **Aperçu console statique** : le fetch live n'est pas actif dans
  l'iframe d'aperçu (origine backend ≠ origine routeur). L'aperçu montre
  le fallback inliné — suffisant pour valider le branding + structure.
- **`html-directory` manuel one-shot** : ne peut pas être automatisé sans
  risque (multi-profils hotspot possibles sur un même routeur). Documenté
  dans ce runbook.
- **Pas de quota MAC côté WiFi Jetable** (cf. audit C2) : le claim inline
  utilise `DailyPerPhone` + `DailyCap` + rate-limit IP. Le quota MAC N°33
  n'est actif que pour l'inscription publique (via le bouton
  « S'inscrire » qui injecte `?mac=$(mac-esc)`).
