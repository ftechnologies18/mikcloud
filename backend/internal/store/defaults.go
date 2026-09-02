// Package store — valeurs par défaut FONCTIONNELLES offertes à chaque compte
// (aucune donnée de démonstration) :
//   - SeedProfilesFor  : le profil « Staff » par défaut (v2) ;
//   - SeedTemplatesFor : les 3 gabarits de tickets par défaut (F2).
//
// NB — le seed de démonstration (BuildSeed, ancien fichier seed.go) a été
// SUPPRIMÉ du code : plus aucune donnée de test/démo ne peut être générée,
// ni en production PostgreSQL ni en développement JSON local. Une base vide
// démarre toujours sur l'état de mise en service BuildEmptyState (empty.go).
// Les données de démo héritées de l'ancien seed se retirent de la production
// via POST /api/admin/purge-demo (api/handlers_purge.go).
package store

import (
	"mikcloud/hotspot-api/internal/model"
)

// SeedProfilesFor — le profil « Staff » par défaut de chaque compte (v2) :
// accès longue durée pour le personnel du hotspot (30 jours, 2 appareils,
// quota illimité, gratuit, sans expiration appliquée aux utilisateurs
// réguliers). Créé à l'inscription (handleRegister), à la création d'un
// compte par la plateforme (handlers_admin) et rétro-rempli en base par la
// migration staff_seeded de pg.go (une seule fois par compte).
func SeedProfilesFor(accID string) []model.Profile {
	return []model.Profile{
		{
			ID:                model.NewID("p-"),
			AccountID:         accID,
			Name:              "Staff",
			RateLimit:         "10M/10M",
			SessionTimeoutMin: 43200, // 30 jours
			SharedUsers:       2,
			ValidityDays:      30,
			Price:             0,
			DataQuotaMb:       0,
			CreatedAt:         model.NowISO(),
			ExpMode:           "notify",
			GracePeriodMin:    0,
			LockUser:          false,
			SellingPrice:      0,
			LockFirstDevice:   false,
		},
	}
}

// SeedTemplatesFor — les 3 modèles de vouchers offerts à chaque compte
// (compte principal au seed, nouveaux comptes à l'inscription). Styles INLINE
// uniquement (l'impression est hors app) ; variables substituées côté client.
func SeedTemplatesFor(accID string) []model.VoucherTemplate {
	now := model.NowISO()
	return []model.VoucherTemplate{
		{
			ID: model.NewID("tpl-"), AccountID: accID, Name: "Grille A4",
			Format: "a4", BodyHTML: templateA4Body, IsDefault: true, CreatedAt: now,
		},
		{
			ID: model.NewID("tpl-"), AccountID: accID, Name: "Ticket thermique 58 mm",
			Format: "58mm", BodyHTML: template58mmBody, IsDefault: false, CreatedAt: now,
		},
		{
			ID: model.NewID("tpl-"), AccountID: accID, Name: "Ticket thermique 80 mm",
			Format: "80mm", BodyHTML: template80mmBody, IsDefault: false, CreatedAt: now,
		},
	}
}

// templateA4Body — ticket pointillé pour impression grille A4 (3 colonnes).
const templateA4Body = `<div style="border:2px dashed #16a34a;border-radius:10px;padding:10px 12px;font-family:Arial,Helvetica,sans-serif;background:#ffffff;color:#111827;">
  <div style="display:flex;justify-content:space-between;align-items:center;border-bottom:1px solid #d1d5db;padding-bottom:6px;margin-bottom:6px;">
    <span style="font-size:12px;font-weight:bold;color:#16a34a;letter-spacing:1px;">{{hotspotName}}</span>
    <span style="font-size:9px;color:#6b7280;">{{dnsName}}</span>
  </div>
  <div style="display:flex;gap:10px;align-items:center;">
    <div style="flex:1;min-width:0;">
      <div style="font-size:8px;color:#6b7280;margin-bottom:1px;">IDENTIFIANT</div>
      <div style="font-family:'Courier New',monospace;font-size:14px;font-weight:bold;letter-spacing:1px;">{{username}}</div>
      <div style="font-size:8px;color:#6b7280;margin:4px 0 1px;">MOT DE PASSE</div>
      <div style="font-family:'Courier New',monospace;font-size:12px;font-weight:bold;letter-spacing:1px;">{{password}}</div>
    </div>
    <img src="{{qrCode}}" alt="QR" style="width:62px;height:62px;flex:none;"/>
  </div>
  <div style="display:flex;justify-content:space-between;border-top:1px solid #d1d5db;margin-top:6px;padding-top:5px;font-size:10px;">
    <span>{{profile}} &middot; {{validity}}</span>
    <span style="font-weight:bold;color:#16a34a;">{{price}}</span>
  </div>
</div>`

// template58mmBody — ticket thermique compact, 58 mm de large (54 mm utiles).
const template58mmBody = `<div style="width:54mm;border:1px dashed #111827;padding:8px 6px;font-family:Arial,Helvetica,sans-serif;font-size:11px;text-align:center;background:#ffffff;color:#000000;">
  <div style="font-weight:bold;font-size:12px;letter-spacing:1px;">{{hotspotName}}</div>
  <div style="font-size:9px;color:#444444;">{{dnsName}}</div>
  <img src="{{qrCode}}" alt="QR" style="width:22mm;height:22mm;margin:4px auto;"/>
  <div style="font-family:'Courier New',monospace;font-size:13px;font-weight:bold;">{{username}}</div>
  <div style="font-family:'Courier New',monospace;font-size:11px;">{{password}}</div>
  <div style="border-top:1px dashed #999999;margin-top:5px;padding-top:4px;font-size:10px;">
    {{profile}} &middot; {{validity}}<br/>
    <span style="font-weight:bold;">{{price}}</span>
  </div>
</div>`

// template80mmBody — ticket thermique large 80 mm (76 mm utiles), 2 colonnes.
const template80mmBody = `<div style="width:76mm;border:1px dashed #111827;padding:10px 8px;font-family:Arial,Helvetica,sans-serif;font-size:12px;background:#ffffff;color:#000000;">
  <div style="display:flex;justify-content:space-between;align-items:baseline;border-bottom:1px solid #dddddd;padding-bottom:4px;">
    <span style="font-weight:bold;font-size:13px;letter-spacing:1px;">{{hotspotName}}</span>
    <span style="font-size:9px;color:#444444;">{{dnsName}}</span>
  </div>
  <div style="display:flex;gap:10px;align-items:center;margin-top:6px;">
    <img src="{{qrCode}}" alt="QR" style="width:24mm;height:24mm;flex:none;"/>
    <div style="flex:1;min-width:0;text-align:left;">
      <div style="font-size:9px;color:#666666;">IDENTIFIANT</div>
      <div style="font-family:'Courier New',monospace;font-size:15px;font-weight:bold;">{{username}}</div>
      <div style="font-size:9px;color:#666666;margin-top:4px;">MOT DE PASSE</div>
      <div style="font-family:'Courier New',monospace;font-size:13px;font-weight:bold;">{{password}}</div>
    </div>
  </div>
  <div style="display:flex;justify-content:space-between;border-top:1px dashed #999999;margin-top:6px;padding-top:4px;font-size:11px;">
    <span>{{profile}} &middot; {{validity}}</span>
    <span style="font-weight:bold;">{{price}}</span>
  </div>
</div>`
