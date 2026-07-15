package events

import "time"

// Kind is the closed, cross-system vocabulary the visualizer renders
// against. Triangulated from OTel GenAI semconv, the Vercel AI SDK, and
// A2A — deliberately not derived from any single system's event names,
// which would reproduce that system's blind spots at the universal layer.
type Kind string

const (
	KindSessionStart Kind = "session_start"
	KindSessionEnd   Kind = "session_end"
	KindStreamStart  Kind = "stream_start"
	KindStreamEnd    Kind = "stream_end"
	KindContent      Kind = "content"
	KindReasoning    Kind = "reasoning"
	KindToolCall     Kind = "tool_call"
	KindToolResult   Kind = "tool_result"
	KindHandoff      Kind = "handoff"
	KindStepStart    Kind = "step_start"
	KindStepEnd      Kind = "step_end"
	KindStatusChange Kind = "status_change"
	KindUsage        Kind = "usage"
	KindTopology     Kind = "topology"
	KindDetail       Kind = "detail"
	KindError        Kind = "error"
	KindUnknown      Kind = "unknown"
)

// Event is the open, cross-system representation every dialect maps onto.
// Name and Fields are open — a stranger's dialect can carry data the
// renderer has never seen. Kind is the small closed set the renderer
// switches on.
type Event struct {
	Name      string // wire name, whatever it was
	Kind      Kind   // normalized — what the visualizer renders against
	Timestamp time.Time
	SourceID  string // agent/lane identity; "" if none
	Content   string // incremental text, if any
	Seq       int
	Fields    map[string]any // everything else — structured, queryable
	Raw       []byte         // always retained
}

// StringField returns Fields[key] as a string, or "" if absent or the wrong type.
func (e *Event) StringField(key string) string {
	v, _ := e.Fields[key].(string)
	return v
}

// IntField returns Fields[key] as an int, or 0 if absent or the wrong type.
// JSON numbers decode as float64, so that form is accepted too.
func (e *Event) IntField(key string) int {
	switch v := e.Fields[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

// Float64Field returns Fields[key] as a float64, or 0 if absent or the wrong type.
func (e *Event) Float64Field(key string) float64 {
	v, _ := e.Fields[key].(float64)
	return v
}

// BoolField returns Fields[key] as a bool, or false if absent or the wrong type.
func (e *Event) BoolField(key string) bool {
	v, _ := e.Fields[key].(bool)
	return v
}

// StringSliceField returns Fields[key] as a []string, or nil if absent or
// the wrong type. JSON arrays decode as []any, so that form is accepted too.
func (e *Event) StringSliceField(key string) []string {
	switch v := e.Fields[key].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// MapField returns Fields[key] as a map[string]any, or nil if absent or the wrong type.
func (e *Event) MapField(key string) map[string]any {
	v, _ := e.Fields[key].(map[string]any)
	return v
}

// BoolMapField returns Fields[key] as a map[string]bool, or nil if absent or
// the wrong type. JSON objects decode as map[string]any, so that form is
// accepted too — non-bool values are skipped.
func (e *Event) BoolMapField(key string) map[string]bool {
	m, ok := e.Fields[key].(map[string]bool)
	if ok {
		return m
	}
	raw, ok := e.Fields[key].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]bool, len(raw))
	for k, v := range raw {
		if b, ok := v.(bool); ok {
			out[k] = b
		}
	}
	return out
}
