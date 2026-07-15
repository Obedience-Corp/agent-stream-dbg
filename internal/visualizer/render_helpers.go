package visualizer

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// getAgentColor returns the lipgloss color for an agent: an
// agent_colors override wins, otherwise the same hash-based palette
// spread tui.go's Model uses — thin wrapper over the shared
// package-level getAgentColor so both TUIs stay in sync rather than
// each keeping its own half-correct copy.
func (m InteractiveModel) getAgentColor(agentID string) lipgloss.Color {
	return lipgloss.Color(getAgentColor(agentID, m.agentColorOverrides()))
}

// agentColorOverrides returns the config-declared agent_colors map, or
// nil if none is configured — safe to pass straight to getAgentColor.
func (m InteractiveModel) agentColorOverrides() map[string]string {
	if m.cfg == nil || m.cfg.Display == nil {
		return nil
	}
	return m.cfg.Display.AgentColors
}

// clipLeft removes the first x columns (approx bytes) from each line for basic horizontal scrolling
func (m InteractiveModel) clipLeft(s string, off int) string {
	if off <= 0 {
		return s
	}
	var out strings.Builder
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if off >= len(line) {
			// If we clip more than line length, write empty
			// Note: we do not try to be rune-precise for performance; fallback to byte slicing
			// For better unicode handling, clip by runes below
			out.WriteString("")
		} else {
			// Clip by runes to avoid cutting multibyte characters
			out.WriteString(clipRunesLeft(line, off))
		}
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// wrapToWidth wraps long lines to the given width (approx runes)
func (m InteractiveModel) wrapToWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out strings.Builder
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line == "" {
			// Preserve blank lines
			//
		} else {
			// Wrap line by rune count
			runes := []rune(line)
			for start := 0; start < len(runes); start += width {
				end := start + width
				if end > len(runes) {
					end = len(runes)
				}
				out.WriteString(string(runes[start:end]))
				if end < len(runes) {
					out.WriteByte('\n')
				}
			}
		}
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// clipRunesLeft clips n columns (by rune) from the left; if n exceeds line length, returns empty
func clipRunesLeft(line string, n int) string {
	if n <= 0 {
		return line
	}
	// Fast path for ASCII
	if utf8.RuneCountInString(line) == len(line) {
		if n >= len(line) {
			return ""
		}
		return line[n:]
	}
	// Unicode-aware path
	i := 0
	for idx := range line {
		if i == n {
			return line[idx:]
		}
		i++
	}
	return ""
}
