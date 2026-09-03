# Registre des traitements & conformité données personnelles

> MikCloud — SaaS de gestion de hotspots MikroTik.
> Cadre juridique : **loi ivoirienne n° 2013-450 du 19 juin 2013** relative à
> la protection des données à caractère personnel (+ autorité ARTCI/CDP) et,
> pour les utilisateurs hors Côte d'Ivoire, **RGPD** (règlement UE 2016/679)
> comme référence de bonnes pratiques. Vague S4 — 2026-09-03.
>
> Ce registre est un document d'appui : l'opérateur (responsable du
> traitement) en assume la mise à jour et la déclaration éventuelle auprès
> de la CDP.

## 1. Traitements mis en œuvre

| # | Traitement | Finalité | Données | Base légale (réf. loi) | Durée |
|---|---|---|---|---|---|
| T1 | Gestion de compte SaaS | Inscription, authentification, 2FA | Identité (nom, identifiant), email, WhatsApp, pays/ville, mot de passe (bcrypt), secret TOTP, sessions | Contrat (art. 9 — consentement/contrat) | Durée du compte + 12 mois |
| T2 | Gestion hotspot | Vouchers, utilisateurs hotspot, sessions, trafic | Identifiants hotspot, MAC/IP, volumes, horodatages | Contrat | Durée du compte |
| T3 | Journal d'activité & sécurité | Traçabilité, détection force brute (JSON `auth_failure` S2) | Identifiants de connexion, IP (premier hop XFF), user-agent, horodatage | Intérêt légitime (sécurité) | 12 mois max |
| T4 | Facturation / abonnement | Essai 90 j, plans, paiements Wave/GeniusPay | Transactions, statut d'abonnement (aucun PAN stocké — passerelles externes) | Contrat + obligation légale | 5 ans (comptable) |
| T5 | Notifications | Alertes opérationnelles (email/WhatsApp/Telegram) | Coordonnées du gérant | Contrat | Durée du compte |

## 2. Sous-traitants / destinataires

| Prestataire | Rôle | Localisation | Garanties |
|---|---|---|---|
| **Neon** (PostgreSQL managé) | Hébergement base de données | UE (eu-central-1) | Chiffrement TLS, clés managées, contrat de sous-traitance |
| **Render** | Exécution du backend | UE/US (région choisie au service) | SOC 2, TLS |
| **Vercel** | Frontend statique | Edge mondial | SOC 2 |
| Passerelles de paiement (Wave, GeniusPay) | Encaissement | — | Aucune donnée carte ne transite par MikCloud |

Aucune revente ni partage publicitaire des données : **pas de profilage**,
pas de cookies publicitaires (stockage local limité à la session de travail :
token, langue, préférences UI — localStorage `mikcloud-auth`).

## 3. Droits des personnes (procédures S4)

| Droit | Mise en œuvre MikCloud |
|---|---|
| **Information** | Présente page + mentions à l'inscription (case politique de confidentialité). |
| **Accès / portabilité** | Exports CSV intégrés par module (vouchers, ventes, utilisateurs hotspot) ; export complet du compte sur demande support (via `mikbackup export` filtré au compte, restitué chiffré). |
| **Rectification** | Paramètres → Général (organisation) ; Identifiant/equipe par le gérant ; support pour l'email. |
| **Suppression** | Fermeture du compte sur demande support : anonymisation de `admin_users` (nom/email remplacés), détachement des données techniques, suppression des coordonnées de `accounts`. Les enregistrements comptables (transactions) sont conservés selon la durée légale (5 ans) en forme anonymisée. Délai : 30 jours maximum. |
| **Opposition/limitation** | Désactivation des notifications (Paramètres → Notifications). |
| **Réclamation** | CDP Côte d'Ivoire (ARTCI) — procédure communiquée sur demande. |

## 4. Sécurité (mesures techniques — renvoi audit S1–S4)

- Mots de passe : bcrypt (coût 12), politique 10 car./denylist (S2) ;
- Sessions : JWT 24 h, révocation immédiate par époque (S1-A3), 2FA TOTP
  disponible (S4) ;
- Transport : TLS partout (HSTS), CORS strict (fail-closed) ;
- Base : PostgreSQL managé Neon, sauvegardes chiffrées AES-256-GCM avec
  **test de restauration hebdomadaire automatisé** (S4) ;
- Journalisation sécurité côté serveur (échecs d'authentification, IP au
  premier hop, raisons fines — S2), rate limiting par IP + verrou PIN
  revendeur + quota d'inscription (S1/S2/S3) ;
- Chaîne : govulncheck en CI, Dependabot, secret scanning/push protection (S3).

## 5. Violations de données

Procédure : détection (journaux S2, alertes plateformes) → qualification
sous 48 h → **notification ARTCI/CDP sous 72 h** si risque → information des
personnes si risque élevé → journal de l'incident (cf.
`docs/RUNBOOK-SECRETS.md` §2.6).

## 6. À faire avant lancement (opérateur)

1. Publier la présente politique sur une page publique du frontend
   (`/legal/confidentialite`) et la lier depuis l'inscription (case à cocher).
2. Nommer le responsable du traitement (l'opérateur MikCloud) et l'adresse
   de contact privacy (ex. `privacy@mikcloud.ftci.fr`).
3. Effectuer la déclaration/traitement à la CDP si le périmètre l'exige
   (2013-450 : régime de déclaration).
4. Annexer les DPA (contrats de sous-traitance) Neon/Render/Vercel.
