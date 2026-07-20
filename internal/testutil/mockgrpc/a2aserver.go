package mockgrpc

import (
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/Obedience-Corp/stream-debugger/internal/testutil/mockgrpc/a2apb"
)

// A2AServer is the gRPC equivalent of Server (above) for the A2A-shaped
// mock service (a2apb.A2AServiceServer) — see testdata/proto/a2a.proto's
// header for why this is a separate, minimal service rather than an
// addition to AgentStream. Modeled directly on Server: NewA2A/Addr/Close,
// one server-streaming RPC (SendStreamingMessage) replaying a scripted
// StreamResponse sequence verbatim, in order.
//
// Deliberately narrower than Server: no TLS, no reflection-disabled
// variant, no bidi RPC. dialects/cross_transport_test.go's
// TestA2ADialect_CrossTransportParity — the only consumer — needs none
// of those; Server already proves those transport-level paths generically
// against agentstreampb's messages, and duplicating them here would
// re-test the same transport code a second time against a different
// message type for no additional coverage.
type A2AServer struct {
	a2apb.UnimplementedA2AServiceServer

	events     []*a2apb.StreamResponse
	grpcServer *grpc.Server
	listener   net.Listener
}

// NewA2A starts a plaintext, reflection-enabled mock A2AService server
// replaying events verbatim, in order, on every SendStreamingMessage
// call. Reflection is always registered (unlike Server, which offers
// NewWithoutReflection): the reflection fallback tiers are already
// covered generically by Server's own tests, so there is no reason to
// re-prove them here too.
func NewA2A(events []*a2apb.StreamResponse) (*A2AServer, error) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("mockgrpc: a2a listen: %w", err)
	}
	s := &A2AServer{events: events, listener: lis}
	srv := grpc.NewServer()
	a2apb.RegisterA2AServiceServer(srv, s)
	reflection.Register(srv)
	s.grpcServer = srv
	go func() { _ = srv.Serve(lis) }()
	return s, nil
}

// Addr returns the plaintext listener's address, e.g. "127.0.0.1:54321".
func (s *A2AServer) Addr() string { return s.listener.Addr().String() }

// SendStreamingMessage implements a2apb.A2AServiceServer: replays the
// server's scripted StreamResponse events verbatim, in order, ignoring
// req (this mock never inspects request content — see
// testdata/proto/a2a.proto's SendMessageRequest doc comment).
func (s *A2AServer) SendStreamingMessage(req *a2apb.SendMessageRequest, stream a2apb.A2AService_SendStreamingMessageServer) error {
	for _, evt := range s.events {
		if err := stream.Send(evt); err != nil {
			return err
		}
	}
	return nil
}

// Close shuts down the listener.
func (s *A2AServer) Close() {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
}
