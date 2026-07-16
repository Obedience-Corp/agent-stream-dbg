package grpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/mapping"
	"github.com/lancekrogers/stream-debugger/internal/testutil/mockgrpc"
	"github.com/lancekrogers/stream-debugger/internal/testutil/mockgrpc/agentstreampb"
)

// connectStreamingTransport connects with a context that outlives this
// call — a server-streaming RPC's context governs the stream for its
// whole lifetime (like internal/transport/sse's Connect), not just the
// initial handshake, so the ctx used here must stay live until the
// caller is done draining Frames() and calls Close.
func connectStreamingTransport(t *testing.T, addr string, cfg Config) *Transport {
	t.Helper()
	cfg.Target = addr
	cfg.Plaintext = true
	tr := New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := tr.Connect(ctx); err != nil {
		cancel()
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() {
		_ = tr.Close()
		cancel()
	})
	return tr
}

func TestDiscriminator_Oneof_DefaultsWhenExactlyOneOneOfExists(t *testing.T) {
	srv, err := mockgrpc.New([]*agentstreampb.StreamEvent{
		{Payload: &agentstreampb.StreamEvent_SessionStart{SessionStart: &agentstreampb.SessionStart{SessionId: "s1"}}},
		{Payload: &agentstreampb.StreamEvent_AgentContent{AgentContent: &agentstreampb.AgentContent{AgentId: "agent_a", Content: "hi", Sequence: 1}}},
		{Payload: &agentstreampb.StreamEvent_ToolCall{ToolCall: &agentstreampb.ToolCall{AgentId: "agent_a", ToolName: "get_weather"}}},
	})
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	// Discriminator left unset — StreamEvent has exactly one top-level
	// oneof, so this must auto-default to oneof mode per architecture.md's
	// stated happy path.
	tr := connectStreamingTransport(t, srv.Addr(), Config{
		Method:  "/agentstream.v1.AgentStream/Stream",
		Request: map[string]any{"session_id": "s1"},
	})

	var names []string
	for f := range tr.Frames() {
		if f.Err != nil {
			t.Fatalf("unexpected frame error: %v", f.Err)
		}
		names = append(names, f.Name)
	}

	want := []string{"session_start", "agent_content", "tool_call"}
	if len(names) != len(want) {
		t.Fatalf("expected %d frames, got %d: %v", len(want), len(names), names)
	}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("frame %d: expected name %q, got %q", i, w, names[i])
		}
	}
}

func TestDiscriminator_Oneof_ExplicitMatchesDefault(t *testing.T) {
	srv, err := mockgrpc.New([]*agentstreampb.StreamEvent{
		{Payload: &agentstreampb.StreamEvent_AgentContent{AgentContent: &agentstreampb.AgentContent{AgentId: "agent_a", Content: "hi"}}},
	})
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	tr := connectStreamingTransport(t, srv.Addr(), Config{
		Method:        "/agentstream.v1.AgentStream/Stream",
		Request:       map[string]any{"session_id": "s1"},
		Discriminator: "oneof",
	})

	f := <-tr.Frames()
	if f.Err != nil {
		t.Fatalf("unexpected frame error: %v", f.Err)
	}
	if f.Name != "agent_content" {
		t.Errorf("expected name 'agent_content', got %q", f.Name)
	}
}

func TestDiscriminator_FieldType_ReadsConfiguredField(t *testing.T) {
	srv, err := mockgrpc.New(nil)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()
	srv.SetTypedEvents([]*agentstreampb.TypedEnvelope{
		{Type: "custom.session_started"},
		{Type: "custom.agent_spoke"},
	})

	tr := connectStreamingTransport(t, srv.Addr(), Config{
		Method:             "/agentstream.v1.AgentStream/StreamTyped",
		Request:            map[string]any{"session_id": "s1"},
		Discriminator:      "field:type",
		DiscriminatorField: "type",
	})

	var names []string
	for f := range tr.Frames() {
		if f.Err != nil {
			t.Fatalf("unexpected frame error: %v", f.Err)
		}
		names = append(names, f.Name)
	}

	want := []string{"custom.session_started", "custom.agent_spoke"}
	if len(names) != len(want) {
		t.Fatalf("expected %d frames, got %d: %v", len(want), len(names), names)
	}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("frame %d: expected name %q, got %q", i, w, names[i])
		}
	}
}

func TestDiscriminator_MessageType_UsesFullyQualifiedName(t *testing.T) {
	srv, err := mockgrpc.New([]*agentstreampb.StreamEvent{
		{Payload: &agentstreampb.StreamEvent_AgentContent{AgentContent: &agentstreampb.AgentContent{AgentId: "agent_a", Content: "hi"}}},
	})
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	tr := connectStreamingTransport(t, srv.Addr(), Config{
		Method:        "/agentstream.v1.AgentStream/Stream",
		Request:       map[string]any{"session_id": "s1"},
		Discriminator: "message_type",
	})

	f := <-tr.Frames()
	if f.Err != nil {
		t.Fatalf("unexpected frame error: %v", f.Err)
	}
	if f.Name != "agentstream.v1.StreamEvent" {
		t.Errorf("expected name 'agentstream.v1.StreamEvent', got %q", f.Name)
	}
}

func TestDiscriminator_None_LeavesFrameNameEmpty(t *testing.T) {
	srv, err := mockgrpc.New([]*agentstreampb.StreamEvent{
		{Payload: &agentstreampb.StreamEvent_AgentContent{AgentContent: &agentstreampb.AgentContent{AgentId: "agent_a", Content: "hi"}}},
	})
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	tr := connectStreamingTransport(t, srv.Addr(), Config{
		Method:        "/agentstream.v1.AgentStream/Stream",
		Request:       map[string]any{"session_id": "s1"},
		Discriminator: "none",
	})

	f := <-tr.Frames()
	if f.Err != nil {
		t.Fatalf("unexpected frame error: %v", f.Err)
	}
	if f.Name != "" {
		t.Errorf("expected empty name for discriminator: none, got %q", f.Name)
	}
}

// TestDiscriminator_Oneof_DecodesThroughUnmodifiedMappingEngine is this
// task's explicit Done-When: a frame decoded via oneof mode, passed
// through the UNMODIFIED mapping.Engine (a small test dialect using
// discriminator: event — exactly what any SSE dialect uses), must
// produce the same Kind/Content/Source a hand-checked equivalent SSE
// frame would. internal/mapping never learns gRPC exists; the
// discriminator: oneof resolution happening entirely in this package,
// before Engine.Decode ever runs, is the whole point.
func TestDiscriminator_Oneof_DecodesThroughUnmodifiedMappingEngine(t *testing.T) {
	srv, err := mockgrpc.New([]*agentstreampb.StreamEvent{
		{Payload: &agentstreampb.StreamEvent_AgentContent{AgentContent: &agentstreampb.AgentContent{AgentId: "agent_a", Content: "hello", Sequence: 3}}},
	})
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	tr := connectStreamingTransport(t, srv.Addr(), Config{
		Method:        "/agentstream.v1.AgentStream/Stream",
		Request:       map[string]any{"session_id": "s1"},
		Discriminator: "oneof",
	})

	f := <-tr.Frames()
	if f.Err != nil {
		t.Fatalf("unexpected frame error: %v", f.Err)
	}
	if f.Name != "agent_content" {
		t.Fatalf("expected name 'agent_content', got %q", f.Name)
	}

	// A dialect that has never heard of gRPC — discriminator: event is
	// exactly what dialects/brainyard.yaml uses for SSE.
	engine, err := mapping.Load([]byte(`
version: 1
name: test
discriminator: event
rules:
  - match: {event: agent_content}
    kind: content
    source: agent_id
    content: content
    seq: sequence
`))
	if err != nil {
		t.Fatalf("mapping.Load: %v", err)
	}

	// Sanity: the same rule decodes an equivalent hand-authored SSE frame
	// identically, proving this isn't a coincidence of the assertions.
	sseData := []byte(`{"type":"agent_content","agent_id":"agent_a","content":"hello","sequence":3}`)
	sseEvt := engine.Decode("agent_content", sseData)

	grpcEvt := engine.Decode(f.Name, f.Data)

	if grpcEvt.Kind != sseEvt.Kind {
		t.Errorf("expected Kind %v (matching SSE), got %v", sseEvt.Kind, grpcEvt.Kind)
	}
	if grpcEvt.SourceID != sseEvt.SourceID {
		t.Errorf("expected SourceID %q (matching SSE), got %q", sseEvt.SourceID, grpcEvt.SourceID)
	}
	if grpcEvt.Content != sseEvt.Content {
		t.Errorf("expected Content %q (matching SSE), got %q", sseEvt.Content, grpcEvt.Content)
	}
	if grpcEvt.Seq != sseEvt.Seq {
		t.Errorf("expected Seq %d (matching SSE), got %d", sseEvt.Seq, grpcEvt.Seq)
	}

	// Independently confirm the field values, not just parity with the
	// SSE side (which could itself be wrong).
	var decoded map[string]any
	if err := json.Unmarshal(f.Data, &decoded); err != nil {
		t.Fatalf("Frame.Data did not decode as JSON: %v", err)
	}
	if grpcEvt.SourceID != "agent_a" || grpcEvt.Content != "hello" || grpcEvt.Seq != 3 {
		t.Errorf("expected SourceID=agent_a Content=hello Seq=3, got SourceID=%q Content=%q Seq=%d",
			grpcEvt.SourceID, grpcEvt.Content, grpcEvt.Seq)
	}
}

// TestDiscriminator_Oneof_FindsPopulatedOneofPastFirstDeclared regression-
// tests the self-review finding on resolveFrame: MultiOneofEvent declares
// two independent top-level oneofs ("first", "second"); this message
// populates only the second one. Before the fix, resolveFrame checked only
// oneofs[0] ("first") and, finding it unpopulated, would resolve an empty
// name instead of searching "second". discriminator: oneof must find
// whichever oneof is actually populated, not just the first declared.
func TestDiscriminator_Oneof_FindsPopulatedOneofPastFirstDeclared(t *testing.T) {
	srv, err := mockgrpc.New(nil)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()
	srv.SetMultiOneofEvents([]*agentstreampb.MultiOneofEvent{
		{Second: &agentstreampb.MultiOneofEvent_SecondContent{SecondContent: &agentstreampb.AgentContent{AgentId: "agent_a", Content: "hi"}}},
	})

	tr := connectStreamingTransport(t, srv.Addr(), Config{
		Method:        "/agentstream.v1.AgentStream/StreamMultiOneof",
		Request:       map[string]any{"session_id": "s1"},
		Discriminator: "oneof",
	})

	f := <-tr.Frames()
	if f.Err != nil {
		t.Fatalf("unexpected frame error: %v", f.Err)
	}
	if f.Name != "second_content" {
		t.Errorf("expected name 'second_content' (the populated oneof, past the unpopulated 'first'), got %q", f.Name)
	}
}
