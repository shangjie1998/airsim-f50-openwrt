package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMU300ConfirmedEndPublishesExplicitTerminalState(t *testing.T) {
	a := testMU300Calls(t, nil)
	a.mu300ApplyCallPoll([]parsedCall{{Index: 1, Direction: "outgoing", State: "alerting"}}, time.Now(), true)
	id := a.calls.Active.ID
	a.mu300ApplyCallPoll(nil, time.Now(), false)
	snapshot := a.callStatusSnapshot()
	if snapshot.Active != nil || len(snapshot.History) != 1 || snapshot.History[0].ID != id || snapshot.History[0].State != "ended" {
		t.Fatal("confirmed end retained a nonterminal wire state")
	}
}

func appendMU300URC(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString(line); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}

func mu300TerminalFixture(t *testing.T) (*agent, *string, string) {
	t.Helper()
	state := `+CLCC: 1,0,3,0,0,"12345",129` + "\nOK"
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		if cmd != "AT+CLCC" {
			t.Fatal("unexpected control command", cmd)
		}
		return state, nil
	})
	path := filepath.Join(t.TempDir(), "stty_nr0.log")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	a.mu300URC = &mu300URCTail{path: path}
	if _, err := a.mu300RefreshCalls(); err != nil {
		t.Fatal(err)
	}
	return a, &state, path
}

func TestMU300FreshTerminalAndIdleCLCCRetireSameUUIDImmediately(t *testing.T) {
	a, state, path := mu300TerminalFixture(t)
	id := a.calls.Active.ID
	revision := a.callRevision
	appendMU300URC(t, path, "^DSCI: 1,0,6,1,0,\"PRIVATE-FIXTURE\"\n")
	*state = "OK"
	if _, err := a.mu300RefreshCalls(); err != nil {
		t.Fatal(err)
	}
	if a.calls.Active != nil || len(a.calls.History) != 1 || a.calls.History[0].ID != id || a.callRevision <= revision {
		t.Fatal("fresh matching terminal event was not published")
	}
	// This is the exact payload used by both long-poll and ordinary status.
	wire, err := json.Marshal(a.callStatusSnapshot())
	if err != nil || !strings.Contains(string(wire), `"state":"ended"`) || !strings.Contains(string(wire), `"active":null`) {
		t.Fatal("terminal notification is not explicit")
	}
	if err := a.hangupCallForID(id); err != nil {
		t.Fatal("ended-call hangup not idempotent", err)
	}
}

func TestMU300TerminalHintWaitsForLaggingCLCCWithoutLosingEnd(t *testing.T) {
	a, state, path := mu300TerminalFixture(t)
	id := a.calls.Active.ID
	appendMU300URC(t, path, "^DSCI: 1,0,6,1\n")
	if _, err := a.mu300RefreshCalls(); err != nil {
		t.Fatal(err)
	}
	if a.calls.Active == nil || a.calls.Active.ID != id {
		t.Fatal("terminal log overrode live modem")
	}
	*state = "OK"
	_, _ = a.mu300RefreshCalls()
	if a.calls.Active != nil || len(a.calls.History) != 1 || a.calls.History[0].ID != id {
		t.Fatal("fresh terminal hint was lost while CLCC lagged")
	}
}

func TestMU300TerminalHintCannotAffectAnotherSlotDirectionOrGeneration(t *testing.T) {
	for _, line := range []string{"^DSCI: 2,0,6,1\n", "^DSCI: 1,1,6,1\n", "^DSCI: 1,0,6,1\n^DSCI: 1,0,2,1\n"} {
		a, state, path := mu300TerminalFixture(t)
		id := a.calls.Active.ID
		appendMU300URC(t, path, line)
		*state = "OK"
		_, _ = a.mu300RefreshCalls()
		if a.calls.Active == nil || a.calls.Active.ID != id {
			t.Fatal("wrong/stale hint ended a call")
		}
	}
	// A slot replacement gets a new UUID; the hint stays bound to the old UUID.
	a, state, path := mu300TerminalFixture(t)
	id := a.calls.Active.ID
	appendMU300URC(t, path, "^DSCI: 1,0,6,1\n")
	*state = `+CLCC: 1,1,4,0,0,"54321",129` + "\nOK"
	_, _ = a.mu300RefreshCalls()
	if a.calls.Active == nil || a.calls.Active.ID == id || a.calls.Active.State != "incoming" {
		t.Fatal("replacement call lost")
	}
	if err := a.hangupCallForID(id); err == nil {
		t.Fatal("stale hangup accepted with a live replacement")
	}
}

func TestMU300URCTailSkipsBacklogRotationAndTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stty_nr0.log")
	terminal := "^DSCI: 1,0,6,1\n"
	if err := os.WriteFile(path, []byte(terminal), 0600); err != nil {
		t.Fatal(err)
	}
	tail := &mu300URCTail{path: path}
	if len(tail.read()) != 0 {
		t.Fatal("startup replayed old terminal")
	}
	appendMU300URC(t, path, terminal)
	if len(tail.read()) != 1 {
		t.Fatal("new terminal not read")
	}
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(terminal), 0600); err != nil {
		t.Fatal(err)
	}
	if len(tail.read()) != 0 {
		t.Fatal("rotation replayed backlog")
	}
	// Same inode, truncated and grown past the old cursor: anchor must detect it.
	if err := os.WriteFile(path, []byte("unrelated-prefix\n"+terminal), 0600); err != nil {
		t.Fatal(err)
	}
	if len(tail.read()) != 0 {
		t.Fatal("truncation/regrowth replayed a terminal")
	}
	appendMU300URC(t, path, terminal)
	if len(tail.read()) != 1 {
		t.Fatal("cursor did not recover")
	}
}

func TestMU300URCTailHandlesPartialLinesWithoutExposingMessageText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stty_nr0.log")
	if err := os.WriteFile(path, []byte("^DSCI: 1,0,"), 0600); err != nil {
		t.Fatal(err)
	}
	tail := &mu300URCTail{path: path}
	_ = tail.read()
	appendMU300URC(t, path, "6,1\n+CMT: PRIVATE-FIXTURE\n")
	if len(tail.read()) != 0 {
		t.Fatal("old partial line counted as fresh event")
	}
	appendMU300URC(t, path, "^DSCI: 1,0,6,")
	if len(tail.read()) != 0 {
		t.Fatal("partial event interpreted")
	}
	appendMU300URC(t, path, "1\n")
	hints := tail.read()
	if len(hints) != 1 || hints[0].slot != 1 {
		t.Fatal("completed event not recognized")
	}
}

func TestMU300URCTailUnavailableDoesNotDisableSafePolling(t *testing.T) {
	a, state, path := mu300TerminalFixture(t)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	*state = "OK"
	if _, err := a.mu300RefreshCalls(); err != nil {
		t.Fatal("missing optional log broke polling", err)
	}
	if a.calls.Active == nil {
		t.Fatal("missing log forced terminal state")
	}
	if err := a.hangupCallForID("unknown-id"); err == nil {
		t.Fatal("unknown hangup accepted")
	}
}

func TestMU300DSCITerminalParserStaysProfileSpecific(t *testing.T) {
	for _, line := range []string{"^DSCI: 1,0,6,1", "^DSCI: 1,0,6,3"} {
		slot, direction, ended, ok := parseMU300TerminalDSCI(line)
		if !ok || slot != 1 || direction != "outgoing" || !ended {
			t.Fatal("verified MU300 terminal rejected")
		}
		if _, _, ok := parseDSCI(line); ok {
			t.Fatal("generic parser behavior changed")
		}
	}
	for _, line := range []string{"NO CARRIER", "^DSCI: 0,0,6,1", "^DSCI: 8,0,6,1", "^DSCI: 1,2,6,1", "^DSCI: 1,0,x,1", "^DSCI: 1,0,6,9"} {
		if _, _, _, ok := parseMU300TerminalDSCI(line); ok {
			t.Fatal("ambiguous terminal accepted", line)
		}
	}
}

func TestMU300TerminalNotificationReachesLongPoll(t *testing.T) {
	a, state, path := mu300TerminalFixture(t)
	id, revision := a.calls.Active.ID, a.callRevision
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", fmt.Sprintf("/api/calls/events?after=%d&timeout_ms=1000", revision), nil)
	done := make(chan struct{})
	go func() { a.callEvents(w, r); close(done) }()
	appendMU300URC(t, path, "^DSCI: 1,0,6,1\n")
	*state = "OK"
	if _, err := a.mu300RefreshCalls(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("terminal notification stalled")
	}
	var payload callStatusPayload
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &payload) != nil || payload.Active != nil ||
		len(payload.History) != 1 || payload.History[0].ID != id || payload.History[0].State != "ended" || payload.Revision <= revision {
		t.Fatal("long poll did not deliver original call's terminal identity")
	}
}

func TestMU300MalformedURCBatchFallsBackToGrace(t *testing.T) {
	a, state, path := mu300TerminalFixture(t)
	appendMU300URC(t, path, "^DSCI: 1,0,6,1\n^DSCI: malformed\n")
	*state = "OK"
	if _, err := a.mu300RefreshCalls(); err != nil {
		t.Fatal(err)
	}
	if a.calls.Active == nil {
		t.Fatal("partly corrupt batch bypassed normal grace")
	}
}

func TestMU300HangupAlreadyEndedKnownCallIsIdempotent(t *testing.T) {
	writes := 0
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		if cmd != "AT+CLCC" {
			writes++
		}
		return "OK", nil
	})
	a.mu300ApplyCallPoll([]parsedCall{{Index: 1, Direction: "outgoing", State: "alerting"}}, time.Now(), true)
	id := a.calls.Active.ID
	a.mu300ApplyCallPoll(nil, time.Now(), false)
	if err := a.hangupCallForID(id); err != nil || writes != 0 {
		t.Fatal("known ended call failed harmless hangup", err, writes)
	}
}

func TestMU300TerminalArrivingDuringQueryEndsInSamePoll(t *testing.T) {
	a, state, path := mu300TerminalFixture(t)
	id := a.calls.Active.ID
	a.callControlCommand = func(cmd string, _ time.Duration) (string, error) {
		if cmd != "AT+CLCC" {
			t.Fatal("unexpected control write")
		}
		appendMU300URC(t, path, "^DSCI: 1,0,6,1\n")
		*state = "OK"
		return *state, nil
	}
	if _, e := a.mu300RefreshCalls(); e != nil {
		t.Fatal(e)
	}
	if a.calls.Active != nil || len(a.calls.History) != 1 || a.calls.History[0].ID != id {
		t.Fatal("end event waited for another poll")
	}
}
func TestMU300PendingTerminalInvalidatedByProgressOrExpiry(t *testing.T) {
	for _, kind := range []string{"progress", "expired", "rotation", "replacement"} {
		t.Run(kind, func(t *testing.T) {
			a, state, path := mu300TerminalFixture(t)
			appendMU300URC(t, path, "^DSCI: 1,0,6,1\n")
			now := time.Now()
			if len(a.mu300TerminalHintsAt(now)) != 1 {
				t.Fatal("missing fresh hint")
			}
			switch kind {
			case "progress":
				appendMU300URC(t, path, "^DSCI: 1,0,2,1\n")
			case "expired":
				now = now.Add(mu300TerminalHintLifetime + time.Nanosecond)
			case "rotation":
				os.Rename(path, path+".old")
				os.WriteFile(path, nil, 0600)
			case "replacement":
				a.mu300ApplyCallPoll([]parsedCall{{Index: 1, Direction: "incoming", State: "incoming"}}, now, true)
			}
			if len(a.mu300TerminalHintsAt(now)) != 0 {
				t.Fatal("stale hint survived", kind)
			}
			*state = "OK"
			_, _ = a.mu300RefreshCalls()
			if a.calls.Active == nil {
				t.Fatal("stale hint ended a live identity", kind)
			}
		})
	}
}
func TestMU300FailedCLCCDoesNotEmitEndButRetainsFreshHint(t *testing.T) {
	a, state, path := mu300TerminalFixture(t)
	id, rev := a.calls.Active.ID, a.callRevision
	appendMU300URC(t, path, "^DSCI: 1,0,6,1\n")
	original := a.callControlCommand
	a.callControlCommand = func(string, time.Duration) (string, error) { return "", fmt.Errorf("unreliable query") }
	if _, e := a.mu300RefreshCalls(); e == nil || a.calls.Active == nil || a.callRevision != rev {
		t.Fatal("failed query confirmed end")
	}
	a.callControlCommand = original
	*state = "OK"
	_, _ = a.mu300RefreshCalls()
	if a.calls.Active != nil || a.calls.History[0].ID != id {
		t.Fatal("fresh end lost after transient query failure")
	}
}
