package explain

import (
	"strings"
	"testing"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/mapping"
)

func testEngine(t *testing.T) *mapping.Engine {
	t.Helper()
	engine, err := mapping.Load([]byte(`
version: 1
name: test
discriminator: event
rules:
  - match: {event: agent_content}
    kind: content
    source: agent_id
    content: content
    seq: sequence
`))
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	return engine
}

func TestTrace_MatchedRule_ExtractsFields(t *testing.T) {
	engine := testEngine(t)
	data := []byte(`{"type":"agent_content","agent_id":"agent_a","content":"hello","sequence":3}`)

	trace := Trace(engine, "agent_content", data, 1)

	if trace.MatchedIdx != 0 {
		t.Fatalf("expected rule index 0, got %d", trace.MatchedIdx)
	}
	if trace.MatchDesc != "{event: agent_content}" {
		t.Errorf("expected match desc '{event: agent_content}', got %q", trace.MatchDesc)
	}
	if trace.Unhealthy() {
		t.Error("expected a fully-matched, non-empty trace to be healthy")
	}

	want := map[string]string{"source": "agent_a", "content": "hello", "seq": "3"}
	if len(trace.Extractions) != 3 {
		t.Fatalf("expected 3 extractions, got %d", len(trace.Extractions))
	}
	for _, e := range trace.Extractions {
		if !e.Found {
			t.Errorf("field %q: expected Found=true", e.Field)
		}
		if e.Empty {
			t.Errorf("field %q: expected Empty=false", e.Field)
		}
		if e.Value != want[e.Field] {
			t.Errorf("field %q: expected value %q, got %q", e.Field, want[e.Field], e.Value)
		}
	}

	rendered := trace.Render()
	if !strings.Contains(rendered, "✓ rules[0] matched {event: agent_content}") {
		t.Errorf("expected rendered trace to show the matched rule, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, `source  ← agent_id`) && !strings.Contains(rendered, `source ← agent_id`) {
		t.Errorf("expected rendered trace to show source ← agent_id, got:\n%s", rendered)
	}
}

func TestTrace_NoRuleMatched_ProducesHint(t *testing.T) {
	engine := testEngine(t)
	data := []byte(`{"type":"weird_event","foo":"bar"}`)

	trace := Trace(engine, "weird_event", data, 13)

	if trace.MatchedIdx != -1 {
		t.Fatalf("expected no match, got rule index %d", trace.MatchedIdx)
	}
	if !trace.Unhealthy() {
		t.Error("expected an unmatched frame to be unhealthy")
	}
	if trace.Hint != "add `- match: {event: weird_event}`" {
		t.Errorf("expected event-mode hint naming the event, got %q", trace.Hint)
	}

	rendered := trace.Render()
	if !strings.Contains(rendered, "✗ no rule matched") {
		t.Errorf("expected ✗ marker in render, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "hint: add `- match: {event: weird_event}`") {
		t.Errorf("expected hint in render, got:\n%s", rendered)
	}
}

// TestTrace_DeliberatelyBrokenContentPath_FlagsEmptyWarning is this task's
// explicit Done-When: a deliberately broken dialect (wrong content path)
// must produce the ⚠ warning pointing at the exact rule and path.
func TestTrace_DeliberatelyBrokenContentPath_FlagsEmptyWarning(t *testing.T) {
	engine, err := mapping.Load([]byte(`
version: 1
name: broken
discriminator: event
rules:
  - match: {event: agent_content}
    kind: content
    source: agent_id
    content: wrong_field
    seq: sequence
`))
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}

	// wrong_field is present but empty — content: should have pointed at
	// "content" instead, which holds the real text.
	data := []byte(`{"type":"agent_content","agent_id":"agent_a","content":"hello","wrong_field":"","sequence":3}`)

	trace := Trace(engine, "agent_content", data, 14)
	if !trace.Unhealthy() {
		t.Fatal("expected a broken content path to make the trace unhealthy")
	}

	var contentExtraction *Extraction
	for i := range trace.Extractions {
		if trace.Extractions[i].Field == "content" {
			contentExtraction = &trace.Extractions[i]
		}
	}
	if contentExtraction == nil {
		t.Fatal("expected a content extraction")
	}
	if contentExtraction.Path != "wrong_field" {
		t.Errorf("expected the trace to point at the exact declared path 'wrong_field', got %q", contentExtraction.Path)
	}
	if !contentExtraction.Empty {
		t.Error("expected the content extraction to be flagged Empty")
	}

	rendered := trace.Render()
	if !strings.Contains(rendered, "wrong_field") {
		t.Errorf("expected rendered trace to name the broken path 'wrong_field', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "⚠ empty — path may be wrong") {
		t.Errorf("expected the ⚠ empty warning in render, got:\n%s", rendered)
	}
}

func TestTrace_FieldNotFound_RendersDistinctly(t *testing.T) {
	engine, err := mapping.Load([]byte(`
version: 1
name: test
discriminator: event
rules:
  - match: {event: agent_content}
    kind: content
    source: agent_id
    content: content
    fields:
      missing: does_not_exist
`))
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	data := []byte(`{"type":"agent_content","agent_id":"agent_a","content":"hello"}`)

	trace := Trace(engine, "agent_content", data, 1)

	var missing *Extraction
	for i := range trace.Extractions {
		if trace.Extractions[i].Field == "missing" {
			missing = &trace.Extractions[i]
		}
	}
	if missing == nil {
		t.Fatal("expected a 'missing' extraction entry")
	}
	if missing.Found {
		t.Error("expected Found=false for a field with no data at this frame")
	}
	// A soft-missing optional field shouldn't, by itself, mark the whole
	// frame unhealthy — only an empty *resolved* value does.
	if trace.Unhealthy() {
		t.Error("expected a soft-missing optional field to stay healthy")
	}

	rendered := trace.Render()
	if !strings.Contains(rendered, "(not found)") {
		t.Errorf("expected '(not found)' marker in render, got:\n%s", rendered)
	}
}

func TestHintFor_VariesByDiscriminatorMode(t *testing.T) {
	pathEngine, err := mapping.Load([]byte(`
version: 1
name: test
discriminator: {path: type}
rules:
  - match: {event: known}
    kind: content
`))
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	trace := Trace(pathEngine, "unknown_type", []byte(`{"type":"unknown_type"}`), 1)
	if !strings.Contains(trace.Hint, "payload field") {
		t.Errorf("expected path-mode hint to mention the payload field, got %q", trace.Hint)
	}

	autoEngine, err := mapping.Load([]byte(`
version: 1
name: test
discriminator: auto
rules:
  - match: {exists: known_field}
    kind: content
`))
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	trace = Trace(autoEngine, "", []byte(`{"other_field":1}`), 1)
	if !strings.Contains(trace.Hint, "exists:") {
		t.Errorf("expected auto-mode hint to suggest an exists: match, got %q", trace.Hint)
	}
}
