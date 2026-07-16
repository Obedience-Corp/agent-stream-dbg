package bridge

import (
	"bufio"
	"os"
	"testing"

	"github.com/lancekrogers/stream-debugger/internal/events"
)

// TestParser_FixtureCoverage feeds every line of the 002 demo fixture
// through the bridge parser and checks the decoded Kind/SourceID/Content
// against what each frame's wire "type" implies — proving the embedded
// dialect covers every event type recorded in a real session, matching the
// old hardcoded switch's behavior exactly.
func TestParser_FixtureCoverage(t *testing.T) {
	f, err := os.Open("../../testdata/fixtures/brainyard-session.jsonl")
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

	wantSources := []string{"sam_harris", "eckhart_tolle", "wizard"}
	for _, s := range wantSources {
		if sawSources[s] == 0 {
			t.Errorf("expected at least one event from source %q, saw none", s)
		}
	}
}

// TestParser_FixtureExactDecode is the acid test's decode-comparison test:
// every one of the 22 frames in the demo fixture, checked against its exact
// expected Kind/SourceID/Content/Seq — not just aggregate presence.
func TestParser_FixtureExactDecode(t *testing.T) {
	type want struct {
		kind    events.Kind
		source  string
		content string
		seq     int
	}
	// Line order matches testdata/fixtures/brainyard-session.jsonl exactly.
	expected := []want{
		{events.KindSessionStart, "", "", 0},                                                                       // 1: session_start
		{events.KindStepStart, "", "", 0},                                                                          // 2: flow_step_start (routing)
		{events.KindStepEnd, "", "", 0},                                                                            // 3: flow_step_end (routing)
		{events.KindStreamStart, "sam_harris", "", 0},                                                              // 4: agent_stream_start
		{events.KindStreamStart, "eckhart_tolle", "", 0},                                                           // 5: agent_stream_start
		{events.KindContent, "sam_harris", "Let's think about this together.", 1},                                  // 6: agent_content
		{events.KindContent, "eckhart_tolle", "Here's another angle worth considering.", 1},                        // 7: agent_content
		{events.KindContent, "sam_harris", " There are a few threads worth pulling on.", 2},                        // 8: agent_content
		{events.KindContent, "eckhart_tolle", " It helps to slow down and notice what's present.", 2},              // 9: agent_content
		{events.KindError, "", "", 0},                                                                              // 10: error (dialect doesn't declare source: for error rule)
		{events.KindContent, "eckhart_tolle", " Continuing from where we left off, the same idea still holds.", 3}, // 11: agent_content
		{events.KindContent, "sam_harris", " Taken together, this points toward a clear next step.", 3},            // 12: agent_content
		{events.KindStreamEnd, "sam_harris", "", 0},                                                                // 13: agent_stream_complete
		{events.KindStreamEnd, "eckhart_tolle", "", 0},                                                             // 14: agent_stream_complete
		{events.KindStepStart, "", "", 0},                                                                          // 15: flow_step_start (synthesis)
		{events.KindDetail, "", "", 0},                                                                             // 16: flow_step_detail
		{events.KindStreamStart, "wizard", "", 0},                                                                  // 17: wizard_stream_start
		{events.KindContent, "wizard", "Bringing both viewpoints together,", 1},                                    // 18: wizard_content
		{events.KindContent, "wizard", " the synthesis suggests a balanced, grounded next step.", 2},               // 19: wizard_content
		{events.KindStreamEnd, "wizard", "", 0},                                                                    // 20: wizard_stream_complete
		{events.KindStepEnd, "", "", 0},                                                                            // 21: flow_step_end (synthesis)
		{events.KindSessionEnd, "", "", 0},                                                                         // 22: session_complete
	}

	f, err := os.Open("../../testdata/fixtures/brainyard-session.jsonl")
	if err != nil {
		t.Fatalf("failed to open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	parser := NewParser()
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
		if evt.SourceID != w.source {
			t.Errorf("line %d: expected source %q, got %q", lineNum+1, w.source, evt.SourceID)
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

func TestParser_WizardIsConstDeclaredPseudoAgent(t *testing.T) {
	parser := NewParser()

	evt, err := parser.Parse("wizard_content", []byte(`{"type":"wizard_content","content":"synthesis text","sequence":1}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.SourceID != "wizard" {
		t.Errorf("expected wizard events to have SourceID 'wizard' (const-declared pseudo-agent), got %q", evt.SourceID)
	}
	if evt.Content != "synthesis text" {
		t.Errorf("expected content extracted, got %q", evt.Content)
	}
}
