package visualizer

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/lancekrogers/stream-debugger/internal/events"
)

// Handoff modes, per architecture.md's "Edges | handoff events (from→to,
// mode)" table. Unrecognized/missing modes fall back to the generic
// chain treatment (handoffModeHandoff) rather than erroring — a
// debugger showing an approximate edge beats showing nothing.
const (
	handoffModeSpawn    = "spawn"
	handoffModeHandoff  = "handoff"
	handoffModeDelegate = "delegate"
)

var (
	handoffFromStyle  = lipgloss.NewStyle().Bold(true)
	handoffToStyle    = lipgloss.NewStyle().Bold(true)
	handoffArrowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
)

// renderHandoffEdge renders evt (Kind must be events.KindHandoff) as an
// edge between two lanes — Fields["from"] -> Fields["to"] — not a row of
// content in either lane. Treatment varies by Fields["mode"]: spawn
// draws as a tree branch (a new lane entering), handoff as a chain
// (control passing linearly, from doesn't resume), delegate as a
// call/return pair (from resumes once to finishes) — the one distinction
// that separates delegate from a plain handoff.
func renderHandoffEdge(evt *events.Event) string {
	from := evt.StringField("from")
	to := evt.StringField("to")
	mode := evt.StringField("mode")

	switch mode {
	case handoffModeSpawn:
		return fmt.Sprintf("%s %s %s %s",
			handoffFromStyle.Render(from),
			handoffArrowStyle.Render("─┬─ spawns ─▶"),
			handoffToStyle.Render(to),
			handoffArrowStyle.Render("(new lane)"),
		)
	case handoffModeDelegate:
		return fmt.Sprintf("%s %s %s %s",
			handoffFromStyle.Render(from),
			handoffArrowStyle.Render("──▶ delegates ──▶"),
			handoffToStyle.Render(to),
			handoffArrowStyle.Render("··· ◀── returns ···"),
		)
	default: // handoffModeHandoff, or an unrecognized mode
		return fmt.Sprintf("%s %s %s",
			handoffFromStyle.Render(from),
			handoffArrowStyle.Render("──────▶ handoff ──────▶"),
			handoffToStyle.Render(to),
		)
	}
}

// renderHandoffEdgeExpanded is the expanded-row detail view (Events
// pane), showing the full from/to/mode fields rather than the compact
// one-line edge summary.
func renderHandoffEdgeExpanded(evt *events.Event, labelStyle, contentStyle lipgloss.Style) string {
	var b []byte
	b = append(b, labelStyle.Render("From:")...)
	b = append(b, ' ')
	b = append(b, contentStyle.Render(evt.StringField("from"))...)
	b = append(b, '\n')
	b = append(b, labelStyle.Render("To:")...)
	b = append(b, ' ')
	b = append(b, contentStyle.Render(evt.StringField("to"))...)
	b = append(b, '\n')
	b = append(b, labelStyle.Render("Mode:")...)
	b = append(b, ' ')
	b = append(b, contentStyle.Render(evt.StringField("mode"))...)
	return string(b)
}
