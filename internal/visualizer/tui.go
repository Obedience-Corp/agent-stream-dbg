package visualizer

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lancekrogers/stream-debugger/internal/client"
	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/logger"
)

// Model represents the TUI application state
type Model struct {
	config  *config.EnhancedConfig
	client  *client.SSEClient
	logger  *logger.StructuredLogger
	ctx     context.Context
	cancel  context.CancelFunc
	message string

	// State
	agents        map[string]*AgentState
	wizardState   *WizardState
	sessionActive bool
	errorCount    int

	// UI State
	width  int
	height int
	paused bool

	// Stats
	startTime   time.Time
	totalTokens int
	totalEvents int

	// Flow status
	flow map[string]*FlowStepStatus
	// Flow metadata
	FlowID string
	// Debug toggles
	showAgentsExpanded bool
	showPromptRef      bool
	showPromptPanel    bool
	// Last prompt_ref received
	lastPromptRef  map[string]interface{}
	lastPromptStep string
	// Prompt/Flow debug info (debug mode only)
	promptInfo map[string]*PromptInfoState // agent_id -> prompt info
	flowConfig *FlowConfigState

	// dialectFlow resolves stage order (and, later, lane roles) from the
	// loaded dialect's flow: spec, or derives them live from observed
	// events when it declared none.
	dialectFlow flowState
}

// AgentState tracks the state of a single agent
type AgentState struct {
	ID             string
	Active         bool
	Content        strings.Builder
	TokenCount     int
	Sequence       int
	BufferedTokens map[int]string // sequence -> token
	StartTime      time.Time
	EndTime        time.Time
	LastUpdate     time.Time
}

// WizardState tracks the wizard synthesis state
type WizardState struct {
	Active         bool
	Content        strings.Builder
	TokenCount     int
	Sequence       int
	BufferedTokens map[int]string
	StartTime      time.Time
	EndTime        time.Time
}

// FlowStepStatus tracks the state of a flow step
type FlowStepStatus struct {
	Step        string
	Enabled     bool
	Started     bool
	Ended       bool
	AgentCount  int
	RoutingMode string
	RouteTaken  string
	RouteReason string
	RouteAgents []string
	DurationMs  int
}

// PromptInfoState tracks prompt info for an agent (debug mode)
type PromptInfoState struct {
	AgentID       string
	PromptFile    string
	PromptSnippet string
	PromptLength  int
	SystemPrompt  string // Only populated when debug=full
}

// FlowConfigState tracks flow configuration (debug mode)
type FlowConfigState struct {
	FlowID         string
	FlowFile       string
	StagesOrder    []string
	StagesEnabled  map[string]bool
	RoutingMode    string
	AgentCount     int
	NonWizardCount int
}

// eventMsg wraps an SSE event for bubbletea
type eventMsg struct {
	event *events.Event
}

// errorMsg wraps an error for bubbletea
type errorMsg struct {
	err error
}

// tickMsg is sent periodically for UI updates
type tickMsg time.Time

// NewModel creates a new TUI model
func NewModel(cfg *config.EnhancedConfig, sseClient *client.SSEClient, structuredLogger *logger.StructuredLogger, message string) *Model {
	ctx, cancel := context.WithCancel(context.Background())

	return &Model{
		config:        cfg,
		client:        sseClient,
		logger:        structuredLogger,
		ctx:           ctx,
		cancel:        cancel,
		message:       message,
		agents:        make(map[string]*AgentState),
		wizardState:   &WizardState{BufferedTokens: make(map[int]string)},
		sessionActive: false,
		startTime:     time.Now(),
		flow:          make(map[string]*FlowStepStatus),
		promptInfo:    make(map[string]*PromptInfoState),
		dialectFlow:   newFlowState(),
	}
}

// Init initializes the bubbletea application
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.waitForEvent(),
		m.waitForError(),
		m.tickCmd(),
	)
}

// Update handles messages and updates the model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case eventMsg:
		return m.handleEvent(msg.event)

	case errorMsg:
		m.errorCount++
		return m, m.waitForError()

	case tickMsg:
		return m, m.tickCmd()
	}

	return m, nil
}

// View renders the TUI
func (m *Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	// Create styles
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("6")).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("6")).
		Padding(0, 1)

	agentStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(1).
		Width(m.width/2 - 4)

	wizardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("11")).
		Padding(1).
		Width(m.width - 4)

	statsStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Padding(0, 1)

	// Build header
	headerText := fmt.Sprintf("Stream Debugger - Session: %s", m.config.Session.ID)
	if m.FlowID != "" {
		headerText = fmt.Sprintf("%s (flow: %s)", headerText, m.FlowID)
	}
	header := headerStyle.Render(headerText)

	// Build flow status line
	flowView := m.renderFlowStatus()
	// Optional prompt_ref view
	promptView := ""
	if m.showPromptRef && m.lastPromptRef != nil {
		pairs := []string{}
		for k, v := range m.lastPromptRef {
			pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
		}
		if len(pairs) > 0 {
			promptView = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(
				fmt.Sprintf("prompt_ref(%s): %s", m.lastPromptStep, strings.Join(pairs, ", ")),
			)
		}
	}

	// Build agent views
	var agentViews []string
	for _, agent := range m.agents {
		if agent.Active || agent.TokenCount > 0 {
			view := m.renderAgent(agent)
			agentViews = append(agentViews, agentStyle.Render(view))
		}
	}

	// Build wizard view
	wizardView := ""
	if m.wizardState.Active || m.wizardState.TokenCount > 0 {
		wizardView = wizardStyle.Render(m.renderWizard())
	}

	// Build stats
	duration := time.Since(m.startTime)
	tokensPerSec := float64(m.totalTokens) / duration.Seconds()
	stats := statsStyle.Render(fmt.Sprintf(
		"Events: %d | Tokens: %d | Tokens/sec: %.1f | Errors: %d | Duration: %s",
		m.totalEvents, m.totalTokens, tokensPerSec, m.errorCount, duration.Round(time.Second),
	))

	// Build controls
	dbg := m.config.Debug.Level
	if dbg == "" {
		dbg = "off"
	}
	controls := statsStyle.Render(fmt.Sprintf("^P pause | ^A agents | ^R refs | ^I prompts | F1 verbose | F2 full | F3 off | ^C quit | debug=%s", dbg))

	// Build prompt panel (debug mode only)
	promptPanelView := m.renderPromptPanel()

	// Combine all sections
	sections := []string{header}
	if flowView != "" {
		sections = append(sections, flowView)
	}
	if promptView != "" {
		sections = append(sections, promptView)
	}
	if promptPanelView != "" {
		sections = append(sections, promptPanelView)
	}

	// Add agent views (side by side)
	if len(agentViews) > 0 {
		rows := (len(agentViews) + 1) / 2
		for i := 0; i < rows; i++ {
			left := agentViews[i*2]
			right := ""
			if i*2+1 < len(agentViews) {
				right = agentViews[i*2+1]
			}
			sections = append(sections, lipgloss.JoinHorizontal(lipgloss.Top, left, right))
		}
	}

	// Add wizard view
	if wizardView != "" {
		sections = append(sections, wizardView)
	}

	// Add stats and controls
	sections = append(sections, stats, controls)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderAgent renders a single agent's view
func (m *Model) renderAgent(agent *AgentState) string {
	statusIcon := "●"
	statusColor := "8"
	if agent.Active {
		statusIcon = "◉"
		statusColor = "10"
	}

	status := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(statusIcon)

	// Get color for agent
	color := getAgentColor(agent.ID, m.agentColorOverrides())
	agentName := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render(agent.ID)

	header := fmt.Sprintf("%s %s", status, agentName)

	// Content preview (last 200 characters)
	content := agent.Content.String()
	if len(content) > 200 {
		content = "..." + content[len(content)-200:]
	}

	// Token info
	tokenInfo := fmt.Sprintf("Tokens: %d | Seq: %d", agent.TokenCount, agent.Sequence)
	if len(agent.BufferedTokens) > 0 {
		tokenInfo += fmt.Sprintf(" | Buffered: %d", len(agent.BufferedTokens))
	}

	return fmt.Sprintf("%s\n%s\n\n%s", header, tokenInfo, content)
}

// renderWizard renders the wizard synthesis view
func (m *Model) renderWizard() string {
	statusIcon := "●"
	statusColor := "8"
	if m.wizardState.Active {
		statusIcon = "◉"
		statusColor = "11"
	}

	status := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(statusIcon)
	wizardName := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).Render("Wizard (Synthesis)")

	header := fmt.Sprintf("%s %s", status, wizardName)

	// Content preview
	content := m.wizardState.Content.String()
	if len(content) > 400 {
		content = "..." + content[len(content)-400:]
	}

	// Token info
	tokenInfo := fmt.Sprintf("Tokens: %d | Seq: %d", m.wizardState.TokenCount, m.wizardState.Sequence)
	if len(m.wizardState.BufferedTokens) > 0 {
		tokenInfo += fmt.Sprintf(" | Buffered: %d", len(m.wizardState.BufferedTokens))
	}

	return fmt.Sprintf("%s\n%s\n\n%s", header, tokenInfo, content)
}

// handleKeyPress handles keyboard input
func (m *Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.cancel()
		return m, tea.Quit

	case "ctrl+p":
		m.paused = !m.paused
		return m, nil

	case "ctrl+a":
		m.showAgentsExpanded = !m.showAgentsExpanded
		return m, nil

	case "ctrl+r":
		m.showPromptRef = !m.showPromptRef
		return m, nil

	case "ctrl+i":
		m.showPromptPanel = !m.showPromptPanel
		return m, nil

	case "F1":
		// Verbose debug mode
		m.config.Debug.Level = "verbose"
		return m, m.reconnect()

	case "F2":
		// Full debug mode
		m.config.Debug.Level = "full"
		return m, m.reconnect()

	case "F3":
		// Turn off debug mode
		m.config.Debug.Level = ""
		return m, m.reconnect()
	}

	return m, nil
}

// reconnect restarts the SSE connection with current debug level
func (m *Model) reconnect() tea.Cmd {
	return func() tea.Msg {
		// Cancel current subscriptions
		if m.cancel != nil {
			m.cancel()
		}
		// Create new context and connect
		m.ctx, m.cancel = context.WithCancel(context.Background())
		// Reset UI state so new stream doesn't append to previous content
		m.agents = make(map[string]*AgentState)
		m.wizardState = &WizardState{BufferedTokens: make(map[int]string)}
		m.totalTokens = 0
		m.totalEvents = 0
		m.errorCount = 0
		m.startTime = time.Now()
		m.flow = make(map[string]*FlowStepStatus)
		m.lastPromptRef = nil
		m.lastPromptStep = ""
		m.sessionActive = false
		m.paused = false
		m.promptInfo = make(map[string]*PromptInfoState)
		m.flowConfig = nil
		// Reconnect SSE client with new debug param
		if err := m.client.Connect(m.ctx, m.message); err != nil {
			return errorMsg{err: fmt.Errorf("reconnect failed: %w", err)}
		}
		// Resume listeners
		return nil
	}
}

// handleEvent processes an SSE event
func (m *Model) handleEvent(event *events.Event) (tea.Model, tea.Cmd) {
	if m.paused {
		return m, m.waitForEvent()
	}

	m.totalEvents++
	m.dialectFlow.observe(event)

	// Log event to structured logger (errors are silently ignored)
	_ = m.logger.LogEvent(event)

	// Stream lifecycle (start/content/end) dispatches on Kind + role, not
	// the dialect's own wire event names — brainyard's agent_*/wizard_*
	// rules already decode to the same Kind, with SourceID/role
	// distinguishing the aggregator lane (wizardState) from every other
	// lane (agents map). Kind-based is the generic equivalent with
	// identical behavior for brainyard specifically.
	isAggregator := m.dialectFlow.role(event) == "aggregator"

	switch event.Kind {
	case events.KindStreamStart:
		if isAggregator {
			m.wizardState.Active = true
			m.wizardState.StartTime = time.Now()
			break
		}
		agentID := event.SourceID
		m.agents[agentID] = &AgentState{
			ID:             agentID,
			Active:         true,
			BufferedTokens: make(map[int]string),
			StartTime:      time.Now(),
			LastUpdate:     time.Now(),
		}

	case events.KindContent:
		if isAggregator {
			m.wizardState.Content.WriteString(event.Content)
			m.wizardState.TokenCount++
			m.wizardState.Sequence = event.Seq
			m.totalTokens++
			break
		}
		agentID := event.SourceID
		if agent, ok := m.agents[agentID]; ok {
			agent.Content.WriteString(event.Content)
			agent.TokenCount++
			agent.Sequence = event.Seq
			agent.LastUpdate = time.Now()
			m.totalTokens++
		}

	case events.KindStreamEnd:
		if isAggregator {
			m.wizardState.Active = false
			m.wizardState.EndTime = time.Now()
			break
		}
		agentID := event.SourceID
		if agent, ok := m.agents[agentID]; ok {
			agent.Active = false
			agent.EndTime = time.Now()
		}
	}

	switch event.Name {
	case "session_start":
		m.sessionActive = true
		if flowID := event.StringField("flow_id"); flowID != "" {
			m.FlowID = flowID
		}

	case "session_complete":
		m.sessionActive = false

	case "error":
		m.errorCount++

	case "flow_step_start":
		step := event.StringField("step")
		st := m.flow[step]
		if st == nil {
			st = &FlowStepStatus{Step: step}
			m.flow[step] = st
		}
		st.Enabled = event.BoolField("enabled")
		st.Started = true
		st.Ended = false
		if step == "agent_exec" {
			st.AgentCount = event.IntField("non_wizard_count")
		}

	case "flow_step_end":
		step := event.StringField("step")
		st := m.flow[step]
		if st == nil {
			st = &FlowStepStatus{Step: step}
			m.flow[step] = st
		}
		st.Enabled = event.BoolField("enabled")
		st.Ended = true
		if agentCount := event.IntField("agent_count"); agentCount > 0 {
			st.AgentCount = agentCount
		}
		if step == "routing" {
			st.RoutingMode = event.StringField("routing_mode")
			st.RouteTaken = event.StringField("route_taken")
			st.RouteReason = event.StringField("route_reason")
			st.RouteAgents = event.StringSliceField("route_agents")
		}
		if durationMs := event.IntField("duration_ms"); durationMs > 0 {
			st.DurationMs = durationMs
		}
		if promptRef := event.MapField("prompt_ref"); promptRef != nil {
			m.lastPromptRef = promptRef
			m.lastPromptStep = step
		}

	case "prompt_info":
		agentID := event.StringField("agent_id")
		state := m.promptInfo[agentID]
		if state == nil {
			state = &PromptInfoState{AgentID: agentID}
			m.promptInfo[agentID] = state
		}
		state.PromptFile = event.StringField("prompt_file")
		state.PromptSnippet = event.StringField("prompt_snippet")
		state.PromptLength = event.IntField("prompt_length")

	case "prompt_full":
		agentID := event.StringField("agent_id")
		state := m.promptInfo[agentID]
		if state == nil {
			state = &PromptInfoState{AgentID: agentID}
			m.promptInfo[agentID] = state
		}
		state.SystemPrompt = event.StringField("system_prompt")

	case "flow_config":
		flowID := event.StringField("flow_id")
		m.flowConfig = &FlowConfigState{
			FlowID:         flowID,
			FlowFile:       event.StringField("flow_file"),
			StagesOrder:    event.StringSliceField("stages_order"),
			StagesEnabled:  event.BoolMapField("stages_enabled"),
			RoutingMode:    event.StringField("routing_mode"),
			AgentCount:     event.IntField("agent_count"),
			NonWizardCount: event.IntField("non_wizard_count"),
		}
		// Also set FlowID if not already set
		if m.FlowID == "" && flowID != "" {
			m.FlowID = flowID
		}
	}

	return m, m.waitForEvent()
}

// renderFlowStatus renders a one-line flow status overview
func (m *Model) renderFlowStatus() string {
	if m.width == 0 {
		return ""
	}

	// Order of steps to show — from the dialect's flow: spec (declared
	// or derived), not a hardcoded literal that can drift out of sync
	// with interactive.go's own copy.
	steps := m.dialectFlow.stages()
	var parts []string

	for _, step := range steps {
		st, ok := m.flow[step]
		label := step
		if step == "agent_exec" {
			label = "agents"
		}

		style := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		sym := "·" // not started / disabled
		if ok {
			if !st.Enabled {
				sym = "–" // explicitly disabled
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
			} else if st.Ended {
				sym = "✓"
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
			} else if st.Started {
				sym = "…"
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
			}
		}

		text := fmt.Sprintf("%s %s", sym, label)

		// Routing details
		if step == "routing" && ok && st.Enabled {
			det := st.RouteTaken
			if det == "" {
				det = "—"
			}
			// Show agents (full when expanded; else trim to first 2)
			agents := ""
			if len(st.RouteAgents) > 0 {
				if m.showAgentsExpanded {
					agents = strings.Join(st.RouteAgents, ",")
				} else {
					max := 2
					if len(st.RouteAgents) < max {
						max = len(st.RouteAgents)
					}
					agents = strings.Join(st.RouteAgents[:max], ",")
					if len(st.RouteAgents) > max {
						agents = fmt.Sprintf("%s,+%d", agents, len(st.RouteAgents)-max)
					}
				}
			}
			// Compose details (type:reason [agents])
			if st.RouteReason != "" && agents != "" {
				text = fmt.Sprintf("%s(%s:%s [%s])", text, det, st.RouteReason, agents)
			} else if st.RouteReason != "" {
				text = fmt.Sprintf("%s(%s:%s)", text, det, st.RouteReason)
			} else if agents != "" {
				text = fmt.Sprintf("%s(%s [%s])", text, det, agents)
			} else {
				text = fmt.Sprintf("%s(%s)", text, det)
			}
		}

		// Append duration if available
		if ok && st.DurationMs > 0 {
			text = fmt.Sprintf("%s [%dms]", text, st.DurationMs)
		}

		parts = append(parts, style.Render(text))
	}

	sep := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("  |  ")
	return lipgloss.JoinHorizontal(lipgloss.Top, parts[0], sep, parts[1], sep, parts[2], sep, parts[3], sep, parts[4])
}

// renderPromptPanel renders the prompt info panel (debug mode only)
func (m *Model) renderPromptPanel() string {
	if !m.showPromptPanel {
		return ""
	}

	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("5")).
		Padding(0, 1).
		Width(m.width - 4)

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("5"))

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8"))

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("7"))

	snippetStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Italic(true)

	var lines []string

	// Flow config section
	if m.flowConfig != nil {
		lines = append(lines, headerStyle.Render("Flow Configuration"))
		lines = append(lines, fmt.Sprintf("  %s %s", labelStyle.Render("ID:"), valueStyle.Render(m.flowConfig.FlowID)))
		lines = append(lines, fmt.Sprintf("  %s %s", labelStyle.Render("File:"), valueStyle.Render(m.flowConfig.FlowFile)))
		if m.flowConfig.RoutingMode != "" {
			lines = append(lines, fmt.Sprintf("  %s %s", labelStyle.Render("Routing:"), valueStyle.Render(m.flowConfig.RoutingMode)))
		}
		lines = append(lines, fmt.Sprintf("  %s %d agents (%d non-wizard)", labelStyle.Render("Agents:"), m.flowConfig.AgentCount, m.flowConfig.NonWizardCount))
		if len(m.flowConfig.StagesOrder) > 0 {
			lines = append(lines, fmt.Sprintf("  %s %s", labelStyle.Render("Stages:"), valueStyle.Render(strings.Join(m.flowConfig.StagesOrder, " → "))))
		}
		lines = append(lines, "")
	}

	// Agent prompts section
	if len(m.promptInfo) > 0 {
		lines = append(lines, headerStyle.Render("Agent Prompts"))
		for agentID, info := range m.promptInfo {
			color := getAgentColor(agentID, m.agentColorOverrides())
			agentName := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render(agentID)

			line := fmt.Sprintf("  %s", agentName)
			if info.PromptFile != "" {
				line += fmt.Sprintf(" %s %s", labelStyle.Render("→"), valueStyle.Render(info.PromptFile))
			}
			if info.PromptLength > 0 {
				line += fmt.Sprintf(" %s", labelStyle.Render(fmt.Sprintf("(%d chars)", info.PromptLength)))
			}
			lines = append(lines, line)

			// Show snippet if available
			if info.PromptSnippet != "" {
				snippet := info.PromptSnippet
				if len(snippet) > 100 {
					snippet = snippet[:100] + "..."
				}
				// Replace newlines with spaces for single-line display
				snippet = strings.ReplaceAll(snippet, "\n", " ")
				lines = append(lines, fmt.Sprintf("    %s", snippetStyle.Render(snippet)))
			}

			// Show full prompt indicator if available
			if info.SystemPrompt != "" {
				fullLen := len(info.SystemPrompt)
				lines = append(lines, fmt.Sprintf("    %s", labelStyle.Render(fmt.Sprintf("[Full prompt loaded: %d chars]", fullLen))))
			}
		}
	}

	if len(lines) == 0 {
		lines = append(lines, labelStyle.Render("No prompt info available. Use [v] for verbose or [f] for full debug mode."))
	}

	return panelStyle.Render(strings.Join(lines, "\n"))
}

// waitForEvent waits for the next SSE event
func (m *Model) waitForEvent() tea.Cmd {
	return func() tea.Msg {
		select {
		case event := <-m.client.Events():
			return eventMsg{event: event}
		case <-m.ctx.Done():
			return nil
		}
	}
}

// waitForError waits for errors from the SSE client
func (m *Model) waitForError() tea.Cmd {
	return func() tea.Msg {
		select {
		case err := <-m.client.Errors():
			return errorMsg{err: err}
		case <-m.ctx.Done():
			return nil
		}
	}
}

// tickCmd sends periodic tick messages for UI updates
func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// agentColorOverrides returns the config-declared agent_colors map, or
// nil if none is configured — safe to pass straight to getAgentColor.
func (m *Model) agentColorOverrides() map[string]string {
	if m.config == nil || m.config.Display == nil {
		return nil
	}
	return m.config.Display.AgentColors
}

// agentColorPalette is a fixed set of visually distinct ANSI colors,
// assigned by hashing the agent ID rather than a hardcoded persona table
// — so a stranger's dialect renders with stable, distinct colors without
// this package knowing any agent's name in advance.
var agentColorPalette = []string{"13", "9", "10", "12", "14", "5", "3", "6", "11", "1", "2", "4"}

// getAgentColor maps an agent ID to a color: an explicit override in
// agentColors (config's display.agent_colors) wins; otherwise the same ID
// always hashes to the same palette entry, spread deterministically
// across different IDs. agentColors may be nil (reading a nil map is a
// safe, ok=false lookup) — every caller without color config gets the
// hash-only behavior this function always had.
func getAgentColor(agentID string, agentColors map[string]string) string {
	if agentID == "" {
		return "7" // Default gray
	}
	if color, ok := agentColors[agentID]; ok {
		return color
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(agentID))
	return agentColorPalette[h.Sum32()%uint32(len(agentColorPalette))]
}
