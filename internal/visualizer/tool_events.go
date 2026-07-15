package visualizer

import (
	"fmt"
	"strings"

	"github.com/lancekrogers/stream-debugger/internal/events"
)

// renderToolEventSummary renders a events.KindToolCall/KindToolResult
// event as a short, visually distinct line — never dumped as plain
// content, since neither Kind's Fields shape maps onto Event.Content the
// way KindContent's does (dialects/openai.yaml's tool_call rule, the
// only shipped rule producing either Kind today, sets Fields only:
// tool_name/args, no content:).
//
// tool_result's field names (tool_name/result) are a best-effort
// convention, not verified against a shipped dialect: no dialect in
// this repo emits KindToolResult yet (openai.yaml's tool_call rule is
// the only producer of either Kind) — this mirrors tool_call's own
// field naming for symmetry until a real one exists to check against.
func renderToolEventSummary(evt *events.Event) string {
	switch evt.Kind {
	case events.KindToolCall:
		name := evt.StringField("tool_name")
		args := evt.StringField("args")
		if args == "" {
			return fmt.Sprintf("🔧 %s()", name)
		}
		return fmt.Sprintf("🔧 %s(%s)", name, args)
	case events.KindToolResult:
		name := evt.StringField("tool_name")
		result := evt.StringField("result")
		if name == "" {
			return fmt.Sprintf("✓ %s", result)
		}
		return fmt.Sprintf("✓ %s → %s", name, result)
	default:
		return evt.Content
	}
}

// appendToolEventLine appends evt's rendered tool-event summary to ar as
// its own line, regardless of what surrounds it: any trailing newlines
// already in ar.FullContent are trimmed first (so content ending in "\n"
// doesn't produce a blank line before the summary), and the summary
// itself always ends in exactly one "\n" (so content resuming
// immediately afterward, which never inserts its own leading newline,
// doesn't get glued onto the same line). ar.ContentChunks keeps the bare
// summary line, undecorated, as the semantic chunk.
func appendToolEventLine(ar *AgentResponse, evt *events.Event) {
	line := renderToolEventSummary(evt)
	ar.ContentChunks = append(ar.ContentChunks, line)
	ar.FullContent = strings.TrimRight(ar.FullContent, "\n")
	if ar.FullContent != "" {
		ar.FullContent += "\n"
	}
	ar.FullContent += line + "\n"
}
