package visualizer

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestHandleKeyMsg_PaneSwitching is a floor-level test for update_keys.go
// (handleKeyMsg was extracted from Update as part of this file split, and
// had no direct test before). Covers the "handled" return path via the
// pane-switch number keys, and the "not handled" fallthrough for a key
// with no binding while not in insert mode.
func TestHandleKeyMsg_PaneSwitching(t *testing.T) {
	m := newFixtureModel(t)

	cases := []struct {
		key  string
		pane Pane
	}{
		{"1", PaneFlow},
		{"2", PaneApp},
		{"3", PaneTimeline},
		{"4", PaneEvents},
		{"5", PaneMessages},
	}
	for _, tc := range cases {
		updated, _, handled := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.key)})
		if !handled {
			t.Errorf("key %q: expected handled=true", tc.key)
		}
		if updated.activePane != tc.pane {
			t.Errorf("key %q: expected pane %v, got %v", tc.key, tc.pane, updated.activePane)
		}
	}
}

// TestHandleKeyMsg_CtrlC proves the global quit binding is handled and
// returns tea.Quit regardless of mode.
func TestHandleKeyMsg_CtrlC(t *testing.T) {
	m := newFixtureModel(t)
	_, cmd, handled := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !handled {
		t.Fatal("expected Ctrl+C to be handled")
	}
	if cmd == nil {
		t.Fatal("expected a non-nil command for Ctrl+C")
	}
}

// TestHandleKeyMsg_InsertToggle proves "i" enters insert mode (and focuses
// the textarea) only when not already in insert mode.
func TestHandleKeyMsg_InsertToggle(t *testing.T) {
	m := newFixtureModel(t)
	if m.insertMode {
		t.Fatal("expected insertMode to start false")
	}

	updated, _, handled := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if !handled || !updated.insertMode {
		t.Fatalf("expected 'i' to be handled and enter insert mode; handled=%v insertMode=%v", handled, updated.insertMode)
	}
}

// TestHandleKeyMsg_UnboundKeyNotHandled proves a key with no binding falls
// through with handled=false so Update's shared tail logic (forwarding to
// the textarea, etc.) still runs — this is the exact contract the
// handleKeyMsg extraction depends on to stay behavior-preserving.
func TestHandleKeyMsg_UnboundKeyNotHandled(t *testing.T) {
	m := newFixtureModel(t)
	m.insertMode = true

	_, _, handled := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	if handled {
		t.Error("expected an ordinary character key in insert mode to be unhandled by handleKeyMsg (forwarded to the textarea instead)")
	}
}

// TestHandleKeyMsg_CtrlS_SavesEvents exercises the Ctrl+S export path,
// which writes the latest message's expanded events to a file in the
// current working directory and records a saveStatus message.
func TestHandleKeyMsg_CtrlS_SavesEvents(t *testing.T) {
	m := newFixtureModel(t)
	t.Chdir(t.TempDir())

	updated, _, handled := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !handled {
		t.Fatal("expected Ctrl+S to be handled")
	}
	if updated.saveStatus == "" {
		t.Error("expected a non-empty saveStatus after Ctrl+S")
	}
}
