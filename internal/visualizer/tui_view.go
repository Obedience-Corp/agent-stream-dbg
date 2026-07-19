package visualizer

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/lancekrogers/stream-debugger/internal/visualizer/anim"
)

// View renders the TUI.
func (m *Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("6")).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("6")).
		Padding(0, 1)
	agentStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(1).
		Width(m.width/2 - 4)
	aggregatorStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("11")).
		Padding(1).
		Width(m.width - 4)
	statsStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Padding(0, 1)

	headerText := fmt.Sprintf("Stream Debugger - Session: %s", m.config.Session.ID)
	if m.FlowID != "" {
		headerText = fmt.Sprintf("%s (flow: %s)", headerText, m.FlowID)
	}
	header := headerStyle.Render(headerText)
	// Energy strip under the legacy header.
	duration := time.Since(m.startTime)
	tokensPerSec := 0.0
	if duration.Seconds() > 0 {
		tokensPerSec = float64(m.totalTokens) / duration.Seconds()
	}
	live := m.sessionActive
	for _, a := range m.agents {
		if a != nil && a.Active {
			live = true
			break
		}
	}
	if m.aggregatorState != nil && m.aggregatorState.Active {
		live = true
	}
	s := anim.DefaultStyles()
	reduced := anim.ReducedMotion()
	meter := anim.HeaderMeter(m.animFrame, tokensPerSec, live, m.errorCount > 0 && !live, m.width-6, s, reduced)

	flowView := m.renderFlowStatus()
	promptView := ""
	if m.showPromptRef && m.lastPromptRef != nil {
		pairs := []string{}
		for k, v := range m.lastPromptRef {
			pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
		}
		if len(pairs) > 0 {
			promptView = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(
				fmt.Sprintf("prompt_ref(%s): %s", m.lastPromptStep, strings.Join(pairs, ", ")),
			)
		}
	}

	var agentViews []string
	for _, agent := range m.agents {
		if agent.Active || agent.TokenCount > 0 {
			agentViews = append(agentViews, agentStyle.Render(m.renderAgent(agent)))
		}
	}
	aggregatorView := ""
	if m.aggregatorState.Active || m.aggregatorState.TokenCount > 0 {
		aggregatorView = aggregatorStyle.Render(m.renderAggregator())
	}

	stats := statsStyle.Render(fmt.Sprintf(
		"Events: %d | Tokens: %d | Tokens/sec: %.1f | Errors: %d | Duration: %s",
		m.totalEvents, m.totalTokens, tokensPerSec, m.errorCount, duration.Round(time.Second),
	))
	dbg := m.config.Debug.Level
	if dbg == "" {
		dbg = "off"
	}
	controls := statsStyle.Render(fmt.Sprintf("^P pause | ^A agents | ^R refs | ^I prompts | F1 verbose | F2 full | F3 off | ^C quit | debug=%s", dbg))
	promptPanelView := m.renderPromptPanel()

	sections := []string{header, meter}
	if flowView != "" {
		sections = append(sections, flowView)
	}
	if promptView != "" {
		sections = append(sections, promptView)
	}
	if promptPanelView != "" {
		sections = append(sections, promptPanelView)
	}
	if len(agentViews) > 0 {
		rows := (len(agentViews) + 1) / 2
		for i := 0; i < rows; i++ {
			left := agentViews[i*2]
			right := ""
			if i*2+1 < len(agentViews) {
				right = agentViews[i*2+1]
			}
			sections = append(sections, lipgloss.JoinHorizontal(lipgloss.Top, left, right))
		}
	}
	if aggregatorView != "" {
		sections = append(sections, aggregatorView)
	}
	sections = append(sections, stats, controls)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderAgent renders a single agent's view.
func (m *Model) renderAgent(agent *AgentState) string {
	s := anim.DefaultStyles()
	color := getAgentColor(agent.ID, m.agentColorOverrides())
	s.Agent = lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)
	reduced := anim.ReducedMotion()
	done := !agent.Active && agent.TokenCount > 0 && !agent.EndTime.IsZero()
	status := anim.AgentGlyph(agent.Active, done, m.animFrame, s, reduced)
	agentName := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render(agent.ID)
	level := 0.0
	if agent.Active {
		level = 0.5
	}
	bar := anim.MiniBar(m.animFrame, level, agent.Active, 8, s, reduced)
	header := fmt.Sprintf("%s %s %s", status, agentName, bar)
	content := agent.Content.String()
	if len(content) > 200 {
		content = "..." + content[len(content)-200:]
	}
	tokenInfo := fmt.Sprintf("Tokens: %d | Seq: %d", agent.TokenCount, agent.Sequence)
	if len(agent.BufferedTokens) > 0 {
		tokenInfo += fmt.Sprintf(" | Buffered: %d", len(agent.BufferedTokens))
	}
	return fmt.Sprintf("%s\n%s\n\n%s", header, tokenInfo, content)
}

// renderAggregator renders the aggregator-role lane's synthesis view.
func (m *Model) renderAggregator() string {
	s := anim.DefaultStyles()
	reduced := anim.ReducedMotion()
	active := m.aggregatorState.Active
	done := !active && m.aggregatorState.TokenCount > 0
	status := anim.AggregatorGlyph(active, done, m.animFrame, s, reduced)
	aggregatorName := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).Render("Aggregator (Synthesis)")
	level := 0.0
	if active {
		level = 0.55
	}
	bar := anim.MiniBar(m.animFrame, level, active, 10, s, reduced)
	header := fmt.Sprintf("%s %s %s", status, aggregatorName, bar)
	content := m.aggregatorState.Content.String()
	if len(content) > 400 {
		content = "..." + content[len(content)-400:]
	}
	tokenInfo := fmt.Sprintf("Tokens: %d | Seq: %d", m.aggregatorState.TokenCount, m.aggregatorState.Sequence)
	if len(m.aggregatorState.BufferedTokens) > 0 {
		tokenInfo += fmt.Sprintf(" | Buffered: %d", len(m.aggregatorState.BufferedTokens))
	}
	return fmt.Sprintf("%s\n%s\n\n%s", header, tokenInfo, content)
}
