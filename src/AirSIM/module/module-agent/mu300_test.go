package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMU300ProfileIsolation(t *testing.T) {
	p := runtimeProfileFrom("mu300-openwrt")
	if !p.CallAudio || !p.RequiresModem || p.AndroidTelecom || p.WRTLite || p.NetworkPolicyNative {
		t.Fatalf("unsafe MU300 capabilities: %+v", p)
	}
	get := func(k string) string {
		return map[string]string{runtimeProfileEnvironment: "mu300-openwrt", "AIRSIM_VOICE_BACKEND": "mu300"}[k]
	}
	b, e := loadVoiceBackend(get)
	if e != nil || b.endpoint != "127.0.0.1:7580" || b.requiresLocalRuntime() {
		t.Fatalf("%+v %v", b, e)
	}
	_, e = loadVoiceBackend(func(k string) string {
		if k == "AIRSIM_VOICE_BACKEND" {
			return "mu300"
		}
		return "android-avf"
	})
	if e == nil {
		t.Fatal("MU300 accepted on Android profile")
	}
}
func TestMU300EndpointRejectsLAN(t *testing.T) {
	_, e := loadVoiceBackend(func(k string) string {
		return map[string]string{runtimeProfileEnvironment: "mu300-openwrt", "AIRSIM_VOICE_BACKEND": "mu300", "AIRSIM_MU300_PCM_ADDRESS": "192.168.78.1:7580"}[k]
	})
	if e == nil {
		t.Fatal("LAN raw PCM accepted")
	}
}
func TestMU300DialLogRedaction(t *testing.T) {
	if safeMU300Verb("ATD123456789;") != "ATD" {
		t.Fatal("dial number in log")
	}
}

func TestMU300NumberRefreshClearsStaleSIMMetadata(t *testing.T) {
	response := "+CNUM: \"\",\"15550001111\",145\nOK"
	var failure error
	a := testMU300Calls(t, func(command string, _ time.Duration) (string, error) {
		if command != "AT+CNUM" {
			t.Fatal("unexpected identity command", command)
		}
		return response, failure
	})
	if number, err := a.mu300RefreshNumber(); err != nil || number != "15550001111" || a.modem.PhoneNumberStatus != "available" {
		t.Fatal("SIM number not loaded", err)
	}
	response = "OK"
	if number, err := a.mu300RefreshNumber(); err != nil || number != "" || a.modem.PhoneNumber != "" || a.modem.PhoneNumberStatus != "not_provided" {
		t.Fatal("old SIM number retained")
	}
	failure = errors.New("SIM unavailable")
	if _, err := a.mu300RefreshNumber(); err == nil || a.modem.PhoneNumberStatus != "unavailable" || a.modem.PhoneNumberUpdatedAt == "" {
		t.Fatal("read failure not represented")
	}
}

func testMU300Calls(t *testing.T, command func(string, time.Duration) (string, error)) *agent {
	return &agent{mu300Operations: &mu300OperationStore{directory: t.TempDir()}, profile: runtimeProfileFrom("mu300-openwrt"), push: newPushManager(filepath.Join(t.TempDir(), "push.json")), voiceBackend: voiceBackendConfig{kind: voiceBackendMU300, endpoint: "127.0.0.1:7580"}, callControlCommand: command, callControlSleep: func(time.Duration) {}, mu300DialWindow: 50 * time.Millisecond}
}

func TestMU300DialReceiptAndIdempotency(t *testing.T) {
	state := "OK"
	dials := 0
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		if cmd == "AT+CLCC" {
			return state, nil
		}
		if cmd == "ATD12345;" {
			dials++
			state = `+CLCC: 1,0,2,0,0,"12345",129` + "\nOK"
			return "OK", nil
		}
		return "", errors.New("unexpected command")
	})
	first, err := a.mu300Dial("12345", "operation-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if first["operation_id"] != "operation-1" || first["android_id"] == "" || first["android_id"] != first["backend_id"] {
		t.Fatalf("invalid receipt: %v", first)
	}
	if a.calls.Active.AndroidID != first["android_id"] {
		t.Fatal("receipt and call events disagree")
	}
	second, err := a.mu300Dial("12345", "operation-1", "")
	if err != nil || second["backend_id"] != first["backend_id"] || dials != 1 {
		t.Fatal("replayed operation redialed")
	}
	if _, err = a.mu300Dial("54321", "operation-1", ""); err == nil {
		t.Fatal("operation reused for another number")
	}
	if _, err = a.mu300Dial("54321", "operation-2", ""); err == nil || dials != 1 {
		t.Fatal("second call admitted")
	}
}

func TestMU300IncomingAnswerAndStaleHangup(t *testing.T) {
	state := `+CLCC: 1,1,4,0,0,"12345",129` + "\nOK"
	answers, hangups := 0, 0
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		switch cmd {
		case "AT+CLCC":
			return state, nil
		case "ATA":
			answers++
			state = `+CLCC: 1,1,0,0,0,"12345",129` + "\nOK"
			return "OK", nil
		case "AT+CHUP":
			hangups++
			state = "OK"
			return "OK", nil
		}
		return "", errors.New("unexpected command")
	})
	if _, err := a.mu300RefreshCalls(); err != nil {
		t.Fatal(err)
	}
	oldID := a.calls.Active.ID
	if err := a.answerCallForID(oldID); err != nil || answers != 1 || a.calls.Active.State != "active" || a.calls.Active.AndroidID != oldID {
		t.Fatal("incoming call not answered with stable identity", err)
	}
	if err := a.hangupCallForID(oldID); err != nil || hangups != 1 || a.calls.Active != nil {
		t.Fatal("hangup failed", err)
	}
	// A different caller reuses the modem's numeric slot; identity must not.
	state = `+CLCC: 1,1,4,0,0,"54321",129` + "\nOK"
	if _, err := a.mu300RefreshCalls(); err != nil {
		t.Fatal(err)
	}
	if a.calls.Active.ID == oldID {
		t.Fatal("reused canonical call identity")
	}
	if err := a.hangupCallForID(oldID); err == nil || hangups != 1 {
		t.Fatal("stale hangup reached replacement call")
	}
	if err := a.answerCallForID(oldID); err == nil || answers != 1 {
		t.Fatal("stale answer reached replacement call")
	}
}

func TestMU300UncertainATDNeverRetries(t *testing.T) {
	dials := 0
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		if strings.HasPrefix(cmd, "ATD") {
			dials++
			return "", errors.New("timeout")
		}
		return "OK", nil
	})
	if _, err := a.mu300Dial("12345", "uncertain", ""); err == nil {
		t.Fatal("unconfirmed dial accepted")
	}
	if _, err := a.mu300Dial("12345", "uncertain", ""); err == nil || dials != 1 {
		t.Fatal("uncertain operation repeated ATD")
	}
}

func TestMU300DialConfirmsAfterSlowATChannel(t *testing.T) {
	failures, dialed := 3, false
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		switch {
		case strings.HasPrefix(cmd, "ATD"):
			dialed = true
			return "OK", nil
		case cmd == "AT+CLCC":
			if !dialed {
				return "OK", nil
			}
			if failures > 0 {
				failures--
				return "", errors.New("mu300-at: busy")
			}
			return "+CLCC: 1,0,3,0,0,\"\",129\nOK", nil
		}
		return "", errors.New("unexpected command")
	})
	receipt, err := a.mu300Dial("13800138000", "operation-slow", "")
	if err != nil {
		t.Fatalf("a busy AT channel turned a placed call into a failure: %v", err)
	}
	if receipt["android_id"] == "" || receipt["android_id"] != receipt["backend_id"] {
		t.Fatalf("invalid receipt: %v", receipt)
	}
}

func TestMU300DialAcceptsNetworkRewrittenNumber(t *testing.T) {
	dialed := false
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		switch {
		case strings.HasPrefix(cmd, "ATD13800138000;"):
			dialed = true
			return "OK", nil
		case cmd == "AT+CLCC":
			if dialed {
				return "+CLCC: 1,0,3,0,0,\"+8613800138000\",145\nOK", nil
			}
			return "OK", nil
		}
		return "", errors.New("unexpected command")
	})
	if _, err := a.mu300Dial("13800138000", "operation-rewrite", ""); err != nil {
		t.Fatalf("network-rewritten number rejected: %v", err)
	}
}

func TestMU300SetupGapKeepsCallIdentity(t *testing.T) {
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		if cmd == "AT+CLCC" {
			return "+CLCC: 1,0,2,0,0,\"13800138000\",129\nOK", nil
		}
		return "", errors.New("unexpected command")
	})
	if _, err := a.mu300RefreshCalls(); err != nil {
		t.Fatal(err)
	}
	id := a.calls.Active.ID
	base := time.Now()
	// The modem does not list an outgoing call for a few seconds between
	// dialing and alerting. The identity has to survive that gap.
	a.mu300ApplyCallPoll(nil, base.Add(time.Second), true)
	a.mu300ApplyCallPoll(nil, base.Add(5*time.Second), true)
	if a.calls.Active == nil || a.calls.Active.ID != id || len(a.calls.History) != 0 {
		t.Fatal("the setup gap ended the call")
	}
	a.mu300ApplyCallPoll([]parsedCall{{Index: 1, Direction: "outgoing", State: "alerting", Number: "13800138000"}}, base.Add(7*time.Second), true)
	if a.calls.Active == nil || a.calls.Active.ID != id || a.calls.Active.State != "alerting" {
		t.Fatal("the call came back with a new identity")
	}
	a.mu300ApplyCallPoll(nil, base.Add(8*time.Second), true)
	if a.calls.Active == nil || a.calls.Active.ID != id {
		t.Fatal("one empty snapshot ended an alerting call")
	}
	a.mu300ApplyCallPoll(nil, base.Add(19*time.Second), true)
	if a.calls.Active != nil || len(a.calls.History) != 1 || a.calls.History[0].ID != id {
		t.Fatal("a confirmed hangup was not reported")
	}
}
