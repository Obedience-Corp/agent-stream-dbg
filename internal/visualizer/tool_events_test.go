package visualizer

import (
	"strings"
	"testing"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/events"
)

// TestRenderToolEventSummary_DistinctFromContent is this task's explicit
// Done-When: KindToolCall/KindToolResult must render as something other
// than plain dumped content, and the two must differ from each other.
func TestRenderToolEventSummary_DistinctFromContent(t *testing.T) {
	call := &events.Event{
		Kind:   events.KindToolCall,
		Fields: map[string]any{"tool_name": "search", "args": `{"q":"weather"}`},
	}
	result := &events.Event{
		Kind:   events.KindToolResult,
		Fields: map[string]any{"tool_name": "search", "result": "72F and sunny"},
	}
	content := &events.Event{Kind: events.KindContent, Content: "search"}

	callOut := renderToolEventSummary(call)
	resultOut := renderToolEventSummary(result)
	contentOut := renderToolEventSummary(content)

	if callOut == resultOut {
		t.Fatalf("expected tool_call and tool_result to render distinctly, both got %q", callOut)
	}
	if !strings.Contains(callOut, "search") || !strings.Contains(callOut, "weather") {
		t.Errorf("expected the tool call summary to name the tool and its args, got %q", callOut)
	}
	if !strings.Contains(resultOut, "search") || !strings.Contains(resultOut, "72F") {
		t.Errorf("expected the tool result summary to name the tool and its result, got %q", resultOut)
	}
	if callOut == contentOut || resultOut == contentOut {
		t.Errorf("expected tool rendering to differ from generic content rendering: call=%q result=%q content=%q", callOut, resultOut, contentOut)
	}
}

// TestApplyParsedEvent_ToolCallAndResultAppendToAgentContent proves the
// incremental path (interactive.go's applyParsedEvent) no longer drops
// KindToolCall/KindToolResult into "default: ignore others" — they must
// surface in the originating lane's AgentResponse content, distinctly
// from a generic KindContent chunk.
func TestApplyParsedEvent_ToolCallAndResultAppendToAgentContent(t *testing.T) {
	m := newTestInteractiveModel(t)

	m.applyParsedEvent(&events.Event{Kind: events.KindStreamStart, SourceID: "agent_a"})
	m.applyParsedEvent(&events.Event{
		Kind:     events.KindToolCall,
		SourceID: "agent_a",
		Fields:   map[string]any{"tool_name": "search", "args": "weather"},
	})
	m.applyParsedEvent(&events.Event{
		Kind:     events.KindToolResult,
		SourceID: "agent_a",
		Fields:   map[string]any{"tool_name": "search", "result": "sunny"},
	})

	agent := m.messages[0].AgentResponses["agent_a"]
	if agent == nil {
		t.Fatal("expected an AgentResponse for agent_a")
	}
	if !strings.Contains(agent.FullContent, "search") {
		t.Errorf("expected agent_a's content to mention the tool call/result, got %q", agent.FullContent)
	}
	if len(agent.ContentChunks) != 2 {
		t.Errorf("expected 2 content chunks (tool_call, tool_result), got %d: %v", len(agent.ContentChunks), agent.ContentChunks)
	}
}

// TestAppendToolEventLine_SeparatesFromSurroundingContent is a
// review-flagged regression test: appendToolEventLine must put its line
// on its own line regardless of what surrounds it, in both directions —
// content immediately before a tool line, and content immediately after
// one — without ever leaving them glued together on one line or
// introducing a spurious blank line.
func TestAppendToolEventLine_SeparatesFromSurroundingContent(t *testing.T) {
	t.Run("content then tool then content, no separator glues them", func(t *testing.T) {
		ar := &AgentResponse{}
		ar.ContentChunks = append(ar.ContentChunks, "Hello ")
		ar.FullContent += "Hello "
		appendToolEventLine(ar, &events.Event{
			Kind:   events.KindToolCall,
			Fields: map[string]any{"tool_name": "search", "args": "weather"},
		})
		ar.ContentChunks = append(ar.ContentChunks, "more text")
		ar.FullContent += "more text"

		if strings.Contains(ar.FullContent, ")more") {
			t.Errorf("expected the tool line and following content to be on separate lines, got %q", ar.FullContent)
		}
		want := "Hello \n🔧 search(weather)\nmore text"
		if ar.FullContent != want {
			t.Errorf("expected %q, got %q", want, ar.FullContent)
		}
	})

	t.Run("content already ending in newline before tool line, no blank line", func(t *testing.T) {
		ar := &AgentResponse{FullContent: "Hello\n"}
		appendToolEventLine(ar, &events.Event{
			Kind:   events.KindToolCall,
			Fields: map[string]any{"tool_name": "search", "args": "weather"},
		})

		if strings.Contains(ar.FullContent, "\n\n") {
			t.Errorf("expected no blank line between prior content and the tool line, got %q", ar.FullContent)
		}
		want := "Hello\n🔧 search(weather)\n"
		if ar.FullContent != want {
			t.Errorf("expected %q, got %q", want, ar.FullContent)
		}
	})
}

// TestBuildAgentResponses_ToolCallAndResult is buildAgentResponses'
// equivalent of the above, for the raw-SSE-rebuild path.
func TestBuildAgentResponses_ToolCallAndResult(t *testing.T) {
	evts := []*events.Event{
		{Kind: events.KindStreamStart, SourceID: "agent_a"},
		{Kind: events.KindToolCall, SourceID: "agent_a", Fields: map[string]any{"tool_name": "search", "args": "weather"}},
		{Kind: events.KindToolResult, SourceID: "agent_a", Fields: map[string]any{"tool_name": "search", "result": "sunny"}},
	}

	responses := buildAgentResponses(evts)

	agent := responses["agent_a"]
	if agent == nil {
		t.Fatal("expected an AgentResponse for agent_a")
	}
	if !strings.Contains(agent.FullContent, "search") {
		t.Errorf("expected agent_a's content to mention the tool call/result, got %q", agent.FullContent)
	}
}

// TestTimelineAddEvent_ToolCallGetsDistinctContent proves timeline.go's
// AddEvent sets entry.Content via renderToolEventSummary for tool
// events, rather than leaving it as the (empty) raw event.Content —
// Kind-based specifically because openai.yaml's discriminator: auto
// means event.Name is always empty for its tool_call events, so a
// Name-based case could never fire for them.
func TestTimelineAddEvent_ToolCallGetsDistinctContent(t *testing.T) {
	tv := NewTimelineVisualizer()
	tv.AddEvent(&events.Event{
		Kind:      events.KindToolCall,
		Name:      "", // openai.yaml's discriminator: auto never sets this
		Timestamp: time.Now(),
		SourceID:  "assistant",
		Fields:    map[string]any{"tool_name": "search", "args": "weather"},
	})

	if len(tv.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(tv.entries))
	}
	if !strings.Contains(tv.entries[0].Content, "search") {
		t.Errorf("expected the tool call's entry.Content to mention the tool name, got %q", tv.entries[0].Content)
	}
}

// TestRenderTimeline_ToolCallShowsDistinctIndicator proves the grid
// rendering (timeline.go:204's switch entry.Event.Kind) shows a distinct
// indicator for tool events, not blank or the generic streaming token
// block.
func TestRenderTimeline_ToolCallShowsDistinctIndicator(t *testing.T) {
	tv := NewTimelineVisualizer()
	now := time.Now()

	tv.AddEvent(&events.Event{Kind: events.KindStreamStart, Timestamp: now, SourceID: "assistant"})
	tv.AddEvent(&events.Event{
		Kind:      events.KindToolCall,
		Timestamp: now.Add(100 * time.Millisecond),
		SourceID:  "assistant",
		Fields:    map[string]any{"tool_name": "search"},
	})

	output := stripANSI(tv.RenderTimeline(100))

	// "TOOL" also appears in the static legend line regardless of
	// whether any tool event was ever rendered, so isolate the actual
	// grid row for the tool_call's time bucket (100ms) rather than
	// searching the whole output — a legend-only match must NOT pass.
	var gridLine string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "100") {
			gridLine = line
			break
		}
	}
	if gridLine == "" {
		t.Fatalf("could not find the 100ms bucket's grid row in output:\n%s", output)
	}
	if !strings.Contains(gridLine, "TOOL") {
		t.Errorf("expected the 100ms bucket's grid row to show a distinct TOOL indicator, got %q", gridLine)
	}
}
