package visualizer

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Passive sessions share the InteractiveModel pane renderers and header. The
// bottom panel reports source health instead of accepting conversation input.
func (p InteractiveModel) renderPassiveView() string {
	m := p.passive
	width, height := min(240, max(30, m.width)), min(100, max(12, m.height))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	state := m.sourceLifecycle()
	if m.source.Name() == "sse" && m.ingesting {
		state = "LIVE · observing"
	}
	selected := m.selectionLabel()
	if m.followLatest {
		selected += " · following latest"
	} else {
		selected += " · selection paused"
	}
	lines := []string{sessionHeader("agent-stream-dbg", observerClip(m.sessionID, 48), m.pane), accent.Render(state + "  " + selected)}
	if m.schemaNotice != "" {
		lines = append(lines, dim.Render(m.schemaNotice))
	}
	if m.err != nil {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.err.Error()))
	}
	available := max(1, height-len(lines)-6)
	body := ""
	windowed := false
	switch {
	case m.details:
		body = strings.Join(m.renderDetails(width-4, available), "\n")
		windowed = true
	case m.selectingScope:
		rows := []string{accent.Render("Choose turn / worker / session")}
		groups := m.store.groups()
		for i, g := range groups {
			prefix := "  "
			if i == m.eventRow {
				prefix = "▸ "
			}
			rows = append(rows, prefix+g.Label+" · "+g.Persona)
		}
		body = strings.Join(rows, "\n")
	case m.pane == PaneFlow:
		body = p.renderFlowPane()
	case m.pane == PaneApp:
		body = p.renderAppPane() + p.passiveActivity()
	case m.pane == PaneTimeline:
		body = p.renderTimelinePane()
		windowed = true
	case m.pane == PaneEvents:
		body = p.renderEventsPane()
		if count := len(m.selectedRecords()); count > passiveEventPage {
			start := (m.eventRow / passiveEventPage) * passiveEventPage
			body = fmt.Sprintf("Events %d–%d of %d · PgUp/PgDown navigate\n", start+1, min(count, start+passiveEventPage), count) + body
		}
	case m.pane == PaneMessages:
		body = accent.Render("You") + "\n" + lipgloss.NewStyle().Width(width-6).Render(p.messages[0].Text) + "\n\n" + p.renderParsedView(p.messages[0])
	}
	rows := strings.Split(body, "\n")
	start := 0
	if !windowed {
		start = min(m.scroll, max(0, len(rows)-available))
		if m.pane == PaneFlow || m.pane == PaneApp || m.pane == PaneEvents || m.selectingScope {
			for i, row := range rows {
				plain := ansi.Strip(row)
				if strings.HasPrefix(plain, "> ") || strings.HasPrefix(plain, "→ ") || strings.HasPrefix(plain, "▸ ") {
					if i < start {
						start = i
					}
					if i >= start+available {
						start = i - available + 1
					}
					break
				}
			}
		}
	}
	rows = rows[start:min(len(rows), start+available)]
	for len(rows) < available {
		rows = append(rows, "")
	}
	for i, row := range rows {
		rows[i] = ansi.Truncate(row, width-4, "…")
	}
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(0, 1).Width(width - 2)
	lines = append(lines, panel.Render(strings.Join(rows, "\n")))
	health := fmt.Sprintf("%d retained events · read-only · %s", len(m.store.records), m.status)
	if m.source.Name() == "sse" {
		health += " · recent history in memory"
	}
	if m.health != "" {
		health += " · " + m.health
	}
	if len(m.store.issues) > 0 {
		health += " · ! " + m.store.issues[len(m.store.issues)-1]
	}
	if m.store.evicted {
		health += " · earlier history omitted"
	}
	lines = append(lines, dim.Render(health), dim.Render("1–5 panes  ↑↓ select  Enter expand  d details  s scopes  [ ] turn"), dim.Render("Space pause/follow  Ctrl+F latest  Ctrl+R history  q back"))
	// Bound styled output by terminal cells, preserving ANSI created by renderers.
	out := []string{}
	for _, line := range lines {
		for _, row := range strings.Split(line, "\n") {
			out = append(out, ansi.Truncate(row, width, "…"))
		}
	}
	return strings.Join(out[:min(len(out), height)], "\n")
}

func (p InteractiveModel) passiveActivity() string {
	lines := []string{"", "Activity"}
	records := p.passive.selectedRecords()
	for _, r := range records {
		e := r.Event
		if e.SourceID == "" || e.Kind == "content" {
			continue
		}
		if e.StringField("job_id") == "" && e.StringField("tool_name") == "" && e.StringField("route_taken") == "" {
			continue
		}
		lines = append(lines, observerClip(fmt.Sprintf("  %s  %s %s %s  %s", e.Name, e.StringField("tool_name"), e.StringField("route_taken"), e.StringField("status"), e.Content), max(20, p.width-8)))
		if len(lines) >= 18 {
			break
		}
	}
	if len(lines) == 2 {
		return ""
	}
	return strings.Join(lines, "\n")
}
