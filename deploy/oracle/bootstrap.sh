#!/usr/bin/env bash
# mikcloud — bootstrap d'une VM Oracle Cloud Always Free (Ubuntu aarch64).
#
# Usage : sudo bash bootstrap.sh <domaine-du-backend>
#   ex.  : sudo bash bootstrap.sh api.mikcloud.ftci.fr
#
# Idempotent : relançable sans risque. Détail complet dans
# docs/MIGRATION-ORACLE.md §2. Ce script NE démarre PAS le backend :
# il pose l'infrastructure (pare-feu, Caddy, utilisateur, systemd, CA
# Supabase, timer de sauvegarde) — le binaire arrive par le workflow
# GitHub Actions deploy-oracle (ou manuellement, cf. §4).
set -euo pipefail

DOMAIN="${1:?Usage: bootstrap.sh <domaine-du-backend>}"
REPO_RAW="https://raw.githubusercontent.com/ftechnologies18/mikcloud/main"

[ "$(id -u)" -eq 0 ] || { echo "ERREUR : lancez avec sudo (root requis)."; exit 1; }
command -v curl >/dev/null || { apt-get update -y && apt-get install -y curl; }

echo "==> [0/8] Verrou apt : attendre les mises à jour du premier boot"
# N°228 — au premier boot d'une VM Oracle, unattended-upgrades tient
# /var/lib/apt/lists/lock plusieurs minutes ; cloud-init peut démarrer
# bootstrap avant la fin. Attendre (max 10 min) au lieu de mourir sur
# « E: Could not get lock » (set -euo pipefail). Idempotent : en relance
# manuelle ultérieure, le verrou est libre et l'attente est immédiate.
i=0
while pgrep -x apt >/dev/null || pgrep -x apt-get >/dev/null \
   || fuser /var/lib/apt/lists/lock >/dev/null 2>&1; do
        i=$((i+1)); [ "$i" -gt 60 ] && break
        echo "    verrou occupé — attente 10 s ($i/60)"
        sleep 10
done

echo "==> [0b/8] MTU 1500 : l'interface Oracle vient en jumbo 9000 face à un"
echo "           internet 1500 — sans ce fix, les gros transferts (sync"
echo "           PostgreSQL au boot) étouffent (piège n°14). Idempotent."
DEFDEV="$(ip route show default 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="dev") print $(i+1); exit}')"
if [ -n "${DEFDEV:-}" ] && [ -e "/sys/class/net/$DEFDEV/mtu" ]; then
        CUR_MTU="$(cat "/sys/class/net/$DEFDEV/mtu")"
        if [ "$CUR_MTU" != "1500" ]; then
                ip link set "$DEFDEV" mtu 1500
                # Persistance : fichier netplan dédié (fusion lexicale — les
                # clés de 99- priment sur 50-cloud-init.yaml SANS le modifier).
                printf 'network:\n  version: 2\n  ethernets:\n    %s:\n      mtu: 1500\n' \
                        "$DEFDEV" > /etc/netplan/99-mtu.yaml
                chmod 600 /etc/netplan/99-mtu.yaml
                netplan apply 2>/dev/null || true
                echo "    $DEFDEV : MTU $CUR_MTU → 1500 (+ /etc/netplan/99-mtu.yaml)"
        else
                echo "    $DEFDEV déjà en 1500 — rien à faire"
        fi
fi

echo "==> [1/8] Paquets de base"
export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y curl ca-certificates gnupg postgresql-client iptables-persistent unattended-upgrades

echo "==> [2/8] Pare-feu local : 80/443 (PIÈGE Ubuntu-Oracle : iptables DROP par défaut,"
echo "           indépendant de la Security List du VCN — les DEUX doivent être ouverts)"
iptables -C INPUT -p tcp --dport 80 -j ACCEPT 2>/dev/null || iptables -I INPUT -p tcp --dport 80 -j ACCEPT
iptables -C INPUT -p tcp --dport 443 -j ACCEPT 2>/dev/null || iptables -I INPUT -p tcp --dport 443 -j ACCEPT
netfilter-persistent save

echo "==> [3/8] Caddy (reverse proxy + TLS Let's Encrypt automatique)"
install -d -m 0755 /usr/share/keyrings
curl -fsSL 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --batch --yes --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -fsSL 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' > /etc/apt/sources.list.d/caddy-stable.list
apt-get update -y
apt-get install -y caddy

echo "==> [4/8] Utilisateur et répertoires"
id mikcloud >/dev/null 2>&1 || useradd --system --home-dir /opt/mikcloud --shell /usr/sbin/nologin mikcloud
install -d -o mikcloud -g mikcloud -m 0755 /opt/mikcloud
install -d -m 0750 /etc/mikcloud
if [ ! -f /etc/mikcloud/mikcloud.env ]; then
        printf '# Variables mikcloud — modèle : deploy/oracle/env.example (docs/MIGRATION-ORACLE.md §3)\n' > /etc/mikcloud/mikcloud.env
        chmod 640 /etc/mikcloud/mikcloud.env
        chown root:mikcloud /etc/mikcloud/mikcloud.env
fi
if [ ! -f /etc/mikcloud/backup.env ]; then
        printf '# Coffre-fort Neon (docs/MIGRATION-ORACLE.md §7)\n# DATABASE_URL_SESSION=postgresql://...pooler.supabase.com:5432/postgres\n# NEON_DATABASE_URL=postgresql://...neon.tech/neondb?sslmode=require\n' > /etc/mikcloud/backup.env
        chmod 640 /etc/mikcloud/backup.env
        chown root:mikcloud /etc/mikcloud/backup.env
fi

echo "==> [5/8] CA privée Supabase (OBLIGATOIRE pour sslmode=verify-full, cf. Dockerfile)"
curl -fsSL "$REPO_RAW/backend/certs/supabase-prod-ca-2021.crt" -o /usr/local/share/ca-certificates/supabase-prod-ca-2021.crt
update-ca-certificates

echo "==> [6/8] Caddyfile, service systemd, sauvegarde Neon (téléchargés depuis le repo)"
curl -fsSL "$REPO_RAW/deploy/oracle/Caddyfile" -o /etc/caddy/Caddyfile
if grep -q '__MIKCLOUD_DOMAIN__' /etc/caddy/Caddyfile; then
        sed -i "s/__MIKCLOUD_DOMAIN__/$DOMAIN/g" /etc/caddy/Caddyfile
fi
systemctl reload caddy 2>/dev/null || systemctl restart caddy
curl -fsSL "$REPO_RAW/deploy/oracle/mikcloud.service" -o /etc/systemd/system/mikcloud.service
curl -fsSL "$REPO_RAW/deploy/oracle/backup-neon.sh" -o /usr/local/sbin/mikcloud-backup
chmod 750 /usr/local/sbin/mikcloud-backup
curl -fsSL "$REPO_RAW/deploy/oracle/mikcloud-backup.service" -o /etc/systemd/system/mikcloud-backup.service
curl -fsSL "$REPO_RAW/deploy/oracle/mikcloud-backup.timer" -o /etc/systemd/system/mikcloud-backup.timer
systemctl daemon-reload
systemctl enable --now caddy
systemctl enable --now mikcloud-backup.timer

echo "==> [7/8] Mises à jour de sécurité automatiques"
systemctl enable --now unattended-upgrades 2>/dev/null || true

echo "==> [8/8] Terminé."
cat <<'EOF'

PROCHAINES ÉTAPES (docs/MIGRATION-ORACLE.md) :
  1. Renseigner /etc/mikcloud/mikcloud.env (modèle : deploy/oracle/env.example)
     puis : sudo systemctl enable --now mikcloud
     (le binaire /opt/mikcloud/mikcloud-server arrive par le workflow
      GitHub Actions deploy-oracle — cf. §6 — ou manuellement, cf. §4)
  2. Optionnel — coffre-fort Neon : /etc/mikcloud/backup.env
     (DATABASE_URL_SESSION port 5432 + NEON_DATABASE_URL) — timer 03:00 UTC.
  3. Vérifier : systemctl status caddy mikcloud-backup.timer

EOF
