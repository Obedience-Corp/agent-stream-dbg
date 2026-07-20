package visualizer

import (
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	dblogger "github.com/Obedience-Corp/agent-stream-dbg/internal/logger"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
)

func (m InteractiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case animTickMsg:
		m.animFrame++
		m.energy.tick(time.Now(), m.streaming)
		// Viewport hosts agent/flow glyphs — refresh only while something is hot.
		if m.hasHotViewportChrome() {
			m.contentDirty = true
		}
		cmds = append(cmds, animTickCmd())

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
		for i := range m.configPanel.fields {
			m.configPanel.fields[i].input.Width = msg.Width - 28
			if m.configPanel.fields[i].input.Width < 30 {
				m.configPanel.fields[i].input.Width = 30
			}
		}
		m.contentDirty = true

	case grpcMethodsMsg:
		m.configPanel.grpcDiscoveryPending = false
		if msg.err != nil {
			m.configPanel.status = "gRPC discovery failed: " + msg.err.Error()
			m.contentDirty = true
			break
		}
		m.configPanel.grpcMethods = msg.methods
		m.configPanel.grpcMethodPickerIndex = 0
		if len(msg.methods) == 0 {
			m.configPanel.status = "Reflection is available, but no streaming RPCs were found"
			m.contentDirty = true
			break
		}
		m.configPanel.grpcMethodPickerOpen = true
		m.configPanel.status = "Choose a streaming RPC. Client-streaming-only methods are shown but cannot be selected yet."
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
		// Initialize frame-level streaming state. The shared transport owns
		// HTTP/SSE setup and wire parsing; the model owns only UI adaptation.
		if msg.sessionReady {
			m.sessionReady = true
		}
		m.streamTransport = msg.stream
		m.streamIndex = msg.index
		m.energy.reset()
		if msg.index < len(m.messages) && msg.stream != nil {
			m.messages[msg.index].TransportName = msg.stream.Name()
		}
		return m, m.readStreamFrameCmd()

	case streamFrameMsg:
		if msg.eof {
			// Finalize after the shared transport has closed its frame channel.
			// Keep incrementally parsed Events — do not re-parse RawSSE as named
			// SSE. That only works for named-event SSE and wipes OpenAI-style
			// data-only streams and gRPC interactive sessions (binary/JSON raw).
			m.closeActiveStream()
			if m.streamIndex < len(m.messages) {
				if len(m.messages[m.streamIndex].AgentResponses) == 0 {
					m.messages[m.streamIndex].AgentResponses = m.buildAgentResponses(m.messages[m.streamIndex].Events)
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

		// Append the exact raw frame and decode through the same bridge parser
		// used by every noninteractive transport path. applyParsedEvent owns
		// the Events list — do not append here or every event is duplicated.
		//
		// For sessionful transports (ACP), frames may arrive after the user
		// turn ends (late updates / reverse-RPC traffic). We still attach them
		// to the latest message index and keep reading so the agent does not
		// stall on a full Frames buffer.
		targetIdx := m.streamIndex
		if targetIdx < 0 || targetIdx >= len(m.messages) {
			if len(m.messages) > 0 {
				targetIdx = len(m.messages) - 1
			}
		}
		if targetIdx >= 0 && targetIdx < len(m.messages) {
			transportName := m.messages[targetIdx].TransportName
			if transportName == "" && m.streamTransport != nil {
				transportName = m.streamTransport.Name()
			}
			if transportName == "" {
				transportName = "sse"
			}
			m.messages[targetIdx].TransportName = transportName
			m.messages[targetIdx].Frames = append(m.messages[targetIdx].Frames, captureFrame(msg.frame))
			if transportName == "sse" && len(msg.frame.Raw) > 0 {
				m.messages[targetIdx].RawSSE += string(msg.frame.Raw)
			}
			if msg.frame.Err != nil {
				m.closeActiveStream()
				m.err = msg.frame.Err
				m.streaming = false
				m.messages[targetIdx].Streaming = false
				m.contentDirty = true
				break
			}
			var evt *events.Event
			if parsed, err := m.parser.Parse(msg.frame.Name, msg.frame.Data); err == nil && parsed != nil {
				evt = parsed
				// applyParsedEvent always uses streamIndex; align for multi-turn.
				prev := m.streamIndex
				m.streamIndex = targetIdx
				m.applyParsedEvent(evt)
				m.streamIndex = prev
			}
			m.contentDirty = true

			// Sessionful turn boundary: end UI "streaming" but keep the process.
			if m.streaming && turnEnded(evt) {
				if _, ok := m.streamTransport.(transport.SessionTransport); ok {
					if len(m.messages[targetIdx].AgentResponses) == 0 {
						m.messages[targetIdx].AgentResponses = m.buildAgentResponses(m.messages[targetIdx].Events)
					}
					m.messages[targetIdx].Streaming = false
					m.streaming = false
					if m.follow {
						m.viewport.GotoBottom()
					}
					// Keep draining frames for reverse RPC / late notifications.
					return m, m.readStreamFrameCmd()
				}
			}
		}
		return m, m.readStreamFrameCmd()

	case streamErrorMsg:
		m.err = msg.err
		if len(m.messages) > 0 {
			m.messages[len(m.messages)-1].Streaming = false
		}
		m.streaming = false
		m.contentDirty = true

	case sessionSetupMsg:
		m.closeActiveStream()
		m.streaming = false
		if msg.sessionID != "" {
			m.cfg.Session.ID = msg.sessionID
		}
		if m.slog != nil {
			_ = m.slog.Close()
		}
		if l, err := dblogger.NewStructuredLogger(m.cfg); err == nil {
			m.slog = l
		}

		// Reset UI state for the newly requested session, retaining any setup
		// error as a visible model error instead of swallowing it.
		m.messages = make([]Message, 0)
		m.err = msg.err
		m.flowTurnIndex = 0
		m.flowContinuous = true
		m.selectedStepIndex = 0
		m.flowExpanded = make(map[string]bool)
		m.showTokens = false
		m.eventsAggregatorOnly = false
		m.appFocus = AppFocusAgents
		m.viewport.SetContent("")
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

func captureFrame(frame transport.Frame) CapturedFrame {
	captured := CapturedFrame{
		Name:      frame.Name,
		Timestamp: frame.Timestamp,
	}
	if len(frame.Data) > 0 {
		captured.Data = append([]byte(nil), frame.Data...)
	}
	if len(frame.Raw) > 0 {
		captured.Raw = append([]byte(nil), frame.Raw...)
	}
	if frame.Err != nil {
		captured.Err = frame.Err.Error()
	}
	return captured
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
	return parseSSEStreamWithParser(rawSSE, bridge.NewParser())
}

func parseSSEStreamWithParser(rawSSE string, parser *bridge.Parser) ([]*events.Event, error) {
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
	isAggregator := m.dialectFlow.role(evt) == events.RoleAggregator

	switch evt.Kind {
	case events.KindStreamStart:
		if aid == "" {
			return
		}
		ar := m.messages[idx].AgentResponses[aid]
		if ar == nil {
			ar = &AgentResponse{AgentID: aid, Role: m.dialectFlow.role(evt)}
			m.messages[idx].AgentResponses[aid] = ar
		}
		ar.Role = m.dialectFlow.role(evt)
		if isAggregator {
			ar.StartTime = time.Now()
		}
	case events.KindContent:
		if aid == "" {
			return
		}
		ar := m.messages[idx].AgentResponses[aid]
		if ar == nil {
			ar = &AgentResponse{AgentID: aid, Role: m.dialectFlow.role(evt)}
			if isAggregator {
				ar.StartTime = time.Now()
			}
			m.messages[idx].AgentResponses[aid] = ar
		}
		ar.Role = m.dialectFlow.role(evt)
		if isAggregator && ar.TokenCount == 0 && !ar.StartTime.IsZero() {
			ar.FirstTokenMs = time.Since(ar.StartTime).Milliseconds()
		}
		c := evt.Content
		ar.ContentChunks = append(ar.ContentChunks, c)
		ar.FullContent += c
		ar.TokenCount++
		// Drive header/stream energy from real content tokens.
		m.energy.onTokens(1, time.Now())
	case events.KindStreamEnd:
		if aid == "" {
			return
		}
		ar := m.messages[idx].AgentResponses[aid]
		if ar == nil {
			ar = &AgentResponse{AgentID: aid, Role: m.dialectFlow.role(evt)}
			m.messages[idx].AgentResponses[aid] = ar
		}
		ar.Role = m.dialectFlow.role(evt)
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
			ar = &AgentResponse{AgentID: aid, Role: m.dialectFlow.role(evt)}
			m.messages[idx].AgentResponses[aid] = ar
		}
		ar.Role = m.dialectFlow.role(evt)
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
		if m.dialectFlow.role(&events.Event{SourceID: id}) == events.RoleAggregator {
			return msg.AgentResponses[id]
		}
	}
	return nil
}

// buildAgentResponses builds agent response map from parsed events
func buildAgentResponses(evts []*events.Event) map[string]*AgentResponse {
	return buildAgentResponsesWithFlow(evts, newFlowState())
}

func (m InteractiveModel) buildAgentResponses(evts []*events.Event) map[string]*AgentResponse {
	return buildAgentResponsesWithFlow(evts, m.dialectFlow)
}

func buildAgentResponsesWithFlow(evts []*events.Event, flow flowState) map[string]*AgentResponse {
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
				Role:          flow.role(event),
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
