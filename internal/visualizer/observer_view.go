package visualizer

import (
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/charmbracelet/lipgloss"
	"strings"
	"time"
	"unicode"
)

func observerClean(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || unicode.IsPrint(r) {
			return r
		}
		return -1
	}, text)
}
func observerClip(text string, width int) string {
	if width < 2 {
		return ""
	}
	var out strings.Builder
	out.Grow(min(len(text), width*4))
	truncated := false
	// Bound traversal too: zero-width/control runs must not scan an entire
	// retained message just to render a single terminal row.
	for index, r := range text {
		if index >= width*16 {
			truncated = true
			break
		}
		if r == '\n' || r == '\t' {
			r = ' '
		}
		if !unicode.IsPrint(r) {
			continue
		}
		out.WriteRune(r)
	}
	clean := out.String()
	if truncated || lipgloss.Width(clean) > width {
		return lipgloss.NewStyle().MaxWidth(width-1).Render(clean) + "…"
	}
	return clean
}

func (m *ObserverModel) selectedRecords() []*observedRecord {
	if m.page != nil {
		return m.page.selected(m.selected)
	}
	return m.store.selected(m.selected)
}
func (m *ObserverModel) View() string { return m.presentation().View() }

// ManifestCaptureID reads only the bounded identity needed to validate a bundle.
func observerSourceTime(event *events.Event, full bool) string {
	if !event.SourceTimestampKnown {
		return "source time unavailable"
	}
	if full {
		return event.Timestamp.Format(time.RFC3339Nano)
	}
	return event.Timestamp.Format("15:04:05.000")
}

// Source lifecycle is independent of transient navigation/loading notices.
func (m *ObserverModel) sourceLifecycle() string {
	if m.sealed {
		return "sealed"
	}
	if m.sourceEnded {
		return "unsealed / stopped"
	}
	if !m.attached {
		return "attaching"
	}
	if !m.ingesting {
		return "EOF / unsealed"
	}
	if m.following {
		return "following source"
	}
	return "reading saved source"
}
