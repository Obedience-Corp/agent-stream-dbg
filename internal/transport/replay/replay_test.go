package replay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/bridge"
	"github.com/lancekrogers/stream-debugger/internal/events"
)

func writeFixture(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestNew_Errors(t *testing.T) {
	tests := []struct {
		name        string
		fixturePath string
		wantErrSub  string
	}{
		{"missing fixture file", "/nonexistent/path/fixture.jsonl", "open fixture"},
		{"empty fixture file", writeFixture(t), "no events"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.fixturePath, 0)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErrSub, err)
			}
		})
	}
}

func TestTransport_HappyPath_RealFixture(t *testing.T) {
	tr, err := New("../../../testdata/fixtures/brainyard-session.jsonl", 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = tr.Close() }()

	var names []string
	for f := range tr.Frames() {
		if f.Err != nil {
			t.Errorf("unexpected frame error: %v", f.Err)
		}
		if f.Timestamp.IsZero() {
			t.Errorf("expected non-zero timestamp")
		}
		if len(f.Raw) == 0 {
			t.Errorf("expected Raw to be populated")
		}
		names = append(names, f.Name)
	}

	if len(names) != 22 {
		t.Fatalf("expected 22 frames, got %d", len(names))
	}
	if names[0] != "session_start" {
		t.Errorf("expected first frame 'session_start', got %q", names[0])
	}
	if names[21] != "session_complete" {
		t.Errorf("expected last frame 'session_complete', got %q", names[21])
	}
}

// TestTransport_DecodesThroughDialectEngine proves the Done-When claim
// this transport exists to satisfy: a dialect decode test needs only a
// fixture + this transport — no live network, no special-cased file
// reader. Frames go straight into the same bridge.Parser (brainyard.yaml)
// a live SSE stream would use.
func TestTransport_DecodesThroughDialectEngine(t *testing.T) {
	tr, err := New("../../../testdata/fixtures/brainyard-session.jsonl", 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = tr.Close() }()

	parser := bridge.NewParser()
	var kinds []events.Kind
	for f := range tr.Frames() {
		evt, err := parser.Parse(f.Name, f.Data)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		kinds = append(kinds, evt.Kind)
	}

	if kinds[0] != events.KindSessionStart {
		t.Errorf("expected first event Kind=%v, got %v", events.KindSessionStart, kinds[0])
	}
	if kinds[len(kinds)-1] != events.KindSessionEnd {
		t.Errorf("expected last event Kind=%v, got %v", events.KindSessionEnd, kinds[len(kinds)-1])
	}
	for i, k := range kinds {
		if k == events.KindUnknown {
			t.Errorf("frame %d decoded as Unknown (dialect gap)", i)
		}
	}
}

func TestTransport_Send_ReturnsClearError(t *testing.T) {
	tr, err := New("../../../testdata/fixtures/brainyard-session.jsonl", 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := tr.Send(context.Background(), []byte("hello")); err == nil {
		t.Fatal("expected Send to return an error for a read-only fixture")
	}
}

func TestTransport_PacingHonorsContextCancellation(t *testing.T) {
	fixture := writeFixture(t,
		`{"type":"session_start","session_id":"s1"}`,
		`{"type":"agent_content","agent_id":"a1","content":"hi","sequence":1}`,
		`{"type":"session_complete","session_id":"s1"}`,
	)
	tr, err := New(fixture, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// First frame arrives immediately (no pre-delay); cancel before the
	// pacing delay for the second frame elapses.
	<-tr.Frames()
	cancel()

	done := make(chan struct{})
	go func() {
		for range tr.Frames() {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Frames() did not close promptly after context cancellation")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- tr.Close() }()
	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not return — emitter goroutine may have leaked")
	}
}

// TestTransport_Close_UndrainedFullBuffer_DoesNotDeadlock reproduces a
// real bug found while building the gRPC transport's analogous shutdown
// path: with more frames than the channel's buffer capacity and nobody
// draining Frames(), the emitter blocks on the channel send itself —
// without an internal close signal, only a caller-canceled ctx could
// unblock that, which Close must not depend on.
func TestTransport_Close_UndrainedFullBuffer_DoesNotDeadlock(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf(`{"type":"agent_content","sequence":%d}`, i)
	}
	fixture := writeFixture(t, lines...)

	tr, err := New(fixture, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Never canceled — Close alone must still be sufficient.
	ctx := context.Background()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	<-tr.Frames()

	closeDone := make(chan error, 1)
	go func() { closeDone <- tr.Close() }()
	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not return — emitter deadlocked on a full, undrained channel")
	}
}
