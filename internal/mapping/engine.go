// Package mapping implements the dialect rule engine: it decodes wire
// frames into the generic events.Event core using a dialect described
// entirely as YAML data. The engine knows no system by name — every
// system-specific behavior comes from the loaded dialect document.
package mapping

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/lancekrogers/stream-debugger/internal/events"
	"gopkg.in/yaml.v3"
)

// knownKinds is the closed Kind vocabulary a rule's kind: value may name,
// resolved directly from internal/events.Kind so the two never drift.
var knownKinds = map[string]events.Kind{
	string(events.KindSessionStart): events.KindSessionStart,
	string(events.KindSessionEnd):   events.KindSessionEnd,
	string(events.KindStreamStart):  events.KindStreamStart,
	string(events.KindStreamEnd):    events.KindStreamEnd,
	string(events.KindContent):      events.KindContent,
	string(events.KindReasoning):    events.KindReasoning,
	string(events.KindToolCall):     events.KindToolCall,
	string(events.KindToolResult):   events.KindToolResult,
	string(events.KindHandoff):      events.KindHandoff,
	string(events.KindStepStart):    events.KindStepStart,
	string(events.KindStepEnd):      events.KindStepEnd,
	string(events.KindStatusChange): events.KindStatusChange,
	string(events.KindUsage):        events.KindUsage,
	string(events.KindTopology):     events.KindTopology,
	string(events.KindDetail):       events.KindDetail,
	string(events.KindError):        events.KindError,
	string(events.KindUnknown):      events.KindUnknown,
}

// Rule is one compiled frame → Event mapping rule.
type Rule struct {
	Match   MatchSpec
	Kind    events.Kind
	Source  ValueForm
	Content ValueForm
	Seq     ValueForm
	// Timestamp extracts a wire timestamp when it is nested inside an
	// envelope (common for protobuf activity messages). When unset, Decode
	// keeps the legacy top-level timestamp fallback.
	Timestamp ValueForm
	Fields    map[string]ValueForm
}

// ruleYAML is the raw YAML shape of a rule, strictly decoded.
type ruleYAML struct {
	Match     MatchSpec            `yaml:"match"`
	Kind      string               `yaml:"kind"`
	Source    ValueForm            `yaml:"source,omitempty"`
	Content   ValueForm            `yaml:"content,omitempty"`
	Seq       ValueForm            `yaml:"seq,omitempty"`
	Timestamp ValueForm            `yaml:"timestamp,omitempty"`
	Fields    map[string]ValueForm `yaml:"fields,omitempty"`
}

// dialectYAML is the top-level dialect document shape.
type dialectYAML struct {
	Version       int           `yaml:"version"`
	Name          string        `yaml:"name"`
	Description   string        `yaml:"description,omitempty"`
	Discriminator Discriminator `yaml:"discriminator"`
	Setup         *setupYAML    `yaml:"setup,omitempty"`
	Send          *sendYAML     `yaml:"send,omitempty"`
	Rules         []ruleYAML    `yaml:"rules"`
	Flow          yaml.Node     `yaml:"flow,omitempty"`
}

// Engine is a compiled dialect: a discriminator plus an ordered rule set.
type Engine struct {
	Version       int
	Name          string
	Discriminator Discriminator
	Setup         *SetupSpec
	Send          *SendSpec
	Rules         []Rule
	// Flow is the dialect's declared flow: projection, nil if it
	// declared none — callers derive instead (see FlowDeriver) rather
	// than treating nil as an empty-but-present spec.
	Flow *FlowSpec
}

// LoadFile reads and parses a dialect YAML file. See Load.
func LoadFile(path string) (*Engine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read dialect file %s: %w", path, err)
	}
	return Load(data)
}

// Load parses and validates a dialect YAML document. Unknown top-level or
// rule keys, unsupported versions, and malformed match/value forms all
// error here rather than misbehaving at decode time.
func Load(data []byte) (*Engine, error) {
	var doc dialectYAML
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("failed to parse dialect: %w", err)
	}

	if doc.Version != 1 {
		return nil, fmt.Errorf("unsupported dialect version %d (only version: 1 is supported)", doc.Version)
	}
	if doc.Name == "" {
		return nil, fmt.Errorf("dialect must set name")
	}
	if doc.Discriminator.Mode == "" {
		return nil, fmt.Errorf("dialect must set discriminator")
	}
	if len(doc.Rules) == 0 {
		return nil, fmt.Errorf("dialect must define at least one rule")
	}

	rules := make([]Rule, 0, len(doc.Rules))
	for i, r := range doc.Rules {
		if err := r.Match.validate(); err != nil {
			return nil, fmt.Errorf("rules[%d].match: %w", i, err)
		}
		if r.Kind == "" {
			return nil, fmt.Errorf("rules[%d]: kind is required", i)
		}
		kind, ok := knownKinds[r.Kind]
		if !ok {
			return nil, fmt.Errorf("rules[%d]: unknown kind %q", i, r.Kind)
		}
		rules = append(rules, Rule{
			Match:     r.Match,
			Kind:      kind,
			Source:    r.Source,
			Content:   r.Content,
			Seq:       r.Seq,
			Timestamp: r.Timestamp,
			Fields:    r.Fields,
		})
	}

	var setup *SetupSpec
	if doc.Setup != nil {
		s := doc.Setup.compile()
		setup = &s
	}

	var send *SendSpec
	if doc.Send != nil {
		s := doc.Send.compile()
		send = &s
	}

	flow, err := compileFlow(doc.Flow)
	if err != nil {
		return nil, err
	}

	return &Engine{
		Version:       doc.Version,
		Name:          doc.Name,
		Discriminator: doc.Discriminator,
		Setup:         setup,
		Send:          send,
		Rules:         rules,
		Flow:          flow,
	}, nil
}

// ResolveName resolves a frame's dispatch name per the dialect's
// discriminator. transportName is what the transport already knows (SSE
// event: line, etc.); data is the frame body, used for "path" mode.
func (e *Engine) ResolveName(transportName string, data []byte) string {
	return e.Discriminator.Resolve(transportName, data)
}

// Match returns the index of the first rule whose criteria are satisfied by
// the given frame name and data, or -1 if none match. Rules are evaluated
// in document order; the first whole match wins.
func (e *Engine) Match(name string, data []byte) int {
	for i, r := range e.Rules {
		if r.Match.Matches(name, data) {
			return i
		}
	}
	return -1
}

// Decode converts a wire frame into the generic Event core. It never
// errors: an unmatched frame becomes Kind=Unknown with a best-effort Fields
// parse, and malformed JSON still passes through with Raw preserved and
// Fields empty — passthrough is non-negotiable for a debugger. Decode holds
// no state across calls (stateless: same input always yields the same
// output, regardless of call order).
func (e *Engine) Decode(transportName string, data []byte) *events.Event {
	name := e.ResolveName(transportName, data)

	// Best-effort parse: malformed JSON (or a non-JSON body, e.g. a bare
	// SSE token) leaves fields nil rather than erroring.
	var fields map[string]any
	_ = json.Unmarshal(data, &fields)

	idx := e.Match(name, data)
	if idx < 0 {
		return &events.Event{
			Name:      name,
			Kind:      events.KindUnknown,
			Timestamp: events.ParseTimestamp(timestampField(fields)),
			Fields:    fields,
			Raw:       data,
		}
	}

	rule := e.Rules[idx]
	timestamp := events.ParseTimestamp(timestampField(fields))
	if rule.Timestamp.IsSet() {
		if value, ok := rule.Timestamp.Extract(data); ok {
			if raw, err := json.Marshal(value); err == nil {
				timestamp = events.ParseTimestamp(raw)
			}
		}
	}
	evt := &events.Event{
		Name:      name,
		Kind:      rule.Kind,
		Timestamp: timestamp,
		Fields:    fields,
		Raw:       data,
	}
	if rule.Source.IsSet() {
		evt.SourceID = rule.Source.String(data)
	}
	if rule.Content.IsSet() {
		evt.Content = rule.Content.String(data)
	}
	if rule.Seq.IsSet() {
		evt.Seq = rule.Seq.Int(data)
	}
	if len(rule.Fields) > 0 {
		if evt.Fields == nil {
			evt.Fields = make(map[string]any, len(rule.Fields))
		}
		for key, form := range rule.Fields {
			if val, ok := form.Extract(data); ok {
				evt.Fields[key] = val
			}
		}
	}
	return evt
}

// timestampField extracts the raw "timestamp" value from a best-effort
// field parse, re-encoded for events.ParseTimestamp's tolerant decoder.
func timestampField(fields map[string]any) json.RawMessage {
	v, ok := fields["timestamp"]
	if !ok {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return raw
}
