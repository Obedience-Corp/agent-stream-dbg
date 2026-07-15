package visualizer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lancekrogers/stream-debugger/internal/events"
)

// renderEventsPane renders the Events pane (F4) showing SSE events
func (m InteractiveModel) renderEventsPane() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	wizardStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true)

	viewLabel := "PARSED"
	if m.viewMode == ViewModeRaw {
		viewLabel = "RAW"
	}
	tokensLabel := "OFF"
	if m.showTokens {
		tokensLabel = "ON"
	}
	wizardOnlyLabel := "OFF"
	if m.eventsWizardOnly {
		wizardOnlyLabel = "ON"
	}

	b.WriteString(headerStyle.Render("Events"))
	b.WriteString(fmt.Sprintf(" (View: %s)", viewLabel))
	if m.viewMode == ViewModeParsed {
		b.WriteString(fmt.Sprintf(" (Tokens: %s)", tokensLabel))
		b.WriteString(fmt.Sprintf(" (Wizard: %s)", wizardOnlyLabel))
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("─────────────────────────────────────────"))
	b.WriteString("\n")
	// Hints
	if m.viewMode == ViewModeParsed {
		b.WriteString(dimStyle.Render("Ctrl+T: RAW view, t: tokens, W: wizard-only"))
	} else {
		b.WriteString(dimStyle.Render("Ctrl+T: PARSED view"))
	}
	b.WriteString("\n")

	// Show wizard metrics summary when in parsed mode
	if m.viewMode == ViewModeParsed && len(m.messages) > 0 {
		msg := m.messages[len(m.messages)-1]
		if msg.AgentResponses != nil {
			if wizardResp := m.aggregatorResponse(msg); wizardResp != nil {
				var metricsLine strings.Builder
				metricsLine.WriteString(fmt.Sprintf("🧙 Wizard: %d tokens", wizardResp.TokenCount))
				if wizardResp.DurationMs > 0 && wizardResp.TokenCount > 0 {
					tokensPerSec := float64(wizardResp.TokenCount) * 1000.0 / float64(wizardResp.DurationMs)
					metricsLine.WriteString(fmt.Sprintf(" | %.1f tok/s", tokensPerSec))
				}
				if wizardResp.FirstTokenMs > 0 {
					metricsLine.WriteString(fmt.Sprintf(" | first: %dms", wizardResp.FirstTokenMs))
				}
				if !wizardResp.Completed {
					metricsLine.WriteString(" ⏳")
				} else {
					metricsLine.WriteString(" ✓")
				}
				b.WriteString(wizardStyle.Render(metricsLine.String()))
				b.WriteString("\n")
			}
		}
	}
	b.WriteString("\n")

	if len(m.messages) == 0 {
		b.WriteString(dimStyle.Render("No messages yet."))
		return b.String()
	}

	msg := m.messages[len(m.messages)-1]

	// RAW view: pretty-print the raw SSE of the last message
	if m.viewMode == ViewModeRaw {
		if msg.RawSSE == "" {
			b.WriteString(dimStyle.Render("No raw SSE captured for latest message."))
			return b.String()
		}
		b.WriteString(m.renderRawView(msg))
		return b.String()
	}

	// PARSED view
	if len(msg.Events) == 0 {
		b.WriteString(dimStyle.Render("No events in latest message."))
		return b.String()
	}

	// Styles for event rendering
	eventStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	tokenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	wizardEventStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	selectedStyle := lipgloss.NewStyle().Background(lipgloss.Color("8")).Foreground(lipgloss.Color("15"))
	expandedContentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7")).PaddingLeft(4)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)

	// Track visible event index (for selection/expansion which counts visible events only)
	visibleIdx := 0

	for _, evt := range msg.Events {
		// Skip events that don't pass the filter
		if !m.isEventVisible(evt) {
			continue
		}

		// Check if this is a wizard-related event (for styling)
		isWizardEvent := evt.Name == "wizard_stream_start" ||
			evt.Name == "wizard_content" ||
			evt.Name == "wizard_stream_complete"
		evtStep := evt.StringField("step")
		isSynthesisFlowEvent := (evt.Name == "flow_step_start" || evt.Name == "flow_step_end") &&
			(evtStep == "synthesis" || evtStep == "wizard")
		isTokenEvent := evt.Name == "agent_content" || evt.Name == "wizard_content"

		// All events are expandable - show raw JSON when expanded
		isExpandable := true

		isExpanded := m.eventExpanded[visibleIdx]
		isSelected := visibleIdx == m.selectedEventIdx

		// Selection cursor and expand/collapse icon
		var prefix string
		if isSelected {
			prefix = "→ "
		} else {
			prefix = "  "
		}
		if isExpandable {
			if isExpanded {
				prefix += "▼ "
			} else {
				prefix += "▶ "
			}
		} else {
			prefix += "  "
		}

		// Format event
		line := fmt.Sprintf("%s[%s]", prefix, evt.Name)

		// Handoff is an edge between lanes, not a row of content —
		// rendered entirely by renderHandoffEdge, never falling through
		// to the generic per-event-name summary below. Kind-based (not
		// evt.Name-based) since this is brand new: there's no existing
		// wire-event-name behavior to preserve here, unlike the switch
		// below, which stays name-based for now (that's a pre-existing
		// pattern this task doesn't touch).
		if evt.Kind == events.KindHandoff {
			edge := prefix + renderHandoffEdge(evt)
			if isSelected {
				// selectedStyle re-colors the whole line; the edge's own
				// from/to/arrow styling would otherwise conflict with it.
				edge = selectedStyle.Render(prefix + fmt.Sprintf("[%s] %s -> %s (%s)",
					evt.Name, evt.StringField("from"), evt.StringField("to"), evt.StringField("mode")))
			}
			b.WriteString(edge)
			b.WriteString("\n")
			if isExpandable && isExpanded {
				b.WriteString(renderHandoffEdgeExpanded(evt, labelStyle, expandedContentStyle))
				b.WriteString("\n")
			}
			visibleIdx++
			continue
		}

		// Add relevant details (compact summary on main line)
		switch evt.Name {
		case "flow_step_start":
			line += fmt.Sprintf(" step=%s enabled=%v", evt.StringField("step"), evt.BoolField("enabled"))
		case "flow_step_end":
			line += fmt.Sprintf(" step=%s duration=%dms", evt.StringField("step"), evt.IntField("duration_ms"))
		case "agent_stream_start":
			line += fmt.Sprintf(" agent=%s", evt.SourceID)
		case "agent_stream_complete":
			line += fmt.Sprintf(" agent=%s tokens=%d", evt.SourceID, evt.IntField("token_count"))
		case "agent_content":
			content := evt.Content
			if len(content) > 30 {
				content = content[:30] + "..."
			}
			line += fmt.Sprintf(" agent=%s content=%q", evt.SourceID, content)
		case "wizard_content":
			content := evt.Content
			if len(content) > 30 {
				content = content[:30] + "..."
			}
			line += fmt.Sprintf(" content=%q", content)
		// New debug event types - show compact summary
		case "filter_detail":
			line += fmt.Sprintf(" agent=%s thinking=%d filtered=%d", evt.StringField("agent_id"), evt.IntField("original_length"), evt.IntField("filtered_length"))
		case "perspective_detail":
			line += fmt.Sprintf(" agent=%s relevance=%.2f", evt.StringField("agent_id"), evt.Float64Field("relevance_score"))
		case "synthesis_detail":
			line += fmt.Sprintf(" plan=%s method=%s sources=%d", evt.StringField("plan_id"), evt.StringField("synthesis_method"), evt.IntField("sources_combined"))
		case "agent_metadata":
			line += fmt.Sprintf(" agent=%s model=%s tokens=%d latency=%dms", evt.StringField("agent_id"), evt.StringField("model"), evt.IntField("total_tokens"), evt.IntField("latency_ms"))
		case "prompt_info":
			line += fmt.Sprintf(" agent=%s file=%s len=%d", evt.StringField("agent_id"), evt.StringField("prompt_file"), evt.IntField("prompt_length"))
		case "prompt_full":
			line += fmt.Sprintf(" agent=%s len=%d", evt.StringField("agent_id"), len(evt.StringField("system_prompt")))
		case "flow_config":
			line += fmt.Sprintf(" flow=%s agents=%d stages=%d", evt.StringField("flow_id"), evt.IntField("agent_count"), len(evt.StringSliceField("stages_order")))
		case "flow_step_detail":
			line += fmt.Sprintf(" step=%s", evt.StringField("step"))
		}

		// Apply appropriate styling
		var styledLine string
		if isSelected {
			styledLine = selectedStyle.Render(line)
		} else if isTokenEvent && !isWizardEvent {
			styledLine = tokenStyle.Render(line)
		} else if isWizardEvent || isSynthesisFlowEvent {
			styledLine = wizardEventStyle.Render(line)
		} else {
			styledLine = eventStyle.Render(line)
		}
		b.WriteString(styledLine)
		b.WriteString("\n")

		// Render expanded content if expanded
		if isExpandable && isExpanded {
			var expandedContent strings.Builder
			switch evt.Name {
			case "filter_detail":
				expandedContent.WriteString(labelStyle.Render("Thinking (full):"))
				expandedContent.WriteString("\n")
				expandedContent.WriteString(expandedContentStyle.Render(evt.StringField("thinking_full")))
				expandedContent.WriteString("\n")
				expandedContent.WriteString(labelStyle.Render("Filtered Response:"))
				expandedContent.WriteString("\n")
				expandedContent.WriteString(expandedContentStyle.Render(evt.StringField("filtered_response")))
			case "perspective_detail":
				expandedContent.WriteString(labelStyle.Render("Perspective (full):"))
				expandedContent.WriteString("\n")
				expandedContent.WriteString(expandedContentStyle.Render(evt.StringField("perspective_full")))
				expandedContent.WriteString("\n")
				expandedContent.WriteString(labelStyle.Render("Summary:"))
				expandedContent.WriteString("\n")
				expandedContent.WriteString(expandedContentStyle.Render(evt.StringField("summary")))
				if insights := evt.StringSliceField("key_insights"); len(insights) > 0 {
					expandedContent.WriteString("\n")
					expandedContent.WriteString(labelStyle.Render("Key Insights:"))
					expandedContent.WriteString("\n")
					for _, insight := range insights {
						expandedContent.WriteString(expandedContentStyle.Render("• " + insight))
						expandedContent.WriteString("\n")
					}
				}
			case "synthesis_detail":
				expandedContent.WriteString(labelStyle.Render("Synthesis (full):"))
				expandedContent.WriteString("\n")
				expandedContent.WriteString(expandedContentStyle.Render(evt.StringField("synthesis_full")))
			case "agent_metadata":
				expandedContent.WriteString(labelStyle.Render("Model:"))
				expandedContent.WriteString(" " + evt.StringField("model") + "\n")
				expandedContent.WriteString(labelStyle.Render("Total Tokens:"))
				expandedContent.WriteString(fmt.Sprintf(" %d\n", evt.IntField("total_tokens")))
				expandedContent.WriteString(labelStyle.Render("Response Length:"))
				expandedContent.WriteString(fmt.Sprintf(" %d chars\n", evt.IntField("response_length")))
				expandedContent.WriteString(labelStyle.Render("Latency:"))
				expandedContent.WriteString(fmt.Sprintf(" %dms\n", evt.IntField("latency_ms")))
			case "prompt_info":
				expandedContent.WriteString(labelStyle.Render("Prompt File:"))
				expandedContent.WriteString(" " + evt.StringField("prompt_file") + "\n")
				expandedContent.WriteString(labelStyle.Render("Snippet:"))
				expandedContent.WriteString("\n")
				expandedContent.WriteString(expandedContentStyle.Render(evt.StringField("prompt_snippet")))
			case "prompt_full":
				expandedContent.WriteString(labelStyle.Render("System Prompt (full):"))
				expandedContent.WriteString("\n")
				expandedContent.WriteString(expandedContentStyle.Render(evt.StringField("system_prompt")))
			case "flow_config":
				expandedContent.WriteString(labelStyle.Render("Flow File:"))
				expandedContent.WriteString(" " + evt.StringField("flow_file") + "\n")
				expandedContent.WriteString(labelStyle.Render("Stages Order:"))
				expandedContent.WriteString(" " + strings.Join(evt.StringSliceField("stages_order"), " → ") + "\n")
				expandedContent.WriteString(labelStyle.Render("Routing Mode:"))
				expandedContent.WriteString(" " + evt.StringField("routing_mode") + "\n")
				expandedContent.WriteString(labelStyle.Render("Agent Count:"))
				expandedContent.WriteString(fmt.Sprintf(" %d\n", evt.IntField("agent_count")))
			case "flow_step_detail":
				if preview := evt.StringField("synthesis_preview"); preview != "" {
					expandedContent.WriteString(labelStyle.Render("Synthesis Preview:"))
					expandedContent.WriteString("\n")
					expandedContent.WriteString(expandedContentStyle.Render(preview))
					expandedContent.WriteString("\n")
				}
				if full := evt.StringField("synthesis_full"); full != "" {
					expandedContent.WriteString(labelStyle.Render("Synthesis Full:"))
					expandedContent.WriteString("\n")
					expandedContent.WriteString(expandedContentStyle.Render(full))
					expandedContent.WriteString("\n")
				}
				if thinking := evt.StringField("thinking_preview"); thinking != "" {
					expandedContent.WriteString(labelStyle.Render("Thinking Preview:"))
					expandedContent.WriteString("\n")
					expandedContent.WriteString(expandedContentStyle.Render(thinking))
					expandedContent.WriteString("\n")
				}
				if perspectives, ok := evt.Fields["perspectives"].([]any); ok && len(perspectives) > 0 {
					expandedContent.WriteString(labelStyle.Render("Perspectives:"))
					expandedContent.WriteString("\n")
					for _, raw := range perspectives {
						p, ok := raw.(map[string]any)
						if !ok {
							continue
						}
						agentID, _ := p["agent_id"].(string)
						summary, _ := p["summary"].(string)
						expandedContent.WriteString(expandedContentStyle.Render(fmt.Sprintf("• %s: %s", agentID, summary)))
						expandedContent.WriteString("\n")
					}
				}
			case "wizard_stream_complete":
				// Show full aggregator response from AgentResponses
				if wizard := m.aggregatorResponse(msg); wizard != nil && wizard.FullContent != "" {
					expandedContent.WriteString(labelStyle.Render("Wizard Response:"))
					expandedContent.WriteString("\n")
					expandedContent.WriteString(expandedContentStyle.Render(wizard.FullContent))
					expandedContent.WriteString("\n")
					if wizard.TokenCount > 0 {
						expandedContent.WriteString(labelStyle.Render("Tokens:"))
						expandedContent.WriteString(fmt.Sprintf(" %d\n", wizard.TokenCount))
					}
				}
			default:
				// For any event type, show the raw JSON
				if len(evt.Raw) > 0 {
					expandedContent.WriteString(labelStyle.Render("Raw Event Data:"))
					expandedContent.WriteString("\n")
					// Pretty-print the JSON
					var prettyJSON bytes.Buffer
					if err := json.Indent(&prettyJSON, evt.Raw, "", "  "); err == nil {
						expandedContent.WriteString(expandedContentStyle.Render(prettyJSON.String()))
					} else {
						expandedContent.WriteString(expandedContentStyle.Render(string(evt.Raw)))
					}
				}
			}
			if expandedContent.Len() > 0 {
				b.WriteString(expandedContent.String())
				b.WriteString("\n")
			}
		}

		// Increment visible index for next iteration
		visibleIdx++
	}

	return b.String()
}
