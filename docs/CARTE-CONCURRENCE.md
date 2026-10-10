# CARTE DE CONCURRENCE — le champ complet autour de MikCloud

> **Suite directe de N°286** (`docs/COMPARATIF-PHENIXSPOT.md`) : PhenixSPOT
> n'était que le concurrent direct le plus visible. Ce document élargit la
> carte à TOUT le champ — le gratuit dominant, les « Mikhmon cloud », la
> nouvelle catégorie « vente automatique Mobile Money », le local établi et
> les suites globales — pour positionner MikCloud comme **la patronne
> indispensable**, pas comme une alternative de plus.
>
> Méthode : recherches web + lecture directe des sites publics des éditeurs
> (le 2026-10-10). Les chiffres cités sont ceux que les éditeurs publient
> eux-mêmes ; les pages 404 ou sans rendu sont signalées comme telles.

---

## 1. Le verdict en 30 secondes

1. **La boucle « achat Mobile Money → voucher automatique » n'est plus un
   avantage à construire — c'est la nouvelle norme du marché.** Trois joueurs
   africains la font nativement (Viamikro, Jasiyo, NextFi). Le chantier N°1 de
   la roadmap N°286 est confirmé en tête — mais il devient un **ticket
   d'entrée**, pas un différenciateur.
2. **Le vrai champ de bataille est le modèle de prix et de flux d'argent** :
   gratuit (Mikhmon) → micro-abonnement + commission (Viamikro) →
   revenue-share avec plancher (Jasiyo, NextFi) → abonnement plat
   (PhenixSPOT, MikCloud) → suites globales hors de prix (Powerlynx, Splynx).
3. **La case vide du marché — personne ne la combine** : paiement client
   final automatisé **avec l'argent qui tombe directement chez le gérant**
   (zéro commission, zéro retrait) + gestion **CGNAT-proof** (agent sortant)
   + sécurité cloud (boucliers) + offline-first + coût d'infra ≈ 0. C'est
   exactement la case que MikCloud peut occuper avec ses briques existantes.

---

## 2. Le champ cartographié (5 cercles, du plus proche au plus lointain)

### Cercle 1 — Le gratuit dominant : **Mikhmon** (l'incumbent à déloger)

| | |
|---|---|
| Qui | Laksa19 (Indonésie), open-source, PHP auto-hébergé, communauté mondiale massivement africaine/asiatique |
| Prix | **0 €** — le concurrent impossible à battre sur le prix, à dépasser sur tout le reste |
| Fait | Gestion hotspot MikroTik, vouchers, sessions, sans serveur RADIUS ; v6.5 annonce suivi PPPoE et intégrations passerelle de paiement (page Facebook, non vérifiable en détail) |
| Ne fait pas | Rien dans le cloud (PC allumé requis), pas de vente automatisée au client final, pas de sécurité cloud, pas de flotte multi-sites, pas de support, pas de DR |
| Pour MikCloud | C'est la **source de migration** : l'offensive « import Mikhmon 1-clic » (P0 N°286) vise exactement cette base ; le pitch « Mikhmon a un PC allumé, MikCloud vit dans le cloud » est le croche-pied idéal |

### Cercle 2 — Les « Mikhmon cloud » : héberger ce que Mikhmon laisse chez vous

- **Easy-Mikhmon** (easy-mikhmon.com) — hébergement géré type-Mikhmon :
  vouchers, sessions live, rapports de revenus, multi-routeurs. Revendique
  **48 pays** dont Côte d'Ivoire, Burkina, Mali, Niger, Nigeria, Sénégal.
  Test gratuit 1 routeur/2 semaines ; palier dédié dès 10 routeurs.
  → *Menace moyenne : ilcloud-ise Mikhmon, mais sans vente automatisée ni
  sécurité. Cible de migration identique à Mikhmon.*
- **MKController** (mkcontroller.com) — contrôleur cloud MikroTik (essai 7 j),
  focus supervision d'appareils, pas la monétisation.
- **Super Mikhmon** (Android) — app de vente de vouchers, terrain, sans cloud.

### Cercle 3 — La catégorie qui monte : **vente automatique Mobile Money** ⚠

> C'est ici que le marché a bougé depuis N°286. Trois joueurs natifs Afrique
> vendent le WiFi payé et livré automatiquement.

- **Viamikro / ViaPay** (viamikro.com, Ouagadougou) — ⚠ **menace n°1, fiche
  complète ci-dessous**.
- **Jasiyo** (jasiyo.com, Kenya/Ouganda) — hotspot **et PPPoE**, MikroTik
  RouterOS 7 par scripts ; paiements M-Pesa (Paybill/Till/banque), PayHero,
  Paystack, ioTec (MTN/Airtel Ouganda) ; réconciliation automatique +
  **renouvellements automatiques** ; SMS confirmations/relances (émetteur
  intégré au Kenya, BYO ailleurs). Prix : essai 7 j, puis **3 % du revenu
  hotspot OU KSh 25/client PPPoE, minimum KSh 1 000/mois** (~6 000 FCFA).
- **NextFi** (nextfisystems.com) — « Africa & Asia », routeurs
  RADIUS-compatibles (MikroTik, Ubiquiti, TP-Link, Cisco, Huawei) ;
  MTN/Airtel/M-Pesa/Paystack/cartes, réconciliation automatique ;
  **white-label NaaS** (« lancez votre propre marque ») ; setup < 30 min.
  Prix : **« Core Rate: 3 % flat · Flex Rate: 1,5 % vouchers / 5 % MoMo »**
  (revenue-share, pas d'abonnement par appareil). 12+ pays listés dont
  **RD Congo (Vodacom M-Pesa & Orange)**.
- **PassiWiFi** (passiwifi.com, BARNESS COMPANY) — app « Vendez votre WiFi
  par Mobile Money » (Orange Money, Moov, Wave) ; site SPA sans rendu
  extractible au 2026-10-10 — à surveiller, profondeur inconnue.

#### Fiche menace n°1 — Viamikro/ViaPay

| | |
|---|---|
| Qui | Ouagadougou, BF — « Pensé pour l'Afrique, par des Africains » ; ~**500 clients** revendiqués, **24 pays** (toute l'Afrique de l'Ouest + Centre + Est), témoignage **Abidjan** publié |
| Offre | ViaPay hotspot (portail captif + paiement **Wave, Orange Money, MTN, Moov, Free Money, Airtel Money** + GNF Guinée), **ViaRadius** (RADIUS central multi-routeurs : *un seul pool de tickets sur tous vos points de vente*), VPN L2TP, **WireGuard**, « Mikhmon Online » (gestion à distance), app **Android**, programme de parrainage |
| Prix | **500 FCFA/mois** (services, 10 crédits) + **1 000 FCFA/mois par hotspot** (20 crédits) + **commission sur chaque vente** (taux non publié à l'accueil) ; crédits : 1 = 50 FCFA ; gains retirables par mobile money → **l'argent transite par Viamikro** |
| Install | Script généré à coller sur le routeur (RouterOS 7.x pour ViaPay, 6-7 pour le reste) — même approche que MikCloud |
| Forces | Prix brûlants, packaging tout-en-un, paiement client final automatisé, marque africaine, tutoriels vidéo, support WhatsApp |
| Faiblesses structurelles | Commission + retrait = **l'argent du gérant dort chez la plateforme** ; ViaRadius exige un RADIUS joignable (question CGNAT non adressée publiquement) ; pas de sécurité cloud publiée ; pas d'offline ; scale-up du prix avec le revenu (commission) |

### Cercle 4 — Le concurrent local établi : **PhenixSPOT**

Référence complète : `docs/COMPARATIF-PHENIXSPOT.md` (N°286). Grand-Bassam,
7 000 → 60 000 FCFA/mois plats, 4 modules manquants côté MikCloud (PPPoE,
cybercafé, SMS clients, WG client final), GTM rodé, **pas de vente en ligne
automatisée publiée**.

### Cercle 5 — Les suites globales (hors de prix, hors contexte)

| Plateforme | Prix publié (2026) | Pourquoi ils ne menacent pas le gérant FCFA |
|---|---|---|
| **Powerlynx** (powerlynx.app) | ~**€ 4 950/an** (≈ 34 000 FCFA/mois) pour la suite 11 passerelles ; facturation « par utilisateur en ligne » | 10-15× MikCloud ; anglo/mid-market ; aucun ancrage FCFA |
| **Splynx** | **$255+/mois** + modules (≈ 155 000+ FCFA/mois) | Suite ISP intégrale — sur-dimensionnée |
| **ISPbox** | $25-250/mois | Même décalage |
| **UISP CRM** (Ubiquiti) | gratuit | Écosystème Ubiquiti, pas MikroTik |
| Antamedia, Spotipo, Purple, SocialWiFi, Tanaza, Adipsys, XceedNet | divers | Guest-WiFi/marketing hôtelier ou intégrateurs — pas le cybercafé/quartier FCFA |

*Sources de prix : pages publiées par les éditeurs (Splynx vs BillMax,
ISPbox « Best ISP Billing 2026 », comparatifs Powerlynx — snippets datés
juin 2026, pages blog 404 à la relecture directe le 2026-10-10).*

---

## 3. La matrice décisive

Légende : ✅ fait · 🟡 partiel/annoncé · ❌ non publié. « MikCloud cible » =
après exécution P0+P1 (N°286 §7).

| Capacité | Mikhmon | Easy-Mikhmon | Viamikro | Jasiyo | NextFi | PhenixSPOT | **MikCloud aujourd'hui** | **MikCloud cible** |
|---|---|---|---|---|---|---|---|---|
| Vente en ligne automatisée (achat→voucher) | ❌ | ❌ | ✅ | ✅ | ✅ | ❌ | 🟡 (offres+waveUrl, sans livraison auto) | ✅ |
| **Argent direct chez le gérant (0 % commission)** | n/a | n/a | ❌ (commission + retrait) | ❌ (3 %, min) | ❌ (1,5-5 %) | ❌ (pas de boucle) | **✅ (Wave direct, −3 % bancaire)** | **✅** |
| Gestion CGNAT-proof (agent sortant) | n/a (local) | 🟡 (VPN dédié) | 🟡 (non adressé) | 🟡 (scripts, non adressé) | 🟡 (RADIUS, non adressé) | ❌ (RADIUS/IP publique) | **✅ (agent 45 s)** | ✅ |
| PPPoE | 🟡 (suivi v6.5) | ❌ | ❌ publié | ✅ | ✅ | ✅ | ❌ | ✅ (P1, via tunnel WG) |
| Sécurité cloud (boucliers, walled garden, QoS) | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | **✅ (4 boucliers + PoolDoctor)** | ✅ |
| Offline-first (PWA vente en tournée) | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | **✅** | ✅ |
| Multi-devises FCFA/NGN/GHS/KES | ❌ | ❌ | 🟡 (FCFA/GNF) | 🟡 (KSh/UGX) | ✅ | ❌ (FCFA) | **✅** | ✅ |
| Flotte : updates RouterOS centralisées | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | **✅** | ✅ |
| Coût d'infra ≈ 0 (Always Free) | n/a | — | — | — | — | — | **✅** | ✅ |

Lecture : **seule la colonne « MikCloud cible » n'a aucun ❌.** C'est le sens
du mot « patronne indispensable » : être le seul à cocher la ligne d'argent
directe ET la ligne CGNAT ET la ligne sécurité.

---

## 4. Le champ de bataille des modèles de prix

```
prix/mois pour le gérant (ordre croissant, ordre de grandeur)

Mikhmon ............ 0                (mais PC allumé + vente manuelle)
Viamikro ........... 500-1 000 + commission par vente (argent chez la plateforme)
NextFi ............. 0 fixe + 1,5-5 % du revenu
Jasiyo ............. min ~6 000 FCFA (plancher) ou 3 % du revenu
MikCloud ........... 2 500 /routeur (25 000/an, essai 60 j) — Wave −3 % bancaire
PhenixSPOT ......... 7 000-60 000 plats
Powerlynx .......... ~34 000
Splynx ............. 155 000+
```

Trois conséquences stratégiques :

1. **La pression vient du bas, pas de PhenixSPOT.** Viamikro à 500-1 000
   FCFA + commission rend le « moins cher » indéfendable. Le terrain à
   occuper n'est pas le prix : c'est **« tout l'argent est à vous, tout de
   suite »** — chez Viamikro le gérant retire ses gains (moins commission),
   chez Jasiyo/NextFi une part du chiffre part en plate-forme ; avec la
   boucle Wave directe de MikCloud, **0 % part en commission et le paiement
   atterrit sur le compte du gérant en temps réel**. Sur un hotspot à
   20 000 FCFA de ventes/mois, 3 % = 600 FCFA/mois perdus à vie — l'argument
   se calcule, pas se promet.
2. **Le modèle commission scale AVEC le revenu du gérant ; le plat scale
   CONTRE.** À petite échelle Viamikro paraît imbattable ; dès que le
   hotspot tourne, la commission rattrape puis dépasse le plat. C'est
   l'argument de vente aux gérants qui réussissent — nos meilleurs clients.
3. **La recommandation tarifaire N°286 (4 000-5 000 FCFA d'entrée) doit être
   re-située** : le plat actuel à 2 500/routeur reste défendable UNIQUEMENT
   si l'argument cash-flow direct est livré avec le chantier N°1. Sans la
   boucle de vente, le plat paraît cher face à Viamikro ; avec elle, il
   paraît juste face à 3 % à vie.

---

## 5. Les 5 menaces classées + ripostes

| # | Menace | Pourquoi c'est une menace | Riposte MikCloud |
|---|---|---|---|
| 1 | **Viamikro/ViaPay** | Prix brûlants + boucle Mobile Money + packaging tout-en-un + marque africaine + app Android + témoignage Abidjan | **L'argent direct** (0 % commission, retrait instantané, pas d'argent dormant) + agent CGNAT-proof + boucliers ; le déploiement N°287 (kit sect-api) montre notre rigueur d'exploitation |
| 2 | **Easy-Mikhmon** | Cloud-ise la base Mikhmon (48 pays) avant nous | Import Mikhmon 1-clic (P0) + pitch « Mikhmon sans PC allumé, en mieux » |
| 3 | **NextFi** | Échelle (Africa+Asia), white-label NaaS, multi-marques RADIUS | Profondeur locale CI + multi-devises + support FR + offline PWA ; nos 4 boucliers n'existent chez personne |
| 4 | **Jasiyo** | PPPoE + Mobile Money + renouvellements auto (Est-africain, pourrait descendre vers l'UEMOA) | PPPoE via API RouterOS **derrière CGNAT** (tunnel WG N°285) = la voie que leur RADIUS n'a pas |
| 5 | **PhenixSPOT** | Établi localement, GTM rodé, 121+ opérateurs publiés | Feuille de route N°286 §7 (vente en ligne d'abord) |

---

## 6. Ce que ça change pour la feuille de route (arbitrage P0)

1. **Chantier N°1 confirmé en tête — avec un twist décisif.** Le
   différenciateur n'est PAS « payer en ligne » (le marché entier le fait
   maintenant) : c'est **« l'argent tombe directement dans le Wave/OM du
   gérant, sans commission ni retrait »**. L'architecture actuelle (grille
   d'offres + `waveUrl` par profil + webhooks GeniusPay idempotents côté
   SaaS) est structurellement orientée « argent direct » — c'est un avantage
   de conception, pas un raccourci. À marteler dans chaque page, chaque
   démo, chaque comparatif.
2. **Ajout P0 recommandé : une page tarifs publique.** PhenixSPOT, Viamikro,
   Jasiyo et NextFi publient tous leurs prix ; les comparatifs SEO (comme
   celui de Powerlynx) n'inventorient que ceux qui publient. MikCloud y est
   invisible aujourd'hui. Une page `/pricing` franche (2 500/routeur, essai
   60 j, **« zéro commission, l'argent est à vous »** en titre) est un
   chantier de jours, pas de semaines.
3. **La doctrine de positionnement à congeler** (une phrase, trois coups) :
   > *Mikhmon exige un PC allumé. PhenixSPOT gère vos tickets. Viamikro
   > prend une commission sur chaque vente. MikCloud pilote votre réseau,
   > protège vos clients et vous laisse tout l'argent.*
4. **Garder en réserve** : le reste du P0 (SMS, import Mikhmon, MAC Access,
   preuve sociale) et le P1 (PPPoE par tunnel WG, WG vendable) inchangés —
   la cartographie ne les réordonne pas, elle renforce seulement le n°1.

---

## 7. Sources (toutes consultées le 2026-10-10)

- viamikro.com — page d'accueil complète (ViaPay, ViaRadius, tarifs 500/1 000
  FCFA + commission, 24 pays, ~500 clients, paiement Wave/OM/MTN/Moov/Free/Airtel)
- jasiyo.com — page d'accueil complète (hotspot+PPPoE, M-Pesa/PayHero/Paystack/
  ioTec, 3 % ou KSh 25/PPPoE, min KSh 1 000/mois, Kenya+Ouganda)
- nextfisystems.com — page d'accueil complète (Core 3 % / Flex 1,5-5 %,
  white-label NaaS, 12+ pays dont RDC)
- easy-mikhmon.com — page d'accueil complète (48 pays, hébergement géré,
  test gratuit 2 semaines)
- powerlynx.app — page d'accueil ; prix suite (€4 950/an, 11 gateways) et
  comparatifs « Best Wi-Fi Hotspot Software 2026 / Africa » : snippets datés
  juin 2026 (pages blog 404 à la relecture directe)
- splynx.com, ispbox.net (snippets prix 2026), antamediahotspot.com,
  xceednet.com, adipsys.com, tanaza.com (présence générale)
- passiwifi.com — titre « Vendez votre WiFi par Mobile Money » (SPA sans
  rendu extractible ; guide blog 404)
- laksa19.github.io + GitHub/Softonic/Docker Hub (Mikhmon : gratuit,
  auto-hébergé, sans RADIUS) ; page Facebook Mikhmon v6.5 (PPPoE + payment
  gateway, non vérifiable en détail)
- mikcloud : `frontend/src/components/landing/landing-copy.ts` (tarifs
  publics 2 500/routeur/mois · 25 000/an · essais 60/30 j · Wave −3 %) ;
  `docs/COMPARATIF-PHENIXSPOT.md` (N°286) ; `docs/PILOTAGE-ORACLE.md`
