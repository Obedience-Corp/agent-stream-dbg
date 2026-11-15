package visualizer

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type testClock struct{ at time.Time }

func (c *testClock) now() time.Time           { return c.at }
func (c *testClock) advance(by time.Duration) { c.at = c.at.Add(by) }

func newClockObserver(t *testing.T, follow bool) (*ObserverModel, *testClock) {
	t.Helper()
	parser, err := bridge.NewParserFor("obey-voice")
	if err != nil {
		t.Fatal(err)
	}
	source := &observerTestSource{frames: make(chan transport.Frame, 16)}
	m := NewObserverModel(context.Background(), source, parser, "source", follow, "")
	t.Cleanup(func() { _ = m.Close() })
	clock := &testClock{at: time.Unix(1_800_000_000, 0)}
	m.now = clock.now
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 60})
	m.Update(observerConnected{})
	return m, clock
}

func modelRequestLines(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/fixtures/pipeline-trace/model-requests.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func editRecord(t *testing.T, line string, edit func(record map[string]any)) string {
	t.Helper()
	var record map[string]any
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatal(err)
	}
	edit(record)
	out, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func deliverAll(t *testing.T, m *ObserverModel, lines []string) {
	t.Helper()
	for i, line := range lines {
		observerDeliver(t, m, line, int64(i*1000))
	}
}

func paneText(m *ObserverModel, pane Pane) string {
	m.pane = pane
	return ansi.Strip(m.View())
}

func lineWith(view, needle string) string {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, needle) {
			return strings.TrimRight(strings.TrimSpace(strings.Trim(line, "│")), " ")
		}
	}
	return ""
}

func TestRunningElapsedUsesItsOwnRuntimeClock(t *testing.T) {
	m, clock := newClockObserver(t, true)
	m.noteSourceClock(&events.Event{Fields: map[string]any{"runtime_id": "runtime-b", "mono_ms": 50000.0}})
	clock.advance(250 * time.Millisecond)
	ms := func(value float64) *float64 { return &value }
	if _, ok := m.runningElapsedMS(&inspectedInterval{Runtime: "runtime-a", Status: "running", Start: ms(100)}); ok {
		t.Fatal("runtime without a source clock borrowed another runtime's clock")
	}
	if got, ok := m.runningElapsedMS(&inspectedInterval{Runtime: "runtime-b", Status: "running", Start: ms(49000)}); !ok || got != 1250 {
		t.Fatalf("elapsed %v %v, want 1250", got, ok)
	}
	if got, _ := m.runningElapsedMS(&inspectedInterval{Runtime: "runtime-b", Status: "running", Start: ms(60000)}); got != 0 {
		t.Fatalf("start after estimated now not clamped: %v", got)
	}
	if _, ok := m.runningElapsedMS(&inspectedInterval{Runtime: "runtime-b", Status: "running"}); ok {
		t.Fatal("elapsed estimated without a start")
	}
	m.noteSourceClock(&events.Event{Fields: map[string]any{"runtime_id": "runtime-c", "mono_ms": nil}})
	if _, ok := m.runningElapsedMS(&inspectedInterval{Runtime: "runtime-c", Status: "running", Start: ms(0)}); ok {
		t.Fatal("null source offset used as a clock")
	}
}

func TestLiveRunningRowsGrowAndResolveToMeasuredDuration(t *testing.T) {
	m, clock := newClockObserver(t, true)
	lines := modelRequestLines(t)
	// Through the route request's start record, received at mono 1410.
	deliverAll(t, m, lines[:7])

	clock.advance(2500 * time.Millisecond)
	flow := paneText(m, PaneFlow)
	if got := lineWith(flow, "↳ ● route"); !strings.HasSuffix(got, "· running 2.5s…") {
		t.Fatalf("route row %q\n%s", got, flow)
	}
	if got := lineWith(flow, " turn   "); !strings.HasSuffix(got, "running 2.9s…") {
		t.Fatalf("turn row %q", got)
	}
	timeline := paneText(m, PaneTimeline)
	route := lineWith(timeline, "model · route")
	if !strings.Contains(route, "2.5s…") || !strings.HasSuffix(route, "…") || !strings.Contains(route, "━") {
		t.Fatalf("running bar %q\n%s", route, timeline)
	}
	if !strings.Contains(timeline, "3910ms") {
		t.Fatal("axis not extended to the estimated source time", timeline)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	clock.advance(1500 * time.Millisecond)
	if got := lineWith(paneText(m, PaneFlow), "↳ ● route"); !strings.HasSuffix(got, "· running 4.0s…") {
		t.Fatalf("paused selection stopped the clock: %q", got)
	}
	for _, interval := range m.IntervalReport().Intervals {
		if interval.Label == "route" && interval.End != nil {
			t.Fatal("estimate stored as a measured end")
		}
	}

	deliverAll(t, m, lines[7:9])
	flow = paneText(m, PaneFlow)
	if got := lineWith(flow, "↳ ✓ route"); !strings.HasSuffix(got, "· completed [1690ms]") {
		t.Fatalf("route not replaced by its measurement: %q\n%s", got, flow)
	}
	if !strings.Contains(lineWith(paneText(m, PaneTimeline), "model · route"), "1690.000ms") {
		t.Fatal("timeline kept the estimate")
	}
}

func TestStaticSourceRunningRowsIgnoreTheClock(t *testing.T) {
	m, clock := newClockObserver(t, false)
	deliverAll(t, m, modelRequestLines(t)[:7])
	before := paneText(m, PaneFlow) + paneText(m, PaneTimeline)
	clock.advance(time.Minute)
	after := paneText(m, PaneFlow) + paneText(m, PaneTimeline)
	if before != after || strings.Contains(after, "s…") {
		t.Fatal("static capture rendered a live estimate", after)
	}
	if !strings.Contains(lineWith(after, "model · route"), "?") {
		t.Fatal("unfinished interval lost its unavailable duration")
	}
}

func TestRunningRowsWithoutSourceTimingStayUnavailable(t *testing.T) {
	lines := modelRequestLines(t)[:7]
	noOffsets := make([]string, len(lines))
	for i, line := range lines {
		noOffsets[i] = editRecord(t, line, func(r map[string]any) { r["trace"].(map[string]any)["mono_ms"] = nil })
	}
	m, clock := newClockObserver(t, true)
	deliverAll(t, m, noOffsets)
	clock.advance(2 * time.Second)
	if view := paneText(m, PaneFlow); strings.Contains(view, "s…") {
		t.Fatal("estimate shown without a source offset", view)
	}

	noStart := append([]string(nil), lines...)
	noStart[6] = editRecord(t, lines[6], func(r map[string]any) { r["span"].(map[string]any)["start_ms"] = nil })
	m, clock = newClockObserver(t, true)
	deliverAll(t, m, noStart)
	clock.advance(2 * time.Second)
	flow := paneText(m, PaneFlow)
	if got := lineWith(flow, "↳ ● route"); !strings.HasSuffix(got, "· running") {
		t.Fatalf("estimate shown without a start: %q", got)
	}
	if !strings.HasSuffix(lineWith(flow, " turn   "), "running 2.4s…") {
		t.Fatal("spans with a start lost their estimate", flow)
	}
}

func TestRunningRowsFallBackWhenTheSourceStops(t *testing.T) {
	lines := modelRequestLines(t)
	for name, stop := range map[string]func(*ObserverModel){
		"disconnect": func(m *ObserverModel) { m.Update(observerFrame{ok: false}) },
		"seal":       func(m *ObserverModel) { observerDeliver(t, m, lines[len(lines)-1], 99000) },
	} {
		t.Run(name, func(t *testing.T) {
			m, clock := newClockObserver(t, true)
			deliverAll(t, m, lines[:7])
			clock.advance(time.Second)
			if !strings.Contains(paneText(m, PaneFlow), "running 1.0s…") {
				t.Fatal("live estimate missing before the source stopped")
			}
			stop(m)
			clock.advance(time.Minute)
			flow := paneText(m, PaneFlow)
			if strings.Contains(flow, "s…") || !strings.HasSuffix(lineWith(flow, "↳ ● route"), "· running") {
				t.Fatal("estimate kept counting after the source stopped", flow)
			}
			if !strings.Contains(lineWith(paneText(m, PaneTimeline), "model · route"), "?") {
				t.Fatal("timeline kept counting after the source stopped")
			}
		})
	}
}
