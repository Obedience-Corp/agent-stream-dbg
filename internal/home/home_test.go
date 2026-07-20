package home

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/config"
)

func TestMaterializeDemo(t *testing.T) {
	demos := OfflineDemos()
	if len(demos) == 0 {
		t.Fatal("expected offline demos")
	}
	dir := t.TempDir()
	path, err := MaterializeDemo(dir, demos[0])
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Transport.Type != "replay" {
		t.Fatalf("type = %q", cfg.Transport.Type)
	}
	if !config.ConfigReady(cfg) {
		t.Fatal("demo config should be ready")
	}
	if _, err := os.Stat(cfg.Transport.BaseURL); err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
}

func TestReloadEntries(t *testing.T) {
	cwd := t.TempDir()
	user := t.TempDir()
	body := config.NewConfigYAML(config.TemplateSSE, "brainyard", "http://x", "/s", "", "", "", "", "")
	if err := config.WriteNewConfig(filepath.Join(user, "a.yaml"), body); err != nil {
		t.Fatal(err)
	}
	entries := ReloadEntries(cwd, user)
	if len(entries) != 1 {
		t.Fatalf("got %d", len(entries))
	}
}
