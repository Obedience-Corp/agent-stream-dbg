// Package sse is a raw-preserving stdlib SSE reader. Deliberately not a
// good client library: it never normalizes a frame, never reconnects, and
// never drops malformed input — each of those would hide the exact thing a
// debugger exists to show. See workflow/design "Drop r3labs/sse".
package sse

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/transport"
)

// Transport reads one SSE response body and emits every frame on it,
// exactly as received, until the stream ends or ctx is canceled.
type Transport struct {
	method  string
	url     string
	body    []byte
	headers map[string]string

	httpClient *http.Client
	resp       *http.Response

	frames chan transport.Frame
	// closed is closed by Close before anything else, so the read-loop
	// goroutine unblocks immediately whether it's waiting on network I/O
	// or blocked sending to a full, undrained frames channel — closing
	// resp.Body alone only unblocks the former.
	closed    chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
}

// New builds an SSE transport for one request. method/url/body are
// typically rendered from a dialect's send: template — Connect issues
// exactly this request and streams its response body as Frames.
func New(method, url string, body []byte, headers map[string]string) *Transport {
	return &Transport{
		method:     method,
		url:        url,
		body:       body,
		headers:    headers,
		httpClient: http.DefaultClient,
		frames:     make(chan transport.Frame, 32),
		closed:     make(chan struct{}),
	}
}

// Name identifies this transport kind.
func (t *Transport) Name() string { return "sse" }

// Connect issues the configured request and starts streaming its response
// body as Frames in the background. It returns once the connection is
// established and the read loop has started; Frames() delivers events as
// they arrive.
func (t *Transport) Connect(ctx context.Context) error {
	var bodyReader io.Reader
	if len(t.body) > 0 {
		bodyReader = bytes.NewReader(t.body)
	}
	req, err := http.NewRequestWithContext(ctx, t.method, t.url, bodyReader)
	if err != nil {
		return fmt.Errorf("sse: build request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sse: connect: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return fmt.Errorf("sse: unexpected status %d: %s", resp.StatusCode, respBody)
	}
	t.resp = resp

	t.wg.Add(1)
	go t.readLoop()
	return nil
}

// Frames returns the channel of frames read off the wire. It is closed
// once the stream ends (EOF), a read error occurs, or Close is called.
func (t *Transport) Frames() <-chan transport.Frame {
	return t.frames
}

// Send is not supported: this transport's request is fixed at Connect
// time (a dialect's send: template renders the full method/url/body up
// front), so there is nothing left to push mid-stream.
func (t *Transport) Send(ctx context.Context, payload []byte) error {
	return fmt.Errorf("sse: send not supported — connection parameters are fixed at Connect time")
}

// Close shuts the connection down and waits for the read loop to fully
// exit before returning, so Frames() is guaranteed closed and no send on
// it can race a close by construction. Closing t.closed first — not just
// resp.Body — matters when nobody is draining Frames(): the read loop
// would otherwise block forever on a full channel send with no network
// I/O left to unblock it, hanging Close indefinitely.
func (t *Transport) Close() error {
	t.closeOnce.Do(func() {
		close(t.closed)
		if t.resp != nil {
			t.closeErr = t.resp.Body.Close()
		}
		t.wg.Wait()
	})
	return t.closeErr
}

// readLoop parses event:/data:/id:/retry: fields, dispatching a Frame on
// every blank line. It never normalizes and never skips: a frame that
// arrives malformed (the stream ends or errors mid-block) is still
// emitted, with Err set and whatever bytes were received intact in Raw.
func (t *Transport) readLoop() {
	defer t.wg.Done()
	defer close(t.frames)

	r := bufio.NewReader(t.resp.Body)

	var rawBuf, dataBuf bytes.Buffer
	var eventName string
	var hasContent bool

	// emit reports whether the frame was delivered; false means Close
	// signaled shutdown while nobody was draining Frames() — the caller
	// must stop reading immediately rather than keep parsing into a
	// reader nobody is still funneling anywhere.
	emit := func(err error) bool {
		data := bytes.TrimSuffix(dataBuf.Bytes(), []byte("\n"))
		frame := transport.Frame{
			Name:      eventName,
			Data:      append([]byte(nil), data...),
			Raw:       append([]byte(nil), rawBuf.Bytes()...),
			Timestamp: time.Now(),
			Err:       err,
		}
		rawBuf.Reset()
		dataBuf.Reset()
		eventName = ""
		hasContent = false

		select {
		case t.frames <- frame:
			return true
		case <-t.closed:
			return false
		}
	}

	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			rawBuf.Write(line)
		}
		if err != nil {
			if hasContent {
				emit(fmt.Errorf("sse: stream ended mid-frame: %w", err))
			}
			return
		}

		trimmed := bytes.TrimRight(line, "\r\n")
		if len(trimmed) == 0 {
			if hasContent {
				if !emit(nil) {
					return
				}
			} else {
				rawBuf.Reset() // pure blank/comment-only block: no frame to dispatch
			}
			continue
		}
		if trimmed[0] == ':' {
			continue // comment line — kept in Raw, not a field
		}

		field, value, _ := bytes.Cut(trimmed, []byte(":"))
		value = bytes.TrimPrefix(value, []byte(" "))
		switch string(field) {
		case "event":
			eventName = string(value)
			hasContent = true
		case "data":
			dataBuf.Write(value)
			dataBuf.WriteByte('\n')
			hasContent = true
		}
		// id:/retry:/unknown fields are preserved in Raw but otherwise
		// ignored — this transport never auto-reconnects, so retry: has
		// no effect, and Frame carries no id.
	}
}
