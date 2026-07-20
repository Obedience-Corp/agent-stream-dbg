// Package replay is a Transport over a recorded JSONL fixture — one flat
// JSON object per line, the same shape the structured logger writes and
// live SSE dialects already expect. Replay is a transport like any other:
// this is what lets every dialect be decode-tested with zero network.
package replay

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Obedience-Corp/stream-debugger/internal/transport"
)

// Transport replays a fixture's lines as Frames, in order.
type Transport struct {
	lines [][]byte
	pace  time.Duration

	frames chan transport.Frame
	// closed is closed by Close before anything else, so the emitter
	// goroutine unblocks immediately even if nobody is draining Frames()
	// (a full, undrained channel) or waiting out a pacing delay —
	// Close no longer depends on the caller having canceled Connect's ctx.
	closed    chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// New reads fixturePath eagerly, failing fast on a missing or empty
// fixture rather than at first Connect. pace delays between frames to
// simulate real timing; zero (the default) replays as fast as possible —
// what CI and `just demo` use.
func New(fixturePath string, pace time.Duration) (*Transport, error) {
	lines, err := readFixtureLines(fixturePath)
	if err != nil {
		return nil, err
	}
	return &Transport{
		lines:  lines,
		pace:   pace,
		frames: make(chan transport.Frame, 32),
		closed: make(chan struct{}),
	}, nil
}

func readFixtureLines(fixturePath string) ([][]byte, error) {
	f, err := os.Open(fixturePath)
	if err != nil {
		return nil, fmt.Errorf("replay: open fixture %s: %w", fixturePath, err)
	}
	defer func() { _ = f.Close() }()

	var lines [][]byte
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		cp := make([]byte, len(line))
		copy(cp, line)
		lines = append(lines, cp)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("replay: read fixture %s: %w", fixturePath, err)
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("replay: fixture %s contains no events", fixturePath)
	}
	return lines, nil
}

// Name identifies this transport kind.
func (t *Transport) Name() string { return "replay" }

// Connect starts emitting the fixture's lines as Frames in the
// background and returns immediately; Frames() delivers them as they're
// paced out.
func (t *Transport) Connect(ctx context.Context) error {
	t.wg.Add(1)
	go t.emit(ctx)
	return nil
}

func (t *Transport) emit(ctx context.Context) {
	defer t.wg.Done()
	defer close(t.frames)

	for i, line := range t.lines {
		if i > 0 && t.pace > 0 {
			select {
			case <-time.After(t.pace):
			case <-ctx.Done():
				return
			case <-t.closed:
				return
			}
		}

		frame := transport.Frame{
			Name:      dispatchName(line),
			Data:      line,
			Raw:       line,
			Timestamp: time.Now(),
		}
		select {
		case t.frames <- frame:
		case <-ctx.Done():
			return
		case <-t.closed:
			return
		}
	}
}

// dispatchName extracts the "type" field from a fixture line — there is
// no separate wire-level event name in this flat format, so the frame
// name comes from the payload itself, exactly like a live SSE frame whose
// discriminator reads the same field.
func dispatchName(line []byte) string {
	var probe struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(line, &probe)
	return probe.Type
}

// Frames returns the channel of replayed frames. It closes once the
// fixture is exhausted or ctx is canceled.
func (t *Transport) Frames() <-chan transport.Frame {
	return t.frames
}

// Send is not supported: a fixture is a read-only recording.
func (t *Transport) Send(ctx context.Context, payload []byte) error {
	return fmt.Errorf("replay: send not supported — fixtures are read-only")
}

// Close signals the emitter to stop (unblocking it whether it's waiting
// out a pacing delay or blocked sending to a full, undrained Frames()
// channel) and waits for it to fully exit before returning — Close alone
// is always sufficient, independent of whether the caller ever cancels
// Connect's ctx.
func (t *Transport) Close() error {
	t.closeOnce.Do(func() {
		close(t.closed)
		t.wg.Wait()
	})
	return nil
}
