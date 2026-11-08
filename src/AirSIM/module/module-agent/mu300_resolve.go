package main

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
)

var mu300PushUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// MU300 already publishes its stable local UUID as the push call_id. Only the
// independently generated call_uuid and generation need to be bound to it.
// Never recover missing push state from a number, timestamp or modem slot.
func (a *agent) mu300ResolvePushedCall(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if a.profile.Name != "mu300-openwrt" {
		http.NotFound(w, r)
		return
	}
	var body struct {
		CallID     string          `json:"call_id"`
		CallUUID   string          `json:"call_uuid"`
		Generation json.RawMessage `json:"generation"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeError(w, 400, "invalid call identity request")
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		writeError(w, 400, "invalid call identity request")
		return
	}
	generationText := string(body.Generation)
	if len(body.Generation) > 0 && body.Generation[0] == '"' {
		if err := json.Unmarshal(body.Generation, &generationText); err != nil {
			writeError(w, 409, "invalid call identity tuple")
			return
		}
	}
	// Match the Android gateway's positive signed-64-bit range. Never use a
	// float decoder (which could round generations), or a legacy default of 1.
	generation, err := strconv.ParseUint(generationText, 10, 63)
	if err != nil || generation == 0 ||
		!mu300ValidIdentity(body.CallID) || !mu300PushUUID.MatchString(body.CallUUID) {
		writeError(w, 409, "invalid call identity tuple")
		return
	}
	matches := func() bool {
		if a.push == nil {
			return false
		}
		a.push.mu.Lock()
		defer a.push.mu.Unlock()
		return a.push.lastCallID == body.CallID && a.push.lastUUID == body.CallUUID &&
			a.push.lastGeneration == generation
	}
	if !matches() {
		writeError(w, 409, "push call identity is stale")
		return
	}
	a.mu300CallMu.Lock()
	defer a.mu300CallMu.Unlock()
	// A retained call during a missing/failed CLCC poll cannot authorize resolution.
	if err := a.mu300ControlSnapshot(); err != nil {
		writeError(w, 409, "live call cannot be confirmed")
		return
	}
	snapshot := a.callStatusSnapshot()
	if !matches() {
		writeError(w, 409, "push call identity is stale")
		return
	}
	for _, call := range snapshot.Calls {
		if call.ID == body.CallID && call.AndroidID == body.CallID && call.EndedAt == nil &&
			call.Direction == "incoming" && (call.State == "incoming" || call.State == "waiting" || call.State == "active" || call.State == "held") {
			// Project the verified real target, even if another call has priority in the
			// default status view. Do not alter the tracker, audio route or modem state.
			snapshot.Active = &call
			writeJSON(w, 200, snapshot)
			return
		}
	}
	writeError(w, 409, "push call is no longer live")
}
