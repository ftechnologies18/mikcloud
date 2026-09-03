// auth_audit_test.go — tests du journal des échecs d'authentification (S2).

package api

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestLogAuthFailureJSON — la sortie est une ligne JSON exploitable, avec
// l'IP prise au PREMIER hop XFF (contrat Render) et les champs attendus.
func TestLogAuthFailureJSON(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	r := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.1.2.3")
	r.Header.Set("User-Agent", "sonde-s2/1.0")
	a := &API{} // logAuthFailure n'utilise ni store ni secret
	a.logAuthFailure(r, "console", "gerant1", "bad_password")

	line := strings.TrimSpace(buf.String())
	if i := strings.Index(line, "{"); i > 0 {
		line = line[i:] // retire le préfixe date/heure de log.Printf
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("sortie non JSON : %q", buf.String())
	}
	want := map[string]string{
		"event":  "auth_failure",
		"ip":     "203.0.113.7",
		"kind":   "console",
		"login":  "gerant1",
		"reason": "bad_password",
		"ua":     "sonde-s2/1.0",
	}
	for field, expected := range want {
		if got, _ := entry[field].(string); got != expected {
			t.Fatalf("champ %q = %q, attendu %q", field, got, expected)
		}
	}
	if _, ok := entry["at"]; !ok {
		t.Fatal("champ « at » (horodatage RFC3339) attendu")
	}
}

// TestLogAuthFailureSansXFF — repli sur RemoteAddr quand XFF est absent
// (tests locaux, appels directs).
func TestLogAuthFailureSansXFF(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	r := httptest.NewRequest("POST", "/api/reseller/login", nil)
	r.RemoteAddr = "198.51.100.9:51234"
	a := &API{}
	a.logAuthFailure(r, "reseller_pin", "vendeur1", "bad_pin")

	line := strings.TrimSpace(buf.String())
	if i := strings.Index(line, "{"); i > 0 {
		line = line[i:]
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("sortie non JSON : %q", buf.String())
	}
	if got, _ := entry["ip"].(string); got != "198.51.100.9" {
		t.Fatalf("ip = %q, attendu 198.51.100.9 (RemoteAddr sans port)", got)
	}
}
