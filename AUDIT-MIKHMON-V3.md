# Audit MikCloud vs Mikhmon v3 (laksa19/mikhmonv3)

> Audit comparatif réalisé sur le code source de Mikhmon v3.20 (dernière version, juin 2021).
> Objectif : identifier les écarts fonctionnels et les axes d'amélioration pour MikCloud.

---

## 1. Rappel du contexte

| | Mikhmon v3 | MikCloud |
|---|---|---|
| Architecture | PHP monolithique auto-hébergé (Termux, VPS, Docker) | SaaS cloud (Next.js + Go + PostgreSQL) |
| Modèle | Mono-utilisateur, session PHP, un routeur actif à la fois | Multi-tenant, JWT, rôles, multi-routeurs natif |
| Accès routeur | RouterOS API **entrante** (port 8728, IP publique requise) | 3 modes : simulé, API directe, **agent sortant** (NAT-friendly) |
| État projet | Abandonné (dernier update 2021, GPL v2) | Actif |

**MikCloud corrige déjà les faiblesses structurelles de Mikhmon** : multi-tenant,
multi-routeurs simultanés, revendeurs avec portefeuille, comptabilité, paiements
Wave, sécurité (bcrypt, JWT, rate-limit), données persistées dans le cloud,
accès routeur derrière NAT via agent.

---

## 2. Matrice des fonctionnalités — écarts détaillés

Légende : ✅ présent · ⚠️ partiel · ❌ absent

### 2.1 Utilisateurs hotspot

| Fonctionnalité | Mikhmon v3 | MikCloud | Écart |
|---|---|---|---|
| Liste, recherche, filtre par profil | ✅ | ✅ | — |
| CRUD utilisateur | ✅ | ✅ | — |
| Enable / disable | ✅ | ✅ | — |
| **Reset statistiques utilisateur** | ✅ (`resethotspotuser`) | ❌ | À ajouter |
| **Prolonger la date d'expiration** | ✅ (`extend expired date`) | ❌ | À ajouter |
| **Suppression des expirés en masse** | ✅ (`removeexpiredhotspotuser`) | ❌ | À ajouter |
| **Suppression par commentaire** | ✅ | ❌ | À ajouter (niche) |
| **Export CSV de la liste utilisateurs** | ✅ (`exportusers.php`) | ❌ | À ajouter — demande récurrente |

### 2.2 Génération de vouchers

| Fonctionnalité | Mikhmon v3 | MikCloud | Écart |
|---|---|---|---|
| Quantité, préfixe, longueur code | ✅ | ✅ | — |
| Profil + routeur cible | ✅ | ✅ | — |
| **Choix du jeu de caractères** (`char`) | ✅ | ❌ | À ajouter |
| **Limite de données à la génération** (`datalimit` + Mo/Go) | ✅ | ⚠️ (quota au niveau profil uniquement) | Surcharge par voucher |
| **Limite de temps à la génération** (`timelimit`) | ✅ | ⚠️ (timeout au niveau profil) | Surcharge par voucher |
| **Commentaire personnalisé** (`adcomment`) | ✅ | ❌ | À ajouter |
| Attribution à un revendeur | ❌ | ✅ | Avantage MikCloud |
| Lots tracés avec statistiques | ❌ | ✅ | Avantage MikCloud |

### 2.3 Profils (user profiles)

| Fonctionnalité | Mikhmon v3 | MikCloud | Écart |
|---|---|---|---|
| Nom, rate-limit, utilisateurs simultanés, validité, prix | ✅ | ✅ | — |
| Quota data au niveau profil | ❌ | ✅ | Avantage MikCloud |
| **Prix de vente distinct du prix coût** (`sprice`) | ✅ | ❌ | À ajouter (calcul de marge) |
| **Mode d'expiration** : remove / remove+record / notify / notify+record (`expmode`) | ✅ | ❌ | **Écart majeur** — voir §3.1 |
| **Période de grâce** (`graceperiod`) | ✅ | ❌ | À ajouter |
| **Verrouiller / déverrouiller** à l'expiration (`lockunlock`) | ✅ | ❌ | À ajouter |
| **File parent** (parent queue) | ✅ | ❌ | Avancé — QoS |
| **Address pool** dédié | ✅ | ❌ | Avancé |
| Scripts `on-login` + scheduler « monitor profile » poussés dans le routeur | ✅ (généré auto) | ❌ (géré côté cloud via agent — plus propre) | Alternative cloud à concevoir |

### 2.4 Impression des vouchers

| Fonctionnalité | Mikhmon v3 | MikCloud | Écart |
|---|---|---|---|
| Impression avec QR code | ✅ | ✅ | — |
| Choix du nombre de colonnes | ✅ | ✅ | — |
| **Éditeur de template intégré** (`vouchereditor.php`) | ✅ | ❌ | **Écart majeur** — voir §3.2 |
| **Variables dynamiques** : `$username, $password, $profile, $validity, $price, $datalimit, $timelimit, $qrcode, $logo, $hotspotname, $dnsname, $num, $comment, $usermode` | ✅ | ❌ | À ajouter |
| **Upload de logo** | ✅ | ❌ | À ajouter |
| **Formats thermiques 58 mm / 80 mm** | ✅ | ❌ | À ajouter (imprimantes tickets) |
| **Aperçu avant impression** (`vpreview`) | ✅ | ❌ | À ajouter |
| **Quick print** (réimprimer le dernier lot en 1 clic) | ✅ | ⚠️ (via liste des lots) | Raccourci à ajouter |
| **Impression Bluetooth** (printbt, Android) | ✅ | ❌ | Mobile — plus tard |

### 2.5 Sessions & supervision

| Fonctionnalité | Mikhmon v3 | MikCloud | Écart |
|---|---|---|---|
| Sessions actives + déconnexion (kick) | ✅ | ✅ | — |
| **Journal utilisateur** login/logout (`userlog`) | ✅ | ❌ | **À ajouter** — précieux pour le support |
| **Live report auto-rafraîchi** (`livereport`) | ✅ | ❌ | À ajouter (poll/WebSocket) |
| **Moniteur de trafic temps réel par interface** (rx/tx graph) | ✅ | ❌ | **Écart majeur** — voir §3.3 |
| Dashboard ressources (CPU, RAM, uptime, version) | ✅ | ✅ | — |
| **Board name, disque libre (free HDD)** | ✅ | ❌ | Compléter le status |
| **Ping test d'une IP depuis le routeur** | ✅ | ⚠️ (test connexion API uniquement) | À ajouter |

### 2.6 Fonctions RouterOS absentes de MikCloud

| Fonctionnalité | Mikhmon v3 | MikCloud |
|---|---|---|
| **IP Bindings** (bypass MAC — clients illimités) | ✅ add/remove/enable/disable | ❌ |
| **Hotspot Hosts** (table des hôtes) | ✅ | ❌ |
| **Hotspot Cookies** (sessions persistées) | ✅ | ❌ |
| **Hotspot Log** (journal du routeur) | ✅ | ❌ |
| **DHCP Leases** (baux DHCP) | ✅ | ❌ |
| **Scheduler** (tâches planifiées du routeur) | ✅ list/enable/disable/remove | ❌ |
| **Reboot / Shutdown du routeur** à distance | ✅ | ❌ |
| **PPP** : secrets, profils, sessions actives | ✅ | ❌ — voir §3.4 |

### 2.7 Rapports & comptabilité

| Fonctionnalité | Mikhmon v3 | MikCloud | Écart |
|---|---|---|---|
| Ventes par jour/mois/année | ✅ | ✅ (day/week/month) | — |
| Ventes par profil + total | ✅ | ✅ | — |
| **Rapport résumé imprimable** (`resumereport`) | ✅ | ❌ | Export PDF à ajouter |
| Export fichier (download) | ✅ | ⚠️ CSV comptabilité uniquement | Étendre aux ventes/utilisateurs |
| Comptabilité multi-sites avec part par site | ❌ | ✅ | Avantage MikCloud |
| Ventes par revendeur | ❌ | ✅ | Avantage MikCloud |
| Transactions portefeuille revendeurs | ❌ | ✅ | Avantage MikCloud |

### 2.8 Paramètres & personnalisation

| Fonctionnalité | Mikhmon v3 | MikCloud | Écart |
|---|---|---|---|
| Devise, nom du hotspot | ✅ | ✅ (tenant) | — |
| **Nom DNS du hotspot** (affiché voucher) | ✅ | ❌ | À ajouter |
| **Idle timeout (déconnexion auto)** | ✅ | ❌ | À ajouter (config par tenant) |
| **Multi-langue** : en, es, id, tl, tr | ✅ | ❌ (FR uniquement) | i18n — voir §3.5 |
| Thème clair/sombre | ✅ | ⚠️ (standard shadcn) | Vérifier/exposer |
| Paiement mobile (Wave CI) | ❌ | ✅ | Avantage MikCloud |

---

## 3. Les 5 écarts majeurs (analyse approfondie)

### 3.1 Le mode d'expiration (`expmode`) — le cœur opérationnel de Mikhmon

C'est la fonctionnalité la plus utilisée de Mikhmon et elle est invisible dans
l'interface : à la création d'un profil, Mikhmon pousse dans le routeur un
**script `on-login`** et un **scheduler « monitor profile »** qui, à chaque
connexion d'un utilisateur :
- enregistre son profil et sa date d'expiration (dans un commentaire) ;
- puis, selon `expmode` :
  - `rem` / `remc` : **supprime** l'utilisateur expiré (avec ou sans trace) ;
  - `ntf` / `ntfc` : **notifie** (le garde actif mais marqué) ;
- `graceperiod` : tolère un dépassement avant application ;
- `lockunlock` : re-verrouille un compte partagé à chaque login.

**Traduction cloud pour MikCloud** : là où Mikhmon embrouille le routeur de
scripts fragiles (source de la moitié des issues GitHub), l'agent MikCloud peut
gérer l'expiration **côté serveur** :
1. ajouter `expMode` (remove|notify), `gracePeriodMin`, `lockUser` au modèle Profil ;
2. au check-in agent : le cloud renvoie la liste des utilisateurs à expirer
   (statut → `expired`, suppression conditionnelle du routeur) ;
3. journaliser chaque expiration dans l'activité + future notification
   (e-mail/SMS/WhatsApp).

### 3.2 L'éditeur de templates de vouchers

Chez Mikhmon, c'est un éditeur HTML live (`vouchereditor.php`) avec variables
`$username $password $profile $validity $price $sprice $datalimit $timelimit
$qrcode $logo $hotspotname $dnsname $num $comment $usermode`, plusieurs gabarits
(défaut, compact, thermique 58/80 mm), upload de logo et aperçu.

**Recommandation MikCloud** : un éditeur de template (stocké en base, par tenant)
avec les mêmes variables + QR code, aperçu live en iframe/React, formats
A4 (N colonnes) et thermique 58/80 mm (CSS `@page`), et par défaut une
reproduction fidèle des 3 gabarits Mikhmon pour faciliter la migration des
opérateurs existants (argument commercial fort).

### 3.3 Moniteur de trafic temps réel

Mikhmon affiche un graphique rx/tx de n'importe quelle interface du routeur.
C'est l'outil quotidien de l'opérateur pour voir « qui consomme la bande
passante ».

**Traduction cloud** : étendre le protocole agent — le script routeur peut
pousser `/interface/monitor-traffic` toutes les X secondes lors du check-in,
et MikCloud affiche un graphique Recharts temps réel (poll 5 s ou WebSocket).
Ajouter le classement des sessions par consommation (bytes in/out déjà
collectés) — Mikhmon ne l'a même pas : avantage possible.

### 3.4 PPP (secrets / profils / actifs)

Mikhmon gère aussi les abonnés **PPPoE** — au-delà du hotspot. Beaucoup
d'opérateurs africains vendent hotspot ET PPPoE avec la même infrastructure.
Réutiliser l'architecture agent pour exposer : PPP secrets (CRUD), PPP profiles,
sessions PPP actives. Ouvre le marché FAI/WISP, pas seulement WiFi public.

### 3.5 Internationalisation

Mikhmon livre 5 langues (en, es, id, tagalog, turc) — l'audience est mondiale.
MikCloud est francophone. Un i18n (`next-intl`) avec FR par défaut + EN/ES
minimum élargit considérablement le marché SaaS.

---

## 4. Ce que MikCloud a déjà en mieux (à valoriser)

1. **Multi-tenant SaaS réel** — comptes, plans, admin plateforme (Mikhmon : 1 admin).
2. **Accès routeur derrière NAT** via agent sortant — Mikhmon exige une IP publique et un port 8728 ouvert (risque sécurité majeur, source d'échecs).
3. **Revendeurs avec portefeuille** + transactions + débit automatique — inexistant chez Mikhmon.
4. **Comptabilité multi-sites** + export CSV comptable.
5. **Paiements Wave CI** intégrés.
6. **Sécurité** : bcrypt, JWT, rate-limit login, token agent haché (Mikhmon : mot de passe routeur en session PHP claire, un seul compte).
7. **Journal d'activité** global.
8. **Haute disponibilité** cloud (Vercel/Render/Neon) vs auto-hébergement fragile.
9. **Supervision multi-routeurs simultanée** avec statut online/offline.

---

## 5. Feuille de route recommandée

### P0 — Parité cœur métier (à faire en premier)
| # | Fonctionnalité | Effort | Impact |
|---|---|---|---|
| 1 | Modes d'expiration (expmode + grâce + verrouillage) pilotés par l'agent | M | ⭐⭐⭐ |
| 2 | Éditeur de templates voucher (variables, logo, thermique 58/80, aperçu) | M | ⭐⭐⭐ |
| 3 | Journal utilisateur login/logout (user log) | S | ⭐⭐⭐ |
| 4 | Export CSV utilisateurs + reset stats + prolonger expiration | S | ⭐⭐⭐ |
| 5 | Nettoyage automatique des expirés (politique par tenant) | S | ⭐⭐⭐ |

### P1 — Supervision routeur complète
| # | Fonctionnalité | Effort | Impact |
|---|---|---|---|
| 6 | Moniteur trafic temps réel par interface | M | ⭐⭐⭐ |
| 7 | IP Bindings (bypass MAC) | S | ⭐⭐ |
| 8 | Status étendu (board, HDD) + ping test | S | ⭐⭐ |
| 9 | DHCP leases + hotspot hosts/cookies/log | S | ⭐⭐ |
| 10 | Scheduler routeur + reboot/shutdown distant | S | ⭐⭐ |

### P2 — Écosystème et marché
| # | Fonctionnalité | Effort | Impact |
|---|---|---|---|
| 11 | Gestion PPPoE (secrets, profils, sessions) | M | ⭐⭐⭐ (marché FAI) |
| 12 | i18n FR/EN/ES (+ arabe ?) | M | ⭐⭐⭐ |
| 13 | Quick print + impression Bluetooth mobile | M | ⭐⭐ |
| 14 | Prix de vente vs prix coût + rapport de marge | S | ⭐⭐ |
| 15 | Options de génération avancées (charset, data/time limit par voucher, commentaire) | S | ⭐⭐ |
| 16 | Rapport résumé PDF imprimable | S | ⭐⭐ |

Légende effort : S = petit (≤ 1 jour) · M = moyen (2–4 jours).

---

## 6. Notes d'implémentation MikCloud

- **Expiration (P0-1)** : le protocole agent supporte déjà des commandes
  bidirectionnelles (`/agent/cmd` → `/agent/result`). Ajouter un type de
  tâche `expire` : le serveur calcule les utilisateurs à expirer depuis la
  base (source de vérité cloud) et envoie au routeur les commandes
  `/ip/hotspot/user/remove` ou `set disabled=yes`. Aucun script à pousser
  dans le routeur — plus robuste que le « monitor profile » Mikhmon.
- **Templates (P0-2)** : table `voucher_templates` (tenant_id, name, format
  a4|58mm|80mm, body HTML avec `{{variables}}`, logo URL). Rendu côté client
  (React) pour l'aperçu, impression via CSS `@page` + `window.print()`.
- **User log (P0-3)** : l'agent remonte déjà les sessions ; ajouter la
  persistance des événements login/logout (table `user_logs`) — l'info existe
  déjà dans les flux check-in.
- **Trafic temps réel (P1-6)** : ajouter au check-in agent un champ
  `interfaces[] {name, rx, tx}` ; poll 5 s depuis le front (ou WebSocket
  existant) → Recharts AreaChart.
- **PPP (P2-11)** : réutiliser le gateway RouterOS (`internal/routeros`) —
  les commandes PPP (`/ppp/secret/print` etc.) suivent le même protocole API.
