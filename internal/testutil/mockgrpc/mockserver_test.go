package mockgrpc

import (
	"context"
	"io"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/testutil/mockgrpc/agentstreampb"
)

func scriptedEvents() []*agentstreampb.StreamEvent {
	return []*agentstreampb.StreamEvent{
		{Payload: &agentstreampb.StreamEvent_SessionStart{SessionStart: &agentstreampb.SessionStart{SessionId: "s1"}}},
		{Payload: &agentstreampb.StreamEvent_AgentContent{AgentContent: &agentstreampb.AgentContent{AgentId: "agent_a", Content: "hello", Sequence: 1}}},
		{Payload: &agentstreampb.StreamEvent_ToolCall{ToolCall: &agentstreampb.ToolCall{AgentId: "agent_a", ToolName: "get_weather", Arguments: `{"city":"Denver"}`}}},
	}
}

// dialInsecure is a THIS-TEST-ONLY generated-client dial, used to prove
// the mock server itself works. The transport under development
// (internal/transport/grpc, later tasks) never uses generated stubs —
// it discovers everything via reflection/descriptor-set/.proto instead.
func dialInsecure(t *testing.T, addr string) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestServer_Stream_ReplaysScriptedEventsVerbatim(t *testing.T) {
	srv, err := New(scriptedEvents())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.Close()

	conn := dialInsecure(t, srv.Addr())
	client := agentstreampb.NewAgentStreamClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := client.Stream(ctx, &agentstreampb.StreamRequest{SessionId: "s1", Message: "hi"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var got []*agentstreampb.StreamEvent
	for {
		evt, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		got = append(got, evt)
	}

	want := scriptedEvents()
	if len(got) != len(want) {
		t.Fatalf("expected %d events, got %d", len(want), len(got))
	}
	if got[0].GetSessionStart().GetSessionId() != "s1" {
		t.Errorf("expected first event session_start.session_id 's1', got %q", got[0].GetSessionStart().GetSessionId())
	}
	if got[1].GetAgentContent().GetContent() != "hello" {
		t.Errorf("expected second event agent_content.content 'hello', got %q", got[1].GetAgentContent().GetContent())
	}
	if got[2].GetToolCall().GetToolName() != "get_weather" {
		t.Errorf("expected third event tool_call.tool_name 'get_weather', got %q", got[2].GetToolCall().GetToolName())
	}
}

func TestServer_CapturesIncomingMetadata(t *testing.T) {
	srv, err := New(scriptedEvents())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.Close()

	conn := dialInsecure(t, srv.Addr())
	client := agentstreampb.NewAgentStreamClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer test-token")

	stream, err := client.Stream(ctx, &agentstreampb.StreamRequest{SessionId: "s1"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for {
		if _, err := stream.Recv(); err != nil {
			break
		}
	}

	md := srv.LastMetadata()
	if got := md.Get("authorization"); len(got) == 0 || got[0] != "Bearer test-token" {
		t.Errorf("expected authorization metadata to arrive, got %v", md.Get("authorization"))
	}
}

func TestServer_Chat_EchoesMessagesAsAgentContent(t *testing.T) {
	srv, err := New(scriptedEvents())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.Close()

	conn := dialInsecure(t, srv.Addr())
	client := agentstreampb.NewAgentStreamClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := client.Chat(ctx)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if err := stream.Send(&agentstreampb.ChatMessage{Text: "ping"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	_ = stream.CloseSend()

	evt, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if evt.GetAgentContent().GetContent() != "ping" {
		t.Errorf("expected echoed content 'ping', got %q", evt.GetAgentContent().GetContent())
	}

	msgs := srv.LastChatMessages()
	if len(msgs) != 1 || msgs[0] != "ping" {
		t.Errorf("expected LastChatMessages ['ping'], got %v", msgs)
	}
}

func TestServer_StartTLS_AcceptsMutuallyTrustedConnection(t *testing.T) {
	srv, err := New(scriptedEvents())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer srv.Close()

	pool, err := srv.StartTLS()
	if err != nil {
		t.Fatalf("StartTLS: %v", err)
	}

	creds := credentials.NewClientTLSFromCert(pool, "localhost")
	conn, err := grpc.NewClient(srv.TLSAddr(), grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatalf("dial TLS: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := agentstreampb.NewAgentStreamClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := client.Stream(ctx, &agentstreampb.StreamRequest{SessionId: "s1"})
	if err != nil {
		t.Fatalf("Stream over TLS: %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv over TLS: %v", err)
	}
}

func TestNewWithoutReflection_StillServesRPCs(t *testing.T) {
	srv, err := NewWithoutReflection(scriptedEvents())
	if err != nil {
		t.Fatalf("NewWithoutReflection: %v", err)
	}
	defer srv.Close()

	conn := dialInsecure(t, srv.Addr())
	client := agentstreampb.NewAgentStreamClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := client.Stream(ctx, &agentstreampb.StreamRequest{SessionId: "s1"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv: %v", err)
	}
}
