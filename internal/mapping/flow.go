package mapping

import (
	"fmt"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"gopkg.in/yaml.v3"
)

// defaultRole is what an unmatched lane renders as — both a declared
// FlowSpec's DefaultRole (when default_role: is unset) and every derived
// lane's role, since derivation has no way to recognize an aggregator
// without a dialect telling it: "a stranger with no flow: block still
// gets lanes" (architecture.md), just plain worker ones.
const defaultRole events.Role = events.RoleWorker

// LaneMatchSpec matches against a DECODED events.Event — a different
// match dimension than MatchSpec (match.go), which matches raw wire
// frames via event/path/equals/exists/raw before decode ever happens.
// flow: operates on the Event core, post-decode; source is the only
// dimension architecture.md's own flow.lanes example uses, so that's
// all this supports until a real need for more appears — adding fields
// here is cheap, guessing at unneeded ones isn't.
type LaneMatchSpec struct {
	Source string `yaml:"source,omitempty"`
}

// validate checks the match spec sets at least one condition.
func (m LaneMatchSpec) validate() error {
	if m.Source == "" {
		return fmt.Errorf("must set source")
	}
	return nil
}

// Matches reports whether this lane rule matches the given event.
func (m LaneMatchSpec) Matches(evt *events.Event) bool {
	return m.Source != "" && evt.SourceID == m.Source
}

// LaneRule declares one lane's role/label, matched by source. The order
// lanes appear in a dialect's flow.lanes matters: the first match wins,
// same evaluation order as mapping.Rule.
type LaneRule struct {
	Match LaneMatchSpec
	Role  events.Role
	Label string
}

// laneRuleYAML is the raw YAML shape of one flow.lanes[] entry.
type laneRuleYAML struct {
	Match LaneMatchSpec `yaml:"match"`
	Role  string        `yaml:"role"`
	Label string        `yaml:"label,omitempty"`
}

// FlowSpec is a dialect's declared projection — flow.stages/lanes/
// default_role — for the visualizer's flow view. A nil *FlowSpec on
// Engine.Flow means the dialect declared no flow: block at all; callers
// derive instead (see FlowDeriver), rather than treating a nil FlowSpec
// as an empty-but-present one — the two mean different things ("nothing
// declared, derive" vs. "declared as empty," which this DSL doesn't
// currently have a way to express and doesn't need to).
type FlowSpec struct {
	Stages      []string
	Lanes       []LaneRule
	DefaultRole events.Role
}

// flowYAML is the raw YAML shape of a flow: block.
type flowYAML struct {
	Stages      []string       `yaml:"stages,omitempty"`
	Lanes       []laneRuleYAML `yaml:"lanes,omitempty"`
	DefaultRole string         `yaml:"default_role,omitempty"`
}

// compileFlow parses raw (the dialect's flow: yaml.Node, decoded
// generically by the strict top-level decoder since its shape wasn't
// known until now) into a *FlowSpec. Returns (nil, nil) — the "no flow:
// block" case derivation exists for, not an error — both when flow: was
// never set at all (yaml.Node's zero value) and when it was set to a
// null scalar (a bare `flow:` key, `flow: null`, or `flow: ~`, all
// tagged "!!null" by the YAML parser). Both read as "I haven't
// configured a flow view" — a bare `flow:` stub is exactly what a
// copy-pasted dialect skeleton or an unfinished edit looks like, and
// treating it as "declared, deliberately empty" would silently suppress
// derivation for what's almost certainly an unintentional empty view.
// An explicit `flow: {}` (an empty MAPPING, not a null scalar) is the
// only way to genuinely opt out of derivation with an empty spec.
func compileFlow(raw yaml.Node) (*FlowSpec, error) {
	if raw.IsZero() || raw.Tag == "!!null" {
		return nil, nil
	}

	var doc flowYAML
	if err := raw.Decode(&doc); err != nil {
		return nil, fmt.Errorf("flow: %w", err)
	}

	lanes := make([]LaneRule, 0, len(doc.Lanes))
	for i, l := range doc.Lanes {
		if err := l.Match.validate(); err != nil {
			return nil, fmt.Errorf("flow.lanes[%d].match: %w", i, err)
		}
		if l.Role == "" {
			return nil, fmt.Errorf("flow.lanes[%d]: role is required", i)
		}
		lanes = append(lanes, LaneRule{
			Match: l.Match,
			Role:  events.Role(l.Role),
			Label: l.Label,
		})
	}

	role := doc.DefaultRole
	if role == "" {
		role = string(defaultRole)
	}

	return &FlowSpec{
		Stages:      doc.Stages,
		Lanes:       lanes,
		DefaultRole: events.Role(role),
	}, nil
}

// RoleFor resolves the role evt's lane should render as: the first
// declared lane rule that matches (document order, first match wins —
// same evaluation discipline as mapping.Rule), or DefaultRole if none do.
func (f *FlowSpec) RoleFor(evt *events.Event) events.Role {
	for _, l := range f.Lanes {
		if l.Match.Matches(evt) {
			return l.Role
		}
	}
	return f.DefaultRole
}

// FlowDeriver derives lanes and stages from a stream of events when a
// dialect declares no flow: block — the "derived by default" half of
// architecture.md's Layer 4 table. Unlike FlowSpec (immutable, compiled
// once at Load), a FlowDeriver is stateful: call Observe for every event
// in a session, in order, and it accumulates what it's seen.
//
// Not safe for concurrent use — Observe and the accumulator reads
// (Lanes/Stages) share unsynchronized maps and slices. This is a
// deliberate, not accidental, omission: the intended caller is a Bubble
// Tea model's single-threaded Update loop (see internal/visualizer/
// tui.go's tea.Cmd/tea.Msg pattern), where nothing outside that loop
// ever touches model state concurrently. Add synchronization if a future
// caller genuinely needs concurrent access — don't pay for it here.
type FlowDeriver struct {
	lanes     []string
	seenLane  map[string]bool
	stages    []string
	seenStage map[string]bool
}

// NewFlowDeriver returns an empty deriver, ready for Observe.
func NewFlowDeriver() *FlowDeriver {
	return &FlowDeriver{
		seenLane:  make(map[string]bool),
		seenStage: make(map[string]bool),
	}
}

// Observe feeds one decoded event into the deriver. Lanes accumulate
// from distinct Event.SourceID values; stages accumulate from
// step_start/step_end events' Fields["step"], or — if a topology event
// arrives — its Fields["stages_order"] (the exact field name a shipped
// flow_config rule produces; derivation
// reads the same key a real dialect already emits rather than inventing
// a new one). Both accumulate in first-seen order, deduplicated.
func (d *FlowDeriver) Observe(evt *events.Event) {
	if evt.SourceID != "" && !d.seenLane[evt.SourceID] {
		d.seenLane[evt.SourceID] = true
		d.lanes = append(d.lanes, evt.SourceID)
	}

	switch evt.Kind {
	case events.KindStepStart, events.KindStepEnd:
		if step, ok := evt.Fields["step"].(string); ok && step != "" {
			d.addStage(step)
		}
	case events.KindTopology:
		if order, ok := evt.Fields["stages_order"].([]any); ok {
			for _, s := range order {
				if step, ok := s.(string); ok && step != "" {
					d.addStage(step)
				}
			}
		}
	}
}

func (d *FlowDeriver) addStage(step string) {
	if !d.seenStage[step] {
		d.seenStage[step] = true
		d.stages = append(d.stages, step)
	}
}

// Lanes returns the distinct SourceIDs observed so far, in first-seen
// order.
func (d *FlowDeriver) Lanes() []string {
	return append([]string(nil), d.lanes...)
}

// Stages returns the distinct stage names observed so far, in
// first-seen order.
func (d *FlowDeriver) Stages() []string {
	return append([]string(nil), d.stages...)
}

// RoleFor always returns the default role: derivation has no aggregator
// (or any other non-default role) concept without a dialect declaring
// one — every derived lane is a plain worker.
func (d *FlowDeriver) RoleFor(evt *events.Event) events.Role {
	return defaultRole
}
