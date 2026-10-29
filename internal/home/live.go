package home

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/captures"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/livesources"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) startLive() (tea.Model, tea.Cmd) {
	m.setCaptureMode(true)
	return m.startCaptures()
}
func readLive(root string, generation int) tea.Cmd {
	return func() tea.Msg {
		entries, skipped, err := livesources.List(context.Background(), filepath.Join(filepath.Dir(root), "debug", "runtimes"))
		listing := captures.Listing{Skipped: skipped}
		for _, v := range entries {
			listing.Entries = append(listing.Entries, captures.Entry{Path: v.Path, CaptureID: v.ID, SessionID: v.AgentName + " · " + v.SessionID, StartedAt: v.StartedAt, Status: "live"})
		}
		return capturesLoaded{root: root, generation: generation, listing: listing, err: err}
	}
}

func (m model) viewLive() string {
	b := m.captures
	lines := []string{titleStyle.Render("Live Obey Voice conversations"), dimStyle.Render("Select a running conversation · refreshes automatically"), ""}
	if m.notice != "" {
		lines = append(lines, statusStyle.Render(captureLabel(m.notice, max(25, m.width-12))))
	}
	if b.err != "" {
		lines = append(lines, statusStyle.Render(captureLabel(b.err, max(25, m.width-12))))
	}
	if len(b.listing.Entries) == 0 {
		lines = append(lines, "Start a conversation in Obey Voice; it will appear here.", "Recording is not required. The debugger observes diagnostic events.", "", "Using an older Obey Voice build? Update it to enable live debugging.", "Press c to choose a custom Obey Voice config folder.")
	} else {
		rows := max(1, m.height-14)
		start := max(0, b.cursor-rows+1)
		for i := start; i < min(len(b.listing.Entries), start+rows); i++ {
			entry := b.listing.Entries[i]
			prefix, style := "  ", dimStyle
			if i == b.cursor {
				prefix, style = "▸ ", cursorStyle
			}
			lines = append(lines, style.Render(captureLabel(prefix+entry.SessionID, max(25, m.width-12))))
		}
		lines = append(lines, "", "Enter opens Flow, App, Timeline, Events, and Messages.")
	}
	if b.listing.Skipped > 0 {
		lines = append(lines, dimStyle.Render("Stale or unreachable runtime entries are hidden."))
	}
	lines = append(lines, "", dimStyle.Render("↑↓ select  enter attach  c config folder  r refresh  esc home"))
	return strings.Join(lines, "\n")
}

func (m *model) setCaptureMode(live bool) {
	if m.captures.live != live {
		m.captures.listing = captures.Listing{}
		m.captures.selected = ""
		m.captures.cursor = -1
	}
	m.captures.live = live
}
