package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMU300CloudSMSSelectionAndReplay(t *testing.T) {
	for _, tc := range []struct {
		account, cpin     string
		queryErr, success bool
	}{
		{"", "", false, true}, {"0", "+CPIN: READY\r\nOK", false, true},
		{"other", "+CPIN: READY\nOK", false, false}, {"SIM 1", "+CPIN: READY\nOK", false, false},
		{"0", "+CPIN: SIM PIN\nOK", false, false}, {"0", "+CME ERROR: SIM not inserted", false, false},
		{"0", "+CPIN: NOT READY\nOK", false, false}, {"0", "", true, false},
	} {
		t.Run(tc.account+tc.cpin, func(t *testing.T) {
			a := smsFixture(t)
			submits := 0
			a.callControlCommand = func(cmd string, _ time.Duration) (string, error) {
				switch cmd {
				case "AT+CLCC":
					return "OK", nil
				case "AT+CPIN?":
					if tc.queryErr {
						return "", errors.New("fixture timeout")
					}
					return tc.cpin, nil
				}
				t.Fatalf("unexpected AT %s", cmd)
				return "", nil
			}
			a.mu300SMS.runSend = func(context.Context, string) (string, error) { submits++; return "sent (reference 1)", nil }
			body := `{"command_id":"891eae43-89f7-4baa-a21e-e15beecbe584","type":"send_sms","number":"15550000001","message":"fixture","expires_at":"` + time.Now().Add(time.Minute).UTC().Format(time.RFC3339) + `"`
			if tc.account != "" {
				body += `,"sim_account":"` + tc.account + `"`
			}
			body += "}"
			var command cloudDeviceCommand
			if err := json.Unmarshal([]byte(body), &command); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				result := a.executeCloudCommand(command)
				if (result.Status == "completed") != tc.success {
					t.Fatalf("unexpected status %s %s", result.Status, result.Error)
				}
			}
			want := 0
			if tc.success {
				want = 1
			}
			if submits != want {
				t.Fatalf("submits=%d want=%d", submits, want)
			}
		})
	}
}

func TestMU300SMSNativeSlotMetadata(t *testing.T) {
	a := smsFixture(t)
	writeSMSFixture(t, a, "000001", "mt", "fixture")
	a.mu300RefreshSMS()
	if len(a.messages) != 1 || a.messages[0].Message.SIMSlot != "SIM 1" {
		t.Fatal("native single-slot metadata missing")
	}
	data, _ := json.Marshal(a.messages[0].Message)
	if !strings.Contains(string(data), `"sim_slot":"SIM 1"`) {
		t.Fatal("wire slot missing")
	}
}

func TestMU300SMSPushPreservesSlot(t *testing.T) {
	var received incomingSMSPushEvent
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events/sms" {
			t.Error("wrong endpoint")
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		w.WriteHeader(202)
	}))
	defer server.Close()
	manager := newPushManager(filepath.Join(t.TempDir(), "push.json"))
	manager.client = server.Client()
	if err := manager.store(testPushRegistration(server.URL)); err != nil {
		t.Fatal(err)
	}
	sent, err := manager.sendSMS(smsMessage{SIMSlot: "SIM 1", Sender: "fixture", Content: "fixture", DeliveryID: "fixture-slot", Timestamp: time.Now()})
	if err != nil || !sent || received.SIMSlot != "SIM 1" {
		t.Fatalf("slot delivery: %v %v %q", sent, err, received.SIMSlot)
	}
}
