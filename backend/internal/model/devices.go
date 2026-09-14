// Device — N°101 — appareil du foyer (Phase 3 Hotspot/HomeNet).
//
// Un appareil domestique est identifié par le bail DHCP que lui accorde la
// box (/ip dhcp-server lease, rapporté par la commande read_dhcp — fin de la
// dépendance aux sessions hotspot : un foyer n'a PAS de portail captif, ses
// appareils rejoignent le WiFi et reçoivent un bail). L'identité STABLE est
// la MAC (l'IP tourne au gré des renouvellements, le host-name dépend du
// constructeur) ; la famille lui affecte un NOM (« TV du salon », « Tel de
// mama ») — le registre des noms vit côté cloud, jamais sur le routeur.
//
// Pause dîner (N°101) : Paused/PausedUntil portent l'ÉTAT DÉSIRÉ calculé par
// le cloud (pattern FamilyGuard N°82 — l'horloge de référence est le cloud,
// en UTC == heure d'Abidjan) ; la commande agent device_pause fait converger
// le routeur (règles filter marquées mikcloud-pause, remove-then-add).
package model

import (
	"strings"
	"time"
)

// Statuts de bail DHCP tels que rapportés par RouterOS (champ status de
// /ip dhcp-server lease) + « gone » : absent du dernier RAPPORT COMPLET
// (le bail a été retiré du routeur — l'appareil n'est plus dans la maison
// ou s'est vu retirer son bail ; la ligne est conservée pour le nom).
const (
	DeviceLeaseBound    = "bound"    // bail actif — appareil en ligne
	DeviceLeaseWaiting  = "waiting"  // en attente de confirmation
	DeviceLeaseOffered  = "offered"  // offre envoyée, pas acceptée
	DeviceLeaseBusy     = "busy"     // adresse déjà prise (conflit)
	DeviceLeaseDeclined = "declined" // refusée par le client
	DeviceLeaseExpired  = "expired"  // expirée côté routeur
	DeviceLeaseGone     = "gone"     // absente du dernier rapport complet
)

// DeviceOnline — vrai si le dernier bail rapporté est actif (bound) :
// l'appareil est chez lui ET en ligne. Tout autre statut (waiting, expired,
// gone…) s'affiche « Hors ligne » — honnêteté du rapport, jamais déduite.
func DeviceOnline(status string) bool {
	return status == DeviceLeaseBound
}

// Device — ligne du registre des appareils (table « devices »).
type Device struct {
	ID         string `json:"id"`
	AccountID  string `json:"accountId"`
	RouterID   string `json:"routerId"`
	RouterName string `json:"routerName"`
	// MAC — identité stable, NORMALISÉE (NormalizeMAC : XX:XX:XX:XX:XX:XX
	// majuscules). Clé d'upsert par routeur.
	MAC string `json:"mac"`
	// Name — nom affecté par la famille (« TV du salon »). Vide = l'UI
	// replie sur host-name puis MAC. 48 caractères max (miroir SanitizeName).
	Name string `json:"name"`
	// Hostname — host-name DHCP rapporté par l'appareil (souvent vide ou
	// générique : « android-21a3… »). Conservé tel quel, ≤ 64 caractères.
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
	// Status — dernier statut de bail rapporté (const DeviceLease*).
	Status string `json:"status"`
	// Expires — durée brute RouterOS du bail (« 30m »), affichée telle quelle.
	Expires string `json:"expires"`
	// LeaseAt — horodatage du rapport agent qui a nourri ces champs.
	LeaseAt   string `json:"leaseAt"`
	CreatedAt string `json:"createdAt"`
	// Pause dîner (N°101) — état DÉSIRÉ côté cloud.
	Paused      bool   `json:"paused"`
	PausedUntil string `json:"pausedUntil"` // RFC3339 ; "" = illimité (jusqu'à réactivation)
}

// PauseActiveAt — vrai si la pause est effective à l'instant `now` :
// demandée (Paused) ET non expirée (PausedUntil vide = illimité). Même
// sémantique cloud-calculée que FamilyGuard.ActiveAt : le cloud décide,
// le routeur converge au check-in suivant. Repli PRUDENT sur échéance
// illisible : pause ILLIMITÉE — couper trop longtemps se répare d'un clic,
// l'inverse mentirait à un parent qui croit son enfant protégé (la valeur
// n'est écrite que par le serveur en RFC3339 : illisible = corruption).
func (d *Device) PauseActiveAt(now time.Time) bool {
	if !d.Paused {
		return false
	}
	if d.PausedUntil == "" {
		return true
	}
	until, err := time.Parse(time.RFC3339, d.PausedUntil)
	if err != nil {
		return true
	}
	return now.Before(until)
}

// NormalizeMAC — « aa:bb:cc:dd:ee:ff » → « AA:BB:CC:DD:EE:FF » ; "" si la
// forme n'est pas exactement 12 chiffres hexadécimaux séparés par 5 « : »
// (format RouterOS). Défensive : un rapport corrompu ne crée JAMAIS de
// ligne d'appareil fantôme.
func NormalizeMAC(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) != 17 {
		return ""
	}
	for i, c := range s {
		if i%3 == 2 { // positions des « : »
			if c != ':' {
				return ""
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F') {
			return ""
		}
	}
	return s
}

// DeviceNameMax — borne du nom affecté (miroir de la validation handler).
const DeviceNameMax = 48

// SanitizeDeviceName — trim + borne du nom affecté PAR RUNES (les caractères
// exotiques restent autorisés : un nom de famille s'écrit avec des accents,
// il ne part JAMAIS vers le routeur — registre purement cloud).
func SanitizeDeviceName(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > DeviceNameMax {
		runes = runes[:DeviceNameMax]
	}
	return string(runes)
}
