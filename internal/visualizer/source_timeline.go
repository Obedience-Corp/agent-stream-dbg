package visualizer

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Source intervals share the Timeline pane while using measured positions,
// rather than summing concurrent stages or substituting token counts for time.
func (p InteractiveModel) renderSourceTimeline() string {
	m := p.passive
	report := m.IntervalReport()
	width := max(26, min(232, p.width-8))
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	lines := []string{title.Render("Timeline · measured pipeline intervals"), ""}
	lines = append(lines, fmt.Sprintf("Turn %s   First output %s", formatMeasured(report.TurnDurationMS), formatMeasured(report.FirstOutputDelayMS)))
	if report.FirstOutputOrigin != "" {
		lines = append(lines, dim.Render("First output measured from "+strings.ReplaceAll(report.FirstOutputOrigin, "_", " ")))
	}
	gap, overlap := "unavailable", "unavailable"
	if report.GapKnown {
		gap = fmt.Sprintf("%.3fms", report.GapTotalMS)
	}
	if report.OverlapKnown {
		overlap = fmt.Sprintf("%.3fms", report.SegmentOverlapMS)
	}
	lines = append(lines, "Gaps "+gap+"   Within-segment overlap "+overlap, "")
	if !m.intervalsEnabled {
		return strings.Join(append(lines, "Interval interpretation disabled; Events and raw details remain available"), "\n")
	}
	if len(report.Intervals) == 0 {
		return strings.Join(append(lines, "Waiting for source interval evidence"), "\n")
	}
	if len(report.Domains) > 1 {
		lines = append(lines, "Clock domains shown separately")
	}
	index := 0
	for _, domain := range report.Domains {
		origin, end := math.Inf(1), math.Inf(-1)
		for _, i := range domain.Intervals {
			if i.Invalid || i.Uncorrelated {
				continue
			}
			for _, v := range []*float64{i.Start, i.End} {
				if v != nil {
					origin = math.Min(origin, *v)
					end = math.Max(end, *v)
				}
			}
		}
		known := domain.ClockKnown && !math.IsInf(origin, 0) && !math.IsInf(end, 0)
		axis := "clock unavailable"
		if known {
			axis = fmt.Sprintf("%.0fms %s %.0fms", origin, strings.Repeat("─", max(2, width-56)), end)
		}
		lines = append(lines, dim.Render(observerClip(domain.Label, width)), dim.Render("                              "+axis))
		for _, i := range domain.Intervals {
			row := index
			index++
			if row < m.scroll {
				continue
			}
			if len(lines) > max(6, m.height-13) {
				break
			}
			name := i.Stage
			if i.Segment != nil {
				name += fmt.Sprintf("[%d]", *i.Segment)
			}
			if i.Attempt != "" {
				name += " @" + i.Attempt
			}
			if i.Gap {
				name += " " + i.Cause
			}
			color := p.getAgentColor(i.Lane)
			glyph := "✓"
			switch i.Status {
			case "running":
				glyph = "●"
			case "cancelled", "interrupted":
				glyph = "■"
			case "failed", "error":
				glyph = "✗"
				color = lipgloss.Color("9")
			case "unknown":
				glyph = "?"
			}
			if i.Gap {
				color = lipgloss.Color("11")
			}
			if i.Invalid || i.Uncorrelated {
				glyph = "!"
				color = lipgloss.Color("9")
			}
			bar := strings.Repeat("·", max(4, width-60))
			if known {
				bar = intervalBar(i, origin, end, max(4, width-60))
			}
			duration := "?"
			if v, ok := intervalDuration(i); ok {
				duration = fmt.Sprintf("%.3fms", v)
			}
			prefix := " "
			if row == m.eventRow {
				prefix = "▸"
			}
			line := fmt.Sprintf("%s %s %-23s %-11s %10s %s", prefix, glyph, observerClip(name, 23), i.Status, duration, bar)
			lines = append(lines, lipgloss.NewStyle().Foreground(color).Render(line))
		}
	}
	if report.Partial {
		lines = append(lines, dim.Render("Earlier history omitted; retained bounds may be incomplete"))
	}
	if report.Limited {
		lines = append(lines, dim.Render("Analysis limit reached; inspect source details"))
	}
	for _, issue := range report.Issues[:min(len(report.Issues), 2)] {
		lines = append(lines, "! "+observerClip(issue, width))
	}
	return strings.Join(lines, "\n")
}
