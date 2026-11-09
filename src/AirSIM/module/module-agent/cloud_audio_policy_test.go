package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"
)

func bufferAction(t *testing.T, extra string) cloudCallAction {
	t.Helper()
	raw := `{"action":"answer","call_id":"call","call_uuid":"uuid","generation":7,"owner":"iphone"` + extra + `}`
	c, err := decodeCloudCallActionForDescriptor([]byte(raw), cloudCallDescriptor{CallID: "call", CallUUID: "uuid", Generation: 7, Direction: "outgoing"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestCloudAudioPolicyWireFallbacks(t *testing.T) {
	for _, extra := range []string{"", `,"audio_buffer_version":2,"audio_buffer_mode":"manual","audio_buffer_ms":720`, `,"audio_buffer_version":"1","audio_buffer_mode":"manual","audio_buffer_ms":720`, `,"audio_buffer_version":1,"audio_buffer_mode":{},"audio_buffer_ms":720`, `,"audio_buffer_version":1,"audio_buffer_mode":"other"`, `,"audio_buffer_version":1,"audio_buffer_mode":"manual","audio_buffer_ms":719`, `,"audio_buffer_version":1,"audio_buffer_mode":"manual","audio_buffer_ms":720.5`, `,"audio_buffer_version":1,"audio_buffer_mode":"manual","audio_buffer_ms":"720"`, `,"audio_buffer_version":1,"audio_buffer_mode":"stable","audio_buffer_ms":null`, `,"audio_buffer_version":1,"audio_buffer_mode":"automatic"`} {
		c := bufferAction(t, extra)
		if got := cloudAudioPolicy(c); got != automaticCloudAudioPolicy() {
			t.Fatal(extra, got)
		}
	}
	for _, ms := range []int{240, 480, 720, 960} {
		c := bufferAction(t, `,"audio_buffer_version":1,"audio_buffer_mode":"manual"`)
		c.AudioBufferMS, _ = json.Marshal(ms)
		p := cloudAudioPolicy(c)
		if p.minimum != ms/20 || p.maximum != ms/20 || p.capacity != ms/20+2 {
			t.Fatal(p)
		}
	}
}
func manual720() cloudAudioBufferPolicy { return cloudAudioBufferPolicy{36, 36, 38, "manual"} }
func TestManual720ReserveGapCapAndReprime(t *testing.T) {
	b := cloudPCMSpeechBuffer{policy: manual720()}
	now := time.Now()
	for i := 0; i < 36; i++ {
		b.enqueue(bytes.Repeat([]byte{byte(i)}, cloudPCMFrameBytes), now.Add(time.Duration(i)*cloudAudioFrameDuration))
		if i < 35 && b.pop() != nil {
			t.Fatal("premature playout")
		}
	}
	// A 400ms network gap consumes twenty frames from the reserve, not silence.
	for i := 0; i < 36; i++ {
		if f := b.pop(); len(f) != cloudPCMFrameBytes || f[0] != byte(i) {
			t.Fatal("gap changed speech", i)
		}
	}
	if b.pop() != nil || b.target != 36 || b.primed {
		t.Fatal("manual target changed on exhaustion")
	}
	for i := 0; i < 35; i++ {
		b.enqueue(make([]byte, cloudPCMFrameBytes), now)
	}
	if b.pop() != nil {
		t.Fatal("did not rebuffer")
	}
	for i := 0; i < 80; i++ {
		b.enqueue(bytes.Repeat([]byte{byte(i)}, cloudPCMFrameBytes), now)
	}
	if len(b.frames) != 38 || b.dropped != 77 || b.target != 36 {
		t.Fatal("unbounded manual backlog", len(b.frames), b.dropped)
	}
	if b.pop()[0] != 42 {
		t.Fatal("did not keep newest bounded speech")
	}
}
func TestStableTargetRisesThenReturnsToFloor(t *testing.T) {
	c := bufferAction(t, `,"audio_buffer_version":1,"audio_buffer_mode":"stable"`)
	b := cloudPCMSpeechBuffer{policy: cloudAudioPolicy(c)}
	now := time.Now()
	for i := 0; i < 12; i++ {
		b.enqueue(make([]byte, cloudPCMFrameBytes), now.Add(time.Duration(i)*cloudAudioFrameDuration))
	}
	for b.pop() != nil {
	}
	if b.target != 13 {
		t.Fatal("underrun did not increase stable target", b.target)
	}
	now = now.Add(12 * cloudAudioFrameDuration)
	for i := 0; i < 400; i++ {
		b.enqueue(make([]byte, cloudPCMFrameBytes), now.Add(time.Duration(i)*cloudAudioFrameDuration))
		b.pop()
	}
	if b.target != 12 {
		t.Fatal("stable target failed to fall", b.target)
	}
	b.enqueue(make([]byte, cloudPCMFrameBytes), now.Add(time.Hour))
	if b.target != 36 {
		t.Fatal("stable maximum", b.target)
	}
	for i := 0; i < 100; i++ {
		b.enqueue(make([]byte, cloudPCMFrameBytes), now)
	}
	if len(b.frames) > 38 {
		t.Fatal("stable hard limit")
	}
}
func TestAudioSnapshotIdentityAndLifetime(t *testing.T) {
	c := bufferAction(t, `,"audio_buffer_version":1,"audio_buffer_mode":"manual","audio_buffer_ms":720`)
	s := &cloudAudioBufferSnapshot{}
	p := s.accept(c)
	if p != manual720() {
		t.Fatal(p)
	}
	c.AudioBufferMS = json.RawMessage(`240`)
	if s.accept(c) != p {
		t.Fatal("duplicate answer replaced snapshot")
	}
	fresh := &cloudAudioBufferSnapshot{}
	if fresh.accept(c).minimum != 12 {
		t.Fatal("new call kept old snapshot")
	}
	c.AudioBufferIdentity = false
	if (&cloudAudioBufferSnapshot{}).accept(c) != automaticCloudAudioPolicy() {
		t.Fatal("incomplete identity accepted custom policy")
	}
	for _, raw := range []string{`{"action":"answer","call_id":"other","call_uuid":"uuid","generation":7}`, `{"action":"answer","call_id":"call","call_uuid":"other","generation":7}`, `{"action":"answer","call_id":"call","call_uuid":"uuid","generation":8}`} {
		if _, err := decodeCloudCallActionForDescriptor([]byte(raw), cloudCallDescriptor{CallID: "call", CallUUID: "uuid", Generation: 7}); err == nil {
			t.Fatal("foreign identity accepted")
		}
	}
	for _, raw := range []string{`{"action":"answer","call_id":"call","generation":7}`, `{"action":"answer","call_id":"call","call_uuid":"uuid"}`} {
		c, err := decodeCloudCallActionForDescriptor([]byte(raw), cloudCallDescriptor{CallID: "call", CallUUID: "uuid", Generation: 7})
		if err != nil || c.AudioBufferIdentity {
			t.Fatal("legacy defaults granted negotiation")
		}
	}
}
func TestManualPCMWorkerAndEarlyTransition(t *testing.T) {
	b := &cloudPCMBridge{}
	b.configureAudioBuffer(manual720())
	if b.write(make([]byte, cloudPCMFrameBytes)) == nil || len(b.uplinkBuffer.frames) != 0 {
		t.Fatal("early media retained uplink")
	}
	sender, receiver := net.Pipe()
	defer receiver.Close()
	b.set(sender)
	defer b.close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { b.playUplink(ctx); close(done) }()
	for i := 0; i < 35; i++ {
		if err := b.write(bytes.Repeat([]byte{1}, cloudPCMFrameBytes)); err != nil {
			t.Fatal(err)
		}
	}
	receiver.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	buf := make([]byte, cloudPCMFrameBytes)
	if _, err := receiver.Read(buf); err == nil {
		t.Fatal("worker played below720ms reserve")
	}
	// Sending/downlink is independent of the still-unprimed receive reserve.
	sample := bytes.Repeat([]byte{9}, cloudPCMFrameBytes)
	if !bytes.Equal(b.frameDownlink(sample, time.Now()), sample) {
		t.Fatal("capture/send delayed or changed")
	}
	if err := b.write(bytes.Repeat([]byte{2}, cloudPCMFrameBytes)); err != nil {
		t.Fatal(err)
	}
	receiver.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := io.ReadFull(receiver, buf); err != nil || buf[0] != 1 {
		t.Fatal("worker did not play", err)
	}
	cancel()
	b.close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker leaked")
	}
	b.mu.Lock()
	remaining := len(b.uplinkBuffer.frames)
	b.mu.Unlock()
	if remaining != 0 {
		t.Fatal("hangup retained audio")
	}
	// A late connector cannot resurrect this disposed bridge.
	late, peer := net.Pipe()
	defer peer.Close()
	b.set(late)
	if b.conn != nil {
		t.Fatal("late reconnect resurrected audio")
	}
}
func TestBridgeReconnectKeepsPolicyButDropsSamples(t *testing.T) {
	b := &cloudPCMBridge{}
	b.configureAudioBuffer(manual720())
	a, peer := net.Pipe()
	defer peer.Close()
	b.set(a)
	b.write(make([]byte, cloudPCMFrameBytes))
	b.clear(a)
	c, peer2 := net.Pipe()
	defer peer2.Close()
	b.set(c)
	defer b.close()
	if len(b.uplinkBuffer.frames) != 0 || b.uplinkBuffer.policy != manual720() || b.uplinkBuffer.primed {
		t.Fatal("reconnect reused audio or lost policy")
	}
	// Completion from an older connector must not clear the new one.
	b.clear(a)
	if b.conn != c {
		t.Fatal("stale completion closed new media")
	}
}
func TestAnswerSnapshotSurvivesActualWebSocketSessions(t *testing.T) {
	a, d := cloudEarlyAgentFixture(t)
	d.AudioBuffer = &cloudAudioBufferSnapshot{}
	for session, ms := range []int{720, 240} {
		server, client := net.Pipe()
		socket := &cloudWebSocket{conn: server, reader: bufio.NewReader(server)}
		peer := &cloudWebSocket{conn: client, reader: bufio.NewReader(client)}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { a.serveCloudCall(ctx, socket, d, newCloudMediaHealthTracker(time.Now(), d)); close(done) }()
		client.SetDeadline(time.Now().Add(2 * time.Second))
		if _, _, err := peer.ReadMessage(); err != nil {
			t.Fatal(err)
		} // agent_ready
		for _, generation := range []uint64{d.Generation + 1, d.Generation, d.Generation} {
			raw, _ := json.Marshal(map[string]any{"action": "answer", "call_id": d.CallID, "call_uuid": d.CallUUID, "generation": generation, "owner": "iphone", "audio_buffer_version": 1, "audio_buffer_mode": "manual", "audio_buffer_ms": ms})
			if err := peer.WriteText(raw); err != nil {
				t.Fatal(err)
			}
			if _, _, err := peer.ReadMessage(); err != nil {
				t.Fatal(err)
			}
			d.AudioBuffer.mu.Lock()
			selected, policy := d.AudioBuffer.selected, d.AudioBuffer.policy
			d.AudioBuffer.mu.Unlock()
			if generation != d.Generation && session == 0 && selected {
				t.Fatal("bad identity selected policy")
			}
			if generation == d.Generation && (!selected || policy != manual720()) {
				t.Fatal("repeated/reconnected answer changed policy", policy)
			}
		}
		cancel()
		client.Close()
		server.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("websocket session leaked")
		}
	}
}
