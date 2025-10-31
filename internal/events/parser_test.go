package events

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParser_ParseAgentContent(t *testing.T) {
	parser := NewParser()

	data := []byte(`{
		"type": "agent_content",
		"agent_id": "sam_harris",
		"content": "Hello",
		"sequence": 42,
		"message_id": "msg_123",
		"timestamp": "2025-01-28T12:00:00Z"
	}`)

	event, err := parser.Parse("agent_content", data)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if event.Type != AgentContent {
		t.Errorf("Expected type %v, got %v", AgentContent, event.Type)
	}

	if event.AgentContent.AgentID != "sam_harris" {
		t.Errorf("Expected agent_id 'sam_harris', got '%s'", event.AgentContent.AgentID)
	}

	if event.AgentContent.Content != "Hello" {
		t.Errorf("Expected content 'Hello', got '%s'", event.AgentContent.Content)
	}

	if event.AgentContent.Sequence != 42 {
		t.Errorf("Expected sequence 42, got %d", event.AgentContent.Sequence)
	}
}

func TestParser_ParseSessionStart(t *testing.T) {
	parser := NewParser()

	data := []byte(`{
		"type": "session_start",
		"session_id": "test-123",
		"message_id": "msg_123",
		"timestamp": "2025-01-28T12:00:00Z"
	}`)

	event, err := parser.Parse("session_start", data)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if event.Type != SessionStart {
		t.Errorf("Expected type %v, got %v", SessionStart, event.Type)
	}

	if event.SessionStart.SessionID != "test-123" {
		t.Errorf("Expected session_id 'test-123', got '%s'", event.SessionStart.SessionID)
	}
}

func TestParser_ParseError(t *testing.T) {
	parser := NewParser()

	data := []byte(`{
		"type": "error",
		"error_type": "rate_limit_error",
		"message": "Rate limit exceeded",
		"details": "Too many requests",
		"agent_id": "sam_harris",
		"retry_after": 60,
		"message_id": "msg_123",
		"timestamp": "2025-01-28T12:00:00Z"
	}`)

	event, err := parser.Parse("error", data)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if event.Type != Error {
		t.Errorf("Expected type %v, got %v", Error, event.Type)
	}

	if event.Error.ErrorType != RateLimitError {
		t.Errorf("Expected error_type %v, got %v", RateLimitError, event.Error.ErrorType)
	}

	if event.Error.RetryAfter != 60 {
		t.Errorf("Expected retry_after 60, got %d", event.Error.RetryAfter)
	}
}

func TestParser_ParseRaw(t *testing.T) {
	parser := NewParser()

	data := []byte(`{
		"type": "agent_content",
		"agent_id": "sam_harris",
		"content": "Test",
		"sequence": 1,
		"message_id": "msg_123",
		"timestamp": "2025-01-28T12:00:00Z"
	}`)

	event, err := parser.ParseRaw(data)
	if err != nil {
		t.Fatalf("Failed to parse raw: %v", err)
	}

	if event.Type != AgentContent {
		t.Errorf("Expected type %v, got %v", AgentContent, event.Type)
	}
}

func TestParser_UnknownEventType(t *testing.T) {
	parser := NewParser()

	data := []byte(`{"type": "unknown"}`)

	_, err := parser.Parse("unknown", data)
	if err == nil {
		t.Fatal("Expected error for unknown event type, got nil")
	}
}

func TestParser_InvalidJSON(t *testing.T) {
	parser := NewParser()

	data := []byte(`invalid json`)

	_, err := parser.Parse("agent_content", data)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestEvent_GetAgentID(t *testing.T) {
	tests := []struct {
		name     string
		event    *Event
		expected string
	}{
		{
			name: "AgentStreamStart",
			event: &Event{
				Type: AgentStreamStart,
				AgentStreamStart: &AgentStreamStartEvent{
					AgentID: "sam_harris",
				},
			},
			expected: "sam_harris",
		},
		{
			name: "AgentContent",
			event: &Event{
				Type: AgentContent,
				AgentContent: &AgentContentEvent{
					AgentID: "tony_robbins",
				},
			},
			expected: "tony_robbins",
		},
		{
			name: "SessionStart (no agent)",
			event: &Event{
				Type:         SessionStart,
				SessionStart: &SessionStartEvent{},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentID := tt.event.GetAgentID()
			if agentID != tt.expected {
				t.Errorf("Expected agent_id '%s', got '%s'", tt.expected, agentID)
			}
		})
	}
}

func TestEvent_GetContent(t *testing.T) {
	tests := []struct {
		name     string
		event    *Event
		expected string
	}{
		{
			name: "AgentContent",
			event: &Event{
				Type: AgentContent,
				AgentContent: &AgentContentEvent{
					Content: "Hello world",
				},
			},
			expected: "Hello world",
		},
		{
			name: "WizardContent",
			event: &Event{
				Type: WizardContent,
				WizardContent: &WizardContentEvent{
					Content: "Synthesizing",
				},
			},
			expected: "Synthesizing",
		},
		{
			name: "SessionStart (no content)",
			event: &Event{
				Type: SessionStart,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := tt.event.GetContent()
			if content != tt.expected {
				t.Errorf("Expected content '%s', got '%s'", tt.expected, content)
			}
		})
	}
}

func TestEvent_GetSequence(t *testing.T) {
	tests := []struct {
		name     string
		event    *Event
		expected int
	}{
		{
			name: "AgentContent with sequence",
			event: &Event{
				Type: AgentContent,
				AgentContent: &AgentContentEvent{
					Sequence: 42,
				},
			},
			expected: 42,
		},
		{
			name: "WizardContent with sequence",
			event: &Event{
				Type: WizardContent,
				WizardContent: &WizardContentEvent{
					Sequence: 99,
				},
			},
			expected: 99,
		},
		{
			name: "SessionStart (no sequence)",
			event: &Event{
				Type: SessionStart,
			},
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seq := tt.event.GetSequence()
			if seq != tt.expected {
				t.Errorf("Expected sequence %d, got %d", tt.expected, seq)
			}
		})
	}
}

func TestEvent_IsStreamingContent(t *testing.T) {
	tests := []struct {
		name     string
		event    *Event
		expected bool
	}{
		{
			name:     "AgentContent",
			event:    &Event{Type: AgentContent},
			expected: true,
		},
		{
			name:     "WizardContent",
			event:    &Event{Type: WizardContent},
			expected: true,
		},
		{
			name:     "SessionStart",
			event:    &Event{Type: SessionStart},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.event.IsStreamingContent()
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestEvent_IsError(t *testing.T) {
	tests := []struct {
		name     string
		event    *Event
		expected bool
	}{
		{
			name:     "Error event",
			event:    &Event{Type: Error},
			expected: true,
		},
		{
			name:     "Non-error event",
			event:    &Event{Type: AgentContent},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.event.IsError()
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestAllEventTypes(t *testing.T) {
	parser := NewParser()
	now := time.Now()

	eventTypes := []struct {
		eventType EventType
		data      interface{}
	}{
		{SessionStart, SessionStartEvent{
			BaseEvent: BaseEvent{Type: SessionStart, Timestamp: now},
			SessionID: "test",
		}},
		{SessionComplete, SessionCompleteEvent{
			BaseEvent: BaseEvent{Type: SessionComplete, Timestamp: now},
			SessionID: "test",
		}},
		{AgentStreamStart, AgentStreamStartEvent{
			BaseEvent: BaseEvent{Type: AgentStreamStart, Timestamp: now},
			AgentID:   "sam_harris",
		}},
		{AgentContent, AgentContentEvent{
			BaseEvent: BaseEvent{Type: AgentContent, Timestamp: now},
			AgentID:   "sam_harris",
			Content:   "test",
			Sequence:  1,
		}},
		{AgentStreamComplete, AgentStreamCompleteEvent{
			BaseEvent:  BaseEvent{Type: AgentStreamComplete, Timestamp: now},
			AgentID:    "sam_harris",
			TokenCount: 100,
		}},
		{WizardStreamStart, WizardStreamStartEvent{
			BaseEvent: BaseEvent{Type: WizardStreamStart, Timestamp: now},
		}},
		{WizardContent, WizardContentEvent{
			BaseEvent: BaseEvent{Type: WizardContent, Timestamp: now},
			Content:   "synthesis",
			Sequence:  1,
		}},
		{WizardStreamComplete, WizardStreamCompleteEvent{
			BaseEvent:  BaseEvent{Type: WizardStreamComplete, Timestamp: now},
			TokenCount: 200,
		}},
		{Error, ErrorEvent{
			BaseEvent: BaseEvent{Type: Error, Timestamp: now},
			ErrorType: RateLimitError,
			Message:   "test error",
		}},
	}

	for _, tt := range eventTypes {
		t.Run(string(tt.eventType), func(t *testing.T) {
			data, err := json.Marshal(tt.data)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			event, err := parser.Parse(string(tt.eventType), data)
			if err != nil {
				t.Fatalf("Failed to parse: %v", err)
			}

			if event.Type != tt.eventType {
				t.Errorf("Expected type %v, got %v", tt.eventType, event.Type)
			}
		})
	}
}
