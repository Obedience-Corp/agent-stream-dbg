package events

import (
	"encoding/json"
	"fmt"
)

// Parser handles parsing SSE events into typed structs
type Parser struct{}

// NewParser creates a new event parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse converts raw SSE event data into a typed Event struct
func (p *Parser) Parse(eventType string, data []byte) (*Event, error) {
	event := &Event{
		Type: EventType(eventType),
		Raw:  data,
	}

	switch EventType(eventType) {
	case SessionStart:
		var e SessionStartEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse session_start: %w", err)
		}
		event.SessionStart = &e

	case SessionComplete:
		var e SessionCompleteEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse session_complete: %w", err)
		}
		event.SessionComplete = &e

	case AgentStreamStart:
		var e AgentStreamStartEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse agent_stream_start: %w", err)
		}
		event.AgentStreamStart = &e

	case AgentContent:
		var e AgentContentEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse agent_content: %w", err)
		}
		event.AgentContent = &e

	case AgentStreamComplete:
		var e AgentStreamCompleteEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse agent_stream_complete: %w", err)
		}
		event.AgentStreamComplete = &e

	case WizardStreamStart:
		var e WizardStreamStartEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse wizard_stream_start: %w", err)
		}
		event.WizardStreamStart = &e

	case WizardContent:
		var e WizardContentEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse wizard_content: %w", err)
		}
		event.WizardContent = &e

	case WizardStreamComplete:
		var e WizardStreamCompleteEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse wizard_stream_complete: %w", err)
		}
		event.WizardStreamComplete = &e

	case FlowStepStart:
		var e FlowStepStartEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse flow_step_start: %w", err)
		}
		event.FlowStepStart = &e

	case FlowStepEnd:
		var e FlowStepEndEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse flow_step_end: %w", err)
		}
		event.FlowStepEnd = &e

	case FlowStepDetail:
		var e FlowStepDetailEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse flow_step_detail: %w", err)
		}
		event.FlowStepDetail = &e

	case Error:
		var e ErrorEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse error: %w", err)
		}
		event.Error = &e

	case PromptInfo:
		var e PromptInfoEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse prompt_info: %w", err)
		}
		event.PromptInfo = &e

	case PromptFull:
		var e PromptFullEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse prompt_full: %w", err)
		}
		event.PromptFull = &e

	case FlowConfig:
		var e FlowConfigEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse flow_config: %w", err)
		}
		event.FlowConfig = &e

	case FilterDetail:
		var e FilterDetailEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse filter_detail: %w", err)
		}
		event.FilterDetail = &e

	case PerspectiveDetail:
		var e PerspectiveDetailEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse perspective_detail: %w", err)
		}
		event.PerspectiveDetail = &e

	case SynthesisDetail:
		var e SynthesisDetailEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse synthesis_detail: %w", err)
		}
		event.SynthesisDetail = &e

	case AgentMetadata:
		var e AgentMetadataEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse agent_metadata: %w", err)
		}
		event.AgentMetadata = &e

	default:
		return nil, fmt.Errorf("unknown event type: %s", eventType)
	}

	return event, nil
}

// ParseRaw attempts to parse event data without knowing the type beforehand
func (p *Parser) ParseRaw(data []byte) (*Event, error) {
	// First, extract just the type field
	var base BaseEvent
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("failed to parse base event: %w", err)
	}

	if base.Type != "" {
		return p.Parse(string(base.Type), data)
	}
	// If payload has no explicit type (e.g., flow_step_* detail), try heuristics
	// Attempt known extension kinds based on fields
	// This is a safe fallback; SSE event name should normally be set and used instead.
	return &Event{Raw: data}, nil
}
