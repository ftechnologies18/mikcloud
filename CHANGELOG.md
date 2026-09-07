# CHANGELOG — MikCloud

Historique des évolutions notables du projet. Format inspiré de
[Keep a Changelog](https://keepachangelog.com/) ; les versions correspondent
aux dates de livraison — le déploiement est continu : chaque push `main` passe
la CI puis se déploie automatiquement (frontend Vercel, backend Render).

## 2026-09-08 — N°63 : Création de site WiFi en wizard 2 étapes animé

### N°63 — « Nouveau site WiFi » : le formulaire plat d'un bloc devient un wizard explicite (vue WiFi Jetable)

- **Constat** : la création/édition d'un site WiFi Jetable se faisait dans
  un dialog d'UN BLOC de ~11 champs (nom, routeur, profil, temps, data,
  limites anti-abus ×3, SSID, mot de passe, 2 switches) — un mur scrollable
  (`max-h-[90vh] overflow-y-auto`) où les trois champs REQUIS se perdaient
  au milieu des réglages optionnels.
- **Wizard 2 étapes** (`parts/wifi-site-wizard.tsx`) :
  - **Étape 1 « Le site »** — l'identité : nom, routeur, profil (les trois
    requis). Bouton « Continuer » **grisé** tant que le socle est invalide
    (validation live + coches vertes + erreur inline au blur sur le nom),
    hints explicites si le compte n'a encore aucun routeur/profil ;
  - **Étape 2 « L'offre »** — les réglages : quotas offerts, protections
    anti-abus, réseau WiFi (SSID/mot de passe), switches — précédée d'un
    **récapitulatif en puces** des choix de l'étape 1 (nom · routeur ·
    profil) : le gérant voit son socle sans revenir en arrière.
- **Animé** : stepper à connecteur qui se remplit (500 ms) et jalon 1
  basculant en **✓ teal** dès l'étape 2 franchie ; transition
  **directionnelle** entre étapes (slide avant en continu, slide inverse au
  retour — `AnimatePresence` + `custom` direction) ; champs en cascade
  (stagger 50 ms, cohérent avec le signup-modal) ; `useReducedMotion`
  respecté (fondu seul, aucune translation).
- **Explicite** : description du dialog par étape + mention « Étape n/2 »,
  boutons « Continuer » / « Retour » / « Enregistrer » (édition) — Entrée
  valide l'étape courante (form natif par étape).
- **Chirurgical** : le payload POST/PUT est **identique à l'ancien dialog
  plat** (12 champs, nom trimmé au submit — payload capturé au smoke) ;
  l'état du formulaire reste chez le parent (`wifi-view.tsx`), le wizard
  est remonté vierge à chaque ouverture via une clé nonce (aucun setState
  en effet — règle react-hooks/set-state-in-effect). Création ET édition
  passent par le même wizard (pré-remplissage intact).
- **Périmètre** : 2 fichiers touchés + 1 créé, +12 clés i18n FR/EN
  (`wifi.wiz.*`), zéro changement backend, zéro dépendance.

## 2026-09-08 — N°62 : Inscription publique /join simplifiée et interactive

### N°62 — 4 champs au lieu de 6, un formulaire qui réagit à chaque frappe (public /join/[token])

- **Simplification demandée** : les champs **« Message (facultatif) »** et
  **« Confirmer le mot de passe »** sont supprimés. Le contrat backend
  n'exigeait ni l'un ni l'autre (le message y a toujours été optionnel, la
  confirmation n'existait que côté client) — le POST n'envoie plus la clé
  `message` : **zéro changement backend**, CONTRACT inchangé. Le nombre
  d'interactions demandées au visiteur du QR code passe de 6 champs à 4.
- **Compensation de la confirmation supprimée** (anti-coquille) :
  jauge de **robustesse du mot de passe** (4 segments animés + libellé
  Faible/Moyen/Solide/Excellent, rouge tant que le plancher de 8 n'est pas
  atteint) et bouton **« copier le mot de passe »** directement dans le
  champ (icône passe à ✓ 1,5 s, toast de confirmation) — le visiteur peut
  sauvegarder son mot de passe dès sa saisie, avant même de soumettre.
- **Interactivité** : validation **live au blur** (champ non vide →
  même schéma zod que la soumission, erreur qui apparaît/disparaît en
  direct), **barre de progression** `0/4 → 4/4` (role="progressbar"
  i18n, largeur animée), **coche verte** par champ valide, **focus +
  scroll automatiques sur la première erreur** à la soumission (fini la
  chasse à l'erreur à l'aveugle sur mobile), erreurs inline animées
  (framer-motion, 180 ms), indices `enterKeyHint` clavier mobile
  (next / go), bouton de soumission tactile (active:scale).
- **Modernisation visuelle** : icônes de tête de champ (User / Phone /
  AtSign / KeyRound), cibles 48 px, bloc « mode de connexion + notice »
  fusionné en une carte compacte teal, carte portée (shadow-lg).
- **Correction d'incohérence repérée au passage** : le placeholder du mot
  de passe annonçait « 6 caractères minimum » alors que la règle (backend
  N°33 et message d'erreur) est **8** — placeholder corrigé FR/EN.
- **Chirurgical** : 2 fichiers frontend (`join-form.tsx`, `i18n.ts`),
  +314/−170. Clés i18n obsolètes retirées (`confirmPassword*`, `message*`,
  `err.confirm`, `err.message` — FR + EN), nouvelles clés (robustesse,
  copie, progression). Écrans post-soumission, kiosque N°33, honeypot
  anti-bots, MAC anti-abus et quotas : inchangés.

## 2026-09-08 — N°61 : Mode Vente offline AU LANCEMENT — repli shell /sell + navigation bornée 4 s

### N°61 — Le comptoir s'ouvre sans réseau, en tournée comme au comptoir (audit PWA, plan d'action 3/3 — P1)
- **Constat (audit)** : le Mode Vente était vendable hors ligne (file
  IndexedDB 409-safe + snapshots localStorage, N°8/UX R6) mais NON
  DÉMARRABLE : la navigation `/sell` était network-first avec repli
  `offline.html` — page générique « hors ligne », pas le comptoir. Et
  sur un réseau captif MikroTik non authentifié, le fetch TCP/TLS peut
  PENDRE 75 s+ : le revendeur restait sur un écran vide au lieu de son
  comptoir. Les snapshots ne servaient que si l'app était déjà ouverte.
- **Navigation /sell enrichie (SW)** : network-first BORNÉE à 4 s (race
  `fetch` vs timeout) avec repli sur le **shell HTML /sell en cache** —
  jamais `offline.html` pour le Mode Vente. Trois cas : réseau OK (≤ 4 s)
  → réponse servie ET copie fraîche mise en cache pour le prochain
  lancement hors ligne (réponse 2xx directe non-redirigée uniquement) ;
  réseau tombé → repli IMMÉDIAT ; réseau captif pendu → repli à 4 s. Le
  shell pré-rendu (`/sell` est une route statique) est pré-caché à
  l'installation du SW et hydraté normalement — snapshots + file
  IndexedDB prennent le relais côté client (timeout API 10 s déjà en
  place sur me/stock/ventes/replay). Bénéficie directement à l'icône
  PWA et au raccourci « Mode Vente » (N°60).
- **Chirurgical** : les AUTRES navigations gardent le comportement du
  N°8 (network-first → `offline.html`) — un timeout global les
  dégraderait inutilement sur réseau lent légitime (2G : une page peut
  légitimement prendre > 4 s).
- **Garde défensive (`isSamePasswordMode`)** : le comptoir offline
  démarre sur le snapshot localStorage — une entrée non conforme
  (écriture partielle, contrat futur) ne doit jamais faire planter le
  lancement hors-ligne au moment où le revendeur a besoin de vendre :
  `password` absent → `false` (ligne mot de passe affichée vide, en
  dégradé) au lieu d'un `TypeError` fatal.
- **Frontend only** : aucun changement backend, aucune migration Neon,
  CONTRACT-V2 inchangé.

## 2026-09-08 — N°59 : PWA « robustesse » — cache versionné par déploiement, stockage persistant, SW toujours frais

### N°59 — Sécurise la durée de vie de la PWA Mode Vente (audit PWA, plan d'action 1/3 — P0)
- **Constat (audit)** : quatre fragilités de long terme. (1) Le cache SW
  `"mikcloud-v2"` était FIGÉ : `public/sw.js` ne changeait jamais entre
  déploiements → jamais de byte-diff → jamais de ré-activation → le
  Cache Storage grossissait sans borne (bundles `/_next/static/<hash>`
  remplacés à chaque build, jamais purgés). (2) Sans
  `navigator.storage.persist()`, le navigateur peut ÉVICTIONNER
  l'IndexedDB sous pression disque — et emporter la file des ventes hors
  ligne (de l'argent réel). (3) Chrome ne vérifie le SW qu'au plus
  1×/24 h : un déploiement n'était visible au comptoir que le lendemain.
  (4) iOS < 15.4 n'ouvre l'app installée en standalone que via la meta
  historique `apple-mobile-web-app-capable`, que Next 16 n'émet plus
  (seule la forme moderne `mobile-web-app-capable` est servie — observé
  en production).
- **SW versionné par déploiement** : `public/sw.js` (statique) est
  remplacé par une ROUTE HANDLER `src/app/sw.js/route.ts` (`force-static`,
  pré-rendue au build) qui génère le script avec le SHA du déploiement
  (`VERCEL_GIT_COMMIT_SHA`) : chaque déploiement produit un sw.js
  différent → byte-diff → réinstallation → l'`activate()` EXISTANT purge
  les caches des versions précédentes (croissance bornée). Stratégies
  inchangées à 100 % (jamais `/api`, navigation network-first →
  offline.html, statiques cache-first — vérifié par diff local : seul
  `const CACHE` change) ; `Cache-Control: no-cache, must-revalidate`
  explicite sur la réponse.
- **Stockage persistant** : `ensureStoragePersisted()` (offline-queue.ts)
  — `navigator.storage.persist()` silencieux et idempotent, appelé au
  register du SW ET à chaque mise en file d'une vente hors ligne (le
  moment précis où les données deviennent critiques). API absente ou
  refus du navigateur = no-op strict, l'UX ne change jamais.
- **SW toujours frais** : `registration.update()` au retour de visibilité
  et au retour du réseau (throttle 30 min — un update sans changement
  n'est qu'un GET conditionnel) : un déploiement devient visible au
  premier rallumage d'écran, pas le lendemain.
- **iOS < 15.4** : meta `apple-mobile-web-app-capable` explicite dans le
  layout (hisée dans le `<head>` par React 19) aux côtés de la forme
  moderne émise par Next — redondance voulue, ignorée des navigateurs
  récents.
- **Frontend only** : aucun changement backend, aucune migration Neon,
  CONTRACT-V2 inchangé.

## 2026-09-08 — N°60 : PWA « installation riche » — manifest complet + CTA « Installer » in-app

### N°60 — Transforme l'installation PWA en parcours first-class (audit PWA, plan d'action 2/3)
- **Constat (audit)** : la PWA est installable depuis le N°8, mais
  l'installation reste un parcours au hasard — mini-infobar Chrome
  (supprimée définitivement par Chrome après quelques rejets), aucun
  accompagnement iOS (Safari n'émet AUCUN événement d'installation),
  manifest sans identité ni captures : Android n'affiche jamais le
  dialogue d'installation riche.
- **Manifest complet** : `id: "/"` (identité STABLE — un futur changement de
  `start_url`/`scope` ne dupliquera plus l'icône chez les revendeurs déjà
  installés) ; `launch_handler: focus-existing` (un lien MikCloud ouvert
  depuis WhatsApp focus l'instance existante plutôt qu'une nouvelle
  fenêtre) ; **3 captures d'écran réelles** de la production servies depuis
  `/screenshots/` (login mobile 780×1688, vitrine mobile 780×1688 —
  `form_factor: narrow`, login desktop 1280×800 — `wide`) : Android
  affiche désormais le dialogue d'installation riche ; **2 raccourcis**
  long-press sur l'icône : « Mode Vente » (/sell) et « Console » (/app),
  routes gardant leurs redirections d'authentification.
- **CTA « Installer » in-app** (`pwa-install-cta.tsx`) : l'événement
  `beforeinstallprompt` est capté AVANT l'hydratation (script inline du
  layout, slot `window.__mikBip` — l'événement peut partir avant le montage
  React, aucun n'est perdu) → bouton natif Chrome/Edge/Android ;
  **feuille d'instructions iOS** (Partager → « Sur l'écran d'accueil » →
  Ajouter, 3 étapes iconifiées) car Safari n'émet aucun événement ;
  détection iPadOS 13+ (Mac desktop masqué par multi-touch) ; toast de
  confirmation sur `appinstalled` ; rejet persistant (localStorage —
  l'utilisateur garde la main, l'installation via le menu du navigateur
  reste possible). Emplacements : login (funnel commun) ET en-tête du Mode
  Vente — le revendeur au token persistant ne repasse jamais par le login.
- **Plein écran sur encoches** : `viewport-fit: cover` + utilitaires
  `env(safe-area-inset-*)` (globals.css) appliqués aux shells mobiles
  (login, Mode Vente) — la barre `black-translucent` du N°8 cesse de
  chevaucher le contenu. Sans viewport-fit, `env()` vaut 0 : zéro effet en
  navigateur classique.
- **Accessibilité** : `maximum-scale: 1` retiré du viewport (le zoom pincé
  est rétabli — conforme WCAG 2.1 AA 1.4.4 ; iOS l'ignorait déjà).
- **Frontend only** : aucun changement backend, aucune migration Neon,
  CONTRACT-V2 inchangé.

## 2026-09-08 — N°57-g : sidebar principale réorganisée — 4 catégories homogènes (Supervision / Hotspot / Personnes / Analyse)

### N°57-g — Réorganisation experte des vues et catégories de la navigation métier (demande utilisateur)
- **Constat** : « Exploitation » était un fourre-tout (5 items mêlant
  supervision temps réel, annuaire clients et produit) ; « Facturation &
  Ventes » portait un libellé mensonger depuis N°57-e (plus de facturation
  dans la nav — l'Abonnement vit en zone Paramètres — et le WiFi jetable
  n'est pas un canal de vente mais un mode d'accès) ; le couplage métier
  Vouchers ↔ Profils (le profil définit ce que le voucher vend) était
  ignoré, et les deux annuaires humains (clients / revendeurs) éclatés.
- **4 catégories dont l'ordre suit le parcours d'usage** (surveiller →
  vendre l'accès → gérer les gens → analyser) :
  1. **Supervision** — Tableau de bord · Sessions actives (le temps réel) ;
  2. **Hotspot** — Vouchers · Profils · WiFi Jetable : LE produit et ses
     trois façons de délivrer de l'accès (prépayé, forfait, offert) —
     réutilise la clé i18n existante `nav.section.hotspot` ;
  3. **Personnes** — Utilisateurs · Revendeurs : les deux annuaires humains
     du business (clients finaux qui se connectent, partenaires qui
     écoulent) — nouvelle clé `nav.section.people` (FR « Personnes », EN
     « People ») ;
  4. **Analyse** — Rapports · Journal · Comptes (comprendre et auditer).
- **Chaque section garde 2-3 items scannables** (groupes repliables O
  inchangés) ; la palette ⌘K suit automatiquement le nouvel ordre (elle
  rend `navItemsFor`, la même source).
- **Contrat strictement inchangé** : ViewIds, icônes, garde-fous de rôles
  (`canView`, comptes admin plateforme filtré), zone Paramètres N°57-c-f,
  console plateforme — pure réorganisation présentationnelle. Seul effet
  bord bénin : l'état replié localStorage (persisté par libellé de section)
  repart ouvert pour les nouvelles catégories.
- **Frontend only** : aucun changement backend, aucune migration Neon.

## 2026-09-08 — N°57-f : l'entrée « Paramètres » quitte la sidebar principale (accès unique : menu utilisateur)

### N°57-f — Retrait de l'entrée nav « Paramètres » (demande utilisateur : déjà présente dans le menu utilisateur)
- **Section « Système » retirée de la sidebar principale** : l'entrée
  « Paramètres » y doublonnait le menu utilisateur (UserCard en pied de
  sidebar desktop, menu profil du topbar sur mobile), qui devient LE point
  d'entrée de la zone — moins de redondance, une sidebar 100 % modules
  métier (exploitation, commercial, analyse).
- **Menu utilisateur inchangé** : « Paramètres » (icône engrenage) y ouvre
  la zone via `settingsLandingView(role)` — le propriétaire atterrit sur
  Général, le gérant sur sa première section accessible (Hotspot) ; en
  mode plateforme il ouvre les paramètres plateforme. La UserCard reste
  visible en zone (pied de sidebar) : on peut ré-atterrir dans la zone
  sans repasser par le Retour.
- **Palette de recherche (⌘K) alignée** : elle reflète la sidebar
  (modules métier uniquement) — « Paramètres » n'y figure plus ; la
  recherche de vues de configuration passe par les URLs directes
  (deep-linkables, historique de la palette).
- **Zone inchangée** : le pattern N°57-c (substitution de sidebar + bouton
  Retour), les 7 sections N°57-d/e, les chemins canoniques et legacy, les
  garde-fous de rôles (client et serveur) et le bandeau d'abonnement du
  dashboard (CTA direct) restent en l'état — seule la porte d'entrée de la
  nav principale disparaît.
- **Nettoyage** : branche `settings` de `navItemsFor` et clauses de
  surlignage/atterrissage associées (NavList) retirées — code mort sinon.
- **Frontend only** : aucun changement backend, aucune migration Neon —
  ViewIds, map VIEWS et contrat de rôles inchangés.

## 2026-09-08 — N°58 : régulariser le solde d'un revendeur prépayé — le remboursement manquant

### N°58 — Action « Régulariser le solde » : boucler la boucle de suppression d'un revendeur portefeuille chargé
- **Constat** : le garde-fou V1 (audit revendeurs) refuse la suppression d'un
  revendeur non soldé (409 « reseller_not_settled » : crédit restant, créance
  dépôt-vente, stock en attente) — juste et immuable. Mais le backend sait
  débiter un portefeuille (`POST /api/resellers/{id}/credit` accepte les
  montants négatifs, contrôle « Crédit insuffisant », journalise une
  Transaction + entrée d'activité « Débit de X FCFA ») alors que la console
  ne propose AUCUN chemin d'écriture : le dialogue « Recharger » filtre les
  montants ≤ 0. La seule issue était un appel API à la main — irréaliste
  pour un gérant.
- **Action « Régulariser le solde »** (menu ⋮ d'un revendeur **prépayé dont
  le crédit > 0** + raccourci sur sa carte, miroir du bouton « Encaisser »
  des cartes dépôt-vente) : dialogue pré-rempli au solde entier — le but est
  de ramener le crédit à zéro (remboursement au revendeur), dernière étape
  avant une suppression possible. Montant libre borné au solde (bouton
  « Tout rembourser »), note optionnelle (défaut backend : « Débit manuel »),
  aperçu « Crédit après remboursement », rappel du garde-fou dans le
  dialogue. Appel : même endpoint `/credit` avec montant négatif — aucune
  route nouvelle, aucun changement de contrat.
- **Cohérence comptable conservée** : le flux passe par la Transaction
  existante (type « credit », montant négatif — visible dans le journal du
  bas de page avec le signe −) et l'entrée d'activité ; le toast final
  affiche le crédit restant ; cache revendeurs + transactions invalidé.
- **Frontend only** : aucun changement backend (l'endpoint est conforme au
  CONTRACT-V2 depuis l'audit V1), Vercel seul — zéro déploiement Render,
  aucune migration Neon.

## 2026-09-08 — N°57-e : l'Abonnement rejoint la zone Paramètres (7ᵉ section, ex /app/subscription)

### N°57-e — Déplacement de la page Abonnement dans la zone Paramètres (demande utilisateur)
- **Nouvelle section « Abonnement »** dans la sidebar de zone :
  `/app/settings/subscription` (segment imbriqué, view-path). La vue
  intégrale (carte statut + formules + historique de facturation, flux de
  renouvellement Wave) vit désormais dans la zone, comme les autres
  préoccupations back-office. Position : AVANT Équipe — l'ordre se lit
  « identité → service → sécurité → infrastructure → alertes →
  facturation → équipe ».
- **Retrait de l'entrée nav dédiée** (section « Commercial » de la sidebar
  principale) : la facturation n'est plus un module métier de première
  ligne ; l'entrée unique « Paramètres » du menu Système y conduit. La
  pastille de statut « anti-churn » de la sidebar principale est retirée
  avec elle (le statut reste visible via le bandeau du dashboard et le mur
  P5 PaywallOverlay).
- **Général allégé** : la carte pont « Abonnement » (lecture + bouton
  Gérer, créée en N°57-d quand la vue vivait hors zone) est retirée —
  redondante avec la section sœur. Le Général reste Organisation + Langue.
- **Bandeau dashboard corrigé** : le CTA « Renouveler » (expiré / échéance
  proche) pointe DIRECTEMENT la section Abonnement (un clic au lieu de
  Général → carte → Gérer).
- **Contrat de rôles inchangé** : `GET /api/subscription` reste ouvert à
  tous les rôles authentifiés (le gérant voit la section en lecture) ; les
  actions de renouvellement/paiement restent rang 3 côté Go. La position
  de la section (après Notifications) garantit que l'atterrissage du
  gérant dans la zone reste Hotspot — aucun changement de destination.
- **URLs compatibles** : l'ancien chemin racine `/app/subscription` reste
  deep-linkable (LEGACY_SLUG_VIEWS) et re-normalisé en replace vers le
  chemin canonique — signets et historiques navigateur conservés.
- **Frontend only** : aucun changement backend, aucune migration Neon —
  ViewIds, map VIEWS, garde-fous de rôles et serveur inchangés.

## 2026-09-08 — N°57-d : zone Paramètres — sections réorganisées (Général / Hotspot / Sécurité / Routeurs / Notifications / Équipe) + fiches routeurs sans modale

### N°57-d — Réorganisation experte des 6 sections (demande utilisateur) : une préoccupation = une section, le Portail et les Modèles deviennent des onglets du Hotspot
- **Sections de la sidebar de zone réorganisées** (la substitution N°57-c et
  les 2 colonnes constantes sont conservées) : l'ancienne section
  « Paramètres » (3 sous-onglets internes) est ÉCLATÉE en trois sections —
  **Général** `/app/settings/general`, **Hotspot** `/app/settings/hotspot`,
  **Sécurité** `/app/settings/security` — aux côtés de Routeurs,
  Notifications et Équipe. Plus aucune navigation à 2 niveaux cachée dans
  une section : une préoccupation = une section.
- **Section Hotspot = hub à onglets** (pattern N°30 « users/registrations →
  hub Utilisateurs ») : les vues Portail et Modèles deviennent les ONGLETS
  du Hotspot — `/app/settings/hotspot` (Expérience, propriétaire),
  `/app/settings/hotspot/portail` et `/app/settings/hotspot/modeles`
  (gérant+). Chaque onglet reste un ViewId deep-linkable ; la section reste
  surlignée sur ses trois onglets ; l'onglet Expérience (PUT /api/settings,
  rang 3) est masqué au gérant qui atterrit sur Portail.
- **Routeurs : cartes cliquables + fiche directe, plus de modale
  d'inspection** (retour utilisateur) : la grille de cartes devient le
  contrôle (clic / Entrée / Espace → fiche) ; la fiche routeur vit en PLEINE
  PAGE (`/app/settings/routers/<id>`, adressable — Retour navigateur et
  bouton « Tous les routeurs » font la même sortie, discipline 192ad9f) et
  concentre toutes les actions (test, stats, import, réparation
  walled-garden, script, édition, suppression). L'ancien RouterToolsDialog
  (trafic temps réel, IP bindings, DHCP/hôtes/cookies/journal, système)
  devient un PANNEAU INLINE (`RouterToolsPanel`) dans la fiche — les
  dialogues restants sont des flux d'action (création, wizard agent,
  réinstallation), pas des inspections.
- **Général sans onglet interne** : Organisation (nom, devise, fuseau,
  lien Wave), Langue (bascule immédiate) et une carte Abonnement de lecture
  (état réel GET /api/subscription + accès direct à la vue dédiée — pas de
  duplicate du flux de paiement).
- **Sécurité sans onglet interne** : mot de passe + 2FA (cartes partagées
  parts/security-cards, mêmes implémentations que la console plateforme)
  et une carte « Activité récente » alimentée par le VRAI journal
  (GET /api/activity, filtré comptes/système) — aucune donnée factice.
- **Notifications : 3 domaines nommés** (sans onglet interne) : Règles
  d'alerte (interrupteur + seuils + rapport quotidien), **Webhooks &
  canaux** (Telegram, WhatsApp Cloud API, e-mail SMTP + test d'envoi),
  Historique des envois — hiérarchie visuelle explicite là où les cartes
  s'empilaient au même niveau.
- **Équipe : gestion des membres repensée** : statistiques réelles
  (membres / gérants / propriétaires, GET /api/team) + GRILLE DE CARTES
  membres (avatar, @identifiant, « membre depuis », badge de rôle, actions
  Modifier/Supprimer) ; la matrice des rôles devient une carte repère en
  pied de page.
- **URLs compatibles** : l'ancienne racine `/app/settings` (et les chemins
  canoniques N°57-b/c `/app/settings/portal`, `/app/settings/templates`)
  restent deep-linkables et sont re-normalisés (replace) vers les nouveaux
  chemins ; les slugs historiques pré-N°57 (`/app/portal`, `/app/templates`…)
  fonctionnent toujours. La résolution d'URL essaie le segment le plus long
  d'abord (3 segments du hub, puis paire, puis slug simple).
- **Frontend only** : aucun changement backend, aucune migration Neon —
  Render sans objet, la CI déploie Vercel.

## 2026-09-07 — N°57-c : zone Paramètres — la sidebar de sections REMPLPLACE la sidebar principale (bouton Retour)

### N°57-c — Deuxième correction UX (retour utilisateur) : fin définitive de la double colonne — la zone vit DANS la sidebar, pas à côté
- **Retour utilisateur (2ᵉ itération)** : N°57 (split-view) empilait une
  sidebar interne sur la sidebar principale = 3 colonnes (« sidebar dans
  sidebar », anti-pattern UX) ; la première correction (N°57-b, sub-nav
  horizontale de pills) a été ANNULÉE sur demande — le pattern retenu est
  la **substitution** : quand une vue de la zone est active, la sidebar
  des sections **prend la place de la navigation principale** dans le même
  `<aside>` — le layout reste TOUJOURS à 2 colonnes (sidebar + contenu),
  jamais 3.
- **Bouton « Retour » en tête de la sidebar de zone** : ramène à la
  **dernière vue métier visitée** (mémorisée par l'app-shell dans un ref
  qui survit aux changements de section ; défaut dashboard pour une entrée
  par lien direct `/app/settings/…`). La sidebar principale reprend alors
  sa place — marque, carte utilisateur et crédit FTCI ne bougent pas : la
  substitution est invisible au regard, seule la liste change (mêmes
  classes `sidebar-nav-item` / `nav-active` que NavList).
- **Substitution partout** : l'aside desktop ET le Sheet mobile rendent la
  sidebar de zone à la place de NavList (burger → drawer de sections avec
  Retour) ; le contenu rend la vue comme tout autre module (la zone ne
  vit plus du tout dans le contenu : plus d'enveloppe, transition
  identique). La section « Paramètres » (vue settings) conserve ses
  sous-onglets internes Général / Hotspot / Sécurité.
- **Contrat N°57 inchangé** : ViewIds, map VIEWS, chemins canoniques
  `/app/settings/<section>`, redirections legacy deep-linkables,
  garde-fou rôle, atterrissage adapté au rôle (gérant → première section
  rang 2, propriétaire → racine). L'entrée « Paramètres » de la navigation
  principale reste le point d'entrée de la substitution.
- **Revert préalable** : le commit N°57-b (sub-nav horizontale) est
  annulé proprement par `git revert` (l'historique public n'est jamais
  réécrit) — N°57-c s'applique sur l'état N°57.
- **Frontend only** : aucun changement backend — Vercel seul, zéro
  déploiement Render, aucune migration Neon.

## 2026-09-07 — N°57 : zone Paramètres en split-view — le gérant reste sur les modules métier

### N°57 — Modèles, Routeurs, Portail, Notifications et Équipe quittent la navigation principale : une zone « Paramètres » dédiée les regroupe
- **Demande gérant** : la sidebar mélangeait modules métier (ventes, vouchers,
  sessions, utilisateurs) et configuration technique (routeurs, portail,
  modèles, alertes, équipe) — le gérant perdait son fil de travail. Toutes
  les vues de configuration vivent désormais sous une zone dédiée
  `/app/settings/<section>`, rendue en **split-view** : sidebar de sections à
  gauche (desktop) / bandeau horizontal défilant (mobile), panneau de
  contenu à droite avec transition douce — la sidebar ne se remonte pas au
  changement de section.
- **Refonte navigation principale** : la section « Infrastructure »
  disparaît (Routeurs + Portail → zone), « Modèles » sort d'Exploitation,
  « Équipe / Notifications / Paramètres » sortent d'Analyse ; une section
  « Système » finale porte l'entrée unique **Paramètres**. La sidebar ne
  montre plus que les modules métier : Exploitation (dashboard, sessions,
  utilisateurs, vouchers, profils), Facturation & Ventes (abonnement,
  revendeurs, WiFi), Analyse (rapports, journal, comptes), Système.
- **Contrat des vues INCHANGÉ** : chaque ViewId, vue et route API restent
  identiques (map `VIEWS` de l'app-shell intacte) — seul le chemin canonique
  change (`view-path.ts` : segments imbriqués `settings/<section>`). Les
  vues conservent leur PageHeader (titre + description), leur chargement
  différé et leurs données ; la zone n'est qu'une enveloppe layout
  (`settings/settings-shell.tsx`), style minimaliste épuré aux tokens du
  projet (bordures fines, deux graisses, aucune nouvelle couleur, aucune
  dépendance externe).
- **URLs historiques deep-linkables** : `/app/templates`, `/app/routers`,
  `/app/portal`, `/app/notifications`, `/app/team` résolvent toujours leur
  vue (`LEGACY_SLUG_VIEWS`) puis sont re-normalisées en `replace` vers le
  chemin canonique — zéro entrée d'historique parasite, le bouton Retour
  n'est jamais piégé (même mécanique que la fusion N°30 « registrations »).
- **Rôles respectés à l'atterrissage** : l'entrée « Paramètres » est visible
  dès qu'UNE section est accessible au rôle ; le gérant (rang 2) atterrit
  sur sa première section (Routeurs), le propriétaire (rang 3) sur la racine
  (Général). Un lien direct vers une section interdite (p.ex. `/app/team`
  d'un gérant) est re-normalisé en `replace` vers la première section
  autorisée (garde-fou URL d'app-route, garde existant conservé). L'entrée
  reste active sur toute la zone (l'utilisateur sait où il est).
- **Entrées recâblées** : sidebar (filtre + atterrissage + surlignage zone),
  palette de recherche (⌘K), menus profil (UserCard + Topbar mobile),
  auto-ouverture de la section « Système » ; la cloche d'activité (rang 2 =
  notifications) reste inchangée ; le padding double de la vue Portail est
  retiré (le panneau de zone fournit déjà le sien).
- **Frontend only** : aucun changement backend — Vercel seul, zéro
  déploiement Render requis, aucune migration Neon (aucun changement de
  schéma).
## 2026-09-07 — N°56 : analytics du portail — « votre menu vu 480 fois cette semaine »

### N°56 — impressions / clics par promo : l'argument de vente chiffré de l'hospitalité
- **Constat** : la vitrine N°55 affiche les produits de l'établissement, mais
  le gérant ne sait pas si elle SERT à quelque chose. L'argument commercial
  décisif (« votre menu a été vu 480 fois cette semaine ») demande deux
  compteurs honnêtes : impressions (carte visible sur le portail) et clics
  (ouverture du lien). C'est aussi l'outil de pilotage : quelle ligne de
  vitrine attire, quelle image convertit.
- **Deux endpoints** :
  - `POST /api/portal/track` — PUBLIC (pré-auth du hotspot, whitelist
    middleware + CORS ouverte, même statut que le portail WiFi N°28/35-c).
    La page dépose `{key, promoId, kind: impression|click, clientKey}` ;
    résolution du compte par la **clé publique du portail**
    (`tenant.portalKey`, 16 hex générée par `ensureSettings`, embarquée dans
    le bloc config — NON secrète par design : elle n'ouvre AUCUN droit de
    lecture). Réponse 204 dans TOUS les cas (aucun oracle).
  - `GET /api/promos/stats` — console (JWT, manager et plus) : par promo
    (jour / 7 jours glissants / total) + totaux, lu en mémoire (zéro requête
    Neon sur le chemin chaud).
- **Garde-fous anti-gonflement** (la metric doit rester HONNÊTE) :
  1. dédup par **ID d'événement déterministe** `sha256(compte|promo|type|appareil|jour)`
     — un même appareil ne compte qu'UNE fois par promo et par jour ; le
     refresh-spam ne gonfle rien et le re-POST est un no-op (upsert Neon
     identique, diff syncTable le voit inchangé) ;
  2. seuls les `promoId` EXISTANTS dans la vitrine du compte sont acceptés ;
  3. limiter IP dédié NAT-friendly (300/10 min + 3000/24 h — pattern N°50) ;
  4. plafonds 3 000 événements/compte/jour, rétention 90 jours, journal
     mémoire ≤ 12 000 lignes (`prunePromoEvents` — les suppressions sont
     répercutées en Neon par la diff).
- **IDs de promos stables** : `PUT /api/settings` pose un id aléatoire
  (`p` + hex) à la première enregistrement et le CONSERVE au round-trip
  console (le GET→PUT ne doit jamais régénérer — sinon les compteurs
  repartiraient de zéro). Les lignes héritées d'avant N°56 (jamais
  ré-enregistrées) reçoivent un id DÉTERMINISTE dérivé du contenu (`h` +
  hash) côté lecture (`portalHospitality` / `promoIDsOf`) : le portail peut
  tracker sans attendre un ré-enregistrement.
- **Nouveau champ promo `link`** (https ≤ 300, validé) : la carte devient
  cliquable (target `_blank`) quand un lien existe — menu PDF, commande
  WhatsApp, page Facebook… — et son ouverture est comptée « click ». Sans
  lien, la carte reste informative (impressions seules).
- **Portail (`login.html`)** : cartes `data-promo-id`, ancre `a.hosp-card`
  (CSS dédié), `mikTrack()` fire-and-forget TOTALEMENT silencieux
  (`fetch keepalive`, catch-all — l'analytics ne doit jamais gêner la
  connexion), `mikWatchPromos()` après rendu de la vitrine (dédup client
  `mikTracked`), `onclick` inline (pattern `onerror` existant — compat vieux
  WebViews). `clientKey` = MAC du portail (`clientMac`, même identité que le
  claim N°50), repli IP serveur.
- **Console** : carte « Portail : vitrine de l'établissement » enrichie —
  champ **Lien** par promo + panneau **Analyse de la vitrine** (« Vos
  produits ont été vu N fois cette semaine », vues/clics par ligne, bouton
  Actualiser, définitions impression/clic). i18n FR/EN (7 clés).
- **Persistance** : table `promo_events` (DDL idempotent boot : PK id + 2
  index compte/promo) + colonne `settings.portal_key` — pattern maison
  (mémoire moteur, Neon durable, diff différentielle).
- **🔥 Correctif incident N°55 (découvert pendant N°56)** : l'UPSERT
  `syncSettings` comptait 31 colonnes pour 30 expressions VALUES — le
  pattern maison (id et account_id partagent `$1`) avait été perdu lors de
  l'ajout des colonnes hospitalité. Conséquence : CHAQUE `Save()` échouait
  en production depuis le déploiement N°55 (parse error PostgreSQL →
  rollback TOTAL de la transaction de synchro → Neon ne recevait plus
  AUCUNE écriture, tout tournant sur la mémoire — régression silencieuse,
  les réponses API restant correctes). Correctif : `VALUES ($1, $1, $2…$31)`
  (32 colonnes / 32 expressions / 31 paramètres) + test statique
  `TestSyncSettingsSQLConsistency` qui verrouille l'invariant
  colonnes = expressions, id/account_id partagés, SET complet — toute
  récidive au prochain ajout de colonne sera attrapée par la CI.
- **Tests** : `TestPromoTrackDedupeAndStats` (dédup, dépôts invalides
  silencieux, stats, 401 sans token), `TestPromoStatsBuckets`
  (jour/semaine/total, orphelin exclu), `TestPromoIDsStableOnRoundTrip`
  (ids stables, lien http refusé, id falsifié refusé). Suite backend verte.
 (N°56-2 — analytics du portail : vitrine trackée (impressions/clics), cartes cliquables et analyse dans la console)

## 2026-09-07 — N°54 : console WiFi — plafonds journaliers renommés + compteur du jour

### N°54 — plus jamais l'ambiguïté des plafonds : chaque champ dit QUI il limite, et le budget restant est visible
- **Demande gérant** (retour terrain CYBER-ESPACE : 12 visiteurs pour un
  plafond cru à 10, même téléphone re-claimé 3 fois sans blocage) — les trois
  plafonds journaliers portaient des intitulés interchangeables et le réglage
  réel n'était visible nulle part : libellés dédoublonnés + compteur temps réel.
- **Libellés console dédoublonnés (fr/en)** : « Tickets max / téléphone / jour »
  → **« Par numéro de téléphone »**, « Tickets max / appareil / jour » →
  **« Par appareil (même WiFi) »**, « Budget gratuit / jour (tickets) » →
  **« TOTAL offerts / jour (tous clients) »** ; nouveaux hints pédagogiques
  (`perPhoneHint` : UN numéro = 1 ticket/jour avec 1 ; `dailyCapHint` : au-delà
  le portail répond « épuisé » jusqu'à minuit) ; `perMacHint` conservé.
- **Compteur du jour sur chaque carte site** : « 7 visiteurs aujourd'hui » →
  **« 7 / 10 offerts aujourd'hui »** (`stats.guestsToday` vs `dailyCap`, déjà
  servis par `GET /api/wifi/sites` — zéro changement backend) + jauge
  du plafond (ambre ≥ 80 %, rouge épuisé, aria-hidden — le texte porte l'info) ;
  la liste se rafraîchit toutes les 30 s (`refetchInterval`) = compteur vivant.
- **Frontend only** : aucun déploiement Render requis — Vercel seul.
- Contexte : correction immédiate des plafonds du site freezone faite en
  console le même soir (par téléphone 10 → 1, budget 100 → 10, preuve
  `429 site_cap`), sans changement de code.
## 2026-09-06 — N°55 : mode hospitalité du portail — la vitrine de l'établissement

### N°55 — MikCloud ne présuppose plus que l'établissement VEND : le portail devient SA vitrine
- **Constat** : le portail captif (héritage Mikhmon) affiche TOUJOURS une
  grille tarifaire (1H 100 F → 30 j 3 000 F), un bandeau Wave et des services
  génériques cybercafé — inadapté aux hôtels, maquis, cafés-glaciers, salons
  et espaces événementiels qui OFFRENT la connexion pour fidéliser. Suite
  directe de l'analyse N°52 (messages contextuels) et de la feuille de route.
- **Deux modes, un seul moteur** (Option A de l'analyse — pas de template
  dupliquée) : `tenant.portalStyle` = `commercial` (défaut, page historique
  INTACTE) ou `hospitality` (vitrine de l'établissement). Bascule console,
  appliquée SANS re-déploiement grâce au fetch live N°48.
- **Portail (`login.html`)** — bloc 7 `applyHospitality` : masquage par classe
  (slider pub générique, grille tarifaire hardcodée, Wave, services, offres
  dynamiques) + injection de la vitrine (message de bienvenue, grille de
  promos produits avec images R2, boutons réseaux sociaux). Réversible et
  idempotent — compatible navigateurs mobiles anciens (pas d'optional
  chaining).
- **Backend** : `Tenant.PortalStyle/PortalWelcome/PortalPromos/PortalSocials`
  (listes persistées en JSON) ; `PUT /api/settings` reçoit des LISTES
  structurées et VALIDE tout côté serveur (≤ 6 promos — titre 1-60, desc ≤
  160, prix ≤ 30, image https ≤ 300 car ; ≤ 4 liens sociaux https ≤ 200 ;
  style borné ; bienvenue ≤ 200) puis sérialise — le client ne peut rien
  injecter d'autre. `PortalConfig` transporte `portalStyle/portalWelcome/
  portalPromos/portalSocials` (fallback inliné + fetch live), décodage JSON
  tolérant (`portalHospitality` : JSON invalide ⇒ listes vides, page jamais
  cassée).
- **Migration Neon** : 4 colonnes idempotentes au boot
  (`settings.portal_style`, `portal_welcome`, `portal_promos`,
  `portal_socials`) — mécanique N°47/49/50, synchronisées au premier Save.
- **Console** : carte « Portail : vitrine de l'établissement » (select mode,
  textarea bienvenue, éditeur de promos structuré avec téléversement d'image
  R2 par ligne — réutilise `apiUpload` N°53, éditeur de liens sociaux) ;
  i18n fr/en (20 clés).
- **Tests** : gofmt/vet/build propres, suite API + hotpage verte (le template
  reste conforme aux marqueurs N°46), ESLint vert.

## 2026-09-06 — N°53 : stockage média Cloudflare R2 (fini les data URLs à rallonge)

### N°53 — les images du gérant vivent dans un vrai stockage objet
- **Pourquoi** : la bannière du portail (N°45) se téléversait en data URL
  encodée EN BASE (≤ 500 Ko) — la charge gonfle la config inlinée du portail
  et chaque lecture de settings ; les futures promos hospitalité (N°54)
  exigent un stockage propre. Le placeholder « Cloudflare R2, à venir »
  devient réel.
- **Infrastructure** : compartiment R2 `mikcloud-media` créé sur le compte
  Cloudflare du tenant (API REST), bucket privé — rien n'est public sauf ce
  que le backend sert explicitement.
- **Canal REST, zéro SDK** : le backend Go parle à l'API REST Cloudflare
  (Bearer `R2_API_TOKEN`) au lieu de l'API S3/SigV4 — pas de nouvelle
  dépendance (go.mod inchangé), un seul secret à configurer, PUT/GET par clé
  suffisent pour des images ≤ 2 Mo.
- **Backend** (`handlers_media.go`, NOUVEAU) :
  - `POST /api/media` (auth gérant, multipart) : type MIME SNIFFÉ dans le
    contenu (pas dans le nom), ≤ 2 Mo, clé `media/{compte}/{année}/{hex32}`
    → 201 `{url, key, size, type}`. Non configuré ⇒ 503 `media_unconfigured`
    (gracieux : la bannière N°45 en data URL reste disponible).
  - `GET /api/media/{key...}` (PUBLIC) : clé validée par regex stricte,
    Content-Type dérivé de l'extension, nosniff,
    `Cache-Control: immutable` — les images du portail captif sont servies
    par le MÊME hôte que `apiBase` (walled-garden N°48 déjà OK, zéro
    entrée nouvelle).
- **Frontend** (console > Bannière du portail) : le bouton « Téléverser »
  pousse vers R2 et remplit le champ avec l'URL permanente (toast de
  confirmation, bouton en état « Téléversement… »). Repli dégradé
  automatique si le stockage est indisponible : data URL intégrée ≤ 500 Ko
  (contrat N°45 inchangé) ou message d'erreur au-delà. i18n fr/en.
- **Render** : env `R2_ACCOUNT_ID` / `R2_API_TOKEN` / `R2_BUCKET`
  configurées via API.
- **Zéro migration** : `bannerUrl` reste la seule donnée persistée.
- **Tests** : `gofmt`/`go vet`/`go build` propres, suite API verte (70 s),
  ESLint frontend vert ; canal R2 validé bout en bout (PUT/GET/suppression
  d'objets de sonde, clés hiérarchiques incluses).

## 2026-09-06 — N°51 : la carte « WiFi offert » du portail suit l'état du site (activée → affichée, en pause → retirée)

### N°51 — portail dynamique : si le site WiFi est désactivé, la carte téléphone disparaît
- **Demande gérant** : l'affichage du formulaire (carte champ téléphone) sur le
  portail captif doit être dynamique — site WiFi activé → la carte s'affiche ;
  site désactivé → elle disparaît.
- **Constat** : la présence de la carte n'était pilotée qu'au DÉPLOIEMENT du
  portail (`buildPortalConfig` ne renseigne `wifiSlug` que pour un site actif)
  et le claim refusait déjà les sites en pause (403 `site_inactive`) — mais
  désactiver un site n'effaçait PAS la carte des portails déjà déployés (la
  config live ne transportait pas d'état, et aucun re-déploiement n'était
  déclenché par un toggle).
- **Backend** : `PortalConfig.Active` (json `active`, SANS omitempty — même
  logique que `joinEnabled` N°46 : un false omis serait ignoré par la page),
  peuplé dans `buildPortalConfig` (site actif lié) et
  `buildPortalConfigForSite` (état réel) ; `handleWifiPortal` répond pour un
  site EN PAUSE avec la config fraîche du ROUTEUR (1er autre site actif, sinon
  wifiSlug vide + active:false) — auto-réparation de la page même si le
  fallback inliné est périmé ; création d'un site ACTIF / bascule active 1 clic
  / changement de routeur / suppression → sig `hotspot_files` vidée → le
  portail se re-déploie au prochain check-in agent (≤ 45 s), ce qui resynchronise
  le fallback inliné (indispensable dans le sens « activé après coup » : un
  fallback sans slug ne fetch jamais la config live). Garde-fou
  `TestWifiPortalClaimDynamicN51` (flag live actif/pause/réactivation, sig vidée
  sur create/toggle/delete, 404 après suppression).
- **Frontend portail** (`login.html`) : bloc 5 `applyConfig` — carte claim
  injectée si `wifiSlug` ET `active !== false` (les portails déployés avant
  N°51 n'embarquent pas le champ : `undefined ≠ false` = carte injectée,
  comportement historique, le fetch live corrige) ; RETIRÉE (idempotent) si
  `active === false` ou slug vide. Le retrait est sans re-déploiement : la
  config live transporte l'état à chaque visite.
## 2026-09-06 — N°52 : messages d'épuisement contextuels (upsell commercial ou ton neutre hospitalité)

### N°52 — MikCloud ne présuppose plus que l'établissement VEND du WiFi
- **Constat** : les refus de claim (`phone_cap`, `device_cap`) se terminaient
  TOUJOURS par « passez à une offre payante » — un copywriting pensé pour la
  vente de tickets. Or MikCloud sert deux usages : la vente (camps, bars,
  événements payants) ET l'offre gratuite de fidélisation (hôtels, maquis,
  cafés-glaciers, salons de coiffure, espaces événementiels) où le WiFi offert
  retient le client au lieu de se vendre. Pousser une « offre payante »
  inexistante décrédibilise l'écran ET l'établissement — et rétrécit notre
  clientèle potentielle et nos arguments de vente.
- **Détection automatique, zéro config, zéro migration** : le signal est le
  catalogue — `wifiOffers(db, site)` (profils du compte à prix > 0), le même
  signal que l'écran « épuisé » de la page /wifi utilise déjà. Catalogue
  présent → mode commercial (upsell) ; catalogue vide → mode hospitalité
  (neutre).
- **Backend (`handleWifiClaim`)** — suffixe `capSuffix` calculé une fois sous
  verrou : « — passez à une offre payante » (commercial) vs « — demandez au
  personnel ou revenez demain » (hospitalité), appliqué aux 429 `device_cap`
  et `phone_cap`. `site_cap` était déjà neutre, inchangé. Le portail captif
  (`login.html`) affiche `data.error` tel quel : il hérite du bon ton sans
  modification.
- **Frontend (`wifi-guest-page.tsx`)** — l'écran « Quota offert épuisé » se
  dédouble : avec catalogue, upsell 1 clic inchangé (liste des offres +
  « Achetez votre ticket au comptoir ») ; sans catalogue, message chaleureux
  (« Revenez demain ou demandez au personnel » + « Un nouveau code gratuit
  vous attendra à votre prochaine visite — merci de votre fidélité »).
- **Honeypot/plafonds inchangés** : N°50 continue de filtrer bots et rotation
  d'appareils — seul le TON du refus s'adapte, la sécurité reste identique.
- **Tests** : suite API verte (`go test ./internal/api/`), `go vet` propre,
  ESLint frontend vert. Aucun test n'assertait les textes (seuls les codes
  machine sont contractuels).

## 2026-09-06 — N°50 : WiFi jetable — garde-fous anti-abus (plafond appareil + honeypot + quota anti-fermage IP)

### N°50 — le gratuit reste un cadeau, pas un gisement à miner
- **Constat** : le WiFi jetable distribuait des tickets gratuits sur la seule
  foi d'un numéro autodéclaré. Un visiteur motivé pouvait taper des numéros
  différents pour multiplier les codes (jusqu'au budget journalier du site),
  et un fermier pouvait automatiser le claim depuis une même IP.
- **Plafond par appareil (MAC)** — la leçon N°33 étendue au claim : derrière
  le NAT du hotspot tous les clients partagent la MÊME IP publique, la MAC
  est la seule clé qui isole réellement un appareil. `WifiSite.DailyPerMac`
  (1–10, défaut 1, console) borne les tickets / appareil / jour sur les
  claims du PORTAIL (qui injecte `$(mac-esc)` dans le POST) ; la page
  /wifi scannée hors portail ne fournit pas de MAC (plafonds existants
  inchangés). L'idempotence téléphone PRIME : le re-claim du même numéro
  renvoie toujours le même code, plafond atteint ou non — un visiteur
  légitime n'est jamais puni. Réponse machine `429 device_cap`.
- **Honeypot « website »** — même contrat que le formulaire d'inscription :
  champ invisible (hors écran, non tabulable, aria-hidden) sur la page
  /wifi ET sur le formulaire de claim inline du portail ; un bot qui le
  remplit reçoit un succès FACTICE (même forme JSON, code aléatoire jamais
  créé) — rien n'est émis, rien n'est enregistré, aucun indice sur le
  filtre. Placé APRÈS résolution site/profil (quotas plausibles), AVANT
  toute écriture.
- **Quota anti-fermage par IP** — `signupLimiter` paramétrable
  (`newSignupLimiterLimits`) : le claim WiFi obtient son propre limiter
  (20/10 min + 100/24 h par IP, NAT-friendly : un établissement entier
  partage une IP) consommé par TOUTE tentative. Un attaquant qui tourne sur
  des numéros falsifiés est coupé avant toute création de voucher.
- **Traçabilité gérant** : `WifiGuest.Mac` (+ normalisée
  `normalizeJoinMac`) et `WifiGuest.IP` (premier hop XFF) stockées à
  l'émission — audit anti-abus dans le registre + colonnes « appareil » et
  « ip » de l'export CSV. Migration boot `ALTER TABLE ... ADD COLUMN IF NOT
  EXISTS` (mécanique N°47/N°49) : `wifi_sites.daily_per_mac`,
  `wifi_guests.mac`, `wifi_guests.ip`.
- **Backend** : handlers claim/create/update ; validations (DailyPerMac
  1–10) ; messages d'audit étendus. **Frontend** : champ console « Tickets
  max / appareil / jour » (+hint), types, i18n fr+en. **Portail** : claim
  inline envoie `mac` + `website`. **Tests** : TestWifiClaimHoneypot
  (succès factice sans écriture, puis claim honnête OK),
  TestWifiClaimDeviceCap (plafond, idempotence prioritaire, claims sans
  MAC / MAC invalide, empreintes tracées). Suite backend 12/12.

## 2026-09-06 — N°49-b : affiche à QR unique (l'ancien QR « page web » quitte l'affiche)

### N°49-b — un seul QR sur l'affiche : celui qui connecte au WiFi
- **Décision gérant** : supprimer le second QR « Page web » conservé en
  secours au N°49. Deux QR côte à côte = hésitation au scan ; le QR de
  connexion (N°49) redevient l'unique geste de l'affiche.
- **Pourquoi c'est sûr** : les affiches déjà collées ne changent pas (leur QR
  page web est imprimé sur le papier et /wifi/{slug} reste en ligne) ; le
  repli « portail qui ne poppe pas » est repris par une ligne imprimée sous
  le QR (« La page ne s'ouvre pas ? Ouvrez simplement votre navigateur. » —
  le hotspot MikroTik redirige le HTTP non authentifié vers le portail) ;
  le repli vieux téléphones reste le SSID imprimé sur l'affiche.
- **Frontend** : `wifi-poster-dialog` — onglets « Connexion WiFi / Page web »
  supprimés (QR unique `WIFI:T:nopass|WPA;S:..;P:..;;`, échappement spec
  Android conservé), prop `publicUrl` retirée ; SSID vide → guidage console
  + Impression désactivée (inchangé). Le bouton console « Copier l'URL » et
  la page /wifi/{slug} restent inchangés.

## 2026-09-06 — N°49 : QR de connexion WiFi sur l'affiche (SSID du hotspot encodé, format universel WIFI:)

### N°49 — le client scanne, le WiFi se connecte tout seul, le portail fait le reste
- **Idée gérant** : remplacer le QR « page web » (qui ouvrait /wifi/{slug},
  obligeait à copier un code puis à basculer vers le portail) par un QR qui
  connecte DIRECTEMENT au SSID du hotspot — même slogan « WiFi Offert »
  imprimé, même argument marketing, parcours raccourci.
- **Backend** : `WifiSite.WifiSSID` (+ `WifiPassword` si le réseau est WPA) ;
  bornes des normes radio (SSID ≤ 32 car. 802.11, phrase secrète ≤ 63 car.,
  trim) dans `validateWifiSitePayload` ; handlers create/update ; migration
  boot `ALTER TABLE wifi_sites ADD COLUMN IF NOT EXISTS wifi_ssid /
  wifi_password` (mécanique N°47 : le CREATE TABLE ne touche pas les bases
  pré-existantes). Mot de passe stocké en clair VOLONTAIREMENT : sa seule
  utilité est d'être encodé dans le QR imprimé (destiné aux clients).
  Garde-fou `TestWifiSiteWifiFieldsN49`.
- **Frontend** : deux champs console dans le formulaire site (« SSID du
  réseau WiFi », « mot de passe optionnel ») ; affiche en 2 modes —
  « Connexion WiFi » (défaut si SSID renseigné) encode
  `WIFI:T:nopass|WPA;S:..;P:..;;` avec l'échappement de la spec Android
  (`\ ; , : "` backslashés), « Page web » conserve l'ancien QR /wifi/{slug}
  (secours : portail qui ne poppe pas, QR déjà imprimés). SSID vide → mode
  connexion indisponible avec message de guidage console. i18n fr+en.
- **Parcours client** : scan appareil photo (natif iOS 11+ / Android 10+) →
  « Rejoindre le réseau ? » → portail captif s'ouvre → formulaire inline
  N°48 (numéro → code → en ligne). La page /wifi/{slug} reste vivante
  (secours + bascule offres payantes + compatibilité affiches passées).

## 2026-09-06 — N°48-b : portail auto-redéployé après chaque édition du template (sig hotspot_files basée contenu)

### N°48-b — fini le login.html périmé sur le routeur sans clic console
- **Constat coulissant** : le correctif N°48 (retrait du bandeau doublon dans
  `login.html`) n'était pas servi aux clients — la signature `hotspot_files`
  ne hashait que la LISTE des chemins, pas le contenu : une édition du
  template ne changeait pas la sig, `ensureHotspotFilesLocked` ne re-filait
  rien, le routeur servait l'ancienne page jusqu'au bouton console
  « Re-déployer maintenant » (les règles walled-garden v2, elles, étaient
  déjà appliquées automatiquement — asymétrie absurde).
- **Backend** (`hotpage.Sig`) : chemin + sha256 (16 hex) du contenu de chaque
  fichier embarqué → toute édition change la sig → re-pousse automatique au
  premier check-in (≤ 45 s). Contenus inchangés → même sig → check-in no-op
  (aucun spam de commandes). Garde-fou `TestSigContentSensitive` (pattern du
  sel `wg-v2-api`) : si le contenu disparaît de la sig, le test échoue.
- **Effet immédiat au déploiement** : sig changée → tous les routeurs agents
  re-tirent le portail (login.html N°48 sans bandeau) au prochain check-in.

## 2026-09-06 — N°48 : claim inline opérationnel en pré-auth (walled-garden HTTPS) + portail dédoublonné

### N°48 — le « WiFi offert » marche vraiment depuis le portail, sans doublon à l'écran
- **Constat** (screenshot client, portail CYBER-ESPACE SC / cyberscwifi.net) :
  (a) « Recevoir » affichait « Service WiFi offert momentanément indisponible
  — demandez votre code au personnel » ; (b) l'offre gratuite apparaissait DEUX
  fois (carte formulaire + gros bouton lien).
- **Diagnostic** — l'API était saine : `POST /api/wifi/site/{slug}/claim`
  répond 200 (code 5 car.) en ~0,6 s, préflight CORS 204 + ACAO. Preuve
  décisive en base : la tentative du téléphone (16:49) n'a laissé AUCUNE
  trace dans `wifi_guests` → le fetch n'a jamais quitté le hotspot. Cause
  racine : le walled-garden v1 ne posait que des règles « page » (variante
  proxy du hotspot = HTTP PUR, port 80) + DNS. L'API est en HTTPS
  (Render/Cloudflare) : le TLS 443 pré-auth était bloqué → `fetch` rejeté →
  message d'échec. Les tests curl (hors hotspot) passaient, le terrain non.
- **Backend** (`internal/agent`, `internal/api`) : le builder de commande
  `walled_garden` ET le bloc d'installation posent désormais, par domaine,
  une règle `ip hotspot walled-garden ip add action=accept dst-host=…`
  (commentaire `mikcloud-wg api`) — couvre TCP 80/443 ET UDP 443 (QUIC) ;
  `action=accept` obligatoire sur la variante ip (leçon N°31-d), adds
  conditionnels + `:set step` (traçage N°32), removes best-effort (N°31-e).
- **Auto-mise à niveau** : `walledGardenSig` est salée avec
  `walledGardenRulesVersion = "wg-v2-api"` — la sig change SANS changer la
  liste des domaines, donc `ensureWalledGardenLocked` re-file la commande sur
  chaque routeur existant à son premier check-in (≤ 45 s). Garde-fou
  `TestWalledGardenSigVersioned` : si le sel disparaît, le test échoue (la
  régression exacte de ce bug : corriger le script sans re-pousser).
- **Template** (`login.html`) : bandeau « WiFi offert — Recevoir mon accès
  gratuit » RETIRÉ — il doublonnait le formulaire de claim inline (même
  donnée, même promesse, deux points d'entrée). Le formulaire (téléphone →
  code 5 car. → `doLogin()` CHAP auto) est seul maintenu.
- **Tests** : marqueurs N°48 dans `TestWalledGardenScript` /
  `TestWalledGardenInstallBlock` (règles api présentes, interdit
  `action=allow` en variante ip maintenu) ; suite backend 11/11 verte.
## 2026-09-06 — N°49 : walled-garden AUTO-RÉPARANT (horodatage + re-file périodique) et réparation à la demande depuis la console

### N°49 — la panne CyberSC (règles page absentes) ne peut plus se reproduire en silence
- **Constat terrain (CyberSC)** : le bouton « S'inscrire » (N°46) — comme le
  lien join ouvert par QR dans un onglet — aboutissait à « vérifiez votre
  connexion internet » sur le WiFi non authentifié. Export walled-garden du
  routeur : règles DNS `mikcloud-wg dns` présentes mais AUCUNE règle page/api
  pour `mikcloud.ftci.fr` / `mikcloud.onrender.com`, alors que la sig côté
  cloud croyait la config appliquée — plus AUCUN re-file possible : panne
  silencieuse et durable. La config Mikhmon supprimée au même moment
  (`walled-garden ip dst-host="laksa19.github.io"` + shadow rules dynamiques
  `dst-address=185.199.x.153` issues du reniflement DNS) établissait le
  mécanisme efficace (cf. N°48 : les règles ip sont la couverture HTTPS).
- **Auto-réparation durable** : le sel de version N°48 (wg-v2-api) répare les
  routeurs existants UNE fois ; si la liste est vidée/amputée localement
  APRÈS (ménage Mikhmon, restauration de backup, ajout manuel partiel), la
  sig posée re-bloquait tout re-file — même panne à terme. Nouveau champ
  `Router.WalledGardenAppliedAt` (RFC3339, colonne
  `routers.walled_garden_applied_at`, DDL idempotent + scan + sync), posé
  avec la signature au retour « ok ». `ensureWalledGardenLocked` re-file le
  bloc idempotent si la sig est identique MAIS `walledGardenFresh` est faux :
  horodatage ABSENT (routeurs antérieurs au N°49 — réparation immédiate au
  premier check-in) ou plus vieux que `walledGardenRefresh` = 6 h.
- **Réparation à la demande (console gérant)** : `POST
  /api/routers/{id}/repair-walled-garden` (404/400 not_agent/402
  subscription_expired ; vide sig + horodatage → re-file au check-in ≤ 45 s)
  + bouton « Réparer le walled-garden » dans le menu ⋯ d'un routeur agent
  (vue Routeurs, i18n FR/EN).
- **Tests (5)** : `TestEnsureWalledGardenAutoRepair` (trois leviers du
  re-file), `TestBuildWalledGardenCoversBothTables` (page host + api ip dans
  le script de commande ET le bloc d'installation),
  `TestRepairWalledGardenOK/NotFound/NotAgent` (endpoint console) ;
  `TestEnsureWalledGardenLocked` adapté (retour ok pose sig + horodatage).
  Suite backend 11/11 packages ; lint ESLint 10 + typecheck tsgo frontend
  verts.
- **Docs** : `docs/RUNBOOK-WALLED-GARDEN.md` — §5-ter auto-réparation,
  entrée de diagnostic « DNS posées / page absentes », révision de
  l'anti-pattern §6 (la règle `dst-host` dans `walled-garden ip` n'est PAS
  inopérante en HTTPS : elle déclenche le sniff DNS — constat Mikhmon) ;
  `docs/CONTRACT-V2.md` addendum N°49.

## 2026-09-06 — N°46 : bouton « S'inscrire » du portail captif rendu dynamique (joinButton)

### N°46 — l'option d'inscription est pilotée depuis la console gérant
- **Constat** : sur la page hotspot hybride, le bouton « S'inscrire »
  n'apparaissait jamais — remplacé par le reliquat Mikhmon « Scanner un QR
  Code » (lien externe laksa19 sans fonction métier). Cause racine : le
  `joinUrl` n'était jamais peuplé en production car `APP_PUBLIC_URL` n'était
  pas défini sur Render (`publicFrontendURL` retournait ""), et l'affichage
  n'était piloté par aucun réglage explicite.
- **Backend** : nouveau réglage par compte `Tenant.JoinButton` (`*bool`,
  nil = défaut ON — zéro-migration, même pattern que
  `AutoImportRouterUsers`), accepté par `PUT /api/settings` (formes plate +
  `tenant{…}`), persisté Neon (`settings.join_button BOOLEAN NOT NULL
  DEFAULT TRUE`, DDL idempotent + scan + sync). Le PortalConfig expose la
  valeur effective via `JoinEnabled` (`joinEnabled` du bloc config JSON,
  SANS omitempty : true/false toujours explicite — l'absence réactiverait le
  bouton) dans le fallback inliné ET la config live
  (`GET /api/wifi/site/{slug}/portal`) → effet immédiat sans re-déploiement.
- **Fix production** : `APP_PUBLIC_URL=https://mikcloud.ftci.fr` posé dans
  `backend/render.yaml` — les liens absolus `/join/{token}` et `/wifi/{slug}`
  sont enfin construits (`buildPortalConfig` → `publicFrontendURL`).
- **Template** : `login.html` — logique à trois états du bouton : activé +
  lien actif → « S'inscrire » (`?mac=$(mac-esc)`, quota MAC N°33) ; activé
  sans lien → aucun bouton ; désactivé → aucun bouton (`btnQr.remove()`).
  Le « Scanner un QR Code » Mikhmon ne peut plus survivre à la page.
- **Frontend** : carte « Inscription sur le portail captif » dans Paramètres
  → Hotspot (interrupteur + description des deux comportements + toast) ;
  i18n FR + EN ; `updateSettings` élargi au champ `joinButton`.
- **Tests** : `TestSettingsJoinButton` (PUT plat/nested, nil inchangé),
  `TestPortalServeJoinButton` (JSON explicite true/false dans le login.html
  servi), `TestWifiPortalJoinButtonDisabled` (config live reflète off sans
  re-déploiement), `TestConfigJSONJoinEnabledAlwaysExplicit` (gèle le contrat
  sans-omitempty), `TestLoginTemplateJoinButtonLogic` (logique 3 états dans
  le template embarqué).
## 2026-09-06 — N°47 : le claim inline attend l'application routeur (anti-course 45 s)

### N°47 — « téléphone → code → en ligne » marche enfin bout en bout en mode agent
- **Motivation** : en production (routeurs en mode agent), le claim émet le
  code et met la commande `voucher_batch` en file ; l'utilisateur MikroTik
  n'existe qu'au **prochain check-in de l'agent (≤ 45 s)**. Le portail lançait
  l'auto-login CHAP **immédiatement** ⇒ « invalid username or password » quasi
  systématique pendant la fenêtre de check-in : le visiteur voyait le code
  s'afficher puis un échec de connexion, et croyait le service cassé. Preuve en
  production : claim du 10:24 (code FM9V7) — commande appliquée à la seconde
  par l'agent (result ok, created=1) mais auto-login parti bien trop tôt.
- **Backend** : le claim trace désormais la commande sur le registre du jour
  (`WifiGuest.ClaimCmdID`, colonne Neon `wifi_guests.claim_cmd_id` — DDL
  idempotent, sync différentielle) et répond `waitForRouter: true` en mode
  agent (champ absent/false en simulated/real : application synchrone).
  `GET /api/wifi/site/{slug}/status` expose `provisioned` (helper
  `wifiProvisioned`) : vrai dès que la commande est `done` au sens agent ;
  `true` par défaut en non-agent et pour tout registre antérieur à N°47
  (on ne bloque jamais par défaut).
- **Portail** (`login.html`) : `autoLoginWhenReady` — si `waitForRouter`, le
  portail sonde `/status?phone=` toutes les **5 s** (1 requête/5 s ≪ rate-limit
  `wifi-read` 30/min/IP) avec affichage « Activation sur le routeur... (X s) »,
  puis déclenche l'auto-login CHAP dès `provisioned` ; plafond de garde
  **2 min** (24 tentatives = 2-3 cycles d'agent) au-delà duquel la connexion
  est tentée quand même. Le message fixe enfin l'attente du visiteur.
- **Déploiement portail** : la modification de `login.html` change la
  signature `HotspotFilesSig` ⇒ au check-in suivant, l'agent re-tire les
  fichiers hotspot (surcharge atomique) — aucun geste gérant requis.
- **Tests** : `TestWifiClaimAgentProvisioningWait` — routeur basculé en mode
  agent : claim ⇒ `waitForRouter=true` + `ClaimCmdID` tracé + commande en
  file ; `/status` ⇒ `provisioned=false` tant que la commande est en file,
  `true` après application. Suite backend complète verte (gofmt, vet, build,
  test ./... — 11 packages). Aucun changement de contrat authentifié ; les
  endpoints publics restent rate-limités (claim 5/min, read 30/min).

## 2026-09-06 — N°45 : bannière personnalisée du portail captif (bannerUrl)

### N°45 — la bannière du portail passe dans le PortalConfig et la console gérant
- **Motivation** : le gérant veut brander le portail captif au-delà du logo —
  une image d'en-tête (offre du jour, nom du cyber-café, photo du lieu)
  affichée en haut de la page de login, visible par TOUS les clients WiFi.
- **Backend** : `Tenant.BannerURL` (data URL image ≤ 500 Ko **ou** URL
  `https://` — prêt pour Cloudflare R2 ; http/ftp/relative/javascript:
  refusés en 400, mixed content impossible). Persisté Neon (`settings.banner_url`,
  DDL idempotent), propagé dans `PortalConfig.BannerURL` (templating :
  marqueur `{{MIKCLOUD_BANNER_URL}}` + champ `bannerUrl` du bloc config JSON)
  et exposé par `GET /api/wifi/site/{slug}/info`. Changer la bannière ne
  force PAS de re-déploiement : la config live rafraîchit la page.
- **Template** : `login.html` insère la bannière (id `mikcloud-banner`) en
  tête de la colonne de connexion (au-dessus du logo et du bandeau WiFi),
  retrait automatique du bloc si l'image ne charge pas (`onerror`).
- **Frontend** : carte « Bannière du portail captif » dans Paramètres →
  Hotspot : saisie d'URL https (placeholder Cloudflare R2), téléversement
  data URL ≤ 500 Ko, aperçu live, retrait, validation https/data:image
  alignée sur le backend ; i18n FR + EN.
- **Tests** : `TestPersonalizeMarkers` (marqueur bannière),
  `TestPersonalizeBannerURLInjection` (échappement HTML d'une URL
  malveillante), `TestPortalServeBanner` (propagation bout en bout dans le
  login.html servi), `TestSettingsBannerValidation` (matrice 400/200/retrait).
  CI backend + frontend vertes.

## 2026-09-06 — N°43/N°44 : vague Dependabot 2 adoptée — ESLint 10 ré-adopté, recharts 3 migré

### N°43 — ESLint 10 ré-adopté via @eslint/compat (#28, gérant)
- Le diagnostic N°40 (« 5 plugins sur 6 incompatibles — PR structurellement
  rouge ») était **factuel mais contournable** : le shim officiel
  `@eslint/compat` (`fixupPluginRules`) rétro-compatibilise les API de
  contexte supprimées par ESLint 10 pour les plugins legacy (react,
  react-hooks, jsx-a11y, import) — typescript-eslint 8.69 supporte
  nativement ^10.
- `eslint.config.mjs` : wrapper `retroCompatPlugins` appliqué à toute la
  config ; `package.json` : `eslint ^10` + `@eslint/compat 2.1` +
  **`overrides` déclaratif** pinant typescript-eslint/@typescript-eslint/*
  sur 8.69.0 (version propre de la résolution forcée N°40) ; l'ignore
  Dependabot `eslint >= 10` est retiré. CI 5/5 verte (lint sous ESLint
  10.10, typecheck tsgo, build, E2E). Fusion 8192c9e.

### N°44 — quatre bumps + migration recharts 3 (#24, #26, #27, #29, #25)
- **prisma 6 → 7** (#24) et **@prisma/client 6 → 7** (#26) : dépendances
  héritées du template, **jamais importées** dans `frontend/src` — zéro
  impact code, CI verte.
- **framer-motion 12 → 13** (#27) : très utilisé (app-shell, paywall,
  dialogs, pages join/wifi) — l'API consommée reste compatible, CI complète
  verte y compris E2E.
- **lucide-react 1.40** + **@types/react-dom** (#29, groupe minor/patch) :
  trivial.
- **recharts 2 → 3** (#25) : migration réelle — recharts 3 omet
  `active`/`payload`/`label` des props du composant `Tooltip`
  (`PropertiesReadFromContext`) et `payload`/`verticalAlign` des props de
  `Legend` :
  - `chart.tsx` : `ChartTooltipContent` typé `TooltipContentProps`
    (génériques par défaut — `TooltipPayload` est
    `Payload<ValueType,NameType>[]` non spécialisé), `ChartLegendContent`
    typé `LegendPayload[]` + union `verticalAlign` locale, clé de série
    React sur la clé calculée string (`DataKey` v3 peut être une fonction) ;
  - `sd-chart-tooltip.tsx` : `ChartTooltip` typé
    `Partial<TooltipContentProps<number,string>>` + formatter requis — les
    4 sites consommateurs (`router-tools.tsx`, `reports-view.tsx`) créent
    `content={<ChartTooltip formatter={...} />}` sans les props que
    recharts injecte au rendu.
- **Leçon tooling** : le cache incrémental de tsgo (`tsconfig "incremental":
  true` → `tsconfig.tsbuildinfo`) **masquait les erreurs transitoires** lors
  d'un changement de types de dépendances — local « 0 erreur » là où la CI
  (arbre frais) en voyait 4. Purger `tsconfig.tsbuildinfo` avant tout
  verdict de typecheck local.
- Fusion aa16ead ; **Render déployé manuellement** sur aa16ead
  (le run CI main du merge N°41 avait été annulé par une fenêtre de
  flakiness GitHub Actions — runs « pending » jamais démarrés, pushes
  synchronize sans run — le deploy-render de N°41 n'a jamais tourné ;
  Vercel, sur son webhook propre, est resté à jour en continu).

## 2026-09-06 — N°41 : salvage PR #21 (jules) — quota gratuit exposé au portail + téléphones locaux CI préfixés 225

### Revue de la PR #21 « Unified Captive Portal & Hybrid Cloud/Local WiFi Jetable Claim »
- **Convergence** : la PR (bot jules, base N°35-b) implémentait le portail
  hybride cloud/local — travail déjà fusionné sur main par la série N°35-c/d
  (fetch live `/api/wifi/site/{slug}/portal` + fallback inliné + claim inline,
  E2E + production). Son `login.html` réécrit (445 lignes) et son handler
  `handleWifiSitePortal` sont **éclipsés** par la version main — et son
  `routes.go` réenregistrait la route `GET /api/wifi/site/{slug}/portal`
  **déjà déclarée** (double enregistrement = panic au démarrage du mux Go,
  Render down). Fusion directe impossible.
- **Deux apports réels sauvés** (le reste fermé avec explication) :
  1. **Quota gratuit dans `PortalConfig`** — `freeTimeMin`/`freeDataMb`
     (0 site = hériter du profil) peuplés dans les **deux** builders
     (`buildPortalConfig` router-ancré pour le fallback inliné,
     `buildPortalConfigForSite` pour l'endpoint live). Le portail peut
     afficher la dotation gratuite (« X min offertes ») sans second appel ;
     le fallback et la config live portent la même donnée. Test API
     `TestWifiPortalFreeQuota` (héritage profil 30 min vérifié).
  2. **`NormalizeWifiPhone` : préfixe 225 automatique** — un numéro local
     Côte d'Ivoire (10 chiffres commençant par 01/05/07, format usuel des
     affiches) est préfixé par l'indicatif 225 avant validation. Fini les
     « numéro invalide » pour les visiteurs qui tapent leur numéro sans
     indicatif. Table de tests `TestNormalizeWifiPhone` (7 cas : séparateurs,
     +225/225 déjà présents, bornes).
- Aucun changement de schéma Neon (champs runtime uniquement, aucune
  colonne ajoutée) — la synchro différentielle du backend n'est pas impactée.

## 2026-09-06 — N°40 : vague Dependabot #16-#20 traitée — 4 majeures fusionnées, ESLint 10 écarté

### Quatre bumps majeurs adoptés (CI verte branche par branche, production vérifiée)
- **uuid 11 → 14** (#16), **lucide-react 0.525 → 1.39** (#19), 
  **react-syntax-highlighter 15.6 → 16.1** (#20), **@mdxeditor/editor 3.52 → 4.2** (#17) :
  chaque branche avait sa CI complète verte (lint + typecheck tsgo + build +
  E2E Playwright) ; fusions squash successives — `bun.lock` auto-fusionne
  (sections alphabétiques disjointes) — et CI main + Vercel vérifiés après
  chaque étape. Aucun code consommateur impacté : les usages du repo restent
  compatibles avec les API v14/v1/v16/v4.
- **Bilan dépendances frontend** : après les vagues N°35 (52 updates) et
  N°40 (4 majeures), l'inventaire Dependabot est à zéro PR ouverte côté bun.

### ESLint 9 → 10 (#18) : écarté proprement, écosystème pas prêt
- Diagnostic complet du crash CI (`Class extends value undefined` dans
  `@typescript-eslint/utils/.../FlatESLint.js`) : **deux couches**.
  1. typescript-eslint 8.53 ne supportait pas ESLint 10 — **résolu** : la
     version 8.69.0 (peer `^10.0.0` ajouté par l'écosystème) est compatible,
     validé en local (résolution forcée `typescript-eslint ^8.69.0` +
     purge de la copie imbriquée 8.53 du lockfile — le crash initial
     disparaît, révélant le bloqueur suivant).
  2. **Bloqueur dur restant** : `eslint-plugin-react` 7.37.5 (latest, peer
     max `^9.7`) plante en runtime sous ESLint 10
     (`getReactVersionFromContext`), et `react-hooks` 7.0.1,
     `jsx-a11y` 6.10.2, `import` 2.32.0 restent peer `^9` — **5 plugins sur
     6 incompatibles**. La PR serait structurellement rouge, comme #13 (TS 7).
- Même traitement que le chantier TS 7 : `.github/dependabot.yml` ignore
  désormais `eslint >= 10` (pas de PR rouge hebdomadaire), PR #18 fermée avec
  le diagnostic complet. Ré-adoption quand `eslint-config-next` (ou
  `eslint-plugin-react`) supportera ESLint 10.

## 2026-09-06 — N°36 : TypeScript 7 natif (compilateur Go) — typecheck gate CI + passif vagues 2-3 corrigé

### Compilateur natif adopté, API JS conservée
- **TypeScript 7.0.2** (le compilateur réécrit en Go, « tsgo » — 10× plus
  rapide) est adopté comme **type-checker du projet** : devDependency alias
  `tsgo: npm:typescript@7.0.2` + script `bun run typecheck`
  (`node node_modules/tsgo/bin/tsc --noEmit` — chemin explicite car bun lie
  les bins sous le nom déclaré du paquet `tsc`, l'alias ne crée pas de lien
  `.bin/tsgo`).
- **Le paquet `typescript` reste en 5.9.3** : TS 7 n'expose plus d'API
  JavaScript classique (`exports['.']` = stub de version, binaire natif par
  plateforme) et la chaîne ESLint (`typescript-estree` de
  `eslint-config-next`) plante au chargement avec TS 7 hoisté
  (`TypeError: Cannot read properties of undefined (reading 'Cjs')`,
  `ts.Extension` undefined — PR #13). typescript-eslint reste verrouillé sur
  `>=4.8.4 <6.0.0` (état écosystème sept. 2026).
- **Dependabot écarté du piège** : `.github/dependabot.yml` ignore
  désormais les bumps `typescript >= 7` (sinon Dependabot rouvrirait chaque
  semaine une PR équivalente à la #13, structurellement rouge). Ré-adoption
  du paquet `typescript` quand typescript-eslint supportera TS 7.

### Passif des vagues 2-3 découvert et corrigé
- Découverte : `next build` a `typescript.ignoreBuildErrors` dans
  `next.config` — **aucun type-check n'a jamais gate la CI frontend**. Les
  bumps majeurs fusionnés en N°35 (react-day-picker 9→10 #11,
  react-resizable-panels 3→4 #14) avaient cassé **en silence** deux
  composants shadcn scaffoldés (non importés : aucun impact runtime) :
  - `calendar.tsx` : clé `table` renommée `month_grid` (v10, enum
    `MonthGrid` de `UI.d.ts`) ;
  - `resizable.tsx` : réécrit pour l'API v4 — `PanelGroup` → `Group`,
    `PanelResizeHandle` → `Separator`, prop `direction` → `orientation`,
    sélecteurs CSS `data-panel-group-direction` → `aria-orientation`
    (v4 pose l'attribut sur le Separator lui-même).
- **Nouveau gate CI** : le job frontend exécute désormais
  `bun run typecheck` (TypeScript 7 natif) après le lint — le type-check
  devient bloquant, il ne peut plus y avoir d'erreurs de type dormantes.

### Vérifications
- `bun run typecheck` (tsgo 7.0.2) : **0 erreur** sur tout le frontend ;
  test négatif validé (sonde d'erreur volontaire → TS2322, exit 1).
- `bun run lint` (ESLint 9 + typescript 5.9.3 API JS) : vert.
- `bun run build` (Next.js 16.3.4) : vert.
- PR #13 (bump direct typescript 5.9→7.0.2) : fermée, remplacée par cette
  adoption en deux couches (compilateur natif via `tsgo` + API JS 5.9 pour
  ESLint) — la seule voie compatible écosystème en sept. 2026.

## 2026-09-05 — N°35 : audit branches & CI — Dependabot npm→bun + gouvernance

### Audit des branches (10 identifiées)
- **Cause racine des échecs CI sur les PR Dependabot frontend** : l'écosystème
  `npm` de Dependabot met à jour `package.json` mais **jamais `bun.lock`** ;
  la CI (`bun install --frozen-lockfile`) échouait donc systématiquement
  (« lockfile had changes, but lockfile is frozen ») — vérifié dans les logs
  des 5 PR concernées (sharp #4, uuid #5, lucide-react #6, react-table #7,
  eslint #8 — le job E2E de #8 échouait pour la même raison). Aucune PR
  frontend Dependabot ne pouvait passer.
- **Correctif structurel** : `.github/dependabot.yml` passe l'écosystème
  frontend de `npm` à `bun` — Dependabot met désormais à jour `bun.lock`
  lui-même et les PR redeviennent CI-compatible. Les 5 PR npm rouges et
  périmées (34–59 commits de retard) seront fermées et recréées par
  Dependabot sous le nouvel écosystème.
- **Branche `feature/agent-poll` supprimée** : entièrement fusionnée dans
  `main` (0 commit propre, 205 de retard) — branche morte.
- **Gouvernance** : protection de branche `main` activée via API —
  force-push et suppression interdits, **sans exiger de PR ni de checks**
  (le workflow « push main = production » est préservé).

### État des 8 PR Dependabot (audit)
| PR | Mise à jour | Fusion Git | CI | Verdict |
|---|---|---|---|---|
| #1 | actions/checkout 4→7 | propre | ✅ verte | prête à fusionner |
| #2 | actions/setup-go 5→7 | propre | ✅ verte | prête à fusionner |
| #3 | groupe go minor/patch | propre | ✅ verte | prête à fusionner |
| #4 | sharp (groupe minor) | propre | ❌ lockfile | fermer → recréée (bun) |
| #5 | uuid 11→14 (majeure) | propre | ❌ lockfile | fermer → reprise dédiée |
| #6 | lucide-react 0→1 (majeure) | propre | ❌ lockfile | fermer → reprise dédiée |
| #7 | react-table 8→9 (majeure) | propre | ❌ lockfile | fermer → reprise dédiée |
| #8 | eslint 9→10 (majeure) | propre | ❌ lockfile | fermer → reprise dédiée |

> Les 4 mises à jour majeures (uuid, lucide-react, react-table, eslint)
> demanderont une adaptation de code — à traiter une par une, pas en fusion
> directe.

## 2026-09-05 — N°34 : Hotspot Page — réorganisation du portail captif

### Nettoyage (`Hotspot Page/`)
- **Fichiers parasites retirés** (tous récupérables via
  `git show 913b151:'<chemin>'`) : `debug.log` (log Windows parasite),
  `euh.html` (brouillon « Mnaspot » remplacé par le login FTCI), `engine1/` +
  `data1/` (slider WOW Slider généré, référencé par aucune page — ~290 Ko),
  `css/style.css` + `css/mikhmon-ui-light.css` + `css/background.css` (CSS du
  template Mikhmon d'origine, non liés), `js/jquery-3.2.1.min.js` (n'exigeait
  que WOW Slider) et `js/typed.min.js` (doublon de `typed.umd.js`),
  `img/bg-body.png` + `img/favicon.png` (non référencés). Gain ~800 Ko de
  flash routeur ; seul l'utile est désormais téléversé.
- **Correction 404** : `login.html` chargeait `js/bootstrap.bundle.min.js`
  absent du dossier (404 systématique sur le portail) ; le script est retiré —
  aucun composant Bootstrap JS n'est utilisé (grille/utilitaires CSS
  uniquement), Swiper/Typed restent inchangés.
- **README du dossier réécrit en français** : contrat des noms de fichiers
  RouterOS (pages obligatoires à la racine, immuables), inventaire assets,
  déploiement FTP/Winbox, intégration MikCloud (`/join/{token}?mac=`,
  walled-garden, portail kiosque 45 s) et guide de personnalisation (couleurs
  `:root`, offres Wave, carrousel, logo, messages Typed).

## 2026-09-05 — N°33 : inscriptions publiques — anti-abus kiosque + redirection portail 45 s

### Sécurité / anti-abus (module inscription N°27)
- **Plafond kiosque par numéro de téléphone** : au plus 1 compte AUTO-VALIDÉ
  par numéro et par 24 h glissantes, par compte (tous liens kiosque
  confondus) — erreur `phone_limit` (409). Sans ce plafond, le
  dédoublonnage téléphone ne couvrait que la file d'attente : en mode
  kiosque la demande passait directement « approved », un même numéro
  pouvait donc créer un compte à chaque soumission (gratuité répétée).
- **Quota anti-abus par APPAREIL (MAC)** : la page de login du routeur peut
  désormais pointer vers `/join/{token}?mac=$(mac-esc)` ; la MAC (normalisée,
  invalidée silencieusement si malformée) porte un second quota cumulé
  5/10 min + 20/24 h à côté du quota IP — derrière le NAT du hotspot, tous
  les clients partagent la MÊME IP publique : la MAC est la seule clé qui
  isole réellement un fermier de comptes sur place. Stockée sur la demande
  (`createdMac`, colonne Neon `created_mac` auto-migrée).
- **Politique mot de passe publique renforcée** : 6 → 8 caractères minimum,
  denylist S2 (mots de passe les plus courants) et interdiction
  « identique au nom d'utilisateur » — côté page publique ET approbation
  console (mot de passe choisi) ; l'auto-génération (6 caractères serveur)
  reste inchangée.
- **Demandes pending oubliées** : le mot de passe clair est VIDÉ au sweep
  après 30 jours (minimisation — avant, une demande jamais tranchée gardait
  son secret indéfiniment) ; la demande reste décidable (l'approbation
  génère alors un mot de passe).

### Ajouté (mode kiosque)
- **Redirection portail après 45 s** : sur la page publique, une inscription
  auto-validée affiche désormais un compte à rebours (45 s) puis envoie le
  navigateur vers une URL HTTP neutre (`connectivitycheck.gstatic.com/generate_204`)
  — interceptée par le routeur MikroTik, elle rouvre la page de login du
  hotspot où l'utilisateur saisit ses nouveaux identifiants. Échappatoires
  manuelles : « Se connecter maintenant » (immédiat) et « Rester sur cette
  page » (annule le compte à rebours pour recopier les codes). HTTP
  obligatoire : seul le trafic HTTP est interceptable sans erreur de
  certificat.

### Page login routeur (`login.html`)
- Livrée corrigée à part : CSP réellement restrictive (l'ancienne
  `default-src *` n'interdisait rien), zoom mobile réautorisé
  (accessibilité), bouton QR externe (site tiers hors walled-garden,
  inutilisable pré-auth) remplacé par « Créer un compte » pointant vers le
  lien d'inscription MikCloud avec la MAC de l'appareil.

## 2026-09-05 — N°31 : audit walled-garden agent — reprise des commandes perdues + diagnostic visible

### Corrigé (audit en profondeur suite à « aucune règle sur un routeur client »)
- **Commande `walled_garden` perdue « en vol » = deadlock silencieux** : un
  rapport jamais reçu (blip réseau entre l'import et le fetch de rapport,
  reboot en cours de check-in…) laissait la commande « sent » à jamais —
  le cloud la croyant en cours, il ne la re-file jamais : walled-garden
  jamais appliqué, aucun signal. Elle est désormais reprise automatiquement
  après 10 min sans rapport (bloc idempotent par conception, marqueur
  `mikcloud-wg`), comme les lectures — auto-guérison en ≤ 2 check-ins.
- **Routeur RouterOS < 7.19 invisible** : au moment de l'installation de
  l'agent, le refus (TLS strict) est désormais journalisé dans le Journal du
  gérant — avant, le routeur semblait « En ligne » mais restait muet à
  jamais (aucune commande, aucun symptôme côté console).

### Documentation
- RUNBOOK-WALLED-GARDEN §5-bis : checklist de diagnostic en 5 points pour
  « aucune règle sur le routeur en mode agent » (déploiement, mode agent +
  dernière connexion, version ≥ 7.19, Journal, commande perdue) + note
  terrain : inscriptions (N°27) et WiFi jetable (N°28) partagent le MÊME
  walled-garden — une seule cause possible quand il manque.
### Complément N°31-b — hygiène des domaines (`wgHostUsable`)
- La configuration ne retient plus que les hôtes réellement joignables depuis
  un client du WiFi : localhost, *.localhost, *.local, loopback, RFC1918,
  link-local et 0.0.0.0 exclus (constat prod : « localhost:3000 » issu des
  origines de dev de ALLOWED_ORIGIN polluait la liste ; un `dst-host` avec
  port ne peut de toute façon pas matcher HTTPS/SNI). Hôtes publics éligibles,
  port numérique compris. Test dédié (15 cas).
### Complément N°31-c — script walled_garden blindé : find exact + battement de cœur + chunk en fin de file
- **Constat approfondi** : le chunk `walled_garden` tue l'import RouterOS du
  script ENTIER — à 17:12:31, le check-in servait [walled_garden, read_state] :
  les DEUX sont restés muets, puis tout est redevenu sain dès 17:19
  (user_remove, read_state… done en 3-5 s). Reproductible 2×/2× (09:50 et
  17:12). Les commandes du même check-in étaient empoisonnées avec lui.
- **Find EXACT** (`find comment="mikcloud-wg page|dns"`) remplaçant le regex
  `find comment~"…"` — même classe syntaxique que les `find name="…"` des
  user_remove (prouvé terrain) ; nos règles portent exactement ces deux
  commentaires, la suppression exacte reste complète. Suspect n°1 éliminé.
- **Battement de cœur** : le script poste `status=started` AVANT les lignes à
  risque (construct fetch prouvé 849×) — si l'import meurt ensuite, le cloud
  sait au moins que le fichier est arrivé ; le serveur tolère ce statut
  (commande laissée « sent », réponse heartbeat).
- **Chunk en fin de file** : les walled_garden sont servis en DERNIER dans le
  script du check-in — plus jamais de commandes métier/télémétrie prises en
  otage par une ligne walled-garden fatale (la reprise zombie N°31 retente).
- Syntaxe identique dans le bloc d'installation des routeurs neufs ; tests
  mis à jour + interdiction du find regex dans le script généré.
### Correctif N°31-d — LA cause racine : `action=allow` invalide sur `walled-garden ip`
- Doc officielle HotSpot : la table `/ip hotspot walled-garden` (domaines)
  accepte `action=allow|deny` — mais la table `/ip hotspot walled-garden ip`
  (DNS udp/tcp 53) n'accepte QUE `accept|drop|reject`. Nos lignes DNS portaient
  `action=allow` → **erreur de validation console qui rejetait le fichier
  d'import ENTIER** (les read_states du même check-in mouraient avec lui —
  4 livraisons muettes 4×/4×, y compris le runbook manuel N°27-D corrigé
  rétroactivement). Correctif : `action=accept` (script + bloc installation
  + runbook §2). Test mis à jour.
### Correctif N°31-e — les removes « en usage » ne font plus échouer la mise à jour
- Après le premier succès (18:26:16, walled-garden appliqué !), les re-filés
  de mise à jour échouaient : les `remove` de règles DÉSORMAIS UTILISÉES par
  les clients du hotspot (flux DNS permanents sur les règles udp/tcp 53)
  lèvent une erreur RouterOS → okVar=false → error, en boucle.
- **Removes silencieux** (on-error={}, best-effort) + **adds conditionnels à
  l'absence** (`:if ([:len [find comment=… dst-host=…]] = 0) do={ add … }`) :
  si le remove échoue, la règle existe DÉJÀ (service assuré) → skip, pas de
  doublon, pas d'erreur. Seule une vraie erreur d'add échoue. Le bloc
  d'installation est aligné (re-collage idempotent).
### Suite N°32 — audit terrain : première application OK, traçage « step » + runbook enfin propre
- **Vérification prod post-déploiement N°31-d** (18:16:23 live) : la
  commande zombie c-998288c03052 re-servie à 18:26:02 (reprise N°31,
  10 min sans rapport) est passée **done/ok en 14 s** — walled-garden
  ENFIN appliqué sur le routeur client, Journal « Walled-garden …
  appliqué ». La cause racine N°31-d est confirmée terrain.
- **Traçage `step`** (complément N°31-e) : les removes étant désormais
  best-effort, seuls les adds peuvent porter okVar à false — chaque bloc
  à risque est précédé de `:set step` et le rapport d'erreur embarque la
  ligne fautive (« &step=" . $step ») : diagnostic sans accès console au
  routeur client.
- **Runbook N°27-D achevé** : la ligne DNS **TCP** §2 et la procédure
  WinBox §3 portaient ENCORE `action=allow` (le correctif N°31-d n'avait
  passé que l'UDP) — corrigées + encadré d'avertissement accept/allow par
  table ; entête N°29 du CHANGELOG dédupliqué.

## 2026-09-05 — N°29 : walled-garden d'inscription publique automatisé par l'agent (routeurs neufs ET déjà en ligne)

### Le runbook N°27-D appliqué par le système lui-même
- Nouvelle commande agent **`walled_garden`** : pose sur le routeur les règles
  qui rendent la page `/join/{token}` et son API joignables SANS
  authentification depuis le WiFi du hotspot (le scan du QR fonctionne sur
  place) — **idempotente** : seules les règles marquées `mikcloud-wg` sont
  remplacées, les règles personnelles du gérant sont préservées ; + 2 règles
  DNS udp/tcp 53 pour la robustesse de la résolution.
- **Routeurs déjà en ligne** : à chaque check-in (≤ 45 s), le cloud compare la
  signature de la configuration à celle déjà appliquée sur le routeur
  (`routers.walled_garden_sig`) — différente → mise à jour en file, servie
  dans le MÊME check-in ; signature posée uniquement à la confirmation du
  routeur (échec → retry automatique au check-in suivant, changement de
  config → re-file). **Aucun recollage manuel** sur le parc existant.
- **Routeurs neufs** : le script d'installation embarque le même bloc
  (multi-lignes — règle du parseur console —, idempotent).
- Domaines déduits du déploiement (hôte API + origines page via CORS
  `ALLOWED_ORIGIN` / `APP_PUBLIC_URL`), assainis (anti-injection), triés,
  plafonnés à 10.
- 6 tests dédiés : assainissement, script (règles + rapport), bloc
  d'installation, exactly-once / re-file sur changement / retry sur échec.

## 2026-09-05 — N°27-D : runbook walled-garden (inscriptions publiques accessibles depuis le WiFi du hotspot)

### Documentation opérateur — `docs/RUNBOOK-WALLED-GARDEN.md`
- Sans walled-garden, le QR de la N°27 ne se charge que depuis la **4G** :
  le portail captif MikroTik intercepte tout le trafic des appareils non
  authentifiés — précisément ceux qui n'ont pas encore de compte.
- Le runbook couvre : les **2 domaines** à autoriser (page `mikcloud.ftci.fr`
  + API `mikcloud.onrender.com` — variante `api.<domaine>` Cloudflare),
  procédures **CLI RouterOS v6/v7 et WinBox** (règles par domaine + DNS
  udp/tcp 53), fonctionnement réel (reniflement DNS → HTTP/HTTPS couverts),
  **vérification de bout en bout** avec appareil témoin, **dépannage**
  (DNS codé en dur, DNS chiffré/DoH, portail captif auto-ouvert, TLS),
  **périmètre de sécurité** (anti-patterns : wildcards génériques, IP en
  dur, ouverture 80/443) et rappel du flux complet N°27.

## 2026-09-05 — N°27 : inscriptions publiques par QR code (Campus & écoles, administration, entreprise)

### N°27 — Le gérant génère un QR, l'utilisateur s'inscrit, le gérant valide
- **Nouveau flux d'inscription publique** : depuis l'onglet console
  **« Inscriptions »** (rubrique Utilisateurs), le gérant crée un LIEN
  d'invitation (nom, profil et routeur pré-attribués optionnels, validation
  automatique « kiosque » opt-in, limite d'usages, expiration) — la console
  l'encode en **QR code** + **affiche A4 imprimable** (QR, URL, 3 étapes).
- **Page publique `/join/{token}`** (SANS authentification — le token de
  32 caractères stocké côté serveur fait l'accès : révocable instantanément,
  compteur d'usages, expiration) : l'utilisateur scanne, remplit le
  formulaire mobile-first (nom, téléphone, identifiant, mot de passe ×2,
  message, honeypot anti-bot) et soumet.
- **File de validation** dans la console : la demande atterrit « en attente »
  (compteurs par statut) ; le gérant **attribue le profil** (+ routeur,
  identifiants éditables, aperçu de validité) et **valide** — l'utilisateur
  hotspot est créé par le même cœur que la console (`createHotspotUser`,
  extrait de `handleUserCreate`) : la validité démarre à l'APPROBATION, la
  file agent `user_add` s'enchaîne, le tombstone éventuel est levé. Refus
  avec motif possible ; historique = annuaire des inscrits.
- **Mode de connexion « Nom d'utilisateur & Mot de passe »** (deux codes
  distincts au choix de l'utilisateur) — distinct des vouchers, restés
  verrouillés « nom d'utilisateur = mot de passe » (N°25).
- **Sécurité** : whitelist publique limitée à `/api/join/{token}`
  (PAS `/api/join-links`), rate-limit dédié (10/min/IP) + quota anti-abus
  par IP réutilisé (5/10 min, 20/24 h), honeypot à succès factice, GET public
  minimal (jamais le catalogue de profils), mot de passe de la demande VIDÉ
  à l'approbation comme au refus, demandes refusées purgées à 30 jours
  (`sweepStaleRegistrations`, hook `enforceExpired`).
- Migration **purement additive** (2 tables : `join_links`,
  `registration_requests` — aucune colonne ajoutée à `hotspot_users`) ;
  QR généré côté navigateur (libs déjà présentes, zéro nouvelle dépendance) ;
  lien kiosque `autoValidate` = création immédiate du compte à la soumission.
- Tests : `handlers_join_test.go` (cycle complet, garde-fous de lien,
  honeypot, kiosque + file agent, scoping inter-comptes, sweep 30 j).

## 2026-09-05 — N°25/N°26 : verrou « code unique » + lots éteints auto-supprimés

### N°25 — Génération verrouillée au code unique
- **Toute création de voucher est désormais en mode « nom d'utilisateur =
  mot de passe »** (un seul code par ticket) — verrouillé côté SERVEUR
  (`samePassword := true`, la valeur `userMode` du client est ignorée :
  console, PWA, appel API direct) ET côté wizard (le choix « 2 codes » est
  retiré, carte « Verrouillé — un seul code par ticket » à la place, aperçu
  sans mot de passe). Contrat inchangé (champ accepté, ignoré).

### N°26 — Un lot dont tous les tickets ont expiré disparaît du système
- **Auto-suppression des « lots éteints »** (sweep `sweepDeadBatches`, branché
  en fin d'`enforceExpired` — passage commun console/agent 45 s/PWA, sous
  verrou, persisté par l'appelant) : tickets supprimés, ligne de lot
  supprimée (plus de lot « Expiré » zombie même sous le filtre « Tous »),
  sessions abandonnées, **tombstones anti-résurrection posés** (les tickets
  restent sur le routeur réel : sans tombstone, la synchro agent les
  réimporterait en fantômes) et **user_remove** par paquets de 50 enfilé
  pour chaque routeur AGENT (RouterOS reste propre).
- Garde-fous : ≥ 1 ticket (les lots vides relèvent de la purge manuelle) ;
  **aucun ticket revendeur** (N°23/W1 — la trace du stock confié prime, la
  reprise puis la suppression individuelle restent la voie) ; ventes et
  transactions INTACTS (la comptabilité du lot supprimé demeure) ; journal
  d'activité par compte (« Lots éteints supprimés automatiquement : N »).
- Test : `TestSweepDeadBatches` (suppression B1/B4, conservation mixte B2 et
  revendeur B3, tombstones, 1 commande agent / 0 simulé, sessions, vente,
  idempotence).

## 2026-09-05 — N°23 : reprise gérant (W6), verrou de destruction du stock revendeur (W1), visibilité de l'allocation (W3/W4)

### Ajoutés
- **Reprise gérant** (POST /api/vouchers/reprise, rôle 2) — miroir exact du
  retour de stock du revendeur (N°20) mais à l'INITIATIVE du gérant (revendeur
  absent, litige, désengagement) : le ticket invendu redevient du stock direct
  (`ResellerID`/`ResellerName`/`CreditSale` vidés) ; l'argent suit le retour —
  prépayé : portefeuille recrédité du prix GROS du stock VIVANT + UNE
  transaction « credit » agrégée par revendeur ; dépôt-vente : aucun recrédit ;
  expiré/désactivé : aucun recrédit (le ticket a péri chez le revendeur) mais
  la reprise ferme la boucle (suppression ensuite possible). Tickets DÉJÀ
  REMIS au client : refusés en 409 tout-le-lot — la créance est née, le ticket
  en est la preuve. Trace d'activité avec décompte par revendeur et recrédits.
  UI : action « Reprendre au stock » sur les tickets revendeur invendus +
  dialogue de confirmation explicite (recrédit / dépôt-vente), toasts de
  résultat.
- **Filtre « Détenteur »** sur la liste des vouchers (W3/W4) : Tous / Stock
  direct (gérant) / Alloués aux revendeurs — paramètre `holder` additif des
  listes (contrat préservé), KPI « Alloués » ajouté à la bande de statistiques.

### Corrigés
- **Verrou de destruction du stock revendeur (W1)** : un ticket attribué à un
  revendeur n'est plus supprimable par AUCUNE porte console — suppression
  unitaire (403 `reseller_voucher_locked`), suppression groupée (409
  tout-le-lot), suppression d'un lot entier (409, comptes sans codes réels) ;
  le nettoyage des expirés épargne désormais les tickets revendeur (même
  expirés, ils restent la trace du stock confié). Après reprise ou retour de
  stock, toutes les portes se rouvrent normalement — aucun zombie.

### Sécurité
- Message de la garde de code N°22 mis à jour : la voie propre est désormais
  « reprise OU retour de stock ».
- Tests : `handlers_reprise_test.go` (recrédit prépayé, dépôt-vente, refus
  all-or-nothing, boucle fermée expiré→suppression, gardes W1 unitaire/bulk/
  lot/cleanup, filtre holder) ; E2E N°23 (reprise 2 tickets → recrédit
  vérifié sur le portefeuille, gardes 403/409, filtres détenteur, boucle
  fermée). CI 5/5 requis avant push.

## 2026-09-05 — N°22 : les codes des tickets revendeur ne fuient plus par la console gérant

### Corrigés
- **Vente en direct aux dépens du revendeur** : le gérant voyait dans la
  console les codes (et mots de passe) de TOUS les vouchers, y compris ceux
  déjà transférés à un revendeur — il pouvait les dicter au comptoir, les
  copier, les réimprimer ou même RÉÉCRIRE leur code, puis encaisser le cash
  pendant que la vente auto (auto_connect) décomptait le stock / créait la
  créance dépôt-vente CHEZ LE REVENDEUR.

### Ajoutés
- **Canal d'impression tracé** (POST /api/vouchers/print) : le gérant reste
  L'IMPRIMEUR DE SERVICE du revendeur (thermique 58/80 mm, grille A4) — il
  demande souvent à son gérant d'imprimer ses tickets. L'impression reste
  donc possible et devient le SEUL canal de sortie des codes : codes complets
  rendus pour l'impression en cours, action TRACÉE dans le journal d'activité
  (« Codes remis pour impression : N ticket(s) revendeur(s) — {revendeur} : M »),
  propriété INCHANGÉE (la vente reste créditée au revendeur à la 1ʳᵉ connexion
  du client). Le stock direct du gérant s'imprime comme avant, sans tracé.

### Modifiés
- **Listes console masquées** (/api/vouchers, /api/users, export CSV) : tout
  voucher attribué sort avec un code « •••••• » et sans mot de passe ; badge
  cadenas « Attribué à {revendeur} », copie et révélation supprimées. La
  recherche par code réel fonctionne toujours (vérifier un ticket papier qui
  revient au comptoir reste possible — la ligne ressort masquée).
- **Code verrouillé** : réécrire username/password d'un ticket revendeur
  depuis la console est refusé (403 structuré `reseller_voucher_locked`) —
  formulaire d'édition verrouillé avec rappel « retour de stock » (la voie
  propre pour récupérer un ticket, recrédite le revendeur).
- **Réponses unitaires masquées** : PUT /api/users/{id}, enable/disable,
  extend ne renvoient plus le vrai code d'un ticket attribué.
- Bandeau d'impression « pour le compte des revendeurs » dans le dialog
  (impression simple, lots, réimpression rapide F12, transfert A4, génération).

### Tests
- Go : masquage listes + CSV, canal d'impression tracé (et sans trace pour le
  stock direct), garde 403, écho username toléré, réponse PUT masquée.
- E2E (projet resellers) : transfert partiel → liste masquée, impression
  tracée (tracedCount=1, code complet rendu), 403 à la réécriture, recherche
  par code réel.

## 2026-09-04 — Mode Vente anti-fuite : le code ne se partage qu'APRÈS confirmation

### Corrigés
- **Partage anticipé (fuite comptoir)** : sur une carte de ticket EN STOCK, le
  bouton « Partager » (Web Share/presse-papiers) et le code + mot de passe en
  clair permettaient au revendeur de remettre le code au client AVANT
  confirmation — la vente n'était alors jamais marquée « vendu » : trace
  SoldAt anti-vol contournée, créance dépôt-vente et CA faussés.

### Modifiés
- **Carte de ticket muette** : code et mot de passe masqués (« •••••• »,
  rien de copiable dans le DOM), bouton « Partager » supprimé, « Vendu » en
  pleine largeur, et rappel « Code visible et partageable après confirmation
  de la vente » ; le récapitulatif pré-confirmation (UX R2) reste lui aussi
  muet sur le code.
- **Reçu « Vente confirmée »** : après confirmation (vente tracée OU file
  hors-ligne), le code + mot de passe s'affichent en grand (sélectionnables)
  avec le bouton « Partager » — le geste de remise au client vit À ce moment,
  jamais avant ; badge « enregistrée hors ligne » le cas échéant. Le code
  d'un ticket vendu reste consultable dans le rapport de journée.

### Tests
- E2E réécrits : la recherche et le récapitulatif n'affichent plus le code ;
  le reçu expose le code et le bouton « Partager » aboutit dans le
  presse-papiers (contenu vérifié).

## 2026-09-04 — V1→V5 : suppression des revendeurs maîtrisée — garde-fous, cascade et purge des orphelines

### Corrigés
- **Transactions orphelines immortelles** : supprimer un revendeur ne touchait
  QUE sa ligne — son historique de transactions (crédit, créance, versements)
  survivait ensuite à TOUTES les purges, y compris « all » (le scope
  « Revendeurs » ne filtrait que les revendeurs encore présents), ce qui rendait
  l'annonce « et leurs N transaction(s) » mensongère. Le scope purge désormais
  les transactions orphelines (ResellerID sans revendeur) — l'historique fantôme
  déjà en production est nettoyé au premier passage, idempotent.
- **Mode Vente zombi** : un token de vente (TTL 24 h) survivait au DELETE du
  revendeur et pouvait encore marquer des ventes et CRÉER des créances
  fantômes, le garde de plafond étant silencieusement sauté quand le revendeur
  n'existait plus. Tout /api/sell/* exige désormais l'existence du revendeur
  (403 « Session expirée : revendeur supprimé » → la PWA déconnecte).
- **Message de suppression trompeur** (i18n FR/EN) : il annonce désormais la
  cascade réelle (historique supprimé, ventes conservées) et la règle des
  garde-fous ; le toast de succès affiche le volume d'historique purgé.

### Ajoutés
- **Garde-fous au DELETE revendeur (V1)** : suppression refusée en 409
  structuré (`code=reseller_not_settled`, payloads credit/debt/stock) tant que
  le revendeur porte un crédit restant, une créance dépôt-vente ou du stock
  attribué vendable — motifs détaillés affichés tels quels par l'UI. Le DELETE
  exige aussi un compte non expiré (guardAccountWrite, aligné sur le reste du
  CRUD).
- **Cascade de suppression (V2)** : une fois soldé, le revendeur part avec
  TOUT son historique de transactions ; ses vouchers attribués restants
  (vendus/expirés) sont détachés (ResellerID vidé, ResellerName conservé en
  trace) ; ventes (Sale) et lots (Batch) volontairement conservés (comptabilité
  du gérant, génération immuable). Réponse enrichie {transactionsPurged,
  vouchersDetached} ; ligne d'activité de traçabilité.
- **Tests** : garde-fous (3 motifs), cascade (compteurs + détachement +
  préservations), purge des orphelines (isolation stricte du compte témoin),
  révocation Mode Vente après DELETE, cas « revendeur inexistant » dans la
  matrice d'authentification.

## 2026-09-04 — UX R7 : sémantique trafic verrouillée — upload/download dans le bon sens

### Corrigés
- **Upload et download affichés inversés** (Sessions) : la doc officielle MikroTik
  (help.mikrotik.com — HotSpot) définit les compteurs du point de vue du ROUTEUR :
  `bytes-in` = bytes **uploadés** par le client, `bytes-out` = bytes **téléchargés**.
  L'UI étiquetait l'inverse (KPI « Trafic descendant » = Σ bytesIn, lignes
  ↓/↑ échangées) — sur routeur réel, download ≫ upload rendait l'inversion visible
  et les valeurs « paraissaient tronquées ». La démo (simulateur) masquait le bug :
  elle générait bytesIn comme le gros débit — hypothèse intuitive mais fausse.
- **Export CSV utilisateurs** : colonnes « Data entrée (Mo)/Data sortie (Mo) » →
  « Upload (Mo)/Download (Mo) » (même piège de point de vue ; ordre inchangé).

### Ajoutés
- **Sémantique verrouillée par accesseurs** (`frontend/src/lib/hotspot/traffic-semantics.ts`)
  : `upBytes`/`downBytes` — un seul endroit connaît la correspondance RouterOS ↔ client ;
  aucun composant ne doit consommer `.bytesIn`/`.bytesOut` brut pour un affichage
  directionnel. Sessions migrée vers les accesseurs.
- **Test de direction** `TestSimulatedSessionTrafficDirection` : le simulateur doit
  accumuler download > upload (intervalles de débit disjoints, dominance déterministe,
  robuste à la déconnexion aléatoire de démo ~12 %).
- **Doc verrouillée** : note « Sémantique des compteurs de trafic » dans
  CONTRACT-V2.md + commentaires de référence (model.HotspotUser/Session, gateway,
  simulateur).

### Corrigés (CI)
- `gofmt` sur `internal/api/purge_tombstones_test.go` (indentation espaces →
  tabulations — job Backend Go en échec depuis 32397f1, déploiement Render bloqué).

## 2026-09-04 — P3-e : pagination du stock + tests E2E Playwright en CI

### Ajoutés
- **Pagination du stock** (`GET /api/sell/stock`) — additive : sans paramètre,
  la réponse reste le tableau historique complet (aucune PWA cassée) ; avec
  `limit` (1..200) + `offset`, la réponse devient une page explicite
  `{items, total, hasMore}` sur le même tri anti-chronologique (stable).
- **PWA revendeur — chargement par pages** (`useInfiniteQuery`, page de 60) :
  un gros stock ne plombe plus le premier rendu ni le payload mobile. Bouton
  « Afficher plus (X sur Y) » en bas de liste ; le snapshot hors-ligne (UX R6)
  persiste l'état COMPLET chargé (toutes pages) et sert de page unique quand
  le réseau tombe. **La recherche reste exhaustive** : si des pages restent à
  charger, elles se chargent automatiquement — « Aucun ticket ne correspond »
  ne peut plus mentir sur un stock partiellement chargé (invariant R3).
- **Tests E2E Playwright en CI** (`frontend/e2e/`, job `e2e` bloquant avant
  déploiement Render) : la stack réelle est levée par le config (backend Go
  `go run` avec store JSON éphémère + frontend Next.js de production) et le
  parcours revendeur complet est vérifié — bootstrap API (compte, routeur,
  profil, lot de 70 + lot de 3, revendeur PIN, une vente tracée), login PIN
  par l'UI, pagination « Afficher plus », recherche exhaustive d'un ticket de
  2ᵉ page, vente tactile avec confirmation R2 (Annuler + Confirmer), rapport
  de journée avec ventilation par canal et export CSV (BOM, contenu, nom de
  fichier). Sessions injectées en localStorage : `/api/reseller/login`
  (5 req/min/IP) n'est appelé qu'une fois par run.
- **CI** : le job `e2e` s'ajoute aux 4 vérifications existantes et bloque
  désormais le déploiement Render (`needs: [backend, frontend, e2e]`) ;
  rapport HTML + traces conservés en artefact 7 jours en cas d'échec.

### Notes
- La simulation d'activité (routeurs « simulated ») ne touche pas le stock de
  la PWA : les routes `/api/sell/*` ne déclenchent pas `store.Tick` — vérifié
  par les tests (stock stable entre les pages).
- Les tests ne comptent jamais sur le hasard du simulateur : les fixtures
  passent par l'API réelle et les codes de 2ᵉ page proviennent de l'endpoint
  paginé lui-même.

## 2026-09-04 — P3-d : rapport de journée enrichi + export comptable « journal de caisse »

### Ajoutés
- **Export CSV comptable** (`GET /api/sell/day-report.csv` — PWA revendeur) :
  journal de caisse Excel-fr (séparateur « ; », BOM UTF-8, CRLF — format aligné
  sur l'export console) téléchargeable depuis le rapport de fin de journée.
  Paramètre `date=AAAA-MM-JJ` optionnel : aujourd'hui par défaut, toute date
  passée admise (compta — les lignes stock/créance, état courant, ne figurent
  que pour le jour en cours). Sections : ventes (heure, code, profil, prix,
  canal), retours de stock (flux cash), versements dépôt-vente, totaux du jour.
- **Rapport enrichi** (champs additifs, `omitempty` — zéro rupture de contrat) :
  `soldVia` par ligne de vente + ventilation `byVia` (tactile / auto connexion /
  papier historique — les ventes pré-R4, sans trace, sont affichées tactiles),
  `returnedCount`/`returnedCredited` (retours du jour avec flux cash — un
  retour en dépôt-vente ne déplace pas d'argent et n'entre donc pas au
  journal), `settledToday` (versements déjà encaissés par le gérant — le reste
  à verser est `toDeposit − settledToday`).
- **UI rapport** : chips de ventilation par canal, ligne retours, ligne
  « déjà versé aujourd'hui » dans l'encart dépôt-vente, icône de canal sur
  chaque vente (title au survol), bouton « Exporter CSV » (téléchargement
  authentifié via `apiDownload`) ; le partage WhatsApp inclut la ventilation
  et les nouvelles lignes.
- **Refactor** : le calcul du rapport est mutualisé (`computeDayJournal`) entre
  la réponse JSON et le CSV — une seule source de vérité, frontière de jour
  UTC identique à `/api/sell/me`.
- **Tests** : `TestSellDayReportEnriched` (ventilation, isolation revendeur,
  exclusion hors-jour, créance/versements dépôt-vente) et `TestSellDayReportCSV`
  (BOM, sections, retours cash, rechargements exclus, date passée, 400 invalide).

### Notes
- Aucun changement de contrat existant : nouveaux champs additifs + une route
  nouvelle ; les PWA déjà installées ignorent les clés inconnues.

---

## 2026-09-04 — Audit purge : tombstones anti-résurgence + réglage d'import auto + purge totale

### Corrigé
- **Résurgence des données purgées** — la purge admin supprimait du cloud
  (mémoire + Neon) mais PAS des routeurs réels ; le read_state agent (≤ 45 s)
  ré-importait ensuite tout ce que le routeur garde encore (bug signalé en
  production : les données purgées « revenaient toutes seules », et un voucher
  purgé revenait en utilisateur régulier fantôme, sans lot ni vente). Le
  correctif pose un **tombstone** (marqueur, TTL 30 jours) sur chaque username
  purgé : la synchro agent refuse de le ré-importer (ni utilisateur, ni
  session, ni journal, ni vente automatique) jusqu'à expiration ou levée
  explicite (création volontaire du même username). La file de commandes qui
  recréerait les entités purgées est annulée ; les commandes déjà envoyées
  restent exécutées par l'agent mais leur read_state suivant est filtré.
  (Table Neon `purge_tombstones` — zéro migration destructive.)

### Ajoutés
- **Réglage d'import automatique par compte** (`autoImportRouterUsers`, défaut
  ON — compatibilité) : à OFF, les comptes créés hors MikCloud (Winbox, autre
  système) ne sont plus importés automatiquement ; ils sont comptés dans le
  nouveau champ `unknownOnRouter` du routeur (badge discret côté front) pour
  adoption manuelle via l'outil d'import existant. (Colonne Neon
  `settings.auto_import_router_users` DEFAULT TRUE.)
- **Purge totale (opt-in)** : option `alsoRouter` de POST /api/admin/purge —
  enfile des commandes `user_remove` sur les routeurs AGENT pour supprimer
  réellement les comptes purgés, y compris du routeur. Double garde
  (case + saisie « SUPPRIMER » côté front, vérification serveur) : l'action
  déconnecte immédiatement les clients concernés. Routeurs REAL (passerelle)
  non commandables après purge (documenté au contrat).
- **Avertissement purge** quand des routeurs réels existent dans la portée :
  « les données ne disparaissent pas des routeurs ; leur ré-import est bloqué
  30 jours » + bilan enrichi (tombstones posés, suppression routeur).

### Sécurité
- Le read_state ne ré-importe plus un username purgé même si la commande
  `user_add` correspondante était déjà partie — aucun retour possible des
  données purgées tant que le tombstone vit.

---

## 2026-09-04 — Phase D perf/UX : UI optimiste + deep-links de détail

### Ajoutés
- **UI optimiste (D2)** — les actions à haute fréquence reflètent le résultat
  DÈS le clic (snapshot → rollback si l'API refuse ; invalidation onSuccess =
  vérité serveur), supprimant la perception d'attente due à l'aller-retour
  réseau (Render free + 3G/4G) :
  - Mode Vente : la vente (`POST /api/sell/{id}/sold`) retire le ticket du
    stock affiché et décrémente les compteurs immédiatement — en coordination
    avec UX R6 : les DEUX caches sont mis à jour (TanStack Query + snapshots
    localStorage), sinon le fallback hors-ligne ré-afficherait le ticket
    vendu ; le rollback complet (les deux caches) ne concerne que les erreurs
    MÉTIER (une erreur réseau part en file IndexedDB — pas de rollback).
    Retour de stock : mêmes principes, crédit prépayé patché avec la valeur
    réelle du serveur (jamais devinée) ; CA non deviné (sémantique différente
    prépayé/dépôt-vente).
  - Utilisateurs : toggle actif/inactif bascule instantanément dans TOUTES
    les pages en cache (`setQueriesData`, clés paginées/filtrées) ; la
    prolongation applique côté client la même règle que le backend
    (`max(maintenant, expiration) + days`, « expiré » → « actif »), corrigée
    par la réponse serveur ; réponse serveur écrite dans le cache avant
    l'invalidation.
  - Sessions : la session kickée quitte la liste immédiatement (KPI suivent
    automatiquement).
- **Deep-links de détail (D3)** — le mécanisme de navigation par URL (Phase A)
  s'étend aux éléments : `view-path.ts` gagne un segment de détail optionnel
  (`viewToPath(view, detail)` + `detailFromPath`, encodé
  `encodeURIComponent`) ; le segment est consommé LOCALEMENT par la vue — le
  store, la synchro bidirectionnelle et le fix Retour (192ad9f) restent
  intacts ; app-route re-normalise les segments orphelins (2ᵉ segment sur une
  vue non adressable) :
  - `/app/users/<id>` → dialog d'édition de l'utilisateur (push → le Retour
    referme le dialog ; fermeture manuelle → replace sans segment ; id absent
    de la page chargée → retour propre à la liste) ;
  - `/app/vouchers/<batchId>` → « détail lot » = onglet vouchers filtré sur
    le lot (`viewBatchVouchers` devient adressable) ; sortie (Retour) →
    filtre levé s'il n'a pas divergé ;
  - `/app/sessions/<username>` → filtre local par utilisateur (nouveau champ
    de recherche côté client — la liste n'est pas paginée) ; état DÉRIVÉ de
    l'URL (segment = filtre tant que l'opérateur n'a pas tapé), aucune
    synchronisation effet→état.
- Test de contrat view-path : 12 assertions (encodage, décodage, vues sans
  détail, 3ᵉ segment ignoré, segment vide).

### Arbitrage
- **D1 batch dashboard : retiré du plan** — `/api/dashboard` agrège déjà
  overview + sessions + ventes + revenus en une requête (poll 15 s) ; le gain
  résiduel est marginal.
- **D4 Render Starter : reporté** jusqu'aux premiers paiements clients (le
  keep-alive Neon Phase C couvre déjà le cold start base ; le cold boot du
  plan free reste la pénalité dominante hors agents en ligne).

## 2026-09-04 — Phase C perf : keep-alive Neon intelligent (fin du cold start en heures d'activité)

### Ajoutés
- **Keep-alive Neon « smart »** (`internal/store/pg.go` — `StartKeepAlive`) :
  une goroutine pings `SELECT 1` (via le pool existant) UNIQUEMENT quand la
  dernière écriture réelle date de plus de 4 min — le garde-fou `lastWrite`
  (atomic, mis à jour à chaque Load/Sync/ping réussi) fait sauter les pings
  superflus : quand au moins un agent est en ligne, son check-in (45 s)
  déclenche `Save()` → `Sync()` et Neon reçoit déjà du trafic continu. Le
  keep-alive ne parle donc à Neon QUE pendant les périodes où la base serait
  de toute façon endormée alors que des usagers peuvent arriver — supprimant
  le cold start (~0,5-1 s) payé par la première mutation après silence.
- **Fenêtrage configurable** — variable Render `NEON_KEEPALIVE`, défaut
  `business` (05:00–24:00 UTC ≈ Abidjan UTC+0 : la nuit, Neon retrouve son
  autosuspend et le plafond gratuit de 191,9 CU-h/mois reste largement couvert
  — ≈ 142 CU-h au pire) ; `on`/`24/7` maintien permanent (≈ 180 CU-h/mois,
  toujours sous le plafond) ; `off` désactive. Modes inconnus ignorés (log).
- **Robustesse** — ping borné 10 s (un compute en cours de réveil peut
  répondre lentement), retry au tick suivant, échecs logués au plus 1 fois/h ;
  arrêt propre de la goroutine sur `Close()` (SIGTERM) ; aucun changement de
  contrat API, aucun coût pour le mode développement JSON.

### Analyse (décision)
- Lecture du code : le dashboard lit depuis la MÉMOIRE (Neon n'est touché
  qu'aux `Save()` et aux insertions vitals par lots) — donc pendant une
  session usager très active en lecture, Neon peut s'endormir, et la mutation
  suivante (vente, création de voucher…) paye le réveil. Le keep-alive comble
  exactement ce trou aux heures d'activité, sans gaspillage quand la fleet
  tourne. La mesure B2 (`GET /api/vitals/summary`, plateforme) permettra de
  valider l'effet sur le p95 TTFB avant/après.

## 2026-09-04 — B2 perf : télémétrie Core Web Vitals (beacon public + synthèse plateforme)

### Ajoutés
- **Mesure de la latence réelle (RUM)** — le frontend remonte les Core Web
  Vitals (LCP, INP, CLS, FCP, TTFB) de TOUS les usagers — visiteurs anonymes
  de la vitrine inclus — vers `POST /api/vitals` : `navigator.sendBeacon`
  (Blob `text/plain`, requête simple sans preflight CORS, réponse 204 jamais
  lue), `web-vitals` v6 importé dynamiquement à l'idle (hors du chemin
  critique qu'il mesure), `sid` éphémère en sessionStorage (sans cookie ni
  PII). Monté dans le layout racine → mesure `/`, `/login`, `/app`, `/sell`.
- **Synthèse plateforme** — `GET /api/vitals/summary?window=h` (1-168, défaut
  24) : p50/p75/p95 (nearest-rank), répartition good/needs-improvement/poor,
  top 8 chemins, mobile/desktop — avec déduplication « dernier rapport par
  (session, page, métrique) » (recommandation Google pour les p75).

### Architecture
- **`internal/telemetry`** — isolé du store métier (aucun impact sur
  `model.DB` ni la synchro différentielle) : ring mémoire 20 000 échantillons
  + table Neon `web_vitals` (DDL idempotente en tâche de fond 45 s après boot
  — le démarrage n'attend JAMAIS Neon, cold start préservé) ; historique 48 h
  rechargé au boot (les agrégats survivent aux redéploiements Render) ;
  insertions par lots asynchrones (10 min ou 200 échantillons, file bornée
  2 000, pertes loguées) — un incident Neon n'impacte jamais une requête API ;
  flush final sur SIGTERM. `POST /api/vitals` whitelisté (vitrine anonyme),
  couvert par le limiteur général ; IP et User-Agent bruts jamais stockés.

## 2026-09-04 — UX NAV : correctif — le bouton Retour rejoue la navigation console

### Corrigés
- **Retour navigateur qui quittait l'application** — la synchronisation
  store → URL reposait sur un effet React dépendant de `[view, pathname]` :
  pendant un popstate (Retour/Avancer), il tournait avec la vue PÉRIMÉE du
  commit et repoussait `router.push()` vers l'URL qu'on venait de quitter —
  annulant la navigation du navigateur, créant un ping-pong d'URLs (2
  entrées parasites par cycle) et consommant l'historique jusqu'à sortir de
  l'application (fermeture de la PWA standalone). Remplacé par un
  **abonnement zustand synchrone** (`store.subscribe`) conscient de
  l'origine du changement de vue : navigations interface (sidebar, palette,
  impersonation, bascule de console) → `push` d'une entrée ; changements
  venus de l'URL (popstate, lien direct) → aucun push ; sans session
  (logout) → aucun push. Normalisation `/app` nu / slug inconnu en
  `replace` (aucune entrée parasite). Le store, les vues, le contrat API et
  le backend restent inchangés.

## 2026-09-04 — UX R6 : Mode Vente hors-ligne — file locale + replay auto (P3-a)

### Ajoutés
- **Ventes hors-ligne** — le comptoir continue de vendre sans réseau : une
  vente dont le POST échoue (réseau coupé, backend injoignable 502/503/504,
  ou requête stallée > 10 s) part dans une file locale IndexedDB
  (`mikcloud-sell/pending-sales`, zéro dépendance, dédoublonnée par voucher).
  Au retour du réseau (event `online`, au montage, puis toutes les 60 s) la
  file est REJOUÉE FIFO : 200 → vente tracée + toast ; 409/404 → entrée
  retirée sans décompte fantôme (le backend idempotent garantit qu'un replay
  ne double jamais un comptage). Bannière ambre « Ventes en attente de
  synchronisation (n) » + chip « En attente de sync » sur les cartes.
- **Snapshot stock/profil en localStorage** — hors-ligne, la vue affiche le
  dernier état connu (`mikcloud-stock-cache`/`mikcloud-me-cache` écrits à
  chaque fetch réussi) au lieu d'un écran vide : le vendeur voit son stock et
  vend, même sans couverture.
- **Timeout client 10 s** (`api()` gagne `timeoutMs`, `AbortSignal.timeout`)
  — un réseau mobile peut stall une requête indéfiniment (socket demi-ouvert
  à travers un proxy) : ni succès ni échec. Découvert en E2E : une requête
  « offline » a stallé ~40 s puis abouti — sans timeout, la vente serait
  restée bloquée en UI sans jamais rejoindre la file. Le pire cas ambigu
  (serveur a vendu, client sans réponse) est couvert : le replay reçoit 409
  « déjà remis » et se résout sans double décompte.

### Modifiés
- Frontend uniquement (`api.ts`, `sell-shell.tsx`, nouveau
  `offline-queue.ts`, i18n +6 clés ×FR/EN). Zéro changement backend, zéro
  changement de contrat. Les retours de stock restent en ligne (opération
  rare, l'invariant anti-fantôme est serveur).

## 2026-09-04 — UX R5 : Mode Vente — retrait de la saisie papier (tactile + auto uniquement)

### Supprimés
- **Saisie du code papier (R3) retirée du Mode Vente** — avec la vente auto à la
  connexion (R4), le 3ᵉ mode de vente était devenu redondant et fragilisait
  l'UX : un code tapé à la main risquait l'erreur d'homoglyphe (O/0, I/l/1) —
  donc de vendre le MAUVAIS ticket — pour un résultat identique au tactile
  (même `POST /sold`, même confirmation R2). La recherche du code dans la
  liste + tap « Vendu » couvre déjà « je comptabilise maintenant » ; la
  connexion du client couvre le papier sans aucun geste. Modèle mental final :
  **« le ticket part : je le tappe, ou le client le connecte — jamais de
  double comptage. »** Le bloc formulaire est remplacé par une bannière
  explicative « Vente automatique à la connexion » (le vendeur comprend
  pourquoi son stock baisse « tout seul »).

### Modifiés
- Frontend uniquement : `sell-shell.tsx` (−~60 lignes : état `physicalCode`,
  `sellPhysical()`, bloc saisie, plumbing `pendingVia`) et `i18n.ts`
  (−4 clés ×FR/EN, +2 clés bannière). Zéro changement backend, zéro changement
  de contrat — la branche `via=paper` reste supportée (additive, les lignes
  historiques `sell_mode_paper` gardent leur sens) mais n'est plus émise.

## 2026-09-04 — UX R4 : Mode Vente — vente automatique à la connexion + vente papier tracée

### Ajoutés
- **Vente automatique à la connexion** — le revendeur remet le ticket (papier
  imprimé ou code dicté) sans toucher l'app : à la PREMIÈRE connexion du client
  au hotspot, la vente se confirme toute seule — décompte du stock, trace
  `SoldVia="auto_connect"` (distincte d'une vente tactile), créance dépôt-vente
  au prix gros (N°19), rapport de journée. Idempotent par `SoldAt` : jamais de
  double comptage, jamais de décompte fantôme.
- **Vente papier tracée à part** — la vente par saisie du code imprimé (R3)
  envoie `via=paper` → `SoldVia="sell_mode_paper"` : le gérant distingue
  désormais vente tactile, vente papier et vente auto dans l'audit.

### Modifiés
- Durcissement : `POST /api/sell/{id}/sold` refuse explicitement (409) un
  voucher expiré ou consommé — entre l'affichage du stock et la confirmation,
  la validité peut tomber ; zéro décompte fantôme sur un ticket mort.

### Contrat
- Additif : corps optionnel `{"via":"paper"}` sur `/api/sell/{id}/sold`
  (rétrocompatible — les PWA installées POSTent sans corps) ; nouvelle valeur
  `SoldVia="auto_connect"`. Documenté dans CONTRACT-V2 (section UX R3/R4).

## 2026-09-04 — UX R3 : Mode Vente — tickets papier connectés, recherche, lot entier, expiration

### Ajoutés
- **Ticket papier « connecté »** — le revendeur qui a imprimé des tickets de
  son stock peut les vendre hors écran : il saisit le code imprimé dans le
  bloc « Ticket papier déjà imprimé ? », le voucher est retrouvé dans SON
  stock actif et suit exactement le chemin d'une vente tactile (confirmation
  R2 obligatoire, trace `SoldAt`/`SoldVia`, créance dépôt-vente, rapport de
  journée). Correspondance insensible à la casse ; code inexistant, déjà
  vendu, retourné ou expiré → refus explicite, jamais de décompte fantôme.
- **Recherche de stock** — champ de recherche local (aucune requête
  supplémentaire) par code, profil ou référence de lot ; les groupes
  correspondants se déplient automatiquement, état « aucun résultat »
  explicite, effacement en un geste. Disponible en vente ET en retour.
- **Retour d'un lot entier** — en mode retour, chaque en-tête de lot porte un
  bouton « Tout » (sélection/désélection du lot complet) : les tickets d'un
  lot expirent ensemble, les rendre un par un n'avait pas de sens au comptoir.
- **Badge « Expire bientôt » (< 48 h)** — sur la carte du ticket concerné et
  en en-tête du lot dès qu'un de ses tickets entre dans la fenêtre des 48 h :
  à vendre en priorité, ou à rendre avant qu'il ne meure (un voucher expiré
  sort du stock sans recyclage possible).

### Aucun impact
- Contrat inchangé (aucune route nouvelle, aucun champ nouveau) : la vente
  papier réutilise `POST /api/sell/{id}/sold`, le retour reste
  `POST /api/sell/return`. i18n FR/EN (+11 clés `sell.*`).

## 2026-09-04 — UX R2 : Mode Vente — confirmation avant remise au client

### Ajoutés
- **Confirmation de vente (anti-misclick)** — le bouton « Vendu » n'exécute
  plus immédiatement la remise : une dialog récapitule le ticket exact
  (profil, prix, code) et exige « Confirmer la vente ». Justification :
  la vente est définitive par design (trace anti-vol `SoldAt` immuable) et
  naît de créance immédiate en dépôt-vente — un faux clic en tournée
  (écran tactile, lumière, mouvement) coûtait une correction manuelle du
  gérant. « Annuler » (ou Échap / clic hors dialog) ne vend rien ; en cas
  d'erreur réseau la dialog reste ouverte pour relancer sans re-sélection.

### Aucun impact
- Contrat inchangé : un seul appel `POST /api/sell/{id}/sold`, même
  sémantique, aucune route nouvelle. Rapport de journée, retour de stock et
  audit inchangés. i18n FR/EN (+3 clés `sell.sellConfirm*`).
## 2026-09-04 — UX NAV / Perf : navigation par URL « Speed App » (retour navigateur, liens directs, preconnect)

### Ajoutés
- **Navigation par URL (Phase A)** — la console quitte la route unique `/` :
  `/login` (connexion), `/app/[[...vue]]` (console, une URL par vue :
  `/app/users`, `/app/vouchers`, `/app/platform-logs`…), `/sell` (Mode
  Vente). Le **bouton Retour/Avancer du navigateur** rejoue désormais la
  navigation de la console, les **liens directs sont partageables et
  bookmarkables**, et **recharger la page restaure la vue courante** (la
  vue n'étant pas persistée, c'est l'URL qui la redéfinit au chargement).
- **Synchronisation bidirectionnelle URL ↔ store** (`view-path.ts` +
  `app-route.tsx`) : la vue reste pilotée par le store (source de vérité
  unique) ; chaque changement de vue crée une entrée d'historique, les
  navigations Retour/Avancer mettent à jour le store. Les deux sens sont
  idempotents (aucune boucle, aucune entrée parasite) ; le premier
  alignement `/app → /app/<vue>` passe par `replace` (historique propre).
- **Gardes d'accès par route** (`app-route` / `sell-route` / `login-route`) :
  session requise sur `/app` et `/sell`, revendeur redirigé vers `/sell`
  (et seul autorisé dessus), session active sur `/login` renvoyée vers son
  espace. Les fenêtres pré-montage rendent un fallback plein écran
  (`ShellFallback`) — aucune erreur d'hydratation, aucun flash.
- **Preconnect API (B1)** — `layout.tsx` émet `preconnect` + `dns-prefetch`
  vers `NEXT_PUBLIC_API_BASE` (rendu uniquement si défini) : le handshake
  TCP+TLS vers Render démarre pendant la saisie du login (~100-200 ms
  économisées sur le premier appel).
- **Préchauffe des vues chaudes (B4)** — après montage de la console, les
  chunks des vues les plus consultées (sessions, utilisateurs, vouchers)
  sont importés à l'idle (`requestIdleCallback`, repli timeout) : premier
  clic instantané, même en réseau lent.
- **QueryClient partagé entre routes** — `QueryProvider` remonte au layout
  racine : le cache TanStack Query survit aux navigations `/` ↔ `/login` ↔
  `/app` au lieu d'être recréé à chaque changement de route.

### Conservé / inchangé
- La vitrine reste servie sur `/` (CTA « Se connecter » → navigation client
  vers `/login`, instantanée) ; PWA standalone : ouverture sur la connexion
  ou la console si session active (comportement inchangé, redirections par
  route). L'impersonation plateforme, les gardes de rôle par vue
  (`canView`, correctif serveur 403) et le store Zustand restent la source
  de vérité — aucun changement de contrat d'API ni de schéma Neon.
- Perf listes (B3) : les requêtes paginées gardaient déjà
  `placeholderData: (previous) => previous` — aucun ajout nécessaire.

### Aucun impact
- Backend : aucun fichier modifié. Contrats inchangés. Synchro Neon
  inchangée. (Frontend uniquement — déploiement Vercel.)

## 2026-09-04 — UX R1 : Mode Vente — stock revendeur groupé par profil et par lot

### Ajoutés
- **PWA revendeur : regroupement du stock « profil → lot » (R1)** — la liste
  plate mélangée laisse place à une lecture de comptoir : un groupe
  **accordéon par profil** (nom, nombre de tickets, valeur faciale cumulée)
  contenant, quand le profil porte plusieurs lots, des **sous-groupes par
  lot** (« Lot 6147 · 04 sept. · 8 restant(s) »). Le profil le plus stocké
  arrive en tête (logique de best-seller), les lots du plus récent au plus
  ancien, les tickets restent triés récent-d'abord dans chaque lot.
- **Bascule de vue « Par profil / Récents »** — la nouvelle vue groupée est
  le défaut ; la liste plate historique (récents d'abord) reste accessible
  en un tap. La préférence est mémorisée en localStorage via
  `useSyncExternalStore` (SSR sûr, synchro inter-onglets, zéro setState en
  effet).
- **API : `GET /api/sell/stock` expose désormais `batchId`** (R1a —
  `HotspotUser.BatchID`, tracé à la génération). Champ additif : les PWA
  déjà déployées ne voient aucun changement breaking ; les données
  historiques sans lot (batchId vide) restent affichées sous leur profil,
  sans sous-groupe.
- i18n : 8 clés `sell.*` (FR + EN) pour la barre de vue, les groupes et
  les sous-groupes de lot.

### Aucun impact
- Contrats inchangés par ailleurs : vente (`/api/sell/{id}/sold`), retour de
  stock (`/api/sell/return` — la multi-sélection fonctionne au sein des
  groupes), rapport de journée, profil de tournée. Le mode retour affiche
  les mêmes groupes (sélection case par case ; la sélection par lot entier
  est planifiée en R2).
- Aucun changement de schéma Neon (lecture d'un champ existant) ; synchro
  différentielle et sauvegardes inchangées.

## 2026-09-03 — UX K3 : purge des données fusionnée (globale + ciblée)

### Fusionnés
- **Paramètres plateforme → Maintenance** : les deux cartes jumelles
  « Purge globale des données » et « Purge ciblée par compte » deviennent
  **UNE seule carte « Purge des données »** avec un sélecteur de **portée**
  (« Tous les comptes (purge globale) » ou un compte client précis) :
  - grille de catégories **unifiée à 10 entrées** (identique dans les deux
    portées — le scope « vouchers » est désormais disponible en global
    aussi) avec compteurs live de la portée courante ;
  - confirmation adaptée à la portée : saisie « PURGER » en global, nom
    exact du compte en ciblé ; bilan détaillé en toast (tickets comptés
    à part) ;
  - état vide vert quand la portée courante est déjà propre.
- **Backend : un seul moteur et un seul endpoint d'exécution** —
  `purgeScopes(accID, scopes)` remplace les deux moteurs dupliqués ;
  `POST /api/admin/purge` accepte un `accountId` OPTIONNEL (vide → global,
  renseigné → ciblé) et **`POST /api/admin/purge/account` est supprimé**
  (fusion). `GET /api/admin/purge/stats` gagne le compteur `vouchers` ;
  harmonisation des cascades (les lots/ventes attachés aux routeurs
  simulés partent aussi en global, comme en ciblé et comme purge-demo) ;
  sélection explicite exigée (corps sans `scopes` → 400, les deux portées).
- Tests backend réécrits autour de l'endpoint fusionné (+ nouveaux tests
  portée globale : `vouchers` seul et `all`) ; `docs/CONTRACT-V2.md` mis à
  jour.

### Aucun impact
- Garanties intactes : routeurs réels (agent), comptes, équipe, réglages,
  abonnement et facturation ne sont JAMAIS purgés ; rien n'est régénéré ;
  synchro différentielle Neon inchangée.

## 2026-09-03 — UX K2 : fusion des paramètres dupliqués (client ↔ plateforme)

### Modifiés
- **Fusion anti-redondance (K2)** — plusieurs réglages existaient en double
  entre la vue client « Paramètres » et la console « Paramètres plateforme » ;
  chaque préoccupation a désormais UN seul foyer :
  - **Mot de passe + 2FA** : cartes extraites dans un module partagé
    (`parts/security-cards.tsx`), utilisé par les deux vues. La console
    plateforme récupère la version riche (bascule de visibilité) et surtout la
    **2FA, jusque-là inaccessible à l'admin en mode plateforme** (le guard de
    vue renvoie la page client vers la console plateforme).
  - **Maintenance plateforme** (rechargement base, nettoyage démo, purges) :
    déplacée de l'onglet « Avancé » de la vue client (où ces outils GLOBAUX
    apparaissaient pendant les sessions support, en plein console client) vers
    un onglet « Maintenance » dédié de la console plateforme.
  - **Zone sensible fusionnée dans la purge par catégories** : l'ancienne
    carte « Purger tout / Vider le journal » (simple `window.confirm`) était un
    sous-ensemble moins sûr de la purge par catégories — vider le journal se
    fait désormais en cochant « Journaux », avec confirmation par saisie
    « PURGER » et bilan détaillé, partout.
  - **Langue** : carte retirée de la console plateforme (elle existait déjà
    dans le menu utilisateur, présent sur chaque écran, et dans l'onglet
    Général du client).
- Console « Paramètres plateforme » réorganisée en 3 onglets : **Général**
  (identité + inscriptions), **Sécurité** (mot de passe + 2FA),
  **Maintenance** (base, démo, purges globale + ciblée).
- Vue client « Paramètres » : onglet « Avancé » renommé **« Sécurité »**
  (mot de passe + 2FA uniquement) ; i18n nettoyé (clés mortes retirées,
  clés maintenance renommées `platformSettings.*`).

### Aucun impact
- Contrat API inchangé (aucun endpoint ni schéma modifié — refonte UI/i18n
  uniquement) ; Neon et Render sans changement.

## 2026-09-03 — Sécurité vague S6 : détection d'identité routeur dupliquée

### Ajoutés
- **Détection d'identité routeur dupliquée (S6)** — ferme la boucle du fermage
  d'essai côté protocole agent : un client sous paywall (guard P3) créait un
  compte neuf et y re-provisionnait le MÊME routeur physique (nouveau script
  écrasant l'ancien scheduler). L'empreinte RouterOS (System Identity +
  board-name) déclarée au `POST /agent/register` est comparée aux routeurs
  ACTIFS (`LastSeen` < 24 h) des autres comptes.
  - Conflit → `409` code `router_identity_conflict` au register + flag
    persistant (`routers.identity_conflict`, migration idempotente Neon).
  - `GET /agent/cmd` : aucune commande livrée à un routeur flaggé tant que
    le porteur reste actif ; levée AUTOMATIQUE (tracée en activité) dès que
    le porteur disparaît ou dort > 24 h.
  - Exclusions : identity générique (« mikrotik » / vide) et même compte
    (re-register, rotate-token). Au passage : l'identity courante écrase
    désormais `Host` à chaque register (un renommage RouterOS était figé au
    premier) et `BoardName` est rempli au register.
  - Procédure support : débloquer un faux positif en supprimant le routeur
    fantôme de l'ancien compte (impersonation) — le check-in reprend en 45 s.
  - Contrat : section « Sécurité S6 » dans docs/CONTRACT-V2.md.

## 2026-09-03 — Sécurité vague S5 : dédoublonnage email/WhatsApp à l'inscription

### Ajoutés
- **Dédoublonnage email/WhatsApp (S5)** — un même email ou un même numéro
  WhatsApp ne peut plus créer qu'UN SEUL compte, aux deux points de création :
  auto-inscription publique (`POST /api/auth/register`) et création depuis la
  console plateforme (`POST /api/admin/accounts`). Objectif : couper le
  fermage « manuel » d'essais de 90 jours — un client tombé sous le paywall
  (guard P3) relançait un essai complet en changeant juste nom et username.
  - `409` avec message distinct (« Un compte existe déjà avec cet email » /
    « … avec ce numéro WhatsApp »), affiché tel quel par le formulaire
    d'inscription (toast) — aucun changement frontend nécessaire.
  - Comparaisons : email trim + insensible à la casse ; WhatsApp en chiffres
    normalisés (E.164 sans « + », 8–15). Comptes désactivés inclus (un client
    banni ne revient pas avec ses coordonnées) ; la suppression d'un compte
    (zone sensible) libère ses coordonnées.
  - Limite assumée (documentée) : formes de numéro différentes
    (« 0701020304 » vs « 2250701020304 ») restent distinctes — le quota
    d'inscription par IP (S3) borne les sondages.
  - Tests : `TestSignupQuotaE2E` adapté (coordonnées uniques par inscription).
  - Contrat : section « Sécurité S5 » dans docs/CONTRACT-V2.md.

## 2026-09-03 — Purge ciblée par compte (zone sensible, console plateforme)

### Ajoutés
- **Purge ciblée par compte (zone sensible)** — complément chirurgical de la
  purge globale : l'admin plateforme choisit UN compte client et supprime des
  catégories d'éléments pour CE compte uniquement (les autres comptes ne sont
  jamais touchés) :
  - Backend : `GET /api/admin/purge/accounts` (compteurs live par élément :
    routeurs simulés, utilisateurs hotspot, vouchers, profils, lots,
    revendeurs, transactions, ventes, sessions, journaux, gabarits) et
    `POST /api/admin/purge/account` (`{accountId, scopes}` — mêmes garanties
    que la purge globale : jamais les routeurs réels, comptes, équipe,
    réglages, abonnement ni facturation ; ne régénère rien ; cascades
    restreintes au compte : routeurs simulés → entités attachées, lots →
    vouchers restants, revendeurs → transactions, utilisateurs/tickets →
    sessions liées closes). Nouveau scope `vouchers` (tickets sans les lots,
    propre à la purge ciblée). Réponse enrichie du champ `vouchers`
    (additif, rétrocompatible). Ligne d'activité tracée sur le compte ciblé
    (« par la plateforme ») après la purge.
  - Frontend : carte « Purge ciblée par compte » dans Paramètres plateforme
    (zone sensible) — sélecteur de compte, cases à cocher par élément avec
    compteurs live (badge), indication des transactions liées aux revendeurs,
    tout cocher/décocher, confirmation par saisie du nom exact du compte
    (irréversibilité), bilan détaillé en toast (résumé serveur).
  - Tests : isolation stricte entre comptes (compte témoin), cascades (all +
    routeur simulé), compteurs avant/après, traçabilité, gardes 400/404 —
    `handlers_purge_account_test.go` (3 tests E2E HTTP).
  - Contrat : clause « Purge ciblée par compte » dans docs/CONTRACT-V2.md.

## 2026-09-03 — Sécurité vague S4 : 2FA TOTP, sauvegardes testées, conformité

### Ajoutés
- **Authentification à deux facteurs TOTP (S4)** — implémentation RFC 6238
  maison (HMAC-SHA1, 30 s, 6 chiffres, fenêtre ±1, comparaison
  constant-time, secret 160 bits base32 — aucune dépendance nouvelle) :
  `POST /api/auth/2fa/setup` (génère le secret + URL otpauth), `/activate`
  (vérifie un premier code puis active), `/disable` (exige le mot de passe
  courant). Au login, un utilisateur avec 2FA active reçoit `401` + code
  machine `totp_required` (l'écran de connexion affiche alors le champ
  code) ; code erroné → `400` générique + journal de raison fine
  `bad_totp` (S2). Migration idempotente : colonnes `admin_users.totp_secret`
  (jamais sérialisé en JSON) et `totp_enabled`. Le statut `totpEnabled` est
  exposé dans login et `/api/auth/me`. Frontend : second étape de connexion
  (champ one-time-code) et carte 2FA dans Paramètres → Avancé (pairage par
  secret copiable, activation, désactivation). Tests : vecteurs officiels
  RFC 6238 (validés par référence indépendante), fenêtre ±1, cycle E2E
  complet. Secours : procédure opérateur (RUNBOOK-SECRETS.md §4).
- **Sauvegardes chiffrées testées (S4)** — nouvel outil
  `backend/cmd/mikbackup` : export de toutes les tables (row_to_json —
  typage Postgres préservé) chiffré AES-256-GCM, et sous-commande
  `restore-check` qui recrée chaque table en miroir `s4check_*`, y réinsère
  l'intégralité des lignes via `json_populate_record` (re-typage natif),
  compare les comptages et nettoie. **Chaque sauvegarde est
  automatiquement vérifiée** : le workflow `backup.yml` (dimanche 03:17
  UTC) exporte PUIS restaure en vérification avant de publier l'artefact
  chiffré (rétention 90 jours). Test réel exécuté en production Neon :
  22 tables / 845 lignes exportées puis 845/845 restaurées. Prérequis
  opérateur : secrets GitHub `BACKUP_KEY` + `DATABASE_URL`
  (RUNBOOK-SECRETS.md §3 — le workflow se met en skip propre sans eux).
- **Runbook secrets & procédures (S4)** — `docs/RUNBOOK-SECRETS.md` :
  inventaire des secrets, emplacements légitimes, périodicités de rotation,
  procédures pas-à-pas (GitHub PAT, Neon, Render, Vercel, JWT_SECRET),
  protocole « secret exposé », procédure de secours 2FA, plan Cloudflare
  (proxy, WAF, rate limit en amont, origin hardening).
- **Conformité données personnelles (S4)** —
  `docs/REGISTRE-TRAITEMENT.md` : registre des traitements (loi ivoirienne
  n° 2013-450 / référence RGPD), sous-traitants (Neon, Render, Vercel,
  passerelles), droits des personnes avec procédures concrètes (accès,
  rectification, suppression/anonymisation, opposition), sécurité
  (renvoi S1–S4), protocole de violation de données (72 h).

### Documentation
- `docs/CONTRACT-V2.md` : clause « Sécurité S4 » (endpoints 2FA, sémantique
  login, sauvegardes).

## 2026-09-03 — Sécurité vague S3 : chaîne d'approvisionnement et anti-abus d'inscription

### Ajoutés
- **Quota d'inscription par IP (S3)** — `POST /api/auth/register` : au-delà
  de 5 tentatives en 10 minutes (anti-burst) ou 20 tentatives en 24 heures
  (anti-farm de comptes d'essai), toute tentative — même invalide — reçoit
  `429` + `Retry-After` (cf. `internal/api/signup_abuse.go`, mêmes
  primitives que le verrou PIN S2 : état mémoire, purge paresseuse,
  garde-fou 10 000 IP). L'imputation par IP reste une première ligne : le
  fermage organisé via XFF forgés reste rattrapé par le plafond global
  d'instance S1 (900 req/min) et, en bêta privée, par `REGISTER_KEY`.
  Derrière un NAT partagé (cybercafé), 20 inscriptions/24 h laissent une
  large marge aux usages légitimes.
- **Scan de vulnérabilités dans la CI (S3)** — nouveau job `govulncheck`
  (vulnérabilités Go *atteignables*, base officielle vuln.go.dev) : rapport
  non bloquant — le job passe au rouge pour visibilité immédiate mais le
  déploiement Render n'attend pas la fraîcheur d'une base de CVE externe.
- **Dependabot (S3)** — montées de version hebdomadaires groupées
  minor/patch sur les trois écosystèmes : `gomod` (backend), `npm`
  (frontend), `github-actions` (sécurité de la chaîne CI elle-même).
- **Secret scanning + secret push protection (S3)** — activés au niveau du
  dépôt GitHub : toute fuite de credential poussé sur main est détectée,
  et un push contenant un secret reconnu est bloqué à la source.

### Documentation
- `docs/CONTRACT-V2.md` : clause « Sécurité S3 » (quota d'inscription,
  hygiène de la chaîne d'approvisionnement).

## 2026-09-03 — Sécurité vague S2 : durcissement P1 anti brute-force

### Ajoutés
- **Verrouillage PIN revendeur par compte (S2-B2)** — `POST /api/reseller/login` :
  après 5 échecs consécutifs de PIN sur le MÊME revendeur, toute nouvelle
  tentative est refusée `429` + `Retry-After` pendant 15 minutes — même avec
  le bon PIN, même depuis une IP neuve (clé = ID interne du revendeur,
  insensible à l'usurpation de X-Forwarded-For ; un succès efface
  l'historique). Comble le trou du limiteur par IP face aux attaques
  distribuées contre un compte ciblé (espace PIN 4-6 chiffres). État en
  mémoire (instance unique) ; sous verrou, le hachage bcrypt n'est même pas
  exécuté. Contrepartie documentée : un attaquant peut verrouiller le PIN
  d'un revendeur légitime 15 min (réinitialisable par le gérant).
- **Journal des échecs d'authentification (S2)** — chaque échec de connexion
  console ou PIN émet une ligne JSON `{"event":"auth_failure",…}` sur la
  sortie standard (horodatage RFC3339, IP au premier hop XFF, kind
  `console`/`reseller_pin`, identifiant soumis, raison fine : `unknown_user`,
  `bad_password`, `disabled`, `unknown_reseller`, `bad_pin`, `locked`,
  `reseller_disabled`, `account_disabled`) — agrégeable depuis les logs
  Render. Les réponses HTTP restent strictement génériques (aucun oracle
  d'énumération) ; la comparaison bcrypt factice sur identifiant inconnu
  supprime l'oracle de timing.
- **Politique centralisée des mots de passe (S2-B4)** — remplaçant les
  vérifications « 8 caractères » dupliquées, appliquée aux 6 points de
  définition (inscription, changement personnel, création et
  réinitialisation d'un membre d'équipe, création d'un compte client par la
  plateforme, création d'un admin plateforme) : 10 caractères minimum
  (runes), 72 octets maximum (limite bcrypt), interdiction des mots de
  passe les plus courants (denylist : fuites publiques, clavier FR, termes
  métier MikCloud/MikroTik) et du nom d'utilisateur. Les mots de passe
  existants plus courts restent valides à la connexion (aucune rupture).

### Modifiés
- Frontend : hints et validations de longueur alignés 8 → 10 caractères
  (inscription, équipe, comptes plateforme, paramètres — fr et en).

### Tests
- 6 nouveaux tests dans le paquet `api` : politique de mots de passe
  (table-driven, casse/denylist/accents/72 octets), verrou PIN unitaire
  (horloge injectable : seuil, expiration, reset au succès, étanchéité
  inter-revendeurs), verrou PIN E2E sur la surface HTTP (5 échecs → `429` +
  `Retry-After` même au bon PIN, revendeur voisin épargné, message
  anti-énumération inchangé), journal JSON (IP premier hop XFF, repli
  RemoteAddr).

## 2026-09-02 — Sécurité vague S1 : durcissement P0 pré-lancement commercial

### Ajoutés
- **Révocation immédiate des sessions (S1-A3)** — nouvelle colonne
  `admin_users.session_epoch` (migration `ensureSchema`, défaut 0) et claim
  JWT `ver` : le middleware refuse tout token dont l'époque ne correspond
  plus (`401 « Session révoquée »`) ou dont le porteur a été supprimé
  (`401 « Compte utilisateur supprimé »`). L'époque est incrémentée à chaque
  opération sensible : changement de mot de passe (`POST /api/auth/password` —
  toutes les sessions, y compris la courante, sont coupées), réinitialisation
  par l'owner et changement de rôle (`PUT /api/team/{id}`) ; la suppression
  d'un membre (`DELETE /api/team/{id}`) est couverte par le contrôle
  d'existence. Migration douce : les tokens sans `ver` se décodent `ver=0`.
- **En-têtes de sécurité HTTP (S1-A4)** — middleware `securityHeaders` :
  `X-Content-Type-Options: nosniff`, `Strict-Transport-Security`,
  `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`,
  `Cache-Control: no-store` sur toutes les réponses.
- **Limite de taille des corps de requête (S1-A1)** — middleware `limitBody` :
  `http.MaxBytesReader` 2 Mio + `413` immédiat si Content-Length dépasse
  (avant : `decodeBody` lisait des corps sans aucune limite).
- **Limiteur de débit global (S1-A2)** — toute route `/api/*` hors
  authentification est plafonnée à 120 requêtes/minute par IP (`429` +
  `Retry-After`) ; les scopes durs existants (auth 12/min, revendeur 5/min)
  restent prioritaires ; `/agent/*` (poll 45 s) reste hors périmètre.
- **Suivi S1 (sondes de production)** — deux découvertes traitées :
  1. l'IP client doit être extraite du **premier** hop de `X-Forwarded-For`
     (convention Render : l'IP réelle est posée en tête, les hops internes
     s'ajoutent à la suite) — l'ancienne règle « dernier hop » visait un hop
     interne qui tourne, ce qui fragmentait silencieusement les buckets du
     limiteur (bug latent pré-existant, révélé par les seuils S1) ;
  2. Render **transmet** le `X-Forwarded-For` du client : le premier hop
     reste forgeable par un attaquant délibéré → plafond **global par
     instance** de 900 requêtes/minute sur `/api/*`, insensible à toute
     usurpation d'en-tête (la rotation d'IP ne le contourne pas).
- Tests : révocation de session (blocage immédiat + réémission + suppression
  du porteur), révocation par changement de mot de passe de bout en bout,
  en-têtes de sécurité, 413 sur corps surdimensionné, limiteur global (120/min
  + indépendance des scopes), aller-retour du claim `ver` + compatibilité
  legacy. `go vet`, `gofmt` et `go test -race` verts sur les 9 paquets.

### Documentés
- `docs/CONTRACT-V2.md` — section « Sécurité S1 » (claims JWT étendus,
  messages 401 de révocation, limites de débit et de corps, en-têtes).

## 2026-09-02 — Refactor vague V1 : geniuspay_stripe et admin_account

### Modifiés
- **`internal/api/handlers_geniuspay_stripe.go` (842 lignes) supprimé et
  scindé par couche** : intégration GeniusPay « API Abonnements Stripe »
  (types réels, appels client, helpers statut/cycle, application des
  échéances, dispatch webhook subscription.*) → `geniuspay_stripe.go`
  (416 l.) ; routes console POST/GET/cancel de l'abonnement récurrent →
  `handlers_subscription_stripe.go` (440 l.).
- **`internal/api/handlers_admin_account.go` (842 → 669 lignes) allégé** :
  la garde d'écriture P3 (`subscriptionGuardView`, états, `guardAccountWrite`,
  `guardAccountRouterLimit` — consommée par 12+ fichiers) rejoint
  `guards.go` (85 l.) ; `writeErrCode` (helper générique) rejoint
  `helpers.go` ; le moteur d'activation `applySubscriptionLocked` (source
  unique partagée plateforme/webhooks/carte) rejoint `handlers_subscription.go`.
- Commentaires de cartographie mis à jour (handlers_geniuspay.go).
- Garanties vérifiées (même méthode que les vagues précédentes) : multiset
  des 413 déclarations top-level strictement identique avant/après, table de
  routage (119 registrations mux) octet pour octet, couverture ligne à ligne
  du code déplacé. gofmt, go vet, `go test -race -count=1` (9 paquets) et
  `go build` tous verts — mouvement pur de code, zéro changement de logique
  ni de contrat.

## 2026-09-02 — Refactor vague P0 : dissolution de handlers_ext.go

### Modifiés
- **`internal/api/handlers_ext.go` (897 lignes) supprimé et redistribué**
  par domaine : F2 templates de vouchers → `handlers_templates.go` (187 l.),
  F3 journal utilisateurs → `handlers_userlogs.go` (90 l.), F4/F5 actions
  utilisateurs (reset stats, extend, export CSV, bulk, cleanup) →
  `handlers_users_ops.go` (504 l.) ; le moteur d'enforcement de
  l'expiration F1 (`enforceExpired`, partagé par 4 fichiers) et les filtres
  sessions live (`onlineSessions`, `onlineKey` — vouchers + rapports)
  rejoignent `helpers.go` ; `filterUsers` (domaine users) rejoint
  `handlers_users.go`.
- **Plus aucun fichier du paquet `api` ne dépasse 900 lignes** (max :
  handlers_geniuspay_stripe.go, 842 l.) ; commentaire de cartographie P0
  mis à jour dans routes.go.
- Garanties vérifiées (même méthode que les Phases B) : multiset des 413
  déclarations top-level strictement identique avant/après, table de routage
  (119 registrations mux) octet pour octet, couverture ligne à ligne du code
  déplacé. gofmt, go vet, `go test -race -count=1` (9 paquets) et `go build`
  tous verts — mouvement pur de code, zéro changement de logique ni de
  contrat.

## 2026-09-02 — Refactor Phase B (suite) : agent_handlers.go et handlers_p1.go

### Modifiés
- **`internal/api/agent_handlers.go` (1 349 → 537 lignes)** réduit au
  protocole agent (register / cmd / result + file de commandes) ; le reste
  rejoint deux nouveaux fichiers :
  `agent_results.go` (487 l. — application des résultats agent sur l'état :
  applyReadState, trafic, uptime, vouchers, normalizePingResult) et
  `handlers_provision.go` (358 l. — provisionning console : provision,
  rotate-token, refresh, import).
- **`internal/api/handlers_p1.go` (1 152 lignes) supprimé et redistribué**
  par vague fonctionnelle : F6 trafic temps réel → `handlers_routers.go`,
  F7 IP bindings → `handlers_ipbindings.go` (253 l.), F8 ping + statut
  commandes → `handlers_commands.go` (133 l.), F9 outils routeur →
  `handlers_router_tools.go` (378 l.), F10 scheduler + alimentation →
  `handlers_scheduler.go` (360 l.) ; `realModeUnsupported` (matrice de modes
  partagée) rejoint `helpers.go`.
- **Plus aucun fichier du paquet `api` ne dépasse 900 lignes** (max :
  handlers_ext.go, 897 l.) ; commentaires de cartographie mis à jour
  (routes.go, docs/CONTRACT-V2.md).
- Garanties vérifiées (même méthode que le découpage de handlers.go) :
  multiset des 413 déclarations top-level strictement identique avant/après
  (specs des blocs const/var éclatés : macPattern et hostnamePattern réémis
  en vars individuelles, payload des regex vérifié identique), table de
  routage (126 registrations mux) octet pour octet, couverture ligne à ligne
  du code déplacé. gofmt, go vet, `go test -race -count=1` (9 paquets) et
  `go build` tous verts — mouvement pur de code, zéro changement de logique
  ni de contrat.

## 2026-09-02 — Refactor Phase B : découpage de handlers.go

### Modifiés
- **`internal/api/handlers.go` (5 188 lignes) supprimé et redistribué en
  17 fichiers par domaine** — mouvement pur de code, zéro changement de
  logique ni de signature :
  `routes.go` (mux + table des ~130 routes), `middleware.go` (JWT, rôles),
  `helpers.go` (outils partagés), et `handlers_<domaine>.go` : auth,
  dashboard, routers, profiles, users, vouchers, sessions, resellers,
  reports, accounting, settings, subscription (le surplus rejoint
  `handlers_admin.go` / `handlers_admin_account.go`).
- Garanties vérifiées : multiset des 391 déclarations top-level strictement
  identique avant/après, table de routage (119 routes) octet pour octet
  identique, imports purgés par `goimports`, `gofmt`/`go vet`/
  `go test -race` (9 paquets)/`go build` tous au vert.
- README (feuille de route cochée) et `docs/CONTRACT-V2.md` (cartographie
  des fichiers) mis à jour.

## 2026-09-02 — Migration Go 1.25 → 1.27.1

### Modifiés
- **Backend migré vers la dernière version stable de Go (1.27.1)** :
  `go.mod` (`go 1.27.0`), image builder Docker `golang:1.27-alpine`,
  `render.yaml` (`GO_VERSION=1.27.1`), README. La CI suit
  `go-version-file: backend/go.mod` automatiquement.
- Validation locale `go1.27.1` : gofmt, `go vet`, `go test -race` (9 paquets),
  `go build -ldflags="-s -w"` — tout au vert, **aucun changement de code ni
  de dépendance requis** (`go.sum` inchangé).

## 2026-09-02 — Chaîne de déploiement Render réparée + filtre monorepo

### Corrigés
- **Déploiements Render en échec (clone GitHub)** : le service Render
  n'était pas réellement lié au dépôt via l'app GitHub Render — il clonait
  anonymement l'URL publique, ce que GitHub bloque désormais par
  intermittence depuis ses IP de build (`could not read Username` /
  `expected flush after ref listing` ×4). Le dépôt est désormais connecté
  via l'app GitHub côté Render : le clonage passe de nouveau (déploiement
  de rattrapage effectué, production à jour).

### Modifiés
- **Filtre monorepo pour Render** : le job `deploy-render` ne déclenche un
  déploiement que si le push a modifié `backend/` (comparaison
  `github.event.before` → `github.sha`) — un push frontend seul ne
  redéploie plus l'API. Côté service Render, l'auto-deploy Git est
  désactivé : le déploiement reste piloté par l'API après CI verte
  (jamais avant la CI, jamais par webhook).

## 2026-09-02 — Filet de sécurité : suite de tests automatisés

### Ajoutés
- **Suite de tests backend complète** : 9 paquets couverts (auth, secretbox,
  store, api, agent, routeros, notify, main) — JWT, bcrypt, chiffrement
  AES-256-GCM, store JSON (roundtrip Save/Reload, état de mise en service,
  défauts), surface HTTP via httptest (santé, 404, 401, matrice rôles,
  liste blanche revendeur, suspension d'abonnement), scripts agent .rsc,
  encodage du protocole RouterOS. Aucun réseau, aucune base :
  `DATABASE_URL` neutralisée, store JSON en répertoire temporaire.
- **CI renforcée** : `go test -race` — le store tient un mutex global, le
  détecteur de races passe désormais à chaque push.

### Corrigés
- **Exemption plateforme de la suspension d'abonnement** : dans
  `authMiddleware`, l'exemption des super-admins plateforme s'appuyait sur le
  contexte de requête, posé APRÈS la garde — code mort : un administrateur
  plateforme consultant un compte suspendu (impersonation support) recevait
  un 402 au lieu du dashboard. L'exemption est désormais évaluée sur les
  claims du token vérifiés (`isPlatformAdminClaims`).

## 2026-09-02 — Préparation du lancement commercial

### Sécurité (sprint P0)
- **Fin des identifiants par défaut** (efc9d7f) : l'administrateur est créé au
  premier démarrage avec un mot de passe aléatoire affiché une seule fois dans
  les logs ; `ADMIN_PASSWORD` obligatoire sur base PostgreSQL vide (le service
  refuse de démarrer sans) ; `JWT_SECRET` obligatoire en production.
- **Limitation de débit renforcée** (efc9d7f) : `/api/reseller/login` à
  5 req/min/IP, `clientIP` basé sur le dernier hop de `X-Forwarded-For`
  (anti-contournement par IP forgée).
- **Rôle revendeur en liste blanche + garde économique des lots** (04a3716) :
  403 pour un revendeur sur dashboard/génération de lots, contrôle prix de
  profil vs montant du lot.
- **Webhook Wave** (095a1a6) : montant strictement positif exigé sur les
  succès de paiement.

### Modifiés
- **Suppression définitive des données de démonstration** (eac1b1d) : le seed
  (~860 lignes) est retiré du code — toute base vide démarre en état de mise
  en service ; nouvelle route `POST /api/admin/purge-demo` pour retirer
  chirurgicalement les artefacts hérités de l'ancien seed en production.
- **Rapports 100 % réels** (eac1b1d, 5c3ab8c) : la courbe de trafic
  synthétique est remplacée par les connexions réelles par jour (UserLogs) ;
  les KPI ne comptent que des événements réels — générer du stock n'est pas
  vendre.
- **QR des vouchers = lien de connexion hotspot** (099168f) : le scan ouvre
  le portail et connecte l'appareil (`http://<dns>/login?username=…&password=…`)
  au lieu d'afficher le code déjà imprimé ; fallback historique sans DNS.
- **Crédit FTCI cliquable** (099168f) : « © 2026 FTCI — Freelance Technologies
  Côte d'Ivoire » → https://ftci.fr/ sur la landing, le login et la console.
- **Impression A4 affinée** (288fe4f, fff810b) : espacements resserrés
  (2–3 mm), tickets agrandis (zoom ×0,75, code 17 px, QR 68 px), plafond
  25–35 tickets/feuille en 5 colonnes, pagination à taille constante au-delà.

### Corrigés
- Retour de stock impossible — corps doublement encodé (143bb41).
- Enforcement routeur des vouchers expirés — les « used » n'expiraient jamais,
  tickets fantômes dans Winbox (ed23cd0).
- L'impression d'un lot ne sort que les tickets ACTIFS (20e641a).
- Mode « mot de passe = identifiant » — la grille A4 héritée n'affiche plus
  qu'un seul élément (ce2625a).

### Ajoutés
- Retour de stock initié par le revendeur (698c5b7) et compensation de la
  dette dépôt-vente avec le crédit prépayé (ff61a9a).
- Logo du client au centre du QR code, importé dans les Paramètres (f04e006).
- Grille A4 adaptative — code en gras en mode same, max de tickets par
  feuille (c4c135d).

## 2026-09-01 — Vouchers, revendeurs, profils

### Ajoutés
- **Rapports v2** (4270bc1) : KPI enrichis sur les 3 onglets (Δ%, canal,
  top revendeurs, heures de pointe, marge avancée).
- **Dépôt-vente revendeurs** (5c9ade5, 54ef745) : mode « il vend puis verse »
  avec plafond de créance, créances au dashboard, recouvrement et reçu de
  versement.
- Transfert de stock des lots déjà générés — distribution revendeur / retour
  de stock (69154d9).
- Impression A4 en PDF réel — mise en page figée, indépendante du navigateur
  (4068645) ; bandeau de marque logo · tenant · prix (be9dec1).
- Wizard de création de vouchers en 3 étapes — Forfait → Codes → Récap
  (de33389).
- Studio Forfait — dialog profil repensé avec aperçu live (ba66d74).
- Statistiques stock vs vendus par revendeur + rapport de fin de journée en
  Mode Vente (8f6e544).
- PWA : ouverture directe sur le login (f5322d3).
- Validité ancrée au 1er login — le stock jamais connecté n'expire plus
  (c245e54).

### Corrigés
- Parité limit-uptime — les tickets coupés par le routeur passent « expirés »,
  plus de statut « utilisé » fantôme (82067a2).
- Impression grille A4 — dialog recentré, grille intachable et couleurs
  forcées (3e34e81, d6517a1) ; suppression du doublon d'impression jsPDF
  (0c70db1).

## Fondations (antérieures)

- CI GitHub Actions (gofmt/vet/build Go + ESLint/build Next.js) avec
  déploiement Render déclenché après CI verte (4c4fefc, f5ed281, 067a167).
- Persistance PostgreSQL/Neon — synchro différentielle à chaque sauvegarde,
  fallback JSON local en développement (4c4fefc).
- Agent HTTP-poll natif : le routeur se connecte lui-même au cloud toutes les
  45 s (connexions 100 % sortantes, aucune IP publique requise).
- Client RouterOS : protocole binaire natif (port 8728), login v6.43+ et
  fallback challenge MD5.
- Contrat d'API V2 issu de l'audit Mikhmon v3 : [`docs/CONTRACT-V2.md`](docs/CONTRACT-V2.md).
