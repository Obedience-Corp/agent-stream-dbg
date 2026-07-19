package anim

import (
	"strings"
	"testing"
)

func TestReducedMotionEnv(t *testing.T) {
	t.Setenv("STREAM_DEBUGGER_REDUCED_MOTION", "1")
	if !ReducedMotion() {
		t.Fatal("expected reduced motion when STREAM_DEBUGGER_REDUCED_MOTION=1")
	}
	t.Setenv("STREAM_DEBUGGER_REDUCED_MOTION", "")
	t.Setenv("NO_MOTION", "true")
	if !ReducedMotion() {
		t.Fatal("expected reduced motion when NO_MOTION=true")
	}
}

func TestNormalizeRate(t *testing.T) {
	if NormalizeRate(0) != 0 {
		t.Fatal("zero rate should be 0")
	}
	if NormalizeRate(80) < 0.99 {
		t.Fatalf("80 tok/s should saturate, got %v", NormalizeRate(80))
	}
	if NormalizeRate(20) <= 0 || NormalizeRate(20) >= 1 {
		t.Fatalf("20 tok/s should be mid-scale, got %v", NormalizeRate(20))
	}
}

func TestEnergyStripLiveVsIdle(t *testing.T) {
	s := DefaultStyles()
	live := EnergyStrip(3, 0.8, true, 16, s, false)
	idle := EnergyStrip(3, 0, false, 16, s, false)
	if live == "" || idle == "" {
		t.Fatal("strips should render")
	}
	// Both produce visible glyphs (ANSI-stripped length >= width).
	if plainLen(live) < 16 || plainLen(idle) < 16 {
		t.Fatalf("strip width short: live=%d idle=%d", plainLen(live), plainLen(idle))
	}
}

func TestHeaderMeterContainsState(t *testing.T) {
	s := DefaultStyles()
	got := HeaderMeter(2, 42, true, false, 20, s, false)
	for _, want := range []string{"STREAM", "tok/s", "LIVE"} {
		if !strings.Contains(stripANSI(got), want) {
			t.Errorf("header missing %q:\n%s", want, stripANSI(got))
		}
	}
	idle := HeaderMeter(0, 0, false, false, 20, s, true)
	if !strings.Contains(stripANSI(idle), "IDLE") {
		t.Fatalf("idle header missing IDLE: %s", stripANSI(idle))
	}
	errLine := HeaderMeter(1, 0, false, true, 20, s, false)
	if !strings.Contains(stripANSI(errLine), "ERROR") {
		t.Fatalf("error header missing ERROR: %s", stripANSI(errLine))
	}
}

func TestAgentAndFlowGlyphsChangeWithState(t *testing.T) {
	s := DefaultStyles()
	active := stripANSI(AgentGlyph(true, false, 1, s, false))
	done := stripANSI(AgentGlyph(false, true, 1, s, false))
	idle := stripANSI(AgentGlyph(false, false, 1, s, false))
	if active == done || active == idle {
		t.Fatalf("agent glyphs should differ: active=%q done=%q idle=%q", active, done, idle)
	}
	prog := stripANSI(FlowGlyph(true, true, false, 2, s, false))
	ok := stripANSI(FlowGlyph(true, false, true, 2, s, false))
	if prog == ok {
		t.Fatalf("flow glyphs should differ in-progress vs done: %q %q", prog, ok)
	}
}

func TestStreamingLabel(t *testing.T) {
	s := DefaultStyles()
	got := stripANSI(StreamingLabel(5, 30, s, false))
	if !strings.Contains(got, "streaming") {
		t.Fatalf("label = %q", got)
	}
}

func plainLen(s string) int {
	return len([]rune(stripANSI(s)))
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
