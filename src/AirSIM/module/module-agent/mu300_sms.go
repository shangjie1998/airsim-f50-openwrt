package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Only read the native daemon's atomically written pool. In particular, do not
// use `show` (marks read), sync, CMGF/CPMS or delete the SIM's/pool's messages.
type mu300SMSBackend struct {
	mu          sync.Mutex
	sendMu      sync.Mutex
	pool, state string
	seen        map[string]bool
	initialized bool
	runSend     func(context.Context, string) (string, error)
}

var mu300PoolID = regexp.MustCompile(`^[0-9]{1,12}$`)
var mu300SMSPhone = regexp.MustCompile(`^\+?[0-9]{1,20}$`)
var mu300SentReply = regexp.MustCompile(`^sent \(reference [0-9]+\)$`)

func newMU300SMSBackend(dataDir string) *mu300SMSBackend {
	return &mu300SMSBackend{pool: "/etc/mu300/sms-pool/msg", state: filepath.Join(dataDir, "mu300-sms"), runSend: runMU300SMSSend}
}

func readMU300PrivateFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("invalid SMS file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return nil, errors.New("SMS file changed; retry later")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, errors.New("SMS file exceeds limit")
	}
	return b, nil
}

func parseMU300PoolMessage(name string, data []byte) (storedSMS, bool, error) {
	bad := errors.New("invalid native SMS pool entry")
	header, body, ok := bytes.Cut(data, []byte("\n\n"))
	if !ok || !utf8.Valid(data) {
		return storedSMS{}, false, bad
	}
	h := map[string]string{}
	for _, line := range strings.Split(string(header), "\n") {
		k, v, valid := strings.Cut(line, ": ")
		if !valid || h[k] != "" {
			return storedSMS{}, false, bad
		}
		h[k] = v
	}
	if h["dir"] == "mo" {
		return storedSMS{}, false, nil
	}
	if h["dir"] != "mt" || h["id"] != name || h["from"] == "" || len(h["from"]) > 256 {
		return storedSMS{}, false, bad
	}
	n, err := strconv.Atoi(h["length"])
	if err != nil || n < 1 || n > 65536 || len(body) != n+1 || body[n] != '\n' {
		return storedSMS{}, false, bad
	}
	stamp, err := strconv.ParseInt(h["received"], 10, 64)
	if err != nil || stamp <= 0 || stamp > 4102444800 {
		return storedSMS{}, false, bad
	}
	content := string(body[:n])
	// The native codec drops SCTS timezone. Use the unambiguous persisted receipt
	// epoch instead of interpreting the sender's wall clock as UTC.
	identity, _ := json.Marshal([]string{name, h["from"], h["scts"], h["received"], content})
	digest := sha256.Sum256(identity)
	message := smsMessage{SIMSlot: "SIM 1", Sender: h["from"], Content: content, Timestamp: time.Unix(stamp, 0).UTC(), DeliveryID: "mu300-" + hex.EncodeToString(digest[:])}
	if code := verificationCodePattern.FindStringSubmatch(content); len(code) > 1 {
		message.Code = code[1]
	}
	return storedSMS{Memory: "MU300_POOL", Message: message}, true, nil
}

func (b *mu300SMSBackend) snapshot() ([]storedSMS, error) {
	d, err := os.Open(b.pool)
	if err != nil {
		return nil, errors.New("native SMS pool unavailable")
	}
	defer d.Close()
	entries, err := d.ReadDir(10001)
	if (err != nil && !errors.Is(err, io.EOF)) || len(entries) > 10000 {
		return nil, errors.New("native SMS pool exceeds scan limit")
	}
	items := []storedSMS{}
	total := 0
	for _, entry := range entries {
		if !mu300PoolID.MatchString(entry.Name()) {
			continue
		}
		data, err := readMU300PrivateFile(filepath.Join(b.pool, entry.Name()), 69632)
		if errors.Is(err, os.ErrNotExist) {
			continue
		} // native deletion during scan
		if err != nil {
			return nil, errors.New("cannot read native SMS pool")
		}
		total += len(data)
		if total > 16<<20 {
			return nil, errors.New("native SMS pool exceeds scan budget")
		}
		item, incoming, err := parseMU300PoolMessage(entry.Name(), data)
		if err != nil {
			return nil, err
		}
		if incoming {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Message.Timestamp.Equal(items[j].Message.Timestamp) {
			return items[i].Message.DeliveryID < items[j].Message.DeliveryID
		}
		return items[i].Message.Timestamp.After(items[j].Message.Timestamp)
	})
	return items, nil
}

func (b *mu300SMSBackend) loadAcks() (map[string]bool, error) {
	ack := map[string]bool{}
	data, err := readMU300PrivateFile(filepath.Join(b.state, "acks.json"), 8<<20)
	if errors.Is(err, os.ErrNotExist) {
		return ack, nil
	}
	if err != nil || json.Unmarshal(data, &ack) != nil || ack == nil || len(ack) > 50000 {
		return nil, errors.New("SMS delivery state unavailable; preserved for recovery")
	}
	return ack, nil
}

func (b *mu300SMSBackend) save(name string, value any) error {
	if err := os.MkdirAll(b.state, 0700); err != nil {
		return errors.New("cannot save SMS state")
	}
	info, err := os.Lstat(b.state)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("invalid SMS state directory")
	}
	if err := os.Chmod(b.state, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(b.state, ".state-*")
	if err != nil {
		return errors.New("cannot save SMS state")
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), filepath.Join(b.state, name))
	}
	if err == nil {
		d, openErr := os.Open(b.state)
		if openErr != nil {
			return openErr
		}
		err = d.Sync()
		d.Close()
	}
	if err != nil {
		return errors.New("cannot durably save SMS state")
	}
	return nil
}

func (a *agent) mu300RefreshSMS() {
	b := a.mu300SMS
	if b == nil {
		a.setSMSError(errors.New("native SMS backend unavailable"))
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	acks, err := b.loadAcks()
	if err != nil {
		a.setSMSError(err)
		return
	}
	items, err := b.snapshot()
	if err != nil {
		a.setSMSError(err)
		return
	}
	seen := make(map[string]bool, len(items))
	pending := []storedSMS{}
	push := []smsMessage{}
	for _, item := range items {
		id := item.Message.DeliveryID
		seen[id] = true
		if acks[id] {
			continue
		}
		if len(pending) < 256 {
			pending = append(pending, item)
		}
		// Do not flood an existing Relay registration with historical inbox items
		// on initial startup. HTTP sync still delivers that history to the phone.
		if b.initialized && !b.seen[id] {
			push = append(push, item.Message)
		}
	}
	b.seen, b.initialized = seen, true
	a.mu.Lock()
	changed := len(a.messages) != len(pending)
	if !changed {
		for i := range pending {
			if a.messages[i].Message.DeliveryID != pending[i].Message.DeliveryID {
				changed = true
				break
			}
		}
	}
	a.messages, a.smsError = pending, ""
	if changed {
		a.smsRevision++
		a.notifyAgentEventChangedLocked()
	}
	a.mu.Unlock()
	for _, message := range push {
		a.scheduleSMSPush(message)
	}
}

func (a *agent) mu300SMSPollLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		a.mu300RefreshSMS()
		<-ticker.C
	}
}

func (a *agent) mu300AckSMS(ids []string) (int, error) {
	b := a.mu300SMS
	if b == nil || len(ids) > 256 {
		return 0, errors.New("invalid SMS acknowledgement")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	acks, err := b.loadAcks()
	if err != nil {
		return 0, err
	}
	requested := map[string]bool{}
	for _, id := range ids {
		requested[id] = true
	}
	a.mu.RLock()
	pending := append([]storedSMS(nil), a.messages...)
	a.mu.RUnlock()
	n := 0
	for _, item := range pending {
		id := item.Message.DeliveryID
		if requested[id] {
			if !acks[id] {
				n++
			}
			acks[id] = true
		}
	}
	if len(acks) > 50000 {
		return 0, errors.New("SMS acknowledgement storage limit reached")
	}
	if n == 0 {
		return 0, nil
	}
	if err := b.save("acks.json", acks); err != nil {
		return 0, err
	}
	a.mu.Lock()
	remaining := a.messages[:0]
	for _, item := range a.messages {
		if !acks[item.Message.DeliveryID] {
			remaining = append(remaining, item)
		}
	}
	a.messages = remaining
	a.smsRevision++
	a.notifyAgentEventChangedLocked()
	a.mu.Unlock()
	return n, nil
}

type mu300SMSSendState struct {
	Fingerprint string `json:"fingerprint"`
	Started     int64  `json:"started"`
	Sent        bool   `json:"sent"`
}

func runMU300SMSSend(ctx context.Context, input string) (string, error) {
	cmd := exec.CommandContext(ctx, "/opt/mu300/bin/mu300-sms", "send", "--stdin")
	cmd.Stdin = strings.NewReader(input)
	// Never surface native stdout/stderr: it can contain a number or message.
	out, err := cmd.Output()
	return string(out), err
}

func (a *agent) mu300SendSMS(phone, text string) (int, error) {
	if !mu300SMSPhone.MatchString(phone) || !utf8.ValidString(text) || strings.TrimSpace(text) == "" || utf8.RuneCountInString(text) > 70 {
		return 0, errors.New("MU300 SMS requires a valid number and 1-70 characters")
	}
	for _, r := range text {
		if r > 0xffff || (r < 32 && r != '\n' && r != '\t') || r == 127 {
			return 0, errors.New("MU300 SMS does not support emoji or control characters")
		}
	}
	b := a.mu300SMS
	if b == nil {
		return 0, errors.New("native SMS backend unavailable")
	}
	if !b.sendMu.TryLock() {
		return 0, errors.New("SMS submission in progress; do not repeat")
	}
	defer b.sendMu.Unlock()
	// Serialize with call control and verify current radio state before sending.
	a.mu300CallMu.Lock()
	calls, err := a.mu300PollRefreshCalls()
	a.mu300CallMu.Unlock()
	if err != nil || len(calls) > 0 {
		return 0, errors.New("finish calls before sending SMS; unknown call state blocks submission")
	}
	h := sha256.Sum256([]byte(phone + "\x00" + text))
	state := mu300SMSSendState{Fingerprint: hex.EncodeToString(h[:]), Started: time.Now().Unix()}
	previous, err := readMU300PrivateFile(filepath.Join(b.state, "send.json"), 4096)
	if err == nil {
		var old mu300SMSSendState
		if json.Unmarshal(previous, &old) != nil {
			return 0, errors.New("SMS submission state needs review")
		}
		if state.Started-old.Started < 120 && (!old.Sent || old.Fingerprint == state.Fingerprint) {
			if old.Sent {
				return 1, nil
			}
			return 0, errors.New("previous SMS outcome unknown; check recipient before retrying after two minutes")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, errors.New("SMS submission state unavailable")
	}
	// A restart or disconnected HTTP client must not cause an implicit retry.
	if err := b.save("send.json", state); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	out, err := b.runSend(ctx, phone+"\n"+text)
	if err != nil || !mu300SentReply.MatchString(strings.TrimSpace(out)) {
		return 0, errors.New("SMS submission not confirmed; check recipient before retrying; no automatic resend")
	}
	state.Sent = true
	if err := b.save("send.json", state); err != nil {
		return 0, errors.New("SMS accepted by modem but receipt not saved; do not resend")
	}
	return 1, nil
}

func (a *agent) mu300SMSSendHTTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phone      string `json:"phone"`
		Message    string `json:"message"`
		SIMAccount string `json:"sim_account,omitempty"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.SIMAccount != "" && body.SIMAccount != "0" {
		writeError(w, http.StatusBadRequest, "MU300 has one SIM; the selected SIM account is not supported")
		return
	}
	n, err := a.sendSMSMessageForSIM(body.Phone, body.Message, body.SIMAccount)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true, "segments": n})
}

// fmt is used here to keep unsupported operations explicit in internal callers.
func mu300SMSUnsupported(operation string) error {
	return fmt.Errorf("MU300 SMS %s is not supported; native messages are preserved", operation)
}

// The MU300 native daemon exposes exactly one slot, with legacy handle "0".
// No Android subscription ID or human-readable label is an alias for that handle.
func (a *agent) sendSMSMessageForSIM(phone, message, account string) (int, error) {
	if account == "" {
		return a.sendSMSMessage(phone, message)
	}
	if a.profile.Name != "mu300-openwrt" || account != "0" {
		return 0, errors.New("selected SIM account is not supported")
	}
	// Do not trust cached metadata: removed, locked or unqueryable SIMs must not
	// fall back to the default sender. This query never changes modem settings.
	a.mu300CallMu.Lock()
	calls, pollErr := a.mu300PollRefreshCalls()
	if pollErr != nil || len(calls) != 0 {
		a.mu300CallMu.Unlock()
		return 0, errors.New("finish calls before selecting SMS SIM; unknown call state blocks submission")
	}
	state, err := a.mu300AT("AT+CPIN?", 3*time.Second)
	a.mu300CallMu.Unlock()
	ready := false
	if err == nil {
		for _, line := range strings.Split(state, "\n") {
			if strings.TrimSpace(line) == "+CPIN: READY" {
				ready = true
			}
		}
	}
	if !ready {
		return 0, errors.New("selected SIM is absent, locked or unavailable")
	}
	return a.mu300SendSMS(strings.TrimSpace(phone), message)
}
