// password_policy.go — politique de robustesse des mots de passe console
// (sécurité S2 — P1-B4 de l'audit pré-lancement commercial).
//
// Avant S2, la seule règle était « 8 caractères minimum », dupliquée sur
// chaque point de définition : ni denylist, ni garde-fou nom d'utilisateur,
// ni borne haute. La politique est désormais centralisée ici et appliquée
// aux SIX points de définition (inscription, changement personnel, création
// et réinitialisation d'un membre d'équipe, création d'un compte client par
// la plateforme, création d'un admin plateforme). Elle ne concerne QUE la
// définition d'un nouveau mot de passe : les mots de passe existants plus
// courts restent parfaitement valides à la connexion (aucune rupture).
//
// Règles (ASVS 2.1.1 / 2.1.7 — longueur minimale et interdiction des mots
// de passe les plus courants, sans règles de composition arbitraires) :
//   - 10 caractères minimum (comptés en runes, pas en octets) ;
//   - 72 octets maximum — bcrypt tronque silencieusement au-delà : sans
//     cette borne, un mot de passe de 100 octets serait réduit à ses 72
//     premiers sans que l'utilisateur soit averti ;
//   - différent du nom d'utilisateur (quand le point d'appel l'a chargé) ;
//   - absent de la denylist des mots de passe les plus utilisés (échantillon
//     des fuites publiques les plus citées, variantes clavier FR, termes
//     propres à la plateforme).
package api

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// minPasswordLen — longueur minimale d'un nouveau mot de passe console.
// Passage de 8 à 10 (S2) : +1 caractère ajoute ~30× l'espace de recherche
// pour un attaquant hors ligne ; 10 reste sous le seuil de friction UX.
const minPasswordLen = 10

// bcryptMaxPasswordBytes — limite physique de bcrypt : au-delà de 72 octets,
// l'excès est tronqué sans erreur par l'algorithme lui-même.
const bcryptMaxPasswordBytes = 72

// passwordDenylist — mots de passe interdits (clés normalisées en minuscules,
// comparaison exacte après passage en minuscules du candidat). Échantillon
// volontairement resserré : il vise les mots de passe présents dans TOUTES
// les listes de fuites publiques, pas une police de complexité.
var passwordDenylist = map[string]struct{}{
	// Top mondial des fuites publiques (rockyou / haveibeenpwned).
	"123456": {}, "password": {}, "123456789": {}, "12345678": {},
	"12345": {}, "1234567": {}, "1234567890": {}, "qwerty": {},
	"abc123": {}, "111111": {}, "123123": {}, "000000": {},
	"iloveyou": {}, "1q2w3e4r": {}, "qwertyuiop": {}, "monkey": {},
	"dragon": {}, "letmein": {}, "login": {}, "princess": {},
	"welcome": {}, "admin": {}, "administrator": {}, "passw0rd": {},
	"password1": {}, "password123": {}, "p@ssw0rd": {}, "starwars": {},
	"master": {}, "hello": {}, "freedom": {}, "whatever": {}, "qazwsx": {},
	"trustno1": {}, "batman": {}, "superman": {}, "michael": {},
	"shadow": {}, "sunshine": {}, "football": {}, "baseball": {},
	"soccer": {}, "hockey": {}, "ranger": {}, "buster": {}, "thomas": {},
	"robert": {}, "access": {}, "love": {}, "secret": {}, "summer": {},
	"winter": {}, "charlie": {}, "jordan": {}, "hunter": {}, "asdfgh": {},
	"zxcvbn": {}, "1qaz2wsx": {}, "zaq12wsx": {},
	// Clavier / usages français courants.
	"azerty": {}, "azertyuiop": {}, "motdepasse": {}, "motdepasse1": {},
	"qsdfgh": {}, "wxcvbn": {}, "soleil": {}, "chocolat": {}, "lalala": {},
	// Métier MikCloud / écosystème MikroTik.
	"mikrotik": {}, "mikcloud": {}, "hotspot": {}, "wifi": {},
	"voucher": {}, "vouchers": {}, "routerboard": {}, "winbox": {},
	"mikcloud123": {}, "mikrotik123": {}, "hotspot123": {},
	"freelance": {}, "ftechnologies": {}, "abidjan": {}, "dakar": {},
}

// passwordPolicyViolation — retourne le message d'erreur français si le
// mot de passe ne respecte pas la politique, "" s'il est acceptable.
// username : identifiant du porteur quand le point d'appel l'a déjà chargé
// ("" sinon — le contrôle ≠ identifiant est alors simplement ignoré).
func passwordPolicyViolation(password, username string) string {
	if n := utf8.RuneCountInString(password); n < minPasswordLen {
		return fmt.Sprintf("Le mot de passe doit faire au moins %d caractères", minPasswordLen)
	}
	if len(password) > bcryptMaxPasswordBytes {
		return fmt.Sprintf("Le mot de passe ne doit pas dépasser %d octets (limite interne bcrypt)", bcryptMaxPasswordBytes)
	}
	if username != "" && strings.EqualFold(password, username) {
		return "Le mot de passe ne doit pas être identique au nom d'utilisateur"
	}
	if _, banned := passwordDenylist[strings.ToLower(password)]; banned {
		return "Ce mot de passe figure parmi les plus utilisés et est interdit — choisissez-en un unique"
	}
	return ""
}
