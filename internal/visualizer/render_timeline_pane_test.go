package visualizer

import (
	"strings"
	"testing"

	"github.com/lancekrogers/stream-debugger/internal/events"
)

// TestRenderTimelinePane is a floor-level test for render_timeline_pane.go
// (previously untested directly): it must render stage bars and an agent
// section without panicking, and handle the no-messages empty state.
func TestRenderTimelinePane(t *testing.T) {
	if out := (InteractiveModel{}).renderTimelinePane(); !strings.Contains(out, "No messages yet") {
		t.Errorf("expected empty-state message, got %q", out)
	}

	m := newFixtureModel(t)
	out := m.renderTimelinePane()
	if !strings.Contains(out, "Stages") {
		t.Error("expected a Stages section")
	}
	if !strings.Contains(out, "Total:") {
		t.Error("expected a Total duration line")
	}
	if !strings.Contains(out, "Agents") {
		t.Error("expected an Agents section given the fixture has agent responses")
	}
}

// TestIsEventVisible_And_CountVisibleEvents cover the aggregator-only and
// token filters, matching what the Events pane header toggles apply.
func TestIsEventVisible_And_CountVisibleEvents(t *testing.T) {
	m := newFixtureModel(t)

	tokenEvt := &events.Event{Kind: events.KindContent, SourceID: "agent"}
	m.showTokens = false
	if m.isEventVisible(tokenEvt) {
		t.Error("expected a token event to be hidden when showTokens is false")
	}
	m.showTokens = true
	if !m.isEventVisible(tokenEvt) {
		t.Error("expected a token event to be visible when showTokens is true")
	}

	aggregatorEvt := &events.Event{Kind: events.KindStreamStart, SourceID: aggregatorStageName}
	nonAggregatorEvt := &events.Event{Kind: events.KindStreamStart, SourceID: "agent"}
	m.eventsAggregatorOnly = true
	if !m.isEventVisible(aggregatorEvt) {
		t.Error("expected an aggregator event to remain visible under aggregator-only filter")
	}
	if m.isEventVisible(nonAggregatorEvt) {
		t.Error("expected a non-aggregator event to be hidden under aggregator-only filter")
	}

	m.eventsAggregatorOnly = false
	m.showTokens = true
	total := 0
	for _, evt := range m.messages[0].Events {
		if m.isEventVisible(evt) {
			total++
		}
	}
	if got := m.countVisibleEvents(); got != total {
		t.Errorf("expected countVisibleEvents to match a manual count (%d), got %d", total, got)
	}
}
