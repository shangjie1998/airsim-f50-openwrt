package main

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

const mu300TestActive = "+CLCC: 1,0,0,0,0,\"10010\",129\n"
const mu300TestWaiting = "+CLCC: 2,1,5,0,0,\"15550001111\",129\n"
const mu300TestHeld = "+CLCC: 1,0,1,0,0,\"10010\",129\n"
const mu300TestAnswered = "+CLCC: 2,1,0,0,0,\"15550001111\",129\n"
const mu300TestCaps = "+CHLD:(0-4,6,7,8,11-17,21-27,71-77)\nOK"

func TestMU300WaitingKeepsBothIDsAndHistory(t *testing.T) {
	a := testMU300Calls(t, nil)
	now := time.Now()
	poll := func(s string) { now = now.Add(time.Second); a.applyCallPoll(parseCLCC(s+"OK"), now) }
	poll(mu300TestActive)
	first := a.calls.Active.ID
	poll(mu300TestActive + mu300TestWaiting)
	second := a.calls.Active.ID
	if first == second || a.mu300Calls[1].ID != first {
		t.Fatal("new caller overwrote first identity")
	}
	revision := a.callRevision
	poll(mu300TestWaiting + mu300TestActive)
	if a.callRevision != revision {
		t.Fatal("response reorder created a revision")
	}
	poll(mu300TestActive)
	if a.calls.Active.ID != first {
		t.Fatal("returning to original call allocated another UUID")
	}
	if len(a.calls.History) != 1 || a.calls.History[0].ID != second || !a.calls.History[0].Missed {
		t.Fatal("waiting call missing from terminal history")
	}
	poll(mu300TestActive + mu300TestWaiting)
	third := a.calls.Active.ID
	if third == second {
		t.Fatal("reused modem slot reused a retired UUID")
	}
	poll(mu300TestHeld + mu300TestAnswered)
	if a.calls.Active.ID != third || a.mu300Calls[1].ID != first {
		t.Fatal("hold/answer changed identities")
	}
	snapshot := a.callStatusSnapshot()
	if len(snapshot.Calls) != 2 || snapshot.Calls[0].ID != first || snapshot.Calls[1].ID != third {
		t.Fatal("diagnostic calls are incomplete or unsorted")
	}
	poll("")
	if a.calls.Active != nil || len(a.calls.History) != 3 {
		t.Fatal("not every ended call was retired")
	}
}

func TestMU300MissedIdlePollStillRetiresReplacedSlot(t *testing.T) {
	a := testMU300Calls(t, nil)
	a.applyCallPoll(parseCLCC(mu300TestAnswered), time.Now())
	id := a.calls.Active.ID
	a.applyCallPoll(parseCLCC(mu300TestWaiting), time.Now())
	if a.calls.Active.ID == id || len(a.calls.History) != 1 || a.calls.History[0].ID != id {
		t.Fatal("replacement inherited old identity")
	}
}

func TestMU300CHLDCapabilityParser(t *testing.T) {
	for _, tc := range []struct {
		response, operation string
		want                bool
	}{
		{mu300TestCaps, "2", true}, {mu300TestCaps, "12", true}, {mu300TestCaps, "19", false},
		{"+CHLD: (0,1,1x,2,2X,3)\nOK", "17", true}, {"+CHLD: (0,1,1x,2,2X,3)\nOK", "22", true},
		{"+CHLD: (12,21)\nOK", "2", false}, {"+CHLD: (1x)\nOK", "18", false},
		{"+CHLD: (not-a-range)\nOK", "2", false}, {"OK", "2", false},
	} {
		if got := mu300SupportsCHLD(tc.response, tc.operation); got != tc.want {
			t.Fatalf("%q %s: %v", tc.response, tc.operation, got)
		}
	}
}

func TestMU300WaitingAnswerAndTargetedHangup(t *testing.T) {
	state := mu300TestActive + mu300TestWaiting + "OK"
	var writes []string
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		switch cmd {
		case "AT+CLCC":
			return state, nil
		case "AT+CHLD=?":
			return mu300TestCaps, nil
		case "AT+CHLD=2":
			writes = append(writes, cmd)
			state = mu300TestHeld + mu300TestAnswered + "OK"
			return "OK", nil
		case "AT+CHLD=12":
			writes = append(writes, cmd)
			state = mu300TestHeld + "OK"
			return "OK", nil
		}
		t.Fatalf("unsafe/unexpected modem write: %s", cmd)
		return "", nil
	})
	_, _ = a.mu300RefreshCalls()
	first, second := a.mu300Calls[1].ID, a.mu300Calls[2].ID
	if err := a.answerCallForID(second); err != nil {
		t.Fatal(err)
	}
	if err := a.answerCallForID(second); err != nil {
		t.Fatal("idempotent answer", err)
	}
	if !reflect.DeepEqual(writes, []string{"AT+CHLD=2"}) {
		t.Fatal("repeated answer swapped held calls", writes)
	}
	if a.mu300Calls[1].ID != first || a.mu300Calls[1].State != "held" || a.mu300Calls[2].ID != second {
		t.Fatal("identity/hold mismatch")
	}
	request := httptest.NewRequest("POST", "/api/calls/hangup", strings.NewReader(fmt.Sprintf(`{"call_id":%q}`, second)))
	response := httptest.NewRecorder()
	a.hangup(response, request)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if a.calls.Active.ID != first || len(a.mu300Calls) != 1 || len(writes) != 2 {
		t.Fatal("hangup affected another call", writes)
	}
	if err := a.answerCallForID(second); err == nil || len(writes) != 2 {
		t.Fatal("stale answer reached surviving call")
	}
}

func TestMU300RejectWaitingDoesNotHangUpActive(t *testing.T) {
	state := mu300TestActive + mu300TestWaiting + "OK"
	writes := 0
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		switch cmd {
		case "AT+CLCC":
			return state, nil
		case "AT+CHLD=?":
			return mu300TestCaps, nil
		case "AT+CHLD=0":
			writes++
			state = mu300TestActive + "OK"
			return "OK", nil
		}
		t.Fatalf("unexpected write %s", cmd)
		return "", nil
	})
	_, _ = a.mu300RefreshCalls()
	first, second := a.mu300Calls[1].ID, a.mu300Calls[2].ID
	if err := a.rejectCallForID(second); err != nil {
		t.Fatal(err)
	}
	if writes != 1 || a.calls.Active.ID != first {
		t.Fatal("reject damaged active call")
	}
	if err := a.rejectCallForID(first); err == nil || writes != 1 {
		t.Fatal("reject terminated connected call")
	}
}

func TestMU300MultiCallGuardsAndCapabilityRace(t *testing.T) {
	for _, mode := range []string{"missing-id", "stale-id", "unsupported", "capability-failure", "caller-ended", "peer-replaced", "extra-held"} {
		t.Run(mode, func(t *testing.T) {
			state := mu300TestActive + mu300TestWaiting + "OK"
			if mode == "extra-held" {
				state = mu300TestActive + mu300TestWaiting + "+CLCC: 3,0,1,0,0\nOK"
			}
			a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
				if cmd == "AT+CLCC" {
					return state, nil
				}
				if cmd == "AT+CHLD=?" {
					switch mode {
					case "unsupported":
						return "+CHLD: (0,1)\nOK", nil
					case "capability-failure":
						return "", errors.New("timeout")
					case "caller-ended":
						state = mu300TestActive + "OK"
					case "peer-replaced":
						state = strings.Replace(mu300TestActive, "1,0,0", "1,1,0", 1) + mu300TestWaiting + "OK"
					}
					return mu300TestCaps, nil
				}
				t.Fatalf("guard allowed mutation %s", cmd)
				return "", nil
			})
			_, _ = a.mu300RefreshCalls()
			id := a.mu300Calls[2].ID
			if mode == "missing-id" {
				id = ""
			}
			if mode == "stale-id" {
				id = "mu300-retired"
			}
			if err := a.answerCallForID(id); err == nil {
				t.Fatal("guard did not reject")
			}
		})
	}
}

func TestMU300AnswerConfirmationAndNoToggleRetry(t *testing.T) {
	for _, mode := range []string{"delayed", "timeout-but-applied", "accepted-not-applied", "caller-ended"} {
		t.Run(mode, func(t *testing.T) {
			state := mu300TestActive + mu300TestWaiting + "OK"
			writes, polls := 0, 0
			a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
				switch cmd {
				case "AT+CHLD=?":
					return mu300TestCaps, nil
				case "AT+CLCC":
					if writes > 0 {
						polls++
						if mode == "delayed" && polls == 3 {
							state = mu300TestHeld + mu300TestAnswered + "OK"
						}
					}
					return state, nil
				case "AT+CHLD=2":
					writes++
					if mode == "timeout-but-applied" {
						state = mu300TestHeld + mu300TestAnswered + "OK"
						return "", errors.New("timeout")
					}
					if mode == "caller-ended" {
						state = mu300TestActive + "OK"
					}
					return "OK", nil
				}
				t.Fatalf("unexpected %s", cmd)
				return "", nil
			})
			_, _ = a.mu300RefreshCalls()
			err := a.answerCallForID(a.mu300Calls[2].ID)
			wantSuccess := mode == "delayed" || mode == "timeout-but-applied"
			if (err == nil) != wantSuccess || writes != 1 {
				t.Fatalf("success=%v writes=%d err=%v", wantSuccess, writes, err)
			}
			if mode == "accepted-not-applied" {
				if err := a.answerCallForID(a.mu300Calls[2].ID); err == nil || writes != 1 {
					t.Fatal("uncertain hold command was repeated")
				}
			}
		})
	}
}

func TestMU300CloudActionRetainsIDGuard(t *testing.T) {
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		if cmd == "AT+CLCC" {
			return mu300TestActive + mu300TestWaiting + "OK", nil
		}
		t.Fatalf("stale cloud action mutated call: %s", cmd)
		return "", nil
	})
	_, _ = a.mu300RefreshCalls()
	for _, action := range []string{"answer", "reject", "end"} {
		if err := a.executeIncomingCloudCallAction(cloudCallAction{CallID: "old-call"}, action); err == nil {
			t.Fatal("stale cloud control accepted", action)
		}
	}
	if !a.cloudCallStillExists(a.mu300Calls[1].ID) {
		t.Fatal("unselected original call was treated as ended")
	}
}

func TestMU300MalformedCLCCPreservesIdentity(t *testing.T) {
	state := mu300TestActive + "OK"
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		if cmd == "AT+CLCC" {
			return state, nil
		}
		t.Fatalf("unexpected mutation %s", cmd)
		return "", nil
	})
	_, _ = a.mu300RefreshCalls()
	id := a.calls.Active.ID
	for _, invalid := range []string{"+CLCC: broken\nOK", mu300TestActive + mu300TestActive + "OK", "+CLCC: 1,0,99,0,0\nOK"} {
		state = invalid
		if _, err := a.mu300RefreshCalls(); err == nil || a.calls.Active.ID != id {
			t.Fatal("invalid snapshot reset identity")
		}
		if err := a.hangupCallForID(id); err == nil {
			t.Fatal("invalid snapshot allowed control")
		}
	}
}
