package visualizer

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type measuredRange struct{ start, end float64 }

// ObserverIntervalReport retains measured values, never summed nested latency.
type ObserverClockReport struct {
	Key, Label, Runtime          string
	Relative, ClockKnown         bool
	Intervals                    []*inspectedInterval
	GapTotalMS, SegmentOverlapMS float64
	GapKnown, OverlapKnown       bool
	GapByCause, OverlapByStages  map[string]float64
	Limited                      bool
}
type ObserverIntervalReport struct {
	Scope                                                                 string
	Intervals                                                             []*inspectedInterval
	Domains                                                               []ObserverClockReport
	PrimaryDomain                                                         string
	TurnDurationMS, FirstOutputDelayMS, FinalOutputDelayMS                *float64
	FirstOutputOrigin, FirstOutputEvidence, FinalOutputOrigin, OutputSink string
	GapTotalMS, SegmentOverlapMS                                          float64
	GapKnown, OverlapKnown                                                bool
	GapByCause, OverlapByStages                                           map[string]float64
	Limited, Partial                                                      bool
	Issues                                                                []string
}

func unionRanges(ranges []measuredRange) []measuredRange {
	if len(ranges) == 0 {
		return nil
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	out := ranges[:0]
	for _, r := range ranges {
		if len(out) == 0 || r.start > out[len(out)-1].end {
			out = append(out, r)
		} else if r.end > out[len(out)-1].end {
			out[len(out)-1].end = r.end
		}
	}
	return out
}
func rangeDuration(ranges []measuredRange) float64 {
	total := 0.0
	for _, r := range unionRanges(ranges) {
		total += r.end - r.start
		if math.IsInf(total, 0) || math.IsNaN(total) {
			return math.NaN()
		}
	}
	return total
}
func intersectRanges(a, b []measuredRange) []measuredRange {
	out := make([]measuredRange, 0)
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		start, end := math.Max(a[i].start, b[j].start), math.Min(a[i].end, b[j].end)
		if end > start {
			out = append(out, measuredRange{start, end})
		}
		if a[i].end < b[j].end {
			i++
		} else {
			j++
		}
	}
	return out
}
func intervalClockKey(interval *inspectedInterval) string {
	return fmt.Sprintf("%s/%t", interval.Runtime, interval.Relative)
}
func analyseClockIntervals(key string, intervals []*inspectedInterval) ObserverClockReport {
	report := ObserverClockReport{Key: key, Intervals: intervals, GapByCause: map[string]float64{}, OverlapByStages: map[string]float64{}}
	if len(intervals) == 0 {
		return report
	}
	report.Runtime = intervals[0].Runtime
	report.Relative = intervals[0].Relative
	report.ClockKnown = report.Relative || report.Runtime != ""
	report.Label = "runtime " + report.Runtime
	if report.Relative {
		report.Label = "turn-relative final evidence; runtime origin unavailable"
	} else if !report.ClockKnown {
		report.Label = "source clock domain unavailable"
		return report
	}
	gaps := []measuredRange{}
	gapCauses := map[string][]measuredRange{}
	outputs := []measuredRange{}
	stages := map[string]map[string][]measuredRange{}
	for _, interval := range intervals {
		if interval.Uncorrelated {
			continue
		}
		if _, known := intervalDuration(interval); !known {
			continue
		}
		if interval.Role == "output" && !interval.StartFinal && !interval.EndFinal {
			outputs = append(outputs, measuredRange{*interval.Start, *interval.End})
		}
		if interval.Gap {
			report.GapKnown = true
			r := measuredRange{*interval.Start, *interval.End}
			gaps = append(gaps, r)
			if len(gapCauses) < 64 || gapCauses[interval.Cause] != nil {
				gapCauses[interval.Cause] = append(gapCauses[interval.Cause], r)
			} else {
				report.Limited = true
			}
			continue
		}
		if interval.Segment == nil || interval.Uncorrelated {
			continue
		}
		segment := fmt.Sprint(*interval.Segment)
		byStage := stages[segment]
		if byStage == nil {
			byStage = map[string][]measuredRange{}
			stages[segment] = byStage
		}
		if len(byStage) >= 16 && byStage[interval.Stage] == nil {
			report.Limited = true
			continue
		}
		byStage[interval.Stage] = append(byStage[interval.Stage], measuredRange{*interval.Start, *interval.End})
	}
	if !report.GapKnown && len(outputs) > 1 {
		report.GapKnown = true
		sort.Slice(outputs, func(i, j int) bool { return outputs[i].start < outputs[j].start })
		end := outputs[0].end
		for _, next := range outputs[1:] {
			if next.start > end {
				gaps = append(gaps, measuredRange{end, next.start})
			}
			end = math.Max(end, next.end)
		}
	}
	report.GapTotalMS = rangeDuration(gaps)
	if math.IsNaN(report.GapTotalMS) {
		report.GapTotalMS = 0
		report.GapKnown = false
		report.Limited = true
	}
	for cause, ranges := range gapCauses {
		if value := rangeDuration(ranges); !math.IsNaN(value) {
			report.GapByCause[cause] = value
		} else {
			report.Limited = true
		}
	}
	overlaps := map[string][]measuredRange{}
	allOverlap := []measuredRange{}
	overlapRows := 0
	for _, byStage := range stages {
		names := make([]string, 0, len(byStage))
		for name, ranges := range byStage {
			names = append(names, name)
			byStage[name] = unionRanges(ranges)
		}
		sort.Strings(names)
		if len(names) > 1 {
			report.OverlapKnown = true
		}
		for i, name := range names {
			for _, other := range names[i+1:] {
				label := name + " / " + other
				if len(overlaps) >= 64 && overlaps[label] == nil {
					report.Limited = true
					continue
				}
				ranges := intersectRanges(byStage[name], byStage[other])
				if overlapRows+len(ranges) > 4096 {
					report.Limited = true
					continue
				}
				overlaps[label] = append(overlaps[label], ranges...)
				allOverlap = append(allOverlap, ranges...)
				overlapRows += len(ranges)
			}
		}
	}
	for label, ranges := range overlaps {
		if value := rangeDuration(ranges); value > 0 {
			report.OverlapByStages[label] = value
		}
	}
	report.SegmentOverlapMS = rangeDuration(allOverlap)
	if math.IsNaN(report.SegmentOverlapMS) {
		report.SegmentOverlapMS = 0
		report.OverlapKnown = false
		report.Limited = true
	}
	return report
}
func (m *ObserverModel) IntervalReport() ObserverIntervalReport {
	report := ObserverIntervalReport{Scope: m.selected, GapByCause: map[string]float64{}, OverlapByStages: map[string]float64{}, Partial: m.store.evicted}
	if m.page != nil {
		report.Partial = m.page.evicted
	}
	if !m.intervalsEnabled {
		return report
	}
	records := m.selectedRecords()
	report.Intervals = projectObservedIntervals(records, m.store.legacy)
	finalReference := ""
	for _, record := range records {
		e := record.Event
		if record.Issue != "" {
			continue
		}
		if n, ok := finiteNumber(e.Fields["first_output_measurement_ms"]); ok {
			if report.FinalOutputDelayMS == nil {
				report.FinalOutputDelayMS = &n
				report.FinalOutputOrigin = boundedIdentity(e.StringField("first_output_origin"))
				finalReference = boundedIdentity(e.StringField("first_output_reference"))
				report.OutputSink = boundedIdentity(e.StringField("output_sink"))
			} else if math.Abs(*report.FinalOutputDelayMS-n) > 1 {
				report.Issues = append(report.Issues, "Conflicting final first-output metrics")
			}
		}
		if e.BoolField("projection_limited") {
			report.Limited = true
		}
	}
	byDomain := map[string][]*inspectedInterval{}
	keys := []string{}
	for _, interval := range report.Intervals {
		key := intervalClockKey(interval)
		if byDomain[key] == nil {
			keys = append(keys, key)
		}
		byDomain[key] = append(byDomain[key], interval)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := byDomain[keys[i]][0], byDomain[keys[j]][0]
		if a.Relative != b.Relative {
			return !a.Relative
		}
		return keys[i] < keys[j]
	})
	for _, key := range keys {
		domain := analyseClockIntervals(key, byDomain[key])
		report.Domains = append(report.Domains, domain)
		report.Limited = report.Limited || domain.Limited
	}
	if len(report.Domains) > 0 {
		primary := report.Domains[0]
		report.PrimaryDomain = primary.Key
		report.GapTotalMS = primary.GapTotalMS
		report.SegmentOverlapMS = primary.SegmentOverlapMS
		report.GapKnown = primary.GapKnown
		report.OverlapKnown = primary.OverlapKnown
		report.GapByCause = primary.GapByCause
		report.OverlapByStages = primary.OverlapByStages
	}
	var inputEnd, outputStart *float64
	sourceReference := ""
	owners := map[string]bool{}
	for _, record := range records {
		if id := record.Event.StringField("turn_id"); id != "" {
			owners[intervalIdentity(record.Event.StringField("runtime_id"), id)] = true
		}
	}
	for _, interval := range report.Intervals {
		if interval.Relative || interval.Runtime == "" || intervalClockKey(interval) != report.PrimaryDomain || interval.Invalid || interval.Uncorrelated {
			continue
		}
		if owners[intervalIdentity(interval.Runtime, interval.ID)] {
			if value, ok := intervalDuration(interval); ok {
				report.TurnDurationMS = &value
			}
		}
		if interval.Role == "input" && interval.End != nil && !interval.EndFinal && (inputEnd == nil || *interval.End > *inputEnd) {
			inputEnd = interval.End
			sourceReference = interval.Reference
		}
		if interval.Role == "output" && interval.Start != nil && !interval.StartFinal && (outputStart == nil || *interval.Start < *outputStart) {
			outputStart = interval.Start
		}
	}
	if inputEnd != nil && outputStart != nil && *outputStart >= *inputEnd {
		value := *outputStart - *inputEnd
		if math.IsInf(value, 0) || math.IsNaN(value) {
			report.Issues = append(report.Issues, "Source first-output arithmetic is unavailable")
			return report
		}
		report.FirstOutputDelayMS = &value
		report.FirstOutputOrigin = sourceReference
		report.FirstOutputEvidence = "explicit source boundaries"
		if report.FinalOutputDelayMS != nil && sourceReference != "" && sourceReference == finalReference && math.Abs(value-*report.FinalOutputDelayMS) > 1 {
			report.Issues = append(report.Issues, fmt.Sprintf("Final first-output %.3fms conflicts with source %.3fms", *report.FinalOutputDelayMS, value))
		}
	} else if report.FinalOutputDelayMS != nil {
		value := *report.FinalOutputDelayMS
		report.FirstOutputDelayMS = &value
		report.FirstOutputOrigin = report.FinalOutputOrigin
		report.FirstOutputEvidence = "final metric only; source boundary comparison unavailable"
	}
	return report
}

func formatMeasured(value *float64) string {
	if value == nil {
		return "unavailable"
	}
	return fmt.Sprintf("%.3fms", *value)
}
func intervalBar(interval *inspectedInterval, origin, end float64, columns int) string {
	if columns < 4 {
		columns = 4
	}
	bar := []rune(strings.Repeat(" ", columns))
	if interval.Invalid || interval.Start == nil || interval.End == nil {
		return strings.Repeat("·", columns)
	}
	extent := end - origin
	if extent <= 0 {
		extent = 1
	}
	start := int(math.Floor((*interval.Start - origin) / extent * float64(columns-1)))
	stop := int(math.Ceil((*interval.End - origin) / extent * float64(columns-1)))
	start = max(0, min(columns-1, start))
	stop = max(start, min(columns-1, stop))
	glyph := '━'
	if interval.Gap {
		glyph = '░'
	}
	if interval.Status == "cancelled" || interval.Status == "interrupted" {
		glyph = '╍'
	}
	if *interval.Start == *interval.End {
		bar[start] = '◆'
	} else {
		for i := start; i <= stop; i++ {
			bar[i] = glyph
		}
	}
	return string(bar)
}
func renderObserverIntervals(report ObserverIntervalReport, width, scroll, rows int) []string {
	lines := []string{"Measured software pipeline intervals"}
	first := "first software output: " + formatMeasured(report.FirstOutputDelayMS)
	if report.FirstOutputOrigin != "" {
		first += " from " + strings.ReplaceAll(report.FirstOutputOrigin, "_", " ")
	}
	lines = append(lines, "turn: "+formatMeasured(report.TurnDurationMS)+" · "+first)
	if report.FirstOutputEvidence != "" {
		lines = append(lines, report.FirstOutputEvidence)
	}
	for _, issue := range report.Issues[:min(len(report.Issues), 2)] {
		lines = append(lines, "! "+observerClip(issue, width-2))
	}
	overlap, gap := "unavailable", "unavailable"
	if report.OverlapKnown {
		overlap = fmt.Sprintf("%.3fms", report.SegmentOverlapMS)
	}
	if report.GapKnown {
		gap = fmt.Sprintf("%.3fms", report.GapTotalMS)
	}
	lines = append(lines, "Per-segment overlap union: "+overlap+" · measured gap union: "+gap)
	names := make([]string, 0, len(report.GapByCause))
	for name := range report.GapByCause {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > 0 {
		parts := []string{}
		for _, name := range names[:min(len(names), 3)] {
			parts = append(parts, fmt.Sprintf("%s %.3fms", observerClip(name, 24), report.GapByCause[name]))
		}
		if len(names) > 3 {
			parts = append(parts, fmt.Sprintf("+%d causes", len(names)-3))
		}
		lines = append(lines, "Gap partitions: "+strings.Join(parts, " · "))
	}
	if report.Limited {
		lines = append(lines, "! Bounded analysis limit reached; raw evidence remains available")
	}
	if report.Partial {
		lines = append(lines, "Earlier history on disk; retained bounds may be incomplete")
	}
	if len(report.Domains) > 1 {
		lines = append(lines, "Clock domains shown separately; totals above use the first domain")
	}
	intervalIndex := 0
	shown := false
	for _, domain := range report.Domains {
		origin, end := math.Inf(1), math.Inf(-1)
		for _, interval := range domain.Intervals {
			if interval.Invalid {
				continue
			}
			for _, bound := range []*float64{interval.Start, interval.End} {
				if bound != nil {
					origin = math.Min(origin, *bound)
					end = math.Max(end, *bound)
				}
			}
		}
		axisKnown := !math.IsInf(origin, 0) && !math.IsInf(end, 0) && !math.IsInf(end-origin, 0) && !math.IsNaN(end-origin)
		if !axisKnown {
			origin = 0
			end = 0
		}
		if math.IsInf(end, 0) {
			end = origin
		}
		heading := false
		for _, interval := range domain.Intervals {
			index := intervalIndex
			intervalIndex++
			if index < scroll {
				continue
			}
			if len(lines)+2 > rows {
				return lines
			}
			if !heading {
				if axisKnown {
					lines = append(lines, fmt.Sprintf("%s · %.3f → %.3fms", observerClip(domain.Label, width-30), origin, end))
				} else {
					lines = append(lines, observerClip(domain.Label+" · axis unavailable", width))
				}
				heading = true
			}
			label := interval.Stage
			if interval.Segment != nil {
				label += fmt.Sprintf("[%d]", *interval.Segment)
			}
			if interval.Attempt != "" {
				label += " @" + observerClip(interval.Attempt, 12)
			}
			if interval.Gap && interval.Cause != "" {
				label += " " + interval.Cause
			}
			status := interval.Status
			badge := ""
			if interval.Invalid {
				status = "invalid"
				badge = " !"
			} else if len(interval.Issues) > 0 {
				badge = " !"
			}
			bar := strings.Repeat("·", max(4, width-48))
			if domain.ClockKnown && axisKnown {
				bar = intervalBar(interval, origin, end, max(4, width-48))
			}
			lines = append(lines, observerClip(fmt.Sprintf("%3d %-12s %-26s %s", index, observerClip(status+badge, 12), observerClip(label, 26), bar), width))
			duration := "unavailable"
			if value, ok := intervalDuration(interval); ok {
				duration = fmt.Sprintf("%.3fms", value)
			}
			evidence := "source"
			if interval.StartFinal || interval.EndFinal {
				evidence = "final fill/evidence"
			}
			lines = append(lines, observerClip("    "+formatMeasured(interval.Start)+" → "+formatMeasured(interval.End)+" · duration "+duration+" · "+evidence, width))
			if len(interval.Issues) > 0 && len(lines) < rows {
				lines = append(lines, observerClip("    ! "+interval.Issues[0], width))
			}
			shown = true
		}
	}
	if !shown && len(report.Intervals) == 0 {
		lines = append(lines, "Interval measurements unavailable; Events retains raw evidence")
	}
	return lines
}
