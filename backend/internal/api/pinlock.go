// pinlock.go — verrouillage progressif du PIN revendeur par compte
// (sécurité S2 — P1-B2 de l'audit pré-lancement commercial).
//
// Le PIN Mode Vente (4-6 chiffres) forme un espace de recherche minuscule :
// au plus 1 111 100 combinaisons. Le limiteur S1-A2 borne chaque IP
// (5 req/min sur /api/reseller/login), mais une attaque DISTRIBUÉE — une IP
// forgeable par requête, cf. le suivi S1 « pass-through Render » — restait
// bornée par le seul plafond global d'instance (900 req/min) : l'espace PIN
// d'un compte ciblé pouvait être épuisé en ~20 minutes. Le verrou est donc
// adossé au COMPTE ciblé (clé = ID interne du revendeur), pas à l'IP :
// après 5 échecs consécutifs, TOUTE tentative — même avec le bon PIN, même
// depuis une IP neuve — est refusée pendant 15 minutes. Un succès efface
// l'historique du revendeur.
//
// Contrepartie assumée (déni de service) : un attaquant peut verrouiller le
// PIN d'un revendeur légitime pendant 15 minutes. L'impact est borné : le
// gérant peut (re)définir le PIN depuis la console à tout moment, et les
// vouchers en stock restent vendables par le gérant lui-même.
//
// État en mémoire : le service tourne en instance unique (Render), comme le
// limiteur S1-A2. Un redémarrage remet les compteurs à zéro — le verrou est
// une deuxième ligne de défense derrière les limiteurs par IP de S1, pas un
// état durable.
package api

import (
	"sync"
	"time"
)

const (
	// pinMaxFailures — échecs consécutifs tolérés avant verrou : 5 essais
	// laissent une marge aux fautes de frappe humaines (paysans en tournée,
	// clavier tactile) tout en réduisant à ~1/1 100 000 la chance de deviner
	// un PIN par fenêtre de 15 minutes.
	pinMaxFailures = 5

	// pinLockoutDuration — durée du verrou déclenché au 5e échec consécutif.
	pinLockoutDuration = 15 * time.Minute

	// pinMaxTracked — garde-fou mémoire : nombre maximal de revendeurs
	// suivis simultanément. Au-delà, la table est purgée d'un bloc (jamais
	// atteint en pratique : une entrée par revendeur ciblé par des échecs,
	// retirée au succès ou après expiration du verrou).
	pinMaxTracked = 10_000
)

// pinFailState — historique d'échecs d'un revendeur ciblé.
type pinFailState struct {
	count       int       // échecs consécutifs (remis à zéro par un succès)
	lockedUntil time.Time // zéro = jamais verrouillé
}

// pinLimiter — table des historiques d'échecs PIN, indexée par ID revendeur.
type pinLimiter struct {
	mu    sync.Mutex
	fails map[string]*pinFailState
	now   func() time.Time // injectable pour les tests
}

func newPinLimiter() *pinLimiter {
	return &pinLimiter{fails: map[string]*pinFailState{}, now: time.Now}
}

// check — le verrou est-il actif pour ce revendeur ? Retourne (verrouillé,
// durée restante). Ne modifie aucun compteur : appelé AVANT la vérification
// bcrypt, il court-circuite aussi le coût CPU du hachage sous attaque.
func (p *pinLimiter) check(resellerID string) (bool, time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.fails[resellerID]
	if st == nil {
		return false, 0
	}
	if remaining := st.lockedUntil.Sub(p.now()); remaining > 0 {
		return true, remaining
	}
	return false, 0
}

// fail — enregistre un échec de PIN pour ce revendeur. Retourne (verrouillé,
// durée restante) : verrouillé vaut true si CET échec déclenche le verrou
// (5e échec consécutif) — le handler peut alors compter sur check() pour les
// tentatives suivantes.
func (p *pinLimiter) fail(resellerID string) (bool, time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	if len(p.fails) >= pinMaxTracked {
		p.fails = map[string]*pinFailState{} // purge d'urgence (garde-fou mémoire)
	}
	st := p.fails[resellerID]
	if st == nil {
		st = &pinFailState{}
		p.fails[resellerID] = st
	}
	// Verrou expiré depuis la dernière visite : le compteur repart de zéro.
	if !st.lockedUntil.IsZero() && !st.lockedUntil.After(now) {
		st.count = 0
	}
	st.count++
	if st.count >= pinMaxFailures {
		st.lockedUntil = now.Add(pinLockoutDuration)
		return true, pinLockoutDuration
	}
	return false, 0
}

// reset — succès de connexion : l'historique d'échecs du revendeur disparaît.
func (p *pinLimiter) reset(resellerID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.fails, resellerID)
}
