package replay

import (
	"context"
	"testing"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/mapping"
)

// grpcTestDialect mirrors internal/transport/grpc's own test dialect
// (see errors_test.go's statusDialect) — the same small, real rule set
// used across phase 006's gRPC tests, kept inline here too rather than
// shared, since this is the one place proving replay itself needs no
// gRPC-specific knowledge: it decodes this exactly like any other
// discriminator: event dialect, with zero awareness the fixture's
// content ever came from a gRPC transport.
const grpcTestDialect = `
version: 1
name: test-grpc-replay
discriminator: event
rules:
  - match: {event: session_start}
    kind: session_start
    fields: {session_id: session_id}
  - match: {event: agent_content}
    kind: content
    source: agent_id
    content: content
    seq: sequence
  - match: {event: tool_call}
    kind: tool_call
    source: agent_id
    fields: {tool_name: tool_name, arguments: arguments}
  - match: {event: grpc_status}
    kind: error
    fields: {grpc_code: code, grpc_message: message, trailers: trailers}
`

// TestTransport_ReplaysRecordedGRPCFixture is phase 006's actual gate,
// mechanically proven: internal/transport/replay, completely UNMODIFIED
// by this phase, replays a gRPC-sourced fixture and decodes it through
// the same dialect engine every SSE fixture uses — replay is a
// transport, agnostic to what produced its JSONL, exactly as designed in
// phase 004. See internal/transport/grpc/fixture_test.go for how the
// fixture was recorded (and why replay needs zero changes to consume
// it).
func TestTransport_ReplaysRecordedGRPCFixture(t *testing.T) {
	tr, err := New("../../../testdata/fixtures/grpc-session.jsonl", 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = tr.Close() }()

	engine, err := mapping.Load([]byte(grpcTestDialect))
	if err != nil {
		t.Fatalf("mapping.Load: %v", err)
	}

	var evts []*events.Event
	for f := range tr.Frames() {
		if f.Err != nil {
			t.Fatalf("unexpected frame error: %v", f.Err)
		}
		evts = append(evts, engine.Decode(f.Name, f.Data))
	}

	if len(evts) != 5 {
		t.Fatalf("expected 5 events, got %d", len(evts))
	}

	if evts[0].Kind != events.KindSessionStart || evts[0].Fields["session_id"] != "demo-grpc" {
		t.Errorf("event 0: expected KindSessionStart session_id=demo-grpc, got Kind=%v Fields=%+v", evts[0].Kind, evts[0].Fields)
	}
	if evts[1].Kind != events.KindContent || evts[1].SourceID != "agent_a" || evts[1].Content != "Let me check the weather." || evts[1].Seq != 1 {
		t.Errorf("event 1: unexpected content event: %+v", evts[1])
	}
	if evts[2].Kind != events.KindToolCall || evts[2].SourceID != "agent_a" || evts[2].Fields["tool_name"] != "get_weather" {
		t.Errorf("event 2: unexpected tool_call event: %+v", evts[2])
	}
	if evts[3].Kind != events.KindContent || evts[3].Content != "It's sunny in Denver." || evts[3].Seq != 2 {
		t.Errorf("event 3: unexpected content event: %+v", evts[3])
	}

	status := evts[4]
	if status.Kind != events.KindError {
		t.Errorf("event 4: expected KindError, got %v", status.Kind)
	}
	if status.Fields["grpc_code"] != "OK" {
		t.Errorf("event 4: expected grpc_code=OK, got %+v", status.Fields["grpc_code"])
	}
	trailers, ok := status.Fields["trailers"].(map[string]any)
	if !ok || trailers["trace-id"] != "demo-trace-001" {
		t.Errorf("event 4: expected trailers[trace-id]=demo-trace-001, got %+v", status.Fields["trailers"])
	}
}
