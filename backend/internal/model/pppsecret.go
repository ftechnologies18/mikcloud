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
// Suspension = disable du secret côté routeur (Disabled) — la suspension
// automatique à expiration (ExpMode → disable) arrive avec le lot
// « suspension auto » ; le renouvellement d'abonné (ExpiresAt) est posé
// comme donnée (RFC3339, vide = illimité) dès la Phase A, sa MÉCANIQUE
// (extension F4) reste à venir.
package model

import (
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
	// (la suspension AUTO à expiration est un lot ultérieur).
	Disabled bool `json:"disabled"`
	// LastSeenOnRouter — RFC3339 du dernier rapport agent listant ce nom
	// (parité read_state) ; vide = jamais vu sur le routeur.
	LastSeenOnRouter string `json:"lastSeenOnRouter,omitempty"`
	// ExpiresAt — échéance de l'abonnement (renouvellement) ; vide =
	// illimité. Donnée dès la Phase A, mécanique (extension/relances) à venir.
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
