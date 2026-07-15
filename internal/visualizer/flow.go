package visualizer

import (
	"github.com/lancekrogers/stream-debugger/internal/bridge"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/mapping"
)

// flowState resolves stages (and, from the next task in this sequence
// onward, lane roles) from the loaded dialect's flow: spec, or derives
// them live from observed events when the dialect declared none — the
// single source both Model (tui.go) and InteractiveModel
// (interactive.go) read stages from, replacing what used to be 6
// independently hardcoded literals that had already drifted out of sync
// with each other (tui.go's copy silently omitted filter).
type flowState struct {
	spec    *mapping.FlowSpec
	deriver *mapping.FlowDeriver
}

// newFlowState resolves the current dialect's flow: spec via
// internal/bridge's facade (the same single-load-per-process pattern
// bridge already uses for Parse/RenderSend/RunSetup), falling back to a
// fresh FlowDeriver when the dialect declared no flow: block.
func newFlowState() flowState {
	spec := bridge.Flow()
	fs := flowState{spec: spec}
	if spec == nil {
		fs.deriver = mapping.NewFlowDeriver()
	}
	return fs
}

// stages returns the declared stage order, or the dynamically-derived
// one if no flow: block was declared.
func (f flowState) stages() []string {
	if f.spec != nil {
		return f.spec.Stages
	}
	return f.deriver.Stages()
}

// observe feeds evt into the deriver — a no-op when a flow: block was
// declared, since there's nothing left to derive. Call this once per
// event, in order, as events are processed.
func (f flowState) observe(evt *events.Event) {
	if f.deriver != nil {
		f.deriver.Observe(evt)
	}
}
