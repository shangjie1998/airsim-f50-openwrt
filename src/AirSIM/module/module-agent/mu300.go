package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// mu300-atd remains the sole owner of the vendor tty. Never open it from Agent.
func (p *atPort) mu300Command(command string, timeout time.Duration) (string, error) {
	if strings.ContainsAny(command, "\r\n\x00") || !strings.HasPrefix(command, "AT") {
		return "", errors.New("invalid MU300 AT command")
	}
	seconds := int(timeout.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	if seconds > 99 {
		seconds = 99
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout+2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/mu300-at", "-t", strconv.Itoa(seconds), "-")
	cmd.Stdin = strings.NewReader(command + "\n")
	output, err := cmd.Output()
	text := normalizeATText(output)
	if err != nil || hasATError(text) || lastNonEmptyLine(text) != "OK" {
		p.recordFailure()
		return "", fmt.Errorf("MU300 AT request failed (%s)", safeMU300Verb(command))
	}
	p.recordSuccess()
	return text, nil
}

func safeMU300Verb(command string) string {
	for i, c := range command {
		if c == '=' || c == '?' || (i > 2 && c >= '0' && c <= '9') {
			return command[:i]
		}
	}
	return command
}

func (a *agent) mu300PollLoop() {
	// No QDC507 USB recovery, network configuration, vendor resets or SMS-mode
	// writes: the OpenWrt radio and SMS daemons own those resources.
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	n := 0
	for {
		a.mu300CallMu.Lock()
		_, err := a.mu300PollRefreshCalls()
		a.mu300CallMu.Unlock()
		a.setCallError(err)
		if n%30 == 0 && a.mu300MetadataIdle() {
			a.mu300RefreshModem()
		}
		n++
		<-ticker.C
	}
}

func (a *agent) mu300AT(command string, timeout time.Duration) (string, error) {
	if a.callControlCommand != nil {
		return a.callControlCommand(command, timeout)
	}
	return a.at.command(command, timeout)
}

// The caller holds mu300CallMu across status verification and its AT command.
func (a *agent) mu300RefreshCalls() ([]parsedCall, error) {
	return a.mu300RefreshCallsMode(true)
}

// mu300PollRefreshCalls feeds the background poller. It refuses to call a call
// gone on the strength of a single CLCC snapshot: the shared AT channel can
// answer slowly, truncated or interleaved with another client, and the client
// would take one such snapshot for a hangup.
func (a *agent) mu300PollRefreshCalls() ([]parsedCall, error) {
	return a.mu300RefreshCallsMode(true)
}

func (a *agent) mu300RefreshCallsMode(confirmGone bool) (result []parsedCall, err error) {
	defer func() {
		if err != nil {
			// Unknown time is not consecutive evidence that a call ended.
			a.mu.Lock()
			a.mu300GoneSince = nil
			a.mu300EarlyCall = ""
			a.mu300EarlyFreshCall = ""
			a.mu300EarlyPollAt = time.Time{}
			a.mu.Unlock()
		}
	}()
	_ = a.mu300FreshTerminalHints()
	response, err := a.mu300AT("AT+CLCC", 2*time.Second)
	if err != nil {
		return nil, err
	}
	calls := parseVoiceCLCC(response, true)
	// A malformed/duplicate slot is not an idle modem. Keep the previous
	// identities and refuse control instead of assigning a new/wrong target.
	seen := make(map[int]bool)
	for _, line := range strings.Split(response, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "+CLCC:") && !clccPattern.MatchString(line) {
			return nil, errors.New("基带通话列表格式异常，未更新通话身份")
		}
	}
	for _, c := range calls {
		if c.Index < 1 || c.Index > 7 || seen[c.Index] || c.State == "unknown" {
			return nil, errors.New("基带通话列表包含重复或无效编号，未执行控制")
		}
		seen[c.Index] = true
	}
	// Read events that arrived while the native CLCC query was in flight too.
	confirmedEnds := a.mu300FreshTerminalHints()
	a.mu300ApplyCallPollWithEnds(calls, time.Now(), confirmGone, confirmedEnds)
	a.mu.Lock()
	a.mu300EarlyFreshCall = ""
	a.mu300EarlyPollAt = time.Now()
	if len(calls) == 1 && calls[0].Direction == "outgoing" && (calls[0].State == "dialing" || calls[0].State == "alerting") {
		if c, ok := a.mu300Calls[calls[0].Index]; ok {
			a.mu300EarlyFreshCall = c.ID
		}
	}
	if a.mu300EarlyCall != a.mu300EarlyFreshCall {
		a.mu300EarlyCall = ""
	}
	a.mu.Unlock()
	return calls, nil
}

func (a *agent) mu300Dial(raw, operation, simAccount string) (result map[string]any, err error) {
	return a.mu300DialFor("local", raw, operation, simAccount)
}

func (a *agent) mu300DialFor(owner, raw, operation, simAccount string) (result map[string]any, err error) {
	number := normalizeDialNumber(raw)
	if number == "" || len(number) > 82 {
		return nil, errors.New("号码为空、过长或包含非法字符")
	}
	if simAccount != "" {
		return nil, errors.New("MU300 仅支持当前物理 SIM，未提供 SIM 切换")
	}
	if operation == "" || len(operation) > 128 || strings.ContainsAny(operation, "\r\n\x00") {
		return nil, errors.New("无效拨号操作 ID")
	}
	a.mu300CallMu.Lock()
	defer a.mu300CallMu.Unlock()
	submitted := false
	if operation != "" {
		if a.mu300Operations == nil {
			return nil, errors.New("持久拨号回执未初始化，未发起电话")
		}
		previous, exists, e := a.mu300Operations.begin(owner, operation, number, simAccount)
		if e != nil {
			return nil, e
		}
		if exists {
			return previous.receipt(operation)
		}
		defer func() {
			var id string
			if result != nil {
				id, _ = result["backend_id"].(string)
			}
			if e := a.mu300Operations.finish(owner, operation, id, submitted); e != nil {
				result, err = nil, e
			}
		}()
	}
	_, err = a.mu300RefreshCalls()
	if err != nil {
		return nil, err
	}
	a.mu.RLock()
	hasCalls := len(a.mu300Calls) != 0
	a.mu.RUnlock()
	if hasCalls {
		return nil, errors.New("MU300 当前已有通话")
	}
	a.mu.Lock()
	a.calls.RecentlyEnded = nil
	a.mu.Unlock()
	// ATD is sent once. A timeout is ambiguous, so confirm using CLCC rather
	// than retransmitting the command. Confirmation has to survive a busy AT
	// channel: another client (the panel, the audio follower) may hold
	// mu300-at's lock, and a single slow answer must not turn a placed call
	// into a failure the client shows as "back to the dial pad". The budget
	// stays below the client's 15 s dial timeout.
	window := mu300DialConfirmationWindow
	if a.mu300DialWindow > 0 {
		window = a.mu300DialWindow
	}
	deadline := time.Now().Add(window)
	submitted = true
	log.Print("MU300 dial: attempting ATD once")
	_, dialErr := a.mu300AT("ATD"+number+";", 6*time.Second)
	for {
		calls, pollErr := a.mu300RefreshCalls()
		if pollErr == nil {
			if matched, ok := mu300DialedCall(calls, number); ok {
				a.mu.RLock()
				owned, exists := a.mu300Calls[matched.Index]
				var id string
				if exists && owned.Direction == "outgoing" {
					id = owned.ID
				}
				a.mu.RUnlock()
				if id != "" {
					return map[string]any{"dialing": true, "operation_id": operation, "android_id": id, "backend_id": id}, nil
				}
			}
		}
		if !time.Now().Before(deadline) {
			break
		}
		if a.callControlSleep != nil {
			a.callControlSleep(200 * time.Millisecond)
		} else {
			time.Sleep(200 * time.Millisecond)
		}
	}
	if dialErr != nil {
		return nil, dialErr
	}
	return nil, errors.New("拨号已发送，但尚未确认通话；请查询状态，勿重复拨号")
}

const mu300DialConfirmationWindow = 10 * time.Second

// mu300DialedCall finds the one outgoing call that is the number just dialled.
// The network may rewrite that number (a country code added or dropped) or
// report none at all while the call is still alerting, so the dialled digits
// are matched as a suffix and an empty CLCC number is accepted.
func mu300DialedCall(calls []parsedCall, number string) (parsedCall, bool) {
	var found parsedCall
	matches := 0
	for _, call := range calls {
		if call.Direction != "outgoing" || !mu300DialedNumberMatches(call.Number, number) {
			continue
		}
		found = call
		matches++
	}
	return found, matches == 1
}

func mu300DialedNumberMatches(reported, dialed string) bool {
	trimmed := normalizeDialNumber(reported)
	if trimmed == "" || dialed == "" {
		return true
	}
	left, right := strings.TrimPrefix(trimmed, "+"), strings.TrimPrefix(dialed, "+")
	if left == right {
		return true
	}
	if len(left) > len(right) {
		left, right = right, left
	}
	return len(left) >= 6 && strings.HasSuffix(right, left)
}

func (a *agent) mu300MetadataIdle() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.mu300Calls) == 0
}

func (a *agent) mu300RefreshModem() {
	if !a.mu300MetadataIdle() {
		return
	}
	cpin, pinErr := a.at.command("AT+CPIN?", 3*time.Second)
	if !a.mu300MetadataIdle() {
		return
	}
	reg, regErr := a.at.command("AT+CEREG?", 3*time.Second)
	if !a.mu300MetadataIdle() {
		return
	}
	csq, csqErr := a.at.command("AT+CSQ", 3*time.Second)
	if !a.mu300MetadataIdle() {
		return
	}
	_, _ = a.mu300RefreshNumber()
	a.mu.Lock()
	defer a.mu.Unlock()
	if pinErr == nil {
		a.modem.SIMInserted = strings.Contains(cpin, "READY")
	}
	if regErr == nil {
		a.modem.RegistrationText = registrationText(parseRegistration(reg))
	}
	if csqErr == nil {
		a.modem.SignalDBM = parseSignalDBM(csq)
	}
}

func (a *agent) mu300RefreshNumber() (string, error) {
	result, err := a.mu300AT("AT+CNUM", 3*time.Second)
	number := ""
	status := "unavailable"
	if err == nil {
		number = parseCNUM(result)
		if number != "" {
			status = "available"
		} else {
			status = "not_provided"
		}
	}
	a.mu.Lock()
	a.modem.PhoneNumber, a.modem.PhoneNumberStatus = number, status
	a.modem.PhoneNumberUpdatedAt = time.Now().UTC().Format(time.RFC3339)
	a.mu.Unlock()
	return number, err
}
