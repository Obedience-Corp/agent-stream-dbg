package bridge

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lancekrogers/stream-debugger/dialects"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/testutil"
)

func TestLoadDialect_EmbeddedByName(t *testing.T) {
	engine, err := LoadDialect("openai")
	if err != nil {
		t.Fatalf("LoadDialect(openai): %v", err)
	}
	if engine.Name != "openai-chat" {
		t.Errorf("expected embedded OpenAI dialect, got %q", engine.Name)
	}
}

func TestLoadDialect_ExplicitPath(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "dialects", "openai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "custom.yaml")
	if err := os.WriteFile(path, source, 0o644); err != nil {
		t.Fatal(err)
	}
	engine, err := LoadDialect(path)
	if err != nil {
		t.Fatalf("LoadDialect(%s): %v", path, err)
	}
	if engine.Name != "openai-chat" {
		t.Errorf("expected path-loaded OpenAI dialect, got %q", engine.Name)
	}
}

func TestLoadDialect_MissingEmbeddedNameListsAvailable(t *testing.T) {
	_, err := LoadDialect("not-a-shipped-dialect")
	if err == nil {
		t.Fatal("expected missing embedded dialect error")
	}
	for _, name := range append([]string{"a2a", "anthropic", "openai"}, dialects.DefaultName) {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("missing dialect error does not list %q: %v", name, err)
		}
	}
}

func referenceFixturePath(t *testing.T) string {
	t.Helper()
	path, err := testutil.ReferenceSessionFixturePath("../../testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func referenceAggregatorSource(t *testing.T) string {
	t.Helper()
	flow := Flow()
	if flow == nil {
		t.Fatal("expected the selected dialect to declare flow metadata")
	}
	for _, lane := range flow.Lanes {
		if lane.Role == events.RoleAggregator {
			return lane.Match.Source
		}
	}
	t.Fatal("expected the selected dialect to declare an aggregator lane")
	return ""
}

// TestParser_FixtureCoverage feeds every line of the 002 demo fixture
// through the bridge parser and checks the decoded Kind/SourceID/Content
// against what each frame's wire "type" implies — proving the embedded
// dialect covers every event type recorded in a real session, matching the
// old hardcoded switch's behavior exactly.
func TestParser_FixtureCoverage(t *testing.T) {
	f, err := os.Open(referenceFixturePath(t))
	if err != nil {
		t.Fatalf("failed to open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	parser := NewParser()
	var (
		lineNum    int
		sawKinds   = map[events.Kind]int{}
		sawSources = map[string]int{}
	)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		evt, err := parser.ParseRaw(line)
		if err != nil {
			t.Fatalf("line %d: Parser must never error, got: %v", lineNum, err)
		}
		if evt == nil {
			t.Fatalf("line %d: Parser must never return nil", lineNum)
		}
		if evt.Kind == events.KindUnknown {
			t.Errorf("line %d: fixture frame decoded as Unknown (dialect gap): %s", lineNum, line)
		}
		sawKinds[evt.Kind]++
		if evt.SourceID != "" {
			sawSources[evt.SourceID]++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("error reading fixture: %v", err)
	}
	if lineNum != 22 {
		t.Fatalf("expected 22 lines in fixture, read %d (fixture changed? update this test)", lineNum)
	}

	wantKinds := []events.Kind{
		events.KindSessionStart, events.KindSessionEnd,
		events.KindStepStart, events.KindStepEnd, events.KindDetail,
		events.KindStreamStart, events.KindStreamEnd, events.KindContent,
		events.KindError,
	}
	for _, k := range wantKinds {
		if sawKinds[k] == 0 {
			t.Errorf("expected at least one %v event in fixture, saw none", k)
		}
	}

	if len(sawSources) != 3 {
		t.Errorf("expected three distinct event sources, got %v", sawSources)
	}
	if source := referenceAggregatorSource(t); sawSources[source] == 0 {
		t.Errorf("expected at least one event from the declared aggregator source %q", source)
	}
}

// TestParser_FixtureExactDecode is the acid test's decode-comparison test:
// every one of the 22 frames in the demo fixture, checked against its exact
// expected Kind/SourceID/Content/Seq — not just aggregate presence.
func TestParser_FixtureExactDecode(t *testing.T) {
	type want struct {
		kind    events.Kind
		content string
		seq     int
	}
	// Line order matches the selected reference session fixture exactly.
	expected := []want{
		{events.KindSessionStart, "", 0},
		{events.KindStepStart, "", 0},
		{events.KindStepEnd, "", 0},
		{events.KindStreamStart, "", 0},
		{events.KindStreamStart, "", 0},
		{events.KindContent, "Let's think about this together.", 1},
		{events.KindContent, "Here's another angle worth considering.", 1},
		{events.KindContent, " There are a few threads worth pulling on.", 2},
		{events.KindContent, " It helps to slow down and notice what's present.", 2},
		{events.KindError, "", 0},
		{events.KindContent, " Continuing from where we left off, the same idea still holds.", 3},
		{events.KindContent, " Taken together, this points toward a clear next step.", 3},
		{events.KindStreamEnd, "", 0},
		{events.KindStreamEnd, "", 0},
		{events.KindStepStart, "", 0},
		{events.KindDetail, "", 0},
		{events.KindStreamStart, "", 0},
		{events.KindContent, "Bringing both viewpoints together,", 1},
		{events.KindContent, " the synthesis suggests a balanced, grounded next step.", 2},
		{events.KindStreamEnd, "", 0},
		{events.KindStepEnd, "", 0},
		{events.KindSessionEnd, "", 0},
	}

	f, err := os.Open(referenceFixturePath(t))
	if err != nil {
		t.Fatalf("failed to open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	parser := NewParser()
	aggregatorSource := referenceAggregatorSource(t)
	var lineNum int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if lineNum >= len(expected) {
			t.Fatalf("fixture has more lines than expected[]; update this test")
		}
		w := expected[lineNum]

		evt, err := parser.ParseRaw(line)
		if err != nil {
			t.Fatalf("line %d: unexpected error: %v", lineNum+1, err)
		}
		if evt.Kind != w.kind {
			t.Errorf("line %d: expected kind %v, got %v", lineNum+1, w.kind, evt.Kind)
		}
		var wire struct {
			AgentID string `json:"agent_id"`
		}
		_ = json.Unmarshal(line, &wire)
		wantSource := ""
		switch evt.Kind {
		case events.KindStreamStart, events.KindStreamEnd, events.KindContent:
			wantSource = wire.AgentID
			if wantSource == "" {
				wantSource = aggregatorSource
			}
		}
		if evt.SourceID != wantSource {
			t.Errorf("line %d: expected source %q, got %q", lineNum+1, wantSource, evt.SourceID)
		}
		if evt.Content != w.content {
			t.Errorf("line %d: expected content %q, got %q", lineNum+1, w.content, evt.Content)
		}
		if evt.Seq != w.seq {
			t.Errorf("line %d: expected seq %d, got %d", lineNum+1, w.seq, evt.Seq)
		}
		lineNum++
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("error reading fixture: %v", err)
	}
	if lineNum != len(expected) {
		t.Fatalf("expected %d lines, read %d", len(expected), lineNum)
	}
}

func TestParser_NeverErrorsOnUnknownEventType(t *testing.T) {
	parser := NewParser()

	evt, err := parser.Parse("a_type_no_dialect_rule_knows_about", []byte(`{"type":"a_type_no_dialect_rule_knows_about","payload":"data"}`))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if evt.Kind != events.KindUnknown {
		t.Errorf("expected Kind %v for unrecognized event type, got %v", events.KindUnknown, evt.Kind)
	}
	if evt.Fields["payload"] != "data" {
		t.Errorf("expected best-effort Fields parse to survive, got %v", evt.Fields)
	}
}

func TestParser_NeverErrorsOnMalformedJSON(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name string
		data string
	}{
		{"garbage", "not json"},
		{"truncated", `{"type":"agent_content"`},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt, err := parser.Parse("agent_content", []byte(tt.data))
			if err != nil {
				t.Fatalf("expected no error for malformed JSON, got: %v", err)
			}
			if evt == nil {
				t.Fatal("expected non-nil event for malformed JSON")
			}
			if string(evt.Raw) != tt.data {
				t.Errorf("expected Raw preserved exactly, got %q", evt.Raw)
			}
		})
	}
}

func TestParser_DeclaredConstantSourceIsPreserved(t *testing.T) {
	parser := NewParser()

	path := referenceFixturePath(t)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var eventName string
	var data []byte
	var expectedContent string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var probe struct {
			Type    string `json:"type"`
			AgentID string `json:"agent_id"`
			Content string `json:"content"`
		}
		line := append([]byte(nil), scanner.Bytes()...)
		if json.Unmarshal(line, &probe) == nil && probe.Content != "" && probe.AgentID == "" {
			eventName, data, expectedContent = probe.Type, line, probe.Content
			break
		}
	}
	if eventName == "" {
		t.Fatal("expected a content frame with a declared constant source")
	}

	evt, err := parser.Parse(eventName, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.SourceID != referenceAggregatorSource(t) {
		t.Errorf("expected the declared constant source %q, got %q", referenceAggregatorSource(t), evt.SourceID)
	}
	if evt.Content != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, evt.Content)
	}
}
