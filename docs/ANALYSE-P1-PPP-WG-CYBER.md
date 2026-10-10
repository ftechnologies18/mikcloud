# ANALYSE P1 — PPPoE · WireGuard vendable · Cybercafé (N°289)

> **Date** : 2026-10-10 · **Auteur** : FTCI / Freelance Technologies Côte d'Ivoire
> **Objet** : analyse approfondie, adossée au CODE, des trois chantiers sélectionnés
> par l'exploitant dans la feuille P1 60–120 j de N°286 (§7) : **⑥ Module PPPoE via
> API RouterOS**, **⑦ WireGuard vendable sur le wg0 existant**, **⑧ Module Cybercafé**.
> Objectif de ce document : donner la matière pour **trancher ensemble** (7 décisions
> listées en §7) avant d'écrire la première ligne de code de l'un des trois.
>
> **Arbitrage P0 de l'exploitant — consigné et acté** (il restructure la lecture de
> N°286/N°288) :
> - **Vente en ligne : DÉJÀ PRISE EN CHARGE.** Le paiement de ticket par lien
>   marchand **Wave direct** est inscrit dans le code (`grille d'offres + waveUrl`
>   du portail) : le client paie son ticket, **le gérant encaisse directement sur
>   son compte Wave marchand**. Wave est le leader du secteur — pas besoin
>   d'agrégateur supplémentaire. Le « chantier N°1 » est ainsi **soldé côté
>   argent** : l'argument « tout l'argent est à vous, 0 % commission » est
>   structurellement vrai dans l'architecture (cf. CARTE-CONCURRENCE §6, le twist).
> - **SMS : reporté.** Trop coûteux à ce stade ; prévu pour une implémentation
>   future quand le nombre de clients le justifiera.
> - **WhatsApp : en cours de développement, choix prioritaire** des canaux
>   clients finaux. **Mail et Telegram : déjà implémentés.**
> - **Arguments MikCloud déjà solides** (rappel de l'exploitant) : claim portail,
>   WiFi jetable, inscriptions publiques (`/registrations`), portail hybride à
>   deux modes (hospitalité / commercial).

---

## 1. Méthode et vérité terrain

Toute affirmation de ce document est vérifiée dans le code (références
`fichier:ligne`), par deux passes d'exploration exhaustive du backend
(`internal/routeros`, `internal/agent`, `internal/api`) et du frontend. Le principe
de N°286 est conservé : **le code est la source de vérité** — on analyse ce que le
produit FAIT, pas ce qu'il promet.

**Rappel architectural — les 3 plans de contrôle routeur** (c'est le socle des
trois chantiers) :

| # | Plan | Transport | État |
|---|---|---|---|
| 1 | **Mode real** — API RouterOS binaire native (port 8728) | TCP direct vers `router.Host:Port` | Client maison complet (`internal/routeros/protocol.go`) — entrée générique `Client.Run/Exec` acceptant TOUTE phrase RouterOS (protocol.go:92-128). Nécessite IP joignable → **rare derrière CGNAT**. |
| 2 | **Agent HTTP-poll sortant** (le chemin canonique) | Le ROUTEUR appelle le cloud (HTTPS 45 s), reçoit un script `.rsc` qu'il importe, et rend compte (`POST /agent/result`) | 44 types de commandes (`model/security.go:11-67`), générateurs dans `agent/agent.go:431-525` (`ScriptFor`), max 10 commandes/check-in (`agent_handlers.go:632-634`). 100 % sortant → **CGNAT-proof par construction**. |
| 3 | **Tunnel WireGuard de gestion** (N°285, opt-in) | Routeur ↔ VM wg0 (10.8.0.1), clés générées SUR l'appareil | Machine à états complète (`models.go:354-376`, `handlers_wg.go`, `agent/wireguard.go`). **La porte est ouverte mais jamais empruntée** : aujourd'hui seul un dial TCP `10.8.0.N:8728` teste la joignabilité (`handlers_wg.go:254-305`) — aucun appel API ne transite encore par le tunnel. |

---

## 2. Chantier ⑥ — PPPoE via API RouterOS

### 2.1 L'enjeu commercial

- **Le segment ISP/WISP est le seul segment récurrent mensuel par abonné** —
  c'est là que vivent les revenus prévisibles. Chez les concurrents :
  PhenixSPOT vend PPPoE **100 comptes à 15 000 / 500 comptes à 25 000 FCFA**
  (Business/Pro) ; Jasiyo facture **KSh 25/abonné/mois** (~10–15 FCFA) avec
  renouvellements automatiques ; Mikhmon 6.5 l'annonce en suivi (non vérifié).
- **L'avantage structurel est le CGNAT.** L'approche RADIUS des concurrents
  exige un NAS joignable / une infrastructure d'accès ; MikCloud pilote un
  routeur derrière le CGNAT le plus hostile via l'agent sortant (et demain le
  tunnel WG). C'est la phrase de vente : *« leur RADIUS ne passe pas là où
  notre agent passe »*.
- Dans la matrice décisive (CARTE §3), PPPoE est la ligne où **Jasiyo/NextFi/
  PhenixSPOT cochent ✅ et MikCloud ❌** — la plus grosse case encore rouge sur
  un segment qui paie.

### 2.2 Ce que le code dit (fait vérifié)

| Constat | Détail |
|---|---|
| **Le client RouterOS est générique** | `Client.Run(words...)` / `Client.Exec(...)` (`routeros/protocol.go:92-128`) envoient n'importe quelle sentence — `/ppp/secret/print`, `/ppp/profile/set`, `/ppp/active/remove` sont **à portée immédiate, zéro travail de protocole**. Mais uniquement en mode real (IP joignable). |
| **L'agent ne connaît pas PPP** | 44 types de commandes, tous hotspot (`read_state`, `user_add`, `voucher_batch`, `ipbinding_*`, `queue_*`, `wg_*`… `security.go:11-67`). Aucune commande PPP ; ajouter PPPoE = nouveaux builders `.rsc` + constantes + application des résultats (pattern `agent/users.go`, résultats `agent_handlers.go:683+`). |
| **« Profiles » = profils HOTSPOT, pas PPP** | `handlers_profiles.go:1-2` + `agent/profiles.go` gèrent `/ip hotspot user profile`. Aucune notion de profil PPP. |
| **Traces PPPoE : quasi nulles** | Seul `pppoe-out1` comme nom d'interface WAN dans les tests de qualité de ligne et la télémétrie read_state — de la surveillance de trafic, pas de gestion d'abonnés. |
| **Le modèle données est presque prêt** | `HotspotUser` (Username/Password/ProfileID/Status/ExpiresAt/DataQuotaMb/TimeLimitMin, `models.go:517-578`) et `Profile` (RateLimit/SessionTimeout/ValidityDays/ExpMode, `models.go:419-469`) se transposent presque 1:1 à un abonné PPP. `enforceExpired` + `ExpMode none|notify|remove` (`helpers.go:364-435`) = mécanique de **suspension à expiration** prête ; l'extension F4 (`handlers_users_ops.go:58-121`) = **renouvellement manuel** prête. |
| **Le gap transport est précis** | Les routeurs en mode agent ne stockent **aucune cred API** (`Router.Host` = identité RouterOS, pas une IP — `agent_handlers.go:294-301`). « Parler à l'API à travers wg0 » exigerait de collecter/stocker des creds API par routeur (l'infrastructure secretbox existe, pattern `WgPSK` `models.go:371-375`). La voie doctrine-consistente (zéro cred, zéro port) = **nouvelles commandes agent `ppp_*`**. |

### 2.3 Architecture cible proposée — D1, D2 à trancher

**D1 — Transport canonique (recommandé : agent d'abord, tunnel en renfort).**

- **Canonique : commandes agent `ppp_*`** (`ppp_read_secrets`, `ppp_read_active`,
  `ppp_secret_add`, `ppp_secret_set` [enable/disable/rate-limit/profile/comment],
  `ppp_secret_remove`, `ppp_kick`) — reprend exactement les conventions des
  builders existants (header/`okVar`/`resultLines`, `agent.go:530-647`). Zéro cred
  stockée, zéro port, CGNAT-proof, dédup/backoff/zombie-requeue offerts par la
  file. Convergence 45 s — amplement suffisante pour du CRUD d'abonnés.
- **Renfort (phase B, opt-in)** : pour les routeurs **déjà tunnelés** (N°285), le
  cloud peut ouvrir l'API directe `10.8.0.N:8728` à travers wg0 pour les
  opérations temps réel (sessions actives live, kick instantané). Prérequis :
  collecter/stocker la cred API par routeur, chiffrée secretbox (pattern existant).
  Le tunnel devient alors ce que sa doctrine annonce depuis N°285 : *« ouvre la
  porte au pilotage direct »* (`agent/wireguard.go:9`).

**D2 — Périmètre v1 (recommandé : gérer, ne pas provisionner).**

- v1 = gestion des **secrets / profils PPP / sessions actives** d'un
  **pppoe-server DÉJÀ configuré** chez le WISP (la plupart l'ont : c'est leur
  cœur de métier). On ne touche pas au provisioning du serveur PPPoE lui-même
  (`/interface pppoe-server server`, service-name, pool d'adresses) en v1 —
  trop variable d'un réseau à l'autre ; provisioning **assisté** en phase B.
- Profils PPP : miroir du modèle Profile existant (rate-limit `up/down`,
  `remote-address`, expiration) ; suspensions = `disable` du secret (ExpMode) ;
  IP statique optionnelle par abonné (phase B, anti-collision type pool_doctor).

**Découpage en phases :**

| Phase | Contenu | Prérequis |
|---|---|---|
| **A — MVP « abonnés PPPoE »** | Modèle `PppSecret` + commandes agent `ppp_*` + parité cloud↔routeur (pattern read_state/repair hotspot) + dashboard abonnés (réutilise `users-view`/`profiles-view`/`sessions-view`) + suspension auto (ExpMode→disable) + renouvellement manuel (extend F4) | Aucun. Agent seulement. |
| **B — Renfort tunnel + confort** | API temps réel via wg0 (sessions/kick), creds API chiffrées, IP statiques + pool dédié, provisioning assisté pppoe-server | Routeur tunnelé opt-in |
| **C — Récurrent** | Renouvellements automatiques + rappels (WhatsApp en cours / SMS futur), RADIUS AAA optionnel pour gros ISP | Canal WhatsApp |

### 2.4 Risques et parades

- **Taille des réponses** : 200+ abonnés → pagination (pattern `ReadChunkSize=500`
  du read_state, `agent/readstate.go:21-32`) ; plafond ~64 KB du rapport respecté.
- **Convergence 45 s** : OK pour CRUD ; le kick « instantané » attend la phase B
  (tunnel) ou reste best-effort agent.
- **Double source de vérité cloud↔routeur** : même problème que le hotspot —
  solution identique (parité read_state + `MissingOnRouter` + repair).
- **RouterOS v6/v7** : syntaxe `/ppp` identique ; de toute façon l'agent impose
  ≥ 7.19 au register (`agent_handlers.go:275-289`).

### 2.5 Effort

**Le plus gros des trois** (vertical complet : modèle, agent, parité, API, UI) —
Phase A ≈ **4–6 semaines** par lots (modèle+API → agent+parité → UI →
suspension+extend). Mais chaque lot est un pattern déjà éprouvé trois fois
(hotspot users, vouchers, bindings).

---

## 3. Chantier ⑦ — WireGuard vendable sur le wg0 existant

### 3.1 L'enjeu commercial

- PhenixSPOT vend des **comptes VPN WireGuard au client final** dès son plan
  Starter (1 VPN ; `.conf` livrés par SMS/email, révocation dashboard). MikCloud :
  la plomberie existe, le produit ❌.
- La douve visée (N°286 §7) : devenir **la seule plateforme qui gère ET revend
  WireGuard sur le même écran** — le tunnel de gestion des routeurs (N°285) et
  le VPN vendu (clients finaux) partagent le même serveur wg0 et la même console.
- **Deux formes de produit sur le même substrat** (D4-adjacent) :
  - **P-A « Accès distant du gérant »** : un peer WireGuard donne au gérant
    l'accès à SON réseau (routeur, caméras, NAS) depuis n'importe où — CGNAT des
    deux côtés géré par le tunnel sortant. Faible bande passante, valeur sûre,
    coût de revient ≈ 0. Presque un cadeau de fidélisation.
  - **P-B « Client final full-tunnel »** (ce que vend PhenixSPOT) : sortie
    Internet par la VM de Marseille (IP fixe FR). Bande passante réelle →
    **garde-fous nécessaires** : plafond egress du tier Always Free (~10 To/mois,
    à confirmer à la console), pool 253 slots, ToS anti-abus (exit node),
    quotas par plan.
- Note de positionnement : MikCloud livre aussi **AntiVPN** (protection des
  hotspots contre le contournement par tunnel). Aucune contradiction — AntiVPN
  protège le réseau du gérant, le WG vendu est un produit séparé, révocable,
  facturé — mais le discours marketing doit trancher proprement la nuance.

### 3.2 Ce que le code dit (fait vérifié)

| Constat | Détail |
|---|---|
| **Le cycle de vie côté ROUTEUR est livré** | `wg_keygen` / `wg_setup` / `wg_teardown` (`agent/wireguard.go:130-206`), machine à états cloud `pending_keygen → ready → pending_setup → active` (`models.go:354-376`, confirmations dans `agent_handlers.go:840-896`), console 5 étapes (`router-wg-card.tsx`). C'est le tunnel de **gestion** — la base d'expérience, pas encore le produit vendu. |
| **Le cycle de vie côté SERVEUR existe… en SSH manuel** | `deploy/oracle/wg-peer.sh` (root) : `add` génère **déjà** une conf client full-tunnel + PSK + IPv6 ULA + QR ; `add-router`, `remove`, `qr`, `reip`, `list` (handshakes/transferts). Tout existe — mais fichiers 600 root, exécution SSH, zéro API. |
| **wg0 est un unique /24 partagé** | Routeurs de gestion (`AllowedIPs = 10.8.0.N/32`) et clients full-tunnel (`10.8.0.N/32 + fd00:8::N/128` côté serveur) cohabitent ; pool de 253 slots ; anti-collision cloud « un tunnel IP = un routeur tous comptes » (`handlers_wg.go:223-231`) mais l'allocation côté VM (`wg-peer.sh alloc`) est **indépendante** → divergence cloud/VM possible, non détectée. |
| **Aucune API cloud → VM** | Aucune création/révocation de peer pilotée depuis la console. Le backend tourne dans un conteneur distroless sur la MÊME VM — il ne peut pas sudo ni toucher `/etc/wireguard`. |
| **Pas de couche produit** | Pas de modèle `VpnPeer`, pas de SKU VPN dans `SaasPlans` (`tenant.go:198-219`), pas de livraison .conf/QR depuis la console, pas de facturation, pas de suspension par usage. |

### 3.3 Architecture cible proposée — D3, D4, D5 à trancher

**D3 — Automatisation cloud ↔ VM (le point §8 de ce chantier).**

- *Option recommandée* : **mini-service hôte** (conventions §8 : unité systemd
  root dédiée, bind **127.0.0.1:4020** — port à réserver au registre,
  `MemoryMax`, secret HMAC dans `/etc/...env` 600, secrets hors logs) qui
  expose `add/remove/qr/list` de `wg-peer.sh` au backend (même machine, pas de
  traversée réseau publique). C'est un **changement d'infrastructure §8** (nouveau
  port réservé + unité root) → **requiert ta validation explicite** — c'est
  précisément le genre de décision que le kit N°287 documente.
- *Option minimale* : flux « pilotote » — la console génère la demande de peer,
  l'exploitant SSH exécute `wg-peer.sh`, colle le résultat. Zéro nouvelle brique,
  mais ruine la promesse « .conf/QR livrés automatiquement » et ne scale pas.

**D4 — Isolation (recommandé : wg0 partitionné en v1, wg1 plus tard).**
Un seul wg0 avec convention de nommage (`router-*` vs `vpn-*`) + le registre
cloud `VpnPeer` comme source de vérité (réconciliation cloud↔VM via
`wg-peer.sh list` = nouveau type de check). Un wg1 dédié clients (51821/udp +
security list) ne se justifie qu'au volume — pas maintenant, pas de changement
de security list en v1.

**D5 — Clés et confs (recommandé : chiffré au repos, re-livraison possible).**
Clé privée client générée côté VM (pattern actuel), conf stockée **chiffrée
secretbox** (pattern `WgPSK` déjà en place) pour permettre re-affichage QR et
re-livraison Mail/Telegram (déjà implémentés) puis WhatsApp (en cours) — c'est
l'argument produit même face au « .conf auto SMS/email » de PhenixSPOT.
L'alternative show-once (livret N°285) est plus parano mais casse la re-livraison.

**Garde-fous P-B** : plafond de slots par plan, monitor egress mensuel (alerte
notify existante), ToS/abus, révocation = `remove` + syncconf, taux de
renouvellement suivi dans les journaux mensuels.

### 3.4 Effort

**Moyen** — le plus court chemin vers « gère ET revend » : helper VM ≈ 1 sem ;
modèle `VpnPeer` + API + console VPN (liste/création/révocation/QR) ≈ 1–2 sem ;
livraison + facturation (wallet/recurring patterns existants) ≈ 1 sem. Total
≈ **2–4 semaines**, dont 1 conditionnée à D3.

---

## 4. Chantier ⑧ — Cybercafé

### 4.1 L'enjeu commercial

- Niche **mais présente dans chaque appel d'offres local** et **verrou de plan**
  chez PhenixSPOT (Cybercafé 20 PCs dès Pro 25 000, illimité ISP) — sans lui,
  la grille de comparaison nous déclasse avant même la démo.
- C'est aussi notre propre réalité client : la segmentation historique
  (Hôtels · Cybercafés · Maquis) est dans nos commentaires et nos pilotes
  (« CYBER ESPACE SC », CHANGELOG).
- **Découverte pivot de l'analyse : Cybercafé et MAC Access (P0 item 4) sont LE
  MÊME substrat.** « Abonnement par appareil sans login » = un poste ; le module
  cybercafé = ce substrat + codes-temps + caisse. **Un chantier ferme deux écarts
  de la matrice** (Cybercafé ✅ et MAC Access ✅) — l'arbitrage N°286 les listait
  séparément (P0-4 et P1-8) ; le code montre qu'il faut les construire ensemble.

### 4.2 Ce que le code dit (fait vérifié)

| Brique | État |
|---|---|
| **Codes-temps** | Existent déjà : voucher `TimeLimitMin` → `limit-uptime` sur le routeur (`agent/users.go:51-90`), validité ancrée au 1er login, extension F4, impression tickets prédécoupés A4 + QR (`uc-print-dialog.tsx` — « uc » = *user credentials* : impression/masquage/clipboard des codes), batches 1–500. |
| **Poste** | **N'existe pas.** `Device` (registre DHCP, identité MAC stable) est **HomeNet seulement** (`handlers_devices.go:9-12`) ; `device_pause` (coupe par MAC via firewall, marqueur `mikcloud-pause`, IPv4+IPv6 idempotent) est **HomeNet seulement** (`agent/devicepause.go`). Les consoles hotspot n'ont aucun inventaire de postes. |
| **Contrôle MAC côté hotspot** | `ip-binding` bypassed/blocked disponible et outillé (F7 `bindings-tab.tsx`, CRUD `handlers_ipbindings.go`) ; `LockFirstDevice` sur les profils = verrou MAC au premier login, MAC mémorisé dans le commentaire routeur sous `mikcloud_lock:` (`models.go:436-441`). |
| **Allocation instantanée « un code maintenant »** | Pattern kiosk des liens join (`autoValidate` → création immédiate, `handlers_join.go:439-473`) et dispensation de ticket WiFi jetable (`handlers_wifi.go:232+`) — deux flux prêts à recycler pour « poste alloué → code-temps ». |
| **Caisse** | Transactions vente/crédit/dette/règlement, consignation revendeurs, comptabilité par période avec marge (`handlers_accounting.go:113-345`), **journaux mensuels gelés** (`MonthlyJournals`) — la mémoire comptable d'un cybercafé existe déjà. |
| **Usage enum** | `hotspot | homenet` UNIQUEMENT (`tenant.go:171-178`) — « cybercafe » est même une valeur **de test invalide** (`usage_guard_test.go:96`). |

### 4.3 Architecture cible proposée — D6 à trancher

**D6 — Forme du module (recommandé : flag, pas de nouvel usage).**
Module **activable par compte** (`CyberEnabled`, conditionné au plan — le « plan
Pro » du doc N°286) **sans toucher à l'usage enum** : ripple minimal (pas de
migration plans/guards/tests). L'alternative usage `'cyber'` à part entière est
plus « propre » conceptuellement mais fait onduler tout le segment billing.

**v1 = overlay du mode hotspot** (pas un nouveau monde) :

1. **Registre Postes** : découverte des MAC (DHCP leases + `read_hosts` déjà en
   agent), nommage manuel, OUI vendor déjà disponible ; par site (l'entité `Site`
   = établissement existe).
2. **Attribution code-temps ↔ poste** : génère un voucher temps et le lie au
   poste (`LockFirstDevice` ou binding bypass filaire) ; flux « allouer au poste
   X » inspiré du kiosk.
3. **Pause/reprise poste** : généraliser `device_pause` (firewall par MAC) aux
   comptes hotspot — petite ouverture du garde HomeNet-only.
4. **Impression & caisse** : tickets postes via `uc-print`, ventes du jour,
   Z mensuel via `MonthlyJournals`.

### 4.4 Effort

**Le plus léger des trois** — ≈ **2–3 semaines** : la plupart des briques
existent ; il s'agit d'assemblage (registre + attribution + pause + caisse +
habillage plan), pas d'invention.

---

## 5. Matrice d'arbitrage des trois chantiers

| | ⑥ PPPoE | ⑦ WG vendable | ⑧ Cybercafé |
|---|---|---|---|
| **Valeur marché** | ★★★ — segment ISP/WISP récurrent, la plus grosse case rouge de la matrice | ★★ — différenciateur unique (« gère ET revend »), réponse directe PhenixSPOT | ★★ — plan-gate + **offre MAC Access (P0-4) en bonus** |
| **Effort** | ●●● Phase A ≈ 4–6 sem (vertical neuf) | ●● ≈ 2–4 sem (1 sem conditionnée à D3) | ● ≈ 2–3 sem (assemblage) |
| **Risque** | Moyen — parité cloud↔routeur à upgrader, volume de scripts | Moyen — changement §8 (D3), abus egress P-B, divergence cloud/VM slots | Faible — overlay, briques éprouvées |
| **Dépendances** | Aucune (agent). Tunnel = phase B opt-in | D3 (mini-service §8), D4/D5 ; ToS pour P-B | Aucune |
| **Déverrouille** | Abonnés mensuels, renfort tunnel temps réel, RADIUS futur | Accès distant gérant (P-A gratuit), livraison auto .conf/QR | MAC Access produit, argument immédiat face aux plans Pro/ISP |

---

## 6. Ordre proposé — D7 à trancher

- **Option R « revenu d'abord »** : ⑥ Phase A → ⑦ → ⑧. On attaque le plus gros
  segment récurrent immédiatement ; Cybercafé arrive en dernier.
- **Option V « vitesse d'abord » (recommandée)** : ⑧ → ⑦ → ⑥ Phase A.
  Raisons : ⑧ est presque gratuit à construire et ferme **deux** lignes de la
  matrice (Cybercafé + MAC Access) — argument commercial immédiat contre les
  plans Pro/ISP de PhenixSPOT pendant qu'on prépare le gros morceau ; ⑦ bénéficie
  du temps laissé pour arbitrer proprement le changement §8 (D3) ; ⑥ Phase A
  démarre ensuite avec un plan de lots clair et l'expérience des deux chantiers
  précédents (patterns de parité consolidés).

Les deux options sont honnêtes — le choix dépend de ce qui presse le plus :
**un segment de revenu** (R) ou **une grille commerciale complète** (V).

---

## 7. Les 7 décisions à trancher ensemble

| # | Décision | Options | Recommandation |
|---|---|---|---|
| **D1** | Transport PPPoE canonique | (a) commandes agent `ppp_*` · (b) API à travers wg0 avec creds stockées | **(a)** agent — CGNAT-proof, zéro cred ; tunnel en renfort phase B |
| **D2** | Périmètre v1 PPPoE | (a) gérer secrets/profils/sessions d'un serveur existant · (b) + provisioning pppoe-server | **(a)** gérer seulement ; provisioning assisté en phase B |
| **D3** | ⑦ Automatisation cloud↔VM | (a) mini-service hôte §8 127.0.0.1:4020 (unité root, HMAC) · (b) flux pilotote SSH manuel | **(a)** — mais c'est un changement §8 (port + unité root) → **validation explicite requise** |
| **D4** | ⑦ Isolation peers | (a) wg0 partitionné (nommage + registre cloud source de vérité) · (b) wg1 dédié clients dès v1 | **(a)** — pas de security list en v1 ; wg1 au volume |
| **D5** | ⑦ Stockage des confs clients | (a) chiffré au repos (secretbox) avec re-livraison · (b) show-once livret | **(a)** — la re-livraison (Mail/Telegram/WhatsApp) est l'argument produit |
| **D6** | ⑧ Forme du module | (a) flag `CyberEnabled` par compte (plan Pro) · (b) usage `'cyber'` à part entière | **(a)** — ripple minimal sur plans/guards/tests |
| **D7** | Ordre des chantiers | Option R (⑥→⑦→⑧) · Option V (⑧→⑦→⑥) | **V** — deux écarts fermés vite, §8 arbitré à froid, puis le gros morceau |

Une fois D1–D7 arbitrées, le chantier retenu reçoit sa décomposition en lots
(numérotés, testables, déployables) et entre en exécution.

---

## 8. Sources

- **Code MikCloud** (vérifié 2026-10-10) :
  `backend/internal/routeros/{protocol,gateway}.go` (client binaire 8728,
  `Run/Exec`, Gateway real/simulé) ; `backend/internal/agent/{agent,users,profiles,
  quota,wireguard,devicepause,ipbindings,readstate,queue}.go` (44 commandes,
  builders, WG, pause, pagination) ; `backend/internal/api/{agent_handlers,
  agent_queue,agent_results,handlers_wg,handlers_ipbindings,handlers_devices,
  handlers_users_ops,handlers_vouchers,handlers_wifi,handlers_join,
  handlers_accounting,handlers_subscription,handlers_billing}.go` ;
  `backend/internal/model/{models,tenant,billing,devices,join,wifi,
  monthlyjournal,security}.go` ; `backend/internal/hotpage/templatize.go` ;
  `frontend/src/components/hotspot/parts/{uc-print-dialog,uc-password-cell,
  uc-clipboard,router-wg-card,bindings-tab}.tsx` ; `deploy/oracle/wg-peer.sh`.
- **Docs** : `docs/COMPARATIF-PHENIXSPOT.md` (N°286, feuille P1 ⑥⑦⑧),
  `docs/CARTE-CONCURRENCE.md` (N°288, matrice + menaces),
  `docs/HANDOFF-INSTALL-BACKEND.md` (N°287, conventions §8), CHANGELOG
  N°279–285 (Docker Phase B, WG N°284/285, leçon N°282).
- **Arbitrage exploitant** (ce document, préambule) : Wave direct = P0 argent
  soldé ; SMS reporté ; WhatsApp en cours prioritaire ; Mail/Telegram livrés.
