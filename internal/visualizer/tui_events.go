package visualizer

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lancekrogers/stream-debugger/internal/events"
)

// handleEvent processes a decoded stream event.
func (m *Model) handleEvent(event *events.Event) (tea.Model, tea.Cmd) {
	if m.paused {
		return m, m.waitForEvent()
	}
	m.totalEvents++
	m.dialectFlow.observe(event)
	_ = m.logger.LogEvent(event)
	isAggregator := m.dialectFlow.role(event) == events.RoleAggregator

	switch event.Kind {
	case events.KindStreamStart:
		if isAggregator {
			m.aggregatorState.Role = m.dialectFlow.role(event)
			m.aggregatorState.Active = true
			m.aggregatorState.StartTime = time.Now()
			break
		}
		agentID := event.SourceID
		m.agents[agentID] = &AgentState{ID: agentID, Role: m.dialectFlow.role(event), Active: true, BufferedTokens: make(map[int]string), StartTime: time.Now(), LastUpdate: time.Now()}
	case events.KindContent:
		if isAggregator {
			m.aggregatorState.Content.WriteString(event.Content)
			m.aggregatorState.TokenCount++
			m.aggregatorState.Sequence = event.Seq
			m.totalTokens++
			break
		}
		agentID := event.SourceID
		if agent, ok := m.agents[agentID]; ok {
			agent.Content.WriteString(event.Content)
			agent.TokenCount++
			agent.Sequence = event.Seq
			agent.LastUpdate = time.Now()
			m.totalTokens++
		}
	case events.KindStreamEnd:
		if isAggregator {
			m.aggregatorState.Active = false
			m.aggregatorState.EndTime = time.Now()
			break
		}
		if agent, ok := m.agents[event.SourceID]; ok {
			agent.Active = false
			agent.EndTime = time.Now()
		}
	}

	switch event.Name {
	case "session_start":
		m.sessionActive = true
		if flowID := event.StringField("flow_id"); flowID != "" {
			m.FlowID = flowID
		}
	case "session_complete":
		m.sessionActive = false
	case "error":
		m.errorCount++
	case "flow_step_start":
		step := event.StringField("step")
		st := m.flow[step]
		if st == nil {
			st = &FlowStepStatus{Step: step, Role: m.dialectFlow.stageRole(step)}
			m.flow[step] = st
		}
		st.Enabled = event.BoolField("enabled")
		st.Started = true
		st.Ended = false
		if step == "agent_exec" {
			st.AgentCount = event.ParticipantCount()
		}
	case "flow_step_end":
		step := event.StringField("step")
		st := m.flow[step]
		if st == nil {
			st = &FlowStepStatus{Step: step, Role: m.dialectFlow.stageRole(step)}
			m.flow[step] = st
		}
		st.Enabled = event.BoolField("enabled")
		st.Ended = true
		if agentCount := event.IntField("agent_count"); agentCount > 0 {
			st.AgentCount = agentCount
		}
		if event.StringField("route_taken") != "" || event.StringField("route_reason") != "" || len(event.StringSliceField("route_agents")) > 0 {
			st.RoutingMode = event.StringField("routing_mode")
			st.RouteTaken = event.StringField("route_taken")
			st.RouteReason = event.StringField("route_reason")
			st.RouteAgents = event.StringSliceField("route_agents")
		}
		if durationMs := event.IntField("duration_ms"); durationMs > 0 {
			st.DurationMs = durationMs
		}
		if promptRef := event.MapField("prompt_ref"); promptRef != nil {
			m.lastPromptRef = promptRef
			m.lastPromptStep = step
		}
	case "prompt_info":
		agentID := event.StringField("agent_id")
		state := m.promptInfo[agentID]
		if state == nil {
			state = &PromptInfoState{AgentID: agentID}
			m.promptInfo[agentID] = state
		}
		state.PromptFile = event.StringField("prompt_file")
		state.PromptSnippet = event.StringField("prompt_snippet")
		state.PromptLength = event.IntField("prompt_length")
	case "prompt_full":
		agentID := event.StringField("agent_id")
		state := m.promptInfo[agentID]
		if state == nil {
			state = &PromptInfoState{AgentID: agentID}
			m.promptInfo[agentID] = state
		}
		state.SystemPrompt = event.StringField("system_prompt")
	case "flow_config":
		flowID := event.StringField("flow_id")
		m.flowConfig = &FlowConfigState{FlowID: flowID, FlowFile: event.StringField("flow_file"), StagesOrder: event.StringSliceField("stages_order"), StagesEnabled: event.BoolMapField("stages_enabled"), RoutingMode: event.StringField("routing_mode"), AgentCount: event.IntField("agent_count"), NonAggregatorCount: event.ParticipantCount()}
		if m.FlowID == "" && flowID != "" {
			m.FlowID = flowID
		}
	}

	return m, m.waitForEvent()
}
