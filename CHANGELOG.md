# CHANGELOG — MikCloud

Historique des évolutions notables du projet. Format inspiré de
[Keep a Changelog](https://keepachangelog.com/) ; les versions correspondent
aux dates de livraison — le déploiement est continu : chaque push `main` passe
la CI puis se déploie automatiquement (frontend Vercel, backend Render).

## 2026-09-17 — N°136 — Slides du carrousel commercial : chaque gérant remplace les 3 visuels publicitaires génériques du portail captif depuis la console (Paramètres → Hotspot → Expérience)

### N°136 — Contexte : retour utilisateur
Renumérotation : N°135 pris par le logo du portail captif (83ee601 — bloc
logo cloud + sig v2 de re-déploiement automatique) pendant que ce travail
attendait son push — il devient N°136.
« Comment chaque client peut changer les 3 slides sur le portail captif
depuis le frontend ? » — le portail commercial affiche par défaut trois
visuels publicitaires génériques (img/pub1/2/3.jpg du template, carrousel
Swiper) ; aucun chemin de console ne permettait de les personnaliser. La
bannière (N°45) et la vitrine hospitalité (N°55) avaient leur carte
Paramètres, pas le carrousel du mode commercial — le trou de la famille.

### Produit — la carte « Portail : images du carrousel »
- CONSOLE (onglet Expérience de la vue Hotspot, entre la bannière N°45 et le
  mode hospitalité N°55) : jusqu'à 3 slots image, téléversement vers R2 via
  `POST /api/media` (URL https permanente, même flux que bannière/promos —
  ≤ 2 Mo, type image sniffé), remplacement slot par slot, suppression par
  slot, compteur n/3, sauvegarde `PUT /api/settings` (corps défensif plats +
  nested `tenant{…}`, pattern VoucherCard) puis invalidation du cache
  settings. Retrait de toutes les images = retour aux visuels génériques.
- VALIDATION SERVEUR (matrice bannière N°45) : `portalSlides` accepte ≤ 3
  URLs `https://` de ≤ 300 car. (même plafond que les images de promos) ;
  4 slides → 400, `http://`/`javascript:`/relatif → 400, entrées vides
  ignorées, liste vide = effacement (retour aux défauts), `nil` = inchangé.
- PORTAIL (login.html) : `applySlides(cfg)` remplace le contenu du
  `.swiper-wrapper` par les images du gérant — idempotent par signature
  d'état (`data-mik-slides` : le fetch live N°48 peut rappeler applyConfig
  sans re-rendu superflu), restaure les visuels pub1/2/3 quand la config se
  vide (un portail déjà ouvert se corrige en direct), échappe chaque URL
  (`escapeHtml` — aucune injection), et duplique une image unique (le mode
  loop de Swiper exige ≥ 2 slides — même visuel, zéro différence pour
  l'invité). L'instance Swiper est exposée (`window.mikSwiper`) pour le
  `update()`/`slideTo(0)` après réécriture (`observer:true` reste le filet).
- CONFIG (deux chemins couverts) : `PortalConfig.Slides` est servi par le
  servage routeur (`/portal/{token}/login.html`, config figée au moment du
  déploiement du portail sur le routeur) ET par le fetch live (N°48,
  `GET /api/wifi/site/{slug}/portal`).
- PROPAGATION AUTOMATIQUE (intégration à la sig v2 du N°135) :
  `portalSlides` rejoint `portalBrandingFingerprint` — la règle du contrat
  y est explicite (« si un champ rejoint la config du portail sans rejoindre
  cette empreinte, le portail déployé garderait une valeur périmée sans
  jamais se re-déployer ») : changer les slides en console change la sig →
  re-déploiement du portail au check-in suivant (≤ 45 s), pour TOUS les
  routeurs du compte — même pure-vouchers (sans site WiFi, donc sans fetch
  live). Et comme le CONTENU du template change aussi (applySlides),
  `hotpage.Sig` change → les portails déployés AVANT ce commit reçoivent le
  nouveau template automatiquement au check-in suivant : zéro manipulation
  manuelle, l'astuce console dit simplement « le portail se met à jour au
  check-in suivant ».

### Technique
Backend — model/tenant.go (`PortalSlides string`, JSON `["url",…]` ≤ 3,
pattern N°55 des listes sérialisées), store/pg_schema.go (colonne à-plat
`settings.portal_slides` TEXT NOT NULL DEFAULT '', migration boot
idempotente), pg_load.go + pg_sync.go (lecture/synchronisation, placeholder
$34), api/handlers_settings.go (champ plat + nested `tenantPut`, validation
stricte, application nil-sûre), api/portal_serve.go (`portalSlidesList` :
décodage DÉFENSIF au servage — JSON invalide/vide/non-https → nil, plafond 3
RE-VÉRIFIÉ, défense en profondeur contre une ligne héritée d'un appel API
direct ; branché dans `buildPortalConfig` ET `buildPortalConfigForSite`),
hotpage/templatize.go (`PortalConfig.Slides []string`, omitempty — absent
des configs pré-N°136), hotpage/template/login.html (applySlides +
mikSwiperRefresh + instance globale), api/hotspot_files.go
(`t.PortalSlides` rejoint portalBrandingFingerprint — la sig v2 du N°135
déclenche le re-déploiement au changement d'images). Frontend —
components/hotspot/parts/hotspot-cards.tsx (carte PortalSlidesCard : état
local dérivé de `tenant.portalSlides`, upload R2 par slot, bouton
« Ajouter une image » pour le prochain slot libre), lib/hotspot/types.ts
(`AppSettings.tenant.portalSlides?: string`), i18n fr/en (8 clés
`settings.slides.*`).

### Vérifié
- gofmt vide, go vet OK, go build OK, go test 12 paquets VERTS — dont
  portal_slides_test.go NOUVEAU : TestPortalSlidesList (11 cas de décodage :
  vide/invalide/objet → nil, http filtré, espaces trimés, plafond 3
  re-vérifié), TestSettingsPortalSlidesValidation (matrice PUT : défaut
  absent, 3 URLs persistées puis relues par GET, 4 slides → 400, schémas
  interdits → 400, URL > 300 car. → 400, entrées vides ignorées, liste vide
  = effacement), TestPortalServeSlides (la config JSON du login.html servi
  embarque portalSlides — chemin routeur), TestWifiPortalSlides (le fetch
  live porte les slides — chemin hybride), TestEnsureHotspotFilesLockedSlides
  (poser les slides change la sig → la commande hotspot_files est re-filée —
  la promesse de propagation automatique est gardée par test).
- eslint 0, tsgo 0. Correctif attrapé par relecture AVANT commit : la carte
  utilisait `t("settings.save")` (clé inexistante) au lieu de
  `t("common.save")` utilisé par toutes les cartes sœurs.

### Docs
CHANGELOG N°136 + CONTRACT-V2 §N°136 (contrat de validation, contrat de
servage/défense en profondeur, comportement du template par ancien/nouveau
portail).

## 2026-09-17 — N°135 — Le portail captif porte le logo du client : le logo d'un autre établissement chassé du template (bloc logo cloud + initiale du tenant), re-déploiement automatique au changement de branding (sig v2)

### N°135 — Contexte : trois causes pour un logo qui n'était pas le bon
Renumérotation : N°134 pris par l'épinglage de la région Vercel (582b895 — vercel.json fra1) pendant que ce travail attendait son push — il devient N°135.
Retour utilisateur : « le logo sur le portail captif doit être le logo du client ». Le diagnostic remonte TROIS causes cumulées — aucune ne touchait la chaîne de données (tenant.logoUrl existait déjà : upload console carte « Vouchers » F2, propagation dans PortalConfig, swap JS côté login.html) : (1) LE DÉFAUT ÉTAIT LE LOGO D'UN AUTRE CLIENT — le template de référence embarquait img/logo.png, le logo du site pilote de l'audit (« CYBER ESPACE SC », ~288 Ko), avec alt « Logo CYBER ESPACE SC » et repli « SC » : tout portail sans logo configuré affichait la marque d'un AUTRE établissement ; (2) LA SIG NE COUVRAIT PAS LE BRANDING — ensureHotspotFilesLocked signait le CONTENU des fichiers du template (N°48-b) mais rien du compte : un logo posé en console ne changeait pas la sig → aucun re-déploiement → le fallback inliné servi par le routeur gardait le branding du dernier déploiement (le bouton « Re-déployer » restait le seul chemin) ; (3) LE FETCH LIVE ÉTAIT CONDITIONNÉ AU SITE WiFi — tryFetchLive rend les armes sans wifiSlug : les routeurs pure-vouchers (cybercafé classique, sans site WiFi jetable lié) ne recevaient JAMAIS le branding frais. Trois chemins, une convergence : le logo du pilote restait à l'écran.

### Produit — le bloc logo est servi par le cloud, jamais par le template
(1) MARQUEUR {{MIKCLOUD_LOGO_BLOCK}} (hotpage.Personalize) : quand tenant.logoUrl est défini, login.html reçoit l'<img> DU CLIENT (data URL échappée, alt « Logo {tenant} », onerror → repli) suivie du repli initiale masqué ; sinon le repli SEUL — l'INITIALE du tenant (tenantInitial : première lettre unicode du nom, majuscule — « promax wifi » → P, « éclair Net » → É ; repli « W » pour un nom sans lettre), posée sur le dégradé teal existant du bloc 84×84. Un portail sans logo configuré est signé par l'établissement lui-même, JAMAIS par un autre client. (2) img/logo.png RETIRÉ du template (~288 Ko de moins par déploiement — prolonge l'amaigrissement N°75) : le retrait change DefaultFiles() donc la sig → au premier check-in qui suit le déploiement de ce correctif, CHAQUE routeur agent re-déploie son portail et récupère le bloc logo neutre (auto-guérison, même philosophie que N°132 ; le fichier résiduel reste sur le routeur mais plus rien ne le référence). (3) CSS : .logo-wrap img passe de object-fit:cover à CONTAIN — le logo d'un vrai client (souvent rectangulaire, pas un carré parfait) n'est plus rogné aux bords du bloc. (4) JS applyConfig (login.html) : le bloc branding devient idempotent — crée l'<img> si le fetch live apporte un logo que le fallback n'avait pas, met à jour le src si le logo a changé, RETIRE l'<img> et rallume le repli si le live n'a plus de logo (un logo retiré en console ne survit pas au fetch live) ; compat vieux WebViews conservée (var, pas d'optional chaining, removeChild plutôt que Element.remove).

### Produit — sig v2 : le branding re-déploie le portail (≤ 45 s)
hotspotFilesSig(files, db, router) = hash(hotpage.Sig(files) ⊕ portalBrandingFingerprint(db, router)) — 16 caractères, colonne inchangée. L'empreinte couvre TOUT ce qui atteint le fallback inliné : nom, LOGO, bannière, Wave, joinButton effectif, hospitalité (style/welcome/promos/socials), portalKey, rétention journal, APP_PUBLIC_URL (façonne wifiUrl/joinUrl cuits au déploiement), 1er site WiFi actif lié (slug + marketingOptIn + quota effectif via wifiQuotaResp), token du 1er lien join actif (une révocation/création re-déploie — l'URL cuite en dépend), offres payables (max 8, même plafond que la config servie : une 9e offre ne déclenche pas de re-déploiement vain). Miroir compact de buildPortalConfig — mêmes helpers, mêmes résolutions : un champ qui rejoint la config du portail sans rejoindre l'empreinte laisserait le portail périmé sans jamais le re-déployer. Les URL dérivées de la requête (apiBase) restent hors empreinte (constantes par déploiement). Le bouton « Re-déployer » console reste pour le forçage. La promesse documentée du package hotpage depuis N°35 (« un changement de config (branding, offres…) change la sig → re-déploiement automatique au check-in suivant ») est enfin TENUE.

### Console — la bonne carte dit où poser le logo
Carte « Vouchers » (onglet Expérience, section Hotspot) : description et indice du logo reformulés — le logo s'affiche sur les tickets, AU CENTRE des QR codes ET sur le PORTAIL captif (FR/EN). Onglet « Portail » : note d'orientation en tête (icône Palette, portal.brandingNote) — le logo du portail est celui de l'établissement, il se pose dans l'onglet Expérience (carte Vouchers), et tout changement de branding est re-déployé automatiquement (≤ 45 s).

### Technique
Backend + copie i18n, AUCUNE route, AUCUN changement de schéma (logoUrl vivait déjà dans settings F2, la sig dans routers.hotspot_files_sig TEXT) : hotpage/templatize.go (marqueur + logoBlock + tenantInitial + import unicode), hotpage/template/login.html (markup {{MIKCLOUD_LOGO_BLOCK}}, CSS contain, JS applyConfig idempotent), hotpage/template/img/logo.png SUPPRIMÉ, api/hotspot_files.go (hotspotFilesSig + portalBrandingFingerprint + imports agent/strconv/strings + doc sig v2), tests : hotpage_test.go (TestFileBinary → pub1.jpg en-tête JPEG + garde logo.png absent, TestHasFile logo.png=false, NOUVEAU TestLoginTemplateLogoBlock — marqueur présent, img/logo.png et « CYBER ESPACE SC » bannis, retrait dynamique JS, object-fit contain), templatize_test.go (NOUVEAU TestPersonalizeLogoBlock — img + repli masqué / initiale seule sans <img> / repli W / initiale accentuée É ; TestPersonalizeLogoBlockInjection — échappement attribut), hotspot_files_test.go (SigMatch posée via hotspotFilesSig, NOUVEAU TestEnsureHotspotFilesLockedBrandingChange — logo posé → 1 commande filée, sig à jour → 0), portal_templating_test.go (BinaryNotTemplated → pub1.jpg JPEG, NOUVEAU TestPortalServeLogoBlock — initiale sans <img>, puis <img> du client + alt + repli masqué après pose du logo, img/logo.png jamais servi) ; frontend : i18n-fr/en settings.ts (voucherCardDesc, logoHintPre) + portal.ts (portal.brandingNote NOUVEAU), portal-view.tsx (note Palette + import). Docs : CONTRACT-V2 §N°135 (marqueur, sig v2, empreinte branding) + settings LogoURL annoté, TEMPLATE.md (img/ sans logo.png, ligne Logo du tableau de personnalisation).

### Vérifié
gofmt vide, go vet OK, go build OK, go test 11 paquets VERTS (dont 4 tests N°135 nouveaux + 4 mis à jour) ; frontend eslint 0, tsgo 0 ; scan mojibake 0 — toutes les éditions (Go, HTML, i18n) octet-précises via scripts Python à ancres uniques assertées (discipline N°128).

### Leçon
Un « défaut » n'est neutre que s'il n'appartient à personne : le logo par défaut d'un template multi-client doit être une abstraction (initiale, glyphe), jamais l'asset du premier client qui a servi de référence — chaque déploiement suivant en fait de la publicité gratuite au mauvais destinataire. Et une signature de déploiement qui n'inclut pas les données personnalisées ferme la boucle AVANT les personnalisations : le template se met à jour tout seul, l'image de marque jamais.

## 2026-09-17 — N°134 — Localisation : « le serveur Vercel en Europe » — c'était déjà le cas (toute la stack est à Francfort), et le choix devient du CODE : la région des fonctions Vercel est épinglée à fra1 dans frontend/vercel.json

### N°134 — Contexte : retour utilisateur
« Changer la localisation du serveur Vercel pour Europe. »

### Diagnostic : la stack est déjà intégralement à Francfort
- VERCEL (API, équipe Ftech CI, projet mikcloud) : `serverlessFunctionRegion: fra1`
  DEPUIS LA CRÉATION du projet (2026-08-29) — les 100 derniers déploiements
  production listés (du 2026-09-06 à 659e01a) portent tous `regions: ["fra1"]`,
  `originCacheRegion: fra1` : les fonctions (rendu SSR) n'ont JAMAIS tourné
  ailleurs qu'en Europe.
- RENDER (API v1) : service mikcloud, `region: "frankfurt"`
  (`ssh.frankfurt.render.com`), plan free, autoDeploy sur main.
- NEON : hôte `*.eu-central-1.aws.neon.tech` — Francfort (eu-central-1).
- Conséquence réseau : les trois étages sont co-localisés dans le même
  métropole (Render→Neon : RTT intra-région ~1-3 ms) ; pour un visiteur
  d'Europe de l'Ouest/Afrique de l'Ouest, Vercel et Render sont à un bond
  de câble sous-marin (RTT Abidjan→Francfort ~140-160 ms contre ~200-230 ms
  vers la côte Est américaine).

### Ce qui change : le réglage quitte le dashboard pour le dépôt
- `frontend/vercel.json` (c'est LUI que Vercel lit — `rootDirectory: frontend`)
  gagne `"regions": ["fra1"]` — STRICTEMENT la même valeur que la
  configuration projet : aucun changement fonctionnel au prochain
  déploiement, mais la région devient versionnée. Une manipulation du
  dashboard ne peut plus la faire dériver silencieusement, et le choix
  « Europe » est lisible, diffable et auditable dans Git.
- Précision d'architecture pour la lecture du réglage : les FICHIERS
  STATIQUES (HTML/CSS/JS) sont servis par l'edge network MONDIAL de Vercel
  (POP au plus près de chaque visiteur — vérifié : HIT depuis un POP Asie
  depuis ce sandbox ; rien à régler, c'est toujours « local »). La région
  fra1 ne concerne que les FONCTIONS (rendu du HTML dynamique). L'app
  étant majoritairement côté client (vues dynamic(), appels directs au
  backend Render depuis le navigateur via NEXT_PUBLIC_API_BASE), l'impact
  de la région sur les performances est modeste — mais le choix est
  désormais explicite, aligné sur Render et Neon, et verrouillé.

### Vérifié
- API Vercel avant modification : `serverlessFunctionRegion: fra1`,
  dernier déploiement production `regions: ["fra1"]`.
- API Render : `region: "frankfurt"` dans serviceDetails.
- Frontend : vercel.json valide (schéma officiel), lint 0, typecheck 0 —
  `next build` ne lit pas vercel.json, la CI est neutre ; le déploiement
  Vercel post-push est surveillé (région fra1 confirmée sur le déploiement
  résultant, mikcloud.ftci.fr 200).


## 2026-09-17 — N°133 — Réactivité P1 « le verrou ne portait plus l'E/S, il portait le calcul » : synchronisation ciblée par tables marquées sales, agrégats du dashboard HORS verrou, en-tête Server-Timing — le plan structurel de l'audit performance passe à l'application

### N°133 — Contexte : P0 rendu, reste la charge CPU du flush et le calcul sous verrou
Renumérotation double : N°131 pris par la vitrine (2e6f38d — retrait du
doublon d'essai + animation du tchat), puis N°132 pris par le correctif du
guillemet routeros (59dcad8 — script routeros_check invalide) pendant que ce
travail attendait son push — il devient donc N°133.
Le P0 (N°130) avait sorti la transaction Neon du verrou global (Save
asynchrone) et vidé le réseau (304 réactivé, prefetch au survol). L'audit
laissait deux travaux structurels : (B-résiduel) CHAQUE flush re-hashait
toujours les 33 tables différentielles COMPLÈTES — sur le 0,1 vCPU Render,
~8 500 lignes (3 130 hotspot_users + 5 000 user_logs + le reste) soit
~1,4-2,8 s de CPU par synchronisation, alors que l'état de croisière d'un
parc AGENT ne change que de ~6 lignes (2 routeurs télémétrie + 4 settings
last_tick) : le CPU rendu au service était dépensé à re-marshaller des
lignes identiques ; (E) le dashboard calculait TOUS ses agrégats
(boucles sessions/utilisateurs/transactions, tris, courbes 14 j, créances)
SOUS le verrou global — le mutex ne portait plus d'E/S mais portait du
calcul : chaque concurrent attendait la fin du poll de chaque autre.

### Produit — la sauvegarde ne re-hashe plus que ce qui a changé (par table)
- MOTEUR MARQUANT : Tick, applyExpiry, tickTraffic, enforceExpired,
  sweepDeadBatches et sweepStaleRegistrations reçoivent un `*TableSet`
  (tables.go — nil-sûr) et marquent les tables qu'ils ont RÉELLEMENT
  modifiées : télémétrie (routers), trafic/qualité de ligne simulés,
  progression/naissance/mort de sessions, compteurs utilisateurs, journal
  (login/logout/expire/kick), ventes simulées (resellers), expiration/
  nettoyage/rétention, drapeaux Enforced, commandes déposées, lots éteints,
  tombstones, inscriptions périmées. En croisière d'un parc agent : SEULS
  routers (+ settings via syncSettings, toujours écrit) sont marqués.
- SAUVETABLES CIBLÉE : les chemins de LECTURE pollés (dashboard, sessions,
  listes utilisateurs/vouchers, stats, trafic/qualité d'un simulateur,
  liste des lots) appellent `SaveTables(...)` au lieu de `Save()` : le
  flush re-hashe UNIQUEMENT les tables marquées, les autres conservent
  leurs empreintes (reportées telles quelles après commit — atomicité
  pending→commit→swap préservée). TOUT chemin d'écriture conserve le
  `Save()` complet : défaut sûr, une mutation non ciblée reste TOUJOURS
  persistée au cycle suivant (et l'état mémoire reste la vérité : un
  marquage incomplet ne perd rien, il retarde).
- PARCOURS DU CIBLAGE : une liste vide retombe sur le diff complet ;
  un échec de synchro ciblée re-marque TOUT (le retry repart de toutes
  les vraies différences) ; la table settings reste TOUJOURS écrite
  (last_tick/last_sweep ne dépendent pas du ciblage) ; le mode JSON
  (développement/E2E) ignore le ciblage (écriture complète synchrone).
- REBUILDHASHES COMPLÉTÉ (latent) : les tables chat_conversations,
  chat_messages et devices existaient dans Sync mais manquaient au cache
  d'empreintes reconstruit au boot — le premier flush post-démarrage les
  re-upsertait intégralement pour rien. Elles sont désormais hashées au
  boot comme les autres (parité des trois listes : constantes ↔ specs ↔
  santé, verrouillée par TestSyncKnownTablesConcordance).

### Produit — le dashboard calcule hors verrou (photographie CloneDeep)
- Le mutex ne porte plus que Tick + enforcement + photographie CloneDeep
  (quelques millisecondes, aucune E/S — même mécanisme que le flush N°130)
  ; TOUS les agrégats (sites, KPI, courbes revenus 14 j, top profils,
  activité récente, timeline 24 h, créances revendeurs) se calculent sur
  le SNAPSHOT, hors verrou : les polls dashboard (15 s) ne sérialisent
  plus les autres requêtes.

### Produit — Server-Timing : le temps serveur devient observable
- Chaque réponse porte désormais `Server-Timing: app;dur=###` (RFC 8006),
  posé au premier octet écrit : DevTools → Network → Timing sépare le
  temps SERVEUR du temps réseau/proxy. Sur le plan Render free, un dur
  systématique ~200 ms signale le plancher cause A de l'audit (décision
  payante P2 : Starter ~7 $/mois) ; une valeur qui grimpe signale une
  contention applicative — désormais traitée.

### Technique
- Backend : store/tables.go (NOUVEAU — constantes Table*, registre,
  TableSet nil-sûr), store/store.go (SaveTables, flush à deux régimes,
  moteur marquant Tick/applyExpiry/tickTraffic/Sweep/logUserEvent/
  kickLockedUsers), store/pg_sync.go (SyncTables + syncPlan commun à
  clôtures typées, rebuildHashes paritaire), api/helpers.go (enforceExpired
  marquant), api/handlers_dashboard.go (photographie + agrégats hors
  verrou), handlers_sessions/users/routers/vouchers (SaveTables ciblée),
  main.go (statusRecorder → Server-Timing), syncstats (3 tables de plus).
- Vérifié : gofmt vide, go vet OK, go build OK, go test 12 paquets verts
  — dont TestTableSet, TestSyncKnownTablesConcordance (alignement
  constantes ↔ plan ↔ santé), TestTickMarksTouchedTables (invariant
  perf : un parc agent ne marque QUE routers ; garde 2 s marque rien ;
  un simulateur marque traffic/line_quality) et TestSaveTablesJSONMode.
- Leçon consignée : un verrou global qui porte du calcul est une file
  d'attente déguisée en mutex — photographier tôt et calculer dehors
  vaut pour toute lecture lourde, pas seulement pour l'écriture.

## 2026-09-17 — N°132 — « Le guillemet qui tuait la vérification » : le script routeros_check généré par N°125 était syntaxiquement invalide (guillemet ouvrant manquant dans le rapport dynamique) — l'import échouait sur le VRAI routeur, la commande restait « sent » sans rapport à jamais et la mise à jour de flotte perdait toutes ses cibles ; au passage, auto-upgrade est lu au bon chemin

### N°132 — Contexte : retour utilisateur
« Le commit N°125 semble ne pas être fonctionnel et crée un bug : la
vérification des mises à jour échoue et la mise à jour du parc ne fonctionne
plus. »

### Diagnostic
- UNE CHAÎNE MANQUANTE : le rapport dynamique du check concatène les valeurs
  lues côté routeur en sandwich RouterOS (« &cle=". $var . »). Le fragment
  Go ajouté par N°125 commençait par `&fwCurrent="` SANS son guillemet
  ouvrant — le script généré contenait `…&channel=". $rosChan .&fwCurrent=".
  $fwCur…`, où l'opérateur de concaténation `.` est suivi de `&` au lieu
  d'une chaîne : ERREUR DE SYNTAXE RouterOS.
- CONSÉQUENCE EN CHAÎNE sur le parc réel (invisible en simulé : l'agent
  joué en python POSTE le rapport lui-même, il ne PARSE jamais le script) :
  l'`/import` du fichier de commandes avorte à la ligne fautive → AUCUN
  rapport ne part → la commande `routeros_check` reste « sent » → la reprise
  zombie (10 min, staleSentReadKinds) la re-file… avec le MÊME script cassé
  → boucle d'échec infinie. La vérification n'aboutit jamais, et la mise à
  jour de flotte — qui ne cible QUE les routeurs en état `available` détecté
  par un check abouti — ne trouve plus aucune cible : « Aucune mise à jour
  à lancer ».
- AGGRAVANT : le script check voyage dans le lot prioritaire du check-in ; son
  erreur de syntaxe avortait l'import du fichier ENTIER — les commandes
  servies derrière dans le même lot n'étaient pas exécutées non plus.
- SECOND DÉFAUT N°125 découvert au passage : `auto-upgrade` était lu sous
  `/system routerboard get …` alors qu'il vit sous
  `/system routerboard settings` sur le vrai matériel (le `set` du même
  commit utilisait d'ailleurs le bon chemin). Lecture isolée donc non
  fatale, mais `fwAuto` restait TOUJOURS vide : l'indicateur auto-upgrade ne
  pouvait jamais s'afficher.

### Technique
- UNE LETTRE : `agent/routerosupdate.go` — le second fragment du rapport
  devient `"&fwCurrent=". $fwCur …` (guillemet ouvrant rétabli, sandwich
  identique aux six autres clés). Le script généré redevient parsable :
  l'import s'exécute, le rapport part, la commande passe « done ».
- CHEMIN CORRIGÉ : lecture `[/system routerboard settings get auto-upgrade]`
  dans le check (miroir du `set` déjà correct de l'update).
- NORMALISATION DURCIE : `firmwareAuto` accepte « true » ET « yes »
  (précédent device-mode : certains builds stringifient les booléens).
- GARDE ANTI-RÉGRESSION : `TestRouterOSScriptsQuoteParity` — chaque ligne
  `http-data=` des trois scripts (check / update / firmware) doit porter un
  nombre PAIR de guillemets (la ligne fautive de N°125 en portait 15).
  Couplé au littéral exact `."&fwCurrent=". $fwCur …` et au chemin
  `settings` assertés dans `TestRouterOSCheckScriptShape`, plus le cas
  « yes » dans `TestNormalizeRouterOSCheckFirmware`.
- Scan systématique : la classe de bug (jonction `.` + `&` entre fragments
  Go) a été recherchée sur TOUT le codebase — une seule occurrence, celle-ci
  (le seul autre hit est le commentaire du test qui cite le bug).

### Auto-guérison du parc (aucune intervention requise)
- `routeros_check` est idempotent et figure dans `staleSentReadKinds` : les
  commandes « sent » zombies encore en base au moment du déploiement sont
  re-filees au check-in suivant (≤ 10 min) et servies cette fois avec le
  script CORRIGÉ (le générateur vit côté cloud) — le parc se rétablit
  seul, check par check.

### Vérifié
- Reconstruction octet-exacte du script généré (Python, miroir du
  compilateur) : sandwich complet `…&channel=". $rosChan ."&fwCurrent=".
  $fwCur ."&fwStaged=". $fwStg ."&fwAuto=". $fwAuto) output=none` ; parité
  des guillemets 16/16 sur la ligne ok du check, 8/8 sur firmware ; les
  lignes statiques reportLine portent 4 guillemets par construction ;
  équilibre accolades/parenthèses OK sur les 4 fichiers modifiés (checker
  séquentiel conscient des chaînes/commentaires Go) ; zéro autre jonction
  suspecte dans le codebase.
- Sans toolchain Go local (sandbox réinitialisé) : la CI GitHub (Backend
  Go : gofmt, vet, build, tests — dont les trois nouveaux gardes) et le build
  Render tranchent au push ; frontend non touché.

## 2026-09-17 — N°131 — La vitrine retire le doublon et réveille son chat : le bouton « Essai gratuit » quitte le rail (le header le porte déjà), l'icône du tchat gagne une animation « wahou » qui dit au visiteur qu'il peut écrire sa question

### N°131 — Contexte : deux retours vitrine
Retour utilisateur : « supprimer le bouton essai gratuit du rail, déjà présent
dans le header, donc redondance ; ajouter une animation (effet wahou) à
l'icône du tchat afin que le visiteur sache qu'il peut écrire ou poser les
questions qu'il veut ». Deux maux : un CTA en double (rail vertical desktop +
rail tactile mobile, alors que la topbar sticky affiche déjà « Essai gratuit »
à TOUTES les largeurs — seul « Se connecter » se cache sous 860 px), et une
icône de chat muette — le widget N°127 est né après la refonte, rien ne
signale au premier visiteur qu'il peut y ÉCRIRE n'importe quelle question.

### Produit
- RAIL ÉPURÉ : le bouton « Essai gratuit » disparaît du rail vertical (desktop)
  ET du rail tactile (mobile) — markup, clé de copie `rail.cta` (FR/EN), règles
  CSS `.mkl-rail-cta`/`.mkl-rail-cta-mobile` et sélecteurs `button` morts du
  rail. Le CTA d'essai reste porté par le header sticky (visible desktop comme
  mobile) et les boutons hero/CTA final — un seul endroit par écran, zéro
  doublon.
- ANIMATION « WAHOU » DE DÉCOUVERTE (icône chat) : tant que le visiteur n'a
  JAMAIS ouvert le chat, le bouton flottant VIT —
  (1) REBOND CLAY squash & stretch (burst ~2 s puis repos dans un cycle
  5,6 s : le bouton s'écrase, s'étire vers le haut, retombe avec un léger
  rebond résiduel — la signature Claymorphisme en mouvement) ;
  (2) ONDES CONCENTRIQUES : deux halos sarcelle partent du bouton toutes les
  2,8 s (décalées de 1,4 s) — l'invitation visuelle « quelque chose vit ici » ;
  (3) MINI-BULLE D'INVITATION : « Une question ? Écrivez-la ici ! » (FR) /
  « Got a question? Ask away! » (EN), bulle ivoire clay à coin vif pointé vers
  le bouton, précédée de trois points de frappe animés — le signe universel
  « on peut écrire ici » ; cycle 9 s (apparition ressort, tenue, rétraction),
  desktop à gauche du bouton, mobile au-dessus (l'écran est étroit).
- LA FÊTE S'ARRÊTE D'ELLE-MÊME : au PREMIER clic sur le chat, l'animation se
  calme pour de bon (l'effort de découverte a fait son office — animer
  éternellement à côté d'une conversation ouverte serait du bruit) ; l'empreinte
  localStorage `mikcloud-chat` (posée à la création de session) garde le teasing
  éteint aux visites suivantes. Un visiteur qui connaît déjà le chat n'a pas
  besoin qu'on lui refasse la démonstration.
- ACCESSIBILITÉ : la bulle est décorative (aria-hidden, pointer-events none —
  le BOUTON reste la cible accessible, 56-62 px, pointer-events auto) ;
  prefers-reduced-motion coupe toutes les animations (règle `.mkl-page *`
  existante — la bulle devient statique, l'information reste).

### Technique
- Frontend uniquement, zéro route, zéro schéma : landing-page.tsx (retrait des
  deux boutons CTA du rail), chat-widget.tsx (état `tease` : armé 1,6 s après
  le montage si `mikcloud-chat` absent du localStorage, éteint au premier
  ouverture), landing-copy.ts (rail.cta supprimée, chat.tease ajoutée FR/EN),
  landing-clay.css (retraits CTA + bloc N°131 : mkl-fab-bounce/mkl-fab-ripple/
  mkl-tease-cycle/mkl-tease-dot + position mobile de la bulle).
- Correctif attrapé au smoke navigateur AVANT tout commit : le modifieur du
  bouton s'appelle `mkl-chat-fab-tease` et JAMAIS `mkl-chat-tease` (classe de
  la BULLE) — la première version partageait la classe, si bien que la règle
  de la bulle (fond ivoire, position right:104px, pointer-events none)
  s'appliquait AUSSI au bouton : chat décoloré, déplacé et NON CLIQUABLE.
  Détecté par témoin DOM (même classe sans modifieur → sarcelle) + analyse VLM
  de la capture ; leçon consignée : un modifieur partage le namespace de
  classes, il ne réutilise jamais la classe d'un autre composant.

### Vérifié
- eslint 0 erreur, tsgo 0 erreur.
- Smoke navigateur (next dev :3021) : desktop 1440 px — rail sans CTA (6
  liens), header « Essai gratuit » présent, FAB sarcelle rgb(14,124,123) à
  right:26px cliquable, rebond + ondes + bulle complets (analyse VLM 5/5) ;
  clic → panneau ouvert, teasing disparu, refermé → il ne revient pas dans la
  vue ; rechargement avec empreinte localStorage → AUCUN teasing (le visiteur
  connaît le chat) ; bascule EN → « Got a question? Ask away! » + « Free
  trial » au header. Mobile 390 px — scrollWidth 390 (zéro débordement), rail
  tactile sans bouton, FAB 56 px cliquable 16 px du bord, bulle au-dessus
  complète dans l'écran (analyse VLM 4/4) ; 0 erreur console.
## 2026-09-17 — N°130 — Réactivité P0 « le nuage qui rendait la main trop tard » : sauvegarde PostgreSQL asynchrone, révalidation 304 réactivée, préchargement au survol — l'audit performance passe à l'application

### N°130 — Contexte : la lenteur au clic sur une architecture pourtant découplée
Retour utilisateur : « J'ai constaté une lenteur au niveau du chargement des
pages pour l'architecture découplée backend Go (Render), frontend Next.js
(Vercel) et base de données Neon. Je ne sais pas d'où vient cette lenteur au
clic. Fais un audit en profondeur suivi d'un plan d'action afin de faire de
MikCloud une application qui réagit vite. » L'audit (mesures production +
lecture du code) avait hiérarchisé quatre causes : (A) ~200 ms fixes par
requête sur le plan Render free (proxy + 0,1 vCPU — non traitable sans
changer de plan) ; (B) chaque lecture pollée exécutait un Save() complet
SOUS LE VERROU GLOBAL — re-hash JSON des 31 tables + transaction Neon
bornée à 20 s : le clic attendait la base ; (C) `cache: "no-store"` sur
tous les fetch neutralisait le système ETag/304 du N°75 — chaque poll
re-téléchargeait la payload entière ; (D) aucun prefetch — le premier clic
sur chaque vue payait chunk (100-390 Ko) PUIS requêtes. Ce commit est le
plan P0 de l'audit : B, C et D.

### Produit — le clic ne attend plus la base (backend)
- SAUVEGARDE ASYNCHRONE (mode PostgreSQL uniquement) : Save() devient un
  marquage « sale » (micro-verrou dédié) + réveil non bloquant du syncreur
  de fond — la requête rend la main IMMÉDIATEMENT. Le syncreur coalesce les
  rafales (debounce 500 ms — les marquages d'un même poll se fondent en une
  photographie), plafonne la cadence Neon (une transaction au plus toutes
  les 3 s — la télémétrie Tick rend l'état sale à chaque lecture pollée ;
  sans plafond, le syncreur enchaînerait au rythme des 37 sources de
  polling), photographie l'état sous le verrou global (CloneDeep — quelques
  millisecondes, aucune E/S) puis synchronise HORS verrou : un Neon gelé ne
  fige plus QUE la sauvegarde, jamais les requêtes. Échec = re-marquage +
  backoff 5 s (retry automatique, borné par syncTimeout).
- FLUSH FINAL À L'ARRÊT : SIGTERM Render → Close() arrête le syncreur après
  un dernier flush (borné syncTimeout) — la fenêtre de perte asynchrone
  (≤ ~3 s en cas de crash brutal) ne survit pas à un arrêt propre.
  Idempotent (double Close sans panique).
- ATOMICITÉ DU CACHE D'EMPREINTES (correctif préventif découvert par
  l'audit du retry) : syncTable rafraîchissait les empreintes table par
  table AVANT le Commit — un échec à mi-parcours (rollback) laissait le
  cache croire synchronisées des lignes jamais écrites (perte silencieuse
  préexistante, masquée par le Save-synchrone-à-chaque-requête). Désormais
  les empreintes fraîches vivent dans « pending » et ne remplacent
  p.hashes qu'APRÈS le Commit — un échec retente les VRAIES différences.
  rebuildHashes (boot/Reload) passe sous le même verrou (syncMu) : plus de
  course avec le syncreur.
- MODE JSON (développement, E2E) : Save() synchrone STRICTEMENT inchangé —
  zéro changement de comportement pour les tests et la CI.

### Produit — les payloads ne voyagent plus pour rien (contrat HTTP)
- ETag/304 ÉTENDU aux listes STABLES et pollées : liste paginée des
  utilisateurs (l'une des payloads les plus lourdes de la console —
  l'ETag couvre le corps scopé, une entrée de cache par variante de
  filtres), statistiques de stock vouchers, profils, modèles de voucher,
  revendeurs — rejoignent dashboard/sessions/devices/branding WiFi du
  N°75. Entre deux changements de données, la revalidation renvoie 304
  SANS CORPS (~200 o d'en-têtes au lieu de plusieurs Ko).
- Mesuré en conditions réelles (stack locale, compte vide) : sessions
  (poll 10 s) et dashboard (poll 15 s) → 304 à CHAQUE poll après le
  premier, 0 octet de corps, temps serveur 0-1 ms.

### Produit — le premier clic ne télécharge plus rien (frontend)
- CACHE « NO-CACHE » SUR LES GET : api() et apiAnon() passent les GET sans
  corps en revalidation conditionnelle (stocké + If-None-Match) au lieu de
  « no-store » — le navigateur renvoie l'ETag stocké et reçoit 304 quand
  rien n'a changé. Mutations et GET à corps : « no-store » inchangé.
  (L'ETag étant calculé sur le corps SCOPÉ par compte, deux comptes aux
  données identiques partagent un 304… dont le corps est identique : aucune
  fuite possible.)
- PRÉCHARGEMENT AU SURVOL : survoler (ou focaliser au clavier) un item de
  navigation télécharge EN AVANCE le chunk de la vue (miroir exact des
  dynamic() de app-shell — 27 chargeurs) ET ses requêtes principales via
  queryClient.prefetchQuery (clés STABLES uniquement, strictement
  identiques à celles des vues : un prefetch mal calé doublerait le fetch
  au clic). Idempotent ; le tactile (sans survol) garde le trajet normal.
- FRAÎCHEUR GRADUÉE PAR NATURE DE DONNÉE : STALE_TIME.reference (5 min)
  pour profils/modèles/revendeurs (ne changent qu'à l'écriture),
  operational (30 s) pour le parc routeurs (check-ins ~45 s) — les
  remontées de vue ne re-demandent plus TOUT pour des données de
  référence inchangées. Le défaut (10 s, données vivantes) et les polls
  explicites (refetchInterval) restent maîtres du rythme.

### Technique
- Backend : model/db.go (CloneDeep + clones ciblés Command.Payload/Result,
  RouterTraffic.Interfaces/History, Settings et ses 4 pointeurs,
  NotificationSettings et ses 2 maps — inventaire par réflexion : tout le
  reste est struct par valeur) ; store/store.go (Store + saveMu/dirty/
  canaux, Save() asynchrone, syncLoop/flush, Close à flush final) ;
  store/pg.go (syncMu) ; store/pg_sync.go (pending→commit→swap,
  syncTable à 2 cartes, rebuildHashes sous verrou, commentaires) ;
  api/handlers_users.go + handlers_profiles.go + handlers_templates.go +
  handlers_resellers.go (writeJSONCacheable).
- Frontend : lib/hotspot/api.ts (cache conditionnel GET), lib/hotspot/
  query.tsx (STALE_TIME gradué), lib/hotspot/prefetch.ts (NOUVEAU —
  chargeurs miroir + requêtes stables), app-shell.tsx (onMouseEnter/
  onFocus sur les items de nav), views users/vouchers/wifi/reports
  (staleTime gradué sur les requêtes de référence).
- Vérifié : gofmt vide, go vet OK, go build OK, go test 12 paquets verts
  (dont TestDBCloneDeepIsolation/TestDBCloneDeepEquality — isolation
  complète du snapshot sous mutation concurrente) ; eslint 0, tsgo 0,
  next build OK (13 routes) ; E2E 14/14 verts (25,8 s, stack réelle) ;
  smoke navigateur bout-en-bout : login → console plateforme → survol de
  3 items (prefetch, 0 erreur) → clics fleet/comptes → bascule console
  client → dashboard — LOGS BACKEND EN PREUVE : sessions → 304 (0 s) à
  chaque poll 10 s, dashboard → 304 (1 ms) à chaque poll 15 s ; mobile
  390 px scrollWidth 390 (zéro débordement) ; 0 erreur console.
- Leçon consignée : un verrou global qui porte une E/S n'est pas un
  verrou, c'est une file d'attente — la sauvegarde devait quitter le
  chemin de la requête AVANT que quiconque ne mesure la lenteur.
## 2026-09-17 — N°129 — Le bot range sa boutique : clôture automatique après 15 minutes d'inactivité + purge périodique des conversations

### N°129 — Contexte : retour utilisateur « la console Conversations doit donner la possibilité au bot de clôturer la conversation automatiquement si pas de message du visiteur pendant 15 min ; combien de temps les conversations clôturées restent-elles dans le système, y a-t-il un mécanisme de purge pour éviter la pollution en cas d'affluence ? »
Deux demandes jumelles : un filet de sécurité pour les fils morts, et
une garantie que l'inbox support ne gonfle pas. AVANT : rien ne clôturait
jamais une conversation que le visiteur avait quittée — sous affluence,
l'inbox accumulait des conversations « bot » muettes depuis des heures
et des transmissions « human » jamais clôturées… donc jamais purgées (la
rétention N°127 ne purgeait qu'à la CRÉATION de session, et jamais les
« human »).

### Clôture automatique (15 minutes)
- `chatAutoCloseLocked` — une conversation vivante (`bot` OU `human`)
  sans nouveau message depuis 15 minutes est fermée par l'assistant :
  opération ATOMIQUE sous le verrou du store (aucune race entre le test
  d'inactivité, un message visiteur qui arrive et la clôture), message
  de fin DÉDIÉ dans la langue de la conversation (distinct de la
  clôture support : « Pas de nouveau message depuis 15 minutes — la
  conversation est fermée automatiquement… »).
- Horloge = DERNIER MESSAGE du fil (`updated_at`) : pour une
  conversation « bot » c'est exactement le dernier message du visiteur
  (l'assistant répond dans la même seconde) ; pour une « human », la
  réponse d'un conseiller RELANCE le délai — le visiteur garde un quart
  d'heure pour lire et répondre.
- Deux déclencheurs : balayage de fond `RunChatSweepForever`
  (goroutine main.go, CHAQUE MINUTE, rattrapage au démarrage, filet
  anti-panique N°74 — fichier NOUVEAU `chat_sweep.go`) et la lecture de
  l'inbox console `GET /api/admin/chat/conversations` (le support ouvre
  « Conversations » : les fils morts y apparaissent déjà fermés — la
  console donne littéralement le relais au bot).
- Réouverture : un message du visiteur sur une conversation clôturée
  (support OU automatique) la ROUVRE — `human` si un conseiller était
  déjà intervenu (`chatAgentEverRepliedLocked` : le bot ne reprend
  jamais la main après un humain), sinon `bot` (l'assistant répond à
  nouveau). Corrige au passage un bord silencieux : un message visiteur
  sur un fil fermé par le support se perdait sans badge non-lu.
- Widget vitrine : rien à changer — au statut `closed` il affiche déjà
  la note de clôture et le bouton « Nouvelle conversation ».

### Rétention (réponse à la question « combien de temps ? »)
- Conversations FERMÉES : purgées (avec leurs messages) **30 jours**
  après la clôture ; conversations « bot » inactives : 7 jours ;
  conversations « human » : jamais purgées directement — la clôture
  d'inactivité les fait passer `closed`, donc purge à 30 j ; garde-fou
  mémoire 2 000 conversations.
- `chatPruneLocked` tourne désormais AUSSI au balayage périodique
  (chaque minute, même fichier `chat_sweep.go`) — plus seulement à la
  création de session : la purge est garantie même sans nouveau
  visiteur. Un Save PostgreSQL n'a lieu que si l'état change.

### Console plateforme
- Vue « Conversations » : note de transparence sous l'en-tête (deux
  lignes, icônes Timer/Archive, i18n FR/EN) — la règle d'auto-clôture
  15 min et la rétention 30 j / 7 j sont écrites noir sur blanc pour
  l'équipe support.

### Technique
- Backend : `internal/api/chat_sweep.go` (NOUVEAU — boucle + passage),
  `handlers_chat.go` (const `chatAutoCloseAfter` + `chatAutoCloseLocked`
  + `chatAgentEverRepliedLocked` + réouverture dans `handleChatMessage`
  + déclencheur dans `handleAdminChatConversations` + en-tête
  documenté), `chatbot.go` (const `chatInactiveFr/En` +
  `chatInactiveMessage`), `model/chat.go` (commentaires statuts +
  rétention), `main.go` (goroutine `RunChatSweepForever`).
- AUCUNE nouvelle route, AUCUN changement de schéma (l'horloge
  d'inactivité est `updated_at`, déjà persisté) — les deux
  déclencheurs réutilisent le verrou existant du store.
- Frontend : `platform-chat-view.tsx` (note règles + imports icônes),
  `i18n-fr/platform-chat.ts` + `i18n-en/platform-chat.ts`
  (`platformChat.note.autoClose` / `.retention`).

### Vérifié
- Frontend : eslint 0 erreur, tsc 0 erreur.
- Backend : patches Go byte-précis (tabs préservés, ancres uniques
  assertées, scanner mojibake 0) — compilation et gofmt validés par la
  CI puis par le build Render (sandbox sans toolchain Go).
- Production : widget vitrine + console « Conversations » (note visible)
  + test live de la clôture automatique (conversation laissée inactive
  > 15 min → fermée par le balayage, message de fin visible des deux
  côtés, réouverture par message visiteur).

## 2026-09-17 — N°128 — Le chatbot corrige son accent : chaînes réparées (mojibake), anglais poli et langue qui suit le visiteur

### N°128 — Contexte : retour utilisateur « la fenêtre du chat bot semble présenter des caractères de lettre non conventionnelle et la traduction FR/EN est imparfaite »
Deux maux distincts derrière une même vitre. (1) ENCODAGE — les chaînes
FR/EN du widget (statusBot, statusHuman, statusClosed, placeholder,
humanBtn, handoffNote, closedNote, sendError) avaient été double-encodées
(UTF-8 décodé en latin-1 puis ré-encodé en UTF-8) : « Assistant Â·
rÃ©ponses instantanÃ©es », « Ãcrivez votre messageâ¦ », « Parler Ã  un
humain », « Sending failed â try again » — avec des contrôles C1
invisibles (U+0089, U+00A0…) en prime. Le reste de la vitrine était
propre : seul le bloc `chat` de landing-copy.ts (plus un commentaire de
landing-page.tsx) datait d'une écriture mal encodée. (2) LANGUE — la
langue d'une conversation était figée à l'ouverture : un visiteur qui
bascule la vitrine FR→EN en cours de route voyait l'habillage du widget
passer en anglais… pendant que le bot continuait de répondre en français.

### Réparation de l'encodage
- 12 chaînes réparées par round-trip byte-précis latin-1→UTF-8, ligne par
  ligne, avec ancres uniques et résultats attendus ASSERTÉS (script
  Python à échec bruyant — aucun remplacement approximatif possible) ;
  le commentaire landing-page.tsx datait en outre de « N°125 » : réparé
  et renuméroté N°127.
- Scanner mojibake global rejoué après coup (frontend src/ + backend
  internal/, .ts/.tsx/.css/.json/.go) : 0 ligne suspecte.

### Anglais poli (Gallicismes du N°127)
- Fallback bot : « I don't have a certain answer to that question » →
  « I'm not sure about that one » ; welcome : « I'm the showcase
  assistant » → « I'm the MikCloud assistant » ; réponse vouchers :
  « a captive portal 100% in your brand » → « a fully branded captive
  portal » ; statut du widget : « An advisor is answering you » → « An
  advisor is replying to you ». Le français natif était sain — aucun
  changement FR de contenu.

### La langue suit le visiteur (changement produit)
- CONTRAT : `POST /api/chat/message` et `POST /api/chat/handoff`
  acceptent un champ optionnel `lang` ("fr"/"en") ; le backend aligne la
  conversation dessus AVANT de générer la réponse bot ou le message de
  transmission — le bot répond toujours dans la langue affichée, et
  l'inbox support voit la préférence à jour. Champ absent/invalide =
  aucun changement (rétro-compatible : les clients qui ne l'envoient pas
  conservent le comportement N°127). `POST /api/chat/session` refactoré
  sur le même helper `chatLang` (comportement inchangé, FR par défaut).
- WIDGET : `langRef` (ref synchronisée par effet — callbacks stables,
  pas de re-création au changement de langue) ; chaque POST emporte la
  langue courante de l'interface.

### Vérifié
- eslint 0 erreur, tsc 0 erreur ; scan mojibake 0 (frontend + backend) ;
  navigateur (next dev :3018) : widget FR (« Assistant · réponses
  instantanées », « Échec d'envoi — réessayez ») et EN (« Assistant ·
  instant answers », « Sending failed — try again ») aux accents propres,
  mobile 390 px zéro débordement, 0 erreur console. Le parcours complet
  (session bot + bascule de langue en cours de conversation) est rejoué
  sur production après déploiement — la sandbox n'a pas de toolchain Go
  pour faire tourner le backend en local, et le CORS du backend Render
  n'autorise que les origines légitimes (localhost est refusé, à juste
  titre).

## 2026-09-17 — N°127 — La FAQ devient un CHATBOT : assistant conversationnel sur la vitrine + relève humaine depuis la console plateforme — rail sans numéros, tarifs qui ne recouvrent plus

### N°127 — Contexte : trois défauts de la vitrine et une demande d'assistance vivante
Retour utilisateur : « Supprime la numérotation 01-06 des sections dans le
rail et améliore la section Tarif, les cartes couvrent les textes au-dessus.
Je souhaite aussi supprimer la section Questions fréquentes et créer un chat
bot qui répond à ces questions avec possibilité de transmission de la
conversation à un humain qui répond depuis la super-admin plateforme. »

### Produit — Vitrine épurée
- RAIL SANS NUMÉROTATION : les pastilles 01-06 devant les libellés de
  sections disparaissent (TSX + CSS) — le rail garde ses icônes et son
  détachement de section active, plus lisible au survol.
- TARIFS QUI NE RECOUVRENT PLUS : la formule centrale (scale 1,05) et son
  badge « Le plus choisi » débordaient de leur boîte de layout sur le hint
  du sélecteur et les chips « clientèle cible » — `.mkl-pricing` gagne une
  marge haute de 54 px (25 px de dégagement RÉEL mesuré au point le plus
  haut, le badge) ; taglines à hauteur fixe (2 lignes réservées) pour que
  les MONTANTS des trois cartes restent alignés d'un mode à l'autre ;
  badge d'entrée animé (ressort clay, coupé par prefers-reduced-motion).
- SECTION « QUESTIONS FRÉQUENTES » SUPPRIMÉE (TSX + CSS + copie FR/EN) —
  son contenu vit désormais dans le cerveau du chatbot.

### Produit — Assistant conversationnel (widget de la vitrine)
- BOUTON FLOTTANT CLAY (sarcelle, ombre clay, point d'attention pulsé quand
  une réponse humaine attend) en bas à droite — au-dessus du rail tactile
  mobile (30 px de dégagement) ; panneau clay (ivoire, en-tête sarcelle au
  VRAI logo MikCloud, statut vivant : « Assistant · réponses instantanées /
  Un conseiller vous répond / Conversation clôturée »).
- BOT FAQ BILINGUE (backend) : 13 intents (modes Hotspot/HomeNet, tarifs,
  essai, routeur compatible, protections, arrêt de paiement, moyens de
  paiement, vouchers, revendeurs, pays, salutations, remerciements +
  l'intent spécial « humain » qui déclenche la transmission) — matching par
  mots-clés normalisés (minuscules, accents retirés, ponctuation espacée ;
  mot entier < 5 caractères, sous-chaîne au-delà), meilleur score gagne,
  ordre de la liste tranche les égalités (pricing avant modes, etc.) ;
  réponses 100 % produit réel (catalogue N°122/N°123, protections N°80+,
  mode vente, grâce 30 j), FR/EN par langue de conversation.
- SUGGESTIONS CLAY cliquables (4 questions types en mode bot), message
  d'accueil, fallback honnête (« je n'ai pas de réponse certaine… je vous
  transmets en un clic »), bulles horodatées (visiteur à droite sarcelle,
  bot menthe, SUPPORT crème avec badge).
- TRANSMISSION À UN HUMAIN : bouton « Parler à un humain » + détection
  automatique de l'intent (« je veux parler à un conseiller ») → la
  conversation passe « human », le bot se tait, un bandeau informe le
  visiteur ; polling 4 s quand le panneau est ouvert — la réponse du
  support arrive toute seule.
- ROBUSTESSE : conversation restaurée au rechargement (localStorage
  `mikcloud-chat`, token secret), message en vol en bulle semi-transparente
  (une seule source de vérité : le serveur, contrat append-only par OFFSET
  — jamais par horloge), clôture → bandeau + bouton « Nouvelle
  conversation », erreur d'envoi restituée dans le champ.

### Produit — Inbox « Conversations » (console plateforme, super-admin)
- NOUVELLE VUE (nav plateforme, après « Parc routeurs », slug
  /app/platform-chat) : 4 KPI (conversations, avec un humain, non lus, bot
  actives), filtres Toutes/Humain/Bot/Fermées, liste (statut pastillé,
  langue, dernier message, non-lus ambre, activité relative — max-h +
  scroll, règle maison des longues listes).
- FIL COMPLET : bulles visiteur/assistant/support horodatées, notes de
  contexte (« l'assistant automatique répond — vous pouvez reprendre la
  main » / « clôturée, lecture seule »), réponse par Textarea (Entrée =
  envoyer, Maj+Entrée = saut de ligne), clôture sous AlertDialog (message
  de fin côté visiteur, purge 30 j).
- RÈGLE MÉTIER : répondre à une conversation bot la fait passer « human »
  — après une intervention humaine, le bot ne reprend JAMAIS la main ;
  l'ouverture du fil marque les non-lus comme lus.
- POLLING ADAPTATIF : liste 8 s ; fil ouvert 4 s tant que la conversation
  est vivante — la réponse du visiteur arrive quelques secondes plus tard.

### Technique — Backend (Go, stdlib pur)
- MODÈLE : `ChatConversation` (id, token_hash SHA-256, lang, status
  bot|human|closed, created_ip, created_at, updated_at, unread) +
  `ChatMessage` (id, conversation_id, sender visitor|bot|agent, body, at) ;
  6 points de branchement (DB, BuildEmptyState, ensureSlices, DDL
  chat_conversations/chat_messages + index, specs, load/sync) — même
  recette que password_resets N°68.
- SÉCURITÉ : secret visiteur 40 hex généré serveur, stocké UNIQUEMENT en
  SHA-256 (l'identifiant public ne suffit ni à lire ni à écrire) ; quota
  IP partagé par les POST publics (a.chat : 30/10 min + 400/24 h, même
  limiteur S3, 429 + Retry-After) ; corps bornés (1 000 car. visiteur,
  2 000 agent) ; le hash du secret ne quitte jamais le serveur (struct
  d'affichage dédié) ; les logs serveur ne couvrent que le path (le token
  en query n'y fuit pas).
- RÉTENTION (chatPruneLocked, sous le verrou à la création de session) :
  conversations « closed » > 30 jours et « bot » sans activité > 7 jours
  purgées avec leurs messages ; « human » JAMAIS purgée automatiquement ;
  garde-fou mémoire 2 000 conversations (les bot les plus anciennes d'abord).
- ROUTES publiques (whitelist authMiddleware, préfixe /api/chat/) :
  POST /session {lang} → 201 {id, token, status, total, messages} ;
  POST /message {token, body, offset} → {status, total, messages} ;
  GET /messages?token&offset → {status, total, messages} ;
  POST /handoff {token, offset} → {status, total, messages}.
- ROUTES plateforme (requireRole 3) : GET /api/admin/chat/conversations →
  {conversations, summary} ; GET …/{id} → {conversation, total, messages}
  (+ unread→0) ; POST …/{id}/reply {body} → {ok, message} ; POST …/{id}/close
  → {ok} ; journalisation logActivityBy (compte plateforme) sur réponse et
  clôture.
- CERVEAU (`api/chatbot.go`) : base d'intents déclarative FR/EN, réponses
  uniques en constantes (welcome, handoff, clôture, fallback),
  normalizeChat stdlib pur (mapping de désaccentuation latin — pas de
  dépendance x/text).

### Technique — Frontend
- `components/landing/chat-widget.tsx` (NOUVEAU) : widget client (bouton +
  panneau), polling 4 s, localStorage, pending optimiste, handoff,
  nouvelle conversation, FR/EN via la copie du landing (section `chat` de
  landing-copy.ts, remplace `faq`).
- `landing-clay.css` : + section CHAT WIDGET (préfixe mkl-chat-*, z-index
  45 — au-dessus du rail 40, sous les modales shadcn 50), animation CSS
  pure (le reduced-motion global la coupe), positionnement mobile
  au-dessus du rail tactile.
- Vue plateforme : `views/platform-chat-view.tsx` (NOUVEAU) + branchements
  types/api/view-path/nav/roles/app-shell + fragments i18n
  platform-chat.ts FR/EN (35 clés) + clés nav FR/EN.

Vérifié : gofmt vide, go vet OK, go build OK, go test 12 paquets verts ;
eslint 0 erreur, tsc 0 erreur ; SMOKE backend bout-en-bout 23/23 (session,
welcome, FAQ modes/tarifs/EN, fallback, handoff intentionnel + explicite,
silence du bot en mode human, 404 mauvais token, garde 401 admin, login,
inbox, fil, unread remis à zéro, réponse agent REÇUE côté visiteur,
clôture, nouvelle conversation) ; NAVIGATEUR bout-en-bout sur backend Go
réel + next dev : rail sans numéros (0 .mkl-idx), FAQ absente, FAB présent,
tarifs 25 px de dégagement réel badge/chips (VLM : aucun chevauchement,
badge lisible, montants alignés), chat complet (question → réponse bot →
transmission → réponse admin dans le fil → REÇUE côté visiteur par polling
→ message visiteur vu par l'admin en 20 s → clôture → statut closed côté
visiteur + nouvelle conversation), vue Conversations (KPIs, liste,
filtres, fil, réponse, clôture), mobile 390 px zéro débordement (FAB à
30 px du rail tactile, VLM confirme le détachement net), 0 erreur console.
## 2026-09-17 — N°126 — E2E HomeNet : « le test qui cherchait une maison rebaptisée »

### Contexte
CI rouge sur `main` depuis le push N°125 (run #396) — et en réalité dès le
N°124 (run #395) : le job « E2E Playwright (Mode Vente + HomeNet) » échoue,
le déploiement Render (needs: e2e) saute donc à chaque push. Cause racine :
le N°124 a renommé la carte radio du sélecteur d'usage dans le modal
d'inscription (« Ma maison » → « HomeNet », i18n FR/EN inclus), mais le
test `homenet.spec.ts` cherchait toujours
`getByRole("radio", { name: /Ma maison/ })` — le locator ne matchait plus
rien, timeout de 90 s (×2 avec le retry CI), puis les 4 tests suivants du
groupe `describe.serial` ne s'exécutaient pas (« did not run »).

### Produit
- Aucun changement produit : le libellé « HomeNet » est la décision N°124,
  c'est le TEST qui était resté sur l'ancien nom.

### Technique
- `frontend/e2e/homenet.spec.ts` : locator `/Ma maison/` → `/HomeNet/`
  (l. 181) + titre du test et commentaire d'en-tête alignés sur le
  renommage N°124 ;
- Vérification préalable que tous les autres libellés utilisés par la série
  HomeNet existent toujours dans le source (« Votre maison », « attend son
  routeur », « connectez votre box en mode agent », « Voir mes routeurs »,
  « Appareils en ligne », « Mettre en pause », « Rétablir internet »,
  « Renommer », « Nommer l'appareil », « 30 minutes ») — aucun autre
  décalage ;
- Suite complète rejouée en local contre la stack réelle (backend Go store
  JSON + Next.js build production, ports 4000/3000) : **14/14 verts**
  (setup 1, resellers 3, homenet 5, sell 5 — 25,8 s).

### Leçon
Un renommage UI (N°124) doit emporter ses tests E2E : le smoke manuel du
N°124 vérifiait le modal visuellement, mais le locator du parcours
d'inscription publique n'avait pas suivi — la CI a tourné rouge en silence
pendant deux push, bloquant le déploiement Render du N°125 (backend).

## 2026-09-17 — N°125 — Firmware RouterBOARD : « le bootloader qui attendait son redémarrage »

### Contexte
Retour utilisateur : « tout le parc est à la dernière version 7.24.4.
Cependant j'ai remarqué depuis Winbox que la mise à jour du routeur ne
change pas automatiquement le firmware routeurboard. » Comportement
RouterOS par défaut (le firmware du bootloader ne s'applique qu'au
redémarrage et seulement si `auto-upgrade=yes`, désactivé d'usine) — mais
une lacune pour MikCloud : le N°115/N°117 mettaient le RouterOS à jour en
laissant le firmware en attente, invisible depuis la console.

### Produit
- **CHECK RÉVÉLATEUR** — `routeros_check` lit l'état du firmware
  RouterBOARD (`current-firmware` / `upgrade-firmware` / `auto-upgrade`,
  lectures isolées on-error : un CHR n'expose rien, la ligne ne s'affiche
  pas). Le panneau de vérification montre « Firmware RouteBOARD 7.24.2 →
  7.24.4 (appliqué au redémarrage du routeur) » ;
- **UPDATE AUTO-SYNC** — `routeros_update` pose `auto-upgrade=yes` AVANT
  l'install : le MÊME redémarrage applique RouterOS ET firmware, les
  prochaines mises à jour ne laissent plus le bootloader en retard ;
- **COMMANDE `routerboard_firmware`** — applique le firmware en attente
  sur un parc déjà à jour côté RouterOS : garde côté routeur (rien à
  appliquer → ok `applied=false` SANS redémarrage), sinon auto-upgrade +
  staging + rapport ok AVANT le reboot (pattern F10) ; POST
  `/api/routers/{id}/routerboard-firmware` (rôle 2, dédup CROISÉE avec
  `routeros_update` — jamais deux redémarrages en parallèle) ;
- **PANNEAU VIVANT** — « Application du firmware… » puis « Firmware
  appliqué A → B » quand la fiche vivante voit l'uptime retomber (la
  version RouterOS ne change pas : c'est l'uptime qui prouve le retour) ;
  AlertDialog forte (coupure 2 à 5 min) ; ligne firmware en attente aussi
  dans la vue flotte N°117 ; 18 clés i18n FR/EN.

### Technique
Agent `routerosupdate.go` (check étendu + update auto-sync +
`buildRouterboardFirmware`), model/security.go (kind), api
`handlers_routeros_update.go` (handler + normalisation firmware +
simulated), routes.go, `agent_handlers.go` (journaux lancé/déjà
synchronisé + fraîcheur read_state), `handlers_admin_fleet.go`
(fwCurrent/fwStaged dérivés du dernier check) ; frontend
`ros-update-card.tsx` (FirmwareStatus + FirmwarePanel + mutation + poll
bref), `types.ts`, `platform-fleet-view.tsx`, i18n fr/en. AUCUNE nouvelle
colonne (l'état firmware voyage dans les résultats de commandes).

### Tests
`TestRouterOSCheckScriptShape` étendu, `TestRouterOSUpdateScriptShape`
étendu (auto-upgrade avant install), `TestRouterboardFirmwareScriptShape`,
`TestRouterOSCheckSimulatedFirmware`, `TestRouterboardFirmwareAgentFlow`
(dédup stricte + croisée, rapport brut routeur, journaux, read_state),
`TestRouterboardFirmwareSimulated`, `TestNormalizeRouterOSCheckFirmware`.
Vérifié : gofmt vide, go vet OK, go build OK, go test 12 paquets verts ;
eslint 0, tsgo 0, next build OK (13 routes) ; smoke navigateur
bout-en-bout sur backend Go réel + protocole agent joué (check-in →
read_state 7.24.4 uptime 2w → check « à jour + firmware 7.24.2 → 7.24.4 »
→ dialogue → commande exécutée → read_state uptime 45s → panneau
« Firmware appliqué »), journal complet (demandé/lancé A → B), mobile
390 px zéro débordement, 0 erreur console.

## 2026-09-16 — N°124 — Vitrine : le mode résidentiel devient « HomeNet », le VRAI logo MikCloud (favicon) prend la barre, la clientèle cible du Hotspot s'affiche — polish clay accrocheur

### N°124 — Contexte : nommer le produit, montrer la marque, cibler la clientèle
Retour utilisateur : « Sur le landing page change l'expression Maison par
HomeNet (mise en avant de sécurité internet résidentiel). Le vrai logo
MikCloud comme identique au favicon. Le mode hotspot revendeur, la clientèle
cible, exemple (hôtel, wifi…). Je trouve que le landing est bien mais
d'énorme [marges de progression] — utilise ton expertise afin de créer
quelque chose d'accrocheur d'unique tout en gardant le style UX actuel,
Claymorphisme & Flat Design avec l'identité FreeTech (jaune crème, vert
sarcelle, vert menthe clair et ivoire). »

### Produit — HomeNet nommé, logo réel, clientèle visible
- « MAISON » → « HOMENET » PARTOUT où le mode est nommé (badge hero
  « Pare-feu HomeNet », hint d'essai, marquee « HomeNet · Sécurité internet
  résidentiel », stats « Hotspot & HomeNet », pilule du sélecteur, formules
  « Essai HomeNet / HomeNet Annuel / HomeNet Mensuel », note frais, FAQ
  « Quelle différence entre Hotspot et HomeNet ? ») — le hint du mode et la
  réponse FAQ portent désormais l'expression « sécurité internet résidentiel » ;
  FR/EN symétrique (HomeNet Trial/Yearly/Monthly, residential internet
  security).
- VRAI LOGO (favicon : nuage blanc + routeur rouge + signal Wi-Fi vert sur
  bleu marine) intégré en 4 emplacements : topbar (à côté du wordmark, rebond
  incliné au survol), tête du rail vertical desktop ET barre tactile mobile,
  sculpture du hero (144 px assis sur le nuage clay avec halo ivoire, remplace
  le bouclier), footer. Il remplace l'ancien badge clay à icône Cloud.
- CLIENTÈLE CIBLE DU HOTSPOT : nouveau champ audience — chips clay (menthe et
  crème alternées) « Hôtels · Cybercafés · Maquis · Boutiques · Campus ·
  Restaurants » rendues (1) sous le hint du sélecteur de tarifs en mode
  Hotspot (cascade animée, réduite sans mouvement) et (2) dans la section
  hotspot derrière un label « Pour qui ? » ; le hint du segment cite hôtels
  et campus + « avec vos revendeurs ».
- REVENDEURS MIS EN AVANT : le corps de la section hotspot décrit désormais
  le Mode Vente (PWA protégée par PIN, stock transféré, ventes hors-ligne,
  rapport de journée).
- POLISH CLAY ACCROCHEUR (style conservé) : thumb sarcelle GLISSANT du
  sélecteur de mode (framer-motion layoutId, coupé sous
  prefers-reduced-motion — la classe .is-active assure le même rendu sans
  JS), numérotation 01-06 des sections du rail au survol, hover lift des
  cartes super-pouvoirs et tarifs avec ombre clay qui s'étire, marquee en
  pause au survol, texture pointillée du bandeau stats, lavage menthe très
  doux derrière la FAQ, chips clientèle compactes en mobile (390 px).
- MODAL D'INSCRIPTION : les options deviennent « Hotspot » (Hôtel, cybercafé,
  boutique — vous vendez l'accès internet) et « HomeNet » (Sécurité internet
  résidentiel — protégez le réseau familial et ses appareils), FR/EN.

### Technique — frontend uniquement, zéro contrat API
- landing-copy.ts : champ optionnel `audience?: string[]` sur les segments de
  tarifs + `audience`/`audienceLabel` sur la section hotspot ; commentaires
  mis à jour.
- landing-page.tsx : composant `AudienceChips` (whileInView en cascade,
  reduced-motion = spans statiques), `next/image` /logo.png (priority sur le
  hero), refactor `activeSegment`, thumb `layoutId="mkl-mode-thumb"`.
- landing-clay.css : .mkl-brand-logo, .mkl-hero-logo, .mkl-rail-logo (image),
  .mkl-idx, .mkl-mode-thumb/.mkl-mode-lbl, .mkl-audience/.mkl-audience-chip,
  .mkl-for-who, hovers étendus, marquee pause, texture stats, gradient FAQ,
  responsive chips.
- i18n FR/EN (signup.usage.*) + signup-modal : labels d'usage.
- Vérifié : eslint 0, tsc 0 ; navigateur (dev :3016) — desktop 1440 px
  (logo ×4, badge « PARE-FEU HOMENET », zéro « Maison »), bascule HomeNet
  (formules + hint + disparition des chips), FR↔EN, mobile 390 px zéro
  débordement, modal (options Hotspot/HomeNet), 0 erreur console.

## 2026-09-16 — N°123 — Badges annuels « 2 mois offerts » retirés + essai Hotspot réduit à 60 jours — les clients ACTIFS mis à jour par migration au démarrage

### N°123 — Contexte : l'argument annuel est le prix, pas une promesse de gratuité
Retour utilisateur : « Supprime les (2 mois offerts) sur les cartes annuelles.
Essai Hotspot réduit à 60 jours. Mettre à jour les clients actifs. » Deux
décisions produit : (1) le badge « 2 mois offerts » comptait à la place du
client — l'argument de l'annuel est le PRIX AFFICHÉ (25 000 F/12 000 F =
routeurs illimités), pas une remise relative à décoder ; (2) trois mois
d'essai laissaient un réseau public tourner sans payer — deux mois
suffisent à installer un hotspot et à valider le produit.

### Produit — cartes annuelles épurées, essai 60 jours
- BADGES ANNUELS : « 2 mois offerts » RETIRÉ partout — vitrine (badge
  annuel « Le plus choisi » / « Most popular », sans suffixe), console
  client (les formules annuelles s'affichent SANS badge — seules les
  mensuelles gardent « Sans engagement »), caractéristiques annuelles
  réécrites (« Forfait annuel, un seul paiement » / « Annual flat rate —
  one single payment »).
- ESSAI HOTSPOT : 90 jours → 60 jours à l'inscription publique (HomeNet
  inchangé : 30 jours) ; durée par défaut de prolongation plateforme :
  3 mois → 2 mois Hotspot (Maison 1).
- CRÉATION PLATEFORME : le compte client créé par la plateforme reçoit
  l'essai SEGMENTÉ (60 j Hotspot / 30 j HomeNet) — avant : 3 mois fixes
  quelle que soit l'usage, un foyer créé par la plateforme avait 3 mois.
- VITRINE : hint hero « 60 jours Hotspot · 30 jours Maison », carte
  Découverte « FCFA · 60 jours », note frais, modal d'inscription (60 j en
  mode Hotspot), FR/EN ; console plateforme : hint d'inscriptions ouvertes
  « essai 60 j Hotspot / 30 j Maison », fiche client « Essai — 30 j
  Maison / 60 j Hotspot, 1 routeur ».

### Technique — migration idempotente des clients ACTIFS en essai
- `store.migrateActiveTrialCap` (exécutée au chargement PG/JSON/Reload,
  juste après migrateUsageScopedPlans) : plafonne la fin d'essai des
  comptes « active » en plan « essai » à PeriodStart + durée segmentée
  courante (60 j Hotspot / 30 j HomeNet). Un essai de 90 jours en cours
  passe à 60 jours comptés depuis son DÉBUT ; un essai entamé au-delà de
  la durée cible voit sa fin passer dans le passé (compte « expired » en
  lecture seule, suspension après la grâce de 30 j — la réduction
  s'applique aussi aux essais déjà largement consommés) ; un essai plus
  court reste intact (la migration ne raccourcit que ce qui dépasse,
  n'allonge jamais). Abonnements PAYÉS, essais de comptes désactivés et
  périodes non expirantes (PeriodEnd vide) : NON TOUCHÉS.
- `trialPeriodEnd` : 60 jours Hotspot ; `trialDefaultMonths` : 2 mois
  Hotspot ; création plateforme alignée sur `trialPeriodEnd(now, usage)`.
- Catalogue serveur : `Badge` retiré des formules annuelles (champ vide,
  omis du JSON) — clés i18n `sub.plan.*-annuel.badge` supprimées FR/EN,
  caractéristiques `sub.feat.ill4`/`homeA4` réécrites.
- Tests : `TestSignupTrialSegmented` attend 60 jours (59-61) en Hotspot.
- Docs : CONTRACT-V2 section N°123 (badges, essai, migration).

## 2026-09-16 — N°122 — « Deux modes, un nuage » : tarifs SEGMENTÉS Hotspot / HomeNet sur TOUT le système de paiement (catalogue serveur, demande client, webhooks Wave, prélèvement carte, console, plateforme, vitrine) — la Maison paie deux fois moins cher que le lieu public

### N°122 — Contexte : un prix unique pour deux produits différents
Retour utilisateur : « MikCloud dispose de deux modes liés — Hotspot (gestion
hotspot) et HomeNet (pare-feu cloud, protection internet résidentiel), mais
cela n'a pas été mis en avant sur le landing page, pas de segmentation. Les
prix pour le HomeNet sont : 1 250/mois/routeur et 12 000/an, essai 30 jours.
Les prix Hotspot eux changent pour 2 500/mois/routeur et 25 000/an routeurs
illimités. Mettre à jour tout le système de paiement et le landing page. »
Le catalogue unique (Essentiel 1 250 F/mois/routeur, Illimité 12 000 F/an)
facturait pareil un cybercafé qui MONÉTISE son WiFi et un foyer qui se
PROTÈGE — deux valeurs, deux budgets, deux concurrences.

### Produit — le catalogue segmenté (4 formules, 2 par mode)
- HOTSPOT (réseaux publics payants — cybercafé, maquis, boutique) :
  Mensuel 2 500 F/mois/routeur (sans engagement) ; Annuel 25 000 F/an
  routeurs illimités (2 mois offerts vs mensuel : 25 000 F = 10 mois au
  tarif mensuel).
- HOMENET (pare-feu cloud des foyers) : Mensuel 1 250 F/mois/routeur ;
  Annuel 12 000 F/an routeurs illimités (2 mois offerts — le prix
  historique du produit : la Maison paie deux fois moins cher).
- ESSAI segmenté : 90 jours (3 mois) en Hotspot, 30 jours en HomeNet —
  posé à l'inscription selon l'usage choisi, prolongeable par la plateforme.
- LANDING PAGE : la section tarifs devient « Deux modes, un nuage. » avec un
  SÉLECTEUR CLAY Hotspot/Maison (pilule segmentée, bouton actif enfoncé en
  sarcelle, aria-pressed, focus visible) qui pilote les 3 formules du mode
  (essai, annuel mis en avant ×1,05 avec badge, mensuel) — le changement de
  mode rejoue l'entrée des cartes ; hero « Que vous exploitiez un hotspot
  public ou protégiez votre maison… », hint d'essai « 90 jours Hotspot ·
  30 jours Maison », badge « Gestion Hotspot · Pare-feu Maison · Cloud
  MikroTik », bandeau stats « 2 modes — Hotspot & Maison », item marquee
  « Pare-feu maison HomeNet », question FAQ « Quelle différence entre
  Hotspot et Maison ? », note frais « Essai offert : 90 j Hotspot, 30 j
  Maison » ; bilingue FR/EN intégral ; le modal d'inscription adapte son
  essai au mode coché (30 ou 90 jours).
- CONSOLE CLIENT : la carte Abonnement ne montre QUE les formules du mode du
  compte (serveur filtré) — repérées par PÉRIODE (mois/an), robustes aux
  identifiants ; caractéristiques segmentées (Hotspot : vouchers, quotas,
  revendeurs ; HomeNet : 4 protections, pause dîner, couvre-feu) ;
  comparatif annuel DÉDUIT du catalogue (2 500×12=30 000 vs 25 000 : −17 %,
  −50 %, −80 %, −89 % en hotspot ; 15 000 vs 12 000 : −20 % à −92 % en
  HomeNet) ; « Soit 1 000 F (2 083 F) / mois équivalent » dynamique.
- CONSOLE PLATEFORME : le dialog d'attribution propose les DEUX formules du
  mode DU COMPTE + l'essai (un compte Maison ne se voit plus proposer le
  tarif hotspot) ; durée par défaut qui suit la formule (12 mois annuel,
  1 mois mensuel, essai 1 mois Maison / 3 mois Hotspot) ; mois multiples
  de 12 pour l'annuel ; preview miroir du serveur (mensuelle = prix ×
  slots × mois, annuelle = forfait pro-ratisé).

### Technique — la segmentation est SERVEUR, pas cosmétique
- `model/tenant.go` : SaasPlan gagne `Usage` (hotspot | homenet) ; catalogue
  4 formules ; `PlansForUsage` (les 2 formules du mode), `ResolvePlan(id,
  usage)` (identifiants exacts + HISTORIQUES « essentiel »/« illimite »
  résolus au mode du compte — source unique du pricing), `IsAnnualPlanID` /
  `IsPerRouterPlanID` (legacy compris, défense en profondeur).
- MIGRATION IDEMPOTENTE au chargement (PG + JSON + Reload) :
  `migrateUsageScopedPlans` réécrit les abonnements et demandes de
  facturation historiques vers les identifiants segmentés du mode du compte
  (périodes et LastAmountFcfa conservés — le RENOUVELLEMENT applique le
  nouveau tarif, conforme à la décision prix) ; libellés compat rafraîchis.
  Les prélèvements carte GeniusPaySubs gardent leur ID (résolu à l'usage à
  chaque facture — leur MONTANT souscrit chez GeniusPay reste le tarif de
  création : résilier/re-créer pour aligner, consigné dans CONTRACT-V2).
- `applySubscriptionLocked` : montants pilotés par le catalogue résolu —
  mensuelle = prix × slots × mois ; annuelle = forfait pro-ratisé
  (prix × mois / 12) ; l'identifiant STOCKÉ est NORMALISÉ (une demande
  historique active et stocke « hotspot-mensuel ») ; l'empilement compare
  les identifiants normalisés.
- GARDE DE MODE : POST /api/subscription et POST /api/subscription/stripe
  refusent la formule d'un autre mode (400, code `wrong_mode`) ;
  GET /api/subscription filtre le catalogue par usage et expose `usage` ;
  guards.go plafonne les mensuelles par routeur (essai à part) ; webhook
  Wave, finalizeBillingSuccess et resync carte alignés sur
  IsAnnualPlanID/IsPerRouterPlanID ; essai signup `trialPeriodEnd`
  (30 jours HomeNet / 3 mois Hotspot) + défaut plateforme par usage.
- Frontend : types.ts (SaasPlan.usage, SubscriptionView.usage,
  SubscriptionUpdatePayload segmenté), sa-subscription-card (lookup par
  période, comparatif dynamique), account-detail-dialog (options par usage,
  preview miroir), signup-modal (essai {days} paramétrique), i18n FR/EN
  (sub.*, accounts.sub.plan-*, signup, platform-settings).
- Vérifié : gofmt vide, go vet OK, go build OK, go test 12 paquets VERTS
  (nouveaux : TestResolvePlanSegmented, TestApplySubscriptionUsagePricing —
  montants/normalisation/empilement legacy, TestSignupTrialSegmented — 30 j
  vs 91 j bout-en-bout, TestSubscriptionPostWrongMode, TestMigrate
  UsageScopedPlans — idempotence) ; eslint 0, tsc 0, next build OK 13
  routes ; smoke bout-en-bout sur backend Go réel (:4030, CORS dev) +
  next dev :3016 : catalogue 4 formules, compte Maison → essai 30 j,
  catalogue filtré, demande cross-mode refusée wrong_mode, demande bon
  mode base 1 250/wave 1 425/liste 1 450, activation plateforme HomeNet
  Annuel 12 000 F, legacy « essentiel » sur compte hotspot → stocké
  « hotspot-mensuel », 7 500 F (2 500×3), vue console hotspot 2 500 F,
  essai hotspot 91 jours ; NAVIGATEUR : landing — toggle clay
  Hotspot/Maison (aria-pressed), cartes Découverte 0 F/90 j · Hotspot
  Annuel 25 000 F/an · Hotspot Mensuel 2 500 F/mois/routeur ↔ Essai
  Maison 0 F/30 j · Maison Annuel 12 000 F/an · Maison Mensuel 1 250
  F/mois/routeur, hint dynamique, badge « Le plus choisi · 2 mois
  offerts », FR→EN (« Two modes, one cloud. »), modal signup 30 j quand
  « Ma maison » cochée, mobile 390 px scrollWidth=390 zéro débordement ;
  console Maison — carte Abonnement HomeNet seule, comparatif −20 %/−60 %/
  −84 %/−92 %, dialog souscription 1 250/1 425 Wave/1 450 carte ;
  plateforme — dialog attribution Maison 122 : options HomeNet Mensuel/
  Annuel/Essai, annuel → 12 mois + « Montant : 12 000 », slots masqués ;
  0 erreur console.

---

## 2026-09-16 — N°121 — Pied de page de la vitrine allégé : retrait du copyright MikCloud, de la ligne lieu/humeur et de l'e-mail de contact

### N°121 — Contexte : retour utilisateur immédiat post-N°120
Juste après la reconstruction de la vitrine (N°120), demande explicite de
supprimer trois éléments du bas du footer : « © 2026 MikCloud — Tous droits
réservés. », « Abidjan · Côte d'Ivoire · Fait avec ☁ et beaucoup de
sarcelle. » et « freelancetechnologies.ci@gmail.com ».

### Produit
- Le bas du footer ne garde QUE le crédit FTCI (« © 2026 FTCI — Freelance
  Technologies Côte d'Ivoire », lien vers ftci.fr, composant FtciCredit
  partagé avec les autres surfaces — écran de connexion, console, page
  légale) : la bande se réduit à une seule ligne propre, alignée à gauche.
- Le reste du footer est inchangé : marque + tagline, colonnes
  Produit/Console/Légal (dont le lien Politique de confidentialité).
- Suppression bilingue FR/EN : « © 2026 MikCloud — All rights reserved. »,
  « Made with ☁ and a lot of teal. » et l'e-mail disparaissent aussi en
  anglais — le retrait est symétrique dans les deux langues.

### Technique
- `landing-copy.ts` : les champs `copyright`, `fun`, `contact`, `location`
  sont retirés de l'interface `LandingCopy.footer` ET des objets `fr`/`en`
  (plus aucune référence nulle part — vérifié par recherche globale) ; le
  champ `legal` (libellé du lien /legal/confidentialite, N°70) reste.
- `landing-page.tsx` : le bloc `mkl-foot-bottom` passe de deux groupes
  (copyright + e-mail/lieu) au seul `FtciCredit` ; aucun changement CSS
  nécessaire (`mkl-foot-bottom` en flex space-between se comporte
  naturellement avec un enfant unique).
- Frontend uniquement — aucune route, aucun contrat API, aucune donnée.
- Vérifié : eslint 0 erreur, tsc 0 erreur.

---

## 2026-09-16 — N°120 — « Le cloud qui protège » : reconstruction complète de la vitrine publique en Claymorphisme & Flat Design (palette FreeTech — ivoire, jaune crème, vert sarcelle, menthe) avec rail de navigation vertical qui s'étend au survol

### N°120 — Contexte : la vitrine ne reflétait plus l'étendue du produit
MikCloud a grandi : le N°80-88 a ajouté le module Protection (4 boucliers
pare-feu posés sur le routeur — SafeWiFi, Shield, FamilyGuard, AntiVPN), le
N°115/N°117 la mise à jour RouterOS unitaire et de flotte, le N°27/49/63 le
WiFi jetable, le N°98-101 la console HomeNet. La vitrine d'origine (style
« Aurora Emerald », hero centré, grilles de cartes shadcn) présentait encore
MikCloud comme un simple gestionnaire de hotspot MikroTik. Retour utilisateur :
« MikCloud a évolué et n'est plus seulement un simple outil de gestion Hotspot
mais aussi un pare-feu cloud et outil de protection et sécurité. Le landing
page actuelle ne reflète pas l'étendu de MikCloud et la puissance de l'outil »
— avec demande explicite d'un style Claymorphisme & Flat Design sur la palette
FreeTech (jaune crème, vert sarcelle, vert menthe clair, ivoire), d'un menu
vertical apparaissant au survol, et d'un rendu unique et captivant.

### Produit
- (1) RAIL DE NAVIGATION VERTICAL (desktop ≥ 1024 px) — pilule clay fixe à
  gauche (64 px au repos), qui s'étend à 218 px au SURVOL ou au focus clavier
  (cubic-bezier rebond 0.34/1.56/0.64) en révélant les libellés : Accueil,
  Super-pouvoirs, Protection, Hotspot, Parc routeurs, Tarifs + CTA « Essai
  gratuit » ; la section ACTIVE se détache (fond sarcelle, dot inversé) via
  IntersectionObserver (rootMargin -40 %/-55 %) — le rail suit le défilement.
  Mobile < 1024 px : barre tactile en bas (dots seuls, scrollable, cibles
  44 px, respecte env(safe-area-inset-bottom)).
- (2) HERO — « Votre WiFi, blindé par le cloud. » : mot « blindé » en
  sarcelle surligné de jaune crème (em::after derrière le texte), eyebrow
  pulsé « Hotspot · Protection cloud · Parc MikroTik », et SCULPTURE CLOUD
  CLAY animée à droite : corps de nuage clay (double inset + ombre dure),
  bouclier sarcelle qui flotte (bob 4,5 s), 4 cercles clay, 2 anneaux
  pointillés en rotation inverse, 3 chips flottantes (4/4 protections
  actives · 500 vouchers par lot · Agent check-in 45 s).
- (3) MARQUEE incliné (-1,2°) sarcelle : les 12 vraies capacités produit
  (Vouchers, Filtrage DNS Quad9, Anti-piratage, Couvre-feu, Bloque-VPN,
  QoS, Mode Vente, Mise à jour RouterOS, Portail captif, WiFi jetable QR,
  Notifications Telegram, Paiement Wave) en défilement infini 26 s.
- (4) SUPER-POUVOIRS — 3 cartes clay (crème « Gestion Hotspot » / sarcelle
  « Protection Cloud » / menthe « Pilotage du parc ») avec tags (Fondation /
  4 boucliers / Parc), icônes lucide sur tuiles clay, listes à coches des
  capacités réelles ; hover : élévation -10 px avec légère rotation -0,6°.
- (5) SECTION PROTECTION — split texte + PANEL SOMBRE « Centre de
  protection » : anneau SVG animé 4/4 (stroke-dashoffset), verdict « Bien
  protégé », 3 stat-mini (4 protections · 6 h auto-réparation · 45 s
  check-in), et FLUX DE SUPERVISION vivant — les 4 protections
  s'illuminent à tour de rôle (2,4 s) avec leur état (FamilyGuard affiche
  sa fenêtre 22:00 → 06:00) + ligne « Auto-réparation cloud · vérifié il y
  a 2 min » ; les 4 feats décrivent les protections réelles (SafeWiFi
  Quad9/AdGuard + DoH fermé, Shield ports admin/SMB, FamilyGuard couvre-feu,
  AntiVPN WhatsApp préservé).
- (6) SECTIONS HOTSPOT & PARC — 2 splits alternés : bar chart clay
  « Affluence horaire » (8 barres animées à l'entrée en vue) et panel
  « Parc routeurs » (3 lignes routeurs avec version → état, boutons
  « Vérifier tout le parc » / « Mettre à jour le parc » illustrant le
  N°117) ; les feats couvrent portail à votre marque (modes commercial
  Wave / hospitalité), vouchers 1-10 appareils, stats live, mise à jour
  RouterOS sans Winbox, télémétrie, alertes Telegram/WhatsApp.
- (7) BANDEAU STATS sarcelle — compteurs animés (ease-out cubique au
  rAF) sur des constantes PRODUIT vérifiables : 500 vouchers par lot,
  4 boucliers pare-feu, 54 pays africains visés, 90 jours d'essai —
  formatage fr-FR/en-US selon la langue.
- (8) TARIFS — 3 cartes clay fidèles au catalogue serveur (model/tenant.go) :
  Découverte 0 FCFA · 90 jours, ILLIMITÉ 12 000 FCFA/an (carte sarcelle
  centrale agrandie ×1,05, badge « Le plus choisi · 2 mois offerts »),
  Essentiel 1 250 FCFA/mois/routeur ; note frais répercutés (carte +6 %,
  Wave −3 %) ; tous les CTA ouvrent l'inscription.
- (9) FAQ clay (details/summary natifs, plus « + » qui pivote en ×), CTA
  final crème avec mini-nuages flottants, footer 4 colonnes (Produit /
  Console / Légal avec /legal/confidentialite, crédit FTCI, contact).
- (10) BILINGUE — toggle FR/EN conservé (store zustand) : intégralité de
  la copie traduite, compteurs localisés.

### Technique
- Frontend uniquement : landing-page.tsx (RECONSTRUIT — 670 → ~640 lignes),
  landing-copy.ts (RECONSTRUIT — nouvelle interface sectionnée rail/hero/
  marquee/powers/protection/hotspot/fleet/stats/pricing/faq/finalCta/footer,
  FR + EN), landing-clay.css (NOUVEAU ~900 lignes — design system clay
  isolé : tout préfixé mkl-/--mkl-*, zéro collision avec le thème shadcn
  « Aurora Emerald » de la console ; z-index rail 40 VOLONTAIREMENT sous
  les modales shadcn z-50 — la SignupModal passe devant), layout.tsx
  (metadata : title/description/keywords « protection cloud & pilotage
  MikroTik »). page.tsx et signup-modal.tsx INCHANGÉS (props onSignIn/
  onSignUp conservées).
- Polices : Fraunces (titres, variable opsz) + Manrope (texte) via
  next/font — self-hostées, zéro requête externe bloquante.
- Animations : framer-motion (Reveal/whileInView, respect
  useReducedMotion) + CSS keyframes (bob/floaty/spin/pulse/marquee) ;
  prefers-reduced-motion coupe tout + masque le surlignage em.
- Accessibilité : rail aria-label + aria-current, focus-visible 3 px
  sarcelle partout, cibles tactiles 44 px, aria-hidden sur les décors,
  scroll-margin-top 90 px pour les ancres sous topbar sticky, contrastes
  AA (encre #123B3A sur ivoire #FDF9EE ≈ 12:1 ; ivoire sur sarcelle
  #0E7C7B ≈ 4,7:1).
- HONNÊTETÉ MARKETING : aucun compteur de « menaces bloquées » (n'existe
  pas dans le produit) — le « flux de supervision » anime les 4 protections
  RÉELLES avec leurs états ; les stats du bandeau sont des constantes
  produit ; les lignes du panel parc sont des illustrations génériques
  (noms d'établissements fictifs, pas de clients réels).
- Vérifié : eslint 0 erreur ; tsgo 0 erreur ; next build OK (13 routes
  inchangées) ; navigateur bout-en-bout (next dev :3016) — desktop
  1440×900 : rail 64 px → 218 px au survol (labels révélés, CTA lisible),
  section active « Tarifs » correcte après scroll, compteurs finaux
  500/4/54/90, cartes tarifs (ivoire/sarcelle ×1,05/crème) alignées +
  badge « Le plus choisi », FAQ ouvrable (body 93 px), surlignage crème
  #F5E3A8 vérifié en style calculé, toggle FR→EN (h1 « Your WiFi,
  shielded by the cloud. ») ; modale Signup au-DESSUS du rail
  (z-index 50 > 40, visible) ; mobile 390×844 : zéro débordement
  horizontal (scrollWidth = 390), rail mobile en bas (display flex,
  desktop none), boutons pleine largeur ; 0 erreur console/page.

## 2026-09-15 — N°119 — « la simulation qui mangeait le stock du revendeur » : le moteur de démo (Tick, ~30 % par lecture console) connectait un voucher actif ALÉATOIRE de N'IMPORTE QUEL compte et le marquait « used » — amputant le stock de vente d'un compte tiers ; le stock confié devient INTOUCHABLE (même garde que N°26/W1)

### N°119 — Contexte : l'échec E2E du N°118 était un lancé de dé
La CI du N°118 (frontend pur — libellés/flèches/QoS) échoue sur UN test :
sell.spec.ts « pagination du stock — "Afficher plus" complète » — attendu
« 60 sur 7x » (72 tickets), obtenu « 60 sur 69 », idem au retry. La trace
Playwright (corps de réponses réseau extraits) dit tout : à 15:34:06,
/api/sell/me ET /api/sell/stock répondent stockCount=69 / total=69, 100 %
du lot B20260915-4661 (70) — ZÉRO du petit lot (3) — alors que le crédit
(35 400 = 50 000 − 14 000 − 600) prouve que les DEUX transferts ont bien
débité : les 3 tickets manquants ONT été transférés puis ont DISPARU du
stock vendable. Reconstitution : le moteur de simulation (store.Tick —
déclenché par CHAQUE lecture console, TOUS comptes : « fait vivre la
simulation (tous comptes) », handlers_sessions.go) crée à ~30 % par tick
(≥ 2 s) une session sur un voucher actif ALÉATOIRE d'un routeur simulé,
PEU IMPORTE SON COMPTE, et le marque Status="used" + UsedAt (→ exclu du
stock vente par EffectiveStatus != "active") en incrémentant au passage
VouchersSold/Revenue du revendeur (vente fantôme). La fenêtre bootstrap →
tests sell (~14 s, lectures console des specs resellers + homenet sur
D'AUTRES comptes) laisse ~7 ticks × 30 % tomber sur les 72 tickets du
compte bootstrap (seuls candidats massifs) : 3 « connectés » → 69. Le
commentaire du bootstrap (« le stock resterait stable ») documentait
l'INTENTION — la simulation la violait.

### Produit
- (1) STOCK CONFIÉ INTOUCHABLE — la boucle des candidats à une session
  démo saute désormais tout voucher `ResellerID != ""` : un ticket remis
  à un revendeur attend sa VENTE (tactile/papier), pas une connexion de
  démo ; le moteur ne peut plus ni l'exclure du stock (« used ») ni
  fabriquer des ventes fantômes (VouchersSold/Revenue). Même principe
  que la garde N°26/W1 de sweepDeadBatches (« AUCUN ticket revendeur :
  le stock confié reste la trace de ce qui a été remis »).
- (2) La démo reste vivante — les vouchers DIRECTS d'un routeur simulé
  continuent d'être connectés au hasard ( rôle du moteur) ; seules les
  sessions sur stock confié étaient un bug.
- (3) Périmètre réel : comptes à routeurs SIMULÉS uniquement (démo/E2E) ;
  les routeurs agents/réels n'ont jamais été candidats (garde existante).

### Technique
- backend/internal/store/store.go — la collecte des candidats (Tick →
  « nouvelle session ~30 % ») saute les vouchers à ResellerID non vide,
  commentaire d'autopsie citant l'incident E2E N°118.
- Test : TestTickNeverConsumesResellerStock (store_test.go) — 300 ticks
  espacés de 3 s sur un store seedé (routeur simulé + revendeur + 1
  ticket confié + 1 direct) ; INVARIANT : le ticket confié reste
  « active », UsedAt vide, AUCUNE session à son nom, ZÉRO VouchersSold
  pour le revendeur. Déterministe : avant correctif, P(y échapper) =
  (0,7)^300 ≈ 10⁻⁴⁶ — vérifié ÉCHEC sans le patch (status="used" au
  premier tirage), PASS avec.

### Vérifications
- gofmt vide, go vet OK, go build OK, go test 12 paquets verts (api 35,4 s).
- Suite E2E COMPLÈTE en local mode CI (retries actifs) : 14/14 passés —
  dont le test de pagination restauré (lot de 72 stable de bout en bout).

---

## 2026-09-15 — N°118 — Convention débit « le libellé qui inversait le sens » : le Studio Forfait annonçait (descendant/montant) alors que RouterOS lit montant/descendant — le bridage du re-test N°116 s'est donc appliqué INVERSÉ (down 512k/up 1M au lieu de down 1M/up 512k) ; libellés, flèches d'aperçu et ordre des champs QoS unifiés sur l'ordre RouterOS

### N°118 — Contexte : re-test N°116 réussi… avec le sens inversé
Le re-test terrain du correctif v3 était une RÉUSSITE complète (voucher
`7388` sur CYBER S.C : file `mikthrottle-7388` en position 0, download
plombé à 512,9 kbps live, dynamique gelée à 53,7 Mo — le bridage quota
fonctionne, prouvé par télémétrie). Mais le retour utilisateur révèle
l'inversion : « dans le formulaire de création de profil, au niveau du
bridage, le download est placé avant l'upload, d'où 1M/512 dans le test —
pareil pour la QoS, plafond agrégat ». RouterOS lit `max-limit`/`rate-limit`
en **montant/descendant** (preuve terrain : `1M/512k` plombait le download
à 512,9 kbps — la 2ᵉ valeur est le descendant) ; le libellé du Studio
Forfait annonçait « Limite de débit **(descendant/montant)** » — l'inverse
exact. L'opérateur tape le 1M (download voulu) en premier, la chaîne part
verbatim au routeur, le routeur l'applique comme MONTANT. Aucun bug de
données : un bug de COMMUNICATION du sens, aggravé par `formatRateLimit`
qui décorait la 1ʳᵉ valeur d'une flèche ↓ (aperçu live du wizard, liste
des profils, presets bridage) et par la carte QoS qui affichait « Plafond
descendant » avant « Plafond montant » — alors que la table des files,
elle, affichait déjà « Plafond (montant/descendant) ».

### Produit
- (1) FORMATTER — `formatRateLimit` : `1M/10M` → « 1M ↑ / 10M ↓ »
  (montant d'abord, la vérité RouterOS) — l'aperçu live du wizard, la
  liste des profils et les presets de bridage ne confirment PLUS le
  mauvais sens.
- (2) STUDIO FORFAIT — libellé « Limite de débit **(montant/descendant)** »,
  hint et toasts explicites (FR : « Format RouterOS montant/descendant,
  ex : 512k/1M » / EN : « RouterOS format (up/down) »), « Débit de
  bridage **(montant/descendant)** » ; la chaîne saisie reste poussée
  VERBATIM au routeur (WYSIWYG Winbox : ce que l'opérateur tape dans
  MikCloud est ce qu'il voit dans ses files).
- (3) CARTE QoS — « Plafond montant » AVANT « Plafond descendant »
  (saisie), forfait FAI déclaré réordonné « montant / descendant »
  (saisie, affichage lecture, exemple 20/110), recommandation affichée
  ↑ d'abord ; les VALEURS étaient déjà correctes (champs séparés,
  backend `maxUp/maxDown` dans le bon ordre depuis N°104) — seul
  l'ordre de présentation prête à confusion.
- (4) CONVENTION UNIQUE — une paire de débit se lit et se saisit dans
  l'ordre RouterOS **montant/descendant** partout dans la console
  (Studio Forfait, QoS, table des files) ; les affichages de télémétrie
  pure (débit live, capacité observée, historique) gardent leurs icônes
  ↓/↑ explicites par valeur.

### Rattrapage de la donnée existante (geste ponctuel, pas une migration)
- Profil `Test` (seul profil throttle du parc) : `throttle_rate`
  `1M/512k` (saisi avec l'ancien libellé, intention down 1M / up 512k)
  → corrigé en base vers `512k/1M`. Les `rate_limit` existants
  (1M/10M, 512k/6M…) étaient déjà tapés RouterOS-style : inchangés.
- Un profil édité doit être re-sauvegardé pour re-pousser son
  `profile_set` ; le marqueur `mikq:` des vouchers EXISTANTS conserve
  le débit de leur création (les nouveaux lots lisent le profil corrigé).

### Technique
- frontend : `lib/hotspot/format.ts` (formatRateLimit inversé + doc),
  `i18n-fr/profiles.ts` + `i18n-en/profiles.ts` (rate, rateInvalid,
  rateHint, rateToast, throttleRate, throttleRateToast), `i18n-fr/tools.ts`
  + `i18n-en/tools.ts` (declaredHint, declaredNone), `router-tools/qos-tab.tsx`
  (ordre des champs montant→descendant : saisie QoS, forfait FAI déclaré,
  recommandation, affichage lecture du forfait).
- AUCUN changement backend (les chaînes débit transitent verbatim ;
  la QoS envoie déjà maxUpBps/maxDownBps séparés) ; docs :
  CONTRACT-V2 §N°106 sous-section N°118.

### Vérifications
- eslint 0 erreur, tsgo 0 erreur, next build OK (13 routes inchangées).
- Navigateur bout-en-bout (backend Go réel :4000 + next dev :3016) :
  wizard — libellé « (montant/descendant) », aperçu live « 512k ↑ / 1M ↓ »,
  bridage « 256k/512k » accepté, payload POST `rateLimit=512k/1M`
  `throttleRate=256k/512k` verbatim, liste « 512k ↑ / 1M ↓ » ;
  QoS — « Plafond montant » avant « Plafond descendant », forfait déclaré
  20/110 (montant/descendant), recommandation ↑ 19 Mbps puis ↓ 104,5 Mbps,
  file posée `maxLimit=19000000/104500000` (montant d'abord — cohérence
  saisie → recommandation → file), mobile 390 px, 0 erreur console.

---

## 2026-09-16 — N°117 — « Mise à jour de flotte » : le super-admin vérifie et met à jour le parc RouterOS de TOUS les clients MikCloud depuis la console plateforme — nouvelle vue « Parc routeurs » (chaque routeur de chaque compte, version installée → disponible, état), « Vérifier tout le parc » en lecture seule, « Mettre à jour le parc » qui ne cible QUE le retard détecté (jamais à l'aveugle : un update redémarre le routeur et coupe le hotspot du client)

### N°117 — Contexte : le super-admin voulait le geste de flotte
Retour utilisateur : « ajouter une fonctionnalité pour le super admin afin
que celui-ci, depuis la console, lance une mise à jour du parc de routeurs
de tous les clients MikCloud ». Le N°115 a donné le geste unitaire au
gérant ; le propriétaire du SaaS veut piloter la FLOTTE — savoir où le
firmware prend retard, et dérouler la mise à jour sans ouvrir chaque
console client.

### Produit
- (1) VUE « PARC ROUTEURS » (console plateforme, nav après « Vue
  d'ensemble ») : chaque routeur de chaque compte client — compte, nom,
  badges mode (agent/simulé/API directe) et ligne, version installée →
  version disponible (mono), badge d'état RouterOS (à jour / mise à jour
  disponible / vérification… / installation… / erreur / inconnu / jamais
  vérifié) + dernière vérification en temps relatif, actions par routeur
  (vérifier / mettre à jour). 4 KPI de synthèse (total, en ligne, mises à
  jour disponibles en ambre, installations en cours) ; liste
  `max-h-[32rem] overflow-y-auto` (règle maison des longues listes) ; poll
  10 s pendant les vols sinon 30 s.
- (2) « VÉRIFIER TOUT LE PARC » — lecture seule, sans risque :
  `POST /api/admin/fleet/routeros-check` enfile un `routeros_check` sur
  chaque routeur agent (dédup par routeur — un check en vol n'est pas
  re-enfilé) ; les réponses arrivent au rythme des check-ins (≤ 45 s par
  routeur en ligne). Simulés : état calculé à la volée au GET (rien à
  enfiler). API directe : non supporté (matrice §0).
- (3) « METTRE À JOUR LE PARC » — JAMAIS À L'AVEUGLE : un update RouterOS
  REDÉMARRE le routeur et coupe le hotspot du client ; la cible par défaut
  est « tous les routeurs avec une mise à jour DÉTECTÉE » (état available
  du dernier check abouti), pas « tous les routeurs ». La barrière
  s'applique AUSSI en ciblage explicite (un routeur à jour ou jamais
  vérifié n'est jamais re-redémarré pour rien — le serveur ne fait pas
  confiance au front). Confirmation forte : nombre EXACT de routeurs,
  nombre de comptes touchés, coupure WiFi 2 à 5 min par routeur, jamais
  deux fois le même (dédup stricte N°115 inchangée). Agents : commande
  `routeros_update` avec la cible du dernier check ; simulés : application
  immédiate (miroir N°115 — version, uptime zéro, sessions coupées et
  journalisées logout).
- (4) ZÉRO NOUVEAU SCHÉMA — l'état de flotte dérive de l'existant :
  `fleetRouterOSStateOf` lit la dernière commande `routeros_check` aboutie
  (état normalisé N°115 dans son Result), les commandes en vol
  (checking/updating), `Router.Version` (read_state) pour l'installée. La
  version finale revient d'elle-même au premier read_state
  post-redémarrage (mécanique N°115 inchangée, journal comprise).
- (5) JOURNAL — une entrée par COMPTE, pas par routeur : un geste de
  flotte ne noie pas le journal (« Mise à jour RouterOS de flotte lancée
  par la plateforme : «A», «B», «C» + N autres en file d'installation… » —
  noms bornés à 3, acteur = le super-admin). Les commandes sont enfilées
  sous le COMPTE CLIENT du routeur : le gérant concerné voit l'opération
  dans SON journal, le rapport agent remonte par le chemin standard N°115.

### Sécurité
Routes super-admin uniquement (`requireRole(3)` + `isPlatformAdmin`, pattern
handleAdminOverview). `latest` validé `^[0-9][0-9A-Za-z.\-]{0,31}$` (repli
du corps, défense en profondeur — la cible réelle vient du check du routeur).
Chaînes libres rapportées bornées (status 160, versions 32).

### Technique
Backend : `api/handlers_admin_fleet.go` (NOUVEAU — GET fleet/routers +
POST fleet/routeros-check + POST fleet/routeros-update +
fleetRouterOSStateOf + boundedString + fleetResolveTargets +
fleetNamesLabel/fleetCountLabel), `api/routes.go` (3 routes rôle 3).
Frontend : `views/platform-fleet-view.tsx` (NOUVEAU), `types.ts` (ViewId
`platformFleet` + FleetRouter/FleetOverview/FleetActionResponse), `roles.ts`
(PLATFORM_VIEWS), `view-path.ts` (slug `platform-fleet`), `nav.ts`
(NAV_PLATFORM_SECTIONS), `app-shell.tsx` (dynamic import + vue + titre),
`api.ts` (fetchFleetRouters/fleetRouterOSCheck/fleetRouterOSUpdate), i18n
fr/en `platform.fleet.*` + `nav.platformFleet`. Docs : CONTRACT-V2 §N°116,
CHANGELOG.

### Vérifié
gofmt vide, go vet OK, go build OK, go test api (33,7 s) + agent verts ;
eslint 0 erreur, tsc 0 erreur.

---

## 2026-09-15 — N°116 — Correctif bridage v3 « le tick armé qui restait silencieux » : la décision ne se fiait qu'aux compteurs UTILISATEUR jamais prouvés sur RouterOS 7.x — 219 Mo sans bridage un tick v2 parfaitement déployé ; la v3 décide sur le MAX des compteurs utilisateur ET des octets de session (source prouvée), + sonde qcounters télémétrique

### N°116 — Contexte : re-test terrain du N°106/N°113, constat identique
Routeur cybere-space sc (RouterOS 7.24.1), voucher `4327` créé PAR MikCloud
(15 min / 50 Mo / bridage 1M/512k) : 219 Mo consommés, le débit ne tombe
jamais à ~512 kbps. Autopsie AVEC les données de production (Neon) :
le scheduler `mikcloud-quota` v2 est déployé et confirmé (`QuotaSchedVer=2`,
on-event relu mot à mot depuis un rapport `read_scheduler`), le marqueur
`mikq:52428800,1M/512k` est présent (payload du voucher_batch vérifié),
le on-login combiné a été livré — et `throttle=""` dans CHAQUE read_state
de la fenêtre de bridage : la file n'a JAMAIS existé, pas même mal placée.
L'hypothèse fasttrack est ÉLIMINÉE par les compteurs (la file dynamique
`<hotspot-4327>` comptait exactement les octets de la session). Reste UNE
opération du script jamais prouvée sur ce routeur : la lecture des
compteurs UTILISATEUR (`/ip hotspot user get <id> bytes-in/out`) — source
EXCLUSIVE de la décision v2. Illisibles ou figés sur RouterOS 7.x, chaque
lecture protégée retombe à 0 → `0 >= quota` FAUX à chaque tick → branche
retrait → rien, SILENCIEUSEMENT.

### Produit
- (1) DÉCISION v3 — MAX DES DEUX SOURCES, JAMAIS LEUR SOMME :
  `quotaApplyLines` (cœur partagé du tick et du on-login) consolide le
  cumul retenu = max(compteurs cumulés UTILISATEUR protégés on-error,
  octets de la PLUS GROSSE session ACTIVE de l'utilisateur via
  `/ip hotspot active` — source prouvée fiable : le read_state la rapporte
  toutes les ~2 min). Pourquoi max : compteurs live → ils incluent déjà la
  session (somme = double comptage, bridage trop tôt) ; figés au logout →
  sous-estimation bornée à un reste de quota ; absents → le max retombe
  EXACTEMENT sur la session : le bridage en cours de session fonctionne,
  seule la fenêtre de re-login peut se rouvrir (dégradation documentée).
- (2) SONDE `qcounters` (ok|err|na) : le chunk final du read_state tente
  la lecture d'un compteur utilisateur (typeof non-nil, protégé on-error)
  et rapporte le résultat — le champ atterrit dans le résultat de la
  commande (queryable à distance dans l'historique) : le mode réel du parc
  est MESURABLE sans Winbox. La décision v3 ne dépend PAS de la sonde
  (dégradation gracieuse).
- (3) CONVERGENCE : `QuotaTickVersion = 3` — le payload de `quota_ensure`
  porte la génération, `ensureQuotaThrottleLocked` re-file tout routeur
  dont `QuotaSchedVer < 3` : le parc re-converge au premier check-in après
  déploiement, SANS geste opérateur (pattern N°113).

### Technique
- `backend/internal/agent/quota.go` : `quotaApplyLines` v3 (`$eff` consolidé,
  repli session `foreach sa in=[/ip hotspot active find where user=$qu]`,
  mise à jour MAX `:if ($sse > $eff)`), `QuotaTickVersion = 3`, en-tête et
  limites documentés (fenêtre de re-login en mode compteurs absents ;
  marqueur mikq: réservé aux créations MikCloud — Winbox/Mikhmon exclus).
- `backend/internal/agent/readstate.go` : sonde `qcounters` dans le chunk
  final (une seule occurrence — les chunks intermédiaires ne changent pas).
- Tests : `TestQuotaDecisionSourceV3` (init depuis compteurs utilisateur,
  repli session sur `$qu`, MAX pas somme, décision sur `$eff`, GARDE
  ANTI-RÉGRESSION : la comparaison directe v2 `($bi + $bo) >=` interdite) ;
  `TestReadStateQuotaTelemetry` (sonde présente une fois, typeof non-nil) ;
  `TestQuotaScriptsShape` étendu aux tokens v3 ; `TestBuildQuotaEnsure`
  vérifie `\$eff` échappé dans le on-event servi. Suite complète : gofmt
  vide, go vet OK, go build OK, go test 12 paquets verts.
- Docs : CONTRACT-V2 §N°106 enrichi de la sous-section N°116 (autopsie
  complète, décision v3, sonde, limites).

---

## 2026-09-15 — N°115 — « Update RouterOS sans Winbox » : le gérant vérifie et installe la dernière version RouterOS de son parc DEPUIS MikCloud — la vérification interroge les serveurs MikroTik DEPUIS le routeur (le canal du routeur fait foi), l'installation télécharge, installe puis REDÉMARRE, et la version finale revient d'elle-même à la première télémétrie post-redémarrage

### N°115 — Contexte : la maintenance firmware, dernier geste qui exigeait Winbox
Retour utilisateur : « je souhaite donner la possibilité à mes clients de
mettre à jour leurs routeurs vers la dernière version RouterOS depuis
MikCloud ». Le gérant vit dans sa console (état du parc, QoS, pool,
scheduler, reboot) mais le firmware restait le dernier rituel Winbox —
téléchargement manuel, câble, fenêtre de maintenance. Parité Mikhmon
« Update RouterOS » : le geste devient deux boutons, avec la vérité du
CANAL du routeur (stable par défaut — pas une version codée en dur côté
cloud qui mentirait sur les canaux beta/long-term) et un avertissement
honnête sur la coupure.

### Produit
- (1) VÉRIFICATION (`POST /api/routers/{id}/routeros-check`) : commande
  `routeros_check` servie à l'agent — `/system package update
  check-for-updates` puis lecture `status`/`latest-version`/
  `installed-version`/`channel`, chaque lecture isolée dans son `:do
  on-error` (un champ absent sur un build exotique ne tue pas la commande) ;
  le status RouterOS passe par « Checking… » le temps que MikroTik
  réponde : le script ROUTEUR boucle (2 s × ≤ 15 — garde `[:typeof] =
  "num"` du find-qui-ne-trouve-pas), le FRONT poll la commande (2 s ×
  90 s max, pattern ping F8). Dédup : une vérification à la fois (le
  second clic récupère la commande en cours). Normalisation cloud
  (`normalizeRouterOSCheck`) : états `latest`/`available`/`error`/
  `unknown` dérivés du status BRUT, REPLI sur la comparaison
  installed != latest (un libellé inconnu ne masque pas une mise à jour
  évidente), status brut préservé et affiché honnêtement (borné 160) ;
  la clé `rosStatus` transporte le status RouterOS (la clé `status`
  reste celle du protocole ok/error du rapport).
- (2) INSTALLATION (`POST /api/routers/{id}/routeros-update` `{latest?}`,
  corps optionnel — `decodeBodyTolerant`) : commande `routeros_update` —
  rapport ok AVANT l'exécution (pattern reboot F10 : le téléchargement
  puis le redémarrage coupent le routeur, le fetch bloquant termine
  premier), `/system package update install` dans un `:do on-error` qui
  rapporte l'échec de téléchargement APRÈS coup (le routeur ne redémarre
  pas dans ce cas). Dédup STRICTE : jamais deux installations en parallèle
  (second clic → commande en vol + `already:true`). Simulated : application
  immédiate (version, uptime à zéro, sessions coupées — miroir reboot F10).
- (3) CONFIRMATION SANS MÉCANIQUE DÉDIÉE : aucune nouvelle colonne — la
  version finale revient au `read_state` de fraîcheur re-enfilé après le
  rapport ok ; `applyReadState` trace le changement (journal « RouterOS de
  «X» mis à jour : A → B » — couvre aussi une mise à jour manuelle
  Winbox), le lancement est journalisé au rapport de la commande. Le
  check ne journalise RIEN (lecture d'outil — bruit) et n'enfile aucun
  read_state. `routeros_check` rejoint `staleSentReadKinds`
  (idempotent) ; `routeros_update` NON (une écriture muette n'est jamais
  rejouée — un redémarrage peut être en cours).
- (4) FRONT — carte « Mise à jour RouterOS » (onglet Système, sous les
  infos) : version installée + « Vérifier les mises à jour » → panneau
  d'état (vert/ambre/rouge/neutre — disponible affiche
  `installé → dispo [canal]` + status brut + bouton « Mettre à jour vers
  X ») → AlertDialog d'avertissement FORT (coupure totale 2 à 5 min,
  sessions coupées, note agent ≤ 45 s) → panneau d'installation SANS
  état dérivé : la fiche « vivante » (poll 15 s) pilote la bascule
  « Installation en cours… » → « Mise à jour installée A → B » — une
  base de version INCONNUE ne conclut jamais sur une version ≠ cible
  (l'arrivée d'une version périmée pendant le téléchargement ne fait pas
  passer le panneau pour terminé), et une vérification fraîche remplace
  le panneau (le check porte la vérité du serveur). 27 clés i18n
  `tools.ros.*` FR/EN.
- Sécurité : `latest` validé `^[0-9][0-9A-Za-z.\-]{0,31}$` ET assaini au
  générateur (`sanitizeRouterOSVersion` — premier chiffre, coupe à la
  première impureté) : la valeur est embarquée dans le script .rsc du
  rapport de lancement. Parc concerné : agents RouterOS ≥ 7.19 (garde
  TLS existante de `/agent/cmd` — un routeur plus ancien ne reçoit aucune
  commande, première mise à jour via Winbox ; version inconnue tolérée).

### Technique
agent/routerosupdate.go (buildRouterOSCheck + buildRouterOSUpdate +
sanitizeRouterOSVersion), agent/agent.go (ScriptFor), model/security.go
(CmdRouterOSCheck/CmdRouterOSUpdate), api/handlers_routeros_update.go
(check + update + dédups + normalizeRouterOSCheck + decodeBodyTolerant +
versionSuffix), api/routes.go (2 routes rôle 2), api/agent_handlers.go
(normalisation au rapport + journal du lancement + case lecture sans
journal), api/agent_results.go (trace N°115 des changements de version
dans applyReadState), api/agent_queue.go (staleSentReadKinds + check),
frontend : router-tools/ros-update-card.tsx (nouveau), system-tab.tsx
(câblage), types.ts (RouterOSCheckResult), i18n fr/en +27 clés.
Docs : CONTRACT-V2 §N°115.

### Tests
TestRouterOSCheckScriptShape (poll borné, lectures isolées, rapport
dynamique — la clé rosStatus ne doit JAMAIS s'appeler status),
TestRouterOSUpdateScriptShape (ORDRE : rapport ok AVANT l'install,
rapport d'échec APRÈS), TestRouterOSUpdatePayloadSanitized (version
hostile assainie), TestSanitizeRouterOSVersion (noyau numérique,
suffixes coupés, junk tronqué, tête-chiffre obligatoire),
TestRouterOSCheckSimulated, TestRouterOSUpdateSimulated (version, uptime
zéro, sessions, activité, boucle démo refermée : update sans cible puis
re-check → latest), TestRouterOSCheckAgentQueuedAndNormalized (file +
dédup + rapport en CORPS BRUT comme le routeur — espaces littérales,
pas d'encodage formulaire — résultat normalisé relu par le poll),
TestRouterOSUpdateAgentFlow (file + dédup stricte + journal du lancement
+ read_state re-enfilé + confirmation de version),
TestRouterOSUpdatePayloadRejected (versions hostiles → 400),
TestNormalizeRouterOSCheck (libellés v7 réels, replis, états honnêtes).
Vérifié : gofmt vide, go vet OK, go build OK, go test 12 paquets verts ;
eslint 0, tsgo 0, next build OK (13 routes). Navigateur bout-en-bout sur
backend Go réel (:4000) + next dev (:3016) : 33 PASS / 0 FAIL — SIM
(check immédiat → dialogue → installation → re-check à jour), AGENT
(protocole RÉEL joué en curl : GET /agent/cmd multi-chunks drainé,
rapport routeros_check → panneau normalisé, rapport routeros_update →
panneau installation, read_state post-redémarrage → panneau installée
7.19.3 → 7.19.4 + journaux lancement/confirmation), mobile 390 px,
0 erreur console, captures VLM conformes (alignements, contrastes,
cibles tactiles).

## 2026-09-15 — N°114 — « l'auto-déploiement qui ne déployait rien » : le backend était figé au N°110 en production pendant que GitHub disait « poussé » — TROIS verrous en chaîne levés : le test E2E périmé qui rendait la CI rouge depuis N°112 (URL /app/settings/routers jamais mise à jour), le 429 fantôme du quota S3 qui masquait tout échec réel sous un retry de groupe serial, et l'autoDeploy Render éteint

### N°114 — Contexte : le correctif N°113 était sur GitHub mais pas en production
Retour utilisateur : « corrige définitivement le problème de l'auto-déploiement
Render désactivé ». L'enquête révèle que le réglage Render n'était que la
pointe de l'iceberg : le service Render (créé par API) n'a JAMAIS reçu de
webhook GitHub — l'historique complet (20 déploiements) montre 100 % de
déclenchements `api` manuels. LE mécanisme de déploiement réel du monorepo
est le job CI `deploy-render` (`.github/workflows/ci.yml` — déclenche
l'API Render après CI verte, uniquement si `backend/` a changé)… mais la CI
est ROUGE depuis N°112 : `needs: [backend, frontend, e2e]` → E2E en échec →
déploiement SAUTÉ en silence. Résultat : N°113 (correctif bridage N°106)
poussé sur GitHub, deployé à la main dans l'urgence, et tout futur push
backend promis au même gel.

### Produit
- (1) LA CI ROUGE — LE TEST PÉRIMÉ : `e2e/homenet.spec.ts:233` attendait
  encore `/app/settings/routers$` alors que N°112 a déplacé la fiche box en
  section Infrastructure de la navigation principale (`/app/routers`) — le
  log CI lui-même le prouvait (`Received: http://localhost:3000/app/routers` :
  l'APPLICATION était correcte, l'ATTENTE était fausse). Correctif : URL
  attendue + commentaires alignés (N°112 : legacy deep-link conservé).
- (2) LE 429 FANTÔME — L'AMPLIFICATEUR : l'échec réel déclenchait le retry
  du groupe serial Playwright, qui REJOUE l'inscription « Ma maison » déjà
  passée — 6e inscription depuis l'IP unique du runner contre le quota S3
  (5/10 min par IP) → `429` → un DEUXIÈME échec fantôme masquait le
  premier (le log montre la cascade exacte : 5×201 puis 429 sur le retry).
  Correctif : bornes S3 configurables par environnement
  (`signupLimiterFromEnv` — `SIGNUP_BURST_MAX`/`SIGNUP_DAILY_MAX`,
  entiers > 0, repli FRANC sur les constantes 5/20 sinon) ; la config
  Playwright pose 20/100 (bornes NAT-friendly N°50, miroir exact du
  pattern `RATE_API_PER_MIN` N°102 — même maladie, même remède).
  Production : env absent côté Render → bornes S3 inchangées.
- (3) L'AUTODEPLOY RENDER ÉTEINT : le réglage service (autoDeploy=no,
  sans webhook de toute façon) est réactivé par API (autoDeploy=yes,
  trigger=commit) — défense en profondeur : si un jour le webhook GitHub App
  est installé, le déploiement natif prendra le relais ; en attendant, le
  job CI reste LE chemin de déploiement.

### Technique
- `backend/internal/api/signup_abuse.go` : `signupLimiterFromEnv`
  (getenv injecté, `strconv.Atoi`, bornes > 0 uniquement).
- `backend/internal/api/routes.go` : `New()` câble
  `signup: signupLimiterFromEnv(os.Getenv)` — les autres limiteurs S3
  (join, reset) gardent les constantes (aucune suite E2E ne les traverse).
- `frontend/playwright.config.ts` : env backend `SIGNUP_BURST_MAX=20`,
  `SIGNUP_DAILY_MAX=100` + commentaire d'autopsie.
- `frontend/e2e/homenet.spec.ts` : URL N°112 + commentaires.
- `docs/CONTRACT-V2.md` (§S3) : bornes configurables documentées.

### Tests
- `TestSignupLimiterFromEnv` (api) : sans env → bornes S3 ; env hostile
  (non numérique, négatif, nul) → repli franc ; env valide → 20/100 ET le
  scénario du bug — la 6e tentative (inscription rejouée par le retry) est
  ADMISE sous bornes E2E.
- Vérifié : gofmt vide, `go vet ./...` OK, `go build` OK, `go test`
  (api 32,7 s + store/agent/model) verts ; eslint 0 erreur, tsgo 0 erreur ;
  suite E2E COMPLÈTE en local mode CI (retries actifs) : 14/14 passés
  (bootstrap, sell, resellers, homenet).

### Effet déploiement — la preuve par le feu
CE commit est son propre test bout-en-bout : premier push backend depuis la
réparation → CI verte attendue → `deploy-render` se déclenche pour la
première fois de l'histoire du dépôt → Render déploie automatiquement.

## 2026-09-15 — N°113 — Correctif bridage N°106 « le bridage qui ne bridait rien » : la file mikthrottle- était posée en BAS de liste, sous la dynamique <user> (premier-match gagnant → AUCUN paquet vu par le bridage — terrain cybere-space sc : voucher testa 15 min/50 Mo/512k-1M, 200+ Mo consommés sans aucun bridage jusqu'à l'épuisement du temps). Trois bugs en chaîne, trois correctifs : ancrage EN TÊTE de liste, set du profil qui porte enfin les scripts, génération du tick versionnée pour rejoindre le parc déjà équipé

### N°113 — Contexte : le test terrain qui a fait tomber la chaîne entière
Test N°106 réel sur un routeur client (cybere-space sc) : voucher « testa »
avec profil 15 min / 50 Mo / bridage 512k/1M — le client a consommé PLUS DE
200 Mo sans AUCUN bridage, jusqu'à l'expiration naturelle du temps. Le mode
« cut » historique fonctionnait (limit-bytes-total coupe), le temps
fonctionnait (limit-uptime coupe)… mais la FILE de bridage n'a jamais vu un
seul paquet. Autopsie contre les sources officielles (manuel RouterOS
manual.mikrotik.com — « Simple queues have a strict order : each packet must
go through every queue until it reaches one queue whose conditions fit » —
et sorties réelles du forum MikroTik : `name="<hotspot-user3>"`).

### Produit
- (1) LA CAUSE RACINE — L'ANCRE MORTE : la file dynamique hotspot se nomme
  `<user>` AVEC CHEVRONS ; l'ancre `find where name=$qu` (nom NU) ne
  trouvait JAMAIS rien → la file `mikthrottle-<user>` était ajoutée en BAS
  de liste, SOUS la dynamique → ordre strict premier-match-gagnant : la
  dynamique matche l'IP du client en premier, le bridage ne voit AUCUN
  paquet. CORRECTIF — pose v2 : remove-then-add SYSTÉMATIQUE (rafraîchit la
  cible IP et la position à chaque évaluation — couvre au passage le
  changement d'IP sans re-login, limite v1 documentée) + `place-before` LA
  PREMIÈRE file de la liste (au-dessus de la dynamique `<user>`, du plafond
  QoS mikcloud-qos N°104 et de toute file opérateur) + repli en profondeur
  `/queue simple move` en tête (l'opération historiquement supportée pour
  placer une statique AVANT les dynamiques — pattern des forums 2007+) si le
  place-before échoue.
- (2) LE SET QUI EFFAÇAIT LE ON-LOGIN : `profileSetLine` écrasait `on-login`
  avec le verrou « 1er appareil » SEUL (ou vide) et n'alignait JAMAIS
  `on-logout` — le on-login de bridage posé par le `add` de la MÊME commande
  était effacé aussitôt ; et sur un profil PRÉEXISTANT (add en échec
  silencieux), le set était la SEULE écriture : AUCUN script de bridage
  n'atteignait jamais le routeur. CORRECTIF — le set porte le on-login
  COMBINÉ (verrou + quota via profileOnLoginScript) ET le on-logout
  (onLogoutQuotaScript), vidés explicitement en mode cut (alignement
  complet : un profil repassé en « couper » perd ses scripts).
- (3) LE TICK FIGÉ SANS VERSION : le script du tick vit dans le `on-event`
  du scheduler ROUTEUR — `QuotaSchedOK` vrai bloquait tout re-déploiement :
  les routeurs déjà équipés du tick v1 (dont cybere-space sc) ne
  recevraient JAMAIS le correctif. CORRECTIF — `Router.QuotaSchedVer`
  (génération du tick confirmée) : la version voyage dans le payload de
  `quota_ensure` et est posée au retour « ok » (vérité de CE script-ci,
  jamais d'un ordre en vol antérieur — pattern sel safeWifiRulesVersion
  N°80) ; le check-in re-file tant que la génération confirmée n'est pas la
  courante (`agent.QuotaTickVersion = 2`). Migration idempotente
  `routers.quota_sched_ver` (0 = pré-N°113) : le parc entier re-converge au
  premier check-in après le déploiement, SANS geste opérateur.

### Diagnostic documenté (au cas où)
Un routeur avec une règle firewall `fasttrack-connection` générique qui
matche le trafic hotspot authentifié contourne TOUTES les simple queues
(manuel RouterOS : « FastTrack packets bypass firewall, connection tracking,
simple queues… »). Signal : même le débit DE BASE du forfait ne s'applique
pas. Les compteurs hotspot continuent de compter (fasttrack ne casse ni
l'auth ni les quotas temps) — le test terrain N°106 n'était PAS ce cas (le
débit de base s'appliquait), mais le diagnostic reste documenté dans
CONTRACT-V2 §N°106 limites v2.

### Technique
- agent/quota.go : quotaApplyLines v2 (remove-then-add + place-before la
  première file + repli move-to-top), QuotaTickVersion = 2, commentaires
  d'autopsie complets (les trois temps du bridage, la leçon du terrain).
- agent/profiles.go : profileSetLine aligne on-login combiné + on-logout
  (régression N°113) ; profileAddParams inchangé (déjà correct).
- model : Router.QuotaSchedVer int.
- store : routerSpec + migration `routers.quota_sched_ver INTEGER DEFAULT 0`.
- api : ensureQuotaThrottleLocked re-file si QuotaSchedOK&&Ver<courante ;
  payload `tickVer` ; applyAgentResult pose QuotaSchedVer depuis LE payload
  de la commande rapportée (payloadTickVer tolère int/float64 — relecture
  JSON du store).
- Docs : CONTRACT-V2 §N°106 (enforcement v2 + limites v2 + sous-section
  N°113 autopsie).

### Tests
- agent : TestQuotaScriptsShape renforcé — tokens `place-before=$qf` et
  `/queue simple move`, ordre remove→ancre, GARDE ANTI-RÉGRESSION : l'ancre
  `find where name=$qu` (nom nu, sans chevrons) ne doit PLUS exister ;
  TestProfileEnsureThrottleScripts étendu — LE test du bug : la ligne SET
  doit porter mikq: ET on-logout (elle les écrasait avant), le cut vide
  explicitement les deux champs, verrou+bridage combinés dans le set.
- api : TestQuotaEnsureQueuedWhenThrottleProfile étendu — QuotaSchedOK posé
  MAIS génération ancienne → re-file (convergence N°113), silence uniquement
  à OK+version courante, payload tickVer embarqué.
- Vérifié : gofmt vide, go vet OK, go build OK, go test 12 paquets verts.

## 2026-09-14 — N°110 — QoS multi-sous-réseaux : la cible accepte 1 à 4 CIDR séparés par des virgules — le cas du hotspot au pool ÉTENDU (ProMax WIFI : clients sur 192.168.10.0/24 ET 10.77.0.0/21, deux mondes disjoints qu'aucun préfixe unique ne couvre) devient configurable en UNE file

### N°110 — Contexte : deux sous-réseaux, une seule ligne
Retour terrain (guide d'activation QoS sur ProMax WIFI) : la table des files
montre des cibles dynamiques `192.168.10.53/32` ET `10.77.0.63/32` — preuve
que l'extension de pool N°108 est ACTIVE (des clients hotspot reçoivent des
adresses du range dédié 10.77.0.0/21). Le hotspot vit donc sur deux
sous-réseaux DISJOINTS : aucune cible CIDR unique ne peut couvrir les deux,
et l'ancien PUT refusait les listes (« Cible invalide ») — la QoS n'était
tout simplement pas activable honnêtement sur ce routeur. RouterOS, lui,
accepte plusieurs préfixes dans le `target` d'UNE simple queue : le plafond
agrégat reste UN, les types PCQ classent chaque client (src/dst-address)
quel que soit son sous-réseau.

### Produit
- (1) SAISIE MULTI-CIBLES — `PUT /api/routers/{id}/qos` accepte `target`
  = 1 à 4 CIDR IPv4 séparés par des virgules (`192.168.10.0/24,10.77.0.0/21`)
  : chaque élément canonisé (IPv4 strict), doublons dédoublonnés, ordre de
  saisie conservé ; UN seul élément invalide rejette le PUT entier (400) —
  jamais de file posée sur un sous-réseau de moins. Champ et hint de la
  carte mis à jour (placeholder `192.168.10.0/24,10.77.0.0/21`, mention
  explicite du range étendu N°108).
- (2) SCRIPT ROUTEUR — `buildQueueEnsure` émet `target=a,b` (le builder
  borne chaque élément : charset CIDR, ≤ 18 caractères, slash requis ; un
  élément corrompu fait retomber TOUTE la cible sur le défaut franc
  192.168.88.0/24 — défense en profondeur, jamais un demi-bridage).
- (3) VÉRIFICATION PAR ENSEMBLE — la relecture RouterOS d'une liste peut
  revenir RÉORDONNÉE ou espacée : `qosRowMatches` compare désormais les
  ENSEMBLES de préfixes (split, trim, tri) — une cible qui couvre MOINS
  (un seul des deux sous-réseaux) reste un mensonge : pas de signature.
- (4) AUCUNE MIGRATION — `routers.qos_target` est `text` (la liste voyage
  telle quelle) ; la signature (hash cible+limites) change avec la nouvelle
  forme → re-convergence automatique du parc déjà équipé.

### Technique
`normalizeCIDRList` + `normalizeTargetSet` (api/handlers_qos.go),
`sanitizeCIDRPart`/`sanitizeCIDRList` (agent/queue.go, sanitizeCIDR absorbé),
i18n `tools.qos.target/targetPlaceholder/targetHint` FR+EN,
CONTRACT-V2 §N°110. Tests : agent (script multi-cibles, contrat du
sanitize : espaces/doublons/5 éléments/injection → repli franc) + api
(normalisation 7 formes valides / 7 rejets, parcours doré ProMax :
PUT liste → relecture RÉORDONNÉE → signature posée ; cible incomplète →
signature refusée ; PUT invalide → 400). Vérifié : gofmt vide, go vet OK,
go build OK, go test 12 paquets verts, eslint 0, tsgo 0, build prod OK.

## 2026-09-14 — N°109 — Le badge de la carte QoS disait « Hors ligne » pour « QoS désactivée » : le badge de CONNECTIVITÉ routeur ne doit jamais porter l'état ON/OFF de la QoS — corrigé en une étiquette dédiée (gris neutre = un choix par défaut, pas une panne)

Correctif d'étiquette suite au retour gérant : la carte QoS (onglet Outils
routeur) affichait un badge rouge « Hors ligne · QoS désactivée » alors que
l'en-tête de la même page disait « En ligne » — le composant réutilisait
`StatusBadge status="offline"` (badge de connectivité) pour l'état ON/OFF de
la QoS. Désormais : actif = badge vert « QoS active » + point pulsant, le
span adjacent porte le qualificatif de convergence (« appliquée » / « en
cours (≤ 45 s) » / « retrait en cours ») ; inactif = badge gris neutre
« QoS désactivée » — inactive est un CHOIX par défaut, le rouge destructif
reste réservé aux vraies alertes. Aucun changement backend (GET /qos
identique), l'en-tête « En ligne » reste LA vérité réseau du routeur.

## 2026-09-14 — N°108 — Correctif docteur pool IP : l'extension « ne fonctionnait pas » — address-pool et addresses-per-mac vivent sur le SERVEUR hotspot (/ip hotspot), PAS sur le profil (menu qui ne les a jamais eues) : le script N°97 lisait des champs vides et ses set échouaient en silence — le pool n'a JAMAIS été étendu (constat production ProMax WIFI, « no more free addresses from pool » persistant aux heures de pointe)

### N°108 — Contexte : la panne invisible du bon menu
Le docteur N°97 promettait trois gestes ; en production sur ProMax WIFI,
l'épuisement persiste aux heures de pointe. Vérification croisée contre la
doc RouterOS (help.mikrotik.com, HotSpot - Captive portal) : `address-pool`
et `addresses-per-mac` sont des propriétés de `/ip hotspot` (le SERVEUR) —
le menu `/ip hotspot profile` ne les possède pas. Le script N°97 lisait et
écrivait les deux sur le PROFIL : chaque `get` échouait en silence
(on-error → chaîne vide — le rapport production montrait bien des profils « sans
pool », contresens complet), et le `set` de l'extension échouait à chaque
fois. Résultat net après un clic « Étendre le pool » : un pool orphelin
`mikcloud-pool` créé mais référencé par personne, l'IP secondaire, l'entrée
network et le NAT posés autour de rien — et le VRAI fournisseur d'adresses
(pool du serveur hotspot, partagé avec le DHCP du bridge : 241 IP chez
ProMax) jamais étendu. Le recyclage était à moitié muet aussi :
`address-per-mac=1` sur le profil n'a jamais collé (mauvais menu, mauvais
nom — la vraie propriété est `addresses-per-mac`, sur le serveur), chaque
appareil a continué de pouvoir prendre DEUX adresses.

### Produit
- (1) EXTENSION QUI ÉTEND — par serveur hotspot : (a) le serveur a un
  address-pool → le range dédié 10.77.0.10-10.77.7.254 (~2 037 IP) est
  AJOUTÉ à CE pool ; (b) sinon, le serveur DHCP de la même interface a un
  pool → le range est ajouté au pool DU DHCP (topologie ProMax WIFI : la
  capacité vient de là) ; (c) sinon, pool dédié `mikcloud-pool` posé SUR LE
  SERVEUR. S'ajoutent l'IP secondaire 10.77.0.1/21 sur l'interface hotspot,
  les entrées `/ip hotspot network` (masquerade) et `/ip dhcp-server
  network` (gateway 10.77.0.1 — sans elle, le DHCP n'offre pas proprement
  le nouveau range) et la règle NAT mikcloud-pool-nat. Ménage inclus : le
  pool orphelin laissé par le N°97 est retiré s'il ne référence plus rien
  (double garde avant /ip pool remove).
- (2) RECYCLAGE COMPLET — trois écrits ISOLÉS sur les bons menus :
  login/idle/keepalive-timeout sur `/ip hotspot`, `addresses-per-mac=1`
  sur `/ip hotspot` (un RouterOS ancien qui l'ignore ne fait plus échouer
  les timeouts), et `lease-time=10m` sur les DHCP des interfaces hotspot
  (un bail long brûle l'IP d'un appareil parti pendant des heures).
- (3) DIAGNOSTIC HONNÊTE — serveurs rapportés à 8 champs (address-pool et
  addresses-per-mac RELUS sur le serveur : la vérité de ce qui a collé),
  DHCP à 4 champs (lease-time) ; la liste « profiles » du N°97 est retirée
  (elle ne rapportait que des champs vides lus sur des propriétés
  inexistantes). Le cloud compte la capacité des pools RÉFÉRENCÉS : pool du
  SERVEUR OU pool du DHCP de son interface, dédoublonnés — tolérant aux
  rapports pré-N°108 encore en vol au déploiement (branche DHCP).
- (4) CONVERGENCE AU DÉMARRAGE — la sémantique du rapport a changé :
  `UPDATE routers SET pool_doctor_at='' WHERE mode='agent'` au boot →
  re-diagnostic (lecture seule, une commande par routeur par démarrage
  cloud) ; la capacité re-mesurée arrive au premier check-in.
- (5) SIMULÉ HONNÊTE — « Étendre le pool » applique VRAIMENT le geste en
  démo : capacité 254 → 2291, ranges enrichis du range dédié.

### Technique
`agent/pooldoctor.go` réécrit (menus corrects, repli DHCP, ménage orphelin,
payload `leaseTimeout`) ; `api/agent_pool.go` : `parseDoctorReferenced`
lit le pool en 7e champ des serveurs + branche DHCP (signature sans la
liste profiles) ; `api/handlers_pool.go` : simulé étendu honnête ;
`store/pg_schema.go` : migration boot de re-diagnostic ; i18n
extendDesc1/2 réécrites (le pool qui alimente réellement le hotspot) ;
CONTRACT-V2 §N°97-108 mis à jour. Tests : garde de régression
`TestPoolDoctorNeverTouchesProfileMenu` (JAMAIS de get/set
address-pool|addresses-per-mac sur /ip hotspot profile), formes du script
(recyclage trois écrits isolés, extension a/b/c, idempotence, sanitisation
lease), parseur trois dialectes (serveur avec pool / sans pool / rapport
pré-N°108 à 6 champs — rétrocompatibilité), cas ProMax complet, simulé
étendu (2291, 6 %). Vérifié : gofmt/vet/build OK, go test 12 paquets verts.

## 2026-09-14 — N°106 — Mode bridage « l'atterrissage en douceur » : le quota data épuisé ne coupe PLUS, il bridle (file mikthrottle- posée au-dessus de la file dynamique, marqueur mikq: en tête du commentaire, scheduler mikcloud-quota 20 s, on-login/on-logout du profil) jusqu'à l'expiration du TEMPS — exactement le contrat « 1 h / 1 Go » demandé par le terrain

### N°106 — Contexte : le quota qui coupe punit le client au pire moment
Le comportement natif RouterOS (limit-bytes-total) DÉCONNECTE l'utilisateur
dès la limite atteinte — un client qui paie « 1 h / 1 Go » et consomme son
Go en 20 minutes perd aussi ses 40 minutes restantes. Le terrain demande
l'inverse : maintenir la connexion au débit réduit (soft-landing) jusqu'à
l'expiration du temps. RouterOS n'a AUCUN réglage natif pour ça (le menu
/ip hotspot active est informationnel, le rate-li-mit du profil ne touche
pas une session en cours) : la construction éprouvée des forums MikroTik
(2010+) est la file simple STATIQUE posée au-dessus de la file dynamique.

### Produit
- PROFIL : nouveau sélecteur « À l'épuisement du quota » — Couper (défaut,
  comportement historique inchangé) ou Brider (débit réduit, connexion
  maintenue) + champ « Débit de bridage » (format RouterOS : 512k, 1M,
  512k/2M, presets rapides). Garde-fous : mode bridage ⇒ quota data > 0 ET
  débit valide (refus 400 sinon) ; la validation du PUT porte sur l'état
  FUTUR du profil (champs non fournis = héritage).
- MARQUEUR mikq: : en mode bridage le quota ne devient PAS un
  limit-bytes-total ; il vit en TÊTE du commentaire routeur
  (« mikq:1073741824,512k/512k custom · mikcloud:b1 ») — survit à la
  troncature d'import (60 car.), aux séparateurs neutralisés (| ; & = % +)
  et au suffixe mikcloud_lock: du verrou 1er appareil. Les overrides de
  quota PAR LOT restent supportés (le marqueur porte la valeur effective).
- ENFORCEMENT ROUTEUR AUTONOME : (1) on-login du profil — à chaque
  connexion, cumul UTILISATEUR (bytes-in+out de /ip hotspot user, jamais
  ceux de la session — LA faille classique : un cumul session repart à
  zéro au login) ≥ quota → file posée (remove-then-add, place-before la
  file dynamique) : la fenêtre de re-login est couverte ; FUSIONNÉ au
  script du verrou 1er appareil si les deux actifs ; (2) on-logout —
  dernière session partie → file retirée (anti-héritage d'IP par le
  prochain occupant du bail DHCP) ; (3) scheduler mikcloud-quota (tick
  20 s) — balayage des orphelins (files sans session active : couvre aussi
  le reboot routeur), ≤ 250 sessions par tick : pose/retrait selon le
  cumul (le reset-counters F4 rouvre le plein débit), zéro octet émis vers
  le cloud : le routeur applique la politique même coupé du cloud.
- CONVERGENCE DU PARC : InstallScript pose le scheduler au provisionnement
  neuf ; la commande quota_ensure (pattern watcher N°77, remove-then-add
  idempotent, reprise stale) converge le parc existant — filée UNIQUEMENT
  si le compte possède ≥ 1 profil throttle (économie de veille N°75
  entière sinon), QuotaSchedOK posé au retour « ok » uniquement.
- CONSOLE : badge « Bridage » sur la carte du profil (title explicite),
  badge « Bridé » sur les sessions actives (i18n FR/EN, 14 nouvelles clés)
  — la vérité vient du routeur (rapport read_state throttle= : noms des
  files mikthrottle- présentes), jamais d'un calcul cloud.

### Technique
- Modèle : Profile.QuotaMode/ThrottleRate, Session.Throttled,
  Router.QuotaSchedOK + ValidQuotaMode/QuotaModeEffective/ValidThrottleRate
  (regex miroir frontend THROTTLE_RATE_RE). Migrations idempotentes
  Neon : profiles.quota_mode/throttle_rate, sessions.throttled,
  routers.quota_sched_ok (ALTER IF NOT EXISTS — boot Render).
- Agent : internal/agent/quota.go (NOUVEAU) — scripts RouterOS une ligne
  (tick, on-login, on-logout, cœur partagé quotaApplyLines), buildQuotaEnsure
  (pattern buildWatcherEnsure), profileOnLoginScript (fusion verrou+quota) ;
  users.go : buildUserAdd/buildVoucherBatch posent le marqueur en mode
  throttle (PAS de limit-bytes-total) ; profiles.go : ProfileRef.QuotaThrottle
  + on-login combiné + on-logout dans add/set ; agent.go : case
  CmdQuotaEnsure + scheduler dans InstallScript ; readstate.go : rapport
  throttle= (chunk final, comme sessions).
- Cloud : profileRef porte quotaMode dans CHAQUE user_add/voucher_batch/
  profile_set ; throttleRate au niveau racine des payloads (users, vouchers,
  wifi claim, user_resync) ; ensureQuotaThrottleLocked au check-in ;
  signature quota_ensure → QuotaSchedOK ; applyReadState pose Session.
  Throttled depuis throttle=.
- RÉTRO-COMPATIBILITÉ TOTALE : quotaMode absent/vide = cut — aucun profil
  existant ne change de comportement au déploiement ; les vouchers pré-N°106
  gardent leur limit-bytes-total ; le read_state grossit de ~30 octets
  (throttle=), les routeurs pré-N°106 convergent au premier rapport servi
  par le nouveau cloud.

### Vérifié
gofmt/vet/build OK ; go test 12 paquets verts (suite complète) dont 8
nouveaux tests agent (marqueur, équilibrage/une-ligne des scripts, tokens
du contrat tick/on-login/on-logout, buildQuotaEnsure échappé, user_add/
voucher_batch throttle vs cut, fusion on-login, profileEnsure) + 4 tests
api (convergence conditionnelle/silence/drapeau/doublon/isolation compte,
script shape remove-then-add, CRUD validations 400/201/bascules, flag
Throttled du read_state + retombée) ; eslint 0, tsgo 0, build prod OK.
Validation terrain recommandée avant généralisation (ordre réel des files
place-before sur v6.43+/v7.x, comptage walled-garden) — documentée au
contrat.

## 2026-09-14 — N°102 — Hotspot/HomeNet Phase 4 « le parcours familial éprouvé » : le zéro inexpliqué n'existe plus (le KPI Appareils dit POURQUOI il est vide et devient LA porte de la pause dîner, enseigne « mode agent requis »), et le parcours doré du foyer est verrouillé par les tests E2E (inscription publique « Ma maison » → box en mode agent → découverte par baux DHCP → nom affecté → pause dîner et convergence → gardes d'usage)

### N°102 — Contexte : la Phase 3 livrait les features, rien ne prouvait le voyage
N°98 (plomberie), N°100 (coquille), N°101 (features) — chaque niveau était
vérifié navigateur en session manuelle, mais le PARCOURS complet du foyer
restait éprouvé à la main : personne n'avait rejoué « une famille s'inscrit,
connecte SA box, voit ses appareils, coupe l'internet du tel de mama » en
un seul trait. Et le dashboard maison avait un anglicisme d'état : un KPI
« Appareils en ligne : 0 » NU alors qu'aucune box agent ne rapporte de baux
— un faux zéro, la pire des réponses à un parent qui vient de s'inscrire.
Phase 4 = E2E/polish : verrouiller le voyage par des tests qui le rejouent,
et faire dire au KPI la vérité terrain.

### Produit
- LE ZÉRO INEXPLIQUÉ N'EXISTE PLUS : le KPI « Appareils en ligne » du
  dashboard maison distingue désormais TROIS vérités — registre vivant
  (« sur votre WiFi, maintenant »), box agent là mais premier rapport en
  route (« votre box découvre votre réseau… »), box en mode non-agent
  (« connectez votre box en mode agent »). Le point live du KPI ne clignote
  que quand une box agent alimente réellement le registre.
- ENSEIGNE « MODE AGENT REQUIS » : des routeurs existent MAIS aucun en
  mode agent → encart ambré sous la grille des box (« Les appareils du
  foyer sont découverts par votre box MikroTik connectée à MikCloud en
  mode agent — l'inventaire et la pause dîner vivent de ses bails DHCP »)
  et CTA « Voir mes routeurs » vers la fiche box. Le foyer sait quoi
  faire, le faux zéro disparaît.
- LE KPI DEVIENT LA PORTE DE LA PAUSE DÎNER : la carte Appareils du
  dashboard est un bouton (sémantique, focus visible, la carte reste un
  div neutre — zéro bouton imbriqué) qui ouvre la vue Appareils — le
  raccourci du parent pressé : cliquer le compteur, choisir l'appareil,
  couper. Accessible au clavier (focus-visible ring).
- PARCOURS FAMILIAL ÉPROUVÉ (E2E) : nouveau projet Playwright « homenet »
  (e2e/homenet.spec.ts, autonome — ses propres comptes, suffixe unique) :
  (1) inscription PUBLIQUE par l'UI avec le sélecteur « Ma maison » →
  atterrissage /app/home et invitation honnête ; (2) box sans mode agent →
  KPI honnête + enseigne + CTA ; (3) la box passe en mode agent (PUT, le
  plan essai couvre UNE box), s'inscrit (version RouterOS 7.20 — la garde
  TLS P0 #5 refuse un check-in sans version connue), rapporte ses baux →
  registre, KPI « 3 », enseigne disparue, le KPI ouvre la vue ; (4) nom
  affecté « TV du salon » + pause dîner 30 min sur tel-mama (chip En
  pause, toast) + CONVERGENCE SERVEUR (device_pause servie, rapport rules
  exact — le compte de règles IPv4 de SON script — puis silence au
  check-in suivant : signature posée, pattern N°93/101) + rétablissement ;
  (5) gardes d'usage : compte hotspot → GET /api/devices 404, /app/devices
  re-normalisé vers SON dashboard, sidebar sans « Votre maison » ; signet
  métier d'un foyer (/app/vouchers) → SA maison.
- Boucle agent honnête du spec : découpe du script par commentaires
  d'audit « # mikcloud cmd <id> <kind> », rapport de read_dhcp (bails
  F9) et de CHAQUE device_pause servie avec rules = le compte exact de
  règles IPv4 posables (un routeur réel applique ce qu'on lui envoie —
  leçon N°101 respectée : tout ce qui est servi est rapporté, aucun
  check-in de diagnostic qui consommerait la file).

### Technique
- backend/main.go — RATE_API_PER_MIN (env, défaut 120 — le contrat S1-A2
  est INCHANGÉ tant qu'il n'est pas posé, le déploiement Render ne le
  pose pas) : le runner E2E exécute quatre suites légitimes derrière UNE
  même IP et, preflights OPTIONS compris, franchissait 120 requêtes/min
  en ~30 s — le 429 tombait sur la dernière suite schedulée (constat
  local reproduit deux fois : pagination puis rapport du Mode Vente en
  échec, la requête /api/sell/stock → 429 dans le log). Le webServer
  Playwright pose 600.
- playwright.config.ts — projet « homenet » + surcharges locales
  (E2E_FRONT_PORT/E2E_API_PORT quand le port 3000/4000 est occupé sur le
  poste de dev — le webServer « réutiliserait » le mauvais serveur ;
  E2E_REGISTER_KEY pour refermer la porte d'inscription en bêta privée).
  La porte d'inscription E2E est OUVERTE par défaut (REGISTER_KEY vide) :
  le parcours UI publique « Ma maison » s'inscrit sans clé — le refus
  sans clé reste couvert par les tests Go. Le bootstrap envoie TOUJOURS
  sa clé (ignorée porte ouverte, exigée porte fermée) ; le compte hotspot
  « voisin » du spec homenet s'inscrit en déclarant SA connexion (premier
  hop X-Forwarded-For — sémantique documentée du limiteur S3) : un run
  complet consomme 6 inscriptions contre 5/10 min PAR IP.
- Le front webServer passe à `bunx next start -p ${FRONT_PORT}` (le port
  suit la surcharge ; CI inchangée : 3000).
- home-view : états honnêtes du KPI (hasAgentRouter/devicesEmpty), KPI
  bouton porte de la vue, enseigne ambrée ; i18n 4 clés neuves × 2
  (parité 70/70 homenet).
- CI : le job E2E s'appelle « E2E Playwright (Mode Vente + HomeNet) ».

### Vérifié
- Suite E2E COMPLÈTE en local (stack réelle : go run + next start prod,
  port 3017 car le 3000 du poste est occupé) : 14/14 — setup, 3
  resellers, 5 homenet, 5 sell — après les deux correctifs de course
  (429 quota inscription → XFF du voisin ; 429 plafond api → env knob).
  Les échecs intermédiaires étaient les DEUX 429, cause racine commune
  constatée au log serveur (GET /api/sell/stock → 429) — pas des flakes
  d'assertion.
- gofmt vide, go vet OK, go build OK ; go test 12 paquets verts SANS
  -race puis AVEC -race sur les paquets touchés (racine 2,3 s — le
  middleware du limiteur ; api 459 s, invocation unique — leçon sandbox).
- Front : eslint 0, tsgo 0, build production 13 routes ;
  parité i18n homenet 70/70 clés.
- navigateur : le parcours du spec EST la vérification navigateur
  (inscription UI réelle avec clics — sélecteur radix du pays, case
  confidentialité, dialog radix de renommage, menu pause, toasts).

### Déploiement
- Frontend Vercel (diff frontend/) ; backend Render (diff main.go —
  RATE_API_PER_MIN non posé en production : comportement strictement
  identique, le déploiement ne fait que shipper le knob).
- Zéro changement de données : ni colonne ni table ni migration — la
  Phase 4 ne touche que du code de présentation et des tests.

## 2026-09-14 — N°101 — Hotspot/HomeNet Phase 3 « les features maison » : les appareils du foyer vivent de leurs bails DHCP — inventaire découvert par la box (cadence 2 min, hotspots exclus), noms affectés (« TV du salon », « Tel de mama »), et LA pause dîner (couper l'internet d'un appareil précis, 30 min / 1 h / 2 h / jusqu'à réactivation, expiration recalculée par le cloud à chaque check-in) ; la Protection parle « maison » (couvre-feu en tête), et l'inscription publique du foyer est OUVERTE (sélecteur « Un lieu public / Ma maison »)

### N°101 — Contexte : la coquille N°100 vivait d'un pis-aller
La vue Appareils de la Phase 2 lisait GET /api/sessions — la table tenue
par read_state pour le métier hotspot. Un foyer n'a PAS de portail captif :
ses appareils rejoignent le WiFi, reçoivent un bail DHCP de la box, et
n'apparaissent dans AUCUNE session. Phase 3 = la source de vérité des
foyers : l'agent rapporte /ip dhcp-server lease (read_dhcp, la commande
F9 déjà construite pour l'outil console), le cloud en tient un registre
(une ligne par MAC — l'identité stable, l'IP tournant au gré des
renouvellements), la famille nomme ses appareils et peut couper l'internet
de l'un d'eux — la pause dîner, LE geste parental du phasage convenu.

### Produit
- APPAREILS RÉELS : trois nouveaux endpoints réservés homenet
  (requireUsage, 404 pour un compte hotspot — miroir exact des vues
  produit hotspot refusées aux foyers) : GET /api/devices (registre :
  nom affecté, host-name DHCP, IP, statut du bail — bound = en ligne,
  absence du dernier rapport complet = « gone », ligne conservée pour le
  nom), PUT /api/devices/{id} (renommage, 48 runes, vide légitime),
  POST /api/devices/{id}/pause (pause dîner : minutes 0-1440, 0 =
  illimité, échéance calculée serveur).
- PAUSE DÎNER : une règle FILTER PAR APPAREIL en pause, chain=forward,
  src-mac-address (survit aux renouvellements DHCP), place-before=0
  (au-dessus du fasttrack : les téléchargements EN COURS sont coupés
  immédiatement), action=drop, marqueur mikcloud-pause — remove-then-add
  idempotent, miroir IPv6 best-effort (on-error silencieux, non compté au
  rapport). L'ÉTAT DÉSIRÉ est cloud-calculé (pattern FamilyGuard N°82 :
  jamais d'horloge routeur) : à chaque check-in, l'ensemble des MAC en
  pause effective est recalculé (une pause expirée en sort TOUTE SEULE),
  sa signature comparée à Router.PauseSig, la commande device_pause
  re-filée en cas de divergence — le parent qui change d'avis pendant le
  vol ne voit jamais figer un état périmé (la version envoyée doit être
  TOUJOURS désirée au moment du rapport, pattern SafeWiFi N°80).
- INSCRIPTION PUBLIQUE HOMENET OUVERTE (renversement du contrat N°98) :
  POST /api/auth/register accepte « homenet » — le sélecteur d'usage
  ouvre le formulaire (« Un lieu public — Maquis, cybercafé, boutique » /
  « Ma maison — Vous protégez le réseau familial »), l'atterrissage suit
  l'usage (dashboard métier vs maison). Champ absent = hotspot : les
  clients existants ne changent pas d'un octet.
- RE-SKIN PROTECTION FOYER : la MÊME vue parle « maison » — description
  (« La protection de votre box et des appareils de la famille »),
  encarts du héros (aucune protection → « commencez par le couvre-feu
  internet », pas le filtrage de sites), et le COUVRE-FEU OUVRANT LA
  GRILLE des cartes (LA protection d'une famille) ; le gérant hotspot
  garde SA page au mot près (vérifié navigateur des deux côtés).
- DASHBOARD MAISON : le KPI « Appareils en ligne » lit le registre DHCP
  (fin du comptage « sessions »), la carte box compte les appareils en
  ligne de SA box.

### Technique
- Modèle : model.Device (MAC normalisée XX:XX:XX:XX:XX:XX — un rapport
  corrompu ne crée jamais de ligne fantôme ; PauseActiveAt(now) avec
  repli PRUDENT illimité sur échéance illisible — couper trop longtemps
  se répare d'un clic, mentir à un parent non) ; DB.Devices + table
  « devices » (DDL idempotent, index router/account) + Router.PauseSig
  (ALTER idempotent) — specs/scan/args/load/sync alignés, BuildEmptyState
  initialisé, suppression d'un routeur purge SON registre.
- Agent : buildDevicePause (une règle IPv4 par MAC + miroir IPv6
  best-effort + rapport rules=N — la signature n'est posée que si
  rules == len(macs), vérité routeur pattern N°93) ; CmdDevicePause dans
  ScriptFor, staleSentReadKinds (idempotent, re-prise zombie), vague 101
  en fermeture du FIFO des différés.
- Cadenceur ensureHomeDevicesLocked : read_dhcp enfilé aux check-ins des
  routeurs agent de comptes HOMENET uniquement (borné à 2 min — même
  régime egress que read_state N°74) ; les parcs hotspot ne paient RIEN
  (leurs clics DHCP restent du cache outil F9, et un rapport F9 d'une box
  hotspot n'alimente JAMAIS le registre — garde accountUsageLocked).
- applyDeviceLeases : upsert par (routeur, MAC), host-name vide conservé,
  déduction « gone » UNIQUEMENT sur rapport complet (< 100 baux, borne du
  script — jamais de badge mensonger sur un rapport tronqué, honnêteté
  v2/v4) ; ETag/304 sur GET /api/devices (poll 10 s de la vue).
- Front : devices-view rework (source /api/devices, repli nom affecté →
  host-name → MAC, icônes heuristiques TV/téléphone/ordinateur, chips
  En ligne/En pause avec compte à rebours vivant, menu pause 30 min/1 h/
  2 h/illimité, dialog de renommage, mutations TanStack + toasts, encart
  d'honnêteté « s'applique à la prochaine synchronisation de votre box
  (~1 min) ») ; i18n 21 clés neuves × 2 (parité 66/66 homenet), clés
  signup.usage.* et protection.home.*.

### Vérifié
- gofmt vide, go vet OK, go build OK ; go test complet 12 paquets verts
  SANS -race PUIS AVEC -race (api 463 s) — dont devices_test.go
  (builders, garde d'usage 404/200, inventaire end-to-end avec agent
  factice honnête : upsert, IP rafraîchie, « gone » sur rapport complet
  uniquement, cadence, hotspot muet ; renommage bornes/isolation ; pause
  posée → servie → signée → silence ; rapport menteur rules≠macs ne signe
  PAS ; version périmée ne signe PAS ; expiration re-file la levée ;
  purge à la suppression de la box) et model/devices_test.go (MAC,
  PauseActiveAt illimité/borné/expiré/illisible, nom par runes).
- eslint 0, tsgo 0, build production 13 routes.
- Navigateur RÉEL (backend :4000 + front :3016, agent factice MAISON
  YOPOUGON répondant à TOUS les kinds multiplexés) : inscription foyer
  par le sélecteur « Ma maison » → atterrissage /app/home ; box agent →
  check-in sert read_dhcp → 3 appareils découverts (tv-salon, tel-mama,
  laptop-enfant) ; KPI maison « Appareils en ligne : 3 » ; vue Appareils
  (3 lignes, statuts, bail) ; renommage « TV du salon » en live ; pause
  30 min sur tel-mama → chip « En pause · 29m 56s » + toast + bouton
  « Rétablir internet » + CONVERGENCE SERVEUR (PauseSig posée, journal
  « Pause d'appareils appliquée (1 appareil coupé) ») ; rétablissement →
  « En ligne » ; Protection foyer (description maison, couvre-feu en
  tête de grille) vs Protection gérant (vocabulaire et ordre historiques,
  session parallèle) ; sidebar métier sans « Votre maison » ; mobile
  390 px scrollWidth 390 ; anglais intégral (Devices/Online/Paused/
  Restore internet) ; 0 erreur console/page ; contrôle VLM des captures
  conforme (boutons entiers à 1280 et 1512, aucune coupe).

### Déploiement
Diff backend (table + commandes + garde) : la CI déploie Render —
l'ALTER devices/pause_sig et le CREATE TABLE s'appliquent au boot
(idempotents, zéro donnée existante touchée) ; Vercel suit pour le
front. Les DEUX routeurs clients réels (hotspot) ne changent de rien :
leurs comptes restent hotspot (aucun cycle read_dhcp cadencé, aucun
device_pause — la garde usage l'exclut), la vue Sessions et le dashboard
métier sont inchangés. Test HomeNet pour le gérant : / → « Créer mon
compte » → « Ma maison » — ou basculer un compte de test en console
plateforme (N°98).

## 2026-09-14 — N°100 — Hotspot/HomeNet Phase 2 « la coquille » : un foyer qui se connecte voit SA console — sidebar « Votre maison » (Tableau de bord · Appareils · Protection), dashboard domestique (routeur, appareils, protection n/4, couvre-feu du soir), vue Appareils, zone Paramètres sans la section Hotspot — et les vues produit hotspot n'existent plus pour lui (lien direct /app/vouchers re-normalisé vers la maison, miroir exact des 404 serveur)

### N°100 — Contexte : la confiance se joue à la première sidebar
Phase 1 (N°98) avait posé la plomberie invisible : la colonne
accounts.usage, l'usage dans la session, la garde serveur requireUsage
(404 sur 65 endpoints produit). Mais un compte homenet qui se connectait
voyait encore la console MÉTIER : « Vouchers », « Revendeurs », un
dashboard de revenus vide — exactement la perte de confiance que la
séparation devait éviter. Phase 2 = la coquille : chaque usage a
maintenant SA navigation, SA page d'accueil, SES pages — les
fonctionnalités partagées (Protection, Routeurs, zone Paramètres,
Abonnement) restant communes. Zéro diff backend : la coquille vit du
pont de données Phase 1 (GET /api/routers et /api/sessions sont ouverts
aux deux usages) — déploiement Vercel seul.

### Technique — trois consoles, une seule barrière conceptuelle
- VIEWIDS : « home » (/app/home — tableau de bord maison) et « devices »
  (/app/devices — appareils connectés) rejoignent l'union ; VIEW_SLUGS,
  VIEWS (imports dynamiques — chunks dédiés, un foyer ne paie jamais le
  bundle du dashboard métier), viewTitle alignés.
- ROLES — canView(role, view, usage) : la polarité change avec la
  console. Hotspot (et plateforme) : default-open inchangé (une vue non
  enregistrée reste visible — la barrière réelle vit côté serveur),
  MAIS home/devices y sont invisibles (vues maison). Homenet : liste
  FERMÉE HOMENET_VIEWS (home, devices, protection + les sections de zone
  partagées settings/security/routers/notifications/subscription/team)
  — une vue inconnue n'y apparaît JAMAIS (le piège default-open identifié
  dans l'analyse est neutralisé à la racine). usageOf() normalise
  (absent/vide = hotspot — admin plateforme, sessions pré-N°98).
  FIX au passage : subscription entre explicitement dans VIEW_MIN_RANK
  (rang 1) — sans entrée, le repli default-open fermait l'Abonnement aux
  comptes homenet alors que GET /api/subscription est ouvert aux deux
  usages (détecté en vérification navigateur : la section manquait).
- NAV : NAV_HOMENET_SECTIONS — UNE section « Votre maison », trois
  items : l'histoire du produit en un regard (superviser, voir qui est
  connecté, garder la famille tranquille). navItemsFor(role, isAdmin,
  mode, usage) alimente sidebar ET palette ⌘K (même usage, mêmes vues) ;
  le badge sessions de la sidebar s'applique aussi à « Appareils ».
- STORE : clientLandingView(user) — un client atterrit sur la vue de SA
  console (home pour homenet, dashboard sinon) au login ET à
  l'impersonation (le gérant teste la console maison d'un client en un
  clic depuis « Comptes ») ; syncUsage(user) remplace le user de session
  après relecture serveur sans toucher au reste.
- AUTO-RÉPARATION : la coquille relit GET /api/auth/me une fois par
  chargement (enabled: user.accountId, hors mode plateforme) — usage
  relu sous verrou côté serveur à chaque appel (N°98). Deux cas réels :
  le gérant bascule le compte dans la console plateforme → le client
  voit SA nouvelle console au prochain rafraîchissement, SANS re-login ;
  session persistée antérieure à N°98 → la coquille se corrige seule.
  Vérifié en aller ET retour (homenet→hotspot→homenet, simple reload).
- GARDES UI (miroir client des 404 serveur, « l'UI masque ce que le
  serveur refuserait de toute façon ») : app-route généralise la garde
  URL — un lien direct hors de SA console (bookmark /app/vouchers d'un
  foyer, /app/home d'un établissement, signet périmé après bascule)
  retombe sur l'atterrissage de la console active (home / dashboard ;
  section de zone refusée → première section autorisée de SA zone).
  app-shell : garde de cohérence console↔vue (rechargement sur une vue
  périmée du localStorage), fallback ActiveView respecte la console
  (HomeView pour homenet), bouton « Retour » de zone dirigé vers
  l'atterrissage de la console courante. Préchauffe B4 par console
  (devices/protection pour homenet, sessions/users/vouchers sinon).
- ZONE PARAMÈTRES : settingsSectionsFor(role, usage) — la section
  Hotspot (hub expérience/portail/modèles) est marquée hotspotOnly :
  absente de la zone d'un foyer (produit des établissements, endpoints
  404 pour lui) ; settingsLandingView(role, usage) dirige les menus
  profil — le propriétaire d'un foyer atterrit sur Général, le gérant
  sur Routeurs, jamais sur le hub Hotspot.
- CLOCHE D'ACTIVITÉ : sa porte passe de canView("logs") (Journal =
  user-logs, produit hotspot, fermé aux foyers) à
  canView("notifications") — sa destination « tout voir » est une
  section partagée et /api/activity est ouvert aux deux usages : un
  foyer garde ses notifications de routeur.
- VUE MAISON (home-view) : les quatre questions d'un parent en un coup
  d'œil — Routeur (n/N, « Votre box MikroTik »), Appareils connectés
  (live), Protection (n/4 du maillon le plus faible + verdict),
  Couvre-feu internet (Actif/Programmé/Désactivé + fenêtre). Carte par
  box : identité + statut, anneau ProtectionScoreRing + badge verdict
  (réutilisation stricte des exports N°96), modèle/uptime/dernier
  contact, CTA « Gérer la protection » (détail adressable
  /app/protection/<id>) et « Voir les appareils ». Aucun routeur →
  EmptyState honnête + CTA vers la zone Routeurs (pas de faux zéro).
  Données : /api/routers (poll 15 s) + /api/sessions (poll 10 s) —
  zéro endpoint neuf.
- VUE APPAREILS (devices-view) : miroir domestique de la vue Sessions —
  MÊME source (/api/sessions), vocabulaire de foyer : un « appareil »,
  pas de colonne profil (un foyer ne vend pas de forfaits), PAS
  d'éjection (couper un membre se décide dans Protection via le
  couvre-feu FamilyGuard, pas au coup par coup) ; filtre nom/IP/MAC,
  durée qui avance, trafic ↓/↑ (sémantique RouterOS verrouillée),
  cadence 5/10/30 s, état vide honnête.
- I18N : fragments homenet FR/EN (40 clés home.*/devices.*) + 3 clés
  nav (nav.section.home/nav.home/nav.devices) — parité 43/43 vérifiée
  par script ; le vocabulaire est domestique (« box », « appareil »,
  « couvre-feu ») : un parent n'y croise jamais « voucher » ni
  « revendeur ».

### Vérifié (localement, miroir CI)
eslint 0 ; tsgo 0 ; build production 13 routes (type-check actif).
Parcours navigateur complet sur stack réelle (backend Go :4000 mode
dev JSON + next dev :3016, compte homenet créé par la console
plateforme — chemin N°98, routeur agent « MAISON YOPOUGON » avec
SafeWiFi threats + FamilyGuard 22:00→06:00 + AntiVPN on = 3/4, et
2 appareils vivants via un read_state agent réaliste) : login foyer →
atterrissage /app/home ; sidebar « VOTRE MAISON » 3 items + badge
« Appareils 2 », zéro vocabulaire métier ; KPI Routeur 1/1, Appareils
2, Protection 3/4 « À renforcer », Couvre-feu « Actif 22:00 → 06:00 » ;
carte box (RB2011UiAS, en ligne, anneau 3/4, CTA) ; /app/vouchers,
/app/users → re-normalisés /app/home ; /app/settings/hotspot →
settings/general ; zone Paramètres SANS section Hotspot (avec
Abonnement après le fix) ; vue Appareils (2 lignes TV-Salon/
Tel-Chambre, trafic, durées, ni profil ni éjection) ; vue Protection
partagée intacte depuis la console maison ; palette ⌘K = 3 vues ;
AUTO-RÉPARATION aller-retour (bascule serveur admin + simple
rafraîchissement : homenet→hotspot → console métier complète puis
/app/home→/app/dashboard, retour homenet → console maison, SANS
re-login) ; mobile 390 px scrollWidth 390 (aucun débordement, badge
visible) ; anglais intégral (Your home, Internet curfew…) ; 0 erreur
console/page. Non-régression compte hotspot (cybertest) : atterrissage
/app/dashboard, 4 sections métier, dashboard revenus intact,
/app/home et /app/devices → /app/dashboard. Contrôle VLM des 4
captures conforme (home FR, appareils, zone Paramètres, mobile).
Déploiement attendu : Vercel UNIQUEMENT (Render saute — aucun diff
backend/).

## 2026-09-14 — N°99 : auto-réparation du pool IP — la correction de l'épuisement devient automatique (opt-in par routeur)

### N°99 — Contexte : « pourquoi cette correction n'est pas automatique ? »
Question du gérant après N°97 : le docteur pool diagnostique, mesure et
alerte automatiquement, mais le RECYCLAGE des IP zombies demandait un clic
(« Recycler les IP zombies »). Trois réponses honnêtes motivaient ce choix :
(1) le recyclage pose des timeouts PERMANENTS sur le routeur (login-timeout
5m, idle-timeout 10m, keepalive 2m, address-per-mac=1) — une fois appliqué,
le routeur recycle seul ses zombies en continu, l'action n'était donc pas
« à chaque fois » mais une fois par routeur ; (2) la doctrine « le cloud ne
modifie jamais la config de son propre chef » protège des équipements de
production clients ; (3) le recyclage change une politique MÉTIER visible
(déconnexion d'un appareil inactif après 10 min — décision du gérant, pas
du cloud). N°99 transforme ce compromis : l'auto-réparation devient un
OPT-IN par routeur — le gérant l'active une fois, le cloud recycle ensuite
seul à chaque alerte.

### Produit
- SWITCH « AUTO-RÉPARATION » sur la carte Pool d'adresses IP (Outils
  routeur → Système, mode agent uniquement — la commande a besoin d'un
  agent pour l'exécuter) : activé, à chaque TRANSITION d'alerte (≥ 80 %
  high, ≥ 95 % full) le moniteur marque le routeur et le check-in suivant
  (≤ 45 s) enfile le recyclage des IP zombies SANS geste humain ;
  désactivé, coupure nette (pending purgé, plus rien ne part).
- JAMAIS l'extension automatiquement : ajouter le range 10.77.0.0/21
  change la TOPOLOGIE réseau (IP secondaire + NAT) — un conflit avec un
  plan d'adressage client ne se détecte pas automatiquement, le geste
  reste humain avec confirmation explicite.
- La notification pool_auto (« 🤖 auto-réparation lancée ») confirme
  l'action automatique (le gérant sait que le cloud a agi seul — parce
  qu'il l'y a autorisé) et nomme le geste suivant si la pression reste
  haute (étendre le pool). L'action est aussi journalisée dans l'activité
  (« Auto-réparation pool IP : recyclage des IP zombies envoyé… »).
- État « en attente d'application » visible sur la carte (poolAutoPending
  entre la marque du moniteur et le check-in qui la sert).

### Technique
- MODÈLE : Router.PoolAuto (opt-in persisté) + Router.PoolAutoPending
  (marque transitoire consommée au filage) ; 2 colonnes routers
  (ALTER idempotent, BOOLEAN DEFAULT FALSE — pattern WatcherOK).
- MONITEUR (notify) : à la transition de pression, si PoolAuto && mode
  agent → PoolAutoPending + notification dédiée (KindPoolAuto) ; anti-
  boucle par mémoire de transition PROPRE au Service (autoPoolMarked —
  une marque par transition high/full, purgée au retour au calme) : le
  flag consommé n'est JAMAIS re-posé tant que la pression n'est pas
  redescendue puis remontée (leçon N°97-ter : jamais ~80 commandes/heure).
  Indépendant des canaux : sans canal configuré, l'action a quand même
  lieu (réglage DÉDIÉ du routeur) et reste tracée en activité.
- VEILLEUR (api/agent_pool.go) : ensurePoolDoctorLocked file le recyclage
  (recycle=true, extend=false) AVANT le contrôle de fraîcheur du
  diagnostic (un signal de pression forte ne doit pas attendre 7 jours) ;
  garde « déjà en file/en vol » inchangée (le pending attend le prochain
  check-in, jamais de file doublée) ; journalisation d'activité au filage
  (acteur vide = moteur interne).
- ENDPOINT : PUT /api/routers/{id}/pool-auto { auto: bool } — idempotent
  (re-cliquer ne journalise pas deux fois), rôle ≥ manager, compte
  expiré refusé, non-agent rejeté 400 (message honnête), désactivation
  purge le pending (coupure nette).
- Front : types RouterDevice.poolAuto/poolAutoPending ; switch + mutation
  + invalidations dans pool-card.tsx ; 5 clés i18n × 2 (parité 235/235
  sur le fragment tools).

### Tests & vérification
- 3 tests moniteur (transition → pending + notif ; anti-boucle après
  consommation ; opt-in + mode agent requis ; fonctionne sans canal) ;
  4 tests API (pending → recyclage filé + journalisé + jamais extend ;
  commande en vol → pending en attente ; endpoint bascule/idempotence/
  coupure nette/404 ; non-agent 400). Suite complète 12 paquets verts
  sans -race PUIS AVEC -race (api 453 s).
- Vérification navigateur sur stack réelle (backend :4000 + next :3016)
  25/25 : inscription → routeur agent → boucle agent COMPLÈTE (check-in
  → diagnostic 210/253 = 83 % → jauge ambre + zombies) → switch OFF par
  défaut → activation (toast + persistance) → attente moniteur 35 s →
  PoolAutoPending posé → check-in suivant sert le recyclage AUTO (le
  script contient « set [find] login-timeout » — preuve du geste
  config) → rapport recycled=yes → jauge retombée 120/253 VERTE →
  switch toujours ON après reload → activité « Auto-réparation pool
  IP… occupation 83 % » tracée → mobile 390 px sans débordement →
  0 erreur console/page.
- Diagnostic pur re-vérifié : le script servi NE contient PAS les set
  de timeouts (le mot-clé login-timeout du GET de lecture ne compte
  pas — critère sur « set [find] login-timeout »).

### Zéro action gérant requise
Le comportement par défaut ne change PAS (opt-in) : les routeurs
existant restent en recyclage manuel tant que le gérant n'active pas le
switch. Rien à faire pour les deux routeurs de production — activer le
switch est un choix (recommandé pour les sites denses).

## 2026-09-14 — N°97-ter : pool_doctor en production — la capacité vient aussi du DHCP du bridge, et l'auto-diagnostic ne boucle plus

### N°97-ter — Contexte : les deux découvertes de la première heure de production
N°97 déployé (Render 8d29938, 21:56 UTC), les deux routeurs agents
(ProMax WIFI, CYBER S.C — RouterOS 7.24.1) checkent en ligne. L'inspection
directe de Neon révèle : (1) le rapport pool_doctor de ProMax montre des
PROFILS HOTSPOT SANS ADDRESS-POOL — ses clients reçoivent leurs IP du
SERVEUR DHCP du bridge (Hotspot-Pool, 192.168.100.10-250 = 241 adresses),
configuration légitime et répandue — la capacité calculée par les seuls
pools de profils restait donc nulle ; (2) l'auto-diagnostic était re-filé à
CHAQUE check-in tant que PoolCap=0 (la fraîcheur exigeait « PoolCap > 0 ET
PoolDoctorAt frais »), soit une commande + une ligne de journal toutes les
20 s avec le veilleur d'invités actif (~180/heure — file et Activity
inondés) ; accessoirement le champ 2 des serveurs (nom du PROFIL) était lu
comme un nom de pool (bug de lecture, sans effet sur les configs à pool).

### Correctifs
- CAPACITÉ : le diagnostic rapporte désormais AUSSI les serveurs DHCP
  (« nom|interface|address-pool;… », /ip dhcp-server) — parseDoctorReferenced
  compte les pools des serveurs DHCP posés sur les INTERFACES de serveurs
  hotspot (bridge), en plus des address-pool de profils ; le pool d'un DHCP
  d'interface non-hotspot (LAN privé) reste hors périmètre.
- ANTI-BOUCLE : la fraîcheur de l'auto-diagnostic se juge sur PoolDoctorAt
  SEUL — un diagnostic abouti sans capacité identifiable repose 7 jours
  comme un autre.
- MIGRATION UNIQUE : `UPDATE routers SET pool_doctor_at='' WHERE pool_cap=0
  AND pool_doctor_at<>''` au démarrage — les deux routeurs de production
  re-diagnostiquent dès le premier check-in après ce déploiement avec le
  parseur DHCP-aware (ProMax : PoolCap=241 attendu, alerte dès 80 % =
  193 hôtes).

### Vérifié
- Test dédié sur les données RÉELLES de production (rapport ProMax 22:06) :
  PoolCap=241, PoolRanges=192.168.100.10-192.168.100.250, usage 61 %
  (149/241), pool LAN (ether2/Prive-Pool) exclu.
- Anti-boucle : diagnostic frais + PoolCap=0 → silence (test).
- Suite complète go test ./... verte (11 packages), gofmt/vet propres.
## 2026-09-14 — N°98 — Hotspot/HomeNet Phase 1 « la plomberie invisible » : la colonne accounts.usage existe (ALTER idempotent, défaut « hotspot » — tout le parc existant reste sur le produit historique, zéro changement visible), la session transporte l'usage (login/register/me/impersonation), la garde serveur requireUsage refuse les endpoints produit HOTSPOT aux comptes homenet (404, effet immédiat sans re-login), et la console plateforme segmente : colonne Usage + badges + bascule (PUT /api/admin/accounts/{id}/usage)

### N°98 — Contexte : deux clients, deux mondes, un produit
Décision produit du gérant (analyse d'expert validée) : MikCloud doit
servir DEUX marchés — les gérants de hotspot publics (le produit
historique : vouchers, revendeurs, Mode Vente) ET les foyers avec un
routeur MikroTik (HomeNet : appareils, famille, couvre-feu). Un client
HomeNet face à « Vouchers » et « Revendeurs » se demande si l'outil est
pour lui — perte de confiance immédiate. La séparation des consoles
(Phase 2 : sidebars dédiées) ne sera JAMAIS le garde : un compte
homenet qui forgerait des appels vers /api/vouchers doit être refusé
par le SERVEUR. Phase 1 = toute la plomberie, ZÉRO changement visible
pour les deux routeurs clients réels (l'inscription publique continue
de ne créer que du hotspot — le formulaire est inchangé, le contrat
POST /api/auth/register accepte « usage » mais n'autorise que
« hotspot » tant que la coquille HomeNet n'existe pas).

### Technique — le champ s'appelle usage, JAMAIS mode
- MODÈLE : model.Account.Usage + constantes AccountUsageHotspot /
  AccountUsageHomeNet. Le nom est une décision de conception :
  Router.Mode désigne déjà le mode de CONNEXION au routeur
  (agent/API) — « mode » aurait créé une collision de vocabulaire
  irrécupérable dans l'API, les scripts RouterOS et les échanges
  support. Un compte EST hotspot OU homenet : il ne bascule pas
  (le cas « je gère un cyber ET ma maison » = deux comptes, comme
  deux espaces Slack — l'identité produit n'est pas un réglage).
- BASE : ALTER TABLE accounts ADD COLUMN IF NOT EXISTS usage TEXT
  NOT NULL DEFAULT 'hotspot' — mécanique idempotente N°47 (sans
  l'ALTER, le SELECT différentiel de la nouvelle colonne ne boote
  pas, SQLSTATE 42703). accountSpec gagne la colonne (cols/scan/
  args alignés) ; la synchro différentielle persiste les valeurs au
  premier Save. migrateMultiTenant normalise les usages vides
  (bases JSON de dev, états pré-colonne) → « hotspot ».
- SESSION : login, register, /api/auth/me et l'impersonation
  transportent « usage » dans l'objet user (vide pour l'admin
  plateforme — opérateur sans compte client). La coquille Phase 2
  lira la même clé partout.
- GARDE requireUsage (usage_guard.go) : miroir de requireRole —
  « l'UI masque, le serveur refuse ». Répond 404 (pas 403) : pour le
  client légitime guidé par SA console, la fonctionnalité n'existe
  simplement pas, et on ne révèle ni l'endpoint ni la taxonomie des
  comptes à un curieux. L'usage est RELU sous verrou à CHAQUE requête
  gardée (pas au login) : la bascule admin agît sans attendre
  l'expiration des JWT (24 h). Exemption plateforme (session support
  comprise) : le garde sépare les CLIENTS, pas l'opérateur du SaaS —
  même choix que guardAccountWrite. Repli défensif : usage vide ou
  inconnu = hotspot (comportement d'avant la colonne), compte
  introuvable = hotspot.
- CÂBLAGE (routes.go) : 65 endpoints produit enveloppés
  requireUsage(hotspot) — Mode Vente (8, autour de requireReseller),
  profils (4), utilisateurs hotspot (12), vouchers (10), liens
  d'inscription + file de validation (8), revendeurs (6),
  ventes/rapports/comptabilité/Wave (5), modèles (4), journaux
  utilisateurs (2), analytics portail (1), WiFi jetable console (5).
  Restent OUVERTS aux deux usages (pont de données Phase 2) :
  dashboard, routeurs et tous leurs outils, sessions, protection,
  réglages, activité, équipe, abonnements, notifications, médias,
  stats/horaires. Les routes PUBLIQUES (/api/join/{token},
  /api/reseller/login, portail WiFi par slug) ne sont pas gardées :
  sans JWT, pas de compte à vérifier — leur accès se borne par le
  token du lien ou le slug.
- BASCULE ADMIN : PUT /api/admin/accounts/{id}/usage (handleAdminAccountUsage)
  — le SEUL point de bascule en Phase 1 (l'identité produit d'un
  compte ne se change pas côté client). Idempotent (même valeur → ok
  sans écriture ni journal), journalisé, refus du compte principal
  (l'ID est réservé : données de l'ère mono-tenant = hotspot —
  réponse déterministe AVANT même la recherche) et des valeurs
  inconnues (400 bad_usage). handleAdminAccountCreate accepte
  « homenet » (le chemin de TEST de la Phase 2), l'inscription
  publique le refuse explicitement (400, message honnête
  « L'inscription HomeNet n'est pas encore ouverte »).
- CONSOLE PLATEFORME : liste des comptes (GET /api/admin/accounts)
  et fiche détail exposent « usage » ; côté front — colonne Usage
  dans la table, composant partagé UsageBadge (émeraude Hotspot /
  ambre HomeNet — même patron que StatusBadge, fichier dédié pour
  éviter la dépendance circulaire accounts-view ↔ detail-dialog),
  select « Usage du compte » dans le dialog de création, champ
  Usage + bascule par select dans la fiche (mutation + invalidation
  liste ET fiche, toast). AuthUser.usage et AccountSummary.usage /
  AccountDetail.usage typés (AccountUsage) — la coquille Phase 2
  lira le store persisté.

### Vérifié localement comme la CI
gofmt vide ; go vet OK ; go build OK ; go test complet 12 paquets
VERTS sans -race PUIS AVEC -race (api 432 s) — dont la suite dédiée
usage_guard_test.go : contrat d'inscription publique (absent=hotspot,
homenet=400, inconnu=400), usage transporté par register/login/me,
création admin valide/refuse, garde (10 endpoints produit 404 pour
homenet, 5 endpoints partagés 200, même token), exemption session
support via impersonation RÉELLE, bascule (effet immédiat même token,
idempotence, 400/403/404 nominaux, liste expose usage), normalisation
(store Reload + repli défensif garde), helpers purs. Frontend :
eslint 0 ; tsgo 0 ; build production 13 routes. Parcours navigateur
complet (backend Go :4000 mode dev JSON + next dev :3016, golden path
API 16/16 par HTTP réel, login UI admin → /app → vue Comptes) :
colonne Usage entre Abonnement et Créé le, badges Hotspot/HomeNet
conformes, fiche « Maison Test » + bascule HomeNet→Hotspot→HomeNet en
direct (badge, toast, journal), dialog de création avec select et
aide, mobile 390 px sans débordement de page (scrollWidth 390 — la
table défile dans son conteneur comme avant, 11 colonnes au lieu de
10) ; 0 erreur console/page hors 403 PRÉEXISTANT de GET
/api/subscription pour l'admin plateforme sans compte client (handler
non modifié, route non gardée — artefact du bandeau SA de la console
plateforme, constaté avant N°98) ; contrôle VLM des 5 captures
conforme. Mobile : la capture statique montre la table tronquée dans
son conteneur — comportement overflow-x-auto préexistant (défilement
au doigt), aucune régression de mise en page.

### Déploiement attendu
Render (backend/ modifié — la migration ALTER s'applique au boot,
idempotente) + Vercel. Zéro action gérant : les deux routeurs clients
réels restent « hotspot » par défaut, l'inscription publique continue
de créer du hotspot, RIEN ne change à l'œil. Premier test HomeNet
quand le gérant voudra : console plateforme → Comptes → créer un
compte avec Usage « homenet » (ou basculer un compte de test) —
l'effet des gardes est immédiat.

## 2026-09-14 — N°97 : docteur du pool d'adresses IP — l'épuisement « no more free addresses from pool » des heures de pointe est diagnostiqué, recyclé et alerté

### N°97 — Contexte : la panne qui frappe les clients PAYANTS au pire moment
Capture gérant du 13/09 (WhatsApp) : un client sur le portail captif affiche
« cannot assign ip address — no more free addresses from pool ». Diagnostic
certain : le pool d'adresses IP du hotspot RouterOS est ÉPUISÉ aux heures de
pointe. Trois causes conjointes, classées : (1) pool trop petit — un /24 =
254 adresses partagées entre les clients payants ET tous les appareils à
portée qui réclament une IP AVANT login (le portail captif exige une IP pour
s'afficher) ; (2) « zombies » — RouterOS conserve l'hôte et son IP
indéfiniment quand login-timeout n'est pas posé (défaut : aucun), les
téléphones en connexion auto qui ne se connectent jamais squattent le pool ;
(3) address-per-mac=2 par défaut — un même appareil peut prendre deux
adresses. Le correctif routeur immédiat avait été livré au gérant en analyse
(commandes Winbox) ; N°97 l'INDUSTRIALISE dans MikCloud en trois couches.

### Produit — trois couches, du diagnostic à l'alerte
- COUCHE 1 (commande agent pool_doctor, .rsc idempotent) : DIAGNOSTIC
  (lecture seule — pools + ranges, serveurs + timeouts, profils +
  address-pool/address-per-mac, hôtes, sessions) ; RECYCLAGE (login-timeout
  5m, idle-timeout 10m, keepalive-timeout 2m sur les serveurs hotspot,
  address-per-mac=1 sur les profils — libère les IP zombies SANS toucher au
  subnet, le cookie hotspot re-connecte l'usager au réveil de son écran,
  le solde du ticket est intact) ; EXTENSION opt-in (range dédié
  10.77.0.10-10.77.7.254 ≈ 2 037 IP ajouté au pool de chaque profil, IP
  secondaire 10.77.0.1/21 sur l'interface hotspot, entrée hotspot network
  masquerade, règle NAT mikcloud-pool-nat en match src seul — aucune
  interface WAN à deviner ; clients connectés non déconnectés, seules les
  NOUVELLES attributions tirent du range étendu). Tokens pilotés par payload
  assainis en bloc (anti-injection .rsc), valeurs rapportées nettoyées des
  séparateurs du protocole (fonction mikClean dans le script).
- COUCHE 2 (mesure continue) : read_state rapporte hosts (hôtes tenant une
  IP) à chaque chunk ; le check-in AUTO-DIAGNOSTIQUE (lecture pure,
  recyclage/extension OFF — le cloud ne modifie jamais la config de son
  propre chef) tout routeur agent dont PoolCap est nul ou dont le diagnostic
  dépasse 7 jours (pattern ensureWatcher) ; le rapport pose PoolCap
  (capacité calculée des ranges — formats a-b et CIDR), PoolHosts,
  PoolRanges, PoolDoctorAt.
- COUCHE 3 (alerte) : le moniteur 30 s calcule l'occupation PoolHosts/
  PoolCap — high ≥ 80 %, full ≥ 95 % — notification Telegram/WhatsApp/
  e-mail (kind pool_alert) à CHAQUE transition (anti-spam mémorisé en base,
  pattern stock), message qui NOMME l'action (Outils routeur → Système →
  Docteur pool) et cite l'erreur terrain. Capacité inconnue → aucune alerte
  (jamais de pourcentage inventé).

### Console — carte « Pool d'adresses IP » (Outils routeur → Système)
Jauge d'occupation colorée par seuil (vert < 80, ambre ≥ 80, destructif
  ≥ 95) + compteur IP occupées + zombies (PoolHosts − sessions actives) +
  ranges du pool (vérité routeur) ; boutons « Recycler les IP zombies »
  (inclus d'office dans chaque docteur) et « Étendre le pool »
  (AlertDialog explicite — range, IP secondaire, network, NAT, idempotence) ;
  mode agent : POST + poll de la commande (pattern ping F8, ≤ 120 s) puis
  invalidation [/api/routers, dashboard] ; simulé : diagnostic synthétique
  honnête (254 IP, sessions + 40 % zombies) ; mode API directe : carte
  muette (matrice §0). i18n : 18 clés × 2 langues (tools.pool.*).

### Technique
- Backend : CmdPoolDoctor (model/security.go) ; builder agent/pooldoctor.go
  (mikClean, sanitiseRosToken) ; dispatch ScriptFor + vague différée 97 en
  fermeture ; applyPoolDoctor (agent_pool.go : ParsePoolCapacity, rangeCapacity,
  parseDoctorPools/Referenced) ; ensurePoolDoctorLocked au check-in ;
  handleRouterPoolDoctor (POST /api/routers/{id}/pool-doctor, rôle ≥ manager,
  garde compte expiré, body {extend?}) ; read_state +param hosts ;
  Router.PoolCap/PoolHosts/PoolRanges/PoolDoctorAt ; NotificationSettings.
  PoolAlertState ; moniteur §3-b + poolMessage ; persistance : routers +
  4 colonnes, notif_settings + pool_alert_state (JSON), migrations
  idempotentes.
- Tests : 5 agent (formes du script, hostile payload, défauts, custom) +
  8 api (capacité/ranges malformés, ensure never/fresh/8j/simulé, rapport
  appliqué, endpoint agent/simulé/real/404, read_state hosts, dispatch) +
  2 notify (transitions high→full→calme avec anti-spam, silence sans
  capacité/sans canal) — suite complète verte, -race inclus.
- Vérification navigateur (stack réelle : backend Go :4000 + next :3016,
  Playwright) : login gérant → fiche routeur agent → Système → carte pool ;
  état « capacité non mesurée » → BOUCLE AGENT COMPLÈTE (check-in tire le
  pool_doctor auto, rapport POST /agent/result, PoolCap posé) → clic
  « Recycler » (indicateur en cours, check-in suivant sert le script
  login-timeout=5m + address-per-mac=1, rapport appliqué, toast) → jauge
  140/253 · 55 % · zombies · ranges affichés ; 11/11 contrôles verts,
  0 erreur console, capture VLM conforme (coquille « Répuisement » signalée
  par le VLM était une erreur de LECTURE, la clé FR est correcte).
- Déploiement attendu : Vercel (carte console + i18n) ET Render (commande
  agent + moniteur + migrations colonnes au démarrage) — un diff backend/
  existe cette fois (contrairement aux N°94/96).

## 2026-09-14 — N°96 : refonte UX/UI de la vue Protection — l'état de sécurité devient littéralement « en un coup d'œil » : anneau de score n/4 dans le héros, encart pédagogique nommant les modules à activer, chips d'état par carte, notes « Bon à savoir » en popover et grille 2 colonnes

### N°96 — Contexte : la promesse affichée, pas encore tenue
La vue Protection (N°83) affiche « L'état de sécurité de votre WiFi, en
un coup d'œil » — promesse fondée sur le fond (verdict calculé depuis les
seuls champs de GET /api/routers, zéro endpoint) mais pas sur la forme :
le score vivait en texte brut « n/4 protections actives », le verdict en
badge isolé, l'état de CHAQUE module ne se lisait qu'en parcourant les
cartes une à une, les footnotes honnêtes (N°85/88 — convergence ≤ 45 s,
auto-réparation, limites connues) formaient des murs de texte 11 px sous
chaque carte, et la grille xl:grid-cols-4 compressait les deux cartes
riches (SafeWiFi et ses 3 options, FamilyGuard et son éditeur complet).
Demande du gérant après la fermeture des incidents N°93/95 : améliorer
l'UX et l'UI de cette vitrine du produit.

### Produit — le verdict dit maintenant QUOI faire
- HÉROS : identité du site (nom + badges) à gauche, colonne score à
  droite — anneau SVG « n/4 » coloré par verdict (primaire / ambre /
  destructif), badge « Bien protégé / À renforcer / Non protégé »
  dessous. L'arc compte des MODULES (le « /4 » au centre dit la vérité),
  jamais un pourcentage de « sécurité » qui n'existerait pas.
- ENSEIGNEMENT : encart selon le verdict — « À activer : Anti-piratage
  du WiFi, Couvre-feu internet, Bloque-VPN » (les manquants NOMMÉS,
  dans l'ordre des cartes — les chips « Inactif » répondent à l'encart) ;
  4/4 → encens sobre ; 0/4 → par où commencer (le filtrage de sites).
- CARTES : en-tête commun — icône d'IDENTITÉ par module (ShieldCheck
  filtrage, Lock anti-piratage, MoonStar couvre-feu, GlobeLock VPN)
  teintée selon l'état (primaire actif / neutre inactif), chip d'état
  (niveau SafeWiFi / Actif / Inactif) et bouton « Bon à savoir » ouvrant
  la footnote en POPOVER — l'honnêteté N°85/88 reste, à un clic au lieu
  d'un mur de texte.
- SAFEWiFi : coche de sélection dans chaque option + pastille
  « Recommandé » sur « Menaces bloquées » (le défaut raisonnable pour
  tout WiFi public). FAMILYGUARD : l'interrupteur est séparé du
  PLANNING (heures + jours + enregistrer), groupé dans un bloc bordé.
- GRILLE md:grid-cols-2 : les cartes respirent — la lecture des
  bénéfices ne reprend plus à la ligne à chaque mot.

### Technique — présentation seule, comportements intacts
- Fichiers : views/protection-view.tsx (héros, VerdictCallout,
  liste des manquants via les MÊMES helpers purs que le score —
  protection.ts inchangé, zéro dérive possible ; skeletons miroir) et
  parts/protection-cards.tsx (ProtectionScoreRing exporté, briques
  internes ModuleCard/ModuleStateChip/FootnoteNote, les 4 cartes,
  ProtectionSummaryCard de l'onglet Système conservé tel quel).
- Anneau : SVG r=33, strokeDasharray posé en style CSS (transition
  500 ms fluide au changement de score), stroke par verdict, role="img"
  + libellé accessible « n protections actives sur 4 » (l'arc est
  décoratif, le texte reste la vérité lecteurs d'écran).
- Zéro endpoint, zéro route, zéro migration, zéro colonne — le contrat
  d'API ne bouge pas ; mutations, toasts, invalidations
  ["/api/routers" + "/api/dashboard"], gardes mode agent, éditeur
  FamilyGuard (état complet envoyé par le Switch) et fiche adressable
  /app/protection/<id> strictement inchangés. ProtectionBanner
  (dashboard) et résumé de la fiche routeur inchangés.
- i18n : 8 clés nouvelles par langue (protection.scoreAria, stateOff,
  recommended, detailsTitle, scheduleTitle, hero.allOn, hero.missing,
  hero.noneOn) — parité FR/EN 8/8, clés tools.* stables (seules les
  footnotes changent de FOYER : paragraphe → popover, contenu identique).

### Vérifié localement comme la CI
- eslint 0 ; tsgo 0 ; build production 13 routes (type-check actif).
- Parcours navigateur complet (backend Go :4000 en mode dev JSON avec
  DATA_DIR temporaire + next dev :3016, compte CLIENT réel créé par
  /api/auth/register, routeur agent « CYBER-ESPACE SC », SafeWiFi
  « threats ») : login UI gérant → dashboard → /app/protection ;
  anneau « 1/4 » ambre + « À renforcer » + encart « À activer :
  Anti-piratage du WiFi, Couvre-feu internet, Bloque-VPN » ; 4 cartes
  en 2×2 avec chips « Menaces bloquées » / « Inactif » ×3 ; popover
  « Bon à savoir » (titre + footnote complète, fermeture Échap) ;
  activation du Bloque-VPN en direct → anneau « 2/4 », encart recalculé
  à 2 manquants, chip « Actif », switch coché, toast reprenant le
  message exact du backend ; mode Jour + mobile 390 px (jour et nuit)
  sans débordement horizontal ; 0 erreur console/page (logs dev seuls) ;
  contrôle VLM des captures (anneau, encart, grille, contraste,
  alignement) conforme — l'unique bouton flottant « N » sur les
  captures est l'overlay Next.js Dev Tools, propre au dev.
- Déploiement attendu : Vercel UNIQUEMENT (Render saute — aucun diff
  backend/).

## 2026-09-14 — N°95 : SafeWiFi — l'ordre des règles NAT devient déterministe et vérifié : le N°93 avait livré les bons boucliers au mauvais étage (la table réelle de CYBER-ESPACE SC inversait la théorie « place-before=0 empile en ordre inverse » — boucliers SOUS les dst-nat, ERR_NAME_NOT_RESOLVED sur le dns-name du portail), un bloc move explicite + la disposition rapportée (layout=RRDD) referment la régression portail captif pour de bon

### N°95 — Contexte : des règles correctes… dans le mauvais ordre
Retour du gérant après le déploiement du N°93 : le portail captif de
CYBER-ESPACE SC ne s'affiche TOUJOURS pas. Le print terrain
(/ip firewall nat print) a apporté la preuve : les quatre règles NAT
marquées sont bien posées (boucliers compris — la convergence sw-v3 a
opéré), mais dans l'ordre [dst-nat udp, dst-nat tcp, redirect udp,
redirect tcp] — les boucliers SOUS les dst-nat. La théorie du N°93
(« place-before=0 empile en ordre inverse : le dernier ajouté est le
plus haut ; on émet les dst-nat d'abord pour que les boucliers
finissent au-dessus ») est FAUSSE sur ce RouterOS : la table réelle
conserve l'ORDRE D'ÉMISSION. Conséquence exacte : le DNS d'un client NON
authentifié matche la dst-nat inconditionnelle (règles 0/1) AVANT le
bouclier — part en forward vers 94.140.14.15 (AdGuard Family), rejeté
par hs-unauth (rien hors walled-garden n'est accepté pré-auth) → plus
de résolution pré-login. La capture du téléphone client l'a confirmé
mot pour mot : le hotspot redirige BIEN vers
http://cyberscwifi.net/login?dst=… (le dns-name Mikhmon du profil — la
détection de portail fonctionne), mais la page meurt sur
net::ERR_NAME_NOT_RESOLVED : le dns-name local n'est résolvable que par
le servlet DNS natif (64872), que les dst-nat privent de requêtes. Et
pire : la signature était POSÉE — rules=34 conforme — un compte exact
cachait un ordre inversé. Compter les règles ne prouve pas leur ordre ;
l'ordre, c'est la disponibilité du portail captif.

### Technique — l'ordre imposé (move) puis prouvé (layout), jamais supposé
Trois changements dans buildSafeWifi (agent/safewifi.go), calqués sur la
leçon du post-mortem :
- ÉMISSION : les boucliers pré-auth sont désormais émis AVANT les
  dst-nat — l'ordre d'émission EST l'ordre de table observé sur le
  terrain, le move devient quasiment un no-op sur un routeur sain ;
- IMPOSITION : un bloc move explicite rend la disposition déterministe
  quelle que soit la sémantique de place-before du RouterOS visé : la
  première dst-nat en ordre de table sert d'ancre (swtgt), chaque
  bouclier est déplacé DEVANT elle, puis les autres dst-nat sont
  regroupées devant la même ancre — bloc final [R,R,D,D] contigu,
  au-dessus des règles dynamiques du hotspot. Échec du réordonnancement
  des boucliers → swnat false → garde N°93 (retrait complet de la
  famille : le portail reste servi par le servlet natif, la commande
  re-file) ; le regroupement des dst-nat est best-effort ;
- PREUVE : le rapport échoe la disposition RÉELLE des règles NAT
  marquées en ordre de table (layout — une lettre par règle : R =
  redirect pré-auth, D = dst-nat), calculée APRÈS le réordonnancement :
  &layout=RRDD attendu. Côté cloud (agent_handlers.go), la signature
  n'est posée que si le compte ET la disposition sont exacts
  (SafeWifiNatLayout = « RRDD », miroir de len == SafeWifiNatRules) : un
  ordre réel inversé — ou un rapport de la forme sw-v3 sans disposition —
  ne peut plus JAMAIS passer pour une application réussie. Bump du sel
  sw-v3 → sw-v4 (garde-fou N°48) : tout le parc reçoit la correction au
  check-in suivant le déploiement Render (≤ 45 s console ouverte /
  ≤ 180 s en veille), sans intervention. Contrat PUT inchangé, zéro
  migration (aucune colonne), zéro diff frontend, /ip dns toujours
  intouché (doctrine N°80 : le check-in agent ne dépend jamais du
  résolveur filtrant).

### Vérifié localement comme la CI
gofmt/vet/build verts ; go test complet 12 paquets verts SANS -race PUIS
AVEC -race (api 401 s) ; tests agent/safewifi_test.go étendus (boucliers
émis AVANT les dst-nat — l'ordre d'émission est l'ordre de table observé
; ancre swtgt présente ; 2 blocs move ; rapport échoant
rules+hs+layout ; calcul swlay D/R présent ; miroir
SafeWifiNatLayout == « RRDD » et len == SafeWifiNatRules) ; tests
api/safewifi_test.go étendus (compte conforme + disposition inversée
DDRR → échec — le post-mortem exact du N°93 ; disposition absente
(rapport sw-v3) → échec ; off + layout vide → passe ; sw-v4 ≠ sw-v3 ≠
sw-v2 ≠ sw-v1 ≠ formule sans sel). Script RouterOS généré inspecté
intégralement (ON family : retraits idempotents → 2 boucliers → 2
dst-nat → move déterministe ancré sur swtgt → garde → liste DoH 28
entrées → foreach hotspot DoT/DoH v4 + miroir IPv6 → rapport dynamique
rules+hs+layout ; OFF : retraits seuls → rapport rules=0, layout vide).
Déploiements attendus : Render SEULEMENT (backend/ touché) — la sig
sw-v3 stockée devient mismatch au check-in de chaque routeur protégé →
re-file → correction posée et confirmée par rules=34 ET layout=RRDD ;
Vercel artefact identique (zéro diff frontend) ; la CI joue le même
gate que localement.

## 2026-09-13 — N°94 : écran de connexion — mention « © 2025 MikCloud » retirée + lisibilité Jour des cartes (panneau branding et carte formulaire)

### N°94 — Contexte : demande du gérant (capture d'écran à l'appui)
Deux retours sur l'écran /login : (1) la mention « © 2025 MikCloud —
Connectez vos routeurs MikroTik en toute simplicité » doit disparaître —
le crédit FTCI reste la seule ligne de pied ; (2) en mode clair, les
cartes sont pénibles à lire. L'enquête remonte au panneau branding :
`.login-brand` est TOUJOURS sombre (dégradé zinc→émeraude, signature —
aucune variante Jour n'a jamais existé) alors que ses textes suivent les
tokens du thème actif. En mode Jour : encre sombre sur fond nuit (le
sous-titre du héros et les descriptions des 4 cartes atouts devenaient
quasi illisibles) et les puces de verre Jour (blanc 50 %) délavaient les
cartes en cartons gris clair sans contraste — exactement ce que montrait
la capture.

### Technique — tokens « sur fond nuit » scopés + verre nuit + carte formulaire affirmée
- Suppression : les deux paragraphes `t("login.footer")` (panneau
  branding desktop + pied mobile) ; le crédit FTCI conserve l'emplacement
  et le rythme d'animation (mt-8/mt-6, délai 0,46 s/0,55 s). La clé i18n
  `login.footer` est retirée des DEUX langues (parité préservée —
  2 459/2 459) ; zéro autre consommateur (grep e2e/src vide).
- Lisibilité Jour : `html:not(.dark) .login-brand` force localement les
  tokens « sur fond nuit » (--foreground encre claire, --muted-foreground
  0,82, --primary émeraude lumineux 0,72, --grad-a/--grad-b du wordmark) —
  les utilitaires Tailwind (text-muted-foreground, text-primary,
  from-primary…) résolvent les variables À l'intérieur du panneau, et la
  déclaration `color` rend l'encre héritée claire pour les intitulés sans
  classe couleur ; `html:not(.dark) .login-brand .glass-chip` reprend le
  verre Nuit (voile blanc 5,5 % + liseré blanc 10 %) pour les 4 cartes
  atouts et le badge plateforme. Mode Nuit : AUCUN effet (les deux règles
  sont sous `html:not(.dark)` — sélecteurs spécifiques, aucune collision
  avec `.dark .glass-chip` global). Contrastes obtenus : titres ≈ 11:1,
  descriptions ≈ 7,7:1, badge ≈ 5,8:1 (WCAG AA atteint partout).
- Carte formulaire (`.glass-card`, utilisée UNIQUEMENT par /login — zéro
  effet de bord) : Jour renforcé — opacité 62 % → 78 %, liseré 14 % →
  22 % : sur fond papier menthe, la frontière du formulaire se situe
  d'un coup d'œil. Variante Nuit inchangée.

### Vérifié
eslint 0 ; tsgo 0 ; build production typecheck actif 13 routes ; parcours
navigateur (Playwright local, stack réelle : backend Go :4000 + next
start :3016, thème injecté via localStorage) : mode clair desktop
1440 px (4 cartes titres+descriptions lisibles, héros et sous-titre
nets, frontière formulaire affirmée), clair mobile 390 px (aucun
débordement, pied FTCI seul), sombre desktop (non-régression intégrale),
onglet Mode Vente cliquable et lisible ; « © 2025 MikCloud » absent du
DOM dans les 4 contextes, FTCI présent ; 0 erreur console/page (le 404
locale du beacon /api/vitals est un artefact de build sans
NEXT_PUBLIC_API_BASE — absent en production) ; contrôle VLM des 4
captures : RAS. Zéro endpoint, zéro route, zéro migration ; 1 clé i18n
retirée par langue. Déploiement attendu : Vercel UNIQUEMENT (Render
saute — aucun diff backend/).

## 2026-09-13 — N°93 : SafeWiFi réparé — le portail captif redevient détectable avant le login : le durcissement N°85 détournait AUSSI le DNS des clients non authentifiés, privant l'OS de résolution pré-auth (plus de popup de connexion, régression production CYBER-ESPACE SC) — deux règles redirect vers le servlet DNS natif du hotspot restaurent le comportement RouterOS sans rouvrir l'échappatoire filtrage

### N°93 — Contexte : une régression de disponibilité, pas de sécurité
Constat du gérant immédiat après le push N°85 : le portail captif du
routeur CYBER-ESPACE SC (migré Mikhmon, SafeWiFi « familles » actif) ne
s'affichait plus du tout — un nouvel appareil se connectait au WiFi et
restait sur « connecté, sans internet », sans popup de login, sans page
accessible même en naviguant. L'enquête (manuel MikroTik « Hotspot
customisation », section Firewall customizations) a établi le mécanisme
exact : le hotspot possède ses PROPRES règles dynamiques NAT — un jump
« chain=dstnat hotspot=from-client → hotspot », puis un redirect natif
du port 53 vers son servlet DNS interne (64872), servlet accepté
PRÉ-authentification par le filtre hs-input (ports 64872-64875 :
services locaux d'authentification). C'est ce mécanisme qui rend le DNS
— donc la détection du portail captif par l'OS (connectivitycheck.
gstatic.com, captive.apple.com…) — possible AVANT le login, y compris
la résolution du dns-name local du profil (config Mikhmon typique).
Les dst-nat N°85 en tête de table (place-before=0) préemptaient ce
redirect pour TOUT le monde : le DNS d'un client NON authentifié
partait en forward vers l'IP EXTERNE du résolveur filtrant (AdGuard),
où le filtre hs-unauth le rejetait (tout ce qui n'est pas walled-garden
est rejected pré-auth) — DNS mort pré-login, portail indétectable, et
dns-name de surcroît indésolvable chez le résolveur public. Les
routeurs installés par MikCloud ne voyaient pas le problème complet
(walled-garden dns N°29 + pas de dns-name), le migré Mikhmon le voyait
intégralement.

### Technique — le bouclier pré-auth, miroir du natif, au-dessus des dst-nat
Le correctif restitue le comportement natif pour les clients NON
authentifiés SANS rouvrir l'échappatoire N°85 : DEUX règles redirect
« hotspot=from-client,!auth action=redirect to-ports=64872 » (le
matcher NATIF du hotspot, celui-là même qu'utilisent ses règles
dynamiques — chain=dstnat, udp + tcp) sont posées APRÈS les dst-nat
dans le script, donc AU-DESSUS d'elles dans la table (place-before=0
empile en ordre inverse : le dernier ajouté est le plus haut — ordre
final voulu : bouclier PUIS dst-nat PUIS règle Mikhmon héritée, rendue
inerte pour le port 53). Le DNS pré-login redevient natif : le servlet
64872 répond lui-même (noms locaux + domaines publics, upstream non
filtré — un invité ne peut de toute façon rien ouvrir hors
walled-garden avant le login, il n'y a rien à filtrer à ce stade).
Dès l'authentification, le matcher ne matche plus : le DNS transite
par les dst-nat → résolveur filtrant. La promesse N°85 est
intégralement conservée pour tout ce qui est authentifié, ainsi que
pour le LAN du gérant et tout DNS externe — le filtrage de contenus
(la raison d'être du module) reste exact, seul le chemin pré-login
change. Garde de disponibilité (doctrine N°80 : la disponibilité du
site prime sur la stricteté du filtrage) : un échec de pose dans la
famille NAT (variable swnat — p.ex. matcher absent d'une version
RouterOS exotique) déclenche le retrait complet des règles marquées de
la famille : aucune dst-nat résiduelle ne préempte le redirect natif,
le portail reste servi, l'échec est rapporté et la commande re-file au
check-in suivant — jamais d'état à moitié posé qui tuerait le portail.
Comptage vérifié (vérité routeur) : SafeWifiNatRules = 4 (2 dst-nat +
2 boucliers) — SafeWifiRulesExpected passe de 2+len(DoH)+2×hs à
4+len(DoH)+2×hs, la signature n'est posée que si le routeur RAPPORTE
ce compte exact. Bump du sel sw-v2 → sw-v3 (garde-fou N°48) : tout le
parc — CYBER-ESPACE SC en tête — reçoit la correction automatiquement
au check-in suivant le déploiement Render (≤ 45 s console ouverte /
≤ 180 s en veille), sans intervention. Zéro endpoint, zéro vue, zéro
migration (aucune colonne : la synchro différentielle Neon n'a rien à
faire), contrat PUT /api/routers/{id}/safewifi inchangé, footnote
frontend inchangée (la promesse produit ne bouge pas : le filtrage
s'applique pareil aux clients authentifiés). Rebase sur l'éclatement Go
N°89 du gérant absorbé : le correctif vit dans le fichier dédié
agent/safewifi.go posé par l'éclatement, le sel dans agent_security.go,
la branche de vérification dans agent_handlers.go — aucun conflit
restant.

### Vérifié localement comme la CI
gofmt/vet/build verts ; go test complet 12 paquets verts SANS -race
PUIS AVEC -race ; tests agent/safewifi_test.go étendus (4 règles NAT
marquées : 2 dst-nat place-before=0 vers le résolveur + 2 boucliers
pré-auth hotspot=from-client,!auth vers 64872, udp ET tcp ; ordre
d'émission : boucliers APRÈS les dst-nat → posés au-dessus ; garde
:if (!$swnat) présente, ajouts NAT alimentant swnat ;
SafeWifiNatRules == 4 miroir du comptage ; off ne pose AUCUN objet —
inchangé) ; tests api/safewifi_test.go étendus (compte attendu
4+len+2×hs via la source unique ; sw-v3 ≠ sw-v2 ≠ sw-v1 ≠ formule sans
sel — le parc ne peut rester ni sur la forme N°85 qui tue le portail
ni sur les précédentes). Script RouterOS généré inspecté
intégralement (ON : retraits idempotents → 4 NAT → garde → liste DoH
28 entrées → foreach hotspot DoT/DoH v4 + miroir IPv6 → rapport
dynamique rules+hs ; OFF : retraits seuls → rapport rules=0) ; syntaxe
conforme aux formes .rsc en production (blocs :do/on-error
multi-lignes du foreach, négation !$ déjà éprouvée par l'agent, matcher
hotspot utilisé tel quel par les règles dynamiques du système).
Déploiements attendus : Render SEULEMENT (backend/ touché :
buildSafeWifi + sel sw-v3 + SafeWifiRulesExpected — le déploiement
redémarre le backend, la sig sw-v2 stockée devient mismatch au check-in
de chaque routeur protégé → re-file → correction posée et confirmée
par le compte 4+28+2×hs) ; Vercel déploie un artefact
fonctionnellement identique (zéro diff frontend) ; la CI joue le même
gate que localement.

## 2026-09-13 — N°92 : éclatement de router-tools (1 789 l.) — le panneau Outils devient une entry de 96 l. + 6 fichiers par domaine, composants déjà autonomes (zéro déplacement d'état), contenu vérifié ligne par ligne (1 541/1 541)

### N°92 — Contexte : troisième et dernière cible citée par l'audit
router-tools.tsx était la 3ᵉ plus grosse vue (1 789 l.) mais sa structure
était différente des deux précédentes : une COLLECTION de 15 composants
indépendants (chacun avec son PROPRE état — TrafficTab, IpBindingsTab +
AddBindingDialog, 4 tables F9 + ToolSection + ToolsTab, SystemInfoCard,
PingCard + PingResultPanel, SchedulerCard + SchedulerAddDialog, PowerCard,
SystemTab) reliés par RouterToolsPanel. L'éclatement idéal : chaque
composant déménage dans son fichier SANS AUCUN déplacement d'état.

### Technique — découpage scripté par plages auditées + imports régénérés
Génération scriptée (méthode N°89) : plages de lignes auditées aux
commentaires de section d'origine, imports régénérés par scan d'usage
(mot entier dans le corps — conservateur), exports ajoutés aux
déclarations de haut niveau, imports inter-fragments générés
automatiquement. Résultat :
- router-tools.tsx (96 l.) — ENTRY : RouterToolsPanel (poll 15 s du
  routeur vivant, 4 TabsTrigger) ;
- router-tools/shared.tsx (125 l.) — constantes (MAC_RE, INTERVAL_RE,
  MAX_SAMPLES), helpers (sleep, shortClock, shortBits, fmtMs),
  fetchToolEnvelope (F9/F10) et les 4 composants d'état (UnsupportedState,
  ToolError, ToolSkeleton, QueuedBanner) ;
- traffic-tab.tsx (297 l.), bindings-tab.tsx (355 l.), tools-tab.tsx
  (339 l.) — les onglets F6/F7/F9 avec leurs dialogs et tables privés ;
- system-tab.tsx (384 l., deux segments) — F8 : SystemInfoCard, la
  chaîne ping complète (PingStats/PingOutcome/toPingStats/errorMessageFrom/
  PingResultPanel/PingCard), PowerCard et l'assembleur SystemTab
  (résumé Protection N°83 + 4 cartes) ;
- scheduler-card.tsx (356 l.) — F10 : SchedulerCard + SchedulerAddDialog.
Deux correctifs post-script : un faux positif d'import circulaire
(shared importait TrafficTab détecté dans un COMMENTAIRE — retiré) et un
en-tête doc mal échappé. Vérification de contenu : les lignes de code du
corps de l'original (1 541) sont reconstituées à 100 % dans les fragments
(script de contrôle par lignes uniques — zéro perte, zéro ajout).
Seul consommateur externe inchangé : routers-view importe RouterToolsPanel.

### Vérifications
eslint 0 ; tsgo 0 ; build production typecheck actif : 13 routes ;
parcours navigateur autonome (agent-browser) sur stack locale (routeur
SIMULÉ, données de démo) : fiche routeur /app/settings/routers/<id> →
panneau rendu avec les 4 onglets ; Trafic (sélecteur d'interface, Rx/Tx,
table des interfaces) ; IP Bindings (liste + Ajouter) ; Outils (sous-
onglets DHCP/Hôtes/Cookies/Journal, sections enveloppe) ; Système COMPLET
(Informations système, Protection 0/4 + lien vers la vue, ping, tâches
planifiées, alimentation Redémarrer/Éteindre) ; PING RÉEL exécuté
(8.8.8.8 → résultat avec Envoyés/Reçus et latences 21/35/49 ms — la
mutation + le panneau de résultat extraits fonctionnent) ; 0 erreur
console/page ; contrôle VLM plein écran : RAS. Zéro endpoint, zéro
route, zéro migration, zéro clé i18n. Déploiement attendu : Vercel
UNIQUEMENT (aucun diff backend/ — Render saute).

## 2026-09-13 — N°91 : éclatement de sell-shell (1 827 l.) — le Mode Vente PWA devient un shell d'état de 945 l. + 4 fichiers, anti-fuite et hors-ligne intacts, E2E 9/9 + parcours navigateur complet

### N°91 — Contexte : deuxième vue de la série (après N°90 vouchers-view)
sell-shell.tsx était la 2ᵉ plus grosse vue (1 827 l. — 20 useState,
useInfiniteQuery paginé + 3 useQuery + 3 useMutation, replay hors-ligne
IndexedDB, partages Web Share/clipboard, 770 l. de JSX). Même remède
conservateur que N°90 : l'état, les requêtes, les mutations, le replay et
les partages RESTENT au shell ; le rendu déménage vers la présentation.
Filet de sécurité idéal : la suite E2E Playwright du Mode Vente (9 tests
navigateur) couvre exactement ce flux.

### Technique — helpers module-level + carte + section stock + dialogs
sell-shell.tsx (945 l.) garde TOUTE la logique (login PIN via store,
optimiste Phase D sur les pages InfiniteData, replay 409-safe, garde
N°39 anti-race sur la recherche exhaustive, snapshots localStorage UX R6,
partages) + header/stats/footer ; le rendu déménage dans sell/ :
- helpers.ts (267 l.) — les 230 l. de types de contrat et fonctions
  MODULE-LEVEL existantes (SellVoucher/StockPage/SellMe/SellPeer…,
  filterPagedStock, useOnline, store externe de la vue R1
  subscribeView/getViewSnapshot, caches readCache/writeCache,
  isNetworkError, expiresSoon, VIA_*/viaIcon, fmtDay, groupStock) —
  transfert pur, aucun identifiant ne change de portée ;
- voucher-card.tsx (144 l.) — la carte ticket UX R1 (code masqué,
  badges expire-bientôt/en-file, mode retour = checkbox anti-misclick,
  bouton Vendu) — les spinners de mutation deviennent des props
  (sellPendingId calculé au shell) ;
- stock-section.tsx (403 l.) — le bloc <main> : squelettes, état vide,
  barre du stock + bascule de vue + imprimer, recherche R3, bannière
  vente auto, bannière file hors-ligne, les DEUX vues (groupée
  profil → lot avec sélection par lot, ou plate « récents »), pagination
  « Afficher plus » — la file passe en Set d'ids (queuedIds) pour les
  chips et la bannière ;
- dialogs.tsx (483 l.) — les 4 dialogs : rapport de journée (ventilation
  par canal, dépôt-vente, export CSV, partage), confirmation de vente
  (UX R2, code muet), reçu anti-fuite (seule porte de sortie du code),
  sortie de stock N°20/N°21 (retour gérant OU transfert — les ids
  sélectionnés restent capturés au shell via fermetures onReturn/
  onTransfer, jamais passés vides).

### Vérifications
eslint 0 ; tsgo 0 ; build production typecheck actif : 13 routes ;
E2E Playwright COMPLÈTE sur stack locale (port 3015 patché puis restauré) :
9/9 verts — login PIN, pagination « Afficher plus », recherche exhaustive
2ᵉ page (garde N°39 re-testée), vente tactile (code masqué avant
confirmation, reçu partageable après), rapport + export CSV comptable ;
parcours navigateur autonome (agent-browser) sur l'état semé par l'E2E
(revendeur prépayé, 60 tickets) : login PIN → comptoir (crédit 35 400
XOF, vue groupée, bannière auto) ; vente complète → reçu avec code
révélé (EEKDZ) APRÈS confirmation ; mode retour → « Sélectionner tout le
lot » → barre sticky (60 sélectionnés, 12 000 XOF) → dialog retour
(destination, recrédit affiché) → annulation propre ; rapport de
journée rendu ; 0 erreur console/page ; contrôle VLM : RAS. Zéro
endpoint, zéro route, zéro migration, zéro clé i18n. Déploiement attendu :
Vercel UNIQUEMENT (aucun diff backend/ — Render saute).

## 2026-09-13 — N°90 : éclatement de vouchers-view (1 893 l.) — la plus grosse vue du frontend devient un shell d'état de 866 l. + 4 fichiers de présentation, zéro déplacement d'état, parcours navigateur vérifié de bout en bout

### N°90 — Contexte : la suite volontaire de l'audit « fichiers monolithiques » (après N°87 i18n et N°89 Go)
Le gérant a donné son feu vert pour la dernière famille de monolithes :
les VUES frontend. vouchers-view.tsx était la plus grosse (1 893 l. —
32 useState, 7 useQuery, 6 useMutation entremêlés de 740 l. de JSX).
Le risque documenté au N°89 était le flux d'état React : la parade est
un éclatement CONSERVATEUR — l'état, les requêtes, les mutations et
les handlers RESTENT dans le shell ; seul le JSX déménage vers des
composants de présentation alimentés par props (pattern déjà prouvé
par batch-detail-sheet/batch-pipeline extraits lors de la refonte v2).

### Technique — shell d'état + onglets de présentation (zéro logique déplacée)
vouchers-view.tsx (866 l.) garde TOUT l'état et le rendu des 8 dialogs ;
le rendu des deux onglets déménage dans views/vouchers/ :
- shared.ts (41 l.) — constantes PAGE_SIZE/BATCH_PAGE_SIZE, options de
  statut, shortBatch, type VouchersStats (module SANS dépendance React,
  importé par le shell ET les onglets — zéro cycle d'import) ;
- vouchers-tab.tsx (498 l.) — KPI du stock, barre de filtres, table des
  tickets, pagination ; les spinners de mutation deviennent des props
  (reprisePendingId/resyncPendingId calculés au shell) ;
- batches-tab.tsx (729 l.) — pipeline « tour de contrôle », filtres
  desktop + sheet mobile, cartes mobiles + table desktop, pagination ;
  les 9 helpers de rendu (holdingsChips, velocityInfo, 4 selects
  partagés, transferIconButton, printIconButton, batchActionsMenu)
  deviennent des closures du composant — mêmes signatures, mêmes rendus ;
- confirm-dialogs.tsx (152 l.) — les 3 AlertDialog de confirmation
  (reprise gérant, suppression voucher, suppression lot).
Fidélité traquée au détail : les filtres actifs des lots restent
calculés au shell sur la recherche DEBOUNCÉE (comportement d'origine,
pas l'input brut) ; le clic sur le #lot d'une ligne voucher appelle
filterByBatch qui ne touche PAS au filtre détenteur (fidèle au code
d'origine) ; l'effet de synchronisation du deep-link Phase D
(/app/vouchers/<batchId>, fix 192ad9f) reste INTÉGRALEMENT au shell ;
la pagination passe le setter natif (onSetPage={setPage}) pour garder
les updaters fonctionnels (p => Math.max(1, p-1)) à l'identique.

### Vérifications
eslint 0 ; tsgo 0 ; build production AVEC typecheck actif (N°84) :
13 routes vertes ; E2E Playwright complète sur stack locale (backend Go
4000 mode JSON, front build NEXT_PUBLIC_API_BASE local, port patché 3015
puis restauré — piège N°81) : 9/9 verts ; puis parcours navigateur
autonome (agent-browser) sur données semées par API réelle (73
vouchers, 2 lots) : onglet Vouchers (KPI 72/1/0/0/14 400 XOF serveur
N°74, table, révélation mot de passe, pagination page 2) ; onglet Lots
(pipeline 14 400 XOF/70 tickets, 2 lots, 3 boutons d'action, menu ⋯ 6
items) ; « Voir les vouchers » → deep-link Phase D avec recherche
pré-remplie B20260913-1183 ; Retour navigateur → filtre levé (branche
leaving de l'effet) ; fiche 360° (drawer complet : cycle de vie,
valeur & marge, possession, écoulement) ; responsive mobile 390 px
(bouton « Filtres » + sheet bottom complet) ; 0 erreur console, 0 page
error ; contrôle visuel VLM desktop + mobile : RAS. Zéro endpoint,
zéro route, zéro migration, zéro clé i18n. Déploiement attendu :
Vercel UNIQUEMENT (aucun diff backend/ — Render saute, job
deploy-render détecte l'absence de diff).

## 2026-09-13 — N°89 : éclatement des monolithes Go — les 4 plus gros fichiers du backend (1 709 à 2 445 lignes, enrichis du N°88) deviennent 31 fichiers par domaine, même package, zéro sémantique changée, suite -race complète verte

### N°89 — Contexte : la suite de l'audit « fichiers monolithiques » (après N°87)
Les quatre plus gros fichiers du backend concentraient des domaines
entiers : models.go (1 764 l. — 39 types tous domaines confondus + les
niveaux AntiVPN), pg.go (2 445 l. — connexion, DDL 750 l., chargement,
moteur de synchro et specs), agent.go (2 364 l. — cœur de l'agent +
générateurs des 7 modules dont le tout nouveau buildAntiVpn),
agent_handlers.go (1 709 l. — routes, file, vérification sécurité des
5 modules). Le remède Go est mécanique et sûr : découpage en fichiers
du MÊME package (aucun identifiant ne change de portée), le
compilateur et la suite -race garantissent l'équivalence stricte.

### Technique — découpage scripté par plages de déclarations, rejoué sur N°88
Génération 100 % scriptée : segmentation aux déclarations de haut
niveau (func/type/var/const, commentaires doc attachés), affectation
par plages de lignes auditées, écriture SANS imports puis régénération
goimports, assertions de couverture (chaque déclaration dans
exactement une cible). model → 10 fichiers (ids, models, tenant,
admin, billing, security — y compris AntiVpnOff/On et
AntiVpnLevelEffective, entities, wifi, join, db) ; store → 5 (pg,
pg_schema DDL+migrations — y compris les colonnes antivpn_*, pg_load,
pg_sync moteur, pg_specs — y compris routerSpec antivpn) ; agent → 12
(agent cœur, safewifi, shield, familyguard, ANTIVPN dédié miroir des
trois autres modules de sécurité, walledgarden, hotspot_files,
profiles, users, readstate, scheduler, ipbindings) ; api → 4
(agent_handlers routes+handlers, agent_queue, agent_security —
y compris ensureAntiVpnLocked et les signatures av-v1, agent_identity).
Plus gros fichier résultant : pg_schema.go 880 l. (ensureSchema est
UNE fonction DDL indivisible sans refactoring réel — documenté).
Un garde-fou statique adapté : pg_settings_sql_test.go (N°56) lit le
littéral SQL de l'UPSERT settings DANS SON FICHIER SOURCE — il pointe
désormais vers pg_sync.go où syncSettings a déménagé (l'invariant
vérifié est inchangé). Zéro endpoint, zéro route, zéro migration, zéro
contrat changés ; les tests N°88 (agent/antivpn_test.go,
api/antivpn_test.go) et handlers_antivpn.go ne bougent pas — ils
trouvent buildAntiVpn dans son nouveau fichier, même package.

Note de session : l'éclatement avait été préparé sur le N°87 quand le
N°88 (AntiVPN, autre session) a atterri sur main en parallèle — le
découpage a été REJOUÉ intégralement sur l'état post-N°88 (les
ajouts AntiVPN répartis dans les fichiers cibles naturels), la
validation complète repassée, et le commit renuméroté N°89.

### Vérifications
go build ./... vert ; go vet ./... vert ; gofmt -l . vide ;
go test -race -timeout 30m ./... : 12 paquets verts (api 404 s,
store 10,7 s, agent 1 s) Y COMPRIS les tests AntiVPN N°88 et le
garde-fou SQL adapté ; aucune référence de chemin ni go:embed ne
dépend des anciens fichiers (hotpage embed inchangé, seul test
concerné adapté) ; déploiement attendu : Render UNIQUEMENT (diff
backend/ — le job deploy-render déploie, l'artefact est
fonctionnellement identique à celui du N°88 : même code, mêmes
identifiants, fichiers réorganisés) ; Vercel redéploie un artefact
identique pour le diff CHANGELOG.

## 2026-09-13 — N°88 : AntiVPN — le bloque-VPN ferme la dernière échappatoire connue de la Protection : les VPN et tunnels standards (WireGuard, OpenVPN, IPsec, PPTP/L2TP, WARP, Tor) sont coupés depuis le WiFi public, 4e module de la vue Protection, pendant que la navigation, l'heure des téléphones et les appels WhatsApp restent intacts

### N°88 — Contexte : la footnote N°85 disait la vérité, N°88 la referme
Le durcissement SafeWiFi (N°85) avait écrit noir sur blanc la limite
résiduelle : « seul un VPN contourne ». Le gérant a donné le feu vert pour
le module qui referme cette porte : un adolescent (ou un client) qui
installe un VPN pour contourner le filtrage de sites retrouve désormais
le tunnel coupé. C'est le 4e module de protection — SafeWiFi N°80,
Shield N°81, FamilyGuard N°82, AntiVPN N°88 — et le verdict « Bien
protégé » passe de 3/3 à 4/4 : sans bloque-VPN, la porte VPN reste
ouverte, le verdict l'exige pour être honnête (un badge « Bien protégé »
avec une échappatoire connue serait un mensonge de vente).

### Technique — L4 honnête, zéro DPI, pattern Shield exact
PUT /api/routers/{id}/antivpn {level: off|on}, contrat miroir de Shield
N°81 : niveau persisté (Router.AntiVpnLevel/Sig/AppliedAt, colonnes
antivpn_* via ALTER TABLE IF NOT EXISTS — migration Neon automatique,
synchro différentielle du backend à chaque sauvegarde), sig inchangée,
ensureAntiVpnLocked re-file au check-in suivant (≤ 45 s console ouverte /
≤ 180 s en veille), retour « ok » VÉRIFIÉ (compte de règles marquées
mikcloud-antivpn == 4 × hotspots RAPPORTÉ — vérité routeur, miroir
agent.AntiVpnRulesPerHotspot, source unique), sel de version av-v1,
auto-réparation 6 h (pattern N°49), silence intégral pour un routeur
dont le gérant n'a jamais ouvert la carte (économie N°75), reprise
zombie (staleSentReadKinds), vague 88 en fermeture du deferred bucket.
Règles FILTER par serveur hotspot (interface lue SUR le routeur, foreach
/ip hotspot find, place-before=0 au-dessus d'un fasttrack éventuel) :
GRE (47) et ESP (50) coupés, UDP 500/4500/1701/1194/51820/2408 coupés
(IKE/NAT-T, L2TP, OpenVPN, WireGuard, Cloudflare WARP), TCP
1723/1194/9001/9030 coupés (PPTP, OpenVPN, Tor) ; miroir IPv6
best-effort (pattern N°85, on-error silencieux, non compté). Choix L4
délibéré : le port 53 (SafeWiFi reste maître du DNS), le NTP 123 et
l'UDP 443 ne sont JAMAIS touchés — l'UDP 443 porte les appels WhatsApp
(critiques en Côte d'Ivoire) et QUIC : la limite résiduelle (un tunnel
camouflé en HTTPS pur, ex. certains clients obfusqués) est écrite noir
sur blanc dans la footnote du module — un MVP sur routeur 128 Mo ne vend
pas de DPI. Frontend : AntiVpnCard (pattern ShieldCard, icône GlobeLock),
4e carte de la vue Protection (grille xl:grid-cols-4), 4e ligne du résumé
de l'onglet Système, bandeau ×4, verdict 4/4 (protectionScore/
protectionVerdict, source unique protection.ts), setRouterAntiVpn ;
i18n dans l'architecture en fragments du N°87 : clés tools.antivpn.*
(8) dans i18n-fr/tools.ts + i18n-en/tools.ts, protection.ofModules «{n}/4»
et summary.desc étendus dans les fragments protection, footnotes SafeWiFi
réécrites (« les VPN standards sont bloqués par le module Bloque-VPN —
seuls les tunnels camouflés en HTTPS pur (rares) peuvent encore le
contourner ») — parité FR/EN maintenue.

### Vocabulaire — promesse vendeur, limite assumée
« Bloque-VPN » / « VPN blocker » : « Les VPN ne peuvent plus contourner
vos protections : les tunnels connus sont coupés depuis le WiFi public. »
La footnote dit ce qui reste intact (navigation, heure des téléphones,
appels WhatsApp) et ce qui reste ouvert (tunnel camouflé en HTTPS pur,
rare) — la règle maison depuis N°80 : un blocage qui ne tient pas sa
promesse est pire que pas de blocage.

### Vérifié localement comme la CI
gofmt/vet/build verts ; go test -race complet 12 paquets verts (api
408 s, agent 1 s) dont les nouveaux tests : agent/antivpn_test.go
(4 règles forward IPv4 par hotspot + miroir IPv6, GRE et ESP présents,
listes de ports sans 443 et sans 53, off ne pose AUCUNE règle et retire
les deux familles, rapport rules+hs dynamiques, miroir de la constante
de comptage) et api/antivpn_test.go (silence du jamais-utilisé,
convergence activation/en-vol/ok-frais/horodatage-ancien/échec, retrait
après extinction, vérification 4×hs et hs illisible et niveau périmé,
sel av-v1 ≠ formule sans sel — garde-fou N°48 —, antivpn en fermeture
du deferred bucket 29<35<80<81<82<88) ; eslint 0, tsgo 0, build
production 13 routes (type-check actif N°84) ; script RouterOS généré
inspecté intégralement (2 180 octets pour on : retraits idempotents
v4+v6, variables avn/avi/avr sans collision avec swn/shn/fgn, fetch de
rapport avec citations propres, off = retrait seul + rapport rules=0).

## 2026-09-13 — N°87 : éclatement des dictionnaires i18n — les deux plus gros fichiers du projet (2 702 + 2 589 lignes) deviennent 90 fragments par domaine fusionnés par deux agrégateurs, contenu vérifié identique clé par clé

### N°87 — Contexte : l'audit « fichiers monolithiques »
Suite des points d'attention de l'audit pré-lancement (après N°84) :
`i18n.ts` (2 702 lignes, 167 Ko) et `i18n-en.ts` (2 589 lignes, 150 Ko)
étaient les deux plus gros fichiers du projet — et les plus TOUCHÉS :
chaque fonctionnalité ajoute des clés FR/EN, chaque commit de texte
nécessitait d'éditer un monolithe de 2 600+ lignes (source réelle
d'erreurs d'édition — la session précédente a subi une duplication
silencieuse en plein MultiEdit). L'éclatement était le refactor au
meilleur rapport valeur/risque : données pures, plat, parité FR/EN
vérifiable mécaniquement, zéro logique déplacée.

### Technique — 90 fragments + 2 agrégateurs, API publique inchangée
Chaque domaine de préfixe devient un fichier : une clé `foo.bar` vit
dans `i18n-fr/foo.ts` (FR) et `i18n-en/foo.ts` (EN) — 45 domaines par
langue, 2 460 clés chacun. Les agrégateurs `i18n.ts` (224 lignes) et
`i18n-en.ts` (108 lignes) importent les fragments et fusionnent par
spread : API publique strictement inchangée (`t`, `tf`, `localeOf`,
`useI18n`, `ensureEnDict`, `Lang`) — aucun des ~40 fichiers
consommateurs n'a bougé. Le lazy-load EN (N°78, ~37 Ko gzip hors bundle
initial) est conservé : le chunk dynamique `i18n-en` embarque
simplement ses fragments. L'ordre d'insertion des clés change
(regroupement par domaine) SANS effet : la résolution ne fait que des
lookups directs, aucune itération sur le dictionnaire (vérifié par
grep). Génération 100 % scriptée (jamais à la main) : parseur à
machine à états gérant entrées mono-lignes, valeurs multi-lignes
(4 clés `portal.*`) et commentaires attachés à l'entrée suivante ;
auto-vérification intégrée : les fragments générés sont re-parsés,
re-fusionnés et comparés CLÉ POUR CLÉ (valeur source brute) avec le
dictionnaire d'origine — 2 460/2 460 identiques des deux côtés, zéro
clé perdue, dupliquée ou altérée. Note CONTRACT-V2 (F11) mise à jour.

### Vérifications
Vérification scriptée du contenu (ci-dessus) ; eslint 0 ; tsgo 0 ;
build production avec typecheck actif — 13 routes ; E2E Playwright
complète sur stack locale (backend Go port 4000, frontend build
NEXT_PUBLIC_API_BASE=http://localhost:4000, port front patché 3015 le
temps du run puis restauré — piège N°81 respecté) : **9/9 verts**
(bootstrap, cycle revendeur ×3, Mode Vente ×5 — login PIN, pagination,
recherche, vente tactile, rapport + CSV) — les textes i18n réels
s'affichent dans le navigateur ; parité FR/EN re-vérifiée par le
script (mêmes clés des deux côtés avant comme après) ; déploiement
attendu : Vercel uniquement (diff frontend/docs — Render saute,
aucun diff backend/).

## 2026-09-13 — N°86 : Keep-alive — l'Option A (UptimeRobot) est en place : le monitor HTTP 5 min élimine l'hibernation Render, le workflow GitHub est désactivé (conservé comme repli) et le runbook reflète la vérité opérationnelle

### N°86 — Contexte : la recommandation du runbook est exécutée
Le gérant a créé le compte UptimeRobot Free (≈ 5 min, 0 $, sans carte
bancaire) et posé le monitor prescrit par le runbook §2 : HTTP(s),
interval 5 minutes, `https://mikcloud.onrender.com/`. Preuve d'efficacité
mesurée le jour même : dernier ping GitHub Actions à 11:06 UTC (retards
plateforme habituels — 5 h 17 entre les runs 114 et 115) ; à 13:18 UTC,
`GET /` répond HTTP 200 en 0,25 s sans cold boot — et structurellement,
une cadence de 5 min est inférieure au seuil d'hibernation Render
(15 min) : le service ne peut plus s'endormir. Le cron GitHub devenait
redondant : désactivé conformément au runbook (« Après activation »),
conservé dans le dépôt comme repli. La défense contre les cold boots est
désormais en profondeur : (1) UptimeRobot en couche primaire (élimine
l'hibernation, sonde de disponibilité avec historique en bonus),
(2) la couche frontend N°84 (réveil proactif + rejeu patient du login —
tout cold boot résiduel, p.ex. pendant un déploiement, reste quasi
invisible), (3) le workflow GitHub en repli dormant (réactivable en une
commande si le compte tiers est un jour abandonné).

### Technique — zéro code, zéro endpoint, zéro migration
Trois fichiers documentaires. RUNBOOK-KEEPALIVE.md : statut en tête
(Option A ACTIVE, date, hiérarchie des trois couches), section 2
retitrée « ACTIVÉE le 2026-09-13 » avec activation effective et preuve
mesurée, remplacement de « Après activation » (consigne) par le récit
de la désactivation réelle (commande, HTTP 204, état `disabled_manually`
vérifié, procédure de réactivation, avertissement « pousser le fichier
ne le réactive pas »), note Option B, recommandation passée au passé
composé, sources d'historique de §5 hiérarchisées (UptimeRobot primaire).
keepalive.yml : bannière « ⛔ DÉSACTIVÉ le 2026-09-13 » en tête (raison,
commandes de réactivation, même avertissement) — le fichier reste
syntaxiquement identique (commentaires seuls). CHANGELOG.md : cette
entrée. La désactivation effective a été effectuée AVANT le commit, par
l'API GitHub (PUT `/repos/ftechnologies18/mikcloud/actions/workflows/
keepalive.yml/disable` → 204, état vérifié : keepalive `disabled_manually`,
ci.yml et backup.yml toujours actifs).

### Vérifications
État des workflows GitHub vérifié par API après désactivation (keepalive
`disabled_manually` ; CI et backup actifs). Render : GET / HTTP 200 en
0,25 s à 13:18 UTC, soit 2 h 12 après le dernier ping GitHub — service
resté éveillé sans l'aide du cron. Neon : connexion pgx contrôlée,
compteurs vivants (commands en croissance, preuve de synchro
différentielle active). Déploiements attendus de ce commit : Vercel
redéploie un artefact fonctionnellement identique (diff docs
uniquement) ; Render NE redéploie PAS (le job deploy-render détecte
l'absence de diff sous `backend/` et saute) ; la CI tourne à vide sur
du texte (aucun code touché).

## 2026-09-13 — N°85 : SafeWiFi durci — le DNS filtré devient le SEUL chemin de sortie — une règle dstnat antérieure ne peut plus passer devant, le DNS chiffré connu (DoT/DoH) et l'IPv6 sont coupés depuis le WiFi public

### N°85 — Contexte : un site adulte accessible sur un routeur « Protection familles » active
Constat du gérant, vérifié sur le routeur client : xvideos.com restait
accessible depuis le WiFi public d'un site en niveau « family » (AdGuard
Family), protection confirmée appliquée (rules=2, sig posée). L'enquête a
établi que le résolveur fonctionne (AdGuard Family bloque bien xvideos.com —
prouvé par requête DNS directe : IP de blocage 94.140.14.35 renvoyée) et que
les 2 règles NAT existaient réellement sur le routeur. La faille était
ailleurs : trois échappatoires, dont une de NOTRE fait :
(1) les règles NAT se posaient en FIN de table — une règle dstnat
antérieure (redirect DNS hérité d'une config Mikhmon ou d'un tutoriel
hotspot) interceptait le port 53 AVANT MikCloud, silencieusement : la
signature ne comptait que la PRÉSENCE des règles marquées (rules=2), jamais
leur EFFECTIVITÉ ; (2) le DNS chiffré — DoT (tcp/853, « DNS privé »
Android) et DoH (tcp/443 navigateurs) — contourne toute redirection de
port 53 (limite documentée du MVP N°80) ; (3) l'IPv6 — le NAT est IPv4, un
appareil dual-stack résolvait hors de portée des règles. Un blocage qui ne
tient pas sa promesse est pire que pas de blocage : il vend une sécurité
fictive. Le durcissement ferme les trois échappatoires connues.

### Technique — quatre corrections, un seul contrat d'API inchangé
buildSafeWifi (agent.go) : (1) les 2 règles NAT dst-nat passent en TÊTE de
table (place-before=0 — miroir Shield N°81 / FamilyGuard N°82) ; (2) par
serveur hotspot (interface lue SUR le routeur, foreach /ip hotspot find —
pattern N°81), 2 règles FILTER : DoT tcp/853 drop + DoH tcp/443 drop vers
l'address-list mikcloud-safewifi-doh (24 endpoints publics v4 : Cloudflare,
Google, Quad9 filtré ET non filtré, AdGuard default/family/non-filtré,
OpenDNS+FamilyShield, CleanBrowsing, Yandex, Comodo, DNS.SB — les
navigateurs en mode automatique retombent sur le DNS simple quand leur DoH
échoue : port 53 → redirigé → filtré) ; (3) IPv6 best-effort : DNS v6
(tcp+udp 53), DoT v6 (tcp/853) et DoH v6 (tcp/443 vers la liste v6 de 10
endpoints) coupés depuis l'interface hotspot (/ipv6 firewall) — on-error
silencieux : un routeur sans pile IPv6 n'a ni règles à poser ni
échappatoire à fermer ; (4) retraits idempotents étendus aux FILTER et aux
listes (v4+v6) — plus d'objets orpheliers au passage à off. Rapport : le
compte d'objets marqués IPv4 (nat + filter + address-list, valeur
DYNAMIQUE côté routeur) ET le nombre de hotspots (pattern Shield N°81) ;
la vérification cloud exige rules == 2 + len(listeDoHv4) + 2×hs rapporté
(agent.SafeWifiRulesExpected — hs illisible → pas de sig, re-file
prudent). Bump du sel sw-v1 → sw-v2 : tout le parc reçoit la nouvelle
forme au check-in suivant (≤ 45 s console ouverte, ≤ 180 s en veille),
sans intervention — CYBER S.C (family) et ProMax WIFI (threats) inclus.
Zéro endpoint neuf, zéro migration, zéro changement de contrat
(PUT /api/routers/{id}/safewifi {level} inchangé) — la console ne voit
aucune différence, le routeur change de bras de fer.

### Vocabulaire — la promesse devient honnête
La footnote de la carte (tools.safewifi.footnote FR/EN) dit désormais la
vérité complète : les échappatoires connues sont fermées (DNS privé
Android et DNS sécurisé navigateur bloqués pour que le filtre s'applique),
seul un VPN peut encore contourner — limite résiduelle assumée et écrite
noir sur blanc (un endpoint DoH exotique hors liste reste joignable, le
DPI est hors de portée d'un routeur 128 Mo).

### Vérifications — localement comme la CI
gofmt/vet/build verts ; go test -race complet : 12 paquets verts (api
397 s sous -race, agent 1 s) dont les tests SafeWiFi étendus — 2 dst-nat
place-before=0 par niveau actif, présence de chaque endpoint DoH v4 dans
la liste, blocages DoT/DoH/IPv6 par hotspot, off ne pose AUCUN objet
(NAT/FILTER/address-list) et retire la liste DoH, rapport rules+hs
dynamique, SafeWifiRulesExpected miroir du comptage, sig sw-v2 ≠ sw-v1
(garde-fou re-pousse N°48), vérification du retour : compte exact requis,
hs divergent ou illisible → pas de sig. Frontend : eslint 0, tsgo 0,
build production 13 routes. API contractuelle inchangée — aucun test E2E
n'avait à bouger.

## 2026-09-13 — N°84-bis : TestVouchersStatsServerSide devient déterministe — le test flaky « active = 2, voulu 3 » déraciné (routeur du semis en mode real, la simulation de Tick ne le touche plus)

### N°84-bis — Contexte : le run CI de N°84 a échoué sur un test backend que N°84 ne touchait pas
Le push N°84 (100 % frontend : next.config, login-screen, api.ts, i18n,
package.json, workflows, docs) a fait rougir le job « Backend Go » sur
`TestVouchersStatsServerSide` — `etag_stats_test.go:138 : active = 2,
voulu 3`. Diagnostic : **flakiness préexistant sans lien avec N°84**,
reproduit et déraciné. Mécanisme : le test sème 3 vouchers actifs sur le
routeur SIMULÉ de `seedWifiEnv` ; le moteur de démo de `store.Tick`
crée une session aléatoire (~30 % par appel) depuis un voucher actif sans
session sur un routeur simulé — sémantique 1er login : `Status → "used"`
+ `UsedAt` + ancrage de validité + vente comptée au revendeur le cas
échéant. Deux garde-fous masquaient le défaut : (1) la garde de Tick
(skip si < 2 s depuis le dernier tick) — en local rapide, l'écart
register→stats reste sous la garde, Tick ne tourne jamais, test vert ;
(2) la probabilité de 30 %. En CI `-race` (runner lent), l'écart DÉPASSE
la garde : Tick démarre et une exécution sur ~ trois flippe un des 3
actifs. Reproduction locale : `go test -race -run TestVouchersStatsServerSide
-count=30` → échecs « active = 2, voulu 3 » ; le même -count sans -race
(40 runs) → 40 verts — signature exacte du défaut timing-dépendant.

### Correctif — le routeur du semis passe en mode "real", la simulation n'a plus de prise
Après `seedWifiEnv`, le test bascule le routeur `rt-wifi-test` en mode
`"real"` avant de semir les vouchers. Justification (lue dans le code) :
le moteur de sessions de Tick ne touche QUE les routeurs `simulated`
(les sessions des routeurs réels vivent au rythme du read_state, aucune
dynamique simulée — audit clignotement) ; `enforceExpired` ne file des
commandes qu'aux routeurs `agent` (mode real : le statut cloud suffit) ;
`applyExpiry` et les filtres/compteurs de `/api/vouchers/stats` sont
indépendants du mode routeur. Le test garde donc exactement la même
couverture (filtres kind/account/holder, statuts résolus, stockValue)
sur un semis désormais immuable. Aucune ligne de production touchée —
le défaut vivait dans le HARNES de test, pas dans le moteur (le
comportement simulé « 1er login → used » est voulu, c'est la démo).

### Vérifications
`go test -race -run TestVouchersStatsServerSide -count=40` → 40/40 verts
(110 s, déterministe — contre ~1 échec sur 3 avant) ; suite backend
complète `-race -timeout 30m` : 11 paquets verts (api 406 s) ; gofmt/vet
propres. Périmètre : 1 fichier de test, 0 ligne de production, 0
endpoint, 0 migration — déploiement Render attendu (diff backend/) mais
artefact fonctionnellement identique (les tests ne sont pas compilés
dans le binaire).

## 2026-09-13 — N°84 : hygiène pré-lancement — le build Vercel type-checke, les cold boots deviennent invisibles, 16 dépendances mortes évacuées

### N°84 — Contexte : trois faiblesses structurelles repérées à l'audit de lancement
L'audit d'architecture a mis en lumière trois fragilités qui n'ont jamais
été des choix délibérés mais des héritages ou des constats subis :
(1) `typescript.ignoreBuildErrors: true` dans `next.config.ts` venait du
**template initial** (commit 8cd2e40 — N°38 l'avait laissé « intact » sans
le questionner) : or le webhook Vercel déploie **dès le push `main`,
indépendamment de la CI** — une régression de types pouvait donc atteindre
la production avant même que le job tsgo de la CI ne rougisse ; (2) le
keep-alive Render mesuré à 99–288 min de retard (RUNBOOK-KEEPALIVE) rend
les cold boots de 30–90 s **une expérience réelle** pour le gérant qui se
connecte le matin : timeout N°78 à 20 s → erreur brutale « signal timed
out » → échec perçu ; (3) `prisma` + `@prisma/client`, `next-auth`,
`z-ai-web-dev-sdk`, `@mdxeditor`, `date-fns`… **16 dépendances sans le
moindre import** dans src/ ni e2e/ — installées, auditées par Dependabot,
pesant sur le lockfile, pour rien.

### Barrière de types — le build devient le troisième verrou
Retrait d'`ignoreBuildErrors` : le build `next build` type-checke
désormais (tsc), en plus du `tsgo --noEmit` de la CI locale et du job CI.
Triple barrière : locale (typecheck), CI (tsgo), build (tsc). Une
régression de types ne peut plus passer inaperçue à aucun étage — le
webhook Vercel déploie toujours en premier, mais il déploie un artefact
qui a passé le tsc. Vérifié immédiatement : build complet vert, 13 routes,
« Finished TypeScript in 14.8 s », zéro erreur. (N°38 conservé : pas de
retour de `output: "standalone"`.)

### Cold boot — le rendre invisible plutôt que l'empêcher (N°84, couche frontend)
Le keep-alive GitHub reste un filet partiel (retards plateforme
non contractuels) ; UptimeRobot/Render Starter restent LES correctifs de
fond (RUNBOOK inchangé sur ce point). Mais entre deux pings retardés,
l'utilisateur ne doit plus payer le cold boot de sa poche. Deux
garde-fous dans l'écran de connexion :
**réveil proactif** — `wakeBackend()` dans `api.ts` (fire-and-forget vers
`GET /` = `handleHealth`, garde module = un seul ping par chargement de
bundle, échec silencieusement ignoré, paramètre anti-cache) part au
premier montage : le serveur Render démarre **pendant que le gérant tape
ses identifiants** ; **rejeu patient** — un échec RÉSEAU (timeout/
connexion — pas une réponse HTTP d'erreur, qui suit la logique normale
401/totp_required) sur le login déclenche UNE seconde tentative à 75 s
avec un message explicite (« Réveil du serveur cloud en cours… », i18n
FR/EN + `login.networkError` humain remplaçant le DOMException brut).
Le login est le SEUL POST autorisé à se rejouer — aucune écriture métier,
au pire deux sessions JWT (la première expire) ; génération de vouchers
et e-mails ne se rejouent JAMAIS : un timeout peut masquer un traitement
serveur réussi. Workflow keepalive durci au passage : `permissions: {}`
(zéro accès GitHub requis — ping HTTP seulement), `cancel-in-progress:
true` (un ping bloqué est supplanté par le suivant), latence mesurée au
résumé de run. RUNBOOK-KEEPALIVE : nouvelle section « 1-bis. Couche
frontend — résilience cold boot ».

### Dépendances — 16 paquets morts évacués, lockfile −893 lignes
Audit exhaustif import par import (src/ + e2e/ + configs racine) :
`prisma`, `@prisma/client` (le backend utilise pgx — le Prisma frontend
était un vestige de template), `next-auth` (auth maison JWT),
`z-ai-web-dev-sdk`, `@mdxeditor/editor`, `next-intl` (i18n maison — un
commentaire du code le disait déjà), `@dnd-kit/*` (×3), `date-fns`
(formatage maison `format.ts`), `react-markdown`,
`react-syntax-highlighter`, `@reactuses/core`, `@tanstack/react-table`
(tableaux à main), `uuid` (seul `crypto.randomUUID` natif est utilisé),
`@hookform/resolvers` (aucun zodResolver) — **zéro import pour chacun**.
Conservés malgré l'absence d'import direct : `react-dom`/`next`
(framework), `sharp` (optimisation next/image, 5 fichiers). Bénéfices :
lockfile −893 lignes, install CI/Vercel plus courte, surface Dependabot
réduite (prisma et next-auth généraient des PR de bump pour rien).

### Vérifications
Frontend : eslint 0, tsgo 0, build production **avec typecheck actif**
vert (13 routes, TypeScript 14.8 s), `bun install --frozen-lockfile`
reproductible. Backend inchangé : gofmt/vet/build/tests 11 paquets verts.
E2E navigateur autonome bout-en-bout sur stack locale (backend Go port
4000 mode JSON + build `NEXT_PUBLIC_API_BASE` — le piège documenté N°81
respecté) : écran de connexion rendu → **ping de réveil constaté dans les
logs backend au montage** (GET / → 200) → login mot de passe erroné →
toast « Identifiants invalides » du serveur + tremblement de carte →
login correct → redirection console, requêtes dashboard 200 → zéro erreur
console → contrôle visuel VLM sans défaut (mise en page, sidebar, KPI).
Cold boot simulé impossible en local (serveur chaud) — le rejeu patient
est couvert par le typage strict et la symétrie du mécanisme (ApiError vs
erreur réseau). i18n : +2 clés FR, +2 EN (parité maintenue).

## 2026-09-13 — N°83 : la Protection sort de l'ombre — vue « Protection » dans la navigation principale, bandeau sur le tableau de bord et vocabulaire vendeur — les modules sécurité deviennent des arguments de vente visibles en un clic

### N°83 — Contexte : des arguments de vente enterrés dans une zone de configuration
Constat du gérant du projet, vérifié dans le code : les 3 modules sécurité
(SafeWiFi N°80, Shield N°81, FamilyGuard N°82) vivaient au fond du 4e onglet
« Système » de la fiche routeur, elle-même au fond de la zone Paramètres —
**5 à 6 interactions et 3 changements de contexte** pour toucher les seules
fonctions qui différencient MikCloud d'un simple outil à vouchers. Le tableau
de bord affichait 6 KPI métier et **zéro** état de protection : un gérant qui
se connecte chaque matin ne voyait jamais que son WiFi était protégé — ou ne
l'était pas. La cible réelle (gérants de maquis/cybercafés ivoiriens,
mobile-first, sans notion réseau) ne trouvera jamais ce qu'elle ne voit pas.
Plan UX/UI proposé puis validé : (1) une vue « Protection » dans la section
Supervision de la sidebar principale ; (2) un bandeau de protection sur le
tableau de bord (rappel quotidien de la valeur + CTA) ; (3) un vocabulaire
vendeur en bénéfices gérant. HomeNet (réseau privé non-hotspot) reste en
attente du feu vert — rien n'a été touché au-delà du périmètre validé.

### Produit — le verdict en 5 secondes, les contrôles à portée de main
Nouvelle vue `/app/protection` (3e item de Supervision, icône bouclier) :
**verdict par routeur calculé automatiquement** — « Bien protégé » (3/3
modules actifs), « À renforcer » (partiel), « Non protégé » (0/3) — avec le
compte « n/3 protections actives ». Les 3 cartes (Sites dangereux bloqués,
Anti-piratage du WiFi, Couvre-feu internet) sont **actionnables directement
dans la vue**, sans navigation imbriquée. Mono-routeur : l'étape de sélection
est sautée (le cas de la quasi-totalité des comptes) ; multi-sites : sélecteur
shadcn dont la sélection vit dans l'URL (`/app/protection/<id>`, pattern
fiche routeur — Retour navigateur et liens directs fonctionnels, segment
orphelin re-normalisé). L'onglet Système de la fiche routeur garde un
**résumé compact** (verdict + 3 lignes d'état + CTA « Ouvrir la vue
Protection » qui rouvre CE routeur) — **aucun contrôle dupliqué**. Le tableau
de bord gagne le bandeau Protection sous la bannière abonnement : vert et
fier quand tout est actif, ambre avec CTA « Renforcez la protection » sinon,
« Votre WiFi n'est pas protégé » au pire — **aucun bandeau sans routeur en
mode agent** (comptes vides : zéro bruit visuel).

### Technique — 100 % frontend, le verdict calcule ce que l'API expose déjà
Aucun endpoint neuf, aucune migration, **zéro octet supplémentaire pour le
parc** : le verdict lit les champs de `GET /api/routers` (`safeWifiLevel`,
`shieldLevel`, `familyGuardSpec` — livrés par N°80/81/82). Helpers purs dans
`lib/hotspot/protection.ts` (source unique partagée vue/bandeau/résumé) :
`safeWifiLevelOf`, `shieldOn`, `parseFamilyGuardSpec` + `familyGuardActiveNow`
(déplacés de router-tools.tsx, miroir exact de `FamilyGuardConfig.ActiveAt`
côté Go — UTC, passage de minuit, jour de début), `protectionScore`,
`protectionVerdict`. Les 3 cartes quittent router-tools.tsx pour
`parts/protection-cards.tsx` (code N°80/81/82 inchangé : mêmes mutations,
toasts, invalidations `["/api/routers","/api/dashboard"]`, gardes mode
agent) + `ProtectionVerdictBadge` et `ProtectionSummaryCard` ; le bandeau
vit dans `parts/protection-banner.tsx` (cache partagé `["/api/routers"]`,
rafraîchi 60 s — l'état change rarement, les mutations invalident la clé).
Enregistrement complet de la vue : ViewId `protection` (types.ts), slug
`/app/protection` + vue adressable (view-path.ts, DETAIL_VIEWS), rang
minimal 2 gérant+ (roles.ts — miroir de l'ancien onglet Système de la fiche
routeur et des PUT `safewifi|shield|familyguard` derrière JWT + accountScope),
entrée Supervision de NAV_SECTIONS (nav.ts — la palette de recherche topbar
la suit automatiquement), import dynamique + map VIEWS + aria de vue
(app-shell.tsx). Économie N°75 préservée : aucune requête agent n'est
déclenchée par la vue, le check-in reste le seul canal.

### Vocabulaire — parler gérant, pas ingénieur
Les clés i18n `tools.*` restent stables (zéro refonte de code), leurs valeurs
passent en bénéfices : « Protection WiFi public » → **« Sites dangereux
bloqués »** (« Vos clients naviguent sans virus ni arnaques : les sites piégés
sont bloqués avant d'atteindre leurs téléphones »), « Bouclier réseau » →
**« Anti-piratage du WiFi »** (« Personne ne peut s'introduire dans votre
routeur depuis le WiFi public : les outils des pirates sont neutralisés »),
« Couvre-feu internet » conservé (« L'internet s'éteint quand vous le
décidez : la nuit, à la fermeture, pendant les heures d'étude. Les vouchers
restent valides »). Les résolveurs et ports techniques (Quad9, AdGuard,
Winbox, 8291…) disparaissent des libellés — la transparence des limites
reste (footnotes « se répare seul », « le WiFi reste opérationnel »).
Nouvelles clés `protection.*` (verdicts, bandeau, résumé, sélecteur) :
**21 clés FR + 21 EN**, parité des dictionnaires vérifiée 2454/2454.

### Vérifications — le parcours complet prouvé en conditions réelles
eslint 0, tsgo 0, build production ✓ (13 routes), go vet/build backend
inchangé ✓. E2E navigateur autonome (binaire Go + `next start` enfants du
script, backend JSON frais) : inscription gérant + 2 routeurs agents (plan
passé en illimité via l'API plateforme P2 pour l'exercice multi-sites) →
login réel → **bandeau dashboard « Votre WiFi n'est pas protégé » + 0/6 · 2
routeurs** → CTA « Gérer la protection » → vue `/app/protection` (verdict
« Non protégé », 0/3, sélecteur rendu, 3 cartes au vocabulaire vendeur) →
activation Shield + SafeWiFi « Menaces bloquées » depuis la vue (toasts,
verdict « À renforcer », 2/3) → sélecteur vers le 2ᵉ routeur (verdict 0/3,
URL adressable) → fiche routeur → onglet Système → **résumé compact sans
contrôle dupliqué** → CTA « Ouvrir la vue Protection » → retour ciblé sur le
1ᵉʳ routeur → bandeau dashboard mis à jour « Renforcez la protection » 2/6 →
**check-in agent simulé : commandes safewifi ET shield servies, rapports
vérifiés (rules=2, rules=5 hs=1), signatures posées, second check-in
silencieux** → mobile 390 px scrollWidth=390 sans débordement → **0 erreur
console**. Suite Playwright « Mode Vente » du repo : **9/9 verts** (ports
ponctuellement patchés 3015 car le bac à sable occupe 3000, fichiers
restaurés — diff git vide vérifié). Contrôle visuel VLM de la capture :
aucun défaut (alignements, contrastes, grille 3 colonnes confirmés).

## 2026-09-13 — N°82 : FamilyGuard — couvre-feu internet du WiFi public (fenêtre horaire programmée) — Phase 3 de la roadmap sécurité, lancement commercial différé au premier revenu

### N°82 — Contexte : la gamme sécurité manquait le QUAND
Phase 3 de la roadmap sécurité validée. SafeWiFi (N°80) filtre QUOI
(menaces, contenus), Shield (N°81) protège CONTRE QUI (administration,
propagation) — FamilyGuard décide **QUAND l'internet du WiFi public est
accessible** : le gérant programme une fenêtre horaire (ex. 22:00 → 06:00
tous les soirs, ou les heures de fermeture du site) pendant laquelle
l'internet des clients est coupé — nuit des enfants en salle familiale,
fermeture du maquis, heures d'étude du cybercafé. Décision produit
confirmée par le gérant du projet : **le module est livré complet et
testé dès maintenant, mais son lancement COMMERCIAL (vendre l'add-on
+2 000-5 000 FCFA/mois) est volontairement différé au premier revenu** —
la carte console l'affiche « module en phase de test, gratuit pendant le
pilote ». Promesse inchangée : SANS réglage technique, sur tout le parc
(MIPS 128 Mo compris), 0 FCFA d'infrastructure, non intrusive.

### Technique — une règle filter par hotspot, le cloud est l'horloge
`buildFamilyGuard` (agent) pose, après retrait idempotent des règles
marquées `mikcloud-familyguard`, exactement **1 règle filter par serveur
hotspot** pendant la fenêtre : `chain=forward`, `place-before=0` (en tête
de chaîne, **au-dessus d'un éventuel fasttrack d'établies** — les
connexions EN COURS sont coupées immédiatement, pas seulement les
nouvelles), `in-interface` = l'interface du hotspot **lue sur le routeur**
(`:foreach fgh in=[/ip hotspot find]` — s'adapte à toute topologie,
pattern N°81), `action=reject reject-with=icmp-network-unreachable`
(échec IMMÉDIAT côté appareil — pas de navigateur qui tourne dans le
vide). La page du portail captif reste accessible (chain=input, servie
par le routeur) : les vouchers restent validables pendant le couvre-feu,
seul l'internet est coupé. Le réseau du gérant et le trafic propre du
routeur ne sont jamais touchés ; `/ip firewall nat` et `/ip dns` non
plus. **Arbitrage central (documenté) : l'ÉTAT désiré — couvre-feu en
cours ou non — est calculé PAR LE CLOUD à chaque check-in, en UTC
(== heure d'Abidjan GMT, la Côte d'Ivoire n'applique pas l'heure
d'été)** : l'horloge routeur n'est JAMAIS consultée (un routeur sans
NTP — fréquent sur le terrain — verrait le couvre-feu partir à la
mauvaise heure via le paramètre natif `time=` de RouterOS).
Contrepartie assumée : la bascule s'applique au check-in suivant
(≤ 45 s console ouverte — attention N°75, ≤ 180 s en veille), et un
routeur hors-ligne pendant une frontière converge vers l'état
« maintenant » à son retour (aucune commande périmée en attente).
0 Mo de RAM (règle sans état), 0 FCFA.

### Modèle — fenêtre canonique, trois colonnes idempotentes
`Router` gagne `FamilyGuardSpec`/`FamilyGuardSig`/`FamilyGuardAppliedAt`
(3 `ALTER TABLE ADD COLUMN IF NOT EXISTS`, pattern N°80/N°81). Le spec
est une chaîne canonique `<enabled>|<HH:MM>|<HH:MM>|<1111111>` (ex.
`1|22:00|06:00|1111111` ; days = lundi→dimanche) ; `""` = jamais utilisé
→ silence intégral (économie N°75). `FamilyGuardConfig` (models.go) porte
la logique pure : validation stricte (heures `HH:MM`, début ≠ fin,
7 jours 0/1 dont un actif) et `ActiveAt(now)` — bornes début inclus /
fin exclue, **passage de minuit** (22:00→06:00 : la portion du matin
appartient à la fenêtre partie la veille), **jour = jour de DÉBUT** de la
fenêtre (« vendredi » + 22:00→06:00 couvre jusqu'au samedi matin même si
le samedi n'est pas coché). API console : `PUT /api/routers/{id}/familyguard`
`{enabled,start,end,days}` (validation stricte, scope compte, agent-only,
garde P3, journal d'activité avec l'acteur et le résumé de fenêtre).

### Convergence — la signature porte l'ÉTAT, les frontières se retournent seules
`ensureFamilyGuardLocked` au check-in, contrat N°80/N°81, avec la
spécificité temporelle : spec vide = jamais utilisé → **RIEN** ;
l'état désiré est **recalculé à CHAQUE check-in** et la signature = hash
(sel `fg-v1` + spec + **état**) — quand la frontière de fenêtre est
franchie (22:00, 06:00…), la signature attendue change, le check-in
suivant re-file la bascule : **le couvre-feu se lève le matin sans autre
orchestration**. Re-file au changement de spec et à l'évolution du sel,
auto-réparation 6 h (`familyGuardRefresh`, pattern N°49), dédoublonnage
queued/sent. `CmdFamilyGuard` rejoint `staleSentReadKinds` (idempotent)
et le bucket différé en FERMETURE du batch (vagues 29 < 35 < 80 < 81 <
**82**). Le rapport échoe le compte de règles marquées présentes ET le
nombre de serveurs hotspots ; la signature n'est posée que si
**rules == 1 × hotspots rapportés** (0 si levé), si le spec rapporté est
toujours celui du routeur, ET si **l'état désiré est toujours courant**
(une frontière franchie pendant le vol ne fige pas un état périmé —
pattern « niveau toujours courant » N°80) ; `hs` illisible → pas de sig.

### Console — une carte planificateur dans l'onglet Système
`FamilyGuardCard` sous `ShieldCard` (outils routeur → Système) : Switch
« Activer le couvre-feu », champs Début/Fin (`input type=time`), 7 puces
de jours (lun→dim, `aria-pressed` + libellés complets), bouton
« Enregistrer le planning » (inactif sans modification), badge « Actif »,
statut temps réel — « **En cours — internet coupé jusqu'à HH:MM** » ou
« Programmé : HH:MM → HH:MM » (miroir exact de `ActiveAt`, calculé en UTC
== heure d'Abidjan comme le cloud) — et footnote d'honnêteté (heure
d'Abidjan GMT, jour = jour de DÉBUT, portail accessible, module en phase
de test — gratuit pendant le pilote). `familyGuardSpec` sur
`RouterDevice`, `setRouterFamilyGuard` dans api.ts, 27 clés i18n FR +
27 EN (« Couvre-feu internet »).

### Vérifications
Tests : 14 nouveaux — 5 model (logique pure de `ActiveAt` : fenêtre
intra-jour bornes incluses/exclues, passage de minuit tous les jours,
sémantique jour de DÉBUT sur fenêtre nocturne, désactivé/jour non coché,
aller-retour du spec canonique + rejet des formes invalides) ; 3 agent
(actif : retrait idempotent puis exactement 1 règle forward reject par
hotspot avec interface dynamique et place-before=0, reject-with ICMP,
jamais nat ni dns, jamais d'horloge routeur (`/system clock`, `time=`),
rapport rules+hs dynamique ; inactif : retrait seul ; normalisation du
payload) ; 6 api (silence si jamais utilisé, convergence complète
programmation → vol → retour ok → réparation 6 h → échec retenté →
**frontière d'état re-filée sans changement de spec**, retrait après
désactivation, vérification 1×hotspots avec spec périmé / état périmé /
hs illisible → échec, sel de version + états distincts, ordre du batch
29<35<80<81<82). Suite complète 11 paquets verts, -race ciblé vert,
gofmt/vet/build propres ; frontend eslint 0, tsgo 0, build production ✓ ;
E2E Playwright 9/9 verts (ports 3012/4000 ponctuellement patchés car le
bac à sable occupe 3000, fichiers restaurés — diff git vide). Vérification
navigateur bout-en-bout (Playwright autonome, backend + frontend enfants
du script) : inscription gérant + routeur agent → login réel → /app/routers
→ fiche → onglet Système → cartes SafeWiFi + Shield + **FamilyGuard**
rendues → fenêtre saisie (±30 min autour de maintenant) → Switch → toast
+ spec `1|HH:MM|HH:MM|1111111` persisté + statut « En cours — internet
coupé jusqu'à HH:MM » affiché → check-in agent simulé (GET /agent/cmd) →
script familyguard servi (12 586 octets, règle reject marquée) → rapport
rules=1 hs=1 → **signature posée** → second check-in : silence — 0 erreur
console.

## 2026-09-13 — N°81 : Shield — bouclier réseau du WiFi public (ports d'administration + vecteurs malveillants bloqués) — Phase 2 de la roadmap sécurité

### N°81 — Contexte : le WiFi public expose le routeur ET les clients
Phase 2 de la roadmap sécurité validée. SafeWiFi (N°80) filtre les noms
de domaine dangereux ; restait la couche RÉSEAU : un client connecté au
WiFi public peut tenter l'administration du routeur (Winbox, SSH,
telnet, API — le vecteur d'attaque classique d'un hotspot), et les
appareils infectés se propagent par SMB/NetBIOS vers les autres clients
et le réseau du gérant (caisse, NAS). Promesse produit inchangée : une
protection SANS réglage technique, sur tout le parc (MIPS 128 Mo
compris), à 0 FCFA d'infrastructure, non intrusive — le réseau du
gérant et le routeur lui-même ne changent pas de comportement.

### Technique — cinq règles filter par hotspot, interface lue sur le routeur
`buildShield` (agent) pose, après retrait idempotent des règles marquées
`mikcloud-shield`, exactement **5 règles filter par serveur hotspot**,
en tête de chaîne (`place-before=0`) et ciblées sur `in-interface` =
l'interface du hotspot **lue sur le routeur au moment de l'exécution**
(`:foreach h in=[/ip hotspot find]` → `/ip hotspot get $h interface`) :
le script s'adapte à toute topologie (wlan1, bridge-hotspot…) et suit un
renommage d'interface à la réparation 6 h. Les règles : input ×2 —
**administration bloquée depuis le WiFi** (tcp 21/22/23/8291/8728/8729,
udp 8728/8729) ; forward ×3 — **connexions invalides droppées**,
**SMB/NetBIOS bloqués** (tcp 135-139/445, udp 137-139). Le réseau du
gérant (hors interface hotspot) et le trafic propre du routeur
(`chain=output` : check-in agent, DNS sortant) ne sont **jamais
touchés** — `/ip firewall nat` et `/ip dns` non plus (SafeWiFi N°80
reste maître du port 53). Coût : 0 Mo de RAM (des règles filter, pas
d'état), 0 FCFA. Limites documentées (MVP) : le blindage couvre le
trafic IPv4 traversant le routeur — l'isolation L2 de deux appareils
d'un même pont relève du réglage du pont (`use-ip-firewall`, coûteux
sur MIPS), hors de portée d'un MVP non intrusif ; l'administration reste
possible depuis MikCloud (agent, connexions sortantes) et depuis le LAN
du gérant.

### Modèle — un booléen par routeur, trois colonnes idempotentes
`Router` gagne `ShieldLevel` (off/on ; `""` = antérieur au N°81 → off
implicite), `ShieldSig` et `ShieldAppliedAt` — `ALTER TABLE ADD COLUMN
IF NOT EXISTS` ×3, pattern SafeWiFi N°80. L'API console :
`PUT /api/routers/{id}/shield` `{level}` (validation stricte off/on,
scope compte, agent-only, garde P3, journal d'activité avec l'acteur).

### Convergence — silence si jamais utilisé, vérification 5 règles × hotspots
`ensureShieldLocked` au check-in, contrat exact du N°80 : off + sig vide
= jamais utilisé → **RIEN** (économie N°75 entière) ; signature = hash
(sel `sh-v1` + niveau), re-file au changement et à l'évolution du sel,
auto-réparation 6 h (`shieldRefresh`, pattern N°49). `CmdShield`
rejoint `staleSentReadKinds` (idempotent) et le bucket différé en
FERMETURE du batch (vagues 29 < 35 < 80 < **81**). Le rapport du script
échoe DEUX valeurs dynamiques — le compte de règles marquées présentes
ET le nombre de serveurs hotspots trouvés — et la signature n'est posée
que si **rules == 5 × hotspots rapportés** (le cloud ne connaît pas la
topologie : c'est le routeur qui la rapporte, vérité routeur), et
uniquement si le niveau rapporté est toujours courant. Un `hs` illisible
→ vérification impossible → pas de sig → re-file prudent.

### Console — une carte à bascule dans l'onglet Système
`ShieldCard` sous `SafeWifiCard` (outils routeur → Système) : Switch
« Activer le bouclier » + badge « Actif » + footnote de disponibilité
(réseau du gérant intact, administrable via MikCloud/LAN) ;
`shieldLevel` sur `RouterDevice`, `setRouterShield` dans api.ts,
7 clés i18n FR + 7 EN (« Bouclier réseau »).

### Vérifications
Tests : 9 nouveaux — 3 agent (niveau on : retrait idempotent puis 5
règles filter par hotspot avec interface dynamique et place-before=0,
ports admin/malveillants présents, rapport rules+hs dynamique, jamais
nat ni dns ; off/inconnu : retrait seul ; normalisation du payload) ;
6 api (silence si jamais utilisé, convergence complète activation puis
échec puis réparation 6 h, retrait après extinction, vérification
5×hotspots avec hs illisible → échec et niveau périmé → jamais figé,
sel de version, ordre du batch 29<35<80<81). Suite complète 11 paquets
verts, -race ciblé vert, gofmt/vet/build propres ; frontend eslint 0,
tsgo 0, build production ✓ ; E2E Playwright 9/9 verts. Vérification
navigateur bout-en-bout (Playwright autonome : backend + frontend
enfants du script, le bac à sable tuant les processus entre les
invocations) : login gérant réel → onglet Système → cartes SafeWiFi +
Shield rendues → bascule du Switch → toast + shieldLevel persisté →
check-in agent simulé → script shield servi (13 356 octets) → rapport
rules=5 hs=1 → signature posée — 0 erreur console. ⚠️ Piège E2E local
documenté : le build de test DOIT être fait avec
`NEXT_PUBLIC_API_BASE=http://localhost:4000` (comme la CI) — sans lui,
`API_BASE` vide active le mode passerelle sandbox (`XTransformPort=4000`
sur localhost) et les appels UI tombent en 404 sur Next.js.

## 2026-09-13 — N°80 : SafeWiFi — protection DNS du WiFi public (filtrage par redirection, 3 niveaux) — Phase 1 de la roadmap sécurité

### N°80 — Contexte : le WiFi public du client est la première porte d'entrée des menaces
La roadmap sécurité (validée) partait d'un constat terrain : les sites
servis — restaurants, maquis, cafés, cybercafés — offrent un WiFi public
où les clients du gérant exposent leurs téléphones aux sites piégés,
logiciels malveillants et arnaques, et où une salle familiale n'a aucun
contrôle sur les contenus adultes. La promesse produit : une protection
SANS réglage technique, fonctionnelle sur tout le parc (du hAP ax³ ARM64
au RB951Ui-2HnD MIPS 128 Mo), à 0 FCFA d'infrastructure — l'objectif
« 0 coût jusqu'au premier client payant » reste la contrainte maîtresse.
MVP validé : le **filtrage DNS par redirection** (le filtrage s'exécute
chez le résolveur public anycast, pas sur le routeur) — l'IPS/DPI/AV
on-router restent proscrits (hors de portée d'un 128 Mo).

### Technique — deux règles NAT, 0 Mo de RAM, 0 FCFA
`buildSafeWifi` (agent) pose, après le retrait idempotent des règles
marquées `mikcloud-safewifi`, exactement deux règles `dst-nat` (udp + tcp
port 53) réécrivant TOUT le DNS transitant vers le résolveur du niveau :
`threats` → **Quad9 9.9.9.9** (malwares, phishing, arnaques) ;
`family` → **AdGuard Family 94.140.14.15** (+ contenus adultes,
publicités) ; `off` → retrait seul (retour à l'état antérieur). **`/ip dns`
n'est JAMAIS touché** : le DNS propre du routeur part en `chain=output`,
hors dstnat — le check-in agent et la résolution locale restent intacts
quel que soit l'état du résolveur filtrant (la disponibilité du site prime
sur la stricteté du filtrage). Coût routeur : deux règles NAT, aucune
mémoire supplémentaire (MIPS 128 Mo compris) ; coût cloud : rien (résolveurs
publics anycast gratuits). Le rapport échoe le compte de règles marquées
PRÉSENTES après application (valeur dynamique — vérité routeur, pattern
fetchResultData). Limite documentée : DoH (port 443) contourne la
redirection — un filtrage par requête exigerait un DPI hors de portée ;
l'immense majorité des appareils en salle utilise le DNS du DHCP.

### Modèle — un niveau par routeur, trois colonnes idempotentes
`Router` gagne `SafeWifiLevel` (off/threats/family ; `""` = antérieur au
N°80 → traité comme off), `SafeWifiSig` (signature de la config appliquée
avec succès) et `SafeWifiAppliedAt` (auto-réparation périodique) —
`ALTER TABLE ADD COLUMN IF NOT EXISTS` ×3 dans `ensureSchema`, pattern
`watcher_ok` N°77. Pas de tables supplémentaires : le MVP est un niveau
par site, pas des groupes d'appareils (simplification assumée de la
spécification initiale — le besoin réel d'un gérant de salon est « tout le
WiFi » ou « rien »).

### Convergence walled-garden — silence si jamais utilisé, auto-réparation 6 h
`ensureSafeWifiLocked` (check-in) : niveau `off` + signature vide =
JAMAIS utilisé → **RIEN**, aucun octet filé — l'économie de veille N°75
reste entière pour un parc qui n'ouvre pas la carte. Sinon : signature =
hash(sel `sw-v1` + niveau) ; elle n'est posée qu'au retour « ok » VÉRIFIÉ
(compte de règles marquées rapporté == attendu : 2 en filtrage actif,
0 sinon) ET uniquement si le niveau rapporté est TOUJOURS courant (un
gérant qui change d'avis pendant le vol ne voit pas un niveau périmé
figé). Re-file automatique au changement de niveau, à l'évolution du sel
de version (toute évolution future de la forme des règles reconverge tout
le parc) et périodiquement (`safeWifiRefresh` 6 h — une règle effacée par
un ménage local ou une restauration de backup est recréée au plus tard
6 h après, pattern N°49). `CmdSafeWifi` rejoint `staleSentReadKinds`
(idempotent) et le bucket différé en FERMETURE du batch (vague 80 :
29 < 35 < 80 — la protection ne dépend d'aucune autre commande).

### API et console — un point d'entrée, une carte
Backend : `PUT /api/routers/{id}/safewifi` `{level}` (validation stricte
off/threats/family, scope compte, agent-only, garde P3 abonnement expiré,
journal d'activité avant/après avec l'acteur). La signature ne bouge pas à
l'écriture : c'est `ensureSafeWifiLocked` qui voit la différence au
check-in suivant et file la commande — servie ≤ 45 s (console ouverte =
attention N°75) ou ≤ 180 s (veille). Frontend : `SafeWifiCard` dans
l'onglet Système des outils routeur — radiogroup trois niveaux avec
descriptions, badge « Actif », toast de confirmation, footnote de
disponibilité ; `safeWifiLevel` sur `RouterDevice`, 12 clés i18n FR + 12
EN (« Protection WiFi public »).

### Vérifications
Tests : 9 nouveaux — 3 agent (niveau actif : retrait idempotent PUIS
exactement 2 règles dst-nat udp+tcp vers LE résolveur du niveau, jamais
`/ip dns set`, rapport dynamique `rules` ; off/valeur inconnue : retrait
seul ; résolveurs par niveau) ; 6 api (silence total si jamais utilisé,
convergence complète niveau actif puis changement, retrait des règles
après extinction, signature posée seulement si compte de règles exact +
niveau courant, sel de version dans la signature, ordre du batch :
safewifi en fermeture après walled_garden/hotspot_files). Suite complète
11 paquets verts, -race ciblé vert, gofmt/vet/build propres ; frontend
eslint 0, tsgo 0, build production ✓ ; E2E Playwright 9/9 verts.

## 2026-09-12 — N°79 : l'e-mail « Mot de passe oublié » devient un courriel brandé HTML « Aurora Emerald » (mode clair) — transport deux pièces Resend html + SMTP multipart/alternative

### N°79 — Contexte : un lien fonctionnel, un e-mail texte brut
Le parcours N°68 (mot de passe oublié) fonctionnait parfaitement côté
sécurité (token 256 bits hashé, usage unique, 60 minutes) mais le courriel
partait en **texte brut** : Resend ne recevait que le champ `text`, le SMTP
un `text/plain` nu. À l'arrivée dans une boîte Gmail/Outlook, MikCloud se
présentait comme un script — aucune couleur, aucun logo, aucun bouton : pour
un SaaS qui vit de la confiance (les e-mails de réinitialisation sont LA
première cible du phishing), l'identité visuelle de l'app devait voyager
jusqu'à la boîte de réception.

### Transport — deux pièces MIME chez les deux fournisseurs
`notify.SendEmailTo` gagne un corps HTML (paramètre `htmlBody`, ignoré si
vide) : **Resend** reçoit désormais `text` + `html` dans le payload (champ
`html` omis quand vide — payload inchangé pour les notifications texte) ;
**SMTP** passe en `multipart/alternative` texte PUIS HTML (frontière dédiée,
chaque client affiche sa meilleure pièce, les clients sans HTML tombent sur
le texte) ; `buildMessage` conserve le chemin texte pur à l'identique quand
aucun HTML n'est fourni (notifications automatiques inchangées).

### Gabarit « Aurora Emerald » — l'identité de la console en mode clair
`password_reset_email.go` (NOUVEAU) rend le courriel aux couleurs du
frontend (globals.css) : fond **papier menthe #F4F9F5**, carte blanche, encre
émeraude #102019, bandeau + liseré au **dégradé signature émeraude→teal**
(#009558 → #008687, replis unis #008B57 pour les clients sans gradients),
**wordmark duotone** « Mik » blanc + « Cloud » menthe clair (lisible même
logo bloqué), `<meta name="color-scheme" content="light">` + couleurs
explicites sur chaque contenant (les clients sombres n'inversent rien).
Le clin d'œil produit : le lien vit dans un **TICKET pointillé** façon
voucher MikCloud — étiquette « LIEN SÉCURISÉ · USAGE UNIQUE », pastille
« ⏱ 60 MIN », phrase explicite sous le bouton ; bouton bulletproof
(td bgcolor + a inline-block, cliquable partout) + **repli du lien en
clair** (word-break), note de sécurité « vous n'êtes pas à l'origine… » sur
fond menthe doux, pied de page avec lien du site et mention automatique.
Compatibilité e-mail : tables `role="presentation"`, styles 100 % inline,
600 px fluides, échappement HTML de tout contenu utilisateur (nom, compte,
identifiant — le lien est échappé pour href ET affichage), logo servi depuis
l'origine résolue du frontend (APP_PUBLIC_URL/ALLOWED_ORIGIN → mikcloud.ftci.fr
en prod, localhost en dev), pré-en-tête caché pour l'aperçu de boîte.
Taille du HTML : ~6 Ko (bien sous les seuils de troncage Gmail).

### Vérifications
Tests : notify étendu (payload Resend AVEC/SANS clé `html`, multipart
texte-puis-HTML + frontière fermée + ordre des pièces) ; password_reset
étendu (stub 5 paramètres, le courriel capturé part en DEUX pièces — le
texte de repli ET le HTML avec lien, couleurs, mentions contractuelles) ;
2 tests NOUVEAUX purs sur les gabarits (contrat complet : identité, mentions,
échappement anti-injection — un nom « <script> » reste du texte affiché —,
lien ≥ 2 occurrences, libellés texte N°68 au mot près) ; suite complète
11 paquets verts, -race ciblé vert, gofmt/vet/build propres ; rendu vérifié
au navigateur desktop 800 px + mobile 390 px (aucun débordement, bouton
entier, ticket intact, contraste pied assombri #6B7F76 pour WCAG).
## 2026-09-11 — N°78-bis : le login reste lent après N°78 — mise à niveau opportuniste du coût bcrypt + synchro Postgres divisée par deux

### N°78-bis — Contexte : la promesse N°78 non tenue, mesurée en production
Après le déploiement de N°78 (bcrypt 10), le login restait à 3,6-4,8 s.
Deux causes découvertes en production (instrumentation N°71
`/api/admin/sync-status` + lecture directe des hash en base Neon) :
- **les hash existants vérifient au coût EMBARQUÉ** : l'abaissement 12 → 10
  ne profitait qu'aux NOUVEAUX hash — les comptes créés avant (les deux
  clients essai : `lerobuste`, `cybersc25`) vérifiaient encore à la vitesse
  coût 12 à chaque login (seul l'admin était épargné : `applyAdminOverride`
  le re-hache au démarrage avec le coût courant) ;
- **la synchro PostgreSQL du login coûtait 2,8 s** : le Save déclenché par
  la simple ligne de journal « Connexion de X » re-marshale TOUTES les
  lignes de TOUTES les tables (3 651 utilisateurs hotspot, ~15 Mo de JSON)
  — **deux fois** (détection de changements PUIS rafraîchissement du cache
  d'empreintes) — sur le 0,1 vCPU Render (`sync.lastSuccessMs = 2809` pour
  `lastChangedRows = 1`).

### Correctif 1 — mise à niveau opportuniste du coût bcrypt au login
Nouveau `auth.NeedsBcryptRehash(hash)` : true si le hash bcrypt existe mais
a été produit à un coût ≠ coût courant. La migration transparente du login
(qui couvrait les anciens hash SHA-256) couvre désormais AUSSI ce cas : au
login réussi, le mot de passe clair est re-haché au coût courant et persisté
— une fois pour toutes. Même traitement pour le PIN du Mode Vente
(`handleResellerLogin`). Chaque compte existant paie UN dernier login lent
(coût 12 + écriture), puis ~1 s pour toujours.

### Correctif 2 — la ligne de journal du login ne déclenche plus de synchro
La trace d'audit « Connexion de X » vit en mémoire et est persistée par la
prochaine mutation réelle — au plus tard le check-in d'un agent (≤ 45 s avec
un routeur en ligne, qui persiste son touchAgent). Perte maximale en cas de
crash dans cette fenêtre : une ligne de journal, sans impact métier. Le login
ne paie plus du tout la synchro complète.

### Correctif 3 — le double marshal de syncTable éliminé
`syncTable` calculait l'empreinte FNV de chaque ligne DEUX fois par synchro
(une pour détecter les changements, une pour rafraîchir le cache après les
écritures). Les empreintes calculées sont désormais réutilisées — chaque
synchro de CHAQUE endpoint mutant passe de ~2,8 s à ~1,4 s sur le parc
actuel (3 651 utilisateurs hotspot), et la moitié des allocations GC part
avec.

### Vérifications
gofmt/vet propres, 11 paquets Go verts dont le nouveau `TestNeedsBcryptRehash`
(coût courant → false, coût 12 → true, vide/legacy → false : laissé à
IsLegacyHash) ; E2E Playwright non impactés (aucun changement de contrat
API — vérifiés par la CI).

## 2026-09-11 — N°78 : latence au chargement — login 4-7 s → ~1 s (bcrypt), bundle initial −19 % (i18n EN différé + framer-motion hors chemin critique)

### N°78 — Contexte : audit de latence chiffré (3 couches empilées)
L'audit de production a mesuré sur le parcours réel : **login à 4,3-7,6 s**
(3 essais reproductibles + navigateur), **bundle JS initial /app à 409 Ko
gzip / 1,32 Mo brut** (dont i18n FR+EN ~76 Ko gz embarquées en permanence et
framer-motion ~44 Ko gz pour des transitions de 0,2 s), et des appels API
intermittents à 2-2,7 s par à-coups de contention CPU sur le 0,1 vCPU du plan
Render gratuit. Le frontend étant bien architecturé (code-splitting, SW
propre, préchauffe), les gains étaient dans le payload initial et le backend
CPU — pas dans une refonte.

### Correctif 1 — bcrypt coût 12 → 10 (backend)
`HashPassword` passait chaque login à la moulinette bcrypt coût 12 : ≈ 250 ms
sur un CPU normal, **4-7 s sur le 0,1 vCPU throttlé de Render** — calcul pur,
à chaque session fraîche. Le coût 10 reste ≥ 2^10 tours de bcrypt (sécurité
amplement suffisante pour ce service) et divise le temps par ~4 : login attendu
**~1-1,7 s** après déploiement. Aucune migration : `CompareHashAndPassword`
lit le coût EMBARQUÉ dans le hash — les comptes existants (coût 12) restent
vérifiés, les nouveaux hash naissent à 10.

### Correctif 2 — dictionnaire anglais en chargement asynchrone (frontend)
Les ~430 clés EN (~2 500 lignes) vivaient dans le bundle initial : chaque
utilisateur français (quasi tout le parc, Côte d'Ivoire) payait ~37 Ko gzip
pour une langue qu'il n'affiche jamais. Le dictionnaire EN est extrait vers
`i18n-en.ts`, chargé par `import()` dynamique UNIQUEMENT quand la langue
active est EN (promesse mémoïsée `ensureEnDict()`, échec réseau → repli
silencieux sur le français — le contrat historique « clé EN manquante → FR »
est conservé ; le hook `useI18n()` amorce le chargement et re-rend à
l'arrivée). Bonus : clés `login.noAccount` / `login.createAccount` promues
dans les DEUX dictionnaires (les replis inline étaient toujours en français).

### Correctif 3 — framer-motion hors du chemin critique (frontend)
5 composants statiques du bundle initial convertis en @keyframes CSS
(`mik-*`, globals.css, bloc `prefers-reduced-motion` complet) : écran de
connexion (cascade des champs par `animation-delay`, anneaux du logo, orbes
dérivants, shake d'erreur relancé par retrait/reflow/rajout de classe — le
focus des champs est préservé), modale « mot de passe oublié », shell de la
console (transition de vue `mik-view-in`, sortie instantanée), bascule de
thème, paywall. Les vues différées (next/dynamic) conservent framer-motion,
payé dans leur chunk dédié. Résultat mesuré : **bundle initial 330 Ko gzip /
1,08 Mo brut / 18 fichiers** (avant : 409 Ko / 1,32 Mo / 19) — **−79 Ko
gzip (−19 %)**, dont le dict EN (37 Ko) et framer-motion (44 Ko) sortis des
chunks initiaux ; les utilisateurs EN téléchargent le leur à la demande.

### Correctif 4 — timeouts fetch par variante (frontend)
Un réseau mobile peut stall une requête indéfiniment (socket demi-ouvert) :
sans délai, un appel pendait pour toujours. Nouveau helper `timeoutSignal()`
avec défaut par variante : **api/apiAnon 20 s, apiUpload 60 s, apiDownload
120 s** (+ aperçu portail 20 s) ; `timeoutMs` explicite prime sur le défaut,
`timeoutMs: 0` désactive ; `AbortSignal.timeout` garde son repli manuel
(AbortController + minuteur) pour les navigateurs anciens.

### Correctif 5 — refresh ciblé sur les requêtes actives (frontend)
Le bouton « Actualiser » déclenchait `invalidateQueries()` SANS filtre : la
rafale invalidait aussi les requêtes INACTIVES des vues non consultées
(6+ refetchs inutiles au 0,1 vCPU Render). Désormais :
`invalidateQueries({ type: "active" })` — seuls les composants MONTÉS se
rafraîchissent, le polling reste tranquille.

### Vérifications
Typecheck, lint et build production verts ; **11 paquets Go verts**
(gofmt/vet propres) ; E2E Playwright 9/9 verts ; parcours navigateur complet
(login FR rendu, animations CSS conformes, bascule EN avec chunk asynchrone
chargé et UI re-rendue en anglais, login admin réel, navigation Comptes avec
transition CSS, bouton Actualiser sans erreur, retour FR) — zéro erreur
console/page à chaque étape.

## 2026-09-10 — N°77 : veilleur d'invités + priorité des actionnables — le claim du portail redevient rapide (45 s → 1-2 min → ≤ 20 s)

### N°77 — Contexte : le claim gratuit devenu très lent (rentabilité menacée)
Constat production juste après N°75/N°76 : le **claim gratuit du portail captif
est passé de ~45 s à 1-2 minutes** avant validation après saisie du numéro.
Un invité qui attend plus d'une minute en salle finit par abandonner — pour
la cible cœur (restaurants, maquis, cafés, buvettes), c'est l'expérience
première du produit qui se dégrade. Deux causes empilées, toutes deux issues
des optimisations de capacité :
- **la veille N°75** : un routeur endormi (pas de console ouverte) ne sert le
  claim du portail qu'à son prochain check-in — le marqueur d'attention ne
  peut pas RÉVEILLER un routeur qui n'appelle pas. Moyenne ~90 s, pire cas
  180 s (un cycle complet de veille) ;
- **les chunks read_state N°76** : un parc de 3 500 users enfile jusqu'à 10
  chunks d'un coup, TOUJOURS plus anciens qu'un claim fraîchement posé (ils
  sont créés au début du cycle) — le FIFO pur leur donnait les 10 slots du
  check-in, et le claim attendait un check-in DE PLUS avant de s'exécuter
  derrière ~30 s de scripts de lecture.

### Correctif 1 — le veilleur d'invités (scheduler `mikcloud-watch`)
- **Tick 20 s conditionné** : à chaque tick, le veilleur compte les hôtes
  hotspot **non autorisés** (`!authorized && !bypassed && !blocked`) — un hôte
  non autorisé = un appareil connecté SANS session = un invité est SUR le
  portail, son claim est imminent ou en cours. Si > 0 → check-in complet ;
  si 0 → **RIEN** (aucun octet émis : l'économie de veille N°75 — ~6
  Mo/mois/routeur, des centaines de routeurs sur le plan gratuit — est
  préservée intégralement, l'attention ne coûte que ~1,2 Ko/min PENDANT
  qu'un invité est réellement sur le portail).
- **Fichier PROPRE au veilleur** (`mikcloud-watch.rsc`, jamais le dst-path du
  scheduler principal) : les deux check-ins tournent en parallèle (20 s vs
  45/180 s) et leurs fenêtres se chevaucheront forcément — deux fetchs
  concurrents ne peuvent pas s'écraser mutuellement le fichier (un import
  de fichier à moitié réécrit tue les commandes du même check-in).
- **Déploiement** : posé directement par l'install des NOUVEAUX agents
  (l'invité du premier soir n'attend pas la convergence) + commande
  `watcher_ensure` (remove-then-add idempotent) qui converge le parc EXISTANT
  au premier check-in — pattern walled-garden : le drapeau `Router.WatcherOK`
  (persisté en base, colonne `watcher_ok`) n'est posé qu'au retour « ok » du
  routeur, un échec ou un veilleur effacé à la main est re-filé au check-in
  suivant (auto-réparation).
- **Exclusions** : `bypassed` (binding MAC permanent — sinon le veilleur
  tirerait 24 h/24 pour l'appareil du gérant) et `blocked` (banni du login —
  aucun claim ne viendra de lui).

### Correctif 2 — priorité des actionnables dans le batch du check-in
Nouvel ordre de service : **actionnables** (écritures métier, claim,
`watcher_ensure`, outils console) → **chunks read_state** (idempotents,
cadencés, ré-enfilés par la boucle zombie) → **walled_garden/hotspot_files**
(fermeture de marche inchangée — une ligne avortée ne doit pas tuer ce qui la
suit). Le claim s'exécute EN PREMIER dans le script du check-in au lieu de
ramper derrière la réconciliation d'un grand parc.

### Résultat
Le claim d'un invité est servi en **≤ 20 s pendant sa fenêtre portail**, sans
réveiller l'économie de veille (0 octet émis sans invité) : les deux leviers
de capacité N°75/N°76 (des centaines de routeurs au plan gratuit, des parcs
de 10 000 users réconciliés) restent entiers, l'expérience invité revient à
son niveau antérieur — mieux : elle devient indépendante du mode veille.

### Tests
5 nouveaux (`agent_watch_test.go`) : mise en file/dédup/drapeau/routeur simulé
de `ensureWatcherLocked` ; forme du script (scheduler `mikcloud-watch`,
intervalle, garde d'hôtes, fichier propre, double échappement `on-event`
conforme au pattern `buildSchedulerAdd` éprouvé, remove-then-add idempotent) ;
l'install déploie le veilleur ; la priorité du batch (7 chunks anciens + un
claim récent → le claim servi EN PREMIER, ≤ 10 commandes) ; E2E complet
(check-in → déploiement → rapport « ok » → `WatcherOK` posé → silence au
check-in suivant). En passant, 3 bugs des NOUVEAUX tests eux-mêmes corrigés
avant livraison (forme échappée du `dst-path`, collecte des KINDS — pas des
IDs — dans l'ordre servi, horodatages de graines réellement au passé et
rapport `status=ok` explicite). Vérifié comme la CI : gofmt/vet/build,
suite complète 11 paquets verts, `-race` ciblé vert.

## 2026-09-10 — N°76 : read_state PAGINÉ — la réconciliation des grands parcs revit (faux badges ProMax WIFI, compteur de parc gelé)

### N°76 — Contexte : 3 334 faux badges « absent du routeur » en production
Constaté sur le compte **ProMax WIFI** (3 478 vouchers actifs) : 3 334 users
actifs badgés « absent du routeur » à tort + 31 « used » badgés + compteur de
parc du routeur **gelé à 150**. Origine en deux temps : (1) avant N°75, le
rapport read_state bornait la liste à 150 users — tout user au-delà passait
« absent » à tort (le badge est persistant en base) ; (2) N°75 a relevé la
borne à 500 et gelé honnêtement TOUTE déduction au-delà (ni pose ni levée) —
correct pour empêcher de nouveaux faux badges, mais fatal aux EXISTANTS : le
rapport d'un parc de 3 478 users étant tronqué EN PERMANENCE, la
réconciliation ne tournait plus JAMAIS — les faux badges étaient prisonniers
à vie et la fonctionnalité (détecter un voucher supprimé dans Winbox mais
actif au cloud) était désactivée de facto pour tous les grands parcs.

### Le read_state devient PAGINÉ (pattern import_hotspot, éprouvé en prod)
- **Script v5** : chaque commande rapporte une FENÊTRE `[start, start+count)`
  du parc (count = 500, inliné par Go) + le **total exact** du parc + le total
  exact de sessions (`stotal`) ; les **sessions ne sont rapportées que par le
  chunk final** (les intermédiaires n'alourdissent pas leur POST pour rien).
- **Cycle automatique** : au résultat du chunk 0, le cloud enfile d'un coup
  TOUTES les fenêtres restantes (≤ 20 = 10 000 users) — servies au check-in
  suivant par paquets de 10 (limite FIFO/check-in), sans réveiller les
  routeurs en veille (les chunks restent du balayage : pas de ping-pong
  45 s ↔ 180 s). Un cycle de 3 500 users s'exécute en ~2 check-ins.
- **Complétude vérifiée avant d'appliquer** : les chunks s'accumulent
  (union des usernames + offsets reçus) ; la réconciliation (badges, import
  inconnus, diff sessions) ne s'applique **QU'AU CYCLE COMPLET** — un chunk
  perdu (blip réseau, reboot routeur) abandonne le cycle SANS AUCUNE
  déduction : l'honnêteté N°75 reste LA règle, elle est désormais atteignable.
  Cycle cassé = relance au cadenceur suivant (auto-réparation).
- **Réparation automatique des faux badges existants** : le premier cycle
  complet post-déploiement trouve les users sur le routeur → 3 334 badges
  levés d'un coup sur ProMax, sans chirurgie de base. Les badges des
  vouchers non actifs (used/expired) — artefacts des rapports tronqués —
  sont levés au passage (le badge n'a de sens que pour un user actif).
- **Compteurs enfin exacts** : `HotspotUsers` = total exact rapporté par
  chaque chunk (plus jamais gelé à une borne), `ActiveSessions` = `stotal`
  exact même quand la liste de sessions est bornée à 250 (le diff est alors
  suspendu, pas le compteur).
- **Grâce anti-faux-badge étendue à la durée du cycle** : un user créé
  PENDANT le cycle (sa fenêtre d'index déjà rapportée) ne peut pas être
  badgé — la grâce passe de 2 min à 2 min + chunks × pas du scheduler.
- **Cadence adaptée à la taille du parc** : l'intervalle minimum entre cycles
  devient 2 min × nb_chunks (un parc de 3 500 users se réconcilie toutes les
  ~14 min) — le coût egress des scripts servis reste dans le régime N°75.
- **Fraîcheur post-écriture et re-sync manuelle protégées** : un cycle en
  cours EST une synchronisation — il n'est plus cassé par un chunk 0
  concurrent (`queueReadStateFreshLocked`) qui désordonnerait l'accumulateur.
- **Allègement de l'historique de commandes** : les listes brutes (users
  d'un chunk ~17 Ko, sessions) ne sont plus persistées dans `Result` — des
  Mo/jour de resynchronisation Neon en moins pour les grands parcs, seuls
  les compteurs restent.
- **Bornes** : par chunk 500 users (~20 Ko POST, loin de la limite RouterOS
  ~64 Ko) ; absolue 20 chunks (10 000 users) — au-delà, compteurs exacts
  mais aucune déduction (le rapport ne sera jamais complet : honnêteté).
  Sessions toujours bornées à 250 par rapport (diff suspendu au-delà).

### Tests
9 nouveaux (cycle complet ProMax lève la masse des faux badges, chunk perdu
= déduction refusée, orphelins inertes, mono-chunk inchangé, sessions
tronquées = état conservé + compteur exact, grâce cycle, import inconnus du
cycle complet, borne absolue, cadence adaptée ×2) + garde-fous fraîcheur et
script v5 (fenêtres inlinées, total/stotal rapportés) + contrat v4 legacy
(trunc sans total) et v4 complet inchangés. Suite complète 11 paquets verts.

## 2026-09-10 — N°75 : veille adaptative des agents, amaigrissement du portail (-66 %), ETag console, durcissement sécurité et 2 bugs (Wave hardcodé, cap 150)

### N°75 — Contexte : livraison de la file d'attente de l'audit (capacité 0 $)
Après l'analyse de capacité N°74/N°15 (mur bande passante ~100 routeurs sur le
plan gratuit Render), livraison des 5 optimisations structurelles restantes +
2 bugs fonctionnels découverts par l'audit, dont un qui facturait les invités
au MAUVAIS marchand Wave.

### Veille adaptative des agents — le cloud pilote le pas des routeurs (45 s ↔ 180 s)
- **Avant** : le scheduler MikCloud posé à l'installation tourne à 45 s à
  JAMAIS — 1 920 check-ins/jour/routeur quel que soit l'usage. **Après** : à
  chaque check-in, le cloud décide — 45 s quand le routeur est « sous
  attention » (console du compte ouverte : toute requête console
  authentifiée marque le compte via le middleware d'auth ; invité sur le
  portail : chargement de page, claim et poll de statut marquent le routeur
  du site ; commandes métier en file), sinon 180 s de veille. La bascule est
  une commande `scheduler_set` ORDINAIRE (même FIFO, même garantie de
  livraison, même fermeture zombie) ; le retour « ok » du routeur échoe
  l'intervalle appliqué et pose `Router.SchedulerSec` — la vérité vient
  toujours du routeur, pattern walled-garden/portail.
- **Piège évité (découvert en traçant le flux)** : le cadenceur read_state
  N°74 (2 min) file TOUJERS une commande au moment de décider — compter
  toutes les commandes en file aurait fait ping-ponger les routeurs
  45 s ↔ 180 s à CHAQUE check-in. Seules les commandes ACTIONNABLES
  (user_add, kick, etc.) réveillent ; les balayages (read_state,
  walled_garden, hotspot_files) sont servis en veille sans dommage.
- **Intégrité** : le seuil « hors ligne » devient max(réglage compte,
  3 × le pas) — `model.Router.EffectiveOfflineAfter`, partagé par le
  moniteur de notifications et la fenêtre en ligne du sync-status (un
  routeur endormi ne flappe plus « hors ligne » entre deux check-ins).
  Latence de réveil : pire cas un cycle de veille (3 min) ; la page de
  claim du portail attend désormais 5 min (60 × 5 s) au lieu de 2 min.
  Débit : ~6 Mo/mois/routeur en veille → capacité ~350-450 routeurs sur le
  plan gratuit (vs ~100 avant).
- **Rate-limit /agent/\*** (le protocole était explicitement hors limiteur) :
  par TOKEN (les sites derrière NAT partagent une IP) — cmd 60/min,
  result 600/min/IP (le token voyage dans le corps POST, illisible au
  niveau middleware sans consommer le body), register 6/min/IP, garde IP
  transverse 600/min sur tout /agent/* (borne la mémoire contre les floods
  à tokens aléatoires). Le 429 agent est en TEXTE (contrat textuel de
  /agent/cmd).

### Amaigrissement du portail — 2,30 Mo → 0,79 Mo déployés par routeur (-66 %)
- **TTF inutiles retirés** (648 Ko) : fa-solid/fa-brands/fa-regular/
  fa-v4compatibility .ttf ne sont fetchés que par les navigateurs
  pré-2015 (les woff2 servent à tout le parc hotspot 2026).
- **Webfonts sous-ensembleés aux icônes UTILISÉES** : extraction
  automatique des classes fa- des 8 pages (parseur complet du CSS FA6 —
  piège : les alias sont des sélecteurs MULTIPLES `.fa-ticket-alt:before,
  .fa-ticket-simple:before{…}`, une extraction mono-sélecteur ratait 9
  icônes), puis pyftsubset : fa-solid 150 Ko → 4,1 Ko (37 icônes),
  fa-brands 108 Ko → 680 o (WhatsApp seul), fa-regular et fa-v4compat
  supprimés (jamais référencés par les pages). Complétude VÉRIFIÉE par
  script (37/37 codepoints présents).
- **Images recompressées** : logo.png 291 Ko → 14,6 Ko (320 px, palette 256
  avec alpha — affiché à ~64-90 px dans l'en-tête), pubs 388 Ko → 133 Ko
  (720 px, q72 progressif).
- **README.md du template déplacé hors de template/** : il était DÉPLOYÉ
  aux routeurs par DefaultFiles() (8 Ko de documentation sur chaque
  routeur client). La signature de contenu re-pousse automatiquement le
  portail amaigri vers tout le parc au check-in suivant.

### ETag console — le plus gros poste restant du trafic console
- GET /api/dashboard et GET /api/sessions passent en `writeJSONCacheable`
  (N°74) : corps déterministe + `Cache-Control: no-cache` + ETag fnv64 → le
  navigateur de la console revalide automatiquement (If-None-Match) et
  reçoit un 304 sans corps entre deux changements de données, au lieu de
  re-télécharger l'intégralité (mesure pointe : 1,5 Mo/777 req de trafic
  console). Les mutations de Tick/expirations restent exécutées — seule la
  réponse est conditionnelle.

### Durcissement sécurité
- **secretbox étendu** : les secrets de notification
  (telegram_bot_token, whatsapp_token, resend_api_key, smtp_pass) et le
  secret 2FA (admin_users.totp_secret) sont désormais chiffrés au repos
  (AES-256-GCM), lecture = déchiffrement / écriture = chiffrement dans les
  specs pg, migration one-shot des valeurs en clair au démarrage
  (migrateSealSecretColumns), scellement du snapshot JSON (mode dev).
  L'empreinte de synchro reste calculée sur l'état mémoire clair — aucune
  tempête de réécriture.
- **Bug caché n°3 corrigé (persistance 2FA)** : `AdminUser.TOTPSecret`
  porte `json:"-"` → l'empreinte de synchro (json.Marshal) l'EXCLUAIT —
  un changement de secret ne déclenchait AUCUN upsert (la 2FA n'était
  persistée que par effet de bord du flip TOTPEnabled : un redémarrage
  entre /2fa/setup et /2fa/activate perdait le secret, et le mode JSON
  dev ne le persistait JAMAIS). Empreinte dédiée via shadow struct qui
  EXPOSE le secret au marshal (uniquement pour le hash — jamais sérialisé
  ailleurs).
- **TLS strict vers Neon** : sslmode=verify-full par défaut (validation
  chaîne + hostname ; « require » chiffrait mais acceptait n'importe quel
  certificat — usurpation d'endpoint possible sur le segment réseau).
  PRÉVALIDÉ contre le Neon de production depuis l'environnement de dev
  (verify-full OK sur l'endpoint pooler). Échappatoires : sslmode= dans
  l'URL, ou MIKCLOUD_PG_SSLMODE (urgence sans re-déploiement).
  L'image Docker Alpine embarque désormais ca-certificates (prérequis
  des racines publiques — sans elles, verify-full casserait la synchro).

### Bug 1 — offres Wave hardcodées : les invités payaient le MAUVAIS marchand
- **Constat** : login.html portait 7 cartes d'offre pointant en dur vers le
  marchand Wave de l'OPÉRATEUR (M_5Mg9EG61ZHDF — FTCI, 100→3000 F). Un
  compte sans waveLink voyait ses invités PAYER l'opérateur ; pire,
  renderOffers ne remplaçait que le href SANS mettre à jour les prix
  (sélecteur `.price` alors que la classe réelle est `.creative-price`) :
  l'invité voyait « 100 F » et payait le montant réel du profil chez Wave ;
  les cartes orphelines (moins d'offres que 7 slots) restaient cliquables
  vers le mauvais marchand ; et le deep-link Android ne se liait qu'aux
  liens présents au chargement (les href dynamiques n'en bénéficiaient
  jamais).
- **Correctif** : les 7 cartes statiques deviennent des PLACEHOLDERS sans
  lien marchand (`href="#"` + data-mik-offer) ; renderOffers réécrit —
  chaque carte épouse l'offre PAYABLE correspondante (libellé de durée
  humain, PRIX et lien Wave DU COMPTE mis à jour), les offres sans waveUrl
  et les cartes orphelines sont MASQUÉES, et la vitrine entière (grille +
  titres + bandeau Wave) se retire quand le compte n'a AUCUNE offre
  payable en ligne (réversible : configurer le lien marchand la fait
  réapparaître via le fetch live, sans re-déploiement). Le deep-link
  Android passe en DÉLÉGATION document-level (attrape tout clic sur une
  ancre pay.wave.com, quelle que soit la date de pose du href). Test
  garde-fou : le template ne doit JAMAIS contenir d'URL marchand Wave.

### Bug 2 — cap 150 users du read_state : réconciliation mensongère au-delà
- **Constat** : le script read_state bornait le rapport à 150 users / 100
  sessions ; applyReadState marquait MissingOnRouter tout user cloud
  absent de la liste → au-delà de 150 utilisateurs sur le routeur, FAUX
  badges « absent du routeur » en cascade, et le diff sessions générait des
  logouts fantômes au-delà de 100 sessions actives.
- **Correctif (double volet)** : bornes portées à 500 users / 250 sessions
  (le script est généré par le CLOUD à chaque commande — la correction
  s'applique à tout le parc dès le déploiement, aucun versionnage agent) ;
  le rapport porte désormais `trunc=true` quand il est tronqué, et le
  cloud NE DÉDUIT RIEN des absents — ni badge MissingOnRouter (ni posé ni
  levé), ni diff sessions (état conservé), ni compteurs (dernière valeur
  honnête gardée). L'honnêteté du rapport prime sur le comptage.

### Tests
- 12 nouveaux : veille adaptative ×6 (bascule veille, attention console/
  portail/expirée, piège des commandes de balayage, dédup, seuil offline,
  et un END-TO-END complet check-in → rapport → réveil console), troncature
  read_state ×3 (déductions différées, comportement historique intact,
  script v4), ETag console ×2 (dashboard + sessions, 200/304/mutation),
  template Wave ×2 (aucun marchand hardcodé + placeholders complets),
  limiteur agent ×2 (isolation par token + 429 texte) ; ancien contrat du
  test « /agent/cmd illimité » mis au nouveau contrat N°75 ; suite
  complète 11 paquets VERTS (api 103 s), -race ciblé vert, gofmt/vet
  propres.

## 2026-09-10 — N°74 : audit de robustesse + optimisation — le backend ne peut plus geler (5 correctifs structurels) et la bande passante agents chute de ~62 %

### N°74 — Contexte : audit d'expert complet (portail hybride, console, chaîne agents, robustesse) après l'incident de quota du 8-10/09
- **Audit (4 axes en parallèle)** : le portail captif (chaîne complète d'une
  session, cache, gzip, robustesse cold start), la console (inventaire
  exhaustif du polling React Query, poids des payloads, ETag), le protocole
  agent (payloads exacts au octet, cadence, robustesse) et la fiabilité du
  backend (verrous, I/O, paniques, sécurité). Constats majeurs : TOUT le
  backend converge vers un verrou global derrière lequel on faisait de
  l'I/O réseau (Neon sans timeout, notifications, SMTP nu), AUCUN recover
  n'existait (une panique entre Lock et Unlock = mutex mort à vie = service
  mort), read_state était servi à CHAQUE check-in 45 s (24 h/24) alors que
  personne ne regarde ces snapshots la nuit, et la vue Vouchers téléchargeait
  200 objets complets toutes les 20 s pour 5 compteurs (faux au-delà de 200
  tickets).
- **Découverte mesurée** : RouterOS /tool fetch ENVOIE bien
  « Accept-Encoding: gzip » — la preuve arithmétique tient dans les compteurs
  N°72 (427 Ko mesurés sur 3 h 17 pour 2 routeurs ≈ l'hypothèse « gzip
  actif », l'hypothèse « sans gzip » prédirait 1,1 Mo) ; les commentaires
  « jamais compressés » de gzip.go/routes.go sont corrigés par cette entrée
  (le gzip agents fonctionne, il ne faut plus le compter comme gain futur).

### Robustesse — le service ne peut plus geler ni mourir d'une panique
- **recoverMiddleware (main.go, en tête de chaîne)** : toute panique de la
  chaîne (middlewares + handlers) devient un 500 propre + trace complète
  dans le log service — les defer des handlers se déroulent à la remontée,
  donc un Unlock différé s'exécute et LE VERROU SURVIT À LA PANIQUE (avant :
  mutex mort à vie → health check Render en échec → crash-loop du service).
  http.ErrAbortHandler traverse sans être converti (contrat net/http). Les
  goroutines de fond reçoivent le même filet : moniteur de notifications
  (notify/monitor.go Run — reprise au tick suivant), balayage de rétention
  (api/retention.go RunRetentionSweepForever — reprise à l'heure suivante)
  et keep-alive Neon (store/pg.go — le ping protégé ne meurt plus à vie).
- **Sync Neon sous contexte borné (store/pg.go)** : BeginTx(ctx, nil) avec
  syncTimeout = 20 s (~8× le temps mesuré en production, 2,6 s) sur TOUTE la
  transaction (29 tables + settings : statements + commit) — un Neon gelé
  (compute en réveil lent, partition réseau) ne peut plus tenir le verrou
  global indéfiniment : l'incident se résout en une erreur retournée,
  retentée au Save suivant (les empreintes différentielles ne sont
  rafraîchies qu'après succès — aucun delta perdu). Tous les appels passent
  en ExecContext/QueryContext (upsertRows, deleteRows, syncSettings).
- **Moniteur de notifications : le réseau sort enfin du verrou
  (notify/monitor.go)** : tick() est scindé en collect() (sous verrou, avec
  defer Unlock garanti — il survit même à une panique interne) puis
  délivrance Deliver() HORS verrou. L'en-tête du fichier le promettait déjà
  (« Les envois réseau ne sont JAMAIS faits sous verrou ») —
  l'implémentation ne le respectait pas : un SMTP muet tenait ~2 min PAR
  tentative, toutes les 30 s, sous le verrou que partagent check-ins agents,
  claims WiFi publics et consoles.
- **SMTP avec deadlines (notify/notify.go)** : tls.Dial → tls.DialWithDialer
  (10 s d'établissement) + SetDeadline (25 s de session) sur 465 ;
  smtp.SendMail (aucun timeout possible) remplacé par un flux manuel borné
  (net.DialTimeout + smtp.NewClient + StartTLS + Auth) sur 587 — même
  sémantique, mêmes messages d'erreur, jamais de connexion muete.
- **bcrypt hors verrou (3 handlers)** : handlePasswordChange,
  handleRegister et handleResetPassword faisaient un hachage/vérification
  bcrypt (coût 12 ≈ 200-500 ms sur le CPU mutualisé Render) SOUS le verrou
  global — les endpoints publics (register, reset) permettaient à un
  attaquant distribué de geler toute l'API plusieurs fois par seconde. Le
  pattern de handleLogin (capture des valeurs sous verrou, hachage dehors,
  re-validation atomique à l'écriture) est désormais appliqué partout. Au
  passage, la course B2 de handleLogin est corrigée : l'ancien pointeur
  « user » capturé avant Unlock servait à lire TOTP et à écrire la
  migration de hash APRÈS re-lock — une inscription concurrente pouvait
  réallouer la tranche Users et perdre silencieusement l'écriture ; les
  valeurs sont capturées, et la migration re-trouve l'utilisateur par ID.
- **readWord plafonné (routeros/protocol.go)** : readLength acceptait toute
  longueur annoncée (encodage 7 bits, jusqu'à ~63 bits) et readWord
  allouait make([]byte, n) SANS borne — un routeur compromis (ou un flux
  MITM sur le port 8728 en TCP clair) annonçant 2^40 octets déclenchait une
  allocation fatale non rattrapable (OOM kill sur les 512 Mo). Plafond
  maxWordBytes = 4 Mio (aucun mot légitime n'en approche) + rejet des
  débordements (n < 0) AVANT allocation.
- **GOMEMLIMIT=400MiB (Dockerfile)** : le runtime Go ignore la limite du
  conteneur — le GC attend ~2× le tas vivant avant d'accélérer, trop tard
  face à l'OOM-kill Render à 512 Mo. Plafond SOFT à 400 MiB (le process ne
  meurt pas s'il doit dépasser, le GC fait tout son possible en dessous).

### Optimisation bande passante — le plus gros poste agents divisé, la console allégée
- **Télémétrie read_state cadencée (agent_handlers.go + routes.go)** : la
  boucle re-enfilait un read_state (snapshot complet O(n) : users, sessions,
  8 ifaces + télémétrie) à CHAQUE résultat — toutes les 45 s, 24 h/24,
  ~1 920 snapshots/jour/routeur dont la grande majorité ne servait à rien
  (la nuit, les comptes sans console ouverte). Nouveau cadenceur
  ensureReadStateDue : un read_state automatique n'est enfilé que si le
  dernier appliqué date de plus de readStateMinInterval (2 min) et qu'aucun
  n'est déjà en file/en vol — soit ~720 snapshots/jour au lieu de 1 920
  (−62 % du volume agents, ~1 Mo/jour/routeur économisé). La fraîcheur qui
  COMPTE est préservée : les commandes d'écriture re-enfilent TOUJOURS un
  read_state immédiat (fraîcheur post-action), le bouton « Synchroniser »
  reste immédiat, les expirations restent servies à CHAQUE check-in, et le
  premier check-in après boot reste instantané. Les vues Sessions/dashboard
  passent d'une fraîcheur de 45 s à ≤ 2 min (le comptage reste exact).
- **GET /api/vouchers/stats (nouveau) + vue Vouchers allégée** : les
  compteurs de stock (actifs/consommés/expirés/alloués/valeur du stock)
  sont calculés côté SERVEUR sur l'ensemble du stock et renvoyés en un
  objet compact (~150 o) — avant, la vue téléchargeait jusqu'à 200 objets
  HotspotUser COMPLETS toutes les 20 s (~10-15 Ko gzip, le plus gros poste
  « console » mesuré) ET les compteurs étaient FAUX dès que le stock
  dépassait le plafond pageSize 200 (le comptage client ne voyait que la
  première page). Frontend : la query stats pointe le nouvel endpoint
  (queryKey ["/api/vouchers","stats"] conservé — l'invalidation existante
  continue de fonctionner).
- **ETag/304 sur les GET publics du portail (helpers.go writeJSONCacheable
  + handlers_wifi.go)** : la config live
  (GET /api/wifi/site/{slug}/portal) et le branding
  (GET /api/wifi/site/{slug}) reçoivent un ETag (FNV-64 du corps) et
  Cache-Control: no-cache — le navigateur STOCKE la réponse et la
  revalide (If-None-Match → 304 sans corps) au lieu de re-télécharger
  l'intégralité à CHAQUE chargement de page. Enjeu mesuré : ces endpoints
  transportent les logo/bannière du compte — jusqu'à ~800 Ko bruts par
  chargement dans le pire cas data-URL (500 Ko bannière + 300 Ko logo) ;
  dès la deuxième visite d'un même appareil, le coût tombe à ~200 o.
  L'ETag porte sur le corps non compressé ; le middleware gzip laisse les
  304 passer en clair et pose Vary — chaque client revalide la variante
  stockée (les deux représentations restent cohérentes).
- **Access-Control-Max-Age: 86400 (main.go)** : les préflights OPTIONS des
  POST cross-origin du portail (track analytics, claim WiFi — jusqu'à
  12 requêtes sur 6 utiles par page en mode hospitalité) sont mis en cache
  par le navigateur pour 24 h (borne haute fetch spec pour des requêtes
  sans credentials).
- **Tokens d'agent masqués dans les logs (main.go logRequests)** : le chemin
  /portal/{token}/fichier écrivait le token 192 bits complet dans le log
  service à chaque fetch de déploiement — le segment est remplacé par «***»
  (même discipline que agent.Preview côté agent).

### Tests — 12 nouveaux, suite complète verte
- read_state_throttle_test.go (5) : boot → enfile immédiat ; read_state
  frais → pas de re-file ; périmé → re-file ; déjà queued/sent → jamais de
  doublon ; cadence PAR routeur (un autre routeur en file ne bloque pas).
- etag_stats_test.go (4) : contrat writeJSONCacheable (200+ETag+no-cache,
  304 sans corps sur concordance, 200 complet sur divergence) ; ETag au
  TRAVERS de la chaîne gzip/egress/sécurité (le 304 passe en clair et
  répète l'ETag) ; compteurs serveur exacts (6 statuts + stockValue = prix
  des actifs uniquement, kind voucher uniquement, périmètre compte
  uniquement) ; 401 sans jeton.
- monitor_test.go (1) : deux ticks consécutifs sans deadlock (le defer
  Unlock de collect joue — sinon le second passerait en timeout).
- main_test.go (1) : recoverMiddleware convertit une panique en 500, la
  requête suivante passe (verrou vivant), ErrAbortHandler traverse.
- protocol_test.go : l'aller-retour des longueurs s'arrête au plafond
  (inclus) et les longueurs au-delà (0xFFFFFFF) sont REFUSÉES — nouveau
  contrat de sûreté.
- Vérifié localement : gofmt/vet/build propres, `go test ./...` complet
  vert (11 paquets, api 102 s), frontend eslint 0 + tsgo 0 + next build ✓.

## 2026-09-10 — N°73 : fermeture des zombies « sent » orphelins — un rapport perdu n'affiche plus « 1 zombie » à vie dans la carte Maintenance

### N°73 — Racine vécue en production : la suspension de quota du 8-10/09 a laissé un user_remove « sent » sans rapport, que RIEN ne fermait
- **Diagnostic (cas réel du 10/09)** : au réveil post-suspension, le sweep de
  rattrapage a filé 9 user_remove d'expiration ; 8 ont rapporté, 1 (« 45Y3 »,
  routeur CYBER S.C) a perdu SEUL son POST /agent/result (blip réseau au
  check-in). Or les écritures ne sont JAMAIS re-exécutées
  (requeueStaleReadsLocked ne reprend que les commandes idempotentes —
  double-exécution interdite, choix assumé), enforceExpired marque
  Enforced=true DÈS la mise en file (correct : pas de re-file sauvage), et
  purgeOldCommands ne balayait que done/error : la commande restait « sent »
  À VIE — compteur « zombies » de la carte Maintenance figé à 1 pour
  toujours, et l'issue réelle (exécutée ou non sur le routeur) invérifiable
  depuis le cloud. Le cas a été réparé à la main (suppression console du
  voucher → NOUVELLE commande user_remove, exécutée et confirmée en 11 s ;
  vérifié au read_state suivant : l'utilisateur a bien disparu du routeur)
  — mais la lacune STRUCTURELLE restait : aucun chemin de fermeture pour un
  « sent » muet.
- **purgeOldCommands (agent_handlers.go) — fermeture en deux phases** : un
  « sent » dont le SentAt dépasse 7 jours est clos « error » avec le message
  « rapport perdu (zombie « sent » fermé après 7 j sans retour) » et
  DoneAt = maintenant — visible 7 jours dans l'historique (l'opérateur
  constate la fermeture et l'issue inconnue), puis balayé par le nettoyage
  existant comme tout done/error ancien. Le statut « error » (et non
  « done ») est le seul honnête : l'issue réelle côté routeur est inconnue.
- **Garde-fous conservés** : les « queued » ne sont PAS touchées (un routeur
  muet qui revient les exécute et les rapporte normalement — seul le
  « sent » sans rapport est une fuite) ; les sent récents gardent leur
  fenêtre de reprise idempotente (10 min) puis d'observation ; fenêtre de
  7 j alignée sur le nettoyage existant des commandes terminées ; la
  comparaison d'horodatages passe en UTC explicite (NowISO est UTC — le
  lim local d'origine ne pouvait diverger que sur un serveur non UTC).
- **Tests** : TestPurgeOldCommandsClosesAncientSentZombies (zombie ancien
  fermé error + DoneAt + message, PAS supprimé ; sent récent intact ;
  queued ancienne intacte) ; TestPurgeOldCommandsSweepsClosedZombies
  (seconde phase : zombie fermé au cycle précédent et done de plus de 7 j
  balayés ; error récent conservé).
- **Vérifié localement** : gofmt, go vet, go build ; tests ciblés verts ;
  paquet api COMPLET vert (99 s).

## 2026-09-09 — N°72-fix : seuil de rentabilité gzip (1 024 o, réponses plus petites servies en clair) + tests métier en « Accept-Encoding: identity » + timeout go test -race porté de 22 à 30 min en CI

### N°72-fix — La CI N°72 a heurté le timeout global « go test -race -timeout 22m » (22 min 38 s contre 18 min 02 s au N°71) : le client de test de net/http annonce « Accept-Encoding: gzip » TOUT SEUL
- **Diagnostic** : chaque réponse de CHAQUE test de la suite -race était
  compressée par le nouveau middleware puis décompressée par le transport de
  test — des milliers de compressions/décompressions sous le détecteur de
  courses ont ajouté ~4,5 minutes au paquet api (N°71 : 18 min, marge 4 ;
  N°72 : timeout atteint au milieu de TestSignupQuotaE2E, aucun test en
  échec, aucun blocage : panic « test timed out after 22m0s », un seul test
  en cours de 22 s).
- **gzip.go — seuil de rentabilité (gzipMinBody = 1 024 o)** : le gzipWriter
  BUFFÉRISE désormais les octets tant que le corps peut rester sous le seuil ;
  passé le seuil, la décision tombe (en-têtes + statut retenu + tampon sur la
  voie choisie) — sous le seuil, la réponse sort en clair : le gain du deflate
  sur quelques centaines d'octets ne paie ni l'en-tête gzip + le CRC, ni le
  CPU des deux côtés. En production cela ne retire que des micro-réponses
  (401, 204, statuts courts — le login 393 o sort en clair) ; les corps qui
  PÈSENT (listes, portail, exports, sync-status : 2 Ko → 484 o compressés,
  -75 % mesuré) restent compressés. WriteHeader continue de retenir le
  statut (correctif du piège d'en-têtes expédiés avant leur pose) ; Flush()
  force la décision sur le tampon courant ; close() tranche le corps resté
  sous le seuil (en clair) et émet le statut retenu.
- **handlers_test.go — doJSON passe en « Accept-Encoding: identity »** : les
  tests métier observent le chemin NON compressé (comme avant N°72) — sans
  cet en-tête, le transport standard de net/http annonce gzip tout seul et
  la suite -race paie la compression de chaque réponse ; le chemin compressé
  a ses tests DÉDIÉS (gzip_test.go, doGzipReq qui pose l'en-tête à la main).
- **ci.yml — timeout 22 m → 30 m** : la suite -race du paquet api grossit
  avec les features (N°71 : 18 min ; N°72 : 22+ min) ; la marge doit survivre
  aux ajouts de tests, pas seulement au prochain commit.
- **Tests adaptés/nouveau** : TestGzipSkipsTinyResponses (la santé GET /
  sous le seuil sort en clair même avec gzip demandé, corps IDENTIQUE au
  chemin identity) ; TestGzipWriterBuffersThenCompresses NOUVEAU (rien ne
  part sous le seuil, la décision tombe au franchissement, le corps compressé
  contient EXACTEMENT les octets écrits — aucune perte au tampon, statut
  retenu émis) ; TestGzipBigJSONActuallyShrinks inchangé et toujours vert
  (sync-status ≥ seuil → compressé) ; 204/binaire/parsing inchangés verts.
- **Vérifié localement** : gofmt, go vet, go build ; tests ciblés verts ;
  paquet api COMPLET vert (99 s) ; validation réelle sur serveur lancé :
  sync-status avec Accept-Encoding: gzip → Content-Encoding: gzip + 484 o
  (vs ~2 Ko clair, -75 %) et corps gunzip = JSON ; login 393 o → Content-Length
  393 en clair (aucun en-tête de compression) ; E2E navigateur re-validé :
  carte « Bande passante sortante » toujours fonctionnelle (« 337 o ·
  9 requêtes » au compteur du jour), aucune régression frontend (aucun
  fichier frontend touché par le fix).

## 2026-09-09 — N°72 : optimisation bande passante — compression gzip de toutes les réponses textuelles + compteur egress journalier par catégorie (agents/portail/médias/console/autre) dans la carte « Santé de la persistance » + entretien des ressources routeurs espacé (15 s → 60 s)

### N°72 — Tenir le plancher free de Render (5 Go/mois) jusqu'aux premiers clients payants — incident du jour : quota épuisé en ~9 jours, workspace suspendu automatiquement
- **Incident déclencheur** : Render a suspendu le workspace « FTech CI » (plan
  Hobby : 5 Go de bande passante sortante gratuits par mois) — email « Workspace
  suspended — free bandwidth », quota épuisé en ~9 jours du mois calendaire alors
  que seuls deux clients sont en essai 90 jours. Diagnostic établi sur le code : la
  bande passante mesure le trafic du SERVICE backend 24 h/24 — check-ins agents
  45 s, read_states déclenchés par les consoles, médias proxifiés R2, status
  polling des invités, bots d'Internet sur le domaine public — PAS le nombre de
  clients MikCloud ; et deux absences rendaient la situation invisible : AUCUNE
  réponse n'était compressée, AUCUNE métrique d'egress n'existait. N°72 attaque
  les deux fronts : diviser les volumes (gzip ÷4-8 sur tout le texte) et piloter
  (compteur quotidien par catégorie, dans la carte déjà rafraîchie 15 s du N°71).
- **Backend — compression HTTP (internal/api/gzip.go, NOUVEAU)** : gzipMiddleware
  posé dans API.Handler() au-dessus de l'auth : compresser UNIQUEMENT les clients
  qui annoncent « Accept-Encoding: gzip » (navigateurs : oui ; agents RouterOS
  /tool fetch : non — le protocole texte des check-ins 45 s reste strictement
  inchangé), UNIQUEMENT les types compressibles (text/, JSON, JS, CSS, XML, SVG —
  jamais images ni polices, déjà compressées), JAMAIS les statuts sans corps (204
  du track analytics portail, 304) ; « Vary: Accept-Encoding » posé par Add (le
  CORS peut déjà avoir posé « Vary: Origin »), Content-Length supprimé ;
  décision au PREMIER Write avec reniflage http.DetectContentType si le handler
  n'a pas posé de Content-Type. Piège découvert et corrigé en construisant la
  feature : la convention du code (writeJSON/serveFile) appelle WriteHeader AVANT
  d'écrire le corps — engager la réponse à ce moment aurait expédié les en-têtes
  de compression APRÈS leur envoi (silencieusement ignorés par net/http : corps
  gzip sans en-tête, illisible) → le gzipWriter RETIENT le statut et ne l'émet
  qu'au moment de la décision, garantissant des en-têtes définitifs ; pool
  sync.Pool de compresseurs (une requête compressée = un Reset, pas une
  allocation) ; Flush() délégué par précaution (aucun flux temps réel
  n'existe, polling partout).
- **Backend — compteur egress (internal/api/httpstats.go, NOUVEAU)** :
  egressStats compte les octets de CORPS réellement écrits sur le réseau (monté
  AU-DESSUS du compresseur : les octets comptés sont les octets compressés, donc
  la réalité facturée) + une requête par requête servie (un 204 reste une
  requête), par CINQ catégories qui matérialisent le décompte de l'incident :
  agents (/agent/*, check-ins + fichiers portail hybride), portail (endpoints
  publics des invités /api/wifi/site/* + track + /portal/{token}), medias
  (/api/media/*, proxy R2), console (le reste de /api/*), autre (santé, 404 des
  bots) ; verrou dédié jamais pris pendant un handler (incrémentations aux
  Write, hors de tout verrou du store) ; reset au changement de jour UTC (la
  fenêtre de facturation Render est calée sur le mois calendaire UTC) ; le total
  est une BORNE BASSE documentée (en-têtes HTTP non mesurables côté application,
  ~200-500 o par réponse en plus).
- **Endpoint** : GET /api/admin/sync-status (N°71) enrichi d'un bloc
  « bandwidth » {day, totalRequests, totalBytes, categories[5]} — catégories dans
  l'ordre canonique, toujours 5 (contrat stable), absentes = zéro ; la requête
  en cours est comptée en « console » (startRequest AVANT le handler) : le
  rapport se mesure lui-même.
- **Frontend — carte Maintenance** : bloc « Bande passante sortante » dans la
  carte « Santé de la persistance » (SyncStatusCard) : total du jour
  formatBytes (o/Ko/Mo localisés, séparateur décimal fr/en) + nombre de
  requêtes, répartition par catégorie (Agents/Portail/Médias/Console/Autre),
  hint explicite « corps uniquement, borne basse du quota Render » ; i18n FR/EN
  6+6 clés platformSettings.syncHealth.bw* ; types BandwidthSnapshot/
  BandwidthCat. ET use-router-resources.ts : l'entretien des ressources routeur
  (pools/files/serveurs des formulaires) passe de 15 s à 60 s — chaque re-poll
  en mode agent finit par déclencher une commande read_resources (trafic 24 h/24
  pour des listes qui changent rarement) ; le re-poll accéléré 5 s pendant
  qu'une commande est en file reste inchangé (borné par le check-in ≤ 45 s) :
  la réactivité des formulaires ne bouge pas, staleTime aligné à 60 s.
- **Positionnement architectural** : les middlewares de main.go (CORS
  fail-closed, securityHeaders, limitBody, rate-limit S1-S6, log) restent
  AU-DESSUS de la chaîne N°72 (observeEgress → gzip → auth) : les réponses
  propres de ces middlewares (preflight 204 CORS, 429 du limiteur) ne sont ni
  compressées ni comptées — volume nul ou marginal, et la chaîne de sécurité
  n'est pas touchée.
- **Tests Go 11 nouveaux** (gzip_test.go : compression réelle du JSON santé
  avec corps décompressé IDENTIQUE au corps clair + en-têtes Content-Encoding/
  Vary ; la réponse volumineuse sync-status est effectivement PLUS PETITE
  compressée (la promesse ÷4-8, prouvée) et reste du JSON valide après
  décompression ; 204 du track jamais compressé ; binaire jamais compressé ;
  parsing Accept-Encoding avec/sans q= ; filtre des Content-Type ;
  httpstats_test.go : classification des 19 chemins pivots (miroir du découpage
  public/console de l'allowlist — « /api/wifi/site/ » ne matche ni « /sites »
  ni « /guests »), compteurs + ordre canonique + reset journalier UTC,
  comptage réel au travers du serveur (la santé GET / écrit en « autre », la
  requête sync-status se compte elle-même en « console ») ;
  handlers_sync_status_test.go étendu : le contrat JSON de N°71 vérifie
  désormais le bloc bandwidth (5 catégories, ordre, totalRequests ≥ 1,
  console.requests ≥ 1).
- **Vérifié localement comme la CI** : gofmt tabulations (vide), go vet, go
  build, go test paquet api COMPLET (tous les tests existants traversent
  désormais le middleware gzip — la décompression transparente du client de test
  valide l'équivalence octet par octet), -race sur les nouveaux tests, paquet
  store inchangé vert ; frontend eslint 0, tsgo 0, next build ✓ (mêmes 13
  routes) ; E2E navigateur : backend Go :4000 (binaire frais — un premier
  passage E2E avait exposé le piège WriteHeader avec l'ancien binaire) +
  frontend build prod :3001, login admin plateforme → Paramètres plateforme →
  Maintenance → bloc « Bande passante sortante » aux valeurs réelles (total
  « 938 o · 11 requêtes », répartition console comptée depuis le boot, hint
  complet), collapsible 30 tables N°71 intact, console navigateur zéro erreur
  JavaScript, mobile 390 px scrollWidth 390 sans débordement, captures
  desktop + mobile.

## 2026-09-09 — N°71 : santé de la persistance — endpoint GET /api/admin/sync-status + carte « Santé de la persistance » (console plateforme, onglet Maintenance) + geniuspay_subs intégré à la synchro différentielle

### N°71 — Instrumenter deux flux silencieux : la synchro différentielle FNV-1a → Neon et les agents routeur (demande utilisateur, application directe de la leçon d'architecture)
- **Demande** : la synchro différentielle ne laissait AUCUNE trace observable — un échec
  n'existait que dans une ligne de journal Render, et rien ne disait à l'opérateur si Neon
  recevait bien les deltas, à quel rythme, ni combien de lignes voyageaient. Avant le
  lancement commercial, « est-ce que mes données tiennent si Render redémarre ? » ne doit
  pas dépendre d'un tail de logs. L'endpoint rend la leçon d'architecture OPÉRABLE.
- **Backend — compteurs (internal/store/syncstats.go, NOUVEAU)** : `syncStats` —
  micro-verrou dédié, jamais tenu pendant une transaction SQL (ordre store.mu → stats.mu
  toujours le même : aucun interblocement, aucune contention mesurable sur le chemin
  critique) : tentatives/succès/échecs, chaîne d'échecs consécutifs, horodatage + durée +
  volumétrie (lignes upsertées/supprimées) du DERNIER delta réussi, dernière erreur bornée
  à 500 caractères. `Store.SyncHealth()` photographie sous verrou : mode
  (postgresql|json), compteurs, dernier contact Neon CONFIRMÉ (`lastWrite`, existant mais
  jamais exposé), mode du keep-alive (mémorisé au démarrage), et par table les lignes
  MÉMOIRE vs RÉPLIQUÉES (taille du cache d'empreintes) — une dérive qui persiste signale
  une synchro qui n'aboutit plus alors que l'état continue d'évoluer.
- **Backend — instrumentation (pg.go)** : `PG.Sync` passe en retour nommé + defer (corps
  transactionnel inchangé, aucun verrou ajouté) ; `syncTable` porte un `*syncDelta`
  rempli uniquement pour les lignes réellement écrites (cohérent avec le rafraîchissement
  du cache après succès) ; `upsertRows`/`deleteRows` généralisés : cible de conflit et
  colonne de suppression = `cols[0]` de la spec au lieu du « id » codé en dur.
- **BUG CORRIGÉ (détecté en construisant N°71)** : la table `geniuspay_subs` (abonnements
  carte Stripe via GeniusPay) était CHARGÉE au boot (loadInto + spec existante) mais
  échappait à la synchro différentielle — sa clé primaire « uuid » (≠ « id ») ne passait
  pas le ON CONFLICT codé en dur. Conséquence : les abonnements créés en mémoire (avec
  `a.store.Save()` bien appelé, handlers_subscription_stripe.go) DISPARAISSAIENT au
  redémarrage. Correctif : machinerie générique ci-dessus + enregistrement dans `Sync` ET
  `rebuildHashes` — zéro migration (la table existait déjà au schéma idempotent), les
  lignes éventuelles convergent au premier Save.
- **Backend — endpoint GET /api/admin/sync-status (handlers_sync_status.go, NOUVEAU)** :
  double garde identique à /overview (requireRole(3) à l'enregistrement + isPlatformAdmin,
  401/403 testés) ; réponse `{mode, sync|null, neon|null, tables, agents}` — STRICTEMENT
  READ-ONLY (aucune mutation, aucun verrou nouveau) ; bloc agents calculé dans le package
  api (propriétaire des constantes de fraîcheur) : routeurs par mode, agents en ligne par
  FRAÎCHEUR du check-in (< OnlineWindow 3 min — la vérité est le LastSeen, pas le champ
  Status posé au dernier passage), conflits d'identité S6, file de commandes
  (queued/sent/zombies > staleSentLimit 10 min), dernier check-in.
- **Frontend — carte « Santé de la persistance » (platform-settings-view.tsx, onglet
  Maintenance, 2ᵉ position)** : TanStack Query (rafraîchissement auto 15 s), Badge mode
  (PostgreSQL (Neon) | Fichier local (développement) + hint dédié en mode JSON), lignes
  StatRow (dernière synchro ago · date, durée, delta ±, tentatives/succès/échecs, dernier
  contact Neon, keep-alive), bloc agents (en ligne X/Y mode agent, file, envoyées,
  zombies, dernier check-in), alerte destructive avec dernière erreur en mono quand la
  chaîne d'échecs > 0, badges échecs consécutifs et conflits S6, Collapsible « N lignes en
  mémoire · M répliquées » → tableau scrollable (max-h-64) Table/Lignes/Répliquées (« — »
  hors mode différentiel), skeletons en chargement, i18n FR/EN 32+32 clés
  (platformSettings.syncHealth.*).
- **Tests Go (7 fonctions nouvelles)** : store — TestSyncStatsRecordAndSnapshot
  (compteurs, chaîne d'échecs remise à zéro, volumétrie du dernier delta, recordFailure
  nil no-op), TestSyncStatsErrorBorne (troncature 500), TestSyncHealthJSONMode (mode json
  : sync/neon absents, 30 tables, mirrored omis), TestLiveTableRowsConcordance (30
  entrées uniques = 29 différentielles + settings — tripwire si une table est ajoutée à
  Sync sans être listée) ; api — TestSyncStatusRoleMatrix (401 sans jeton, 403 owner
  client « Réservé », 403 manager « rôle insuffisant », 200 plateforme),
  TestSyncStatusContract (mode json, sync/neon null, 30 tables aux comptes exacts du seed,
  agents 1/1 en ligne, file 1/2 dont 1 zombie, lastCheckIn renseigné),
  TestSyncStatusAgentOffline (agent vu il y a 30 min : 1 agent / 0 en ligne).
- **Vérifié localement comme la CI + navigateur** : gofmt TABULATIONS conforme, go vet
  0 erreur, build ✓ ; eslint EXIT 0, tsgo --noEmit EXIT 0, next build ✓ ; E2E
  agent-browser (backend Go mode JSON port 4000 + frontend dev port 3001, admin
  plateforme) : login → console → Paramètres plateforme → onglet Maintenance → carte
  complète (Badge « Fichier local (développement) » + hint, agents « 1 / 1 (mode
  agent) », file 1, envoyées 2, zombies 1, check-in « il y a 19 s »), Collapsible ouvert →
  tableau des 30 tables (admin_users 1, routers 1, commands 3, « — » répliquées), console
  ZÉRO erreur, mobile 390 px scrollWidth 390 (aucun débordement), captures desktop +
  mobile.

## 2026-09-09 — N°70 : page publique /legal/confidentialité + case « politique de confidentialité » à l'inscription

### N°70 — Conformité pré-lancement : publier la politique de confidentialité (registre des traitements §6.1, dernier TODO technique bloquant)
- **Demande** : le registre des traitements (docs/REGISTRE-TRAITEMENT.md §6)
  conditionne le lancement commercial à la publication de la politique sur une
  page publique (`/legal/confidentialite`) **liée depuis l'inscription (case à
  cocher)** — droit à l'information, loi 2013-450. Depuis N°69, le téléphone
  des invités WiFi est collecté avec opt-in prouvé : la base légale tient, mais
  l'information publique manquait.
- **Page `/legal/confidentialite`** : composant **serveur** (contenu statique,
  zéro JavaScript client, métadonnées title/description/keywords pour le
  référencement), mobile-first (tables en défilement horizontal, cibles
  tactiles ≥ 44 px), thèmes jour/nuit via tokens existants, FR par convention
  des pages visiteurs (**la version française fait foi**, la loi de référence
  est ivoirienne). Contenu fidèle au registre : traitements **T1-T6**
  (T6 = marketing WiFi N°69 : consentement art. 9, OFF par défaut, retrait
  symétrique « Ne plus recevoir », durée jusqu'au retrait/suppression du
  site), sous-traitants (Neon UE eu-central-1 / Render / Vercel / Wave &
  GeniusPay — aucune donnée carte ne transite), stockage local (localStorage
  `mikcloud-auth` session de travail + file IndexedDB du Mode Vente purgée
  après synchronisation ; aucun traceur tiers, pas de profilage), sécurité
  (bcrypt coût 12, JWT 24 h révocables, 2FA TOTP, TLS/HSTS, CORS fail-closed,
  sauvegardes AES-256-GCM avec test de restauration hebdomadaire, journalisation
  des échecs d'auth, rate limiting, chaîne auditée), droits (information,
  accès/portabilité CSV + export complet chiffré sur demande, rectification,
  suppression anonymisée sous 30 j — comptabilité anonymisée 5 ans —,
  opposition/limitation, réclamation CDP/ARTCI), violations (qualification
  48 h, **notification ARTCI/CDP 72 h**, information des personnes si risque
  élevé), contact `privacy@mikcloud.ftci.fr` (registre §6.2). En-tête avec
  logo + retour accueil ; pied collant en bas de fenêtre (min-h-screen flex +
  mt-auto) avec crédit FTCI et mention du responsable de traitement.
- **Registre enrichi (même commit)** : ligne **T6** ajoutée — le traitement
  marketing de N°69 (téléphone + `opt_in_at`) n'était pas encore consigné ;
  §2 complété du stockage IndexedDB du Mode Vente (N°61) ; §6.1 marqué fait,
  §6.2 précisé (boîte mail à activer côté opérateur). La source de vérité du
  contenu de la page EST le registre : toute évolution future doit être
  répercutée dans les DEUX fichiers du même commit.
- **Inscription (wizard étape 2)** : case « J'ai lu et j'accepte la
  politique de confidentialité » avec lien vers la page (nouvel onglet —
  le formulaire en cours n'est pas perdu), **« Créer mon compte » reste
  désactivé sans acceptation** (canSubmitStep2), réinitialisée avec les
  autres champs à la fermeture ; clés i18n FR/EN
  (`signup.privacyPrefix` / `signup.privacyLink`).
- **Vitrine** : lien « Politique de confidentialité » / « Privacy policy »
  dans le pied de page (libellé via `landing-copy.ts` fr + en, à côté du
  crédit FTCI, focus visible).
- **Portail captif inchangé volontairement** : la note de confidentialité
  dynamique (N°65 : jours de rétention réels par compte) reste LA mention du
  portail ; `/legal` n'est lié que depuis des surfaces NON captives (vitrine,
  inscription) — aucun walled-garden supplémentaire à ouvrir, zéro impact
  routeur.
- **Zéro backend / zéro migration** : page statique frontend uniquement —
  aucun changement de schéma Neon (convergence au boot inchangée), pas de
  redéploiement Render attendu (diff limité à `frontend/` + `docs/`).

## 2026-09-08 — N°69 : opt-in marketing explicite (interrupteur) au claim WiFi — /wifi + portail captif

### N°69 — Consentement marketing légalement valable sur les DEUX surfaces de claim (demande utilisateur)
- **Demande** : le claim du WiFi jetable collecte le numéro de téléphone, mais
  sans opt-in valable le registre est inutilisable pour le marketing. Pas de
  case à cocher (l'ancienne était PRÉ-COCHÉE — consentement juridiquement
  nul, Planet49/CJUE + loi ivoirienne n°2013-450), pas de cadeau/promesse,
  pas de double bouton : un **interrupteur discret**, OFF par défaut.
- **Page `/wifi/{slug}`** : la case pré-cochée est remplacée par un
  **interrupteur shadcn** « Me tenir informé des actualités — 2 messages/mois
  · STOP gratuit à tout moment » — OFF PAR DÉFAUT (consentement **univoque** :
  l'action affirmative du visiteur crée le consentement), finalité + fréquence
  + moyen de retrait annoncés dans le libellé (**éclairé**), toute la ligne
  tactile, ne rien toucher = refus sans pénalité — le code arrive pareil
  (**libre**). Affiché seulement si le site a le marketing activé
  (réglage console du wizard, défaut ON).
- **Portail captif `login.html`** : même interrupteur (~30 lignes CSS pur,
  compatible routeur, focus visible) rendu dans le formulaire de claim inline
  quand `cfg.marketingOptIn === true` — l'ancien `optIn: false` EN DUR (zéro
  consentement collecté sur la voie portail) est remplacé par l'état réel de
  l'interrupteur. Propagation aux portails déployés par la config LIVE
  (endpoint + fallback inliné, `HotspotFilesSig` au check-in ≤ 45 s) ;
  `undefined` (portails antérieurs) = false = pas d'interrupteur, le fetch
  live corrige au chargement.
- **Preuve opposable** : nouvelle colonne `WifiGuest.OptInAt` (RFC3339) —
  QUI a consenti, QUAND, via quel geste. L'état suit le **NUMÉRO** (toutes
  les lignes du registre du même téléphone portent le même état), pas la
  ligne du jour. Migrations Neon idempotentes (ALTER `opt_in_at`, spec
  Load/Sync 14 colonnes — pattern N°47/N°50, zéro impact comptes existants).
- **Héritage + upgrade** : un numéro déjà consenti garde son consentement
  (claim du lendemain interrupteur non touché ⇒ héritage — ne rien toucher
  n'est pas un retrait) ; au re-claim idempotent du même jour, poser
  l'interrupteur ENREGISTRE le consentement immédiatement (upgrade, même
  code). Le sens inverse n'existe PAS par omission — le retrait est
  explicite.
- **Retrait symétrique « Ne plus recevoir »** : carte code de `/wifi` —
  ligne d'état discrète « Vous recevez les actualités · Ne plus recevoir »
  (1 geste, exactement comme le consentement), silencieuse si non abonné.
  `POST /api/wifi/site/{slug}/consent` PUBLIC, mêmes gardes que le claim :
  rate-limit wifiClaim (20/10 min + 100/24 h par IP), honeypot « website »
  (succès factice aux bots, zéro écriture), validation téléphone 8-15
  chiffres, réduction par `site.MarketingOptIn` (marketing éteint ⇒ un
  opt-in sauvage ne s'enregistre pas). Retire TOUTES les lignes du numéro +
  efface la preuve ; possible même site en pause/compte expiré (un droit de
  retrait ne se suspend jamais). Journal d'activité côté gérant
  (activation/retrait, téléphone masqué).
- **Console gérant** : registre invités — le ✓ opt-in porte la date de
  preuve (`optInAt`, tooltip horodatage) ; export CSV colonne
  `opt_in_since` (la « base marketing » légale = filtre optIn + cette
  colonne, « - » si retiré/jamais consenti).
- **Contrat API** : claim + status renvoient `optIn` (état EFFECTIF du
  numéro, héritage inclus) — la carte code l'affiche même à un re-scan sans
  nouveau claim. `PortalConfig.MarketingOptIn` sérialisé SANS omitempty
  (false toujours explicite), posé par les DEUX builders (token agent +
  slug live).
- **Tests Go** : 7 nouveaux cas — interrupteur OFF/ON (preuve RFC3339),
  réduction par MarketingOptIn, upgrade au re-claim idempotent, héritage,
  retrait + ré-activation via /consent (+ /status suit le numéro), gardes
  (400/404/honeypot/réduction), CSV opt_in_since + omitempty JSON ;
  template servi `marketingOptIn` true ET false explicite + interrupteur
  embarqué. i18n : page visiteur déjà 100 % FR (convention existante).

## 2026-09-08 — N°68 : « Mot de passe oublié ? » (lien e-mail à usage unique)

### N°68 — Réinitialisation du mot de passe depuis l'écran de connexion (demande utilisateur)
- **Demande** : option « mot de passe oublié » en fenêtre modale sur la page
  de connexion ; l'utilisateur saisit l'e-mail enregistré à la création de son
  compte — e-mail inconnu → signalé ; sinon réception d'un lien de
  réinitialisation valable une durée déterminée (60 minutes) et utilisable
  une seule fois.
- **Modale (login)** : lien « Mot de passe oublié ? » sous le champ mot de
  passe de l'onglet Console → `ForgotPasswordModal` (composant
  `parts/forgot-password-modal`) — saisie e-mail validée localement, envoi
  `POST /api/auth/forgot-password`, toast d'erreur (le message du backend
  signale explicitement « Aucun compte n'est associé à cet e-mail »), écran de
  confirmation après envoi (durée de validité + rappel anti-spam). Cohérence
  visuelle avec la modale d'inscription (mêmes animations, carte qui tremble
  en erreur).
- **Page publique `/reset-password?token=…`** : consommation du lien e-maillé
  — mobile-first (ouvert depuis un client mail : colonne centrée max-w-md,
  safe-area iOS, cibles ≥ 44 px, bascule de langue FR/EN, même coquille que
  `/join/[token]`) — nouveau mot de passe + confirmation (miroir client de la
  politique S2), affichage optionnel, états succès (retour /login) et lien
  invalide/expiré/déjà utilisé (carte d'état + raison du backend).
- **Backend** (`internal/api/password_reset.go`) :
  - `POST /api/auth/forgot-password` {email} — public, quota IP (5/10 min +
    20/24 h, même limiteur que l'inscription S3) : compte recherché par e-mail
    (trim + insensible à la casse), compte désactivé → 403, token aléatoire
    256 bits base64url **stocké HASHÉ en SHA-256** (le clair n'est jamais
    persisté — une fuite de la base ne permet aucune réinitialisation),
    expiration 60 minutes, e-mail transactionnel (sujet, lien, durée, mention
    usage unique, avertissement « si vous n'êtes pas à l'origine… ») ;
  - `POST /api/auth/reset-password` {token, password} — public : hash →
    recherche, **usage unique** (UsedAt), **expiration stricte**, politique S2
    centralisée (10 caractères, denylist, ≠ identifiant), bcrypt +
    `PasswordSetByUser` (protège contre l'override ADMIN_PASSWORD), **révocation
    de TOUTES les sessions** (SessionEpoch++, même garde que le changement de
    mot de passe classique), journal d'activité des deux étapes ;
  - une nouvelle demande **invalide les liens en attente** du même compte (un
    seul lien vivant) ; purge paresseuse des lignes > 24 h (registre borné,
    sans cron) ; origine du lien : APP_PUBLIC_URL > origine de la requête **si
    autorisée (ALLOWED_ORIGIN)** > URL canonique du frontend — jamais une
    origine forgeable (anti-phishing du lien).
- **E-mails transactionnels** (`internal/notify`) : `SendEmailTo` (Resend ou
  SMTP selon le provider du compte — N°67 — avec destinataire fourni par
  l'appelant) + `EmailCredentialsOK` (identifiants seuls, sans exiger
  EmailEnabled/EmailTo des alertes) + `KindPasswordReset`. Chaîne
  d'expédition : réglages e-mail du COMPTE demandeur, à défaut ceux du compte
  principal (plateforme) — en production le compte principal est déjà
  configuré Resend depuis N°67 : **les clients n'ont rien à régler**, le lien
  part dès le déploiement. Historique `notif_log` (kind `password_reset`,
    statut sent/error) pour chaque envoi.
- **Migrations Neon automatiques** (ensureSchema au boot, idempotentes) :
  nouvelle table `password_resets` (8 colonnes, PK id) + 2 index
  (account_id, token_hash) — table additive, invisible pour les versions
  antérieures du backend ; `passwordResetSpec` (Load/Sync/rebuildHashes).
- **Sécurité** : routes publiques ajoutées à l'allowlist du middleware
  d'authentification ; envoi réseau hors verrou du store (règle du moniteur) ;
  réponse « 503 Envoi d'e-mail indisponible » si aucun fournisseur configuré ;
  429 + Retry-After au-delà du quota IP.
- **Périmètre assumé** : la réinitialisation cible le PROPRIÉTAIRE du compte
  (porteur de l'e-mail d'inscription) — les membres d'équipe passent par leur
  gérant, l'admin plateforme (sans e-mail de compte) garde les canaux
  ADMIN_PASSWORD/support.
- **Compatibilité** : aucun changement de contrat existant (login, register,
  /api/auth/password inchangés) ; tests Go dédiés (10 cas : inconnu 404,
  désactivé 403, sans fournisseur 503, flux complet, invalidation par nouvelle
  demande, expiration, denylist, quota 429, origine du lien, token non
  persisté en clair).

## 2026-09-08 (00h11 UTC) — N°67 : Resend comme fournisseur du canal e-mail (alternative à SMTP)

### N°67 — Notifications par e-mail via l'API Resend (demande utilisateur)
- **Demande** : « je souhaite implémenté resend » — intégrer le service
  d'e-mail Resend (https://resend.com) au système de notifications.
- **Fournisseur e-mail choisissable par compte** : le canal e-mail de la vue
  Paramètres → Notifications gagne un sélecteur « Fournisseur » — **SMTP
  direct** (comportement historique, défaut) ou **Resend (API)**. Les deux
  partagent le même destinataire, les mêmes règles d'alerte (routeur hors
  ligne, stock bas, rapport quotidien), le même test d'envoi et le même
  historique (`channel: email`).
- **Backend** (`internal/notify`) : `sendEmailResend` — POST
  `https://api.resend.com/emails` (Authorization Bearer, JSON from/to/subject/
  text), timeout borné 12 s comme les autres canaux, messages d'erreur Resend
  repris tels quels (« clé invalide », « domaine non vérifié », rate limit) ;
  expéditeur vide → `MikCloud <onboarding@resend.dev>` (domaine d'essai :
  ne délivre qu'au propriétaire du compte Resend). `Deliver` et `Configured`
  dispatchent selon `EmailProvider` — le moniteur automatique
  (routeurs/stock/rapport) bénéficie du provider sans autre changement.
- **Contrat des secrets inchangé** : la clé API Resend est stockée par compte
  dans `notif_settings`, **jamais renvoyée par l'API** (seul le booléen
  `resendApiKeySet` l'annonce) ; un PUT avec champ vide conserve la valeur
  stockée (même contrat que le mot de passe SMTP et les tokens
  Telegram/WhatsApp). L'expéditeur `resendFrom` est un champ ordinaire
  (vidable). Toute valeur de provider autre que `resend` retombe sur SMTP.
- **Migrations Neon automatiques** (ensureSchema au boot, idempotentes) :
  `notif_settings.email_provider`, `resend_api_key`, `resend_from`
  (TEXT NOT NULL DEFAULT '') ; `notifSettingsSpec` étendu (25 colonnes) —
  compatibilité stricte : les comptes existants restent sur SMTP
  (`email_provider = ''`), aucune donnée migrée.
- **Frontend** : carte « E-mail » de la vue Notifications — sélecteur
  shadcn/ui, champs Resend (clé API masquée + placeholder « configuré »,
  expéditeur avec aide sur le domaine vérifié) ou champs SMTP selon le
  fournisseur, destinataire commun ; garde « prêt pour test » adaptée par
  fournisseur ; i18n FR/EN (9 clés neuves + `emailDesc` généralisée).
- **Tests** (`internal/notify`) : `EmailProviderOf` (normalisation y compris
  casse/espaces), `Configured` provider resend (clé requise, SMTP ne suffit
  pas, canal désactivé), `sendEmailResend` contre un serveur httptest (Bearer,
  payload JSON, from par défaut/explicite, erreurs JSON reprises, repli HTTP).
  Aucun réseau réel en CI.

## 2026-09-09 — N°66 : limite d'appareils simultanés par compte revendeur (Mode Vente)

### N°66 — Anti-partage du PIN : le gérant borne le nombre de téléphones connectés en même temps (demande utilisateur)
- **Demande** : « donner la possibilité au gérant/propriétaire de limiter le
  nombre d'appareils simultanés sur lesquels le compte revendeur peut se
  connecter » — la cible est le COMPTE REVENDEUR (login PIN de la PWA Mode
  Vente), pas l'utilisateur hotspot final (déjà couvert par `shared-users`
  au niveau profil RouterOS).
- **Nouvelle propriété `maxDevices` sur le revendeur** (0-20, 0 = illimité,
  défaut 1 à la création via le formulaire) : settable à la création et à
  l'édition dans la vue Revendeurs (champ « Appareils simultanés » avec
  garde-fou 0-20 des deux côtés).
- **Registre de sessions `sell_sessions`** (nouvelle table Neon) : un login
  PIN d'un revendeur limité inscrit une session (jti embarqué dans le JWT,
  user-agent + IP horodatés). Chaque requête `/api/sell/*` recontrôle la
  présence de la session au registre — l'appareil évincé reçoit 401, que
  la PWA traite comme une fin de session (retour à l'écran PIN).
- **Politique d'éviction FIFO** : au-delà de la limite, le nouvel login
  déconnecte l'appareil connecté depuis le plus longtemps ; **baisser la
  limite déconnecte immédiatement les surnuméraires** (trim à l'édition) ;
  **activer la limite (0 → N) révoque les tokens en vol** (sans jti → 401
  « Session réinitialisée », le login suivant régularise l'appareil) ;
  supprimer le revendeur purge son registre (aucun orphelin).
- **Purge autonome** : les sessions plus vieilles que le TTL du token
  (24 h) + 1 h de grâce sont purgées au login suivant du même revendeur —
  registre borné, aucun balayage global, aucun cron.
- **Compteur live sur la carte console** : « X/N appareils connectés »
  (registre vivant) sous le téléphone du revendeur, seulement si une limite
  est définie ; journal d'activité du compte trace chaque éviction.
- **Compatibilité stricte** : `maxDevices = 0` (tous les revendeurs
  existants) conserve le comportement historique — token stateless, aucune
  écriture de registre, aucun contrôle par requête.
- **Migrations Neon automatiques** (`ensureSchema`) : `resellers.max_devices`
  (INTEGER NOT NULL DEFAULT 0) + table `sell_sessions` + index
  `idx_sell_sessions_reseller` ; synchro différentielle étendue
  (`sellSessionSpec`).
- **Tests** (`sell_devices_test.go`) : éviction FIFO + trim à la baisse,
  activation révoquant les tokens legacy (+ token forgé sans jti refusé),
  illimité stateless intact, bornes de validation 400.

## 2026-09-08 — N°65 : Rétention du journal paramétrable par compte (30/60/90 j, défaut 90)

### N°65 — Journaux utilisateurs : la durée de conservation devient un réglage PAR COMPTE (recommandation 2 de l'audit rétention)

- **Constat** : la rétention à 90 jours (N°64) était une constante globale
  (`store.userLogRetention`) — tous les comptes subissaient la même durée,
  alors que la minimisation (RGPD/ARTPD, loi ivoirienne 2013-450) appelle
  un paramétrage par exploitant selon son besoin propre (litiges,
  obligations locales, volume).
- **Réglage par compte** (`tenant.logRetentionDays`, 30/60/90 j, défaut 90) :
  - pattern **zéro-migration** du codebase (pointeur, cf. `joinButton`) :
    nil = défaut 90 sans écrire le champ dans le JSON persisté — les
    comptes existants gardent EXACTEMENT le comportement N°64 ; la colonne
    Neon `settings.log_retention_days` (NOT NULL DEFAULT 90, `ALTER IF NOT
    EXISTS`) reporte la valeur effective au premier Save ;
  - contrat **borné côté serveur** : `PUT /api/settings` n'accepte que 30,
    60 ou 90 (formes plate + imbriquée `tenant{…}`, mêmes résolutions
    défensives que les autres réglages) — toute autre valeur est refusée
    en 400 ; une valeur invalide glissée en base retombe sur 90
    (`LogRetentionDaysEffective`, jamais de rétention illimitée) ;
  - **moteur** : `applyExpiry` (étape 3) purge chaque log selon la rétention
    de SON compte (cutoffs par `SettingsByAccount`) — le balayage
    périodique N°64 (horaire + rattrapage boot) applique la valeur du
    compte sans aucun changement de scheduling ; le garde-fou volumétrie
    (5 000 dernières entrées) reste GLOBAL.
- **Console** (section Paramètres → Général, propriétaire) : carte « Rétention
  du journal » — sélecteur 30/60/90 jours (« 90 jours (défaut) » libellé),
  hint reprenant la mécanique (purge horaire, note portail, plafond 5 000) ;
  la **bannière de la vue Journal** devient dynamique (durée effective du
  compte affichée, FR/EN).
- **Portail captif** (`login.html`) : la note de confidentialité du footer
  devient **dynamique** — « conservées N jours maximum » où N est la
  rétention du compte, portée par le fallback inliné ET l'endpoint live
  (`PortalConfig.LogRetentionDays`, mis à jour par `applyConfig` — les
  portails déjà déployés reflètent le réglage sans re-déploiement ;
  repli 90 pour les configs antérieures).
- **Registre des traitements** (`docs/REGISTRE-TRAITEMENT.md`) : la durée de
  la ligne T2 documente désormais le journal de connexion (30/60/90 j
  selon réglage du compte, défaut 90, purge automatique horaire).
- **Tests** : `TestSettingsLogRetention` (validation 30/60/90, refus 400 des
  autres valeurs, nil = inchangé, repli nested, GET reflète),
  `TestApplyExpiryLogRetentionPerAccount` (purge par compte : 45 j purgé à
  30 j, conservé à 90 j, valeur invalide → 90) et
  `TestPortalServeLogRetention` (login.html servi porte
  `"logRetentionDays":30` + le span de la note dynamique).
- **Portée** : backend (7 fichiers) + frontend (3 fichiers) + template
  portail + registre ; ajout de champ purement additif au contrat
  `PUT/GET /api/settings` et au bloc config du portail — aucun contrat
  existant ne change.

## 2026-09-08 — N°64 : Balayage de rétention périodique + transparence (privacy/audit)

### N°64 — Journaux utilisateurs : la purge à 90 jours devient une garantie stricte, vérifiable et documentée

- **Constat** : la rétention du journal utilisateurs (90 j + plafond 5 000,
  `applyExpiry`) ne s'exécutait qu'au fil des **lectures console**
  (`store.Tick` en tête des handlers — balayage *paresseux*). Un compte
  dormant, jamais consulté, conservait ses journaux connexion au-delà des
  90 jours annoncés : la garantie n'était vraie que pour les comptes actifs.
- **Balayage périodique** (`internal/api/retention.go`, goroutine `main.go`) :
  - rattrapage **immédiat au démarrage** (le service Render redémarre
    souvent — chaque boot nettoie ce qui doit l'être), puis passage
    **toutes les heures** (`time.Tick`, pattern des fenêtres de rate-limit) ;
  - le passage reprend EXACTEMENT le cœur commun des handlers —
    `store.Sweep` (applyExpiry : expirations vouchers, politique « remove »,
    purge UserLogs > 90 j + plafond 5 000) puis `enforceExpired` (commandes
    agent, réparation limit-uptime, lots morts, inscriptions stalées 30 j) —
    **sans** la progression de la simulation (sessions/uptime/télémétrie
    restent au rythme des polls) ;
  - purge **réelle** en base (Save → syncTable : les lignes `user_logs`
    sont supprimées de PostgreSQL/Neon, pas seulement masquées).
- **Preuve d'audit** : `db.LastSweep` (persistée, colonne
  `settings.last_sweep`, même mécanique que `last_tick`) exposée par
  `GET /` → `lastSweepAt` — un auditeur vérifie en un curl que le balayage
  vit ; chaque purge non vide est tracée dans le log service.
- **Transparence côté portail captif** (`login.html`) : note
  confidentialité discrète dans le footer — « Données de connexion
  conservées 90 jours maximum, puis supprimées automatiquement » (cadenas
  teal) — alignée sur la purge réelle (RGPD/ARTPD, minimisation).
- **Transparence côté console** (vue Journal utilisateurs) : bannière
  technique `ShieldCheck` — 90 jours max + purge horaire même sans visite,
  ET le garde-fou volumétrie : les 5 000 dernières entrées sont conservées,
  un site à fort trafic peut voir son journal **élagué avant 90 jours**
  (acceptable en privacy, à savoir pour l'audit technique). FR/EN.
- **Portée** : backend Go (6 fichiers + `retention.go`) + frontend (2
  fichiers) ; zéro changement de contrat API existant (ajout du seul champ
  d'information `lastSweepAt` au healthcheck public `GET /`).

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
