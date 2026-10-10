// VpnPeer — N°291 — un peer WireGuard VENDU (produit « VPN client final »)
// hébergé sur le wg0 de la VM (Marseille), à côté des peers de GESTION des
// routeurs (tunnel N°285). C'est le chantier ⑦ de N°289 : « gère ET revend
// WireGuard sur le même écran ».
//
// Deux mondes partagent le même /24 — décision D4-a de N°289 : wg0
// partitionné par CONVENTION DE NOMMAGE, pas de wg1 dédié ni de security
// list supplémentaire en v1 :
//   - peers ROUTEURS (tunnel de gestion) : nommage côté VM router-*/r-*
//     (suggestion agent), registre = Router.WgPeerName (N°285) ;
//   - peers VPN VENDUS (ce fichier) : préfixe « vpn- » OBLIGATOIRE, le
//     registre cloud (table vpn_peers) est la SOURCE DE VÉRITÉ — le pool
//     10.8.0.2..254 est GLOBAL à la VM, la jauge de slots couvre les deux
//     registres (VpnPeerSlotsUsed) et le check de réconciliation compare
//     cloud ↔ wg-mini list (wg-peer.sh list).
//
// Cycle de vie : la création passe par le mini-service hôte wg-mini
// (127.0.0.1:4020, décision D3-a de N°289 — unité systemd root, secret
// HMAC, même machine que le backend, zéro traversée réseau publique) qui
// pilote wg-peer.sh add/remove. La conf client est générée CÔTÉ VM (la clé
// privée n'existe nulle part avant wg genkey), revient au cloud UNE fois
// par wg-mini, et est stockée CHIFFRÉE (secretbox, pattern WgPSK N°285) :
// décision D5-a — chiffré au repos + re-livraison Mail/Telegram à la
// demande, l'argument produit même face au « .conf auto SMS/email » du
// concurrent.
//
// Formes de produit (N°289 §3.1) : « fulltunnel » (P-B, sortie Internet par
// la VM — IP fixe FR) est la SEULE livrée en v1 ; « remote » (P-A, accès
// distant du gérant) exige l'élargissement allowed-address CÔTÉ ROUTEUR
// (plomberie agent, phase B) — l'API refuse avec un message clair tant que
// cette plomberie n'existe pas (verdict honnête plutôt qu'un produit en
// trompe-l'œil : le peer serait joignable mais le réseau du gérant resterait
// invisible, la réponse du routeur ne pouvant pas rentrer dans le tunnel).
//
// Garde-fous P-B (N°289 §3.3) : plafond par compte (MaxVpnPeersPerAccount),
// jauge du pool global wg0, alerte egress documentée côté console, révocation
// = wg-peer.sh remove (syncconf à chaud, les autres peers restent intacts).
// Le plafond PAR FORMULE (plan) arrive avec l'éditeur de formules — v1 pose
// la borne constante, honnête et suffisante pour un lancement.
package model

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
)

// Formes de produit (kind).
const (
	// VpnKindFullTunnel — P-B « client final » : tout le trafic sort par la
	// VM. Livré en v1 (wg-peer.sh add, conf 0.0.0.0/0 + ::/0).
	VpnKindFullTunnel = "fulltunnel"
	// VpnKindRemote — P-A « accès distant du gérant » : réservé phase B
	// (nécessite allowed-address routeur élargi côté agent — cf. N°289
	// §3.3/D4). Refusé à la création avec le code « vpn_kind_unavailable ».
	VpnKindRemote = "remote"
)

// États du peer (State).
const (
	VpnStatePending = "pending" // nom réservé au cloud, wg-mini add en vol
	VpnStateActive  = "active"  // conf générée côté VM, peer présent sur wg0
	VpnStateError   = "error"   // dernier geste wg-mini échoué (voir ErrorMsg)
)

// VpnPeer — ligne du registre des peers VPN vendus (table « vpn_peers »).
type VpnPeer struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	// Name — nom du peer CÔTÉ VM (wg-peer.sh) : préfixe « vpn- » obligatoire
	// (convention D4-a), unique à l'échelle GLOBALE du wg0 (pool partagé
	// avec les routeurs — unicité vérifiée contre les deux registres).
	Name string `json:"name"`
	// Label — nom commercial affecté par le gérant (« Téléphone client Ali »).
	// Vide = l'UI replie sur le nom VM. Même borne que les postes cyber
	// (SanitizeDeviceName, 48 caractères).
	Label string `json:"label"`
	// Kind — forme de produit (VpnKindFullTunnel en v1).
	Kind string `json:"kind"`
	// IPv4 — adresse tunnel allouée côté VM (10.8.0.N, informationnelle :
	// la vérité d'allocation reste wg-peer.sh sur la VM).
	IPv4 string `json:"ipv4"`
	// State — cycle de vie (pending/active/error).
	State string `json:"state"`
	// ErrorMsg — dernier échec wg-mini (affiché console ; ne contient JAMAIS
	// de secret : wg-mini ne logue ni ne renvoie de conf).
	ErrorMsg string `json:"errorMsg,omitempty"`
	// Conf — la configuration client COMPLÈTE (PrivateKey + PresharedKey
	// incluses). JAMAIS sérialisée (json:"-" — secrets hors JSON, double
	// barrière avec le seal/unseal du store, pattern WgPSK N°285) :
	// chiffrée au repos (secretbox, D5-a), re-livrable à la demande
	// (reveal console + Mail/Telegram).
	Conf string `json:"-"`
	// CreatedAt / UpdatedAt — RFC3339 (model.NowISO).
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// VpnPeerLabel — libellé d'affichage : le nom du gérant, sinon le nom VM.
func VpnPeerLabel(p VpnPeer) string {
	if p.Label != "" {
		return p.Label
	}
	return p.Name
}

// vpnPeerNameRe — nommage VM des peers vendus : préfixe « vpn- » + minuscules
// (même règle de caractères que wg-peer.sh et agent.ValidWgPeerName, sans
// underscores ni points : AUCUN caractère d'échappement ne peut traverser
// exec — la défense est aux DEUX bords, cloud et wg-mini).
var vpnPeerNameRe = regexp.MustCompile(`^vpn-[a-z0-9][a-z0-9-]{2,26}$`)

// ValidVpnPeerName — discipline de nommage côté cloud.
func ValidVpnPeerName(name string) bool {
	return vpnPeerNameRe.MatchString(name)
}

// NewVpnPeerName — un nom VM tiré au hasard (12 hex comme les IDs : 48 bits
// d'entropie ; la boucle d'unicité de l'appelant re-tire en cas de heur
// exacte — improbable et bénin : le tirage précède la réservation).
func NewVpnPeerName() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		// secours horloge — encodé hex : reste dans l'alphabet du nom.
		return "vpn-" + hex.EncodeToString([]byte(time.Now().UTC().Format("150405.000000000")))
	}
	return "vpn-" + hex.EncodeToString(b)
}

// FindVpnPeerScoped — retrouve le peer d'un compte par son ID exact (nil si
// absent ou appartenant à un autre compte — l'isolation multi-tenant se
// vérifie à CHAQUE résolution, même discipline que FindCyberPosteScoped).
func FindVpnPeerScoped(db *DB, id, accountID string) *VpnPeer {
	if id == "" {
		return nil
	}
	for i := range db.VpnPeers {
		if db.VpnPeers[i].ID == id && db.VpnPeers[i].AccountID == accountID {
			return &db.VpnPeers[i]
		}
	}
	return nil
}

// VpnPeerNameTaken — le nom est-il déjà pris, TOUT COMPTE CONFONDU ? Le pool
// wg0 est global à la VM : un nom vpn-* réservé par un autre compte bloque,
// ET les peers de gestion routeurs (Router.WgPeerName) aussi.
func VpnPeerNameTaken(db *DB, name string) bool {
	for i := range db.VpnPeers {
		if db.VpnPeers[i].Name == name {
			return true
		}
	}
	for i := range db.Routers {
		if db.Routers[i].WgPeerName == name {
			return true
		}
	}
	return false
}

// MaxVpnPeersPerAccount — plafond de peers VPN par compte (garde-fou P-B v1 :
// borne constante large pour un lancement ; le plafond PAR FORMULE suivra
// avec l'éditeur de formules — cf. N°289 §3.3).
const MaxVpnPeersPerAccount = 25

// VpnWg0PoolSize — capacité du pool wg0 : 10.8.0.2..254 (le .1 est le
// serveur). Global à la VM, partagé avec les routeurs de gestion.
const VpnWg0PoolSize = 253

// VpnPeerSlotsUsed — adresses tunnel réservées, tous registres confondus :
// chaque ligne VpnPeer (pending comprise — le nom est réservé) PLUS chaque
// routeur équipé du tunnel de gestion (WgIPv4). Jauge honnête du pool.
func VpnPeerSlotsUsed(db *DB) int {
	used := len(db.VpnPeers)
	for i := range db.Routers {
		if strings.TrimSpace(db.Routers[i].WgIPv4) != "" {
			used++
		}
	}
	return used
}
