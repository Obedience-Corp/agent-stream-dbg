package dialects_test

import (
	"context"
	"testing"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/mapping"
	"github.com/lancekrogers/stream-debugger/internal/testutil/mockgrpc"
	"github.com/lancekrogers/stream-debugger/internal/testutil/mockgrpc/agentstreampb"
	grpctransport "github.com/lancekrogers/stream-debugger/internal/transport/grpc"
)

// TestBrainyardDialect_CrossTransportParity is phase 006's actual gate,
// proven as a permanent test rather than a one-off manual check:
// dialects/brainyard.yaml — the real production dialect, completely
// unmodified — decodes an SSE frame and a gRPC frame carrying equivalent
// field values to identical Kind/SourceID/Content/Seq. This is the
// "stronger proof" the task allows for: not a phase-006-specific test
// dialect, but the shipped dialect itself, unmodified, over both
// transports.
//
// Two sub-cases deliberately exercise BOTH of ValueForm's extraction
// paths (see internal/mapping/value.go — source:/content:/seq: and a
// rule's fields: block both compile to the same ValueForm.Extract
// mechanism, but a single sub-case would only prove one call site):
// agent_content (source:/content:/seq:, no fields: block) and
// session_start (a fields: block, no source:/content:/seq:).
//
// Frame.Name and Frame.Raw are deliberately not compared: an SSE event:
// line and a gRPC oneof case name are legitimately different strings for
// the same logical name, and Raw is each transport's own wire bytes by
// definition — Timestamp is excluded for the same reason (present on the
// SSE fixture line, absent from gRPC's protojson). Event.Fields is
// likewise not compared as a whole map: Decode's best-effort raw-JSON
// passthrough means Fields also carries whatever OTHER keys happened to
// be on the wire (SSE's message_id/timestamp have no gRPC equivalent) —
// legitimately transport-shaped in the same way Raw is. What must match
// is everything the dialect's own rule actually declares.
func TestBrainyardDialect_CrossTransportParity(t *testing.T) {
	engine, err := mapping.LoadFile("brainyard.yaml")
	if err != nil {
		t.Fatalf("mapping.LoadFile: %v", err)
	}

	t.Run("agent_content_source_content_seq", func(t *testing.T) {
		// SSE side: the same wire shape testdata/fixtures/brainyard-session.jsonl
		// already uses for an agent_content frame.
		sseData := []byte(`{"type":"agent_content","message_id":"msg_x","timestamp":"2025-10-31T16:08:00.500000Z","agent_id":"agent_a","content":"hello from sse","sequence":3}`)
		sseEvt := engine.Decode("agent_content", sseData)

		// gRPC side: the same logical event (agent_id/content/sequence),
		// through a real Transport against the real mock server — not a
		// hand-constructed Frame.
		srv, err := mockgrpc.New([]*agentstreampb.StreamEvent{
			{Payload: &agentstreampb.StreamEvent_AgentContent{AgentContent: &agentstreampb.AgentContent{AgentId: "agent_a", Content: "hello from sse", Sequence: 3}}},
		})
		if err != nil {
			t.Fatalf("mockgrpc.New: %v", err)
		}
		defer srv.Close()

		tr := grpctransport.New(grpctransport.Config{
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
		defer func() { _ = tr.Close() }()

		f := <-tr.Frames()
		if f.Err != nil {
			t.Fatalf("unexpected frame error: %v", f.Err)
		}
		grpcEvt := engine.Decode(f.Name, f.Data)

		if grpcEvt.Kind != sseEvt.Kind {
			t.Errorf("Kind mismatch: sse=%v grpc=%v", sseEvt.Kind, grpcEvt.Kind)
		}
		if grpcEvt.SourceID != sseEvt.SourceID {
			t.Errorf("SourceID mismatch: sse=%q grpc=%q", sseEvt.SourceID, grpcEvt.SourceID)
		}
		if grpcEvt.Content != sseEvt.Content {
			t.Errorf("Content mismatch: sse=%q grpc=%q", sseEvt.Content, grpcEvt.Content)
		}
		if grpcEvt.Seq != sseEvt.Seq {
			t.Errorf("Seq mismatch: sse=%d grpc=%d", sseEvt.Seq, grpcEvt.Seq)
		}

		// Independently confirm the actual values, not just sse==grpc
		// parity (which could itself be coincidentally wrong on both sides).
		if grpcEvt.Kind != events.KindContent || grpcEvt.SourceID != "agent_a" || grpcEvt.Content != "hello from sse" || grpcEvt.Seq != 3 {
			t.Errorf("expected Kind=content SourceID=agent_a Content='hello from sse' Seq=3, got Kind=%v SourceID=%q Content=%q Seq=%d",
				grpcEvt.Kind, grpcEvt.SourceID, grpcEvt.Content, grpcEvt.Seq)
		}
	})

	t.Run("session_start_fields_block", func(t *testing.T) {
		// SSE side: brainyard's session_start rule (fields: {session_id:
		// session_id, flow_id: flow_id}) against a wire shape matching
		// testdata/fixtures/brainyard-session.jsonl's first line.
		sseData := []byte(`{"type":"session_start","message_id":"msg_0001","timestamp":"2025-10-31T16:08:00.000000Z","session_id":"demo-9f2a1c","flow_id":"default"}`)
		sseEvt := engine.Decode("session_start", sseData)

		// gRPC side: the test proto's SessionStart only carries session_id
		// (no flow_id concept) — flow_id is absent from Fields on both
		// sides equally, so this still tests the same fields: extraction
		// path without asserting something the schema can't express.
		srv, err := mockgrpc.New([]*agentstreampb.StreamEvent{
			{Payload: &agentstreampb.StreamEvent_SessionStart{SessionStart: &agentstreampb.SessionStart{SessionId: "demo-9f2a1c"}}},
		})
		if err != nil {
			t.Fatalf("mockgrpc.New: %v", err)
		}
		defer srv.Close()

		tr := grpctransport.New(grpctransport.Config{
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
		defer func() { _ = tr.Close() }()

		f := <-tr.Frames()
		if f.Err != nil {
			t.Fatalf("unexpected frame error: %v", f.Err)
		}
		grpcEvt := engine.Decode(f.Name, f.Data)

		if grpcEvt.Kind != sseEvt.Kind {
			t.Errorf("Kind mismatch: sse=%v grpc=%v", sseEvt.Kind, grpcEvt.Kind)
		}
		if grpcEvt.Fields["session_id"] != sseEvt.Fields["session_id"] {
			t.Errorf("Fields[session_id] mismatch: sse=%v grpc=%v", sseEvt.Fields["session_id"], grpcEvt.Fields["session_id"])
		}

		if grpcEvt.Kind != events.KindSessionStart || grpcEvt.Fields["session_id"] != "demo-9f2a1c" {
			t.Errorf("expected Kind=session_start Fields[session_id]=demo-9f2a1c, got Kind=%v Fields[session_id]=%v",
				grpcEvt.Kind, grpcEvt.Fields["session_id"])
		}
	})
}
