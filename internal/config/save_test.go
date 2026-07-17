package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveConfigFile_UpdatesRuntimeFieldsAndPreservesAuthConfig(t *testing.T) {
	t.Setenv("API_KEY", "test-key")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	original := `transport:
  type: sse
  base_url: https://old.example
  stream_endpoint:
    url: /old-stream
    auth:
      type: bearer
      token_env: API_KEY
dialect:
  file: dialects/reference.yaml
vars:
  old: value
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := &EnhancedConfig{
		Transport: TransportConfig{
			Type:           "sse",
			BaseURL:        "https://new.example",
			StreamEndpoint: "/stream",
		},
		Dialect: DialectConfig{File: "dialects/openai.yaml"},
		Vars:    map[string]string{"region": "us-west", "tier": "premium"},
	}
	if err := SaveConfigFile(path, cfg); err != nil {
		t.Fatalf("SaveConfigFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	saved := string(data)
	for _, want := range []string{
		"https://new.example",
		"/stream",
		"dialects/openai.yaml",
		"region: us-west",
		"token_env: API_KEY",
	} {
		if !strings.Contains(saved, want) {
			t.Errorf("saved config missing %q:\n%s", want, saved)
		}
	}
	if strings.Contains(saved, "test-key") {
		t.Error("saved config leaked the resolved API key")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat saved config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("expected permissions 0600 to be preserved, got %o", got)
	}
}

func TestSaveConfigFile_RequiresPathAndConfig(t *testing.T) {
	if err := SaveConfigFile("", &EnhancedConfig{}); err == nil {
		t.Error("expected empty path to fail")
	}
	if err := SaveConfigFile(filepath.Join(t.TempDir(), "config.yaml"), nil); err == nil {
		t.Error("expected nil config to fail")
	}
}
