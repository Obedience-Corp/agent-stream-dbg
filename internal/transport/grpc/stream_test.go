package grpc

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/testutil/mockgrpc"
	"github.com/lancekrogers/stream-debugger/internal/testutil/mockgrpc/agentstreampb"
)

func TestTransport_Frames_DecodesScriptedEventsAsProtoJSON(t *testing.T) {
	srv, err := mockgrpc.New([]*agentstreampb.StreamEvent{
		{Payload: &agentstreampb.StreamEvent_SessionStart{SessionStart: &agentstreampb.SessionStart{SessionId: "s1"}}},
		{Payload: &agentstreampb.StreamEvent_AgentContent{AgentContent: &agentstreampb.AgentContent{AgentId: "agent_a", Content: "hello", Sequence: 1}}},
		{Payload: &agentstreampb.StreamEvent_ToolCall{ToolCall: &agentstreampb.ToolCall{AgentId: "agent_a", ToolName: "get_weather", Arguments: `{"city":"Denver"}`}}},
	})
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	tr := New(Config{
		Target:        srv.Addr(),
		Plaintext:     true,
		Method:        "/agentstream.v1.AgentStream/Stream",
		Request:       map[string]any{"session_id": "s1", "message": "hi"},
		Discriminator: "message_type",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = tr.Close() }()

	var frames []struct {
		name string
		data map[string]any
	}
	for f := range tr.Frames() {
		if f.Err != nil {
			t.Fatalf("unexpected frame error: %v", f.Err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(f.Data, &decoded); err != nil {
			t.Fatalf("Frame.Data did not decode as JSON: %v (data=%s)", err, f.Data)
		}
		if !bytes.Equal(f.Data, f.Raw) {
			t.Errorf("expected Raw == Data for gRPC frames, got Data=%q Raw=%q", f.Data, f.Raw)
		}
		if f.Timestamp.IsZero() {
			t.Error("expected non-zero Timestamp")
		}
		frames = append(frames, struct {
			name string
			data map[string]any
		}{f.Name, decoded})
	}

	if len(frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(frames))
	}

	// discriminator: message_type is explicit here, so Frame.Name is the
	// response message's static type — StreamEvent, the oneof wrapper —
	// for every frame; see TestDiscriminator_Oneof_* for the case-name
	// extraction this test deliberately isn't exercising.
	for i, fr := range frames {
		if fr.name != "agentstream.v1.StreamEvent" {
			t.Errorf("frame %d: expected name 'agentstream.v1.StreamEvent', got %q", i, fr.name)
		}
	}

	// Field names must stay snake_case (proto original names), matching
	// every SSE dialect's field paths (agent_id, not agentId) — the one
	// thing that would silently break cross-transport dialect parity.
	// The oneof case itself becomes a nested snake_case key too
	// (session_start, agent_content, tool_call) since Frame.Data is the
	// whole StreamEvent, unwrapped by nothing at this stage.
	sessionStart, _ := frames[0].data["session_start"].(map[string]any)
	if sessionStart["session_id"] != "s1" {
		t.Errorf("expected session_start.session_id (snake_case) = 's1', got %+v", frames[0].data)
	}

	agentContent, _ := frames[1].data["agent_content"].(map[string]any)
	if agentContent["agent_id"] != "agent_a" || agentContent["content"] != "hello" {
		t.Errorf("expected agent_content.agent_id/content (snake_case), got %+v", frames[1].data)
	}
	// sequence is int32 field number 3; protojson renders numbers as
	// json.Number-compatible float64 when decoded generically.
	if seq, ok := agentContent["sequence"].(float64); !ok || seq != 1 {
		t.Errorf("expected sequence 1, got %+v", agentContent["sequence"])
	}

	toolCall, _ := frames[2].data["tool_call"].(map[string]any)
	if toolCall["tool_name"] != "get_weather" {
		t.Errorf("expected tool_name (snake_case) = 'get_weather', got %+v", frames[2].data)
	}
}

// TestTransport_Frames_PreserveFieldNamesFalse_ProducesCamelCase proves
// Config.PreserveFieldNames's other setting (false) actually renders
// lowerCamelCase JSON — the option a dialect whose real wire protocol
// mandates camelCase JSON field names needs, verified generically here
// against the same test proto every other gRPC test in this package
// already uses, not tied to any one dialect. Deliberately the mirror
// image of TestTransport_Frames_DecodesScriptedEventsAsProtoJSON's
// snake_case assertions above: agent_id/tool_name/session_id become
// agentId/toolName/sessionId, and the original snake_case keys must be
// entirely absent — not just "also present alongside" a camelCase key.
func TestTransport_Frames_PreserveFieldNamesFalse_ProducesCamelCase(t *testing.T) {
	srv, err := mockgrpc.New([]*agentstreampb.StreamEvent{
		{Payload: &agentstreampb.StreamEvent_AgentContent{AgentContent: &agentstreampb.AgentContent{AgentId: "agent_a", Content: "hello", Sequence: 1}}},
		{Payload: &agentstreampb.StreamEvent_ToolCall{ToolCall: &agentstreampb.ToolCall{AgentId: "agent_a", ToolName: "get_weather", Arguments: `{"city":"Denver"}`}}},
	})
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	preserveFieldNames := false
	tr := New(Config{
		Target:             srv.Addr(),
		Plaintext:          true,
		Method:             "/agentstream.v1.AgentStream/Stream",
		Request:            map[string]any{"session_id": "s1"},
		Discriminator:      "message_type",
		PreserveFieldNames: &preserveFieldNames,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = tr.Close() }()

	var frames []map[string]any
	for f := range tr.Frames() {
		if f.Err != nil {
			t.Fatalf("unexpected frame error: %v", f.Err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(f.Data, &decoded); err != nil {
			t.Fatalf("Frame.Data did not decode as JSON: %v (data=%s)", err, f.Data)
		}
		frames = append(frames, decoded)
	}
	if len(frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(frames))
	}

	agentContent, ok := frames[0]["agentContent"].(map[string]any)
	if !ok {
		t.Fatalf("expected top-level camelCase key 'agentContent', got %+v", frames[0])
	}
	if agentContent["agentId"] != "agent_a" || agentContent["content"] != "hello" {
		t.Errorf("expected agentContent.agentId/content (camelCase), got %+v", agentContent)
	}
	if _, present := agentContent["agent_id"]; present {
		t.Errorf("expected snake_case key agent_id to be entirely absent, got %+v", agentContent)
	}

	toolCall, ok := frames[1]["toolCall"].(map[string]any)
	if !ok {
		t.Fatalf("expected top-level camelCase key 'toolCall', got %+v", frames[1])
	}
	if toolCall["toolName"] != "get_weather" {
		t.Errorf("expected toolCall.toolName (camelCase) = 'get_weather', got %+v", toolCall)
	}
	if _, present := toolCall["tool_name"]; present {
		t.Errorf("expected snake_case key tool_name to be entirely absent, got %+v", toolCall)
	}
}

func TestTransport_Frames_EmptyWhenMethodUnset(t *testing.T) {
	srv, err := mockgrpc.New(nil)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	tr := New(Config{Target: srv.Addr(), Plaintext: true})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = tr.Close() }()

	select {
	case f, ok := <-tr.Frames():
		if ok {
			t.Fatalf("expected no frames when Method is unset, got %+v", f)
		}
	case <-time.After(200 * time.Millisecond):
		// No frame within a reasonable window and channel still open —
		// also acceptable (nothing was ever going to be sent).
	}
}

func TestTransport_Close_StopsReadLoopAndClosesFrames(t *testing.T) {
	// A large scripted sequence so the stream is still open when we close.
	events := make([]*agentstreampb.StreamEvent, 0, 50)
	for i := range 50 {
		events = append(events, &agentstreampb.StreamEvent{Payload: &agentstreampb.StreamEvent_AgentContent{
			AgentContent: &agentstreampb.AgentContent{AgentId: "agent_a", Content: "x", Sequence: int32(i)},
		}})
	}
	srv, err := mockgrpc.New(events)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	tr := New(Config{
		Target:    srv.Addr(),
		Plaintext: true,
		Method:    "/agentstream.v1.AgentStream/Stream",
		Request:   map[string]any{"session_id": "s1"},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Drain one frame, then close without draining the rest.
	<-tr.Frames()

	closeDone := make(chan error, 1)
	go func() { closeDone <- tr.Close() }()
	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not return — read-loop goroutine may have leaked")
	}

	// Frames() must be closed (range terminates) after Close returns.
	drainDone := make(chan struct{})
	go func() {
		for range tr.Frames() {
		}
		close(drainDone)
	}()
	select {
	case <-drainDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Frames() channel was not closed after Close()")
	}
}

func TestTransport_Send_ErrorsWithoutABidiStream(t *testing.T) {
	tr := New(Config{Target: "127.0.0.1:1", Plaintext: true})
	if err := tr.Send(context.Background(), []byte("x")); err == nil {
		t.Fatal("expected Send to error when no bidi stream was ever started")
	}
}

// TestTransport_Send_ErrorsForServerStreamingMethod is this phase's
// explicit Done-When: Send on a real, connected server-streaming method
// (the common case) must return a clear, typed-enough-to-assert-on
// error, not a panic or silent drop — never a silent no-op.
func TestTransport_Send_ErrorsForServerStreamingMethod(t *testing.T) {
	srv, err := mockgrpc.New([]*agentstreampb.StreamEvent{
		{Payload: &agentstreampb.StreamEvent_AgentContent{AgentContent: &agentstreampb.AgentContent{AgentId: "agent_a", Content: "hi"}}},
	})
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	tr := connectStreamingTransport(t, srv.Addr(), Config{
		Method:  "/agentstream.v1.AgentStream/Stream",
		Request: map[string]any{"session_id": "s1"},
	})

	if err := tr.Send(context.Background(), []byte(`{}`)); err == nil {
		t.Fatal("expected Send to error for a server-streaming method")
	}
}
