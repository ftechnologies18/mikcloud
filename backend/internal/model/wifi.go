// Sites WiFi publics, invités, événements promo et normalisation slug/téléphone.
// Extrait du monolithe models.go (N°88) — même package, contenu inchangé.
package model

import (
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// N°28 — WiFi Jetable : mode d'accès offert aux établissements (restaurants,
// cafés, salons de coiffure, maquis, buvettes). Le client scanne un QR code,
// laisse son numéro (marketing opt-in), reçoit un code à quota gratuit
// (limit-uptime / limit-bytes-total côté routeur) directement sur la page
// publique, puis bascule en 1 clic vers une offre payante au-delà du quota.
// ---------------------------------------------------------------------------

// WifiSite — un établissement proposant le WiFi jetable. Le slug identifie
// la page publique /wifi/{slug} encodée dans le QR code affiché sur les
// tables (UNICITÉ GLOBALE : l'URL publique ne porte pas le compte).
// FreeTimeMin / FreeDataMb = quotas gratuits surchargés à l'émission
// (0 = hériter du profil) ; DailyPerPhone / DailyCap = garde-fous anti-abus
// (budget gratuit du gérant) ; Active = bascule 1 clic du gérant.
// N°49 : WifiSSID (+ WifiPassword si le réseau est WPA) alimente le QR de
// CONNEXION de l'affiche — format universel « WIFI:T:...;S:...;P:...;; »,
// scanné par l'appareil photo (iOS 11+ / Android 10+) : le téléphone
// propose de rejoindre le réseau, puis le portail captif inline (N°48)
// prend le relais (numéro → code → en ligne). Vide = affiche limitée au
// QR page web (/wifi/{slug}).
type WifiSite struct {
	ID             string `json:"id"`
	AccountID      string `json:"accountId"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	RouterID       string `json:"routerId"`
	RouterName     string `json:"routerName"`
	ProfileID      string `json:"profileId"`
	ProfileName    string `json:"profileName"`
	FreeTimeMin    int64  `json:"freeTimeMin"`    // minutes offertes (0 = hériter profil)
	FreeDataMb     int64  `json:"freeDataMb"`     // Mo offerts (0 = hériter profil)
	MarketingOptIn bool   `json:"marketingOptIn"` // case consentement affichée
	DailyPerPhone  int    `json:"dailyPerPhone"`  // tickets max / téléphone / jour
	DailyPerMac    int    `json:"dailyPerMac"`    // N°50 — tickets max / appareil (MAC) / jour
	DailyCap       int    `json:"dailyCap"`       // budget gratuit : tickets max / site / jour
	WifiSSID       string `json:"wifiSsid"`       // N°49 — SSID du réseau du hotspot (QR de connexion, ≤ 32 car. 802.11)
	WifiPassword   string `json:"wifiPassword"`   // N°49 — mot de passe WPA (≤ 63 car., vide = réseau ouvert)
	Active         bool   `json:"active"`
	CreatedAt      string `json:"createdAt"`
}

// WifiGuest — registre marketing + anti-abus : une ligne = un code délivré.
// Day (AAAA-MM-JJ, fuseau du compte) alimente les plafonds journaliers et
// l'idempotence « même téléphone + même jour ⇒ même code ».

// WifiGuest — registre marketing + anti-abus : une ligne = un code délivré.
// Day (AAAA-MM-JJ, fuseau du compte) alimente les plafonds journaliers et
// l'idempotence « même téléphone + même jour ⇒ même code ».
type WifiGuest struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	SiteID    string `json:"siteId"`
	SiteName  string `json:"siteName"`
	Phone     string `json:"phone"` // E.164 chiffres sans "+" (ex. 2250707080909)
	OptIn     bool   `json:"optIn"`
	VoucherID string `json:"voucherId"`
	Code      string `json:"code"`
	Day       string `json:"day"`
	CreatedAt string `json:"createdAt"`
	// ClaimCmdID — N°47 : ID de la commande voucher_batch émise par le claim
	// (mode agent uniquement). Le portail l'utilise via /status (champ
	// « provisioned ») pour n'auto-loguer le visiteur qu'une fois le code
	// réellement appliqué au routeur (anti-course du check-in ≤ 45 s).
	// Vide en mode simulated/real (application immédiate).
	ClaimCmdID string `json:"claimCmdId,omitempty"`
	// N°50 — empreintes anti-abus du claim : MAC normalisée (claim depuis le
	// portail, qui injecte $(mac-esc)) et IP client (premier hop XFF). Le
	// téléphone reste la clé métier ; MAC/IP alimentent le plafond par
	// appareil (DailyPerMac) et l'audit anti-abus du gérant. Vides pour les
	// claims antérieurs au N°50 ou sans MAC (page /wifi scannée hors portail).
	Mac string `json:"mac,omitempty"`
	IP  string `json:"ip,omitempty"`
	// OptInAt — N°69 : horodatage RFC3339 du consentement marketing
	// explicite du numéro (interrupteur posé par le visiteur au claim, ou
	// bascule via POST /api/wifi/site/{slug}/consent). C'est la PREUVE
	// opposable exigée par la loi ivoirienne n°2013-450 (ARTCI) : qui,
	// quand, quoi. Vide = jamais consenti OU retrait effectué (OptIn à
	// false) — l'état courant suit le NUMÉRO (toutes les lignes du même
	// téléphone portent le même état, cf. handleWifiConsent).
	OptInAt string `json:"optInAt,omitempty"`
}

// PromoEvent — N°56 : un événement analytics du portail captif (mode
// hospitalité). Deux seuls types : "impression" (la carte promo est devenue
// visible à l'écran) et "click" (l'invité a ouvert le lien de la carte).
//
// L'ID est DÉTERMINISTE (hash de compte|promo|type|appareil|jour) : un même
// appareil ne compte qu'UNE fois par promo et par jour — re-POSTer un
// événement identique est un no-op idempotent (l'upsert Neon écrase la même
// ligne, la diff syncTable la voit inchangée). C'est le garde-fou principal
// contre le gonflement des compteurs (refresh-spam d'un invité) : la metric
// « vu 480 fois cette semaine » compte des VUES-APPAREIL-JOUR, honnêtes et
// stables, pas des rafraîchissements.

// PromoEvent — N°56 : un événement analytics du portail captif (mode
// hospitalité). Deux seuls types : "impression" (la carte promo est devenue
// visible à l'écran) et "click" (l'invité a ouvert le lien de la carte).
//
// L'ID est DÉTERMINISTE (hash de compte|promo|type|appareil|jour) : un même
// appareil ne compte qu'UNE fois par promo et par jour — re-POSTer un
// événement identique est un no-op idempotent (l'upsert Neon écrase la même
// ligne, la diff syncTable la voit inchangée). C'est le garde-fou principal
// contre le gonflement des compteurs (refresh-spam d'un invité) : la metric
// « vu 480 fois cette semaine » compte des VUES-APPAREIL-JOUR, honnêtes et
// stables, pas des rafraîchissements.
type PromoEvent struct {
	ID        string `json:"id"` // hash déterministe (voir ci-dessus)
	AccountID string `json:"accountId"`
	PromoID   string `json:"promoId"`   // id de la ligne de vitrine (stable)
	Kind      string `json:"kind"`      // "impression" | "click"
	ClientKey string `json:"clientKey"` // MAC normalisée ou "ip:x.x.x.x"
	Day       string `json:"day"`       // jour UTC "2006-01-02" (fenêtre de dédup)
	CreatedAt string `json:"createdAt"` // première occurrence (RFC3339)
}

// NormalizeWifiSlug — normalise un nom d'établissement en slug public
// (minuscules, espaces/ponctuation → tiret, trim des tirets, max 48 chars).
// Renvoie "" si aucun caractère exploitable.

// NormalizeWifiSlug — normalise un nom d'établissement en slug public
// (minuscules, espaces/ponctuation → tiret, trim des tirets, max 48 chars).
// Renvoie "" si aucun caractère exploitable.
func NormalizeWifiSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			return r
		case r == ' ' || r == '-' || r == '_' || r == '.' || r == '\'':
			return '-'
		default:
			// Accentué / autre : supprimé (é→"", è→""… suffisant pour un slug court).
			return -1
		}
	}, s)
	// collapse des tirets
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	return s
}

// NormalizeWifiPhone — normalise un téléphone visiteur : chiffres seuls
// (E.164 sans "+"). Si le numéro comporte 10 chiffres et commence par 01,
// 05 ou 07 (format local Côte d'Ivoire), il est automatiquement préfixé par
// l'indicatif 225 — les visiteurs saisissent usuellement leur numéro sans
// indicatif sur les affiches locales. Renvoie "" si invalide (8 à 15 chiffres
// après normalisation).

// NormalizeWifiPhone — normalise un téléphone visiteur : chiffres seuls
// (E.164 sans "+"). Si le numéro comporte 10 chiffres et commence par 01,
// 05 ou 07 (format local Côte d'Ivoire), il est automatiquement préfixé par
// l'indicatif 225 — les visiteurs saisissent usuellement leur numéro sans
// indicatif sur les affiches locales. Renvoie "" si invalide (8 à 15 chiffres
// après normalisation).
func NormalizeWifiPhone(phone string) string {
	var sb strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			sb.WriteRune(r)
		}
	}
	s := sb.String()
	if len(s) == 10 && (strings.HasPrefix(s, "01") || strings.HasPrefix(s, "05") || strings.HasPrefix(s, "07")) {
		s = "225" + s
	}
	if len(s) < 8 || len(s) > 15 {
		return ""
	}
	return s
}

// WifiDayKey — clé de jour (AAAA-MM-JJ) dans le fuseau du compte (tenant
// timezone) : base des plafonds journaliers du WiFi jetable. Fuseau inconnu
// → UTC (comportement neutre, sans panic).

// WifiDayKey — clé de jour (AAAA-MM-JJ) dans le fuseau du compte (tenant
// timezone) : base des plafonds journaliers du WiFi jetable. Fuseau inconnu
// → UTC (comportement neutre, sans panic).
func WifiDayKey(timezone string, t time.Time) string {
	loc, err := time.LoadLocation(timezone)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	return t.In(loc).Format("2006-01-02")
}

// JoinLink — N°27 — lien d'inscription publique (campus, écoles,
// administration, entreprise) : le gérant le crée depuis la console, la
// console l'encode en QR (URL /join/{token}). Le token est stocké côté
// serveur — révocable instantanément, compteur d'usages, expiration :
// aucun JWT, le lien FAIT l'authentification de la page publique.
