package visualizer

import (
	"fmt"
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
func (m *ObserverModel) View() string {
	width := min(240, max(30, m.width))
	height := min(100, max(12, m.height))
	cyan := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	lines := []string{cyan.Render(observerClip("agent-stream-dbg · observe · "+m.sourceLifecycle()+" · "+m.status, width)), observerClip(m.path, width), observerClip(fmt.Sprintf("capture %s · runtime %s · session %s", m.captureID, m.runtimeID, m.sessionID), width)}
	if m.schemaNotice != "" {
		lines = append(lines, observerClip(m.schemaNotice, width))
	}
	if m.err != nil {
		lines = append(lines, observerClip(m.err.Error(), width))
	}
	if m.health != "" {
		lines = append(lines, observerClip(m.health, width))
	}
	if len(m.store.issues) > 0 {
		lines = append(lines, observerClip("! "+m.store.issues[len(m.store.issues)-1], width))
	}
	groups := m.store.groups()
	labelGroups := groups
	if m.page != nil {
		labelGroups = append(labelGroups, m.page.groups()...)
	}
	label := m.selected
	for _, g := range labelGroups {
		if g.Key == m.selected {
			label = fmt.Sprintf("%s · response:%s · persona:%s", g.Label, g.Response, g.Persona)
		}
	}
	follow := "selection paused"
	if m.followLatest {
		follow = "following latest"
	}
	lines = append(lines, observerClip(label+" · "+follow, width))
	paneNames := []string{"1 Flow", "2 App", "3 Timeline", "4 Events", "5 Messages"}
	for i := range paneNames {
		if Pane(i) == m.pane {
			paneNames[i] = "[" + paneNames[i] + "]"
		}
	}
	lines = append(lines, strings.Join(paneNames, "  "), strings.Repeat("─", width))
	records := m.selectedRecords()
	windowed := false
	availableRows := max(1, height-len(lines)-3)
	body := make([]string, 0)
	switch {
	case m.details:
		windowed = true
		body = m.renderDetails(width, availableRows)

	case m.pane == PaneFlow || m.pane == PaneApp:
		for _, g := range groups {
			mark := " "
			if g.Key == m.selected {
				mark = ">"
			}
			body = append(body, fmt.Sprintf("%s %s · %d events · response %s", mark, g.Label, g.Count, g.Response))
		}
	case m.pane == PaneTimeline:
		windowed = true
		if !m.intervalsEnabled {
			body = []string{"Interval interpretation disabled; Events and raw details remain available"}
			break
		}
		body = renderObserverIntervals(m.IntervalReport(), width, m.scroll, availableRows)

	case m.pane == PaneMessages:
		windowed = true
		filtered := make([]*observedRecord, 0, len(records))
		for _, r := range records {
			if r.Event.Content != "" {
				filtered = append(filtered, r)
			}
		}
		start := min(m.scroll, max(0, len(filtered)-availableRows))
		for _, r := range filtered[start:min(len(filtered), start+availableRows)] {
			body = append(body, observerClip(r.Event.Name, 64)+": "+observerClip(r.Event.Content, width))
		}
	default:
		windowed = true
		start := min(m.scroll, max(0, len(records)-availableRows))
		for i, r := range records[start:min(len(records), start+availableRows)] {
			i += start
			mark := " "
			if i == m.eventRow {
				mark = ">"
			}
			body = append(body, fmt.Sprintf("%s #%d %s · %s %s", mark, r.Event.Seq, observerSourceTime(r.Event, false), observerClip(r.Event.Name, 64), observerClip(r.Event.Content, width)))
			if r.Issue != "" {
				body = append(body, "  ! "+r.Issue)
			}
		}
	}
	if len(body) == 0 {
		body = append(body, "Waiting for source evidence; ingestion requires no prompt")
	}
	if m.store.evicted {
		body = append(body, "Earlier history on disk")
	}
	for _, issue := range m.store.issues {
		body = append(body, "! "+issue)
	}
	available := max(1, height-len(lines)-3)
	start := min(m.scroll, max(0, len(body)-available))
	if windowed {
		start = 0
	}
	for _, line := range body[start:min(len(body), start+available)] {
		lines = append(lines, observerClip(line, width))
	}
	for len(lines) < height-2 {
		lines = append(lines, "")
	}
	pageCount, pageBytes := 0, 0
	if m.page != nil {
		pageCount = len(m.page.records)
		pageBytes = m.page.bytes
	}
	lines = append(lines, dim.Render(observerClip(fmt.Sprintf("%d live + %d page events · %d retained bytes · read-only", len(m.store.records), pageCount, m.store.bytes+pageBytes), width)), dim.Render(observerClip("[ ] turn  Ctrl+R reload  Ctrl+F latest  Space pause/follow  Tab panes  Enter details  q quit", width)))
	return strings.Join(lines, "\n")
}

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
