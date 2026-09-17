package model_test

import (
	"reflect"
	"testing"
	"time"

	"mikcloud/hotspot-api/internal/model"
)

// TestDBCloneDeepIsolation — N°130 : le syncreur de fond photographie l'état
// (CloneDeep sous verrou) puis synchronise PostgreSQL HORS verrou pendant que
// les requêtes continuent de MUTER l'état vivant. Le clone doit donc être
// totalement détaché : muter l'original ne doit JAMAIS traverser le snapshot
// (et réciproquement). Couvre les quatre familles porteuses de références
// (Command.Payload/Result, RouterTraffic.Interfaces/History, Settings et ses
// pointeurs, NotificationSettings et ses maps) plus la recopie des tranches
// par valeur.
func TestDBCloneDeepIsolation(t *testing.T) {
	join := false
	retention := 90
	autoImport := false
	db := &model.DB{
		Accounts: []model.Account{{ID: "acc-1", Name: "Cyber Abidjan"}},
		Routers:  []model.Router{{ID: "rt-1", Mode: "agent", UptimeSec: 100}},
		Commands: []model.Command{{
			ID:      "cmd-1",
			Payload: map[string]any{"script": "printf hello", "reboot": true},
			Result:  map[string]any{"ok": true},
		}},
		Traffic: []model.RouterTraffic{{
			RouterID:   "rt-1",
			Interfaces: []model.IfaceTraffic{{Name: "ether1", RxBytes: 10, TxBytes: 20}},
			History: []model.TrafficPoint{
				{T: "2024-01-01T00:00:00Z", RxBps: 1, TxBps: 2},
				{T: "2024-01-01T00:00:05Z", RxBps: 3, TxBps: 4},
			},
		}},
		Sessions: []model.Session{{ID: "se-1", Username: "alice", BytesIn: 1}},
		SettingsByAccount: map[string]model.Settings{
			"acc-1": {
				Tenant: model.Tenant{
					Name:             "Cyber",
					JoinButton:       &join,
					LogRetentionDays: &retention,
				},
				Platform:              &model.PlatformConfig{Name: "MikCloud", RegisterOpen: true},
				AutoImportRouterUsers: &autoImport,
			},
		},
		NotifSettings: map[string]model.NotificationSettings{
			"acc-1": {
				AccountID:       "acc-1",
				StockAlertState: map[string]string{"rt-1": "2024-01-01T00:00:00Z"},
				PoolAlertState:  map[string]string{"rt-1": "sent"},
			},
		},
		LastTick: time.Now().UTC().Truncate(time.Second),
	}

	snap := db.CloneDeep()

	// ── L'original mute pendant que la synchro lit le snapshot ──────────
	db.Accounts[0].Name = "MUTÉ"
	db.Routers[0].UptimeSec = 999
	db.Commands[0].Payload["script"] = "MUTÉ"
	db.Commands[0].Result["ok"] = false
	db.Traffic[0].Interfaces[0] = model.IfaceTraffic{Name: "MUTÉ"}
	db.Traffic[0].History[0] = model.TrafficPoint{T: "MUTÉ"}
	db.Sessions = append(db.Sessions, model.Session{ID: "se-2"})
	*db.SettingsByAccount["acc-1"].Tenant.JoinButton = true
	*db.SettingsByAccount["acc-1"].Tenant.LogRetentionDays = 30
	db.SettingsByAccount["acc-1"].Platform.Name = "MUTÉ"
	*db.SettingsByAccount["acc-1"].AutoImportRouterUsers = true
	db.NotifSettings["acc-1"].StockAlertState["rt-1"] = "MUTÉ"
	db.NotifSettings["acc-1"].PoolAlertState["rt-1"] = "MUTÉ"
	db.LastTick = time.Time{}

	// Le snapshot est resté FIDÈLE à l'instant de la photographie.
	if got := snap.Accounts[0].Name; got != "Cyber Abidjan" {
		t.Errorf("Accounts : le snapshot a suivi la mutation (%q)", got)
	}
	if got := snap.Routers[0].UptimeSec; got != 100 {
		t.Errorf("Routers : le snapshot a suivi la mutation (%d)", got)
	}
	if got := snap.Commands[0].Payload["script"]; got != "printf hello" {
		t.Errorf("Command.Payload : le snapshot a suivi la mutation (%v)", got)
	}
	if got := snap.Commands[0].Result["ok"]; got != true {
		t.Errorf("Command.Result : le snapshot a suivi la mutation (%v)", got)
	}
	if got := snap.Traffic[0].Interfaces[0].Name; got != "ether1" {
		t.Errorf("Traffic.Interfaces : le snapshot a suivi la mutation (%q)", got)
	}
	if got := snap.Traffic[0].History[0].T; got != "2024-01-01T00:00:00Z" {
		t.Errorf("Traffic.History : le snapshot a suivi la mutation (%q)", got)
	}
	if len(snap.Sessions) != 1 || snap.Sessions[0].ID != "se-1" {
		t.Errorf("Sessions : le snapshot a suivi la mutation (%+v)", snap.Sessions)
	}
	st := snap.SettingsByAccount["acc-1"]
	if *st.Tenant.JoinButton {
		t.Error("Settings.JoinButton : le pointeur du snapshot est partagé avec l'original")
	}
	if *st.Tenant.LogRetentionDays != 90 {
		t.Error("Settings.LogRetentionDays : le pointeur du snapshot est partagé")
	}
	if st.Platform.Name != "MikCloud" {
		t.Errorf("Settings.Platform : le pointeur du snapshot est partagé (%q)", st.Platform.Name)
	}
	if *st.AutoImportRouterUsers {
		t.Error("Settings.AutoImportRouterUsers : le pointeur du snapshot est partagé")
	}
	if st.Tenant.Name != "Cyber" {
		t.Errorf("Settings : la valeur du snapshot a suivi la mutation (%q)", st.Tenant.Name)
	}
	ns := snap.NotifSettings["acc-1"]
	if ns.StockAlertState["rt-1"] != "2024-01-01T00:00:00Z" {
		t.Errorf("NotifSettings.StockAlertState : map partagée (%q)", ns.StockAlertState["rt-1"])
	}
	if ns.PoolAlertState["rt-1"] != "sent" {
		t.Errorf("NotifSettings.PoolAlertState : map partagée (%q)", ns.PoolAlertState["rt-1"])
	}
	if snap.LastTick.IsZero() {
		t.Error("LastTick : le snapshot a suivi la mutation")
	}

	// ── Réciproque : muter le snapshot ne touche pas l'original ─────────
	snap.Commands[0].Payload["script"] = "SNAP-MUTÉ"
	snap.Traffic[0].History[1] = model.TrafficPoint{T: "SNAP"}
	if got := db.Commands[0].Payload["script"]; got != "MUTÉ" {
		t.Errorf("Command.Payload : la mutation du snapshot a traversé l'original (%v)", got)
	}
	if got := db.Traffic[0].History[1].T; got != "2024-01-01T00:00:05Z" {
		t.Errorf("Traffic.History : la mutation du snapshot a traversé l'original (%q)", got)
	}
}

// TestDBCloneDeepEquality — le clone d'un état non muté doit être
// structurellement identique à l'original (aucune perte de champ) : la
// synchro différentielle hash les lignes du snapshot — une ligne perdue
// serait une SUPPRESSION fantôme en base.
func TestDBCloneDeepEquality(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	db := &model.DB{
		Accounts:             []model.Account{{ID: "acc-1"}},
		SettingsByAccount:    map[string]model.Settings{"acc-1": {Tenant: model.Tenant{Name: "T"}}},
		Users:                []model.AdminUser{{ID: "u-1"}},
		Routers:              []model.Router{{ID: "rt-1"}},
		Profiles:             []model.Profile{{ID: "pr-1"}},
		HotspotUsers:         []model.HotspotUser{{ID: "hu-1"}},
		Batches:              []model.Batch{{ID: "ba-1"}},
		Resellers:            []model.Reseller{{ID: "re-1"}},
		Transactions:         []model.Transaction{{ID: "tx-1"}},
		Sessions:             []model.Session{{ID: "se-1"}},
		Activity:             []model.Activity{{ID: "ac-1"}},
		Sales:                []model.Sale{{ID: "sa-1"}},
		Commands:             []model.Command{{ID: "cmd-1", Payload: map[string]any{"k": "v"}}},
		Templates:            []model.VoucherTemplate{{ID: "vt-1"}},
		UserLogs:             []model.UserLog{{ID: "ul-1"}},
		IPBindings:           []model.IPBinding{{ID: "ib-1"}},
		SchedulerTasks:       []model.SchedulerTask{{ID: "st-1"}},
		Traffic:              []model.RouterTraffic{{RouterID: "rt-1", Interfaces: []model.IfaceTraffic{{Name: "ether1"}}}},
		LineQuality:          []model.LineQualityDay{{RouterID: "rt-1"}},
		NotifSettings:        map[string]model.NotificationSettings{"acc-1": {AccountID: "acc-1"}},
		NotifLog:             []model.NotificationLog{{ID: "nl-1"}},
		BillingRequests:      []model.BillingRequest{{ID: "br-1"}},
		PurgeTombstones:      []model.PurgeTombstone{{ID: "pt-1"}},
		JoinLinks:            []model.JoinLink{{ID: "jl-1"}},
		RegistrationRequests: []model.RegistrationRequest{{ID: "rr-1"}},
		WifiSites:            []model.WifiSite{{ID: "ws-1"}},
		WifiGuests:           []model.WifiGuest{{ID: "wg-1"}},
		PromoEvents:          []model.PromoEvent{{ID: "pe-1"}},
		GeniusPaySubs:        []model.GeniusPaySub{{UUID: "gp-1"}},
		SellSessions:         []model.SellSession{{ID: "ss-1"}},
		Devices:              []model.Device{{ID: "dv-1"}},
		PasswordResets:       []model.PasswordReset{{ID: "pw-1"}},
		ChatConversations:    []model.ChatConversation{{ID: "cc-1"}},
		ChatMessages:         []model.ChatMessage{{ID: "cm-1"}},
		LastTick:             now,
		LastSweep:            now,
	}
	snap := db.CloneDeep()
	if !reflect.DeepEqual(db, snap) {
		t.Fatal("le clone doit être structurellement identique à l'original (DeepEqual)")
	}
}
