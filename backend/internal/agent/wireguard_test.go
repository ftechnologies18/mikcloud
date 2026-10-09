// Tests N°285 — renfort WireGuard : validations + forme des scripts .rsc.
// La forme est CONTRACTUELLE (le routeur exécute ce qui est généré) : chaque
// assertion est un incident évité (injection, guillemets, valeurs non
// échappées, rapport au mauvais endpoint).
package agent

import (
	"strings"
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

const wgTestKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

func TestValidWgKey(t *testing.T) {
	good := []string{
		wgTestKey,
		"6Ke9fJ1aGVwTx1sJlP5YhUw0t6B7R3QaQZ8mOa9V2Xk=",
		"abcdefghijklmnopqrstuvwxyz01234567890123456=",
	}
	for _, s := range good {
		if !ValidWgKey(s) {
			t.Errorf("ValidWgKey(%q) = false, attendu true", s)
		}
	}
	bad := []string{
		"",
		"court=",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",   // 44 sans '='
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==", // 45
		"AAAA!AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=A", // 45
	}
	for _, s := range bad {
		if ValidWgKey(s) {
			t.Errorf("ValidWgKey(%q) = true, attendu false", s)
		}
	}
}

func TestValidWgIPv4(t *testing.T) {
	for _, s := range []string{"10.8.0.2", "10.8.0.254", "10.8.0.100"} {
		if !ValidWgIPv4(s) {
			t.Errorf("ValidWgIPv4(%q) = false, attendu true", s)
		}
	}
	for _, s := range []string{
		"", "10.8.0.1", "10.8.0.255", "10.8.0.0", "10.8.0.999",
		"10.8.1.5", "10.8.0.2/32", "192.168.1.5", "10.8.0.abc", "10.8.0.",
	} {
		if ValidWgIPv4(s) {
			t.Errorf("ValidWgIPv4(%q) = true, attendu false", s)
		}
	}
}

func TestValidWgPeerName(t *testing.T) {
	for _, s := range []string{"a", "cyber-espace-1", "r-0123456789ab", "x2345678901234567890123456789012"} {
		if !ValidWgPeerName(s) {
			t.Errorf("ValidWgPeerName(%q) = false, attendu true", s)
		}
	}
	for _, s := range []string{"", "-abc", "Achat", "up_per", "32chars3256748392012345678901234567890", "péage"} {
		if ValidWgPeerName(s) {
			t.Errorf("ValidWgPeerName(%q) = true, attendu false", s)
		}
	}
}

func TestValidWgHost(t *testing.T) {
	for _, s := range []string{"203.0.113.10", "api.mikcloud.ftci.fr", "a.b"} {
		if !ValidWgHost(s) {
			t.Errorf("ValidWgHost(%q) = false, attendu true", s)
		}
	}
	for _, s := range []string{"", "deux mots", "injection\"; do bad", "host\nname", strings.Repeat("a", 254)} {
		if ValidWgHost(s) {
			t.Errorf("ValidWgHost(%q) = true, attendu false", s)
		}
	}
}

func TestWgPeerNameSuggestion(t *testing.T) {
	cases := []struct {
		name, id, want string
	}{
		{"Cyber Espace", "r-abcd1234", "cyber-espace"},
		{"ProMax_2", "r-xyz", "promax-2"},
		{"**", "r-9876fedc5432", "r-9876fedc5432"}, // repli : suffixe d'ID
		{"Château d'eau", "r-1", "ch-teau-d-eau"},  // accents → tirets (SanitizeName)
	}
	for _, c := range cases {
		got := WgPeerNameSuggestion(c.name, c.id)
		if got != c.want {
			t.Errorf("WgPeerNameSuggestion(%q, %q) = %q, attendu %q", c.name, c.id, got, c.want)
		}
		if !ValidWgPeerName(got) {
			t.Errorf("suggestion %q invalide pour wg-peer.sh", got)
		}
	}
}

func TestScriptForWgKinds(t *testing.T) {
	b := Builder{BaseURL: "https://api.mikcloud.ftci.fr", Token: "tok-token"}
	cmds := []string{model.CmdWgKeygen, model.CmdWgSetup, model.CmdWgTeardown}
	for _, kind := range cmds {
		cmd := model.Command{ID: "cmd-ab1", Kind: kind, Payload: map[string]any{
			"peerPub": wgTestKey, "psk": wgTestKey, "address": "10.8.0.7/32",
			"endpointHost": "203.0.113.10", "endpointPort": 51820,
		}}
		s, err := b.ScriptFor(cmd)
		if err != nil {
			t.Fatalf("ScriptFor(%s) : %v", kind, err)
		}
		if !strings.HasPrefix(s, "# mikcloud cmd cmd-ab1 "+kind) {
			t.Errorf("header manquant pour %s", kind)
		}
		if !strings.Contains(s, "/agent/result?token=tok-token") {
			t.Errorf("rapport result manquant pour %s", kind)
		}
	}
}

func TestBuildWgKeygenShape(t *testing.T) {
	b := Builder{BaseURL: "https://api.mikcloud.ftci.fr", Token: "tok"}
	s := b.buildWgKeygen(model.Command{ID: "c1", Kind: model.CmdWgKeygen})
	for _, want := range []string{
		`/interface/wireguard find name="mikcloud-wg"`,
		"/interface/wireguard add name=mikcloud-wg listen-port=13231",
		`comment="mikcloud-wg"`,
		"public-key",
		"&status=ok&wgpub=",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("buildWgKeygen : %q absent", want)
		}
	}
}

func TestBuildWgSetupShape(t *testing.T) {
	b := Builder{BaseURL: "https://api.mikcloud.ftci.fr", Token: "tok"}
	cmd := model.Command{ID: "c2", Kind: model.CmdWgSetup, Payload: WgSetupPayloadFrom(
		wgTestKey, wgTestKey, "10.8.0.9/32", "203.0.113.10", 51820)}
	s := b.buildWgSetup(cmd)
	for _, want := range []string{
		`public-key="` + wgTestKey + `"`,
		`preshared-key="` + wgTestKey + `"`,
		`endpoint-address="203.0.113.10"`,
		"endpoint-port=51820",
		"allowed-address=10.8.0.1/32",
		"persistent-keepalive=25",
		`address="10.8.0.9/32"`,
		`comment="mikcloud-wg"`,
		"&status=ok&peers=",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("buildWgSetup : %q absent", want)
		}
	}
	// Anti-injection : un guillemet dans un payload ne doit pas survivre
	// (rosEscape transforme « " » en « \" » — jamais de rupture de citation).
	cmd2 := model.Command{ID: "c3", Kind: model.CmdWgSetup, Payload: WgSetupPayloadFrom(
		wgTestKey, wgTestKey, "10.8.0.9/32", `evil"$(bad)`, 51820)}
	s2 := b.buildWgSetup(cmd2)
	if strings.Contains(s2, `endpoint-address="evil"`) {
		t.Error("buildWgSetup : injection possible via endpointHost non échappé")
	}
}

func TestBuildWgTeardownShape(t *testing.T) {
	b := Builder{BaseURL: "https://api.mikcloud.ftci.fr", Token: "tok"}
	s := b.buildWgTeardown(model.Command{ID: "c4", Kind: model.CmdWgTeardown})
	for _, want := range []string{
		`/interface/wireguard peers remove [find interface=mikcloud-wg`,
		`/ip address remove [find interface=mikcloud-wg`,
		`/interface/wireguard remove [find name="mikcloud-wg"]`,
		"&removed=1",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("buildWgTeardown : %q absent", want)
		}
	}
}

func TestWgIPv4WithPrefix(t *testing.T) {
	if got := WgIPv4WithPrefix("10.8.0.9"); got != "10.8.0.9/32" {
		t.Errorf("WgIPv4WithPrefix simple = %q", got)
	}
	if got := WgIPv4WithPrefix("10.8.0.9/32"); got != "10.8.0.9/32" {
		t.Errorf("WgIPv4WithPrefix déjà préfixé = %q", got)
	}
	if got := WgIPv4WithPrefix(""); got != "" {
		t.Errorf("WgIPv4WithPrefix vide = %q", got)
	}
}
