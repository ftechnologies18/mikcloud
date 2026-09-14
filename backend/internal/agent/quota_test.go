// Tests N°106 — mode bridage (quota data) : marqueur mikq:, scripts RouterOS
// du tick/on-login/on-logout, builders user_add/voucher_batch en mode
// throttle, scripts combinés du profil et buildQuotaEnsure.
package agent

import (
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

// TestQuotaMarker — format canonique « mikq:<octets>,<débit> ».
func TestQuotaMarker(t *testing.T) {
	if got := QuotaMarker(1073741824, "512k/512k"); got != "mikq:1073741824,512k/512k" {
		t.Fatalf("QuotaMarker = %q, attendu mikq:1073741824,512k/512k", got)
	}
	// Le marqueur ne doit contenir AUCUN séparateur neutralisé par l'import
	// (| ; & = % +) : il survivrait sinon mutilé à la relecture routeur.
	for _, bad := range []string{"|", ";", "&", "=", "%", "+"} {
		if strings.ContainsAny(QuotaMarker(1, "512k/512k"), bad) {
			t.Fatalf("séparateur interdit %q dans le marqueur", bad)
		}
	}
}

// TestPrefixQuotaComment — le marqueur est TOUJOURS en tête (survit à la
// troncature d'import à 60 caractères), suivi du commentaire existant.
func TestPrefixQuotaComment(t *testing.T) {
	if got := PrefixQuotaComment("mikq:5,1M", ""); got != "mikq:5,1M" {
		t.Fatalf("commentaire vide : %q", got)
	}
	if got := PrefixQuotaComment("mikq:5,1M", "custom · mikcloud:b1"); got != "mikq:5,1M custom · mikcloud:b1" {
		t.Fatalf("préfixage : %q", got)
	}
}

// quotaScriptBalance — compte les accolades d'un script RouterOS (aucun
// littéral des scripts N°106 n'en contient) : elles doivent s'équilibrer.
func quotaScriptBalance(t *testing.T, name, script string) {
	t.Helper()
	if n := strings.Count(script, "{"); n != strings.Count(script, "}") {
		t.Fatalf("%s : accolades déséquilibrées (%d ouvrantes / %d fermantes)", name,
			strings.Count(script, "{"), strings.Count(script, "}"))
	}
	if strings.Contains(script, "\n") {
		t.Fatalf("%s : le script doit être UNE LIGNE (embarqué dans on-event=\"…\")", name)
	}
}

// TestQuotaScriptsShape — les trois scripts du bridage portent les tokens
// du contrat : préfixe de file, marqueur, place-before la file dynamique,
// compteurs CUMULÉS de l'utilisateur (jamais ceux de la session).
func TestQuotaScriptsShape(t *testing.T) {
	for _, s := range []struct{ name, script string }{
		{"quotaTickScript", quotaTickScript},
		{"onLoginQuotaScript", onLoginQuotaScript},
	} {
		quotaScriptBalance(t, s.name, s.script)
		for _, token := range []string{
			`"mikq:"`,       // lecture du marqueur en tête de commentaire
			`mikthrottle-`,  // nom de la file de bridage
			`place-before`,  // au-dessus de la file dynamique du profil
			`bytes-in`,      // compteurs CUMULÉS utilisateur
			`bytes-out`,     // (in + out comparés au quota du marqueur)
			`/queue simple`, // pose ET retrait de la file
		} {
			if !strings.Contains(s.script, token) {
				t.Fatalf("%s : token %q absent", s.name, token)
			}
		}
	}
	quotaScriptBalance(t, "onLogoutQuotaScript", onLogoutQuotaScript)
	if !strings.Contains(onLogoutQuotaScript, `/queue simple remove`) {
		t.Fatal("onLogoutQuotaScript : le retrait de la file est absent")
	}
	// Le tick balaie les orphelins (files sans session active).
	if !strings.Contains(quotaTickScript, `name~"^mikthrottle-"`) {
		t.Fatal("quotaTickScript : le balayage d'orphelins est absent")
	}
	// Le tick est borné à 250 sessions par passage (miroir read_state).
	if !strings.Contains(quotaTickScript, `$qn2 < 250`) {
		t.Fatal("quotaTickScript : la borne de 250 sessions est absente")
	}
}

// TestBuildQuotaEnsure — remove-then-add idempotent du scheduler
// mikcloud-quota (pattern watcher N°77) + rapport au cloud.
func TestBuildQuotaEnsure(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tk"}
	cmd := model.Command{ID: "cmd-quota1", Kind: model.CmdQuotaEnsure}
	script := b.buildQuotaEnsure(cmd)
	for _, token := range []string{
		`/system scheduler remove [find name="mikcloud-quota"]`,
		`/system scheduler add name="mikcloud-quota" interval=20s start-time=startup on-event="`,
		`/agent/result?token=tk`,
		`# mikcloud cmd cmd-quota1 quota_ensure`,
	} {
		if !strings.Contains(script, token) {
			t.Fatalf("buildQuotaEnsure : %q absent", token)
		}
	}
	// L'on-event embarque le tick ÉCHAPPÉ ($ protégés — le niveau de
	// citation on-event="…" décode les siens).
	if !strings.Contains(script, `\$qn2`) {
		t.Fatal("buildQuotaEnsure : le tick doit être échappé pour on-event (variables \\$…)")
	}
}

// TestBuildUserAddThrottle — en mode throttle : PAS de limit-bytes-total
// (il déconnecterait — l'inverse du contrat), le quota vit dans le marqueur
// mikq: EN TÊTE du commentaire. Sans mode : comportement historique intact.
func TestBuildUserAddThrottle(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tk"}
	base := func(extra map[string]any) map[string]any {
		payload := map[string]any{
			"name":     "ticket-1",
			"password": "pass1234",
			"profile": map[string]any{
				"name": "1h-1go", "rateLimit": "2M/2M",
				"sessionTimeoutMin": 60, "sharedUsers": 1,
			},
			"comment": "custom · mikcloud:b1",
		}
		for k, v := range extra {
			payload[k] = v
		}
		return payload
	}

	// Mode throttle : marqueur en tête, AUCUN limit-bytes-total.
	cmd := model.Command{ID: "cmd-ua1", Kind: model.CmdUserAdd, Payload: base(map[string]any{
		"limitBytesTotal": int64(1073741824),
		"throttleRate":    "512k/512k",
		"profile": map[string]any{
			"name": "1h-1go", "rateLimit": "2M/2M",
			"sessionTimeoutMin": 60, "sharedUsers": 1,
			"quotaMode": "throttle",
		},
	})}
	script := b.buildUserAdd(cmd)
	if strings.Contains(script, "limit-bytes-total") {
		t.Fatal("mode throttle : limit-bytes-total interdit (déconnexion native)")
	}
	if want := `mikq:1073741824,512k/512k custom · mikcloud:b1`; !strings.Contains(script, want) {
		t.Fatalf("marqueur mikq: en tête attendu dans le commentaire : %q", want)
	}
	if !strings.Contains(script, "limit-uptime=1h") {
		t.Fatal("le quota temps (limit-uptime) doit rester posé en mode bridage")
	}

	// Sans mode (héritage) : limit-bytes-total posé, aucun marqueur.
	cmd2 := model.Command{ID: "cmd-ua2", Kind: model.CmdUserAdd, Payload: base(map[string]any{
		"limitBytesTotal": int64(1073741824),
	})}
	script2 := b.buildUserAdd(cmd2)
	if !strings.Contains(script2, "limit-bytes-total=1073741824") {
		t.Fatal("mode cut (défaut) : limit-bytes-total doit rester posé")
	}
	if strings.Contains(script2, "mikq:") {
		t.Fatal("mode cut : aucun marqueur mikq: attendu")
	}
}

// TestBuildVoucherBatchThrottle — même contrat pour les lots : le marqueur
// est préfixé au commentaire de traçabilité, limit-bytes-total retiré.
func TestBuildVoucherBatchThrottle(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tk"}
	cmd := model.Command{ID: "cmd-vb1", Kind: model.CmdVoucherBatch, Payload: map[string]any{
		"profile": map[string]any{
			"name": "1h-1go", "rateLimit": "2M/2M",
			"sessionTimeoutMin": 60, "sharedUsers": 1,
			"quotaMode": "throttle",
		},
		"users": []map[string]any{
			{"name": "vc-a1", "password": "p1"},
			{"name": "vc-a2", "password": "p2"},
		},
		"batch":           "b-thr1",
		"comment":         "promo",
		"limitBytesTotal": int64(5368709120), // 5 Go (override par lot)
		"throttleRate":    "1M",
	}}
	script := b.buildVoucherBatch(cmd)
	if strings.Contains(script, "limit-bytes-total") {
		t.Fatal("mode throttle : limit-bytes-total interdit dans le lot")
	}
	if want := `mikq:5368709120,1M promo · mikcloud:b-thr1`; !strings.Contains(script, want) {
		t.Fatalf("marqueur du lot (quota override + rate) attendu : %q", want)
	}
}

// TestProfileOnLoginScriptCombiné — un seul champ on-login par profil : le
// verrou « 1er appareil » et le bridage quota s'y concatènent.
func TestProfileOnLoginScriptCombiné(t *testing.T) {
	if got := profileOnLoginScript(ProfileRef{}); got != "" {
		t.Fatalf("aucun verrou ni bridage : on-login vide attendu, obtenu %d caractères", len(got))
	}
	onlyLock := profileOnLoginScript(ProfileRef{LockFirstDevice: true})
	if !strings.Contains(onlyLock, "mikcloud_lock:") || strings.Contains(onlyLock, "mikq:") {
		t.Fatal("verrou seul : le script de liaison MAC uniquement")
	}
	onlyThrottle := profileOnLoginScript(ProfileRef{QuotaThrottle: true})
	if !strings.Contains(onlyThrottle, "mikq:") || strings.Contains(onlyThrottle, "mikcloud_lock:") {
		t.Fatal("bridage seul : le script de quota uniquement")
	}
	both := profileOnLoginScript(ProfileRef{LockFirstDevice: true, QuotaThrottle: true})
	if !strings.Contains(both, "mikcloud_lock:") || !strings.Contains(both, "mikq:") {
		t.Fatal("verrou + bridage : les deux scripts concaténés attendus")
	}
}

// TestProfileEnsureThrottleScripts — profileEnsureLine pose le on-login
// combiné ET le on-logout en mode throttle ; sans mode, on-login est vidé
// (alignement complet du profil — sémantique set existante).
func TestProfileEnsureThrottleScripts(t *testing.T) {
	throttle := ProfileRef{
		Name: "1h-1go", RateLimit: "2M/2M", SessionTimeoutMin: 60, SharedUsers: 1,
		HasRate: true, HasTimeout: true, HasShared: true,
		QuotaThrottle: true,
	}
	script := profileEnsureLine(throttle)
	if !strings.Contains(script, `on-login="`) || !strings.Contains(script, "mikq:") {
		t.Fatal("profil throttle : le on-login de bridage doit être posé")
	}
	if !strings.Contains(script, `on-logout="`) {
		t.Fatal("profil throttle : le on-logout (retrait de la file) doit être posé")
	}

	cut := ProfileRef{
		Name: "1h-cut", RateLimit: "2M/2M", SessionTimeoutMin: 60, SharedUsers: 1,
		HasRate: true, HasTimeout: true, HasShared: true,
	}
	scriptCut := profileEnsureLine(cut)
	if strings.Contains(scriptCut, "on-logout=\"") {
		// profileSetLine vide on-logout (alignement) : la ligne add ne pose
		// JAMAIS de on-logout en mode cut — seul le set le vide explicitement.
		if strings.Contains(scriptCut, `on-logout="`+rosScriptValue(onLogoutQuotaScript)) {
			t.Fatal("profil cut : le script de bridage ne doit pas être posé dans on-logout")
		}
	}
}
