package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteStarterConfigCreatesPrivateBackendNeutralFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	if err := WriteStarterConfig(path); err != nil {
		t.Fatalf("WriteStarterConfig: %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read starter config: %v", err)
	}
	if !strings.Contains(string(contents), "transport:\n  type: sse") {
		t.Fatalf("starter config did not contain the default transport:\n%s", contents)
	}
	if !strings.Contains(string(contents), "dialect:\n  file: \"\"") {
		t.Fatalf("starter config did not leave dialect selection open:\n%s", contents)
	}
	if !strings.Contains(string(contents), "auto_setup: true") {
		t.Fatalf("starter config did not enable dialect setup:\n%s", contents)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat starter config: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Errorf("starter config mode = %o, want %o", got, want)
	}
}

func TestWriteStarterConfigDoesNotOverwriteExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("existing\n"), 0o600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}
	if err := WriteStarterConfig(path); err == nil {
		t.Fatal("expected existing starter config path to be rejected")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read existing config: %v", err)
	}
	if string(contents) != "existing\n" {
		t.Errorf("existing config was changed: %q", contents)
	}
}
