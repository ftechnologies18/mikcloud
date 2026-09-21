// transactional_emails_test.go — N°146 : couverture des e-mails
// transactionnels « Reçu de paiement » et « Bienvenue ».
//
//   - gabarits purs (texte + HTML brandé) : contenus attendus, dates et
//     montants français, échappement HTML des contenus utilisateur ;
//   - file d'envoi asynchrone : stubs déterministes (sendAccountEmail capturé,
//     dispatchEmailTask synchrone) — aucun réseau en CI ;
//   - bout en bout : inscription publique → e-mail de bienvenue + trace
//     notif_log ; résolution plateforme d'une demande (markPaid) → reçu de
//     paiement, et PAS de reçu pour une simple extension non encaissée.
package api

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/auth"
	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/notify"
	"mikcloud/hotspot-api/internal/store"
)

// ---------------------------------------------------------------------------
// Stubs déterministes (même patron que password_reset_test.go)
// ---------------------------------------------------------------------------

// sentEmailMu — verrou du tampon de capture : les goroutines d'envoi
// wg-suivies peuvent se chevaucher, et la réinitialisation (drain du
// welcome avant l'observation propre au test, N°178) doit elle aussi
// passer par resetSentEmails pour rester synchronisée.
var sentEmailMu sync.Mutex

// resetSentEmails — vide le tampon de capture SOUS verrou : s'utilise après
// un wg.Wait() pour rejeter les e-mails hors sujet (ex. le welcome d'une
// inscription préparatoire) sans course avec un append résiduel.
func resetSentEmails(calls *[]sentEmail) {
	sentEmailMu.Lock()
	*calls = nil
	sentEmailMu.Unlock()
}

// stubAccountEmailCapture — remplace sendAccountEmail (capture) et rend
// dispatchEmailTask observable : chaque tâche part BIEN en goroutine (la
// discipline de verrou de la production — un dispatch synchrone under lock
// serait un deadlock), le test attend leur complétion via wg.Wait(). Tout est
// restauré à la fin du test (t.Cleanup).
//
// N°178 : les remplacements s'écrivent SOUS emailIndirectMu (les goroutines
// d'envoi en vol lisent les pointeurs via les accesseurs synchronisés de la
// production), et la capture append-e sous sentEmailMu. À installer AVANT
// toute action déclenchant un envoi — le patron de
// TestRegisterSendsWelcomeEmail — sinon le welcome part sous le dispatch
// PRODUCTION, insuivable par wg et donc non drainable avant l'assertion.
func stubAccountEmailCapture(t *testing.T, calls *[]sentEmail) *sync.WaitGroup {
	t.Helper()
	resetSentEmails(calls)
	var wg sync.WaitGroup
	emailIndirectMu.Lock()
	oldSend := sendAccountEmail
	sendAccountEmail = func(cfg *model.NotificationSettings, to, title, textBody, htmlBody string) error {
		sentEmailMu.Lock()
		*calls = append(*calls, sentEmail{*cfg, to, title, textBody, htmlBody})
		sentEmailMu.Unlock()
		return nil
	}
	oldDispatch := dispatchEmailTask
	dispatchEmailTask = func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}
	emailIndirectMu.Unlock()
	t.Cleanup(func() {
		emailIndirectMu.Lock()
		sendAccountEmail = oldSend
		dispatchEmailTask = oldDispatch
		emailIndirectMu.Unlock()
	})
	return &wg
}

// newTransactionalServer — serveur + store + envs éphémères (ni APP_PUBLIC_URL
// ni ALLOWED_ORIGIN : le lien replie sur l'URL canonique, déterministe).
func newTransactionalServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ADMIN_PASSWORD", "admin-test-1234")
	t.Setenv("ADMIN_USERNAME", "")
	t.Setenv("ALLOWED_ORIGIN", "")
	t.Setenv("APP_PUBLIC_URL", "")
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New impossible : %v", err)
	}
	ts := startTestServer(st)
	t.Cleanup(ts.Close)
	return ts, st
}

// ---------------------------------------------------------------------------
// Gabarits — reçu de paiement
// ---------------------------------------------------------------------------

func TestReceiptEmailBuilders(t *testing.T) {
	paidAt := time.Date(2026, 3, 18, 14, 30, 0, 0, time.UTC)
	d := receiptEmailData{
		AccountID:    "acc-x",
		PlanLabel:    "MikCloud Hotspot Mensuel",
		PeriodLabel:  "1 mois",
		AmountFcfa:   15000,
		Method:       "carte bancaire (Stripe)",
		Ref:          "MC-AB12CD34",
		PaidAt:       paidAt,
		PeriodEnd:    paidAt.AddDate(0, 1, 0).Format(time.RFC3339),
		FrontendBase: "https://mikcloud.ftci.fr",
	}
	text := buildReceiptEmailText(d, "Awa Koné", "Cyber Yopougon")
	for _, want := range []string{
		"Awa Koné", "Cyber Yopougon", "MikCloud Hotspot Mensuel", "1 mois",
		"15 000 FCFA", "carte bancaire (Stripe)", "MC-AB12CD",
		"18 mars 2026", "Actif jusqu'au : 18 avril 2026",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("reçu texte : attendu %q, absent du corps :\n%s", want, text)
		}
	}
	html := buildReceiptEmailHTML(d, "Awa Koné", "Cyber Yopougon")
	for _, want := range []string{
		"REÇU DE PAIEMENT", "Paiement confirmé", "Awa Koné",
		"15 000 FCFA", "PAYÉ", "carte bancaire (Stripe)", "MC-AB12CD",
		"18 avril 2026", "color-scheme:light",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("reçu HTML : attendu %q, absent du corps", want)
		}
	}
}

// TestReceiptEmailHTMLEscape — un nom de compte hostile (<script>) reste du
// texte affiché : l'injection HTML dans le courriel brandé est impossible.
func TestReceiptEmailHTMLEscape(t *testing.T) {
	d := receiptEmailData{
		AccountID: "acc-x", PlanLabel: "Essentiel <b>gras</b>", PeriodLabel: "1 mois",
		AmountFcfa: 2500, Method: "Wave", Ref: "MC-1", PaidAt: time.Now().UTC(),
		PeriodEnd:    time.Now().AddDate(0, 1, 0).Format(time.RFC3339),
		FrontendBase: "https://mikcloud.ftci.fr",
	}
	html := buildReceiptEmailHTML(d, "Awa <script>alert(1)</script>", "Compte \" hostile")
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Fatal("reçu HTML : le script du nom n'est pas échappé")
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Fatal("reçu HTML : l'échappement du nom est absent")
	}
}

// ---------------------------------------------------------------------------
// Gabarits — bienvenue (Hotspot ET HomeNet : copies et essais distincts)
// ---------------------------------------------------------------------------

func TestWelcomeEmailBuildersHotspot(t *testing.T) {
	d := welcomeEmailData{
		OwnerName: "Awa Koné", AccountName: "Cyber Yopougon", Username: "awa",
		Usage: model.AccountUsageHotspot, TrialDays: 60,
		TrialEnd:     time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		FrontendBase: "https://mikcloud.ftci.fr",
	}
	text := buildWelcomeEmailText(d)
	for _, want := range []string{
		"60 jours", "awa", "Cyber Yopougon", "vouchers", "17 mai 2026", "/app",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("bienvenue texte (hotspot) : attendu %q, absent du corps :\n%s", want, text)
		}
	}
	html := buildWelcomeEmailHTML(d)
	for _, want := range []string{
		"VOTRE HOTSPOT", "Bienvenue sur MikCloud", "60 JOURS", "Connectez votre routeur", "ESSAI GRATUIT",
		"Ouvrir ma console", "mikcloud.ftci.fr/app",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("bienvenue HTML (hotspot) : attendu %q, absent du corps", want)
		}
	}
}

func TestWelcomeEmailBuildersHomeNet(t *testing.T) {
	d := welcomeEmailData{
		OwnerName: "Famille Diallo", AccountName: "Maison Cocody", Username: "diallo",
		Usage: model.AccountUsageHomeNet, TrialDays: 30,
		TrialEnd:     time.Now().AddDate(0, 0, 30).Format(time.RFC3339),
		FrontendBase: "https://mikcloud.ftci.fr",
	}
	text := buildWelcomeEmailText(d)
	if !strings.Contains(text, "30 jours") {
		t.Error("bienvenue texte (homenet) : essai 30 jours absent")
	}
	if !strings.Contains(text, "réseau familial") {
		t.Error("bienvenue texte (homenet) : copie maison absente")
	}
	html := buildWelcomeEmailHTML(d)
	if !strings.Contains(html, "VOTRE RÉSEAU MAISON") {
		t.Error("bienvenue HTML (homenet) : eyebrow maison absent")
	}
	if strings.Contains(html, "VOTRE HOTSPOT") {
		t.Error("bienvenue HTML (homenet) : copie hotspot présente à tort")
	}
}

// ---------------------------------------------------------------------------
// Mise en forme française
// ---------------------------------------------------------------------------

func TestFormatDateFrEtMontants(t *testing.T) {
	d := time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC)
	if got := formatDateFr(d); got != "31 décembre 2026" {
		t.Errorf("formatDateFr = %q", got)
	}
	if got := formatFcfaMail(1250); got != "1 250 FCFA" {
		t.Errorf("formatFcfaMail(1250) = %q", got)
	}
	if got := formatFcfaMail(12000000); got != "12 000 000 FCFA" {
		t.Errorf("formatFcfaMail(12000000) = %q", got)
	}
	if got := dateLabelOf("pas-une-date"); got != "pas-une-date" {
		t.Errorf("dateLabelOf (repli) = %q", got)
	}
}

// ---------------------------------------------------------------------------
// Bout en bout — inscription publique → e-mail de bienvenue + trace
// ---------------------------------------------------------------------------

func TestRegisterSendsWelcomeEmail(t *testing.T) {
	ts, st := newTransactionalServer(t)
	// Un compte fraîchement inscrit n'a AUCUN réglage e-mail : l'expéditeur
	// est celui du compte principal (plateforme — Resend en production,
	// N°67). On le sème comme le ferait l'exploitation réelle.
	seedResend(t, st, model.AccountMainID)

	var calls []sentEmail
	wg := stubAccountEmailCapture(t, &calls)

	status, body := doJSON(t, ts, "POST", "/api/auth/register", "", map[string]string{
		"name": "Awa Koné", "username": "awa-welcome", "password": "mot-de-passe-8+",
		"email": "awa@example.ci", "phone": "0701020304", "country": "ci", "city": "Abidjan",
		"usage": "hotspot",
	})
	if status != 201 {
		t.Fatalf("inscription = %d : %v", status, body)
	}
	wg.Wait() // la goroutine d'envoi termine après le déverrouillage du handler
	if len(calls) != 1 {
		t.Fatalf("un seul e-mail de bienvenue attendu, got %d", len(calls))
	}
	m := calls[0]
	if m.to != "awa@example.ci" {
		t.Errorf("destinataire = %q", m.to)
	}
	if !strings.Contains(m.title, "Bienvenue sur MikCloud") {
		t.Errorf("sujet = %q", m.title)
	}
	for _, want := range []string{"60 jours", "awa-welcome", "Awa Koné", "vouchers"} {
		if !strings.Contains(m.body, want) {
			t.Errorf("bienvenue texte : %q absent", want)
		}
	}
	if !strings.Contains(m.html, "Bienvenue sur MikCloud") || !strings.Contains(m.html, "60 JOURS") {
		t.Error("bienvenue HTML : blocs brandés absents")
	}
	// Trace d'historique : kind welcome, statut envoyé.
	st.Lock()
	found := false
	for _, l := range st.Data().NotifLog {
		if l.Kind == notify.KindWelcome && l.Status == "sent" {
			found = true
		}
	}
	st.Unlock()
	if !found {
		t.Error("aucune trace notif_log (kind welcome, sent) après l'inscription")
	}
}

// ---------------------------------------------------------------------------
// Bout en bout — résolution plateforme markPaid → reçu ; non payé → rien
// ---------------------------------------------------------------------------

// seedReceiptAccount — compte avec e-mail + propriétaire + Resend + demande
// en attente (essentiel legacy → hotspot-mensuel, 1 routeur, 2 500 FCFA).
func seedReceiptAccount(t *testing.T, st *store.Store, brID, ref string) {
	t.Helper()
	st.Lock()
	db := st.Data()
	db.Accounts = append(db.Accounts, model.Account{
		ID: "acc-recu", Name: "Cyber Yopougon", Status: "active",
		CreatedAt: model.NowISO(), Email: "fermier@example.ci",
		Phone: "0701020304", Country: "ci", City: "Abidjan",
		Usage: model.AccountUsageHotspot,
	})
	db.Users = append(db.Users, model.AdminUser{
		ID: "usr-recu", AccountID: "acc-recu", Name: "Awa Koné", Username: "fermier",
		Role: model.RoleOwner, CreatedAt: model.NowISO(),
	})
	db.BillingRequests = append(db.BillingRequests, model.BillingRequest{
		ID: brID, AccountID: "acc-recu", PlanID: "essentiel", PlanName: "Essentiel",
		AmountFcfa: 2500, PayMethod: "wave", PeriodLabel: "1 mois", RouterCount: 1,
		Ref: ref, Status: "pending",
	})
	st.Save()
	st.Unlock()
	seedResend(t, st, "acc-recu")
}

func TestAdminResolveMarkPaidSendsReceipt(t *testing.T) {
	ts, st := newTransactionalServer(t)
	seedReceiptAccount(t, st, "br-recu", "MC-TEST0001")
	seedUser(t, st, "usr-plat", "", "usr-plat", model.RolePlatformAdmin)
	token := auth.Sign(testJWTSecret, auth.NewClaims("usr-plat", "Plateforme", model.RolePlatformAdmin, "", 0))

	var calls []sentEmail
	wg := stubAccountEmailCapture(t, &calls)

	status, body := doJSON(t, ts, "POST", "/api/admin/billing-requests/br-recu/resolve", token,
		map[string]any{"action": "activate", "markPaid": true})
	if status != 200 {
		t.Fatalf("résolution = %d : %v", status, body)
	}
	wg.Wait()
	if len(calls) != 1 {
		t.Fatalf("un seul reçu attendu, got %d", len(calls))
	}
	m := calls[0]
	if m.to != "fermier@example.ci" {
		t.Errorf("destinataire = %q", m.to)
	}
	if !strings.Contains(m.title, "Reçu de paiement") || !strings.Contains(m.title, "2 500 FCFA") {
		t.Errorf("sujet = %q", m.title)
	}
	for _, want := range []string{
		"Awa Koné", "MikCloud Hotspot Mensuel", "2 500 FCFA", "Wave", "MC-TEST0001",
	} {
		if !strings.Contains(m.body, want) {
			t.Errorf("reçu texte : %q absent", want)
		}
	}
	// Trace d'historique : kind payment_receipt.
	st.Lock()
	found := false
	for _, l := range st.Data().NotifLog {
		if l.Kind == notify.KindPaymentReceipt && l.Status == "sent" {
			found = true
		}
	}
	st.Unlock()
	if !found {
		t.Error("aucune trace notif_log (kind payment_receipt, sent)")
	}
}

// TestAdminResolveUnpaidSendsNothing — extension OFFERTE (markPaid faux) :
// la période s'active mais AUCUN reçu n'est envoyé (ce n'est pas un paiement).
func TestAdminResolveUnpaidSendsNothing(t *testing.T) {
	ts, st := newTransactionalServer(t)
	seedReceiptAccount(t, st, "br-offert", "MC-TEST0002")
	seedUser(t, st, "usr-plat2", "", "usr-plat2", model.RolePlatformAdmin)
	token := auth.Sign(testJWTSecret, auth.NewClaims("usr-plat2", "Plateforme", model.RolePlatformAdmin, "", 0))

	var calls []sentEmail
	wg := stubAccountEmailCapture(t, &calls)

	status, body := doJSON(t, ts, "POST", "/api/admin/billing-requests/br-offert/resolve", token,
		map[string]any{"action": "activate", "markPaid": false})
	if status != 200 {
		t.Fatalf("résolution = %d : %v", status, body)
	}
	wg.Wait()
	if len(calls) != 0 {
		t.Fatalf("aucun reçu attendu pour une extension non encaissée, got %d", len(calls))
	}
}

// TestReceiptSkippedWithoutCredentials — compte sans fournisseur e-mail (ni
// Resend ni SMTP, aucun réglage plateforme) : l'encaissement réussit quand
// même, l'e-mail est silencieusement écarté (best-effort, jamais bloquant).
func TestReceiptSkippedWithoutCredentials(t *testing.T) {
	ts, st := newTransactionalServer(t)
	seedReceiptAccount(t, st, "br-sans-mail", "MC-TEST0003")
	// Aucun seedResend : NotifSettings vide.
	st.Lock()
	st.Data().NotifSettings = map[string]model.NotificationSettings{}
	st.Save()
	st.Unlock()
	seedUser(t, st, "usr-plat3", "", "usr-plat3", model.RolePlatformAdmin)
	token := auth.Sign(testJWTSecret, auth.NewClaims("usr-plat3", "Plateforme", model.RolePlatformAdmin, "", 0))

	var calls []sentEmail
	wg := stubAccountEmailCapture(t, &calls)

	status, body := doJSON(t, ts, "POST", "/api/admin/billing-requests/br-sans-mail/resolve", token,
		map[string]any{"action": "activate", "markPaid": true})
	if status != 200 {
		t.Fatalf("résolution = %d : %v", status, body)
	}
	wg.Wait()
	if len(calls) != 0 {
		t.Fatalf("aucun envoi attendu sans fournisseur configuré, got %d", len(calls))
	}
}
