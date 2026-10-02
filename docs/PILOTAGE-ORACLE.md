# PILOTAGE ORACLE — Mode « l'agent aux commandes » (N°212)

> Complément opérationnel de [`docs/MIGRATION-ORACLE.md`](MIGRATION-ORACLE.md)
> (N°211). Le runbook N°211 prévoyait la création de la VM aux clics par
> l'exploitant ; **ce document installe le MODE PILOTAGE** : l'agent technique
> pilote l'API Oracle Cloud (OCI CLI) avec une clé API déposée par
> l'exploitant, qui garde le contrôle total (révocation en 2 clics) et apprend
> en observant chaque commande expliquée. Lecteur : exploitant DÉBUTANT
> Oracle — aucun prérequis technique.

## 1. Le modèle de sécurité — « le cadenas et la clé »

Oracle n'utilise pas de « token » comme GitHub : il utilise une **clé API de
signature** — une paire de clés RSA, comme un cadenas et sa clé :

```
   SANDBOX DE PILOTAGE                    CONSOLE ORACLE (l'exploitant)
   ┌──────────────────────┐               ┌──────────────────────────┐
   │ clé PRIVÉE api-key   │               │ clé PUBLIQUE déposée     │
   │ .pem (chmod 600)     │               │ dans le profil utilisateur│
   │ signe chaque appel   │───HTTPS──────▶│ Oracle vérifie la         │
   │ API (aucun mot de    │               │ signature = identité      │
   │ passe ne circule)    │               │ confirmée                │
   └──────────────────────┘               └──────────────────────────┘
```

- **La clé privée ne quitte JAMAIS le sandbox de pilotage.** Rien de sensible
  ne transite dans le chat.
- Les **OCIDs** (identifiants `ocid1.user…`, `ocid1.tenancy…`) transmis en
  clair ne permettent RIEN seuls — sans la clé privée, ils sont inertes.
- **Couper le pilotage** : console → profil → Clés API → ⋏ → Delete. 2 clics,
  effet immédiat. C'est le garde-fou de l'exploitant.
- La clé SSH de la VM suit le même modèle : paire ed25519 générée côté
  pilotage, publique injectée à la création de l'instance.

## 2. Les DEUX seuls gestes console de l'exploitant

### Geste 1 — déposer la clé publique API (une fois)

1. https://cloud.oracle.com → connexion.
2. Icône **profil** (silhouette, en haut à droite) → **Mon profil / User
   Settings**.
3. Menu de gauche, en bas : **Clés API / API keys** → **Ajouter une clé API /
   Add API key**.
4. Choisir **Coller une clé publique / Paste a public key**.
5. Coller le bloc `-----BEGIN PUBLIC KEY-----…-----END PUBLIC KEY-----`
   fourni par l'agent → **Ajouter**.

### Geste 2 — recopier la configuration générée

La fenêtre « Informations de configuration » qui s'affiche ensuite contient
un bloc `user=`, `fingerprint=`, `tenancy=`, `region=` → **le transmettre
tel quel à l'agent** (avec la région confirmée en haut à droite de la
console). C'est tout.

## 3. Ce que pilote l'agent ensuite (dans l'ordre du runbook N°211)

| # | Action pilotée | Équivalent runbook | Vérification visible |
|---|---|---|---|
| 1 | VCN + sous-réseau + Security Lists 22/80/443 | §1.3 | console → Networking |
| 2 | Instance `mikcloud-backend` A1.Flex 2 OCPU/12 Go, Ubuntu 24.04, 50 Go | §1.2 | console → Compute → Instances |
| 3 | IP publique réservée + relevé | §1.2 | console → instance → IP |
| 4 | `bootstrap.sh` en SSH (iptables, Caddy, systemd, CA Supabase) | §2 | `systemctl status caddy` |
| 5 | Secrets GitHub `ORACLE_HOST/USER/SSH_KEY` | §6 | repo → Settings → Secrets |
| 6 | Déploiement CI/CD (build ARM64 → scp → restart) | §6 | Actions vertes |
| 7 | Récupération des 27 variables depuis Render | §3 | `/etc/mikcloud/mikcloud.env` |
| 8 | Bascule domaine en deux temps T1/T2 | §5/§8 | runbook §8 (13 étapes) |

Chaque commande exécutée est consignée au §6 ci-dessous avec son explication
en une ligne — c'est le journal d'apprentissage de l'exploitant.

## 4. Infrastructure MULTI-BACKENDS (demande de l'exploitant)

L'exploitant hébergera **d'autres backends** sur la même VM. Architecture
retenue (délibérément SANS Docker) :

```
VM Oracle A1.Flex (12 Go RAM — précieuse, pas de pool de conteneurs)
├── Caddy (reverse proxy unique, TLS automatique)
│     ├── api.mikcloud.ftci.fr  → 127.0.0.1:4000  (mikcloud.service)
│     ├── app2.exemple.fr       → 127.0.0.1:4001  (app2.service, futur)
│     └── app3.exemple.fr       → 127.0.0.1:4002  (app3.service, futur)
├── /opt/mikcloud/   binaire + droits dédiés
├── /opt/app2/       (futur)
├── /etc/mikcloud/   env de production (chmod 640)
└── systemd : un service par backend, MemoryMax individuel
```

**Recette d'ajout d'un backend n°2** (le jour venu) :
1. `deploy/<app2>/` dans le monorepo : service systemd + bloc Caddy + env ;
2. un port local libre (4001, 4002…) et un sous-domaine DNS ;
3. un job ou workflow de build/scp calqué sur `deploy-oracle.yml` ;
4. `systemctl enable --now app2` — mikcloud ne touche à rien du voisin.

Pourquoi pas Docker : les binaires Go statiques (`CGO_ENABLED=0`) n'ont
**aucune dépendance** — un service systemd direct consomme 0 RAM de plus,
alors que Docker + un registry coûteraient de la RAM, du disque et de la
complexité sur une VM gratuite. Un backend = un binaire = un service.

## 5. Inventaire du poste de pilotage (sandbox)

| Fichier | Rôle | Permission |
|---|---|---|
| `/home/z/.oci/api-key.pem` | clé PRIVÉE API Oracle (signature) | 600 |
| `/home/z/.oci/api-key-public.pem` | clé publique déposée en console | 644 |
| `/home/z/.oci/ssh-oracle[.pub]` | paire SSH de la VM | 600 |
| `/home/z/.oci/config` | profil OCI (OCIDs + région + empreinte) | — |

En cas de doute ou d'incident : **révoquer la clé API en console** (§1),
puis régénérer (l'agent reproduit le geste 1 avec une nouvelle paire).

## 6. Journal des commandes pilotes

| Date | Commande (résumé) | Explication une ligne | Résultat |
|---|---|---|---|
| 02/10/2026 | `oci setup config` (squelette) | préparation du profil de pilotage | config créée, placeholders |
| 02/10/2026 | (attend les OCIDs de l'exploitant) | geste 2 du §2 | — |
