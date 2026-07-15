package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeYAMLConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadConfigFile_StrictUnknownFields(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantErrSub []string
	}{
		{
			name: "typo'd top-level key",
			yaml: `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      token_env: "API_KEY"
sesion:
  id_env: "SESSION_ID"
`,
			wantErrSub: []string{"sesion"},
		},
		{
			name: "typo'd nested key",
			yaml: `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      typ: "bearer"
      token_env: "API_KEY"
`,
			wantErrSub: []string{"typ"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv("API_KEY", "test-key")
			defer func() { _ = os.Unsetenv("API_KEY") }()

			path := writeYAMLConfig(t, tt.yaml)
			_, err := LoadConfigFile(path)
			if err == nil {
				t.Fatalf("expected error for unknown key, got nil")
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("expected error to name the config file %q, got: %v", path, err)
			}
			for _, sub := range tt.wantErrSub {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("expected error to mention offending key %q, got: %v", sub, err)
				}
			}
		})
	}
}

func TestLoadConfigFile_MalformedYAML(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "bad indentation",
			yaml: `
transport:
  base_url: "http://localhost:8080"
 stream_endpoint:
    url: "/api/stream"
`,
		},
		{
			name: "unterminated flow sequence",
			yaml: `
transport:
  base_url: "http://localhost:8080"
session:
  default_agents: [a, b
`,
		},
		{
			name: "not a mapping at all",
			yaml: `- just
- a
- list
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeYAMLConfig(t, tt.yaml)
			_, err := LoadConfigFile(path)
			if err == nil {
				t.Fatalf("expected error for malformed YAML, got nil")
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("expected error to name the config file %q, got: %v", path, err)
			}
		})
	}
}

func TestLoadConfigFile_ValidConfigLoads(t *testing.T) {
	_ = os.Setenv("API_KEY", "test-key")
	defer func() { _ = os.Unsetenv("API_KEY") }()

	path := writeYAMLConfig(t, `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    method: "POST"
    auth:
      type: "bearer"
      token_env: "API_KEY"
session:
  id_env: "SESSION_ID"
`)

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error loading valid config: %v", err)
	}
	if cfg.Transport.BaseURL != "http://localhost:8080" {
		t.Errorf("expected base_url to load, got %q", cfg.Transport.BaseURL)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("expected APIKey from env, got %q", cfg.APIKey)
	}
}

func TestLoadConfigFile_DialectFileReference(t *testing.T) {
	_ = os.Setenv("API_KEY", "test-key")
	defer func() { _ = os.Unsetenv("API_KEY") }()

	path := writeYAMLConfig(t, `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      token_env: "API_KEY"
dialect:
  file: "dialects/brainyard.yaml"
`)

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Dialect.File != "dialects/brainyard.yaml" {
		t.Errorf("expected dialect.file to be stored, got %q", cfg.Dialect.File)
	}
}

func TestLoadConfigFile_VarsResolution(t *testing.T) {
	_ = os.Setenv("API_KEY", "test-key")
	defer func() { _ = os.Unsetenv("API_KEY") }()

	path := writeYAMLConfig(t, `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      token_env: "API_KEY"
vars:
  region: "us-west"
  tier: "premium"
`)

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Vars["region"] != "us-west" || cfg.Vars["tier"] != "premium" {
		t.Errorf("expected vars to be passed through, got %+v", cfg.Vars)
	}
}
