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
	FlowStepStart EventType = "flow_step_start"
	FlowStepEnd   EventType = "flow_step_end"
	// Flow step detail (BrainyardV3 extension) — compact, step-specific payloads
	FlowStepDetail EventType = "flow_step_detail"

	// Prompt/flow debug events (only emitted when backend is in debug mode)
	PromptInfo EventType = "prompt_info"
	PromptFull EventType = "prompt_full"
	FlowConfig EventType = "flow_config"

	// Full transparency debug events (debug=verbose)
	FilterDetail      EventType = "filter_detail"
	PerspectiveDetail EventType = "perspective_detail"
	SynthesisDetail   EventType = "synthesis_detail"
	AgentMetadata     EventType = "agent_metadata"
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
	FlowID    string `json:"flow_id,omitempty"`
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
	Step           string `json:"step"`
	Enabled        bool   `json:"enabled"`
	AgentCount     int    `json:"agent_count,omitempty"`
	NonWizardCount int    `json:"non_wizard_count,omitempty"`
}

// FlowStepEndEvent represents a flow step ending (BrainyardV3)
type FlowStepEndEvent struct {
	BaseEvent
	Step          string `json:"step"`
	Enabled       bool   `json:"enabled"`
	AgentCount    int    `json:"agent_count,omitempty"`
	FilteredCount int    `json:"filtered_count,omitempty"`
	// Optional routing details (for step==routing)
	RoutingMode string   `json:"routing_mode,omitempty"`
	RouteTaken  string   `json:"route_taken,omitempty"`
	RouteReason string   `json:"route_reason,omitempty"`
	RouteAgents []string `json:"route_agents,omitempty"`
	DurationMs  int      `json:"duration_ms,omitempty"`
	// Optional prompt references for provenance (e.g., filter_config, synthesis_plan_id)
	PromptRef map[string]interface{} `json:"prompt_ref,omitempty"`
}

// FlowStepDetailEvent provides structured, step-specific debug details
// This event is emitted only when the backend is called with debug flags
// and is intended for developer tooling like stream-debugger.
type FlowStepDetailEvent struct {
    BaseEvent
    Step string `json:"step"`

	// Synthesis details
	PlanID           string `json:"plan_id,omitempty"`
	SynthesisPreview string `json:"synthesis_preview,omitempty"`
	SynthesisFull    string `json:"synthesis_full,omitempty"`
	Perspectives     []struct {
		AgentID string `json:"agent_id"`
		Summary string `json:"summary"`
	} `json:"perspectives,omitempty"`

	// Filter details
	FilteredAgents     []string `json:"filtered_agents,omitempty"`
	ThinkingCharsTotal int      `json:"thinking_chars_total,omitempty"`
	ThinkingPreview    string   `json:"thinking_preview,omitempty"`

    // Agent execution details
    Agents           []string `json:"agents,omitempty"`
    // Optional provider-level details emitted during agent_exec
    AgentID          string `json:"agent_id,omitempty"`
    ProviderThreadID string `json:"provider_thread_id,omitempty"`
    AssistantID      string `json:"assistant_id,omitempty"`
    Provider         string `json:"provider,omitempty"`
}

// PromptInfoEvent contains agent prompt metadata (debug=verbose)
// Only emitted when backend is in debug mode
type PromptInfoEvent struct {
	BaseEvent
	AgentID       string `json:"agent_id"`
	PromptFile    string `json:"prompt_file"`
	PromptSnippet string `json:"prompt_snippet,omitempty"`
	PromptLength  int    `json:"prompt_length,omitempty"`
}

// PromptFullEvent contains complete system prompt (debug=full only)
// Only emitted when backend is in debug mode with full verbosity
type PromptFullEvent struct {
	BaseEvent
	AgentID      string `json:"agent_id"`
	SystemPrompt string `json:"system_prompt"`
}

// FlowConfigEvent contains flow configuration snapshot (debug=verbose)
// Emitted at session start when backend is in debug mode
type FlowConfigEvent struct {
	BaseEvent
	FlowID         string          `json:"flow_id"`
	FlowFile       string          `json:"flow_file"`
	StagesOrder    []string        `json:"stages_order"`
	StagesEnabled  map[string]bool `json:"stages_enabled"`
	RoutingMode    string          `json:"routing_mode,omitempty"`
	AgentCount     int             `json:"agent_count"`
	NonWizardCount int             `json:"non_wizard_count"`
}

// FilterDetailEvent contains full thinking and filtered response for an agent
// Emitted during filter stage when backend is in debug mode
type FilterDetailEvent struct {
	BaseEvent
	AgentID          string `json:"agent_id"`
	ThinkingFull     string `json:"thinking_full"`
	FilteredResponse string `json:"filtered_response"`
	OriginalLength   int    `json:"original_length"`
	FilteredLength   int    `json:"filtered_length"`
}

// PerspectiveDetailEvent contains full agent perspective from synthesis
// Emitted during synthesis stage when backend is in debug mode
type PerspectiveDetailEvent struct {
	BaseEvent
	AgentID         string   `json:"agent_id"`
	PerspectiveFull string   `json:"perspective_full"`
	Summary         string   `json:"summary"`
	KeyInsights     []string `json:"key_insights"`
	RelevanceScore  float64  `json:"relevance_score"`
}

// SynthesisDetailEvent contains full synthesis output
// Emitted during synthesis stage when backend is in debug mode
type SynthesisDetailEvent struct {
	BaseEvent
	PlanID          string `json:"plan_id"`
	SynthesisFull   string `json:"synthesis_full"`
	SynthesisMethod string `json:"synthesis_method"`
	SourcesCombined int    `json:"sources_combined"`
}

// AgentMetadataEvent contains LLM metadata for an agent response
// Emitted after agent stream completes when backend is in debug mode
type AgentMetadataEvent struct {
	BaseEvent
	AgentID        string `json:"agent_id"`
	Model          string `json:"model"`
	TotalTokens    int    `json:"total_tokens"`
	ResponseLength int    `json:"response_length"`
	LatencyMs      int    `json:"latency_ms"`
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
	FlowStepDetail       *FlowStepDetailEvent
	// Debug-only events (prompt/flow visibility)
	PromptInfo *PromptInfoEvent
	PromptFull *PromptFullEvent
	FlowConfig *FlowConfigEvent
	// Full transparency debug events
	FilterDetail      *FilterDetailEvent
	PerspectiveDetail *PerspectiveDetailEvent
	SynthesisDetail   *SynthesisDetailEvent
	AgentMetadata     *AgentMetadataEvent
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
