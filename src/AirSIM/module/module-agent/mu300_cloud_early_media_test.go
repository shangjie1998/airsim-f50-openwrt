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
	"testing"
	"time"
)

func cloudEarlyAgentFixture(t *testing.T) (*agent, cloudCallDescriptor) {
	t.Helper()
	a, _, path, _ := earlyAgentFixture(t)
	id := a.calls.Active.ID
	if _, _, err := a.mu300Operations.begin("relay", "cloud-op", "12345", ""); err != nil {
		t.Fatal(err)
	}
	if err := a.mu300Operations.finish("relay", "cloud-op", id, true); err != nil {
		t.Fatal(err)
	}
	appendMU300URC(t, path, "+EARLYMEDIA: 1\n")
	if _, err := a.mu300RefreshCalls(); err != nil {
		t.Fatal(err)
	}
	d := cloudCallDescriptor{LocalCallID: id, CallID: "relay-call", CallUUID: "relay-uuid", Generation: 7, Direction: "outgoing", StartedAt: time.Now()}
	a.cloudCallCurrent = &d
	return a, d
}
func cloudEarlyAgentStatus(a *agent, d cloudCallDescriptor) int {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/calls/cloud-early-media?"+cloudEarlyQuery(d).Encode(), nil)
	a.mu300CloudEarlyMediaStatus(w, r)
	return w.Code
}
func TestMU300CloudEarlyUsesReceiptAndExactDescriptor(t *testing.T) {
	a, d := cloudEarlyAgentFixture(t)
	if cloudEarlyAgentStatus(a, d) != 200 {
		t.Fatal("owned native source unavailable")
	}
	if a.calls.Active.State != "alerting" {
		t.Fatal("early source faked answer")
	}
	for _, change := range []string{"uuid", "generation", "relay-id", "local-id", "incoming", "no-receipt", "poll-stale", "active", "replacement", "ended", "pending"} {
		t.Run(change, func(t *testing.T) {
			a, d := cloudEarlyAgentFixture(t)
			want := 410
			switch change {
			case "uuid":
				d.CallUUID = "wrong"
			case "generation":
				d.Generation++
			case "relay-id":
				d.CallID = "wrong"
			case "local-id":
				a.cloudCallCurrent.LocalCallID = "mu300-wrong"
			case "incoming":
				a.cloudCallCurrent.Direction = "incoming"
			case "no-receipt":
				a.mu300Operations = &mu300OperationStore{directory: t.TempDir()}
				want = 403
			case "poll-stale":
				a.mu300EarlyPollAt = time.Now().Add(-3 * time.Second)
			case "active":
				c := a.mu300Calls[1]
				c.State = "active"
				a.mu300Calls[1] = c
			case "replacement":
				c := a.mu300Calls[1]
				c.ID = "mu300-next"
				a.mu300Calls[1] = c
			case "ended":
				a.mu300Calls = nil
			case "pending":
				a.mu300EarlyCall = ""
				want = 202
			}
			if code := cloudEarlyAgentStatus(a, d); code != want {
				t.Fatalf("got %d want %d", code, want)
			}
		})
	}
}

func TestMU300CloudDescriptorCannotAdoptNextOutgoingCall(t *testing.T) {
	a, d := cloudEarlyAgentFixture(t)
	if !a.cloudCallDescriptorAlive(d) {
		t.Fatal("live receipt lost")
	}
	c := a.mu300Calls[1]
	c.ID = "mu300-new-call"
	a.mu300Calls[1] = c
	if a.cloudCallDescriptorAlive(d) {
		t.Fatal("old cloud descriptor adopted new call")
	}
}

func TestMU300CloudEarlyFramesDoNotActivateOrOpenUplink(t *testing.T) {
	for _, ending := range []string{"active", "ended", "replaced", "cancel"} {
		t.Run(ending, func(t *testing.T) {
			a, d := cloudEarlyAgentFixture(t)
			serverEnded := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(serverEnded)
				if r.URL.RawQuery != cloudEarlyQuery(d).Encode() {
					t.Error("descriptor lost")
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				// Deliberately fragment a PCM frame. Never forward a partial sample/frame.
				io.WriteString(w, strings.Repeat("z", 117))
				w.(http.Flusher).Flush()
				io.WriteString(w, strings.Repeat("z", 203))
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer server.Close()
			transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
			}}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport}
			send, recv := net.Pipe()
			defer send.Close()
			defer recv.Close()
			socket := &cloudWebSocket{conn: send, reader: bufio.NewReader(send)}
			peer := &cloudWebSocket{conn: recv, reader: bufio.NewReader(recv)}
			health := newCloudMediaHealthTracker(time.Now(), d)
			bridge := &cloudPCMBridge{health: health}
			bridge.setMediaProtocol(cloudPCMProtocolFramedV1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			go func() { a.forwardMU300CloudEarly(ctx, client, socket, bridge, d); close(done) }()
			recv.SetReadDeadline(time.Now().Add(3 * time.Second))
			op, data, err := peer.ReadMessage()
			if err != nil || op != cloudOpcodeText {
				t.Fatal(op, err)
			}
			var status map[string]any
			json.Unmarshal(data, &status)
			if status["status"] != "early_media_ready_v1" || status["call_id"] != d.CallID || status["call_uuid"] != d.CallUUID || status["generation"] != float64(d.Generation) {
				t.Fatal("premature active", string(data))
			}
			op, data, err = peer.ReadMessage()
			if err != nil || op != cloudOpcodeBinary {
				t.Fatal(op, err)
			}
			frame, framed, err := decodeCloudPCMFrame(data)
			if err != nil || !framed || string(frame.Payload) != strings.Repeat("z", 320) {
				t.Fatal("PCM corrupted", err)
			}
			if err := bridge.write(make([]byte, 320)); err == nil {
				t.Fatal("microphone accepted before connection")
			}
			bridge.mu.Lock()
			opened := bridge.conn != nil
			bridge.mu.Unlock()
			if opened {
				t.Fatal("early capture became duplex")
			}
			a.mu.Lock()
			switch ending {
			case "active":
				c := a.mu300Calls[1]
				c.State = "active"
				a.mu300Calls[1] = c
			case "ended":
				a.mu300Calls = nil
			case "replaced":
				c := a.mu300Calls[1]
				c.ID = "mu300-next"
				a.mu300Calls[1] = c
			}
			a.mu.Unlock()
			if ending == "cancel" {
				cancel()
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("stale early stream survived")
			}
			select {
			case <-serverEnded:
			case <-time.After(2 * time.Second):
				t.Fatal("capture request leaked")
			}
		})
	}
}
