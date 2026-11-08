package main

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestMU300MixedVoiceModeKeepsRingingCall(t *testing.T) {
	mode, state := 0, 2
	a := testMU300Calls(t, func(command string, _ time.Duration) (string, error) {
		if command != "AT+CLCC" {
			t.Fatalf("unexpected command %s", command)
		}
		return fmt.Sprintf("+CLCC: 1,0,%d,%d,0,\"15550001111\",129\nOK", state, mode), nil
	})
	_, _ = a.mu300RefreshCalls()
	id := a.calls.Active.ID
	mode, state = 3, 3
	for i := 0; i < 3; i++ {
		calls, err := a.mu300PollRefreshCalls()
		if err != nil || len(calls) != 1 || calls[0].State != "alerting" {
			t.Fatalf("real modem voice-mode alerting row was filtered: calls=%v err=%v", calls, err)
		}
	}
	mode, state = 0, 0
	_, _ = a.mu300PollRefreshCalls()
	if a.calls.Active.ID != id || a.calls.Active.State != "active" || len(a.calls.History) != 0 {
		t.Fatal("mode transition ended or replaced the call")
	}
}

func TestMU300CLCCVoiceModesStayProfileScoped(t *testing.T) {
	for mode := 0; mode <= 9; mode++ {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			response := fmt.Sprintf("+CLCC: 1,1,4,%d,0,\"15550001111\",129\nOK", mode)
			a := testMU300Calls(t, func(string, time.Duration) (string, error) { return response, nil })
			calls, err := a.mu300RefreshCalls()
			wantVoice := mode == 0 || mode == 3 || mode == 4 || mode == 5
			if err != nil || (len(calls) == 1) != wantVoice {
				t.Fatalf("mode %d: calls=%v err=%v", mode, calls, err)
			}
			if (len(parseCLCC(response)) == 1) != (mode == 0) {
				t.Fatal("MU300 fix changed other platform parsing")
			}
		})
	}
}

func TestMU300AlertingGapKeepsOutgoingIdentity(t *testing.T) {
	a := testMU300Calls(t, nil)
	now := time.Now()
	setup := parsedCall{Index: 1, Direction: "outgoing", State: "alerting", Number: "15550001111"}
	a.mu300ApplyCallPoll([]parsedCall{setup}, now, true)
	id := a.calls.Active.ID
	a.mu300ApplyCallPoll(nil, now.Add(time.Second), true)
	a.mu300ApplyCallPoll(nil, now.Add(6*time.Second), true)
	if a.calls.Active == nil || a.calls.Active.ID != id || len(a.calls.History) != 0 {
		t.Fatal("waiting for peer answer was prematurely reported ended")
	}
	setup.State = "active"
	a.mu300ApplyCallPoll([]parsedCall{setup}, now.Add(8*time.Second), true)
	if a.calls.Active.ID != id || a.calls.Active.State != "active" {
		t.Fatal("answer changed outgoing identity")
	}
	// A real, sustained absence must still terminate; grace is not endless.
	a.mu300ApplyCallPoll(nil, now.Add(9*time.Second), true)
	a.mu300ApplyCallPoll(nil, now.Add(14*time.Second), true)
	if a.calls.Active != nil || len(a.calls.History) != 1 {
		t.Fatal("confirmed end was hidden")
	}
}

func TestMU300PollFailureBreaksAbsenceConfirmation(t *testing.T) {
	for _, response := range []string{"timeout", "+CLCC: malformed\nOK"} {
		t.Run(response, func(t *testing.T) {
			a := testMU300Calls(t, func(string, time.Duration) (string, error) {
				if response == "timeout" {
					return "", errors.New("timeout")
				}
				return response, nil
			})
			now := time.Now()
			a.mu300ApplyCallPoll([]parsedCall{{Index: 1, Direction: "outgoing", State: "alerting"}}, now, true)
			id := a.calls.Active.ID
			a.mu300ApplyCallPoll(nil, now.Add(time.Second), true)
			if _, e := a.mu300PollRefreshCalls(); e == nil {
				t.Fatal("invalid poll accepted")
			}
			a.mu300ApplyCallPoll(nil, now.Add(time.Minute), true)
			if a.calls.Active == nil || a.calls.Active.ID != id {
				t.Fatal("time without reliable evidence counted as a confirmed end")
			}
		})
	}
}

func TestMU300ControlDuringGapDoesNotEndOrRetarget(t *testing.T) {
	mutations := 0
	a := testMU300Calls(t, func(command string, _ time.Duration) (string, error) {
		if command == "AT+CLCC" {
			return "OK", nil
		}
		mutations++
		return "OK", nil
	})
	a.mu300ApplyCallPoll([]parsedCall{{Index: 1, Direction: "outgoing", State: "alerting"}}, time.Now(), true)
	id := a.calls.Active.ID
	if e := a.hangupCallForID(id); e == nil {
		t.Fatal("control accepted without a fresh target")
	}
	if mutations != 0 || a.calls.Active == nil || a.calls.Active.ID != id {
		t.Fatal("failed control falsely ended the waiting call")
	}
}

func TestMU300NewDialDuringGapCannotRedial(t *testing.T) {
	mutations := 0
	a := testMU300Calls(t, func(command string, _ time.Duration) (string, error) {
		if command == "AT+CLCC" {
			return "OK", nil
		}
		mutations++
		return "OK", nil
	})
	a.mu300ApplyCallPoll([]parsedCall{{Index: 1, Direction: "outgoing", State: "dialing"}}, time.Now(), true)
	id := a.calls.Active.ID
	if _, e := a.mu300Dial("15550002222", "new-operation", ""); e == nil {
		t.Fatal("second dial accepted")
	}
	if mutations != 0 || a.calls.Active == nil || a.calls.Active.ID != id {
		t.Fatal("one missing snapshot allowed redial")
	}
}

func TestMU300AnswerGapRetainsIdentity(t *testing.T) {
	writes, polls := 0, 0
	a := testMU300Calls(t, func(command string, _ time.Duration) (string, error) {
		switch command {
		case "AT+CLCC":
			if writes == 0 {
				return mu300TestWaiting + "OK", nil
			}
			polls++
			if polls == 1 {
				return "OK", nil
			}
			return mu300TestAnswered + "OK", nil
		case "ATA":
			writes++
			return "OK", nil
		default:
			t.Fatalf("unexpected command %s", command)
			return "", nil
		}
	})
	_, _ = a.mu300RefreshCalls()
	id := a.calls.Active.ID
	if err := a.answerCallForID(id); err != nil {
		t.Fatal(err)
	}
	if writes != 1 || a.calls.Active.ID != id || len(a.calls.History) != 0 {
		t.Fatal("answer gap changed the identity or repeated ATA")
	}
}

func TestMU300TargetedEndDoesNotRetireMissingPeer(t *testing.T) {
	ended := false
	a := testMU300Calls(t, func(command string, _ time.Duration) (string, error) {
		switch command {
		case "AT+CLCC":
			if ended {
				return "OK", nil
			}
			return mu300TestActive + mu300TestWaiting + "OK", nil
		case "AT+CHLD=?":
			return mu300TestCaps, nil
		case "AT+CHLD=0":
			ended = true
			return "OK", nil
		default:
			t.Fatalf("unexpected command %s", command)
			return "", nil
		}
	})
	_, _ = a.mu300RefreshCalls()
	peer, target := a.mu300Calls[1].ID, a.mu300Calls[2].ID
	if err := a.rejectCallForID(target); err != nil {
		t.Fatal(err)
	}
	if a.calls.Active == nil || a.calls.Active.ID != peer || len(a.calls.History) != 1 || a.calls.History[0].ID != target {
		t.Fatal("targeted reject also retired an unobserved peer")
	}
}

func TestMU300ControlRequiresFreshPeer(t *testing.T) {
	a := testMU300Calls(t, func(command string, _ time.Duration) (string, error) {
		if command != "AT+CLCC" {
			t.Fatalf("missing peer permitted modem command %s", command)
		}
		return mu300TestWaiting + "OK", nil
	})
	a.mu300ApplyCallPoll(parseCLCC(mu300TestActive+mu300TestWaiting), time.Now(), true)
	if err := a.answerCallForID(a.mu300Calls[2].ID); err == nil {
		t.Fatal("missing peer permitted single-call ATA")
	}
	if len(a.mu300Calls) != 2 || len(a.calls.History) != 0 {
		t.Fatal("preflight retired a missing peer")
	}
}
