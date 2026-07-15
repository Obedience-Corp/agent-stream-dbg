package grpc

import (
	"encoding/json"
	"flag"
	"os"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"github.com/lancekrogers/stream-debugger/internal/testutil/mockgrpc"
	"github.com/lancekrogers/stream-debugger/internal/testutil/mockgrpc/agentstreampb"
)

// recordGRPCFixture regenerates testdata/fixtures/grpc-session.jsonl —
// run via `go test ./internal/transport/grpc/... -run
// TestRecordGRPCSessionFixture -record-grpc-fixture`, mirroring
// dialects/golden_test.go's -update-goldens convention. Off by default:
// normal `go test` runs never touch the checked-in fixture.
var recordGRPCFixture = flag.Bool("record-grpc-fixture", false, "regenerate testdata/fixtures/grpc-session.jsonl")

const grpcSessionFixturePath = "../../../testdata/fixtures/grpc-session.jsonl"

// TestRecordGRPCSessionFixture records one full session from the real
// mock server built across this phase — every response message plus a
// terminating trailer-bearing status — into
// testdata/fixtures/grpc-session.jsonl, in the flat "type"-tagged JSON
// shape internal/transport/replay's dispatchName already expects (the
// same shape every SSE fixture already uses).
//
// The one thing worth documenting: a live oneof-decoded Frame's Data
// does NOT itself carry a "type" field (resolveFrame unwraps to the bare
// inner message — see stream.go's doc comment on why), so recording
// injects Frame.Name as a "type" key before writing each line. This is
// the ONLY difference from just dumping Frame.Data verbatim; replay
// itself needs zero changes; it doesn't know or care that a "type" field
// wasn't already there on the wire — it never was for SSE either.
func TestRecordGRPCSessionFixture(t *testing.T) {
	if !*recordGRPCFixture {
		t.Skip("run with -record-grpc-fixture to regenerate testdata/fixtures/grpc-session.jsonl")
	}

	srv, err := mockgrpc.New([]*agentstreampb.StreamEvent{
		{Payload: &agentstreampb.StreamEvent_SessionStart{SessionStart: &agentstreampb.SessionStart{SessionId: "demo-grpc"}}},
		{Payload: &agentstreampb.StreamEvent_AgentContent{AgentContent: &agentstreampb.AgentContent{AgentId: "agent_a", Content: "Let me check the weather.", Sequence: 1}}},
		{Payload: &agentstreampb.StreamEvent_ToolCall{ToolCall: &agentstreampb.ToolCall{AgentId: "agent_a", ToolName: "get_weather", Arguments: `{"city":"Denver"}`}}},
		{Payload: &agentstreampb.StreamEvent_AgentContent{AgentContent: &agentstreampb.AgentContent{AgentId: "agent_a", Content: "It's sunny in Denver.", Sequence: 2}}},
	})
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()
	srv.SetEndStatus(codes.OK, "", metadata.MD{"trace-id": []string{"demo-trace-001"}})

	tr := connectStreamingTransport(t, srv.Addr(), Config{
		Method:  "/agentstream.v1.AgentStream/Stream",
		Request: map[string]any{"session_id": "demo-grpc"},
	})

	var lines [][]byte
	for f := range tr.Frames() {
		if f.Err != nil {
			t.Fatalf("unexpected frame error: %v", f.Err)
		}
		var fields map[string]any
		if err := json.Unmarshal(f.Data, &fields); err != nil {
			t.Fatalf("unmarshal frame data: %v", err)
		}
		// Guard against silently overwriting a real field literally named
		// "type" (none of the scripted messages have one today, but
		// nothing prevents a future one from adding it) — a clobber here
		// would produce a wrong fixture with no test failure to catch it.
		if existing, ok := fields["type"]; ok && existing != f.Name {
			t.Fatalf("frame %q already has a \"type\" field (%v) that doesn't match Frame.Name — recording would silently clobber it", f.Name, existing)
		}
		fields["type"] = f.Name
		line, err := json.Marshal(fields)
		if err != nil {
			t.Fatalf("marshal fixture line: %v", err)
		}
		lines = append(lines, line)
	}

	out, err := os.Create(grpcSessionFixturePath)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	defer func() { _ = out.Close() }()
	for _, line := range lines {
		if _, err := out.Write(append(line, '\n')); err != nil {
			t.Fatalf("write fixture line: %v", err)
		}
	}

	t.Logf("wrote %d lines to %s", len(lines), grpcSessionFixturePath)
}
