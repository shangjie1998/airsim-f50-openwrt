package main

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CLCC indexes are reused by the modem. A UUID belongs to the lifetime of a
// slot, NOT to whichever call currently wins the legacy Active projection.
func (a *agent) mu300LiveCallsLocked() []callRecord {
	var result []callRecord
	for _, c := range a.mu300Calls {
		result = append(result, c)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Index < result[j].Index })
	return result
}

// A single CLCC snapshot that does not list a live call is not proof that the
// call ended: the shared AT channel can answer late, truncated or interleaved
// with another client's request.
//
// Keep setup resilient to genuinely missing snapshots. The observed MU300
// dialing-to-alerting transition uses CLCC mode 3: it must be parsed as voice,
// not hidden by extending this grace period. A stable alerting row can remain
// live indefinitely; this budget applies only when a row is actually absent.
const (
	mu300CallSetupGrace = 10 * time.Second
	mu300CallEndGrace   = 4 * time.Second
)

func mu300EndGraceFor(state string) time.Duration {
	if state == "dialing" || state == "alerting" {
		return mu300CallSetupGrace
	}
	return mu300CallEndGrace
}

func (a *agent) mu300ApplyCallPoll(calls []parsedCall, now time.Time, confirmGone bool) {
	a.mu300ApplyCallPollWithEnd(calls, now, confirmGone, "")
}

// confirmedEnd is used only after an acknowledged targeted hangup/reject and
// a fresh snapshot missing that slot. Other missing calls retain their grace.
func (a *agent) mu300ApplyCallPollWithEnd(calls []parsedCall, now time.Time, confirmGone bool, confirmedEnd string) {
	a.mu300ApplyCallPollWithEnds(calls, now, confirmGone, map[string]bool{confirmedEnd: true})
}

func (a *agent) mu300ApplyCallPollWithEnds(calls []parsedCall, now time.Time, confirmGone bool, confirmedEnds map[string]bool) {
	a.mu.Lock()
	next := make(map[int]callRecord, len(calls))
	var changed, ended []callRecord
	for _, p := range calls {
		old, exists := a.mu300Calls[p.Index]
		// A connected slot returning to setup, or changing direction, is a
		// replacement even if an idle poll was missed. Never reuse its UUID.
		replaced := exists && (old.Direction != p.Direction ||
			(old.Number != "" && p.Number != "" && normalizeDialNumber(old.Number) != normalizeDialNumber(p.Number)) ||
			((old.State == "active" || old.State == "held") &&
				(p.State == "incoming" || p.State == "waiting" || p.State == "dialing" || p.State == "alerting")))
		if !exists || replaced {
			uuid, err := randomUUID()
			if err != nil {
				a.calls.LastPollError = "cannot allocate MU300 call identity"
				a.mu.Unlock()
				return
			}
			old = callRecord{ID: "mu300-" + uuid, AndroidID: "mu300-" + uuid,
				Index: p.Index, Direction: p.Direction, Number: p.Number, StartedAt: now}
		}
		if old.State != p.State || (p.Number != "" && old.Number != p.Number) {
			old.State, old.UpdatedAt = p.State, now
			if p.Number != "" {
				old.Number = p.Number
			}
			changed = append(changed, old)
		}
		delete(a.mu300GoneSince, old.ID)
		next[p.Index] = old
	}
	for index, old := range a.mu300Calls {
		if c, exists := next[index]; exists && c.ID == old.ID {
			continue
		}
		if _, exists := next[index]; exists {
			// The slot now carries another identity: the previous call is over
			// (redial, direction change, replacement). Never reuse its UUID.
			delete(a.mu300GoneSince, old.ID)
			old.EndedAt, old.UpdatedAt = &now, now
			old.Missed = old.Direction == "incoming" && (old.State == "incoming" || old.State == "waiting")
			old.State = "ended"
			ended = append(ended, old)
			continue
		}
		if confirmGone && !confirmedEnds[old.ID] {
			since, pending := a.mu300GoneSince[old.ID]
			if !pending {
				if a.mu300GoneSince == nil {
					a.mu300GoneSince = make(map[string]time.Time)
				}
				a.mu300GoneSince[old.ID] = now
				next[index] = old
				continue
			}
			if now.Sub(since) < mu300EndGraceFor(old.State) {
				next[index] = old
				continue
			}
		}
		delete(a.mu300GoneSince, old.ID)
		old.EndedAt, old.UpdatedAt = &now, now
		old.Missed = old.Direction == "incoming" && (old.State == "incoming" || old.State == "waiting")
		old.State = "ended"
		ended = append(ended, old)
	}
	if len(ended) > 0 {
		log.Print("MU300 call: terminal state published")
	}
	a.mu300Calls = next
	// Stable tie breaking, independent of modem response order. Keep the old
	// single-call wire contract; the additive calls array is for diagnostics.
	a.calls.Active = nil
	for _, c := range a.mu300LiveCallsLocked() {
		if a.calls.Active == nil || callStatePriority(c.State) > callStatePriority(a.calls.Active.State) {
			copy := c
			a.calls.Active = &copy
		}
	}
	if len(ended) > 0 {
		a.calls.History = append(ended, a.calls.History...)
	}
	idle := len(next) == 0
	if idle {
		a.muted, a.isRecording = false, false
	}
	if len(changed)+len(ended) > 0 {
		a.notifyCallChangedLocked()
	}
	a.mu.Unlock()
	if a.mu300Operations != nil {
		a.mu300Operations.ended(ended)
	}
	for _, c := range ended {
		if a.push != nil {
			go func(id string) { _ = a.push.sendCallState(id, "ended") }(c.ID)
		}
	}
	for _, c := range changed {
		if c.Direction == "incoming" && (c.State == "incoming" || c.State == "waiting") {
			a.scheduleIncomingPush(c.ID)
		}
		if a.push != nil {
			if phase := lifecyclePhaseForModemState(c.State); phase != "" {
				go func(id, phase string) { _ = a.push.sendCallState(id, phase) }(c.ID, phase)
			}
		}
		if c.State == "active" {
			go a.ensureVoiceRouteIfHostEnabled()
		}
	}
	if idle && len(ended) > 0 {
		go func() {
			time.Sleep(1500 * time.Millisecond)
			a.mu.RLock()
			stillIdle := len(a.mu300Calls) == 0
			a.mu.RUnlock()
			if stillIdle {
				a.stopVoiceRoute()
			}
		}()
	}
}

func (a *agent) mu300Target(id string) (callRecord, []callRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	calls := a.mu300LiveCallsLocked()
	if len(calls) == 0 {
		return callRecord{}, calls, errors.New("当前没有通话；来电可能已经结束")
	}
	if id == "" {
		if len(calls) != 1 {
			return callRecord{}, calls, errors.New("多路通话必须指定 call_id，未执行操作")
		}
		return calls[0], calls, nil
	}
	for _, c := range calls {
		if c.ID == id {
			return c, calls, nil
		}
	}
	return callRecord{}, calls, errors.New("通话 ID 已失效，未操作其他通话")
}

// Support both standard (0,1,1x,2,2x) and this modem's numeric ranges
// (0-4,6,7,8,11-17,21-27,71-77). Never infer capability from substrings.
func mu300SupportsCHLD(response, operation string) bool {
	want, err := strconv.Atoi(operation)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "+CHLD:") {
			continue
		}
		list := strings.TrimSpace(strings.TrimPrefix(line, "+CHLD:"))
		if !strings.HasPrefix(list, "(") || !strings.HasSuffix(list, ")") {
			continue
		}
		for _, raw := range strings.Split(list[1:len(list)-1], ",") {
			token := strings.ToLower(strings.TrimSpace(raw))
			if token == operation {
				return true
			}
			if len(operation) == 2 && operation[1] >= '1' && operation[1] <= '7' && token == operation[:1]+"x" {
				return true
			}
			bounds := strings.Split(token, "-")
			if len(bounds) == 2 {
				lo, e1 := strconv.Atoi(bounds[0])
				hi, e2 := strconv.Atoi(bounds[1])
				if e1 == nil && e2 == nil && lo <= want && want <= hi {
					return true
				}
			}
		}
	}
	return false
}

func mu300ControlCommand(action, digit string, target callRecord, calls []callRecord) (string, error) {
	switch action {
	case "answer":
		if target.State == "active" || target.State == "held" {
			return "", nil
		} // repeated answer must never toggle hold
		if target.State != "incoming" && target.State != "waiting" {
			return "", errors.New("目标通话当前不可接听")
		}
		if len(calls) == 1 {
			return "ATA", nil
		}
		// CHLD=2 is unambiguous only with one waiting call and one active
		// call. Do not swap held calls or answer another waiting party.
		if len(calls) == 2 && target.State == "waiting" {
			for _, c := range calls {
				if c.ID != target.ID && c.State == "active" {
					return "AT+CHLD=2", nil
				}
			}
		}
		return "", errors.New("当前通话组合无法安全接听；仅支持一路通话加一路等待来电")
	case "reject", "hangup":
		if action == "reject" && target.State != "incoming" && target.State != "waiting" {
			return "", errors.New("目标通话已接通，不能作为来电拒接")
		}
		if len(calls) == 1 {
			return "AT+CHUP", nil
		}
		if target.State == "waiting" && len(calls) == 2 {
			for _, c := range calls {
				if c.ID != target.ID && c.State == "active" {
					return "AT+CHLD=0", nil
				}
			}
		}
		if target.State == "active" && target.Index >= 1 && target.Index <= 7 {
			return "AT+CHLD=1" + strconv.Itoa(target.Index), nil
		}
		return "", errors.New("当前通话组合不支持安全定向挂断，未挂断其他通话")
	case "dtmf":
		active := 0
		for _, c := range calls {
			if c.State == "active" {
				active++
			}
		}
		if target.State != "active" || active != 1 {
			return "", errors.New("DTMF 只能发送到唯一已接通的通话")
		}
		return `AT+VTS="` + digit + `"`, nil
	}
	return "", errors.New("unknown MU300 call action")
}

// Retaining an identity across a gap must not authorize a modem mutation.
// Require every retained slot in the fresh snapshot, including peer calls.
func (a *agent) mu300ControlSnapshot() error {
	calls, err := a.mu300RefreshCalls()
	if err != nil {
		return err
	}
	a.mu.RLock()
	complete := len(calls) == len(a.mu300Calls)
	a.mu.RUnlock()
	if !complete {
		return errors.New("基带通话状态暂未完整确认，请等待刷新后重试；未发送控制命令")
	}
	return nil
}

func (a *agent) mu300Control(action, expectedID, digit string) error {
	a.mu300CallMu.Lock()
	defer a.mu300CallMu.Unlock()
	if err := a.mu300ControlSnapshot(); err != nil {
		return err
	}
	if (action == "hangup" || action == "reject") && a.mu300KnownEndedWhileIdle(expectedID) {
		return nil
	}
	target, calls, err := a.mu300Target(expectedID)
	if err != nil {
		return err
	}
	for id := range a.mu300Uncertain {
		if _, _, err := a.mu300Target(id); err != nil {
			delete(a.mu300Uncertain, id)
		}
	}
	command, err := mu300ControlCommand(action, digit, target, calls)
	if err != nil || command == "" {
		if err == nil {
			delete(a.mu300Uncertain, target.ID)
		}
		return err
	}
	if command == "AT+CHLD=2" && a.mu300Uncertain[target.ID] {
		return errors.New("上次接听结果尚未确认，为避免交换通话未重复发送；请等待状态更新")
	}
	if strings.HasPrefix(command, "AT+CHLD=") {
		capabilities, err := a.mu300AT("AT+CHLD=?", 3*time.Second)
		if err != nil {
			return fmt.Errorf("无法确认基带呼叫保持能力，未执行操作: %w", err)
		}
		if !mu300SupportsCHLD(capabilities, strings.TrimPrefix(command, "AT+CHLD=")) {
			return errors.New("基带未声明支持该定向通话操作，未执行操作")
		}
		// The capability query takes time. Re-read CLCC and bind the action
		// to the same UUID and topology before any state-changing command.
		if err = a.mu300ControlSnapshot(); err != nil {
			return err
		}
		fresh, peers, err := a.mu300Target(target.ID)
		if err != nil {
			return err
		}
		next, err := mu300ControlCommand(action, digit, fresh, peers)
		if err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		if next != command || !mu300SamePeers(calls, peers) {
			return errors.New("通话状态已变化，请重新操作；未发送控制命令")
		}
	}
	// Send at most once, even if AT times out. Only a confirmed CLCC result
	// is success. In particular, never retry CHLD=2 (it would swap calls).
	_, commandErr := a.mu300AT(command, 5*time.Second)
	if command == "AT+CHLD=2" {
		if a.mu300Uncertain == nil {
			a.mu300Uncertain = make(map[string]bool)
		}
		a.mu300Uncertain[target.ID] = true
	}
	if action == "dtmf" {
		return commandErr
	}
	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			if a.callControlSleep != nil {
				a.callControlSleep(200 * time.Millisecond)
			} else {
				time.Sleep(200 * time.Millisecond)
			}
		}
		fresh, err := a.mu300RefreshCalls()
		if err != nil {
			continue
		}
		observed := false
		for _, c := range fresh {
			if c.Index == target.Index {
				observed = true
			}
		}
		if !observed && commandErr == nil && (action == "hangup" || action == "reject") {
			a.mu300ApplyCallPollWithEnd(fresh, time.Now(), true, target.ID)
		}
		current, _, lookupErr := a.mu300Target(target.ID)
		if action == "answer" {
			if observed && lookupErr == nil && current.State == "active" {
				delete(a.mu300Uncertain, target.ID)
				return nil
			}
			if lookupErr != nil {
				return errors.New("接听期间目标来电已结束，未接听其他通话")
			}
		} else if lookupErr != nil {
			return nil
		}
	}
	if commandErr != nil {
		return fmt.Errorf("通话操作结果未确认，请刷新状态，勿重复操作: %w", commandErr)
	}
	return errors.New("基带已接受命令，但通话状态尚未确认；请刷新状态，勿重复操作")
}

func mu300SamePeers(a, b []callRecord) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].State != b[i].State {
			return false
		}
	}
	return true
}

// A delayed user hangup of an already-confirmed terminal UUID is successful
// without a modem command. Unknown IDs or any live replacement stay rejected.
func (a *agent) mu300KnownEndedWhileIdle(id string) bool {
	if id == "" {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if len(a.mu300Calls) != 0 {
		return false
	}
	for _, c := range a.calls.History {
		if c.ID == id && c.EndedAt != nil {
			return true
		}
	}
	return false
}
