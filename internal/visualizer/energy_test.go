package visualizer

import (
	"testing"
	"time"
)

func TestEnergyStateTracksTokenBursts(t *testing.T) {
	var e energyState
	e.cap = 8
	now := time.Unix(1_700_000_000, 0)

	// Quiet tick — no tokens yet.
	e.tick(now, true)
	if e.Rate() != 0 {
		t.Fatalf("rate = %v, want 0", e.Rate())
	}

	// Burst of content tokens.
	e.onTokens(5, now)
	if e.Level() < 0.4 {
		t.Fatalf("level after burst too low: %v", e.Level())
	}
	e.tick(now.Add(80*time.Millisecond), true)
	if e.Rate() != 5 {
		t.Fatalf("rate = %v, want 5 (tokens in last second)", e.Rate())
	}
	if len(e.Samples()) == 0 {
		t.Fatal("expected samples after tick")
	}

	// Later, hits age out of the 1s window.
	e.tick(now.Add(2*time.Second), true)
	if e.Rate() != 0 {
		t.Fatalf("rate after window expiry = %v, want 0", e.Rate())
	}
}

func TestEnergyStateDecaysWhenStreamEnds(t *testing.T) {
	e := newEnergyState()
	now := time.Now()
	e.onTokens(3, now)
	e.tick(now, true)
	if e.Level() <= 0 {
		t.Fatal("expected energy while streaming")
	}
	for i := 0; i < 20; i++ {
		e.tick(now.Add(time.Duration(i+1)*80*time.Millisecond), false)
	}
	if e.Level() > 0.05 {
		t.Fatalf("level should decay to ~0 after stream end, got %v", e.Level())
	}
}

func TestEnergyStateReset(t *testing.T) {
	e := newEnergyState()
	e.onTokens(2, time.Now())
	e.tick(time.Now(), true)
	e.reset()
	if e.Level() != 0 || e.Rate() != 0 || len(e.Samples()) != 0 {
		t.Fatalf("reset left state: level=%v rate=%v samples=%d", e.Level(), e.Rate(), len(e.Samples()))
	}
}
