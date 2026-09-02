# Guide du gérant — MikCloud (ProMax Wifi)

Bienvenue ! Ce guide vous accompagne pour gérer votre hotspot Wi-Fi **ProMax Wifi —
Freelance Technologies (FTCI)** depuis n'importe quel téléphone ou ordinateur.

---

## 1. Connecter un routeur MikroTik en 30 secondes (la « facilité déconcertante »)

Aucune adresse IP publique, aucun port-forward, aucun identifiant à stocker :
le routeur se connecte **lui-même** à MikCloud toutes les 45 secondes.

1. Sur MikCloud : **Routeurs → Ajouter un routeur**
2. Choisissez **Agent MikCloud (recommandé)** — seul le **nom** du site est requis
3. Cliquez **Créer et générer le script**
4. **Copiez le script** affiché (gros bouton bleu)
5. Sur le routeur : ouvrez **Winbox → New Terminal**
6. **Collez** le script et appuyez sur **Entrée**
7. Regardez l'écran : au premier check-in (≤ 45 s), le statut passe **vert « En ligne »** ✅

> Le script installe un « agent » qui survit aux redémarrages du routeur.
> Il ne communique **que sortant** : c'est pourquoi ça marche derrière l'Internet
> Orange CI, en CGNAT et avec Starlink.

### Règles d'or de l'agent
- Si vous perdez le script : menu du routeur → **Script d'installation** (⚠️ un
  nouveau token est créé — l'ancien agent cesse de fonctionner)
- Testez un routeur **connecté au même réseau local** seulement avec le mode
  « Réel (LAN) » — le mode Agent n'en a pas besoin
- Le routeur apparaît « Hors ligne » s'il est débranché ou sans Internet ; il
  revient tout seul dès que le réseau revient

---

## 2. Vendre des vouchers

1. **Vouchers → Générer** : choisissez le **site**, le **forfait**, la quantité
2. Le lot part dans la file — le routeur le crée au **prochain check-in (≤ 45 s)**
3. **Imprimez** le lot (QR codes) et vendez !
4. Onglet **Lots** : traçabilité complète (site, canal, revendeur, statuts en direct)

## 3. Vos tarifs (vous décidez)

Les 7 forfaits pré-remplis sont **modifiables/supprimables** dans **Profils** :

| Forfait    | Prix (FCFA) | Débit   |
| ---------- | ----------- | ------- |
| 1 Heure    | 100         | 1M/1M   |
| 3 Heures   | 200         | 2M/2M   |
| 24 Heures  | 300         | 2M/2M   |
| 3 Jours    | 500         | 3M/3M   |
| 7 Jours    | 1 000       | 3M/3M   |
| 15 Jours   | 1 500       | 4M/4M   |
| 30 Jours   | 3 000       | 5M/5M   |

## 4. Comptabilité

- **Rapports → Comptabilité** : ventes et chiffre d'affaires par **jour /
  semaine / mois**, filtrables par **site**, avec la part de chaque site
- **Exporter CSV** : fichier Excel (séparateur `;`, accents OK) — parfait pour
  votre comptable ou une déclaration
- Chaque vente est rattachée à son **site émetteur** et son **lot** ; les ventes
  revendeurs débitent automatiquement le portefeuille du revendeur

## 5. Wave (paiement mobile)

Wave Côte d'Ivoire n'a pas d'API publique. MikCloud compose vos demandes de
paiement à partir de votre **lien marchand** :

1. Dans Wave Business, copiez l'adresse de votre boutique
   (forme `https://pay.wave.com/m/M_xxxxx/c/ci/`)
2. Collez-la dans **Paramètres → Lien marchand Wave**
3. L'API compose les liens par montant :
   `/api/wave/link?amount=300` → `https://pay.wave.com/m/M_xxxxx/c/ci/amount/300/`

## 6. Sécurité intégrée

- Mots de passe **bcrypt** (les comptes créés avant la mise à jour sont migrés
  automatiquement au premier login)
- Le **token agent n'est jamais stocké en clair** sur le serveur (haché SHA-256)
  et n'est affiché qu'une seule fois, dans le script
- Le script valide **strictement le certificat TLS** du serveur (aucun repli
  `check-certificate=no`) — **RouterOS 7.19 ou plus récent requis** : les
  versions antérieures n'embarquent pas les certificats racine nécessaires à
  la validation Let's Encrypt. Le cloud refuse l'inscription d'un agent qui
  déclare une version inférieure (mettre à jour le firmware ou importer le
  certificat **ISRG Root X1**)
- CORS restreint et limitation de tentatives sur le login (12/min/IP)

## 7. Déploiement

| Brique  | Service                | Note                                     |
| ------- | ---------------------- | ---------------------------------------- |
| Front   | Vercel (Next.js)       | `NEXT_PUBLIC_API_BASE` → URL de l'API    |
| API     | Render (Go)            | `JWT_SECRET`, `DATABASE_URL`, `ALLOWED_ORIGIN`, `REGISTER_KEY` |
| Données | **Neon Postgres**      | Gratuit 0,5 Go — **les données survivent** aux redéploiements |

> ⚠️ Sans `DATABASE_URL`, les données sont stockées dans un fichier **effacé à
> chaque déploiement Render** — configurez Neon dès la bêta.

---

## 8. Checklist test réel sur le hAP ax³ (porte G3)

- [ ] Routeur hAP ax³ branché, Internet Orange CI actif (pas d'IP publique requise)
- [ ] Winbox connecté (MAC ou IP locale) → Terminal
- [ ] Script collé → aucun message d'erreur, log `MikCloud: agent installe`
- [ ] MikCloud : routeur **En ligne** en ≤ 45 s, RouterOS et uptime visibles
- [ ] Générer 3 vouchers « 1 Heure » → visibles sur le routeur après 45 s
- [ ] Un client se connecte au Wi-Fi, saisit le code → session visible dans Sessions
- [ ] Reboot du routeur → il repasse **En ligne tout seul** (scheduler persistant)
- [ ] Débrancher 5 min → « Hors ligne », rebrancher → « En ligne » (≤ 45 s)
- [ ] Vente comptabilisée dans Rapports → Comptabilité du jour
- [ ] Export CSV téléchargé et lisible dans Excel

**En cas d'échec** : `System → Log` du routeur, chercher `MikCloud` — le message
« check-in echoue (reseau ?) » indique un blocage HTTPS sortant (firmware ancien
ou port 443 fermé).
