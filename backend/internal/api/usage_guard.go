// usage_guard.go — N°98, Phase 1 du produit bi-mode : garde d'usage de compte
// (Hotspot vs HomeNet).
//
// La séparation des CONSOLES (sidebars dédiées, Phase 2) ne sera jamais le
// garde : un compte HomeNet qui forgerait des appels vers /api/vouchers doit
// être refusé par le SERVEUR (défense en profondeur — même philosophie que
// requireRole : « l'UI masque, le serveur refuse »).
//
// Phase 1 = plomberie invisible : la colonne accounts.usage existe, la
// session la transporte (login/register/me), les endpoints produit HOTSPOT
// refusent les comptes homenet — mais AUCUN compte homenet n'est créable
// publiquement : l'inscription publique n'accepte que « hotspot » (le
// formulaire est inchangé), seuls la console plateforme et l'API admin
// peuvent poser « homenet » (comptes de test en attendant la coquille de
// navigation Phase 2).
//
// Exemption : les administrateurs plateforme (session support comprise) —
// le garde sépare les CLIENTS entre eux, pas l'opérateur du SaaS de ses
// comptes (même choix que guardAccountWrite pour les abonnements).
package api

import (
	"net/http"

	"mikcloud/hotspot-api/internal/model"
)

// normalizeAccountUsage — repli défensif : tout usage vide ou inconnu est lu
// comme « hotspot » (produit historique = le comportement d'avant la colonne,
// y compris pour les états non encore normalisés par migrateMultiTenant).
func normalizeAccountUsage(u string) string {
	if u == model.AccountUsageHomeNet {
		return model.AccountUsageHomeNet
	}
	return model.AccountUsageHotspot
}

// accountUsageLocked — usage du compte demandé, À APPELER LE VERROU PRIS.
// Compte introuvable → « hotspot » (comportement historique, défensif : le
// middleware refuse déjà les tokens d'un compte disparu).
func accountUsageLocked(db *model.DB, accID string) string {
	for i := range db.Accounts {
		if db.Accounts[i].ID == accID {
			return normalizeAccountUsage(db.Accounts[i].Usage)
		}
	}
	return model.AccountUsageHotspot
}

// accountUsage — usage du compte du porteur du token (sous verrou). Lecture
// À CHAQUE requête gardée, pas au login : l'usage est l'identité produit du
// compte, la bascule admin (PUT /api/admin/accounts/{id}/usage) prend effet
// immédiatement, sans attendre l'expiration des JWT (24 h).
func (a *API) accountUsage(accID string) string {
	a.store.Lock()
	defer a.store.Unlock()
	return accountUsageLocked(a.store.Data(), accID)
}

// requireUsage — autorisation SERVEUR par usage de compte : le endpoint est
// réservé aux comptes de cet usage (« hotspot » pour le produit historique ;
// « homenet » pour les futurs endpoints domestiques). Répond 404 et non 403 :
// pour le client légitime guidé par SA console, la fonctionnalité n'existe
// simplement pas — et on ne révèle ni l'existence de l'endpoint ni la
// taxonomie des comptes à un curieux. Miroir de requireRole : s'utilise en
// enveloppe (`a.requireUsage(..., a.requireRole(2, h))` — la garde d'usage
// précède la garde de rôle : le refus le moins bavard gagne).
func (a *API) requireUsage(usage string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if isPlatformAdmin(r) || a.accountUsage(accountScope(r)) == usage {
			next(w, r)
			return
		}
		writeErrCode(w, http.StatusNotFound, "wrong_usage",
			"Fonctionnalité non disponible pour ce type de compte", nil)
	}
}
