package visualizer

import (
	"fmt"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
)

// clockAnchor pairs the source monotonic offset of the latest record received
// for a runtime with the local time the viewer received it.
type clockAnchor struct {
	sourceMS   float64
	receivedAt time.Time
}

const maxClockAnchors = 64

func (m *ObserverModel) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

func (m *ObserverModel) noteSourceClock(event *events.Event) {
	runtime := boundedIdentity(event.StringField("runtime_id"))
	mono, ok := finiteNumber(event.Fields["mono_ms"])
	if runtime == "" || !ok {
		return
	}
	if m.clockAnchors == nil {
		m.clockAnchors = make(map[string]clockAnchor)
	}
	if _, known := m.clockAnchors[runtime]; !known && len(m.clockAnchors) >= maxClockAnchors {
		return
	}
	m.clockAnchors[runtime] = clockAnchor{sourceMS: mono, receivedAt: m.clock()}
}

// liveSource reports whether records can still arrive for the retained view:
// a followed file or live feed that has neither ended nor disconnected.
func (m *ObserverModel) liveSource() bool {
	return m.following && m.attached && m.ingesting && !m.sourceEnded && m.page == nil
}

// runningElapsedMS estimates how long an unfinished interval has been running
// on its own runtime clock: the latest received source offset plus local time
// since receipt, minus the interval start. It is a display estimate only and
// is never stored as a measurement.
func (m *ObserverModel) runningElapsedMS(interval *inspectedInterval) (float64, bool) {
	if !m.liveSource() || interval.Status != "running" || interval.Start == nil || interval.End != nil ||
		interval.Invalid || interval.Uncorrelated || interval.Relative || interval.Gap || interval.StartFinal {
		return 0, false
	}
	anchor, ok := m.clockAnchors[interval.Runtime]
	if !ok {
		return 0, false
	}
	now := anchor.sourceMS + float64(m.clock().Sub(anchor.receivedAt))/float64(time.Millisecond)
	return max(0, now-*interval.Start), true
}

func formatElapsed(ms float64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms…", int(ms))
	}
	return fmt.Sprintf("%.1fs…", ms/1000)
}

// runningLabel is the live elapsed estimate for an unfinished interval, or ""
// when the source is static or the estimate is unavailable.
func (m InteractiveModel) runningLabel(interval *inspectedInterval) string {
	if m.passive == nil {
		return ""
	}
	if elapsed, ok := m.passive.runningElapsedMS(interval); ok {
		return formatElapsed(elapsed)
	}
	return ""
}

// runningBar draws an unfinished interval up to the estimated source time and
// ends it with the in-progress marker so it reads differently from a measured bar.
func runningBar(interval *inspectedInterval, elapsed, origin, end float64, columns int) string {
	shown := *interval
	estimated := *interval.Start + elapsed
	shown.End = &estimated
	bar := []rune(intervalBar(&shown, origin, end, columns))
	for i := len(bar) - 1; i >= 0; i-- {
		if bar[i] != ' ' {
			bar[i] = '…'
			break
		}
	}
	return string(bar)
}
