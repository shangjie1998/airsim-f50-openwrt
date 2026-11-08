package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Read the existing AT owner's capped log, never its tty. The cursor is used
// only under mu300CallMu. Startup/rotation/truncation discard backlog; partial
// lines and SMS contents are never exposed as diagnostics or retained events.
type mu300URCTail struct {
	path          string
	file          os.FileInfo
	offset        int64
	anchor        []byte
	skipPartial   bool
	pending       map[string]mu300PendingTerminal
	earlySequence uint64
	earlyOn       bool
}

type mu300TerminalHint struct {
	slot      int
	direction string
	ended     bool
}

type mu300PendingTerminal struct {
	slot int
	at   time.Time
}

const mu300TerminalHintLifetime = 5 * time.Second

func (t *mu300URCTail) reset(f *os.File, info os.FileInfo) {
	t.pending = nil
	t.earlySequence++
	t.earlyOn = false
	t.file, t.offset = info, info.Size()
	t.anchor = make([]byte, min(uint64(32), uint64(t.offset)))
	if len(t.anchor) > 0 {
		_, _ = f.ReadAt(t.anchor, t.offset-int64(len(t.anchor)))
	}
	t.skipPartial = len(t.anchor) > 0 && t.anchor[len(t.anchor)-1] != '\n'
}

func (t *mu300URCTail) read() []mu300TerminalHint {
	info, err := os.Lstat(t.path)
	if err != nil || !info.Mode().IsRegular() {
		t.file = nil
		t.pending = nil
		t.earlySequence++
		t.earlyOn = false
		return nil
	}
	f, err := os.Open(t.path)
	if err != nil {
		t.pending = nil
		t.earlySequence++
		t.earlyOn = false
		return nil
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !os.SameFile(info, actual) {
		t.file = nil
		t.pending = nil
		t.earlySequence++
		t.earlyOn = false
		return nil
	}
	if t.file == nil || !os.SameFile(t.file, actual) || actual.Size() < t.offset || actual.Size()-t.offset > 65536 {
		t.reset(f, actual)
		return nil
	}
	anchor := make([]byte, len(t.anchor))
	if len(anchor) > 0 {
		if _, err = f.ReadAt(anchor, t.offset-int64(len(anchor))); err != nil || !bytes.Equal(anchor, t.anchor) {
			t.reset(f, actual)
			return nil
		}
	}
	data, err := io.ReadAll(io.NewSectionReader(f, t.offset, actual.Size()-t.offset))
	if err != nil {
		t.pending = nil
		t.earlySequence++
		t.earlyOn = false
		return nil
	}
	end := bytes.LastIndexByte(data, '\n') + 1
	if end == 0 {
		return nil
	}
	complete := data[:end]
	if t.skipPartial {
		complete = complete[bytes.IndexByte(complete, '\n')+1:]
		t.skipPartial = false
	}
	// A later progress event for the same slot supersedes an earlier terminal
	// line in this batch. We do not interpret progress as a live call: CLCC does.
	last := map[int]mu300TerminalHint{}
	malformed := false
	for _, line := range strings.Split(string(complete), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "+EARLYMEDIA:") {
			t.earlySequence++
			t.earlyOn = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "+EARLYMEDIA:")) == "1"
			continue
		}
		slot, direction, ended, ok := parseMU300TerminalDSCI(line)
		if !ok {
			if strings.HasPrefix(strings.TrimSpace(line), "^DSCI:") {
				malformed = true
			}
			continue
		}
		last[slot] = mu300TerminalHint{slot: slot, direction: direction, ended: ended}
	}
	t.offset += int64(end)
	t.anchor = make([]byte, min(uint64(32), uint64(t.offset)))
	if len(t.anchor) > 0 {
		_, _ = f.ReadAt(t.anchor, t.offset-int64(len(t.anchor)))
	}
	var hints []mu300TerminalHint
	if malformed {
		t.pending = nil
		t.earlySequence++
		t.earlyOn = false
		return nil
	}
	for _, hint := range last {
		hints = append(hints, hint)
	}
	return hints
}

func parseMU300TerminalDSCI(line string) (slot int, direction string, ended, ok bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "^DSCI:") {
		return
	}
	// Only inspect the four numeric fields. Remaining fields may contain a
	// phone number and are neither retained nor passed to the generic parser.
	parts := strings.SplitN(strings.TrimPrefix(line, "^DSCI:"), ",", 5)
	if len(parts) < 4 {
		return
	}
	var values [4]int
	for i := range values {
		n, err := strconv.Atoi(strings.TrimSpace(parts[i]))
		if err != nil {
			return 0, "", false, false
		}
		values[i] = n
	}
	if values[0] < 1 || values[0] > 7 || values[1] < 0 || values[1] > 1 || values[2] < 0 || values[2] > 7 {
		return
	}
	// The verified U30 reports mode 1 (and 3 during setup), unlike the generic
	// Agent's DSCI parser. A terminal hint still requires a matching cached UUID
	// and a fresh successful CLCC snapshot in which that slot is absent.
	if values[3] != 0 && values[3] != 1 && values[3] != 3 {
		return
	}
	direction = "outgoing"
	if values[1] == 1 {
		direction = "incoming"
	}
	return values[0], direction, values[2] == 6, true
}

// Keep a fresh end hint bound to the current UUID for a few polls. The native
// daemon can log termination before CLCC drops the slot. A progress event,
// changed UUID, log discontinuity or expiry invalidates it; a live CLCC row
// always wins. Never apply a hint directly as a hangup command.
func (a *agent) mu300FreshTerminalHints() map[string]bool {
	return a.mu300TerminalHintsAt(time.Now())
}
func (a *agent) mu300TerminalHintsAt(now time.Time) map[string]bool {
	if a.mu300URC == nil {
		return nil
	}
	tail := a.mu300URC
	hints := tail.read()
	if tail.pending == nil {
		tail.pending = map[string]mu300PendingTerminal{}
	}
	ended := map[string]bool{}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.mu300EarlySequence != tail.earlySequence {
		a.mu300EarlySequence = tail.earlySequence
		a.mu300EarlyCall = ""
		if tail.earlyOn && len(a.mu300Calls) == 1 {
			for _, c := range a.mu300Calls {
				if c.Direction == "outgoing" && (c.State == "dialing" || c.State == "alerting") {
					a.mu300EarlyCall = c.ID
				}
			}
		}
	}
	for _, hint := range hints {
		for id, old := range tail.pending {
			if old.slot == hint.slot {
				delete(tail.pending, id)
			}
		}
		if c, ok := a.mu300Calls[hint.slot]; hint.ended && ok && c.Direction == hint.direction {
			tail.pending[c.ID] = mu300PendingTerminal{slot: hint.slot, at: now}
			log.Print("MU300 call: native termination observed")
		}
	}
	for id, hint := range tail.pending {
		c, ok := a.mu300Calls[hint.slot]
		if !ok || c.ID != id || now.Sub(hint.at) < 0 || now.Sub(hint.at) > mu300TerminalHintLifetime {
			delete(tail.pending, id)
			continue
		}
		ended[id] = true
	}
	return ended
}
