package visualizer

import (
	"cmp"
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// requestEvidence is provider-reported evidence for one request interval.
// A nil measurement was not reported; it is never treated as zero.
type requestEvidence struct {
	Provider, Model                        string
	FirstToken, PromptTokens, OutputTokens *float64
	Load, Prefill, Generation              *float64
}

func requestFromFields(fields map[string]any) *requestEvidence {
	measured := func(key string) *float64 {
		if n, ok := finiteNumber(fields[key]); ok {
			return &n
		}
		return nil
	}
	text := func(key string) string { value, _ := fields[key].(string); return boundedIdentity(value) }
	request := &requestEvidence{
		Provider:     text("request_provider"),
		Model:        text("request_model"),
		FirstToken:   measured("first_token_ms"),
		PromptTokens: measured("prompt_tokens"),
		OutputTokens: measured("output_tokens"),
		Load:         measured("load_ms"),
		Prefill:      measured("prefill_ms"),
		Generation:   measured("generation_ms"),
	}
	if *request == (requestEvidence{}) {
		return nil
	}
	return request
}

// mergeRequestEvidence keeps the first identity and the latest reported
// measurement, matching cumulative start/end snapshots of one operation.
func mergeRequestEvidence(current, incoming *requestEvidence) *requestEvidence {
	if current == nil || incoming == nil {
		return cmp.Or(current, incoming)
	}
	latest := func(retained, next *float64) *float64 { return cmp.Or(next, retained) }
	current.Provider = cmp.Or(current.Provider, incoming.Provider)
	current.Model = cmp.Or(current.Model, incoming.Model)
	current.FirstToken = latest(current.FirstToken, incoming.FirstToken)
	current.PromptTokens = latest(current.PromptTokens, incoming.PromptTokens)
	current.OutputTokens = latest(current.OutputTokens, incoming.OutputTokens)
	current.Load = latest(current.Load, incoming.Load)
	current.Prefill = latest(current.Prefill, incoming.Prefill)
	current.Generation = latest(current.Generation, incoming.Generation)
	return current
}

// requestUnaccountedMS is wall time not covered by the provider-reported load,
// prefill and generation durations. It is nil unless the wall time and at
// least one reported duration are known, and it is not clamped at zero.
func requestUnaccountedMS(interval *inspectedInterval) *float64 {
	if interval.Request == nil {
		return nil
	}
	wall, ok := intervalDuration(interval)
	if !ok {
		return nil
	}
	reported, known := 0.0, false
	for _, value := range []*float64{interval.Request.Load, interval.Request.Prefill, interval.Request.Generation} {
		if value != nil {
			reported += *value
			known = true
		}
	}
	if !known {
		return nil
	}
	unaccounted := wall - reported
	return &unaccounted
}

func requestDetails(interval *inspectedInterval) map[string]any {
	request := interval.Request
	count := func(value *float64) any {
		if value == nil {
			return "unavailable"
		}
		return *value
	}
	var wall, afterStart *float64
	if value, ok := intervalDuration(interval); ok {
		wall = &value
	}
	if request.FirstToken != nil && interval.Start != nil && !interval.Invalid {
		value := *request.FirstToken - *interval.Start
		afterStart = &value
	}
	details := map[string]any{
		"label":                      cmp.Or(interval.Label, "unavailable"),
		"provider":                   cmp.Or(request.Provider, "unavailable"),
		"model":                      cmp.Or(request.Model, "unavailable"),
		"prompt_tokens":              count(request.PromptTokens),
		"output_tokens":              count(request.OutputTokens),
		"first_token_runtime_offset": formatMeasured(request.FirstToken),
		"first_token_after_start":    formatMeasured(afterStart),
		"wall_time":                  formatMeasured(wall),
		"load":                       formatMeasured(request.Load),
		"prefill":                    formatMeasured(request.Prefill),
		"generation":                 formatMeasured(request.Generation),
	}
	if unaccounted := requestUnaccountedMS(interval); unaccounted != nil {
		details["unaccounted_time"] = formatMeasured(unaccounted)
	}
	return details
}

// intervalName labels a nested interval by its parent stage and source label.
func intervalName(interval *inspectedInterval) string {
	name := interval.Stage
	if interval.Label != "" {
		name = cmp.Or(interval.ParentStage, interval.Stage) + " · " + interval.Label
	}
	return name
}

func intervalStatusGlyph(status string) string {
	switch status {
	case "running":
		return "●"
	case "cancelled", "interrupted":
		return "■"
	case "failed", "error":
		return "✗"
	case "unknown":
		return "?"
	}
	return "✓"
}

// flowRow is one selectable Flow line: a stage, or an interval nested under it.
type flowRow struct {
	stage string
	child *inspectedInterval
}

func (m InteractiveModel) flowRows() []flowRow {
	rows := []flowRow{}
	for _, stage := range m.dialectFlow.stages() {
		rows = append(rows, flowRow{stage: stage})
		for _, child := range m.flowChildren[stage] {
			rows = append(rows, flowRow{stage: stage, child: child})
		}
	}
	return rows
}

func renderFlowChild(interval *inspectedInterval, selected bool) string {
	line := fmt.Sprintf("  ↳ %s %-14s · %s", intervalStatusGlyph(interval.Status), cmp.Or(interval.Label, interval.Stage), interval.Status)
	if wall, ok := intervalDuration(interval); ok {
		line += fmt.Sprintf(" [%dms]", int(wall))
	}
	if selected {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true).Render("> "+line) + "\n"
	}
	if interval.Status == "failed" || interval.Status == "error" {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("  "+line) + "\n"
	}
	return "  " + line + "\n"
}
