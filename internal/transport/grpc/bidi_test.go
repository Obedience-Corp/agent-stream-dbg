package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/testutil/mockgrpc"
)

// TestTransport_Send_BidiRoundTrip is this task's explicit Done-When: a
// message sent on the bidi test method (Chat) arrives at the mock
// server, which echoes it back as a StreamEvent — received here as a
// Frame, decoded through the same protojson pipeline every other gRPC
// frame uses.
func TestTransport_Send_BidiRoundTrip(t *testing.T) {
	srv, err := mockgrpc.New(nil)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	// srv.Close (GracefulStop) waits for the still-open Chat stream to
	// end — registered as Cleanup, not defer, and BEFORE
	// connectStreamingTransport's own Cleanup, so LIFO ordering closes
	// the transport (which ends the stream) first. A plain defer here
	// would run before any Cleanup and hang for the full connect
	// timeout waiting on a stream nothing has told to end yet.
	t.Cleanup(srv.Close)

	tr := connectStreamingTransport(t, srv.Addr(), Config{
		Method: "/agentstream.v1.AgentStream/Chat",
	})

	if err := tr.Send(context.Background(), []byte(`{"text":"hello"}`)); err != nil {
		t.Fatalf("Send: %v", err)
	}

	f := <-tr.Frames()
	if f.Err != nil {
		t.Fatalf("unexpected frame error: %v", f.Err)
	}

	// Default discriminator (oneof — StreamEvent has exactly one) unwraps
	// the response, same as every other oneof-decoded gRPC frame: Name is
	// the case ("agent_content") and Data is the unwrapped AgentContent,
	// not nested under an "agent_content" key.
	if f.Name != "agent_content" {
		t.Errorf("expected frame name 'agent_content', got %q", f.Name)
	}
	var decoded map[string]any
	if err := json.Unmarshal(f.Data, &decoded); err != nil {
		t.Fatalf("Frame.Data did not decode as JSON: %v (data=%s)", err, f.Data)
	}
	if decoded["content"] != "hello" {
		t.Errorf("expected echoed content 'hello', got %+v", decoded)
	}

	// Confirm the server genuinely received it, not just that some frame
	// happened to arrive.
	if msgs := srv.LastChatMessages(); len(msgs) != 1 || msgs[0] != "hello" {
		t.Errorf(`expected server to have recorded ["hello"], got %v`, msgs)
	}
}

// TestTransport_Send_ConcurrentWithFramesDraining is this task's
// concurrency requirement: sending on one goroutine while another drains
// Frames() must be race-free — the standard bidi-streaming pattern this
// library explicitly supports (one goroutine calling SendMsg, another
// calling RecvMsg on the same stream).
func TestTransport_Send_ConcurrentWithFramesDraining(t *testing.T) {
	srv, err := mockgrpc.New(nil)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	// srv.Close (GracefulStop) waits for the still-open Chat stream to
	// end — registered as Cleanup, not defer, and BEFORE
	// connectStreamingTransport's own Cleanup, so LIFO ordering closes
	// the transport (which ends the stream) first. A plain defer here
	// would run before any Cleanup and hang for the full connect
	// timeout waiting on a stream nothing has told to end yet.
	t.Cleanup(srv.Close)

	tr := connectStreamingTransport(t, srv.Addr(), Config{
		Method: "/agentstream.v1.AgentStream/Chat",
	})

	const n = 20
	sendErrs := make(chan error, n)
	go func() {
		defer close(sendErrs)
		for i := 0; i < n; i++ {
			sendErrs <- tr.Send(context.Background(), []byte(fmt.Sprintf(`{"text":"msg-%d"}`, i)))
		}
	}()

	received := 0
	for received < n {
		select {
		case f := <-tr.Frames():
			if f.Err != nil {
				t.Fatalf("unexpected frame error: %v", f.Err)
			}
			received++
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for frames; got %d/%d", received, n)
		}
	}
	for err := range sendErrs {
		if err != nil {
			t.Errorf("Send error: %v", err)
		}
	}
}

// TestTransport_Send_ConcurrentFromMultipleGoroutines regression-tests
// Send's internal mutex: grpc.ClientStream.SendMsg is documented as
// unsafe to call on the same stream from different goroutines (unlike
// Send-from-one-goroutine concurrent with Recv-from-another, which is
// explicitly fine and is TestTransport_Send_ConcurrentWithFramesDraining's
// case). Without serialization here, this test would be exactly the kind
// of scenario -race is meant to catch — many goroutines all calling
// tr.Send at once, racing inside the underlying stream.
func TestTransport_Send_ConcurrentFromMultipleGoroutines(t *testing.T) {
	srv, err := mockgrpc.New(nil)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	t.Cleanup(srv.Close)

	tr := connectStreamingTransport(t, srv.Addr(), Config{
		Method: "/agentstream.v1.AgentStream/Chat",
	})

	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- tr.Send(context.Background(), []byte(fmt.Sprintf(`{"text":"msg-%d"}`, i)))
		}(i)
	}

	received := 0
	for received < n {
		select {
		case f := <-tr.Frames():
			if f.Err != nil {
				t.Fatalf("unexpected frame error: %v", f.Err)
			}
			received++
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for frames; got %d/%d", received, n)
		}
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("Send error: %v", err)
		}
	}
}
