package visualizer

import (
	"context"
	"errors"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	filetransport "github.com/Obedience-Corp/agent-stream-dbg/internal/transport/file"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
	tea "github.com/charmbracelet/bubbletea"
)

const observerTestDialect = `version: 1
name: generic-capture
discriminator: {path: type}
rules:
  - match: {event: header}
    kind: session_start
    timestamp: ts
    seq: seq
    fields:
      capture_id: capture_id
      runtime_id: runtime_id
      session_id: session_id
      supported_trace_major: {const: 1}
      supported_trace_family: {const: capture}
  - match: {event: footer}
    kind: session_end
    seq: seq
    fields:
      capture_id: capture_id
  - match: {event: '*'}
    kind: detail
    timestamp: ts
    seq: seq
    fields:
      capture_id: capture_id
      runtime_id: runtime_id
      turn_id: turn_id
      response_id: response_id
      turn_start_ms: turn_start_ms
      span_id: span_id
`

type observerTestSource struct {
	frames                  chan transport.Frame
	connects, sends, closes atomic.Int32
}

func (s *observerTestSource) Name() string                       { return "test" }
func (s *observerTestSource) Connect(context.Context) error      { s.connects.Add(1); return nil }
func (s *observerTestSource) Frames() <-chan transport.Frame     { return s.frames }
func (s *observerTestSource) Send(context.Context, []byte) error { s.sends.Add(1); return nil }
func (s *observerTestSource) Close() error                       { s.closes.Add(1); return nil }
func newTestObserver(t *testing.T, manifest string) (*ObserverModel, *observerTestSource) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dialect.yaml")
	if err := os.WriteFile(path, []byte(observerTestDialect), 0600); err != nil {
		t.Fatal(err)
	}
	parser, err := bridge.NewParserFor(path)
	if err != nil {
		t.Fatal(err)
	}
	source := &observerTestSource{frames: make(chan transport.Frame, 16)}
	model := NewObserverModel(context.Background(), source, parser, "explicit.jsonl", true, manifest)
	t.Cleanup(func() { _ = model.Close() })
	return model, source
}
func observerDeliver(t *testing.T, m *ObserverModel, line string, offset int64) tea.Cmd {
	t.Helper()
	data := []byte(line)
	_, cmd := m.Update(observerFrame{frame: transport.Frame{Data: data, Raw: data, Offset: offset}, ok: true})
	return cmd
}
func TestObserverInitPassiveTwoTurnsAndEOF(t *testing.T) {
	m, source := newTestObserver(t, "")
	connected := m.Init()()
	m.Update(connected)
	if source.connects.Load() != 1 || source.sends.Load() != 0 {
		t.Fatal("passive attach prompted or reconnected")
	}
	observerDeliver(t, m, `{"type":"header","capture_id":"capture","runtime_id":"runtime","trace_schema":"capture/v1","seq":1}`, 0)
	observerDeliver(t, m, `{"type":"delta","capture_id":"capture","runtime_id":"runtime","turn_id":"one","turn_start_ms":0,"seq":2,"ts":"2026-10-27T12:00:00Z"}`, 100)
	observerDeliver(t, m, `{"type":"delta","capture_id":"capture","runtime_id":"runtime","turn_id":"two","turn_start_ms":10,"seq":3}`, 200)
	if len(m.store.groups()) != 3 || !strings.HasSuffix(m.selected, "two") {
		t.Fatalf("turns merged or not selected: %s", m.selected)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	selected := m.selected
	observerDeliver(t, m, `{"type":"delta","capture_id":"capture","runtime_id":"runtime","turn_id":"two","turn_start_ms":10,"seq":4}`, 300)
	if m.selected != selected || len(m.store.records) != 4 {
		t.Fatal("pausing selection stopped ingest or moved selection")
	}
	_, cmd := m.Update(observerFrame{ok: false})
	if cmd != nil || m.ingesting || !strings.Contains(m.status, "unsealed") || !strings.Contains(m.View(), "agent-stream-dbg") {
		t.Fatal("EOF closed inspection or lost unsealed status")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if source.sends.Load() != 0 || source.connects.Load() != 1 {
		t.Fatal("observer control action reached producer")
	}
}
func TestObserverCaptureChangeManifestAndUnsupportedVersion(t *testing.T) {
	for _, mode := range []string{"manifest", "changed", "version", "family"} {
		t.Run(mode, func(t *testing.T) {
			manifest := ""
			if mode == "manifest" {
				manifest = "different"
			}
			m, _ := newTestObserver(t, manifest)
			line := `{"type":"header","capture_id":"capture","trace_schema":"capture/v2","seq":1}`
			if mode == "family" {
				line = strings.Replace(line, "capture/v2", "different/v1", 1)
			}
			cmd := observerDeliver(t, m, line, 0)
			if mode == "version" || mode == "family" {
				if m.intervalsEnabled || !strings.Contains(m.View(), "Unsupported") {
					t.Fatal("unsupported major interpreted intervals")
				}
				return
			}
			if mode == "changed" {
				cmd = observerDeliver(t, m, `{"type":"delta","capture_id":"next-run","seq":2}`, 100)
			}
			if m.err == nil || cmd == nil {
				t.Fatal("identity mismatch did not stop ingestion")
			}
		})
	}
}
func TestObserverDedupConflictAndSourceTime(t *testing.T) {
	m, _ := newTestObserver(t, "")
	line := `{"type":"delta","capture_id":"capture","runtime_id":"runtime","turn_id":"turn","seq":1,"ts":"2026-10-27T12:00:00Z"}`
	observerDeliver(t, m, line, 0)
	observerDeliver(t, m, line, 100)
	if len(m.store.records) != 1 || m.store.records[0].Event.Timestamp.Year() != 2026 {
		t.Fatal("duplicate or source time mishandled")
	}
	observerDeliver(t, m, strings.Replace(line, `"turn"`, `"other"`, 1), 200)
	if len(m.store.records) != 2 || m.store.records[1].Issue == "" {
		t.Fatal("conflicting duplicate hidden")
	}
}

func TestObserverFooterDrainsTrailingRecords(t *testing.T) {
	m, _ := newTestObserver(t, "")
	path := filepath.Join(t.TempDir(), "events.jsonl")
	data := "{\"type\":\"header\",\"capture_id\":\"capture\",\"trace_schema\":\"capture/v1\",\"seq\":1}\n{\"type\":\"footer\",\"capture_id\":\"capture\",\"status\":\"sealed\",\"seq\":2}\n{\"type\":\"trailing\",\"capture_id\":\"capture\",\"seq\":3}\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	source := filetransport.New(path, true)
	m.source = source
	defer source.Close()
	_, cmd := m.Update(m.Init()())
	for m.ingesting {
		if cmd == nil {
			t.Fatal("ingest stopped before EOF")
		}
		_, cmd = m.Update(cmd())
	}
	if len(m.store.records) != 3 || m.store.records[2].Event.Name != "trailing" || !m.sealed || !strings.Contains(m.status, "inspection open") {
		t.Fatalf("footer discarded trailing data: records=%d status=%s", len(m.store.records), m.status)
	}
}
func TestObserverTerminalErrorRemainsVisible(t *testing.T) {
	m, _ := newTestObserver(t, "")
	_, cmd := m.Update(observerFrame{frame: transport.Frame{Err: errors.New("Source changed; reopen explicitly"), Terminal: true}, ok: true})
	if cmd == nil {
		t.Fatal("fatal error did not close source")
	}
	m.Update(cmd())
	if m.err == nil || !strings.Contains(m.status, "reopen explicitly") || !strings.Contains(m.View(), "Source changed") {
		t.Fatal("fatal source reason replaced by ordinary EOF")
	}
}
func TestObserverLatestTurnUsesSourceAnchorAfterEviction(t *testing.T) {
	m, _ := newTestObserver(t, "")
	m.runtimeID = "runtime"
	for i := 0; i < observerPageEvents+2; i++ {
		turn := "new"
		start := 10.0
		if i == 0 {
			turn = "old"
			start = 0
		}
		e := &events.Event{Name: "span", Fields: map[string]any{"runtime_id": "runtime", "turn_id": turn, "turn_start_ms": start}}
		m.store.add(e, transport.Frame{Offset: int64(i), Data: []byte("{}")}, false)
	}
	m.store.add(&events.Event{Name: "late", Fields: map[string]any{"runtime_id": "runtime", "turn_id": "old", "turn_start_ms": 0.0}}, transport.Frame{Offset: 99999, Data: []byte("{}")}, false)
	if got := latestObservedGroup(m.store.groups(), "runtime"); !strings.HasSuffix(got, "new") {
		t.Fatalf("late old invocation selected as latest: %s", got)
	}
}
func TestObserverLongContentPaneRetainsOnlyViewportStrings(t *testing.T) {
	m, _ := newTestObserver(t, "")
	m.pane = PaneMessages
	m.width = 120
	m.height = 30
	for i := 0; i < 50; i++ {
		e := &events.Event{Name: "content", Content: strings.Repeat("😀", 50000), Fields: map[string]any{"runtime_id": "runtime", "turn_id": "one"}}
		m.store.add(e, transport.Frame{Data: []byte("{}")}, false)
	}
	m.selected = "runtime/turn/one"
	if m.store.bytes > observerPageBytes || len(m.View()) > 120*30*4 {
		t.Fatal("pane rendering retained full long content")
	}
}
