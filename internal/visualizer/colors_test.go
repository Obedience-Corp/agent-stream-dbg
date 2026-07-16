package visualizer

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lancekrogers/stream-debugger/internal/config"
)

// TestModelAgentColorOverrides proves Model.agentColorOverrides safely
// unwraps nil config/nil Display down to a nil map (getAgentColor's
// hash-only path), and correctly surfaces a configured agent_colors map
// otherwise.
func TestModelAgentColorOverrides(t *testing.T) {
	m := &Model{}
	if got := m.agentColorOverrides(); got != nil {
		t.Errorf("expected nil overrides for a Model with no config, got %v", got)
	}

	m.config = &config.EnhancedConfig{}
	if got := m.agentColorOverrides(); got != nil {
		t.Errorf("expected nil overrides for a Model with no Display config, got %v", got)
	}

	want := map[string]string{"agent_a": "42"}
	m.config = &config.EnhancedConfig{Display: &config.DisplayConfig{AgentColors: want}}
	got := m.agentColorOverrides()
	if len(got) != 1 || got["agent_a"] != "42" {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// TestInteractiveModelGetAgentColor_HonorsOverrideAndHashFallback
// regression-tests the bug this task's unification fixed: before it,
// interactive.go's getAgentColor fell back to flat gray for any agent
// absent from agent_colors, with no hash spread at all.
func TestInteractiveModelGetAgentColor_HonorsOverrideAndHashFallback(t *testing.T) {
	m := InteractiveModel{cfg: &config.EnhancedConfig{
		Display: &config.DisplayConfig{AgentColors: map[string]string{"agent_a": "42"}},
	}}

	if got := m.getAgentColor("agent_a"); got != lipgloss.Color("42") {
		t.Errorf("expected the configured override color %q, got %q", "42", got)
	}

	unconfigured := m.getAgentColor("agent_b")
	if unconfigured == lipgloss.Color("7") {
		t.Error("expected agent_b (absent from agent_colors) to get a hash-spread color, not flat gray")
	}
}

// TestModelAndInteractiveModel_AgreeOnAgentColor is this task's explicit
// Done-When: the same agent ID, with the same agent_colors config, must
// resolve to the same color from BOTH call paths (tui.go's Model and
// interactive.go's InteractiveModel) — proving the unification is a
// single shared function, not two copies that happen to agree today.
func TestModelAndInteractiveModel_AgreeOnAgentColor(t *testing.T) {
	overrides := map[string]string{"agent_a": "42"}
	cfg := &config.EnhancedConfig{Display: &config.DisplayConfig{AgentColors: overrides}}

	model := &Model{config: cfg}
	im := InteractiveModel{cfg: cfg}

	for _, id := range []string{"agent_a", "agent_b", ""} {
		fromModel := getAgentColor(id, model.agentColorOverrides())
		fromInteractive := string(im.getAgentColor(id))
		if fromModel != fromInteractive {
			t.Errorf("expected Model and InteractiveModel to resolve the same color for %q, got %q vs %q", id, fromModel, fromInteractive)
		}
	}
}
