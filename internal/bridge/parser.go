// Package bridge is the temporary Brainyard-specific bridge from wire JSON
// to the generic events.Event core, expressed as dialect data (see
// dialect.yaml.go) instead of a Go switch. It exists only until
// dialects/brainyard.yaml lands in phase 004's dialects-as-data sequence —
// at that point this package is deleted in favor of loading the real
// dialect file directly. It is Brainyard-specific by necessity (nothing
// else has been recorded to test against yet); the engine that evaluates
// it (internal/mapping) knows no system by name — this package is exactly
// the seam where system-specific knowledge is allowed to live.
package bridge

import (
	"encoding/json"

	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/mapping"
)

// dialectYAML becomes dialects/brainyard.yaml in the next sequence (a
// source-location change, not a semantic one) once that file lands.
const dialectYAML = `
version: 1
name: brainyard-bridge
description: temporary embedded dialect; becomes dialects/brainyard.yaml in the next sequence
discriminator: event
rules:
  - match: {event: session_start}
    kind: session_start
    fields:
      session_id: session_id
      flow_id: flow_id
  - match: {event: session_complete}
    kind: session_end
    fields:
      session_id: session_id
  - match: {event: agent_stream_start}
    kind: stream_start
    source: agent_id
  - match: {event: agent_content}
    kind: content
    source: agent_id
    content: content
    seq: sequence
  - match: {event: agent_stream_complete}
    kind: stream_end
    source: agent_id
    fields:
      token_count: token_count
  - match: {event: wizard_stream_start}
    kind: stream_start
    source: {const: wizard}
  - match: {event: wizard_content}
    kind: content
    source: {const: wizard}
    content: content
    seq: sequence
  - match: {event: wizard_stream_complete}
    kind: stream_end
    source: {const: wizard}
    fields:
      token_count: token_count
  - match: {event: error}
    kind: error
    source: {path: agent_id, default: ""}
    fields:
      error_type: error_type
      message: message
      details: details
      agent_id: agent_id
      retry_after: retry_after
  - match: {event: flow_step_start}
    kind: step_start
    fields:
      step: step
      enabled: enabled
      agent_count: agent_count
      non_wizard_count: non_wizard_count
  - match: {event: flow_step_end}
    kind: step_end
    fields:
      step: step
      enabled: enabled
      agent_count: agent_count
      filtered_count: filtered_count
      routing_mode: routing_mode
      route_taken: route_taken
      route_reason: route_reason
      route_agents: route_agents
      duration_ms: duration_ms
      prompt_ref: prompt_ref
  - match: {event: flow_step_detail}
    kind: detail
    fields:
      step: step
      plan_id: plan_id
      synthesis_preview: synthesis_preview
      synthesis_full: synthesis_full
      perspectives: perspectives
      filtered_agents: filtered_agents
      thinking_chars_total: thinking_chars_total
      thinking_preview: thinking_preview
      agents: agents
      agent_id: agent_id
      provider_thread_id: provider_thread_id
      assistant_id: assistant_id
      provider: provider
  - match: {event: prompt_info}
    kind: detail
    fields:
      agent_id: agent_id
      prompt_file: prompt_file
      prompt_snippet: prompt_snippet
      prompt_length: prompt_length
  - match: {event: prompt_full}
    kind: detail
    fields:
      agent_id: agent_id
      system_prompt: system_prompt
  - match: {event: flow_config}
    kind: topology
    fields:
      flow_id: flow_id
      flow_file: flow_file
      stages_order: stages_order
      stages_enabled: stages_enabled
      routing_mode: routing_mode
      agent_count: agent_count
      non_wizard_count: non_wizard_count
  - match: {event: filter_detail}
    kind: detail
    fields:
      agent_id: agent_id
      thinking_full: thinking_full
      filtered_response: filtered_response
      original_length: original_length
      filtered_length: filtered_length
  - match: {event: perspective_detail}
    kind: detail
    fields:
      agent_id: agent_id
      perspective_full: perspective_full
      summary: summary
      key_insights: key_insights
      relevance_score: relevance_score
  - match: {event: synthesis_detail}
    kind: detail
    fields:
      plan_id: plan_id
      synthesis_full: synthesis_full
      synthesis_method: synthesis_method
      sources_combined: sources_combined
  - match: {event: agent_metadata}
    kind: usage
    source: agent_id
    fields:
      model: model
      total_tokens: total_tokens
      response_length: response_length
      latency_ms: latency_ms
`

// engine is compiled once at package init from dialectYAML. A malformed
// embedded dialect is a programmer error, not a runtime condition, so a
// load failure panics rather than being surfaced from every Parse call.
var engine = mustLoad()

func mustLoad() *mapping.Engine {
	e, err := mapping.Load([]byte(dialectYAML))
	if err != nil {
		panic("bridge: embedded dialect failed to load: " + err.Error())
	}
	return e
}

// Parser decodes wire frames into the generic events.Event core via the
// embedded bridge dialect. It knows no event types itself —
// internal/mapping.Engine does the work; this is purely a stable call-site
// facade for internal/client and cmd/stream-debugger.
type Parser struct{}

// NewParser creates a new event parser.
func NewParser() *Parser {
	return &Parser{}
}

// Parse converts raw wire data into the generic Event, given its event name.
// It never errors — an unrecognized event name or malformed body decodes to
// a Kind=Unknown event rather than being dropped (passthrough is
// non-negotiable for a debugger).
func (p *Parser) Parse(eventType string, data []byte) (*events.Event, error) {
	return engine.Decode(eventType, data), nil
}

// ParseRaw parses event data without a separately-known event name, reading
// the "type" field from the payload itself if present. Like Parse, it never
// errors.
func (p *Parser) ParseRaw(data []byte) (*events.Event, error) {
	return engine.Decode(rawEventType(data), data), nil
}

// rawEventType extracts the "type" field from a payload with no separate
// SSE event name (e.g. a JSONL log line), for use as the dispatch name.
func rawEventType(data []byte) string {
	var probe struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(data, &probe)
	return probe.Type
}
