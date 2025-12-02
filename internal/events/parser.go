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

	case Error:
		var e ErrorEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse error: %w", err)
		}
		event.Error = &e

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

	return p.Parse(string(base.Type), data)
}
