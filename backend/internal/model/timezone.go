// Timezone de compte — résolution centralisée côté modèle.
//
// N°199 : l'accumulateur journalier de volume (AccumulateVolumeDay) vit dans
// le package model mais découpe les jours AU FUSEAU DU COMPTE (même doctrine
// que les périodes calendaires des rapports, N°198). Le moteur de simulation
// (store.Tick) doit en faire autant — or la résolution était une fonction
// privée du package api (accountTimezone), inaccessible depuis store et
// model. Elle déménage ici, SANS duplication : api.accountTimezone délègue.
package model

import "time"

// AccountTimezone — fuseau du compte (Tenant.Timezone), repli UTC si le
// compte est inconnu ou le fuseau invalide (time/tzdata embarqué dans le
// binaire : LoadLocation ne peut pas échouer en production).
func AccountTimezone(db *DB, accID string) *time.Location {
	if s, ok := db.SettingsByAccount[accID]; ok && s.Tenant.Timezone != "" {
		if loc, err := time.LoadLocation(s.Tenant.Timezone); err == nil {
			return loc
		}
	}
	return time.UTC
}
