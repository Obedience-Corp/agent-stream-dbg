package mapping_test

import (
	"testing"

	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/mapping"
)

// TestLoad_BrainyardFlowBlock is this task's explicit Done-When: loading
// the real, shipped dialects/brainyard.yaml produces a FlowSpec matching
// its flow: block exactly — proving compileFlow works against real data,
// not just a hand-crafted test fixture.
func TestLoad_BrainyardFlowBlock(t *testing.T) {
	engine, err := mapping.LoadFile("../../dialects/brainyard.yaml")
	if err != nil {
		t.Fatalf("mapping.LoadFile: %v", err)
	}
	if engine.Flow == nil {
		t.Fatal("expected non-nil Flow — brainyard.yaml declares a flow: block")
	}

	wantStages := []string{"routing", "discovery", "agent_exec", "filter", "synthesis", "wizard"}
	if len(engine.Flow.Stages) != len(wantStages) {
		t.Fatalf("expected %d stages, got %d: %v", len(wantStages), len(engine.Flow.Stages), engine.Flow.Stages)
	}
	for i, want := range wantStages {
		if engine.Flow.Stages[i] != want {
			t.Errorf("stage %d: expected %q, got %q", i, want, engine.Flow.Stages[i])
		}
	}

	if len(engine.Flow.Lanes) != 1 {
		t.Fatalf("expected exactly 1 lane rule, got %d: %+v", len(engine.Flow.Lanes), engine.Flow.Lanes)
	}
	lane := engine.Flow.Lanes[0]
	if lane.Match.Source != "wizard" {
		t.Errorf("expected lane match source 'wizard', got %q", lane.Match.Source)
	}
	if lane.Role != "aggregator" {
		t.Errorf("expected lane role 'aggregator', got %q", lane.Role)
	}
	if lane.Label != "Wizard (Synthesis)" {
		t.Errorf("expected lane label 'Wizard (Synthesis)', got %q", lane.Label)
	}

	if engine.Flow.DefaultRole != "worker" {
		t.Errorf("expected default_role 'worker', got %q", engine.Flow.DefaultRole)
	}
}

// TestFlowSpec_RoleFor_ResolvesAggregatorForWizard proves role: aggregator
// is now functional, not decorative: a SourceID of "wizard" resolves to
// role "aggregator" via RoleFor, and anything else resolves to the
// default role.
func TestFlowSpec_RoleFor_ResolvesAggregatorForWizard(t *testing.T) {
	engine, err := mapping.LoadFile("../../dialects/brainyard.yaml")
	if err != nil {
		t.Fatalf("mapping.LoadFile: %v", err)
	}

	wizard := &events.Event{SourceID: "wizard"}
	if role := engine.Flow.RoleFor(wizard); role != "aggregator" {
		t.Errorf("expected role 'aggregator' for SourceID 'wizard', got %q", role)
	}

	other := &events.Event{SourceID: "sam_harris"}
	if role := engine.Flow.RoleFor(other); role != "worker" {
		t.Errorf("expected role 'worker' for an unmatched SourceID, got %q", role)
	}
}

// TestLoad_NoFlowBlock_FlowIsNil proves the "no flow: block" case is
// distinguishable from a declared-but-empty one — callers derive on nil,
// not on a FlowSpec with zero lanes/stages.
func TestLoad_NoFlowBlock_FlowIsNil(t *testing.T) {
	engine, err := mapping.Load([]byte(`
version: 1
name: test-no-flow
discriminator: event
rules:
  - match: {event: content}
    kind: content
`))
	if err != nil {
		t.Fatalf("mapping.Load: %v", err)
	}
	if engine.Flow != nil {
		t.Errorf("expected nil Flow for a dialect with no flow: block, got %+v", engine.Flow)
	}
}

// TestLoad_NullFlowBlock_AlsoFlowIsNil regression-tests a self-review
// finding: a bare `flow:` key (or `flow: null`/`flow: ~`) is exactly
// what a copy-pasted dialect skeleton or an unfinished edit looks like —
// it must derive, the same as a fully absent flow: key, not silently
// suppress derivation by being treated as "declared, deliberately
// empty."
func TestLoad_NullFlowBlock_AlsoFlowIsNil(t *testing.T) {
	for _, variant := range []string{"flow:\n", "flow: null\n", "flow: ~\n"} {
		engine, err := mapping.Load([]byte(`
version: 1
name: test-null-flow
discriminator: event
rules:
  - match: {event: content}
    kind: content
` + variant))
		if err != nil {
			t.Fatalf("mapping.Load(%q): %v", variant, err)
		}
		if engine.Flow != nil {
			t.Errorf("variant %q: expected nil Flow, got %+v", variant, engine.Flow)
		}
	}
}

// TestLoad_ExplicitEmptyFlowBlock_IsDeclaredNotDerived proves the other
// side of that same distinction: `flow: {}` is an explicit empty MAPPING
// (not a null scalar) — a genuine, deliberate opt-out of derivation, and
// must stay non-nil with zero lanes/stages, not be treated the same as
// an absent/null key.
func TestLoad_ExplicitEmptyFlowBlock_IsDeclaredNotDerived(t *testing.T) {
	engine, err := mapping.Load([]byte(`
version: 1
name: test-empty-flow
discriminator: event
rules:
  - match: {event: content}
    kind: content
flow: {}
`))
	if err != nil {
		t.Fatalf("mapping.Load: %v", err)
	}
	if engine.Flow == nil {
		t.Fatal("expected non-nil Flow for an explicit flow: {}, got nil (should not derive)")
	}
	if len(engine.Flow.Stages) != 0 || len(engine.Flow.Lanes) != 0 {
		t.Errorf("expected empty Stages/Lanes for flow: {}, got %+v", engine.Flow)
	}
}

// TestFlowDeriver_DerivesLanesAndStagesInFirstSeenOrder is this task's
// other explicit Done-When: proves the derive-by-default path
// independently of brainyard.yaml (the only dialect that happens to
// declare flow: today) — a stranger dialect with no flow: block still
// gets lanes and stages, derived purely from the events it emits.
func TestFlowDeriver_DerivesLanesAndStagesInFirstSeenOrder(t *testing.T) {
	d := mapping.NewFlowDeriver()

	// Stage order deliberately inverted from alphabetical (synthesis
	// before discovery): a broken implementation that sorted instead of
	// preserving first-seen order would pass an alphabetically-ordered
	// fixture by coincidence — this fixture only passes if order is
	// genuinely first-seen.
	seq := []*events.Event{
		{SourceID: "agent_b", Kind: events.KindStepStart, Fields: map[string]any{"step": "synthesis"}},
		{SourceID: "agent_a", Kind: events.KindContent},
		{SourceID: "agent_b", Kind: events.KindStepEnd, Fields: map[string]any{"step": "synthesis"}},
		{SourceID: "agent_a", Kind: events.KindStepStart, Fields: map[string]any{"step": "discovery"}},
		{SourceID: "agent_c", Kind: events.KindContent},
		// A repeated stage/lane must not duplicate.
		{SourceID: "agent_b", Kind: events.KindStepStart, Fields: map[string]any{"step": "synthesis"}},
	}
	for _, evt := range seq {
		d.Observe(evt)
	}

	wantLanes := []string{"agent_b", "agent_a", "agent_c"}
	if gotLanes := d.Lanes(); !equalStrings(gotLanes, wantLanes) {
		t.Errorf("expected lanes %v (first-seen order, deduplicated), got %v", wantLanes, gotLanes)
	}

	wantStages := []string{"synthesis", "discovery"}
	if gotStages := d.Stages(); !equalStrings(gotStages, wantStages) {
		t.Errorf("expected stages %v (first-seen order, deduplicated), got %v", wantStages, gotStages)
	}

	// Every derived lane is role: worker — derivation has no aggregator
	// concept without a dialect declaring one.
	for _, evt := range seq {
		if role := d.RoleFor(evt); role != "worker" {
			t.Errorf("expected derived role 'worker' for SourceID %q, got %q", evt.SourceID, role)
		}
	}
}

// TestFlowDeriver_TopologyEvent_DerivesStagesFromStagesOrder proves the
// topology-event derivation path reads the exact same Fields["stages_order"]
// key dialects/brainyard.yaml's flow_config rule already produces, not an
// invented one.
func TestFlowDeriver_TopologyEvent_DerivesStagesFromStagesOrder(t *testing.T) {
	d := mapping.NewFlowDeriver()
	d.Observe(&events.Event{
		Kind:   events.KindTopology,
		Fields: map[string]any{"stages_order": []any{"routing", "discovery", "synthesis"}},
	})

	want := []string{"routing", "discovery", "synthesis"}
	if got := d.Stages(); !equalStrings(got, want) {
		t.Errorf("expected stages %v from a topology event, got %v", want, got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
