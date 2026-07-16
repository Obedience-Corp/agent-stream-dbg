package mapping

import (
	"strings"
	"testing"
)

func TestLoad_Errors(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantErrSub string
	}{
		{
			name: "unsupported version",
			yaml: `
version: 2
name: test
discriminator: event
rules:
  - match: {event: x}
    kind: content
`,
			wantErrSub: "unsupported dialect version",
		},
		{
			name: "missing name",
			yaml: `
version: 1
discriminator: event
rules:
  - match: {event: x}
    kind: content
`,
			wantErrSub: "must set name",
		},
		{
			name: "missing discriminator",
			yaml: `
version: 1
name: test
rules:
  - match: {event: x}
    kind: content
`,
			wantErrSub: "must set discriminator",
		},
		{
			name: "no rules",
			yaml: `
version: 1
name: test
discriminator: event
rules: []
`,
			wantErrSub: "at least one rule",
		},
		{
			name: "unknown kind",
			yaml: `
version: 1
name: test
discriminator: event
rules:
  - match: {event: x}
    kind: not_a_real_kind
`,
			wantErrSub: "unknown kind",
		},
		{
			name: "match with no criteria",
			yaml: `
version: 1
name: test
discriminator: event
rules:
  - match: {}
    kind: content
`,
			wantErrSub: "at least one of event, path, exists, or raw",
		},
		{
			name: "path without equals",
			yaml: `
version: 1
name: test
discriminator: event
rules:
  - match: {path: kind}
    kind: content
`,
			wantErrSub: "match.path requires match.equals",
		},
		{
			name: "bad glob pattern",
			yaml: `
version: 1
name: test
discriminator: event
rules:
  - match: {event: "[unterminated"}
    kind: content
`,
			wantErrSub: "invalid event glob",
		},
		{
			name: "unknown top-level key",
			yaml: `
version: 1
name: test
discriminator: event
extra_key: oops
rules:
  - match: {event: x}
    kind: content
`,
			wantErrSub: "extra_key",
		},
		{
			name: "unknown rule key",
			yaml: `
version: 1
name: test
discriminator: event
rules:
  - match: {event: x}
    kind: content
    bogus: field
`,
			wantErrSub: "bogus",
		},
		{
			name: "unknown discriminator path key",
			yaml: `
version: 1
name: test
discriminator: {paht: type}
rules:
  - match: {event: x}
    kind: content
`,
			wantErrSub: "invalid discriminator",
		},
		{
			name: "invalid discriminator scalar",
			yaml: `
version: 1
name: test
discriminator: bogus
rules:
  - match: {event: x}
    kind: content
`,
			wantErrSub: "invalid discriminator",
		},
		{
			name: "malformed yaml",
			yaml: `
version: 1
name: test
 discriminator: event
`,
			wantErrSub: "failed to parse dialect",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load([]byte(tt.yaml))
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErrSub, err)
			}
		})
	}
}

func TestLoad_ValidDialect(t *testing.T) {
	engine, err := Load([]byte(`
version: 1
name: test-dialect
description: a test dialect
discriminator: event
rules:
  - match: {event: agent_content}
    kind: content
    source: agent_id
    content: content
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if engine.Name != "test-dialect" {
		t.Errorf("expected name 'test-dialect', got %q", engine.Name)
	}
	if len(engine.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(engine.Rules))
	}
	if engine.Rules[0].Kind != "content" {
		t.Errorf("expected kind 'content', got %q", engine.Rules[0].Kind)
	}
}

func TestDiscriminator_Resolve(t *testing.T) {
	tests := []struct {
		name          string
		discriminator string
		transportName string
		data          string
		want          string
	}{
		{
			name:          "event mode uses transport name",
			discriminator: `event`,
			transportName: "agent_content",
			data:          `{"type":"ignored"}`,
			want:          "agent_content",
		},
		{
			name:          "path mode reads from payload",
			discriminator: `{path: type}`,
			transportName: "ignored-by-path-mode",
			data:          `{"type":"content_block_delta"}`,
			want:          "content_block_delta",
		},
		{
			name:          "path mode missing field yields empty",
			discriminator: `{path: type}`,
			transportName: "irrelevant",
			data:          `{"other":"value"}`,
			want:          "",
		},
		{
			name:          "auto mode always yields empty",
			discriminator: `auto`,
			transportName: "agent_content",
			data:          `{"type":"agent_content"}`,
			want:          "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlDoc := "version: 1\nname: test\ndiscriminator: " + tt.discriminator + "\nrules:\n  - match: {exists: irrelevant}\n    kind: unknown\n"
			engine, err := Load([]byte(yamlDoc))
			if err != nil {
				t.Fatalf("unexpected load error: %v", err)
			}
			got := engine.ResolveName(tt.transportName, []byte(tt.data))
			if got != tt.want {
				t.Errorf("expected resolved name %q, got %q", tt.want, got)
			}
		})
	}
}

func TestEngine_Match_AllForms(t *testing.T) {
	engine, err := Load([]byte(`
version: 1
name: test
discriminator: event
rules:
  - match: {event: session_start}
    kind: session_start
  - match: {event: [agent_content, synthesis_delta]}
    kind: content
  - match: {event: "agent_*"}
    kind: stream_start
  - match: {path: kind, equals: status-update}
    kind: status_change
  - match: {exists: choices.0.delta.content}
    kind: content
  - match: {raw: "[DONE]"}
    kind: session_end
  - match: {event: content_block_delta, exists: delta.text}
    kind: content
`))
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}

	tests := []struct {
		name      string
		frameName string
		data      string
		wantIdx   int
	}{
		{"exact event match", "session_start", `{}`, 0},
		{"event list match - first", "agent_content", `{}`, 1},
		{"event list match - second", "synthesis_delta", `{}`, 1},
		{"glob event match", "agent_stream_start", `{}`, 2},
		{"path equals match", "unrelated", `{"kind":"status-update"}`, 3},
		{"path equals mismatch", "unrelated", `{"kind":"other"}`, -1},
		{"exists match", "unrelated", `{"choices":[{"delta":{"content":"hi"}}]}`, 4},
		{"exists no match", "unrelated", `{"choices":[{"delta":{}}]}`, -1},
		{"raw literal match", "unrelated", `[DONE]`, 5},
		{"raw literal mismatch", "unrelated", `[NOT DONE]`, -1},
		{"AND combo: event+exists both true", "content_block_delta", `{"delta":{"text":"hi"}}`, 6},
		{"AND combo: event true, exists false", "content_block_delta", `{"delta":{}}`, -1},
		{"AND combo: event false, exists true", "other_event", `{"delta":{"text":"hi"}}`, -1},
		{"no rule matches", "totally_unknown", `{}`, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.Match(tt.frameName, []byte(tt.data))
			if got != tt.wantIdx {
				t.Errorf("expected match index %d, got %d", tt.wantIdx, got)
			}
		})
	}
}

func TestEngine_Match_FirstRuleWinsOnOrdering(t *testing.T) {
	// Two rules could both match "agent_content"; the first in document
	// order must win, regardless of which is "more specific".
	engine, err := Load([]byte(`
version: 1
name: test
discriminator: event
rules:
  - match: {event: "agent_*"}
    kind: stream_start
  - match: {event: agent_content}
    kind: content
`))
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}

	idx := engine.Match("agent_content", []byte(`{}`))
	if idx != 0 {
		t.Errorf("expected first matching rule (index 0) to win, got %d", idx)
	}
	if engine.Rules[idx].Kind != "stream_start" {
		t.Errorf("expected kind from first rule 'stream_start', got %q", engine.Rules[idx].Kind)
	}
}

func TestEngine_Match_AutoDiscriminatorStructuralOnly(t *testing.T) {
	// Under auto discriminator, frame name is always empty, so only
	// structural (path/exists/raw) match forms can ever fire.
	engine, err := Load([]byte(`
version: 1
name: test
discriminator: auto
rules:
  - match: {exists: choices.0.delta.content}
    kind: content
  - match: {raw: "[DONE]"}
    kind: session_end
`))
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}

	name := engine.ResolveName("some-transport-name", []byte(`{"choices":[{"delta":{"content":"hi"}}]}`))
	if name != "" {
		t.Fatalf("expected auto discriminator to always resolve empty name, got %q", name)
	}

	idx := engine.Match(name, []byte(`{"choices":[{"delta":{"content":"hi"}}]}`))
	if idx != 0 {
		t.Errorf("expected structural match on index 0, got %d", idx)
	}
}
