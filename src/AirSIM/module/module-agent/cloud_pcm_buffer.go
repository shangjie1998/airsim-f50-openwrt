package main

import (
	"math"
	"time"
)

// The receiver, not the sender, absorbs network bursts. A single 20 ms
// playout clock preserves speech cadence instead of dumping a burst into PCM.
// Access is serialized by cloudPCMBridge.mu.
type cloudPCMSpeechBuffer struct {
	policy       cloudAudioBufferPolicy
	stableFrames int
	frames       [][]byte
	target       int
	primed       bool
	lastArrival  time.Time
	jitter       float64
	dropped      uint64
	underruns    uint64
}

func (b *cloudPCMSpeechBuffer) enqueue(frame []byte, at time.Time) {
	if len(frame) != cloudPCMFrameBytes {
		return
	}
	if b.policy.minimum == 0 {
		b.policy = automaticCloudAudioPolicy()
	}
	if b.target == 0 {
		b.target = b.policy.minimum
	}
	if !b.lastArrival.IsZero() && b.policy.mode != "manual" {
		variation := math.Abs(at.Sub(b.lastArrival).Seconds() - 0.02)
		b.jitter += (variation - b.jitter) / 16
		desired := b.policy.minimum + int(4*b.jitter/cloudAudioFrameDuration.Seconds())
		if desired > b.policy.maximum {
			desired = b.policy.maximum
		}
		if desired > b.target {
			b.target = desired
		}
		if b.policy.mode == "stable" {
			delta := at.Sub(b.lastArrival)
			if delta >= 15*time.Millisecond && delta <= 25*time.Millisecond {
				b.stableFrames++
			} else {
				b.stableFrames = 0
			}
			if b.stableFrames >= 100 {
				if b.target > b.policy.minimum {
					b.target--
				}
				b.stableFrames = 0
				b.trim(b.target + 2)
			}
		}
	}
	b.lastArrival = at
	b.frames = append(b.frames, append([]byte(nil), frame...))
	b.trim(b.policy.capacity)
}

func (b *cloudPCMSpeechBuffer) pop() []byte {
	if !b.primed {
		if len(b.frames) < b.target || b.target == 0 {
			return nil
		}
		b.primed = true
	}
	if len(b.frames) == 0 {
		b.primed = false
		b.underruns++
		b.stableFrames = 0
		if b.policy.mode != "manual" {
			step := 5
			if b.policy.mode == "stable" {
				step = 1
			}
			b.target += step
			if b.target > b.policy.maximum {
				b.target = b.policy.maximum
			}
		}
		return nil
	}
	frame := b.frames[0]
	b.frames[0] = nil
	b.frames = b.frames[1:]
	return frame
}

func (b *cloudPCMSpeechBuffer) trim(maximum int) {
	for len(b.frames) > maximum {
		b.frames[0] = nil
		b.frames = b.frames[1:]
		b.dropped++
	}
}
