package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const resolveTestUUID = "891eae43-89f7-4baa-a21e-e15beecbe584"

func TestMU300ResolveRequiresExactLivePushTuple(t *testing.T) {
	for _, scenario := range []string{"incoming", "active", "held", "wrong-id", "wrong-uuid", "old-generation", "restart", "missing", "poll-error", "slot-reuse", "outgoing", "other-priority", "integer-generation"} {
		t.Run(scenario, func(t *testing.T) {
			state := "+CLCC: 1,1,4,0,0\nOK"
			a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
				if cmd != "AT+CLCC" {
					t.Errorf("unexpected modem mutation %s", cmd)
				}
				if scenario == "poll-error" {
					return "", errors.New("fixture unavailable")
				}
				return state, nil
			})
			a.applyCallPoll(parseCLCC(state), time.Now())
			id := a.calls.Active.ID
			a.push.mu.Lock()
			a.push.lastCallID, a.push.lastUUID, a.push.lastGeneration = id, resolveTestUUID, 7
			a.push.mu.Unlock()
			wantedID, uuid, generation := id, resolveTestUUID, "7"
			want := 409
			switch scenario {
			case "integer-generation":
				want = 200
			case "incoming":
				want = 200
			case "active":
				state = "+CLCC: 1,1,0,0,0\nOK"
				want = 200
			case "held":
				state = "+CLCC: 1,1,1,0,0\nOK"
				want = 200
			case "wrong-id":
				wantedID = "mu300-another"
			case "wrong-uuid":
				uuid = "00000000-0000-0000-0000-000000000000"
			case "old-generation":
				generation = "6"
			case "restart":
				a.push = newPushManager(t.TempDir() + "/push.json")
			case "missing":
				state = "OK"
			case "slot-reuse":
				a.applyCallPoll(parseCLCC("+CLCC: 1,1,0,0,0\nOK"), time.Now())
				a.applyCallPoll(parseCLCC(state), time.Now())
			case "outgoing":
				state = "+CLCC: 1,0,0,0,0\nOK"
			case "other-priority":
				state = "+CLCC: 1,1,1,0,0\n+CLCC: 2,1,5,0,0\nOK"
				want = 200
			}
			w := httptest.NewRecorder()
			body := fmt.Sprintf(`{"call_id":%q,"call_uuid":%q,"generation":%q}`, wantedID, uuid, generation)
			if scenario == "integer-generation" {
				body = strings.Replace(body, `"generation":"7"`, `"generation":7`, 1)
			}
			a.mu300ResolvePushedCall(w, httptest.NewRequest("POST", "/api/calls/resolve", strings.NewReader(body)))
			if w.Code != want {
				t.Fatalf("got %d want %d: %s", w.Code, want, w.Body.String())
			}
			if want == 200 {
				var result callStatusPayload
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Active == nil || result.Active.ID != id || result.Active.AndroidID != id {
					t.Fatal("not real target")
				}
				if scenario == "other-priority" && a.calls.Active.ID == id {
					t.Fatal("resolve changed tracker projection")
				}
			}
		})
	}
}

func TestMU300ResolveStrictInput(t *testing.T) {
	a := testMU300Calls(t, func(string, time.Duration) (string, error) { t.Fatal("invalid tuple queried modem"); return "", nil })

	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{}`, 409}, {`null`, 409},
		{`{"call_id":"x","call_uuid":"bad","generation":"1"}`, 409},
		{`{"call_id":"x","call_uuid":"` + resolveTestUUID + `","generation":null}`, 409},
		{`{"call_id":"x","call_uuid":"` + resolveTestUUID + `","generation":true}`, 409},
		{`{"call_id":"x","call_uuid":"` + resolveTestUUID + `","generation":1.5}`, 409},
		{`{"call_id":"x","call_uuid":"` + resolveTestUUID + `","generation":"0"}`, 409},
		{`{"call_id":"x","call_uuid":"` + resolveTestUUID + `","generation":"-1"}`, 409},
		{`{"call_id":"x","call_uuid":"` + resolveTestUUID + `","generation":"bad"}`, 409},
		{`{"call_id":"x","call_uuid":"` + resolveTestUUID + `","generation":"9223372036854775808"}`, 409},
		{`{"call_id":"x","call_uuid":"` + resolveTestUUID + `","generation":"1","number":"fixture"}`, 400},
		{`{"call_id":"x","call_uuid":"` + resolveTestUUID + `","generation":"1"} {}`, 400},
	} {
		w := httptest.NewRecorder()
		a.mu300ResolvePushedCall(w, httptest.NewRequest("POST", "/api/calls/resolve", strings.NewReader(tc.body)))
		if w.Code != tc.status {
			t.Fatalf("invalid body returned %d want %d", w.Code, tc.status)
		}
	}

	w := httptest.NewRecorder()
	a.mu300ResolvePushedCall(w, httptest.NewRequest("GET", "/api/calls/resolve", nil))
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
}

func TestMU300RepeatedAnswerHeldDoesNotSwapOrRouteAudio(t *testing.T) {
	a := testMU300Calls(t, func(cmd string, _ time.Duration) (string, error) {
		if cmd != "AT+CLCC" {
			t.Fatalf("repeat answer issued %s", cmd)
		}
		return mu300TestHeld + mu300TestAnswered + "OK", nil
	})
	if _, err := a.mu300RefreshCalls(); err != nil {
		t.Fatal(err)
	}
	id := a.mu300Calls[1].ID
	for i := 0; i < 2; i++ {
		if err := a.answerCallForID(id); err != nil {
			t.Fatal(err)
		}
	}
	if a.mu300Calls[1].State != "held" {
		t.Fatal("held call changed")
	}
}
