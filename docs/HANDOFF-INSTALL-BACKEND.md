# KIT DE TRANSMISSION — Installer un back-end sur la VM Ftechci

> **Document autonome (handoff)** — conçu pour être lu et appliqué par une
> session qui travaille dans **un autre dépôt** (cas d'usage premier :
> `sect-api`, le second back-end de l'exploitant) **sans jamais ouvrir ni
> cloner le dépôt mikcloud**. Ce fichier porte tout ce qui est transférable :
> les conventions d'installation éprouvées sur la VM, les modèles de commandes,
> les interdits et la liste des informations à demander à l'exploitant.
>
> Source : conventions §8 + §9 de `docs/RUNBOOK-HEBERGEMENT.md` (mikcloud),
> leçons N°274/275/279/281/282 (CHANGELOG mikcloud) — établies au feu entre
> le 02/10 et le 10/10/2026. Version : 2026-10-10.

---

## 0. Mode d'emploi de ce kit

**Pour la session destinataire (ex. une session dans le dépôt sect-api)** :
lisez ce fichier de bout en bout AVANT toute installation. Il répond à trois
questions : quelle est la machine (§1), quelles sont les règles (§2-§5), et
que demander à l'exploitant si une information manque (§7).

**Pour l'exploitant — comment transmettre ce kit (sans cloner mikcloud)**,
trois options, de la meilleure à la moins bonne :

- **Option A — committer le fichier dans le dépôt destinataire** (recommandée) :
  copier ce fichier tel quel dans `docs/` du dépôt sect-api (ex.
  `docs/HANDOFF-INSTALL-BACKEND.md`) et pousser. La connaissance vit là où elle
  s'applique ; toute session future du dépôt la relit de son propre dépôt.
  C'est la réponse à « faciliter la gestion et éviter les erreurs futures ».
- **Option B — coller le contenu intégral du fichier** comme premier message
  de la session destinataire (contexte de conversation). Rapide, mais ne
  survit pas à la session.
- **Option C — gist / lien privé** : déconseillée (péremption, accessibilité,
  hygiène).

**Pourquoi NE PAS cloner mikcloud dans l'autre dépôt** : les dépôts doivent
rester indépendants ; cloner mikcloud traîne dans le contexte de la session
destinataire du code, de l'historique et des workflows qui ne la concernent
pas (et inverse l'hygiène de contextes). Ce kit contient exactement ce qui est
transférable — le reste est propre à chaque dépôt.

---

## 1. La machine cible — faits établis

VM **Ftechci** — Oracle Cloud Infrastructure (OCI), région Marseille, plan
**Always Free**, ARM Ampere **4 OCPU / 24 Go RAM** (enveloppe max retaurée
N°272 ; l'IP publique doit être RÉSERVÉE — une IP éphémère meurt au
stop/start). C'est le serveur multi-services de l'entreprise : plusieurs
back-ends y cohabitent.

### Carte d'identité réseau (registre des ports, à TENIR À JOUR)

| Port local | Service | Notes |
|---|---|---|
| 22/tcp | SSH | seul accès admin hors VPN |
| 80/443/tcp | **Caddy** | **UNIQUE porte publique** (reverse proxy + TLS Let's Encrypt) |
| 4000/tcp (127.0.0.1) | mikcloud-server | conteneur docker, `--network host` |
| 8090/tcp (127.0.0.1) | **sect-api** | conteneur docker préexistant (ghcr.io/udevrard7/sect) |
| 4020/tcp (127.0.0.1) | **wg-mini** (N°291) | unité systemd ROOT, HMAC `WG_MINI_SECRET` partagé avec l'env backend — cf. deploy/oracle/wg-mini/README.md |
| 5432/tcp (127.0.0.1) | **PostgreSQL 18.6** | PRIMAIRE des données — ne jamais exposer, ne jamais conteneuriser |
| 4010/tcp (127.0.0.1) | (jetable, smoke tests) | libre pour les tests |
| 51820/udp | WireGuard (wg0) | accès privé, tunnel 10.8.0.0/24 |

Règles réseau qui découlent de cette carte :

1. La **security list OCI n'expose que 22/80/443** — mais on ne compte PAS
   dessus : chaque service se protège en ne publiant que sur `127.0.0.1`.
2. **Caddy est la seule porte 80/443** : un nouveau back-end publie sur
   `127.0.0.1:<port>` et c'est un bloc `reverse_proxy` dans
   `/etc/caddy/Caddyfile` qui le rend public sous son domaine.
3. **PostgreSQL écoute 127.0.0.1:5432** uniquement. Toute application qui a
   besoin d'une base s'y connecte en local (`sslmode=disable` est acceptable
   sur loopback ; `verify-full` n'a de sens que pour un PG distant).
4. Docker est installé (v29+) avec `/etc/docker/daemon.json` : rotation des
   logs conteneurs **3 × 10 Mo** (un log qui creuse = incident disque — leçon
   N°275) et `live-restore: true`. NE PAS modifier ce fichier.

---

## 2. Les conventions non négociables (§8)

Ces règles sont le fruit des incidents réels de la machine. Elles ne se
négocient pas, quel que soit le dépôt concerné :

1. **Publication TOUJOURS sur `127.0.0.1`** (`-p 127.0.0.1:P:P`, ou bind
   loopback applicatif avec `--network host`) — jamais `0.0.0.0` sans
   décision explicite de l'exploitant, jamais de port public en direct.
2. **Config par `--env-file /etc/<app>/<service>.env`** — fichier **600 root**
   (variante 640 root:<groupe> si l'app tourne sous un utilisateur dédié).
   **Jamais de secret dans l'image, ni en argument de ligne de commande**
   (visible par `ps`), ni dans le dépôt Git, ni dans un log.
3. **Limites cgroups (`--memory`, `--cpus`) OBLIGATOIRES** pour tout service
   qui partage la VM avec PostgreSQL — un service qui fuit ne doit JAMAIS
   affamer le primaire. (Suggestion enregistrée : `sect-api` actuel tourne
   sans limites — à ajouter à sa prochaine recréation.)
4. **Rollback par tag** : garder l'image précédente taguée (`previous`),
   swap + restart ≈ 10 s. Le tag `latest` est **interdit** en production.
5. **NE JAMAIS conteneuriser PostgreSQL**, ni les timers/sauvegardes/DR de
   la machine — ils vivent sur l'hôte et appartiennent à leur dépôt respectif.
6. **Un service = un port local réservé**, vérifié libre avant le premier
   démarrage (`ss -tlnp | grep :<port>`), puis inscrit dans le registre §1.
7. **Ne pas toucher aux autres services** : mikcloud-server, Caddy (hors
   ajout d'un bloc dédié), PostgreSQL, wg0, et les conteneurs des autres
   dépôts ne sont pas votre rayon. Toute modification du Caddyfile global
   se limite à l'AJOUT de votre bloc de site.
8. **Secrets et clés jamais dans les logs** (dépôts publics) : ce qui
   transite par chat/issue/CI log doit rester du non-secret (noms, ports,
   fingerprints).

---

## 3. Leçon fondatrice N°282 — le poweroff interdit

**Ne JAMAIS éteindre la VM depuis l'intérieur du guest** (`sudo shutdown -h`,
`poweroff`, ou appui « power key » dans la console OCI) : le guest s'éteint
proprement mais l'état d'instance OCI peut rester **RUNNING fantôme** sans
rien relancer (~77 min d'indisponibilité mesurées le 09/10/2026, et le
monitoring vit sur la VM — donc personne n'alerte).

- Pour redémarrer : **`sudo reboot`** (tous les reboots propres de la machine
  sont revenus seuls) ou l'action **Stop/Start de la console OCI** (réconcilie
  l'état hyperviseur).
- Si un poweroff invité est malgré tout arrivé : vérifier l'état de l'instance
  dans la console OCI juste après, **Start** manuel si « RUNNING » sans
  réponse, et prévenir l'exploitant.

---

## 4. Recette d'installation standard (pas-à-pas)

### 4.1 Avant tout — la checklist d'informations (voir §7)

Ne rien installer tant que le port, le domaine et le budget ressources ne
sont pas confirmés par l'exploitant.

### 4.2 Le fichier d'environnement (sur la VM, en SSH)

```bash
sudo mkdir -p /etc/<app>
sudo nano /etc/<app>/<service>.env     # variables du §5.1
sudo chmod 600 /etc/<app>/<service>.env
```

### 4.3 Le conteneur

```bash
# image taggée précise (jamais latest)
docker pull ghcr.io/<org>/<image>:<version-precise>

docker run -d \
  --name <service> \
  --restart unless-stopped \
  --network host \
  --env-file /etc/<app>/<service>.env \
  --memory 1g --cpus 1 \
  ghcr.io/<org>/<image>:<version-precise>
```

Variante bridge (si l'app ne sait pas se binder sur loopback seule) :

```bash
docker run -d --name <service> --restart unless-stopped \
  -p 127.0.0.1:<PORT>:<PORT_INTERNE> \
  --env-file /etc/<app>/<service>.env \
  --memory 1g --cpus 1 \
  ghcr.io/<org>/<image>:<version-precise>
```

- `--network host` + bind applicatif `127.0.0.1:<PORT>` = le pattern éprouvé
  (mikcloud-server tourne exactement comme ça).
- **Registry privé (ghcr.io)** : faire le `docker login ghcr.io` au besoin,
  avec un token fourni par l'exploitant — jamais stocké dans le dépôt.
- Application Go : binaire statique `CGO_ENABLED=0` dans une image
  **distroless** — le pattern le plus sûr (pas de shell, surface minimale).

### 4.4 Les tags de rollback

```bash
docker tag  ghcr.io/<org>/<image>:<version-N>   <service>:current
docker tag  ghcr.io/<org>/<image>:<version-N-1> <service>:previous
```

À chaque nouvelle version : `previous` ← l'ancien `current`, `current` ← la
nouvelle. Rollback instantané = retag + `docker restart <service>`. Le binaire
Go visé en 2026 (référence mikcloud) : toolchain **1.27.x** alignée sur le
`go.mod` du dépôt — que gosec et govulncheck restent verts.

### 4.5 Healthcheck + vérification

```bash
# santé locale (adapter la route de santé de l'app)
curl -fsS http://127.0.0.1:<PORT>/healthz
docker ps --filter name=<service>          # STATUS = Up (healthy si HEALTHCHECK)
docker logs --tail 50 <service>
```

Ajouter si possible un `HEALTHCHECK` dans le Dockerfile (intervalle 10-30 s)
— référence mikcloud : healthcheck mesuré **10 s** après démarrage.

### 4.6 Exposition publique (bloc Caddy DÉDIÉ — rien d'autre)

Dans `/etc/caddy/Caddyfile`, AJOUTER (ne jamais réécrire les blocs existants) :

```caddy
<domaine-ou-sous-domaine>.ftci.fr {
        encode zstd gzip
        header {
                Strict-Transport-Security "max-age=31536000; includeSubDomains"
                X-Content-Type-Options nosniff
                Referrer-Policy strict-origin-when-cross-origin
                -server
        }
        reverse_proxy 127.0.0.1:<PORT>
        request_body {
                max_size 25MB
        }
}
```

```bash
sudo caddy validate --config /etc/caddy/Caddyfile && sudo systemctl reload caddy
```

Piège TLS connu (mikcloud N°245-b, valable pour tout Caddy/certmagic) : si
le domaine pointait ailleurs AVANT (échecs ACME préalables), certmagic peut
rester en backoff exponentiel sans nouvelle tentative → `sudo systemctl
restart caddy` re-déclenche l'émission immédiatement. Le domaine doit pointer
la VM en **DNS-only** (nuage gris derrière Cloudflare), sinon le challenge
HTTP-01 échoue.

### 4.7 Après l'installation

- Inscrire le port + le domaine dans la **table du registre** (§1) — ici dans
  ce document si le kit vit dans votre dépôt.
- Prévenir l'exploitant : le monitoring de la machine (heartbeat, comptage
  docker) appartient à mikcloud — tout nouveau service doit être déclaré pour
  être compté et surveillé honnêtement.

---

## 5. Modèles prêts à copier

### 5.1 Fichier `.env` (STRUCTURE — les vraies valeurs ne se posent QUE sur la VM)

```dotenv
# /etc/<app>/<service>.env — 600 root. JAMAIS dans le dépôt Git.
LISTEN_ADDR=127.0.0.1:<PORT>
APP_ENV=production
LOG_LEVEL=info

# Base (si nécessaire) : DB et rôle DÉDIÉS sur le PG local —
# jamais la base/rôle d'une autre application.
DATABASE_URL=postgres://<role_dedie>:<mot_de_passe>@127.0.0.1:5432/<db_dediee>?sslmode=disable

# Secrets propres à l'application (fournis par l'exploitant, hors dépôt) :
# <SECRET_A>=
# <SECRET_B>=
```

### 5.2 Mise à jour d'une version (le geste de deploy minimal)

```bash
docker pull ghcr.io/<org>/<image>:<version-N+1>
docker stop <service> && docker rm <service>
# recréer le conteneur (commande §4.3 inchangée, nouveau tag)
# puis retag : previous ← current, current ← version-N+1
# puis : curl healthz + docker logs — en cas d'échec, rollback §6
```

### 5.3 Fallback systemd (si docker n'est pas souhaité pour ce service)

Unité `/etc/systemd/system/<service>.service` :

```ini
[Unit]
Description=<service> back-end
After=network-online.target

[Service]
User=<utilisateur-dedie>
EnvironmentFile=/etc/<app>/<service>.env
ExecStart=/opt/<app>/<service>            # binaire statique
Restart=always
RestartSec=5
# Garde-fou mémoire équivalent aux limites cgroups docker :
MemoryMax=1G

[Install]
WantedBy=multi-user.target
```

---

## 6. Rollback et incidents

| Situation | Geste |
|---|---|
| Nouvelle version défaillante | retag `previous` → `current`, `docker restart <service>` (≈ 10 s) — ou `deploy_mode` équivalent de votre CI |
| Service down, hôte sain | `docker logs` d'abord ; `docker restart <service>` ensuite |
| Soupçon mémoire (lenteur générale) | `docker stats --no-stream` — vérifier que PG respire ; la limite `--memory` du fautif peut être abaissée |
| Redémarrage nécessaire | **`sudo reboot`** — JAMAIS `shutdown -h`/poweroff (cf. §3) |
| VM injoignable après poweroff accidentel | console OCI : état « RUNNING » sans réponse = **Stop puis Start** (réconcilie l'hyperviseur) ; prévenir l'exploitant |
| Disque qui se remplit | premiers suspects : logs conteneurs (la rotation 3×10 Mo doit être intacte dans `/etc/docker/daemon.json`) et données d'app non purgées |

Règle transversale : **un geste risqué se prépare par un geste réversible** —
tag `previous` en place, fichier env sauvegardé (`sudo cp -a`), bloc Caddy
versionné ou sauvegardé avant édition.

---

## 7. Ce que la session doit demander à l'exploitant (checklist)

C'est la liste exacte des informations à transmettre d'une session à l'autre —
rien d'autre n'est requis, rien de tout cela ne doit circuler dans le dépôt
Git (les SECRETS transitent hors dépôt : SSH, canal direct de l'exploitant) :

1. **Le port local** réservé au service (127.0.0.1:<PORT>), vérifié libre et
   inscrit au registre §1 ;
2. **Le domaine public** (ou sous-domaine) attribué + l'état DNS (pointe déjà
   la VM ? Cloudflare en mode DNS-only ?) ;
3. **Le budget ressources** : limites `--memory` / `--cpus` allouées au
   service (partagé avec PostgreSQL — jamais illimité) ;
4. **Si base de données nécessaire** : le nom de la DB DÉDIÉE, le rôle et le
   mot de passe — créés par l'exploitant sur le PG local, transmis hors dépôt,
   jamais réutiliser la base/rôle d'une autre application ;
5. **La liste des variables d'environnement** attendues par l'app (la
   STRUCTURE, pas les valeurs) + où obtenir les valeurs ;
6. **L'image et le registry** : URL d'image précise (jamais latest), accès
   ghcr.io privé (token) le cas échéant ;
7. **Le plan de rollback** : qui retague, qui redémarre, qui prévient ;
8. **Les interdits confirmés** pour ce service : pas de port public direct,
   pas de modification hors de son bloc Caddy, pas de toucher PG/mikcloud/wg0,
   pas de poweroff invité.

---

## 8. Autorité et portée de ce kit

- Ce kit **résume** les conventions éprouvées sur Ftechci ; en cas de conflit
  entre ce fichier et la réalité de la machine, la réalité gagne — et le
  présent document se corrige dans le même passage.
- Ce qui est volontairement **HORS de portée** de ce kit : le CI/CD de
  mikcloud (`deploy-oracle`, workflows `ops-*`), le schéma de données
  mikcloud, le monitor DR et ses 11 familles, les secrets de qui que ce soit.
  Chaque dépôt garde ses workflows ; ce kit ne transfère que la discipline
  d'installation.
- Toute évolution des conventions source (mikcloud, RUNBOOK-HEBERGEMENT §8/§9)
  se répercute ici — c'est le contrat de maintenance de ce document.
