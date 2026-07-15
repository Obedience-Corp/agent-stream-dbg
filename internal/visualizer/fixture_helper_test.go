package visualizer

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/lancekrogers/stream-debugger/internal/bridge"
	"github.com/lancekrogers/stream-debugger/internal/config"
)

// newFixtureModel builds an InteractiveModel with one completed turn driven
// entirely from testdata/fixtures/brainyard-session.jsonl via applyParsedEvent
// (the same live-stream incremental path fixture_parity_test.go exercises),
// plus the equivalent raw SSE text for that turn. Shared by the render/update
// tests added for the 007_VISUALIZER_FLOW_VIEW interactive.go file split, each
// of which needs a populated model rather than a zero-value one.
func newFixtureModel(t *testing.T) *InteractiveModel {
	t.Helper()

	f, err := os.Open("../../testdata/fixtures/brainyard-session.jsonl")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan fixture: %v", err)
	}

	cfg := &config.EnhancedConfig{}
	cfg.Session.ID = "fixture-test-session"
	cfg.Normalize()

	m := NewInteractiveModel(cfg)
	m.messages = append(m.messages, Message{Text: "What is consciousness?"})
	m.streamIndex = 0

	parser := bridge.NewParser()
	var rawSSE strings.Builder
	for _, line := range lines {
		evt, err := parser.ParseRaw([]byte(line))
		if err != nil {
			t.Fatalf("ParseRaw: %v", err)
		}
		m.applyParsedEvent(evt)

		typ := evt.Name
		rawSSE.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", typ, line))
	}
	m.messages[0].RawSSE = rawSSE.String()
	m.messages[0].Streaming = false

	return &m
}
