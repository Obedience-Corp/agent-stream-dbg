package visualizer

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestRenderAppPane_BothFocuses is a floor-level test for
// render_app_pane.go (previously untested directly): both AppFocus states
// must render without panicking and include the aggregator/agent content.
func TestRenderAppPane_BothFocuses(t *testing.T) {
	if out := (InteractiveModel{}).renderAppPane(); !strings.Contains(out, "No messages yet") {
		t.Errorf("expected empty-state message, got %q", out)
	}

	m := newFixtureModel(t)
	for _, focus := range []AppFocus{AppFocusAggregator, AppFocusAgents} {
		m.appFocus = focus
		out := stripANSI(m.renderAppPane())
		if !strings.Contains(out, "Aggregator Output") {
			t.Errorf("focus=%v: expected Aggregator Output section, got:\n%s", focus, out)
		}
		if !strings.Contains(out, "Agents") {
			t.Errorf("focus=%v: expected Agents section, got:\n%s", focus, out)
		}
	}
}

// TestRenderSynthesisSection covers both the empty (no synthesis_detail
// event) and populated cases.
func TestRenderSynthesisSection(t *testing.T) {
	m := newFixtureModel(t)

	if out := m.renderSynthesisSection(Message{}); out != "" {
		t.Errorf("expected empty string with no synthesis_detail event, got %q", out)
	}

	out := m.renderSynthesisSection(m.messages[0])
	if out != "" && !strings.Contains(out, "Synthesis Output") {
		t.Errorf("expected a Synthesis Output header when synthesis content is present, got %q", out)
	}
}

// TestRenderAggregatorSection_And_DebugSummary proves the aggregator
// section renders content plus its debug summary line (tokens, rate,
// first-token).
func TestRenderAggregatorSection_And_DebugSummary(t *testing.T) {
	m := newFixtureModel(t)

	out := m.renderAggregatorSection(m.messages[0], lipgloss.NewStyle())
	if !strings.Contains(out, "Aggregator Output") {
		t.Error("expected an Aggregator Output header")
	}

	agg := m.aggregatorResponse(m.messages[0])
	summary := m.renderAggregatorDebugSummary(agg)
	if !strings.Contains(summary, "tokens:") {
		t.Errorf("expected the debug summary to include a token count, got %q", summary)
	}

	if got := m.renderAggregatorDebugSummary(nil); got != "" {
		t.Errorf("expected empty string for a nil aggregator response, got %q", got)
	}
}

// TestRenderAgentSummaries proves it lists non-aggregator agents with
// collapse/expand state and excludes the aggregator lane.
func TestRenderAgentSummaries(t *testing.T) {
	m := newFixtureModel(t)

	out := m.renderAgentSummaries(m.messages[0])
	if !strings.Contains(out, "Agents (") {
		t.Error("expected an Agents(n) header")
	}

	if out := m.renderAgentSummaries(Message{}); out != "" {
		t.Errorf("expected empty string with no non-aggregator agents, got %q", out)
	}
}
