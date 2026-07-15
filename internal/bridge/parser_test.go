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
