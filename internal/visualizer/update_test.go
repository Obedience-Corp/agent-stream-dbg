package visualizer

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/transport"
)

// TestUpdate_WindowSizeMsg is a floor-level test for update.go (previously
// untested directly): a WindowSizeMsg must resize the textarea and
// viewport and mark content dirty (which Update then clears via
// refreshViewportContent before returning).
func TestUpdate_WindowSizeMsg(t *testing.T) {
	m := newFixtureModel(t)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	im, ok := updated.(InteractiveModel)
	if !ok {
		t.Fatalf("expected Update to return an InteractiveModel, got %T", updated)
	}
	if im.width != 120 || im.height != 40 {
		t.Errorf("expected width/height 120/40, got %d/%d", im.width, im.height)
	}
	if im.viewport.Width != 116 {
		t.Errorf("expected viewport width 116 (120-4), got %d", im.viewport.Width)
	}
	if im.contentDirty {
		t.Error("expected contentDirty cleared by the shared refresh at the end of Update")
	}
}

// TestUpdate_StreamLifecycle drives Update through the streamStartMsg ->
// streamFrameMsg (EOF) sequence a real streaming turn goes through, without a
// network connection (a nil transport reads as immediate EOF).
func TestUpdate_StreamLifecycle(t *testing.T) {
	cfg := &config.EnhancedConfig{LogDir: t.TempDir()}
	cfg.Session.ID = "stream-lifecycle-test"
	m := NewInteractiveModel(cfg)
	m.messages = append(m.messages, Message{Text: "hi", Streaming: true})

	updated, cmd := m.Update(streamStartMsg{index: 0, stream: nil})
	m2 := updated.(InteractiveModel)
	if m2.streamIndex != 0 {
		t.Errorf("expected streamIndex 0, got %d", m2.streamIndex)
	}
	if cmd == nil {
		t.Fatal("expected streamStartMsg to kick off a read command")
	}

	msg := cmd()
	frameMsg, ok := msg.(streamFrameMsg)
	if !ok {
		t.Fatalf("expected streamFrameMsg from readStreamFrameCmd, got %T", msg)
	}
	if !frameMsg.eof {
		t.Fatal("expected a nil stream transport to read as immediate EOF")
	}

	final, _ := m2.Update(frameMsg)
	m3 := final.(InteractiveModel)
	if m3.streaming {
		t.Error("expected streaming to be false after EOF")
	}
	if m3.messages[0].Streaming {
		t.Error("expected the message's Streaming flag cleared after EOF")
	}
}

// TestUpdate_StreamFrameDoesNotDoubleAppendEvents ensures each decoded frame
// is recorded once: applyParsedEvent owns Message.Events.
func TestUpdate_StreamFrameDoesNotDoubleAppendEvents(t *testing.T) {
	cfg := &config.EnhancedConfig{LogDir: t.TempDir()}
	cfg.Session.ID = "no-double-append"
	m := NewInteractiveModel(cfg)
	m.messages = append(m.messages, Message{Text: "hi", Streaming: true})
	m.streaming = true

	updated, _ := m.Update(streamStartMsg{index: 0, stream: nil})
	m2 := updated.(InteractiveModel)

	payload := []byte(`{"agent_id":"agent_a","content":"hello"}`)
	frameMsg := streamFrameMsg{
		index: 0,
		frame: transport.Frame{
			Name: "agent_content",
			Data: payload,
			Raw:  []byte("event: agent_content\ndata: " + string(payload) + "\n\n"),
		},
	}
	updated, _ = m2.Update(frameMsg)
	m3 := updated.(InteractiveModel)
	if got := len(m3.messages[0].Events); got != 1 {
		t.Fatalf("expected exactly 1 event after one frame, got %d", got)
	}
	if m3.messages[0].Events[0].Kind != events.KindContent {
		t.Fatalf("expected content event, got %v", m3.messages[0].Events[0].Kind)
	}
}

// TestUpdate_StreamEOFKeepsIncrementalEvents proves EOF finalization does not
// replace Events by re-parsing RawSSE as named SSE (which would wipe data-only
// and non-SSE raw buffers).
func TestUpdate_StreamEOFKeepsIncrementalEvents(t *testing.T) {
	cfg := &config.EnhancedConfig{LogDir: t.TempDir()}
	cfg.Session.ID = "eof-keep-events"
	m := NewInteractiveModel(cfg)
	m.messages = append(m.messages, Message{
		Text:      "hi",
		Streaming: true,
		// Non-named / binary-looking raw that named-SSE reparse would not recover.
		RawSSE: string([]byte{0x00, 0x01, 0x02}),
		Events: []*events.Event{
			{Kind: events.KindContent, SourceID: "agent_a", Content: "kept"},
		},
		AgentResponses: map[string]*AgentResponse{
			"agent_a": {AgentID: "agent_a", FullContent: "kept"},
		},
	})
	m.streamIndex = 0
	m.streaming = true

	updated, _ := m.Update(streamFrameMsg{index: 0, eof: true})
	m2 := updated.(InteractiveModel)
	if got := len(m2.messages[0].Events); got != 1 {
		t.Fatalf("expected EOF to preserve incremental events, got %d", got)
	}
	if m2.messages[0].Events[0].Content != "kept" {
		t.Fatalf("expected preserved content, got %q", m2.messages[0].Events[0].Content)
	}
	if m2.streaming || m2.messages[0].Streaming {
		t.Error("expected streaming flags cleared after EOF")
	}
}

// TestUpdate_StreamErrorMsg covers the streamErrorMsg branch: it should
// record the error and stop streaming without touching message content.
func TestUpdate_StreamErrorMsg(t *testing.T) {
	m := newFixtureModel(t)
	m.streaming = true

	wantErr := errBoom
	updated, _ := m.Update(streamErrorMsg{err: wantErr})
	im := updated.(InteractiveModel)
	if im.err != wantErr {
		t.Errorf("expected err %v, got %v", wantErr, im.err)
	}
	if im.streaming {
		t.Error("expected streaming false after streamErrorMsg")
	}
}

var errBoom = &testErr{"boom"}

type testErr struct{ msg string }

func (e *testErr) Error() string { return e.msg }

// TestCurrentPaneName covers every Pane constant plus an out-of-range
// value, matching the documented "?" fallback.
func TestCurrentPaneName(t *testing.T) {
	m := InteractiveModel{}
	cases := map[Pane]string{
		PaneFlow:     "Flow",
		PaneApp:      "App",
		PaneTimeline: "Timeline",
		PaneEvents:   "Events",
		PaneMessages: "Messages",
		Pane(99):     "?",
	}
	for pane, want := range cases {
		m.activePane = pane
		if got := m.currentPaneName(); got != want {
			t.Errorf("pane %v: expected %q, got %q", pane, want, got)
		}
	}
}

// TestParseSSEStream_And_BuildAgentResponses proves the non-incremental
// "finalize" code path (used when a stream completes) parses the same
// fixture-derived raw SSE text into events and rebuilds a non-empty,
// completed aggregator AgentResponse — the counterpart to
// fixture_parity_test.go's coverage of the incremental applyParsedEvent
// path.
func TestParseSSEStream_And_BuildAgentResponses(t *testing.T) {
	m := newFixtureModel(t)
	rawSSE := m.messages[0].RawSSE
	if rawSSE == "" {
		t.Fatal("expected the fixture helper to populate RawSSE")
	}

	evts, err := parseSSEStream(rawSSE)
	if err != nil {
		t.Fatalf("parseSSEStream: %v", err)
	}
	if len(evts) == 0 {
		t.Fatal("expected parseSSEStream to produce events from the fixture SSE text")
	}

	responses := buildAgentResponses(evts)
	agg := responses[aggregatorStageName]
	if agg == nil {
		t.Fatal("expected an aggregator AgentResponse to be built")
	}
	if !agg.Completed {
		t.Error("expected the aggregator response to be Completed")
	}
	if agg.FullContent == "" {
		t.Error("expected the aggregator response to have content")
	}
}

// TestAggregatorResponse covers the direct lookup used throughout the
// render files: it must find the aggregator-role lane and return nil for
// a message with no agent responses at all.
func TestAggregatorResponse(t *testing.T) {
	m := newFixtureModel(t)

	got := m.aggregatorResponse(m.messages[0])
	if got == nil || got.AgentID != aggregatorStageName {
		t.Errorf("expected the aggregator response, got %+v", got)
	}

	if got := m.aggregatorResponse(Message{}); got != nil {
		t.Errorf("expected nil for a message with no agent responses, got %+v", got)
	}
}
