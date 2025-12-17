package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/rs/zerolog"
)

// StructuredLogger handles multi-dimensional logging
type StructuredLogger struct {
	config *config.Config

	// Log files organized by different dimensions
	eventTypeFiles map[string]*os.File
	agentFiles     map[string]*os.File
	sessionFile    *os.File
	apiCallFile    *os.File

	// Loggers for each dimension
	eventTypeLoggers map[string]zerolog.Logger
	agentLoggers     map[string]zerolog.Logger
	sessionLogger    zerolog.Logger
	apiLogger        zerolog.Logger

	mu sync.Mutex
}

// NewStructuredLogger creates a new multi-dimensional logger
func NewStructuredLogger(cfg *config.Config) (*StructuredLogger, error) {
	sl := &StructuredLogger{
		config:           cfg,
		eventTypeFiles:   make(map[string]*os.File),
		agentFiles:       make(map[string]*os.File),
		eventTypeLoggers: make(map[string]zerolog.Logger),
		agentLoggers:     make(map[string]zerolog.Logger),
	}

	// Ensure all log directories exist
	dirs := []string{
		filepath.Join(cfg.LogDir),
		filepath.Join(cfg.LogDir, "by-event-type"),
		filepath.Join(cfg.LogDir, "by-agent"),
		filepath.Join(cfg.LogDir, "by-session"),
		filepath.Join(cfg.LogDir, "api-calls"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Create session log file
	sessionLogPath := filepath.Join(
		cfg.LogDir,
		"by-session",
		fmt.Sprintf("session_%s_%s.jsonl", cfg.SessionID, time.Now().Format("20060102_150405")),
	)
	sessionFile, err := os.OpenFile(sessionLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create session log file: %w", err)
	}
	sl.sessionFile = sessionFile
	sl.sessionLogger = zerolog.New(sessionFile).With().Timestamp().Logger()

	// Create API call log file
	apiLogPath := filepath.Join(
		cfg.LogDir,
		"api-calls",
		fmt.Sprintf("http_%s.jsonl", time.Now().Format("20060102_150405")),
	)
	apiFile, err := os.OpenFile(apiLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		sessionFile.Close()
		return nil, fmt.Errorf("failed to create API log file: %w", err)
	}
	sl.apiCallFile = apiFile
	sl.apiLogger = zerolog.New(apiFile).With().Timestamp().Logger()

	return sl, nil
}

// LogEvent logs an event to all relevant dimensions
func (sl *StructuredLogger) LogEvent(event *events.Event) error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	// 1. Log to event type dimension
	if err := sl.logToEventType(event); err != nil {
		return fmt.Errorf("failed to log to event type: %w", err)
	}

	// 2. Log to agent dimension (if agent-related)
	if agentID := event.GetAgentID(); agentID != "" {
		if err := sl.logToAgent(event, agentID); err != nil {
			return fmt.Errorf("failed to log to agent: %w", err)
		}
	}

	// Special case for wizard events
	if event.Type == events.WizardStreamStart ||
		event.Type == events.WizardContent ||
		event.Type == events.WizardStreamComplete {
		if err := sl.logToAgent(event, "wizard"); err != nil {
			return fmt.Errorf("failed to log to wizard: %w", err)
		}
	}

	// 3. Log to session timeline
	if err := sl.logToSession(event); err != nil {
		return fmt.Errorf("failed to log to session: %w", err)
	}

	return nil
}

// logToEventType logs to event-type-specific file
func (sl *StructuredLogger) logToEventType(event *events.Event) error {
	eventType := string(event.Type)

	// Get or create logger for this event type
	logger, exists := sl.eventTypeLoggers[eventType]
	if !exists {
		logPath := filepath.Join(sl.config.LogDir, "by-event-type", fmt.Sprintf("%s.jsonl", eventType))
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to create event type log file: %w", err)
		}
		sl.eventTypeFiles[eventType] = file
		logger = zerolog.New(file).With().Timestamp().Logger()
		sl.eventTypeLoggers[eventType] = logger
	}

	logger.Info().RawJSON("event", event.Raw).Msg(eventType)
	return nil
}

// logToAgent logs to agent-specific file
func (sl *StructuredLogger) logToAgent(event *events.Event, agentID string) error {
	// Get or create logger for this agent
	logger, exists := sl.agentLoggers[agentID]
	if !exists {
		logPath := filepath.Join(sl.config.LogDir, "by-agent", fmt.Sprintf("%s.jsonl", agentID))
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to create agent log file: %w", err)
		}
		sl.agentFiles[agentID] = file
		logger = zerolog.New(file).With().Timestamp().Logger()
		sl.agentLoggers[agentID] = logger
	}

	logger.Info().
		Str("agent_id", agentID).
		Str("event_type", string(event.Type)).
		RawJSON("event", event.Raw).
		Msg("agent_event")
	return nil
}

// logToSession logs to session timeline
func (sl *StructuredLogger) logToSession(event *events.Event) error {
	sl.sessionLogger.Info().
		Str("event_type", string(event.Type)).
		Str("agent_id", event.GetAgentID()).
		RawJSON("event", event.Raw).
		Msg("session_event")
	return nil
}

// LogAPICall logs an API request/response
func (sl *StructuredLogger) LogAPICall(method, url string, statusCode int, duration time.Duration, err error) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	logEvent := sl.apiLogger.Info().
		Str("method", method).
		Str("url", url).
		Int("status_code", statusCode).
		Dur("duration_ms", duration)

	if err != nil {
		logEvent = logEvent.Err(err)
	}

	logEvent.Msg("api_call")
}

// WizardTurnMetrics contains computed metrics for a wizard turn
type WizardTurnMetrics struct {
	SessionID    string  `json:"session_id"`
	TurnID       int     `json:"turn_id"`
	TokenCount   int     `json:"token_count"`
	FirstTokenMs int64   `json:"first_token_ms"`
	DurationMs   int64   `json:"duration_ms"`
	TokensPerSec float64 `json:"tokens_per_sec,omitempty"`
	PlanID       string  `json:"plan_id,omitempty"`
}

// LogWizardTurnMetrics logs computed wizard metrics for a turn
func (sl *StructuredLogger) LogWizardTurnMetrics(metrics WizardTurnMetrics) error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	// Get or create wizard logger
	logger, exists := sl.agentLoggers["wizard"]
	if !exists {
		logPath := filepath.Join(sl.config.LogDir, "by-agent", "wizard.jsonl")
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to create wizard log file: %w", err)
		}
		sl.agentFiles["wizard"] = file
		logger = zerolog.New(file).With().Timestamp().Logger()
		sl.agentLoggers["wizard"] = logger
	}

	logEvent := logger.Info().
		Str("session_id", metrics.SessionID).
		Int("turn_id", metrics.TurnID).
		Int("token_count", metrics.TokenCount).
		Int64("first_token_ms", metrics.FirstTokenMs).
		Int64("duration_ms", metrics.DurationMs)

	if metrics.TokensPerSec > 0 {
		logEvent = logEvent.Float64("tokens_per_sec", metrics.TokensPerSec)
	}
	if metrics.PlanID != "" {
		logEvent = logEvent.Str("plan_id", metrics.PlanID)
	}

	logEvent.Msg("wizard_turn_metrics")
	return nil
}

// Close closes all log files
func (sl *StructuredLogger) Close() error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	var errors []error

	// Close event type files
	for _, file := range sl.eventTypeFiles {
		if err := file.Close(); err != nil {
			errors = append(errors, err)
		}
	}

	// Close agent files
	for _, file := range sl.agentFiles {
		if err := file.Close(); err != nil {
			errors = append(errors, err)
		}
	}

	// Close session file
	if sl.sessionFile != nil {
		if err := sl.sessionFile.Close(); err != nil {
			errors = append(errors, err)
		}
	}

	// Close API file
	if sl.apiCallFile != nil {
		if err := sl.apiCallFile.Close(); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors closing log files: %v", errors)
	}

	return nil
}
