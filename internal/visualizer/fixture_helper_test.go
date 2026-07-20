package visualizer

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/config"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
)

// aggregatorStageName is the stage/source ID declared as the aggregator by
// the loaded dialect. Tests use the semantic role rather than a wire-name
// constant.
var aggregatorStageName = func() string {
	for _, lane := range newFlowState().FlowModel().Lanes {
		if lane.Role == events.RoleAggregator {
			return lane.SourceID
		}
	}
	return ""
}()

func findAggregatorEvent(m InteractiveModel, evts []*events.Event, kind events.Kind) *events.Event {
	for _, evt := range evts {
		if evt.Kind == kind && m.dialectFlow.role(evt) == events.RoleAggregator {
			return evt
		}
	}
	return nil
}

func referenceFixturePath(t *testing.T) string {
	t.Helper()
	paths, err := filepath.Glob("../../testdata/fixtures/*-session.jsonl")
	if err != nil {
		t.Fatalf("find fixtures: %v", err)
	}
	parser := bridge.NewParser()
	flow := newFlowState()
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		found := false
		for scanner.Scan() {
			evt, err := parser.ParseRaw(scanner.Bytes())
			if err == nil && evt.Kind == events.KindStreamStart && flow.role(evt) == events.RoleAggregator {
				found = true
				break
			}
		}
		_ = f.Close()
		if found {
			return path
		}
	}
	t.Fatal("no fixture with a declared aggregator lane")
	return ""
}

// newFixtureModel builds an InteractiveModel with one completed turn driven
// entirely from the reference fixture via applyParsedEvent
// (the same live-stream incremental path fixture_parity_test.go exercises),
// plus the equivalent raw SSE text for that turn. Shared by the render/update
// tests added for the 007_VISUALIZER_FLOW_VIEW interactive.go file split, each
// of which needs a populated model rather than a zero-value one.
func newFixtureModel(t *testing.T) *InteractiveModel {
	t.Helper()

	f, err := os.Open(referenceFixturePath(t))
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
		_, _ = fmt.Fprintf(&rawSSE, "event: %s\ndata: %s\n\n", typ, line)
	}
	m.messages[0].RawSSE = rawSSE.String()
	m.messages[0].Streaming = false

	return &m
}
