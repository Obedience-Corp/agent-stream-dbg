package visualizer

import (
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"
)

func intervalTestRecord(seq int, fields map[string]any) *observedRecord {
	return &observedRecord{Event: &events.Event{Seq: seq, Fields: fields}, Offset: int64(seq * 100)}
}
func liveTestFields(start, end any) map[string]any {
	return map[string]any{"runtime_id": "runtime", "turn_id": "turn", "response_id": "response", "span_id": "operation", "step": "compute", "span_state": "update", "status": "running", "segment_index": 0.0, "attempt_id": "attempt", "start_ms": start, "end_ms": end}
}
func finalTestRecord(seq int, start, end any, ambiguous bool) *observedRecord {
	return intervalTestRecord(seq, map[string]any{"runtime_id": "runtime", "response_id": "response", "interval_evidence": []any{map[string]any{"stage": "compute", "segment_index": 0.0, "start_ms": start, "end_ms": end, "ambiguous_attempts": ambiguous}}})
}
func TestIntervalSourceAuthorityAndFinalMissingOnly(t *testing.T) {
	records := []*observedRecord{intervalTestRecord(1, liveTestFields(0.0, nil)), finalTestRecord(2, 10.0, 20.0, false)}
	rows := projectObservedIntervals(records, false)
	if len(rows) != 1 || rows[0].Start == nil || *rows[0].Start != 0 || rows[0].End == nil || *rows[0].End != 20 || !rows[0].EndFinal || len(rows[0].Issues) == 0 {
		t.Fatalf("source authority/fill lost: %+v", rows)
	}
	records = append(records, intervalTestRecord(3, liveTestFields(0.0, 30.0)))
	rows = projectObservedIntervals(records, false)
	if *rows[0].End != 30 || rows[0].EndFinal {
		t.Fatal("explicit live end did not remain authoritative before final reconciliation")
	}
}
func TestIntervalInvalidAggregateAndImmutableConflicts(t *testing.T) {
	rows := projectObservedIntervals([]*observedRecord{intervalTestRecord(1, liveTestFields(0.0, 10.0)), finalTestRecord(2, 2.0, map[string]any{"wrong": true}, false)}, false)
	if len(rows) != 2 || !rows[1].Invalid || *rows[0].Start != 0 || *rows[0].End != 10 {
		t.Fatal("invalid final aggregate disappeared or changed source bounds")
	}
	for _, key := range []string{"response_id", "parent_span_id"} {
		t.Run(key, func(t *testing.T) {
			first := liveTestFields(0.0, nil)
			first[key] = "first"
			second := liveTestFields(100.0, 200.0)
			second[key] = "different"
			rows := projectObservedIntervals([]*observedRecord{intervalTestRecord(1, first), intervalTestRecord(2, second)}, false)
			if len(rows) != 1 || rows[0].End != nil || len(rows[0].Issues) == 0 {
				t.Fatal("contradictory immutable identity merged")
			}
		})
	}
}
func TestIntervalFallbackMissingAndInvalidTiming(t *testing.T) {
	first := liveTestFields(1.0, 2.0)
	first["status"] = "failed"
	second := liveTestFields(3.0, 5.0)
	second["span_id"] = "fallback"
	second["attempt_id"] = "fallback-attempt"
	second["status"] = "completed"
	rows := projectObservedIntervals([]*observedRecord{intervalTestRecord(1, first), intervalTestRecord(2, second), finalTestRecord(3, 1.0, 5.0, true)}, false)
	statuses := map[string]string{}
	for _, row := range rows {
		if row.Attempt != "" {
			statuses[row.Attempt] = row.Status
		}
	}
	if len(rows) != 3 || statuses["attempt"] != "failed" || statuses["fallback-attempt"] != "completed" {
		t.Fatalf("fallback attempts reconstructed/overwritten: %v", statuses)
	}

	inverted := liveTestFields(10.0, 0.0)
	rows = projectObservedIntervals([]*observedRecord{intervalTestRecord(1, inverted)}, false)
	if _, ok := intervalDuration(rows[0]); ok {
		t.Fatal("inverted timing acquired a duration")
	}
	missing := liveTestFields(nil, 0.0)
	rows = projectObservedIntervals([]*observedRecord{intervalTestRecord(1, missing)}, false)
	if rows[0].Start != nil || rows[0].End == nil || *rows[0].End != 0 {
		t.Fatal("missing changed into measured zero")
	}
}
func TestLegacyMetricsWithoutUserAnchorsStaySeparate(t *testing.T) {
	store := newObserverStore()
	store.legacy = true
	for i := 1; i <= 2; i++ {
		e := finalTestRecord(i, 0.0, float64(i), false).Event
		e.Fields["local_terminal_boundary"] = true
		store.add(e, transport.Frame{Data: []byte("{}"), Offset: int64(i * 100)}, false)
	}
	groups := store.groups()
	if len(groups) != 2 || groups[0].Key == groups[1].Key || !strings.HasPrefix(groups[0].Key, "legacy-local/") {
		t.Fatal("unanchored legacy metrics overlaid")
	}
}

func TestIntervalRepeatedFinalGapPartitionsDoNotDuplicate(t *testing.T) {
	row := map[string]any{"stage": "silence", "segment_index": 0.0, "start_ms": 10.0, "end_ms": 20.0, "is_gap": true, "cause": "waiting"}
	records := []*observedRecord{intervalTestRecord(1, map[string]any{"interval_evidence": []any{row}}), intervalTestRecord(2, map[string]any{"interval_evidence": []any{row}})}
	rows := projectObservedIntervals(records, false)
	if len(rows) != 1 || len(rows[0].SourceOffsets) != 2 {
		t.Fatal("repeated cumulative gap became two operations")
	}
}
func TestIntervalReportUsesUnionsAndKeepsZeroPoints(t *testing.T) {
	m, _ := newTestObserver(t, "")
	m.intervalsEnabled = true
	m.selected = "runtime/turn/turn"
	for seq, tuple := range [][3]any{{"a", 0.0, 10.0}, {"b", 5.0, 15.0}, {"c", 7.0, 12.0}, {"point", 0.0, 0.0}} {
		fields := liveTestFields(tuple[1], tuple[2])
		fields["step"] = tuple[0]
		fields["span_id"] = tuple[0]
		fields["status"] = "completed"
		m.store.add(&events.Event{Seq: seq + 1, Fields: fields}, transport.Frame{Data: []byte("{}")}, false)
	}
	report := m.IntervalReport()
	// a∩b=[5,10], a∩c=[7,10], b∩c=[7,12]: union=[5,12], not13ms pairwise sum.
	if report.SegmentOverlapMS != 7 {
		t.Fatalf("overlap was summed: %f", report.SegmentOverlapMS)
	}
	if len(report.Intervals) != 4 || !strings.Contains(m.View(), "0.000ms") {
		t.Fatal("measured zero milestone hidden")
	}
}

func TestIntervalReportSeparatesRelativeAndRuntimeDomains(t *testing.T) {
	m, _ := newTestObserver(t, "")
	m.intervalsEnabled = true
	m.height = 100
	m.selected = "runtime/turn/turn"
	compute := liveTestFields(1000.0, 1100.0)
	compute["status"] = "completed"
	output := liveTestFields(1050.0, 1150.0)
	output["span_id"] = "output"
	output["step"] = "output"
	output["status"] = "completed"
	for i, fields := range []map[string]any{compute, output} {
		m.store.add(&events.Event{Seq: i + 1, Fields: fields}, transport.Frame{Data: []byte("{}")}, false)
	}
	rows := []any{map[string]any{"stage": "gap", "segment_index": 0.0, "start_ms": 1100.0, "end_ms": 1200.0, "is_gap": true, "cause": "absolute"}, map[string]any{"stage": "gap", "segment_index": 0.0, "start_ms": 10.0, "end_ms": 20.0, "is_gap": true, "relative": true, "cause": "relative"}}
	m.store.add(&events.Event{Seq: 3, Fields: map[string]any{"runtime_id": "runtime", "turn_id": "turn", "interval_evidence": rows}}, transport.Frame{Data: []byte("{}")}, false)
	report := m.IntervalReport()
	if len(report.Domains) != 2 || report.GapTotalMS != 100 || report.Domains[1].GapTotalMS != 10 || report.SegmentOverlapMS != 50 {
		t.Fatalf("clock domains mixed: %+v", report)
	}
	if !strings.Contains(m.View(), "Clock domains shown separately") {
		t.Fatal("clock distinction hidden")
	}
}
func TestIntervalReportSourceFirstOutputContradictsFinal(t *testing.T) {
	m, _ := newTestObserver(t, "")
	m.intervalsEnabled = true
	m.selected = "runtime/turn/turn"
	input := liveTestFields(0.0, 20.0)
	input["span_id"] = "input"
	input["step"] = "recognition"
	input["span_role"] = "input"
	input["boundary_reference"] = "input_end"
	delete(input, "segment_index")
	output := liveTestFields(200.0, 400.0)
	output["span_id"] = "output"
	output["step"] = "sink"
	output["span_role"] = "output"
	for i, fields := range []map[string]any{input, output} {
		m.store.add(&events.Event{Seq: i + 1, Fields: fields}, transport.Frame{Data: []byte("{}")}, false)
	}
	m.store.add(&events.Event{Seq: 3, Fields: map[string]any{"runtime_id": "runtime", "turn_id": "turn", "first_output_measurement_ms": 999.0, "first_output_reference": "input_end", "first_output_origin": "final input"}}, transport.Frame{Data: []byte("{}")}, false)
	report := m.IntervalReport()
	if report.FirstOutputDelayMS == nil || *report.FirstOutputDelayMS != 180 || len(report.Issues) == 0 || !strings.Contains(m.View(), "conflicts with source") {
		t.Fatal("final metric replaced explicit source latency")
	}
}
func TestIntervalStatusAndConflictVisibleAtNormalWidth(t *testing.T) {
	interval := intervalFromFields(map[string]any{"stage": "a very long stage label that must not hide outcomes", "start_ms": 0.0, "end_ms": 10.0, "status": "interrupted"})
	interval.ID = "id"
	interval.Runtime = "runtime"
	interval.issue("conflicting source evidence")
	domain := analyseClockIntervals("runtime/false", []*inspectedInterval{interval})
	report := ObserverIntervalReport{Intervals: []*inspectedInterval{interval}, Domains: []ObserverClockReport{domain}}
	view := strings.Join(renderObserverIntervals(report, 100, 0, 20), "\n")
	if !strings.Contains(view, "interrupted") || !strings.Contains(view, "! conflicting") {
		t.Fatalf("status/conflict clipped: %s", view)
	}
}

func TestUncorrelatedOutputDoesNotCreateTrustedGap(t *testing.T) {
	a := intervalFromFields(map[string]any{"start_ms": 0.0, "end_ms": 10.0})
	a.Runtime = "runtime"
	a.Role = "output"
	a.Uncorrelated = true
	b := intervalFromFields(map[string]any{"start_ms": 20.0, "end_ms": 30.0})
	b.Runtime = "runtime"
	b.Role = "output"
	domain := analyseClockIntervals("runtime", []*inspectedInterval{a, b})
	if domain.GapKnown || domain.GapTotalMS != 0 {
		t.Fatal("uncorrelated output fabricated a trusted gap")
	}
	row := intervalFromFields(map[string]any{"start_ms": -1e308, "end_ms": 1e308})
	if _, known := intervalDuration(row); known || !row.Invalid {
		t.Fatal("finite endpoints produced infinite duration")
	}
}

func TestObserverParsedAndRawIntervalDetailsKeepFrozenSelection(t *testing.T) {
	m, _ := newTestObserver(t, "")
	m.intervalsEnabled = true
	m.pane = PaneTimeline
	m.height = 100
	m.selected = "runtime/turn/turn"
	fields := liveTestFields(0.0, nil)
	m.store.add(&events.Event{Seq: 1, Fields: fields}, transport.Frame{Data: []byte(`{"original":"start"}`), Raw: []byte(`{"original":"start"}`), Offset: 10}, false)
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.details || m.detailInterval == "" {
		t.Fatal("interval details did not freeze operation identity")
	}
	view := m.View()
	if !strings.Contains(view, "parsed normalized") || !strings.Contains(view, "start_ms") || !strings.Contains(view, `"end_ms": null`) {
		t.Fatal("parsed details lost null/zero fields")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if !strings.Contains(m.View(), "original") {
		t.Fatal("original source raw unavailable")
	}
	more := liveTestFields(0.0, 2.0)
	more["span_id"] = "aaa"
	m.store.add(&events.Event{Seq: 2, Fields: more}, transport.Frame{Data: []byte("{}"), Offset: 20}, false)
	if m.detailInterval != intervalIdentity("runtime", "operation") || !strings.Contains(m.View(), "original") {
		t.Fatal("ingest moved frozen detail selection")
	}
}
func TestObserverOpaqueIdentityCannotCollideOnSlash(t *testing.T) {
	first := &events.Event{Fields: map[string]any{"runtime_id": "a/turn/b", "turn_id": "c"}}
	second := &events.Event{Fields: map[string]any{"runtime_id": "a", "turn_id": "b/turn/c"}}
	if eventScope(first) == eventScope(second) || intervalIdentity("a/b", "c") == intervalIdentity("a", "b/c") {
		t.Fatal("opaque source IDs collided on separators")
	}
}
