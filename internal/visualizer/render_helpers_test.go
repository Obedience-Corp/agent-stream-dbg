package visualizer

import "testing"

// TestClipLeft covers ASCII and unicode-safe clipping, plus the
// off-larger-than-line-length case.
func TestClipLeft(t *testing.T) {
	m := InteractiveModel{}

	if got := m.clipLeft("hello", 0); got != "hello" {
		t.Errorf("expected off<=0 to be a no-op, got %q", got)
	}
	if got := m.clipLeft("hello\nworld", 2); got != "llo\nrld" {
		t.Errorf("expected clipped lines, got %q", got)
	}
	if got := m.clipLeft("hi", 10); got != "" {
		t.Errorf("expected empty string when off exceeds line length, got %q", got)
	}
	if got := m.clipLeft("wörld", 1); got != "örld" {
		t.Errorf("expected rune-safe clipping of a multibyte char, got %q", got)
	}
}

// TestWrapToWidth covers wrapping, blank-line preservation, and the
// width<=0 no-op guard.
func TestWrapToWidth(t *testing.T) {
	m := InteractiveModel{}

	if got := m.wrapToWidth("hello", 0); got != "hello" {
		t.Errorf("expected width<=0 to be a no-op, got %q", got)
	}
	if got := m.wrapToWidth("abcdef", 2); got != "ab\ncd\nef" {
		t.Errorf("expected wrapped output, got %q", got)
	}
	if got := m.wrapToWidth("a\n\nb", 10); got != "a\n\nb" {
		t.Errorf("expected blank lines preserved, got %q", got)
	}
}

// TestClipRunesLeft covers the ASCII fast path, the unicode path, and the
// n<=0 and n>=length edge cases.
func TestClipRunesLeft(t *testing.T) {
	if got := clipRunesLeft("hello", 0); got != "hello" {
		t.Errorf("expected n<=0 to be a no-op, got %q", got)
	}
	if got := clipRunesLeft("hello", 2); got != "llo" {
		t.Errorf("expected ASCII clip, got %q", got)
	}
	if got := clipRunesLeft("hello", 100); got != "" {
		t.Errorf("expected empty string when n exceeds length, got %q", got)
	}
	if got := clipRunesLeft("wörld", 2); got != "rld" {
		t.Errorf("expected unicode-aware clip, got %q", got)
	}
}
