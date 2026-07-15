package events

import (
	"testing"
)

func TestParser_InvalidJSON(t *testing.T) {
	parser := NewParser()

	_, err := parser.Parse("agent_content", []byte(`invalid json`))
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestParser_UnknownEventType(t *testing.T) {
	parser := NewParser()

	_, err := parser.Parse("unknown", []byte(`{"type": "unknown"}`))
	if err == nil {
		t.Fatal("Expected error for unknown event type, got nil")
	}
}

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

	if event.Kind != KindContent {
		t.Errorf("Expected kind %v, got %v", KindContent, event.Kind)
	}
	if event.SourceID != "sam_harris" {
		t.Errorf("Expected source 'sam_harris', got '%s'", event.SourceID)
	}
	if event.Content != "Hello" {
		t.Errorf("Expected content 'Hello', got '%s'", event.Content)
	}
	if event.Seq != 42 {
		t.Errorf("Expected sequence 42, got %d", event.Seq)
	}
}

func TestParser_ParseWizardContentSetsPseudoAgent(t *testing.T) {
	parser := NewParser()

	data := []byte(`{"type": "wizard_content", "content": "synthesis", "sequence": 1}`)

	event, err := parser.Parse("wizard_content", data)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}
	if event.SourceID != "wizard" {
		t.Errorf("Expected wizard events to have SourceID 'wizard', got '%s'", event.SourceID)
	}
	if event.Kind != KindContent {
		t.Errorf("Expected kind %v, got %v", KindContent, event.Kind)
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
	if event.Kind != KindSessionStart {
		t.Errorf("Expected kind %v, got %v", KindSessionStart, event.Kind)
	}
	if event.StringField("session_id") != "test-123" {
		t.Errorf("Expected session_id 'test-123', got '%s'", event.StringField("session_id"))
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
	if event.Kind != KindError {
		t.Errorf("Expected kind %v, got %v", KindError, event.Kind)
	}
	if event.StringField("error_type") != "rate_limit_error" {
		t.Errorf("Expected error_type 'rate_limit_error', got '%s'", event.StringField("error_type"))
	}
	if event.IntField("retry_after") != 60 {
		t.Errorf("Expected retry_after 60, got %d", event.IntField("retry_after"))
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
	if event.Kind != KindContent {
		t.Errorf("Expected kind %v, got %v", KindContent, event.Kind)
	}
}

func TestParser_ParseRaw_NoTypeField(t *testing.T) {
	parser := NewParser()

	event, err := parser.ParseRaw([]byte(`{"foo": "bar"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Kind != KindUnknown {
		t.Errorf("Expected kind %v for payload with no type field, got %v", KindUnknown, event.Kind)
	}
}

func TestAllEventTypesMapToExpectedKind(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		eventType string
		wantKind  Kind
	}{
		{"session_start", KindSessionStart},
		{"session_complete", KindSessionEnd},
		{"agent_stream_start", KindStreamStart},
		{"agent_content", KindContent},
		{"agent_stream_complete", KindStreamEnd},
		{"wizard_stream_start", KindStreamStart},
		{"wizard_content", KindContent},
		{"wizard_stream_complete", KindStreamEnd},
		{"error", KindError},
		{"flow_step_start", KindStepStart},
		{"flow_step_end", KindStepEnd},
		{"flow_step_detail", KindDetail},
		{"prompt_info", KindDetail},
		{"prompt_full", KindDetail},
		{"flow_config", KindTopology},
		{"filter_detail", KindDetail},
		{"perspective_detail", KindDetail},
		{"synthesis_detail", KindDetail},
		{"agent_metadata", KindUsage},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			event, err := parser.Parse(tt.eventType, []byte(`{"type":"`+tt.eventType+`"}`))
			if err != nil {
				t.Fatalf("Failed to parse %s: %v", tt.eventType, err)
			}
			if event.Kind != tt.wantKind {
				t.Errorf("Expected kind %v for %s, got %v", tt.wantKind, tt.eventType, event.Kind)
			}
			if event.Name != tt.eventType {
				t.Errorf("Expected Name %q, got %q", tt.eventType, event.Name)
			}
		})
	}
}
