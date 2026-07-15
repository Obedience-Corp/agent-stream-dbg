package mapping

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidatePlaceholders_UnknownPlaceholderErrors(t *testing.T) {
	tests := []struct {
		name     string
		template string
		vars     InterpolationVars
	}{
		{
			name:     "typo'd fixed placeholder",
			template: `{"url": "{bas_url}/stream"}`,
			vars:     InterpolationVars{BaseURL: "http://x"},
		},
		{
			name:     "undeclared var",
			template: `{"deployment": "{deployment_id}"}`,
			vars:     InterpolationVars{Vars: map[string]string{"region": "us-west"}},
		},
		{
			name:     "env-var-shaped placeholder is not magically available",
			template: `{"key": "{API_KEY}"}`,
			vars:     InterpolationVars{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidatePlaceholders(tt.template, tt.vars); err == nil {
				t.Fatalf("expected error for unknown placeholder, got nil")
			}
			if _, err := Interpolate(tt.template, tt.vars); err == nil {
				t.Errorf("expected Interpolate to also reject unknown placeholder")
			}
		})
	}
}

func TestValidatePlaceholders_KnownPlaceholdersOK(t *testing.T) {
	template := `{"base_url":"{base_url}","session_id":"{session_id}","message":"{message}","agents":{agents},"region":"{region}"}`
	vars := InterpolationVars{
		BaseURL:   "http://localhost:5003",
		SessionID: "s1",
		Message:   "hello",
		Agents:    []string{"a1", "a2"},
		Vars:      map[string]string{"region": "us-west"},
	}
	if err := ValidatePlaceholders(template, vars); err != nil {
		t.Errorf("unexpected error for all-known placeholders: %v", err)
	}
}

func TestInterpolate_EachPlaceholder(t *testing.T) {
	vars := InterpolationVars{
		BaseURL:   "http://localhost:5003",
		SessionID: "session-abc",
		Message:   "hello world",
		Agents:    []string{"a1", "a2"},
		Vars:      map[string]string{"region": "us-west"},
	}

	tests := []struct {
		name     string
		template string
		want     string
	}{
		{"base_url", `{base_url}/api`, "http://localhost:5003/api"},
		{"session_id", `/sessions/{session_id}`, "/sessions/session-abc"},
		{"message in quotes", `"{message}"`, `"hello world"`},
		{"user var", `region={region}`, "region=us-west"},
		{"multiple placeholders", `{base_url}/sessions/{session_id}`, "http://localhost:5003/sessions/session-abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Interpolate(tt.template, vars)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestInterpolate_AgentsRendersAsJSONArray(t *testing.T) {
	vars := InterpolationVars{Agents: []string{"a1", "a2", "a3"}}

	got, err := Interpolate(`{"agents": {agents}, "reuse_existing": true}`, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := `{"agents": ["a1","a2","a3"], "reuse_existing": true}`
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}

	// And the result must actually be valid, parseable JSON with a real array.
	var parsed struct {
		Agents []string `json:"agents"`
	}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("rendered body is not valid JSON: %v", err)
	}
	if len(parsed.Agents) != 3 {
		t.Errorf("expected 3 agents, got %d", len(parsed.Agents))
	}
}

func TestInterpolate_AgentsEmptyList(t *testing.T) {
	vars := InterpolationVars{Agents: []string{}}
	got, err := Interpolate(`{"agents": {agents}}`, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"agents": []}` {
		t.Errorf("expected empty JSON array, got %q", got)
	}
}

func TestInterpolate_InjectionAttemptsStayEscaped(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{"embedded quote", `hello "world"`},
		{"json-breakout attempt", `", "evil_key": "evil_value`},
		{"backslash", `C:\path\to\file`},
		{"newline", "line one\nline two"},
		{"control characters", "tab\there"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := InterpolationVars{Message: tt.message}
			rendered, err := InterpolateJSON(`{"message": "{message}", "stream": true}`, vars)
			if err != nil {
				t.Fatalf("expected valid JSON after interpolation, got error: %v", err)
			}

			var parsed struct {
				Message string `json:"message"`
				Stream  bool   `json:"stream"`
			}
			if err := json.Unmarshal([]byte(rendered), &parsed); err != nil {
				t.Fatalf("rendered body is not valid JSON: %v\nrendered: %s", err, rendered)
			}
			if parsed.Message != tt.message {
				t.Errorf("expected message to round-trip exactly, got %q want %q", parsed.Message, tt.message)
			}
			if !parsed.Stream {
				t.Error("expected sibling JSON key 'stream' to survive interpolation untouched")
			}
		})
	}
}

func TestInterpolateJSON_MalformedTemplateErrors(t *testing.T) {
	// A template that isn't valid JSON even before interpolation must fail
	// InterpolateJSON's re-validation.
	_, err := InterpolateJSON(`{"message": {message}, not valid json`, InterpolationVars{Message: "hi"})
	if err == nil {
		t.Fatal("expected error for malformed template, got nil")
	}
	if !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("expected 'not valid JSON' error, got: %v", err)
	}
}

func TestInterpolate_URLTemplateNotJSON(t *testing.T) {
	// Interpolate (not InterpolateJSON) must work fine on non-JSON templates
	// like URLs — only InterpolateJSON enforces JSON validity.
	vars := InterpolationVars{BaseURL: "http://localhost:5003", SessionID: "s1"}
	got, err := Interpolate(`{base_url}/api/v3/sessions/{session_id}/stream`, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "http://localhost:5003/api/v3/sessions/s1/stream"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}
