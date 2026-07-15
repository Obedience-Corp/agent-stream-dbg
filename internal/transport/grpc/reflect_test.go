package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/testutil/mockgrpc"
)

func connectedTransport(t *testing.T, addr string) *Transport {
	t.Helper()
	tr := New(Config{Target: addr, Plaintext: true})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	return tr
}

func TestDiscover_ResolvesMethodViaReflection(t *testing.T) {
	srv, err := mockgrpc.New(nil)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	tr := connectedTransport(t, srv.Addr())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	md, err := Discover(ctx, tr.Conn(), "/agentstream.v1.AgentStream/Stream")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if md.GetName() != "Stream" {
		t.Errorf("expected method name 'Stream', got %q", md.GetName())
	}
	if md.GetService().GetFullyQualifiedName() != "agentstream.v1.AgentStream" {
		t.Errorf("expected service 'agentstream.v1.AgentStream', got %q", md.GetService().GetFullyQualifiedName())
	}
	if md.GetInputType().GetName() != "StreamRequest" {
		t.Errorf("expected input type 'StreamRequest', got %q", md.GetInputType().GetName())
	}
	if md.GetOutputType().GetName() != "StreamEvent" {
		t.Errorf("expected output type 'StreamEvent', got %q", md.GetOutputType().GetName())
	}
	if !md.IsServerStreaming() {
		t.Error("expected Stream to be server-streaming")
	}
}

func TestDiscover_ResolvesBidiMethod(t *testing.T) {
	srv, err := mockgrpc.New(nil)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	tr := connectedTransport(t, srv.Addr())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	md, err := Discover(ctx, tr.Conn(), "/agentstream.v1.AgentStream/Chat")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !md.IsClientStreaming() || !md.IsServerStreaming() {
		t.Error("expected Chat to be bidi (client- and server-streaming)")
	}
}

func TestDiscover_ReflectionDisabled_ReturnsDistinguishableError(t *testing.T) {
	srv, err := mockgrpc.NewWithoutReflection(nil)
	if err != nil {
		t.Fatalf("mockgrpc.NewWithoutReflection: %v", err)
	}
	defer srv.Close()

	tr := connectedTransport(t, srv.Addr())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = Discover(ctx, tr.Conn(), "/agentstream.v1.AgentStream/Stream")
	if err == nil {
		t.Fatal("expected an error discovering against a reflection-disabled server")
	}
	if !errors.Is(err, ErrReflectionUnavailable) {
		t.Errorf("expected errors.Is(err, ErrReflectionUnavailable), got: %v", err)
	}
}

func TestDiscover_UnknownService_IsNotReflectionUnavailable(t *testing.T) {
	srv, err := mockgrpc.New(nil)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	tr := connectedTransport(t, srv.Addr())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = Discover(ctx, tr.Conn(), "/nonexistent.v1.NoSuchService/Method")
	if err == nil {
		t.Fatal("expected an error for a nonexistent service")
	}
	if errors.Is(err, ErrReflectionUnavailable) {
		t.Error("expected a 'service not found' error, not ErrReflectionUnavailable — reflection worked fine here")
	}
}

func TestDiscover_UnknownMethod_ReturnsClearError(t *testing.T) {
	srv, err := mockgrpc.New(nil)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	tr := connectedTransport(t, srv.Addr())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = Discover(ctx, tr.Conn(), "/agentstream.v1.AgentStream/NoSuchMethod")
	if err == nil {
		t.Fatal("expected an error for a nonexistent method")
	}
}

func TestSplitMethod(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		wantService string
		wantMethod  string
		wantErr     bool
	}{
		{"valid", "/agent.v1.AgentService/StreamSession", "agent.v1.AgentService", "StreamSession", false},
		{"no leading slash", "agent.v1.AgentService/StreamSession", "agent.v1.AgentService", "StreamSession", false},
		{"no slash at all", "agent.v1.AgentServiceStreamSession", "", "", true},
		{"trailing slash", "/agent.v1.AgentService/", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, method, err := splitMethod(tt.method)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if svc != tt.wantService || method != tt.wantMethod {
				t.Errorf("expected (%q, %q), got (%q, %q)", tt.wantService, tt.wantMethod, svc, method)
			}
		})
	}
}
