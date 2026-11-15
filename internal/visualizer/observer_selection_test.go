package visualizer

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func selectedLine(view string) string {
	for _, line := range strings.Split(view, "\n") {
		trimmed := strings.TrimLeft(line, "│ ")
		if strings.HasPrefix(trimmed, "> ") || strings.HasPrefix(trimmed, "▸ ") {
			return strings.TrimSpace(strings.Trim(line, "│"))
		}
	}
	return ""
}

func pressKey(m *ObserverModel, key string) {
	switch key {
	case "down":
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
	case "enter":
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	default:
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	}
}

func TestPausedFlowSelectionStaysOnItsRowWhileRequestsArrive(t *testing.T) {
	lines := modelRequestLines(t)
	m, _ := newClockObserver(t, true)
	deliverAll(t, m, lines[:5]) // through the skill check's start
	pressKey(m, "1")
	pressKey(m, " ")
	for range 4 { // stt, admission, model, skill_check, segment_queue
		pressKey(m, "down")
	}
	if got := selectedLine(paneText(m, PaneFlow)); !strings.Contains(got, "segment_queue") {
		t.Fatalf("setup selected %q", got)
	}
	deliverAll(t, m, lines[5:7]) // skill check ends, route starts: a row is added above
	if got := selectedLine(paneText(m, PaneFlow)); !strings.Contains(got, "segment_queue") {
		t.Fatalf("selection moved to %q", got)
	}

	for range 2 { // back up to the skill check request row
		m.movePassiveSelection(-1)
	}
	if got := selectedLine(paneText(m, PaneFlow)); !strings.Contains(got, "skill_check") {
		t.Fatalf("setup selected %q", got)
	}
	observerDeliver(t, m, lines[7], 7000)
	observerDeliver(t, m, lines[8], 8000)
	if got := selectedLine(paneText(m, PaneFlow)); !strings.Contains(got, "skill_check") {
		t.Fatalf("request selection moved to %q", got)
	}
	pressKey(m, "enter")
	if view := paneText(m, PaneFlow); !strings.Contains(view, `"label": "skill_check"`) {
		t.Fatal("Enter opened another span's details", view)
	}
	observerDeliver(t, m, lines[9], 9000)
	if view := paneText(m, PaneFlow); !strings.Contains(view, `"label": "skill_check"`) {
		t.Fatal("open details moved to another span", view)
	}
}

func TestPausedTimelineSelectionStaysOnItsSpanWhenAnEarlierSpanArrives(t *testing.T) {
	lines := modelRequestLines(t)
	m, _ := newClockObserver(t, true)
	deliverAll(t, m, lines[:7])
	pressKey(m, "3")
	pressKey(m, " ")
	for range 2 { // turn, model, skill_check
		pressKey(m, "down")
	}
	if got := selectedLine(paneText(m, PaneTimeline)); !strings.Contains(got, "model · skill_check") {
		t.Fatalf("setup selected %q", got)
	}
	// An end-only admission span that started before the skill check sorts above it.
	admission := editRecord(t, lines[6], func(r map[string]any) {
		r["seq"] = 8
		span := r["span"].(map[string]any)
		span["id"], span["stage"], span["lane"], span["state"], span["status"] = "opaque-admission-1", "admission", "admission", "end", "completed"
		span["start_ms"], span["end_ms"] = 1002, 1004
		delete(span, "request")
	})
	observerDeliver(t, m, admission, 8000)
	if got := selectedLine(paneText(m, PaneTimeline)); !strings.Contains(got, "model · skill_check") {
		t.Fatalf("timeline selection moved to %q", got)
	}
}
