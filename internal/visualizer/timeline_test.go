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
		Type: events.AgentStreamStart,
		AgentStreamStart: &events.AgentStreamStartEvent{
			BaseEvent: events.BaseEvent{
				Type:      events.AgentStreamStart,
				Timestamp: now,
			},
			AgentID: "sam_harris",
		},
	}

	tv.AddEvent(event)

	if len(tv.entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(tv.entries))
	}

	entry := tv.entries[0]
	if entry.AgentID != "sam_harris" {
		t.Errorf("Expected agent_id 'sam_harris', got '%s'", entry.AgentID)
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
	events := []*events.Event{
		{
			Type: events.AgentStreamStart,
			AgentStreamStart: &events.AgentStreamStartEvent{
				BaseEvent: events.BaseEvent{Timestamp: now},
				AgentID:   "agent1",
			},
		},
		{
			Type: events.AgentStreamStart,
			AgentStreamStart: &events.AgentStreamStartEvent{
				BaseEvent: events.BaseEvent{Timestamp: later},
				AgentID:   "agent2",
			},
		},
		{
			Type: events.AgentStreamStart,
			AgentStreamStart: &events.AgentStreamStartEvent{
				BaseEvent: events.BaseEvent{Timestamp: earlier},
				AgentID:   "agent3",
			},
		},
	}

	for _, event := range events {
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
		Type: events.AgentStreamStart,
		AgentStreamStart: &events.AgentStreamStartEvent{
			BaseEvent: events.BaseEvent{Timestamp: now},
			AgentID:   "sam_harris",
		},
	})

	tv.AddEvent(&events.Event{
		Type: events.AgentContent,
		AgentContent: &events.AgentContentEvent{
			BaseEvent: events.BaseEvent{Timestamp: now.Add(100 * time.Millisecond)},
			AgentID:   "sam_harris",
			Content:   "test",
		},
	})

	tv.AddEvent(&events.Event{
		Type: events.AgentStreamComplete,
		AgentStreamComplete: &events.AgentStreamCompleteEvent{
			BaseEvent: events.BaseEvent{Timestamp: now.Add(200 * time.Millisecond)},
			AgentID:   "sam_harris",
		},
	})

	output := tv.RenderTimeline(100)

	// Verify output contains key elements
	if !strings.Contains(output, "Timeline View") {
		t.Error("Expected timeline header")
	}

	if !strings.Contains(output, "sam_harris") {
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
		Type: events.AgentContent,
		AgentContent: &events.AgentContentEvent{
			BaseEvent: events.BaseEvent{Timestamp: now},
			AgentID:   "sam_harris",
			Content:   "Hello world",
		},
	})

	output := tv.RenderDetailedLog()

	// Verify output
	if !strings.Contains(output, "Detailed Event Log") {
		t.Error("Expected detailed log header")
	}

	if !strings.Contains(output, "sam_harris") {
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
	tv.AddEvent(&events.Event{
		Type: events.AgentStreamStart,
		AgentStreamStart: &events.AgentStreamStartEvent{
			BaseEvent: events.BaseEvent{Timestamp: now},
			AgentID:   "agent1",
		},
	})

	tv.AddEvent(&events.Event{
		Type: events.AgentStreamComplete,
		AgentStreamComplete: &events.AgentStreamCompleteEvent{
			BaseEvent: events.BaseEvent{Timestamp: now.Add(1 * time.Second)},
			AgentID:   "agent1",
		},
	})

	// Agent 2 starts after agent 1 completes
	tv.AddEvent(&events.Event{
		Type: events.AgentStreamStart,
		AgentStreamStart: &events.AgentStreamStartEvent{
			BaseEvent: events.BaseEvent{Timestamp: now.Add(2 * time.Second)},
			AgentID:   "agent2",
		},
	})

	tv.AddEvent(&events.Event{
		Type: events.AgentStreamComplete,
		AgentStreamComplete: &events.AgentStreamCompleteEvent{
			BaseEvent: events.BaseEvent{Timestamp: now.Add(3 * time.Second)},
			AgentID:   "agent2",
		},
	})

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
	tv.AddEvent(&events.Event{
		Type: events.AgentStreamStart,
		AgentStreamStart: &events.AgentStreamStartEvent{
			BaseEvent: events.BaseEvent{Timestamp: now},
			AgentID:   "agent1",
		},
	})

	// Agent 2 starts while agent 1 is running
	tv.AddEvent(&events.Event{
		Type: events.AgentStreamStart,
		AgentStreamStart: &events.AgentStreamStartEvent{
			BaseEvent: events.BaseEvent{Timestamp: now.Add(500 * time.Millisecond)},
			AgentID:   "agent2",
		},
	})

	// Both complete
	tv.AddEvent(&events.Event{
		Type: events.AgentStreamComplete,
		AgentStreamComplete: &events.AgentStreamCompleteEvent{
			BaseEvent: events.BaseEvent{Timestamp: now.Add(2 * time.Second)},
			AgentID:   "agent1",
		},
	})

	tv.AddEvent(&events.Event{
		Type: events.AgentStreamComplete,
		AgentStreamComplete: &events.AgentStreamCompleteEvent{
			BaseEvent: events.BaseEvent{Timestamp: now.Add(2500 * time.Millisecond)},
			AgentID:   "agent2",
		},
	})

	output := tv.RenderParallelSummary()

	// Should show parallel execution
	if !strings.Contains(output, "ran in parallel with") {
		t.Error("Expected parallel execution message")
	}

	if !strings.Contains(output, "agent1") && !strings.Contains(output, "agent2") {
		t.Error("Expected agent names in parallel summary")
	}
}

func TestGetAgentColor(t *testing.T) {
	tests := []struct {
		agentID string
		want    string
	}{
		{"sam_harris", "13"},
		{"tony_robbins", "9"},
		{"wizard", "11"},
		{"unknown_agent", "7"}, // Default
	}

	for _, tt := range tests {
		t.Run(tt.agentID, func(t *testing.T) {
			color := getAgentColor(tt.agentID)
			if color != tt.want {
				t.Errorf("Expected color %s for %s, got %s", tt.want, tt.agentID, color)
			}
		})
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
			&events.Event{
				Type: events.AgentContent,
				AgentContent: &events.AgentContentEvent{
					BaseEvent: events.BaseEvent{Timestamp: now},
					AgentID:   "test",
					Content:   "data",
				},
			},
		},
		{
			"WizardContent",
			&events.Event{
				Type: events.WizardContent,
				WizardContent: &events.WizardContentEvent{
					BaseEvent: events.BaseEvent{Timestamp: now},
					Content:   "synthesis",
				},
			},
		},
		{
			"Error",
			&events.Event{
				Type: events.Error,
				Error: &events.ErrorEvent{
					BaseEvent: events.BaseEvent{Timestamp: now},
					ErrorType: events.RateLimitError,
				},
			},
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
