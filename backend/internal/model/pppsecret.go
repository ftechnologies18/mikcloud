// PppSecret — N°293 — un abonné PPPoE (chantier ⑥ de N°289, Option V lot 3,
// Phase A : gestion d'un pppoe-server EXISTANT).
//
// Décisions appliquées (docs/ANALYSE-P1-PPP-WG-CYBER.md §2, arbitrées) :
//   - D1 — transport canonique = commandes agent ppp_* (zéro cred API
//     stockée, zéro port public, CGNAT-proof : le routeur initie tout) ;
//     le renfort API directe à travers wg0 est la phase B.
//   - D2 — périmètre v1 = GÉRER les secrets d'un pppoe-server déjà
//     configuré chez le WISP. Le cloud ne provisionne NI l'interface
//     pppoe-server, NI les profils PPP (phase B) : Profile référence un
//     nom de profil EXISTANT côté routeur.
//
// Le secret PPP vit sur UN routeur précis (RouterID — le pppoe-server du
// WISP) : l'unicité du Name est PAR ROUTEUR (deux POPs peuvent porter le
// même nom d'abonné, la DDL pose UNIQUE(router_id, name) — contrairement
// aux peers VPN N°291 dont le nom est global au wg0 de la VM).
//
// Parité cloud ↔ routeur : le read_state du chunk final rapporte la liste
// des secrets présents (name|disabled, plafonnée 500) — LastSeenOnRouter
// est rafraîchi, les pending passent active. HONNÊTETÉ v1 : une ligne
// absente du routeur n'est NI détruite NI re-créée automatiquement
// (contrairement aux vouchers N°162 et leur autoréparation) — l'absence
// se voit dans l'état de parité (LastSeenOnRouter vieilli) et la
// réparation reste un geste EXPLICITE (re-enregistrement) : un abonné
// payant supprimé à la main en Winbox ne doit pas ressusciter en silence.
// Le champ MissingOnRouter des vouchers n'est donc PAS transposé en v1.
//
// Suspension = disable du secret côté routeur (Disabled). N°294 — la
// suspension AUTO à expiration est livrée : ExpMode « disable » (défaut)
// file ppp_secret_set {disabled:true} au passage commun (enforceExpired —
// gabarit hotspot F1) ; « none » = jamais suspendu automatiquement.
// Le renouvellement F4 (extension ExpiresAt + réactivation d'un suspendu
// auto) et le récurrent (AutoRenew/RenewDays + rappels RemindDays via
// Mail/Telegram) complètent le cycle de vie de l'abonné.
//
// N°294 — Phase B (renfort tunnel, opt-in) : StaticAddress = IP distante
// STATIQUE optionnelle de l'abonné (remote-address RouterOS, unicité PAR
// ROUTEUR) ; les creds API du renfort tunnel restent les Router.Username/
// Password existants (scellés secretbox — réutilisés tels quels, aucune
// nouvelle credential).
package model

import (
	"net"
	"regexp"
	"strings"
)

// États du secret (State) — même style que VpnPeer (N°291).
const (
	PppStatePending = "pending" // création/modification/suppression en file d'agent
	PppStateActive  = "active"  // confirmé présent sur le routeur (rapport agent)
	PppStateError   = "error"   // dernier geste agent échoué (voir ErrorMsg)
)

// PppService — service RouterOS du secret. v1 = PPPoE uniquement (le
// chantier ⑥ est PPPoE ; les autres services ppp (l2tp/pptp/sstp…) ne
// sont pas au contrat).
const PppService = "pppoe"

// PppMarker — marqueur de traçabilité du commentaire routeur : TOUT secret
// créé par MikCloud porte comment="mikcloud-ppp <commentaire>" (pattern
// mikcloud-wg N°285 / mikcloud-pause N°101 : seuls les objets marqués nous
// appartiennent, le ménage et l'idempotence s'appuient dessus).
const PppMarker = "mikcloud-ppp"

// PppSecret — ligne du registre des abonnés PPPoE (table « ppp_secrets »).
type PppSecret struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	// RouterID — LE routeur porteur du pppoe-server (le secret est créé,
	// modifié et supprimé sur CE routeur uniquement).
	RouterID string `json:"routerId"`
	// Name — nom du secret PPP côté routeur (= l'identifiant de connexion
	// de l'abonné). Les FAI utilisent souvent un email@domaine : regex
	// LARGE sans caractères dangereux (jamais de " \ $ espace — ValidPppName).
	Name string `json:"name"`
	// Password — secret d'authentification PPP. Sérialisé comme
	// HotspotUser.Password (json:"password" — parité, la console affiche
	// et modifie le secret de l'abonné).
	Password string `json:"password"`
	// Profile — nom du profil PPP EXISTANT côté routeur (D2 : le cloud ne
	// crée pas les profils en v1).
	Profile string `json:"profile"`
	// Comment — commentaire libre du gérant (le commentaire ROUTEUR porte
	// en plus le marqueur PppMarker).
	Comment string `json:"comment"`
	// Service — "pppoe" (constante PppService, v1).
	Service string `json:"service"`
	// State — cycle de vie (pending/active/error).
	State string `json:"state"`
	// Disabled — suspension de l'abonné : disable du secret côté routeur
	// (manuelle via la console, ou automatique à l'échéance — voir
	// AutoSuspended).
	Disabled bool `json:"disabled"`
	// N°294 — ExpMode — politique de suspension automatique à l'échéance
	// (ExpiresAt) : ""/"disable" (défaut) = ppp_secret_set {disabled:true}
	// au passage commun ; "none" = jamais suspendu automatiquement (parité
	// ExpMode hotspot — l'action PPP est TOUJOURS disable, jamais remove :
	// un abonné PAYANT n'est pas détruit par un timer).
	ExpMode string `json:"expMode,omitempty"`
	// N°294 — AutoSuspended — la suspension ACTIVE a été appliquée
	// AUTOMATIQUEMENT à l'échéance (distincte d'une suspension manuelle :
	// le renouvellement F4 la lève, une suspension manuelle reste).
	AutoSuspended bool `json:"autoSuspended,omitempty"`
	// N°294 — Enforced — l'échéance courante a été traitée par le passage
	// commun (commande filée, renouvellement auto appliqué ou "none") :
	// appliqué UNE fois par échéance (pattern HotspotUser.Enforced F1).
	// Sérialisé : le store JSON et la réplication PG le persistance.
	Enforced bool `json:"enforced,omitempty"`
	// N°294 — Phase B — StaticAddress — IP distante STATIQUE optionnelle
	// (remote-address RouterOS) : vide = attribuée par le pool du profil.
	// Unicité PAR ROUTEUR (deux abonnés d'un même POP ne partagent jamais
	// une IP statique — PppSecretAddressTaken) ; validée IPv4 stricte.
	StaticAddress string `json:"staticAddress,omitempty"`
	// N°294 — Phase C — AutoRenew/RenewDays — récurrent : à l'échéance, le
	// passage commun PROLONGE l'abonnement de RenewDays jours (base =
	// max(maintenant, échéance)) au lieu de suspendre, et réactive un
	// suspendu automatique. Honnêteté : aucun encaissement n'est déclenché
	// (la vente reste le geste du gérant — Wave/comptant) ; le récurrent
	// automatique suppose un accord commercial prépayé avec l'abonné.
	AutoRenew bool `json:"autoRenew,omitempty"`
	RenewDays int  `json:"renewDays,omitempty"`
	// N°294 — Phase C — RemindDays/RemindedAt — rappel d'échéance : J-Remind
	// (0 = off), notification Mail/Telegram du compte (canal WhatsApp en
	// cours côté plateforme), DÉDUPLIQUÉE par échéance via RemindedAt
	// (ré-armée à chaque renouvellement — F4 comme récurrent). Jamais
	// déclenché pour un abonné déjà suspendu.
	RemindDays int    `json:"remindDays,omitempty"`
	RemindedAt string `json:"remindedAt,omitempty"`
	// LastSeenOnRouter — RFC3339 du dernier rapport agent listant ce nom
	// (parité read_state) ; vide = jamais vu sur le routeur.
	LastSeenOnRouter string `json:"lastSeenOnRouter,omitempty"`
	// ExpiresAt — échéance de l'abonnement (RFC3339) ; vide = illimité.
	// N°294 — mécanique livrée : suspension auto (ExpMode), renouvellement
	// F4 (console), récurrent (AutoRenew) et rappels (RemindDays).
	ExpiresAt string `json:"expiresAt,omitempty"`
	// ErrorMsg — dernier échec agent (résumé, JAMAIS le secret ni le mot
	// de passe).
	ErrorMsg string `json:"errorMsg,omitempty"`
	// CreatedAt / UpdatedAt — RFC3339 (model.NowISO).
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// pppNameRe — nom d'un secret PPP : minuscules, chiffres, point, underscore,
// tiret et @ (emails d'abonnés), 1-64. AUCUN caractère dangereux : jamais
// de guillemet, backslash, $ (injection d'expression RouterOS), espace ou
// séparateurs du rapport (| ; — le format de parité resterait ambigu).
var pppNameRe = regexp.MustCompile(`^[a-z0-9._@-]{1,64}$`)

// pppProfileRe — nom de profil PPP référencé : mêmes exclusions dangereuses,
// majuscules acceptées (les WISPs nomment leurs offres « ABONNE-10M »).
var pppProfileRe = regexp.MustCompile(`^[A-Za-z0-9._@-]{1,64}$`)

// ValidPppName — nom de secret PPP côté cloud ET re-vérifié par le builder
// agent (défense aux deux bords — pattern WgPeerName N°285).
func ValidPppName(s string) bool { return pppNameRe.MatchString(s) }

// ValidPppProfileName — nom de profil PPP référencé par un secret.
func ValidPppProfileName(s string) bool { return pppProfileRe.MatchString(s) }

// NormalizePppName — normalisation d'entrée (console) : trim + minuscules.
// Le nom d'abonné est insensible à la casse par convention produit (les FAI
// saisissent souvent « Abdou@FAI.fr ») ; la regex n'accepte que des
// minuscules, la normalisation garantit l'échec le plus tard possible.
func NormalizePppName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// N°294 — modes de suspension automatique (ExpMode). "" est accepté partout
// et traité comme le défaut PppExpModeDisable.
const (
	PppExpModeNone    = "none"    // jamais suspendu automatiquement
	PppExpModeDisable = "disable" // suspension auto : disable du secret
)

// PppExpModeEffective — mode effectif ("" = défaut disable) ; toute autre
// valeur inconnue retombe sur le défaut (jamais d'état bloquant imprévu).
func PppExpModeEffective(m string) string {
	if m == PppExpModeNone {
		return PppExpModeNone
	}
	return PppExpModeDisable
}

// ValidPppExpMode — valeur saisie console acceptée ("" = défaut).
func ValidPppExpMode(m string) bool {
	return m == "" || m == PppExpModeNone || m == PppExpModeDisable
}

// ValidPppStaticIP — IPv4 STRICTE pour une IP statique d'abonné (net.ParseIP
// + forme quadruplet pointé : IPv6 refusé — le parc PPPoE visé est IPv4 ;
// « 10.0.00.1 » avec zéros de remplissage refusé aussi : ce qui part au
// routeur doit être la forme canonique, pas une surprise de parseur).
func ValidPppStaticIP(s string) bool {
	ip := net.ParseIP(strings.TrimSpace(s))
	if ip == nil || ip.To4() == nil || !strings.Contains(s, ".") {
		return false
	}
	parts := strings.Split(strings.TrimSpace(s), ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if len(p) == 0 || len(p) > 3 || (len(p) > 1 && p[0] == '0') {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// PppSecretAddressTaken — l'IP statique est-elle déjà portée par un AUTRE
// abonné du MÊME routeur ? (unicité PAR ROUTEUR — deux POPs peuvent
// légitimement attribuer la même IP privée derrière leur NAT.)
func PppSecretAddressTaken(db *DB, routerID, ip, exceptID string) bool {
	for i := range db.PppSecrets {
		s := &db.PppSecrets[i]
		if s.RouterID == routerID && s.ID != exceptID && s.StaticAddress == ip {
			return true
		}
	}
	return false
}

// FindPppSecretScoped — retrouve le secret d'un compte par son ID exact
// (nil si absent ou appartenant à un autre compte — isolation multi-tenant
// vérifiée à CHAQUE résolution, même discipline que FindVpnPeerScoped).
func FindPppSecretScoped(db *DB, id, accountID string) *PppSecret {
	if id == "" {
		return nil
	}
	for i := range db.PppSecrets {
		if db.PppSecrets[i].ID == id && db.PppSecrets[i].AccountID == accountID {
			return &db.PppSecrets[i]
		}
	}
	return nil
}

// PppSecretNameTaken — le nom est-il déjà pris SUR CE ROUTEUR ? (unicité
// PAR ROUTEUR : deux POPs du même compte peuvent porter le même nom
// d'abonné, la DDL pose UNIQUE(router_id, name).)
func PppSecretNameTaken(db *DB, routerID, name string) bool {
	for i := range db.PppSecrets {
		if db.PppSecrets[i].RouterID == routerID && db.PppSecrets[i].Name == name {
			return true
		}
	}
	return false
}

// MaxPppSecretsPerRouter — plafond de secrets PPP par ROUTER (garde-fou
// Phase A : borne constante large, au-dessus du parc typique d'un WISP
// mono-POP ; le plafond PAR FORMULE suivra avec l'éditeur de formules —
// même doctrine que MaxVpnPeersPerAccount N°291). Ce plafond couvre aussi
// la pagination du rapport de parité : 400 < ReadChunkSize (500) — le
// chunk final read_state porte TOUJOURS la liste complète en un morceau.
const MaxPppSecretsPerRouter = 400
