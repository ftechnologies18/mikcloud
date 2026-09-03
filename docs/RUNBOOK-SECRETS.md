# RUNBOOK — Secrets & procédures sensibles (Sécurité S4)

> Document opérateur MikCloud — à conserver HORS du périmètre public (les
> procédures sont publiques, les secrets jamais). Dernière mise à jour :
> vague S4 (2026-09-03).

## 0. Inventaire des secrets de production

| Secret | Usage | Emplacement légitime | Rotation conseillée |
|---|---|---|---|
| `JWT_SECRET` / `SECRETBOX_KEY` | Signature des tokens, chiffrement des secrets stockés | Variables d'environnement Render | 12 mois ou sur incident |
| `ADMIN_PASSWORD` | Bootstrap de l'admin plateforme | Variable Render | 6 mois |
| GitHub PAT (`GH_TOKEN`) | Push, API dépôt, déclenchement CI | Coffre de l'opérateur (1Password/Bitwarden) | **90 jours** |
| Neon API token (`napi_…`) | Gestion du projet Neon | Coffre de l'opérateur | 6 mois |
| Neon DSN (`postgresql://…`) | Connexion base du backend | Variable Render `DATABASE_URL` + secret GitHub `DATABASE_URL` (workflow backup) | Sur fuite ou départ d'un collaborateur |
| Render API key (`rnd_…`) | Déclenchement des déploiements (CI → job deploy-render) | Secret GitHub `RENDER_API_KEY` + coffre | 6 mois |
| Vercel token (`vcp_…`) | CLI/CI Vercel | Coffre | 90 jours |
| `BACKUP_KEY` | Chiffrement des sauvegardes mikbackup | Secret GitHub `BACKUP_KEY` + coffre de l'opérateur | 12 mois (la rotation impose un nouveau cycle de sauvegarde complet) |
| Webhook GeniusPay HMAC | Anti-falsification webhooks | Variable Render | Sur incident |

Règles transverses :
- **Aucun secret dans le dépôt** (le secret scanning + push protection S3
  bloquent les poussées accidentelles) — les secrets vivent dans les
  variables d'environnement des plateformes et dans le coffre de l'opérateur.
- Tout secret collé dans un canal non coffré (chat, ticket, capture d'écran)
  est considéré **compromis** : rotation immédiate (procédure §2).

## 1. Où sont configurés les secrets côté plateformes

- **Render** (backend `mikcloud`, service `srv-da974o142hec73euul60`) :
  Dashboard → Environment → Variables (`DATABASE_URL`, `JWT_SECRET`,
  `ADMIN_PASSWORD`, `REGISTER_KEY`, `WEBHOOK_…`).
- **Vercel** (frontend) : Settings → Environment Variables
  (`NEXT_PUBLIC_API_BASE`).
- **GitHub** (repo `ftechnologies18/mikcloud`) : Settings → Secrets and
  variables → Actions (`RENDER_API_KEY`, `BACKUP_KEY`, `DATABASE_URL`).
- **Neon** : Console → projet → (branches, rôles, réinitialisation du mot de
  passe du rôle).

## 2. Procédures de rotation (à froid, < 30 min chacune)

### 2.1 GitHub PAT
1. GitHub → Settings → Developer settings → Personal access tokens →
   *Generate new token* (scopes : `repo`, `workflow`, `admin:repo_hook`).
2. GitHub → repo → Settings → Secrets → Actions : mettre à jour
   `RENDER_API_KEY` n'est pas concerné ; le PAT n'est PAS stocké côté
   GitHub (usage one-shot par l'opérateur).
3. Révoquer l'ancien token (Revoke) — vérifier que `git push` fonctionne
   toujours avec le nouveau.
4. Vérifier : `git ls-remote origin` + un push de test (branche jetable).

### 2.2 Neon (DSN + token API)
1. Neon Console → rôle du branchement `production` → **Reset password** —
   copier le nouveau DSN.
2. Render → Environment → remplacer `DATABASE_URL` → le service redémarre.
3. GitHub → Secrets → `DATABASE_URL` (utilisé par le workflow de sauvegarde)
   → remplacer.
4. Neon Console → API Keys → révoquer l'ancienne clé `napi_…`, en créer une
   nouvelle, la ranger au coffre.
5. Vérifier : `/api/health` (200), une connexion applicative, un
   `mikbackup export` de contrôle (cf. §3).

### 2.3 Render API key
1. Render → Account Settings → API Keys → **Roll** (l'ancienne meurt).
2. GitHub → Secrets → `RENDER_API_KEY` → remplacer.
3. Vérifier : un push vide sur `backend/` déclenche deploy-render (CI verte).

### 2.4 Vercel token
1. Vercel → Settings → Tokens → créer, révoquer l'ancien.
2. Mettre à jour l'usage CI/CLI le cas échéant.

### 2.5 JWT_SECRET / SECRETBOX_KEY (rotation *délicate*)
1. `JWT_SECRET` : la rotation **révoque toutes les sessions** (tokens
   re-signés impossible à valider) — à faire hors heures de pointe :
   Render → Environment → nouvelle valeur → redeploy. Les utilisateurs se
   reconnectent (les mots de passe ne changent pas).
2. `SECRETBOX_KEY` : protège les secrets chiffrés AT REST (tokens agents,
   credentials routeur stockés). La rotation exige un re-chiffrement des
   données : procédure support — contacter le développeur ; NE PAS la
   faire à chaud sans plan de re-chiffrement.

### 2.6 Incident « secret exposé »
1. Révoquer immédiatement le secret exposé (pas de délai d'attente).
2. En émettre un neuf (procédures ci-dessus).
3. Chercher les traces d'usage abusif (logs Render, audit Neon, GitHub
   Security → Secret scanning alerts).
4. Journaliser l'incident (date, périmètre, actions) — exigence loi 2013-450
   (article 15 : notification des violations) et RGPD art. 33/34.

## 3. Sauvegardes Neon chiffrées (mikbackup) — testées chaque semaine

Outil : `backend/cmd/mikbackup` (export chiffré AES-256-GCM + **test de
restauration intégré**). La CI exécute le workflow `backup.yml` chaque
**dimanche 03:17 UTC** :

1. `mikbackup export` → toutes les tables du schéma public (row_to_json —
   typage Postgres préservé) → chiffrement `BACKUP_KEY` (AES-256-GCM).
2. Artefact GitHub `mikcloud-backup-<date>` (rétention 90 jours, fichier
   chiffré — inutilisable sans la clé).
3. **Vérification périodique de restauration** (l'opérateur, une fois par
   mois au minimum) :
   ```bash
   # Télécharger l'artefact le plus récent (GitHub → Actions → backup → run)
   mikbackup restore-check -dsn "$DATABASE_URL" -file backup.enc -key "$BACKUP_KEY"
   ```
   `restore-check` recrée chaque table en miroir `s4check_*`, y réinsère
   toutes les lignes (re-typage Postgres natif via json_populate_record),
   compare les comptages puis supprime les miroirs. Verdict attendu :
   `RESTAURATION TESTÉE : OK`.
4. Restauration complète réelle (perte de données) :
   ```bash
   # 1. Export frais si la base reste lisible, sinon dernier artefact sain.
   # 2. Vider le schéma public (hors sessions) puis réinsérer :
   #    chaque table : TRUNCATE … CASCADE puis réinsertion des lignes du
   #    fichier (même mécanique que restore-check, sur les tables réelles).
   # 3. Redémarrer le service Render (Settings → Manual deploy → Clear cache).
   ```
   Alternativement, Neon propose une restauration point-in-time selon le
   plan (Console → Branch → Restore) — la consulter en premier recours.

**Première mise en service** (à faire par l'opérateur) :
- GitHub → Secrets → créer `BACKUP_KEY` : `openssl rand -hex 32` (à ranger
  au coffre — sans elle, aucune sauvegarde n'est restaurable).
- GitHub → Secrets → créer `DATABASE_URL` (DSN Neon de production).
- Le workflow passe en « skipped » tant que ces secrets manquent.

## 4. 2FA TOTP — procédure de secours

- Un utilisateur qui **perd son téléphone** ne peut plus se connecter
  (aucun code de secours en v1 — choix assumé, cf. totp.go).
- Procédure : l'opérateur plateforme efface le secret côté base —
  ```sql
  UPDATE admin_users SET totp_secret = '', totp_enabled = false
   WHERE username = '<login>' AND account_id = '<compte-vérifié>';
  ```
  — après **vérification d'identité du demandeur** (canal WhatsApp connu du
  compte). **Puis redéployer le service** (Render → Manual Deploy, ou
  `POST /v1/services/<id>/deploys` — HTTP 202, réponse vide) : le backend
  garde l'état en mémoire et ne re-lit la base qu'au démarrage — sans
  redémarrage, la ligne mémoorisée (2FA active) resynchroniserait la base.
  Testé en conditions réelles (vague S4). Puis l'utilisateur re-paire sa
  2FA depuis Paramètres → Sécurité.
- La 2FA n'est pas écrasée par une réinitialisation de mot de passe.

## 5. Cloudflare devant l'API (recommandé avant lancement)

Objectif : masquer l'IP d'origine Render, absorber L7/L3, WAF sur `/api/*`.

1. Zone DNS : `ftci.fr` (ou domaine produit) sur Cloudflare (plan gratuit
   suffit pour proxy + WAF basique).
2. Sous-domaine API : `api.<domaine>` → CNAME `mikcloud.onrender.com`,
   **proxy orange** (Cloudflare). ⚠️ TLS/SSL mode : **Full (strict)**.
3. Render → Settings → Custom Domain : ajouter `api.<domaine>` (TLS géré).
4. WAF (Security → WAF) :
   - Règle : `http.request.uri.path contains "/api/"` et pays hors zone
     d'activité → Managed Challenge (à calibrer avec les données réelles) ;
   - Rate limiting Cloudflare en amont (ex. 100 req/min/IP) — double barrière
     avec les limiteurs applicatifs S1 ;
   - Bot Fight Mode activé.
5. Origin hardening (option avancée) : RESTRICT l'ingress Render aux IP de
   Cloudflare ou mTLS Render↔Cloudflare.
6. Frontend Vercel : idem sur le domaine vitrine (le frontend reste
   `mikcloud.ftci.fr`, proxifié orange).
7. **Rollout progressif** : d'abord DNS-only (gris) pour vérifier le TLS,
   puis proxy orange, puis règles WAF une par une.

## 6. Contacts & urgence

- Statut des services : Render Status, Vercel Status, Neon Status.
- Incident base de données : procédure §3 ; incident secret : §2.6 ;
  verrouillage utilisateur : §4.
- Les sondes de santé (`GET /` → 200, `X-Content-Type-Options`…) sont
  détaillées dans `docs/CONTRACT-V2.md` §0 (Sécurité S1–S4).
