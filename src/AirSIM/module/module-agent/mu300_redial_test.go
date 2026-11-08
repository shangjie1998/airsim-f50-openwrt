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

func redialFixture(t *testing.T) (*agent, *string, *int) {
	state, writes := "OK", 0
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		if cmd == "AT+CLCC" {
			return state, nil
		}
		if strings.HasPrefix(cmd, "ATD") {
			writes++
			state = `+CLCC: 1,0,3,0,0,"12345",129` + "\nOK"
			return "OK", nil
		}
		return "", fmt.Errorf("unexpected control command")
	})
	return a, &state, &writes
}
func TestMU300ConfirmedEndAllowsImmediateManualRedialButNeverReplays(t *testing.T) {
	a, state, writes := redialFixture(t)
	first, e := a.mu300Dial("12345", "first-click", "")
	if e != nil {
		t.Fatal(e)
	}
	*state = "OK"
	a.mu300ApplyCallPoll(nil, time.Now(), false)
	old, e := a.mu300Dial("12345", "first-click", "")
	if e != nil || old["state"] != "ended" || *writes != 1 {
		t.Fatal("old intent repeated ATD", e)
	}
	next, e := a.mu300Dial("12345", "second-click", "")
	if e != nil || *writes != 2 || next["backend_id"] == first["backend_id"] {
		t.Fatal("new manual dial blocked or reused old call", e)
	}
	a.mu300Dial("12345", "first-click", "")
	if *writes != 2 || a.calls.Active.ID != next["backend_id"] {
		t.Fatal("old retry altered replacement")
	}
}
func TestMU300OldQuietUntilDoesNotBlockNewIntent(t *testing.T) {
	a, _, writes := redialFixture(t)
	old := []byte(`{"version":1,"number_digest":"fixture","pending":false,"quiet_until":"2099-01-01T00:00:00Z"}`)
	path := filepath.Join(a.mu300Operations.directory, "dial.json")
	if e := os.WriteFile(path, old, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := a.mu300Dial("12345", "new-click", ""); e != nil || *writes != 1 {
		t.Fatal("legacy time lock still enforced", e)
	}
	unchanged, _ := os.ReadFile(path)
	if string(unchanged) != string(old) {
		t.Fatal("legacy rollback record modified")
	}
}
func TestMU300MissingIntentCannotBypassPersistentDeduplication(t *testing.T) {
	a, _, writes := redialFixture(t)
	if _, e := a.mu300Dial("12345", "", ""); e == nil || *writes != 0 {
		t.Fatal("unidentified non-idempotent dial accepted")
	}
}
func TestMU300SameIntentUnknownOutcomeNeverRepeatsATD(t *testing.T) {
	writes := 0
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		if cmd == "AT+CLCC" {
			return "OK", nil
		}
		writes++
		return "", fmt.Errorf("reply lost")
	})
	for i := 0; i < 3; i++ {
		if _, e := a.mu300Dial("12345", "same-click", ""); e == nil {
			t.Fatal("unknown outcome accepted")
		}
	}
	if writes != 1 {
		t.Fatal("unknown operation repeated", writes)
	}
}
func TestMU300ConcurrentFreshRequestsStillRespectLiveCall(t *testing.T) {
	a, _, writes := redialFixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); a.mu300Dial("12345", fmt.Sprint(i), "") }(i)
	}
	wg.Wait()
	if *writes != 1 {
		t.Fatal("concurrent calls emitted multiple ATD", *writes)
	}
}
func TestMU300HTTPNewClickImmediatelyAfterEnd(t *testing.T) {
	a, state, writes := redialFixture(t)
	call := func(op string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"number": "12345", "operation_id": op})
		w := httptest.NewRecorder()
		a.dial(w, httptest.NewRequest("POST", "/api/calls/dial", strings.NewReader(string(body))))
		return w
	}
	if w := call("first"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	*state = "OK"
	a.mu300ApplyCallPoll(nil, time.Now(), false)
	if w := call("second"); w.Code != 200 || *writes != 2 {
		t.Fatal("HTTP redial still has cooldown", w.Code)
	}
	if w := call("second"); w.Code != 200 || *writes != 2 {
		t.Fatal("HTTP intent replay redialed", w.Code)
	}
}
