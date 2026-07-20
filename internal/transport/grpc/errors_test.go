package grpc

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/mapping"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/testutil/mockgrpc"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/testutil/mockgrpc/agentstreampb"
)

// statusDialect is the companion test dialect this task's Done-When
// requires: a real rule declared entirely as data, proving grpc_status
// is just another named event a dialect can match — not a special case
// internal/mapping knows about. Kept local to this test file rather than
// the shipped dialect, which stays production-only.
const statusDialect = `
version: 1
name: test-grpc-status
discriminator: event
rules:
  - match: {event: agent_content}
    kind: content
    source: agent_id
    content: content
  - match: {event: grpc_status}
    kind: error
    fields: {grpc_code: code, grpc_message: message, trailers: trailers}
`

// TestGRPCStatus_NonOKStatusAndTrailers_EmitsSyntheticFrame is this
// task's explicit Done-When: a stream ending with codes.Internal and a
// custom trailer, decoded through the companion dialect fragment's rule,
// must produce Kind=Error with Fields["grpc_code"]=="Internal" and
// Fields["trailers"] containing the custom key — proving the mapping is
// pure data, same as every other rule in this codebase.
func TestGRPCStatus_NonOKStatusAndTrailers_EmitsSyntheticFrame(t *testing.T) {
	srv, err := mockgrpc.New([]*agentstreampb.StreamEvent{
		{Payload: &agentstreampb.StreamEvent_AgentContent{AgentContent: &agentstreampb.AgentContent{AgentId: "agent_a", Content: "hi"}}},
	})
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()
	srv.SetEndStatus(codes.Internal, "boom", metadata.MD{"debug-key": []string{"debug-value"}})

	tr := connectStreamingTransport(t, srv.Addr(), Config{
		Method:        "/agentstream.v1.AgentStream/Stream",
		Request:       map[string]any{"session_id": "s1"},
		Discriminator: "oneof",
	})

	var frames []struct {
		name string
		data []byte
		err  error
	}
	for f := range tr.Frames() {
		frames = append(frames, struct {
			name string
			data []byte
			err  error
		}{f.Name, f.Data, f.Err})
	}

	if len(frames) != 2 {
		t.Fatalf("expected 2 frames (agent_content, then grpc_status), got %d", len(frames))
	}
	if frames[0].name != "agent_content" {
		t.Errorf("expected first frame name 'agent_content', got %q", frames[0].name)
	}

	status := frames[1]
	if status.err != nil {
		t.Fatalf("unexpected frame error: %v", status.err)
	}
	if status.name != grpcStatusEventName {
		t.Fatalf("expected sentinel name %q, got %q", grpcStatusEventName, status.name)
	}

	engine, err := mapping.Load([]byte(statusDialect))
	if err != nil {
		t.Fatalf("mapping.Load: %v", err)
	}
	evt := engine.Decode(status.name, status.data)

	if evt.Kind != events.KindError {
		t.Errorf("expected Kind=error, got %v", evt.Kind)
	}
	if evt.Fields["grpc_code"] != "Internal" {
		t.Errorf("expected Fields[grpc_code]='Internal', got %+v", evt.Fields["grpc_code"])
	}
	if evt.Fields["grpc_message"] != "boom" {
		t.Errorf("expected Fields[grpc_message]='boom', got %+v", evt.Fields["grpc_message"])
	}
	trailers, ok := evt.Fields["trailers"].(map[string]any)
	if !ok {
		t.Fatalf("expected Fields[trailers] to be a map, got %T: %+v", evt.Fields["trailers"], evt.Fields["trailers"])
	}
	if trailers["debug-key"] != "debug-value" {
		t.Errorf("expected trailers[debug-key]='debug-value', got %+v", trailers)
	}
}

// TestGRPCStatus_CleanEndNoTrailers_EmitsNothing is the negative case:
// a clean end (OK, no trailers) must not synthesize a grpc_status frame
// at all — existing behavior for every prior test in this package.
func TestGRPCStatus_CleanEndNoTrailers_EmitsNothing(t *testing.T) {
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

	var names []string
	for f := range tr.Frames() {
		if f.Err != nil {
			t.Fatalf("unexpected frame error: %v", f.Err)
		}
		names = append(names, f.Name)
	}
	if len(names) != 1 {
		t.Fatalf("expected exactly 1 frame (no synthetic grpc_status frame on a clean end), got %d: %v", len(names), names)
	}
}

// TestGRPCStatus_CleanEndWithTrailers_StillEmitsFrame covers the
// requirement's "or trailers are non-empty even on a clean end" branch:
// an OK status with trailers must still synthesize a grpc_status frame,
// just with code "OK".
func TestGRPCStatus_CleanEndWithTrailers_StillEmitsFrame(t *testing.T) {
	srv, err := mockgrpc.New(nil)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()
	srv.SetEndStatus(codes.OK, "", metadata.MD{"trace-id": []string{"abc123"}})

	tr := connectStreamingTransport(t, srv.Addr(), Config{
		Method:  "/agentstream.v1.AgentStream/Stream",
		Request: map[string]any{"session_id": "s1"},
	})

	f := <-tr.Frames()
	if f.Err != nil {
		t.Fatalf("unexpected frame error: %v", f.Err)
	}
	if f.Name != grpcStatusEventName {
		t.Fatalf("expected sentinel name %q, got %q", grpcStatusEventName, f.Name)
	}

	engine, err := mapping.Load([]byte(statusDialect))
	if err != nil {
		t.Fatalf("mapping.Load: %v", err)
	}
	evt := engine.Decode(f.Name, f.Data)
	if evt.Fields["grpc_code"] != "OK" {
		t.Errorf("expected Fields[grpc_code]='OK', got %+v", evt.Fields["grpc_code"])
	}
	trailers, ok := evt.Fields["trailers"].(map[string]any)
	if !ok || trailers["trace-id"] != "abc123" {
		t.Errorf("expected trailers[trace-id]='abc123', got %+v", evt.Fields["trailers"])
	}

	// Confirm the channel closes cleanly right after (no further frames).
	if _, ok := <-tr.Frames(); ok {
		t.Error("expected no further frames after the synthetic status frame")
	}
}

// TestGRPCStatus_MultiValueTrailer_JoinsWithComma locks in the documented,
// deliberate tradeoff in emitStreamEnd's trailer flattening: a real
// multi-valued gRPC trailer (metadata.MD's []string) becomes one
// comma-joined string, matching the task's mandated flat
// map[string]string JSON shape. This doesn't claim the join is lossless
// (it isn't, for a value containing a literal comma) — it pins down the
// actual behavior so a future change to the join strategy is a visible,
// intentional diff instead of a silent behavior change.
func TestGRPCStatus_MultiValueTrailer_JoinsWithComma(t *testing.T) {
	srv, err := mockgrpc.New(nil)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()
	srv.SetEndStatus(codes.Internal, "boom", metadata.MD{"multi": []string{"a", "b", "c"}})

	tr := connectStreamingTransport(t, srv.Addr(), Config{
		Method:  "/agentstream.v1.AgentStream/Stream",
		Request: map[string]any{"session_id": "s1"},
	})

	f := <-tr.Frames()
	if f.Err != nil {
		t.Fatalf("unexpected frame error: %v", f.Err)
	}

	engine, err := mapping.Load([]byte(statusDialect))
	if err != nil {
		t.Fatalf("mapping.Load: %v", err)
	}
	evt := engine.Decode(f.Name, f.Data)
	trailers, ok := evt.Fields["trailers"].(map[string]any)
	if !ok {
		t.Fatalf("expected Fields[trailers] to be a map, got %T", evt.Fields["trailers"])
	}
	if trailers["multi"] != "a,b,c" {
		t.Errorf("expected trailers[multi]='a,b,c' (comma-joined), got %+v", trailers["multi"])
	}
}
