package file

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
)

func next(t *testing.T, tr *Transport) transport.Frame {
	t.Helper()
	select {
	case f, ok := <-tr.Frames():
		if !ok {
			t.Fatal("unexpected EOF")
		}
		return f
	case <-time.After(2 * time.Second):
		t.Fatal("reader stalled")
		return transport.Frame{}
	}
}
func appendFile(t *testing.T, path, data string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(data); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestFollowPartialAndConnectLifetime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte("{\"type\":\"first\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tr := New(path, true)
	defer tr.Close()
	ctx, cancel := context.WithCancel(context.Background())
	if err := tr.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	first := next(t, tr)
	if first.Err != nil || first.Offset != 0 {
		t.Fatalf("first=%+v", first)
	}
	appendFile(t, path, `{"type":"second"`)
	select {
	case f := <-tr.Frames():
		t.Fatalf("torn line emitted: %+v", f)
	case <-time.After(2 * PollInterval):
	}
	start := time.Now()
	appendFile(t, path, "}\n")
	second := next(t, tr)
	if second.Err != nil || second.Offset != first.Size || !strings.Contains(string(second.Data), "second") {
		t.Fatalf("second=%+v", second)
	}
	if time.Since(start) > time.Second {
		t.Fatal("complete-line follow stalled")
	}
	if err := tr.Send(context.Background(), nil); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
}
func TestFiniteRecoveryAndIncomplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	data := "not json\n" + strings.Repeat("x", MaxLineBytes+100) + "\n{\"type\":\"ok\"}\n{\"torn\":"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	tr := New(path, false)
	defer tr.Close()
	if err := tr.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		f := next(t, tr)
		if (i == 2) != (f.Err == nil) {
			t.Fatalf("frame %d err=%v", i, f.Err)
		}
		if len(f.Data) > MaxLineBytes {
			t.Fatal("oversized frame retained")
		}
	}
	if _, ok := <-tr.Frames(); ok {
		t.Fatal("finite source did not reach EOF")
	}
}
func TestSourceReplacementAndTruncation(t *testing.T) {
	for _, mode := range []string{"truncate", "replace"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "events.jsonl")
			if err := os.WriteFile(path, []byte("{}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			tr := New(path, true)
			defer tr.Close()
			if err := tr.Connect(context.Background()); err != nil {
				t.Fatal(err)
			}
			next(t, tr)
			if mode == "truncate" {
				if err := os.Truncate(path, 0); err != nil {
					t.Fatal(err)
				}
			} else {
				replacement := filepath.Join(dir, "new")
				if err := os.WriteFile(replacement, []byte("{}\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
			}
			if f := next(t, tr); !errors.Is(f.Err, ErrSourceChanged) {
				t.Fatalf("source change=%v", f.Err)
			}
		})
	}
}
func TestCloseReleasesSlowConsumer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(strings.Repeat("{}\n", 100000)), 0600); err != nil {
		t.Fatal(err)
	}
	tr := New(path, true)
	if err := tr.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = tr.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close waited on slow consumer")
	}
}

func TestRawWhitespaceAndSavedPartial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	raw := "  {\"x\":1} \r\n"
	partial := " {\"torn\":"
	if err := os.WriteFile(path, []byte(raw+partial), 0600); err != nil {
		t.Fatal(err)
	}
	tr := New(path, false)
	defer tr.Close()
	if err := tr.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := next(t, tr)
	if string(first.Raw) != raw || string(first.Data) != `{"x":1}` {
		t.Fatalf("raw=%q data=%q", first.Raw, first.Data)
	}
	last := next(t, tr)
	if last.Err == nil || string(last.Raw) != partial {
		t.Fatalf("partial bytes lost: %+v", last)
	}
}
func TestTruncationAheadAndRegrownHeader(t *testing.T) {
	for _, mode := range []string{"ahead", "regrown"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "events.jsonl")
			data := "{\"run\":\"old\"}\n" + strings.Repeat("{}\n", 100000)
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			tr := New(path, true)
			defer tr.Close()
			if err := tr.Connect(context.Background()); err != nil {
				t.Fatal(err)
			}
			if mode == "ahead" {
				if err := os.Truncate(path, 100000); err != nil {
					t.Fatal(err)
				}
			} else {
				data = strings.Replace(data, "old", "new", 1)
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			f := next(t, tr)
			for i := 0; f.Err == nil && i < 23000; i++ {
				f = next(t, tr)
			}
			if !errors.Is(f.Err, ErrSourceChanged) {
				t.Fatalf("continuity missed %s: %v", mode, f.Err)
			}
		})
	}
}
