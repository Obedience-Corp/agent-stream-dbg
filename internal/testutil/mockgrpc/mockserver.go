// Package mockgrpc is the gRPC equivalent of internal/testutil's mock SSE
// server: an in-process, reflection-enabled server that replays a
// scripted sequence of StreamEvents, so the gRPC transport (and later
// tasks in this phase) have something real to dial, stream from, and
// break — no live external server, no Docker, no network egress.
package mockgrpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"

	"github.com/lancekrogers/stream-debugger/internal/testutil/mockgrpc/agentstreampb"
)

// Server replays a scripted sequence of StreamEvents to every Stream
// call, and echoes Chat messages back as AgentContent events for bidi
// tests. Reflection is always registered — see WithoutReflection for the
// variant that turns it off, used to test non-reflection fallback tiers.
type Server struct {
	agentstreampb.UnimplementedAgentStreamServer

	events           []*agentstreampb.StreamEvent
	skipReflection   bool
	grpcServer       *grpc.Server
	listener         net.Listener
	tlsListener      net.Listener
	tlsGRPCServer    *grpc.Server
	tlsCertPool      *x509.CertPool
	mu               sync.Mutex
	lastMetadata     metadata.MD
	lastChatMessages []string
}

// New starts a plaintext (h2c-equivalent — grpc-go serves HTTP/2 without
// TLS transparently when configured insecure) mock server replaying
// events verbatim, in order, on every Stream call.
func New(events []*agentstreampb.StreamEvent) (*Server, error) {
	s := &Server{events: events}
	if err := s.startPlaintext(); err != nil {
		return nil, err
	}
	return s, nil
}

// NewWithoutReflection is like New but never registers the reflection
// service — for testing non-reflection schema-acquisition fallbacks
// (descriptor-set, .proto) against a server shaped like production,
// where reflection is frequently disabled.
func NewWithoutReflection(events []*agentstreampb.StreamEvent) (*Server, error) {
	s := &Server{events: events, skipReflection: true}
	if err := s.startPlaintext(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) startPlaintext() error {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("mockgrpc: listen: %w", err)
	}
	srv := grpc.NewServer()
	agentstreampb.RegisterAgentStreamServer(srv, s)
	if !s.skipReflection {
		reflection.Register(srv)
	}
	s.grpcServer = srv
	s.listener = lis
	go func() { _ = srv.Serve(lis) }()
	return nil
}

// StartTLS additionally starts a TLS listener on a second port, using a
// freshly generated self-signed certificate (no static testdata cert to
// rotate or expire). Returns the CA pool a client should trust.
func (s *Server) StartTLS() (*x509.CertPool, error) {
	cert, pool, err := generateSelfSignedCert()
	if err != nil {
		return nil, fmt.Errorf("mockgrpc: generate cert: %w", err)
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("mockgrpc: tls listen: %w", err)
	}
	creds := credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{cert}})
	srv := grpc.NewServer(grpc.Creds(creds))
	agentstreampb.RegisterAgentStreamServer(srv, s)
	if !s.skipReflection {
		reflection.Register(srv)
	}
	s.tlsGRPCServer = srv
	s.tlsListener = lis
	s.tlsCertPool = pool
	go func() { _ = srv.Serve(lis) }()
	return pool, nil
}

// Addr returns the plaintext listener's address, e.g. "127.0.0.1:54321".
func (s *Server) Addr() string { return s.listener.Addr().String() }

// TLSAddr returns the TLS listener's address. Only valid after StartTLS.
func (s *Server) TLSAddr() string { return s.tlsListener.Addr().String() }

// Stream implements agentstreampb.AgentStreamServer: replays the
// server's scripted events verbatim, in order, capturing incoming
// metadata for LastMetadata.
func (s *Server) Stream(req *agentstreampb.StreamRequest, stream grpc.ServerStreamingServer[agentstreampb.StreamEvent]) error {
	s.captureMetadata(stream.Context())
	for _, evt := range s.events {
		if err := stream.Send(evt); err != nil {
			return err
		}
	}
	return nil
}

// Chat implements the bidi RPC: every received message is echoed back as
// an AgentContent event, and also recorded for LastChatMessages.
func (s *Server) Chat(stream grpc.BidiStreamingServer[agentstreampb.ChatMessage, agentstreampb.StreamEvent]) error {
	s.captureMetadata(stream.Context())
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.lastChatMessages = append(s.lastChatMessages, msg.GetText())
		s.mu.Unlock()

		reply := &agentstreampb.StreamEvent{Payload: &agentstreampb.StreamEvent_AgentContent{
			AgentContent: &agentstreampb.AgentContent{AgentId: "assistant", Content: msg.GetText()},
		}}
		if err := stream.Send(reply); err != nil {
			return err
		}
	}
}

func (s *Server) captureMetadata(ctx context.Context) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return
	}
	s.mu.Lock()
	s.lastMetadata = md
	s.mu.Unlock()
}

// LastMetadata returns the incoming metadata of the most recent call.
// Useful for asserting auth metadata arrived as configured.
func (s *Server) LastMetadata() metadata.MD {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastMetadata
}

// LastChatMessages returns every message text received on the most
// recent Chat stream.
func (s *Server) LastChatMessages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.lastChatMessages...)
}

// Close shuts down both listeners, if started.
func (s *Server) Close() {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
	if s.tlsGRPCServer != nil {
		s.tlsGRPCServer.GracefulStop()
	}
}

// generateSelfSignedCert produces an ephemeral ECDSA cert/key pair valid
// for 127.0.0.1 and localhost, plus the CA pool a client needs to trust
// it — no static testdata cert to rotate or expire.
func generateSelfSignedCert() (tls.Certificate, *x509.CertPool, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	// A self-signed leaf, trusted directly via its own cert (see
	// generateSelfSignedCert's caller: pool.AddCert(leaf)) rather than by
	// chain-building — so this is a server cert, not a CA.
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "mockgrpc"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	cert.Leaf = leaf

	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return cert, pool, nil
}
