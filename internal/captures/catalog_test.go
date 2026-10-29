package captures

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func bundle(t *testing.T, root, name, status, stamp string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	raw := `{"schema":"obey-voice.capture/v1","capture_id":"id","session_id":"session","status":"` + status + `","started_at":"` + stamp + `","dir":"/old/install"}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestListUsesMetadataAndActualLocation(t *testing.T) {
	root := t.TempDir()
	old := bundle(t, root, "old", "open", "2026-01-01T00:00:00Z")
	recent := bundle(t, root, "recent", "sealed", "2026-02-01T00:00:00Z")
	// A recently touched old capture must not displace the newer conversation.
	os.Chtimes(old, time.Now(), time.Now())
	got, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 || got.Entries[0].Path != filepath.Join(recent, "events.jsonl") {
		t.Fatalf("listing: %+v", got)
	}
	if got.Entries[0].Follow() || !got.Entries[1].Follow() {
		t.Fatal("saved vs open follow mode incorrect")
	}
	if got.Entries[1].Path != filepath.Join(old, "events.jsonl") {
		t.Fatal("trusted stale manifest dir")
	}
}

func TestListRecoversFromMissingAndBrokenBundles(t *testing.T) {
	root := t.TempDir()
	got, err := List(filepath.Join(root, "missing"))
	if err != nil || !got.Missing || len(got.Entries) != 0 {
		t.Fatalf("missing: %+v %v", got, err)
	}
	for name, raw := range map[string]string{"broken": "{", "large": strings.Repeat("x", manifestLimit+1), "foreign": `{"schema":"something-else"}`} {
		dir := bundle(t, root, name, "sealed", "2026-01-01T00:00:00Z")
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	dir := bundle(t, root, "missing-events", "sealed", "2026-01-01T00:00:00Z")
	os.Remove(filepath.Join(dir, "events.jsonl"))
	dir = bundle(t, root, "non-file", "sealed", "2026-01-01T00:00:00Z")
	os.Remove(filepath.Join(dir, "events.jsonl"))
	os.Mkdir(filepath.Join(dir, "events.jsonl"), 0700)
	bundle(t, root, "good", "incomplete", "2026-01-01T00:00:00Z")
	got, err = List(root)
	if err != nil || len(got.Entries) != 1 || got.Skipped != 5 || got.Entries[0].Follow() {
		t.Fatalf("listing: %+v %v", got, err)
	}
}

func TestPreferencesPersistCustomFolderAndRejectFiles(t *testing.T) {
	configDir, root := t.TempDir(), t.TempDir()
	custom := filepath.Join(root, "recordings with spaces")
	os.Mkdir(custom, 0700)
	path, err := ResolveRoot(`"`+custom+`"`, root)
	if err != nil || path != custom {
		t.Fatalf("resolve: %q %v", path, err)
	}
	if err := SaveRoot(configDir, path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadRoot(configDir)
	if err != nil || got != custom {
		t.Fatalf("reload: %q %v", got, err)
	}
	raw, err := os.ReadFile(filepath.Join(configDir, "capture-browser.json"))
	if err != nil || !json.Valid(raw) {
		t.Fatalf("persisted: %s %v", raw, err)
	}
	if _, err := ResolveRoot(filepath.Join(configDir, "capture-browser.json"), root); err == nil {
		t.Fatal("accepted a file as root")
	}
	if _, err := ResolveRoot("missing", root); err == nil {
		t.Fatal("accepted nonexistent custom root")
	}
}

func TestDefaultAndCorruptPreferences(t *testing.T) {
	dir := t.TempDir()
	got, err := LoadRoot(dir)
	want, defaultErr := DefaultRoot()
	if err != nil || defaultErr != nil || got != want {
		t.Fatalf("default: %q %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "capture-browser.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRoot(dir); err == nil {
		t.Fatal("silently ignored corrupt settings")
	}
}
