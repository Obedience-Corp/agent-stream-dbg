package visualizer

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m InteractiveModel) View() string {
	var b strings.Builder

	// Header with pane indicator
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("6")).
		Padding(0, 1)

	paneStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("14")).
		Background(lipgloss.Color("8")).
		Padding(0, 1)

	// Pane tabs
	paneNames := []string{"Flow", "App", "Timeline", "Events", "Messages"}
	var tabs strings.Builder
	for i, name := range paneNames {
		if Pane(i) == m.activePane {
			tabs.WriteString(paneStyle.Render(fmt.Sprintf("[%d]%s", i+1, name)))
		} else {
			tabs.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fmt.Sprintf(" %d:%s ", i+1, name)))
		}
	}

	dbg := m.cfg.Debug.Level
	if dbg == "" {
		dbg = "off"
	}
	b.WriteString(headerStyle.Render(fmt.Sprintf("🚀 Stream Debugger (debug=%s)", dbg)))
	b.WriteString(" ")
	b.WriteString(tabs.String())
	b.WriteString("  ")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fmt.Sprintf("(session: %s)", m.cfg.Session.ID)))
	b.WriteString("\n\n")

	// Display viewport (scrollable message history)
	// Content is set in Update() via refreshViewportContent()
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Error display
	if m.err != nil {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true)
		b.WriteString(errorStyle.Render(fmt.Sprintf("❌ Error: %v", m.err)))
		b.WriteString("\n\n")
	}

	// Input box
	inputBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1)

	inputLabel := "Your message:"
	if m.streaming {
		inputLabel = "⏳ Streaming response..."
	}

	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(inputLabel))
	b.WriteString("\n")
	b.WriteString(inputBoxStyle.Render(m.textarea.View()))
	b.WriteString("\n")

	// Status line with pane-specific hints
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Italic(true)

	mode := "INSERT"
	if !m.insertMode {
		mode = "NORMAL"
	}

	// Pane-specific status hints
	var paneHints string
	switch m.activePane {
	case PaneFlow:
		toggle := "off"
		if m.flowContinuous {
			toggle = "on"
		}
		paneHints = fmt.Sprintf("j/k:select Enter:expand(wizard details) p:prompt [/]:turn a:all(%s)", toggle)
	case PaneEvents:
		tokensStr := "OFF"
		if m.showTokens {
			tokensStr = "ON"
		}
		wizardStr := "OFF"
		if m.eventsWizardOnly {
			wizardStr = "ON"
		}
		paneHints = fmt.Sprintf("j/k:navigate Enter:expand t:tokens(%s) W:wizard-only(%s)", tokensStr, wizardStr)
	case PaneMessages:
		paneHints = "Ctrl+T:raw/parsed"
	default:
		paneHints = ""
	}

	wrapStr := "OFF"
	if m.wrap {
		wrapStr = "ON"
	}
	status := fmt.Sprintf(
		"Mode:%s Pane:%s Msgs:%d | 1-6:panes %s w:wrap(%s) F7:verbose F8:full F9:off Ctrl+S:save",
		mode, m.currentPaneName(), len(m.messages), paneHints, wrapStr,
	)
	b.WriteString(statusStyle.Render(status))

	// Show save status if present
	if m.saveStatus != "" {
		b.WriteString("\n")
		saveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
		b.WriteString(saveStyle.Render(m.saveStatus))
	}

	return b.String()
}

// renderRawView renders the raw SSE stream
func (m InteractiveModel) renderRawView(msg Message) string {
	if msg.RawSSE == "" {
		return ""
	}

	var b strings.Builder

	// While streaming, render the raw SSE as-is for responsiveness;
	// pretty printing on every chunk is expensive.
	if msg.Streaming {
		b.WriteString(msg.RawSSE)
		b.WriteString("\n")
		return b.String()
	}

	responseStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("13")).
		Bold(true)

	b.WriteString(responseStyle.Render("Raw SSE Stream (JSON Pretty-Printed):"))
	b.WriteString("\n\n")

	// Parse and pretty-print JSON in data fields
	lines := strings.Split(msg.RawSSE, "\n")

	eventStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14")).
		Bold(true)

	jsonKeyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("6"))

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")

		if strings.HasPrefix(line, "event:") {
			// Style event lines
			b.WriteString(eventStyle.Render(line))
			b.WriteString("\n")

		} else if strings.HasPrefix(line, "data:") {
			// Extract JSON from data line
			jsonStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

			if jsonStr == "" {
				b.WriteString("data:\n")
				continue
			}

			// Try to parse and pretty-print JSON
			var jsonData interface{}
			if err := json.Unmarshal([]byte(jsonStr), &jsonData); err == nil {
				prettyJSON, err := json.MarshalIndent(jsonData, "  ", "  ")
				if err == nil {
					b.WriteString(jsonKeyStyle.Render("data:"))
					b.WriteString("\n  ")
					b.WriteString(string(prettyJSON))
					b.WriteString("\n")
				} else {
					// Fallback to original if indent fails
					b.WriteString(line)
					b.WriteString("\n")
				}
			} else {
				// Not JSON, render as-is
				b.WriteString(line)
				b.WriteString("\n")
			}

		} else if line == "" {
			// Preserve empty lines (event separators)
			b.WriteString("\n")
		} else {
			// Other lines (shouldn't happen in valid SSE, but handle anyway)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	return b.String()
}

// renderParsedView renders agent responses color-coded by agent
func (m InteractiveModel) renderParsedView(msg Message) string {
	if len(msg.AgentResponses) == 0 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Italic(true).
			Render("No agent responses parsed")
	}

	var b strings.Builder

	// Aggregator (brainyard: wizard) debug summary at top
	if aggResp := m.aggregatorResponse(msg); aggResp != nil {
		b.WriteString(m.renderWizardDebugSummary(aggResp))
		b.WriteString("\n")
	}

	// Render each agent's response (deterministic order)
	// Collect and sort agent IDs for stable rendering to avoid flicker
	keys := make([]string, 0, len(msg.AgentResponses))
	for k := range msg.AgentResponses {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, agentID := range keys {
		agentResp := msg.AgentResponses[agentID]
		if agentResp.FullContent == "" {
			continue
		}

		// Agent header with color
		agentColor := m.getAgentColor(agentID)
		agentHeaderStyle := lipgloss.NewStyle().
			Foreground(agentColor).
			Bold(true)

		// Display agent header
		header := fmt.Sprintf("%s (%d tokens)", agentID, agentResp.TokenCount)
		if agentResp.Completed {
			header += " ✓"
		} else {
			header += " ⏳"
		}

		b.WriteString(agentHeaderStyle.Render(header))
		b.WriteString("\n")

		// Agent content with same color
		contentStyle := lipgloss.NewStyle().
			Foreground(agentColor)

		b.WriteString(contentStyle.Render(agentResp.FullContent))
		b.WriteString("\n\n")
	}

	return b.String()
}

func (m InteractiveModel) renderMessages() string {
	var b strings.Builder

	// Show last or all messages depending on history toggle
	start := 0
	if !m.showHistory && len(m.messages) > 0 {
		start = len(m.messages) - 1
	}
	for i := start; i < len(m.messages); i++ {
		msg := m.messages[i]

		// User message
		userStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true)

		b.WriteString(userStyle.Render(fmt.Sprintf("You [%s]:", msg.Timestamp.Format("15:04:05"))))
		b.WriteString("\n")
		b.WriteString(msg.Text)
		b.WriteString("\n\n")

		// Response
		if msg.Streaming {
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("8")).
				Italic(true).
				Render("⏳ Streaming response from agents..."))
			b.WriteString("\n\n")
		} else if msg.RawSSE != "" {
			// Render based on view mode
			if m.viewMode == ViewModeRaw {
				b.WriteString(m.renderRawView(msg))
			} else {
				b.WriteString(m.renderParsedView(msg))
			}
		}
	}

	return b.String()
}
