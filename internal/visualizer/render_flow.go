package visualizer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lancekrogers/stream-debugger/internal/events"
)

// =============================================================================
// PANE RENDERERS
// =============================================================================

// buildFlowNodes extracts FlowNode state from parsed events
func (m InteractiveModel) buildFlowNodes(evts []*events.Event) map[string]*FlowNode {
	nodes := map[string]*FlowNode{}
	for _, e := range evts {
		if e.Name == "flow_step_start" {
			step := e.StringField("step")
			n := nodes[step]
			if n == nil {
				n = &FlowNode{Step: step}
			}
			n.Enabled = e.BoolField("enabled")
			n.InProgress = true
			if nonAggregatorCount := e.IntField(nonAggregatorCountFieldKey); step == "agent_exec" && nonAggregatorCount > 0 {
				n.AgentCount = nonAggregatorCount
			}
			nodes[step] = n
		}
		if e.Name == "flow_step_end" {
			step := e.StringField("step")
			n := nodes[step]
			if n == nil {
				n = &FlowNode{Step: step}
			}
			n.Enabled = e.BoolField("enabled")
			n.InProgress = false
			if agentCount := e.IntField("agent_count"); agentCount > 0 {
				n.AgentCount = agentCount
			}
			if durationMs := e.IntField("duration_ms"); durationMs > 0 {
				n.DurationMs = durationMs
			}
			n.PromptRef = e.MapField("prompt_ref")
			n.RoutingMode = e.StringField("routing_mode")
			n.RouteTaken = e.StringField("route_taken")
			n.RouteReason = e.StringField("route_reason")
			n.RouteAgents = e.StringSliceField("route_agents")
			if filteredCount := e.IntField("filtered_count"); filteredCount > 0 {
				n.FilteredCount = filteredCount
			}
			nodes[step] = n
		}
	}
	return nodes
}

// statusIcon returns the status indicator for a flow step
func statusIcon(n *FlowNode) string {
	if !n.Enabled {
		return "–"
	}
	if n.InProgress {
		return "…"
	}
	if n.DurationMs > 0 {
		return "✓"
	}
	return "…"
}

// renderFlowPane renders the Flow pane (F1) showing step rows
func (m InteractiveModel) renderFlowPane() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	b.WriteString(headerStyle.Render("Flow Steps"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("─────────────────────────────────────────"))
	b.WriteString("\n\n")

	// Select message for Flow view (latest by default, or navigated turn)
	if len(m.messages) == 0 {
		b.WriteString(dimStyle.Render("No messages yet. Send a message to see flow steps."))
		return b.String()
	}
	turn := m.flowTurnIndex
	if turn < 0 || turn >= len(m.messages) {
		turn = len(m.messages) - 1
	}
	msg := m.messages[turn]
	if len(msg.Events) == 0 {
		b.WriteString(dimStyle.Render("No flow events in latest message."))
		return b.String()
	}

	nodes := m.buildFlowNodes(msg.Events)
	steps := m.dialectFlow.stages()

	// Header detail: show turn index for context
	b.WriteString(dimStyle.Render(fmt.Sprintf("Turn %d of %d  ([ ] to navigate)", turn+1, len(m.messages))))
	b.WriteString("\n\n")

	for i, step := range steps {
		n := nodes[step]

		// Build the line
		var line strings.Builder

		// Status icon
		if n != nil {
			line.WriteString(statusIcon(n))
		} else {
			line.WriteString(" ")
		}
		line.WriteString(" ")

		// Step name
		line.WriteString(fmt.Sprintf("%-12s", step))

		// Metrics (if available)
		if n != nil {
			// Duration
			if n.DurationMs > 0 {
				line.WriteString(fmt.Sprintf(" [%dms]", n.DurationMs))
			}

			// Agent count for agent_exec
			if step == "agent_exec" && n.AgentCount > 0 {
				line.WriteString(fmt.Sprintf(" agents:%d", n.AgentCount))
			}

			// Filtered count for filter
			if step == "filter" && n.FilteredCount > 0 {
				line.WriteString(fmt.Sprintf(" filtered:%d", n.FilteredCount))
			}

			// Routing info
			if step == "routing" && n.RouteTaken != "" {
				line.WriteString(fmt.Sprintf(" → %s", n.RouteTaken))
				if len(n.RouteAgents) > 0 {
					line.WriteString(fmt.Sprintf(" [%s]", strings.Join(n.RouteAgents, ", ")))
				}
			}

			// Aggregator inline metrics (tokens, tokens/sec)
			if step == aggregatorStageName && msg.AgentResponses != nil {
				if aggResp := m.aggregatorResponse(msg); aggResp != nil {
					line.WriteString(fmt.Sprintf(" tokens:%d", aggResp.TokenCount))
					if aggResp.DurationMs > 0 && aggResp.TokenCount > 0 {
						tokensPerSec := float64(aggResp.TokenCount) * 1000.0 / float64(aggResp.DurationMs)
						line.WriteString(fmt.Sprintf(" %.1ftok/s", tokensPerSec))
					}
				}
			}
		}

		// Render with selection highlight
		lineStr := line.String()
		if i == m.selectedStepIndex {
			b.WriteString(selectedStyle.Render("> " + lineStr))
		} else {
			b.WriteString("  " + lineStr)
		}
		b.WriteString("\n")

		// Expanded details (if expanded and node exists)
		if m.flowExpanded[step] && n != nil {
			// For agent_exec, show agent list for this turn
			if step == "agent_exec" {
				agents := m.deriveAgentsFromEvents(msg.Events)
				if len(agents) == 0 && msg.RawSSE != "" {
					if parsed, err := parseSSEStream(msg.RawSSE); err == nil {
						agents = m.deriveAgentsFromEvents(parsed)
					}
				}
				if len(agents) > 0 {
					b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).PaddingLeft(4).Render(
						fmt.Sprintf("agents: [%s]", strings.Join(agents, ", ")),
					))
					b.WriteString("\n")
				}
			}
			// Pass aggregator response for the aggregator step's metrics
			var aggResp *AgentResponse
			if step == aggregatorStageName && msg.AgentResponses != nil {
				aggResp = m.aggregatorResponse(msg)
			}
			b.WriteString(m.renderFlowNodeDetails(n, aggResp))
		}
	}

	return b.String()
}

// renderFlowAllPane renders a continuous flow across all messages (turns)
func (m InteractiveModel) renderFlowAllPane() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	b.WriteString(headerStyle.Render("Flow (All Turns)"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("─────────────────────────────────────────"))
	b.WriteString("\n\n")

	if len(m.messages) == 0 {
		b.WriteString(dimStyle.Render("No messages yet. Send a message to see flow steps."))
		return b.String()
	}

	for i, msg := range m.messages {
		// Per-turn header with timestamp and user message
		ts := msg.Timestamp.Format("15:04:05")
		preview := msg.Text
		if len(preview) > 80 {
			preview = preview[:80] + "…"
		}
		b.WriteString(lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("Turn %d • %s", i+1, ts)))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("You: %s\n", preview))

		// Build nodes for this turn
		evts := msg.Events
		if len(evts) == 0 && msg.RawSSE != "" {
			if parsed, err := parseSSEStream(msg.RawSSE); err == nil {
				evts = parsed
			}
		}
		nodes := m.buildFlowNodes(evts)

		// Agent list: prefer routing.route_agents; else derive from AgentStreamStart
		var agents []string
		if rn := nodes["routing"]; rn != nil && len(rn.RouteAgents) > 0 {
			agents = append(agents, rn.RouteAgents...)
		}
		if len(agents) == 0 {
			// Derive from events: any stream_start whose lane isn't the
			// aggregator role, which gets its own dedicated summary
			// elsewhere rather than appearing in this worker-agent list.
			uniq := map[string]struct{}{}
			for _, e := range evts {
				if e.Kind == events.KindStreamStart && e.SourceID != "" && m.dialectFlow.role(e) != "aggregator" {
					uniq[e.SourceID] = struct{}{}
				}
			}
			for aid := range uniq {
				agents = append(agents, aid)
			}
			sort.Strings(agents)
		}
		if len(agents) > 0 {
			b.WriteString(fmt.Sprintf("Agents: [%s]\n", strings.Join(agents, ", ")))
		}

		// Step rows
		steps := m.dialectFlow.stages()
		for _, step := range steps {
			n := nodes[step]
			if n == nil {
				continue
			}

			var line strings.Builder
			line.WriteString(statusIcon(n))
			line.WriteString(" ")
			line.WriteString(fmt.Sprintf("%-12s", step))
			if n.DurationMs > 0 {
				line.WriteString(fmt.Sprintf(" [%dms]", n.DurationMs))
			}
			if step == "agent_exec" && n.AgentCount > 0 {
				line.WriteString(fmt.Sprintf(" agents:%d", n.AgentCount))
			}
			if step == "filter" && n.FilteredCount > 0 {
				line.WriteString(fmt.Sprintf(" filtered:%d", n.FilteredCount))
			}
			if step == "routing" && n.RouteTaken != "" {
				line.WriteString(fmt.Sprintf(" → %s", n.RouteTaken))
				if len(n.RouteAgents) > 0 {
					line.WriteString(fmt.Sprintf(" [%s]", strings.Join(n.RouteAgents, ", ")))
				}
			}
			// Aggregator inline metrics (tokens, tokens/sec)
			if step == aggregatorStageName && msg.AgentResponses != nil {
				if aggResp := m.aggregatorResponse(msg); aggResp != nil {
					line.WriteString(fmt.Sprintf(" tokens:%d", aggResp.TokenCount))
					if aggResp.DurationMs > 0 && aggResp.TokenCount > 0 {
						tokensPerSec := float64(aggResp.TokenCount) * 1000.0 / float64(aggResp.DurationMs)
						line.WriteString(fmt.Sprintf(" %.1ftok/s", tokensPerSec))
					}
				}
			}
			b.WriteString("  " + line.String() + "\n")

			// Expanded details across all turns for the selected step
			if m.flowExpanded[step] {
				if step == "agent_exec" {
					agents := m.deriveAgentsFromEvents(evts)
					if len(agents) > 0 {
						b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).PaddingLeft(4).Render(
							fmt.Sprintf("agents: [%s]", strings.Join(agents, ", ")),
						))
						b.WriteString("\n")
					}
				}
				// Pass aggregator response for the aggregator step's metrics
				var aggResp *AgentResponse
				if step == aggregatorStageName && msg.AgentResponses != nil {
					aggResp = m.aggregatorResponse(msg)
				}
				b.WriteString(m.renderFlowNodeDetails(n, aggResp))
			}
		}

		// Spacer between turns
		if i < len(m.messages)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// deriveAgentsFromEvents returns a sorted list of non-aggregator agents
// that streamed in this turn.
func (m InteractiveModel) deriveAgentsFromEvents(evts []*events.Event) []string {
	uniq := map[string]struct{}{}
	for _, e := range evts {
		if e.Kind == events.KindStreamStart && e.SourceID != "" && m.dialectFlow.role(e) != "aggregator" {
			uniq[e.SourceID] = struct{}{}
		}
	}
	if len(uniq) == 0 {
		return nil
	}
	agents := make([]string, 0, len(uniq))
	for aid := range uniq {
		agents = append(agents, aid)
	}
	sort.Strings(agents)
	return agents
}

// renderFlowNodeDetails renders expanded details for a flow node.
// aggResp is optional and only used when n is the aggregator step, to
// show its metrics.
func (m InteractiveModel) renderFlowNodeDetails(n *FlowNode, aggResp *AgentResponse) string {
	var b strings.Builder
	detailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).PaddingLeft(4)
	aggregatorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("13")).PaddingLeft(4)
	wrote := false

	// Basic fields always useful
	b.WriteString(detailStyle.Render(fmt.Sprintf("enabled: %v", n.Enabled)))
	b.WriteString("\n")
	wrote = true
	if n.DurationMs > 0 {
		b.WriteString(detailStyle.Render(fmt.Sprintf("duration_ms: %d", n.DurationMs)))
		b.WriteString("\n")
	}
	if n.AgentCount > 0 {
		b.WriteString(detailStyle.Render(fmt.Sprintf("agent_count: %d", n.AgentCount)))
		b.WriteString("\n")
	}

	// Aggregator-specific metrics (only for the aggregator step)
	if n.Step == aggregatorStageName && aggResp != nil {
		b.WriteString(aggregatorStyle.Render("── Aggregator Metrics ──"))
		b.WriteString("\n")
		b.WriteString(aggregatorStyle.Render(fmt.Sprintf("token_count: %d", aggResp.TokenCount)))
		b.WriteString("\n")
		if aggResp.FirstTokenMs > 0 {
			b.WriteString(aggregatorStyle.Render(fmt.Sprintf("first_token_ms: %d", aggResp.FirstTokenMs)))
			b.WriteString("\n")
		}
		if aggResp.DurationMs > 0 {
			b.WriteString(aggregatorStyle.Render(fmt.Sprintf("duration_ms: %d", aggResp.DurationMs)))
			b.WriteString("\n")
			// Calculate tokens/sec (cleaner formula: tokens * 1000 / ms)
			if aggResp.TokenCount > 0 {
				tokensPerSec := float64(aggResp.TokenCount) * 1000.0 / float64(aggResp.DurationMs)
				b.WriteString(aggregatorStyle.Render(fmt.Sprintf("tokens/sec: %.1f", tokensPerSec)))
				b.WriteString("\n")
			}
		}
		// Show plan_id from synthesis step prompt_ref if available
		if n.PromptRef != nil {
			if planID, ok := n.PromptRef["synthesis_plan_id"]; ok {
				b.WriteString(aggregatorStyle.Render(fmt.Sprintf("plan_id: %v", planID)))
				b.WriteString("\n")
			}
		}
		wrote = true
	}

	if n.RoutingMode != "" {
		b.WriteString(detailStyle.Render(fmt.Sprintf("mode: %s", n.RoutingMode)))
		b.WriteString("\n")
		wrote = true
	}
	if n.RouteReason != "" {
		b.WriteString(detailStyle.Render(fmt.Sprintf("reason: %s", n.RouteReason)))
		b.WriteString("\n")
		wrote = true
	}
	if m.showPromptRef && n.PromptRef != nil && len(n.PromptRef) > 0 {
		for k, v := range n.PromptRef {
			b.WriteString(detailStyle.Render(fmt.Sprintf("%s: %v", k, v)))
			b.WriteString("\n")
		}
		wrote = true
	}

	if !wrote {
		b.WriteString(detailStyle.Render("No details. Press 'p' to show YAML refs."))
		b.WriteString("\n")
	}

	return b.String()
}
