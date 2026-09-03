// auth_audit.go — journal des échecs d'authentification
// (sécurité S2 — P1 de l'audit pré-lancement commercial).
//
// Avant S2, un échec de connexion (console ou PIN revendeur) ne laissait
// AUCUNE trace exploitable : impossible de détecter une attaque en cours ou
// d'en mesurer l'ampleur a posteriori (les buckets du limiteur S1-A2 sont
// éphémères par nature). Désormais, chaque échec émet UNE ligne JSON sur la
// sortie standard du service — persistée par Render (logs retenus 7 jours),
// agrégeable par tout collecteur et lisible par jq :
//
//	{"event":"auth_failure","at":"2026-09-05T10:12:33Z","ip":"203.0.113.7",
//	 "kind":"console","login":"gerant1","reason":"bad_password","ua":"…"}
//
// RÈGLE D'OR : le journal ne change RIEN aux réponses HTTP. Les messages
// génériques anti-énumération (« Identifiants invalides ») sont conservés ;
// la raison fine (compte inconnu, mauvais mot de passe, verrou actif…) n'a
// d'existence que côté serveur, jamais dans la réponse — aucun oracle
// d'énumération n'est introduit par la journalisation.
package api

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// authFailure — entrée de journal d'échec d'authentification.
type authFailure struct {
	Event  string `json:"event"`        // toujours « auth_failure »
	At     string `json:"at"`           // RFC3339 UTC
	IP     string `json:"ip"`           // client (premier hop XFF, cf. clientIP)
	Kind   string `json:"kind"`         // « console » | « reseller_pin »
	Login  string `json:"login"`        // identifiant SOUMIS (non vérifié)
	Reason string `json:"reason"`       // unknown_user | bad_password | disabled | unknown_reseller | bad_pin | locked | reseller_disabled | account_disabled
	Agent  string `json:"ua,omitempty"` // User-Agent brut (omis si vide)
}

// logAuthFailure — émet la ligne de journal d'un échec d'authentification.
// Helper unique (testable en capturant la sortie standard de log).
func (a *API) logAuthFailure(r *http.Request, kind, login, reason string) {
	entry := authFailure{
		Event:  "auth_failure",
		At:     time.Now().UTC().Format(time.RFC3339),
		IP:     clientIP(r),
		Kind:   kind,
		Login:  login,
		Reason: reason,
		Agent:  r.UserAgent(),
	}
	if b, err := json.Marshal(entry); err == nil {
		log.Printf("%s", b)
	}
}

// clientIP — l'adresse client réelle : PREMIER hop de X-Forwarded-For
// (convention Render : le proxy de confiance écrit l'IP réelle du client en
// tête de chaîne, puis ajoute ses hops internes — le dernier hop tourne
// d'une requête à l'autre et fragmentait les buckets du limiteur, cf. le
// suivi S1-A2), sinon l'hôte de RemoteAddr sans le port source.
//
// Miroir délibéré de clientIP() dans main.go (paquet main) : le paquet api
// ne peut pas importer main, et déplacer la fonction ici aurait étendu le
// périmètre S2 au middleware S1 et à ses tests. Toute évolution future de
// l'une DOIT être répercutée sur l'autre (les deux sont couvertes par des
// tests de premier hop).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
