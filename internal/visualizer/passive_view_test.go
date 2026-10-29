package visualizer

import (
	"context"
	"fmt"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	filetransport "github.com/Obedience-Corp/agent-stream-dbg/internal/transport/file"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestPassiveSessionUsesSharedPanesWithMeasuredEvidence(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/fixtures/pipeline-trace/overlap-interrupted.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	// Include actual response content for the selected turn alongside source timings.
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	content := `{"type":"agent","seq":30,"text":"Inspect overlapping synthesis and playback before optimizing.","trace":{"capture_id":"fixture-capture","runtime_id":"fixture-runtime","session_id":"fixture-session","conversation_epoch":"epoch-1","turn_id":"turn-1","persona_id":"assistant"}}`
	lines = append(lines[:len(lines)-1], content)
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	parser, err := bridge.NewParserFor("obey-voice")
	if err != nil {
		t.Fatal(err)
	}
	m := NewObserverModel(context.Background(), filetransport.New(path, false), parser, path, false, "")
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_, cmd := m.Update(m.Init()())
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		_, cmd = m.Update(msg)
	}
	key := func(s string) { m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}) }
	key("[")
	for _, tc := range []struct{ key, want string }{{"1", "Flow Steps"}, {"2", "Inspect overlapping synthesis"}, {"3", "2200.000ms"}, {"4", "trace_span"}, {"5", "Synthetic long reply"}} {
		key(tc.key)
		view := ansi.Strip(m.View())
		if !strings.Contains(view, tc.want) || !strings.Contains(view, "1:Flow") && tc.key != "1" {
			t.Fatal(tc.key, view)
		}
		if strings.Contains(view, "0 tokens") || strings.Contains(view, "Ctrl+T:") {
			t.Fatal("fabricated count or unavailable control", view)
		}
	}
	key("2")
	if !strings.Contains(m.View(), "token count unavailable") {
		t.Fatal("unknown usage shown as known")
	}
	key("1")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.View(), "completed") || !strings.Contains(m.View(), "→") {
		t.Fatal("stage expansion lost measured source evidence", m.View())
	}
	key("3")
	if m.IntervalReport().SegmentOverlapMS != 250 {
		t.Fatal("shared rendering changed concurrent timing")
	}
	key("]")
	if !strings.Contains(m.View(), "cancelled") || !strings.Contains(m.View(), "interrupted") {
		t.Fatal("terminal interval outcomes hidden", m.View())
	}
}

func TestEventNavigationCanInspectBeyondFirstProjectionPage(t *testing.T) {
	m, _ := newTestObserver(t, "")
	m.pane = PaneEvents
	m.selected = "runtime/turn/turn"
	for i := 0; i < 700; i++ {
		fields := map[string]any{"runtime_id": "runtime", "turn_id": "turn"}
		m.store.add(&events.Event{Name: fmt.Sprintf("event-%d", i), Seq: i + 1, Fields: fields}, transport.Frame{Data: []byte("{}"), Offset: int64(i + 1), Size: 1}, false)
	}
	m.movePassiveSelection(600)
	p := m.presentation()
	if p.selectedEventIdx != 88 || p.messages[0].Events[88].Name != "event-600" {
		t.Fatal("projected page detached from selected evidence")
	}
	m.openObserverDetails()
	if m.detailRecordOffset != 601 {
		t.Fatal("wrong source opened", m.detailRecordOffset)
	}
	m.movePassiveSelection(10000)
	if m.eventRow != 699 {
		t.Fatal("selection escaped retained evidence")
	}
}
