package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCloudEarlyNegotiationRequiresExplicitExactIdentity(t *testing.T) {
	d := cloudCallDescriptor{CallID: "relay-call", CallUUID: "relay-uuid", Generation: 7, Direction: "outgoing"}
	for _, tc := range []struct {
		name                 string
		change               func(map[string]any)
		negotiated, rejected bool
	}{
		{"v1", func(map[string]any) {}, true, false},
		{"legacy", func(v map[string]any) { delete(v, "early_media_version") }, false, false},
		{"future", func(v map[string]any) { v["early_media_version"] = 2 }, false, false},
		{"missing uuid", func(v map[string]any) { delete(v, "call_uuid") }, false, false},
		{"missing generation", func(v map[string]any) { delete(v, "generation") }, false, false},
		{"zero generation", func(v map[string]any) { v["generation"] = 0 }, false, false},
		{"foreign uuid", func(v map[string]any) { v["call_uuid"] = "other" }, false, true},
		{"foreign generation", func(v map[string]any) { v["generation"] = 8 }, false, true},
		{"foreign call", func(v map[string]any) { v["call_id"] = "other" }, false, true},
		{"end is not offer", func(v map[string]any) { v["action"] = "end" }, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := map[string]any{"action": "answer", "call_id": d.CallID, "call_uuid": d.CallUUID, "generation": d.Generation, "early_media_version": 1, "owner": "iphone"}
			tc.change(v)
			raw, _ := json.Marshal(v)
			got, err := decodeCloudCallActionForDescriptor(raw, d)
			if (err != nil) != tc.rejected {
				t.Fatal("wrong control result", err)
			}
			if got.EarlyMediaNegotiated != tc.negotiated {
				t.Fatal("wrong early negotiation")
			}
		})
	}
}

func TestMU300CloudExclusiveEarlyAndFreshDuplexState(t *testing.T) {
	a, d := cloudEarlyAgentFixture(t)
	if a.mu300CloudEarlyCode(d) != 200 || a.mu300CloudDuplexReady(d) {
		t.Fatal("DIALING incorrectly connected")
	}
	a.mu300Calls[2] = callRecord{ID: "mu300-other", Direction: "incoming", State: "waiting"}
	if a.mu300CloudEarlyCode(d) != 410 {
		t.Fatal("global capture allowed with two calls")
	}
	delete(a.mu300Calls, 2)
	c := a.mu300Calls[1]
	c.State = "active"
	a.mu300Calls[1] = c
	if a.mu300CloudEarlyCode(d) == 200 || !a.mu300CloudDuplexReady(d) {
		t.Fatal("ACTIVE transition incorrect")
	}
	a.mu300EarlyPollAt = time.Now().Add(-3 * time.Second)
	if a.mu300CloudDuplexReady(d) {
		t.Fatal("stale modem state claimed connection")
	}
}

func TestMU300CloudEarlySourceFailureKeepsSignalingAndHasIdentity(t *testing.T) {
	a, d := cloudEarlyAgentFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "unavailable", 503) }))
	defer server.Close()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}}
	defer transport.CloseIdleConnections()
	s, c := net.Pipe()
	defer s.Close()
	defer c.Close()
	socket := &cloudWebSocket{conn: s, reader: bufio.NewReader(s)}
	peer := &cloudWebSocket{conn: c, reader: bufio.NewReader(c)}
	done := make(chan struct{})
	go func() {
		a.forwardMU300CloudEarly(context.Background(), &http.Client{Transport: transport}, socket, &cloudPCMBridge{}, d)
		close(done)
	}()
	c.SetReadDeadline(time.Now().Add(time.Second))
	op, payload, err := peer.ReadMessage()
	if err != nil || op != cloudOpcodeText {
		t.Fatal(op, err)
	}
	var v map[string]any
	json.Unmarshal(payload, &v)
	if v["status"] != "early_media_unavailable" || v["call_id"] != d.CallID || v["call_uuid"] != d.CallUUID || v["generation"] != float64(d.Generation) {
		t.Fatal("invalid unavailable", string(payload))
	}
	<-done
	if a.mu300CloudCallState(d) != "alerting" {
		t.Fatal("source failure ended call")
	}
}

func TestMU300CloudPCMReadyDoesNotFakeAnswer(t *testing.T) {
	a, d := cloudEarlyAgentFixture(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	a.voiceBackend.endpoint = listener.Addr().String()
	// The fake PCM backend incorrectly offers READY even while the modem is
	// no longer connected. The Agent must recheck before claiming ACTIVE.
	handshakes := make(chan struct{}, 2)
	backendDone := make(chan struct{})
	go func() {
		defer close(backendDone)
		for i := 0; i < 2; i++ {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			conn.SetDeadline(time.Now().Add(3 * time.Second))
			hello := make([]byte, len("AIRSIMPCM1\n"))
			_, e = io.ReadFull(conn, hello)
			if e == nil && string(hello) == "AIRSIMPCM1\n" {
				if i == 0 {
					a.mu.Lock()
					call := a.mu300Calls[1]
					call.State = "alerting"
					a.mu300Calls[1] = call
					a.mu.Unlock()
				}
				io.WriteString(conn, "AIRSIMREADY")
				handshakes <- struct{}{}
				conn.Write([]byte(strings.Repeat("z", 320)))
				io.Copy(io.Discard, conn)
			}
			conn.Close()
		}
	}()
	for _, active := range []bool{false, true} {
		{
			a.mu.Lock()
			call := a.mu300Calls[1]
			call.State = "active"
			a.mu300Calls[1] = call
			a.mu300EarlyPollAt = time.Now()
			a.mu.Unlock()
		}
		s, c := net.Pipe()
		socket := &cloudWebSocket{conn: s, reader: bufio.NewReader(s)}
		peer := &cloudWebSocket{conn: c, reader: bufio.NewReader(c)}
		ctx, cancel := context.WithCancel(context.Background())
		health := newCloudMediaHealthTracker(time.Now(), d)
		bridge := &cloudPCMBridge{health: health}
		done := make(chan struct{})
		go func() { a.connectCloudPCM(ctx, socket, bridge, d, health); close(done) }()
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		op, payload, e := peer.ReadMessage()
		if e != nil || op != cloudOpcodeText || !strings.Contains(string(payload), "pcm_listening") {
			t.Fatal("no listening milestone", e)
		}
		select {
		case <-handshakes:
		case <-time.After(time.Second):
			t.Fatal("no PCM handshake")
		}
		if !active {
			c.SetReadDeadline(time.Now().Add(80 * time.Millisecond))
			_, _, e = peer.ReadMessage()
			if e == nil {
				t.Fatal("READY produced active before modem answered")
			}
		} else {
			op, payload, e = peer.ReadMessage()
			if e != nil || op != cloudOpcodeText || !strings.Contains(string(payload), `"status":"active"`) {
				t.Fatal("real ACTIVE not reported", e, string(payload))
			}
			op, payload, e = peer.ReadMessage()
			if e != nil || op != cloudOpcodeText || !strings.Contains(string(payload), "pcm_ready") {
				t.Fatal("no ready", e)
			}
			op, payload, e = peer.ReadMessage()
			if e != nil || op != cloudOpcodeBinary || string(payload) != strings.Repeat("z", 320) {
				t.Fatal("no connected PCM", e)
			}
		}
		cancel()
		bridge.close()
		s.Close()
		c.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("PCM session leaked")
		}
	}
	select {
	case <-backendDone:
	case <-time.After(time.Second):
		t.Fatal("backend leaked")
	}
}

func TestMU300LegacyCloudDoesNotOpenEarlySource(t *testing.T) {
	a, d := cloudEarlyAgentFixture(t)
	var opened atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { opened.Add(1); w.WriteHeader(503) }))
	defer server.Close()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}}
	defer transport.CloseIdleConnections()
	s, c := net.Pipe()
	defer s.Close()
	defer c.Close()
	// No peer consumes status or PCM: without an offer this path must remain
	// entirely silent, even though owned native early media is already available.
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	a.waitMU300OutgoingCloudPCM(ctx, &cloudWebSocket{conn: s, reader: bufio.NewReader(s)}, &cloudPCMBridge{}, d, nil, false, &http.Client{Transport: transport})
	if opened.Load() != 0 {
		t.Fatal("legacy client opened receive-only capture")
	}
	if a.mu300CloudCallState(d) != "alerting" {
		t.Fatal("legacy fallback changed call")
	}
}
