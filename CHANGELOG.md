# CHANGELOG — MikCloud

Historique des évolutions notables du projet. Format inspiré de
[Keep a Changelog](https://keepachangelog.com/) ; les versions correspondent
aux dates de livraison — le déploiement est continu : chaque push `main` passe
la CI puis se déploie automatiquement (frontend Vercel, backend Render).

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
