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
5. Coller le bloc `-----BEGIN PUBLIC KEY-----…-----END PUBLIC KEY-----` —
   **SOURCE CANONIQUE (N°213)** : l'URL raw du repo
   `https://raw.githubusercontent.com/ftechnologies18/mikcloud/main/deploy/oracle/pilot-api-public-key.pem`
   (ouvrir dans le navigateur, Ctrl+A puis Ctrl+C, coller dans la console).
   Copier depuis le chat reste possible mais le presse-papiers à deux
   conversations est un piège CONSTATÉ (collage du mauvais bloc → empreinte
   inattendue 30:71:75:…) → **Ajouter**.
6. **Juge de paix** : la NOUVELLE ligne de la liste (la plus récente) doit
   afficher l'empreinte `07:be:4a:04:8c:40:57:38:6b:75:91:a4:3f:f8:e6:a5`
   (MD5 du DER de la clé — vérifiable par `openssl pkey -pubin -in
   pilot-api-public-key.pem -outform DER | openssl md5`).

### Geste 2 — recopier la configuration générée

La fenêtre « Informations de configuration » qui s'affiche ensuite contient
un bloc `user=`, `fingerprint=`, `tenancy=`, `region=` → **le transmettre
tel quel à l'agent** (avec la région confirmée en haut à droite de la
console). C'est tout.

## 3. Ce que pilote l'agent ensuite (dans l'ordre du runbook N°211)

| # | Action pilotée | Équivalent runbook | Vérification visible |
|---|---|---|---|
| 1 | VCN + sous-réseau + Security Lists 22/80/443 | §1.3 | console → Networking |
| 2 | Instance **Ftechci** (ex-`mikcloud-backend`, N°272) A1.Flex **4 OCPU/24 Go** (N°272, 09/10/2026), Ubuntu 24.04, 50 Go | §1.2 | console → Compute → Instances |
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
VM Oracle A1.Flex (**24 Go RAM depuis le N°272 — précieuse, pas de pool de conteneurs**)
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
| 08/10/2026 | Runbook §7 : rename + resize 4/24 (voie console, N°271) | doctrine révisée, exécution exploitant | à consigner après exécution |
| 09/10/2026 | **Exécution via workflow `ops-resize-marseille`** (N°272) : rename → SOFT stop → shape 4/24 → Start #2 (capacité refusée au #1, libérée au #2) | API verte 01:43 UTC ; vérif lecture-seule `mode=check` run #5 : **Ftechci [VM.Standard.A1.Flex] RUNNING — 4 OCPU**, garde enveloppe OK (1 A1) | ✅ RÉUSSI |
| 09/10/2026 | **Consolidation DR `ops-db-hybrid` modes arm-dr/reverse-sync/health/drill** (N°275, GO « GO N°275 ») : archive_command gzip + WAL→Object Storage (bucket `mikcloud-wal`, upload */5 min), reverse-sync nocturne local→Supabase→Neon (40/40 tables, RLS ré-armé, rétention web_vitals 90 j), monitor Telegram */15 min, drill de restaurabilité, artefact DR basculé sur le primaire | arm-dr vert (run #23), reverse-sync vert (run #30, `last_success=05:08:59Z`), drill vert (run #42, comptages identiques 7 257/7 257), health `fails=0` (run #47) | ✅ RÉUSSI |
| 09/10/2026 | **Incident VM hung** (~05:14-06:31 UTC) : SSH+443 morts, OCI=RUNNING ; SOFTRESET ignoré (noyau hung) ; **RESET dur via `ops-vm-diag`** (nouveau workflow : état + console history + START/RESET) | API 200 à 06:31 UTC, downtime ~1 h 17, **zéro perte** (DSN local préservé, PG a relu son WAL) ; cause racine indéterminée — console history maintenant ARMABLE avant reboot | ✅ RÉTABLI |
| 09/10/2026 | **Lifecycle 21 j du bucket `mikcloud-wal` débloqué** (N°276, mode `arm-dr` enrichi) : policy IAM `mikcloud-objectstorage-lifecycle` posée PAR API (`Allow service objectstorage-<region> to manage object-family in tenancy`) + règle `expire-21d` posée puis RELUE depuis le bucket | arm-dr vert en ~2 min — **la console n'était pas nécessaire** (le credentials du coffre a `manage policies`) ; PITR borné 21 j, plateau ~0,3-1,2 Go (plafond gratuit 10 Go) | ✅ RÉUSSI |
| 09/10/2026 | **Chiffrement client WAL + basebackups avant upload** (N°277/277-bis) : AES-256-CBC/PBKDF2 `WAL_ENC_KEY`=BACKUP_KEY par stdin, bucket 100 % `.enc`, refus strict d'upload en clair, drill avec déchiffrement + fallback `.gz` héritage ; fix piège 22 (export openssl -pass env:) | preuves : timer `new=25 fail=0`, base backup CHIFFRÉ 7.3M, drill « CHIFFRÉ, déchiffré + décompressé 16M », monitor fails=0, API 200 ; **dette OCI moindre privilège enregistrée → fin du développement produit** | ✅ RÉUSSI |
| 09/10/2026 | **Appairage Telegram armé** (N°278) : exploitant a appairé le bot depuis la console admin plateforme ; chat `7026277370` relu d'`notif_settings` par arm-dr #53 → `/etc/mikcloud/monitor.env` ; monitor assaini (`printf '%b'` ×4 + écho arm-dr fidèle) posé par arm-dr #54 | health #55 `fails=0 state=ok`, API 200/200 ; alertes DR délivrables (heartbeat 06:00 UTC / premier incident) ; les 4 comptes utilisateurs peuvent activer Telegram pour leurs alertes propres | ✅ RÉUSSI |
| 09/10/2026 | **Phase A — terrain Docker sur Ftechci** (N°279, GO « Go phase A ») : nouveau workflow `ops-docker` (install/status/prune), `daemon.json` rotation logs 3×10 Mo + live-restore, smoke test jetable (127.0.0.1:4010, DNAT, egress bridge) ; monitor DR famille n°10 docker posé par arm-dr #56 | Docker 29.1.3 + Compose 2.40.3 déjà présents (idempotence) ; conteneur préexistant `sect-api` (127.0.0.1:8090) observé NON touché ; health #57 `fails=0`, API 200/200 ; backend mikcloud + PostgreSQL intouchés — **Phase B = décision séparée après éclaircissement du hang** | ✅ RÉUSSI |
| 09/10/2026 | **Éclaircissement du « hang » du 09/10** (N°280, `ops-vm-diag` mode `postmortem` v1→v4) : le boot -5 finit à 05:13:23 par **« Power key pressed short »** (logind) → poweroff 100 % PROPRE à 05:13:28 ; **zéro signal noyau** sur les 10 boots de la nuit ; OCI resté **RUNNING fantôme** → 77 min down jusqu'au RESET dur ; initiateur = bouton ACPI côté hyperviseur/console (sessions tty console série actives 05:03-05:10, aucun sudo shutdown) | **ce n'était PAS un kernel hang** ; « reboot spontané 04:25 » = série de poweroffs propres identiques (01:42→09:38) ; question à l'exploitant : gestes console ? si NON → ticket support OCI ; DR : zéro perte confirmé | ✅ ÉCLAIRCI |
| 09/10/2026 | **Phase B — backend mikcloud DOCKERISÉ** (N°281, GO « démarrage de la phase B ») : `deploy-oracle` transport-aware (auto/docker/systemd), image distroless buildée sur VM, tags current/previous, `--network host --env-file mikcloud.env --restart unless-stopped --memory 4g --cpus 3`, rollback systemd auto si KO ; monitor famille 2 transport-agnostique ; systemd arrêté+conservé | migration run #20 verte : « BACKEND OK (healthcheck au bout de 10 s) » (vs ~1 min à l'ère Johannesburg), conteneur Up, images current+previous ; arm-dr #58 + health #59 `fails=0`, API 200/200 ; rollback = dispatch `deploy_mode=systemd` | ✅ RÉUSSI |
| 09/10/2026 | **Réponse exploitant — dossier « hang » CLÔT** (N°282) : les gestes de la nuit venaient de l'exploitant, **pendant l'installation du second back-end** (power key 05:13:23 + série de boots propres 01:42→09:38 + `sect-api` = ce second back-end, découvert N°279) | pas de ticket OCI, pas d'action plateforme ; leçon RUNBOOK §8 : éviter le poweroff invité (OCI peut rester RUNNING fantôme sans rien relancer — monitor muet car co-résident), préférer `sudo reboot` ou Stop/Start console ; suggestion : limites cgroups `sect-api` à sa prochaine recréation | ✅ CLÔT |
| 09/10/2026 | **CI rouge → verte** (N°283) : govulncheck détecte GO-2026-6617/CVE-2026-97032 (HPACK encoder race, HTTP/2 **stdlib go1.27.0**, entrée publiée ce jour) — atteignable via nos chemins réels (TelegramSetWebhook, ListenAndServe) ; CI rouge depuis 10:11 (5 runs, aucun lien avec nos changements) ; deploy-oracle non gated sur CI (par design) → prod jamais bloquée | fix = `backend/go.mod` go 1.27.0 → **1.27.2** ; validation locale : gofmt/vet/tests 100 % verts + govulncheck 0 atteignable ; deploy auto SUCCESS (« go1.27.2 » au build runner, healthcheck OK, API ok:true sweep post-deploy) ; CI 2677be7 SUCCESS (première verte depuis 10:11) ; reste 1 vuln module non atteignable (GO-2026-5932 openpgp, Fixed N/A) | ✅ RÉUSSI |
| 09/10/2026 | **WireGuard full-tunnel + accès services** (N°284, GO « full tunnel Wi-Fi + accès services, complet et évolutif ») : voie A **wg-quick hôte-native** (pas de UI tierce, pas de conteneur NET_ADMIN, IP sources préservées), `ops-wg` (install/status/peer-add/peer-remove/prune gardé), `wg0` 10.8.0.1/24 UDP 51820, PostUp iptables gardés `-C\|\|-I` (MASQUERADE → **enp0s6** résolu dynamiquement — pas d'`eth0` codé en dur), `ip_forward` persisté, `wg-peer.sh` (PresharedKey/peer, `syncconf` zéro interruption, clés jamais dans les logs) ; monitor DR famille 11 | install #1 vert (run 38000922564) : wg0 UP, UDP 51820 en écoute, INPUT/FORWARD/MASQUERADE posés, backend `ok:true` + Caddy 443 → 200 (coexistence prouvée) ; arm-dr + health `fails=0 state=ok` ; gestes exploitant restants : Security List UDP 51820 + IP réservée + 1er peer (QR via SSH) — cf. RUNBOOK §9 | ✅ RÉUSSI |

## 7. Serveur d'entreprise « Ftechci » — rename + resize 4/24 (N°271, voie console)

> Décision N°271 : la VM devient le **serveur multi-services de
> l'entreprise** (charte §4) sous le nom **Ftechci**, shape porté
> **2 OCPU/12 Go → 4 OCPU/24 Go** (enveloppe max Always Free — voir
> CHANGELOG N°271 pour la révision de la règle 6 N°251-b et les GARDES).
> Toute l'opération se fait à la console, ~15 minutes, downtime
> ~5-10 minutes (sessions hotspot actives : non affectées ; nouvelles
> connexions : en pause le temps du cycle).

### Pré-vol (2 minutes)
1. Vérifier https://api.mikcloud.ftci.fr/ → `{"ok":true,…}` (état vert
   AVANT manipulation) ;
2. Budget : Billing & Cost Management → Budgets → l'alerte 1 €/mois
   existe toujours (N°211) — c'est le détecteur d'incendie post-resize.

### Phase A — Rename (zéro downtime, sans impact CI)
1. Console → sélecteur de région (haut droite) = **France Central
   (Marseille)** / eu-marseille-1 ;
2. ☰ menu → **Compute → Instances** → cliquer **mikcloud-backend** ;
3. À côté du nom : icône **crayon (Edit)** → taper **Ftechci** →
   **Save changes**. C'est tout — IP réservée 84.235.228.160 conservée,
   `ORACLE_HOST`/`ORACLE_SSH_KEY` (CI deploy-oracle) inchangés, systemd
   et Caddy non concernés.

### Phase B — Resize 2/12 → 4/24 (downtime ~5-10 min)
4. Sur la page de l'instance (toujours région Marseille) → **Stop** —
   choisir **Soft stop** (défaut) : le backend reçoit SIGTERM → **flush
   final propre de son état mémoire vers la base** (store.Close) → c'est
   la garantie zéro-perte ; attendre l'état **STOPPED** ; si bloqué
   > 5 min → **Force stop (hard)** acceptable ;
5. ⚠️ **Ne PAS laisser l'instance STOPPée prolongé** (réclamation
   possible des Always Free inactifs) — enchaîner immédiatement ;
6. Panneau **Shape** → **Edit shape** → `VM.Standard.A1.Flex` (inchangé)
   → **OCPU = 4**, **Memory = 24 GB** → Save. La console doit afficher
   le statut « Always Free eligible » / coût 0 $ — **si un prix
   apparaît : NE PAS valider**, revenir à l'agent ;
7. **Start** → état STARTING → RUNNING (~1-2 min).

### Si le Start échoue (« Out of host capacity »)
8. Retry **Start** toutes les ~90 s pendant **10 minutes maximum** (la
   capacité se libère par vagues) ;
9. Toujours bloqué → **revert** : Edit shape → 2 OCPU / 12 GB → Start →
   la production repart (le 2/12 tournait avant) ; l'IP et le rename
   sont conservés ; on retente le 4/24 un autre jour (fenêtre nocturne).

### Post-vol (5 minutes)
10. https://api.mikcloud.ftci.fr/ → `{"ok":true,…}` (le service
    systemd `Restart=always` repart seul ; reload des 37 tables ~20 s) ;
11. https://mikcloud.ftci.fr/ → HTTP 200 (Vercel, non concerné) ;
12. SSH : `free -h` (≈ 24 Go visibles) + `nproc` (= 4) + `uptime` ;
13. Console : état **RUNNING**, shape **4 OCPU / 24 GB**, nom **Ftechci** ;
14. 24 h : **aucun email d'alerte budget** (sinon investigation
    immédiate + revert — garde 2 N°271).

### Gardes permanents (rappel N°271)
| Garde | Règle |
|---|---|
| Enveloppe A1 tenancy Marseille | **ZÉRO autre instance A1** tant que Ftechci tourne en 4/24 (97,3 % de l'allocation consommée) |
| Captures/atterrissages A1 | Tenancy **JNB uniquement** |
| Instances A1 de test | Interdites dans cette tenancy |
| Alerte budget 1 € | Doit rester muette — sinon revert |

### Ce que le resize prépare (roadmap N°272+)
- Migration hybride DB : PostgreSQL co-hébergé (config calibrée 4/24 :
  `shared_buffers` 2-4 Go, `effective_cache_size` 10-12 Go,
  `max_connections` 50-60 multi-services, une base + un rôle par
  service) — voir analyse session N°271 ;
- Services d'entreprise futurs : recette §4 (port 4001, 4002… + bloc
  Caddy + service systemd + `MemoryMax`) ;
- Pont E5 : décommission possible **avant le 31/10** (hors enveloppe
  A1 — AMD ; seule ressource facturable après crédits).
