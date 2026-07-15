package sse

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/testutil"
)

func TestTransport_HappyPath_RealFixture(t *testing.T) {
	srv, err := testutil.NewMockSSEServer("../../../testdata/fixtures/brainyard-session.jsonl", 0)
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	tr := New(http.MethodGet, srv.URL(), nil, nil)
	if err := tr.Connect(context.Background()); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer func() { _ = tr.Close() }()

	var frames []struct {
		name string
		err  error
	}
	for f := range tr.Frames() {
		if f.Err != nil {
			t.Errorf("unexpected malformed frame from a well-formed fixture: %v (raw=%q)", f.Err, f.Raw)
		}
		if f.Timestamp.IsZero() {
			t.Errorf("expected a non-zero receipt timestamp")
		}
		frames = append(frames, struct {
			name string
			err  error
		}{f.Name, f.Err})
	}

	if len(frames) != 22 {
		t.Fatalf("expected 22 frames (one per fixture line), got %d", len(frames))
	}
	if frames[0].name != "session_start" {
		t.Errorf("expected first frame name 'session_start', got %q", frames[0].name)
	}
	if frames[21].name != "session_complete" {
		t.Errorf("expected last frame name 'session_complete', got %q", frames[21].name)
	}
}

func TestTransport_MalformedFrame_ErrAndRawSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "event: content\ndata: hello\n\n")
		flusher.Flush()
		// Truncated mid-line: no trailing newline, no closing blank
		// line — the connection just ends here.
		_, _ = fmt.Fprint(w, "event: broken\ndata: {\"incomplete")
		flusher.Flush()
	}))
	defer srv.Close()

	tr := New(http.MethodGet, srv.URL, nil, nil)
	if err := tr.Connect(context.Background()); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer func() { _ = tr.Close() }()

	var got []struct {
		name string
		data string
		raw  string
		err  error
	}
	for f := range tr.Frames() {
		got = append(got, struct {
			name string
			data string
			raw  string
			err  error
		}{f.Name, string(f.Data), string(f.Raw), f.Err})
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 frames (one clean, one malformed), got %d: %+v", len(got), got)
	}

	clean := got[0]
	if clean.err != nil {
		t.Errorf("expected first frame to decode cleanly, got err: %v", clean.err)
	}
	if clean.name != "content" || clean.data != "hello" {
		t.Errorf("expected clean frame {content, hello}, got {%q, %q}", clean.name, clean.data)
	}

	broken := got[1]
	if broken.err == nil {
		t.Fatal("expected the truncated final frame to surface an error, got nil")
	}
	if !strings.Contains(broken.raw, `event: broken`) || !strings.Contains(broken.raw, `data: {"incomplete`) {
		t.Errorf("expected Raw to preserve the exact truncated bytes, got %q", broken.raw)
	}
}

func TestTransport_ContextCancellation_CleanShutdown(t *testing.T) {
	srv, err := testutil.NewMockSSEServer("../../../testdata/fixtures/brainyard-session.jsonl", 20*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to start mock server: %v", err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	tr := New(http.MethodGet, srv.URL(), nil, nil)
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	// Let a couple of frames through, then cancel mid-stream.
	<-tr.Frames()
	<-tr.Frames()
	cancel()

	// Frames() must close (readLoop must exit) without panicking or
	// hanging, however the pending frames are drained.
	done := make(chan struct{})
	go func() {
		for range tr.Frames() {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Frames() channel did not close after context cancellation")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- tr.Close() }()
	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not return — readLoop goroutine may have leaked")
	}
}
