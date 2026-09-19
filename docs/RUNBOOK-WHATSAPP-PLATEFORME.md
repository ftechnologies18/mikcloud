# RUNBOOK — WhatsApp plateforme (N°148-c) : démarches Meta pas à pas

> Document opérateur MikCloud. Objectif : ouvrir le TROISIÈME canal plateforme
> (après le relais e-mail et le bot Telegram du N°150) — les alertes d'un
> client arrivent sur le WhatsApp du gérant sans qu'il crée RIEN chez Meta :
> l'envoi est porté par le WABA du compte principal, exactement comme le
> relais e-mail est porté par ses identifiants Resend/SMTP. Ce runbook couvre
> VOS démarches côté Meta (entreprise, numéro, jeton, templates) ; l'extension
> du backend MikCloud qui consommera ces identifiants suit en N°148-c une fois
> les démarches terminées. Dernière mise à jour : N°156 (2026-09-19) — flux
> Meta re-vérifié sur la documentation officielle (mise à jour sept. 2026) :
> création d'app par CAS D'USAGE (fin du « Autre → type Business »), tableau de
> bord « Quickstart → Start using the API », nouveau modèle de compte WhatsApp
> (Coexistence) à la vérification du numéro, et option **Direct Send** (GA
> utility depuis juillet 2026) qui peut dispenser de la soumission manuelle
> des templates §8.

## 0. Ce que vous allez obtenir (et ce qu'il faut avant)

| Élément | Valeur attendue |
|---|---|
| Business Manager vérifié | « Freelance Technologies CI » (ou le nom juridique exact de vos documents) — vérifié UNE fois pour TOUS vos produits |
| Application Meta (parapluie) | UNE seule app pour tous vos produits (§12) — cas d'usage « WhatsApp » actif |
| WABA de production | UN WhatsApp Business Account PAR PRODUIT — celui de MikCloud porte le numéro d'envoi et le nom « MikCloud Alertes » |
| Numéro d'envoi dédié | un numéro qui n'est actif sur AUCUN autre WhatsApp (un PAR produit qui envoie) |
| Jeton d'accès permanent | System User (ne s'expire pas) — UN SEUL jeton partagé entre produits, secret de niveau coffre |
| Templates utility approuvés | 4 obligatoires + 1 optionnel (§8) — propres au WABA MikCloud |

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

## 1. Créer le Business Portfolio (ex-Business Manager)

1. Ouvrir https://business.facebook.com → « Créer un compte » (se connecter
   avec le compte Facebook de l'opérateur). Meta parle désormais de
   **Business Portfolio** — même chose, nouvelle dénomination.
2. Nom de l'entreprise : **Freelance Technologies CI** (nom juridique exact),
   e-mail professionnel, pays : Côte d'Ivoire.
3. Une fois créé : Paramètres de l'entreprise → Renseigner l'adresse, le site
   (https://mikcloud.ftci.fr), le numéro — ces champs alimentent la
   vérification du §5.

⚠️ Si vous n'avez pas encore de Business Portfolio, pas d'inquiétude : le
flux de création d'app du §2 propose d'en créer un EN COURS DE ROUTE.

## 2. Créer l'application Meta (par cas d'usage — flux 2026)

> CHANGEMENT 2026 : l'écran « Autre → type Business » a disparu. La création
> d'app est désormais pilotée par CAS D'USAGE ; pour WhatsApp c'est le chemin
> direct « Connect with customers through WhatsApp ».

1. Ouvrir https://developers.facebook.com → « Mes applications » → « Créer une
   application ».
2. Nom : `ftci-apps` (app PARAPLUIE — voir §12 : elle servira MikCloud ET vos
   autres applications ; ce nom n'apparaît JAMAIS aux destinataires, seul le
   nom du WABA — « MikCloud Alertes » — est visible) ; e-mail de contact
   professionnel ; cas d'usage : **« Connect with customers through
   WhatsApp »** → Suivant.
3. **Sélectionner un Business Portfolio existant ou en créer un nouveau**
   (si vous en créez un ici, Meta peut créer AUTOMATIQUEMENT un WABA — le
   vérifier au §3 avant d'en créer un second).
4. Une liste d'exigences de publication peut s'afficher (aucune à ce stade)
   → Suivant ; confirmez → **Créer l'app**.
5. Vous arrivez sur le tableau de bord « Customize use case → Connect on
   WhatsApp → **Quickstart** ». Cliquer **« Start using the API »** : c'est
   la nouvelle porte d'entrée vers la page **API Setup**.
6. La page API Setup affiche le jeton TEMPORAIRE (24 h — développement
   uniquement, le jeton permanent suit au §6) et les identifiants de repère
   (Phone number ID, WABA ID) — les relever.

## 3. Créer le WABA de production et y rattacher le numéro

> Rappel architecture (§12) : le WABA est PAR PRODUIT. Celui créé ici est
> celui de MikCloud ; un futur produit aura le sien, dans le MÊME portfolio
> et la MÊME app — sans nouvelle démarche d'entreprise.

1. Depuis l'app (produit WhatsApp → API Setup) ou depuis Business Manager →
   Comptes → Comptes WhatsApp : **créer un compte WhatsApp Business** (et non
   réutiliser le WABA de test).
2. Nom de profil : **MikCloud Alertes** — c'est le nom que verront les
   destinataires (les gérants clients qui activent le canal).
3. Rattacher le WABA à l'application `ftci-apps` (Business Manager →
   Paramètres → Applications → « Ajouter » → lier au WABA).

## 4. Ajouter le numéro d'envoi

1. WhatsApp Manager → « Vue d'ensemble » → **Numéros de téléphone** →
   « Ajouter un numéro » (ou depuis la page API Setup de l'app, champ
   « From phone number » → ajouter).
2. Saisir le numéro dédié (code pays + numéro, ex. +225 …) ; choisir la
   vérification par **SMS ou appel** ; noter le code à 6 chiffres (ou le
   suivre à l'écran en cas d'appel vocal).
3. ⚠️ Le numéro ne doit être actif sur AUCUN autre compte WhatsApp : si c'est
   un numéro que vous utilisiez dans l'app WhatsApp personnelle, désinstallez
   l'app / supprimez le compte WhatsApp lié AVANT la vérification — sinon
   Meta propose désormais le flux **Coexistence**.
4. **CHANGEMENT sept. 2026 — nouveau modèle de compte / Coexistence** : si le
   numéro est (ou a été) actif sur l'app WhatsApp Business, Meta ne refuse
   plus systématiquement — l'onboarding entre AUTOMATIQUEMENT dans le flux
   « Coexistence » : le compte WhatsApp existant est converti en compte
   « Messaging » rétrocompatible (l'ID `waba_id` est conservé, le partage
   fonctionne). Pour un numéro DÉDIÉ jamais utilisé sur WhatsApp, vous ne
   verrez pas cet écran — vérification SMS/appel classique.
5. Une fois affiché « Connecté » avec un Phone number ID (ex.
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

### 8.0 AVANT TOUT — vérifier l'éligibilité Direct Send (nouveau, GA 07/2026)

> CHANGEMENT JUILLET 2026 : **Direct Send** est désormais GA pour les
> messages UTILITY. Principe : on envoie le message SANS template avec un
> champ `category: "utility"`, et Meta génère/matche automatiquement les
> templates en arrière-plan (contenu PII-redaté, langue détectée). S'il est
> éligible, votre compte peut DISPENSER de la soumission manuelle ci-dessous.

1. WhatsApp Manager → chercher le **bandeau Direct Send** : il indique si le
   compte est éligible. (Sinon, exprimer votre intérêt via le lien du bandeau.)
2. Test d'éligibilité direct : envoyer un message utility avec le champ
   `category` — si le compte n'y a pas droit, l'API répond `(#100) Invalid
   parameter … requires Direct Send, which isn't enabled for this account.
   Use an approved message template instead.` → revenir à la voie classique
   ci-dessous.
3. Si éligible : les alertes MikCloud partent en `type:"text"` +
   `category:"utility"` — mêmes limites que les templates (corps 1 024 car.,
   formats text/interactifs, en-têtes image/vidéo/document en accès
   restreint). Discipline inchangée : contenu strictement transactionnel —
   Meta surveille l'usage utility comme marketing (e-mails d'avertissement,
   templates auto-pausés ; revue possible via wadirectsendapisupport@meta.com).
4. ⚠️ Direct Send reste une solution « premium » : mêmes tarifs par message
   que les templates utility (§10) et éligibilité au cas par cas — si le
   bandeau n'apparaît pas, NE PAS compter dessus pour le go-live : soumettre
   les 4 templates du tableau ci-dessous (ils restent requis par le runbook
   MikCloud et fonctionnent dans tous les cas).

### 8.1 Voie classique — soumission manuelle

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
   - `WHATSAPP_PLATFORM_TOKEN` = jeton permanent du §6 — le JETON est partagé
     entre produits (§12) ; c'est l'identifiant de chaque produit qui change
   - `WHATSAPP_PLATFORM_PHONE_ID` = Phone Number ID du §7 (celui du WABA
     MikCloud)
   - `WHATSAPP_PLATFORM_WABA_ID` = WABA ID du §7 (celui de MikCloud)
   Un futur produit aura ses propres PHONE_ID/WABA_ID (variables de SON
   backend), avec le MÊME jeton.
2. Save → redéploiement automatique.
3. Prévenir l'opérateur de développement : l'implémentation N°148-c du
   backend (relais WhatsApp — les clients activent le canal en ne donnant que
   leur numéro, l'envoi passe par le WABA du compte principal avec les
   templates du §8, repli e-mail si template rejeté) peut alors être livrée.
   Elle fera apparaître la carte « WhatsApp plateforme » dans la console
   plateforme et une carte simplifiée dans les consoles clients.

## 10. Coûts et limites — taux vérifiés pour la Côte d'Ivoire (région « Rest of Africa », USD, grille effective juil. 2026)

Taux par message **livré** (lus sur la grille officielle interactive
business.whatsapp.com/products/platform-pricing, marché « Rest of Africa » —
la Côte d'Ivoire +225 en fait partie) :

| Catégorie | Taux CIF (Rest of Africa) | ≈ FCFA* |
|---|---|---|
| **Utility** (toutes les alertes MikCloud) | **0,0040 $/message** | ~2,5 |
| Authentication (non utilisé par MikCloud) | 0,0040 $/message | ~2,5 |
| Marketing (interdit par la discipline MikCloud) | 0,0225 $/message | ~13,5 |
| Service (réponses dans la fenêtre 24 h) | **gratuit** | 0 |

\* conversion indicative 1 USD ≈ 600 XOF.

Paliers de volume utility/authentication (remises automatiques) :
0-100 k msg/mois : 0,0040 $ · 100 k-1 M : 0,0038 $ (-5 %) · 1-4,5 M : 0,0036 $
(-10 %) · 4,5-40 M : 0,0034 $ (-15 %) · 40-80 M : 0,0032 $ (-20 %) · >80 M :
0,0030 $ (-25 %) — sans objet pour les volumes MikCloud, cités pour mémoire.

Règles de facturation (détail doc pricing Meta, effective juil. 2025) :
- On ne paie que le message **livré** (pas « envoyé ») ; les non-livrés
  (numéro invalide, hors opt-in) ne sont pas facturés.
- **Utility GRATUITS dans une fenêtre de service ouverte** : si le gérant a
  répondu au canal depuis moins de 24 h, les templates utility envoyés dans
  cette fenêtre ne sont PAS facturés (statut webhook
  `type:"free_customer_service"`).
- Fenêtre « Free Entry Point » 72 h (ads Click-to-WhatsApp) : hors sujet
  MikCloud, citée pour mémoire.
- Mise à jour tarifaire connue au 1er octobre 2026 : marchés standalone
  (Bangladesh, Irak, Maroc, etc.) — **aucun impact pour « Rest of
  Africa »** ; les changements n'interviennent plus qu'aux 1er
  janv./avr./juil./oct.
- **Budget MikCloud** : ~4-6 alertes/mois par client actif, toutes utility →
  **~0,02 $/mois par client (~12 FCFA)**, soit ~0,25 $/an. Ex. 50 clients
  actifs : ~1 $/mois. La carte bancaire (§5.5) reste le seul engagement
  réel — pas d'abonnement, pas de minimum : on ne paie que ce qui part.

### 10.1 Comparatif avec l'API SMS d'Orange Côte d'Ivoire (relevé sept. 2026)

Grille officielle developer.orange.com → APIs → SMS Cote d'Ivoire 2.0 →
Pricing (paiement Airtime ou Orange Money, USSD #144*621#) :

| Bundle Orange | SMS | Prix (FCFA) | Validité | Prix/SMS |
|---|---|---|---|---|
| Bundle 0* (1 seul achat) | 20 | 145 | 7 j | 7,25 F |
| **Bundle 1** | 100 | **725** | **30 j** | **7,25 F** |
| Bundle 2 | 1 000 | 7 260 | 45 j | 7,26 F |
| Bundle 3 | 10 000 | 72 600 | 60 j | 7,26 F |

Contraintes Orange : achat plafonné à 100 000 FCFA/jour/SIM ; 5
transactions/s ; SMS non consommés PERDUS à expiration (sauf rachat qui
fusionne et relance la validité) ; sender name personnalisable gratuit
(approbation équipe locale) ; livraison en CI tous opérateurs.

**Verdict par le calcul (alertes MikCloud ~5/mois/client, toutes
utility/texte court)** :

| Profil | Orange (bundles) | WhatsApp (à l'usage) | Écart |
|---|---|---|---|
| Prix unitaire | 7,25 F/SMS | ~2,4 F/msg | **3x** |
| 10 clients (~50 msg/mois) | Bundle 1 : 725 F/mois (50 SMS perdus) | ~120 F/mois | **6x** |
| 50 clients (~250 msg/mois) | Bundle 2 : ~7 260 F/45 j ≈ 4 900 F/mois | ~600 F/mois | **8x** |
| 100 clients (~500 msg/mois) | Bundle 2 : ~7 260 F/30 j | ~1 200 F/mois | **6x** |

Pourquoi l'écart dépasse le simple rapport 3x : (1) les bundles EXPIRENT —
à petit volume on paie des SMS jamais envoyés (expiration 30-60 j,
rachat obligé pour garder le solde) ; (2) WhatsApp ne facture que le livré
à l'unité, sans minimum ; (3) les utility WhatsApp sont gratuits en fenêtre
de service ouverte (gérant ayant répondu < 24 h) ; (4) un SMS long
(> 160 car.) = plusieurs SMS facturés, un message WhatsApp = 1 024 car.

Ce que le SMS garde pour lui : universalité (téléphone basique sans
internet), inscription légère (pas de vérification Business Meta, pas de
carte bancaire — paiement Orange Money local), démarrage en 10 minutes.
Mais la cible MikCloud = gérants de hotspots/WISP, connectés par
définition et déjà sur WhatsApp — l'argument d'universalité ne pèse pas.

**Conclusion tenue dans le runbook : le choix WhatsApp (N°148-c) est
confirmé par les chiffres — 6 à 8x moins cher sur les profils MikCloud
réalistes ; l'API SMS Orange reste une piste de canal de repli si un jour
des clients sans WhatsApp apparaissent (développement backend séparé,
hors périmètre actuel).**

## 11. Problèmes fréquents

| Symptôme | Cause probable | Remède |
|---|---|---|
| « Recipient phone number not in allowed list » | toujours en mode développement | terminer la vérification entreprise (§5) + paiement actif |
| Template « En attente » > 48 h | revue manuelle | vérifier la catégorie UTILITY et le contenu strictement transactionnel ; resoumettre |
| Numéro refusé à la vérification | actif sur un autre WhatsApp | désinstaller l'app WhatsApp liée au numéro, attendre, recommencer (§4.3) — ou suivre le flux Coexistence proposé (§4.4) |
| Token invalide après quelques semaines | jeton temporaire utilisé (celui de l'app, 24 h) | regénérer un jeton SYSTEM USER (§6), pas celui du tableau de bord |
| Envoi 131047 / « Re-engagement message » | template inexistant pour ce kind | soumettre le template manquant du §8 |
| `(#100) … requires Direct Send` | compte non éligible Direct Send (§8.0) | utiliser un template approuvé (voie classique §8.1) |
| Avertissement « utility used as marketing » | contenu sorti du cadre transactionnel | resserrer le libellé des alertes ; revue possible auprès de wadirectsendapisupport@meta.com |

## 12. Une app Meta pour TOUS vos produits (architecture parapluie)

> Décision opérateur (N°161) : l'app créée au §2 ne sert pas qu'à MikCloud —
> elle porte les notifications WhatsApp de toutes vos applications
> (FTCI). L'architecture : UN portfolio + UNE app + UN jeton, et UN WABA
> (avec son numéro et ses templates) PAR PRODUIT.

### Ce qui est PARTAGÉ (une seule fois pour tout)

| Élément | Où | Effort |
|---|---|---|
| Business Portfolio « Freelance Technologies CI » | §1 | créé + vérifié UNE fois (§5) |
| Application Meta `ftci-apps` | §2 | créée UNE fois |
| Jeton System User permanent | §6 | UN SEUL jeton — il opère tous les WABAs qu'on lui assigne |
| Vérification entreprise + carte bancaire | §5 | UNE fois — exigence du PORTFOLIO, pas du produit |

### Ce qui est PAR PRODUIT (isolation native)

| Élément | Isolation |
|---|---|
| WABA (compte WhatsApp Business) | ses propres templates, son propre paiement/pays de facturation |
| Numéro d'envoi dédié | une SIM par produit qui envoie |
| Nom affiché aux destinataires | « MikCloud Alertes » côté MikCloud — chaque produit porte SA marque |
| Note de qualité (quality rating) | PAR NUMÉRO : un produit dégradé (spam, blocages) n'entraîne PAS les autres |
| Éligibilité Direct Send (§8.0) | PAR WABA |
| Templates | PAR WABA — les 4-5 du §8 vivent sur le WABA MikCloud uniquement |

### Ajouter un futur produit (ex. « appX ») — la recette

1. Business Settings → Comptes → Comptes WhatsApp → **créer un WABA**
   (nom affiché = la marque du produit, ex. « appX Notifications »).
2. WhatsApp Manager (sur ce WABA) → Numéros de téléphone → **ajouter le
   numéro dédié** du produit (nouvelle SIM, vérification SMS/appel §4).
3. Business Settings → Utilisateurs système → votre system user →
   **Attribuer des éléments** → cocher le NOUVEAU WABA (le jeton du §6
   l'opère immédiatement — rien à regénérer).
4. Soumettre les templates du nouveau produit sur CE WABA (discipline §8.1).
5. Côté backend du produit : le MÊME jeton + SES Phone Number ID / WABA ID
   (variables d'environnement propres, cf. §9).

Coût d'ajout : UNE SIM + les messages à l'usage (mêmes taux §10). Zéro
nouvelle démarche d'entreprise, zéro nouvelle app, zéro nouveau jeton.

### Pourquoi PAS une app par produit ?

- Aucun bénéfice : le nom de l'app n'est JAMAIS visible des destinataires
  (seul le nom du WABA l'est) ; la qualité est notée par numéro, pas par app.
- Que des coûts : multiplié les jetons à garder, les écrans de dashboard, la
  charge mentale — et Meta limite le nombre d'apps par compte développeur.

### Limite du modèle (pour mémoire)

Ce modèle couvre VOS produits qui envoient depuis VOS numéros. Le jour où
vous voudriez que vos CLIENTS apportent LEUR propre numéro WhatsApp dans
votre plateforme (onboarding automatisé), c'est un AUTRE programme Meta —
« Tech Provider » / Embedded Signup (validation d'app, revue, contrat
partenaire) : hors périmètre de ce runbook, à ouvrir seulement si ce besoin
concret apparaît.
