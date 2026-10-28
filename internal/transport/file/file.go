// Package file reads bounded newline-delimited frames from an explicit local
// artifact. It owns framing and file identity, never capture/dialect semantics.
package file

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
)

const MaxLineBytes = 1 << 20
const PollInterval = 50 * time.Millisecond

var ErrReadOnly = errors.New("file source is read-only")
var ErrSourceChanged = errors.New("Source changed; reopen explicitly")

// Transport uses a synchronous handoff (stricter than the 256 frame/4 MiB
// ceilings). At most one complete frame and one unfinished line are retained;
// a slow viewer can backpressure only its own file reader.
type Transport struct {
	finishing    atomic.Bool
	observedSize int64
	prefix       []byte
	path         string
	follow       bool
	frames       chan transport.Frame
	closed       chan struct{}
	mu           sync.Mutex
	f            *os.File
	initial      os.FileInfo
	connected    bool
	stopped      bool
	closeOnce    sync.Once
	wg           sync.WaitGroup
}

func New(path string, follow bool) *Transport {
	return &Transport{path: path, follow: follow, frames: make(chan transport.Frame), closed: make(chan struct{})}
}
func (t *Transport) Name() string                       { return "file" }
func (t *Transport) Frames() <-chan transport.Frame     { return t.frames }
func (t *Transport) Send(context.Context, []byte) error { return ErrReadOnly }
func (t *Transport) Connect(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return errors.New("file source is closed")
	}
	if t.connected {
		return errors.New("file source already connected")
	}
	f, err := os.Open(t.path)
	if err != nil {
		return fmt.Errorf("open file source: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return errors.New("file source must be a regular file")
	}
	t.f = f
	t.initial = info
	t.observedSize = info.Size()
	prefixSize := min(info.Size(), int64(64<<10))
	if prefixSize > 0 {
		t.prefix = make([]byte, prefixSize)
		if _, err := f.ReadAt(t.prefix, 0); err != nil {
			_ = f.Close()
			return err
		}
	}
	t.connected = true
	t.wg.Add(1)
	go t.read()
	return nil
}
func (t *Transport) Close() error {
	var err error
	t.closeOnce.Do(func() {
		t.mu.Lock()
		t.stopped = true
		close(t.closed)
		f := t.f
		connected := t.connected
		t.mu.Unlock()
		if f != nil {
			err = f.Close()
		}
		if !connected {
			close(t.frames)
		}
	})
	t.wg.Wait()
	return err
}
func (t *Transport) changed(offset int64) error {
	current, err := os.Stat(t.path)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSourceChanged, err)
	}
	if !os.SameFile(t.initial, current) || current.Size() < max(offset, t.observedSize) {
		return ErrSourceChanged
	}
	t.observedSize = max(t.observedSize, current.Size())
	size := min(current.Size(), int64(64<<10))
	if size > 0 {
		prefix := make([]byte, size)
		if _, err := t.f.ReadAt(prefix, 0); err != nil {
			return fmt.Errorf("%w: %v", ErrSourceChanged, err)
		}
		t.mu.Lock()
		previous := t.prefix
		t.mu.Unlock()
		if len(prefix) < len(previous) || !bytes.Equal(previous, prefix[:len(previous)]) {
			return ErrSourceChanged
		}
		t.mu.Lock()
		t.prefix = prefix
		t.mu.Unlock()
	}
	return nil
}
func (t *Transport) emit(data, raw []byte, offset, size int64, err error, terminal ...bool) bool {
	frame := transport.Frame{Data: data, Raw: raw, Timestamp: time.Now(), Err: err, Offset: offset, Size: size}
	if len(terminal) > 0 {
		frame.Terminal = terminal[0]
	}
	select {
	case t.frames <- frame:
		return true
	case <-t.closed:
		return false
	}
}
func (t *Transport) read() {
	defer t.wg.Done()
	defer close(t.frames)
	chunk := make([]byte, 64<<10)
	pending := make([]byte, 0, 64<<10)
	var offset, lineStart, lineSize int64
	oversize := false
	for {
		select {
		case <-t.closed:
			return
		default:
		}
		if err := t.changed(offset); err != nil {
			t.emit(nil, nil, offset, 0, err, true)
			return
		}
		n, err := t.f.Read(chunk)
		if n > 0 {
			offset += int64(n)
			remaining := chunk[:n]
			for len(remaining) > 0 {
				end := bytes.IndexByte(remaining, '\n')
				take := len(remaining)
				if end >= 0 {
					take = end + 1
				}
				part := remaining[:take]
				remaining = remaining[take:]
				lineSize += int64(take)
				if !oversize {
					if len(pending)+take > MaxLineBytes {
						oversize = true
						pending = nil
					} else {
						pending = append(pending, part...)
					}
				}
				if end < 0 {
					continue
				}
				var frameErr error
				var data, raw []byte
				if oversize {
					frameErr = fmt.Errorf("record exceeds %d bytes (%d bytes skipped)", MaxLineBytes, lineSize)
				} else {
					raw = bytes.Clone(pending)
					data = bytes.TrimSpace(raw)
					if len(data) > 0 {
						if !json.Valid(data) || data[0] != '{' {
							frameErr = errors.New("malformed record: expected one JSON object")
						}
					}
				}
				if (len(data) > 0 || frameErr != nil) && !t.emit(data, raw, lineStart, lineSize, frameErr) {
					return
				}
				lineStart += lineSize
				lineSize = 0
				pending = make([]byte, 0, 64<<10)
				oversize = false
			}
		}
		if err != nil && !errors.Is(err, io.EOF) {
			select {
			case <-t.closed:
				return
			default:
			}
			t.emit(nil, nil, offset, 0, fmt.Errorf("read source: %w", err), true)
			return
		}
		if errors.Is(err, io.EOF) {
			if !t.follow || t.finishing.Load() {
				if lineSize > 0 {
					t.emit(bytes.TrimSpace(pending), pending, lineStart, lineSize, errors.New("incomplete final line; capture is unsealed"))
				}
				return
			}
			timer := time.NewTimer(PollInterval)
			select {
			case <-timer.C:
			case <-t.closed:
				timer.Stop()
				return
			}
		}
	}
}

// FinishFollow drains complete records already on disk, then reaches finite EOF.
func (t *Transport) FinishFollow() { t.finishing.Store(true) }

// Snapshot is an immutable inode/prefix anchor for later bounded disk inspection.
func (t *Transport) Snapshot() (os.FileInfo, []byte) {
	t.mu.Lock()
	info, prefix := t.initial, t.prefix
	t.mu.Unlock()
	return info, bytes.Clone(prefix)
}
