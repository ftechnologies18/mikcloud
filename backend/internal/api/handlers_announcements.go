// handlers_announcements.go — N°152 : diffusion de messages de la plateforme
// aux comptes clients MikCloud (super-admin).
//
// Console plateforme (rang 3) :
//
//	GET    /api/admin/announcements          → toutes (tri récentes, pour l'historique)
//	POST   /api/admin/announcements          → crée + journalise + e-mail optionnel
//	                                         (N°165 : publishAt futur = programmée,
//	                                         e-mail différé au moment de la publication)
//	DELETE /api/admin/announcements/{id}     → retire définitivement
//
// Côté clients (rang 2) :
//
//	GET    /api/announcements                → annonces ACTIVES pour le compte
//	                                         (audience × programmation × expiration ;
//	                                         le compte principal plateforme n'en reçoit pas)
//
// La cloche (GET /api/bell, N°151) injecte les annonces actives en items
// synthétiques type « announcement » : elles comptent dans le badge via le
// read-state serveur (une annonce créée après le dernier acquit = non lue),
// sans dupliquer une ligne d'activité par compte.
package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/notify"
)

// ---------------------------------------------------------------------------
// Console plateforme
// ---------------------------------------------------------------------------

// handleAnnouncementsList — GET /api/admin/announcements (rang 3) : toutes
// les annonces, triées de la plus récente à la plus ancienne. La réponse
// porte le nombre de comptes actifs destinataires de CHAQUE annonce
// (audience) — l'opérateur voit la portée réelle de sa diffusion.
func (a *API) handleAnnouncementsList(w http.ResponseWriter, r *http.Request) {
	// requireRole(3) laisse passer un OWNER de compte client (rang 3) : les
	// annonces parlent au nom de la PLATEFORME — garde isPlatformAdmin, comme
	// toutes les routes /api/admin/* (défense en profondeur).
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}

	a.store.Lock()
	db := a.store.Data()
	type row struct {
		model.Announcement
		Active        bool   `json:"active"`        // visible des clients à l'instant présent
		AccountsCount int    `json:"accountsCount"` // comptes clients actifs correspondant à l'audience
		State         string `json:"state"`         // N°165 — active | scheduled | expired
	}
	rows := []row{}
	now := model.NowISO()
	activeByUsage := map[string]int{"all": 0, "hotspot": 0, "homenet": 0}
	for _, acc := range db.Accounts {
		if acc.ID == model.AccountMainID || acc.Status != "active" {
			continue
		}
		usage := normalizeAccountUsage(acc.Usage)
		activeByUsage["all"]++
		activeByUsage[usage]++
	}
	for _, ann := range db.Announcements {
		rows = append(rows, row{
			Announcement:  ann,
			Active:        ann.Active("hotspot", now) || ann.Active("homenet", now),
			AccountsCount: activeByUsage[ann.Audience],
			State:         ann.State(now), // N°165 — état calculé, badge de la liste
		})
	}
	a.store.Unlock()
	writeJSON(w, http.StatusOK, rows)
}

// handleAnnouncementCreate — POST /api/admin/announcements (rang 3).
// Corps : {title, body?, level, audience, expiresInDays?, expiresInHours?,
// email?, publishAt?} — validations strictes (le super-admin écrit à TOUS
// les clients : jamais de titre vide ou de niveau inconnu). N°165 —
// publishAt (RFC 3339) futur : l'annonce est PROGRAMMÉE, invisible des
// clients jusqu'à cette date, et l'éventuel e-mail part au moment de la
// publication (EmailPending + balayage d'annonces), jamais avant.
// N°179 — durée de visibilité en JOURS et/ou HEURES (durée totale = jours
// + heures, plafonnée à 365 jours ; une maintenance de 6 h se règle enfin
// à l'heure près sans jamais « traîner » des jours entiers).
func (a *API) handleAnnouncementCreate(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}

	var req struct {
		Title          string `json:"title"`
		Body           string `json:"body"`
		Level          string `json:"level"`
		Audience       string `json:"audience"`
		ExpiresInDays  int    `json:"expiresInDays"`
		ExpiresInHours int    `json:"expiresInHours"` // N°179 — granularité horaire
		Email          bool   `json:"email"`
		PublishAt      string `json:"publishAt"` // N°165 — RFC 3339 ; futur = programmée
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "Corps de requête invalide")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	if len(req.Title) < 3 || len(req.Title) > 120 {
		writeErr(w, http.StatusBadRequest, "Le titre est requis (3 à 120 caractères)")
		return
	}
	if len(req.Body) > 2000 {
		writeErr(w, http.StatusBadRequest, "Le message est trop long (2000 caractères max)")
		return
	}
	levelOK := false
	for _, lv := range model.AnnouncementLevels {
		if req.Level == lv {
			levelOK = true
			break
		}
	}
	if !levelOK {
		writeErr(w, http.StatusBadRequest, "Niveau invalide (info, success, maintenance, warning ou critical)")
		return
	}
	switch req.Audience {
	case model.AnnouncementAudienceAll, model.AnnouncementAudienceHotspot, model.AnnouncementAudienceHomeNet:
	default:
		writeErr(w, http.StatusBadRequest, "Audience invalide (all, hotspot ou homenet)")
		return
	}
	expires := ""
	// N°179 — durée TOTALE = jours + heures (les deux bornes seules ne
	// veulent rien dire : 365 jours + 23 h reste > 365 j). Plancher implicite :
	// toute durée fournie vit au moins une heure.
	if req.ExpiresInDays != 0 || req.ExpiresInHours != 0 {
		if req.ExpiresInDays < 0 || req.ExpiresInDays > 365 {
			writeErr(w, http.StatusBadRequest, "Expiration : 0 à 365 jours")
			return
		}
		if req.ExpiresInHours < 0 || req.ExpiresInHours > 365*24 {
			writeErr(w, http.StatusBadRequest, "Expiration : 0 à 8760 heures")
			return
		}
		dur := time.Duration(req.ExpiresInDays)*24*time.Hour + time.Duration(req.ExpiresInHours)*time.Hour
		if dur <= 0 || dur > 365*24*time.Hour {
			writeErr(w, http.StatusBadRequest, "Durée de visibilité : 1 heure à 365 jours maximum")
			return
		}
		expires = time.Now().UTC().Add(dur).Format(time.RFC3339)
	}

	// N°165 — programmation : publishAt RFC 3339 optionnel. Vide ou PASSÉ =
	// diffusion immédiate (comportement historique) ; futur = l'annonce devient
	// visible d'elle-même à cette date (Active borne chaque lecture, aucune
	// action de fond pour « publier »). Bornée à 365 jours d'avance.
	publish := ""
	if req.PublishAt != "" {
		pt, err := time.Parse(time.RFC3339, req.PublishAt)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Date de diffusion invalide (horodatage attendu)")
			return
		}
		now := time.Now().UTC()
		if pt.After(now.Add(365 * 24 * time.Hour)) {
			writeErr(w, http.StatusBadRequest, "Programmation : 365 jours maximum à l'avance")
			return
		}
		if pt.After(now) {
			publish = pt.UTC().Format(time.RFC3339)
		}
	}

	claims := claimsFrom(r)
	ann := model.Announcement{
		Title: req.Title, Body: req.Body, Level: req.Level, Audience: req.Audience,
		PublishAt: publish, ExpiresAt: expires,
	}
	if claims != nil {
		ann.CreatedBy, ann.CreatedByName = claims.Sub, claims.Name
	}
	// N°165 — annonce programmée + e-mail demandé : l'envoi est DIFFÉRÉ à la
	// publication (RunAnnouncementSweep) — jamais d'e-mail avant que le
	// bandeau n'apparaisse. Une annonce immédiate envoie tout de suite.
	deferredEmail := false
	if publish != "" && req.Email {
		ann.EmailPending = true
		deferredEmail = true
	}

	// Résolution des destinataires e-mail SOUS le verrou (copies), envoi en
	// goroutine après (discipline N°146 : la réponse HTTP n'attend jamais
	// Resend/SMTP, et AUCUNE lecture d'état hors verrou).
	var targets []announcementMailTarget

	a.store.Lock()
	db := a.store.Data()
	// ID/Création posés AVANT l'insertion : AppendAnnouncement respecte les
	// valeurs fournies, et la réponse renvoie l'annonce TELLE que diffusée.
	ann.ID = model.NewID("ann-")
	ann.CreatedAt = model.NowISO()
	model.AppendAnnouncement(db, ann)
	if req.Email && !deferredEmail {
		ann.EmailedAt = model.NowISO()
		ann.EmailedCount = 0
		targets = resolveAnnouncementTargetsLocked(db, ann)
		ann.EmailedCount = len(targets)
		// la copie en tête porte la trace de diffusion
		db.Announcements[0] = ann
	}
	logMsg := "Annonce diffusée aux clients (" + announceScopeLabel(req.Audience) + ") : «" + ann.Title + "»"
	if publish != "" {
		if pt, err := time.Parse(time.RFC3339, publish); err == nil {
			logMsg = "Annonce programmée pour le " + formatDateFr(pt) + " (" + announceScopeLabel(req.Audience) + ") : «" + ann.Title + "»"
		}
	}
	a.logActivityBy(r, db, "", "system", logMsg)
	a.store.Save()
	a.store.Unlock()

	// E-mails best-effort : un par compte destinataire, expéditeurs déjà
	// résolus sous verrou — la goroutine ne touche plus l'état partagé.
	// (Programmée : RIEN maintenant — le balayage enverra à la publication.)
	if !deferredEmail {
		a.sendAnnouncementEmails(ann, targets)
	}

	writeJSON(w, http.StatusCreated, ann)
}

// handleAnnouncementDelete — DELETE /api/admin/announcements/{id} (rang 3).
func (a *API) handleAnnouncementDelete(w http.ResponseWriter, r *http.Request) {
	if !isPlatformAdmin(r) {
		writeErr(w, http.StatusForbidden, "Réservé aux administrateurs de la plateforme")
		return
	}

	id := r.PathValue("id")
	a.store.Lock()
	db := a.store.Data()
	found := false
	title := ""
	kept := db.Announcements[:0]
	for _, ann := range db.Announcements {
		if ann.ID == id {
			found = true
			title = ann.Title
			continue
		}
		kept = append(kept, ann)
	}
	if !found {
		a.store.Unlock()
		writeErr(w, http.StatusNotFound, "Annonce introuvable")
		return
	}
	db.Announcements = kept
	a.logActivityBy(r, db, "", "system", "Annonce retirée : «"+title+"»")
	a.store.Save()
	a.store.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// announceScopeLabel — libellé humain de l'audience pour le journal.
func announceScopeLabel(audience string) string {
	switch audience {
	case model.AnnouncementAudienceHotspot:
		return "comptes Hotspot"
	case model.AnnouncementAudienceHomeNet:
		return "comptes HomeNet"
	default:
		return "tous les comptes"
	}
}

// ---------------------------------------------------------------------------
// Destinataires e-mail d'une annonce (N°165 — factorisés : corps de la
// création immédiate ET balayage de publication, announcement_sweep.go)
// ---------------------------------------------------------------------------

// announcementMailTarget — copie de destinataire résolue SOUS le verrou
// (l'envoi, lui, part en goroutine après déverrouillage — discipline N°146).
type announcementMailTarget struct {
	acc  model.Account
	name string // nom de salutation (propriétaire sinon compte)
	cfg  model.NotificationSettings
}

// resolveAnnouncementTargetsLocked — comptes destinataires d'une annonce :
// actifs, dans l'audience, e-mail connu, expéditeur résoluble. TOUJOURS sous
// le verrou du store ; ne retourne que des copies. (N°165 — extrait du corps
// de la création, sémantique inchangée.)
func resolveAnnouncementTargetsLocked(db *model.DB, ann model.Announcement) []announcementMailTarget {
	targets := []announcementMailTarget{}
	for _, acc := range db.Accounts {
		if acc.ID == model.AccountMainID || acc.Status != "active" {
			continue // pas le compte plateforme, pas les comptes désactivés
		}
		if ann.Audience != model.AnnouncementAudienceAll &&
			ann.Audience != normalizeAccountUsage(acc.Usage) {
			continue
		}
		if strings.TrimSpace(acc.Email) == "" {
			continue // compte sans e-mail connu : bandeau + cloche suffisent
		}
		cfg, ok := transactionalSenderLocked(db, acc.ID)
		if !ok {
			continue // aucun expéditeur exploitable : silencieux, jamais bloquant
		}
		name := acc.Name
		for _, u := range db.Users {
			if u.AccountID == acc.ID && u.Role == model.RoleOwner && u.Name != "" {
				name = u.Name
				break
			}
		}
		targets = append(targets, announcementMailTarget{acc: acc, name: name, cfg: cfg})
	}
	return targets
}

// sendAnnouncementEmails — file les envois best-effort (un par compte
// destinataire) APRÈS déverrouillage : ni la réponse HTTP, ni le passage de
// balayage, n'attendent Resend/SMTP.
func (a *API) sendAnnouncementEmails(ann model.Announcement, targets []announcementMailTarget) {
	if len(targets) == 0 {
		return
	}
	title, textBody, htmlBody := buildAnnouncementEmail(ann)
	for _, tg := range targets {
		logBody := "Annonce plateforme — " + ann.Title
		closureTarget := tg
		emailTaskDispatch()(func() {
			a.dispatchAccountEmail(notify.KindAnnouncement, title, logBody, closureTarget.cfg,
				strings.TrimSpace(closureTarget.acc.Email), textBody+closureTarget.name, htmlBody)
		})
	}
}

// ---------------------------------------------------------------------------
// Côté clients
// ---------------------------------------------------------------------------

// handleClientAnnouncements — GET /api/announcements (rang 2) : les annonces
// ACTIVES pour le compte du porteur (audience × programmation × expiration),
// les plus récentes d'abord, plafonnées à 10 — le bandeau console et la
// destination « tout voir » de la cloche y lisent la même vérité. Le compte
// principal (plateforme) n'est pas un client : liste vide.
func (a *API) handleClientAnnouncements(w http.ResponseWriter, r *http.Request) {
	acc := accountScope(r)
	usage := model.AccountUsageHotspot
	a.store.Lock()
	db := a.store.Data()
	if acc == model.AccountMainID {
		a.store.Unlock()
		writeJSON(w, http.StatusOK, []model.Announcement{})
		return
	}
	for _, a2 := range db.Accounts {
		if a2.ID == acc {
			usage = normalizeAccountUsage(a2.Usage)
			break
		}
	}
	now := model.NowISO()
	out := []model.Announcement{}
	for _, ann := range db.Announcements {
		if ann.Active(usage, now) {
			out = append(out, ann)
		}
	}
	a.store.Unlock()
	// N°165 — tri par date EFFECTIVE décroissante puis plafond 10 : une
	// annonce programmée qui vient d'être publiée précède une info rédigée
	// avant elle (l'insertion en tête suit la date de RÉDACTION, pas celle de
	// diffusion) — le bandeau montre toujours l'annonce la plus pertinente du
	// moment. Miroir du tri de la cloche (EffectiveAt).
	sort.Slice(out, func(i, j int) bool { return out[i].EffectiveAt() > out[j].EffectiveAt() })
	if len(out) > 10 {
		out = out[:10]
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------------------
// E-mail d'annonce (gabarit Aurora Emerald, discipline N°146)
// ---------------------------------------------------------------------------

// buildAnnouncementEmail — gabarits texte + HTML de l'e-mail d'annonce
// (l'expéditeur et la trace vivent dans dispatchAccountEmail). Le texte est
// suffixé du nom du destinataire par l'appelant (salutation personnalisée).
func buildAnnouncementEmail(ann model.Announcement) (title, textBody, htmlBody string) {
	title = "MikCloud — Annonce : " + ann.Title
	// N°179 — 5 niveaux : l'émeraude passe aux nouveautés, la sarcelle à la
	// maintenance, l'info redevient neutre (miroir du bandeau console).
	levelLabel := map[string]string{
		model.AnnouncementInfo:        "Information",
		model.AnnouncementSuccess:     "Nouveauté",
		model.AnnouncementMaintenance: "Maintenance planifiée",
		model.AnnouncementWarning:     "Action recommandée",
		model.AnnouncementCritical:    "Incident en cours",
	}[ann.Level]
	if levelLabel == "" {
		levelLabel = "Information"
	}

	var b strings.Builder
	b.WriteString("MikCloud — " + levelLabel + "\n\n")
	b.WriteString(ann.Title + "\n")
	if ann.Body != "" {
		b.WriteString(strings.Repeat("-", 40) + "\n" + ann.Body + "\n" + strings.Repeat("-", 40) + "\n")
	}
	if ann.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, ann.ExpiresAt); err == nil {
			b.WriteString("\nCette annonce s'affiche dans votre console jusqu'au " + formatDateFr(t) + ".\n")
		}
	}
	b.WriteString("\nL'annonce reste disponible dans la cloche de votre console MikCloud.\n\n— L'équipe MikCloud\n")
	textBody = b.String()

	// ── HTML ──
	const aurora = "linear-gradient(90deg,#009558 0%,#009073 55%,#008687 100%)"
	levelColor := map[string]string{
		model.AnnouncementInfo:        "#53645C",
		model.AnnouncementSuccess:     "#009558",
		model.AnnouncementMaintenance: "#0F766E",
		model.AnnouncementWarning:     "#B45309",
		model.AnnouncementCritical:    "#B91C1C",
	}[ann.Level]
	if levelColor == "" {
		levelColor = "#53645C"
	}
	levelBg := map[string]string{
		model.AnnouncementInfo:        "#F2F6F4",
		model.AnnouncementSuccess:     "#E7F6EE",
		model.AnnouncementMaintenance: "#E6F4F2",
		model.AnnouncementWarning:     "#FCF3E3",
		model.AnnouncementCritical:    "#FDECEC",
	}[ann.Level]
	if levelBg == "" {
		levelBg = "#F2F6F4"
	}
	esc := func(s string) string {
		s = strings.ReplaceAll(s, "&", "&amp;")
		s = strings.ReplaceAll(s, "<", "&lt;")
		s = strings.ReplaceAll(s, ">", "&gt;")
		s = strings.ReplaceAll(s, "\"", "&quot;")
		return s
	}
	bodyHTML := ""
	for _, line := range strings.Split(ann.Body, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		bodyHTML += "<p style=\"margin:0 0 10px;\">" + esc(line) + "</p>"
	}
	if bodyHTML == "" {
		bodyHTML = "<p style=\"margin:0;color:#53645C;\">" + esc(ann.Title) + "</p>"
	}
	expireNote := ""
	if ann.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, ann.ExpiresAt); err == nil {
			expireNote = "<p style=\"margin:14px 0 0;font-size:12px;color:#7C8F85;\">Affichée dans votre console jusqu'au <strong style=\"color:#53645C;\">" + formatDateFr(t) + "</strong>.</p>"
		}
	}
	year := time.Now().UTC().Year()
	htmlBody = fmt.Sprintf(`<!DOCTYPE html><html lang="fr"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background-color:#F2F6F4;">
<div style="display:none;max-height:0;overflow:hidden;">Annonce MikCloud — %s</div>
<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%%" style="background-color:#F2F6F4;padding:24px 12px;">
<tr><td align="center">
<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%%" style="max-width:560px;background-color:#FFFFFF;border-radius:14px;overflow:hidden;box-shadow:0 2px 12px rgba(16,32,25,.08);">
<tr><td bgcolor="#009558" height="6" style="background-color:#009558;background-image:%s;font-size:0;line-height:6px;">&nbsp;</td></tr>
<tr><td style="padding:30px 28px 8px;font-family:%s;">
<p style="margin:0 0 10px;"><span style="display:inline-block;padding:4px 12px;border-radius:999px;background-color:%s;color:%s;font-size:11px;font-weight:700;letter-spacing:.06em;text-transform:uppercase;">%s</span></p>
<h1 style="margin:0 0 16px;font-size:21px;line-height:1.35;color:#102019;">%s</h1>
%s
%s
<table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%%" style="width:100%%;margin:22px 0 0;">
<tr><td align="center">
<a href="https://mikcloud.ftci.fr/app" style="display:inline-block;padding:13px 28px;font-family:%s;font-size:15px;font-weight:700;color:#FFFFFF;text-decoration:none;border-radius:10px;background-color:#009558;">Ouvrir ma console&nbsp;→</a>
</td></tr></table>
</td></tr>
<tr><td bgcolor="#009558" height="6" style="background-color:#009558;background-image:%s;border-radius:0 0 14px 14px;font-size:0;line-height:6px;">&nbsp;</td></tr>
<tr><td style="padding:24px 16px 8px;font-family:%s;font-size:12px;line-height:1.7;color:#7C8F85;text-align:center;">
<p style="margin:0 0 4px;"><span style="font-weight:700;color:#53645C;">MikCloud</span> · la console de gestion hotspot MikroTik</p>
<p style="margin:0;font-size:11px;color:#6B7F76;">Vous recevez cet e-mail car un compte MikCloud est enregistré avec cette adresse.</p>
<p style="margin:10px 0 0;font-size:11px;color:#6B7F76;">© %d MikCloud · FTech CI</p>
</td></tr>
</table>
</td></tr></table>
</body></html>`,
		esc(ann.Title), aurora, emailFontStack,
		levelBg, levelColor, levelLabel, esc(ann.Title), bodyHTML, expireNote,
		emailFontStack, aurora, emailFontStack, year)
	return title, textBody, htmlBody
}
