package config

import (
	"strings"
	"testing"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/mapping"
)

func TestInterpolationVarsFromConfigRoundTrip(t *testing.T) {
	cfg := &EnhancedConfig{
		Transport: TransportConfig{BaseURL: "https://api.example.test"},
		Session:   SessionConfig{ID: "session-1", DefaultAgents: []string{"agent-a"}},
		Vars:      map[string]string{"region": "us-west", "tier": "gold"},
	}
	vars := InterpolationVarsFromConfig(cfg)

	got, err := mapping.InterpolateURL("{base_url}/{region}/{tier}/{session_id}", vars)
	if err != nil {
		t.Fatalf("InterpolateURL: %v", err)
	}
	want := "https://api.example.test/us-west/gold/session-1"
	if got != want {
		t.Fatalf("rendered config vars = %q, want %q", got, want)
	}

	vars.Vars["region"] = "mutated-copy"
	if cfg.Vars["region"] != "us-west" {
		t.Fatal("interpolation context must not alias cfg.Vars")
	}

	_, err = mapping.InterpolateURL("{undeclared}", vars)
	if err == nil || !strings.Contains(err.Error(), "undeclared") {
		t.Fatalf("expected clear undeclared-placeholder error, got %v", err)
	}
}

func TestInterpolationVarsFromConfigProtectsBuiltIns(t *testing.T) {
	cfg := &EnhancedConfig{
		Transport: TransportConfig{BaseURL: "https://safe.example.test"},
		Session:   SessionConfig{ID: "safe-session"},
		Vars: map[string]string{
			"base_url":   "https://attacker.example.test",
			"session_id": "attacker-session",
			"message":    "attacker-message",
			"custom":     "allowed",
		},
	}
	vars := InterpolationVarsFromConfig(cfg)

	got, err := mapping.InterpolateURL("{base_url}/{session_id}/{message}/{custom}", func() mapping.InterpolationVars {
		vars.Message = "safe-message"
		return vars
	}())
	if err != nil {
		t.Fatalf("InterpolateURL: %v", err)
	}
	want := "https://safe.example.test/safe-session/safe-message/allowed"
	if got != want {
		t.Fatalf("built-in precedence result = %q, want %q", got, want)
	}
}
