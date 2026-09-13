package agent

// Tests N°97 — docteur pool IP : formes du script selon le payload
// (diagnostic pur / recyclage / extension), assainissement des tokens
// pilotés (anti-injection), défauts sur payload vide.

import (
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

// poolDoctorCmd — commande du docteur avec payload minimal.
func poolDoctorCmd(payload map[string]any) model.Command {
	return model.Command{ID: "cmd-pd97", Kind: model.CmdPoolDoctor, Payload: payload}
}

// TestPoolDoctorDiagnosticOnly — l'auto-diagnostic du check-in (payload
// sans clés) ne MODIFIE RIEN sur le routeur : aucun set de timeouts, aucun
// address-per-mac, aucune extension — les corrections restent des gestes
// explicites du gérant.
func TestPoolDoctorDiagnosticOnly(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tk"}
	script, err := b.ScriptFor(poolDoctorCmd(map[string]any{}))
	if err != nil {
		t.Fatalf("ScriptFor impossible : %v", err)
	}
	for _, forbidden := range []string{
		"/ip hotspot set [find]",
		"address-per-mac=1",
		"/ip pool set",
		"/ip address add",
		"/ip hotspot network add",
		"/ip firewall nat add",
	} {
		if strings.Contains(script, forbidden) {
			t.Errorf("diagnostic pur : %q ne doit PAS apparaître", forbidden)
		}
	}
	// Lectures présentes : hôtes, actifs, pools, serveurs, profils.
	for _, needle := range []string{
		"[/ip hotspot host print count-only]",
		"[/ip hotspot active print count-only]",
		"[/ip pool find]",
		"[/ip hotspot find]",
		"[/ip hotspot profile find]",
		"/agent/result?token=",
	} {
		if !strings.Contains(script, needle) {
			t.Errorf("diagnostic : %q absent", needle)
		}
	}
}

// TestPoolDoctorRecycle — recyclage : timeouts serveurs + address-per-mac,
// avec les valeurs par défaut (5m/10m/2m).
func TestPoolDoctorRecycle(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tk"}
	script, err := b.ScriptFor(poolDoctorCmd(map[string]any{"recycle": true}))
	if err != nil {
		t.Fatalf("ScriptFor impossible : %v", err)
	}
	if !strings.Contains(script, "/ip hotspot set [find] login-timeout=5m idle-timeout=10m keepalive-timeout=2m") {
		t.Error("timeouts de recyclage absents ou non conformes aux défauts")
	}
	if !strings.Contains(script, "/ip hotspot profile set [find] address-per-mac=1") {
		t.Error("address-per-mac=1 absent")
	}
	// Le recyclage n'étend PAS le pool : pas de commande d'extension.
	if strings.Contains(script, "/ip pool set") || strings.Contains(script, "mikcloud-pool-nat") {
		t.Error("recyclage seul ne doit pas étendre le pool")
	}
}

// TestPoolDoctorExtend — extension : range dédié ajouté aux pools des
// profils, IP secondaire sur l'interface, entrée network masquerade et
// règle NAT marquée (idempotence par find).
func TestPoolDoctorExtend(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tk"}
	script, err := b.ScriptFor(poolDoctorCmd(map[string]any{"recycle": true, "extend": true}))
	if err != nil {
		t.Fatalf("ScriptFor impossible : %v", err)
	}
	for _, needle := range []string{
		`ranges=($pr . "," . "` + PoolDoctorExtRange + `")`,
		`/ip pool add name="` + PoolDoctorPoolName + `" ranges="` + PoolDoctorExtRange + `"`,
		`/ip address add address="` + PoolDoctorExtGateway + `"`,
		`/ip hotspot network add address="` + PoolDoctorExtSubnet + `" masquerade=yes comment="mikcloud-pool"`,
		`/ip firewall nat add chain=srcnat src-address="` + PoolDoctorExtSubnet + `" action=masquerade comment="mikcloud-pool-nat"`,
	} {
		if !strings.Contains(script, needle) {
			t.Errorf("extension : %q absent", needle)
		}
	}
	// Idempotence : les find-before-add de l'IP secondaire et du NAT.
	if !strings.Contains(script, `[/ip address find where address="`+PoolDoctorExtGateway+`" and interface=$sif]`) {
		t.Error("garde idempotence IP secondaire absente")
	}
	if !strings.Contains(script, `[/ip firewall nat find where comment="mikcloud-pool-nat"]`) {
		t.Error("garde idempotence NAT absente")
	}
	// Rapport : drapeaux recycled/extended + hôtes + pools.
	for _, needle := range []string{`&recycled=".`, `&extended=".`, `&hosts=".`, `&pools=".`} {
		if !strings.Contains(script, needle) {
			t.Errorf("rapport : %q absent", needle)
		}
	}
}

// TestPoolDoctorSanitizesHostilePayload — un range piloté contenant des
// caractères hostiles est REJETÉ en bloc → défaut 10.77 retombe (jamais
// de valeur partielle injectée dans le .rsc).
func TestPoolDoctorSanitizesHostilePayload(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tk"}
	script, err := b.ScriptFor(poolDoctorCmd(map[string]any{
		"recycle":      true,
		"extend":       true,
		"extRange":     "10.0.0.2-10.0.0.9; /system identity set name=pwned",
		"loginTimeout": "5m\n/ip pool remove [find]",
	}))
	if err != nil {
		t.Fatalf("ScriptFor impossible : %v", err)
	}
	if strings.Contains(script, "pwned") || strings.Contains(script, "identity set") {
		t.Error("charge hostile du payload retrouvée dans le script")
	}
	if !strings.Contains(script, PoolDoctorExtRange) {
		t.Error("valeur hostile doit retomber sur le range par défaut")
	}
	if !strings.Contains(script, "login-timeout=5m") {
		t.Error("timeout hostile doit retomber sur le défaut 5m")
	}
}

// TestPoolDoctorCustomPayloads — les timeouts et range custom (sains)
// passent tels quels.
func TestPoolDoctorCustomPayloads(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tk"}
	script, err := b.ScriptFor(poolDoctorCmd(map[string]any{
		"recycle":          true,
		"extend":           true,
		"extRange":         "10.99.0.10-10.99.0.254",
		"loginTimeout":     "2m",
		"idleTimeout":      "15m",
		"keepaliveTimeout": "3m",
	}))
	if err != nil {
		t.Fatalf("ScriptFor impossible : %v", err)
	}
	if !strings.Contains(script, "login-timeout=2m idle-timeout=15m keepalive-timeout=3m") {
		t.Error("timeouts custom absents")
	}
	if !strings.Contains(script, "10.99.0.10-10.99.0.254") {
		t.Error("range custom absent")
	}
}
