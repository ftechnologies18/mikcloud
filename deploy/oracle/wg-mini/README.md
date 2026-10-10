# wg-mini — passerelle locale WireGuard de MikCloud (N°291, D3-a de N°289)

Le backend MikCloud tourne dans un conteneur distroless : il ne peut ni
`sudo` ni toucher `/etc/wireguard`. **wg-mini** est une unité systemd ROOT
sur la MÊME VM qui expose les gestes de `wg-peer.sh` au backend, en boucle
locale exclusivement (127.0.0.1:4020 — **port à réserver au registre**,
conventions §8). C'est la brique qui rend le VPN WireGuard **vendable**
depuis la console (chantier ⑦ de N°289, arbitrage D1-D7 de l'exploitant).

| Endpoint | Rôle | wg-peer.sh |
|---|---|---|
| `POST /v1/add {"name":"vpn-…"}` | génère clés + conf client CÔTÉ VM | `add <nom>` |
| `POST /v1/remove {"name":"vpn-…"}` | retire le peer, syncconf à chaud | `remove <nom>` |
| `GET /v1/conf?name=vpn-…` | relit une conf (re-livraison) | — (cat 600 root) |
| `GET /v1/list` | réconciliation cloud ↔ VM (check D4) | `list` |
| `GET /v1/ping` | santé (dot console) | — |

Tout est signé **HMAC-SHA256** (`X-WGMini-Timestamp` + `X-WGMini-Signature`
sur `<ts>.<body>`, fenêtre ±120 s, comparaison temps constant) — le même
schéma que les webhooks GeniusPay du backend, inversé. Seuls les peers
`vpn-*` passent par wg-mini ; les peers ROUTEURS (tunnel de gestion N°285)
gardent leur flux dispatch ops-wg existant.

## Installation (recette §8, ~10 minutes)

```bash
# 1. Compiler (sur la VM ou en cross depuis n'importe où)
cd wg-mini && GOOS=linux GOARCH=amd64 go build -o wg-mini .
#    puis poser le binaire :
sudo install -m 755 wg-mini /usr/local/bin/wg-mini

# 2. Le secret (64 hex) — JAMAIS dans un chat, un log ou un commit
sudo install -d -m 700 /etc/mikcloud
sudo sh -c 'umask 077 && printf "WG_MINI_SECRET=%s\n" "$(openssl rand -hex 32)" > /etc/mikcloud/wg-mini.env'
sudo chmod 600 /etc/mikcloud/wg-mini.env
#    REPRENDRE LA MÊME VALEUR dans l'env du backend (WG_MINI_SECRET,
#    cf. deploy/oracle/env.example) — c'est le partage de secret HMAC.

# 3. Unité systemd
sudo install -m 644 wg-mini.service /etc/systemd/system/wg-mini.service
sudo systemctl daemon-reload && sudo systemctl enable --now wg-mini

# 4. Vérifications de santé
systemctl status wg-mini --no-pager
ss -ltnp | rg 4020            # doit être 127.0.0.1:4020 UNIQUEMENT
curl -s http://127.0.0.1:4020/v1/ping   # 401 attendu SANS signature
journalctl -u wg-mini -n 20 --no-pager
```

### Smoke test bout en bout (optionnel, avec la clé du backend)

```bash
S=$(sudo grep -oP '(?<=WG_MINI_SECRET=).*' /etc/mikcloud/wg-mini.env)
TS=$(date +%s); BODY='{"name":"vpn-smoketest"}'
SIG=$(printf '%s.%s' "$TS" "$BODY" | openssl dgst -sha256 -hmac "$S" -hex | sed 's/^.* //')
curl -s -X POST http://127.0.0.1:4020/v1/add -H "Content-Type: application/json" \
     -H "X-WGMini-Timestamp: $TS" -H "X-WGMini-Signature: $SIG" -d "$BODY" | head -c 300
# puis le ménage (même signature sur /v1/remove) — NE PAS coller la conf nulle part
```

## Mise à jour (hot upgrade)

`wg-mini` n'engendre AUCUNE re-dépense : recompiler → `sudo install -m 755
wg-mini /usr/local/bin/wg-mini && sudo systemctl restart wg-mini` (~1 s).
`wg-peer.sh` reste re-posé à chaque dispatch ops (discipline §9 inchangée) :
wg-mini appelle TOUJOURS le script disque, jamais une copie figée.

## Rollback

`sudo systemctl disable --now wg-mini` : la console affiche « VM injoignable »
(dot rouge + messages `wg_mini_error`), les peers EXISTANTS du wg0 continuent
de router (aucun état destructif), la création/révocation est suspendue.
Aucun rollback du backend n'est requis (le backend dégrade proprement).

## Registre des ports (à tenir à jour — cf. HANDOFF-INSTALL-BACKEND.md)

| Port | Service | Notes |
|---|---|---|
| **4020/tcp (127.0.0.1)** | **wg-mini** | unité systemd root, env-file 600 root, MemoryMax 100M |

## Garde-fous intégrés

- bind 127.0.0.1 uniquement — testable : `ss -ltnp | rg 4020` ;
- HMAC obligatoire sur TOUT endpoint (ping inclus) ;
- regex de nom stricte côté mini ET côté cloud — exec sans shell ;
- confs jamais loguées (logs = op + nom + durée + statut) ;
- timeouts script 20 s / client 10 s (backend), corps ≤ 8 Ko ;
- `MemoryMax=100M` + `CPUQuota=20%` face au PostgreSQL primaire (§8.4).
