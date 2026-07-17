package visualizer

import (
	"testing"

	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/mapping"
)

// newTestInteractiveModel returns a model ready for applyParsedEvent,
// with one empty Message queued at streamIndex 0 — mirroring how a real
// streaming turn is set up before events start arriving.
func newTestInteractiveModel(t *testing.T) *InteractiveModel {
	t.Helper()
	m := NewInteractiveModel(&config.EnhancedConfig{})
	m.messages = append(m.messages, Message{})
	m.streamIndex = 0
	return &m
}

// TestApplyParsedEvent_AggregatorGetsExclusiveMetrics regression-tests
// this task's Kind-based merge of what used to be separate per-lane
// wire-event-shaped cases: the aggregator lane still exclusively gets
// StartTime/FirstTokenMs/DurationMs tracking — a plain worker agent
// going through the identical Kind sequence must not, preserving the
// asymmetry the original aggregator-lane-only cases already had.
func TestApplyParsedEvent_AggregatorGetsExclusiveMetrics(t *testing.T) {
	m := newTestInteractiveModel(t)

	// A regular worker agent: stream_start -> content -> stream_end.
	m.applyParsedEvent(&events.Event{Kind: events.KindStreamStart, SourceID: "agent_a"})
	m.applyParsedEvent(&events.Event{Kind: events.KindContent, SourceID: "agent_a", Content: "hi"})
	m.applyParsedEvent(&events.Event{Kind: events.KindStreamEnd, SourceID: "agent_a"})

	// The aggregator: same Kind sequence, different SourceID.
	m.applyParsedEvent(&events.Event{Kind: events.KindStreamStart, SourceID: aggregatorStageName})
	m.applyParsedEvent(&events.Event{Kind: events.KindContent, SourceID: aggregatorStageName, Content: "synthesized"})
	m.applyParsedEvent(&events.Event{Kind: events.KindStreamEnd, SourceID: aggregatorStageName})

	agent := m.messages[0].AgentResponses["agent_a"]
	if agent == nil {
		t.Fatal("expected an AgentResponse for agent_a")
	}
	if !agent.StartTime.IsZero() {
		t.Errorf("expected agent_a's StartTime to stay zero (non-aggregator lanes never tracked it), got %v", agent.StartTime)
	}
	if agent.DurationMs != 0 {
		t.Errorf("expected agent_a's DurationMs to stay 0, got %d", agent.DurationMs)
	}
	if !agent.Completed {
		t.Error("expected agent_a to be marked Completed")
	}
	if agent.FullContent != "hi" {
		t.Errorf("expected agent_a's FullContent 'hi', got %q", agent.FullContent)
	}

	aggregator := m.aggregatorResponse(m.messages[0])
	if aggregator == nil {
		t.Fatal("expected aggregatorResponse to find the aggregator lane's response")
	}
	if aggregator.StartTime.IsZero() {
		t.Error("expected the aggregator's StartTime to be tracked")
	}
	if !aggregator.Completed {
		t.Error("expected the aggregator to be marked Completed")
	}
	if aggregator.FullContent != "synthesized" {
		t.Errorf("expected aggregator's FullContent 'synthesized', got %q", aggregator.FullContent)
	}
}

// TestApplyParsedEvent_AggregatorStreamEndIsIdempotent regression-tests
// the duplicate-completion guard that used to be exclusive to the
// aggregator lane's own stream-complete case: a second stream_end for
// the aggregator must not re-run duration/metrics calculation, matching
// the original guard.
func TestApplyParsedEvent_AggregatorStreamEndIsIdempotent(t *testing.T) {
	m := newTestInteractiveModel(t)

	m.applyParsedEvent(&events.Event{Kind: events.KindStreamStart, SourceID: aggregatorStageName})
	m.applyParsedEvent(&events.Event{Kind: events.KindStreamEnd, SourceID: aggregatorStageName})
	firstEnd := m.messages[0].AgentResponses[aggregatorStageName].EndTime

	m.applyParsedEvent(&events.Event{Kind: events.KindStreamEnd, SourceID: aggregatorStageName})
	secondEnd := m.messages[0].AgentResponses[aggregatorStageName].EndTime

	if !firstEnd.Equal(secondEnd) {
		t.Errorf("expected a duplicate stream_end to be ignored (EndTime unchanged), got first=%v second=%v", firstEnd, secondEnd)
	}
}

// TestDeriveAgentsFromEvents_ExcludesAggregator regression-tests the
// role-based generalization of what used to be a hardcoded
// aid != <aggregator source> filter: the aggregator lane must not appear
// in the derived worker-agent list, regardless of its actual SourceID
// spelling.
func TestDeriveAgentsFromEvents_ExcludesAggregator(t *testing.T) {
	m := NewInteractiveModel(&config.EnhancedConfig{})
	evts := []*events.Event{
		{Kind: events.KindStreamStart, SourceID: "agent_a"},
		{Kind: events.KindStreamStart, SourceID: "agent_b"},
		{Kind: events.KindStreamStart, SourceID: aggregatorStageName},
	}
	agents := m.deriveAgentsFromEvents(evts)

	want := []string{"agent_a", "agent_b"}
	if len(agents) != len(want) {
		t.Fatalf("expected %v, got %v", want, agents)
	}
	for i, w := range want {
		if agents[i] != w {
			t.Errorf("agent %d: expected %q, got %q", i, w, agents[i])
		}
	}
}

// testFlowStateWithAggregator builds a flowState directly from a
// hand-constructed FlowSpec, bypassing bridge.Flow()'s single global
// loaded reference dialect entirely — the only way to prove role
// resolution generalizes to a DIFFERENT aggregator source name than the
// default dialect's own, rather than merely regression-testing against
// the one string every other test in this file happens to use.
func testFlowStateWithAggregator(aggregatorSource string) flowState {
	flow := flowState{spec: &mapping.FlowSpec{
		Lanes:       []mapping.LaneRule{{Match: mapping.LaneMatchSpec{Source: aggregatorSource}, Role: "aggregator"}},
		DefaultRole: "worker",
	}}
	flow.refreshModel()
	return flow
}

// TestApplyParsedEvent_AggregatorRoleGeneralizesBeyondDefaultSource is
// the genericity proof a self-review found missing: every other test in
// this file uses the default dialect's own aggregator SourceID, so a
// regression to hardcoded string comparison would pass them all
// undetected (a mutation test confirmed this directly). Here the
// aggregator lane is named "supervisor" — a lane using the default
// dialect's own aggregator source name is ALSO present and must get NO
// special treatment, proving resolution is genuinely role-based, not the
// same old string check moved one layer down.
func TestApplyParsedEvent_AggregatorRoleGeneralizesBeyondDefaultSource(t *testing.T) {
	m := NewInteractiveModel(&config.EnhancedConfig{})
	m.dialectFlow = testFlowStateWithAggregator("supervisor")
	m.messages = append(m.messages, Message{})
	m.streamIndex = 0

	// A lane using the default dialect's own aggregator source name —
	// NOT the declared aggregator here — must be treated as an ordinary
	// worker.
	m.applyParsedEvent(&events.Event{Kind: events.KindStreamStart, SourceID: aggregatorStageName})
	m.applyParsedEvent(&events.Event{Kind: events.KindContent, SourceID: aggregatorStageName, Content: "not the aggregator"})
	m.applyParsedEvent(&events.Event{Kind: events.KindStreamEnd, SourceID: aggregatorStageName})

	// The declared aggregator lane, named "supervisor".
	m.applyParsedEvent(&events.Event{Kind: events.KindStreamStart, SourceID: "supervisor"})
	m.applyParsedEvent(&events.Event{Kind: events.KindContent, SourceID: "supervisor", Content: "synthesized"})
	m.applyParsedEvent(&events.Event{Kind: events.KindStreamEnd, SourceID: "supervisor"})

	otherLane := m.messages[0].AgentResponses[aggregatorStageName]
	if otherLane == nil {
		t.Fatal("expected an AgentResponse for the default-dialect-aggregator-source-named lane")
	}
	if !otherLane.StartTime.IsZero() {
		t.Errorf("expected that lane's StartTime to stay zero (it is NOT the declared aggregator here), got %v", otherLane.StartTime)
	}
	if otherLane.Role != events.RoleWorker {
		t.Errorf("expected the undeclared lane to carry worker role, got %q", otherLane.Role)
	}

	aggregator := m.aggregatorResponse(m.messages[0])
	if aggregator == nil {
		t.Fatal("expected aggregatorResponse to find the 'supervisor' lane")
	}
	if aggregator.AgentID != "supervisor" {
		t.Errorf("expected the resolved aggregator to be 'supervisor', got %q", aggregator.AgentID)
	}
	if aggregator.StartTime.IsZero() {
		t.Error("expected the 'supervisor' lane's StartTime to be tracked (it IS the declared aggregator)")
	}
	if aggregator.Role != events.RoleAggregator {
		t.Errorf("expected the declared lane role to be aggregator, got %q", aggregator.Role)
	}

	nonAggregators := m.deriveAgentsFromEvents(m.messages[0].Events)
	found := false
	for _, id := range nonAggregators {
		if id == "supervisor" {
			found = true
		}
	}
	if found {
		t.Errorf("expected 'supervisor' (the aggregator) excluded from the derived worker list, got %v", nonAggregators)
	}
	if len(nonAggregators) != 1 || nonAggregators[0] != aggregatorStageName {
		t.Errorf("expected the default-dialect-aggregator-source-named lane to appear as an ordinary worker, got %v", nonAggregators)
	}
}
