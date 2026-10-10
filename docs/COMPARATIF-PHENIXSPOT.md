# COMPARATIF CONCURRENTIEL — MikCloud vs PhenixSPOT (N°286)

> **Date** : 2026-10-10 · **Auteur** : FTCI / Freelance Technologies Côte d'Ivoire
> **Objet** : comparaison experte MikCloud ↔ [phenixspot.com](https://phenixspot.com/), écarts,
> avantages de chacun, et feuille de route pour faire de MikCloud **le concurrent idéal —
> le patron, l'indispensable** du marché africain de la gestion Wi-Fi/Hotspot.
> **Méthode** : lecture directe du site public PhenixSPOT (accueil, fonctionnalités, tarifs)
> le 2026-10-10 + inventaire du code MikCloud (source de vérité : ce que le produit FAIT,
> pas ce qu'il promet). Les allégations PhenixSPOT sont marquées « *publié* » (non vérifiable
> de l'extérieur) ; les faits MikCloud sont vérifiés dans le code.

---

## 1. Résumé exécutif

| Verdict | Détail |
|---|---|
| **PhenixSPOT est un concurrent réel et bien armé** | Même marché (Côte d'Ivoire → Afrique de l'Ouest), même cible (WISP, ISP locaux, cybercafés, hôtels), même cœur (tickets Hotspot MikroTik + portail captif + Mobile Money). Grand-Bassam, « Powered by Phenix IT Solutions ». |
| **Son avance est commerciale, pas technique** | 4 modules que MikCloud n'a pas : **PPPoE/AAA RADIUS**, **Cybercafé**, **SMS clients finaux** (rappels D-7/D-3/D-0), **VPN WireGuard vendu au client final**. Plus un go-to-market déjà rodé : tarifs publics, migration Mikhmon 1-clic, témoignages (121+ opérateurs *publié*), blog/guides, codes promo, funnel WhatsApp. |
| **L'avance de MikCloud est technique et structurelle** | Mode **agent sortant 100 % CGNAT-proof** (zéro port ouvert côté routeur), **tunnel WireGuard de renfort routeur** (N°285, clé privée générée sur l'appareil), **4 boucliers cloud** (SafeWiFi/Shield/FamilyGuard/AntiVPN) + PoolDoctor + QoS parent queue, **gestion de flotte** (updates RouterOS unitaire + flotte, télémétrie), **PWA offline-first** avec file d'attente hors ligne, **mode simulé** (démo complète sans matériel), multi-devises pan-africain, discipline ingénierie (CI, tests, DR monitor 11 familles, backups chiffrés). |
| **Le point de bascule commercial** | PhenixSPOT vend « **Vente en ligne 24h/24** » (achat ticket sur le portail → Mobile Money → reçu auto → renouvellement 1 clic). MikCloud a déjà la grille d'offres + lien Wave par profil (`waveUrl` dans le portail) et l'encaissement GeniusPay/Wave côté SaaS — mais **pas encore la boucle automatique achat → livraison voucher → reçu**. C'est le chantier N°1. |
| **Arme structurelle** | Infra MikCloud = **OCI Always Free** (coût fixe ≈ 0) + DB hybride memory-first → MikCloud peut pratiquer des **prix inférieurs à périmètre égal** avec des quotas plus généreux, durablement. PhenixSPOT facture 7 000 → 60 000 FCFA/mois avec quotas stricts et SMS facturés en plus. |

**En une phrase** : PhenixSPOT a une longueur d'avance sur le *business* (PPPoE, cybercafé, SMS,
vente en ligne automatisée, présence marché) ; MikCloud a plusieurs longueurs d'avance sur le
*produit-réseau* (connexion CGNAT-proof, sécurité, protection, flotte, offline). Le chemin pour
« le patron » est clair : fermer les 5 écarts commerciaux en ~90 jours, puis creuser les douves
techniques que le concurrent ne pourra pas copier vite.

---

## 2. Fiche signalétique

| | **MikCloud** (FTCI) | **PhenixSPOT** (Phenix IT Solutions) |
|---|---|---|
| Positionnement | Plateforme SaaS de gestion professionnelle de hotspot MikroTik + pare-feu cloud + pilotage de flotte | « Simplified Hotspot, PPPoE & Cybercafe Management » — WISPs, ISP locaux, cybercafés, intégrateurs |
| Marché visé | Afrique pan-continental (UEMOA, CEMAC, Afrique de l'Est, Nigeria, Ghana) — FR/EN, FCFA/NGN/GHS/KES/EUR/USD | 15+ pays *publié* — site FR/EN/DE/ES, facturation FCFA |
| Architecture | Monorepo Go 1.27 + Next.js 16 PWA ; backend OCI VM (Docker distroless + Caddy, Render gelé) ; DB hybride memory-first (RAM → PG → WAL chiffré bucket OCI → backups chiffrés) ; monitor DR 11 familles | Non public (cloud SaaS classique, badge SSL, SLA 99,9 % *publié*) |
| Connexion routeurs | **3 voies** : agent HTTP-poll sortant (TLS strict, 45 s, anti-orphan) · API RouterOS binaire native (mode real) · **tunnel WireGuard renfort opt-in par routeur** (N°285) · mode simulé intégré | RADIUS/NAS + « Add your routers, connect RADIUS » ; « Optimized for MikroTik » ; onboarding 3 min *publié* |
| Essai / onboarding | 60 jours Hotspot / 30 jours HomeNet ; **mode simulé = démo complète sans matériel** ; inscription self-serve | Essai gratuit + onboarding 3 min + démo par WhatsApp ; migration Mikhmon 1-clic |
| Tarifs publics | Segmentés par mode (Hotspot / HomeNet), frais paiement répercutés (Wave −3 %) | **Starter 7 000 / Business 15 000 / Pro 25 000 / ISP 60 000 FCFA par mois** ; SMS non inclus (packs rechargeables) ; support prioritaire Pro+ |
| Presence marché | Pré-lancement commercial (vitrine + PWA + e2e) ; audit Mikhmon V3 comme feuille de parité | 121+ opérateurs actifs *publié*, témoignages clients, blog, guides, programme partenaires, codes promo (MIGRATIONMK, DECOLLAGE30), app stores « coming soon » |

---

## 3. Grille fonctionnelle détaillée

Légende : ✅ = oui, livré (vérifié côté MikCloud / publié côté PhenixSPOT) · 🟡 = partiel · ❌ = non.
Pour PhenixSPOT, tout provient de son site public — les profondeurs réelles (qualité, fiabilité,
sécurité) ne sont pas observables de l'extérieur.

### 3.1 Cœur Hotspot / tickets

| Capacité | MikCloud | PhenixSPOT |
|---|---|---|
| Génération vouchers en lot (durée/quota/débit/prix) | ✅ 1–500/lot, préfixe, longueur | ✅ multi-profils |
| Expiration automatique des sessions | ✅ | ✅ |
| Impression tickets prédécoupés / affiches Wi-Fi | ✅ (+ affiches posters) | ✅ |
| Traçabilité des lots (site, canal, revendeur, statut/voucher) | ✅ onglet Lots | 🟡 non détaillé |
| Transferts de vouchers entre comptes | ✅ | ❌ non publié |
| Anti-vol revendeur (stock vs remis, SoldAt/SoldVia) | ✅ PWA Vente en tournée (PIN 4–6) | ❌ non publié |
| Portail captif brandé (logo, fonds, mentions) | ✅ templates + éditeur + slides + ticker + WhatsApp | ✅ |
| Offres commerciales sur le portail (prix + paiement) | 🟡 grille d'offres + `waveUrl` par profil (lien Wave) — **livraison auto du voucher post-paiement à construire** | ✅ « Vente en ligne 24h/24 » : achat → Mobile Money → reçu auto SMS/email → renouvellement 1 clic |
| Social voucher / campagnes (partage contre bonus) | ❌ | ✅ « Social Marketing » (Pro/ISP) |

### 3.2 Segments au-delà du hotspot

| Capacité | MikCloud | PhenixSPOT |
|---|---|---|
| **PPPoE abonnés** (AAA RADIUS, comptes, profils débit, suspensions/reprises) | ❌ | ✅ jusqu'à illimité (Business/ISP), dashboard abonnés temps réel |
| Renouvellements auto + rappels **SMS** D-7/D-3/D-0 + suspension auto | ❌ (Telegram/annonces côté opérateur seulement) | ✅ |
| **Cybercafé** (postes, codes-temps, facturation) | ❌ | ✅ 20 PCs (Pro) → illimité (ISP) |
| **MAC Access** — abonnement par appareil sans login | 🟡 briques présentes (IP bindings, bypass, pause appareil) — pas packagé « produit » | ✅ |
| **VPN WireGuard client final** (comptes vendus, .conf par SMS/email) | 🟡 plomberie complète en place (wg0 N°284 serveur + N°285 routeurs) — produit de vente à créer | ✅ tunnels managés, révocation dashboard, .conf auto SMS/email |
| Résidentiel / HomeNet (sécurité foyer) | ✅ mode dédié | ❌ non publié |
| Hôtels / hospitalité (promos, réseaux sociaux, consentement) | ✅ mode hospitalité | 🟡 « Hotels » dans les ressources du site |

### 3.3 Réseau, sécurité, exploitation

| Capacité | MikCloud | PhenixSPOT |
|---|---|---|
| Connexion routeur ** derrière CGNAT sans port ouvert** | ✅ agent sortant = socle (check-in 45 s) | 🟡 non documenté (RADIUS initié par le routeur = OK, mais gestion/pilotage non documenté) |
| **Tunnel WireGuard de gestion** routeur ↔ plateforme | ✅ N°285 : opt-in par routeur, clé privée générée SUR l'appareil, PSK chiffrée au repos, test dial 8728/8291, anti-collision d'adresses | ❌ (WG = produit client final chez eux, pas chemin de gestion) |
| Pare-feu cloud / protections | ✅ **4 boucliers** : SafeWiFi, Shield, FamilyGuard (contrôle parental), AntiVPN + PoolDoctor + walled garden | ❌ non publié |
| QoS avancée (parent queue, linequality) | ✅ | ❌ non publié (profils débit PPPoE seulement) |
| Gestion de flotte : updates RouterOS (unitaire + flotte), télémétrie CPU/uptime | ✅ | ❌ non publié |
| IP bindings / pause appareil / quotas par utilisateur | ✅ | 🟡 (limits hotspot) |
| Détection/anti VPN client | ✅ AntiVPN | ❌ non publié |
| Multi-routeurs + stats par site | ✅ 1 compte = N hotspots | ✅ |
| Délégation opérateurs (accès limité) | ✅ rôles gérant/équipe/revendeur, hiérarchie, RBAC + tests | ✅ |
| Alertes multi-niveaux (déconnexion routeur, seuils) | ✅ notifications + cloche + monitor DR | ✅ |
| Canaux opérateur : Telegram (pairing, notifs), chat interne, chatbot, annonces | ✅ | 🟡 non publié |

### 3.4 Plateforme, données, conformité

| Capacité | MikCloud | PhenixSPOT |
|---|---|---|
| Multi-devises | ✅ FCFA, EUR, USD, NGN, GHS, KES… | 🟡 FCFA (facturation) |
| i18n applicatif | ✅ FR/EN complet (30+ dictionnaires) | ✅ FR/EN/DE/ES (site) |
| 2FA / verrou | ✅ TOTP + PIN lock + politique mots de passe + audit auth | ❌ non publié |
| Chiffrement au repos des secrets | ✅ secretbox (PSK, mots de passe) | ❌ non publié |
| RGPD / registre de traitement | ✅ registre + page confidentialité | 🟡 Privacy + Terms publiés |
| Purge/retention outillée | ✅ purge par catégories + tombstones | ❌ non publié |
| Sauvegardes / DR | ✅ WAL chiffré bucket OCI, backups Tier3, monitor DR 11 familles, recovery testé | 🟡 « perte de données après incident » citée par un client comme résolue |
| PWA installable + **mode hors ligne** (file d'attente offline) | ✅ | 🟡 PWA site + app stores « coming soon » |
| Export comptable CSV/PDF | 🟡 impression + journaux mensuels ; export machine à industrialiser | ✅ CSV/PDF 1 clic + rapports par email |
| Heatmap horaire / heures de pointe | ✅ | ✅ |
| Encaissement SaaS de la plateforme | ✅ GeniusPay (Wave direct + carte via Stripe), webhooks HMAC + anti-replay | 🟡 non public |

### 3.5 Go-to-market

| Capacité | MikCloud | PhenixSPOT |
|---|---|---|
| Tarifs publics détaillés + comparateur | 🟡 pricing segmenté sur la vitrine | ✅ 4 plans + tableau comparatif filtrable |
| Outil de migration concurrent | ❌ (audit Mikhmon V3 interne seulement) | ✅ import Mikhmon CSV 1-clic + 1er mois offert (MIGRATIONMK) + offensive anti-Mikhmon |
| Preuve sociale | ❌ à construire | ✅ témoignages nommés + modules utilisés |
| Contenu SEO (blog, guides) | ❌ | ✅ blog + guides + pages solutions (hôtels, ISP/WISP) |
| Programme partenaires | 🟡 revendeurs produit (wallet) — pas de programme officiel | ✅ page Partners |
| Funnel WhatsApp démo | 🟡 intégration WhatsApp portail ; funnel démo à créer | ✅ formulaire WhatsApp → démo + offre |
| App stores | 🟡 PWA installable ; TWA à publier | 🟡 « coming soon » — personne n'a encore sorti |

---

## 4. Écarts — ce que PhenixSPOT a et que MikCloud doit combler

Classés par impact commercial décroissant :

1. **Boucle de vente en ligne automatisée (portail)** — Le client final achète une offre sur le
   portail → Mobile Money (Wave/OM/MTN via agrégateur, ex. GeniusPay déjà intégré ou CinetPay) →
   **voucher délivré automatiquement** (affiché + WhatsApp/SMS) → reçu → renouvellement 1 clic.
   MikCloud a la grille d'offres et le lien Wave par profil, mais pas la livraison automatique
   post-paiement. C'est L'argument marketing n°1 du concurrent (« Vente en ligne 24h/24 ») et le
   premier motif de churn d'un opérateur (vendre la nuit sans être présent).
2. **PPPoE / abonnés fixes** — Segment ISP/WISP entier inaccessible sans lui. À construire via
   l'API RouterOS (secrets, profils débit, suspensions) — MikCloud peut même le faire **derrière
   CGNAT** grâce à l'agent + tunnel WG (l'approche RADIUS du concurrent exige un NAS joignable) ;
   RADIUS AAA ensuite pour les gros ISP.
3. **Canal SMS clients finaux** — rappels D-7/D-3/D-0, reçus, packs rechargeables. MikCloud a
   Telegram (opérateur) et WhatsApp (portail) ; il manque le SMS universel (téléphones basiques).
4. **Module Cybercafé** — postes, codes-temps, facturation. Niche mais présent dans chaque
   appel d'offres local ; conditionne les plans Pro/ISP du concurrent.
5. **Social voucher / campagnes** — partage WhatsApp/Facebook contre bonus ; levier d'acquisition
   virale pour les opérateurs.
6. **MAC Access packagé** — « abonnement appareil » sans login : les briques existent
   (ipbindings, bypass, pause appareil), il manque l'habillage produit + facturation récurrente.
7. **Go-to-market** — preuve sociale (témoignages, études de cas pilotes), outil d'import
   Mikhmon 1-clic (+ page « migrer depuis PhenixSPOT »), blog/guides SEO, programme partenaires,
   funnel WhatsApp démo, app stores (TWA).

---

## 5. Douves — ce que MikCloud a et que PhenixSPOT ne montre pas

1. **Connexion CGNAT-proof par construction** : l'agent sortant (check-in HTTPS 45 s, TLS strict,
   anti-orphan N°230) fonctionne derrière le CGNAT le plus hostile, sans toucher au pare-feu du
   routeur. Le concurrent s'appuie sur RADIUS/NAS — la gestion fine (updates, QoS, walled garden)
   n'a pas de chemin documenté dans ce contexte.
2. **Renfort WireGuard de gestion (N°285)** : deuxième chemin chiffré routeur ↔ plateforme, clé
   privée jamais transportée, PSK chiffrée au repos, test dial honnête. Personne ne l'offre.
3. **Suite protection cloud** : SafeWiFi, Shield, FamilyGuard, AntiVPN + PoolDoctor + walled
   garden — MikCloud est un **pare-feu cloud**, pas seulement un vendeur de tickets.
4. **Pilotage de flotte** : updates RouterOS unitaires + flotte, télémétrie, scheduler, QoS
   parent queue, linequality — le concurrent gère des accès, MikCloud gère des réseaux.
5. **Mode simulé intégré** : démonstration complète de la plateforme sans aucun matériel — arme
   d'acquisition et de support inégalée.
6. **PWA offline-first** : file d'attente hors ligne, installation sur l'écran d'accueil — la
   réalité des connexions africaines est traitée en première classe.
7. **Structure de coûts quasi nulle** : OCI Always Free + DB hybride memory-first = coût fixe ≈ 0.
   Possibilité durable de sous-coter PhenixSPOT avec des quotas plus généreux (le concurrent doit
   financer son infrastructure à chaque client).
8. **Rigueur d'ingénierie** : CI (gofmt/vet/tests/build + ESLint/build), tests unitaires + e2e
   Playwright, monitor DR 11 familles, backups chiffrés testés, runbooks, registre RGPD — un
   argument de confiance enterprise que le concurrent ne publie pas.
9. **Distribution native** : wallet revendeur + PWA Vente en tournée (PIN, anti-vol) + transferts
   = un réseau de distribution multi-niveaux déjà dans le produit, à activer comme programme
   partenaires.

---

## 6. Analyse tarifs

| Plan PhenixSPOT | Prix/mois | Contenu clé |
|---|---|---|
| Starter | 7 000 FCFA | 1 routeur, 200 tickets, 1 VPN, hotspot seulement |
| Business | 15 000 FCFA | 3 routeurs, 500 tickets, PPPoE 100 comptes, MAC Access |
| Pro | 25 000 FCFA | 5 routeurs, 1 000 tickets, PPPoE 500, Cyber 20 PCs, Social Marketing |
| ISP | 60 000 FCFA | Illimité, support prioritaire |

**Recommandations MikCloud :**
- **Sous-coter systématiquement à périmètre égal** (ex. équivalent Starter à 4 000–5 000 FCFA,
  équivalent ISP ~40 000 FCFA) — la structure Always Free le permet durablement.
- **Quotas plus généreux** (routeurs et tickets) : notre limite est l'infrastructure (largement
  dimensionnée), pas une grille marketing.
- **Inclure un petit quota SMS** par plan (le concurrent en inclut zéro) — différenciation
  immédiate et visible.
- **Conserver l'essai 60 jours** comme arme d'acquisition (le concurrent ne le publie pas).
- **Remise mobile money conservée** (Wave −3 % vs carte +6 %) : alignée avec le marché.
- Le **mode HomeNet** reste un segment propre à MikCloud : ne pas le diluer dans la grille
  hotspot, c'est un marché additionnel que PhenixSPOT ne adresse pas.

---

## 7. Feuille de route « concurrent idéal »

### P0 — Combler les écarts qui font perdre des ventes (0–60 jours)
1. **Vente en ligne automatisée** : achat portail → paiement Mobile Money (GeniusPay déjà intégré
   côté SaaS ; évaluer CinetPay pour l'agrégation OM/MTN) → génération + livraison voucher
   automatique (page + WhatsApp) + reçu. Idempotence webhooks (pattern déjà en place GeniusPay).
2. **Canal SMS end-customer** : intégration agrégateur SMS local, packs rechargeables, templates
   de rappels (D-7/D-3/D-0), reçus automatiques.
3. **Import Mikhmon 1-clic** (CSV/.rsc : profils, users, vouchers) + page « Migrer » avec double
   offensive : Mikhmon ET PhenixSPOT (export CSV réimportable) — répliquer MIGRATIONMK avec
   « 1er mois offert + onboarding inclus ».
4. **MAC Access produit** : abonnement appareil sans login (ipbindings + profils + récurrence).
5. **Preuve sociale** : 3–5 études de cas pilotes (Wi-Fi public, hôtel, cybercafé), page
   témoignages, programme early adopters limité (rareté + onboarding inclus).

### P1 — Produits différenciants (60–120 jours)
6. **Module PPPoE** via API RouterOS (secrets, profils débit, suspensions/reprises, dashboard
   abonnés, renouvellements) — jouable **derrière CGNAT** grâce à l'agent + tunnel WG : c'est un
   avantage que l'approche RADIUS du concurrent n'a pas. RADIUS AAA en option pour les ISP
   équipés (étape 2).
7. **WireGuard vendable** : comptes VPN clients finaux sur le wg0 existant (N°284), .conf/QR
   livrés par WhatsApp/SMS/email, facturation par profil, révocation console → MikCloud devient
   la **seule** plateforme qui gère ET revend WireGuard sur le même écran.
8. **Module Cybercafé** : postes, codes-temps, facturation (module activable, plan Pro).
9. **Social voucher** : partage WhatsApp/Facebook contre bonus de quota/durée.
10. **Renouvellements automatiques + rappels** pour hotspot et PPPoE (suspension auto à
    expiration, reprise après paiement — pattern abonnement déjà en place côté SaaS).

### P2 — Creuser les douves, devenir l'indispensable (120–365 jours)
11. **App stores** : TWA Play Store à partir de la PWA — le concurrent n'a pas encore sorti la
    sienne : course gagnable.
12. **API publique + webhooks** : intégrateurs, PMS hôteliers, solutions de caisse.
13. **Programme partenaires officiel** : packager la distribution native (master agent →
    revendeur → sous-revendeur), remises par palier, portail partenaire.
14. **Statut public + SLA** : status page, monitoring externe (déjà en feuille de route README),
    publier les métriques DR — transformer la discipline interne en argument commercial.
15. **Contenu SEO FR** : guides « Wi-Fi public », « WISP », « cybercafé », « hôtels » +
    marketplace de templates de portail (effet réseau).
16. **Conformité ISP** : cartographier les exigences ARTCI/ARCEP pour le segment abonnés.

---

## 8. Le pitch « le patron, l'indispensable »

> **« PhenixSPOT gère vos tickets. MikCloud pilote votre réseau, protège vos clients et vend
> 24h/24. »**

La seule plateforme qui réunit, sur un même écran :
- une connexion routeur **qui survit au CGNAT** (agent sortant) **renforcée par un tunnel
  WireGuard direct** ;
- un **pare-feu cloud** (SafeWiFi, Shield, FamilyGuard, AntiVPN) et une **gestion de flotte**
  (updates RouterOS, QoS, télémétrie) ;
- la **vente en ligne Mobile Money automatisée** (achat → voucher → reçu, 24h/24) ;
- des **revendeurs outillés sur le terrain** (PWA tournée, PIN, anti-vol, wallet) ;
- une **PWA offline-first**, multi-devises pan-africaines, un **essai de 60 jours** et un
  **mode simulé** qui démontre tout sans matériel ;
- une infrastructure **souveraine et à coût quasi nul** (OCI Always Free) qui rend les prix
  agressifs durables — et une discipline d'ingénierie (CI, tests, DR chiffrée, RGPD) que
  personne d'autre ne publie.

---

## 9. Sources

- PhenixSPOT : accueil + fonctionnalités + tarifs — https://phenixspot.com/ (lu le 2026-10-10 ;
  pages `/pricing`, `/features`, `/fr/pricing`, `/en/pricing` : 404 nginx, le contenu est sur la
  page unique accueil).
- MikCloud : `README.md`, `CHANGELOG.md` (N°274–N°285), `AUDIT-MIKHMON-V3.md`,
  `docs/CONTRACT-V2.md`, `docs/DB-HYBRIDE.md`, `docs/PILOTAGE-ORACLE.md`, code
  `backend/internal/**` (agent, hotpage/templatize — offres `price + waveUrl`, geniuspay,
  handlers_wg), `frontend/src/components/landing/landing-copy.ts`.
- Conventions : RUNBOOK §8–§10, N°284/N°285 (WireGuard), N°281 (Phase B Docker).
