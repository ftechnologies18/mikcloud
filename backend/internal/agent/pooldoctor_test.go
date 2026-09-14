package agent

// Tests N°97/N°108 — docteur pool IP : formes du script selon le payload
// (diagnostic pur / recyclage / extension), assainissement des tokens
// pilotés (anti-injection), défauts sur payload vide — et la garde de
// régression N°108 : JAMAIS d'écriture address-pool/addresses-per-mac sur
// /ip hotspot profile (le menu du profil n'a pas ces propriétés — le bug
// N°97 qui rendait l'extension muette en production).

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
// addresses-per-mac, aucun lease-time DHCP, aucune extension — les
// corrections restent des gestes explicites du gérant.
func TestPoolDoctorDiagnosticOnly(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tk"}
	script, err := b.ScriptFor(poolDoctorCmd(map[string]any{}))
	if err != nil {
		t.Fatalf("ScriptFor impossible : %v", err)
	}
	for _, forbidden := range []string{
		"/ip hotspot set [find]",
		"addresses-per-mac=1",
		"/ip pool set",
		"/ip pool add",
		"/ip pool remove",
		"/ip address add",
		"/ip hotspot network add",
		"/ip dhcp-server set",
		"/ip dhcp-server network add",
		"/ip firewall nat add",
	} {
		if strings.Contains(script, forbidden) {
			t.Errorf("diagnostic pur : %q ne doit PAS apparaître", forbidden)
		}
	}
	// Lectures présentes : hôtes, actifs, pools, serveurs (8 champs, avec
	// address-pool + addresses-per-mac RELUS sur le serveur), DHCP (4 champs,
	// avec lease-time).
	for _, needle := range []string{
		"[/ip hotspot host print count-only]",
		"[/ip hotspot active print count-only]",
		"[/ip pool find]",
		"[/ip hotspot find]",
		"[/ip hotspot get $se address-pool]",
		"[/ip hotspot get $se addresses-per-mac]",
		"[/ip dhcp-server find]",
		"[/ip dhcp-server get $de address-pool]",
		"[/ip dhcp-server get $de lease-time]",
		"&servers=",
		"&dhcp=",
		"/agent/result?token=",
	} {
		if !strings.Contains(script, needle) {
			t.Errorf("diagnostic : %q absent", needle)
		}
	}
	// N°108 — plus de liste « profiles » : le rapport ne porte que des
	// champs qui EXISTENT côté RouterOS.
	if strings.Contains(script, "&profiles=") {
		t.Error("la liste profiles (propriétés inexistantes sur le profil) ne doit plus être rapportée")
	}
}

// TestPoolDoctorNeverTouchesProfileMenu — GARDE DE RÉGRESSION N°108 :
// address-pool et addresses-per-mac sont des propriétés du SERVEUR
// (/ip hotspot). Le script ne doit JAMAIS les lire/écrire sur
// /ip hotspot profile — c'était le bug N°97 : chaque get échouait en
// silencieux (on-error), tout profil paraissait « sans pool » et le set
// de l'extension échouait à chaque fois (extension muette, constat
// production ProMax WIFI).
func TestPoolDoctorNeverTouchesProfileMenu(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tk"}
	for _, payload := range []map[string]any{
		{},
		{"recycle": true},
		{"recycle": true, "extend": true},
	} {
		script, err := b.ScriptFor(poolDoctorCmd(payload))
		if err != nil {
			t.Fatalf("ScriptFor impossible : %v", err)
		}
		if strings.Contains(script, "/ip hotspot profile set") {
			t.Errorf("payload %v : écriture sur /ip hotspot profile interdite (le profil n'a pas address-pool/addresses-per-mac)", payload)
		}
		if strings.Contains(script, "/ip hotspot profile get") {
			t.Errorf("payload %v : lecture de propriétés inexistantes sur /ip hotspot profile", payload)
		}
	}
}

// TestPoolDoctorRecycle — recyclage : trois écrits ISOLÉS sur les BONS
// menus (un RouterOS ancien qui ignore addresses-per-mac ne doit pas faire
// échouer les timeouts) — timeouts serveurs, addresses-per-mac serveurs,
// lease-time des DHCP des interfaces hotspot.
func TestPoolDoctorRecycle(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tk"}
	script, err := b.ScriptFor(poolDoctorCmd(map[string]any{"recycle": true}))
	if err != nil {
		t.Fatalf("ScriptFor impossible : %v", err)
	}
	if !strings.Contains(script, "/ip hotspot set [find] login-timeout=5m idle-timeout=10m keepalive-timeout=2m") {
		t.Error("timeouts de recyclage absents ou non conformes aux défauts")
	}
	if !strings.Contains(script, "/ip hotspot set [find] addresses-per-mac=1") {
		t.Error("addresses-per-mac=1 (serveur, menu correct) absent")
	}
	if !strings.Contains(script, "/ip dhcp-server set [find where interface=$sif] lease-time=10m") {
		t.Error("recyclage lease-time des DHCP d'interface hotspot absent")
	}
	// Le recyclage n'étend PAS le pool : pas de commande d'extension.
	if strings.Contains(script, "/ip pool set") || strings.Contains(script, "mikcloud-pool-nat") {
		t.Error("recyclage seul ne doit pas étendre le pool")
	}
}

// TestPoolDoctorExtend — extension : le range dédié rejoint le VRAI
// fournisseur d'adresses de chaque serveur — (a) son address-pool lu sur
// /ip hotspot, (b) sinon le pool du DHCP de la même interface, (c) sinon un
// pool dédié posé SUR LE SERVEUR. IP secondaire, entrées network hotspot +
// DHCP (gateway), règle NAT marquée, ménage du pool orphelin N°97 — tout
// idempotent par find.
func TestPoolDoctorExtend(t *testing.T) {
	b := Builder{BaseURL: "https://cloud.example", Token: "tk"}
	script, err := b.ScriptFor(poolDoctorCmd(map[string]any{"recycle": true, "extend": true}))
	if err != nil {
		t.Fatalf("ScriptFor impossible : %v", err)
	}
	for _, needle := range []string{
		// (a) pool du serveur lu sur le BON menu + extension du range.
		`[:tostr [/ip hotspot get $se address-pool]]`,
		`ranges=($pr . "," . "` + PoolDoctorExtRange + `")`,
		// (b) repli DHCP : pool du serveur DHCP de l'interface du serveur hotspot.
		`[/ip dhcp-server find where interface=$sif]`,
		`[:tostr [/ip dhcp-server get $de address-pool]]`,
		// (c) pool dédié posé SUR LE SERVEUR (jamais sur le profil).
		`/ip pool add name="` + PoolDoctorPoolName + `" ranges="` + PoolDoctorExtRange + `"`,
		`/ip hotspot set $se address-pool="` + PoolDoctorPoolName + `"`,
		// Passerelle + entrées réseau + NAT.
		`/ip address add address="` + PoolDoctorExtGateway + `"`,
		`/ip hotspot network add address="` + PoolDoctorExtSubnet + `" masquerade=yes comment="mikcloud-pool"`,
		`/ip dhcp-server network add address="` + PoolDoctorExtSubnet + `" gateway=10.77.0.1 comment="mikcloud-pool"`,
		`/ip firewall nat add chain=srcnat src-address="` + PoolDoctorExtSubnet + `" action=masquerade comment="mikcloud-pool-nat"`,
		// Ménage N°108 : le pool orphelin laissé par le N°97 (créé, jamais
		// référencé) est retiré s'il ne sert plus.
		`/ip pool remove $mkOrph`,
	} {
		if !strings.Contains(script, needle) {
			t.Errorf("extension : %q absent", needle)
		}
	}
	// Idempotence : les find-before-add de l'IP secondaire, du NAT et de
	// l'entrée dhcp network.
	if !strings.Contains(script, `[/ip address find where address="`+PoolDoctorExtGateway+`" and interface=$sif]`) {
		t.Error("garde idempotence IP secondaire absente")
	}
	if !strings.Contains(script, `[/ip firewall nat find where comment="mikcloud-pool-nat"]`) {
		t.Error("garde idempotence NAT absente")
	}
	if !strings.Contains(script, `[/ip dhcp-server network find where address="`+PoolDoctorExtSubnet+`"]`) {
		t.Error("garde idempotence entrée dhcp network absente")
	}
	// Rapport : drapeaux recycled/extended + hôtes + pools + serveurs + dhcp.
	for _, needle := range []string{`&recycled=".`, `&extended=".`, `&hosts=".`, `&pools=".`, `&servers=".`, `&dhcp=".`} {
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
		"leaseTimeout": "10m /ip hotspot profile set [find] address-pool=evil",
	}))
	if err != nil {
		t.Fatalf("ScriptFor impossible : %v", err)
	}
	if strings.Contains(script, "pwned") || strings.Contains(script, "identity set") || strings.Contains(script, "evil") {
		t.Error("charge hostile du payload retrouvée dans le script")
	}
	if !strings.Contains(script, PoolDoctorExtRange) {
		t.Error("valeur hostile doit retomber sur le range par défaut")
	}
	if !strings.Contains(script, "login-timeout=5m") {
		t.Error("timeout hostile doit retomber sur le défaut 5m")
	}
	if !strings.Contains(script, "lease-time=10m") {
		t.Error("lease hostile doit retomber sur le défaut 10m")
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
		"leaseTimeout":     "5m",
	}))
	if err != nil {
		t.Fatalf("ScriptFor impossible : %v", err)
	}
	if !strings.Contains(script, "login-timeout=2m idle-timeout=15m keepalive-timeout=3m") {
		t.Error("timeouts custom absents")
	}
	if !strings.Contains(script, "lease-time=5m") {
		t.Error("lease custom absent")
	}
	if !strings.Contains(script, "10.99.0.10-10.99.0.254") {
		t.Error("range custom absent")
	}
}
