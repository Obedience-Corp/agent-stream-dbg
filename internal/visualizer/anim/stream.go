package anim

import (
	"fmt"
	"math"
	"strings"
)

var barGlyphs = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// LED is a one-glyph connection indicator.
// live → solid pulse, idle → breath, err → blink, reduced → static.
// frame only advances the glyph cycle; energy comes from token samples elsewhere.
func LED(live, err bool, frame int, s Styles, reduced bool) string {
	if err {
		if reduced || frame%2 == 0 {
			return s.Error.Render("●")
		}
		return s.Muted.Render("○")
	}
	if live {
		if reduced {
			return s.Live.Render("●")
		}
		spin := []string{"●", "◉", "◎", "◉"}
		return s.Live.Render(spin[frame%len(spin)])
	}
	if reduced {
		return s.Idle.Render("○")
	}
	phase := []string{"○", "◌", "○", "·"}
	return s.Idle.Render(phase[frame%len(phase)])
}

// EnergyStrip draws a single-line equalizer from real energy samples.
//
// samples is oldest→newest history of 0–1 levels (token-driven). When live is
// false or samples is empty, a flat low idle line is drawn — no random wiggle.
func EnergyStrip(samples []float64, live bool, width int, s Styles, reduced bool) string {
	if width < 8 {
		width = 8
	}
	if width > 48 {
		width = 48
	}

	// Fit samples to width: pad left with zeros or resample.
	cols := make([]float64, width)
	if live && len(samples) > 0 {
		if reduced {
			// Single frozen level (latest sample).
			v := samples[len(samples)-1]
			for i := range cols {
				cols[i] = v
			}
		} else {
			fillSamples(cols, samples)
		}
	}

	var b strings.Builder
	b.Grow(width * 12)
	for i := 0; i < width; i++ {
		h := clamp01(cols[i])
		if !live {
			// Quiet baseline — static, not oscillating.
			h = 0.08
		}
		idx := int(math.Round(h * float64(len(barGlyphs)-1)))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(barGlyphs) {
			idx = len(barGlyphs) - 1
		}
		rel := float64(idx) / float64(len(barGlyphs)-1)
		style := colorByHeat(rel, s)
		if !live || h < 0.05 {
			style = s.Idle
		}
		b.WriteString(style.Render(string(barGlyphs[idx])))
	}
	return b.String()
}

// fillSamples maps oldest→newest samples onto cols (newest on the right).
func fillSamples(cols, samples []float64) {
	n := len(cols)
	if n == 0 {
		return
	}
	if len(samples) == 0 {
		return
	}
	if len(samples) == 1 {
		for i := range cols {
			cols[i] = samples[0]
		}
		return
	}
	// Stretch/compress samples across columns.
	for i := 0; i < n; i++ {
		// Map column i to sample index.
		t := float64(i) / float64(n-1)
		src := t * float64(len(samples)-1)
		lo := int(src)
		hi := lo + 1
		if hi >= len(samples) {
			cols[i] = samples[len(samples)-1]
			continue
		}
		frac := src - float64(lo)
		cols[i] = samples[lo]*(1-frac) + samples[hi]*frac
	}
}

// HeaderMeter builds the title chrome line from token-driven samples:
//
//	● STREAM  ▁▂▄█▇▅  42 tok/s  LIVE
func HeaderMeter(samples []float64, tokensPerSec float64, live, err bool, frame, width int, s Styles, reduced bool) string {
	led := LED(live, err, frame, s, reduced)
	stripW := width
	if stripW <= 0 {
		stripW = 20
	}
	if stripW > 28 {
		stripW = 28
	}
	strip := EnergyStrip(samples, live && !err, stripW, s, reduced)

	state := s.Muted.Render("IDLE")
	if err {
		state = s.Error.Render("ERROR")
	} else if live {
		state = s.Live.Render("LIVE")
	}

	rate := s.Muted.Render("— tok/s")
	if tokensPerSec > 0 {
		rate = s.Accent.Render(fmt.Sprintf("%.0f tok/s", tokensPerSec))
	}

	var b strings.Builder
	b.WriteString(led)
	b.WriteString(" ")
	b.WriteString(s.Accent.Render("STREAM"))
	b.WriteString("  ")
	b.WriteString(strip)
	b.WriteString("  ")
	b.WriteString(rate)
	b.WriteString("  ")
	b.WriteString(state)
	return b.String()
}

// StreamingLabel replaces the static "⏳ Streaming..." input prompt.
// The mini strip uses the same token sample history as the header.
func StreamingLabel(samples []float64, tokensPerSec float64, frame int, s Styles, reduced bool) string {
	spin := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	glyph := "◈"
	if !reduced {
		glyph = spin[frame%len(spin)]
	}
	strip := EnergyStrip(samples, true, 12, s, reduced)
	rate := ""
	if tokensPerSec > 0 {
		rate = fmt.Sprintf("  %.0f tok/s", tokensPerSec)
	}
	return s.Live.Render(glyph+" streaming  ") + strip + s.Muted.Render(rate)
}

// MiniBar is a short inline meter for agent/aggregator token activity.
// level should come from that agent's recent token rate (0–1), not a free clock.
func MiniBar(level float64, active bool, width int, s Styles, reduced bool) string {
	if width < 4 {
		width = 4
	}
	if width > 16 {
		width = 16
	}
	level = clamp01(level)
	filled := int(math.Round(level * float64(width)))
	if active && filled < 1 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	var b strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			b.WriteString(s.Agent.Render("█"))
		} else {
			b.WriteString(s.Muted.Render("░"))
		}
	}
	return b.String()
}
