// Liens d'invitation et demandes d'inscription.
// Extrait du monolithe models.go (N°88) — même package, contenu inchangé.
package model

// JoinLink — N°27 — lien d'inscription publique (campus, écoles,
// administration, entreprise) : le gérant le crée depuis la console, la
// console l'encode en QR (URL /join/{token}). Le token est stocké côté
// serveur — révocable instantanément, compteur d'usages, expiration :
// aucun JWT, le lien FAIT l'authentification de la page publique.
type JoinLink struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	Name      string `json:"name"` // ex. « Rentrée 2026 — Bâtiment A »
	Token     string `json:"token"`
	// Pré-attribution optionnelle : profil et routeur imposés par le lien
	// (validation 1 clic). autoValidate exige les deux (mode kiosque).
	ProfileID     string `json:"profileId,omitempty"`
	ProfileName   string `json:"profileName,omitempty"`
	RouterID      string `json:"routerId,omitempty"`
	RouterName    string `json:"routerName,omitempty"`
	AutoValidate  bool   `json:"autoValidate"`
	MaxUses       int    `json:"maxUses"` // nombre max de SOUMISSIONS, 0 = illimité
	Uses          int    `json:"uses"`
	ExpiresAt     string `json:"expiresAt,omitempty"`
	Revoked       bool   `json:"revoked"`
	CreatedBy     string `json:"createdBy,omitempty"`
	CreatedByName string `json:"createdByName,omitempty"`
	CreatedAt     string `json:"createdAt"`
}

// RegistrationRequest — N°27 — demande d'inscription publique en attente de
// validation par le gérant. L'utilisateur choisit son couple « nom
// d'utilisateur & mot de passe » (mode RÉGULIER, distinct des vouchers qui
// restent verrouillés username = password). Le mot de passe n'est conservé
// que le temps de la décision : VIDÉ à l'approbation comme au refus.

// RegistrationRequest — N°27 — demande d'inscription publique en attente de
// validation par le gérant. L'utilisateur choisit son couple « nom
// d'utilisateur & mot de passe » (mode RÉGULIER, distinct des vouchers qui
// restent verrouillés username = password). Le mot de passe n'est conservé
// que le temps de la décision : VIDÉ à l'approbation comme au refus.
type RegistrationRequest struct {
	ID              string `json:"id"`
	AccountID       string `json:"accountId"`
	LinkID          string `json:"linkId"`
	LinkName        string `json:"linkName"`
	FullName        string `json:"fullName"`
	Phone           string `json:"phone"`
	DesiredUsername string `json:"desiredUsername"`
	Password        string `json:"password"`
	Message         string `json:"message,omitempty"` // message libre de l'utilisateur
	Status          string `json:"status"`            // pending | approved | rejected
	RejectionReason string `json:"rejectionReason,omitempty"`
	ReviewedBy      string `json:"reviewedBy,omitempty"`
	ReviewedByName  string `json:"reviewedByName,omitempty"`
	ReviewedAt      string `json:"reviewedAt,omitempty"`
	UserID          string `json:"userId,omitempty"` // utilisateur créé à l'approbation
	CreatedIP       string `json:"createdIp,omitempty"`
	CreatedMac      string `json:"createdMac,omitempty"` // N°33 — MAC de l'appareil (?mac= page login routeur), si fournie
	CreatedAt       string `json:"createdAt"`
}

// DB — base de données persistée en JSON.
//   - Accounts/SettingsByAccount : modèle multi-tenant (source de vérité) ;
//   - Tenant/Settings : champs LEGACY mono-tenant, uniquement lus pour migrer
//     un ancien db.json — vidés après migration puis ignorés.
