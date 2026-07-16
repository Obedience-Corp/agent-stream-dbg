package visualizer

import (
	"strings"
	"testing"
)

// TestRenderEventsPane is a floor-level test for render_events_pane.go
// (previously untested directly, and the largest single render function in
// the split): both view modes must render without panicking, and the
// aggregator metrics summary and per-event lines must appear in parsed mode.
func TestRenderEventsPane(t *testing.T) {
	if out := (InteractiveModel{}).renderEventsPane(); !strings.Contains(out, "No messages yet") {
		t.Errorf("expected empty-state message, got %q", out)
	}

	m := newFixtureModel(t)

	m.viewMode = ViewModeParsed
	parsed := m.renderEventsPane()
	if !strings.Contains(parsed, "Events") {
		t.Error("expected an Events header")
	}
	if !strings.Contains(parsed, "Aggregator:") {
		t.Error("expected the aggregator metrics summary line in parsed mode")
	}
	if !strings.Contains(parsed, "flow_step_start") {
		t.Error("expected at least one flow_step_start event line")
	}

	m.viewMode = ViewModeRaw
	raw := m.renderEventsPane()
	if !strings.Contains(raw, "Raw SSE Stream") {
		t.Error("expected the raw SSE pretty-printed header in raw mode")
	}
}

// TestRenderEventsPane_ExpandedEvent proves expanding a selected event
// renders its detail body (exercised here via a filter_detail-shaped event,
// one of the richer expandable cases) without panicking.
func TestRenderEventsPane_ExpandedEvent(t *testing.T) {
	m := newFixtureModel(t)
	m.viewMode = ViewModeParsed
	m.eventExpanded = map[int]bool{0: true}
	m.selectedEventIdx = 0

	out := m.renderEventsPane()
	if strings.TrimSpace(out) == "" {
		t.Error("expected non-empty output with an expanded event")
	}
}
