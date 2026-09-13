// Utilisateurs admin, notifications et réinitialisations de mot de passe.
// Extrait du monolithe models.go (N°88) — même package, contenu inchangé.
package model

// AdminUser — compte d'accès à la console (login), rattaché à un compte SaaS.
type AdminUser struct {
	ID           string `json:"id"`
	AccountID    string `json:"accountId"`
	Name         string `json:"name"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	PasswordHash string `json:"passwordHash"`
	Salt         string `json:"salt"`
	CreatedAt    string `json:"createdAt"`
	// PasswordSetByUser — true quand le mot de passe a été modifié par
	// l'utilisateur via POST /api/auth/password : applyAdminOverride ne
	// l'écrase alors PAS (sauf si la variable ADMIN_PASSWORD change).
	PasswordSetByUser bool `json:"passwordSetByUser,omitempty"`
	// EnvPasswordHash — hash du DERNIER mot de passe appliqué par la
	// variable d'environnement ADMIN_PASSWORD. Sert à détecter un
	// changement d'intention de l'opérateur (env modifiée) par rapport à
	// un mot de passe changé par l'utilisateur depuis la console.
	EnvPasswordHash string `json:"envPasswordHash,omitempty"`
	// SessionEpoch — compteur de révocation des sessions (sécurité S1-A3).
	// Incrémenté à chaque opération sensible (changement de mot de passe,
	// réinitialisation par l'owner, changement de rôle) : tout token JWT
	// portant un claim « ver » ≠ SessionEpoch est refusé IMMÉDIATEMENT par
	// le middleware — sans attendre l'expiration naturelle (24 h). La
	// suppression du membre rend l'utilisateur introuvable : refus aussi.
	// Valeur 0 = aucune révocation (compatible tokens antérieurs au
	// correctif, décodés avec ver=0).
	SessionEpoch int `json:"sessionEpoch,omitempty"`
	// TOTPSecret — secret 2FA (base32, RFC 6238 — sécurité S4). JAMAIS
	// sérialisé en JSON (tag « - ») : il n'apparaît que dans la réponse de
	// /api/auth/2fa/setup, au moment du pairage.
	TOTPSecret string `json:"-"`
	// TOTPEnabled — 2FA active : le login exige alors un code à 6 chiffres.
	TOTPEnabled bool `json:"totpEnabled,omitempty"`
}

// NotificationSettings — canaux et règles d'alerte d'un compte SaaS. Les
// secrets (tokens, mot de passe SMTP) sont stockés mais JAMAIS renvoyés par
// l'API (l'API expose uniquement des booléens « …Set ») ; un PUT avec un
// champ secret vide conserve la valeur existante.

// NotificationSettings — canaux et règles d'alerte d'un compte SaaS. Les
// secrets (tokens, mot de passe SMTP) sont stockés mais JAMAIS renvoyés par
// l'API (l'API expose uniquement des booléens « …Set ») ; un PUT avec un
// champ secret vide conserve la valeur existante.
type NotificationSettings struct {
	AccountID string `json:"accountId"`
	Enabled   bool   `json:"enabled"` // interrupteur général des alertes automatiques
	// Telegram — bot API (https://core.telegram.org/bots)
	TelegramEnabled  bool   `json:"telegramEnabled"`
	TelegramBotToken string `json:"telegramBotToken,omitempty"`
	TelegramChatID   string `json:"telegramChatId,omitempty"`
	// WhatsApp Cloud API (Meta Graph)
	WhatsAppEnabled bool   `json:"whatsappEnabled"`
	WhatsAppToken   string `json:"whatsappToken,omitempty"`
	WhatsAppPhoneID string `json:"whatsappPhoneId,omitempty"`
	WhatsAppTo      string `json:"whatsappTo,omitempty"`
	// Email — SMTP direct (STARTTLS 587 / TLS implicite 465) ou API Resend
	// (https://resend.com). EmailProvider choisit le fournisseur du canal :
	// "" ou "smtp" → SMTP direct (défaut historique), "resend" → API HTTP
	// Resend (la clé est un secret par compte, idem mot de passe SMTP).
	EmailProvider string `json:"emailProvider,omitempty"`
	ResendAPIKey  string `json:"resendApiKey,omitempty"`
	// ResendFrom — expéditeur Resend, ex. « MikCloud <alertes@ftci.fr> » ;
	// vide → « MikCloud <onboarding@resend.dev> » (domaine d'essai Resend :
	// ne délivre qu'à l'adresse du propriétaire du compte Resend).
	ResendFrom   string `json:"resendFrom,omitempty"`
	EmailEnabled bool   `json:"emailEnabled"`
	SMTPHost     string `json:"smtpHost,omitempty"`
	SMTPPort     int    `json:"smtpPort,omitempty"`
	SMTPUser     string `json:"smtpUser,omitempty"`
	SMTPPass     string `json:"smtpPass,omitempty"`
	EmailTo      string `json:"emailTo,omitempty"`
	// Règles d'alerte
	OfflineAfterSec   int  `json:"offlineAfterSec"`   // sans check-in depuis X s → hors ligne (défaut 135 = 3 × 45 s)
	LowStockThreshold int  `json:"lowStockThreshold"` // vouchers actifs restants < X → alerte stock (défaut 25)
	DailyReport       bool `json:"dailyReport"`       // rapport quotidien
	ReportHour        int  `json:"reportHour"`        // heure d'envoi (UTC = Abidjan GMT+0), défaut 20
	// État interne anti-spam : dernier jour de rapport envoyé (YYYY-MM-DD)
	LastReportDate string `json:"lastReportDate,omitempty"`
	// État anti-spam stock : routerID → "low" | "empty" (dernier état notifié)
	StockAlertState map[string]string `json:"stockAlertState,omitempty"`
	// N°97 — état anti-spam pool IP : routerID → "high" (≥ 80 %) |
	// "full" (≥ 95 %) (dernier état notifié — même mécanique que le stock).
	PoolAlertState map[string]string `json:"poolAlertState,omitempty"`
}

// Normalize applique les défauts et bornes (appelé avant chaque lecture/écriture).

// Normalize applique les défauts et bornes (appelé avant chaque lecture/écriture).
func (s *NotificationSettings) Normalize() {
	switch {
	case s.OfflineAfterSec == 0:
		s.OfflineAfterSec = 135
	case s.OfflineAfterSec < 60:
		s.OfflineAfterSec = 60
	}
	if s.LowStockThreshold == 0 {
		s.LowStockThreshold = 25
	}
	if s.ReportHour < 0 || s.ReportHour > 23 {
		s.ReportHour = 20
	}
}

// NotificationLog — trace d'un envoi de notification (historique console).

// NotificationLog — trace d'un envoi de notification (historique console).
type NotificationLog struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	Channel   string `json:"channel"` // telegram | whatsapp | email | system
	Kind      string `json:"kind"`    // router_offline | router_back | low_stock | daily_report | test | settings | password_reset
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
	Status    string `json:"status"` // sent | error
	Error     string `json:"error,omitempty"`
	At        string `json:"at"`
}

// PasswordReset — N°68 : demande de réinitialisation du mot de passe console
// (« Mot de passe oublié ? » de l'écran de connexion). L'utilisateur saisit
// l'e-mail enregistré à la création de son compte ; le propriétaire du compte
// correspondant reçoit un lien UNIQUE et ÉPHÉMÈRE vers /reset-password.
//   - TokenHash : SHA-256 hex du token envoyé par e-mail — le token en clair
//     n'est JAMAIS persisté (une fuite de la base ne permet aucune
//     réutilisation) ;
//   - ExpiresAt : expiration stricte (TTL 60 minutes) ;
//   - UsedAt : consommation à usage unique (le lien sert UNE fois, puis il est
//     définitivement mort — une nouvelle demande invalide les liens en attente
//     du même compte) ;
//   - CreatedIP : audit de l'origine de la demande (même usage que CreatedIP
//     des inscriptions, jamais exposé par l'API).

// PasswordReset — N°68 : demande de réinitialisation du mot de passe console
// (« Mot de passe oublié ? » de l'écran de connexion). L'utilisateur saisit
// l'e-mail enregistré à la création de son compte ; le propriétaire du compte
// correspondant reçoit un lien UNIQUE et ÉPHÉMÈRE vers /reset-password.
//   - TokenHash : SHA-256 hex du token envoyé par e-mail — le token en clair
//     n'est JAMAIS persisté (une fuite de la base ne permet aucune
//     réutilisation) ;
//   - ExpiresAt : expiration stricte (TTL 60 minutes) ;
//   - UsedAt : consommation à usage unique (le lien sert UNE fois, puis il est
//     définitivement mort — une nouvelle demande invalide les liens en attente
//     du même compte) ;
//   - CreatedIP : audit de l'origine de la demande (même usage que CreatedIP
//     des inscriptions, jamais exposé par l'API).
type PasswordReset struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	UserID    string `json:"userId"`
	TokenHash string `json:"tokenHash"`
	ExpiresAt string `json:"expiresAt"`
	UsedAt    string `json:"usedAt,omitempty"`
	CreatedAt string `json:"createdAt"`
	CreatedIP string `json:"createdIp,omitempty"`
}

// BillingRequest — demande de souscription / renouvellement d'abonnement
// créée par un compte client (verrou du cycle de facturation, POST
// /api/subscription). Complète le journal (type billing) par une FILE
// actionnable dans la console plateforme : chaque demande porte un statut,
// une référence de paiement (appariement webhook Wave) et sa résolution.
