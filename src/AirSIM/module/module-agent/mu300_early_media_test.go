package main

import (
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func earlyAgentFixture(t *testing.T) (*agent, *string, string, string) {
	t.Helper()
	a, state, path := mu300TerminalFixture(t)
	owner := "wlan:" + strings.Repeat("a", 64)
	a.mu300Operations = &mu300OperationStore{directory: t.TempDir()}
	if _, _, err := a.mu300Operations.begin(owner, "fixture-op", "12345", ""); err != nil {
		t.Fatal(err)
	}
	if err := a.mu300Operations.finish(owner, "fixture-op", a.calls.Active.ID, true); err != nil {
		t.Fatal(err)
	}
	return a, state, path, owner
}
func earlyAgentStatus(a *agent, owner, id string) int {
	r := httptest.NewRequest("GET", "/api/calls/early-media?call_id="+id, nil)
	r.Header.Set(mu300OwnerHeader, owner)
	w := httptest.NewRecorder()
	a.mu300EarlyMediaStatus(w, r)
	return w.Code
}
func TestMU300EarlyMediaRequiresOwnedFreshSignalWithoutChangingCall(t *testing.T) {
	a, _, path, owner := earlyAgentFixture(t)
	id := a.calls.Active.ID
	revision := a.callRevision
	if code := earlyAgentStatus(a, owner, id); code != 202 {
		t.Fatal("signal not pending", code)
	}
	appendMU300URC(t, path, "+EARLYMEDIA: 1\n")
	if _, err := a.mu300RefreshCalls(); err != nil {
		t.Fatal(err)
	}
	if code := earlyAgentStatus(a, owner, id); code != 200 {
		t.Fatal("signal not available", code)
	}
	if a.calls.Active.State != "alerting" || a.callRevision != revision {
		t.Fatal("early media faked a connected call")
	}
	if code := earlyAgentStatus(a, "wlan:"+strings.Repeat("b", 64), id); code != 403 {
		t.Fatal("cross-owner evidence", code)
	}
	if code := earlyAgentStatus(a, owner, "unknown-call"); code != 403 {
		t.Fatal("unknown call adopted", code)
	}
	a.mu300EarlyPollAt = time.Now().Add(-3 * time.Second)
	if code := earlyAgentStatus(a, owner, id); code != 410 {
		t.Fatal("stale state authorized", code)
	}
}
func TestMU300EarlyMediaRevokedOnSignalLossRotationAndPollFailure(t *testing.T) {
	for _, why := range []string{"zero", "malformed", "rotation", "poll-failure", "active", "gone", "replacement"} {
		t.Run(why, func(t *testing.T) {
			a, state, path, owner := earlyAgentFixture(t)
			id := a.calls.Active.ID
			appendMU300URC(t, path, "+EARLYMEDIA: 1\n")
			a.mu300RefreshCalls()
			if earlyAgentStatus(a, owner, id) != 200 {
				t.Fatal("fixture")
			}
			switch why {
			case "zero":
				appendMU300URC(t, path, "+EARLYMEDIA: 0\n")
			case "malformed":
				appendMU300URC(t, path, "+EARLYMEDIA: invalid\n")
			case "rotation":
				os.Rename(path, path+".old")
				os.WriteFile(path, []byte("+EARLYMEDIA: 1\n"), 0600)
			case "poll-failure":
				a.callControlCommand = func(string, time.Duration) (string, error) { return "", errors.New("fixture timeout") }
			case "active":
				*state = "+CLCC: 1,0,0,0,0,\"12345\",129\nOK"
			case "gone":
				*state = "OK"
			case "replacement":
				*state = "+CLCC: 1,0,3,0,0,\"54321\",129\nOK"
			}
			a.mu300RefreshCalls()
			if earlyAgentStatus(a, owner, id) == 200 {
				t.Fatal("invalidated source retained")
			}
		})
	}
}
func TestMU300EarlyMediaBacklogAndNoOwnerDigestCannotAuthorize(t *testing.T) {
	a, _, path, owner := earlyAgentFixture(t)
	id := a.calls.Active.ID
	a.mu300URC = &mu300URCTail{path: path}
	appendMU300URC(t, path, "+EARLYMEDIA: 1\n")
	a.mu300RefreshCalls()
	if earlyAgentStatus(a, owner, id) != 202 {
		t.Fatal("backlog authorized")
	}
	appendMU300URC(t, path, "+EARLYMEDIA: 1\n")
	a.mu300RefreshCalls()
	key := mu300OperationKey(owner, "fixture-op")
	r := a.mu300Operations.journal.Records[key]
	r.OwnerDigest = ""
	a.mu300Operations.journal.Records[key] = r
	if earlyAgentStatus(a, owner, id) != 403 {
		t.Fatal("legacy record guessed owner")
	}
	if err := a.mu300Operations.save(); err != nil {
		t.Fatal(err)
	}
	other := &mu300OperationStore{directory: a.mu300Operations.directory}
	if _, found, err := other.lookup(owner, "fixture-op"); err != nil || !found {
		t.Fatal("legacy journal broke")
	}
}
