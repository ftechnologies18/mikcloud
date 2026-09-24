// serviceicons.go — N°187 — source unique de la whitelist des icônes de la
// section « Nos Services » du portail captif (introduite N°137).
//
// RÉSIDENTE DANS HOTPAGE (pas api) : c'est une donnée du DOMAINE portail —
// elle borne ce que la console peut assigner À ce que la police embarquée
// sait rendre. Le test TestPortalWebfontsCoverIcons (webfonts_test.go) ferme
// la boucle : chaque icône de cette whitelist DOIT avoir son glyphe dans
// webfonts/fa-solid-900.woff2 (via le manifeste), sinon la CI casse.
//
// HISTORIQUE DU PIÈGE (pour ne jamais le reproduire) : N°75 a amaigri le
// portail en sous-ensemblant les polices FA aux SEULES icônes alors codées
// en dur dans les 8 pages (37 glyphes). N°137 a ensuite ajouté cette
// whitelist de 20 icônes SANS régénérer la police — le CSS contenait bien
// toutes les classes, mais 14 glyphes manquaient dans le woff2 : les
// services des gérants s'affichaient avec une case à icône VIDE sur le
// portail (fa-money-bill-wave, fa-phone, fa-print, fa-store… en production).
//
// AJOUTER UNE ICÔNE : 1) l'ajouter ci-dessous, 2) régénérer la police via
// ops/portal/fa-subset.py (documenté dans TEMPLATE.md §Webfonts), 3) le
// miroir frontend PORTAL_SERVICE_ICONS (types.ts) + les traductions
// settings.svc.icon.* (i18n-fr/en). Le test gardien refuse le reste.
package hotpage

// PortalServiceIcons — N°137/N°187 — icônes Font Awesome 6 free acceptées
// pour la section « Nos Services » du portail (toutes couvertes par la
// police sous-ensemblée embarquée — gardé par test). La validation EST la
// whitelist : aucune classe arbitraire ne peut rejoindre le portail (défense
// en profondeur — le rendu hotpage échappe déjà, mais une classe inconnue
// casserait le glyphe). Miroir frontend : PORTAL_SERVICE_ICONS (types.ts).
var PortalServiceIcons = map[string]bool{
	"fa-wifi": true, "fa-globe": true, "fa-laptop": true, "fa-tools": true,
	"fa-code": true, "fa-print": true, "fa-credit-card": true, "fa-money-bill-wave": true,
	"fa-phone": true, "fa-headset": true, "fa-gamepad": true, "fa-mug-hot": true,
	"fa-utensils": true, "fa-car": true, "fa-bolt": true, "fa-store": true,
	"fa-camera": true, "fa-scissors": true, "fa-book": true, "fa-spa": true,
}
