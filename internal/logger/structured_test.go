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
		Logging: config.LoggingConfig{Dimensions: config.DimensionsConfig{ByEventType: true, ByAgent: true, BySession: true, APICalls: true}},
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
		Logging: config.LoggingConfig{Dimensions: config.DimensionsConfig{ByEventType: true, ByAgent: true, BySession: true, APICalls: true}},
	}

	logger, err := NewStructuredLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	// Create test event
	event := &events.Event{
		Name:      "agent_content",
		Kind:      events.KindContent,
		Timestamp: time.Now(),
		SourceID:  "agent_a",
		Content:   "test",
		Raw:       []byte(`{"type":"agent_content","agent_id":"agent_a","content":"test"}`),
	}

	err = logger.LogEvent(event)
	if err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	// Verify log files were created
	eventTypeLog := filepath.Join(tmpDir, "by-event-type", "agent_content.jsonl")
	agentLog := filepath.Join(tmpDir, "by-agent", "agent_a.jsonl")

	if _, err := os.Stat(eventTypeLog); os.IsNotExist(err) {
		t.Errorf("Expected event type log file to exist: %s", eventTypeLog)
	}

	if _, err := os.Stat(agentLog); os.IsNotExist(err) {
		t.Errorf("Expected agent log file to exist: %s", agentLog)
	}
}

func TestLogEvent_AggregatorSourceContent(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.EnhancedConfig{
		LogDir:  tmpDir,
		Session: config.SessionConfig{ID: "test"},
		Logging: config.LoggingConfig{Dimensions: config.DimensionsConfig{ByEventType: true, ByAgent: true, BySession: true, APICalls: true}},
	}

	logger, err := NewStructuredLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	event := &events.Event{
		Name:      "synth_content",
		Kind:      events.KindContent,
		Timestamp: time.Now(),
		SourceID:  "aggregator",
		Content:   "synthesis",
		Raw:       []byte(`{"type":"synth_content","content":"synthesis"}`),
	}

	err = logger.LogEvent(event)
	if err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	// Verify the aggregator's own log file was created — same
	// per-source splitting any agent ID gets, no special case.
	aggregatorLog := filepath.Join(tmpDir, "by-agent", "aggregator.jsonl")
	if _, err := os.Stat(aggregatorLog); os.IsNotExist(err) {
		t.Errorf("Expected aggregator log file to exist: %s", aggregatorLog)
	}
}

func TestLogEvent_SessionEvents(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.EnhancedConfig{
		LogDir:  tmpDir,
		Session: config.SessionConfig{ID: "test"},
		Logging: config.LoggingConfig{Dimensions: config.DimensionsConfig{ByEventType: true, ByAgent: true, BySession: true, APICalls: true}},
	}

	logger, err := NewStructuredLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	event := &events.Event{
		Name:      "session_start",
		Kind:      events.KindSessionStart,
		Timestamp: time.Now(),
		Fields:    map[string]any{"session_id": "test"},
		Raw:       []byte(`{"type":"session_start","session_id":"test"}`),
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
		Logging: config.LoggingConfig{Dimensions: config.DimensionsConfig{ByEventType: true, ByAgent: true, BySession: true, APICalls: true}},
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
		Logging: config.LoggingConfig{Dimensions: config.DimensionsConfig{ByEventType: true, ByAgent: true, BySession: true, APICalls: true}},
	}

	logger, err := NewStructuredLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Log some events to open files
	event := &events.Event{
		Name:     "agent_content",
		Kind:     events.KindContent,
		SourceID: "test_agent",
		Raw:      []byte(`{"type":"agent_content"}`),
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
		Logging: config.LoggingConfig{Dimensions: config.DimensionsConfig{ByEventType: true, ByAgent: true, BySession: true, APICalls: true}},
	}

	logger, err := NewStructuredLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	// Log multiple agents
	agents := []string{"agent_a", "agent_b", "aggregator"}
	for _, agentID := range agents {
		event := &events.Event{
			Name:     "agent_content",
			Kind:     events.KindContent,
			SourceID: agentID,
			Raw:      []byte(`{"type":"agent_content"}`),
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

func TestLogEvent_RespectsDisabledDimensions(t *testing.T) {
	tmpDir := t.TempDir()

	// Only by_event_type enabled: by_agent, by_session, api_calls all off.
	cfg := &config.EnhancedConfig{
		LogDir:  tmpDir,
		Session: config.SessionConfig{ID: "test"},
		Logging: config.LoggingConfig{Dimensions: config.DimensionsConfig{ByEventType: true}},
	}

	logger, err := NewStructuredLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	event := &events.Event{
		Name:     "agent_content",
		Kind:     events.KindContent,
		SourceID: "agent_a",
		Raw:      []byte(`{"type":"agent_content","agent_id":"agent_a"}`),
	}
	if err := logger.LogEvent(event); err != nil {
		t.Fatalf("LogEvent: %v", err)
	}
	logger.LogAPICall("GET", "http://example.com", 200, 0, nil)

	if _, err := os.Stat(filepath.Join(tmpDir, "by-event-type", "agent_content.jsonl")); os.IsNotExist(err) {
		t.Error("expected by-event-type log to exist (dimension enabled)")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "by-agent", "agent_a.jsonl")); err == nil {
		t.Error("expected by-agent log NOT to exist (dimension disabled)")
	}
	sessionFiles, _ := filepath.Glob(filepath.Join(tmpDir, "by-session", "*.jsonl"))
	for _, f := range sessionFiles {
		if info, statErr := os.Stat(f); statErr == nil && info.Size() > 0 {
			t.Errorf("expected by-session log to stay empty (dimension disabled), got content in %s", f)
		}
	}
	apiFiles, _ := filepath.Glob(filepath.Join(tmpDir, "api-calls", "*.jsonl"))
	for _, f := range apiFiles {
		if info, statErr := os.Stat(f); statErr == nil && info.Size() > 0 {
			t.Errorf("expected api-calls log to stay empty (dimension disabled), got content in %s", f)
		}
	}
}
