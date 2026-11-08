package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMU300PersistentOperationReturnsSameReceiptAfterRestart(t *testing.T) {
	state, writes := "OK", 0
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		if cmd == "AT+CLCC" {
			return state, nil
		}
		writes++
		state = `+CLCC: 1,0,3,0,0,"12345",129` + "\nOK"
		return "OK", nil
	})
	first, e := a.mu300DialFor("owner-A", "12345", "intent-one", "")
	if e != nil {
		t.Fatal(e)
	}
	dir := a.mu300Operations.directory
	restarted := testMU300Calls(t, func(string, time.Duration) (string, error) { t.Fatal("replayed receipt touched modem"); return "", nil })
	restarted.mu300Operations = &mu300OperationStore{directory: dir}
	again, e := restarted.mu300DialFor("owner-A", "12345", "intent-one", "")
	if e != nil || again["backend_id"] != first["backend_id"] || writes != 1 {
		t.Fatal("receipt lost across restart", e)
	}
	if _, e = restarted.mu300DialFor("owner-A", "54321", "intent-one", ""); e == nil {
		t.Fatal("changed parameters accepted")
	}
	data, e := os.ReadFile(filepath.Join(dir, "operations.json"))
	if e != nil {
		t.Fatal(e)
	}
	for _, private := range []string{"12345", "intent-one", "owner-A"} {
		if strings.Contains(string(data), private) {
			t.Fatal("private input persisted")
		}
	}
	info, _ := os.Stat(filepath.Join(dir, "operations.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("nonprivate journal")
	}
}
func TestMU300CrashOccupancyAndManyIntentsNeverRedial(t *testing.T) {
	dir := t.TempDir()
	s := &mu300OperationStore{directory: dir}
	// Simulate a crash after durable occupancy, possibly after ATD; no receipt.
	if _, _, e := s.begin("owner", "original-number", "12345", ""); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 10; i++ {
		if _, _, e := s.begin("owner", fmt.Sprint(i), "54321", ""); e != nil {
			t.Fatal(e)
		}
	}
	a := testMU300Calls(t, func(string, time.Duration) (string, error) {
		t.Fatal("unknown old intent reached modem")
		return "", nil
	})
	a.mu300Operations = &mu300OperationStore{directory: dir}
	if _, e := a.mu300DialFor("owner", "12345", "original-number", ""); e == nil {
		t.Fatal("unknown submission treated as safe new dial")
	}
}
func TestMU300OperationQueryIsReadOnlyAndOwnerScoped(t *testing.T) {
	a := testMU300Calls(t, func(string, time.Duration) (string, error) { t.Fatal("query touched modem"); return "", nil })
	owner := "wlan:" + mu300DialHash("paired-A")
	if _, _, e := a.mu300Operations.begin(owner, "intent", "12345", ""); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		owner string
		code  int
	}{{owner, 200}, {"wlan:" + mu300DialHash("paired-B"), 404}, {"", 404}, {"forged", 400}} {
		r := httptest.NewRequest("GET", "/api/calls/operation?operation_id=intent", nil)
		r.Header.Set(mu300OwnerHeader, tc.owner)
		w := httptest.NewRecorder()
		a.mu300OperationStatus(w, r)
		if w.Code != tc.code {
			t.Fatalf("status %d expected %d", w.Code, tc.code)
		}
		if tc.code == 200 && !strings.Contains(w.Body.String(), `"status":"unknown"`) {
			t.Fatal("unknown outcome lost")
		}
	}
}
func TestMU300OperationEndSurvivesHistoryAckAndRestart(t *testing.T) {
	s := &mu300OperationStore{directory: t.TempDir()}
	s.begin("owner", "intent", "12345", "")
	if e := s.finish("owner", "intent", "mu300-call-one", true); e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	s.ended([]callRecord{{ID: "mu300-another-call", EndedAt: &now}})
	r, _, _ := s.lookup("owner", "intent")
	if r.EndedAt != nil {
		t.Fatal("foreign terminal ended intent")
	}
	s.ended([]callRecord{{ID: "mu300-call-one", EndedAt: &now}})
	s = &mu300OperationStore{directory: s.directory}
	r, ok, e := s.lookup("owner", "intent")
	if e != nil || !ok || r.EndedAt == nil || !r.EndedAt.Equal(now) {
		t.Fatal("terminal receipt not durable")
	}
	receipt, e := r.receipt("intent")
	if e != nil || receipt["dialing"] != false || receipt["state"] != "ended" {
		t.Fatal("ended receipt still claims dialing")
	}
}
func TestMU300OperationJournalFailsClosed(t *testing.T) {
	for _, kind := range []string{"corrupt", "symlink", "unwritable", "full"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			s := &mu300OperationStore{directory: dir}
			switch kind {
			case "corrupt":
				os.WriteFile(filepath.Join(dir, "operations.json"), []byte(`{"version":1}`), 0600)
			case "symlink":
				os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "operations.json"))
			case "unwritable":
				os.WriteFile(filepath.Join(dir, "file"), nil, 0600)
				s.directory = filepath.Join(dir, "file", "child")
			case "full":
				s.loaded = true
				s.journal = mu300OperationJournal{Version: 1, Records: map[string]mu300Operation{}}
				for i := 0; i < mu300OperationLimit; i++ {
					s.journal.Records[mu300DialHash(fmt.Sprint(i))] = mu300Operation{Parameters: mu300DialHash("fixture"), Status: "unknown"}
				}
			}
			a := testMU300Calls(t, func(string, time.Duration) (string, error) { t.Fatal("invalid journal touched modem"); return "", nil })
			a.mu300Operations = s
			if _, e := a.mu300Dial("12345", "fresh", ""); e == nil {
				t.Fatal("invalid journal accepted")
			}
		})
	}
}
func TestMU300ConcurrentIntentSubmissionAndOwnerCollision(t *testing.T) {
	s := &mu300OperationStore{directory: t.TempDir()}
	var wg sync.WaitGroup
	fresh := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, exists, e := s.begin("owner-A", "same-intent", "12345", "")
			if e != nil {
				t.Error(e)
			}
			fresh <- !exists
		}()
	}
	wg.Wait()
	close(fresh)
	n := 0
	for f := range fresh {
		if f {
			n++
		}
	}
	if n != 1 {
		t.Fatal("multiple reservations", n)
	}
	if _, exists, e := s.begin("owner-B", "same-intent", "54321", ""); e != nil || exists {
		t.Fatal("owners shared a receipt", e)
	}
	if _, _, e := s.begin("owner-A", "same-intent", "12345", "other-sim"); e == nil {
		t.Fatal("SIM change accepted")
	}
}
func TestMU300ReceiptCommitFailureNeverAllowsRetry(t *testing.T) {
	a := testMU300Calls(t, nil)
	state, writes := "OK", 0
	a.callControlCommand = func(cmd string, _ time.Duration) (string, error) {
		if cmd == "AT+CLCC" {
			return state, nil
		}
		writes++
		state = `+CLCC: 1,0,3,0,0,"12345",129` + "\nOK"
		// The occupied journal remains on disk, but later updates cannot commit.
		a.mu300Operations.directory = filepath.Join(a.mu300Operations.directory, "operations.json", "bad")
		return "OK", nil
	}
	dir := a.mu300Operations.directory
	if _, e := a.mu300Dial("12345", "intent", ""); e == nil {
		t.Fatal("unpersisted success returned")
	}
	restarted := testMU300Calls(t, func(string, time.Duration) (string, error) { t.Fatal("failed commit caused replay"); return "", nil })
	restarted.mu300Operations = &mu300OperationStore{directory: dir}
	if _, e := restarted.mu300Dial("12345", "intent", ""); e == nil || writes != 1 {
		t.Fatal("lost ambiguous occupancy")
	}
}
func TestMU300OperationQueryConfirmedReceiptUsesExactID(t *testing.T) {
	a := testMU300Calls(t, nil)
	a.mu300Operations.begin("local", "intent", "12345", "")
	a.mu300Operations.finish("local", "intent", "mu300-known", true)
	a.mu300Calls = map[int]callRecord{1: {ID: "mu300-other", State: "active"}}
	w := httptest.NewRecorder()
	a.mu300OperationStatus(w, httptest.NewRequest("GET", "/api/calls/operation?operation_id=intent", nil))
	var got map[string]any
	json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || got["backend_id"] != "mu300-known" || got["call_state"] != "unknown" {
		t.Fatal("operation adopted unrelated current call")
	}
}

func TestMU300DialReceiptNeverAdoptsAnotherActiveProjection(t *testing.T) {
	state := "OK"
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		if cmd == "AT+CLCC" {
			return state, nil
		}
		state = "+CLCC: 1,1,0,0,0,\"54321\",129\n+CLCC: 2,0,2,0,0,\"12345\",129\nOK"
		return "OK", nil
	})
	receipt, e := a.mu300Dial("12345", "outgoing-intent", "")
	if e != nil {
		t.Fatal(e)
	}
	if a.calls.Active.ID == a.mu300Calls[2].ID {
		t.Fatal("fixture did not select other active call")
	}
	if receipt["backend_id"] != a.mu300Calls[2].ID {
		t.Fatal("receipt bound to another call")
	}
}
