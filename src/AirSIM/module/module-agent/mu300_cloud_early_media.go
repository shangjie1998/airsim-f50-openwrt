package main

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Relay call IDs are not modem UUIDs. Only the persisted successful dial
// receipt may bind the two; never adopt whatever outgoing call happens to exist.
func (a *agent) mu300CloudCallState(d cloudCallDescriptor) string {
	current, ok := a.currentCloudCallDescriptor()
	if !ok || d.Direction != "outgoing" || d.LocalCallID == "" ||
		current.Direction != "outgoing" || current.CallID != d.CallID || current.LocalCallID != d.LocalCallID ||
		current.CallUUID != d.CallUUID || current.Generation != d.Generation {
		return ""
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, c := range a.mu300Calls {
		if c.ID == d.LocalCallID && c.Direction == "outgoing" {
			return c.State
		}
	}
	return ""
}

func (a *agent) mu300CloudEarlyMediaStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if a.profile.Name != "mu300-openwrt" || a.mu300Operations == nil {
		writeError(w, 501, "unavailable")
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	gen, ge := strconv.ParseUint(q.Get("generation"), 10, 64)
	if err != nil || len(q) != 3 || len(q["call_id"]) != 1 || len(q["call_uuid"]) != 1 || len(q["generation"]) != 1 || !mu300ValidIdentity(q.Get("call_id")) || !mu300ValidIdentity(q.Get("call_uuid")) || ge != nil || gen == 0 || strconv.FormatUint(gen, 10) != q.Get("generation") {
		writeError(w, 400, "identity")
		return
	}
	d, ok := a.currentCloudCallDescriptor()
	if !ok || d.Direction != "outgoing" || d.CallID != q.Get("call_id") || d.CallUUID != q.Get("call_uuid") || d.Generation != gen || a.mu300CloudCallState(d) == "" {
		writeError(w, 410, "call ended or replaced")
		return
	}
	if !a.mu300Operations.ownsEarlyCall("relay", d.LocalCallID) {
		writeError(w, 403, "unowned call")
		return
	}
	code := a.mu300CloudEarlyCode(d)
	if code != 200 {
		writeError(w, code, http.StatusText(code))
		return
	}
	writeJSON(w, 200, map[string]any{"call_id": d.CallID, "native_early_media": true})
}

func cloudEarlyQuery(d cloudCallDescriptor) url.Values {
	return url.Values{"call_id": {d.CallID}, "call_uuid": {d.CallUUID}, "generation": {strconv.FormatUint(d.Generation, 10)}}
}

func (a *agent) startMU300OutgoingCloudPCM(ctx context.Context, socket *cloudWebSocket, bridge *cloudPCMBridge, d cloudCallDescriptor, health *cloudMediaHealthTracker, earlyMediaNegotiated bool) {
	client := &http.Client{Transport: &http.Transport{Proxy: nil, ResponseHeaderTimeout: 10 * time.Second}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	a.waitMU300OutgoingCloudPCM(ctx, socket, bridge, d, health, earlyMediaNegotiated, client)
}

func (a *agent) waitMU300OutgoingCloudPCM(ctx context.Context, socket *cloudWebSocket, bridge *cloudPCMBridge, d cloudCallDescriptor, health *cloudMediaHealthTracker, earlyMediaNegotiated bool, client *http.Client) {
	// Only the pre-connect phase is bounded here; duplex has its own lifecycle.
	earlyCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	attempted := false
	for earlyCtx.Err() == nil {
		state := a.mu300CloudCallState(d)
		if state == "" {
			return
		}
		if (state == "active" || state == "held") && a.mu300CloudDuplexReady(d) {
			a.voice.mu.Lock()
			a.voice.hostEnabled = true
			a.voice.mu.Unlock()
			a.connectCloudPCM(ctx, socket, bridge, d, health)
			return
		}
		if earlyMediaNegotiated && !attempted && a.mu300CloudEarlyCode(d) == 200 {
			attempted = true
			a.forwardMU300CloudEarly(earlyCtx, client, socket, bridge, d)
		}
		if !waitCloudRetry(earlyCtx, 250*time.Millisecond) {
			return
		}
	}
}

func (a *agent) forwardMU300CloudEarly(ctx context.Context, client *http.Client, socket *cloudWebSocket, bridge *cloudPCMBridge, d cloudCallDescriptor) {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer func() {
		// Failure only ends receive-only playback. Do not send a late fallback
		// notification after real ACTIVE, terminal state, or descriptor replacement.
		state := a.mu300CloudCallState(d)
		if ctx.Err() == nil && (state == "dialing" || state == "alerting") {
			_ = socket.WriteText(cloudStatusForAction("early_media_unavailable", "", d, cloudCallAction{}))
		}
	}()
	req, err := http.NewRequestWithContext(streamCtx, "GET", "http://127.0.0.1:7581/internal/cloud-early-media?"+cloudEarlyQuery(d).Encode(), nil)
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/octet-stream") {
		return
	}
	// Unlike normal PCM, never attach this reader to bridge.conn. All client
	// microphone frames are discarded until duplex explicitly installs a socket.
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-streamCtx.Done():
				return
			case <-tick.C:
				if a.mu300CloudEarlyCode(d) != 200 {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-watchDone }()
	stall := time.AfterFunc(3*time.Second, cancel)
	defer stall.Stop()
	reported := false
	frame := make([]byte, cloudPCMFrameBytes)
	for {
		if _, err = io.ReadFull(resp.Body, frame); err != nil {
			return
		}
		if streamCtx.Err() != nil || a.mu300CloudEarlyCode(d) != 200 {
			return
		}
		if !reported {
			if err = socket.WriteText(cloudStatusForAction("early_media_ready_v1", "", d, cloudCallAction{})); err != nil {
				return
			}
			reported = true
		}
		if err = socket.WriteBinary(bridge.frameDownlink(frame, time.Now())); err != nil {
			return
		}
		stall.Reset(3 * time.Second)
	}
}

// MU300 CLCC dialing/alerting both represent the client's unconnected DIALING
// phase. Never authorize a global capture tap while another call coexists.
func (a *agent) mu300CloudEarlyCode(d cloudCallDescriptor) int {
	if a.mu300CloudCallState(d) == "" {
		return 410
	}
	a.mu.RLock()
	exclusive := len(a.mu300Calls) == 1
	a.mu.RUnlock()
	if !exclusive {
		return 410
	}
	return a.mu300EarlySignalCode(d.LocalCallID)
}

func (a *agent) mu300CloudDuplexReady(d cloudCallDescriptor) bool {
	id := d.CallID
	if d.Direction == "outgoing" {
		if a.mu300CloudCallState(d) == "" {
			return false
		}
		id = d.LocalCallID
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	age := time.Since(a.mu300EarlyPollAt)
	if a.mu300EarlyPollAt.IsZero() || age < 0 || age > 2500*time.Millisecond {
		return false
	}
	for _, c := range a.mu300Calls {
		if c.ID == id && (c.State == "active" || c.State == "held") {
			return true
		}
	}
	return false
}
