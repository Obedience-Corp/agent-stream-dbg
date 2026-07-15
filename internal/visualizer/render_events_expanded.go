package visualizer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lancekrogers/stream-debugger/internal/events"
)

// renderEventExpandedPlainText returns the human-readable expanded content for an event
// This is the plain-text version of what the TUI shows when a node is expanded
// Used for Ctrl+S export to file
func (m InteractiveModel) renderEventExpandedPlainText(evt *events.Event, msg *Message) string {
	var b strings.Builder

	switch evt.Name {
	case "filter_detail":
		b.WriteString("Thinking (full):\n")
		b.WriteString(evt.StringField("thinking_full"))
		b.WriteString("\n\nFiltered Response:\n")
		b.WriteString(evt.StringField("filtered_response"))
	case "perspective_detail":
		b.WriteString("Perspective (full):\n")
		b.WriteString(evt.StringField("perspective_full"))
		b.WriteString("\n\nSummary:\n")
		b.WriteString(evt.StringField("summary"))
		if insights := evt.StringSliceField("key_insights"); len(insights) > 0 {
			b.WriteString("\n\nKey Insights:\n")
			for _, insight := range insights {
				b.WriteString("• " + insight + "\n")
			}
		}
	case "synthesis_detail":
		b.WriteString("Plan ID: ")
		b.WriteString(evt.StringField("plan_id"))
		b.WriteString("\nMethod: ")
		b.WriteString(evt.StringField("synthesis_method"))
		b.WriteString(fmt.Sprintf("\nSources Combined: %d", evt.IntField("sources_combined")))
		b.WriteString("\n\nSynthesis (full):\n")
		b.WriteString(evt.StringField("synthesis_full"))
	case "agent_metadata":
		b.WriteString("Model: " + evt.StringField("model") + "\n")
		b.WriteString(fmt.Sprintf("Total Tokens: %d\n", evt.IntField("total_tokens")))
		b.WriteString(fmt.Sprintf("Response Length: %d chars\n", evt.IntField("response_length")))
		b.WriteString(fmt.Sprintf("Latency: %dms\n", evt.IntField("latency_ms")))
	case "prompt_info":
		b.WriteString("Prompt File: " + evt.StringField("prompt_file") + "\n")
		b.WriteString(fmt.Sprintf("Prompt Length: %d chars\n", evt.IntField("prompt_length")))
		b.WriteString("\nSnippet:\n")
		b.WriteString(evt.StringField("prompt_snippet"))
	case "prompt_full":
		b.WriteString("Agent: " + evt.StringField("agent_id") + "\n")
		b.WriteString("\nSystem Prompt (full):\n")
		b.WriteString(evt.StringField("system_prompt"))
	case "flow_config":
		b.WriteString("Flow ID: " + evt.StringField("flow_id") + "\n")
		b.WriteString("Flow File: " + evt.StringField("flow_file") + "\n")
		b.WriteString("Stages Order: " + strings.Join(evt.StringSliceField("stages_order"), " → ") + "\n")
		b.WriteString("Routing Mode: " + evt.StringField("routing_mode") + "\n")
		b.WriteString(fmt.Sprintf("Agent Count: %d\n", evt.IntField("agent_count")))
		b.WriteString(fmt.Sprintf("Non-Aggregator Count: %d\n", evt.IntField(nonAggregatorCountFieldKey)))
		if stagesEnabled := evt.BoolMapField("stages_enabled"); len(stagesEnabled) > 0 {
			b.WriteString("Stages Enabled:\n")
			for stage, enabled := range stagesEnabled {
				b.WriteString(fmt.Sprintf("  %s: %v\n", stage, enabled))
			}
		}
	case "flow_step_start":
		b.WriteString("Step: " + evt.StringField("step") + "\n")
		b.WriteString(fmt.Sprintf("Enabled: %v\n", evt.BoolField("enabled")))
		if agentCount := evt.IntField("agent_count"); agentCount > 0 {
			b.WriteString(fmt.Sprintf("Agent Count: %d\n", agentCount))
		}
		if nonAggregatorCount := evt.IntField(nonAggregatorCountFieldKey); nonAggregatorCount > 0 {
			b.WriteString(fmt.Sprintf("Non-Aggregator Count: %d\n", nonAggregatorCount))
		}
	case "flow_step_end":
		b.WriteString("Step: " + evt.StringField("step") + "\n")
		b.WriteString(fmt.Sprintf("Enabled: %v\n", evt.BoolField("enabled")))
		if durationMs := evt.IntField("duration_ms"); durationMs > 0 {
			b.WriteString(fmt.Sprintf("Duration: %dms\n", durationMs))
		}
		if routingMode := evt.StringField("routing_mode"); routingMode != "" {
			b.WriteString("Routing Mode: " + routingMode + "\n")
		}
		if routeTaken := evt.StringField("route_taken"); routeTaken != "" {
			b.WriteString("Route Taken: " + routeTaken + "\n")
		}
		if routeReason := evt.StringField("route_reason"); routeReason != "" {
			b.WriteString("Route Reason: " + routeReason + "\n")
		}
		if routeAgents := evt.StringSliceField("route_agents"); len(routeAgents) > 0 {
			b.WriteString("Route Agents: " + strings.Join(routeAgents, ", ") + "\n")
		}
	case "flow_step_detail":
		b.WriteString("Step: " + evt.StringField("step") + "\n")
		if planID := evt.StringField("plan_id"); planID != "" {
			b.WriteString("Plan ID: " + planID + "\n")
		}
		if preview := evt.StringField("synthesis_preview"); preview != "" {
			b.WriteString("\nSynthesis Preview:\n")
			b.WriteString(preview)
			b.WriteString("\n")
		}
		if full := evt.StringField("synthesis_full"); full != "" {
			b.WriteString("\nSynthesis Full:\n")
			b.WriteString(full)
			b.WriteString("\n")
		}
		if thinking := evt.StringField("thinking_preview"); thinking != "" {
			b.WriteString("\nThinking Preview:\n")
			b.WriteString(thinking)
			b.WriteString("\n")
		}
		if perspectives, ok := evt.Fields["perspectives"].([]any); ok && len(perspectives) > 0 {
			b.WriteString("\nPerspectives:\n")
			for _, raw := range perspectives {
				p, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				agentID, _ := p["agent_id"].(string)
				summary, _ := p["summary"].(string)
				b.WriteString(fmt.Sprintf("• %s: %s\n", agentID, summary))
			}
		}
		if agents := evt.StringSliceField("agents"); len(agents) > 0 {
			b.WriteString("\nAgents: " + strings.Join(agents, ", ") + "\n")
		}
	case aggregatorStreamStartEventName:
		b.WriteString("Message ID: " + evt.StringField("message_id") + "\n")
	case aggregatorStreamCompleteEventName:
		// Show full aggregator response from AgentResponses
		if msg != nil {
			if agg := m.aggregatorResponse(*msg); agg != nil && agg.FullContent != "" {
				b.WriteString("Aggregator Response:\n")
				b.WriteString(agg.FullContent)
				b.WriteString("\n")
				if agg.TokenCount > 0 {
					b.WriteString(fmt.Sprintf("\nTokens: %d\n", agg.TokenCount))
				}
			}
		}
	case "agent_stream_start":
		b.WriteString("Agent: " + evt.SourceID + "\n")
		b.WriteString("Message ID: " + evt.StringField("message_id") + "\n")
	case "agent_stream_complete":
		b.WriteString("Agent: " + evt.SourceID + "\n")
		b.WriteString(fmt.Sprintf("Token Count: %d\n", evt.IntField("token_count")))
		// Show full agent response from AgentResponses
		if msg != nil {
			if agent := msg.AgentResponses[evt.SourceID]; agent != nil && agent.FullContent != "" {
				b.WriteString("\nFull Response:\n")
				b.WriteString(agent.FullContent)
			}
		}
	case "session_start":
		b.WriteString("Session ID: " + evt.StringField("session_id") + "\n")
		b.WriteString("Message ID: " + evt.StringField("message_id") + "\n")
		if flowID := evt.StringField("flow_id"); flowID != "" {
			b.WriteString("Flow ID: " + flowID + "\n")
		}
	case "session_complete":
		b.WriteString("Session ID: " + evt.StringField("session_id") + "\n")
	case "error":
		b.WriteString("Error Type: " + evt.StringField("error_type") + "\n")
		b.WriteString("Message: " + evt.StringField("message") + "\n")
		if details := evt.StringField("details"); details != "" {
			b.WriteString("Details: " + details + "\n")
		}
		if agentID := evt.StringField("agent_id"); agentID != "" {
			b.WriteString("Agent: " + agentID + "\n")
		}
	default:
		// For any unknown event type, show the raw JSON (pretty-printed)
		if len(evt.Raw) > 0 {
			b.WriteString("Raw Event Data:\n")
			var prettyJSON bytes.Buffer
			if err := json.Indent(&prettyJSON, evt.Raw, "", "  "); err == nil {
				b.WriteString(prettyJSON.String())
			} else {
				b.WriteString(string(evt.Raw))
			}
		}
	}
	return b.String()
}
