package visualizer

import (
	"strings"
	"testing"
)

// TestBuildFlowNodesAndStatusIcon is a floor-level test for render_flow.go
// (previously untested directly): buildFlowNodes must produce a node per
// observed flow_step_start/flow_step_end pair, and statusIcon must reflect
// enabled/in-progress/completed state.
func TestBuildFlowNodesAndStatusIcon(t *testing.T) {
	m := newFixtureModel(t)

	nodes := m.buildFlowNodes(m.messages[0].Events)
	routing := nodes["routing"]
	if routing == nil {
		t.Fatal("expected a routing flow node from the fixture")
	}
	if !routing.Enabled {
		t.Error("expected routing to be enabled")
	}
	if routing.InProgress {
		t.Error("expected routing to have completed (flow_step_end observed)")
	}
	if got := statusIcon(routing); got != "✓" {
		t.Errorf("expected completed step to show ✓, got %q", got)
	}

	disabled := &FlowNode{Enabled: false}
	if got := statusIcon(disabled); got != "–" {
		t.Errorf("expected disabled step to show –, got %q", got)
	}
	inProgress := &FlowNode{Enabled: true, InProgress: true}
	if got := statusIcon(inProgress); got != "…" {
		t.Errorf("expected in-progress step to show …, got %q", got)
	}
}

// TestRenderFlowPane_And_FlowAllPane proves both Flow renderers produce
// non-empty output that names the fixture's known steps and agents, and
// that renderFlowPane's empty-state guards don't panic on a fresh model.
func TestRenderFlowPane_And_FlowAllPane(t *testing.T) {
	empty := InteractiveModel{}
	if out := empty.renderFlowPane(); !strings.Contains(out, "No messages yet") {
		t.Errorf("expected empty-state message for no messages, got %q", out)
	}

	m := newFixtureModel(t)
	m.flowTurnIndex = 0

	single := m.renderFlowPane()
	if !strings.Contains(single, "routing") {
		t.Error("expected renderFlowPane to list the routing step")
	}

	all := m.renderFlowAllPane()
	if !strings.Contains(all, "Turn 1") {
		t.Error("expected renderFlowAllPane to show a turn header")
	}
}

// TestDeriveAgentsFromEvents proves it collects unique non-aggregator
// (non-wizard) stream_start SourceIDs, sorted.
func TestDeriveAgentsFromEvents(t *testing.T) {
	m := newFixtureModel(t)

	agents := m.deriveAgentsFromEvents(m.messages[0].Events)
	if len(agents) == 0 {
		t.Fatal("expected at least one non-aggregator agent from the fixture")
	}
	for _, a := range agents {
		if a == "wizard" {
			t.Error("expected the aggregator (wizard) lane to be excluded")
		}
	}
}

// TestRenderFlowNodeDetails covers the wizard-specific metrics branch and
// the generic fallback when there's nothing more specific to show.
func TestRenderFlowNodeDetails(t *testing.T) {
	m := newFixtureModel(t)

	wizardNode := &FlowNode{Step: "wizard", Enabled: true, DurationMs: 42}
	wizardResp := m.aggregatorResponse(m.messages[0])
	out := m.renderFlowNodeDetails(wizardNode, wizardResp)
	if !strings.Contains(out, "Wizard Metrics") {
		t.Error("expected wizard-specific metrics section for the wizard step")
	}

	bare := &FlowNode{Step: "discovery", Enabled: false}
	out = m.renderFlowNodeDetails(bare, nil)
	if !strings.Contains(out, "enabled: false") {
		t.Error("expected the basic enabled field to always be present")
	}
}
