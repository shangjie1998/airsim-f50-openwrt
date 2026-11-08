package main

import (
	"net/http"
	"net/url"
	"time"
)

// Existing journals without an owner digest remain valid for deduplication, but
// cannot authorize media. Never infer ownership from a number, slot or history.
func (s *mu300OperationStore) ownsEarlyCall(owner, call string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.load() != nil {
		return false
	}
	digest := mu300DialHash(owner)
	for _, r := range s.journal.Records {
		if r.Status == "confirmed" && r.CallID == call && r.OwnerDigest == digest {
			return true
		}
	}
	return false
}

// Loopback-only signaling evidence. The authenticated gateway separately gates
// physical source verification and issues the signed lease. This endpoint never
// opens audio, sends AT, changes call state, or claims a verified audio source.
func (a *agent) mu300EarlyMediaStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if a.profile.Name != "mu300-openwrt" || a.mu300Operations == nil {
		writeError(w, 501, "早期媒体信令不可用")
		return
	}
	query, err := parseEarlyQuery(r)
	owner, ownerErr := mu300RequestOwner(r)
	if err || ownerErr != nil || owner == "local" {
		writeError(w, 403, "早期媒体身份无效")
		return
	}
	if !a.mu300Operations.ownsEarlyCall(owner, query) {
		writeError(w, 403, "此身份未持有该通话")
		return
	}
	code := a.mu300EarlySignalCode(query)
	if code != 200 {
		writeError(w, code, http.StatusText(code))
		return
	}
	writeJSON(w, 200, map[string]any{"call_id": query, "native_early_media": true})
}

// No I/O or state mutation: native signal and a fresh, single outgoing snapshot
// are required independently from authentication and the capture adapter.
func (a *agent) mu300EarlySignalCode(id string) int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	age := time.Since(a.mu300EarlyPollAt)
	if a.mu300EarlyFreshCall != id || a.mu300EarlyPollAt.IsZero() || age < 0 || age > 2500*time.Millisecond {
		return 410
	}
	live := false
	for _, c := range a.mu300Calls {
		if c.ID == id && c.Direction == "outgoing" && (c.State == "dialing" || c.State == "alerting") {
			live = true
		}
	}
	if !live {
		return 410
	}
	if a.mu300EarlyCall != id {
		return 202
	}
	return 200
}

func parseEarlyQuery(r *http.Request) (string, bool) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	id := q.Get("call_id")
	return id, err != nil || len(q) != 1 || len(q["call_id"]) != 1 || !mu300ValidIdentity(id)
}
