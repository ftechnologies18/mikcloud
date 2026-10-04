// Command mikderive — imprime la CREDENTIALS_KEY (hex 64) dérivée d'un
// JWT_SECRET, via exactement le même HKDF que secretbox.Init quand
// CREDENTIALS_KEY est vide (source de vérité unique :
// internal/secretbox.DeriveCredentialsKey).
//
// Usage :
//
//	printf '%s' "$JWT_SECRET" | mikderive
//
// Pourquoi (N°244, docs/MIGRATION-ORACLE.md §8-bis) : quand
// CREDENTIALS_KEY est vide, la clé de chiffrement des identifiants
// RouterOS est DÉRIVÉE de JWT_SECRET — rotater JWT_SECRET sans épingler
// d'abord la clé dérivée rendrait les credentials chiffrés au repos
// indéchiffrables. La procédure de rotation est donc :
//
//  1. dériver la clé de l'ANCIEN JWT_SECRET (cet outil) ;
//  2. la poser comme CREDENTIALS_KEY dans /etc/mikcloud/mikcloud.env ;
//  3. ALORS SEULEMENT rotater JWT_SECRET (les sessions sont révoquées,
//     les utilisateurs se reconnectent — les mots de passe ne changent
//     pas) ;
//  4. redémarrer le service.
//
// Sécurité : le secret se lit sur STDIN (jamais en argv — invisible de
// ps et des historiques de shell) ; seule la clé hex est écrite sur
// stdout. Aucune valeur sensible ne transite dans les logs.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"mikcloud/hotspot-api/internal/secretbox"
)

func main() {
	secret, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mikderive : lecture de STDIN impossible :", err)
		os.Exit(1)
	}
	jwt := strings.TrimSpace(string(secret))
	if jwt == "" {
		fmt.Fprintln(os.Stderr, "mikderive : STDIN vide — pipez le JWT_SECRET sur l'entrée standard (cf. en-tête du fichier)")
		os.Exit(1)
	}
	key, err := secretbox.DeriveCredentialsKey(jwt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mikderive :", err)
		os.Exit(1)
	}
	fmt.Println(key)
}
