#!/usr/bin/env bash
# wg-peer.sh — cycle de vie des peers WireGuard (N°284, v2 N°285) — ROOT only
#
# usage:
#   wg-peer.sh add <nom>              — peer FULL-TUNNEL (clé générée ici, conf client .conf)
#   wg-peer.sh add-router <nom> <pub> — peer ROUTEUR MikroTik (clé publique fournie par le
#                                       routeur, livret .router.txt pour la console MikCloud)
#   wg-peer.sh remove <nom>           — révocation (bloc + fichiers livrables supprimés)
#   wg-peer.sh qr <nom>               — QR de la conf client full-tunnel
#   wg-peer.sh show-router <nom>      — ré-affiche le livret d'un routeur
#   wg-peer.sh reip <ip>              — changement d'endpoint (IP publique)
#   wg-peer.sh list                   — tableau des peers (handshake, transferts)
#
# Discipline (runbook §9/§10) : les clés privées et les PSK ne sortent JAMAIS
# ici — les livrables sont des fichiers 600 root ; les logs ne portent que du
# non-secret (noms, adresses tunnel, fingerprints).
set -euo pipefail
WG_CONF=/etc/wireguard/wg0.conf
PEERS_DIR=/opt/wireguard/peers
EP_FILE=/opt/wireguard/endpoint
[ "$(id -u)" = "0" ] || { echo "root requis"; exit 1; }
[ -f "$WG_CONF" ] || { echo "wg0.conf absent — lancer mode=install d'abord"; exit 1; }
cmd="${1:-}"; name="${2:-}"
usage(){ echo "usage: wg-peer.sh add <nom> | add-router <nom> <pubkey> | remove <nom> | qr <nom> | show-router <nom> | reip <ip> | list"; exit 1; }
name2pub(){ awk -v n="$1" 'BEGIN{RS=""} index($0,"# peer: "n"\n")==1{c=split($0,L,"\n"); for(i=1;i<=c;i++) if(L[i]~/^PublicKey = /){sub("PublicKey = ","",L[i]);print L[i]}}' "$WG_CONF"; }
apply(){ wg syncconf wg0 <(wg-quick strip wg0); }
# next_ip — prochaine adresse 10.8.0.N libre (N ≥ 2 : .1 = le serveur).
next_ip(){
  grep -oE '10\.8\.0\.[0-9]+' "$WG_CONF" | awk -F. '{print $4}' | sort -un
}
alloc(){
  USED=$(next_ip)
  local N=2
  while echo "$USED" | grep -qx "$N"; do N=$((N+1)); done
  [ "$N" -le 254 ] || { echo "❌ pool 10.8.0.0/24 épuisé"; exit 1; }
  echo "$N"
}
server_pub(){ awk '/^PrivateKey/{print $3; exit}' "$WG_CONF" | wg pubkey; }
case "$cmd" in
  add)
    [ -n "$name" ] || usage
    echo "$name" | grep -qE '^[a-z0-9][a-z0-9-]{0,31}$' || { echo "❌ nom invalide"; exit 1; }
    [ -z "$(name2pub "$name")" ] || { echo "❌ peer '$name' existe déjà"; exit 1; }
    N=$(alloc)
    EP=$(cat "$EP_FILE")
    SPUB=$(server_pub)
    PRIV=$(umask 077; wg genkey)
    PUB=$(printf '%s' "$PRIV" | wg pubkey)
    PSK=$(umask 077; wg genpsk)
    printf '\n# peer: %s\n[Peer]\nPublicKey = %s\nPresharedKey = %s\nAllowedIPs = 10.8.0.%s/32, fd00:8::%s/128\n' "$name" "$PUB" "$PSK" "$N" "$N" >> "$WG_CONF"
    apply
    mkdir -p "$PEERS_DIR"
    cat > "$PEERS_DIR/${name}.conf" <<CLIENTCONF
[Interface]
PrivateKey = ${PRIV}
Address = 10.8.0.${N}/32, fd00:8::${N}/128
DNS = 1.1.1.1, 8.8.8.8
MTU = 1420

[Peer]
PublicKey = ${SPUB}
PresharedKey = ${PSK}
Endpoint = ${EP}:51820
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25
CLIENTCONF
    chmod 600 "$PEERS_DIR/${name}.conf"
    echo "✅ peer '$name' ajouté — IP 10.8.0.$N — fingerprint $(printf '%s' "$PUB" | sha256sum | cut -c1-12)…"
    echo "   conf client : $PEERS_DIR/${name}.conf (600 root — SECRET, ne jamais coller dans un log)"
    echo "   QR (SSH interactif) : sudo qrencode -t ansiutf8 < $PEERS_DIR/${name}.conf"
    ;;
  add-router)
    # N°285 — peer ROUTEUR : la clé publique est générée SUR le routeur
    # (commande wg_keygen, clé privée jamais transportée). Le livret
    # .router.txt porte les 4 valeurs à coller dans la console MikCloud
    # (PUT /api/routers/{id}/wg-params) : address / server_pub / psk / endpoint.
    [ -n "$name" ] || usage
    RPUB="${3:-}"
    echo "$name" | grep -qE '^[a-z0-9][a-z0-9-]{0,31}$' || { echo "❌ nom invalide"; exit 1; }
    printf '%s' "$RPUB" | grep -qE '^[A-Za-z0-9+/]{43}=$' || { echo "❌ clé publique routeur invalide (44 caractères base64, « = » final)"; exit 1; }
    [ -z "$(name2pub "$name")" ] || { echo "❌ peer '$name' existe déjà (remove d'abord pour réémbarquer)"; exit 1; }
    N=$(alloc)
    EP=$(cat "$EP_FILE")
    SPUB=$(server_pub)
    PSK=$(umask 077; wg genpsk)
    # Pas d'IPv6 pour les routeurs (tunnel de gestion v4 uniquement —
    # l'ULA fd00:8::/64 reste réservée aux clients full-tunnel N°284).
    printf '\n# peer: %s\n[Peer]\nPublicKey = %s\nPresharedKey = %s\nAllowedIPs = 10.8.0.%s/32\n' "$name" "$RPUB" "$PSK" "$N" >> "$WG_CONF"
    apply
    mkdir -p "$PEERS_DIR"
    cat > "$PEERS_DIR/${name}.router.txt" <<LIVRET
address=10.8.0.${N}
server_pub=${SPUB}
psk=${PSK}
endpoint=${EP}:51820
LIVRET
    chmod 600 "$PEERS_DIR/${name}.router.txt"
    echo "✅ peer routeur '$name' ajouté — IP tunnel 10.8.0.$N — fingerprint $(printf '%s' "$RPUB" | sha256sum | cut -c1-12)…"
    echo "   livret (600 root, SECRET) : $PEERS_DIR/${name}.router.txt"
    echo "   → coller les 4 valeurs dans la console MikCloud (Routeur → Tunnel WireGuard → Paramètres serveur)"
    echo "   ré-affichage : sudo /opt/wireguard/wg-peer.sh show-router $name"
    ;;
  remove)
    [ -n "$name" ] || usage
    [ -n "$(name2pub "$name")" ] || { echo "❌ peer '$name' introuvable"; exit 1; }
    BEFORE=$(grep -c '^\[Peer\]' "$WG_CONF")
    awk -v m="# peer: ${name}" 'BEGIN{RS="";ORS=""} index($0,m"\n")!=1{if(out!="")out=out"\n\n";out=out$0} END{print out"\n"}' "$WG_CONF" > /tmp/wg0.conf.new
    AFTER=$(grep -c '^\[Peer\]' /tmp/wg0.conf.new || true)
    [ "$AFTER" -lt "$BEFORE" ] || { echo "❌ retrait raté"; rm -f /tmp/wg0.conf.new; exit 1; }
    install -m 600 /tmp/wg0.conf.new "$WG_CONF"; rm -f /tmp/wg0.conf.new
    apply
    rm -f "$PEERS_DIR/${name}.conf" "$PEERS_DIR/${name}.router.txt"
    echo "✅ peer '$name' révoqué (bloc + livrables supprimés, tunnel des autres peers intact)"
    ;;
  qr)
    [ -n "$name" ] || usage
    [ -f "$PEERS_DIR/${name}.conf" ] || { echo "❌ conf inexistante"; exit 1; }
    qrencode -t ansiutf8 < "$PEERS_DIR/${name}.conf"
    ;;
  show-router)
    [ -n "$name" ] || usage
    [ -f "$PEERS_DIR/${name}.router.txt" ] || { echo "❌ livret inexistant (peer full-tunnel ou routeur jamais embarqué)"; exit 1; }
    cat "$PEERS_DIR/${name}.router.txt"
    ;;
  reip)
    NEWIP="${2:-}"
    echo "$NEWIP" | grep -qE '^[0-9]{1,3}(\.[0-9]{1,3}){3}$' || { echo "❌ IP invalide"; exit 1; }
    echo "$NEWIP" > "$EP_FILE"
    for f in "$PEERS_DIR"/*.conf; do [ -f "$f" ] && sed -i "s/^Endpoint = .*/Endpoint = ${NEWIP}:51820/" "$f"; done
    for f in "$PEERS_DIR"/*.router.txt; do [ -f "$f" ] && sed -i "s/^endpoint=.*/endpoint=${NEWIP}:51820/" "$f"; done
    echo "✅ endpoint -> $NEWIP:51820 (confs clients + livrets routeurs mis à jour, clés conservées)"
    ;;
  list)
    echo "nom | IP tunnel | dernier handshake | reçu/envoyé"
    awk 'BEGIN{RS=""} /^# peer: /{c=split($0,L,"\n"); n=L[1]; sub("# peer: ","",n); ip=""; k=""; for(i=1;i<=c;i++){if(L[i]~/^AllowedIPs = /){ip=L[i];sub("AllowedIPs = ","",ip)}; if(L[i]~/^PublicKey = /){k=L[i];sub("PublicKey = ","",k)}} print k"|"n"|"ip}' "$WG_CONF" > /tmp/wgmap.$$
    while IFS='|' read -r KEY NM IP; do
      HS=$(wg show wg0 latest-handshakes | awk -v k="$KEY" '$1==k{print $2}')
      TR=$(wg show wg0 transfer | awk -v k="$KEY" '$1==k{print $2"/"$3}')
      [ -n "$TR" ] || TR="—"
      AGE="jamais"
      if [ -n "${HS:-}" ] && [ "$HS" != "0" ]; then AGE="$(( $(date +%s) - HS ))s"; fi
      echo "$NM | $IP | $AGE | $TR"
    done < /tmp/wgmap.$$
    rm -f /tmp/wgmap.$$
    ;;
  *) usage ;;
esac
