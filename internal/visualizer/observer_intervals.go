package visualizer

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
)

type inspectedInterval struct {
	ID, Parent, Stage, Lane                   string
	Attempt, Status, Runtime, Response, Cause string
	Role, Reference                           string
	Segment                                   *int
	Start, End                                *float64
	Relative, Gap, Invalid                    bool
	FinalOnly, Uncorrelated                   bool
	StartFinal, EndFinal                      bool
	Issues                                    []string
	SourceOffsets                             []int64
}

func (i *inspectedInterval) issue(message string) {
	if len(i.Issues) < 8 {
		i.Issues = append(i.Issues, observerClip(message, 256))
	}
}
func intervalBound(fields map[string]any, key string) (*float64, bool) {
	value, exists := fields[key]
	if !exists || value == nil {
		return nil, false
	}
	if n, ok := finiteNumber(value); ok {
		return &n, false
	}
	return nil, true
}
func intervalSegment(value any) *int {
	n, ok := finiteNumber(value)
	if !ok || n < 0 || n > 1<<31 || n != math.Trunc(n) {
		return nil
	}
	i := int(n)
	return &i
}
func mergeIntervalBound(interval *inspectedInterval, start bool, value *float64, final bool) {
	if value == nil {
		return
	}
	target := &interval.End
	if start {
		target = &interval.Start
	}
	if *target == nil {
		n := *value
		*target = &n
		if start {
			interval.StartFinal = final
		} else {
			interval.EndFinal = final
		}
		return
	}
	if math.Abs(**target-*value) > 1 {
		if !final || (start && interval.StartFinal) || (!start && interval.EndFinal) {
			interval.Uncorrelated = true
		}
		interval.issue(fmt.Sprintf("conflicting %s evidence: retained %.3fms, incoming %.3fms", map[bool]string{true: "start", false: "end"}[start], **target, *value))
	}
}
func intervalFromFields(fields map[string]any) *inspectedInterval {
	start, badStart := intervalBound(fields, "start_ms")
	end, badEnd := intervalBound(fields, "end_ms")
	stringValue := func(key string) string { value, _ := fields[key].(string); return boundedIdentity(value) }
	status := stringValue("status")
	if status == "" {
		status = "unknown"
	}
	relative, _ := fields["relative"].(bool)
	gap, _ := fields["is_gap"].(bool)
	invalid, _ := fields["invalid_timing"].(bool)
	result := &inspectedInterval{Role: stringValue("span_role"), Reference: stringValue("boundary_reference"), Stage: stringValue("stage"), Lane: stringValue("lane"), Parent: stringValue("parent_span_id"), Attempt: stringValue("attempt_id"), Status: status, Segment: intervalSegment(fields["segment_index"]), Start: start, End: end, Relative: relative, Gap: gap, Cause: stringValue("cause"), Invalid: badStart || badEnd || invalid}
	if result.Invalid {
		result.issue("invalid timing shape/non-finite measurement")
	}
	if start != nil && end != nil && (*end < *start || math.IsInf(*end-*start, 0) || math.IsNaN(*end-*start)) {
		result.Invalid = true
		result.issue("invalid timing: end precedes start")
	}
	return result
}
func sameSegment(a, b *int) bool { return a != nil && b != nil && *a == *b }

// projectObservedIntervals combines cumulative live snapshots by source-owned
// operation ID. Final aggregate evidence fills only unambiguous absent bounds;
// relative/ambiguous attempts never borrow a runtime clock or another attempt.
func projectObservedIntervals(records []*observedRecord, legacy bool) []*inspectedInterval {
	result := make([]*inspectedInterval, 0)
	operations := make(map[string]*inspectedInterval)
	type finalRow struct {
		fields map[string]any
		record *observedRecord
		index  int
	}
	final := make([]finalRow, 0)
	conflicts := map[string]bool{}
	for _, record := range records {
		if record.dedup != "" && strings.HasPrefix(record.Issue, "Capture conflict") {
			conflicts[record.dedup] = true
		}
	}
	for _, record := range records {
		e := record.Event
		if conflicts[record.dedup] || record.Issue != "" || e.StringField("projection_unavailable") != "" {
			continue
		}
		if rows, ok := e.Fields["interval_evidence"].([]any); ok {
			for index, row := range rows {
				if fields, ok := row.(map[string]any); ok {
					final = append(final, finalRow{fields, record, index})
				}
			}
		}
		if legacy {
			continue
		}
		id := boundedIdentity(e.StringField("span_id"))
		if id == "" {
			continue
		}
		runtime := boundedIdentity(e.StringField("runtime_id"))
		key := intervalIdentity(runtime, id)
		fields := e.Fields
		incoming := intervalFromFields(fields)
		incoming.Stage = boundedIdentity(e.StringField("step"))
		incoming.Runtime = runtime
		incoming.Response = boundedIdentity(e.StringField("response_id"))
		current, exists := operations[key]
		if !exists {
			incoming.ID = id
			incoming.SourceOffsets = []int64{record.Offset}
			operations[key] = incoming
			result = append(result, incoming)
			continue
		}
		appendIntervalOffset(current, record.Offset)
		if (current.Role != "" && incoming.Role != "" && current.Role != incoming.Role) || (current.Reference != "" && incoming.Reference != "" && current.Reference != incoming.Reference) || (current.Stage != "" && incoming.Stage != "" && current.Stage != incoming.Stage) || (current.Attempt != "" && incoming.Attempt != "" && current.Attempt != incoming.Attempt) || (current.Response != "" && incoming.Response != "" && current.Response != incoming.Response) || (current.Parent != "" && incoming.Parent != "" && current.Parent != incoming.Parent) || (current.Lane != "" && incoming.Lane != "" && current.Lane != incoming.Lane) || (current.Segment != nil && incoming.Segment != nil && *current.Segment != *incoming.Segment) {
			current.Uncorrelated = true
			current.issue("conflicting immutable operation identity")
			continue
		}
		if current.Role == "" {
			current.Role = incoming.Role
		}
		if current.Reference == "" {
			current.Reference = incoming.Reference
		}
		if current.Parent == "" {
			current.Parent = incoming.Parent
		}
		if current.Attempt == "" {
			current.Attempt = incoming.Attempt
		}
		if current.Segment == nil {
			current.Segment = incoming.Segment
		}
		if current.Stage == "" {
			current.Stage = incoming.Stage
		}
		if current.Lane == "" {
			current.Lane = incoming.Lane
		}

		mergeIntervalBound(current, true, incoming.Start, false)
		mergeIntervalBound(current, false, incoming.End, false)
		if incoming.Status != "running" && incoming.Status != "unknown" {
			if current.Status != "running" && current.Status != "unknown" && current.Status != incoming.Status {
				current.issue("conflicting terminal outcomes")
			} else {
				current.Status = incoming.Status
			}
		}
		current.Invalid = current.Invalid || incoming.Invalid
		for _, issue := range incoming.Issues {
			current.issue(issue)
		}
		if current.Response == "" {
			current.Response = incoming.Response
		}

	}
	derived := map[string]*inspectedInterval{}
	for _, row := range final {
		incoming := intervalFromFields(row.fields)
		incoming.Runtime = boundedIdentity(row.record.Event.StringField("runtime_id"))
		incoming.Response = boundedIdentity(row.record.Event.StringField("response_id"))
		incoming.ID = fmt.Sprintf("final:%d:%d", row.record.Event.Seq, row.index)
		incoming.SourceOffsets = []int64{row.record.Offset}
		incoming.FinalOnly = true
		ambiguous, _ := row.fields["ambiguous_attempts"].(bool)
		if incoming.Gap {
			ambiguous = false
		}
		candidates := make([]*inspectedInterval, 0, 2)
		if !incoming.Relative && !ambiguous && !incoming.Gap {
			for _, current := range result {
				if current.Runtime == incoming.Runtime && current.Stage == incoming.Stage && sameSegment(current.Segment, incoming.Segment) && !current.Relative && (incoming.Response == "" || current.Response == incoming.Response) {
					candidates = append(candidates, current)
				}
			}
		}
		if incoming.Invalid {
			incoming.issue("invalid final aggregate not merged")
			incoming.StartFinal = incoming.Start != nil
			incoming.EndFinal = incoming.End != nil
			result = append(result, incoming)
			continue
		}
		if len(candidates) == 1 {
			current := candidates[0]
			if current.Status == "failed" || current.Status == "cancelled" || current.Status == "interrupted" {
				incoming.Uncorrelated = true
				incoming.issue("final aggregate retained separately from unsuccessful attempt")
			} else {
				mergeIntervalBound(current, true, incoming.Start, true)
				mergeIntervalBound(current, false, incoming.End, true)
				continue
			}
		}
		if ambiguous || len(candidates) > 1 {
			incoming.Uncorrelated = true
			incoming.issue("aggregate attempt identity unavailable; not merged")
		}
		incoming.StartFinal = incoming.Start != nil
		incoming.EndFinal = incoming.End != nil
		if incoming.Segment != nil {
			key := fmt.Sprintf("%s/%s/%t/%t/%s/%d/%s/%s/%s", incoming.Runtime, incoming.Response, incoming.Relative, incoming.Gap, incoming.Stage, *incoming.Segment, incoming.Cause, formatMeasured(incoming.Start), formatMeasured(incoming.End))
			if previous := derived[key]; previous != nil {
				if len(previous.SourceOffsets) < 8 {
					previous.SourceOffsets = append(previous.SourceOffsets, row.record.Offset)
				}
				continue
			}
			derived[key] = incoming
		}
		if incoming.Start == nil && incoming.End == nil && !incoming.Invalid {
			continue
		}
		result = append(result, incoming)
	}
	for _, interval := range result {
		if interval.Start != nil && interval.End != nil && (*interval.End < *interval.Start || math.IsInf(*interval.End-*interval.Start, 0) || math.IsNaN(*interval.End-*interval.Start)) {
			interval.Invalid = true
			interval.issue("invalid timing: end precedes start")
		}
		if interval.Start == nil {
			interval.issue("start measurement unavailable")
		}
		if interval.End == nil {
			interval.issue("end measurement unavailable")
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Runtime != b.Runtime {
			return a.Runtime < b.Runtime
		}
		if a.Relative != b.Relative {
			return !a.Relative
		}
		if a.Start == nil {
			return false
		}
		if b.Start == nil {
			return true
		}
		if *a.Start != *b.Start {
			return *a.Start < *b.Start
		}
		return strings.Compare(a.ID, b.ID) < 0
	})
	return result
}

func intervalDuration(interval *inspectedInterval) (float64, bool) {
	if interval.Invalid || interval.Start == nil || interval.End == nil {
		return 0, false
	}
	value := *interval.End - *interval.Start
	return value, !math.IsInf(value, 0) && !math.IsNaN(value)
}
func intervalSourceEvent(e *events.Event) bool {
	return e.StringField("span_id") != "" || e.Fields["interval_evidence"] != nil
}
