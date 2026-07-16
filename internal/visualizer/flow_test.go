package visualizer

import (
	"strings"
	"testing"

	"github.com/lancekrogers/stream-debugger/internal/config"
)

// TestModelAndInteractiveModel_AgreeOnStages is this task's explicit
// Done-When: tui.go's Model and interactive.go's InteractiveModel must
// read the identical stage set — structurally impossible to silently
// diverge again, not just fixed once by hand-editing both literals to
// match. Both now resolve through the same flowState.stages() call.
func TestModelAndInteractiveModel_AgreeOnStages(t *testing.T) {
	tuiModel := &Model{dialectFlow: newFlowState()}
	interactive := NewInteractiveModel(&config.EnhancedConfig{})

	tuiStages := tuiModel.dialectFlow.stages()
	interactiveStages := interactive.dialectFlow.stages()

	if len(tuiStages) != len(interactiveStages) {
		t.Fatalf("expected identical stage counts, got tui=%v interactive=%v", tuiStages, interactiveStages)
	}
	for i := range tuiStages {
		if tuiStages[i] != interactiveStages[i] {
			t.Errorf("stage %d differs: tui=%q interactive=%q", i, tuiStages[i], interactiveStages[i])
		}
	}
}

// TestModel_RenderFlowStatus_IncludesFilterStage is the historical bug,
// fixed by construction: tui.go's renderFlowStatus used to hardcode a
// 5-stage literal that silently omitted "filter" while interactive.go's
// copies all included it. Both now read from the same dialect-declared
// (or derived) stage list, so the omission is structurally impossible,
// not just patched for the one dialect that happened to expose it.
func TestModel_RenderFlowStatus_IncludesFilterStage(t *testing.T) {
	m := &Model{
		width:       80,
		flow:        make(map[string]*FlowStepStatus),
		dialectFlow: newFlowState(),
	}
	out := m.renderFlowStatus()
	if !strings.Contains(out, "filter") {
		t.Errorf("expected renderFlowStatus to include the 'filter' stage, got: %s", out)
	}
}
