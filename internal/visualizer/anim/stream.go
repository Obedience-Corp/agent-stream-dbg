package anim

import (
	"fmt"
	"math"
	"strings"
)

var barGlyphs = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// LED is a one-glyph connection indicator.
// live → solid pulse, idle → breath, err → blink, reduced → static.
func LED(live, err bool, frame int, s Styles, reduced bool) string {
	if err {
		if reduced || frame%2 == 0 {
			return s.Error.Render("●")
		}
		return s.Muted.Render("○")
	}
	if live {
		spin := []string{"●", "◉", "◎", "◉"}
		if reduced {
			return s.Live.Render("●")
		}
		return s.Live.Render(spin[frame%len(spin)])
	}
	// Idle breath
	if reduced {
		return s.Idle.Render("○")
	}
	phase := []string{"○", "◌", "○", "·"}
	return s.Idle.Render(phase[frame%len(phase)])
}

// EnergyStrip is a single-line equalizer for wire throughput.
// level is 0–1 (use NormalizeRate). When not live, draws a soft idle breath.
func EnergyStrip(frame int, level float64, live bool, width int, s Styles, reduced bool) string {
	if width < 8 {
		width = 8
	}
	if width > 48 {
		width = 48
	}
	level = clamp01(level)
	if reduced {
		frame = 0
		if !live {
			level = 0.08
		}
	}

	var b strings.Builder
	b.Grow(width * 12)
	for i := 0; i < width; i++ {
		pos := float64(i) / float64(max(width-1, 1))
		// Center-weighted lobe — reads as a stream pipe, not noise.
		lobe := math.Sin(pos * math.Pi)
		lobe *= lobe
		phase := float64((frame*2+i*3)%24) / 24
		wiggle := 0.2 * math.Sin(phase*2*math.Pi)
		h := clamp01(level*(0.35+0.65*lobe) + wiggle*level*0.9)
		if !live || level < 0.06 {
			// Idle breath floor.
			h = 0.1 + 0.08*math.Abs(math.Sin(float64(frame)*0.3+float64(i)*0.22))
		}
		idx := int(h * float64(len(barGlyphs)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(barGlyphs) {
			idx = len(barGlyphs) - 1
		}
		rel := float64(idx) / float64(len(barGlyphs)-1)
		style := colorByHeat(rel, s)
		if !live {
			style = s.Idle
		}
		b.WriteString(style.Render(string(barGlyphs[idx])))
	}
	return b.String()
}

// HeaderMeter builds the title chrome line:
//   ● STREAM  ▁▂▄█▇▅  42 tok/s  LIVE
func HeaderMeter(frame int, tokensPerSec float64, live, err bool, width int, s Styles, reduced bool) string {
	level := NormalizeRate(tokensPerSec)
	led := LED(live, err, frame, s, reduced)
	stripW := width
	if stripW <= 0 {
		stripW = 20
	}
	if stripW > 28 {
		stripW = 28
	}
	strip := EnergyStrip(frame, level, live && !err, stripW, s, reduced)

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
func StreamingLabel(frame int, tokensPerSec float64, s Styles, reduced bool) string {
	spin := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	glyph := "◈"
	if !reduced {
		glyph = spin[frame%len(spin)]
	}
	level := NormalizeRate(tokensPerSec)
	strip := EnergyStrip(frame, level, true, 12, s, reduced)
	rate := ""
	if tokensPerSec > 0 {
		rate = fmt.Sprintf("  %.0f tok/s", tokensPerSec)
	}
	return s.Live.Render(glyph+" streaming  ") + strip + s.Muted.Render(rate)
}

// MiniBar is a short inline meter for agent/aggregator token activity.
func MiniBar(frame int, level float64, active bool, width int, s Styles, reduced bool) string {
	if width < 4 {
		width = 4
	}
	if width > 16 {
		width = 16
	}
	if reduced {
		frame = 0
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
			// Leading edge wiggles while active.
			g := "█"
			if active && !reduced && i == filled-1 {
				edge := []string{"█", "▓", "▒", "▓"}
				g = edge[frame%len(edge)]
			}
			b.WriteString(s.Agent.Render(g))
		} else {
			b.WriteString(s.Muted.Render("░"))
		}
	}
	return b.String()
}
