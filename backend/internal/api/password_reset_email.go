// password_reset_email.go — N°79 : gabarits du courriel « Mot de passe oublié ».
//
// Deux versions du même message :
//   - buildResetEmailText : texte pur (pièce text/plain du multipart, repli
//     des clients sans HTML — libellés N°68 inchangés) ;
//   - buildResetEmailHTML : version brandée « Aurora Emerald », l'identité
//     visuelle MikCloud (frontend/src/app/globals.css) portée en e-mail :
//     · mode CLAIR garanti — fond papier menthe #F4F9F5, carte blanche,
//     encre émeraude #102019, <meta name="color-scheme" content="light">
//     et couleurs explicites sur CHAQUE contenant (les clients sombres
//     n'inversent rien) ;
//     · bandeau + liseré au dégradé signature émeraude→teal
//     (--grad-a #009558 → --grad-b #008687, repli uni #008B57) ;
//     · wordmark duotone « Mik » blanc + « Cloud » menthe clair — écho du
//     .text-aurora de la console, lisible même si le logo est bloqué ;
//     · LE CLIN D'ŒIL PRODUIT : le lien est présenté en TICKET pointillé
//     (comme les vouchers MikCloud) — pastille « ⏱ 60 MIN », mention
//     « usage unique », la réinitialisation devient un ticket d'accès.
//
// Compatibilité e-mail (Gmail, Outlook, Apple Mail…) : tables
// role="presentation", styles 100 % inline, largeur 600 px fluide, bouton
// « bulletproof » (td bgcolor + a inline-block), alt sur le logo, repli du
// lien en clair, échappement HTML de tout contenu utilisateur (nom du compte,
// identifiant — le lien est URL-encodé par l'appelant puis échappé ici).
package api

import (
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// resetEmailData — champs du courriel de réinitialisation (texte + HTML).
type resetEmailData struct {
	OwnerName    string // nom affiché du propriétaire du compte
	AccountName  string // nom du compte MikCloud
	Username     string // identifiant de connexion
	Link         string // lien de réinitialisation (token URL-encodé)
	TTLMinutes   int    // durée de validité du lien
	FrontendBase string // origine du frontend (logo, pied de page, lien)
}

// emailFontStack — pile de polices système (aucune web-font en e-mail : les
// clients les bloquent ou les réindentent ; les systèmes couvrent tout).
const emailFontStack = "-apple-system,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif"

// buildResetEmailText — version texte (pièce text/plain du multipart, repli
// des clients sans HTML — libellés N°68 conservés au mot près).
func buildResetEmailText(d resetEmailData) string {
	var b strings.Builder
	b.WriteString("Bonjour " + d.OwnerName + ",\n\n")
	b.WriteString("Vous (ou quelqu'un utilisant cette adresse) avez demandé la réinitialisation du mot de passe du compte MikCloud « " + d.AccountName + " » (identifiant : " + d.Username + ").\n\n")
	b.WriteString("Choisissez votre nouveau mot de passe via ce lien, valable " + strconv.Itoa(d.TTLMinutes) + " minutes et utilisable une seule fois :\n\n")
	b.WriteString(d.Link + "\n\n")
	b.WriteString("Si vous n'êtes pas à l'origine de cette demande, ignorez simplement cet e-mail : votre mot de passe actuel reste inchangé.\n\n")
	b.WriteString("— MikCloud")
	return b.String()
}

// buildResetEmailHTML — version HTML brandée « Aurora Emerald » (mode clair).
func buildResetEmailHTML(d resetEmailData) string {
	// Échappement HTML de tout ce qui vient de la base (anti-injection dans
	// le HTML — un nom de compte « <script> » reste du texte affiché).
	name := html.EscapeString(d.OwnerName)
	account := html.EscapeString(d.AccountName)
	username := html.EscapeString(d.Username)
	link := html.EscapeString(d.Link)
	ttl := strconv.Itoa(d.TTLMinutes)
	logoURL := html.EscapeString(strings.TrimRight(d.FrontendBase, "/") + "/logo.png")
	siteHost := frontendHostLabel(d.FrontendBase)
	site := html.EscapeString(strings.TrimRight(d.FrontendBase, "/"))
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
<title>MikCloud — Réinitialisation de votre mot de passe</title>
</head>
<body style="margin:0;padding:0;background-color:#F4F9F5;word-spacing:normal;color-scheme:light;">`)
	// Pré-en-tête caché : texte d'aperçu dans la boîte de réception.
	b.WriteString(`
<div style="display:none;max-height:0;overflow:hidden;mso-hide:all;opacity:0;">Un lien unique, valable ` + ttl + ` minutes, pour définir votre nouveau mot de passe MikCloud.</div>
`)
	// Fond papier menthe + conteneur 600 px.
	b.WriteString(`
<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="background-color:#F4F9F5;">
<tr><td align="center" style="padding:32px 12px 44px;">
<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="600" style="width:100%;max-width:600px;">
`)
	// ── Bandeau aurora : dégradé signature + logo + wordmark duotone ──
	b.WriteString(`<tr><td bgcolor="#008B57" style="background-color:#008B57;background-image:linear-gradient(135deg,#009558 0%,#008687 100%);border-radius:14px 14px 0 0;padding:24px 28px;">
<table role="presentation" border="0" cellpadding="0" cellspacing="0"><tr>
<td style="padding-right:12px;font-size:0;line-height:0;">
<img src="` + logoURL + `" width="40" height="40" alt="MikCloud" style="display:block;border:0;border-radius:9px;width:40px;height:40px;">
</td>
<td style="font-family:` + emailFontStack + `;font-size:20px;font-weight:700;color:#FFFFFF;letter-spacing:.2px;">Mik<span style="color:#CFF5E1;">Cloud</span></td>
</tr></table>
</td></tr>
`)
	// ── Carte principale (blanche) ──
	b.WriteString(`<tr><td bgcolor="#FFFFFF" style="background-color:#FFFFFF;padding:34px 30px 26px;font-family:` + emailFontStack + `;">
<p style="margin:0 0 8px;font-size:11px;font-weight:700;letter-spacing:1.6px;color:#008B57;">SÉCURITÉ DES ACCÈS</p>
<h1 style="margin:0 0 16px;font-size:22px;line-height:1.3;font-weight:700;color:#102019;">Réinitialisez votre mot de passe</h1>
<p style="margin:0 0 12px;font-size:15px;line-height:1.65;color:#53645C;">Bonjour ` + name + `,</p>
<p style="margin:0 0 6px;font-size:15px;line-height:1.65;color:#53645C;">Vous — ou quelqu'un utilisant cette adresse — avez demandé la réinitialisation du mot de passe du compte <strong style="color:#102019;">` + account + `</strong> (identifiant&nbsp;: <strong style="color:#102019;">` + username + `</strong>).</p>
`)
	// ── Ticket sécurisé : bordure pointillée façon voucher MikCloud ──
	b.WriteString(`<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="width:100%;margin:24px 0 0;background-color:#F4F9F5;border:1px dashed #66C79E;border-radius:12px;">
<tr><td style="padding:20px 22px 22px;">
<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="width:100%;"><tr>
<td style="font-family:` + emailFontStack + `;font-size:11px;font-weight:700;letter-spacing:1.2px;color:#007644;">LIEN SÉCURISÉ · USAGE UNIQUE</td>
<td align="right" style="font-family:` + emailFontStack + `;">
<span style="display:inline-block;padding:4px 10px;background-color:#D1ECDE;border-radius:999px;font-size:11px;font-weight:700;color:#007644;">⏱ ` + ttl + ` MIN</span>
</td>
</tr></table>
`)
	// Bouton bulletproof (td bgcolor + a inline-block — cliquable partout,
	// Outlook desktop excepté pour l'arrondi).
	b.WriteString(`<table role="presentation" border="0" cellpadding="0" cellspacing="0" style="margin:18px auto 0;"><tr>
<td bgcolor="#008B57" style="background-color:#008B57;background-image:linear-gradient(135deg,#009558 0%,#008687 100%);border-radius:10px;">
<a href="` + link + `" style="display:inline-block;padding:14px 30px;font-family:` + emailFontStack + `;font-size:15px;font-weight:700;color:#FFFFFF;text-decoration:none;border-radius:10px;">Définir mon nouveau mot de passe&nbsp;→</a>
</td>
</tr></table>
<p style="margin:16px 0 0;font-size:12px;line-height:1.6;color:#53645C;text-align:center;word-break:break-word;">Valable <strong style="color:#007644;">` + ttl + `&nbsp;minutes</strong> · utilisable <strong style="color:#007644;">une seule fois</strong>.<br>Le bouton ne fonctionne pas&nbsp;? Copiez-collez ce lien dans votre navigateur&nbsp;:<br><a href="` + link + `" style="color:#008B57;text-decoration:underline;word-break:break-all;">` + link + `</a></p>
</td></tr>
</table>
`)
	// ── Note de sécurité (fond menthe doux) ──
	b.WriteString(`<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%" style="width:100%;margin:20px 0 0;background-color:#EAF5EE;border-radius:10px;">
<tr><td style="padding:14px 16px;font-family:` + emailFontStack + `;font-size:13px;line-height:1.6;color:#3B5248;">
<strong style="color:#102019;">Vous n'êtes pas à l'origine de cette demande&nbsp;?</strong> Ignorez simplement cet e-mail&nbsp;: votre mot de passe actuel reste inchangé et ce lien expirera de lui-même.
</td></tr>
</table>
</td></tr>
`)
	// ── Liseré aurora : le pied de carte signature de la console ──
	b.WriteString(`<tr><td bgcolor="#009558" height="6" style="background-color:#009558;background-image:linear-gradient(90deg,#009558 0%,#009073 55%,#008687 100%);border-radius:0 0 14px 14px;font-size:0;line-height:6px;">&nbsp;</td></tr>
`)
	// ── Pied de page ──
	b.WriteString(`<tr><td style="padding:26px 16px 8px;font-family:` + emailFontStack + `;font-size:12px;line-height:1.7;color:#7C8F85;text-align:center;">
<p style="margin:0 0 4px;"><span style="font-weight:700;color:#53645C;">MikCloud</span> · la console de gestion hotspot MikroTik</p>
<p style="margin:0 0 10px;"><a href="` + site + `" style="color:#008B57;text-decoration:none;">` + siteHost + `</a></p>
<p style="margin:0;font-size:11px;color:#6B7F76;">Cet e-mail automatique a été envoyé car une réinitialisation a été demandée pour votre compte. Aucune action n'est requise si vous en ignorez la cause.</p>
<p style="margin:10px 0 0;font-size:11px;color:#6B7F76;">© ` + year + ` MikCloud · FTech CI</p>
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`)
	return b.String()
}

// frontendHostLabel — étiquette lisible du frontend (host nu en production,
// ex. « mikcloud.ftci.fr » ; repli sur l'URL complète si non décomposable).
func frontendHostLabel(base string) string {
	if u, err := url.Parse(strings.TrimSpace(base)); err == nil && u.Host != "" {
		return u.Host
	}
	return base
}
