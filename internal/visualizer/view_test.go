package visualizer

import (
	"strings"
	"testing"
)

// TestView_ContainsHeaderAndTabs is a floor-level test for view.go
// (previously untested directly): View must render without panicking and
// include the pane tabs and session id.
func TestView_ContainsHeaderAndTabs(t *testing.T) {
	m := newFixtureModel(t)
	m.width = 100
	m.height = 40
	m.viewport.Width = 96
	m.viewport.Height = 20
	m.refreshViewportContent()

	out := m.View()
	for _, want := range []string{"Flow", "App", "Timeline", "Events", "Messages", "fixture-test-session"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected View() output to contain %q", want)
		}
	}
}

// TestRenderParsedView covers renderParsedView's two branches: the
// "no responses" placeholder, and a real agent response rendered with its
// aggregator debug summary and token count.
func TestRenderParsedView(t *testing.T) {
	m := newFixtureModel(t)

	empty := m.renderParsedView(Message{})
	if !strings.Contains(empty, "No agent responses parsed") {
		t.Errorf("expected the empty-state message, got %q", empty)
	}

	out := m.renderParsedView(m.messages[0])
	if !strings.Contains(out, aggregatorStageName) {
		t.Error("expected the aggregator lane to appear in the parsed view")
	}
	if !strings.Contains(out, "tokens") {
		t.Error("expected token counts to appear in the parsed view")
	}
}

// TestRenderRawView covers the streaming-passthrough branch and the
// pretty-printed branch.
func TestRenderRawView(t *testing.T) {
	m := newFixtureModel(t)

	streaming := Message{RawSSE: "event: x\ndata: {}\n\n", Streaming: true}
	if got := m.renderRawView(streaming); got != streaming.RawSSE+"\n" {
		t.Errorf("expected streaming raw view to pass through as-is, got %q", got)
	}

	out := m.renderRawView(m.messages[0])
	if !strings.Contains(out, "Raw SSE Stream") {
		t.Error("expected the pretty-printed header for a completed message")
	}
}

// TestRenderMessages covers the history toggle and view-mode dispatch.
func TestRenderMessages(t *testing.T) {
	m := newFixtureModel(t)
	m.messages = append(m.messages, Message{Text: "second turn"})

	m.showHistory = true
	all := m.renderMessages()
	if !strings.Contains(all, "second turn") {
		t.Error("expected showHistory=true to include earlier turns")
	}

	m.showHistory = false
	latestOnly := m.renderMessages()
	if strings.Contains(latestOnly, "What is consciousness?") {
		t.Error("expected showHistory=false to omit earlier turns")
	}
}
