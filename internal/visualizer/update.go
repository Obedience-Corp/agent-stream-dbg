package visualizer

import (
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lancekrogers/stream-debugger/internal/bridge"
	"github.com/lancekrogers/stream-debugger/internal/events"
	dblogger "github.com/lancekrogers/stream-debugger/internal/logger"
)

func (m InteractiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		updated, cmd, handled := m.handleKeyMsg(msg)
		if handled {
			return updated, cmd
		}
		m = updated

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msg.Width - 4)

		// Update viewport size (leave room for header, input, status)
		viewportHeight := msg.Height - 15 // Adjust for UI elements
		if viewportHeight < 5 {
			viewportHeight = 5
		}
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = viewportHeight
		m.contentDirty = true

	case streamCompleteMsg:
		if msg.index < len(m.messages) {
			m.messages[msg.index].RawSSE = msg.rawSSE
			m.messages[msg.index].Events = msg.events
			m.messages[msg.index].AgentResponses = msg.agentResponses
			m.messages[msg.index].Streaming = false
		}
		m.streaming = false
		m.contentDirty = true
		if m.follow {
			m.viewport.GotoBottom()
		}

	case streamStartMsg:
		// Initialize incremental streaming state
		m.streamBody = msg.body
		m.streamIndex = msg.index
		m.streamCancel = msg.cancel
		m.sseBuf = ""
		m.sseEvent = ""
		m.sseDataBuf.Reset()
		// Kick off first read
		return m, m.readStreamChunkCmd()

	case streamChunkMsg:
		if msg.err != nil {
			// Treat as completion with error
			if m.streamBody != nil {
				_ = m.streamBody.Close()
				m.streamBody = nil
			}
			if m.streamCancel != nil {
				m.streamCancel()
				m.streamCancel = nil
			}
			m.err = msg.err
			m.streaming = false
			if m.streamIndex < len(m.messages) {
				m.messages[m.streamIndex].Streaming = false
			}
			m.contentDirty = true
			break
		}
		if msg.eof {
			// Finalize
			if m.streamBody != nil {
				_ = m.streamBody.Close()
				m.streamBody = nil
			}
			if m.streamCancel != nil {
				m.streamCancel()
				m.streamCancel = nil
			}
			// Build parsed events once at end to fill Events if needed
			if m.streamIndex < len(m.messages) {
				raw := m.messages[m.streamIndex].RawSSE
				parsed, _ := parseSSEStream(raw)
				m.messages[m.streamIndex].Events = parsed
				// Ensure AgentResponses is fully built at end as well
				if len(m.messages[m.streamIndex].AgentResponses) == 0 {
					m.messages[m.streamIndex].AgentResponses = buildAgentResponses(parsed)
				}
				m.messages[m.streamIndex].Streaming = false
			}
			m.streaming = false
			m.contentDirty = true
			if m.follow {
				m.viewport.GotoBottom()
			}
			break
		}

		// Append raw and incrementally parse
		if m.streamIndex < len(m.messages) {
			if len(msg.chunk) > 0 {
				m.messages[m.streamIndex].RawSSE += string(msg.chunk)
				m.incrementalParseSSE(msg.chunk)
				m.contentDirty = true
			}
		}
		// Schedule next read
		return m, m.readStreamChunkCmd()

	case streamErrorMsg:
		m.err = msg.err
		if len(m.messages) > 0 {
			m.messages[len(m.messages)-1].Streaming = false
		}
		m.streaming = false
		m.contentDirty = true
	}

	// For non-key messages (stream chunks, completions, window size),
	// ensure we refresh the viewport when content changed.
	if m.contentDirty {
		m.refreshViewportContent()
	}

	// Update textarea only in insert mode for KeyMsg; always for non-key msgs (blink etc.)
	if _, isKey := msg.(tea.KeyMsg); isKey {
		if m.insertMode {
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			cmds = append(cmds, cmd)
		}
	} else {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// currentPaneName returns the display name for the active pane
func (m InteractiveModel) currentPaneName() string {
	switch m.activePane {
	case PaneFlow:
		return "Flow"
	case PaneApp:
		return "App"
	case PaneTimeline:
		return "Timeline"
	case PaneEvents:
		return "Events"
	case PaneMessages:
		return "Messages"
	default:
		return "?"
	}
}

// parseSSEStream parses raw SSE stream into events
func parseSSEStream(rawSSE string) ([]*events.Event, error) {
	parser := bridge.NewParser()
	var parsedEvents []*events.Event

	lines := strings.Split(rawSSE, "\n")
	var currentEvent string
	var currentData strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "event:") {
			// Save previous event if exists
			if currentEvent != "" && currentData.Len() > 0 {
				event, err := parser.Parse(currentEvent, []byte(currentData.String()))
				if err == nil && event != nil {
					parsedEvents = append(parsedEvents, event)
				}
			}

			// Start new event
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			currentData.Reset()

		} else if strings.HasPrefix(line, "data:") {
			// Accumulate data (remove "data: " prefix)
			dataContent := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if currentData.Len() > 0 {
				currentData.WriteString("\n")
			}
			currentData.WriteString(dataContent)

		} else if line == "" && currentEvent != "" {
			// Empty line marks end of event
			if currentData.Len() > 0 {
				event, err := parser.Parse(currentEvent, []byte(currentData.String()))
				if err == nil && event != nil {
					parsedEvents = append(parsedEvents, event)
				}
			}
			currentEvent = ""
			currentData.Reset()
		}
	}

	// Handle last event if stream doesn't end with newline
	if currentEvent != "" && currentData.Len() > 0 {
		event, err := parser.Parse(currentEvent, []byte(currentData.String()))
		if err == nil && event != nil {
			parsedEvents = append(parsedEvents, event)
		}
	}

	return parsedEvents, nil
}

// incrementalParseSSE parses SSE incrementally from chunks to update AgentResponses as tokens arrive
func (m *InteractiveModel) incrementalParseSSE(chunk []byte) {
	if m.streamIndex >= len(m.messages) {
		return
	}
	data := m.sseBuf + string(chunk)
	lines := strings.Split(data, "\n")
	// If the chunk doesn't end with a newline, keep last partial for next time
	carry := ""
	if !strings.HasSuffix(data, "\n") {
		carry = lines[len(lines)-1]
		lines = lines[:len(lines)-1]
	}
	parser := bridge.NewParser()
	for _, line := range lines {
		s := strings.TrimRight(line, "\r")
		if strings.HasPrefix(s, "event:") {
			// Finish previous event if any
			if m.sseEvent != "" && m.sseDataBuf.Len() > 0 {
				evt, err := parser.Parse(m.sseEvent, []byte(m.sseDataBuf.String()))
				if err == nil && evt != nil {
					m.applyParsedEvent(evt)
				}
				m.sseDataBuf.Reset()
			}
			m.sseEvent = strings.TrimSpace(strings.TrimPrefix(s, "event:"))
		} else if strings.HasPrefix(s, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(s, "data:"))
			if m.sseDataBuf.Len() > 0 {
				m.sseDataBuf.WriteString("\n")
			}
			m.sseDataBuf.WriteString(payload)
		} else if s == "" {
			// End of one event
			if m.sseEvent != "" && m.sseDataBuf.Len() > 0 {
				evt, err := parser.Parse(m.sseEvent, []byte(m.sseDataBuf.String()))
				if err == nil && evt != nil {
					m.applyParsedEvent(evt)
				}
			}
			m.sseEvent = ""
			m.sseDataBuf.Reset()
		}
		// Unknown lines are silently ignored
	}
	m.sseBuf = carry
}

// applyParsedEvent updates agent responses incrementally from a parsed
// event. Dispatches on evt.Kind + role, not on the dialect's own wire
// event names (previously separate per-lane wire-event-shaped cases) —
// a dialect's agent-lane and aggregator-lane rules already decode to the
// same Kind (stream_start/content/stream_end) with SourceID
// distinguishing which lane, so Kind-based dispatch is the generic
// equivalent with identical behavior for any such dialect, and the only
// version of this function that doesn't special-case one dialect's own
// event vocabulary in Go source. First-token latency, duration, and
// turn-metrics logging remain role: aggregator-exclusive (matching what
// the prior per-lane-name-only handling already did) — regular agents
// never got these, and still don't.
func (m *InteractiveModel) applyParsedEvent(evt *events.Event) {
	m.dialectFlow.observe(evt)

	idx := m.streamIndex
	if idx >= len(m.messages) {
		return
	}
	if m.messages[idx].AgentResponses == nil {
		m.messages[idx].AgentResponses = make(map[string]*AgentResponse)
	}
	aid := evt.SourceID
	isAggregator := m.dialectFlow.role(evt) == "aggregator"

	switch evt.Kind {
	case events.KindStreamStart:
		if aid == "" {
			return
		}
		ar := m.messages[idx].AgentResponses[aid]
		if ar == nil {
			ar = &AgentResponse{AgentID: aid}
			m.messages[idx].AgentResponses[aid] = ar
		}
		if isAggregator {
			ar.StartTime = time.Now()
		}
	case events.KindContent:
		if aid == "" {
			return
		}
		ar := m.messages[idx].AgentResponses[aid]
		if ar == nil {
			ar = &AgentResponse{AgentID: aid}
			if isAggregator {
				ar.StartTime = time.Now()
			}
			m.messages[idx].AgentResponses[aid] = ar
		}
		if isAggregator && ar.TokenCount == 0 && !ar.StartTime.IsZero() {
			ar.FirstTokenMs = time.Since(ar.StartTime).Milliseconds()
		}
		c := evt.Content
		ar.ContentChunks = append(ar.ContentChunks, c)
		ar.FullContent += c
		ar.TokenCount++
	case events.KindStreamEnd:
		if aid == "" {
			return
		}
		ar := m.messages[idx].AgentResponses[aid]
		if ar == nil {
			ar = &AgentResponse{AgentID: aid}
			m.messages[idx].AgentResponses[aid] = ar
		}
		if !isAggregator {
			ar.Completed = true
			break
		}
		// Skip duplicate completion events — aggregator-only, matching
		// the aggregator lane's prior exclusive completion guard.
		if ar.Completed {
			return
		}
		ar.Completed = true
		ar.EndTime = time.Now()
		// Calculate total duration using EndTime - StartTime (not time.Since)
		if !ar.StartTime.IsZero() {
			ar.DurationMs = ar.EndTime.Sub(ar.StartTime).Milliseconds()
		}
		// Also capture token count from event if available
		if tc := evt.IntField("token_count"); tc > 0 {
			ar.TokenCount = tc
		}
		// Log turn metrics for the aggregator lane only, matching what
		// the prior aggregator-lane-only handling already did.
		if m.slog != nil && m.cfg != nil {
			var tokensPerSec float64
			if ar.DurationMs > 0 && ar.TokenCount > 0 {
				tokensPerSec = float64(ar.TokenCount) * 1000.0 / float64(ar.DurationMs)
			}
			_ = m.slog.LogAgentTurnMetrics(dblogger.AgentTurnMetrics{
				SessionID:    m.cfg.Session.ID,
				AgentID:      aid,
				TurnID:       idx,
				TokenCount:   ar.TokenCount,
				FirstTokenMs: ar.FirstTokenMs,
				DurationMs:   ar.DurationMs,
				TokensPerSec: tokensPerSec,
			})
		}
	case events.KindToolCall, events.KindToolResult:
		if aid == "" {
			return
		}
		ar := m.messages[idx].AgentResponses[aid]
		if ar == nil {
			ar = &AgentResponse{AgentID: aid}
			m.messages[idx].AgentResponses[aid] = ar
		}
		appendToolEventLine(ar, evt)
	default:
		// ignore others
	}
	// Append to events list so Flow/Events can update incrementally
	m.messages[idx].Events = append(m.messages[idx].Events, evt)
}

// aggregatorResponse returns the AgentResponse for msg's aggregator-role
// lane, or nil if none is present yet. If more than one lane resolves to
// role: aggregator — the flow model permits this in principle, though no
// shipped dialect does it — the first in sorted AgentID order wins,
// deterministically; true multi-aggregator support is deferred until a
// real dialect needs it.
func (m InteractiveModel) aggregatorResponse(msg Message) *AgentResponse {
	ids := make([]string, 0, len(msg.AgentResponses))
	for id := range msg.AgentResponses {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if m.dialectFlow.role(&events.Event{SourceID: id}) == "aggregator" {
			return msg.AgentResponses[id]
		}
	}
	return nil
}

// buildAgentResponses builds agent response map from parsed events
func buildAgentResponses(evts []*events.Event) map[string]*AgentResponse {
	responses := make(map[string]*AgentResponse)

	for _, event := range evts {
		agentID := event.SourceID
		if agentID == "" {
			continue
		}

		// Initialize agent response if needed
		if _, exists := responses[agentID]; !exists {
			responses[agentID] = &AgentResponse{
				AgentID:       agentID,
				ContentChunks: make([]string, 0),
			}
		}

		agent := responses[agentID]

		// Dispatch on Kind, not the dialect's own wire event names — a
		// dialect's agent-lane and aggregator-lane rules already decode
		// to the same Kind, treated identically here (unlike
		// applyParsedEvent's incremental path, this rebuild never
		// distinguished the aggregator lane's handling from any other
		// agent's).
		switch event.Kind {
		case events.KindStreamStart:
			agent.StartTime = time.Now()

		case events.KindContent:
			content := event.Content
			if content != "" {
				agent.ContentChunks = append(agent.ContentChunks, content)
				agent.FullContent += content
				agent.TokenCount++
			}

		case events.KindStreamEnd:
			agent.EndTime = time.Now()
			agent.Completed = true

		case events.KindToolCall, events.KindToolResult:
			appendToolEventLine(agent, event)
		}
	}

	return responses
}
