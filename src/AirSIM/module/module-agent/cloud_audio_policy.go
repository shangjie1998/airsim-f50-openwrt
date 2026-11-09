package main

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// The negotiated cloud format remains S16LE, 8 kHz, mono, 20 ms.
// Frame counts are derived from duration; physical 48/16 kHz ALSA formats
// are resampled by the bridge and do not enter this queue.
const cloudAudioFrameDuration = 20 * time.Millisecond

type cloudAudioBufferPolicy struct {
	minimum, maximum, capacity int
	mode                       string
}

func automaticCloudAudioPolicy() cloudAudioBufferPolicy {
	// Preserve the existing U30 automatic policy, not Android's different default.
	return cloudAudioBufferPolicy{10, 30, 50, "automatic"}
}
func cloudAudioPolicy(c cloudCallAction) cloudAudioBufferPolicy {
	fallback := automaticCloudAudioPolicy()
	var version, millis int
	var mode string
	if json.Unmarshal(c.AudioBufferVersion, &version) != nil || version != 1 || json.Unmarshal(c.AudioBufferMode, &mode) != nil {
		return fallback
	}
	if len(c.AudioBufferMS) > 0 && (string(c.AudioBufferMS) == "null" || json.Unmarshal(c.AudioBufferMS, &millis) != nil) {
		return fallback
	}
	frames := func(ms int) int { return int(time.Duration(ms) * time.Millisecond / cloudAudioFrameDuration) }
	switch mode {
	case "stable":
		return cloudAudioBufferPolicy{frames(240), frames(720), frames(760), mode}
	case "manual":
		if millis == 240 || millis == 480 || millis == 720 || millis == 960 {
			n := frames(millis)
			return cloudAudioBufferPolicy{n, n, n + frames(40), mode}
		}
	}
	return fallback
}
func sameCloudAudioIdentity(a, b cloudCallDescriptor) bool {
	return a.CallID == b.CallID && a.CallUUID != "" && strings.EqualFold(a.CallUUID, b.CallUUID) && a.Generation > 0 && a.Generation == b.Generation
}

// Owned by one call identity, retained across WebSocket reconnects, never global.
// Media samples are owned by each bridge and are deliberately NOT retained.
type cloudAudioBufferSnapshot struct {
	mu       sync.Mutex
	selected bool
	policy   cloudAudioBufferPolicy
}

func (s *cloudAudioBufferSnapshot) accept(c cloudCallAction) cloudAudioBufferPolicy {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.Action != "answer" {
		return automaticCloudAudioPolicy()
	}
	if !s.selected {
		s.policy = automaticCloudAudioPolicy()
		if c.AudioBufferIdentity {
			s.policy = cloudAudioPolicy(c)
		}
		s.selected = true
	}
	return s.policy
}
func (b *cloudPCMBridge) configureAudioBuffer(policy cloudAudioBufferPolicy) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.conn != nil {
		return
	}
	b.bufferPolicy = policy
	b.uplinkBuffer = cloudPCMSpeechBuffer{policy: policy}
}
