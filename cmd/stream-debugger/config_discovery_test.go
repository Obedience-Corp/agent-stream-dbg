package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/stream-debugger/internal/config"
)

func TestResolveExplicitConfigUsesPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.yaml")
	if err := os.WriteFile(path, []byte("transport:\n  type: sse\n  base_url: http://x\n  stream_endpoint:\n    url: /s\ndialect:\n  file: brainyard\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := resolveExplicitConfig(path)
	if err != nil {
		t.Fatalf("resolveExplicitConfig: %v", err)
	}
	if got != path {
		t.Fatalf("resolved = %q, want %q", got, path)
	}
}

func TestResolveExplicitConfigMissing(t *testing.T) {
	_, err := resolveExplicitConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected missing config error")
	}
	if !strings.Contains(err.Error(), "home hub") {
		t.Errorf("error should mention home hub: %v", err)
	}
}

func TestResolveInteractiveConfigPrefersWorkingDirectory(t *testing.T) {
	cwd := t.TempDir()
	userConfigDir := t.TempDir()
	localPath := filepath.Join(cwd, "stream-debugger.yaml")
	userPath := filepath.Join(userConfigDir, "stream-debugger.yaml")
	body := "transport:\n  type: sse\n  base_url: http://x\n  stream_endpoint:\n    url: /s\ndialect:\n  file: brainyard\n"
	for _, path := range []string{localPath, userPath} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write config %s: %v", path, err)
		}
	}

	got, created, err := resolveInteractiveConfigAt("", cwd, userConfigDir)
	if err != nil {
		t.Fatalf("resolveInteractiveConfigAt: %v", err)
	}
	if got != localPath || created {
		t.Fatalf("resolved config = (%q, %v), want (%q, false)", got, created, localPath)
	}
}

func TestResolveInteractiveConfigNoAutoCreate(t *testing.T) {
	userConfigDir := filepath.Join(t.TempDir(), "stream-debugger")
	_, created, err := resolveInteractiveConfigAt("", t.TempDir(), userConfigDir)
	if err == nil {
		t.Fatal("expected error when no configs exist")
	}
	if created {
		t.Fatal("must not create starter configs")
	}
	if !strings.Contains(err.Error(), "home hub") {
		t.Errorf("error should point at home hub: %v", err)
	}
	// Directory may exist empty; no config.yaml should have been written.
	if config.ListConfigs(t.TempDir(), userConfigDir) != nil {
		// list can be empty slice not nil
	}
	entries := config.ListConfigs(t.TempDir(), userConfigDir)
	if len(entries) != 0 {
		t.Fatalf("expected no configs, got %d", len(entries))
	}
}
