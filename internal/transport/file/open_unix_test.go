//go:build unix

package file

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestExplicitFIFOIsRejectedWithoutWaitingForWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { tr := New(path, true); defer tr.Close(); done <- tr.Connect(context.Background()) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO accepted as capture")
		}
	case <-time.After(time.Second):
		t.Fatal("FIFO attach waited for pipe writer")
	}
	if f, err := OpenRegular(path); err == nil {
		f.Close()
		t.Fatal("FIFO sidecar accepted")
	}
}
