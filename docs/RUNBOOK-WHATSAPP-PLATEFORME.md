# RUNBOOK — WhatsApp plateforme (N°148-c) : démarches Meta pas à pas

> Document opérateur MikCloud. Objectif : ouvrir le TROISIÈME canal plateforme
> (après le relais e-mail et le bot Telegram du N°150) — les alertes d'un
> client arrivent sur le WhatsApp du gérant sans qu'il crée RIEN chez Meta :
> l'envoi est porté par le WABA du compte principal, exactement comme le
> relais e-mail est porté par ses identifiants Resend/SMTP. Ce runbook couvre
> VOS démarches côté Meta (entreprise, numéro, jeton, templates) ; l'extension
> du backend MikCloud qui consommera ces identifiants suit en N°148-c une fois
> les démarches terminées. Dernière mise à jour : N°154 (2026-09-18).

## 0. Ce que vous allez obtenir (et ce qu'il faut avant)

| Élément | Valeur attendue |
|---|---|
| Business Manager vérifié | « Freelance Technologies CI » (ou le nom juridique exact de vos documents) |
| Application Meta | type Business, produit « WhatsApp » actif |
| WABA de production | WhatsApp Business Account portant le numéro d'envoi |
| Numéro d'envoi dédié | un numéro qui n'est actif sur AUCUN autre WhatsApp |
| Jeton d'accès permanent | System User (ne s'expire pas) — secret de niveau coffre |
| Templates utility approuvés | 4 obligatoires + 1 optionnel (§8) |

**Prérequis** : un compte Facebook personnel (celui de l'opérateur), les
documents de l'entreprise (registre de commerce / extrait — le nom doit
correspondre EXACTEMENT au nom saisi chez Meta), un numéro de téléphone à
DÉDIER (SIM ou fixe : il sera détaché de toute utilisation WhatsApp
personnelle), une carte bancaire pour le paiement des envois templates.

**Budget temps** : ~1 à 3 h de démarches actives + délais de revue Meta
(entreprise : quelques minutes à quelques jours ; templates : quelques
minutes à 48 h).

⚠️ Les chemins d'interface cités sont ceux de la console Meta au moment de la
rédaction — Meta remanie régulièrement ses menus : en cas d'écart, chercher
l'équivalent dans « Paramètres de l'entreprise » / « WhatsApp Manager ».

## 1. Créer le Business Manager

1. Ouvrir https://business.facebook.com → « Créer un compte » (se connecter
   avec le compte Facebook de l'opérateur).
2. Nom de l'entreprise : **Freelance Technologies CI** (nom juridique exact),
   e-mail professionnel, pays : Côte d'Ivoire.
3. Une fois créé : Paramètres de l'entreprise → Renseigner l'adresse, le site
   (https://mikcloud.ftci.fr), le numéro — ces champs alimentent la
   vérification du §5.

## 2. Créer l'application Meta (type Business)

1. Ouvrir https://developers.facebook.com → « Mes applications » → « Créer une
   application ».
2. Cas d'usage : « Autre » → type **Business** ; nom : `mikcloud-alertes` ;
   e-mail de contact professionnel ; créer.
3. Dans le tableau de bord de l'app : ajouter le produit **WhatsApp** (« API
   WhatsApp » → Configurer).
4. Meta crée alors automatiquement un WABA de TEST avec un numéro de test et
   un jeton temporaire — **suffisant pour développer, PAS pour la
   production**. Notez le « Phone number ID » et le « WhatsApp Business
   Account ID » affichés sur la page API Setup : ils servent de repère.

## 3. Créer le WABA de production et y rattacher le numéro

1. Depuis l'app (produit WhatsApp → API Setup) ou depuis Business Manager →
   Comptes → Comptes WhatsApp : **créer un compte WhatsApp Business** (et non
   réutiliser le WABA de test).
2. Nom de profil : **MikCloud Alertes** — c'est le nom que verront les
   destinataires (les gérants clients qui activent le canal).
3. Rattacher le WABA à l'application `mikcloud-alertes` (Business Manager →
   Paramètres → Applications → « Ajouter » → lier au WABA).

## 4. Ajouter le numéro d'envoi

1. WhatsApp Manager → « Vue d'ensemble » → **Numéros de téléphone** →
   « Ajouter un numéro ».
2. Saisir le numéro dédié (code pays + numéro, ex. +225 …) ; choisir la
   vérification par **SMS ou appel** ; noter le code à 6 chiffres (ou le
   suivre à l'écran en cas d'appel vocal).
3. ⚠️ Le numéro ne doit être actif sur AUCUN autre compte WhatsApp : si c'est
   un numéro que vous utilisiez dans l'app WhatsApp personnelle, désinstallez
   l'app / supprimez le compte WhatsApp lié AVANT la vérification — sinon
   Meta la refuse.
4. Une fois affiché « Connecté » avec un Phone number ID (ex.
   `123456789012345`), le relever : c'est l'identifiant d'envoi.

## 5. Vérifier l'entreprise (obligatoire pour produire)

1. Business Manager → Paramètres → **Centre de sécurité** → « Vérification de
   l'entreprise » → Démarrer la vérification.
2. Sélectionner l'entreprise, justifier : « Je fournis un service à d'autres
   entreprises » (SaaS B2B).
3. Document exigé (registre de commerce / certificat d'immatriculation /
   facture d'utilités au nom de l'entreprise) : le nom doit correspondre
   CARACTÈRE PAR CARACTÈRE au nom du Business Manager.
4. Statut « Vérifié » : quelques minutes à quelques jours (Meta envoie
   parfois un e-mail/code de confirmation).
5. Ajouter ensuite un **moyen de paiement** (facturation Meta → ajouter une
   carte) : sans paiement actif, le numéro reste en mode test et les envois
   de production sont bloqués.

Sans vérification : envois limités aux numéros de test et plafond de
destinataires bas. Avec vérification : plafond initial de l'ordre de 250
destinataires uniques / 24 h, qui s'élargit automatiquement selon la qualité
et le volume (1 000 → 10 000 → 100 000).

## 6. Créer le jeton d'accès permanent (System User)

1. Business Manager → Paramètres → Utilisateurs → **Utilisateurs système** →
   « Ajouter » : nom `mikcloud-backend`, rôle « Employé de l'entreprise »
   (ou Admin si l'interface l'exige).
2. Sélectionner l'utilisateur système → « Attribuer des éléments » →
   l'application `mikcloud-alertes` → gestion complète.
3. « **Générer un jeton** » : cocher les permissions
   `whatsapp_business_messaging`, `whatsapp_business_management`,
   `business_management` (+ `show_in_console` si proposé).
4. Copier le jeton `EAAG…` et le ranger AU COFFRE (1Password/Bitwarden) :
   niveau de sensibilité identique à la clé Resend (cf. RUNBOOK-SECRETS §0).
   Un jeton d'utilisateur système **ne s'expire pas** — aucune rotation
   planifiée, à régénérer uniquement sur incident.

## 7. Relever les identifiants (la « fiche finale »)

| Identifiant | Où le trouver | Exemple |
|---|---|---|
| Access token permanent | §6 (coffre) | `EAAG…` |
| Phone Number ID | WhatsApp Manager → API Setup | `123456789012345` |
| WABA ID | Paramètres du compte WhatsApp Business | `987654321098765` |
| Numéro d'envoi | §4 | `2250701020304` |

Test immédiat (optionnel, depuis l'explorateur Graph API
https://developers.facebook.com/tools/explorer avec le jeton du §6) :
POST `/{phone-number-id}/messages` corps
`{"messaging_product":"whatsapp","to":"<votre numéro>","type":"template","template":{"name":"hello_world","language":{"code":"en_US"}}}`
→ votre téléphone reçoit le message de bienvenue : la chaîne d'envoi est
ouverte.

## 8. Soumettre les templates « utility » (l'anti-fenêtre 24 h)

Règle WhatsApp : hors d'une conversation ouverte par le destinataire depuis
moins de 24 h, SEULS les messages partant d'un **template approuvé** partent.
Chaque type d'alerte MikCloud doit donc avoir son template — les corps
ci-dessous sont des propositions (variables `{{n}}`, pas de mise en forme) :

| # | Nom du template | Catégorie | Langue | Corps proposé | Kind MikCloud couvert |
|---|---|---|---|---|---|
| 1 | `mikcloud_routeur` | UTILITY | fr | `{{1}} : le routeur « {{2}} » est {{3}}.` | router_offline / router_back ({{3}} = « hors ligne » / « de retour en ligne ») |
| 2 | `mikcloud_stock_bas` | UTILITY | fr | `{{1}} : stock de tickets bas — {{2}} vouchers actifs restants.` | low_stock |
| 3 | `mikcloud_rapport_quotidien` | UTILITY | fr | `{{1}} — rapport du {{2}} : {{3}}` | daily_report ({{3}} = le corps condensé) |
| 4 | `mikcloud_test` | UTILITY | fr | `Test MikCloud : le canal WhatsApp est opérationnel ✔` | test |
| 5 (optionnel) | `mikcloud_pool` | UTILITY | fr | `{{1}} : pool de licences {{2}} % ({{3}}/{{4}}).` | pool_alert / pool_auto — à soumettre si vous utilisez le pool de licences |

Démarche, pour chaque template :

1. WhatsApp Manager → **Modèles de messages** → « Créer un modèle ».
2. Nom exact (minuscules + soulignés, sans espace), catégorie **UTILITY**,
   langue **français (fr)**.
3. Saisir le corps avec les variables `{{1}}`, `{{2}}`… (boutons inutiles
   pour des alertes).
4. Soumettre → revue Meta : de quelques minutes à 48 h ; statut
   « **Approuvé** » requis avant l'activation côté MikCloud.
5. En cas de rejet : le motif est affiché — les causes classiques sont une
   catégorie erronée (marketing ≠ utility), un contenu promotionnel, ou des
   variables mal appariées. Corriger et resoumettre.

## 9. Activer côté MikCloud (une fois tout « Approuvé »/« Vérifié »)

1. Render (service `mikcloud`, `srv-da974o142hec73euul60`) → Environment →
   ajouter les variables (même discipline que
   `TELEGRAM_PLATFORM_BOT_TOKEN` au N°150) :
   - `WHATSAPP_PLATFORM_TOKEN` = jeton permanent du §6
   - `WHATSAPP_PLATFORM_PHONE_ID` = Phone Number ID du §7
   - `WHATSAPP_PLATFORM_WABA_ID` = WABA ID du §7
2. Save → redéploiement automatique.
3. Prévenir l'opérateur de développement : l'implémentation N°148-c du
   backend (relais WhatsApp — les clients activent le canal en ne donnant que
   leur numéro, l'envoi passe par le WABA du compte principal avec les
   templates du §8, repli e-mail si template rejeté) peut alors être livrée.
   Elle fera apparaître la carte « WhatsApp plateforme » dans la console
   plateforme et une carte simplifiée dans les consoles clients.

## 10. Coûts et limites (indicatif — la grille Meta fait foi)

- Depuis juillet 2025, la facturation est **par template envoyé** : les
  utility se facturent typiquement quelques centimes d'USD par message selon
  le marché (grille : Meta Business Help Center → Pricing). Les réponses
  libres dans la fenêtre 24 h (conversations de service) restent gratuites.
- Ordre de grandeur MikCloud : ~4-6 alertes/mois par client actif → quelques
  centimes par client et par mois — à intégrer au prix de l'abonnement.
- Les messages rejetés (numéro invalide, hors opt-in) ne sont pas facturés.

## 11. Problèmes fréquents

| Symptôme | Cause probable | Remède |
|---|---|---|
| « Recipient phone number not in allowed list » | toujours en mode développement | terminer la vérification entreprise (§5) + paiement actif |
| Template « En attente » > 48 h | revue manuelle | vérifier la catégorie UTILITY et le contenu strictement transactionnel ; resoumettre |
| Numéro refusé à la vérification | actif sur un autre WhatsApp | désinstaller l'app WhatsApp liée au numéro, attendre, recommencer (§4.3) |
| Token invalide après quelques semaines | jeton temporaire utilisé (celui de l'app) | regénérer un jeton SYSTEM USER (§6), pas celui du tableau de bord |
| Envoi 131047 / « Re-engagement message » | template inexistant pour ce kind | soumettre le template manquant du §8 |
