package visualizer

import (
	"strings"
	"testing"

	"github.com/Obedience-Corp/stream-debugger/internal/events"
)

// TestRenderEventExpandedPlainText is a floor-level test for
// render_events_expanded.go (previously untested directly): covers the
// aggregator-lane-complete and agent_stream_complete cases (which pull full
// content from the model's AgentResponses), the error case, and the
// default raw-JSON fallback for an event type it has no dedicated case for.
func TestRenderEventExpandedPlainText(t *testing.T) {
	m := newFixtureModel(t)

	var aggComplete, agentComplete *events.Event
	for _, evt := range m.messages[0].Events {
		if evt.Kind == events.KindStreamEnd && m.dialectFlow.role(evt) == events.RoleAggregator {
			aggComplete = evt
		} else if evt.Name == "agent_stream_complete" {
			if agentComplete == nil {
				agentComplete = evt
			}
		}
	}
	if aggComplete == nil || agentComplete == nil {
		t.Fatal("expected the fixture to contain an aggregator-complete and an agent_stream_complete event")
	}

	out := m.renderEventExpandedPlainText(aggComplete, &m.messages[0])
	if !strings.Contains(out, "Aggregator Response:") {
		t.Error("expected the aggregator's full content to be included")
	}

	out = m.renderEventExpandedPlainText(agentComplete, &m.messages[0])
	if !strings.Contains(out, "Agent: "+agentComplete.SourceID) {
		t.Error("expected the agent id to be included")
	}
	if !strings.Contains(out, "Full Response:") {
		t.Error("expected the agent's full content to be included")
	}

	errEvt := &events.Event{
		Name:   "error",
		Fields: map[string]any{"error_type": "timeout", "message": "boom"},
	}
	out = m.renderEventExpandedPlainText(errEvt, &m.messages[0])
	if !strings.Contains(out, "Error Type: timeout") || !strings.Contains(out, "Message: boom") {
		t.Errorf("expected error fields to be rendered, got %q", out)
	}

	unknown := &events.Event{Name: "some_future_event_type", Raw: []byte(`{"a":1}`)}
	out = m.renderEventExpandedPlainText(unknown, &m.messages[0])
	if !strings.Contains(out, "Raw Event Data:") {
		t.Error("expected the default case to fall back to pretty-printed raw JSON")
	}
}
