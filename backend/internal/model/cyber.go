// CyberPoste — N°290 — un poste de cybercafé (module Cybercafé, overlay du
// mode hotspot).
//
// Un poste est un appareil du cybercafé (PC filaire du bar à écrans, machine
// du client) identifié par sa MAC — la même identité STABLE que les appareils
// HomeNet (N°101) : l'IP tourne, la MAC reste. Le registre vit côté cloud
// (jamais sur le routeur) et se nourrit de deux sources : la saisie manuelle
// du gérant et l'import des bails DHCP déjà rapportés par read_dhcp (les
// comptes hotspot du module bénéficient de la même cadence d'inventaire que
// les foyers — voir le garde élargi de ensureHomeDevicesLocked).
//
// Pause poste : Paused/PausedUntil portent l'ÉTAT DÉSIRÉ calculé par le cloud
// (pattern N°101/N°82 — l'horloge de référence est le cloud, en UTC) ; la
// commande agent device_pause fait converger le routeur. L'ensemble désiré
// fusionne appareils foyers ET postes cyber dans desiredPauseMacsLocked —
// même marqueur mikcloud-pause, même idempotence remove-then-add.
//
// Attribution code-temps : ActiveUserID/ActiveUsername portent le lien vers
// le voucher (limit-uptime) créé pour CE poste — un code = un poste. La
// caisse (Transaction + Sale) est enregistrée à l'attribution, visible dans
// les rapports/journaux existants, canal « direct », trace d'origine
// SoldVia = « cyber_poste » (pattern anti-vol N°8).
//
// Le module est ACTIVABLE par compte (settings.Tenant.CyberEnabled, défaut
// OFF) — décision D6 de N°289 : un flag, PAS un nouvel usage (l'enum
// hotspot|homenet reste inchangé).
package model

import "time"

// CyberPoste — ligne du registre des postes (table « cyber_postes »).
type CyberPoste struct {
	ID         string `json:"id"`
	AccountID  string `json:"accountId"`
	RouterID   string `json:"routerId"`
	RouterName string `json:"routerName"`
	// MAC — identité stable, NORMALISÉE (NormalizeMAC : XX:XX:XX:XX:XX:XX).
	// Unique par (compte, routeur) — même discipline que les appareils N°101.
	MAC string `json:"mac"`
	// Name — nom affecté par le gérant (« PC-1 », « Machine fenêtre »). Vide =
	// l'UI replie sur le host-name puis la MAC. Même borne que les appareils
	// (SanitizeDeviceName, 48 caractères) — registre purement cloud.
	Name string `json:"name"`
	// IP — dernière IP connue (bail DHCP rapporté, informationnel).
	IP string `json:"ip"`
	// Pause poste (N°290) — état DÉSIRÉ côté cloud.
	Paused      bool   `json:"paused"`
	PausedUntil string `json:"pausedUntil"` // RFC3339 ; "" = illimité
	// Code-temps attribué (un code = un poste) — lien vers le voucher créé
	// par POST /api/cyber/postes/{id}/assign. Vide = poste libre.
	ActiveUserID      string `json:"activeUserId"`
	ActiveUsername    string `json:"activeUsername"`
	ActiveProfileName string `json:"activeProfileName"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt"`
}

// PauseActiveAt — vrai si la pause est effective à l'instant `now` :
// sémantique IDENTIQUE à Device.PauseActiveAt (demandée ET non expirée ;
// échéance illisible = pause ILLIMITÉE — couper trop longtemps se répare
// d'un clic, l'inverse mentirait au gérant).
func (p *CyberPoste) PauseActiveAt(now time.Time) bool {
	if !p.Paused {
		return false
	}
	if p.PausedUntil == "" {
		return true
	}
	until, err := time.Parse(time.RFC3339, p.PausedUntil)
	if err != nil {
		return true
	}
	return now.Before(until)
}

// FindCyberPosteScoped — retrouve le poste d'un compte par son ID exact
// (nil si absent ou appartenant à un autre compte — l'isolation multi-tenant
// se vérifie à CHAQUE résolution, même discipline que FindSiteScoped).
func FindCyberPosteScoped(db *DB, id, accountID string) *CyberPoste {
	if id == "" {
		return nil
	}
	for i := range db.CyberPostes {
		if db.CyberPostes[i].ID == id && db.CyberPostes[i].AccountID == accountID {
			return &db.CyberPostes[i]
		}
	}
	return nil
}

// MaxCyberPostesPerAccount — plafond de postes par compte (un cybercafé
// réel compte 10-50 machines ; 100 borne la table avec une large marge).
const MaxCyberPostesPerAccount = 100

// CyberPosteLabel — libellé d'affichage : le nom du gérant, sinon la MAC.
func CyberPosteLabel(p CyberPoste) string {
	if p.Name != "" {
		return p.Name
	}
	return p.MAC
}
