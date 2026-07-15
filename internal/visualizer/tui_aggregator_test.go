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

// TestModelHandleEvent_AggregatorGetsExclusiveWizardState regression-
// tests this task's Kind-based merge of tui.go's handleEvent (formerly
// separate "agent_stream_start"/"wizard_stream_start"-shaped string
// cases): a role: aggregator lane's stream lifecycle updates
// m.wizardState, while any other lane updates m.agents — matching the
// original wizard_*-exclusive behavior for brainyard specifically.
func TestModelHandleEvent_AggregatorGetsExclusiveWizardState(t *testing.T) {
	m := newTestModel(t)

	m.handleEvent(&events.Event{Kind: events.KindStreamStart, SourceID: "agent_a"})
	m.handleEvent(&events.Event{Kind: events.KindContent, SourceID: "agent_a", Content: "hi"})
	m.handleEvent(&events.Event{Kind: events.KindStreamEnd, SourceID: "agent_a"})

	m.handleEvent(&events.Event{Kind: events.KindStreamStart, SourceID: "wizard"})
	m.handleEvent(&events.Event{Kind: events.KindContent, SourceID: "wizard", Content: "synthesized"})
	m.handleEvent(&events.Event{Kind: events.KindStreamEnd, SourceID: "wizard"})

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

	if m.wizardState.Content.String() != "synthesized" {
		t.Errorf("expected wizardState's content 'synthesized', got %q", m.wizardState.Content.String())
	}
	if m.wizardState.Active {
		t.Error("expected wizardState to no longer be active after stream_end")
	}
	if _, ok := m.agents["wizard"]; ok {
		t.Error("expected the aggregator lane to never appear in m.agents")
	}
}

// TestModelHandleEvent_AggregatorRoleGeneralizesBeyondWizard proves
// handleEvent's aggregator dispatch is genuinely role-based, not a
// string comparison against "wizard" moved into Kind-based code: with
// an aggregator lane named "supervisor", a literal "wizard"-named lane
// must be treated as an ordinary worker (into m.agents), and
// "supervisor"'s events must land in m.wizardState instead.
func TestModelHandleEvent_AggregatorRoleGeneralizesBeyondWizard(t *testing.T) {
	m := newTestModel(t)
	m.dialectFlow = testFlowStateWithAggregator("supervisor")

	m.handleEvent(&events.Event{Kind: events.KindStreamStart, SourceID: "wizard"})
	m.handleEvent(&events.Event{Kind: events.KindContent, SourceID: "wizard", Content: "not the aggregator"})

	m.handleEvent(&events.Event{Kind: events.KindStreamStart, SourceID: "supervisor"})
	m.handleEvent(&events.Event{Kind: events.KindContent, SourceID: "supervisor", Content: "synthesized"})

	wizardLane, ok := m.agents["wizard"]
	if !ok {
		t.Fatal("expected the 'wizard'-named lane to be treated as an ordinary worker agent")
	}
	if wizardLane.Content.String() != "not the aggregator" {
		t.Errorf("expected agents[\"wizard\"] content %q, got %q", "not the aggregator", wizardLane.Content.String())
	}

	if m.wizardState.Content.String() != "synthesized" {
		t.Errorf("expected wizardState's content 'synthesized' (from the declared aggregator 'supervisor'), got %q", m.wizardState.Content.String())
	}
	if _, ok := m.agents["supervisor"]; ok {
		t.Error("expected the declared aggregator 'supervisor' to never appear in m.agents")
	}
}
