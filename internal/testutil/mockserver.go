// Package testutil provides test doubles for exercising agent-stream-dbg
// against realistic data without a live backend.
package testutil

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"time"
)

// MockSSEServer replays a JSONL fixture of SSE events over a real HTTP
// SSE endpoint, so the SSE client can be exercised end-to-end offline.
type MockSSEServer struct {
	srv   *httptest.Server
	delay time.Duration
	lines [][]byte

	mu          sync.Mutex
	lastHeaders http.Header
}

// NewMockSSEServer reads the JSONL fixture at fixturePath and returns a
// server that replays each line as an SSE frame on every request, waiting
// delay between events and honoring request context cancellation.
func NewMockSSEServer(fixturePath string, delay time.Duration) (*MockSSEServer, error) {
	lines, err := readFixtureLines(fixturePath)
	if err != nil {
		return nil, err
	}

	m := &MockSSEServer{delay: delay, lines: lines}
	m.srv = httptest.NewServer(http.HandlerFunc(m.handle))
	return m, nil
}

func readFixtureLines(fixturePath string) ([][]byte, error) {
	f, err := os.Open(fixturePath)
	if err != nil {
		return nil, fmt.Errorf("open fixture %s: %w", fixturePath, err)
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
		return nil, fmt.Errorf("read fixture %s: %w", fixturePath, err)
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("fixture %s contains no events", fixturePath)
	}
	return lines, nil
}

// handle serves a session-stream endpoint: it streams fixture
// lines as SSE frames, in order, over a single response. A `stream` query
// parameter (as sent by the real SSE client, one connection per event type)
// filters replay to only that event type, matching the real backend contract.
func (m *MockSSEServer) handle(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	streamFilter := r.URL.Query().Get("stream")

	m.mu.Lock()
	m.lastHeaders = r.Header.Clone()
	m.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	first := true
	for _, line := range m.lines {
		eventType := eventTypeOf(line)
		if streamFilter != "" && eventType != streamFilter {
			continue
		}

		if !first && m.delay > 0 {
			select {
			case <-time.After(m.delay):
			case <-ctx.Done():
				return
			}
		}
		first = false

		select {
		case <-ctx.Done():
			return
		default:
		}

		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, line); err != nil {
			return
		}
		flusher.Flush()
	}
}

type typeProbe struct {
	Type string `json:"type"`
}

// eventTypeOf extracts the SSE event name from a fixture line's "type" field.
func eventTypeOf(line []byte) string {
	var p typeProbe
	if err := json.Unmarshal(line, &p); err != nil || p.Type == "" {
		return "message"
	}
	return p.Type
}

// URL returns the base URL of the mock server.
func (m *MockSSEServer) URL() string {
	return m.srv.URL
}

// LastHeaders returns the headers of the most recently received request.
// Useful for asserting auth headers arrived as configured.
func (m *MockSSEServer) LastHeaders() http.Header {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastHeaders
}

// Close shuts down the underlying test server.
func (m *MockSSEServer) Close() {
	m.srv.Close()
}
