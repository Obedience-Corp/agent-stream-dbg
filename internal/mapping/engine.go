// Package mapping implements the dialect rule engine: it decodes wire
// frames into the generic events.Event core using a dialect described
// entirely as YAML data. The engine knows no system by name — every
// system-specific behavior comes from the loaded dialect document.
package mapping

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// knownKinds is the closed Kind vocabulary a rule's kind: value may name.
// Kept local (not imported from internal/events) to avoid a dependency
// cycle risk and keep the engine's YAML validation self-contained; the
// values are kept in sync with internal/events.Kind by the engine tests.
var knownKinds = map[string]bool{
	"session_start": true, "session_end": true,
	"stream_start": true, "stream_end": true,
	"content": true, "reasoning": true,
	"tool_call": true, "tool_result": true,
	"handoff":       true,
	"step_start":    true,
	"step_end":      true,
	"status_change": true,
	"usage":         true,
	"topology":      true,
	"detail":        true,
	"error":         true,
	"unknown":       true,
}

// Rule is one compiled frame → Event mapping rule.
type Rule struct {
	Match   MatchSpec
	Kind    string
	Source  ValueForm
	Content ValueForm
	Seq     ValueForm
	Fields  map[string]ValueForm
}

// ruleYAML is the raw YAML shape of a rule, strictly decoded.
type ruleYAML struct {
	Match   MatchSpec            `yaml:"match"`
	Kind    string               `yaml:"kind"`
	Source  ValueForm            `yaml:"source,omitempty"`
	Content ValueForm            `yaml:"content,omitempty"`
	Seq     ValueForm            `yaml:"seq,omitempty"`
	Fields  map[string]ValueForm `yaml:"fields,omitempty"`
}

// dialectYAML is the top-level dialect document shape.
type dialectYAML struct {
	Version       int           `yaml:"version"`
	Name          string        `yaml:"name"`
	Description   string        `yaml:"description,omitempty"`
	Discriminator Discriminator `yaml:"discriminator"`
	Setup         yaml.Node     `yaml:"setup,omitempty"`
	Send          yaml.Node     `yaml:"send,omitempty"`
	Rules         []ruleYAML    `yaml:"rules"`
	Flow          yaml.Node     `yaml:"flow,omitempty"`
}

// Engine is a compiled dialect: a discriminator plus an ordered rule set.
type Engine struct {
	Version       int
	Name          string
	Discriminator Discriminator
	Rules         []Rule
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
		if !knownKinds[r.Kind] {
			return nil, fmt.Errorf("rules[%d]: unknown kind %q", i, r.Kind)
		}
		rules = append(rules, Rule{
			Match:   r.Match,
			Kind:    r.Kind,
			Source:  r.Source,
			Content: r.Content,
			Seq:     r.Seq,
			Fields:  r.Fields,
		})
	}

	return &Engine{
		Version:       doc.Version,
		Name:          doc.Name,
		Discriminator: doc.Discriminator,
		Rules:         rules,
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
