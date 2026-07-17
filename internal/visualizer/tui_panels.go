package visualizer

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderFlowStatus renders a one-line flow status overview.
func (m *Model) renderFlowStatus() string {
	if m.width == 0 {
		return ""
	}
	steps := m.dialectFlow.stages()
	var parts []string
	for _, step := range steps {
		st, ok := m.flow[step]
		label := step
		if step == "agent_exec" {
			label = "agents"
		}
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		sym := "·"
		if ok {
			if !st.Enabled {
				sym = "–"
			} else if st.Ended {
				sym = "✓"
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
			} else if st.Started {
				sym = "…"
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
			}
		}
		text := fmt.Sprintf("%s %s", sym, label)
		if ok && st.Enabled && (st.RouteTaken != "" || st.RouteReason != "" || len(st.RouteAgents) > 0) {
			det := st.RouteTaken
			if det == "" {
				det = "—"
			}
			agents := ""
			if len(st.RouteAgents) > 0 {
				if m.showAgentsExpanded {
					agents = strings.Join(st.RouteAgents, ",")
				} else {
					max := 2
					if len(st.RouteAgents) < max {
						max = len(st.RouteAgents)
					}
					agents = strings.Join(st.RouteAgents[:max], ",")
					if len(st.RouteAgents) > max {
						agents = fmt.Sprintf("%s,+%d", agents, len(st.RouteAgents)-max)
					}
				}
			}
			if st.RouteReason != "" && agents != "" {
				text = fmt.Sprintf("%s(%s:%s [%s])", text, det, st.RouteReason, agents)
			} else if st.RouteReason != "" {
				text = fmt.Sprintf("%s(%s:%s)", text, det, st.RouteReason)
			} else if agents != "" {
				text = fmt.Sprintf("%s(%s [%s])", text, det, agents)
			} else {
				text = fmt.Sprintf("%s(%s)", text, det)
			}
		}
		if ok && st.DurationMs > 0 {
			text = fmt.Sprintf("%s [%dms]", text, st.DurationMs)
		}
		parts = append(parts, style.Render(text))
	}
	if len(parts) == 0 {
		return ""
	}
	sep := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("  |  ")
	joined := parts[0]
	for _, part := range parts[1:] {
		joined = lipgloss.JoinHorizontal(lipgloss.Top, joined, sep, part)
	}
	return joined
}

// renderPromptPanel renders the prompt info panel (debug mode only).
func (m *Model) renderPromptPanel() string {
	if !m.showPromptPanel {
		return ""
	}
	panelStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("5")).Padding(0, 1).Width(m.width - 4)
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	snippetStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	var lines []string
	if m.flowConfig != nil {
		lines = append(lines, headerStyle.Render("Flow Configuration"))
		lines = append(lines, fmt.Sprintf("  %s %s", labelStyle.Render("ID:"), valueStyle.Render(m.flowConfig.FlowID)))
		lines = append(lines, fmt.Sprintf("  %s %s", labelStyle.Render("File:"), valueStyle.Render(m.flowConfig.FlowFile)))
		if m.flowConfig.RoutingMode != "" {
			lines = append(lines, fmt.Sprintf("  %s %s", labelStyle.Render("Routing:"), valueStyle.Render(m.flowConfig.RoutingMode)))
		}
		lines = append(lines, fmt.Sprintf("  %s %d agents (%d non-aggregator)", labelStyle.Render("Agents:"), m.flowConfig.AgentCount, m.flowConfig.NonAggregatorCount))
		if len(m.flowConfig.StagesOrder) > 0 {
			lines = append(lines, fmt.Sprintf("  %s %s", labelStyle.Render("Stages:"), valueStyle.Render(strings.Join(m.flowConfig.StagesOrder, " → "))))
		}
		lines = append(lines, "")
	}
	if len(m.promptInfo) > 0 {
		lines = append(lines, headerStyle.Render("Agent Prompts"))
		for agentID, info := range m.promptInfo {
			color := getAgentColor(agentID, m.agentColorOverrides())
			agentName := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render(agentID)
			line := fmt.Sprintf("  %s", agentName)
			if info.PromptFile != "" {
				line += fmt.Sprintf(" %s %s", labelStyle.Render("→"), valueStyle.Render(info.PromptFile))
			}
			if info.PromptLength > 0 {
				line += fmt.Sprintf(" %s", labelStyle.Render(fmt.Sprintf("(%d chars)", info.PromptLength)))
			}
			lines = append(lines, line)
			if info.PromptSnippet != "" {
				snippet := info.PromptSnippet
				if len(snippet) > 100 {
					snippet = snippet[:100] + "..."
				}
				snippet = strings.ReplaceAll(snippet, "\n", " ")
				lines = append(lines, fmt.Sprintf("    %s", snippetStyle.Render(snippet)))
			}
			if info.SystemPrompt != "" {
				lines = append(lines, fmt.Sprintf("    %s", labelStyle.Render(fmt.Sprintf("[Full prompt loaded: %d chars]", len(info.SystemPrompt)))))
			}
		}
	}
	if len(lines) == 0 {
		lines = append(lines, labelStyle.Render("No prompt info available. Use [v] for verbose or [f] for full debug mode."))
	}
	return panelStyle.Render(strings.Join(lines, "\n"))
}
