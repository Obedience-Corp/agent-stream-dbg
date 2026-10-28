//go:build unix

package visualizer

import (
	"context"
	"errors"
	filetransport "github.com/Obedience-Corp/agent-stream-dbg/internal/transport/file"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestHistoryFIFOReplacementDoesNotBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		f, err := historyOpen(context.Background(), historyRequest{Path: path})
		if f != nil {
			f.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, filetransport.ErrSourceChanged) {
			t.Fatalf("replacement not labeled source changed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("history blocked waiting for FIFO writer")
	}
}
