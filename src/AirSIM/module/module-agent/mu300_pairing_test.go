package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestMU300PairingAcceptsAndPreservesLiveActivityPresentation(t *testing.T) {
	t.Setenv("AIRSIM_RUNTIME_PROFILE", "mu300-openwrt")
	t.Setenv("AIRSIM_MU300_MODE", "cloud")
	t.Setenv("AIRSIM_MU300_RELAY", "")
	// Current iOS sends this field only for cloud registrations. Local mode
	// bypassed the strict decoder failure, making a valid code appear wrong.
	for _, local := range []bool{false, true} {
		p := newPushManager(filepath.Join(t.TempDir(), "push.json"))
		reg := testPushRegistration("https://relay.example.test")
		encoded, _ := json.Marshal(reg)
		var body map[string]any
		json.Unmarshal(encoded, &body)
		body["cloud_enabled"] = false
		if local {
			body = map[string]any{"local_mode": true, "cloud_enabled": false}
		} else {
			body["live_activity_presentation"] = "agent_status"
		}
		encoded, _ = json.Marshal(body)
		w := httptest.NewRecorder()
		var registration pushRegistration
		if !decodeJSON(w, httptest.NewRequest("POST", "/api/push/register", strings.NewReader(string(encoded))), &registration) {
			t.Fatalf("valid pairing rejected before persistence: %s", w.Body.String())
		}
		if err := p.store(registration); err != nil {
			t.Fatal(err)
		}
		p = newPushManager(p.configPath)
		if p.status().CloudEnabled {
			t.Fatal("pairing enabled cloud without consent")
		}
		if local {
			continue
		}
		// Relay still receives the presentation contract once cloud is enabled;
		// dropping the new field at the gateway would lose this behavior.
		var received map[string]any
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/devices/register" {
				t.Error("unexpected relay endpoint")
			}
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Error(err)
			}
			io.WriteString(w, `{}`)
		}))
		p.config.RelayURL = server.URL
		p.client = server.Client()
		if err := p.setCloudEnabled(true); err != nil {
			t.Fatal(err)
		}
		if err := p.sendRegistration(); err != nil {
			t.Fatal(err)
		}
		server.Close()
		if received["live_activity_presentation"] != "agent_status" {
			t.Fatal("pairing lost the client's Live Activity presentation")
		}
	}
}

func TestMU300PairingStillRejectsUnrecognizedFields(t *testing.T) {
	w := httptest.NewRecorder()
	var reg pushRegistration
	if decodeJSON(w, httptest.NewRequest("POST", "/api/push/register", strings.NewReader(`{"local_mode":true,"unexpected_privilege":true}`)), &reg) {
		t.Fatal("strict registration validation was disabled")
	}
}
