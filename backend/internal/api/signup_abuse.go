// signup_abuse.go — quota d'inscription par IP (sécurité S3 — P1 de l'audit
// pré-lancement commercial).
//
// POST /api/auth/register crée une ressource persistante lourde (compte +
// propriétaire + réglages + modèles de vouchers + profils + essai de 90
// jours). Le middleware S1-A2 (main.go) borne le DÉBIT (12 req/min par IP
// sur /api/auth/*), mais un fermier de comptes d'essai peut étaler ses
// créations dans le temps (12/min pendant des heures) et gonfler la base de
// comptes gratuits — pollution métier directe (quotas d'essai, stockage,
// notifications). Ce quota borne le VOLUME par IP :
//   - 5 tentatives par fenêtre glissante de 10 minutes (anti-burst) ;
//   - 20 tentatives par fenêtre glissante de 24 heures (anti-farm).
//
// Chaque TENTATIVE consomme le quota — succès comme échec de validation : un
// attaquant qui sonde le format ou tourne sur des usernames déjà pris est
// coupé aussi. Une IP qui heurte le quota reçoit 429 + Retry-After (même
// contrat que le verrou PIN S2).
//
// Clé = IP client (premier hop XFF, cf. clientIP d'auth_audit.go, miroir
// documenté de main.go). Limite assumée (même compromis que S1-A2) : le
// premier hop est forgeable en production (constat S1 « pass-through
// Render ») — un attaquant qui contrôle ses en-têtes peut diluer ce quota ;
// il reste rattrapé par le plafond global d'instance S1 (900 req/min,
// insensible aux en-têtes). La défense forte contre le fermage organisé
// reste REGISTER_KEY (mode bêta privée, déjà implémenté) et la surveillance
// des journaux (S2 + logs Render). Derrière un NAT partagé (cybercafé,
// marché cible), 20 inscriptions/24 h laissent une large marge aux usages
// légitimes.
//
// État en mémoire : instance unique (Render), comme le limiteur S1-A2 et le
// verrou PIN S2. Un redémarrage remet les compteurs à zéro — c'est un
// garde-fou anti-volume, pas un état durable.
package api

import (
	"sync"
	"time"
)

const (
	// signupBurstMax / signupBurstWindow — anti-burst : 5 tentatives par
	// fenêtre glissante de 10 minutes par IP. Un humain qui se trompe dans
	// le formulaire reste très loin du seuil ; un script de spam, non.
	signupBurstMax    = 5
	signupBurstWindow = 10 * time.Minute

	// signupDailyMax / signupDailyWindow — anti-farm : 20 tentatives par
	// fenêtre glissante de 24 h par IP. Rend le fermage industriel de
	// comptes d'essai économiquement inintéressant sans gêner un NAT
	// partagé légitime.
	signupDailyMax    = 20
	signupDailyWindow = 24 * time.Hour

	// signupMaxTracked — garde-fou mémoire : nombre maximal d'IP suivies
	// simultanément. Au-delà, purge d'un bloc (jamais atteint en pratique :
	// une entrée par IP qui tente une inscription, purgée paresseusement
	// après 24 h d'inactivité).
	signupMaxTracked = 10_000
)

// signupState — horodatages des tentatives récentes d'une IP (triés par
// ordre chronologique d'ajout).
type signupState struct {
	stamps []time.Time
}

// signupLimiter — table des historiques de tentatives, indexée par IP.
type signupLimiter struct {
	mu  sync.Mutex
	ips map[string]*signupState
	now func() time.Time // injectable pour les tests
}

func newSignupLimiter() *signupLimiter {
	return &signupLimiter{ips: map[string]*signupState{}, now: time.Now}
}

// allow — enregistre une tentative d'inscription pour cette IP et dit si elle
// est admise. Retourne (true, 0) si admise ; sinon (false, délai conseillé)
// où le délai alimente Retry-After : le temps avant que le compteur
// incriminé (burst ou quotidien) libère une place.
func (s *signupLimiter) allow(ip string) (bool, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if len(s.ips) >= signupMaxTracked {
		s.ips = map[string]*signupState{} // purge d'urgence (garde-fou mémoire)
	}
	st := s.ips[ip]
	if st == nil {
		st = &signupState{}
		s.ips[ip] = st
	}
	// Purge paresseuse : on ne garde que les tentatives de la fenêtre
	// quotidienne (la fenêtre burst est un sous-ensemble).
	fresh := st.stamps[:0]
	for _, t := range st.stamps {
		if now.Sub(t) < signupDailyWindow {
			fresh = append(fresh, t)
		}
	}
	st.stamps = fresh

	// Quota quotidien : au seuil, refus jusqu'à ce que la plus ancienne
	// tentative de la fenêtre glissante en sorte (st.stamps[0] — tri
	// chronologique garanti par l'ordre d'ajout).
	if len(st.stamps) >= signupDailyMax {
		return false, st.stamps[0].Add(signupDailyWindow).Sub(now)
	}
	// Fenêtre burst : comptage du suffixe récent (les tentatives des
	// signupBurstWindow dernières minutes — le tableau est trié).
	burst := 0
	for i := len(st.stamps) - 1; i >= 0 && now.Sub(st.stamps[i]) < signupBurstWindow; i-- {
		burst++
	}
	if burst >= signupBurstMax {
		// La place se libère quand la première tentative de la série
		// burst actuelle sort de la fenêtre.
		first := st.stamps[len(st.stamps)-signupBurstMax]
		return false, first.Add(signupBurstWindow).Sub(now)
	}
	st.stamps = append(st.stamps, now)
	return true, 0
}
