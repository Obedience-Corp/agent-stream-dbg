package visualizer

import (
	"testing"

	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/logger"
)

// newTestModel returns a Model ready for handleEvent, with a real
// StructuredLogger backed by a temp dir (handleEvent calls
// m.logger.LogEvent unconditionally, and a nil *StructuredLogger would
// panic taking its mutex).
func newTestModel(t *testing.T) *Model {
	t.Helper()
	cfg := &config.EnhancedConfig{LogDir: t.TempDir()}
	sl, err := logger.NewStructuredLogger(cfg)
	if err != nil {
		t.Fatalf("NewStructuredLogger: %v", err)
	}
	return NewModel(cfg, nil, sl, "")
}

// TestModelHandleEvent_AggregatorGetsExclusiveState regression-tests this
// task's Kind-based merge of tui.go's handleEvent (formerly separate
// per-lane wire-event-shaped string cases): a role: aggregator lane's
// stream lifecycle updates m.aggregatorState, while any other lane
// updates m.agents — matching the original aggregator-lane-exclusive
// behavior for the default (brainyard) dialect specifically.
func TestModelHandleEvent_AggregatorGetsExclusiveState(t *testing.T) {
	m := newTestModel(t)

	m.handleEvent(&events.Event{Kind: events.KindStreamStart, SourceID: "agent_a"})
	m.handleEvent(&events.Event{Kind: events.KindContent, SourceID: "agent_a", Content: "hi"})
	m.handleEvent(&events.Event{Kind: events.KindStreamEnd, SourceID: "agent_a"})

	m.handleEvent(&events.Event{Kind: events.KindStreamStart, SourceID: aggregatorStageName})
	m.handleEvent(&events.Event{Kind: events.KindContent, SourceID: aggregatorStageName, Content: "synthesized"})
	m.handleEvent(&events.Event{Kind: events.KindStreamEnd, SourceID: aggregatorStageName})

	agent, ok := m.agents["agent_a"]
	if !ok {
		t.Fatal("expected an AgentState for agent_a")
	}
	if agent.Content.String() != "hi" {
		t.Errorf("expected agent_a's content 'hi', got %q", agent.Content.String())
	}
	if agent.Active {
		t.Error("expected agent_a to no longer be active after stream_end")
	}

	if m.aggregatorState.Content.String() != "synthesized" {
		t.Errorf("expected aggregatorState's content 'synthesized', got %q", m.aggregatorState.Content.String())
	}
	if m.aggregatorState.Active {
		t.Error("expected aggregatorState to no longer be active after stream_end")
	}
	if _, ok := m.agents[aggregatorStageName]; ok {
		t.Error("expected the aggregator lane to never appear in m.agents")
	}
}

// TestModelHandleEvent_AggregatorRoleGeneralizesBeyondDefaultSource
// proves handleEvent's aggregator dispatch is genuinely role-based, not
// a string comparison against the default dialect's aggregator source
// name moved into Kind-based code: with an aggregator lane named
// "supervisor", a lane using the default dialect's own aggregator
// source name must be treated as an ordinary worker (into m.agents), and
// "supervisor"'s events must land in m.aggregatorState instead.
func TestModelHandleEvent_AggregatorRoleGeneralizesBeyondDefaultSource(t *testing.T) {
	m := newTestModel(t)
	m.dialectFlow = testFlowStateWithAggregator("supervisor")

	m.handleEvent(&events.Event{Kind: events.KindStreamStart, SourceID: aggregatorStageName})
	m.handleEvent(&events.Event{Kind: events.KindContent, SourceID: aggregatorStageName, Content: "not the aggregator"})

	m.handleEvent(&events.Event{Kind: events.KindStreamStart, SourceID: "supervisor"})
	m.handleEvent(&events.Event{Kind: events.KindContent, SourceID: "supervisor", Content: "synthesized"})

	otherLane, ok := m.agents[aggregatorStageName]
	if !ok {
		t.Fatal("expected the default dialect's aggregator-source-named lane to be treated as an ordinary worker agent")
	}
	if otherLane.Content.String() != "not the aggregator" {
		t.Errorf("expected agents[%q] content %q, got %q", aggregatorStageName, "not the aggregator", otherLane.Content.String())
	}

	if m.aggregatorState.Content.String() != "synthesized" {
		t.Errorf("expected aggregatorState's content 'synthesized' (from the declared aggregator 'supervisor'), got %q", m.aggregatorState.Content.String())
	}
	if _, ok := m.agents["supervisor"]; ok {
		t.Error("expected the declared aggregator 'supervisor' to never appear in m.agents")
	}
}
