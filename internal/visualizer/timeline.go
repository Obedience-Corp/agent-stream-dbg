package visualizer

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/lancekrogers/stream-debugger/internal/events"
)

// TimelineEntry represents a single event in the timeline
type TimelineEntry struct {
	Timestamp time.Time
	Event     *events.Event
	AgentID   string
	Type      string
	Content   string
}

// TimelineVisualizer renders a timeline view of events
type TimelineVisualizer struct {
	entries   []TimelineEntry
	startTime time.Time
	endTime   time.Time
}

// NewTimelineVisualizer creates a new timeline visualizer
func NewTimelineVisualizer() *TimelineVisualizer {
	return &TimelineVisualizer{
		entries: make([]TimelineEntry, 0),
	}
}

// AddEvent adds an event to the timeline
func (tv *TimelineVisualizer) AddEvent(event *events.Event) {
	entry := TimelineEntry{
		Event: event,
		Type:  string(event.Type),
	}

	// Extract timestamp based on event type
	switch event.Type {
	case events.SessionStart:
		entry.Timestamp = event.SessionStart.Timestamp
	case events.SessionComplete:
		entry.Timestamp = event.SessionComplete.Timestamp
	case events.AgentStreamStart:
		entry.Timestamp = event.AgentStreamStart.Timestamp
		entry.AgentID = event.AgentStreamStart.AgentID
	case events.AgentContent:
		entry.Timestamp = event.AgentContent.Timestamp
		entry.AgentID = event.AgentContent.AgentID
		entry.Content = event.AgentContent.Content
	case events.AgentStreamComplete:
		entry.Timestamp = event.AgentStreamComplete.Timestamp
		entry.AgentID = event.AgentStreamComplete.AgentID
	case events.WizardStreamStart:
		entry.Timestamp = event.WizardStreamStart.Timestamp
		entry.AgentID = "wizard"
	case events.WizardContent:
		entry.Timestamp = event.WizardContent.Timestamp
		entry.AgentID = "wizard"
		entry.Content = event.WizardContent.Content
	case events.WizardStreamComplete:
		entry.Timestamp = event.WizardStreamComplete.Timestamp
		entry.AgentID = "wizard"
	case events.Error:
		entry.Timestamp = event.Error.Timestamp
		entry.AgentID = event.Error.AgentID
	}

	tv.entries = append(tv.entries, entry)

	// Update time bounds
	if tv.startTime.IsZero() || entry.Timestamp.Before(tv.startTime) {
		tv.startTime = entry.Timestamp
	}
	if entry.Timestamp.After(tv.endTime) {
		tv.endTime = entry.Timestamp
	}
}

// RenderTimeline renders a visual timeline showing parallel execution
func (tv *TimelineVisualizer) RenderTimeline(width int) string {
	if len(tv.entries) == 0 {
		return "No events to display"
	}

	// Sort entries by timestamp
	sort.Slice(tv.entries, func(i, j int) bool {
		return tv.entries[i].Timestamp.Before(tv.entries[j].Timestamp)
	})

	// Get unique agents
	agentMap := make(map[string]bool)
	for _, entry := range tv.entries {
		if entry.AgentID != "" {
			agentMap[entry.AgentID] = true
		}
	}

	agents := make([]string, 0, len(agentMap))
	for agent := range agentMap {
		agents = append(agents, agent)
	}
	sort.Strings(agents)

	// Build timeline
	var sb strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	sb.WriteString(headerStyle.Render("📊 Timeline View - Parallel Execution Visualization"))
	sb.WriteString("\n\n")

	// Duration info
	duration := tv.endTime.Sub(tv.startTime)
	sb.WriteString(fmt.Sprintf("Duration: %s | Events: %d | Agents: %d\n\n",
		duration.Round(time.Millisecond), len(tv.entries), len(agents)))

	// Column headers
	timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	sb.WriteString(timeStyle.Render("Time (ms)"))
	sb.WriteString("  ")

	for _, agent := range agents {
		color := getAgentColor(agent)
		agentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Width(15)
		sb.WriteString(agentStyle.Render(agent))
		sb.WriteString("  ")
	}
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", width))
	sb.WriteString("\n")

	// Group events by time buckets (100ms)
	type TimeBucket struct {
		time   time.Time
		events map[string][]TimelineEntry
	}

	buckets := make(map[int64]*TimeBucket)
	for _, entry := range tv.entries {
		ms := entry.Timestamp.Sub(tv.startTime).Milliseconds()
		bucket := ms / 100 * 100 // Round to 100ms
		if _, ok := buckets[bucket]; !ok {
			buckets[bucket] = &TimeBucket{
				time:   tv.startTime.Add(time.Duration(bucket) * time.Millisecond),
				events: make(map[string][]TimelineEntry),
			}
		}
		if entry.AgentID != "" {
			buckets[bucket].events[entry.AgentID] = append(buckets[bucket].events[entry.AgentID], entry)
		}
	}

	// Sort buckets
	bucketKeys := make([]int64, 0, len(buckets))
	for k := range buckets {
		bucketKeys = append(bucketKeys, k)
	}
	sort.Slice(bucketKeys, func(i, j int) bool {
		return bucketKeys[i] < bucketKeys[j]
	})

	// Render each time bucket
	for _, ms := range bucketKeys {
		bucket := buckets[ms]

		// Time column
		timeStr := fmt.Sprintf("%7d", ms)
		sb.WriteString(timeStyle.Render(timeStr))
		sb.WriteString("  ")

		// Agent columns
		for _, agent := range agents {
			color := getAgentColor(agent)
			cellStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Width(15)

			entries := bucket.events[agent]
			if len(entries) == 0 {
				sb.WriteString(cellStyle.Render("│"))
			} else {
				// Render event indicator
				indicator := ""
				for _, entry := range entries {
					switch entry.Type {
					case "agent_stream_start", "wizard_stream_start":
						indicator = "▶ START"
					case "agent_content", "wizard_content":
						if indicator == "" {
							indicator = "█" // Token
						}
					case "agent_stream_complete", "wizard_stream_complete":
						indicator = "■ DONE"
					case "error":
						indicator = "✗ ERROR"
					}
				}
				sb.WriteString(cellStyle.Render(indicator))
			}
			sb.WriteString("  ")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(strings.Repeat("─", width))
	sb.WriteString("\n")

	// Legend
	sb.WriteString("\nLegend: ▶ START  █ Streaming  ■ DONE  ✗ ERROR  │ Idle\n")

	return sb.String()
}

// RenderDetailedLog renders a detailed sequential log
func (tv *TimelineVisualizer) RenderDetailedLog() string {
	if len(tv.entries) == 0 {
		return "No events to display"
	}

	// Sort entries by timestamp
	sort.Slice(tv.entries, func(i, j int) bool {
		return tv.entries[i].Timestamp.Before(tv.entries[j].Timestamp)
	})

	var sb strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	sb.WriteString(headerStyle.Render("📋 Detailed Event Log"))
	sb.WriteString("\n\n")

	timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	typeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

	for i, entry := range tv.entries {
		// Timestamp relative to start
		elapsed := entry.Timestamp.Sub(tv.startTime)

		// Agent color
		color := getAgentColor(entry.AgentID)
		agentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))

		// Format: [+1.234s] sam_harris | agent_content | "hello"
		line := fmt.Sprintf("[%s] ",
			timeStyle.Render(fmt.Sprintf("+%7.3fs", elapsed.Seconds())))

		if entry.AgentID != "" {
			line += agentStyle.Render(fmt.Sprintf("%-15s", entry.AgentID))
		} else {
			line += fmt.Sprintf("%-15s", "system")
		}

		line += " | "
		line += typeStyle.Render(fmt.Sprintf("%-20s", entry.Type))

		// Add content preview for content events
		if entry.Content != "" {
			preview := entry.Content
			if len(preview) > 50 {
				preview = preview[:47] + "..."
			}
			line += " | " + fmt.Sprintf("%q", preview)
		}

		sb.WriteString(line)
		sb.WriteString("\n")

		// Add blank line every 10 events for readability
		if (i+1)%10 == 0 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// RenderParallelSummary shows which agents ran in parallel
func (tv *TimelineVisualizer) RenderParallelSummary() string {
	if len(tv.entries) == 0 {
		return "No events to display"
	}

	// Track active agents at each time
	type ActivePeriod struct {
		agent string
		start time.Time
		end   time.Time
	}

	periods := make([]ActivePeriod, 0)
	activeAgents := make(map[string]time.Time)

	// Sort entries
	sort.Slice(tv.entries, func(i, j int) bool {
		return tv.entries[i].Timestamp.Before(tv.entries[j].Timestamp)
	})

	for _, entry := range tv.entries {
		if entry.AgentID == "" {
			continue
		}

		switch entry.Type {
		case string(events.AgentStreamStart), string(events.WizardStreamStart):
			activeAgents[entry.AgentID] = entry.Timestamp

		case string(events.AgentStreamComplete), string(events.WizardStreamComplete):
			if start, ok := activeAgents[entry.AgentID]; ok {
				periods = append(periods, ActivePeriod{
					agent: entry.AgentID,
					start: start,
					end:   entry.Timestamp,
				})
				delete(activeAgents, entry.AgentID)
			}
		}
	}

	var sb strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	sb.WriteString(headerStyle.Render("🔀 Parallel Execution Summary"))
	sb.WriteString("\n\n")

	// Find overlaps
	overlaps := make(map[string][]string) // agent -> agents it overlapped with

	for i, p1 := range periods {
		for j, p2 := range periods {
			if i >= j {
				continue
			}

			// Check if periods overlap
			if p1.start.Before(p2.end) && p2.start.Before(p1.end) {
				overlaps[p1.agent] = append(overlaps[p1.agent], p2.agent)
				overlaps[p2.agent] = append(overlaps[p2.agent], p1.agent)
			}
		}
	}

	// Report
	for agent, others := range overlaps {
		color := getAgentColor(agent)
		agentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))

		// Deduplicate
		seen := make(map[string]bool)
		unique := make([]string, 0)
		for _, other := range others {
			if !seen[other] {
				seen[other] = true
				unique = append(unique, other)
			}
		}

		sb.WriteString(agentStyle.Render(agent))
		sb.WriteString(" ran in parallel with: ")
		sb.WriteString(strings.Join(unique, ", "))
		sb.WriteString("\n")
	}

	if len(overlaps) == 0 {
		sb.WriteString("No parallel execution detected (agents ran sequentially)\n")
	}

	return sb.String()
}
