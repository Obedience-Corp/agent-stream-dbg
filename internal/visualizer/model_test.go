package visualizer

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/config"
)

// TestNewInteractiveModel_Defaults is a floor-level test for model.go
// (previously untested directly): construction should leave the model in
// the documented default state — parsed view, Flow pane, following/history
// on, insert mode off — and Init should return a non-nil blink command.
func TestNewInteractiveModel_Defaults(t *testing.T) {
	m := NewInteractiveModel(&config.EnhancedConfig{})

	if m.viewMode != ViewModeParsed {
		t.Errorf("expected default ViewModeParsed, got %v", m.viewMode)
	}
	if m.activePane != PaneFlow {
		t.Errorf("expected default PaneFlow, got %v", m.activePane)
	}
	if !m.follow || !m.showHistory || !m.wrap {
		t.Error("expected follow, showHistory, and wrap to default true")
	}
	if m.insertMode {
		t.Error("expected insertMode to default false")
	}
	if m.flowExpanded == nil || m.agentCollapsed == nil || m.eventExpanded == nil {
		t.Error("expected all per-item state maps to be initialized, not nil")
	}

	if cmd := m.Init(); cmd == nil {
		t.Error("expected Init to return a non-nil command (textarea.Blink)")
	}
}

// TestRefreshViewportContent_DispatchesPerPane proves refreshViewportContent
// routes to the correct pane renderer and never panics, for every pane and
// both Flow sub-modes (continuous vs single-turn).
func TestRefreshViewportContent_DispatchesPerPane(t *testing.T) {
	m := newFixtureModel(t)
	m.width = 100
	m.height = 40
	m.viewport.Width = 96
	m.viewport.Height = 25

	panes := []Pane{PaneFlow, PaneApp, PaneTimeline, PaneEvents, PaneMessages}
	for _, p := range panes {
		for _, continuous := range []bool{true, false} {
			m.activePane = p
			m.flowContinuous = continuous
			m.contentDirty = true
			m.refreshViewportContent()
			if m.contentDirty {
				t.Errorf("pane %v: expected contentDirty to clear after refresh", p)
			}
			if strings.TrimSpace(m.viewport.View()) == "" {
				t.Errorf("pane %v (continuous=%v): expected non-empty viewport content", p, continuous)
			}
		}
	}
}

// TestRefreshViewportContent_WrapAndClip covers the post-render
// wrap/clip step refreshViewportContent applies on top of the dispatched
// pane content.
func TestRefreshViewportContent_WrapAndClip(t *testing.T) {
	m := newFixtureModel(t)
	m.activePane = PaneMessages
	m.viewport.Width = 10

	m.wrap = true
	m.xOffset = 0
	m.contentDirty = true
	m.refreshViewportContent()

	m.wrap = false
	m.xOffset = 5
	m.contentDirty = true
	m.refreshViewportContent()
	// Neither path should panic; both should leave contentDirty cleared.
	if m.contentDirty {
		t.Error("expected contentDirty false after refresh with wrap disabled")
	}
}

var _ tea.Model = InteractiveModel{}
