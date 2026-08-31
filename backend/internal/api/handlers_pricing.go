// Package api — répercussion des frais de paiement GeniusPay sur les clients
// (stratégie tarifaire VALIDÉE — « prix de liste + remise mobile money ») :
//
//   - le PRIX DE LISTE couvre les frais CARTE (Stripe via GeniusPay :
//     5 % + 1 % + 100 XOF) : montant débité en GROSS-UP, arrondi au multiple
//     de 25 XOF supérieur — la plateforme encaisse exactement son prix
//     catalogue, net de frais, à chaque échéance ;
//   - Wave (1,5 % + 1 % + 100 XOF) bénéficie d'une REMISE MOBILE MONEY de
//     3 % sur le prix de liste : le client voit un avantage (jamais une
//     surcharge — interdite par les règles Stripe), la plateforme reste
//     au-dessus de son net cible, et le client est incité à choisir le
//     moyen le moins coûteux ;
//   - les activations MANUELLES de la plateforme (encaissement hors ligne,
//     résolution de la file billing) restent au prix NET : exemption
//     administrative — les frais ne s'appliquent qu'aux paiements en ligne.
//
// Formules (net cible N, frais fixe F = 100, taux total t) :
//
//	prix de liste = ceil_25((N + F) / (1 − t_carte))          (t_carte = 6 %)
//	prix Wave     = ceil_25(prix de liste × (1 − 0,03))
//
// Exemples : N = 1 250 → liste 1 450 / Wave 1 420 ; N = 12 000 → liste
// 12 875 / Wave 12 500 — dans tous les cas le net reçu ≥ N (gross-up).
package api

import (
	"math"

	"mikcloud/hotspot-api/internal/model"
)

// Frais de traitement GeniusPay (compte marchand live) — répercutés au client.
const (
	// payFeesFixedFcfa — frais fixe par transaction (Wave et carte).
	payFeesFixedFcfa = 100
	// payFeesCardRate — taux total carte (Stripe via GeniusPay) : 5 % + 1 %.
	payFeesCardRate = 0.06
	// payFeesWaveRate — taux total Wave : 1,5 % + 1 %.
	payFeesWaveRate = 0.025
	// payFeesWaveDiscount — remise mobile money appliquée au prix de liste
	// quand le client paie par Wave.
	payFeesWaveDiscount = 0.03
	// payFeesRoundStep — pas d'arrondi des montants à débiter (XOF).
	payFeesRoundStep = 25
)

// roundUpStep — arrondi au multiple de payFeesRoundStep supérieur.
func roundUpStep(v float64) int {
	return int(math.Ceil(v/payFeesRoundStep)) * payFeesRoundStep
}

// grossUpFcfa — montant à débiter pour toucher exactement « net » après
// déduction du taux proportionnel « rate » et du frais fixe :
//
//	débit = ceil_25((net + fixe) / (1 − taux))
func grossUpFcfa(net int, rate float64) int {
	return roundUpStep((float64(net) + payFeesFixedFcfa) / (1 - rate))
}

// cardListPriceFcfa — prix de liste (paiement carte) : gross-up des frais carte.
func cardListPriceFcfa(net int) int { return grossUpFcfa(net, payFeesCardRate) }

// wavePayPriceFcfa — montant payé par Wave : prix de liste moins la remise
// mobile money (arrondi 25 XOF supérieur — jamais sous le net cible).
func wavePayPriceFcfa(list int) int {
	return roundUpStep(float64(list) * (1 - payFeesWaveDiscount))
}

// planPricing — tarification complète d'une période : net cible plateforme
// (prix catalogue), montant carte (prix de liste) et montant Wave (remise
// mobile money incluse). Exposée au client pour un choix éclairé du moyen.
type planPricing struct {
	BaseFcfa int `json:"baseFcfa"` // net cible — prix catalogue
	ListFcfa int `json:"listFcfa"` // carte — prix de liste (frais inclus)
	WaveFcfa int `json:"waveFcfa"` // Wave — remise mobile money −3 % incluse
}

// payPricingOf — tarification pour un net cible donné.
func payPricingOf(base int) planPricing {
	list := cardListPriceFcfa(base)
	return planPricing{BaseFcfa: base, ListFcfa: list, WaveFcfa: wavePayPriceFcfa(list)}
}

// planPricingOfPlan — tarification d'une formule pour une assiette routeurs.
func planPricingOfPlan(p model.SaasPlan, routerCount int) planPricing {
	return payPricingOf(planAmount(p, routerCount))
}

// requestPricingBase — base (net cible) d'une demande de facturation : la
// base stockée, ou le montant historique pour les demandes antérieures à la
// répercussion des frais (leur montant valait alors le net catalogue).
func requestPricingBase(br model.BillingRequest) int {
	if br.BaseAmountFcfa > 0 {
		return br.BaseAmountFcfa
	}
	return br.AmountFcfa
}

// wavePriceOfRequest — montant WAVE d'une demande (gross-up + remise sur la
// base de la demande).
func wavePriceOfRequest(br model.BillingRequest) int {
	return payPricingOf(requestPricingBase(br)).WaveFcfa
}

// listPriceOfRequest — montant CARTE (prix de liste) d'une demande.
func listPriceOfRequest(br model.BillingRequest) int {
	return payPricingOf(requestPricingBase(br)).ListFcfa
}

// payMethodLabel — libellé français du moyen de paiement d'une demande
// (journaux de facturation : moyen effectivement payé ou initié).
func payMethodLabel(m string) string {
	if m == "card" {
		return "carte bancaire"
	}
	return "Wave"
}
