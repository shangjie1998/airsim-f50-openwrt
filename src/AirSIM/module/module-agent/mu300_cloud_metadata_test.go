package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestMU300CloudNumberRefreshClearAndLegacyCompatibility(t *testing.T) {
	var received []map[string]json.RawMessage
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		received = append(received, body)
		w.WriteHeader(202)
	}))
	defer server.Close()
	m := newPushManager(filepath.Join(t.TempDir(), "push.json"))
	m.client = server.Client()
	reg := testPushRegistration(server.URL)
	reg.AndroidPhoneNumber = "old-fixture-number"
	if err := m.store(reg); err != nil {
		t.Fatal(err)
	}
	a := &agent{profile: runtimeProfile{Name: "mu300-openwrt"}}
	for _, tc := range []struct {
		profile, status, number string
		present                 bool
		want                    string
	}{
		{"mu300-openwrt", "available", "new-fixture-number", true, "new-fixture-number"},
		{"mu300-openwrt", "available", "replacement-fixture-number", true, "replacement-fixture-number"},
		{"mu300-openwrt", "not_provided", "", true, ""},
		{"mu300-openwrt", "unavailable", "stale-fixture-number", true, ""},
		{"mu300-openwrt", "available", "stale-fixture-number", false, ""},
		{"qdc507", "available", "unused", true, "old-fixture-number"},
	} {
		a.profile.Name = tc.profile
		a.modem.PhoneNumber = tc.number
		a.modem.PhoneNumberStatus = tc.status
		a.modem.SIMInserted = tc.present
		if err := m.sendHeartbeat(agentHeartbeatSnapshot{CellularState: "registered", PhoneNumber: a.mu300HeartbeatPhoneNumber()}); err != nil {
			t.Fatal(err)
		}
		body := received[len(received)-1]
		raw, ok := body["phone_number"]
		if !ok {
			t.Fatal("number clear omitted")
		}
		var number string
		json.Unmarshal(raw, &number)
		if number != tc.want {
			t.Fatalf("number = %q want %q", number, tc.want)
		}
	}
	if m.config.AndroidPhoneNumber != "old-fixture-number" {
		t.Fatal("poll changed pairing configuration")
	}
}
