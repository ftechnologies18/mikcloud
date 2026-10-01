// oui_lookup.go — N°197 — résolution de la MARQUE PROBABLE d'un appareil
// depuis le préfixe constructeur (OUI) de son adresse MAC, via la table
// générée oui_vendors.go (registres publics IEEE MA-L / MA-M / MA-S).
//
// Pourquoi un lookup LOCAL et non un service web : la marque est un simple
// indice d'affichage sur la carte « détails de connexion » de la vue
// Sessions — aucun appel réseau, aucune latence, aucune donnée qui sort
// (la MAC d'un client ne doit JAMAIS quitter le cloud pour un service
// tiers : confidentialité par conception).
package api

import "strings"

// macVendor — résout la marque probable d'un appareil depuis sa MAC.
// Retourne "" quand le préfixe est inconnu de la table (marque non
// pertinente pour la console ou non enregistrée) : l'appelant affiche
// « — », jamais une déduction risquée.
//
// Les trois registres IEEE sont interrogés du plus spécifique au plus
// court : MA-S (36 bits, 9 hex), MA-M (28 bits, 7 hex), MA-L (24 bits,
// 6 hex) — un préfixe long bat toujours un court (Apple a des MA-S qui
// commencent comme le MA-L d'un autre constructeur).
func macVendor(mac string) string {
	digits := make([]byte, 0, 9)
	for _, c := range strings.ToUpper(strings.TrimSpace(mac)) {
		var d byte
		switch {
		case c >= '0' && c <= '9':
			d = byte(c)
		case c >= 'A' && c <= 'F':
			d = byte(c)
		default:
			continue // séparateurs « : », « - », «. » et tout parasite
		}
		digits = append(digits, d)
		if len(digits) == 9 {
			break
		}
	}
	for _, n := range []int{9, 7, 6} {
		if len(digits) < n {
			continue
		}
		if brand, ok := ouiVendors[string(digits[:n])]; ok {
			return brand
		}
	}
	return ""
}
