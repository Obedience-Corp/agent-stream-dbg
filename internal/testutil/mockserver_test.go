package testutil

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestNewMockSSEServer_Errors(t *testing.T) {
	tests := []struct {
		name        string
		fixturePath string
		wantErrSub  string
	}{
		{
			name:        "missing fixture file",
			fixturePath: "/nonexistent/path/fixture.jsonl",
			wantErrSub:  "open fixture",
		},
		{
			name:        "empty fixture file",
			fixturePath: writeFixture(t),
			wantErrSub:  "no events",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, err := NewMockSSEServer(tt.fixturePath, 0)
			if err == nil {
				srv.Close()
				t.Fatalf("expected error containing %q, got nil", tt.wantErrSub)
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Errorf("expected error containing %q, got %q", tt.wantErrSub, err.Error())
			}
		})
	}
}

func TestMockSSEServer_ContextCancellation(t *testing.T) {
	// Long delay so the request would hang if cancellation weren't honored.
	fixture := writeFixture(t,
		`{"type":"session_start","session_id":"s1"}`,
		`{"type":"agent_content","agent_id":"a1","content":"one","sequence":1}`,
		`{"type":"agent_content","agent_id":"a1","content":"two","sequence":2}`,
	)
	srv, err := NewMockSSEServer(fixture, 5*time.Second)
	if err != nil {
		t.Fatalf("NewMockSSEServer: %v", err)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL(), nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	scanner := bufio.NewScanner(resp.Body)
	if !scanner.Scan() {
		t.Fatalf("expected at least one line before cancellation")
	}
	firstLine := scanner.Text()
	if !strings.HasPrefix(firstLine, "event: session_start") {
		t.Errorf("expected first frame to be session_start event, got %q", firstLine)
	}

	done := make(chan struct{})
	go func() {
		cancel()
		for scanner.Scan() { //nolint:revive // draining until the cancelled connection closes
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not honor context cancellation within 2s")
	}
}

func TestMockSSEServer_ReplaysFixtureVerbatim(t *testing.T) {
	fixture := writeFixture(t,
		`{"type":"session_start","session_id":"s1"}`,
		`{"type":"agent_content","agent_id":"a1","content":"hi","sequence":1}`,
	)
	srv, err := NewMockSSEServer(fixture, 0)
	if err != nil {
		t.Fatalf("NewMockSSEServer: %v", err)
	}
	defer srv.Close()

	resp, err := http.Get(srv.URL())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", resp.Header.Get("Content-Type"))
	}

	var frames []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			frames = append(frames, line)
		}
	}

	want := []string{
		"event: session_start",
		`data: {"type":"session_start","session_id":"s1"}`,
		"event: agent_content",
		`data: {"type":"agent_content","agent_id":"a1","content":"hi","sequence":1}`,
	}
	if len(frames) != len(want) {
		t.Fatalf("expected %d frames, got %d: %v", len(want), len(frames), frames)
	}
	for i, w := range want {
		if frames[i] != w {
			t.Errorf("frame %d: expected %q, got %q", i, w, frames[i])
		}
	}
}

func TestMockSSEServer_StreamQueryFilters(t *testing.T) {
	fixture := writeFixture(t,
		`{"type":"session_start","session_id":"s1"}`,
		`{"type":"agent_content","agent_id":"a1","content":"hi","sequence":1}`,
		`{"type":"agent_content","agent_id":"a1","content":"there","sequence":2}`,
	)
	srv, err := NewMockSSEServer(fixture, 0)
	if err != nil {
		t.Fatalf("NewMockSSEServer: %v", err)
	}
	defer srv.Close()

	resp, err := http.Get(srv.URL() + "?stream=agent_content")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var eventLines int
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			eventLines++
			if line != "event: agent_content" {
				t.Errorf("expected only agent_content events, got %q", line)
			}
		}
	}
	if eventLines != 2 {
		t.Errorf("expected 2 filtered agent_content events, got %d", eventLines)
	}
}
