// Package anim renders terminal activity visuals for Stream Debugger.
//
// Design intent: mission-control chrome for live streams — energy meters,
// agent pulses, and flow-stage chase — not decorative noise over log text.
package anim

import (
	"math"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styles colors stream chrome. Callers pass lipgloss styles so anim stays free
// of visualizer package cycles.
type Styles struct {
	Live   lipgloss.Style // hot wire / streaming
	Idle   lipgloss.Style // breath when quiet
	Error  lipgloss.Style
	Muted  lipgloss.Style
	Accent lipgloss.Style
	Agent  lipgloss.Style
	Agg    lipgloss.Style
	Tip    lipgloss.Style // peak meter bars
	Mid    lipgloss.Style
	Core   lipgloss.Style
}

// DefaultStyles is a truecolor-first kit (maps cleanly under ANSI256/16 too).
// Hex keeps VHS/truecolor demos vivid when the recorder supports color.
func DefaultStyles() Styles {
	return Styles{
		Live:   lipgloss.NewStyle().Foreground(lipgloss.Color("#67e8f9")).Bold(true), // cyan
		Idle:   lipgloss.NewStyle().Foreground(lipgloss.Color("#64748b")),             // slate
		Error:  lipgloss.NewStyle().Foreground(lipgloss.Color("#fb7185")).Bold(true), // rose
		Muted:  lipgloss.NewStyle().Foreground(lipgloss.Color("#64748b")),
		Accent: lipgloss.NewStyle().Foreground(lipgloss.Color("#22d3ee")).Bold(true), // bright cyan
		Agent:  lipgloss.NewStyle().Foreground(lipgloss.Color("#d8b4fe")).Bold(true), // magenta
		Agg:    lipgloss.NewStyle().Foreground(lipgloss.Color("#fde68a")).Bold(true), // amber
		Tip:    lipgloss.NewStyle().Foreground(lipgloss.Color("#a5f3fc")),             // ice
		Mid:    lipgloss.NewStyle().Foreground(lipgloss.Color("#93c5fd")),             // blue
		Core:   lipgloss.NewStyle().Foreground(lipgloss.Color("#22d3ee")),             // teal
	}
}

// ReducedMotion reports whether ambient animation should freeze on a calm frame.
func ReducedMotion() bool {
	for _, key := range []string{
		"STREAM_DEBUGGER_REDUCED_MOTION",
		"NO_MOTION",
		"FESTIVAL_REDUCED_MOTION",
	} {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

// Frame normalizes a free-running counter for cyclic glyphs.
func Frame(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// NormalizeRate maps tokens-per-second into a 0–1 energy level.
// 80 tok/s ≈ full meter — typical multi-agent streams land mid-scale.
func NormalizeRate(tokensPerSec float64) float64 {
	if tokensPerSec <= 0 {
		return 0
	}
	// Soft curve so low rates still show motion.
	return clamp01(math.Sqrt(tokensPerSec / 80.0))
}

func colorByHeat(rel float64, s Styles) lipgloss.Style {
	switch {
	case rel >= 0.75:
		return s.Tip
	case rel >= 0.4:
		return s.Mid
	default:
		return s.Core
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
