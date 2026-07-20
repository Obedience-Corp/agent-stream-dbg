package grpc

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/testutil/mockgrpc"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/testutil/mockgrpc/agentstreampb"
)

func waitForGoroutines(t *testing.T, baseline int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline+8 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("goroutines did not settle near baseline: baseline=%d current=%d", baseline, runtime.NumGoroutine())
}

func TestTransport_Connect_Plaintext(t *testing.T) {
	srv, err := mockgrpc.New(nil)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	tr := New(Config{Target: srv.Addr(), Plaintext: true})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = tr.Close() }()

	if tr.Conn() == nil {
		t.Fatal("expected a non-nil ClientConn after Connect")
	}
}

func TestTransport_Connect_TLS(t *testing.T) {
	srv, err := mockgrpc.New(nil)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	pool, err := srv.StartTLS()
	if err != nil {
		t.Fatalf("StartTLS: %v", err)
	}

	tr := New(Config{Target: srv.TLSAddr(), Plaintext: false, TLSRootCAs: pool})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect over TLS: %v", err)
	}
	defer func() { _ = tr.Close() }()
}

func TestTransport_Connect_MetadataAuthArrivesServerSide(t *testing.T) {
	srv, err := mockgrpc.New([]*agentstreampb.StreamEvent{
		{Payload: &agentstreampb.StreamEvent_SessionStart{SessionStart: &agentstreampb.SessionStart{SessionId: "s1"}}},
	})
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	tr := New(Config{
		Target:        srv.Addr(),
		Plaintext:     true,
		MetadataKey:   "authorization",
		MetadataValue: "Bearer test-token",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = tr.Close() }()

	// Drive one RPC through the transport's outgoingContext to prove the
	// metadata actually attaches — using a generated client here is fine,
	// this test is proving the dial+metadata layer, not the (not yet
	// built) reflection-driven streaming path.
	client := agentstreampb.NewAgentStreamClient(tr.Conn())
	stream, err := client.Stream(tr.outgoingContext(ctx), &agentstreampb.StreamRequest{SessionId: "s1"})
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

func TestTransport_Connect_UnreachableTarget_FailsWithinDeadline(t *testing.T) {
	// A closed listener: nothing will ever accept this connection.
	tr := New(Config{Target: "127.0.0.1:1", Plaintext: true})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := tr.Connect(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error connecting to an unreachable target")
	}
	if !strings.Contains(err.Error(), "grpc: connect") {
		t.Errorf("expected a wrapped, actionable error naming the failed step, got: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("expected Connect to fail within roughly ctx's deadline, took %v", elapsed)
	}
}

func TestTransport_ConnectError_ClosesFrames(t *testing.T) {
	baseline := runtime.NumGoroutine()
	tr := New(Config{Target: "127.0.0.1:1", Plaintext: true})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := tr.Connect(ctx); err == nil {
		t.Fatal("expected Connect to fail")
	}

	done := make(chan struct{})
	go func() {
		for range tr.Frames() {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Frames() remained open after Connect error")
	}
	waitForGoroutines(t, baseline)
}

func TestTransport_Close_WithoutConnect_IsSafe(t *testing.T) {
	tr := New(Config{Target: "127.0.0.1:1", Plaintext: true})
	if err := tr.Close(); err != nil {
		t.Errorf("expected Close before Connect to be a safe no-op, got: %v", err)
	}
}
