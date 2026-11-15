package visualizer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
)

// presentation adapts bounded observer evidence to the existing TUI renderers.
// No interactive constructor is called: no logger, prompt client or setup request.
func (m *ObserverModel) presentation() InteractiveModel {
	p := InteractiveModel{passive: m, width: m.width, height: m.height, activePane: m.pane, viewMode: ViewModeParsed, flowTurnIndex: 0, selectedStepIndex: m.eventRow, flowExpanded: m.flowExpanded, agentCollapsed: m.agentCollapsed, eventExpanded: map[int]bool{}, selectedEventIdx: m.eventRow, showTokens: true, dialectFlow: newFlowStateWithParser(m.parser), animFrame: m.animFrame}
	message := Message{AgentResponses: map[string]*AgentResponse{}}
	records := m.selectedRecords()
	// Bound display projection independently of retained raw evidence.
	start := 0
	if m.pane == PaneEvents {
		start = (m.eventRow / passiveEventPage) * passiveEventPage
		p.selectedEventIdx = m.eventRow - start
	}
	for index, r := range records {
		e := r.Event
		if index >= start && len(message.Events) < passiveEventPage {
			copy := *e
			copy.Content = observerClip(e.Content, 1024)
			copy.Raw = nil
			copy.Fields = map[string]any{}
			for _, key := range []string{"step", "status", "tool_name", "call_id", "job_id", "route_taken", "reason", "message_role", "from", "to", "mode"} {
				copy.Fields[key] = observerClip(e.StringField(key), 256)
			}
			message.Events = append(message.Events, &copy)
		}
		p.dialectFlow.observe(e)
		role := e.StringField("message_role")
		if role == "user" {
			message.Text = observerClip(e.Content, 1024)
			continue
		}
		id := e.StringField("persona_id")
		if job := e.StringField("job_id"); job != "" {
			id = "worker " + job
		}
		if id == "" {
			id = e.SourceID
		}
		if id == "" {
			id = "conversation"
		}
		if e.Content == "" || (role != "assistant" && e.Kind != events.KindContent && e.Kind != events.KindToolCall && e.Kind != events.KindToolResult && e.Kind != events.KindStatusChange && e.Kind != events.KindHandoff) {
			continue
		}
		response := message.AgentResponses[id]
		if response == nil {
			if len(message.AgentResponses) >= 16 {
				continue
			}
			response = &AgentResponse{AgentID: id, Observed: true}
			message.AgentResponses[id] = response
		}
		content := observerClip(e.Content, 2048)
		if role == "assistant" && e.Kind != events.KindContent {
			response.FullContent = content
			response.Completed = true
		} else if len(response.FullContent) < 4096 {
			if e.Kind == events.KindContent {
				response.FullContent += content
			} else {
				response.FullContent += e.StringField("tool_name") + " " + content + "\n"
			}
		}
		if status := e.StringField("status"); status != "" {
			response.Completed = status == "finished" || status == "failed" || status == "aborted" || status == "completed"
		}
	}
	if message.Text == "" {
		message.Text = "Input text unavailable in retained events"
	}
	p.messages = []Message{message}
	p.flowNodesOverride, p.flowChildren = m.stageNodes()
	p.includeObservedStages()
	for _, n := range p.flowNodesOverride {
		if n.InProgress {
			p.streaming = true
		}
	}
	return p
}

// stageNodes measures each stage from its own intervals. Intervals nested
// under a stage are returned separately in start order and never contribute
// to that stage's status or duration.
func (m *ObserverModel) stageNodes() (map[string]*FlowNode, map[string][]*inspectedInterval) {
	nodes := map[string]*FlowNode{}
	children := map[string][]*inspectedInterval{}
	ranges := map[string][]measuredRange{}
	clocks := map[string]string{}
	running := map[string]float64{}
	for _, i := range m.IntervalReport().Intervals {
		if i.Gap {
			continue
		}
		if i.ParentStage != "" {
			children[i.ParentStage] = append(children[i.ParentStage], i)
			continue
		}
		n := nodes[i.Stage]
		if n == nil {
			n = &FlowNode{Step: i.Stage, Enabled: true}
			nodes[i.Stage] = n
		}
		if n.Status == "" || i.Status != "completed" {
			n.Status = i.Status
		}
		if i.Status == "running" && i.End == nil {
			n.InProgress = true
		}
		if elapsed, ok := m.runningElapsedMS(i); ok {
			if previous, seen := running[i.Stage]; !seen || elapsed > previous {
				running[i.Stage] = elapsed
			}
		}
		if !i.Invalid && !i.Uncorrelated && i.Start != nil && i.End != nil {
			clock := intervalClockKey(i)
			if prev := clocks[i.Stage]; prev != "" && prev != clock {
				clocks[i.Stage] = "mixed"
			} else {
				clocks[i.Stage] = clock
			}
			ranges[i.Stage] = append(ranges[i.Stage], measuredRange{*i.Start, *i.End})
		}
		if len(n.Evidence) < 8 {
			label := i.Lane
			if i.Segment != nil {
				label += fmt.Sprintf(" segment %d", *i.Segment)
			}
			n.Evidence = append(n.Evidence, fmt.Sprintf("%s · %s · %s → %s", label, i.Status, formatMeasured(i.Start), formatMeasured(i.End)))
		}
	}
	for stage, elapsed := range running {
		nodes[stage].RunningFor = formatElapsed(elapsed)
	}
	for stage, n := range nodes {
		if len(ranges[stage]) > 0 && clocks[stage] != "mixed" {
			n.DurationMs = int(rangeDuration(ranges[stage]))
			n.DurationKnown = true
		}
	}
	return nodes, children
}
func (m *ObserverModel) togglePassiveRow() {
	p := m.presentation()
	if m.pane == PaneFlow {
		rows := p.flowRows()
		if len(rows) == 0 {
			return
		}
		row := rows[min(m.eventRow, len(rows)-1)]
		if row.child != nil {
			m.details = true
			m.openObserverDetails()
			return
		}
		m.flowExpanded[row.stage] = !m.flowExpanded[row.stage]
	} else {
		ids := []string{}
		for id := range p.messages[0].AgentResponses {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		if len(ids) > 0 {
			id := ids[min(m.eventRow, len(ids)-1)]
			m.agentCollapsed[id] = !m.agentCollapsed[id]
		}
	}
}

func (m *ObserverModel) selectionLabel() string {
	groups := m.store.groups()
	if m.page != nil {
		groups = append(groups, m.page.groups()...)
	}
	for _, g := range groups {
		if g.Key == m.selected {
			return strings.TrimSpace(g.Label + " · " + g.Persona)
		}
	}
	return "Waiting for conversation events"
}

// Keep configured order, then include source stages such as background worker
// queue/execution that are absent from a dialect's foreground stage list.
func (p *InteractiveModel) includeObservedStages() {
	if p.dialectFlow.spec == nil {
		return
	}
	spec := *p.dialectFlow.spec
	spec.Stages = append([]string(nil), spec.Stages...)
	known := map[string]bool{}
	for _, stage := range spec.Stages {
		known[stage] = true
	}
	extra := []string{}
	for stage := range p.flowNodesOverride {
		if !known[stage] {
			extra = append(extra, stage)
			known[stage] = true
		}
	}
	for stage := range p.flowChildren {
		if !known[stage] {
			extra = append(extra, stage)
			known[stage] = true
		}
	}
	sort.Strings(extra)
	spec.Stages = append(spec.Stages, extra...)
	p.dialectFlow.spec = &spec
}
