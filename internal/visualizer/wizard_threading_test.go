package visualizer

import (
	"testing"

	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
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
// this task's Kind-based merge of what used to be separate
// "agent_stream_start"/"wizard_stream_start"-shaped cases: the
// aggregator lane (brainyard: the wizard) still exclusively gets
// StartTime/FirstTokenMs/DurationMs tracking — a plain worker agent
// going through the identical Kind sequence must not, preserving the
// asymmetry the original wizard_*-only cases already had.
func TestApplyParsedEvent_AggregatorGetsExclusiveMetrics(t *testing.T) {
	m := newTestInteractiveModel(t)

	// A regular worker agent: stream_start -> content -> stream_end.
	m.applyParsedEvent(&events.Event{Kind: events.KindStreamStart, SourceID: "agent_a"})
	m.applyParsedEvent(&events.Event{Kind: events.KindContent, SourceID: "agent_a", Content: "hi"})
	m.applyParsedEvent(&events.Event{Kind: events.KindStreamEnd, SourceID: "agent_a"})

	// The aggregator: same Kind sequence, different SourceID.
	m.applyParsedEvent(&events.Event{Kind: events.KindStreamStart, SourceID: "wizard"})
	m.applyParsedEvent(&events.Event{Kind: events.KindContent, SourceID: "wizard", Content: "synthesized"})
	m.applyParsedEvent(&events.Event{Kind: events.KindStreamEnd, SourceID: "wizard"})

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
		t.Fatal("expected aggregatorResponse to find the wizard lane's response")
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
// the duplicate-completion guard that used to be wizard_stream_complete-
// exclusive: a second stream_end for the aggregator must not re-run
// duration/metrics calculation, matching the original guard.
func TestApplyParsedEvent_AggregatorStreamEndIsIdempotent(t *testing.T) {
	m := newTestInteractiveModel(t)

	m.applyParsedEvent(&events.Event{Kind: events.KindStreamStart, SourceID: "wizard"})
	m.applyParsedEvent(&events.Event{Kind: events.KindStreamEnd, SourceID: "wizard"})
	firstEnd := m.messages[0].AgentResponses["wizard"].EndTime

	m.applyParsedEvent(&events.Event{Kind: events.KindStreamEnd, SourceID: "wizard"})
	secondEnd := m.messages[0].AgentResponses["wizard"].EndTime

	if !firstEnd.Equal(secondEnd) {
		t.Errorf("expected a duplicate stream_end to be ignored (EndTime unchanged), got first=%v second=%v", firstEnd, secondEnd)
	}
}

// TestDeriveAgentsFromEvents_ExcludesAggregator regression-tests the
// role-based generalization of what used to be a hardcoded aid !=
// "wizard" filter: the aggregator lane must not appear in the derived
// worker-agent list, regardless of its actual SourceID spelling.
func TestDeriveAgentsFromEvents_ExcludesAggregator(t *testing.T) {
	m := NewInteractiveModel(&config.EnhancedConfig{})
	evts := []*events.Event{
		{Kind: events.KindStreamStart, SourceID: "agent_a"},
		{Kind: events.KindStreamStart, SourceID: "agent_b"},
		{Kind: events.KindStreamStart, SourceID: "wizard"},
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
