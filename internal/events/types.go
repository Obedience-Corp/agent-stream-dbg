package events

import "time"

// EventType represents the type of SSE event
type EventType string

const (
	SessionStart         EventType = "session_start"
	SessionComplete      EventType = "session_complete"
	AgentStreamStart     EventType = "agent_stream_start"
	AgentContent         EventType = "agent_content"
	AgentStreamComplete  EventType = "agent_stream_complete"
	WizardStreamStart    EventType = "wizard_stream_start"
	WizardContent        EventType = "wizard_content"
	WizardStreamComplete EventType = "wizard_stream_complete"
	Error                EventType = "error"
    // Flow control step events (BrainyardV3 extension)
    FlowStepStart       EventType = "flow_step_start"
    FlowStepEnd         EventType = "flow_step_end"
)

// BaseEvent contains fields common to all events
type BaseEvent struct {
	Type      EventType `json:"type"`
	MessageID string    `json:"message_id"`
	Timestamp time.Time `json:"timestamp"`
}

// SessionStartEvent represents session initialization
type SessionStartEvent struct {
	BaseEvent
	SessionID string `json:"session_id"`
}

// SessionCompleteEvent represents session completion
type SessionCompleteEvent struct {
	BaseEvent
	SessionID string `json:"session_id"`
}

// AgentStreamStartEvent represents an agent beginning to stream
type AgentStreamStartEvent struct {
	BaseEvent
	AgentID string `json:"agent_id"`
}

// AgentContentEvent represents streaming tokens from an agent
type AgentContentEvent struct {
	BaseEvent
	AgentID  string `json:"agent_id"`
	Content  string `json:"content"`
	Sequence int    `json:"sequence"`
}

// AgentStreamCompleteEvent represents an agent finishing streaming
type AgentStreamCompleteEvent struct {
	BaseEvent
	AgentID    string `json:"agent_id"`
	TokenCount int    `json:"token_count"`
}

// WizardStreamStartEvent represents wizard synthesis beginning
type WizardStreamStartEvent struct {
	BaseEvent
}

// WizardContentEvent represents streaming tokens from wizard synthesis
type WizardContentEvent struct {
	BaseEvent
	Content  string `json:"content"`
	Sequence int    `json:"sequence"`
}

// WizardStreamCompleteEvent represents wizard synthesis completion
type WizardStreamCompleteEvent struct {
    BaseEvent
    TokenCount int `json:"token_count"`
}

// FlowStepStartEvent represents a flow step starting (BrainyardV3)
type FlowStepStartEvent struct {
    BaseEvent
    Step       string `json:"step"`
    Enabled    bool   `json:"enabled"`
    AgentCount int    `json:"agent_count,omitempty"`
    NonWizardCount int `json:"non_wizard_count,omitempty"`
}

// FlowStepEndEvent represents a flow step ending (BrainyardV3)
type FlowStepEndEvent struct {
    BaseEvent
    Step       string `json:"step"`
    Enabled    bool   `json:"enabled"`
    AgentCount int    `json:"agent_count,omitempty"`
    // Optional routing details (for step==routing)
    RoutingMode string `json:"routing_mode,omitempty"`
    RouteTaken  string `json:"route_taken,omitempty"`
    RouteReason string `json:"route_reason,omitempty"`
}

// ErrorType represents different categories of errors
type ErrorType string

const (
	AuthenticationError ErrorType = "authentication_error"
	ConnectionError     ErrorType = "connection_error"
	RateLimitError      ErrorType = "rate_limit_error"
	ModelNotFoundError  ErrorType = "model_not_found"
	ContentFilterError  ErrorType = "content_filter_error"
	StreamError         ErrorType = "stream_error"
	TimeoutError        ErrorType = "timeout_error"
	UnknownError        ErrorType = "unknown_error"
)

// ErrorEvent represents an error during streaming
type ErrorEvent struct {
	BaseEvent
	ErrorType  ErrorType `json:"error_type"`
	Message    string    `json:"message"`
	Details    string    `json:"details,omitempty"`
	AgentID    string    `json:"agent_id,omitempty"`
	RetryAfter int       `json:"retry_after,omitempty"` // Seconds to wait before retry
}

// Event is a union type that can hold any event
type Event struct {
    Type EventType
    Raw  []byte // Raw JSON for logging

	// Only one of these will be populated based on Type
	SessionStart         *SessionStartEvent
	SessionComplete      *SessionCompleteEvent
	AgentStreamStart     *AgentStreamStartEvent
	AgentContent         *AgentContentEvent
	AgentStreamComplete  *AgentStreamCompleteEvent
	WizardStreamStart    *WizardStreamStartEvent
	WizardContent        *WizardContentEvent
    WizardStreamComplete *WizardStreamCompleteEvent
    Error                *ErrorEvent
    FlowStepStart        *FlowStepStartEvent
    FlowStepEnd          *FlowStepEndEvent
}

// GetAgentID returns the agent ID if the event is agent-related
func (e *Event) GetAgentID() string {
	switch e.Type {
	case AgentStreamStart:
		return e.AgentStreamStart.AgentID
	case AgentContent:
		return e.AgentContent.AgentID
	case AgentStreamComplete:
		return e.AgentStreamComplete.AgentID
	case Error:
		return e.Error.AgentID
	default:
		return ""
	}
}

// GetContent returns the content if the event contains streaming content
func (e *Event) GetContent() string {
	switch e.Type {
	case AgentContent:
		return e.AgentContent.Content
	case WizardContent:
		return e.WizardContent.Content
	default:
		return ""
	}
}

// GetSequence returns the sequence number if the event has one
func (e *Event) GetSequence() int {
	switch e.Type {
	case AgentContent:
		return e.AgentContent.Sequence
	case WizardContent:
		return e.WizardContent.Sequence
	default:
		return -1
	}
}

// IsStreamingContent returns true if this is a content event
func (e *Event) IsStreamingContent() bool {
	return e.Type == AgentContent || e.Type == WizardContent
}

// IsError returns true if this is an error event
func (e *Event) IsError() bool {
	return e.Type == Error
}
