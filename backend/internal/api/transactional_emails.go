// transactional_emails.go — N°146 : e-mails transactionnels « Reçu de
// paiement » et « Bienvenue », en deux pièces (texte de repli + HTML brandé
// « Aurora Emerald » — même mécanique que le courriel de réinitialisation
// N°79, délivrés par le fournisseur du compte : API Resend ou SMTP direct).
//
// DÉCLENCHEURS (un reçu par ENCAISSEMENT RÉEL, jamais par essai/extension
// gratuite) :
//   - finalizeBillingSuccess  : Wave confirmé via GeniusPay (poll client +
//     webhook signé) — source unique des demandes de renouvellement ;
//   - handleWaveWebhook       : webhook Wave direct (secret partagé) ;
//   - handleAdminBillingRequestResolve (markPaid) : encaissement confirmé par
//     la plateforme sur une demande en attente ;
//   - applyStripeRenewalByUUID : prélèvement carte Stripe confirmé (webhook
//     signé + resynchronisation sur les factures réelles — idempotent, donc
//     un seul reçu par paiement).
//   - handleRegister          : e-mail de bienvenue à la création du compte
//     (essai gratuit 60 j Hotspot / 30 j HomeNet).
//
// DISCIPLINE DE VERROU (règle absolue du store) : les résolutions (compte,
// propriétaire, expéditeur) se font sous le verrou de l'APPELANT (les
// fonctions queueXxx sont prévues pour être appelées SOUS verrou, comme
// applySubscriptionLocked) ; l'envoi réseau (jusqu'à 12 s par canal) part
// dans une goroutine isolée — la réponse HTTP d'un webhook ou d'une
// inscription n'attend JAMAIS Resend ; la trace d'historique reprend le
// verrou brièvement à la fin (même format que N°68/N°79 : notif_log, kind
// dédié, statut sent/error).
//
// EXPÉDITEUR : réglages e-mail du compte demandeur, sinon ceux du compte
// principal (plateforme — en production Resend y est configuré depuis N°67 :
// les clients n'ont RIEN à régler pour recevoir leurs reçus). Compte sans
// e-mail connu ou fournisseur non configuré : envoi silencieusement écarté
// (best-effort, tracé dans les journaux serveur — jamais d'échec de paiement
// à cause d'un e-mail).
package api

import (
	"fmt"
	"html"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/notify"
)

// ---------------------------------------------------------------------------
// Indirections de test (aucun réseau en CI — même patron que sendResetEmail)
// ---------------------------------------------------------------------------

// emailIndirectMu — verrou des indirections d'envoi (N°178). Les tests
// remplacent ces pointeurs à CHAUD alors que des goroutines d'envoi encore
// en vol (welcome d'une inscription d'un test antérieur, dispatchées par la
// production) peuvent les lire — le race detector de la CI l'a prouvé (run
// 457 sur df1111f, TestAnnouncementSweepDeferredEmail). Les lecteurs
// passent par les accesseurs ci-dessous, le helper de stub écrit sous
// verrou : lecture/écriture du pointeur toujours synchronisées.
var emailIndirectMu sync.RWMutex

// sendAccountEmail — envoi réel via le fournisseur du compte (Resend ou SMTP).
var sendAccountEmail = notify.SendEmailTo

// accountEmailSender — copie synchronisée du pointeur d'envoi (la valeur est
// lue sous verrou puis appelée hors verrou : l'appel long ne bloque personne).
func accountEmailSender() func(cfg *model.NotificationSettings, to, title, textBody, htmlBody string) error {
	emailIndirectMu.RLock()
	defer emailIndirectMu.RUnlock()
	return sendAccountEmail
}

// dispatchEmailTask — exécute une tâche d'envoi : goroutine + recover en
// production (un plantage d'envoi ne doit JAMAIS abattre le serveur) ;
// les tests la remplacent par un exécuteur synchrone déterministe.
var dispatchEmailTask = func(fn func()) {
	go func() {
		defer func() {
			if p := recover(); p != nil {
				log.Printf("[email] panique de la goroutine d'envoi : %v", p)
			}
		}()
		fn()
	}()
}

// emailTaskDispatch — copie synchronisée du pointeur de file d'envoi.
func emailTaskDispatch() func(fn func()) {
	emailIndirectMu.RLock()
	defer emailIndirectMu.RUnlock()
	return dispatchEmailTask
}

// appPublicBaseURL — origine canonique du frontend pour les webhooks (pas
// d'en-tête Origin exploitable : l'appelant est GeniusPay/Wave, pas le
// navigateur du client). APP_PUBLIC_URL d'abord, URL canonique sinon.
func appPublicBaseURL() string {
	if v := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_PUBLIC_URL")), "/"); v != "" {
		return v
	}
	return defaultFrontendURL
}

// ---------------------------------------------------------------------------
// Données des courriels
// ---------------------------------------------------------------------------

// receiptEmailData — champs du reçu de paiement (texte + HTML).
type receiptEmailData struct {
	AccountID    string    // pour résoudre destinataire + expéditeur
	PlanLabel    string    // « Essentiel mensuel »
	PeriodLabel  string    // « 1 mois » / « 1 an » — période couverte
	AmountFcfa   int       // montant encaissé
	Method       string    // « Wave » / « carte bancaire » / « carte bancaire (Stripe) »
	Ref          string    // référence demande (MC-…) ou abonnement (UUID)
	PaidAt       time.Time // date d'encaissement
	PeriodEnd    string    // fin de période couverte (RFC3339)
	FrontendBase string    // origine du frontend (logo, pied, CTA)
}

// welcomeEmailData — champs du courriel de bienvenue (texte + HTML).
type welcomeEmailData struct {
	OwnerName    string // nom affiché du propriétaire
	AccountName  string // nom du compte MikCloud
	Username     string // identifiant de connexion
	Usage        string // hotspot | homenet — copie et étapes adaptées
	TrialDays    int    // 60 (Hotspot) / 30 (HomeNet)
	TrialEnd     string // fin d'essai (RFC3339)
	FrontendBase string
}

// ---------------------------------------------------------------------------
// Mise en forme partagée
// ---------------------------------------------------------------------------

// moisFr — noms de mois français (formatage des dates des courriels).
var moisFr = [...]string{"janvier", "février", "mars", "avril", "mai", "juin",
	"juillet", "août", "septembre", "octobre", "novembre", "décembre"}

// formatDateFr — « 18 novembre 2026 » (date locale Abidjan = UTC, GMT+0 sans
// DST : les montants et échéances s'affichent comme le gérant les vit).
func formatDateFr(t time.Time) string {
	return fmt.Sprintf("%d %s %d", t.Day(), moisFr[int(t.Month())-1], t.Year())
}

// dateLabelOf — libellé français d'une date RFC3339 (repli : valeur brute —
// jamais de date inventée).
func dateLabelOf(rfc3339 string) string {
	if t, err := time.Parse(time.RFC3339, rfc3339); err == nil {
		return formatDateFr(t)
	}
	return rfc3339
}

// formatFcfa — « 15 000 FCFA » (séparateur de milliers espace, usage courant
// en Côte d'Ivoire — même rendu que la console). Le formatage du nombre est
// partagé avec les factures HTML (handlers_billing_client.go) : seul le
// suffixe monétaire est ajouté ici.
func formatFcfaMail(n int) string {
	return formatFcfa(n) + " FCFA"
}

// usageLabelFr — libellé français de l'usage du compte.
func usageLabelFr(usage string) string {
	if usage == model.AccountUsageHomeNet {
		return "réseau maison"
	}
	return "hotspot"
}

// ---------------------------------------------------------------------------
// Reçu de paiement — corps TEXTE
// ---------------------------------------------------------------------------

func buildReceiptEmailText(d receiptEmailData, ownerName, accountName string) string {
	var b strings.Builder
	b.WriteString("Bonjour " + ownerName + ",\n\n")
	b.WriteString("Nous confirmons la réception de votre paiement pour le compte MikCloud « " + accountName + " ».\n\n")
	b.WriteString("— Reçu de paiement —\n")
	b.WriteString("Abonnement : " + d.PlanLabel + " (" + d.PeriodLabel + ")\n")
	b.WriteString("Montant : " + formatFcfaMail(d.AmountFcfa) + "\n")
	b.WriteString("Moyen de paiement : " + d.Method + "\n")
	b.WriteString("Référence : " + d.Ref + "\n")
	b.WriteString("Date : " + formatDateFr(d.PaidAt) + "\n")
	if end := dateLabelOf(d.PeriodEnd); end != "" {
		b.WriteString("Actif jusqu'au : " + end + "\n")
	}
	b.WriteString("\nVotre abonnement est actif — merci de votre confiance.\n\n")
	b.WriteString("— MikCloud")
	return b.String()
}

// ---------------------------------------------------------------------------
// Reçu de paiement — corps HTML brandé « Aurora Emerald »
// ---------------------------------------------------------------------------

func buildReceiptEmailHTML(d receiptEmailData, ownerName, accountName string) string {
	name := html.EscapeString(ownerName)
	account := html.EscapeString(accountName)
	plan := html.EscapeString(d.PlanLabel)
	period := html.EscapeString(d.PeriodLabel)
	method := html.EscapeString(d.Method)
	ref := html.EscapeString(d.Ref)
	amount := html.EscapeString(formatFcfaMail(d.AmountFcfa))
	datePaid := html.EscapeString(formatDateFr(d.PaidAt))
	until := html.EscapeString(dateLabelOf(d.PeriodEnd))
	logoURL := html.EscapeString(strings.TrimRight(d.FrontendBase, "/") + "/logo.png")
	siteHost := frontendHostLabel(d.FrontendBase)
	site := html.EscapeString(strings.TrimRight(d.FrontendBase, "/"))
	console := html.EscapeString(strings.TrimRight(d.FrontendBase, "/") + "/app")
	year := strconv.Itoa(time.Now().UTC().Year())

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="fr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="x-apple-disable-message-reformatting">
<meta name="color-scheme" content="light">
<meta name="supported-color-schemes" content="light">
<title>MikCloud — Reçu de paiement</title>
</head>
<body style="margin:0;padding:0;background-color:#F4F9F5;word-spacing:normal;color-scheme:light;">`)
	// Pré-en-tête caché : aperçu dans la boîte de réception.
	b.WriteString(`
<div style="display:none;max-height:0;overflow:hidden;mso-hide:all;opacity:0;">Paiement confirmé — ` + amount + ` · abonnement actif jusqu'au ` + until + `.</div>
`)
	b.WriteString(`
<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="background-color:#F4F9F5;">
<tr><td align="center" style="padding:32px 12px 44px;">
<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="600" style="width:100%;max-width:600px;">
`)
	// ── Bandeau aurora ──
	b.WriteString(`<tr><td bgcolor="#008B57" style="background-color:#008B57;background-image:linear-gradient(135deg,#009558 0%,#008687 100%);border-radius:14px 14px 0 0;padding:24px 28px;">
<table role="presentation" border="0" cellpadding="0" cellspacing="0"><tr>
<td style="padding-right:12px;font-size:0;line-height:0;">
<img src="` + logoURL + `" width="40" height="40" alt="MikCloud" style="display:block;border:0;border-radius:9px;width:40px;height:40px;">
</td>
<td style="font-family:` + emailFontStack + `;font-size:20px;font-weight:700;color:#FFFFFF;letter-spacing:.2px;">Mik<span style="color:#CFF5E1;">Cloud</span></td>
</tr></table>
</td></tr>
`)
	// ── Carte principale ──
	b.WriteString(`<tr><td bgcolor="#FFFFFF" style="background-color:#FFFFFF;padding:34px 30px 26px;font-family:` + emailFontStack + `;">
<p style="margin:0 0 8px;font-size:11px;font-weight:700;letter-spacing:1.6px;color:#008B57;">REÇU DE PAIEMENT</p>
<h1 style="margin:0 0 16px;font-size:22px;line-height:1.3;font-weight:700;color:#102019;">Paiement confirmé, merci&nbsp;!</h1>
<p style="margin:0 0 6px;font-size:15px;line-height:1.65;color:#53645C;">Bonjour ` + name + `,</p>
<p style="margin:0;font-size:15px;line-height:1.65;color:#53645C;">Nous confirmons la réception de votre paiement pour le compte <strong style="color:#102019;">` + account + `</strong>. Voici votre reçu&nbsp;:</p>
`)
	// ── Ticket reçu : bordure pointillée façon voucher MikCloud ──
	b.WriteString(`<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="width:100%;margin:24px 0 0;background-color:#F4F9F5;border:1px dashed #66C79E;border-radius:12px;">
<tr><td style="padding:20px 22px 22px;">
<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="width:100%;"><tr>
<td style="font-family:` + emailFontStack + `;font-size:11px;font-weight:700;letter-spacing:1.2px;color:#007644;">REÇU · ` + ref + `</td>
<td align="right" style="font-family:` + emailFontStack + `;">
<span style="display:inline-block;padding:4px 10px;background-color:#D1ECDE;border-radius:999px;font-size:11px;font-weight:700;color:#007644;">PAYÉ ✔</span>
</td>
</tr></table>
<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="width:100%;margin:16px 0 0;">`)
	receiptRow := func(label, value string, strong bool) {
		vStyle := "font-size:15px;line-height:1.5;color:#102019;"
		if strong {
			vStyle = "font-size:18px;line-height:1.4;font-weight:700;color:#102019;"
		}
		b.WriteString(`<tr>
<td style="padding:7px 0;font-family:` + emailFontStack + `;font-size:12px;font-weight:700;letter-spacing:.8px;color:#7C8F85;white-space:nowrap;padding-right:16px;">` + label + `</td>
<td align="right" style="padding:7px 0;font-family:` + emailFontStack + `;` + vStyle + `">` + value + `</td>
</tr>`)
	}
	receiptRow("MONTANT", amount, true)
	receiptRow("ABONNEMENT", plan+" <span style=\"color:#7C8F85;font-weight:400;\">· "+period+"</span>", false)
	receiptRow("MOYEN DE PAIEMENT", method, false)
	receiptRow("DATE", datePaid, false)
	if until != "" {
		receiptRow("ACTIF JUSQU'AU", "<strong style=\"color:#007644;\">"+until+"</strong>", false)
	}
	b.WriteString(`</table>
</td></tr>
</table>
`)
	// ── CTA console ──
	b.WriteString(`<table role="presentation" border="0" cellpadding="0" cellspacing="0" style="margin:22px auto 0;"><tr>
<td bgcolor="#008B57" style="background-color:#008B57;background-image:linear-gradient(135deg,#009558 0%,#008687 100%);border-radius:10px;">
<a href="` + console + `" style="display:inline-block;padding:14px 30px;font-family:` + emailFontStack + `;font-size:15px;font-weight:700;color:#FFFFFF;text-decoration:none;border-radius:10px;">Voir mon abonnement&nbsp;→</a>
</td>
</tr></table>
`)
	// ── Note de conservation (fond menthe doux) ──
	b.WriteString(`<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="width:100%;margin:20px 0 0;background-color:#EAF5EE;border-radius:10px;">
<tr><td style="padding:14px 16px;font-family:` + emailFontStack + `;font-size:13px;line-height:1.6;color:#3B5248;">
Conservez cet e-mail&nbsp;: il tient lieu de reçu pour votre paiement ` + method + `. L'historique complet de vos factures reste disponible dans votre console, onglet Abonnement.
</td></tr>
</table>
</td></tr>
`)
	// ── Liseré aurora + pied de page ──
	b.WriteString(`<tr><td bgcolor="#009558" height="6" style="background-color:#009558;background-image:linear-gradient(90deg,#009558 0%,#009073 55%,#008687 100%);border-radius:0 0 14px 14px;font-size:0;line-height:6px;">&nbsp;</td></tr>
<tr><td style="padding:26px 16px 8px;font-family:` + emailFontStack + `;font-size:12px;line-height:1.7;color:#7C8F85;text-align:center;">
<p style="margin:0 0 4px;"><span style="font-weight:700;color:#53645C;">MikCloud</span> · la console de gestion hotspot MikroTik</p>
<p style="margin:0 0 10px;"><a href="` + site + `" style="color:#008B57;text-decoration:none;">` + siteHost + `</a></p>
<p style="margin:0;font-size:11px;color:#6B7F76;">Cet e-mail automatique confirme un paiement reçu sur votre compte MikCloud.</p>
<p style="margin:10px 0 0;font-size:11px;color:#6B7F76;">© ` + year + ` MikCloud · FTech CI</p>
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`)
	return b.String()
}

// ---------------------------------------------------------------------------
// Bienvenue — corps TEXTE
// ---------------------------------------------------------------------------

func buildWelcomeEmailText(d welcomeEmailData) string {
	var b strings.Builder
	b.WriteString("Bonjour " + d.OwnerName + ",\n\n")
	if d.Usage == model.AccountUsageHomeNet {
		b.WriteString("Bienvenue sur MikCloud ! Votre compte maison « " + d.AccountName + " » est prêt : pilotez votre réseau familial (appareils, protection, couvre-feu) depuis une seule console.\n\n")
	} else {
		b.WriteString("Bienvenue sur MikCloud ! Votre compte hotspot « " + d.AccountName + " » est prêt : générez vos vouchers, suivez vos sessions et vendez du Wi-Fi payant comme un pro.\n\n")
	}
	b.WriteString("— Votre essai gratuit —\n")
	b.WriteString("Durée : " + strconv.Itoa(d.TrialDays) + " jours\n")
	b.WriteString("Compte : " + d.AccountName + "\n")
	b.WriteString("Identifiant : " + d.Username + "\n")
	b.WriteString("Fin de l'essai : " + dateLabelOf(d.TrialEnd) + "\n\n")
	b.WriteString("Pour commencer :\n")
	if d.Usage == model.AccountUsageHomeNet {
		b.WriteString("1. Ajoutez votre box ou routeur MikroTik dans la console\n")
		b.WriteString("2. Découvrez vos appareils connectés, bail par bail\n")
		b.WriteString("3. Protégez votre famille (contrôle parental, couvre-feu)\n")
	} else {
		b.WriteString("1. Connectez votre routeur MikroTik (agent MikCloud)\n")
		b.WriteString("2. Générez vos premiers vouchers Wi-Fi\n")
		b.WriteString("3. Installez l'app mobile (PWA) pour vendre partout\n")
	}
	b.WriteString("\nOuvrez votre console : " + strings.TrimRight(d.FrontendBase, "/") + "/app\n\n")
	b.WriteString("— MikCloud")
	return b.String()
}

// ---------------------------------------------------------------------------
// Bienvenue — corps HTML brandé « Aurora Emerald »
// ---------------------------------------------------------------------------

func buildWelcomeEmailHTML(d welcomeEmailData) string {
	name := html.EscapeString(d.OwnerName)
	account := html.EscapeString(d.AccountName)
	username := html.EscapeString(d.Username)
	days := strconv.Itoa(d.TrialDays)
	until := html.EscapeString(dateLabelOf(d.TrialEnd))
	logoURL := html.EscapeString(strings.TrimRight(d.FrontendBase, "/") + "/logo.png")
	siteHost := frontendHostLabel(d.FrontendBase)
	site := html.EscapeString(strings.TrimRight(d.FrontendBase, "/"))
	console := html.EscapeString(strings.TrimRight(d.FrontendBase, "/") + "/app")
	year := strconv.Itoa(time.Now().UTC().Year())

	// Copie et étapes adaptées à l'usage du compte.
	intro, eyebrow, steps := "", "", [3][2]string{}
	if d.Usage == model.AccountUsageHomeNet {
		eyebrow = "VOTRE RÉSEAU MAISON"
		intro = "Votre compte maison <strong style=\"color:#102019;\">" + account + "</strong> est prêt. Pilotez votre réseau familial — appareils, protection, couvre-feu — depuis une seule console."
		steps = [3][2]string{
			{"Ajoutez votre box", "Déclarez votre box ou routeur MikroTik : MikCloud découvre votre réseau."},
			{"Regardez vos appareils", "La liste des appareils connectés, bail par bail, en temps réel."},
			{"Protégez votre famille", "Contrôle parental, blocage des contenus et couvre-feu des enfants."},
		}
	} else {
		eyebrow = "VOTRE HOTSPOT"
		intro = "Votre compte hotspot <strong style=\"color:#102019;\">" + account + "</strong> est prêt. Générez vos vouchers, suivez vos sessions et vendez du Wi-Fi payant comme un pro."
		steps = [3][2]string{
			{"Connectez votre routeur", "L'agent MikCloud relie votre MikroTik à la console en quelques minutes."},
			{"Générez vos vouchers", "Créez vos tickets Wi-Fi, imprimez-les ou envoyez-les par WhatsApp."},
			{"Vendez partout", "Installez l'app mobile (PWA) : vos revendeurs vendent même sans réseau."},
		}
	}

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="fr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="x-apple-disable-message-reformatting">
<meta name="color-scheme" content="light">
<meta name="supported-color-schemes" content="light">
<title>MikCloud — Bienvenue !</title>
</head>
<body style="margin:0;padding:0;background-color:#F4F9F5;word-spacing:normal;color-scheme:light;">`)
	b.WriteString(`
<div style="display:none;max-height:0;overflow:hidden;mso-hide:all;opacity:0;">Votre compte MikCloud est prêt — essai gratuit ` + days + ` jours, jusqu'au ` + until + `.</div>
`)
	b.WriteString(`
<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="background-color:#F4F9F5;">
<tr><td align="center" style="padding:32px 12px 44px;">
<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="600" style="width:100%;max-width:600px;">
`)
	// ── Bandeau aurora ──
	b.WriteString(`<tr><td bgcolor="#008B57" style="background-color:#008B57;background-image:linear-gradient(135deg,#009558 0%,#008687 100%);border-radius:14px 14px 0 0;padding:24px 28px;">
<table role="presentation" border="0" cellpadding="0" cellspacing="0"><tr>
<td style="padding-right:12px;font-size:0;line-height:0;">
<img src="` + logoURL + `" width="40" height="40" alt="MikCloud" style="display:block;border:0;border-radius:9px;width:40px;height:40px;">
</td>
<td style="font-family:` + emailFontStack + `;font-size:20px;font-weight:700;color:#FFFFFF;letter-spacing:.2px;">Mik<span style="color:#CFF5E1;">Cloud</span></td>
</tr></table>
</td></tr>
`)
	// ── Carte principale ──
	b.WriteString(`<tr><td bgcolor="#FFFFFF" style="background-color:#FFFFFF;padding:34px 30px 26px;font-family:` + emailFontStack + `;">
<p style="margin:0 0 8px;font-size:11px;font-weight:700;letter-spacing:1.6px;color:#008B57;">` + eyebrow + `</p>
<h1 style="margin:0 0 16px;font-size:22px;line-height:1.3;font-weight:700;color:#102019;">Bienvenue sur MikCloud, ` + name + `&nbsp;!</h1>
<p style="margin:0;font-size:15px;line-height:1.65;color:#53645C;">` + intro + `</p>
`)
	// ── Ticket essai : bordure pointillée façon voucher ──
	b.WriteString(`<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="width:100%;margin:24px 0 0;background-color:#F4F9F5;border:1px dashed #66C79E;border-radius:12px;">
<tr><td style="padding:20px 22px 22px;">
<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="width:100%;"><tr>
<td style="font-family:` + emailFontStack + `;font-size:11px;font-weight:700;letter-spacing:1.2px;color:#007644;">ESSAI GRATUIT · SANS CARTE BANCAIRE</td>
<td align="right" style="font-family:` + emailFontStack + `;">
<span style="display:inline-block;padding:4px 10px;background-color:#D1ECDE;border-radius:999px;font-size:11px;font-weight:700;color:#007644;">⏱ ` + days + ` JOURS</span>
</td>
</tr></table>
<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="width:100%;margin:16px 0 0;">
<tr>
<td style="padding:7px 0;font-family:` + emailFontStack + `;font-size:12px;font-weight:700;letter-spacing:.8px;color:#7C8F85;white-space:nowrap;padding-right:16px;">COMPTE</td>
<td align="right" style="padding:7px 0;font-family:` + emailFontStack + `;font-size:15px;line-height:1.5;color:#102019;">` + account + `</td>
</tr>
<tr>
<td style="padding:7px 0;font-family:` + emailFontStack + `;font-size:12px;font-weight:700;letter-spacing:.8px;color:#7C8F85;white-space:nowrap;padding-right:16px;">IDENTIFIANT</td>
<td align="right" style="padding:7px 0;font-family:` + emailFontStack + `;font-size:15px;line-height:1.5;color:#102019;">` + username + `</td>
</tr>
<tr>
<td style="padding:7px 0;font-family:` + emailFontStack + `;font-size:12px;font-weight:700;letter-spacing:.8px;color:#7C8F85;white-space:nowrap;padding-right:16px;">FIN DE L'ESSAI</td>
<td align="right" style="padding:7px 0;font-family:` + emailFontStack + `;font-size:15px;line-height:1.5;color:#102019;"><strong style="color:#007644;">` + until + `</strong></td>
</tr>
</table>
</td></tr>
</table>
`)
	// ── Trois premiers pas ──
	b.WriteString(`<p style="margin:26px 0 12px;font-size:15px;line-height:1.65;color:#102019;font-weight:700;">Pour bien démarrer</p>
<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="width:100%;">`)
	for i, s := range steps {
		b.WriteString(`<tr>
<td width="34" valign="top" style="padding:0 12px 14px 0;font-size:0;line-height:0;">
<table role="presentation" border="0" cellpadding="0" cellspacing="0"><tr><td bgcolor="#D1ECDE" width="28" height="28" align="center" valign="middle" style="background-color:#D1ECDE;border-radius:999px;width:28px;height:28px;">
<span style="font-family:` + emailFontStack + `;font-size:13px;font-weight:700;color:#007644;">` + strconv.Itoa(i+1) + `</span>
</td></tr></table>
</td>
<td valign="top" style="padding:2px 0 14px;font-family:` + emailFontStack + `;">
<span style="font-size:14px;font-weight:700;color:#102019;">` + s[0] + `</span><br>
<span style="font-size:13px;line-height:1.55;color:#53645C;">` + s[1] + `</span>
</td>
</tr>`)
	}
	b.WriteString(`</table>
`)
	// ── CTA console ──
	b.WriteString(`<table role="presentation" border="0" cellpadding="0" cellspacing="0" style="margin:8px auto 0;"><tr>
<td bgcolor="#008B57" style="background-color:#008B57;background-image:linear-gradient(135deg,#009558 0%,#008687 100%);border-radius:10px;">
<a href="` + console + `" style="display:inline-block;padding:14px 30px;font-family:` + emailFontStack + `;font-size:15px;font-weight:700;color:#FFFFFF;text-decoration:none;border-radius:10px;">Ouvrir ma console&nbsp;→</a>
</td>
</tr></table>
`)
	// ── Note assistance ──
	b.WriteString(`<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="width:100%;margin:20px 0 0;background-color:#EAF5EE;border-radius:10px;">
<tr><td style="padding:14px 16px;font-family:` + emailFontStack + `;font-size:13px;line-height:1.6;color:#3B5248;">
<strong style="color:#102019;">Une question&nbsp;?</strong> L'assistant MikCloud répond directement dans la console — ou écrivez-nous en répondant à cet e-mail.
</td></tr>
</table>
</td></tr>
`)
	// ── Liseré aurora + pied de page ──
	b.WriteString(`<tr><td bgcolor="#009558" height="6" style="background-color:#009558;background-image:linear-gradient(90deg,#009558 0%,#009073 55%,#008687 100%);border-radius:0 0 14px 14px;font-size:0;line-height:6px;">&nbsp;</td></tr>
<tr><td style="padding:26px 16px 8px;font-family:` + emailFontStack + `;font-size:12px;line-height:1.7;color:#7C8F85;text-align:center;">
<p style="margin:0 0 4px;"><span style="font-weight:700;color:#53645C;">MikCloud</span> · la console de gestion hotspot MikroTik</p>
<p style="margin:0 0 10px;"><a href="` + site + `" style="color:#008B57;text-decoration:none;">` + siteHost + `</a></p>
<p style="margin:0;font-size:11px;color:#6B7F76;">Cet e-mail vous a été envoyé car un compte MikCloud vient d'être créé avec cette adresse.</p>
<p style="margin:10px 0 0;font-size:11px;color:#6B7F76;">© ` + year + ` MikCloud · FTech CI</p>
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`)
	return b.String()
}

// ---------------------------------------------------------------------------
// File d'envoi asynchrone (résolution sous verrou de l'appelant)
// ---------------------------------------------------------------------------

// accountOwnerLocked — compte + utilisateur propriétaire (copies, consultables
// hors verrou ensuite). Verrou POSÉ par l'appelant.
func accountOwnerLocked(db *model.DB, accID string) (model.Account, model.AdminUser, bool) {
	var acc model.Account
	found := false
	for i := range db.Accounts {
		if db.Accounts[i].ID == accID {
			acc = db.Accounts[i]
			found = true
			break
		}
	}
	if !found {
		return model.Account{}, model.AdminUser{}, false
	}
	for i := range db.Users {
		if db.Users[i].AccountID == accID && db.Users[i].Role == model.RoleOwner {
			return acc, db.Users[i], true
		}
	}
	return acc, model.AdminUser{}, true // compte sans owner : l'e-mail du compte suffit
}

// transactionalSenderLocked — réglages d'expédition d'un e-mail
// transactionnel : ceux du compte s'ils sont exploitables (Resend ou SMTP),
// sinon ceux du compte principal (plateforme — Resend en production depuis
// N°67). Même logique que le mot de passe oublié. Verrou POSÉ par l'appelant.
func transactionalSenderLocked(db *model.DB, accID string) (model.NotificationSettings, bool) {
	if cfg, ok := db.NotifSettings[accID]; ok && notify.EmailCredentialsOK(&cfg) {
		return cfg, true
	}
	if cfg, ok := db.NotifSettings[model.AccountMainID]; ok && notify.EmailCredentialsOK(&cfg) {
		return cfg, true
	}
	return model.NotificationSettings{}, false
}

// dispatchAccountEmail — envoi + trace d'historique. Toujours appelé via
// dispatchEmailTask (goroutine en production) : l'appelant ne bloque jamais.
func (a *API) dispatchAccountEmail(kind, title, logBody string, cfg model.NotificationSettings, to, textBody, htmlBody string) {
	// Pointeur lu synchronisé (N°178) : une goroutine en vol depuis un test
	// antérieur peut croiser un remplacement de stub sans course mémoire.
	err := accountEmailSender()(&cfg, to, title, textBody, htmlBody)
	entry := notify.LogEntry(&cfg, "email", kind, title, "", err)
	if err != nil {
		// Best-effort : l'échec d'un reçu ne remonte jamais au flux de
		// paiement — trace serveur + historique « error » suffisent.
		log.Printf("[email] %s non envoyé à %s : %v", kind, to, err)
	}
	a.store.Lock()
	db := a.store.Data()
	entry.ID = model.NewID("n-")
	entry.At = model.NowISO()
	entry.Body = logBody
	db.NotifLog = append(db.NotifLog, entry)
	a.store.Save()
	a.store.Unlock()
}

// queueReceiptEmail — programme l'envoi du reçu de paiement. Appelable SOUS
// VERROU (les résolutions se font immédiatement, l'envoi part en goroutine —
// la réponse HTTP d'un webhook n'attend jamais le réseau). Compte sans
// e-mail ou expéditeur non configuré : écarté silencieusement (best-effort).
func (a *API) queueReceiptEmail(db *model.DB, rc receiptEmailData) {
	acc, owner, ok := accountOwnerLocked(db, rc.AccountID)
	if !ok || strings.TrimSpace(acc.Email) == "" {
		return
	}
	cfg, ok := transactionalSenderLocked(db, rc.AccountID)
	if !ok {
		return
	}
	ownerName := owner.Name
	if ownerName == "" {
		ownerName = acc.Name
	}
	to := acc.Email
	title := "MikCloud — Reçu de paiement " + formatFcfaMail(rc.AmountFcfa)
	logBody := "Reçu de paiement — abonnement " + rc.PlanLabel + " (" + rc.PeriodLabel + "), " +
		formatFcfaMail(rc.AmountFcfa) + " via " + rc.Method + ", réf. " + rc.Ref
	textBody := buildReceiptEmailText(rc, ownerName, acc.Name)
	htmlBody := buildReceiptEmailHTML(rc, ownerName, acc.Name)
	emailTaskDispatch()(func() {
		a.dispatchAccountEmail(notify.KindPaymentReceipt, title, logBody, cfg, to, textBody, htmlBody)
	})
}

// queueWelcomeEmail — programme l'envoi du courriel de bienvenue à la création
// d'un compte (handleRegister). Même discipline de verrou que le reçu : les
// données (compte, propriétaire, essai, expéditeur) sont résolues sous le
// verrou de l'appelant, l'envoi part en goroutine.
func (a *API) queueWelcomeEmail(db *model.DB, acc model.Account, owner model.AdminUser, sub model.Subscription, frontendBase string) {
	if strings.TrimSpace(acc.Email) == "" {
		return // compte sans e-mail connu (pré-signup enrichi) : rien à envoyer
	}
	cfg, ok := transactionalSenderLocked(db, acc.ID)
	if !ok {
		return
	}
	trialDays := 60
	if acc.Usage == model.AccountUsageHomeNet {
		trialDays = 30
	}
	ownerName := owner.Name
	if ownerName == "" {
		ownerName = acc.Name
	}
	d := welcomeEmailData{
		OwnerName:    ownerName,
		AccountName:  acc.Name,
		Username:     owner.Username,
		Usage:        acc.Usage,
		TrialDays:    trialDays,
		TrialEnd:     sub.PeriodEnd,
		FrontendBase: frontendBase,
	}
	to := acc.Email
	title := "Bienvenue sur MikCloud, " + d.OwnerName + " !"
	logBody := "E-mail de bienvenue envoyé à " + to + " (essai " + strconv.Itoa(d.TrialDays) +
		" jours, " + usageLabelFr(d.Usage) + ")"
	textBody := buildWelcomeEmailText(d)
	htmlBody := buildWelcomeEmailHTML(d)
	emailTaskDispatch()(func() {
		a.dispatchAccountEmail(notify.KindWelcome, title, logBody, cfg, to, textBody, htmlBody)
	})
}
