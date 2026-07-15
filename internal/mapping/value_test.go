package mapping

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func unmarshalValueForm(t *testing.T, yamlSnippet string) ValueForm {
	t.Helper()
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yamlSnippet), &node); err != nil {
		t.Fatalf("failed to parse yaml snippet: %v", err)
	}
	// Document node wraps the actual content node.
	content := &node
	if node.Kind == yaml.DocumentNode {
		content = node.Content[0]
	}
	var vf ValueForm
	if err := vf.UnmarshalYAML(content); err != nil {
		t.Fatalf("failed to unmarshal value form: %v", err)
	}
	return vf
}

func TestValueForm_UnmarshalErrors(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{"empty mapping", `{}`},
		{"unknown key", `{bogus: true}`},
		{"raw false is not a declaration", `{raw: false}`},
		{"sequence node", `[a, b]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tt.yaml), &node); err != nil {
				t.Fatalf("failed to parse yaml: %v", err)
			}
			content := &node
			if node.Kind == yaml.DocumentNode {
				content = node.Content[0]
			}
			var vf ValueForm
			if err := vf.UnmarshalYAML(content); err == nil {
				t.Errorf("expected error for %q, got nil", tt.yaml)
			}
		})
	}
}

func TestValueForm_BarePath(t *testing.T) {
	vf := unmarshalValueForm(t, `agent_id`)

	val, ok := vf.Extract([]byte(`{"agent_id":"a1"}`))
	if !ok || val != "a1" {
		t.Errorf("expected 'a1', got %v (ok=%v)", val, ok)
	}

	// Missing path is a soft failure, never an error.
	val, ok = vf.Extract([]byte(`{"other":"x"}`))
	if ok || val != nil {
		t.Errorf("expected no value for missing path, got %v (ok=%v)", val, ok)
	}
	if s := vf.String([]byte(`{"other":"x"}`)); s != "" {
		t.Errorf("expected empty string for missing path, got %q", s)
	}
}

func TestValueForm_Const(t *testing.T) {
	vf := unmarshalValueForm(t, `{const: synthesizer}`)

	// const ignores the frame body entirely — this is how a pseudo-agent
	// (a system's synthesis/aggregator lane with no real agent ID) gets declared.
	for _, data := range []string{`{}`, `not even json`, ``} {
		if s := vf.String([]byte(data)); s != "synthesizer" {
			t.Errorf("expected const 'synthesizer' regardless of body, got %q for body %q", s, data)
		}
	}
}

func TestValueForm_PathWithDefault(t *testing.T) {
	vf := unmarshalValueForm(t, `{path: agent_id, default: unknown}`)

	if s := vf.String([]byte(`{"agent_id":"a1"}`)); s != "a1" {
		t.Errorf("expected 'a1' when path present, got %q", s)
	}
	if s := vf.String([]byte(`{}`)); s != "unknown" {
		t.Errorf("expected default 'unknown' when path missing, got %q", s)
	}
}

func TestValueForm_PathNoDefaultMissing(t *testing.T) {
	vf := unmarshalValueForm(t, `{path: agent_id}`)

	val, ok := vf.Extract([]byte(`{}`))
	if ok || val != nil {
		t.Errorf("expected soft failure (nil, false) for missing path with no default, got %v (ok=%v)", val, ok)
	}
}

func TestValueForm_First(t *testing.T) {
	vf := unmarshalValueForm(t, `{first: [agent_id, source, name]}`)

	tests := []struct {
		name string
		data string
		want string
	}{
		{"first path present", `{"agent_id":"a1","source":"s1"}`, "a1"},
		{"first missing, second present", `{"source":"s1","name":"n1"}`, "s1"},
		{"only last present", `{"name":"n1"}`, "n1"},
		{"none present", `{}`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if s := vf.String([]byte(tt.data)); s != tt.want {
				t.Errorf("expected %q, got %q", tt.want, s)
			}
		})
	}
}

func TestValueForm_Raw(t *testing.T) {
	vf := unmarshalValueForm(t, `{raw: true}`)

	tests := []struct {
		name string
		data string
		want string
	}{
		{"plain JSON body", `{"foo":"bar"}`, `{"foo":"bar"}`},
		{"bare token, not JSON", `Hello`, "Hello"},
		{"literal DONE marker", `[DONE]`, "[DONE]"},
		{"whitespace trimmed", "  Hello  \n", "Hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if s := vf.String([]byte(tt.data)); s != tt.want {
				t.Errorf("expected %q, got %q", tt.want, s)
			}
		})
	}
}

func TestValueForm_Int(t *testing.T) {
	vf := unmarshalValueForm(t, `sequence`)

	if n := vf.Int([]byte(`{"sequence":42}`)); n != 42 {
		t.Errorf("expected 42, got %d", n)
	}
	// Missing path yields zero value, not an error.
	if n := vf.Int([]byte(`{}`)); n != 0 {
		t.Errorf("expected 0 for missing path, got %d", n)
	}
}

func TestValueForm_Unset(t *testing.T) {
	var vf ValueForm // zero value: field was omitted from the rule entirely

	if vf.IsSet() {
		t.Error("expected zero-value ValueForm to report unset")
	}
	if val, ok := vf.Extract([]byte(`{"anything":"here"}`)); ok || val != nil {
		t.Errorf("expected unset value form to extract nothing, got %v (ok=%v)", val, ok)
	}
}

func TestRule_FieldsMapExtractsArbitraryKeys(t *testing.T) {
	engine, err := Load([]byte(`
version: 1
name: test
discriminator: event
rules:
  - match: {event: agent_metadata}
    kind: usage
    fields:
      model: model
      tokens: total_tokens
      pseudo_agent: {const: synthesizer}
`))
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}

	rule := engine.Rules[0]
	data := []byte(`{"model":"gpt-5","total_tokens":123}`)

	if s := rule.Fields["model"].String(data); s != "gpt-5" {
		t.Errorf("expected fields.model 'gpt-5', got %q", s)
	}
	if n := rule.Fields["tokens"].Int(data); n != 123 {
		t.Errorf("expected fields.tokens 123, got %d", n)
	}
	if s := rule.Fields["pseudo_agent"].String(data); s != "synthesizer" {
		t.Errorf("expected fields.pseudo_agent 'synthesizer' (const), got %q", s)
	}
}

func TestValueForm_Describe(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"bare path", `agent_id`, "agent_id"},
		{"const", `{const: synthesizer}`, "const: synthesizer"},
		{"path with default", `{path: model, default: unknown}`, "model (default: unknown)"},
		{"path without default", `{path: model}`, "model"},
		{"first", `{first: [a, b]}`, "first: [a b]"},
		{"raw", `{raw: true}`, "raw"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vf := unmarshalValueForm(t, tt.yaml)
			if got := vf.Describe(); got != tt.want {
				t.Errorf("expected Describe() %q, got %q", tt.want, got)
			}
		})
	}

	var unset ValueForm
	if got := unset.Describe(); got != "" {
		t.Errorf("expected empty Describe() for unset ValueForm, got %q", got)
	}
}
