package events

import (
	"encoding/json"
	"fmt"
)

// Parser is the temporary Brainyard-specific bridge from wire JSON to the
// generic Event core. It exists only until internal/mapping (phase 004's
// rule engine) takes over via dialects/brainyard.yaml — see the
// mapping-engine design docs. It knows Brainyard's field names by necessity
// (nothing else has been recorded to test against yet); the engine that
// replaces it knows no system by name.
type Parser struct{}

// NewParser creates a new event parser.
func NewParser() *Parser {
	return &Parser{}
}

// wireEnvelope captures the fields common to Brainyard's wire events, for
// extracting the Event core's typed fields before the rest lands in Fields.
type wireEnvelope struct {
	MessageID string          `json:"message_id"`
	Timestamp json.RawMessage `json:"timestamp"`
	AgentID   string          `json:"agent_id"`
	Content   string          `json:"content"`
	Sequence  int             `json:"sequence"`
}

// kindOf maps Brainyard's 19 wire event names to the closed Kind
// vocabulary. Not derived from Brainyard — see internal/events/types.go.
func kindOf(eventType string) (Kind, error) {
	switch eventType {
	case "session_start":
		return KindSessionStart, nil
	case "session_complete":
		return KindSessionEnd, nil
	case "agent_stream_start", "wizard_stream_start":
		return KindStreamStart, nil
	case "agent_content", "wizard_content":
		return KindContent, nil
	case "agent_stream_complete", "wizard_stream_complete":
		return KindStreamEnd, nil
	case "error":
		return KindError, nil
	case "flow_step_start":
		return KindStepStart, nil
	case "flow_step_end":
		return KindStepEnd, nil
	case "flow_config":
		return KindTopology, nil
	case "agent_metadata":
		return KindUsage, nil
	case "flow_step_detail", "prompt_info", "prompt_full",
		"filter_detail", "perspective_detail", "synthesis_detail":
		return KindDetail, nil
	default:
		return "", fmt.Errorf("unknown event type: %s", eventType)
	}
}

// Parse converts raw wire data into the generic Event, given its event name.
func (p *Parser) Parse(eventType string, data []byte) (*Event, error) {
	kind, err := kindOf(eventType)
	if err != nil {
		return nil, err
	}

	var env wireEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", eventType, err)
	}

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("failed to parse %s fields: %w", eventType, err)
	}

	sourceID := env.AgentID
	switch eventType {
	case "wizard_stream_start", "wizard_content", "wizard_stream_complete":
		sourceID = "wizard"
	}

	return &Event{
		Name:      eventType,
		Kind:      kind,
		Timestamp: ParseTimestamp(env.Timestamp),
		SourceID:  sourceID,
		Content:   env.Content,
		Seq:       env.Sequence,
		Fields:    fields,
		Raw:       data,
	}, nil
}

// ParseRaw parses event data without a separately-known event name, reading
// the "type" field from the payload itself if present.
func (p *Parser) ParseRaw(data []byte) (*Event, error) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("failed to parse base event: %w", err)
	}
	if probe.Type != "" {
		return p.Parse(probe.Type, data)
	}
	// No explicit type in the payload — safe fallback; the SSE event name
	// should normally be set and used instead.
	return &Event{Kind: KindUnknown, Raw: data}, nil
}
