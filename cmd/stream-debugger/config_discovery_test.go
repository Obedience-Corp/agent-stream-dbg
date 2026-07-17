package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveInteractiveConfigUsesExplicitPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.yaml")
	if err := os.WriteFile(path, []byte("transport: {}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, created, err := resolveInteractiveConfigAt(path, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("resolveInteractiveConfigAt: %v", err)
	}
	if got != path || created {
		t.Fatalf("resolved explicit config = (%q, %v), want (%q, false)", got, created, path)
	}
}

func TestResolveInteractiveConfigPrefersWorkingDirectory(t *testing.T) {
	cwd := t.TempDir()
	userConfigDir := t.TempDir()
	localPath := filepath.Join(cwd, "stream-debugger.yaml")
	userPath := filepath.Join(userConfigDir, "stream-debugger.yaml")
	for _, path := range []string{localPath, userPath} {
		if err := os.WriteFile(path, []byte("transport: {}\n"), 0o600); err != nil {
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

func TestResolveInteractiveConfigCreatesStarterInUserDirectory(t *testing.T) {
	userConfigDir := filepath.Join(t.TempDir(), "stream-debugger")
	got, created, err := resolveInteractiveConfigAt("", t.TempDir(), userConfigDir)
	if err != nil {
		t.Fatalf("resolveInteractiveConfigAt: %v", err)
	}
	want := filepath.Join(userConfigDir, "config.yaml")
	if got != want || !created {
		t.Fatalf("resolved starter = (%q, %v), want (%q, true)", got, created, want)
	}
	contents, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("read starter config: %v", err)
	}
	if !strings.Contains(string(contents), "Fill in the connection details") {
		t.Errorf("starter config did not include setup guidance:\n%s", contents)
	}
}

func TestResolveInteractiveConfigExplainsMissingExplicitPath(t *testing.T) {
	_, _, err := resolveInteractiveConfigAt(filepath.Join(t.TempDir(), "missing.yaml"), t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("expected missing explicit config to return an error")
	}
	if !strings.Contains(err.Error(), "without --config") {
		t.Errorf("missing config error was not actionable: %v", err)
	}
}
