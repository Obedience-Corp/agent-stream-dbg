package visualizer

import (
	"strings"
	"testing"

	"github.com/Obedience-Corp/stream-debugger/internal/config"
	"github.com/Obedience-Corp/stream-debugger/internal/events"
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

func TestRenderRawView_GRPCShowsDecodedAndWireData(t *testing.T) {
	m := InteractiveModel{}
	msg := Message{
		TransportName: "grpc",
		Frames: []CapturedFrame{{
			Name: "AGENT_MESSAGE_DELTA",
			Data: []byte(`{"content":"hello"}`),
			Raw:  []byte{0x0a, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f},
		}},
	}
	out := m.renderRawView(msg)
	for _, want := range []string{"Raw gRPC Frames", "decoded:", "hello", "wire (hex): 0a0568656c6c6f"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected gRPC raw view to contain %q, got %q", want, out)
		}
	}
	if strings.Contains(out, "Raw SSE") {
		t.Error("gRPC raw view must not be labeled as SSE")
	}
}

func TestRenderEventsPane_GenericNormalizedSummary(t *testing.T) {
	m := NewInteractiveModel(&config.EnhancedConfig{})
	m.showTokens = true
	m.messages = []Message{{Events: []*events.Event{{
		Name:     "AGENT_MESSAGE_DELTA",
		Kind:     events.KindContent,
		SourceID: "worker-a",
		Content:  "hello",
		Fields:   map[string]any{"turn_id": "turn-1", "durable_sequence": float64(12)},
	}}}}
	got := m.renderEventsPane()
	for _, want := range []string{"AGENT_MESSAGE_DELTA", "source=worker-a", "content=\"hello\"", "turn_id=\"turn-1\""} {
		if !strings.Contains(got, want) {
			t.Errorf("expected generic event summary to contain %q, got %q", want, got)
		}
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
