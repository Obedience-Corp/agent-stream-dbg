package visualizer

import (
	"fmt"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
)

// clockAnchor pairs the source monotonic offset of the latest record received
// for a runtime with the local time the viewer received it.
type clockAnchor struct {
	sourceMS   float64
	receivedAt time.Time
}

const maxClockAnchors = 64

// followQuietPeriod is how long a followed file may go without new bytes
// before running spans stop counting. A producer can legitimately write
// nothing for the length of one long model request.
const followQuietPeriod = 3 * time.Minute

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
		oldest := ""
		for key, anchor := range m.clockAnchors {
			if oldest == "" || anchor.receivedAt.Before(m.clockAnchors[oldest].receivedAt) {
				oldest = key
			}
		}
		delete(m.clockAnchors, oldest)
	}
	m.clockAnchors[runtime] = clockAnchor{sourceMS: mono, receivedAt: m.clock()}
}

const maxSequenceGaps = 64

// noteSourceProgress records sequence gaps and turn starts so an estimate can
// tell when a span's end may have been lost.
func (m *ObserverModel) noteSourceProgress(event *events.Event, previousSeq int) {
	if event.Seq > previousSeq+1 && previousSeq > 0 && m.store.lastSeq == event.Seq {
		if len(m.sequenceGaps) == maxSequenceGaps {
			m.sequenceGaps = m.sequenceGaps[1:]
		}
		m.sequenceGaps = append(m.sequenceGaps, previousSeq)
	}
	runtime := boundedIdentity(event.StringField("runtime_id"))
	if runtime != "" && event.StringField("span_role") == "turn" && event.StringField("span_state") == "start" {
		if m.turnStarts == nil {
			m.turnStarts = make(map[string]int)
		}
		if _, known := m.turnStarts[runtime]; known || len(m.turnStarts) < maxClockAnchors {
			m.turnStarts[runtime] = max(m.turnStarts[runtime], event.Seq)
		}
	}
}

// noteFileGrowth marks when a followed file last grew past its size at open.
func (m *ObserverModel) noteFileGrowth(frame transport.Frame) {
	if m.openedSize >= 0 && frame.Offset+frame.Size > m.openedSize {
		m.grewAt = m.clock()
	}
}

// liveSource reports whether records can still arrive for the retained view:
// a followed file or live feed that has neither ended nor disconnected.
// A live feed is live while connected. A followed file is live only after it
// has grown since it was opened, and until it has been quiet for
// followQuietPeriod.
func (m *ObserverModel) liveSource() bool {
	if !m.following || !m.attached || !m.ingesting || m.sourceEnded || m.page != nil {
		return false
	}
	if m.openedSize < 0 {
		return true
	}
	return !m.grewAt.IsZero() && m.clock().Sub(m.grewAt) < followQuietPeriod
}

// lostEnd reports whether the source skipped records after the interval began
// and a later turn has started since, so the interval's end may have been lost.
func (m *ObserverModel) lostEnd(interval *inspectedInterval) bool {
	started := m.turnStarts[interval.Runtime]
	for _, gap := range m.sequenceGaps {
		if interval.FirstSeq <= gap && started > gap {
			return true
		}
	}
	return false
}

// runningElapsedMS estimates how long an unfinished interval has been running
// on its own runtime clock: the latest received source offset plus local time
// since receipt, minus the interval start. It is a display estimate only and
// is never stored as a measurement.
func (m *ObserverModel) runningElapsedMS(interval *inspectedInterval) (float64, bool) {
	if !m.liveSource() || interval.Status != "running" || interval.Start == nil || interval.End != nil || interval.Ended ||
		interval.Invalid || interval.Uncorrelated || interval.Relative || interval.Gap || interval.StartFinal || m.lostEnd(interval) {
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

// runningEstimates estimates every unfinished interval in one view. An interval
// stops counting once its parent span has ended, since its own end was lost.
func (m *ObserverModel) runningEstimates(intervals []*inspectedInterval) map[*inspectedInterval]float64 {
	estimates := map[*inspectedInterval]float64{}
	if !m.liveSource() {
		return estimates
	}
	ended := map[string]bool{}
	for _, interval := range intervals {
		if interval.End != nil || interval.Ended {
			ended[intervalIdentity(interval.Runtime, interval.ID)] = true
		}
	}
	for _, interval := range intervals {
		if interval.Parent != "" && ended[intervalIdentity(interval.Runtime, interval.Parent)] {
			continue
		}
		if elapsed, ok := m.runningElapsedMS(interval); ok {
			estimates[interval] = elapsed
		}
	}
	return estimates
}

// runningLabel is the live elapsed estimate for an unfinished interval, or ""
// when the source is static or the estimate is unavailable.
func (m InteractiveModel) runningLabel(interval *inspectedInterval) string {
	if elapsed, ok := m.flowEstimates[interval]; ok {
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
