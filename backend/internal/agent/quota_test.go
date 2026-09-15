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
// du contrat : préfixe de file, marqueur, ancrage EN TÊTE de liste (N°113 :
// place-before la PREMIÈRE file + repli move — la dynamique hotspot se nomme
// <user>, l'ancre historique par nom d'utilisateur ne trouvait JAMAIS rien
// et la file tombait SOUS la dynamique : premier-match gagnant, aucun
// bridage), compteurs CUMULÉS de l'utilisateur (jamais ceux de la session).
//
// N°116 — la décision v3 passe par les DEUX sources : compteurs utilisateur
// ET octets des sessions actives (max, jamais somme — cf. TestQuotaDecisionSourceV3).
func TestQuotaScriptsShape(t *testing.T) {
	for _, s := range []struct{ name, script string }{
		{"quotaTickScript", quotaTickScript},
		{"onLoginQuotaScript", onLoginQuotaScript},
	} {
		quotaScriptBalance(t, s.name, s.script)
		for _, token := range []string{
			`"mikq:"`,                             // lecture du marqueur en tête de commentaire
			`mikthrottle-`,                        // nom de la file de bridage
			`place-before=$qf`,                    // ancrage AVANT la première file (v2 N°113)
			`/queue simple move`,                  // repli : remontée en tête si place-before échoue
			`bytes-in`,                            // compteurs CUMULÉS utilisateur
			`bytes-out`,                           // (in + out comparés au quota du marqueur)
			`/queue simple`,                       // pose ET retrait de la file
			`/ip hotspot active get $sa bytes-in`, // v3 (N°116) : repli session PROUVÉ fiable
			`[/ip hotspot active find where user=$qu]`, // v3 : sessions de CET utilisateur
		} {
			if !strings.Contains(s.script, token) {
				t.Fatalf("%s : token %q absent", s.name, token)
			}
		}
		// N°113 — remove-then-add à CHAQUE évaluation (rafraîchit la cible
		// IP et la position) : le remove précède TOUJOURS l'add.
		if idxRemove, idxAdd := strings.Index(s.script, `/queue simple remove`), strings.Index(s.script, `place-before=$qf`); idxRemove < 0 || idxAdd < 0 || idxRemove > idxAdd {
			t.Fatalf("%s : le remove de la file doit précéder l'ancrage (remove=%d, add=%d)", s.name, idxRemove, idxAdd)
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
	// N°113 — l'ancre historique par nom d'utilisateur NU (sans chevrons)
	// ne doit PLUS exister : la dynamique hotspot se nomme <user>, cette
	// ancre ne trouvait jamais rien (bug du terrain N°106).
	for _, s := range []struct{ name, script string }{
		{"quotaTickScript", quotaTickScript},
		{"onLoginQuotaScript", onLoginQuotaScript},
	} {
		if strings.Contains(s.script, `find where name=$qu]`) {
			t.Fatalf("%s : l'ancre find where name=$qu (sans chevrons) ne doit plus exister — la dynamique se nomme <user>", s.name)
		}
	}
}

// TestQuotaDecisionSourceV3 — N°116 : la décision de bridage passe par le
// MAX des deux sources (compteurs UTILISATEUR, octets de la plus grosse
// session ACTIVE), JAMAIS par leur somme ni par les seuls compteurs
// utilisateur. Rappel terrain : le tick v2 armé (marqueur présent, scheduler
// v2 confirmé) restait silencieusement inactif — 219 Mo sans bridage — parce
// que la seule source était les compteurs utilisateur, illisibles ou figés
// sur RouterOS 7.24 (bi=bo=0 < quota à chaque tick).
func TestQuotaDecisionSourceV3(t *testing.T) {
	for _, s := range []struct{ name, script string }{
		{"quotaTickScript", quotaTickScript},
		{"onLoginQuotaScript", onLoginQuotaScript},
	} {
		// 1. Initialisation depuis des compteurs utilisateur (protégés on-error).
		if !strings.Contains(s.script, `:local eff ($bi + $bo)`) {
			t.Fatalf("%s : l'initialisation du cumul v3 (:local eff ($bi + $bo)) est absente", s.name)
		}
		// 2. Repli session : parcours des sessions ACTIVES de $qu uniquement.
		if !strings.Contains(s.script, `:foreach sa in=[/ip hotspot active find where user=$qu] do=`) {
			t.Fatalf("%s : le repli session v3 (foreach sur les sessions actives de $qu) est absent", s.name)
		}
		// 3. MAX (jamais somme) : la session ne remplace le cumul que si ELLE
		// est plus grosse — double comptage impossible quand les compteurs
		// utilisateur sont live (ils incluent déjà la session en cours).
		if !strings.Contains(s.script, `:if ($sse > $eff) do={ :set eff $sse }`) {
			t.Fatalf("%s : la mise à jour MAX du cumul v3 est absente", s.name)
		}
		// 4. La décision compare $eff (le cumul consolidé), pas ($bi + $bo).
		if !strings.Contains(s.script, `:if ($eff >= [:tonum $qb]) do=`) {
			t.Fatalf("%s : la comparaison v3 (:if ($eff >= ...)) est absente", s.name)
		}
		// GARDE ANTI-RÉGRESSION : l'ancienne décision v2 (comparaison directe
		// des seuls compteurs utilisateur) ne doit PLUS exister — c'est ELLE
		// qui laissait le tick silencieusement inactif sur le terrain.
		if strings.Contains(s.script, `($bi + $bo) >=`) {
			t.Fatalf("%s : la comparaison directe v2 (($bi + $bo) >=) ne doit plus exister — la décision passe par $eff", s.name)
		}
		// Le repli session est PROTÉGÉ : une session au compteur illisible
		// n'interrompt jamais l'évaluation (miroir des protections v2).
		if !strings.Contains(s.script, `+ [:tonum [:tostr [/ip hotspot active get $sa bytes-out]]]); :if ($sse > $eff)`) {
			t.Fatalf("%s : le repli session doit sommer bytes-in+bytes-out avant le MAX", s.name)
		}
	}
}

// TestReadStateQuotaTelemetry — N°116 : le read_state du chunk final rapporte
// la sonde qcounters (lisibilité des compteurs utilisateur — diagnostic à
// distance du mode de dégradation v3) et le champ throttle (files posées).
func TestReadStateQuotaTelemetry(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tk"}
	script := b.buildReadState(model.Command{ID: "cmd-rs1", Kind: model.CmdReadState, Payload: map[string]any{}})
	for _, token := range []string{
		`&throttle=". $rthr`,                                 // liste des files mikthrottle- (contrat N°106)
		`&qcounters=". $rqc`,                                 // sonde N°116
		`[/ip hotspot user get $uqc bytes-in]`,               // la sonde lit BIEN les compteurs utilisateur
		`:if ([:typeof $qcq] != "nil") do={ :set rqc "ok" }`, // lecture réussie = typeof non-nil
	} {
		if !strings.Contains(script, token) {
			t.Fatalf("read_state : token de télémétrie quota %q absent", token)
		}
	}
	// La sonde est dans le chunk FINAL uniquement (les chunks intermédiaires
	// ne rapportent que stotal/hosts — miroir sessions/throttle).
	if strings.Count(script, `&qcounters=`) != 1 {
		t.Fatalf("read_state : la sonde qcounters doit apparaître exactement une fois (chunk final), obtenu %d", strings.Count(script, `&qcounters=`))
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
	// N°116 — le tick déployé est bien v3 : la variable $eff (cumul MAX des
	// deux sources) doit survivre à l'échappement dans le on-event servi.
	if !strings.Contains(script, `\$eff`) {
		t.Fatal("buildQuotaEnsure : la décision v3 ($eff) doit être embarquée échappée dans le on-event")
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
//
// N°113 — LE test de régression du bug du terrain : le set qui suit TOUJOURS
// le add dans profileEnsureLine doit porter LUI AUSSI les scripts de bridage
// (AVANT : il écrasait on-login avec le verrou seul et n'alignait jamais
// on-logout — le on-login de bridage était effacé dans la MÊME commande, la
// fenêtre de re-login n'était couverte nulle part).
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

	// N°113 — la ligne SET (2e ligne de profileEnsureLine) doit ELLE-MÊME
	// porter le on-login de bridage (mikq:) et le on-logout : le add
	// échoue sur un profil préexistant, le set est alors la SEULE écriture.
	setLine := profileSetLine(throttle.Name, throttle)
	if !strings.Contains(setLine, "mikq:") {
		t.Fatal("profil throttle : le SET doit porter le on-login de bridage (régression N°113 — il l'écrasait avant)")
	}
	if !strings.Contains(setLine, rosScriptValue(onLogoutQuotaScript)) {
		t.Fatal("profil throttle : le SET doit aligner le on-logout de bridage")
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
	// N°113 — en mode cut, le set vide EXPLICITEMENT on-login et on-logout
	// (un profil repassé en « couper » perd ses scripts de bridage).
	setCut := profileSetLine(cut.Name, cut)
	if !strings.Contains(setCut, `on-login=""`) {
		t.Fatal("profil cut : le set doit vider on-login (alignement complet)")
	}
	if !strings.Contains(setCut, `on-logout=""`) {
		t.Fatal("profil cut : le set doit vider on-logout (alignement complet)")
	}

	// N°113 — verrou + bridage : le set porte le on-login COMBINÉ.
	both := ProfileRef{
		Name: "1h-lock-thr", RateLimit: "2M/2M", SessionTimeoutMin: 60, SharedUsers: 1,
		HasRate: true, HasTimeout: true, HasShared: true,
		LockFirstDevice: true, QuotaThrottle: true,
	}
	setBoth := profileSetLine(both.Name, both)
	if !strings.Contains(setBoth, "mikcloud_lock:") || !strings.Contains(setBoth, "mikq:") {
		t.Fatal("verrou + bridage : le set doit porter le on-login COMBINÉ (régression N°113)")
	}
}
