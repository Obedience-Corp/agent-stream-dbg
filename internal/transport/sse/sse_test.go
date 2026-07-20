package sse

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/testutil"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
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

func framesFromBytes(t *testing.T, fixture string) []transport.Frame {
	t.Helper()
	tr := &Transport{
		resp:   &http.Response{Body: io.NopCloser(bytes.NewBufferString(fixture))},
		frames: make(chan transport.Frame, 32),
		closed: make(chan struct{}),
	}
	tr.wg.Add(1)
	tr.readLoop(context.Background())

	var frames []transport.Frame
	for frame := range tr.Frames() {
		frames = append(frames, frame)
	}
	return frames
}

func TestTransport_FinalSingleLineWithoutTrailingNewline(t *testing.T) {
	const fixture = "data: tail"
	frames := framesFromBytes(t, fixture)

	if len(frames) != 1 {
		t.Fatalf("expected one final frame, got %d: %+v", len(frames), frames)
	}
	if string(frames[0].Data) != "tail" {
		t.Errorf("expected final data to be parsed, got %q", frames[0].Data)
	}
	if string(frames[0].Raw) != fixture {
		t.Errorf("expected Raw to preserve the final line, got %q", frames[0].Raw)
	}
	if frames[0].Err == nil {
		t.Error("expected EOF mid-frame error")
	}
}

func TestTransport_FinalMultilineFrameWithoutTrailingNewline(t *testing.T) {
	const fixture = "event: content\ndata: first\ndata: final"
	frames := framesFromBytes(t, fixture)

	if len(frames) != 1 {
		t.Fatalf("expected one final frame, got %d: %+v", len(frames), frames)
	}
	if frames[0].Name != "content" {
		t.Errorf("expected event name content, got %q", frames[0].Name)
	}
	if string(frames[0].Data) != "first\nfinal" {
		t.Errorf("expected both data lines, got %q", frames[0].Data)
	}
	if string(frames[0].Raw) != fixture {
		t.Errorf("expected Raw to preserve both lines, got %q", frames[0].Raw)
	}
	if frames[0].Err == nil {
		t.Error("expected EOF mid-frame error")
	}
}

func TestTransport_FinalFrameWithTrailingNewline(t *testing.T) {
	const fixture = "event: content\ndata: complete\n\n"
	frames := framesFromBytes(t, fixture)

	if len(frames) != 1 {
		t.Fatalf("expected one complete frame, got %d: %+v", len(frames), frames)
	}
	if frames[0].Name != "content" || string(frames[0].Data) != "complete" {
		t.Errorf("expected complete content frame, got name=%q data=%q", frames[0].Name, frames[0].Data)
	}
	if frames[0].Err != nil {
		t.Errorf("expected no error for trailing-newline frame, got %v", frames[0].Err)
	}
}

func TestTransport_HappyPath_RealFixture(t *testing.T) {
	fixture, err := testutil.ReferenceSessionFixturePath("../../../testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := testutil.NewMockSSEServer(fixture, 0)
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
	fixture, err := testutil.ReferenceSessionFixturePath("../../../testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := testutil.NewMockSSEServer(fixture, 20*time.Millisecond)
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

func TestTransport_StreamContextCancelWithoutClose(t *testing.T) {
	baseline := runtime.NumGoroutine()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for i := 0; ; i++ {
			if _, err := fmt.Fprintf(w, "event: tick\ndata: %d\n\n", i); err != nil {
				return
			}
			flusher.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(5 * time.Millisecond):
			}
		}
	}))

	tr := New(http.MethodGet, srv.URL, nil, nil)
	if err := tr.Connect(context.Background()); err != nil {
		srv.Close()
		t.Fatalf("connect failed: %v", err)
	}
	if _, ok := <-tr.Frames(); !ok {
		srv.Close()
		t.Fatal("stream ended before the cancellation test started")
	}

	// Connect owns a private stream context under the Transport contract;
	// cancel it directly here to exercise the producer's ctx.Done path
	// without calling the public Close method.
	tr.streamCancel()
	done := make(chan struct{})
	go func() {
		for range tr.Frames() {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		srv.Close()
		t.Fatal("Frames() did not close after stream context cancellation")
	}
	srv.Close()
	waitForGoroutines(t, baseline)
}

func TestTransport_ConnectError_ClosesFrames(t *testing.T) {
	baseline := runtime.NumGoroutine()
	tr := New(http.MethodGet, "://invalid-url", nil, nil)
	if err := tr.Connect(context.Background()); err == nil {
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

// TestTransport_Close_UndrainedFullBuffer_DoesNotDeadlock reproduces a
// real bug found while building the analogous gRPC transport: with more
// frames buffered than the channel's capacity and nobody draining
// Frames(), readLoop blocks on the channel send itself — closing
// resp.Body (network I/O) doesn't unblock that. Close must still return.
func TestTransport_Close_UndrainedFullBuffer_DoesNotDeadlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for i := range 100 {
			_, _ = fmt.Fprintf(w, "event: agent_content\ndata: {\"n\":%d}\n\n", i)
		}
		flusher.Flush()
	}))
	defer srv.Close()

	tr := New(http.MethodGet, srv.URL, nil, nil)
	if err := tr.Connect(context.Background()); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	// Drain exactly one frame — far fewer than the 100 sent and more than
	// the channel's buffer capacity — then close without draining the rest.
	<-tr.Frames()

	closeDone := make(chan error, 1)
	go func() { closeDone <- tr.Close() }()
	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not return — readLoop deadlocked on a full, undrained channel")
	}
}
