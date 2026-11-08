package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func smsFixture(t *testing.T) *agent {
	t.Helper()
	dir := t.TempDir()
	b := newMU300SMSBackend(dir)
	b.pool = filepath.Join(dir, "pool")
	if err := os.Mkdir(b.pool, 0700); err != nil {
		t.Fatal(err)
	}
	a := &agent{profile: runtimeProfileFrom("mu300-openwrt"), mu300SMS: b, eventChanged: make(chan struct{}), callChanged: make(chan struct{})}
	a.callControlCommand = func(command string, _ time.Duration) (string, error) {
		if command == "AT+CPIN?" {
			return "+CPIN: READY\r\nOK\r\n", nil
		}
		if command != "AT+CLCC" {
			t.Errorf("unexpected modem write: %s", command)
		}
		return "OK\r\n", nil
	}
	b.runSend = func(context.Context, string) (string, error) {
		t.Error("unexpected SMS submit")
		return "", errors.New("blocked")
	}
	return a
}

func writeSMSFixture(t *testing.T, a *agent, id, direction, content string) string {
	t.Helper()
	path := filepath.Join(a.mu300SMS.pool, id)
	text := fmt.Sprintf("id: %s\ndir: %s\nstatus: unread\nfrom: TEST-SENDER\nscts: 2026-01-01 08:00:00\nreceived: 1767225600\nlength: %d\n\n%s\n", id, direction, len(content), content)
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMU300SMSInboxAckPersistsWithoutNativeMutation(t *testing.T) {
	a := smsFixture(t)
	path := writeSMSFixture(t, a, "000001", "mt", "fixture 123456\nsecond line")
	writeSMSFixture(t, a, "000002", "mo", "outgoing fixture")
	before, _ := os.ReadFile(path)
	a.mu300RefreshSMS()
	if a.smsError != "" || len(a.messages) != 1 || a.messages[0].Message.Code != "123456" {
		t.Fatal("inbox mapping failed")
	}
	id := a.messages[0].Message.DeliveryID
	if a.messages[0].Message.Timestamp.Format(time.RFC3339) != "2026-01-01T00:00:00Z" {
		t.Fatal("receipt epoch misinterpreted")
	}
	n, err := a.mu300AckSMS([]string{id, "unknown"})
	if err != nil || n != 1 || len(a.messages) != 0 {
		t.Fatalf("ack: %d %v", n, err)
	}
	a.mu300RefreshSMS()
	if len(a.messages) != 0 {
		t.Fatal("acknowledged SMS redelivered")
	}
	restarted := smsFixture(t)
	restarted.mu300SMS.pool = a.mu300SMS.pool
	restarted.mu300SMS.state = a.mu300SMS.state
	restarted.mu300RefreshSMS()
	if restarted.smsError != "" || len(restarted.messages) != 0 {
		t.Fatal("ack did not survive restart")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("native message changed")
	}
	info, _ := os.Stat(filepath.Join(a.mu300SMS.state, "acks.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("ack state not private")
	}
	// Reuse of an index must not acknowledge a different message.
	writeSMSFixture(t, a, "000001", "mt", "replacement fixture")
	a.mu300RefreshSMS()
	if len(a.messages) != 1 || a.messages[0].Message.DeliveryID == id {
		t.Fatal("index reuse lost new message")
	}
}

func TestMU300SMSPoolRejectsIncompleteAndLinkedFiles(t *testing.T) {
	for _, kind := range []string{"truncated", "symlink", "invalid-utf8"} {
		t.Run(kind, func(t *testing.T) {
			a := smsFixture(t)
			path := writeSMSFixture(t, a, "000001", "mt", "fixture")
			switch kind {
			case "truncated":
				os.WriteFile(path, []byte("id: 000001\ndir: mt\n\npartial"), 0600)
			case "symlink":
				os.Remove(path)
				os.Symlink("/dev/null", path)
			case "invalid-utf8":
				os.WriteFile(path, []byte{255}, 0600)
			}
			a.mu300RefreshSMS()
			if a.smsError == "" || len(a.messages) != 0 {
				t.Fatal("invalid pool accepted")
			}
		})
	}
}

func TestMU300SMSAckFailureRetainsDelivery(t *testing.T) {
	a := smsFixture(t)
	writeSMSFixture(t, a, "000001", "mt", "fixture")
	a.mu300RefreshSMS()
	id := a.messages[0].Message.DeliveryID
	if err := os.MkdirAll(a.mu300SMS.state, 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(a.mu300SMS.state, "acks.json"), []byte("broken"), 0600)
	if _, err := a.mu300AckSMS([]string{id}); err == nil {
		t.Fatal("corrupt state overwritten")
	}
	if len(a.messages) != 1 {
		t.Fatal("unpersisted ack removed message")
	}
	a.mu300RefreshSMS()
	if a.smsError == "" {
		t.Fatal("corrupt state hidden")
	}
}

func TestMU300SMSBatchDoesNotLoseOlderMessages(t *testing.T) {
	a := smsFixture(t)
	for i := 1; i <= 270; i++ {
		writeSMSFixture(t, a, fmt.Sprintf("%06d", i), "mt", "fixture")
	}
	a.mu300RefreshSMS()
	if len(a.messages) != 256 {
		t.Fatal("batch not bounded")
	}
	ids := []string{}
	for _, item := range a.messages {
		ids = append(ids, item.Message.DeliveryID)
	}
	if n, err := a.mu300AckSMS(ids); err != nil || n != 256 {
		t.Fatal("batch ack failed")
	}
	a.mu300RefreshSMS()
	if len(a.messages) != 14 {
		t.Fatal("older messages lost")
	}
}

func TestMU300SMSSendValidationAndReceipt(t *testing.T) {
	a := smsFixture(t)
	submits := 0
	a.mu300SMS.runSend = func(_ context.Context, input string) (string, error) {
		submits++
		if input != "+15550000001\nfixture" {
			t.Fatal("number/text not on stdin")
		}
		if !a.mu300CallMu.TryLock() {
			t.Fatal("SMS blocks call status polling")
		}
		a.mu300CallMu.Unlock()
		return "sent (reference 7)\n", nil
	}
	for _, tc := range [][2]string{{"1; reboot", "fixture"}, {"+15550000001", strings.Repeat("x", 71)}, {"+15550000001", "\U0001F600"}, {"+15550000001", "bad\x00text"}, {"+15550000001", ""}} {
		if _, err := a.sendSMSMessage(tc[0], tc[1]); err == nil {
			t.Fatal("invalid payload accepted")
		}
	}
	if submits != 0 {
		t.Fatal("invalid SMS submitted")
	}
	if n, err := a.sendSMSMessage("+15550000001", "fixture"); err != nil || n != 1 {
		t.Fatalf("send %d %v", n, err)
	}
	if n, err := a.sendSMSMessage("+15550000001", "fixture"); err != nil || n != 1 || submits != 1 {
		t.Fatal("duplicate SMS submitted")
	}
}

func TestMU300SMSUnknownOutcomeNeverAutomaticallyResubmits(t *testing.T) {
	a := smsFixture(t)
	submits := 0
	a.mu300SMS.runSend = func(context.Context, string) (string, error) {
		submits++
		return "private fixture leaked", errors.New("private native error")
	}
	_, err := a.sendSMSMessage("15550000001", "fixture")
	if err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("unknown result not sanitized")
	}
	for _, text := range []string{"fixture", "different fixture"} {
		if _, err := a.sendSMSMessage("15550000001", text); err == nil {
			t.Fatal("unknown send state ignored")
		}
	}
	if submits != 1 {
		t.Fatal("SMS retried")
	}
	restarted := smsFixture(t)
	restarted.mu300SMS.state = a.mu300SMS.state
	if _, err := restarted.sendSMSMessage("15550000001", "fixture"); err == nil {
		t.Fatal("restart lost submission fence")
	}
}

func TestMU300SMSBusyAndConcurrentSubmission(t *testing.T) {
	a := smsFixture(t)
	a.callControlCommand = func(string, time.Duration) (string, error) { return "", errors.New("unknown radio state") }
	if _, err := a.sendSMSMessage("15550000001", "fixture"); err == nil {
		t.Fatal("unknown call state accepted")
	}
	a.callControlCommand = func(string, time.Duration) (string, error) { return "OK\n", nil }
	started, release := make(chan struct{}), make(chan struct{})
	a.mu300SMS.runSend = func(context.Context, string) (string, error) {
		close(started)
		<-release
		return "sent (reference 1)", nil
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := a.sendSMSMessage("15550000001", "fixture"); err != nil {
			t.Error(err)
		}
	}()
	<-started
	if _, err := a.sendSMSMessage("15550000002", "other"); err == nil {
		t.Fatal("parallel send accepted")
	}
	close(release)
	wg.Wait()
}

func TestMU300SMSHTTPMatchesClientContractAndPreservesPool(t *testing.T) {
	a := smsFixture(t)
	writeSMSFixture(t, a, "000001", "mt", "fixture")
	a.mu300RefreshSMS()
	w := httptest.NewRecorder()
	a.smsList(w, httptest.NewRequest("GET", "/api/sms", nil))
	var items []smsMessage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &items) != nil || len(items) != 1 {
		t.Fatal("client inbox schema mismatch")
	}
	body, _ := json.Marshal(map[string]any{"ids": []string{items[0].DeliveryID}})
	w = httptest.NewRecorder()
	a.smsAck(w, httptest.NewRequest("POST", "/api/sms/ack", strings.NewReader(string(body))))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"acknowledged":1`) {
		t.Fatal("client ack schema mismatch")
	}
	w = httptest.NewRecorder()
	a.smsStatus(w, httptest.NewRequest("GET", "/api/sms/status", nil))
	if !strings.Contains(w.Body.String(), `"auto_cleanup_me":false`) {
		t.Fatal("incorrect deletion capability")
	}
	w = httptest.NewRecorder()
	a.smsClear(w, httptest.NewRequest("POST", "/api/sms/clear-module", nil))
	if w.Code != 403 {
		t.Fatal("native deletion exposed")
	}
	if _, err := os.Stat(filepath.Join(a.mu300SMS.pool, "000001")); err != nil {
		t.Fatal("native inbox deleted")
	}
}

func TestMU300SMSHTTPChecksSIMAndReportsConfirmedSend(t *testing.T) {
	a := smsFixture(t)
	submits := 0
	a.mu300SMS.runSend = func(context.Context, string) (string, error) { submits++; return "sent (reference 8)", nil }
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"phone":"15550000001","message":"fixture","sim_account":"other"}`, 400},
		{`{"phone":"15550000001","message":"fixture","sim_account":"0"}`, 200},
	} {
		w := httptest.NewRecorder()
		a.smsSend(w, httptest.NewRequest("POST", "/api/sms/send", strings.NewReader(tc.body)))
		if w.Code != tc.status {
			t.Fatalf("SIM/send status %d", w.Code)
		}
		if w.Code == 200 && !strings.Contains(w.Body.String(), `"sent":true`) {
			t.Fatal("confirmed receipt missing")
		}
	}
	if submits != 1 {
		t.Fatal("wrong number of submissions")
	}
}

func TestMU300SMSConcurrentRefreshAndAck(t *testing.T) {
	a := smsFixture(t)
	writeSMSFixture(t, a, "000001", "mt", "fixture")
	a.mu300RefreshSMS()
	id := a.messages[0].Message.DeliveryID
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); a.mu300RefreshSMS() }()
		go func() {
			defer wg.Done()
			if _, err := a.mu300AckSMS([]string{id}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	a.mu300RefreshSMS()
	if len(a.messages) != 0 {
		t.Fatal("ack lost during concurrent refresh")
	}
}
