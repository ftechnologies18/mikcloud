package api

import (
	"net/http"
	"testing"

	"mikcloud/hotspot-api/internal/model"
	"mikcloud/hotspot-api/internal/store"
)

// TestNotifViewPlatformAccount — N°153 : GET /api/notifications dit à la
// console si le porteur EST le compte principal de la plateforme. C'est la
// clef de la différenciation des consoles : le super-admin sur SON compte voit
// « vos identifiants portent le relais de tous les clients », un compte client
// (y compris avec le relais actif) voit la présentation client classique.
func TestNotifViewPlatformAccount(t *testing.T) {
	st, ts := newTestServerWithStore(t)

	// Compte client : ni plateforme, ni relais (principal sans identifiants).
	token, _, _ := registerAccount(t, ts, "gerant-n153", "")
	status, out := doJSON(t, ts, "GET", "/api/notifications", token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET notifications client : statut %d", status)
	}
	if v, _ := out["isPlatformAccount"].(bool); v {
		t.Fatal("compte client : isPlatformAccount doit être false")
	}
	if v, _ := out["emailPlatformRelay"].(bool); v {
		t.Fatal("principal sans identifiants : emailPlatformRelay doit être false")
	}

	// Compte principal (super-admin sur SON compte) : isPlatformAccount true,
	// et le relais reste éteint tant que ses identifiants ne sont pas posés.
	admin := adminTokenOf(t, ts)
	status, out = doJSON(t, ts, "GET", "/api/notifications", admin, nil)
	if status != http.StatusOK {
		t.Fatalf("GET notifications admin : statut %d", status)
	}
	if v, _ := out["isPlatformAccount"].(bool); !v {
		t.Fatal("compte principal : isPlatformAccount doit être true")
	}
	if v, _ := out["emailPlatformRelay"].(bool); v {
		t.Fatal("identifiants absents : emailPlatformRelay doit être false")
	}

	// Identifiants Resend posés au principal : le relais s'annonce au CLIENT
	// (présentation « envoyé par la plateforme ») sans jamais le promouvoir
	// compte plateforme ; le principal, lui, garde isPlatformAccount true.
	st.Lock()
	db := st.Data()
	store.SetNotifSettings(db, model.NotificationSettings{
		AccountID:     model.AccountMainID,
		EmailProvider: "resend",
		ResendAPIKey:  "re_main",
	})
	st.Save()
	st.Unlock()

	status, out = doJSON(t, ts, "GET", "/api/notifications", token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET notifications client (relais actif) : statut %d", status)
	}
	if v, _ := out["emailPlatformRelay"].(bool); !v {
		t.Fatal("relais actif : emailPlatformRelay doit être true côté client")
	}
	if v, _ := out["isPlatformAccount"].(bool); v {
		t.Fatal("client avec relais actif : isPlatformAccount doit rester false")
	}

	status, out = doJSON(t, ts, "GET", "/api/notifications", admin, nil)
	if status != http.StatusOK {
		t.Fatalf("GET notifications admin (relais actif) : statut %d", status)
	}
	if v, _ := out["isPlatformAccount"].(bool); !v {
		t.Fatal("principal avec relais actif : isPlatformAccount doit rester true")
	}
	if v, _ := out["emailPlatformRelay"].(bool); !v {
		t.Fatal("principal avec identifiants : emailPlatformRelay doit être true (le relais EST ses réglages)")
	}
}
