package visualizer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lancekrogers/stream-debugger/internal/events"
)

// renderTimelinePane renders the Timeline pane (F3) showing agent execution timeline
func (m InteractiveModel) renderTimelinePane() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	b.WriteString(headerStyle.Render("Timeline"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("─────────────────────────────────────────"))
	b.WriteString("\n\n")

	if len(m.messages) == 0 {
		b.WriteString(dimStyle.Render("No messages yet."))
		return b.String()
	}

	msg := m.messages[len(m.messages)-1]

	// Show stage durations from flow events
	nodes := m.buildFlowNodes(msg.Events)
	stages := m.dialectFlow.stages()

	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Stages"))
	b.WriteString("\n")

	totalDuration := 0
	for _, stage := range stages {
		if n := nodes[stage]; n != nil && n.DurationMs > 0 {
			totalDuration += n.DurationMs
		}
	}

	for _, stage := range stages {
		n := nodes[stage]
		if n == nil {
			continue
		}

		// Calculate bar width proportional to duration
		barWidth := 0
		if totalDuration > 0 && n.DurationMs > 0 {
			barWidth = (n.DurationMs * 40) / totalDuration
			if barWidth < 1 {
				barWidth = 1
			}
		}

		bar := strings.Repeat("█", barWidth)

		b.WriteString(fmt.Sprintf("%-12s %s %dms\n", stage, bar, n.DurationMs))
	}

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Total: %dms\n", totalDuration))

	// Show agent timeline bars
	if len(msg.AgentResponses) > 0 {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Agents"))
		b.WriteString("\n")

		// Calculate max tokens for bar scaling
		maxTokens := 0
		for _, resp := range msg.AgentResponses {
			if resp.TokenCount > maxTokens {
				maxTokens = resp.TokenCount
			}
		}

		// Sort agents for stable display
		agentIDs := make([]string, 0, len(msg.AgentResponses))
		for id := range msg.AgentResponses {
			agentIDs = append(agentIDs, id)
		}
		sort.Strings(agentIDs)

		for _, agentID := range agentIDs {
			resp := msg.AgentResponses[agentID]
			status := "⏳"
			if resp.Completed {
				status = "✓"
			}

			// Calculate bar width proportional to tokens
			barWidth := 0
			if maxTokens > 0 && resp.TokenCount > 0 {
				barWidth = (resp.TokenCount * 30) / maxTokens
				if barWidth < 1 {
					barWidth = 1
				}
			}

			bar := strings.Repeat("▓", barWidth)
			agentColor := m.getAgentColor(agentID)
			barStyled := lipgloss.NewStyle().Foreground(agentColor).Render(bar)

			b.WriteString(fmt.Sprintf("%s %-15s %s %d tokens\n", status, agentID, barStyled, resp.TokenCount))
		}
	}

	return b.String()
}

// isEventVisible checks if an event should be visible based on current filter settings
func (m InteractiveModel) isEventVisible(evt *events.Event) bool {
	// Check the normalized event kind and the dialect-declared lane role.
	isAggregatorEvent := m.dialectFlow.role(evt) == events.RoleAggregator &&
		(evt.Kind == events.KindStreamStart || evt.Kind == events.KindContent || evt.Kind == events.KindStreamEnd)
	// Aggregator-role stages are also relevant for the aggregator-only view.
	step := evt.StringField("step")
	isSynthesisFlowEvent := (evt.Kind == events.KindStepStart || evt.Kind == events.KindStepEnd) &&
		m.dialectFlow.stageRole(step) == events.RoleAggregator

	// Skip non-aggregator events if the aggregator-only filter is on
	if m.eventsAggregatorOnly && !isAggregatorEvent && !isSynthesisFlowEvent {
		return false
	}

	// Skip token events if showTokens is false
	isTokenEvent := evt.Kind == events.KindContent
	if isTokenEvent && !m.showTokens {
		return false
	}

	return true
}

// countVisibleEvents counts events that are visible after filtering
func (m InteractiveModel) countVisibleEvents() int {
	if len(m.messages) == 0 {
		return 0
	}
	msg := m.messages[len(m.messages)-1]
	count := 0
	for _, evt := range msg.Events {
		if m.isEventVisible(evt) {
			count++
		}
	}
	return count
}
