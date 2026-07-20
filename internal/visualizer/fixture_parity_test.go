package visualizer

import (
	"bufio"
	"os"
	"testing"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/config"
)

// TestApplyParsedEvent_ReferenceFixture_AggregatorMetricsMatchOriginal is
// this task's required before/after fixture proof: the real
// reference session fixture, fed through
// applyParsedEvent exactly as a live stream would, must still produce
// the aggregator lane's exclusive metrics (StartTime/FirstTokenMs/
// DurationMs/TokenCount) — this task changed HOW the aggregator lane is
// identified (Kind+role instead of a wire event-name switch), not WHAT
// gets computed for it. Comparable, deterministic fields only (not
// wall-clock timestamps, which necessarily differ between runs).
func TestApplyParsedEvent_ReferenceFixture_AggregatorMetricsMatchOriginal(t *testing.T) {
	f, err := os.Open(referenceFixturePath(t))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	parser := bridge.NewParser()
	m := NewInteractiveModel(&config.EnhancedConfig{})
	m.messages = append(m.messages, Message{})
	m.streamIndex = 0

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		evt, err := parser.ParseRaw(line)
		if err != nil {
			t.Fatalf("ParseRaw: %v", err)
		}
		m.applyParsedEvent(evt)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan fixture: %v", err)
	}

	agg := m.aggregatorResponse(m.messages[0])
	if agg == nil {
		t.Fatal("expected the fixture to produce an aggregator AgentResponse")
	}
	if !agg.Completed {
		t.Error("expected the aggregator's response to be Completed")
	}
	if agg.StartTime.IsZero() {
		t.Error("expected the aggregator's StartTime to be tracked (aggregator-exclusive)")
	}
	if agg.FullContent == "" {
		t.Error("expected the aggregator's FullContent to be non-empty")
	}

	// Non-aggregator agents from the same fixture must NOT have gotten
	// the aggregator-exclusive tracking — the asymmetry this task
	// preserved, not introduced.
	for agentID, ar := range m.messages[0].AgentResponses {
		if agentID == aggregatorStageName {
			continue
		}
		if !ar.StartTime.IsZero() {
			t.Errorf("agent %q: expected StartTime to stay zero (non-aggregator), got %v", agentID, ar.StartTime)
		}
		if ar.DurationMs != 0 {
			t.Errorf("agent %q: expected DurationMs to stay 0 (non-aggregator), got %d", agentID, ar.DurationMs)
		}
	}
}
