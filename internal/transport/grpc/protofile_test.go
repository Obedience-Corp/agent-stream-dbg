package grpc

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/testutil/mockgrpc"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/testutil/mockgrpc/agentstreampb"
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

// TestLoadProtoFile_AbsolutePath_ResolvesEvenWithImportPaths regression-
// tests a bug an adversarial review caught: SourceResolver finds a file
// by filepath.Join-ing it onto each import path, which doesn't
// special-case an absolute second argument — it just concatenates and
// cleans, so an absolute protoPath combined with any non-empty
// importPaths previously never resolved (e.g. "/a/b.proto" joined onto
// "/c" tries to open "/c/a/b.proto"). The fix treats the file's own
// directory as an implicit, highest-priority import root.
func TestLoadProtoFile_AbsolutePath_ResolvesEvenWithImportPaths(t *testing.T) {
	abs, err := filepath.Abs(filepath.Join(protoFileImportRoot, protoFileFixture))
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	// A deliberately wrong/unrelated import path, to prove resolution
	// doesn't depend on getting lucky with the caller's own import roots
	// — only the absolute path's own directory should matter for finding
	// the file itself.
	md, err := LoadProtoFile(context.Background(), abs, []string{"/nonexistent/unrelated/path"}, "/agentstream.v1.AgentStream/Stream")
	if err != nil {
		t.Fatalf("LoadProtoFile with absolute path: %v", err)
	}
	if md.GetName() != "Stream" {
		t.Errorf("expected method name 'Stream', got %q", md.GetName())
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

// TestTransport_Connect_BothDescriptorSetAndProtoFile_Rejected regression-
// tests a footgun an adversarial review flagged: with no fallback-on-
// failure logic (only ever one resolution attempt), a Config with both
// DescriptorSetPath and ProtoFilePath set is always either a mistake or
// stale leftover config — silently preferring one (as the code
// previously did) would let a stale descriptor set shadow every edit to
// a .proto file the caller thinks is the one in effect. Connect now
// rejects the ambiguous config outright, before any dial.
func TestTransport_Connect_BothDescriptorSetAndProtoFile_Rejected(t *testing.T) {
	tr := New(Config{
		Target:            "127.0.0.1:1",
		Plaintext:         true,
		Method:            "/agentstream.v1.AgentStream/Stream",
		DescriptorSetPath: descriptorSetFixture,
		ProtoFilePath:     protoFileFixture,
		ProtoImportPaths:  []string{protoFileImportRoot},
	})
	if err := tr.Connect(context.Background()); err == nil {
		t.Fatal("expected Connect to reject a Config with both DescriptorSetPath and ProtoFilePath set")
	}
}
