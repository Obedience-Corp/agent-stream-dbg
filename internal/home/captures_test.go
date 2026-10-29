package home

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/captures"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func captureModel(t *testing.T) model {
	t.Helper()
	opts := Options{Cwd: t.TempDir(), UserConfigDir: t.TempDir()}
	if err := captures.SaveRoot(opts.UserConfigDir, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	m := newModel(opts)
	m.width = 100
	m.height = 30
	return m
}
func key(m model, s string) (model, tea.Cmd) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	switch s {
	case "enter":
		msg.Type = tea.KeyEnter
	case "esc":
		msg.Type = tea.KeyEsc
	case "down":
		msg.Type = tea.KeyDown
	}
	next, cmd := m.Update(msg)
	return next.(model), cmd
}
func accept(m model, entries ...captures.Entry) model {
	next, _ := m.Update(capturesLoaded{root: m.captures.root, generation: m.captures.generation, listing: captures.Listing{Entries: entries}})
	return next.(model)
}
func TestVoiceLaunchAndFollowModes(t *testing.T) {
	for _, status := range []string{"open", "sealed", "incomplete"} {
		t.Run(status, func(t *testing.T) {
			m := captureModel(t)
			m, _ = key(m, "down")
			m, cmd := key(m, "enter")
			if m.screen != screenCaptures || cmd == nil || !m.captures.loading {
				t.Fatal("bare home did not open capture discovery")
			}
			m = accept(m, captures.Entry{Path: "/captures/one/events.jsonl", Status: status})
			m, _ = key(m, "enter")
			if m.result.Action != ActionObserve || m.result.Path != "/captures/one/events.jsonl" || m.result.Follow != (status == "open") {
				t.Fatalf("result: %+v", m.result)
			}
		})
	}
}
func TestCaptureRefreshPreservesSelectionAndRejectsStaleResults(t *testing.T) {
	m := captureModel(t)
	m, _ = key(m, "enter")
	a, b := captures.Entry{Path: "a"}, captures.Entry{Path: "b"}
	m = accept(m, a)
	m = accept(m, b, a)
	if m.captures.cursor != 1 {
		t.Fatal("new recording stole selection")
	}
	m = accept(m, b)
	m, _ = key(m, "enter")
	if m.quitting || m.captures.cursor != -1 {
		t.Fatal("disappeared recording opened a different capture")
	}
	m, _ = key(m, "down")
	if m.captures.selected != "b" {
		t.Fatal("cannot recover selection")
	}
	stale := capturesLoaded{root: m.captures.root, generation: m.captures.generation, listing: captures.Listing{Entries: []captures.Entry{a}}}
	m, _ = key(m, "esc")
	m, _ = key(m, "enter")
	next, _ := m.Update(stale)
	m = next.(model)
	if len(m.captures.listing.Entries) != 1 || m.captures.listing.Entries[0].Path != "b" {
		t.Fatal("stale response replaced newer screen")
	}
}
func TestEmptyStateAndFolderPersistence(t *testing.T) {
	m := captureModel(t)
	m, _ = key(m, "down")
	m, cmd := key(m, "enter")
	next, _ := m.Update(cmd())
	m = next.(model)
	if view := m.View(); !strings.Contains(view, "/record talk") || !strings.Contains(view, "conversation audio") {
		t.Fatal(view)
	}
	m, _ = key(m, "c")
	m.captures.folder.SetValue(filepath.Join(t.TempDir(), "missing"))
	m, _ = key(m, "enter")
	if m.screen != screenCaptureFolder || m.status == "" {
		t.Fatal("missing folder was accepted")
	}
	root := t.TempDir()
	m.captures.folder.SetValue(root)
	m, cmd = key(m, "enter")
	if m.screen != screenCaptures || cmd == nil {
		t.Fatal("folder did not return to browser")
	}
	fresh := newModel(m.opts)
	if fresh.captures.root != root {
		t.Fatal("folder not persisted across restart")
	}
	m = accept(m)
	if m.captures.err != "" {
		t.Fatal("folder error stuck after recovery")
	}
}
func TestConfigsAndDemosRemainReachable(t *testing.T) {
	m := captureModel(t)
	path := filepath.Join(m.opts.UserConfigDir, "endpoint.yaml")
	content := config.NewConfigYAML(config.TemplateSSE, "openai", "http://localhost", "/s", "", "", "", "", "")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	m.reload()
	m, _ = key(m, "down")
	configModel, _ := key(m, "down")
	for _, action := range []string{"enter", "e", "d"} {
		next, _ := key(configModel, action)
		if action == "d" {
			if next.screen != screenConfirmDelete || next.deleteIndex != 0 {
				t.Fatal("delete targets wrong config")
			}
		} else if next.result.Path != path {
			t.Fatalf("%s opened %q", action, next.result.Path)
		}
	}
	m.cursor = len(m.homeItems()) - 1
	m, _ = key(m, "enter")
	if m.screen != screenOffline {
		t.Fatal("offline action unreachable")
	}
	m, _ = key(m, "esc")
	if m.homeItems()[m.cursor] != "action:offline" {
		t.Fatal("offline return selection incorrect")
	}
}
func TestCaptureListScrollAndMetadataSanitization(t *testing.T) {
	m := captureModel(t)
	m.height = 24
	m, _ = key(m, "enter")
	var entries []captures.Entry
	for i := 0; i < 100; i++ {
		entries = append(entries, captures.Entry{Path: fmt.Sprint(i), SessionID: fmt.Sprintf("session-%03d", i)})
	}
	m = accept(m, entries...)
	for range 99 {
		m, _ = key(m, "down")
	}
	view := m.View()
	if !strings.Contains(view, "session-099") || strings.Contains(view, "session-000") {
		t.Fatal("selection scrolled out of view")
	}
	if clean := captureLabel("test\x1b[31m\nname", 40); strings.ContainsAny(clean, "\x1b\n") {
		t.Fatal("unsafe terminal metadata")
	}
	if len(strings.Split(view, "\n")) > m.height {
		t.Fatalf("view exceeds terminal height: %d", len(strings.Split(view, "\n")))
	}
}
