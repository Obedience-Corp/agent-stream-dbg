package visualizer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"sort"
	"strings"
)

const detailChunkBytes = 16 << 10

type detailBudget struct{ nodes, bytes int }

func boundedDetail(value any, budget *detailBudget, depth int) any {
	if budget.nodes <= 0 || budget.bytes <= 0 || depth > 5 {
		return "… more evidence retained"
	}
	budget.nodes--
	budget.bytes -= 32
	switch value := value.(type) {
	case string:
		limit := min(512, max(0, budget.bytes))
		text := observerClip(value, limit)
		budget.bytes -= len(text)
		return text
	case map[string]any:
		keys := make([]string, 0, min(len(value), 64))
		for key := range value {
			if len(keys) >= 64 {
				break
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make(map[string]any, min(len(keys), 16))
		for _, key := range keys[:min(len(keys), 16)] {
			out[observerClip(key, 64)] = boundedDetail(value[key], budget, depth+1)
			if budget.bytes <= 0 {
				break
			}
		}
		if len(value) > len(out) {
			out["…"] = "additional fields retained in raw source"
		}
		return out
	case []any:
		out := make([]any, 0, min(len(value), 16))
		for _, item := range value[:min(len(value), 16)] {
			out = append(out, boundedDetail(item, budget, depth+1))
			if budget.bytes <= 0 {
				break
			}
		}
		if len(value) > len(out) {
			out = append(out, "… additional items retained in raw source")
		}
		return out
	default:
		return value
	}
}
func sourcePretty(raw []byte, width int) []string {
	// Indenting a deeply nested tiny JSON document can amplify it quadratically.
	// Shallow complete chunks are pretty printed; others wrap bounded original bytes.
	depth, maxDepth := 0, 0
	quoted, escaped := false, false
	for _, c := range raw {
		if quoted {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
			continue
		}
		if c == '{' || c == '[' {
			depth++
			maxDepth = max(maxDepth, depth)
		} else if c == '}' || c == ']' {
			depth--
		}
	}
	if maxDepth <= 16 && json.Valid(raw) {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, raw, "", "  "); err == nil {
			return strings.Split(observerClean(pretty.String()), "\n")
		}
	}
	out := []string{}
	for len(raw) > 0 {
		n := min(len(raw), max(16, width))
		out = append(out, observerClean(string(raw[:n])))
		raw = raw[n:]
	}
	return out
}
func (m *ObserverModel) openObserverDetails() {
	m.detailRaw = false
	m.detailRawOffset = 0
	m.detailScroll = 0
	m.detailEvidence = 0
	m.detailInterval = ""
	if m.pane == PaneTimeline {
		report := m.IntervalReport()
		if len(report.Intervals) == 0 {
			m.details = false
			return
		}
		interval := report.Intervals[min(m.eventRow, len(report.Intervals)-1)]
		m.detailInterval = intervalIdentity(interval.Runtime, interval.ID)
		return
	}
	records := m.selectedRecords()
	if len(records) == 0 {
		m.details = false
		return
	}
	m.detailRecordOffset = records[min(m.eventRow, len(records)-1)].Offset
}
func (m *ObserverModel) detailSource() (map[string]any, *observedRecord) {
	records := m.selectedRecords()
	offset := m.detailRecordOffset
	if m.detailInterval != "" {
		report := m.IntervalReport()
		for _, interval := range report.Intervals {
			if intervalIdentity(interval.Runtime, interval.ID) != m.detailInterval {
				continue
			}
			var duration *float64
			if value, ok := intervalDuration(interval); ok {
				duration = &value
			}
			if len(interval.SourceOffsets) > 0 {
				offset = interval.SourceOffsets[min(m.detailEvidence, len(interval.SourceOffsets)-1)]
			}
			data := map[string]any{"span_id": interval.ID, "parent_span_id": interval.Parent, "scope": m.selected, "runtime_id": interval.Runtime, "response_id": interval.Response, "stage": interval.Stage, "lane": interval.Lane, "segment_index": interval.Segment, "attempt_id": interval.Attempt, "start_ms": interval.Start, "end_ms": interval.End, "duration_ms": duration, "status": interval.Status, "relative_clock": interval.Relative, "start_from_final": interval.StartFinal, "end_from_final": interval.EndFinal, "uncorrelated": interval.Uncorrelated, "invalid_timing": interval.Invalid, "issues": interval.Issues, "source_offsets": interval.SourceOffsets, "gap_cause": interval.Cause}
			for _, record := range records {
				if record.Offset == offset {
					data["source_context"] = observerNormalizedContext(record.Event)
					return data, record
				}
			}
			data["raw_status"] = "selected observation outside retained page; Ctrl+R reload turn from source"
			return data, nil
		}
		return map[string]any{"status": "selected interval outside retained page; Ctrl+R reload turn from source"}, nil
	}
	for _, record := range records {
		if record.Offset == offset {
			e := record.Event
			normalized := map[string]any{}
			for _, key := range []string{"capture_id", "runtime_id", "session_id", "conversation_epoch", "turn_id", "parent_turn_id", "response_id", "persona_id", "job_id", "span_id", "parent_span_id", "step", "span_state", "status", "start_ms", "end_ms", "mono_ms", "turn_start_ms", "segment_index", "attempt_id", "projection_unavailable", "projection_limited"} {
				if value, ok := e.Fields[key]; ok {
					normalized[key] = boundedDetail(value, &detailBudget{nodes: 32, bytes: 2 << 10}, 0)
				}
			}
			return map[string]any{"name": e.Name, "kind": e.Kind, "source_timestamp": observerSourceTime(e, true), "seq": e.Seq, "scope": record.Scope, "source_offset": record.Offset, "issue": record.Issue, "normalized": normalized, "fields": boundedDetail(e.Fields, &detailBudget{nodes: 128, bytes: 8 << 10}, 0)}, record
		}
	}
	return map[string]any{"status": "selected event outside retained page; Ctrl+R reload turn from source"}, nil
}
func (m *ObserverModel) renderDetails(width, rows int) []string {
	data, record := m.detailSource()
	mode := "parsed normalized"
	body := []string{}
	if m.detailRaw {
		mode = "raw source JSON (formatted for display)"
		if record == nil {
			body = []string{"Raw observation is outside this page; Ctrl+R reload the selected turn"}
		} else {
			raw := record.Event.Raw
			start := min(m.detailRawOffset, max(0, (len(raw)-1)/detailChunkBytes*detailChunkBytes))
			end := min(len(raw), start+detailChunkBytes)
			body = append(body, fmt.Sprintf("source offset %d · bytes %d–%d/%d · %s", record.Offset, start, end, len(raw), observerSourceTime(record.Event, true)))
			body = append(body, sourcePretty(raw[start:end], width)...)
			if len(raw) > detailChunkBytes {
				body = append(body, "PgUp/PgDown selects raw byte chunks; ↑/↓ scrolls this chunk")
			}
		}
	} else {
		raw, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			body = []string{"Normalized data cannot be encoded; original raw remains available"}
		} else {
			body = strings.Split(string(raw), "\n")
		}
	}
	lines := []string{fmt.Sprintf("Details · %s · p parsed / r raw · ←/→ source observation · Esc back", mode)}
	available := max(1, rows-1)
	start := min(m.detailScroll, max(0, len(body)-available))
	lines = append(lines, body[start:min(len(body), start+available)]...)
	return lines
}

func appendIntervalOffset(interval *inspectedInterval, offset int64) {
	if len(interval.SourceOffsets) < 8 {
		interval.SourceOffsets = append(interval.SourceOffsets, offset)
		return
	}
	copy(interval.SourceOffsets[1:], interval.SourceOffsets[2:])
	interval.SourceOffsets[len(interval.SourceOffsets)-1] = offset
}

func observerNormalizedContext(event *events.Event) map[string]any {
	out := make(map[string]any)
	for _, key := range []string{"capture_id", "runtime_id", "session_id", "conversation_epoch", "turn_id", "parent_turn_id", "response_id", "persona_id", "job_id", "mono_ms", "turn_start_ms"} {
		if value, ok := event.Fields[key]; ok {
			out[key] = boundedDetail(value, &detailBudget{nodes: 32, bytes: 2 << 10}, 0)
		}
	}
	return out
}
