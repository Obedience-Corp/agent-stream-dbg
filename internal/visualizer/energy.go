package visualizer

import (
	"math"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/visualizer/anim"
)

// energyState tracks stream chrome from real token arrivals — not a free-running
// oscillator. Each content token spikes the level; each tick decays and records
// a sample so the strip is a short history of throughput.
type energyState struct {
	// samples is a ring of recent energy levels (0–1), oldest→newest when read.
	samples []float64
	cap     int

	level float64 // smoothed instantaneous energy 0–1
	rate  float64 // tokens observed in the last second

	// hits are timestamps of individual content tokens for rate windows.
	hits []time.Time
}

const (
	energySampleCap = 28
	energyRateWindow = time.Second
)

func newEnergyState() energyState {
	return energyState{
		cap:     energySampleCap,
		samples: make([]float64, 0, energySampleCap),
	}
}

// onTokens records n content tokens arriving now (usually 1 per KindContent).
func (e *energyState) onTokens(n int, now time.Time) {
	if e == nil || n <= 0 {
		return
	}
	for i := 0; i < n; i++ {
		e.hits = append(e.hits, now)
	}
	// Burst spike: more simultaneous tokens → taller attack.
	burst := math.Min(1, 0.28+0.14*float64(n))
	if burst > e.level {
		e.level = burst
	} else {
		e.level = e.level*0.35 + burst*0.65
	}
	e.recomputeRate(now)
}

// tick decays energy when the wire is quiet and appends one history sample.
// Call once per anim frame (~80ms).
func (e *energyState) tick(now time.Time, streaming bool) {
	if e == nil {
		return
	}
	e.recomputeRate(now)

	target := anim.NormalizeRate(e.rate)
	if !streaming {
		// Drop quickly when the stream ends.
		e.level *= 0.72
		if e.level < 0.02 {
			e.level = 0
		}
		e.push(e.level)
		return
	}

	// Live: decay toward rate-based floor; empty ticks still leave a low hum
	// so LIVE is visible between sparse tokens.
	if e.level > target {
		e.level *= 0.78
		if e.level < target {
			e.level = target
		}
	} else {
		e.level = e.level*0.55 + target*0.45
	}
	if e.rate == 0 && e.level < 0.12 {
		e.level = 0.12 // waiting for next token while stream open
	}
	e.push(e.level)
}

func (e *energyState) recomputeRate(now time.Time) {
	cutoff := now.Add(-energyRateWindow)
	i := 0
	for i < len(e.hits) && e.hits[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		e.hits = append([]time.Time(nil), e.hits[i:]...)
	}
	// Tokens in the last second ≈ instantaneous tok/s.
	e.rate = float64(len(e.hits))
}

func (e *energyState) push(v float64) {
	if e.cap <= 0 {
		e.cap = energySampleCap
	}
	if len(e.samples) < e.cap {
		e.samples = append(e.samples, v)
		return
	}
	copy(e.samples, e.samples[1:])
	e.samples[len(e.samples)-1] = v
}

// Samples returns a copy of the history (oldest first) for rendering.
func (e energyState) Samples() []float64 {
	if len(e.samples) == 0 {
		return nil
	}
	out := make([]float64, len(e.samples))
	copy(out, e.samples)
	return out
}

func (e energyState) Rate() float64 { return e.rate }
func (e energyState) Level() float64 { return e.level }

// reset clears state when a new stream turn begins.
func (e *energyState) reset() {
	if e == nil {
		return
	}
	e.samples = e.samples[:0]
	e.hits = e.hits[:0]
	e.level = 0
	e.rate = 0
}
