// Package dialects_test decode-verifies the shipped dialect YAML files
// against hand-authored fixtures. It lives outside internal/mapping
// deliberately: the engine package must stay system-agnostic (see its grep
// gate), so per-dialect, system-named tests belong here instead, next to
// the data files they verify.
package dialects_test

import (
	"bufio"
	"os"
	"testing"

	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/mapping"
)

// TestOpenAIDialect_ExactDecode is the openai.yaml wire shape's acid test:
// no event names at all (discriminator: auto), matched purely by JSON
// structure. Every line of the hand-authored fixture is checked against its
// exact expected Kind/SourceID/Content/Fields, including two deliberately
// unmapped frames (role-only chunk, finish_reason-only trailer) that must
// decode as Kind=Unknown rather than error or vanish.
func TestOpenAIDialect_ExactDecode(t *testing.T) {
	engine, err := mapping.LoadFile("openai.yaml")
	if err != nil {
		t.Fatalf("failed to load dialect: %v", err)
	}

	type want struct {
		kind    events.Kind
		source  string
		content string
		fields  map[string]any
	}
	expected := []want{
		{events.KindUnknown, "", "", nil},                                                              // 1: role-only chunk, deliberately unmapped
		{events.KindContent, "assistant", "The", nil},                                                  // 2: content delta
		{events.KindContent, "assistant", " answer is 42.", nil},                                       // 3: content delta
		{events.KindToolCall, "assistant", "", map[string]any{"tool_name": "get_weather", "args": ""}}, // 4: tool_calls delta (first, with name)
		{events.KindToolCall, "assistant", "", map[string]any{"args": `{"location":"Denver"}`}},        // 5: tool_calls delta (fragment, no name)
		{events.KindUnknown, "", "", nil},                                                              // 6: finish_reason-only trailer, deliberately unmapped
		{events.KindUsage, "", "", map[string]any{"tokens": float64(27), "model": "gpt-4o-mini"}},      // 7: usage trailer
		{events.KindSessionEnd, "", "", nil},                                                           // 8: [DONE] sentinel
	}

	f, err := os.Open("../testdata/fixtures/openai-chat.jsonl")
	if err != nil {
		t.Fatalf("failed to open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

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

		evt := engine.Decode("", line)
		if evt.Kind != w.kind {
			t.Errorf("line %d: expected kind %v, got %v", lineNum+1, w.kind, evt.Kind)
		}
		if evt.SourceID != w.source {
			t.Errorf("line %d: expected source %q, got %q", lineNum+1, w.source, evt.SourceID)
		}
		if evt.Content != w.content {
			t.Errorf("line %d: expected content %q, got %q", lineNum+1, w.content, evt.Content)
		}
		for key, val := range w.fields {
			if evt.Fields[key] != val {
				t.Errorf("line %d: expected fields[%q] = %v, got %v", lineNum+1, key, val, evt.Fields[key])
			}
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

// TestAnthropicDialect_ExactDecode is the anthropic.yaml wire shape's acid
// test: the frame name lives in the payload's "type" field
// (discriminator: {path: type}), not a transport-supplied name. Every line
// of the hand-authored fixture is checked against its exact expected
// Kind/SourceID/Content/Fields, including three deliberately unmapped
// frames (content_block_start/stop, signature_delta) that must decode as
// Kind=Unknown.
func TestAnthropicDialect_ExactDecode(t *testing.T) {
	engine, err := mapping.LoadFile("anthropic.yaml")
	if err != nil {
		t.Fatalf("failed to load dialect: %v", err)
	}

	type want struct {
		kind    events.Kind
		source  string
		content string
		fields  map[string]any
	}
	expected := []want{
		{events.KindSessionStart, "", "", map[string]any{"model": "claude-opus-4-6-20260115"}}, // 1: message_start
		{events.KindUnknown, "", "", nil},                                 // 2: content_block_start (thinking), deliberately unmapped
		{events.KindReasoning, "assistant", "Let me consider this.", nil}, // 3: thinking_delta
		{events.KindUnknown, "", "", nil},                                 // 4: signature_delta, deliberately unmapped
		{events.KindUnknown, "", "", nil},                                 // 5: content_block_stop, deliberately unmapped
		{events.KindUnknown, "", "", nil},                                 // 6: content_block_start (text), deliberately unmapped
		{events.KindContent, "assistant", "The answer is 42.", nil},       // 7: text_delta
		{events.KindUnknown, "", "", nil},                                 // 8: content_block_stop, deliberately unmapped
		{events.KindUsage, "", "", map[string]any{"tokens": float64(24)}}, // 9: message_delta usage
		{events.KindSessionEnd, "", "", nil},                              // 10: message_stop
	}

	f, err := os.Open("../testdata/fixtures/anthropic-messages.jsonl")
	if err != nil {
		t.Fatalf("failed to open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

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

		evt := engine.Decode("", line)
		if evt.Kind != w.kind {
			t.Errorf("line %d: expected kind %v, got %v", lineNum+1, w.kind, evt.Kind)
		}
		if evt.SourceID != w.source {
			t.Errorf("line %d: expected source %q, got %q", lineNum+1, w.source, evt.SourceID)
		}
		if evt.Content != w.content {
			t.Errorf("line %d: expected content %q, got %q", lineNum+1, w.content, evt.Content)
		}
		for key, val := range w.fields {
			if evt.Fields[key] != val {
				t.Errorf("line %d: expected fields[%q] = %v, got %v", lineNum+1, key, val, evt.Fields[key])
			}
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
