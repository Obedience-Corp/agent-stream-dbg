package visualizer

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
	filetransport "github.com/Obedience-Corp/agent-stream-dbg/internal/transport/file"
	tea "github.com/charmbracelet/bubbletea"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func historyFixture(t *testing.T, body string) historyRequest {
	t.Helper()
	m, _ := newTestObserver(t, "")
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return historyRequest{Path: path, Parser: m.parser, CaptureID: "capture", Runtime: "runtime", Identity: info, MinSize: info.Size(), Prefix: []byte(body[:min(len(body), 64<<10)])}
}

const historyHeader = "{\"type\":\"header\",\"capture_id\":\"capture\",\"runtime_id\":\"runtime\"}\r\n"

func historyTurn(turn string, start int) string {
	return fmt.Sprintf("{\"type\":\"delta\",\"capture_id\":\"capture\",\"runtime_id\":\"runtime\",\"turn_id\":%q,\"turn_start_ms\":%d}\r\n", turn, start)
}
func TestHistoryNearestTurnIgnoresDelayedSparseCandidate(t *testing.T) {
	body := historyHeader + historyTurn("T1", 10) + historyTurn("T2", 100) + historyTurn("T3", 1000) + historyTurn("T1", 10)
	request := historyFixture(t, body)
	request.Anchor = observerSelection{Key: eventScope(&events.Event{Fields: map[string]any{"runtime_id": "runtime", "turn_id": "T3"}}), Runtime: "runtime", Turn: "T3", Offset: int64(strings.Index(body, `"T3"`)), StartMS: func() *float64 { v := 1000.; return &v }(), RootSeen: true}
	request.Direction = -1
	request.Index = []sourceIndexEntry{{Offset: int64(len(historyHeader) + len(historyTurn("T1", 10)) + len(historyTurn("T2", 100)) + len(historyTurn("T3", 1000)))}}
	page, anchor, err := loadObserverHistory(context.Background(), request)
	if err != nil || page == nil || anchor.Turn != "T2" {
		t.Fatalf("wrong predecessor: %+v %v", anchor, err)
	}
}
func TestHistoryRetainedNavigationBeyondSummaryLimit(t *testing.T) {
	m, _ := newTestObserver(t, "")
	observerDeliver(t, m, strings.TrimSpace(historyHeader), 0)
	for i := 0; i < 150; i++ {
		observerDeliver(t, m, strings.TrimSpace(historyTurn(fmt.Sprintf("turn-%03d", i), i)), int64(i+1)*100)
	}
	if len(m.store.groups()) != 100 {
		t.Fatal("summary bound not exercised")
	}
	for i := 148; i >= 0; i-- {
		if cmd := m.navigateTurn(-1); cmd != nil {
			t.Fatal("retained history unexpectedly needed disk")
		}
		if m.selectionAnchor.Turn != fmt.Sprintf("turn-%03d", i) {
			t.Fatalf("stuck at %d: %+v", i, m.selectionAnchor)
		}
	}
}
func TestHistoryRapidNavigationKeepsOneWorkerAndClearsOldPage(t *testing.T) {
	m, _ := newTestObserver(t, "")
	m.page = newObserverStore()
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls int
	m.historyLoad = func(ctx context.Context, r historyRequest) (*observerStore, observerSelection, error) {
		calls++
		if calls == 1 {
			close(entered)
			<-release
		}
		return newObserverStore(), r.Anchor, ctx.Err()
	}
	command := m.requestHistory(-1)
	if m.page != nil {
		t.Fatal("old page retained while loading")
	}
	result := make(chan any, 1)
	go func() { result <- command() }()
	<-entered
	for i := 0; i < 100; i++ {
		if cmd := m.requestHistory(-1); cmd != nil {
			t.Fatal("parallel worker spawned")
		}
	}
	if calls != 1 || m.historyPending == nil {
		t.Fatal("missing serialized pending request")
	}
	close(release)
	message := (<-result).(*observerHistoryResult)
	next := m.historyResult(message)
	if next == nil || message.Page != nil {
		t.Fatal("result page retained across replacement")
	}
	m.historyResult(next().(*observerHistoryResult))
	if calls != 2 || m.historyActive {
		t.Fatal("wrong worker lifecycle")
	}
	m.page = newObserverStore()
	m.followLatest = false
	m.followLatestSelection()
	if m.page != nil || m.historyPending != nil || !m.followLatest {
		t.Fatal("follow retained history")
	}
}

func TestHistoryEqualStartUsesFirstSourceOffset(t *testing.T) {
	body := historyHeader + historyTurn("one", 10) + historyTurn("two", 10) + historyTurn("three", 10) + historyTurn("one", 10)
	request := historyFixture(t, body)
	firstTwo := int64(len(historyHeader) + len(historyTurn("one", 10)))
	firstThree := firstTwo + int64(len(historyTurn("two", 10)))
	value := 10.
	request.Anchor = observerSelection{Key: "runtime/turn/three", Runtime: "runtime", Turn: "three", Offset: firstThree, LastOffset: firstThree, StartMS: &value}
	// Use the actual opaque grouping function rather than assuming formatting.
	request.Anchor.Key = eventScope(&events.Event{Fields: map[string]any{"runtime_id": "runtime", "turn_id": "three"}})
	request.Direction = -1
	request.Index = []sourceIndexEntry{{Offset: 0}, {Offset: firstTwo}, {Offset: firstThree}}
	page, anchor, err := loadObserverHistory(context.Background(), request)
	if err != nil || page == nil || anchor.Turn != "two" || anchor.Offset != firstTwo {
		t.Fatalf("wrong equal-clock predecessor: %+v %v", anchor, err)
	}
	if string(page.records[0].Event.Raw) != historyTurn("two", 10) {
		t.Fatal("seek lost exact source evidence")
	}
	request.Anchor = anchor
	request.Direction = 1
	_, anchor, err = loadObserverHistory(context.Background(), request)
	if err != nil || anchor.Turn != "three" {
		t.Fatalf("equal-clock successor unavailable: %+v %v", anchor, err)
	}
	m, _ := newTestObserver(t, "")
	observerDeliver(t, m, strings.TrimSpace(historyHeader), 0)
	offset := int64(len(historyHeader))
	for _, turn := range []string{"one", "two", "three", "one"} {
		observerDeliver(t, m, strings.TrimSpace(historyTurn(turn, 10)), offset)
		offset += int64(len(historyTurn(turn, 10)))
	}
	if m.selectionAnchor.Turn != "three" {
		t.Fatal("late tie stole latest", m.selectionAnchor)
	}
	if command := m.navigateTurn(-1); command != nil || m.selectionAnchor.Turn != "two" {
		t.Fatal("retained tie unreachable", m.selectionAnchor)
	}
}
func TestHistoryEvictedSelectionNavigatesAndRejectsReplacedSource(t *testing.T) {
	var body strings.Builder
	body.WriteString(historyHeader)
	for i := 0; i < 5100; i++ {
		body.WriteString(historyTurn(fmt.Sprintf("turn-%04d", i), i))
	}
	request := historyFixture(t, body.String())
	m, _ := newTestObserver(t, "")
	m.path = request.Path
	m.captureID = "capture"
	m.runtimeID = "runtime"
	offset := int64(0)
	reader := bufio.NewReader(strings.NewReader(body.String()))
	for {
		raw, size, err := historyLine(context.Background(), reader)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		e, err := m.parser.ParseBounded("", bytes.TrimSpace(raw), 4<<20)
		if err != nil {
			t.Fatal(err)
		}
		m.store.add(e, transport.Frame{Data: bytes.TrimSpace(raw), Raw: raw, Offset: offset, Size: size}, true)
		if e.StringField("turn_id") == "turn-0000" {
			m.selectScope(eventScope(e), m.store)
		}
		offset += size
	}
	if !m.store.evicted || len(m.store.selected(m.selected)) != 0 {
		t.Fatal("selected scope not evicted")
	}
	m.followLatest = false
	_, reload := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if reload == nil {
		t.Fatal("reload control absent")
	}
	m.Update(reload())
	if m.page == nil || m.selectionAnchor.Turn != "turn-0000" {
		t.Fatal("selected turn reload failed", m.selectionAnchor)
	}
	command := m.navigateTurn(1)
	if command == nil {
		t.Fatal("evicted selection did not load source")
	}
	m.Update(command())
	if m.page == nil || m.selectionAnchor.Turn != "turn-0001" {
		t.Fatal("evicted selection navigation stuck", m.selectionAnchor)
	}
	if m.store.bytes+m.page.bytes+int(m.historyInFlight.Load()) > 20<<20 || m.historyPeak.Load() > observerPageBytes {
		t.Fatal("history page budget exceeded")
	}
	command = m.navigateTurn(1)
	if command == nil {
		t.Fatal("old-page navigation didn't request missing adjacent turn")
	}
	replacement := filepath.Join(t.TempDir(), "replacement")
	if err := os.WriteFile(replacement, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	// Test the persisted-source identity guard retained by a real transport snapshot.
	m.historyCancel()
	m.Update(command())
	request.Identity, _ = os.Stat(request.Path)
	if err := os.Rename(replacement, request.Path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadObserverHistory(context.Background(), request); !errors.Is(err, filetransport.ErrSourceChanged) {
		t.Fatal("replaced EOF source accepted", err)
	}
	m.page = newObserverStore()
	m.followLatest = false
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.page != nil || !m.followLatest {
		t.Fatal("Space follow kept historical page")
	}
}

func TestHistoryImmediateSelectionSupersedesStalePage(t *testing.T) {
	for _, selectJob := range []bool{false, true} {
		t.Run(fmt.Sprint(selectJob), func(t *testing.T) {
			m, _ := newTestObserver(t, "")
			observerDeliver(t, m, strings.TrimSpace(historyHeader), 0)
			observerDeliver(t, m, strings.TrimSpace(historyTurn("one", 1)), 100)
			observerDeliver(t, m, strings.TrimSpace(historyTurn("two", 2)), 200)
			// Session-level errors remain selectable in Flow even without a turn.
			observerDeliver(t, m, `{"type":"error","capture_id":"capture","runtime_id":"runtime"}`, 300)
			m.historyGeneration = 7
			m.historyActive = true
			cancelled := false
			m.historyCancel = func() { cancelled = true }
			m.page = newObserverStore()
			if selectJob {
				m.pane = PaneFlow
				m.selectingScope = true
				groups := m.store.groups()
				for i, group := range groups {
					if group.Turn == "" {
						m.eventRow = i
						break
					}
				}
				m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			} else {
				m.navigateTurn(-1)
			}
			selected := m.selected
			m.Update(&observerHistoryResult{Generation: 7, Page: newObserverStore(), Anchor: observerSelection{Key: "stale"}})
			if !cancelled || m.page != nil || m.selected != selected || selected == "stale" {
				t.Fatal("stale result overrode immediate selection")
			}
			if len(m.selectedRecords()) == 0 {
				t.Fatal("live session/turn selection inaccessible behind old page")
			}
		})
	}
}
func TestHistoryManyDistinctTurnsHasNoDiscoveryCeiling(t *testing.T) {
	if testing.Short() {
		t.Skip("large source history")
	}
	var body strings.Builder
	body.WriteString(historyHeader)
	for i := 0; i < 20000; i++ {
		body.WriteString(historyTurn(fmt.Sprintf("turn-%05d", i), i))
	}
	request := historyFixture(t, body.String())
	start := 20000.
	request.Anchor = observerSelection{Key: "not-present", Runtime: "runtime", Offset: int64(body.Len()), StartMS: &start}
	request.Direction = -1
	page, anchor, err := loadObserverHistory(context.Background(), request)
	if err != nil || page == nil || anchor.Turn != "turn-19999" {
		t.Fatal("valid long history unavailable", anchor, err)
	}
}

func TestHistoryJobScopeSelectableWhileOldLoadCompletes(t *testing.T) {
	m, _ := newTestObserver(t, "")
	job := &events.Event{Name: "worker", Fields: map[string]any{"runtime_id": "runtime", "job_id": "job-1", "parent_turn_id": "turn-old"}}
	m.store.add(job, transport.Frame{Data: []byte(`{"job":"job-1"}`), Offset: 10}, true)
	m.page = newObserverStore()
	m.historyGeneration = 10
	m.historyActive = true
	m.pane = PaneFlow
	m.selectingScope = true
	groups := m.store.groups()
	found := false
	for i, group := range groups {
		if group.Job == "job-1" {
			m.eventRow = i
			found = true
			break
		}
	}
	if !found {
		t.Fatal("job absent from selectable scopes")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(&observerHistoryResult{Generation: 10, Page: newObserverStore(), Anchor: observerSelection{Key: "old-turn"}})
	records := m.selectedRecords()
	if m.page != nil || len(records) != 1 || records[0].Event.StringField("job_id") != "job-1" {
		t.Fatal("job scope inaccessible after stale page")
	}
}

func TestHistoryActionsPreserveSourceLifecycle(t *testing.T) {
	for _, sealed := range []bool{false, true} {
		t.Run(fmt.Sprint(sealed), func(t *testing.T) {
			m, _ := newTestObserver(t, "")
			m.attached = true
			m.ingesting = false
			m.sealed = sealed
			expected := "EOF / unsealed"
			if sealed {
				expected = "sealed"
			}
			m.historyGeneration = 1
			m.historyActive = true
			m.Update(&observerHistoryResult{Generation: 1, Page: newObserverStore(), Anchor: observerSelection{Key: "turn"}})
			if !strings.Contains(m.View(), expected) {
				t.Fatal("history hid source lifecycle", m.View())
			}
			m.status = "turn boundary reached"
			if !strings.Contains(m.View(), expected) {
				t.Fatal("boundary hid source lifecycle")
			}
		})
	}
}

func TestHistoryUnknownClockUsesFirstOffsetFallback(t *testing.T) {
	line := func(turn string) string {
		return fmt.Sprintf("{\"type\":\"delta\",\"capture_id\":\"capture\",\"runtime_id\":\"runtime\",\"turn_id\":%q}\n", turn)
	}
	body := historyHeader + line("one") + line("two") + line("three") + line("one")
	request := historyFixture(t, body)
	request.Anchor = observerSelection{Key: eventScope(&events.Event{Fields: map[string]any{"runtime_id": "runtime", "turn_id": "three"}}), Runtime: "runtime", Turn: "three", Offset: int64(len(historyHeader) + len(line("one")) + len(line("two")))}
	request.Direction = -1
	_, anchor, err := loadObserverHistory(context.Background(), request)
	if err != nil || anchor.Turn != "two" || anchor.StartMS != nil {
		t.Fatal("unknown-clock fallback followed delayed arrival", anchor, err)
	}
}
