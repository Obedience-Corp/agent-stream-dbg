package home

import (
	"github.com/Obedience-Corp/agent-stream-dbg/internal/captures"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultLaunchChoosesLiveAndSwitchClearsPreviousSource(t *testing.T) {
	m := captureModel(t)
	m, cmd := key(m, "enter")
	if !m.captures.live || cmd == nil {
		t.Fatal("default launch did not discover live conversations")
	}
	if view := m.View(); !strings.Contains(view, "Recording is not required") {
		t.Fatal(view)
	}
	m = accept(m, captures.Entry{Path: "live-descriptor.json", Status: "live"})
	selected, _ := key(m, "enter")
	if selected.result.Action != ActionLive {
		t.Fatal(selected.result)
	}
	m, _ = key(m, "esc")
	m, _ = key(m, "down")
	m, _ = key(m, "enter")
	if m.captures.live || len(m.captures.listing.Entries) > 0 || m.captures.selected != "" {
		t.Fatal("live selection leaked into recordings")
	}
	m, _ = key(m, "enter")
	if m.quitting {
		t.Fatal("opened stale descriptor as recording")
	}
}
func TestLiveCustomConfigFolderPersistsWithoutCaptureDirectory(t *testing.T) {
	m := captureModel(t)
	m, _ = key(m, "enter")
	m, _ = key(m, "c")
	root := t.TempDir()
	m.captures.folder.SetValue(root)
	m, _ = key(m, "enter")
	if m.captures.root != filepath.Join(root, "captures") {
		t.Fatal(m.status, m.captures.root)
	}
	restored := newModel(m.opts)
	if restored.captures.root != m.captures.root {
		t.Fatal("custom config folder was not restored")
	}
}
