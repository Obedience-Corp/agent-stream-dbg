package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
)

func TestNewStructuredLogger(t *testing.T) {
	// Create temp directory for logs
	tmpDir := t.TempDir()

	cfg := &config.EnhancedConfig{
		LogDir:  tmpDir,
		Session: config.SessionConfig{ID: "test-session"},
	}

	logger, err := NewStructuredLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	// Verify directories were created
	dirs := []string{
		filepath.Join(tmpDir, "by-event-type"),
		filepath.Join(tmpDir, "by-agent"),
		filepath.Join(tmpDir, "by-session"),
		filepath.Join(tmpDir, "api-calls"),
	}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Expected directory %s to exist", dir)
		}
	}
}

func TestLogEvent_AgentContent(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.EnhancedConfig{
		LogDir:  tmpDir,
		Session: config.SessionConfig{ID: "test"},
	}

	logger, err := NewStructuredLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	// Create test event
	event := &events.Event{
		Type: events.AgentContent,
		Raw:  []byte(`{"type":"agent_content","agent_id":"sam_harris","content":"test"}`),
		AgentContent: &events.AgentContentEvent{
			BaseEvent: events.BaseEvent{
				Type:      events.AgentContent,
				Timestamp: time.Now(),
			},
			AgentID: "sam_harris",
			Content: "test",
		},
	}

	err = logger.LogEvent(event)
	if err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	// Verify log files were created
	eventTypeLog := filepath.Join(tmpDir, "by-event-type", "agent_content.jsonl")
	agentLog := filepath.Join(tmpDir, "by-agent", "sam_harris.jsonl")

	if _, err := os.Stat(eventTypeLog); os.IsNotExist(err) {
		t.Errorf("Expected event type log file to exist: %s", eventTypeLog)
	}

	if _, err := os.Stat(agentLog); os.IsNotExist(err) {
		t.Errorf("Expected agent log file to exist: %s", agentLog)
	}
}

func TestLogEvent_WizardContent(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.EnhancedConfig{
		LogDir:  tmpDir,
		Session: config.SessionConfig{ID: "test"},
	}

	logger, err := NewStructuredLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	event := &events.Event{
		Type: events.WizardContent,
		Raw:  []byte(`{"type":"wizard_content","content":"synthesis"}`),
		WizardContent: &events.WizardContentEvent{
			BaseEvent: events.BaseEvent{
				Type:      events.WizardContent,
				Timestamp: time.Now(),
			},
			Content: "synthesis",
		},
	}

	err = logger.LogEvent(event)
	if err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	// Verify wizard log was created
	wizardLog := filepath.Join(tmpDir, "by-agent", "wizard.jsonl")
	if _, err := os.Stat(wizardLog); os.IsNotExist(err) {
		t.Errorf("Expected wizard log file to exist: %s", wizardLog)
	}
}

func TestLogEvent_SessionEvents(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.EnhancedConfig{
		LogDir:  tmpDir,
		Session: config.SessionConfig{ID: "test"},
	}

	logger, err := NewStructuredLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	event := &events.Event{
		Type: events.SessionStart,
		Raw:  []byte(`{"type":"session_start","session_id":"test"}`),
		SessionStart: &events.SessionStartEvent{
			BaseEvent: events.BaseEvent{
				Type:      events.SessionStart,
				Timestamp: time.Now(),
			},
			SessionID: "test",
		},
	}

	err = logger.LogEvent(event)
	if err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	// Session events should be logged to event type and session logs
	eventTypeLog := filepath.Join(tmpDir, "by-event-type", "session_start.jsonl")
	if _, err := os.Stat(eventTypeLog); os.IsNotExist(err) {
		t.Errorf("Expected event type log file to exist: %s", eventTypeLog)
	}
}

func TestLogAPICall(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.EnhancedConfig{
		LogDir:  tmpDir,
		Session: config.SessionConfig{ID: "test"},
	}

	logger, err := NewStructuredLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	logger.LogAPICall("POST", "http://localhost:5003/api/stream", 200, 123*time.Millisecond, nil)

	// Verify API call log was created
	apiLogPattern := filepath.Join(tmpDir, "api-calls", "http_*.jsonl")
	matches, err := filepath.Glob(apiLogPattern)
	if err != nil {
		t.Fatalf("Failed to glob: %v", err)
	}

	if len(matches) == 0 {
		t.Errorf("Expected API call log file to exist matching pattern: %s", apiLogPattern)
	}
}

func TestClose(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.EnhancedConfig{
		LogDir:  tmpDir,
		Session: config.SessionConfig{ID: "test"},
	}

	logger, err := NewStructuredLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Log some events to open files
	event := &events.Event{
		Type: events.AgentContent,
		Raw:  []byte(`{"type":"agent_content"}`),
		AgentContent: &events.AgentContentEvent{
			AgentID: "test_agent",
		},
	}
	_ = logger.LogEvent(event)

	// Close should not error
	err = logger.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Second close should not panic
	_ = logger.Close()
	// We allow double close to error or succeed
}

func TestMultiDimensionalLogging(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.EnhancedConfig{
		LogDir:  tmpDir,
		Session: config.SessionConfig{ID: "test"},
	}

	logger, err := NewStructuredLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	// Log multiple agents
	agents := []string{"sam_harris", "tony_robbins", "wizard"}
	for _, agentID := range agents {
		event := &events.Event{
			Type: events.AgentContent,
			Raw:  []byte(`{"type":"agent_content"}`),
			AgentContent: &events.AgentContentEvent{
				AgentID: agentID,
			},
		}
		_ = logger.LogEvent(event)
	}

	// Verify all agent logs were created
	for _, agentID := range agents {
		agentLog := filepath.Join(tmpDir, "by-agent", agentID+".jsonl")
		if _, err := os.Stat(agentLog); os.IsNotExist(err) {
			t.Errorf("Expected agent log for %s to exist", agentID)
		}
	}

	// Verify event type log exists
	eventTypeLog := filepath.Join(tmpDir, "by-event-type", "agent_content.jsonl")
	if _, err := os.Stat(eventTypeLog); os.IsNotExist(err) {
		t.Errorf("Expected event type log to exist")
	}
}
