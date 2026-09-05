# RUNBOOK — Walled-garden MikroTik & inscriptions publiques par QR (N°27)

> Document opérateur MikCloud. Objectif : rendre la page d'inscription
> publique `/join/{token}` accessible **depuis le WiFi du hotspot, avant
> toute authentification**, pour que le scan du QR fonctionne sur place
> (campus, écoles, administrations, entreprises). Dernière mise à jour :
> N°27-D (2026-09-05).

## 0. Quand ce runbook est nécessaire

| Situation | Le QR fonctionne-t-il ? | Action |
|---|---|---|
| Le visiteur scanne l'affiche en **4G / data mobile** (non connecté au WiFi) | ✅ immédiatement, aucune configuration | Rien à faire |
| Le visiteur est **connecté au WiFi du hotspot** (ouvert, non authentifié) | ❌ sans configuration : le portail captif intercepte tout le trafic | **Appliquer ce runbook** |
| Kiosque (tablette déjà authentifiée, poste filaire) | ✅ | Rien à faire (mode « validation automatique » N°27) |

Le portail captif MikroTik bloque **tout** le trafic des clients non
authentifiés. Or la page d'inscription vit sur Internet (Vercel) et appelle
l'API (Render) : sans liste blanche, le scan du QR depuis le WiFi local
aboutit à une page injoignable — alors que le visiteur visé est précisément
la personne qui n'a **pas encore** de compte pour se connecter au portail.

**Le walled-garden** est la liste blanche du hotspot : les destinations qui
y figurent restent joignables SANS authentification. On y ajoute donc
exactement les deux domaines nécessaires à l'inscription — et rien d'autre :
le reste du Web reste derrière le portail.

## 1. Les domaines à autoriser (production)

| Domaine | Rôle | Appelé quand |
|---|---|---|
| `mikcloud.ftci.fr` | Page publique `/join/{token}` — c'est l'URL encodée dans le QR | Ouverture du lien (scan ou affiche) |
| `mikcloud.onrender.com` | API backend — `GET/POST /api/join/{token}` | Chargement du lien + soumission du formulaire |

> Ces deux domaines correspondent au mode « direct » documenté dans
> `frontend/README.md` : le navigateur appelle l'API Render directement
> (`NEXT_PUBLIC_API_BASE=https://mikcloud.onrender.com`). Si vous avez suivi
> `docs/RUNBOOK-SECRETS.md` §5 (sous-domaine API `api.<domaine>` proxifié
> Cloudflare), autorisez `api.<domaine>` **à la place** de
> `mikcloud.onrender.com`.
>
> Aucun domaine tiers n'est requis : les polices sont auto-hébergées
> (`next/font`, servies par `mikcloud.ftci.fr` elle-même) et la page
> `/join` ne charge aucune ressource externe.

## 2. Procédure CLI RouterOS (v6.44+ et v7 — syntaxe identique)

```routeros
/ip hotspot walled-garden
add action=allow dst-host=mikcloud.ftci.fr comment="N27 inscription publique (page QR)"
add action=allow dst-host=mikcloud.onrender.com comment="N27 inscription publique (API)"

# Robustesse : clients avec DNS codé en dur (8.8.8.8, etc.) —
# la résolution DNS doit traverser le routeur pour que les règles
# par domaine fonctionnent (le hotspot « renifle » les réponses DNS).
/ip hotspot walled-garden ip
add action=allow protocol=udp dst-port=53 comment="DNS pour regles par domaine"
add action=allow protocol=tcp dst-port=53 comment="DNS TCP (reponses tronquees)"
```

Vérification immédiate :

```routeros
/ip hotspot walled-garden print
/ip hotspot walled-garden ip print
```

Les règles sont actives immédiatement — aucun redémarrage du hotspot.

### Comment ça marche (et pourquoi HTTPS est couvert)

- Les règles **par domaine** de la table `walled-garden` ne lisent pas le
  trafic chiffré : le hotspot intercepte les **requêtes DNS** du client ;
  quand un nom autorisé est résolu, l'adresse IP correspondante est ajoutée
  à une liste blanche dynamique. HTTP **et** HTTPS passent donc.
- Conséquence : le client doit résoudre via un DNS **visible du routeur**
  (le DNS du hotspot par défaut, ou tout DNS en clair si le port 53 est
  autorisé). Voir §5 pour le cas « DNS chiffré ».
- Ne pas épingler d'adresses IP (`walled-garden ip dst-address=…`) pour
  Vercel/Render : leurs IP sont anycast et **évoluent** — seul le nom est
  stable.

## 3. Procédure WinBox (sans CLI)

1. WinBox → **IP → Hotspot**.
2. Bouton **Walled Garden** (table par domaine) → `+` :
   - `Dst. Host` : `mikcloud.ftci.fr` — `Action` : `allow`
   - refaire avec `mikcloud.onrender.com`
3. Bouton **Walled Garden IP** → `+` :
   - `Protocol` : `udp`, `Dst. Port` : `53`, `Action` : `allow`
   - refaire en `tcp`
4. `Apply` — actif immédiatement.

## 4. Vérification de bout en bout (appareil témoin non authentifié)

1. Joindre le SSID du hotspot **sans se connecter** — fermer la fenêtre du
   portail captif qui s'ouvre automatiquement (comportement normal
   iOS/Android).
2. Scanner le QR de l'affiche (appareil photo) **ou** ouvrir
   `https://mikcloud.ftci.fr/join/<token>` — la page doit afficher
   l'en-tête de l'organisation et le formulaire.
3. Soumettre une demande de test → écran « Demande envoyée ! ».
4. Console → **Inscriptions → Demandes** : valider la demande (profil,
   identifiants) — le compte hotspot est créé.
5. Sur l'appareil témoin, se connecter au portail captif avec les
   identifiants (Nom d'utilisateur **et** Mot de passe — mode N°27) :
   la session s'ouvre.

Si l'étape 2 échoue → §5.

## 5. Dépannage

| Symptôme | Cause probable | Remède |
|---|---|---|
| La page ne charge pas depuis le WiFi (mais OK en 4G) | Règle manquante ou faute de frappe (`dst-host` doit être le nom **exact**, sans protocole ni slash) | `walled-garden print` ; comparer caractère par caractère |
| « Impossible de trouver le serveur » (résolution DNS) | Client avec DNS codé en dur, port 53 non autorisé | Ajouter les règles 53 udp/tcp (§2) |
| Page injoignable alors que le DNS fonctionne | Navigateur configuré en **DNS chiffré** (DoH : « Secure DNS » Chrome/Edge/Firefox, « DNS privé » Android) — le routeur ne voit plus les requêtes, la règle par domaine ne peut plus correspondre | Désactiver « Secure DNS » / DNS privé sur l'appareil ; en durcissement avancé, bloquer le DoH externe et forcer le DNS du hotspot |
| Le scan ouvre le portail captif au lieu du lien | Comportement normal : l'OS ouvre le portail à la connexion du WiFi | Fermer le portail, scanner depuis l'appareil photo (ou saisir l'URL) — le lien s'ouvre alors dans le navigateur normal |
| Erreur TLS sur la page | Horloge de l'appareil fausse, ou proxy HTTP explicite configuré sur le hotspot pour ces domaines | Horloge correcte ; ne jamais placer de proxy explicite pour les domaines du walled-garden |
| La page s'ouvre mais affiche « Lien introuvable » / verrouillé | Ce n'est **pas** un problème de walled-garden : lien révoqué, expiré ou saturé côté serveur | Console → Inscriptions → Liens & QR : vérifier le badge d'état du lien |

## 6. Périmètre de sécurité (ce que le walled-garden ouvre — et n'ouvre pas)

- Seuls `mikcloud.ftci.fr` et `mikcloud.onrender.com` (ou l'homologue API
  personnalisé) sont joignables avant authentification ; tout le reste du
  Web reste derrière le portail.
- Côté serveur, cette exposition est déjà cadrée (N°27) : whitelist
  publique limitée à `/api/join/{token}`, rate-limit dédié (10/min/IP),
  quota anti-abus par IP (5/10 min, 20/24 h), honeypot à succès factice,
  GET public minimal, révocation instantanée des liens depuis la console.
- **Anti-patterns** :
  - autoriser `*.onrender.com`, `*.vercel.app` ou `*.ftci.fr` (ouvrirait
    d'autres services que MikCloud) ;
  - ouvrir 80/443 en général via `walled-garden ip` (annule le portail) ;
  - épingler des IP Vercel/Render (elles changent) ;
  - utiliser la règle `dst-host` dans la table **`walled-garden ip`**
    pour ces domaines (elle ne matche que l'en-tête HTTP — inopérant en
    HTTPS) : utiliser la table **par domaine** (§2).

## 7. Rappel du flux complet (N°27)

1. Console → **Inscriptions → Liens & QR → Nouveau lien** (nom, profil et
   routeur pré-attribués optionnels, limite d'usages, expiration, kiosque
   opt-in) → le QR s'affiche sur la carte du lien.
2. **Affiche A4** imprimable (QR géant + URL + 3 étapes + rappel du mode
   de connexion) à poser sur place : accueil, halls, salles.
3. Le visiteur scanne (WiFi local avec ce runbook, ou 4G) → formulaire
   `/join/{token}` (nom, téléphone, identifiant, mot de passe ×2) →
   « Demande envoyée ! ».
4. Le gérant valide dans **Inscriptions → Demandes** (profil, routeur,
   identifiants éditables) — le compte hotspot est créé par le même cœur
   que la console, la validité démarre à l'APPROBATION.
5. Le visiteur se connecte au portail avec **Nom d'utilisateur + Mot de
   passe** (deux codes distincts — différent des vouchers « nom =
   mot de passe », N°25).

Références : `docs/CONTRACT-V2.md` (section N°27), `frontend/README.md`
(mode direct Vercel→Render), `docs/RUNBOOK-SECRETS.md` §5 (API proxifiée
Cloudflare).
