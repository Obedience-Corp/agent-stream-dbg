package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListConfigsEmpty(t *testing.T) {
	entries := ListConfigs(t.TempDir(), t.TempDir())
	if len(entries) != 0 {
		t.Fatalf("got %d entries, want 0", len(entries))
	}
}

func TestListConfigsFindsCwdAndUser(t *testing.T) {
	cwd := t.TempDir()
	user := t.TempDir()
	ready := `transport:
  type: sse
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/stream"
    method: GET
    auth:
      type: none
dialect:
  file: brainyard
session:
  auto_setup: false
logging:
  dir: ./logs
`
	incomplete := `transport:
  type: sse
  base_url: ""
  stream_endpoint:
    url: ""
dialect:
  file: ""
`
	if err := os.WriteFile(filepath.Join(cwd, "config.yaml"), []byte(ready), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(user, "starter.yaml"), []byte(incomplete), 0o600); err != nil {
		t.Fatal(err)
	}

	entries := ListConfigs(cwd, user)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	// ready first
	if entries[0].Status != EntryReady {
		t.Fatalf("first entry status = %s, want ready", entries[0].Status)
	}
	if entries[1].Status != EntryNeedsSetup {
		t.Fatalf("second entry status = %s, want needs setup", entries[1].Status)
	}
}

func TestListConfigsAuthNeededNotError(t *testing.T) {
	_ = os.Unsetenv("LIST_AUTH_NEEDED_KEY")
	cwd := t.TempDir()
	body := `transport:
  type: sse
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/stream"
    auth:
      type: bearer
      token_env: LIST_AUTH_NEEDED_KEY
dialect:
  file: brainyard
logging:
  dir: ./logs
`
	if err := os.WriteFile(filepath.Join(cwd, "config.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	entries := ListConfigs(cwd, t.TempDir())
	if len(entries) != 1 {
		t.Fatalf("got %d entries", len(entries))
	}
	if entries[0].Status != EntryAuthNeeded {
		t.Fatalf("status = %s, want auth needed (err=%q)", entries[0].Status, entries[0].Err)
	}
	if entries[0].Transport != "sse" {
		t.Fatalf("transport = %q", entries[0].Transport)
	}
	if !strings.Contains(entries[0].Err, "LIST_AUTH_NEEDED_KEY") || !strings.Contains(entries[0].Err, ".env") {
		t.Fatalf("err should explain .env setup, got %q", entries[0].Err)
	}
}

func TestDisplayPathRelativeAndUser(t *testing.T) {
	cwd := t.TempDir()
	path := filepath.Join(cwd, "configs", "brainyard-v3.yaml")
	if got := DisplayPath(path, cwd); got != filepath.Join("configs", "brainyard-v3.yaml") {
		t.Fatalf("relative display = %q", got)
	}
}

func TestConfigReady(t *testing.T) {
	if ConfigReady(nil) {
		t.Fatal("nil should not be ready")
	}
	if ConfigReady(&EnhancedConfig{Dialect: DialectConfig{File: "x"}}) {
		t.Fatal("empty SSE should not be ready")
	}
	ok := &EnhancedConfig{
		Dialect: DialectConfig{File: "brainyard"},
		Transport: TransportConfig{
			Type:           "sse",
			BaseURL:        "http://x",
			StreamEndpoint: "/s",
		},
	}
	if !ConfigReady(ok) {
		t.Fatal("expected ready")
	}
}

func TestSanitizeConfigName(t *testing.T) {
	if got := SanitizeConfigName("My Backend!"); got != "my-backend" {
		t.Fatalf("got %q", got)
	}
	if got := SanitizeConfigName("???"); got != "config" {
		t.Fatalf("got %q", got)
	}
}

func TestWriteNewConfigAndTemplate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.yaml")
	yaml := NewConfigYAML(TemplateSSE, "brainyard", "http://localhost:9", "/v1", "", "", "", "", "")
	if err := WriteNewConfig(path, yaml); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !ConfigReady(cfg) {
		t.Fatal("templated SSE config should be ready")
	}
	if err := WriteNewConfig(path, yaml); err == nil {
		t.Fatal("expected refuse overwrite")
	}
}
