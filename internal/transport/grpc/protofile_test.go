package grpc

import (
	"context"
	"testing"

	"github.com/lancekrogers/stream-debugger/internal/testutil/mockgrpc"
	"github.com/lancekrogers/stream-debugger/internal/testutil/mockgrpc/agentstreampb"
)

// protoFileFixture and its import root — the same testdata/proto/agentstream.proto
// the mock server's stubs and the descriptor-set fixture both come from,
// so all three tiers resolve the exact same schema.
const (
	protoFileFixture    = "agentstream.proto"
	protoFileImportRoot = "../../../testdata/proto"
)

func TestLoadProtoFile_ResolvesMethod(t *testing.T) {
	md, err := LoadProtoFile(context.Background(), protoFileFixture, []string{protoFileImportRoot}, "/agentstream.v1.AgentStream/Stream")
	if err != nil {
		t.Fatalf("LoadProtoFile: %v", err)
	}
	if md.GetName() != "Stream" {
		t.Errorf("expected method name 'Stream', got %q", md.GetName())
	}
	if !md.IsServerStreaming() {
		t.Error("expected Stream to be server-streaming")
	}
	if md.GetOutputType().GetFullyQualifiedName() != "agentstream.v1.StreamEvent" {
		t.Errorf("expected output type agentstream.v1.StreamEvent, got %q", md.GetOutputType().GetFullyQualifiedName())
	}
}

func TestLoadProtoFile_MissingFile_ReturnsClearError(t *testing.T) {
	_, err := LoadProtoFile(context.Background(), "does-not-exist.proto", []string{protoFileImportRoot}, "/agentstream.v1.AgentStream/Stream")
	if err == nil {
		t.Fatal("expected an error for a missing .proto file")
	}
}

func TestLoadProtoFile_UnknownService_ReturnsClearError(t *testing.T) {
	_, err := LoadProtoFile(context.Background(), protoFileFixture, []string{protoFileImportRoot}, "/nonexistent.v1.NoSuchService/Stream")
	if err == nil {
		t.Fatal("expected an error for an unknown service")
	}
}

func TestLoadProtoFile_UnknownMethod_ReturnsClearError(t *testing.T) {
	_, err := LoadProtoFile(context.Background(), protoFileFixture, []string{protoFileImportRoot}, "/agentstream.v1.AgentStream/NoSuchMethod")
	if err == nil {
		t.Fatal("expected an error for an unknown method")
	}
}

// TestTransport_AllThreeTiers_ProduceIdenticalStreamingBehavior extends
// the descriptor-set task's parity test to all three acquisition tiers:
// reflection, descriptor-set, and runtime .proto compilation, streaming
// the same scripted events through each and asserting byte-identical
// Frame sequences — one source of truth (agentstream.proto), one
// expected descriptor shape, three ways to get there.
func TestTransport_AllThreeTiers_ProduceIdenticalStreamingBehavior(t *testing.T) {
	events := []*agentstreampb.StreamEvent{
		{Payload: &agentstreampb.StreamEvent_SessionStart{SessionStart: &agentstreampb.SessionStart{SessionId: "s1"}}},
		{Payload: &agentstreampb.StreamEvent_AgentContent{AgentContent: &agentstreampb.AgentContent{AgentId: "agent_a", Content: "hi", Sequence: 1}}},
	}

	reflectSrv, err := mockgrpc.New(events)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer reflectSrv.Close()

	noReflectSrv, err := mockgrpc.NewWithoutReflection(events)
	if err != nil {
		t.Fatalf("mockgrpc.NewWithoutReflection: %v", err)
	}
	defer noReflectSrv.Close()

	tiers := []struct {
		name string
		addr string
		cfg  Config
	}{
		{"reflection", reflectSrv.Addr(), Config{}},
		{"descriptor-set", noReflectSrv.Addr(), Config{DescriptorSetPath: descriptorSetFixture}},
		{"proto-compile", noReflectSrv.Addr(), Config{ProtoFilePath: protoFileFixture, ProtoImportPaths: []string{protoFileImportRoot}}},
	}

	type frame struct{ name, data string }
	results := make(map[string][]frame, len(tiers))

	for _, tier := range tiers {
		cfg := tier.cfg
		cfg.Method = "/agentstream.v1.AgentStream/Stream"
		cfg.Request = map[string]any{"session_id": "s1"}
		tr := connectStreamingTransport(t, tier.addr, cfg)

		var frames []frame
		for f := range tr.Frames() {
			if f.Err != nil {
				t.Fatalf("tier %s: unexpected frame error: %v", tier.name, f.Err)
			}
			frames = append(frames, frame{f.Name, string(f.Data)})
		}
		results[tier.name] = frames
	}

	want := results["reflection"]
	for _, tier := range tiers[1:] {
		got := results[tier.name]
		if len(got) != len(want) {
			t.Fatalf("tier %s: expected %d frames (matching reflection), got %d", tier.name, len(want), len(got))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("tier %s: frame %d differs:\n  reflection: %+v\n  %s: %+v", tier.name, i, want[i], tier.name, got[i])
			}
		}
	}
}
