package visualizer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/visualizer/anim"
)

// renderAppPane renders the App pane (F2) showing aggregator output and agent summaries
func (m InteractiveModel) renderAppPane() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	focusStyle := lipgloss.NewStyle().Bold(true).Underline(true)

	b.WriteString(headerStyle.Render("App Output"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("─────────────────────────────────────────"))
	b.WriteString("\n\n")

	if len(m.messages) == 0 {
		b.WriteString(dimStyle.Render("No messages yet."))
		return b.String()
	}

	msg := m.messages[len(m.messages)-1]

	// Render based on focus: aggregator first if focused, agents first if focused
	if m.appFocus == AppFocusAggregator {
		b.WriteString(m.renderSynthesisSection(msg))
		b.WriteString(m.renderAggregatorSection(msg, focusStyle))
		b.WriteString(m.renderAgentSummaries(msg))
	} else {
		b.WriteString(m.renderAgentSummaries(msg))
		b.WriteString(m.renderSynthesisSection(msg))
		b.WriteString(m.renderAggregatorSection(msg, lipgloss.NewStyle()))
	}

	return b.String()
}

// renderSynthesisSection renders the synthesis output (separate from the aggregator's own output)
func (m InteractiveModel) renderSynthesisSection(msg Message) string {
	var b strings.Builder

	// Look for synthesis_detail events in the message
	var synthesisContent string
	var synthesisPlanID string
	var synthesisMethod string
	var sourcesCombined int

	for _, evt := range msg.Events {
		if evt.Name == "synthesis_detail" {
			synthesisContent = evt.StringField("synthesis_full")
			synthesisPlanID = evt.StringField("plan_id")
			synthesisMethod = evt.StringField("synthesis_method")
			sourcesCombined = evt.IntField("sources_combined")
			break
		}
	}

	if synthesisContent == "" {
		return ""
	}

	synthesisStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Italic(true)

	b.WriteString(synthesisStyle.Render("Synthesis Output"))
	b.WriteString("\n")

	// Show synthesis metadata
	if synthesisPlanID != "" || synthesisMethod != "" {
		var meta []string
		if synthesisPlanID != "" {
			meta = append(meta, fmt.Sprintf("plan: %s", synthesisPlanID))
		}
		if synthesisMethod != "" {
			meta = append(meta, fmt.Sprintf("method: %s", synthesisMethod))
		}
		if sourcesCombined > 0 {
			meta = append(meta, fmt.Sprintf("sources: %d", sourcesCombined))
		}
		b.WriteString(metaStyle.Render("[Synthesis] " + strings.Join(meta, ", ")))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(synthesisContent)
	b.WriteString("\n\n")
	return b.String()
}

// renderAggregatorSection renders the aggregator output section
func (m InteractiveModel) renderAggregatorSection(msg Message, titleStyle lipgloss.Style) string {
	var b strings.Builder
	agg := m.aggregatorResponse(msg)
	if agg == nil || agg.FullContent == "" {
		return ""
	}

	aggregatorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true)
	if titleStyle.GetUnderline() {
		b.WriteString(titleStyle.Foreground(lipgloss.Color("13")).Render("Aggregator Output"))
	} else {
		b.WriteString(aggregatorStyle.Render("Aggregator Output"))
	}
	b.WriteString("\n")

	// Show aggregator debug summary above content
	b.WriteString(m.renderAggregatorDebugSummary(agg))
	b.WriteString("\n")

	b.WriteString(agg.FullContent)
	b.WriteString("\n\n")
	return b.String()
}

// renderAggregatorDebugSummary renders a compact one-line aggregator metrics summary
func (m InteractiveModel) renderAggregatorDebugSummary(agg *AgentResponse) string {
	if agg == nil {
		return ""
	}

	summaryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Italic(true)

	var parts []string

	// Token count
	parts = append(parts, fmt.Sprintf("tokens: %d", agg.TokenCount))

	// Tokens per second (only if we have duration)
	if agg.DurationMs > 0 && agg.TokenCount > 0 {
		tokensPerSec := float64(agg.TokenCount) * 1000.0 / float64(agg.DurationMs)
		parts = append(parts, fmt.Sprintf("rate: %.1f tok/s", tokensPerSec))
	}

	// First token latency
	if agg.FirstTokenMs > 0 {
		parts = append(parts, fmt.Sprintf("first-token: %dms", agg.FirstTokenMs))
	}

	// Status indicator + mini energy bar only while actively streaming.
	s := anim.DefaultStyles()
	reduced := anim.ReducedMotion()
	active := !agg.Completed && (m.streaming || agg.TokenCount > 0)
	parts = append(parts, anim.AggregatorGlyph(active, agg.Completed, m.animFrame, s, reduced))
	if active {
		level := 0.35
		if agg.DurationMs > 0 && agg.TokenCount > 0 {
			level = anim.NormalizeRate(float64(agg.TokenCount) * 1000.0 / float64(agg.DurationMs))
		}
		parts = append(parts, anim.MiniBar(m.animFrame, level, true, 8, s, reduced))
	}

	return summaryStyle.Render("[Aggregator] " + strings.Join(parts, ", "))
}

// renderAgentSummaries renders the agent summaries section with collapse/expand
func (m InteractiveModel) renderAgentSummaries(msg Message) string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))

	// Sort agents for stable display (excluding the aggregator lane,
	// which gets its own dedicated summary elsewhere).
	agentIDs := make([]string, 0, len(msg.AgentResponses))
	for id := range msg.AgentResponses {
		if m.dialectFlow.role(&events.Event{SourceID: id}) != "aggregator" {
			agentIDs = append(agentIDs, id)
		}
	}
	sort.Strings(agentIDs)

	if len(agentIDs) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render(fmt.Sprintf("Agents (%d)", len(agentIDs))))
	b.WriteString("\n")

	for _, agentID := range agentIDs {
		resp := msg.AgentResponses[agentID]
		if resp == nil || resp.FullContent == "" {
			continue
		}

		agentColor := m.getAgentColor(agentID)
		agentStyle := lipgloss.NewStyle().Foreground(agentColor).Bold(true)

		// Collapse/expand indicator
		collapsed := m.agentCollapsed[agentID]
		caret := "▼"
		if collapsed {
			caret = "▶"
		}

		s := anim.DefaultStyles()
		// Tint agent anim with the agent color when possible.
		s.Agent = lipgloss.NewStyle().Foreground(agentColor).Bold(true)
		reduced := anim.ReducedMotion()
		active := !resp.Completed && (m.streaming || resp.TokenCount > 0)
		glyph := anim.AgentGlyph(active, resp.Completed, m.animFrame, s, reduced)
		header := fmt.Sprintf("%s %s %s (%d tokens)", caret, glyph, agentID, resp.TokenCount)
		if active {
			level := 0.4
			if resp.DurationMs > 0 && resp.TokenCount > 0 {
				level = anim.NormalizeRate(float64(resp.TokenCount) * 1000.0 / float64(resp.DurationMs))
			}
			header += " " + anim.MiniBar(m.animFrame, level, true, 8, s, reduced)
		}
		b.WriteString(agentStyle.Render(header))
		b.WriteString("\n")

		// Show content only if not collapsed
		if !collapsed {
			b.WriteString(lipgloss.NewStyle().Foreground(agentColor).Render(resp.FullContent))
			b.WriteString("\n\n")
		}
	}

	return b.String()
}
