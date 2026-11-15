package visualizer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
	tea "github.com/charmbracelet/bubbletea"
)

// fixtureSeq returns the canonical fixture record with the given sequence number.
func fixtureSeq(t *testing.T, lines []string, seq int) string {
	t.Helper()
	return lines[seq-1]
}

func barCells(line string) int { return strings.Count(line, "━") }

func TestEndRecordWithUnknownStatusStopsTheEstimate(t *testing.T) {
	lines := modelRequestLines(t)
	m, clock := newClockObserver(t, true)
	deliverAll(t, m, lines[:7])
	// The route's end arrives without a measured end or a terminal status.
	unknownEnd := editRecord(t, fixtureSeq(t, lines, 9), func(r map[string]any) {
		r["seq"] = 8
		span := r["span"].(map[string]any)
		span["status"], span["end_ms"] = "unknown", nil
	})
	observerDeliver(t, m, unknownEnd, 7000)
	clock.advance(10 * time.Minute)
	flow := paneText(m, PaneFlow)
	if got := lineWith(flow, "↳ ● route"); !strings.HasSuffix(got, "· running") {
		t.Fatalf("ended span still estimated: %q", got)
	}
	timeline := paneText(m, PaneTimeline)
	if !strings.Contains(lineWith(timeline, "model · route"), "?") || strings.Contains(timeline, "601") {
		t.Fatal("ended span stretched the timeline", timeline)
	}
}

func TestLostEndStopsAtTheParentEndAndKeepsBarWidths(t *testing.T) {
	lines := modelRequestLines(t)
	var kept []string
	for seq := 1; seq <= 15; seq++ {
		if seq != 9 { // the route's end record is lost
			kept = append(kept, fixtureSeq(t, lines, seq))
		}
	}
	live, clock := newClockObserver(t, true)
	deliverAll(t, live, kept)
	clock.advance(10 * time.Minute)
	static, _ := newClockObserver(t, false)
	deliverAll(t, static, kept)
	liveTimeline, staticTimeline := paneText(live, PaneTimeline), paneText(static, PaneTimeline)
	if !strings.Contains(liveTimeline, "1000ms") || !strings.Contains(liveTimeline, "5300ms") {
		t.Fatal("axis stretched past the finished turn", liveTimeline)
	}
	for _, row := range []string{"model · skill_check", "model · reply", "model                "} {
		if got, want := barCells(lineWith(liveTimeline, row)), barCells(lineWith(staticTimeline, row)); got != want || want == 0 {
			t.Fatalf("%s bar is %d cells live, %d static\n%s", row, got, want, liveTimeline)
		}
	}
	if got := lineWith(paneText(live, PaneFlow), "↳ ● route"); !strings.HasSuffix(got, "· running") {
		t.Fatalf("child kept counting after its turn ended: %q", got)
	}
}

func TestSequenceGapBeforeALaterTurnStopsTheEarlierEstimate(t *testing.T) {
	lines := modelRequestLines(t)
	m, clock := newClockObserver(t, true)
	deliverAll(t, m, lines[:7])
	// Records 8–15, including turn 1's ends, are lost; turn 2 starts.
	for _, seq := range []int{16, 17, 18} {
		observerDeliver(t, m, fixtureSeq(t, lines, seq), int64(seq*1000))
	}
	clock.advance(30 * time.Second)
	if got := lineWith(paneText(m, PaneFlow), "↳ ● route"); !strings.Contains(got, "running 30.0s…") {
		t.Fatalf("new turn's request lost its estimate: %q", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	flow := paneText(m, PaneFlow)
	if got := lineWith(flow, "↳ ● route"); !strings.HasSuffix(got, "· running") || strings.Contains(flow, "s…") {
		t.Fatal("span whose end fell in a gap kept counting after a later turn started", flow)
	}
}

func TestSequenceGapWithinTheSameTurnKeepsCounting(t *testing.T) {
	lines := modelRequestLines(t)
	m, clock := newClockObserver(t, true)
	deliverAll(t, m, lines[:7])
	observerDeliver(t, m, fixtureSeq(t, lines, 10), 10000) // records 8 and 9 lost
	clock.advance(2 * time.Second)
	if got := lineWith(paneText(m, PaneFlow), "↳ ● route "); !strings.Contains(got, "running") || !strings.HasSuffix(got, "s…") {
		t.Fatalf("estimate disabled by a gap without a later turn: %q", got)
	}
}

func TestNewestRuntimeKeepsAClockWhenTheTableIsFull(t *testing.T) {
	m, clock := newClockObserver(t, true)
	for i := range maxClockAnchors + 1 {
		clock.advance(time.Millisecond)
		m.noteSourceClock(&events.Event{Fields: map[string]any{"runtime_id": fmt.Sprintf("runtime-%d", i), "mono_ms": 1000.0}})
	}
	start := 0.0
	if _, ok := m.runningElapsedMS(&inspectedInterval{Runtime: fmt.Sprintf("runtime-%d", maxClockAnchors), Status: "running", Start: &start}); !ok {
		t.Fatal("newest runtime has no clock")
	}
	if _, ok := m.runningElapsedMS(&inspectedInterval{Runtime: "runtime-0", Status: "running", Start: &start}); ok {
		t.Fatal("oldest runtime kept its clock past the table limit")
	}
}

// fileTestSource is a followed file: it reports its size when opened.
type fileTestSource struct {
	observerTestSource
	info os.FileInfo
}

func (s *fileTestSource) Name() string                    { return "file" }
func (s *fileTestSource) Snapshot() (os.FileInfo, []byte) { return s.info, nil }
func (s *fileTestSource) FinishFollow()                   {}
func (s *fileTestSource) sized(t *testing.T, size int) *fileTestSource {
	t.Helper()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	s.info = info
	return s
}

func TestFollowedFileEstimatesOnlyWhileItGrows(t *testing.T) {
	lines := modelRequestLines(t)
	parser, err := bridge.NewParserFor("obey-voice")
	if err != nil {
		t.Fatal(err)
	}
	opened := 0
	for _, line := range lines[:7] {
		opened += len(line) + 1
	}
	source := (&fileTestSource{observerTestSource: observerTestSource{frames: make(chan transport.Frame, 16)}}).sized(t, opened)
	m := NewObserverModel(context.Background(), source, parser, "events.jsonl", true, "")
	t.Cleanup(func() { _ = m.Close() })
	clock := &testClock{at: time.Unix(1_800_000_000, 0)}
	m.now = clock.now
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 60})
	m.Update(observerConnected{})
	offset := 0
	deliver := func(line string) {
		data := []byte(line)
		m.Update(observerFrame{frame: transport.Frame{Data: data, Raw: data, Offset: int64(offset), Size: int64(len(line) + 1)}, ok: true})
		offset += len(line) + 1
	}
	for _, line := range lines[:7] {
		deliver(line)
	}
	clock.advance(2 * time.Second)
	if view := paneText(m, PaneFlow); strings.Contains(view, "s…") {
		t.Fatal("estimated from records already on disk when the file was opened", view)
	}
	deliver(lines[7]) // the file grows
	clock.advance(2 * time.Second)
	if got := lineWith(paneText(m, PaneFlow), "↳ ● route"); !strings.HasSuffix(got, "s…") {
		t.Fatalf("growing file not estimated: %q", got)
	}
	clock.advance(followQuietPeriod)
	if view := paneText(m, PaneFlow); strings.Contains(view, "s…") {
		t.Fatal("quiet file kept counting", view)
	}
}
