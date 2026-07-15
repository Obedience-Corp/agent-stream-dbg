package visualizer

import (
	"strings"
	"testing"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/events"
)

func TestNewTimelineVisualizer(t *testing.T) {
	tv := NewTimelineVisualizer()
	if tv == nil {
		t.Fatal("Expected non-nil visualizer")
	}

	if len(tv.entries) != 0 {
		t.Errorf("Expected empty entries, got %d", len(tv.entries))
	}
}

func TestAddEvent(t *testing.T) {
	tv := NewTimelineVisualizer()
	now := time.Now()

	event := &events.Event{
		Name:      "agent_stream_start",
		Kind:      events.KindStreamStart,
		Timestamp: now,
		SourceID:  "agent_a",
	}

	tv.AddEvent(event)

	if len(tv.entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(tv.entries))
	}

	entry := tv.entries[0]
	if entry.AgentID != "agent_a" {
		t.Errorf("Expected agent_id 'agent_a', got '%s'", entry.AgentID)
	}

	if entry.Type != "agent_stream_start" {
		t.Errorf("Expected type 'agent_stream_start', got '%s'", entry.Type)
	}
}

func TestAddEvent_UpdatesTimeBounds(t *testing.T) {
	tv := NewTimelineVisualizer()

	now := time.Now()
	later := now.Add(5 * time.Second)
	earlier := now.Add(-3 * time.Second)

	// Add events out of order
	evts := []*events.Event{
		{Name: "agent_stream_start", Timestamp: now, SourceID: "agent1"},
		{Name: "agent_stream_start", Timestamp: later, SourceID: "agent2"},
		{Name: "agent_stream_start", Timestamp: earlier, SourceID: "agent3"},
	}

	for _, event := range evts {
		tv.AddEvent(event)
	}

	// Start time should be earliest
	if !tv.startTime.Equal(earlier) {
		t.Errorf("Expected start time %v, got %v", earlier, tv.startTime)
	}

	// End time should be latest
	if !tv.endTime.Equal(later) {
		t.Errorf("Expected end time %v, got %v", later, tv.endTime)
	}
}

func TestRenderTimeline_Empty(t *testing.T) {
	tv := NewTimelineVisualizer()
	output := tv.RenderTimeline(80)

	if !strings.Contains(output, "No events to display") {
		t.Error("Expected 'No events to display' message for empty timeline")
	}
}

func TestRenderTimeline_WithEvents(t *testing.T) {
	tv := NewTimelineVisualizer()
	now := time.Now()

	// Add agent stream
	tv.AddEvent(&events.Event{
		Name:      "agent_stream_start",
		Kind:      events.KindStreamStart,
		Timestamp: now,
		SourceID:  "agent_a",
	})

	tv.AddEvent(&events.Event{
		Name:      "agent_content",
		Kind:      events.KindContent,
		Timestamp: now.Add(100 * time.Millisecond),
		SourceID:  "agent_a",
		Content:   "test",
	})

	tv.AddEvent(&events.Event{
		Name:      "agent_stream_complete",
		Kind:      events.KindStreamEnd,
		Timestamp: now.Add(200 * time.Millisecond),
		SourceID:  "agent_a",
	})

	output := tv.RenderTimeline(100)

	// Verify output contains key elements
	if !strings.Contains(output, "Timeline View") {
		t.Error("Expected timeline header")
	}

	if !strings.Contains(output, "agent_a") {
		t.Error("Expected agent name in output")
	}

	if !strings.Contains(output, "Events:") {
		t.Error("Expected event count")
	}

	if !strings.Contains(output, "Legend:") {
		t.Error("Expected legend")
	}
}

func TestRenderDetailedLog_Empty(t *testing.T) {
	tv := NewTimelineVisualizer()
	output := tv.RenderDetailedLog()

	if !strings.Contains(output, "No events to display") {
		t.Error("Expected 'No events to display' message for empty log")
	}
}

func TestRenderDetailedLog_WithEvents(t *testing.T) {
	tv := NewTimelineVisualizer()
	now := time.Now()

	tv.AddEvent(&events.Event{
		Name:      "agent_content",
		Kind:      events.KindContent,
		Timestamp: now,
		SourceID:  "agent_a",
		Content:   "Hello world",
	})

	output := tv.RenderDetailedLog()

	// Verify output
	if !strings.Contains(output, "Detailed Event Log") {
		t.Error("Expected detailed log header")
	}

	if !strings.Contains(output, "agent_a") {
		t.Error("Expected agent name")
	}

	if !strings.Contains(output, "agent_content") {
		t.Error("Expected event type")
	}

	if !strings.Contains(output, "Hello world") {
		t.Error("Expected content")
	}
}

func TestRenderParallelSummary_Sequential(t *testing.T) {
	tv := NewTimelineVisualizer()
	now := time.Now()

	// Agent 1 runs then completes
	tv.AddEvent(&events.Event{Name: "agent_stream_start", Kind: events.KindStreamStart, Timestamp: now, SourceID: "agent1"})
	tv.AddEvent(&events.Event{Name: "agent_stream_complete", Kind: events.KindStreamEnd, Timestamp: now.Add(1 * time.Second), SourceID: "agent1"})

	// Agent 2 starts after agent 1 completes
	tv.AddEvent(&events.Event{Name: "agent_stream_start", Kind: events.KindStreamStart, Timestamp: now.Add(2 * time.Second), SourceID: "agent2"})
	tv.AddEvent(&events.Event{Name: "agent_stream_complete", Kind: events.KindStreamEnd, Timestamp: now.Add(3 * time.Second), SourceID: "agent2"})

	output := tv.RenderParallelSummary()

	// Should indicate no parallel execution
	if !strings.Contains(output, "No parallel execution") {
		t.Error("Expected 'No parallel execution' message for sequential agents")
	}
}

func TestRenderParallelSummary_Parallel(t *testing.T) {
	tv := NewTimelineVisualizer()
	now := time.Now()

	// Agent 1 starts
	tv.AddEvent(&events.Event{Name: "agent_stream_start", Kind: events.KindStreamStart, Timestamp: now, SourceID: "agent1"})

	// Agent 2 starts while agent 1 is running
	tv.AddEvent(&events.Event{Name: "agent_stream_start", Kind: events.KindStreamStart, Timestamp: now.Add(500 * time.Millisecond), SourceID: "agent2"})

	// Both complete
	tv.AddEvent(&events.Event{Name: "agent_stream_complete", Kind: events.KindStreamEnd, Timestamp: now.Add(2 * time.Second), SourceID: "agent1"})
	tv.AddEvent(&events.Event{Name: "agent_stream_complete", Kind: events.KindStreamEnd, Timestamp: now.Add(2500 * time.Millisecond), SourceID: "agent2"})

	output := tv.RenderParallelSummary()

	// Should show parallel execution
	if !strings.Contains(output, "ran in parallel with") {
		t.Error("Expected parallel execution message")
	}

	if !strings.Contains(output, "agent1") && !strings.Contains(output, "agent2") {
		t.Error("Expected agent names in parallel summary")
	}
}

// TestGetAgentColor checks the generic contract getAgentColor must hold
// for ANY dialect's agent IDs — determinism and a sane empty-ID default
// — rather than pinning specific persona-name-to-color assignments.
func TestGetAgentColor(t *testing.T) {
	if got := getAgentColor("", nil); got != "7" {
		t.Errorf("expected default gray %q for empty agent ID, got %q", "7", got)
	}

	for _, id := range []string{"agent_a", "some-agent-from-a-strangers-dialect", "assistant"} {
		first := getAgentColor(id, nil)
		if first == "" {
			t.Errorf("expected a non-empty color for agent %q", id)
		}
		for i := 0; i < 5; i++ {
			if got := getAgentColor(id, nil); got != first {
				t.Errorf("expected stable color for repeated calls with %q, got %q then %q", id, first, got)
			}
		}
	}
}

// TestGetAgentColor_AgentColorsOverride proves an agent_colors entry
// wins over the hash-based default, while an agent absent from the
// override map still gets a hash-derived color, not flat gray — the two
// halves architecture.md describes as one function's job (this task
// unified them; previously only interactive.go's copy had the override
// half, and only tui.go's had the hash half).
func TestGetAgentColor_AgentColorsOverride(t *testing.T) {
	overrides := map[string]string{"agent_a": "42"}

	if got := getAgentColor("agent_a", overrides); got != "42" {
		t.Errorf("expected the configured override %q for agent_a, got %q", "42", got)
	}

	// An agent absent from the override map must still get a
	// hash-derived color — not the override map's absence collapsing it
	// to flat gray (interactive.go's pre-unification bug).
	unconfigured := getAgentColor("agent_b", overrides)
	hashOnly := getAgentColor("agent_b", nil)
	if unconfigured != hashOnly {
		t.Errorf("expected agent_b (absent from overrides) to get the same hash-derived color as with no overrides at all: got %q, want %q", unconfigured, hashOnly)
	}
	if unconfigured == "7" {
		t.Error("expected agent_b to get a hash-spread color, not flat gray, despite an (unrelated) agent_colors map being present")
	}
}

func TestTimelineEntry_ExtractTimestamps(t *testing.T) {
	tv := NewTimelineVisualizer()
	now := time.Now()

	// Test all event types extract timestamps correctly
	eventTypes := []struct {
		name  string
		event *events.Event
	}{
		{
			"AgentContent",
			&events.Event{Name: "agent_content", Timestamp: now, SourceID: "test", Content: "data"},
		},
		{
			"AggregatorContent",
			&events.Event{Name: "synth_content", Kind: events.KindContent, Timestamp: now, SourceID: "aggregator", Content: "synthesis"},
		},
		{
			"Error",
			&events.Event{Name: "error", Timestamp: now, Fields: map[string]any{"error_type": "rate_limit_error"}},
		},
	}

	for _, tt := range eventTypes {
		t.Run(tt.name, func(t *testing.T) {
			tv.entries = nil // Reset
			tv.AddEvent(tt.event)

			if len(tv.entries) != 1 {
				t.Fatalf("Expected 1 entry, got %d", len(tv.entries))
			}

			if !tv.entries[0].Timestamp.Equal(now) {
				t.Errorf("Expected timestamp %v, got %v", now, tv.entries[0].Timestamp)
			}
		})
	}
}
