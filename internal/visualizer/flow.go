package visualizer

import (
	"github.com/lancekrogers/stream-debugger/internal/bridge"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/mapping"
)

// flowState resolves stages and lane roles from the loaded dialect's
// flow: spec, or derives them live from observed events when the dialect
// declared none. Both TUI models read this state, so role metadata follows
// the same path as stage order instead of being rediscovered by renderers.
type flowState struct {
	spec    *mapping.FlowSpec
	deriver *mapping.FlowDeriver
	model   FlowModel
}

// newFlowState resolves the current dialect's flow: spec via internal/bridge's
// facade. When the dialect has no flow: block, the model starts empty and is
// filled from the observed event stream by FlowDeriver.
func newFlowState() flowState {
	spec := bridge.Flow()
	fs := flowState{spec: spec}
	if spec == nil {
		fs.deriver = mapping.NewFlowDeriver()
	}
	fs.refreshModel()
	return fs
}

// observe feeds evt into the deriver — a no-op when a flow: block was
// declared — and refreshes the derived flow model after each event.
func (f *flowState) observe(evt *events.Event) {
	if f.deriver != nil {
		f.deriver.Observe(evt)
		f.refreshModel()
	}
}

// role resolves evt's lane role from the loaded dialect, or the worker
// default when a flow: block was not declared.
func (f flowState) role(evt *events.Event) events.Role {
	if f.spec != nil {
		return f.spec.RoleFor(evt)
	}
	return f.deriver.RoleFor(evt)
}

// stageRole resolves a declared or derived stage through the flow model.
// A stage and its aggregator lane share the same source name in the dialect
// projection, so this is the stage-side view of the same role metadata.
func (f flowState) stageRole(name string) events.Role {
	for _, stage := range f.FlowModel().Stages {
		if stage.Name == name {
			return stage.Role
		}
	}
	return events.RoleWorker
}

// stages returns the declared stage order, or the dynamically-derived one
// if no flow: block was declared.
func (f flowState) stages() []string {
	if f.spec != nil {
		return f.spec.Stages
	}
	return f.deriver.Stages()
}

// refreshModel builds the renderer-facing flow projection from the dialect
// data or the observations accumulated by FlowDeriver.
func (f *flowState) refreshModel() {
	model := FlowModel{}
	if f.spec != nil {
		model.Lanes = make([]FlowLane, 0, len(f.spec.Lanes))
		for _, lane := range f.spec.Lanes {
			model.Lanes = append(model.Lanes, FlowLane{
				SourceID: lane.Match.Source,
				Role:     lane.Role,
				Label:    lane.Label,
			})
		}
		model.Stages = make([]FlowStage, 0, len(f.spec.Stages))
		for _, name := range f.spec.Stages {
			model.Stages = append(model.Stages, FlowStage{
				Name: name,
				Role: f.spec.RoleFor(&events.Event{SourceID: name}),
			})
		}
	} else if f.deriver != nil {
		lanes := f.deriver.Lanes()
		model.Lanes = make([]FlowLane, 0, len(lanes))
		for _, sourceID := range lanes {
			model.Lanes = append(model.Lanes, FlowLane{
				SourceID: sourceID,
				Role:     events.RoleWorker,
			})
		}
		stages := f.deriver.Stages()
		model.Stages = make([]FlowStage, 0, len(stages))
		for _, name := range stages {
			model.Stages = append(model.Stages, FlowStage{
				Name: name,
				Role: events.RoleWorker,
			})
		}
	}
	f.model = model
}

// FlowModel returns a copy of the current projection so callers can inspect
// dialect-declared roles without mutating flowState's internal slices.
func (f flowState) FlowModel() FlowModel {
	f.refreshModel()
	model := f.model
	model.Lanes = append([]FlowLane(nil), model.Lanes...)
	model.Stages = append([]FlowStage(nil), model.Stages...)
	return model
}
