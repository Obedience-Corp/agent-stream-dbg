package visualizer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	clientapi "github.com/lancekrogers/stream-debugger/internal/client"
	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	dblogger "github.com/lancekrogers/stream-debugger/internal/logger"
)

// ViewMode represents the display mode for responses
type ViewMode int

const (
	ViewModeRaw ViewMode = iota
	ViewModeParsed
)

// Pane represents the active pane in the TUI
type Pane int

const (
	PaneFlow Pane = iota
	PaneApp
	PaneYAML
	PaneTimeline
	PaneEvents
	PaneMessages
)

// AppFocus represents the focused section in App pane
type AppFocus int

const (
	AppFocusWizard AppFocus = iota
	AppFocusAgents
)

// FlowNode represents a flow step's state for rendering
type FlowNode struct {
	Step          string
	Enabled       bool
	AgentCount    int
	FilteredCount int
	DurationMs    int
	RoutingMode   string
	RouteTaken    string
	RouteReason   string
	RouteAgents   []string
	PromptRef     map[string]interface{}
	InProgress    bool
}

// AgentResponse tracks a single agent's response
type AgentResponse struct {
	AgentID       string
	ContentChunks []string
	FullContent   string
	TokenCount    int
	StartTime     time.Time
	EndTime       time.Time
	Completed     bool
	// Wizard-specific metrics (only populated for wizard responses)
	FirstTokenMs int64 // Time from stream_start to first content event (ms)
	DurationMs   int64 // Total duration from stream_start to stream_complete (ms)
}

// InteractiveModel represents the interactive TUI state
type InteractiveModel struct {
	cfg      *config.EnhancedConfig
	apiKey   string
	textarea textarea.Model
	viewport viewport.Model
	viewMode ViewMode

	// Message history
	messages  []Message
	streaming bool

	// UI state
	width  int
	height int
	err    error

	// UX toggles
	follow      bool // keep viewport pinned to bottom when true
	showHistory bool // render all messages when true; else latest only
	wrap        bool // soft-wrap lines to viewport width
	xOffset     int  // horizontal scroll offset (columns)
	insertMode  bool // when true, keys go to textarea; when false, keys control viewport

	// Structured logger
	slog *dblogger.StructuredLogger

	// Track if viewport content needs refresh
	contentDirty bool

	// Pane management
	activePane        Pane
	selectedStepIndex int
	flowExpanded      map[string]bool // track expanded/collapsed state per step
	showTokens        bool            // Events pane: show token events
	showPromptRef     bool            // Flow pane: show prompt references
	eventsWizardOnly  bool            // Events pane: show only wizard events when true

	// App pane state
	appFocus       AppFocus
	agentCollapsed map[string]bool // per-agent collapsed state

	// YAML pane state
	yamlSnapshot     string
	yamlPrevSnapshot string
	yamlReloadStatus string
	yamlDiff         string
	configClient     *clientapi.ConfigAPIClient

	// Streaming (incremental) state
	streamBody   io.ReadCloser
	streamIndex  int
	sseBuf       string          // carryover between chunks
	sseEvent     string          // current event type being assembled
	sseDataBuf   strings.Builder // current event data buffer
	streamCancel context.CancelFunc

	// Flow pane state
	flowTurnIndex  int  // which message index is displayed in Flow (default latest)
	flowContinuous bool // when true, render all turns continuously

	// Events pane state (expandable nodes)
	eventExpanded    map[int]bool // event index -> expanded state
	selectedEventIdx int          // cursor position in events list

	// Synthesis content for App pane attribution
	synthesisContent string

	// Save status message
	saveStatus string
}

type Message struct {
	Text      string
	Timestamp time.Time
	Streaming bool

	// Store complete raw SSE (no truncation)
	RawSSE string

	// Store parsed events
	Events []*events.Event

	// Store per-agent responses
	AgentResponses map[string]*AgentResponse
}

// NewInteractiveModel creates a new interactive TUI model
func NewInteractiveModel(cfg *config.EnhancedConfig, apiKey string) InteractiveModel {
	ta := textarea.New()
	ta.Placeholder = "Type your message and press Enter to send (Ctrl+C to quit)..."
	ta.Focus()
	ta.CharLimit = 1000
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	// Initialize viewport for scrolling
	vp := viewport.New(80, 20)

	// Initialize structured logger (using legacy config wrapper)
	var slog *dblogger.StructuredLogger
	{
		legacy := &config.Config{
			BackendURL:       cfg.Backend.BaseURL,
			APIKey:           cfg.APIKey,
			SessionID:        cfg.Session.ID,
			LogDir:           cfg.LogDir,
			EnableColors:     cfg.EnableColors,
			MaxAgentsVisible: cfg.MaxAgentsVisible,
		}
		if l, err := dblogger.NewStructuredLogger(legacy); err == nil {
			slog = l
		}
	}

	m := InteractiveModel{
		cfg:          cfg,
		apiKey:       apiKey,
		textarea:     ta,
		viewport:     vp,
		viewMode:     ViewModeParsed, // Default to parsed view (Ctrl+T to toggle to raw)
		messages:     make([]Message, 0),
		follow:       true,
		showHistory:  true,
		wrap:         true,
		xOffset:      0,
		insertMode:   false,
		slog:         slog,
		contentDirty: true,
		// Pane defaults
		activePane:        PaneFlow,
		selectedStepIndex: 0,
		flowExpanded:      make(map[string]bool),
		showTokens:        false,
		showPromptRef:     false,
		flowContinuous:    true,
		// App pane defaults
		appFocus:       AppFocusAgents,
		agentCollapsed: make(map[string]bool),
		// YAML pane
		configClient: clientapi.NewConfigAPIClient(cfg, apiKey),
		// Events pane expandable nodes
		eventExpanded:    make(map[int]bool),
		selectedEventIdx: 0,
	}
	// Set an initial placeholder so the viewport isn't blank before first refresh
	m.viewport.SetContent(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(
		"Press 1–5 to switch panes • i to type • Ctrl+T toggles RAW/PARSED in Events",
	))
	return m
}

func (m InteractiveModel) Init() tea.Cmd {
	return textarea.Blink
}

// refreshViewportContent updates the viewport content based on current state
func (m *InteractiveModel) refreshViewportContent() {
	var viewportContent string

	// Dispatch to pane-specific renderer
	switch m.activePane {
	case PaneFlow:
		if m.flowContinuous {
			viewportContent = m.renderFlowAllPane()
		} else {
			viewportContent = m.renderFlowPane()
		}
	case PaneApp:
		viewportContent = m.renderAppPane()
	case PaneYAML:
		viewportContent = m.renderYAMLPane()
	case PaneTimeline:
		viewportContent = m.renderTimelinePane()
	case PaneEvents:
		viewportContent = m.renderEventsPane()
	case PaneMessages:
		// Reuse the combined RAW/PARSED message renderer
		viewportContent = m.renderMessages()
	default:
		viewportContent = m.renderMessages()
	}

	// Apply wrapping/clipping
	if m.wrap {
		viewportContent = m.wrapToWidth(viewportContent, m.viewport.Width)
	} else if m.xOffset > 0 {
		viewportContent = m.clipLeft(viewportContent, m.xOffset)
	}

	m.viewport.SetContent(viewportContent)
	m.contentDirty = false
}

func (m InteractiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global ctrl bindings
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if msg.Type == tea.KeyCtrlT {
			// Toggle view mode (used by Events and Messages panes to switch RAW↔PARSED views)
			if m.viewMode == ViewModeRaw {
				m.viewMode = ViewModeParsed
			} else {
				m.viewMode = ViewModeRaw
			}
			m.contentDirty = true
			m.refreshViewportContent()
			return m, nil
		}
		if msg.Type == tea.KeyCtrlN {
			// Start a new session
			return m, m.newSessionCmd()
		}
		if msg.Type == tea.KeyCtrlF {
			m.follow = !m.follow
			if m.follow {
				m.viewport.GotoBottom()
			}
			return m, nil
		}
		if msg.Type == tea.KeyCtrlS {
			// Save events to file with full expanded content (human-readable)
			if len(m.messages) > 0 {
				lastMsg := m.messages[len(m.messages)-1]
				filename := fmt.Sprintf("events_%s.txt", time.Now().Format("20060102_150405"))

				var output strings.Builder
				output.WriteString("# Stream Debugger Event Export\n")
				output.WriteString(fmt.Sprintf("# Timestamp: %s\n", time.Now().Format(time.RFC3339)))
				output.WriteString(fmt.Sprintf("# Session: %s\n", m.cfg.Session.ID))
				output.WriteString(fmt.Sprintf("# Event Count: %d\n\n", len(lastMsg.Events)))
				output.WriteString(strings.Repeat("=", 80) + "\n\n")

				for i, evt := range lastMsg.Events {
					// Event header
					output.WriteString(fmt.Sprintf("## Event %d: %s\n", i+1, evt.Type))
					output.WriteString(strings.Repeat("-", 40) + "\n")

					// Expanded content (same as TUI shows when expanded)
					expanded := m.renderEventExpandedPlainText(evt, &lastMsg)
					output.WriteString(expanded)
					output.WriteString("\n\n")
				}

				err := os.WriteFile(filename, []byte(output.String()), 0644)
				if err != nil {
					m.saveStatus = fmt.Sprintf("Save failed: %v", err)
				} else {
					m.saveStatus = fmt.Sprintf("Saved to %s", filename)
				}
				m.contentDirty = true
				m.refreshViewportContent()
			}
			return m, nil
		}
		if msg.Type == tea.KeyEsc {
			if m.insertMode {
				m.insertMode = false
				m.textarea.Blur()
			}
			return m, nil
		}
		if msg.Type == tea.KeyEnter {
			// In insert mode, Enter sends the message. In normal mode, let the
			// key fall through to pane-specific handling (e.g., Flow expand).
			if m.insertMode && !m.streaming {
				message := m.textarea.Value()
				if strings.TrimSpace(message) != "" {
					m.messages = append(m.messages, Message{Text: message, Timestamp: time.Now(), Streaming: true})
					m.flowTurnIndex = len(m.messages) - 1
					m.textarea.Reset()
					m.streaming = true
					m.contentDirty = true
					return m, m.startStreamingCmd(message, len(m.messages)-1)
				}
				return m, nil
			}
			// Not in insert mode: do not return here; handle below.
		}

		// Insert toggle
		if msg.String() == "i" && !m.insertMode {
			m.insertMode = true
			m.textarea.Focus()
			return m, nil
		}

		// Pane switching and debug toggles (works in normal mode only)
		if !m.insertMode {
			switch msg.String() {
			case "1", "f1":
				m.activePane = PaneFlow
				m.contentDirty = true
				m.refreshViewportContent()
				return m, nil
			case "2", "f2":
				m.activePane = PaneApp
				m.contentDirty = true
				m.refreshViewportContent()
				return m, nil
			case "3", "f3":
				m.activePane = PaneYAML
				m.contentDirty = true
				m.refreshViewportContent()
				return m, nil
			case "4", "f4":
				m.activePane = PaneTimeline
				m.contentDirty = true
				m.refreshViewportContent()
				return m, nil
			case "5", "f5":
				m.activePane = PaneEvents
				m.contentDirty = true
				m.refreshViewportContent()
				return m, nil
			case "6", "f6":
				m.activePane = PaneMessages
				m.contentDirty = true
				m.refreshViewportContent()
				return m, nil
			// Debug level toggles (F7=verbose, F8=full, F9=off)
			case "f7":
				m.cfg.Debug.Level = "verbose"
				m.contentDirty = true
				m.refreshViewportContent()
				return m, nil
			case "f8":
				m.cfg.Debug.Level = "full"
				m.contentDirty = true
				m.refreshViewportContent()
				return m, nil
			case "f9":
				m.cfg.Debug.Level = ""
				m.contentDirty = true
				m.refreshViewportContent()
				return m, nil
			}
		}

		// Normal-mode navigation only when not inserting
		if !m.insertMode {
			switch msg.Type {
			case tea.KeyUp:
				m.viewport.ScrollUp(1)
				m.follow = false
				return m, nil
			case tea.KeyDown:
				m.viewport.ScrollDown(1)
				m.follow = false
				return m, nil
			case tea.KeyPgUp:
				m.viewport.HalfPageUp()
				m.follow = false
				return m, nil
			case tea.KeyPgDown:
				m.viewport.HalfPageDown()
				m.follow = false
				return m, nil
			case tea.KeyHome:
				m.viewport.GotoTop()
				m.follow = false
				return m, nil
			case tea.KeyEnd:
				m.viewport.GotoBottom()
				m.follow = true
				return m, nil
			case tea.KeyLeft:
				if m.xOffset > 0 {
					m.xOffset--
					m.contentDirty = true
				}
				return m, nil
			case tea.KeyRight:
				m.xOffset++
				m.contentDirty = true
				return m, nil
			}
			switch msg.String() {
			case "j":
				// Flow pane: move selection down; Events pane: navigate events; others: scroll viewport
				if m.activePane == PaneFlow {
					if m.selectedStepIndex < 5 { // 6 steps: 0-5
						m.selectedStepIndex++
						m.contentDirty = true
						m.refreshViewportContent()
					}
				} else if m.activePane == PaneEvents {
					// Navigate down in visible events list
					visibleCount := m.countVisibleEvents()
					if m.selectedEventIdx < visibleCount-1 {
						m.selectedEventIdx++
						m.contentDirty = true
						m.refreshViewportContent()
						// Scroll viewport down to keep selection visible
						m.viewport.ScrollDown(3)
					}
				} else {
					m.viewport.ScrollDown(1)
					m.follow = false
				}
				return m, nil
			case "k":
				// Flow pane: move selection up; Events pane: navigate events; others: scroll viewport
				if m.activePane == PaneFlow {
					if m.selectedStepIndex > 0 {
						m.selectedStepIndex--
						m.contentDirty = true
						m.refreshViewportContent()
					}
				} else if m.activePane == PaneEvents {
					// Navigate up in visible events list
					if m.selectedEventIdx > 0 {
						m.selectedEventIdx--
						m.contentDirty = true
						m.refreshViewportContent()
						// Scroll viewport up to keep selection visible
						m.viewport.ScrollUp(3)
					}
				} else {
					m.viewport.ScrollUp(1)
					m.follow = false
				}
				return m, nil
			case "[":
				// Flow pane: previous turn
				if m.activePane == PaneFlow {
					if m.flowTurnIndex > 0 {
						m.flowTurnIndex--
						m.contentDirty = true
						m.refreshViewportContent()
					}
				}
				return m, nil
			case "]":
				// Flow pane: next turn
				if m.activePane == PaneFlow {
					if m.flowTurnIndex < len(m.messages)-1 {
						m.flowTurnIndex++
						m.contentDirty = true
						m.refreshViewportContent()
					}
				}
				return m, nil
			case "enter":
				// Flow pane: toggle expand/collapse
				if m.activePane == PaneFlow {
					steps := []string{"routing", "discovery", "agent_exec", "filter", "synthesis", "wizard"}
					if m.selectedStepIndex < len(steps) {
						step := steps[m.selectedStepIndex]
						m.flowExpanded[step] = !m.flowExpanded[step]
						m.contentDirty = true
						m.refreshViewportContent()
					}
				} else if m.activePane == PaneEvents {
					// Events pane: toggle expand/collapse for selected event
					m.eventExpanded[m.selectedEventIdx] = !m.eventExpanded[m.selectedEventIdx]
					m.contentDirty = true
					m.refreshViewportContent()
				}
				return m, nil
			case "o":
				// Flow pane: open in App pane with focus on selected step's content
				if m.activePane == PaneFlow {
					steps := []string{"routing", "discovery", "agent_exec", "filter", "synthesis", "wizard"}
					if m.selectedStepIndex < len(steps) {
						step := steps[m.selectedStepIndex]
						switch step {
						case "wizard", "synthesis":
							m.appFocus = AppFocusWizard
						case "agent_exec":
							m.appFocus = AppFocusAgents
						default:
							m.appFocus = AppFocusWizard
						}
						m.activePane = PaneApp
						m.contentDirty = true
						m.refreshViewportContent()
					}
				}
				return m, nil
			case "f":
				// App pane: toggle focus between Agents and Wizard
				if m.activePane == PaneApp {
					if m.appFocus == AppFocusAgents {
						m.appFocus = AppFocusWizard
					} else {
						m.appFocus = AppFocusAgents
					}
					m.contentDirty = true
					m.refreshViewportContent()
				}
				return m, nil
			case "p":
				// Flow pane: toggle prompt ref display
				if m.activePane == PaneFlow {
					m.showPromptRef = !m.showPromptRef
					m.contentDirty = true
					m.refreshViewportContent()
				}
				return m, nil
			case "a":
				// Flow pane: toggle continuous view (all turns)
				if m.activePane == PaneFlow {
					m.flowContinuous = !m.flowContinuous
					m.contentDirty = true
					m.refreshViewportContent()
				}
				return m, nil
			case "t":
				// Events pane: toggle token visibility
				if m.activePane == PaneEvents {
					m.showTokens = !m.showTokens
					m.contentDirty = true
					m.refreshViewportContent()
				}
				return m, nil
			case "W":
				// Events pane: toggle wizard-only filter
				if m.activePane == PaneEvents {
					m.eventsWizardOnly = !m.eventsWizardOnly
					m.contentDirty = true
					m.refreshViewportContent()
				}
				return m, nil
			case "y":
				// YAML pane: take snapshot
				if m.activePane == PaneYAML && m.configClient != nil {
					m.yamlPrevSnapshot = m.yamlSnapshot
					data, status, err := m.configClient.GetFlowVizConfig()
					if err != nil {
						m.yamlReloadStatus = fmt.Sprintf("Snapshot error: %v", err)
					} else if status/100 != 2 {
						m.yamlReloadStatus = fmt.Sprintf("HTTP %d", status)
					} else {
						m.yamlSnapshot = clientapi.PrettyJSON(data)
						m.yamlReloadStatus = fmt.Sprintf("Snapshot taken at %s", time.Now().Format("15:04:05"))
						// Compute diff if we have a previous snapshot
						if m.yamlPrevSnapshot != "" {
							m.yamlDiff = lineDiff(m.yamlPrevSnapshot, m.yamlSnapshot)
						} else {
							m.yamlDiff = ""
						}
					}
					m.contentDirty = true
					m.refreshViewportContent()
				}
				return m, nil
			case "R":
				// YAML pane: reload prompts
				if m.activePane == PaneYAML && m.configClient != nil {
					data, status, err := m.configClient.ReloadPrompts()
					if err != nil {
						m.yamlReloadStatus = fmt.Sprintf("Reload error: %v", err)
					} else {
						m.yamlReloadStatus = fmt.Sprintf("HTTP %d\n%s", status, clientapi.PrettyJSON(data))
					}
					m.contentDirty = true
					m.refreshViewportContent()
				}
				return m, nil
			case "g":
				m.viewport.GotoTop()
				m.follow = false
				return m, nil
			case "G":
				m.viewport.GotoBottom()
				m.follow = true
				return m, nil
			case " ":
				m.viewport.HalfPageDown()
				m.follow = false
				return m, nil
			case "b":
				m.viewport.HalfPageUp()
				m.follow = false
				return m, nil
			case "H":
				m.showHistory = !m.showHistory
				m.contentDirty = true
				if !m.showHistory {
					m.follow = true
					m.viewport.GotoBottom()
				}
				return m, nil
			case "w":
				m.wrap = !m.wrap
				m.contentDirty = true
				if m.wrap {
					m.xOffset = 0
				}
				return m, nil
			case "h":
				if m.xOffset > 0 {
					m.xOffset--
					m.contentDirty = true
				}
				return m, nil
			case "l":
				m.xOffset++
				m.contentDirty = true
				return m, nil
			}
		}

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
	case PaneYAML:
		return "YAML"
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

func (m InteractiveModel) View() string {
	var b strings.Builder

	// Header with pane indicator
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("6")).
		Padding(0, 1)

	paneStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("14")).
		Background(lipgloss.Color("8")).
		Padding(0, 1)

	// Pane tabs
	paneNames := []string{"Flow", "App", "YAML", "Timeline", "Events", "Messages"}
	var tabs strings.Builder
	for i, name := range paneNames {
		if Pane(i) == m.activePane {
			tabs.WriteString(paneStyle.Render(fmt.Sprintf("[%d]%s", i+1, name)))
		} else {
			tabs.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fmt.Sprintf(" %d:%s ", i+1, name)))
		}
	}

	dbg := m.cfg.Debug.Level
	if dbg == "" {
		dbg = "off"
	}
	b.WriteString(headerStyle.Render(fmt.Sprintf("🚀 Stream Debugger (debug=%s)", dbg)))
	b.WriteString(" ")
	b.WriteString(tabs.String())
	b.WriteString("  ")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fmt.Sprintf("(session: %s)", m.cfg.Session.ID)))
	b.WriteString("\n\n")

	// Display viewport (scrollable message history)
	// Content is set in Update() via refreshViewportContent()
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Error display
	if m.err != nil {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true)
		b.WriteString(errorStyle.Render(fmt.Sprintf("❌ Error: %v", m.err)))
		b.WriteString("\n\n")
	}

	// Input box
	inputBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1)

	inputLabel := "Your message:"
	if m.streaming {
		inputLabel = "⏳ Streaming response..."
	}

	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(inputLabel))
	b.WriteString("\n")
	b.WriteString(inputBoxStyle.Render(m.textarea.View()))
	b.WriteString("\n")

	// Status line with pane-specific hints
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Italic(true)

	mode := "INSERT"
	if !m.insertMode {
		mode = "NORMAL"
	}

	// Pane-specific status hints
	var paneHints string
	switch m.activePane {
	case PaneFlow:
		toggle := "off"
		if m.flowContinuous {
			toggle = "on"
		}
		paneHints = fmt.Sprintf("j/k:select Enter:expand(wizard details) p:prompt [/]:turn a:all(%s)", toggle)
	case PaneEvents:
		tokensStr := "OFF"
		if m.showTokens {
			tokensStr = "ON"
		}
		wizardStr := "OFF"
		if m.eventsWizardOnly {
			wizardStr = "ON"
		}
		paneHints = fmt.Sprintf("j/k:navigate Enter:expand t:tokens(%s) W:wizard-only(%s)", tokensStr, wizardStr)
	case PaneYAML:
		paneHints = "R:reload y:snapshot"
	case PaneMessages:
		paneHints = "Ctrl+T:raw/parsed"
	default:
		paneHints = ""
	}

	wrapStr := "OFF"
	if m.wrap {
		wrapStr = "ON"
	}
	status := fmt.Sprintf(
		"Mode:%s Pane:%s Msgs:%d | 1-6:panes %s w:wrap(%s) F7:verbose F8:full F9:off Ctrl+S:save",
		mode, m.currentPaneName(), len(m.messages), paneHints, wrapStr,
	)
	b.WriteString(statusStyle.Render(status))

	// Show save status if present
	if m.saveStatus != "" {
		b.WriteString("\n")
		saveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
		b.WriteString(saveStyle.Render(m.saveStatus))
	}

	return b.String()
}

// parseSSEStream parses raw SSE stream into events
func parseSSEStream(rawSSE string) ([]*events.Event, error) {
	parser := events.NewParser()
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
	parser := events.NewParser()
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

// applyParsedEvent updates agent responses incrementally from a parsed event
func (m *InteractiveModel) applyParsedEvent(evt *events.Event) {
	idx := m.streamIndex
	if idx >= len(m.messages) {
		return
	}
	if m.messages[idx].AgentResponses == nil {
		m.messages[idx].AgentResponses = make(map[string]*AgentResponse)
	}
	switch evt.Type {
	case events.AgentStreamStart:
		aid := evt.AgentStreamStart.AgentID
		if aid == "" {
			return
		}
		if m.messages[idx].AgentResponses[aid] == nil {
			m.messages[idx].AgentResponses[aid] = &AgentResponse{AgentID: aid}
		}
	case events.AgentContent:
		aid := evt.AgentContent.AgentID
		if aid == "" {
			return
		}
		ar := m.messages[idx].AgentResponses[aid]
		if ar == nil {
			ar = &AgentResponse{AgentID: aid}
			m.messages[idx].AgentResponses[aid] = ar
		}
		c := evt.AgentContent.Content
		ar.ContentChunks = append(ar.ContentChunks, c)
		ar.FullContent += c
		ar.TokenCount++
	case events.AgentStreamComplete:
		aid := evt.AgentStreamComplete.AgentID
		if aid == "" {
			return
		}
		ar := m.messages[idx].AgentResponses[aid]
		if ar == nil {
			ar = &AgentResponse{AgentID: aid}
			m.messages[idx].AgentResponses[aid] = ar
		}
		ar.Completed = true
	case events.WizardStreamStart:
		// ensure wizard entry exists and record start time
		if m.messages[idx].AgentResponses["wizard"] == nil {
			m.messages[idx].AgentResponses["wizard"] = &AgentResponse{AgentID: "wizard", StartTime: time.Now()}
		} else {
			m.messages[idx].AgentResponses["wizard"].StartTime = time.Now()
		}
	case events.WizardContent:
		ar := m.messages[idx].AgentResponses["wizard"]
		if ar == nil {
			ar = &AgentResponse{AgentID: "wizard", StartTime: time.Now()}
			m.messages[idx].AgentResponses["wizard"] = ar
		}
		// Track first token latency
		if ar.TokenCount == 0 && !ar.StartTime.IsZero() {
			ar.FirstTokenMs = time.Since(ar.StartTime).Milliseconds()
		}
		c := evt.WizardContent.Content
		ar.ContentChunks = append(ar.ContentChunks, c)
		ar.FullContent += c
		ar.TokenCount++
	case events.WizardStreamComplete:
		ar := m.messages[idx].AgentResponses["wizard"]
		if ar == nil {
			ar = &AgentResponse{AgentID: "wizard"}
			m.messages[idx].AgentResponses["wizard"] = ar
		}
		// Skip duplicate completion events
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
		if evt.WizardStreamComplete != nil && evt.WizardStreamComplete.TokenCount > 0 {
			ar.TokenCount = evt.WizardStreamComplete.TokenCount
		}
		// Log wizard turn metrics
		if m.slog != nil && m.cfg != nil {
			var tokensPerSec float64
			if ar.DurationMs > 0 && ar.TokenCount > 0 {
				tokensPerSec = float64(ar.TokenCount) * 1000.0 / float64(ar.DurationMs)
			}
			_ = m.slog.LogWizardTurnMetrics(dblogger.WizardTurnMetrics{
				SessionID:    m.cfg.Session.ID,
				TurnID:       idx,
				TokenCount:   ar.TokenCount,
				FirstTokenMs: ar.FirstTokenMs,
				DurationMs:   ar.DurationMs,
				TokensPerSec: tokensPerSec,
			})
		}
	default:
		// ignore others
	}
	// Append to events list so Flow/Events can update incrementally
	m.messages[idx].Events = append(m.messages[idx].Events, evt)
}

// buildAgentResponses builds agent response map from parsed events
func buildAgentResponses(events []*events.Event) map[string]*AgentResponse {
	responses := make(map[string]*AgentResponse)

	for _, event := range events {
		agentID := event.GetAgentID()
		if agentID == "" {
			// Treat wizard events as coming from a pseudo-agent "wizard"
			if event.WizardStreamStart != nil || event.WizardContent != nil || event.WizardStreamComplete != nil {
				agentID = "wizard"
			} else {
				continue
			}
		}

		// Initialize agent response if needed
		if _, exists := responses[agentID]; !exists {
			responses[agentID] = &AgentResponse{
				AgentID:       agentID,
				ContentChunks: make([]string, 0),
			}
		}

		agent := responses[agentID]

		// Handle different event types
		switch {
		case event.AgentStreamStart != nil || event.WizardStreamStart != nil:
			agent.StartTime = time.Now()

		case event.AgentContent != nil || event.WizardContent != nil:
			content := event.GetContent()
			if content != "" {
				agent.ContentChunks = append(agent.ContentChunks, content)
				agent.FullContent += content
				agent.TokenCount++
			}

		case event.AgentStreamComplete != nil || event.WizardStreamComplete != nil:
			agent.EndTime = time.Now()
			agent.Completed = true
		}
	}

	return responses
}

// getAgentColor returns the lipgloss color for an agent
func (m InteractiveModel) getAgentColor(agentID string) lipgloss.Color {
	// Check if config has agent colors
	if m.cfg != nil && m.cfg.Display != nil && m.cfg.Display.AgentColors != nil {
		if color, exists := m.cfg.Display.AgentColors[agentID]; exists {
			return lipgloss.Color(color)
		}
	}
	// Default color
	return lipgloss.Color("7")
}

// renderRawView renders the raw SSE stream
func (m InteractiveModel) renderRawView(msg Message) string {
	if msg.RawSSE == "" {
		return ""
	}

	var b strings.Builder

	// While streaming, render the raw SSE as-is for responsiveness;
	// pretty printing on every chunk is expensive.
	if msg.Streaming {
		b.WriteString(msg.RawSSE)
		b.WriteString("\n")
		return b.String()
	}

	responseStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("13")).
		Bold(true)

	b.WriteString(responseStyle.Render("Raw SSE Stream (JSON Pretty-Printed):"))
	b.WriteString("\n\n")

	// Parse and pretty-print JSON in data fields
	lines := strings.Split(msg.RawSSE, "\n")

	eventStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14")).
		Bold(true)

	jsonKeyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("6"))

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")

		if strings.HasPrefix(line, "event:") {
			// Style event lines
			b.WriteString(eventStyle.Render(line))
			b.WriteString("\n")

		} else if strings.HasPrefix(line, "data:") {
			// Extract JSON from data line
			jsonStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

			if jsonStr == "" {
				b.WriteString("data:\n")
				continue
			}

			// Try to parse and pretty-print JSON
			var jsonData interface{}
			if err := json.Unmarshal([]byte(jsonStr), &jsonData); err == nil {
				prettyJSON, err := json.MarshalIndent(jsonData, "  ", "  ")
				if err == nil {
					b.WriteString(jsonKeyStyle.Render("data:"))
					b.WriteString("\n  ")
					b.WriteString(string(prettyJSON))
					b.WriteString("\n")
				} else {
					// Fallback to original if indent fails
					b.WriteString(line)
					b.WriteString("\n")
				}
			} else {
				// Not JSON, render as-is
				b.WriteString(line)
				b.WriteString("\n")
			}

		} else if line == "" {
			// Preserve empty lines (event separators)
			b.WriteString("\n")
		} else {
			// Other lines (shouldn't happen in valid SSE, but handle anyway)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	return b.String()
}

// renderParsedView renders agent responses color-coded by agent
func (m InteractiveModel) renderParsedView(msg Message) string {
	if len(msg.AgentResponses) == 0 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Italic(true).
			Render("No agent responses parsed")
	}

	var b strings.Builder

	// Wizard debug summary at top
	if wizardResp := msg.AgentResponses["wizard"]; wizardResp != nil {
		b.WriteString(m.renderWizardDebugSummary(wizardResp))
		b.WriteString("\n")
	}

	// Render each agent's response (deterministic order)
	// Collect and sort agent IDs for stable rendering to avoid flicker
	keys := make([]string, 0, len(msg.AgentResponses))
	for k := range msg.AgentResponses {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, agentID := range keys {
		agentResp := msg.AgentResponses[agentID]
		if agentResp.FullContent == "" {
			continue
		}

		// Agent header with color
		agentColor := m.getAgentColor(agentID)
		agentHeaderStyle := lipgloss.NewStyle().
			Foreground(agentColor).
			Bold(true)

		// Display agent header
		header := fmt.Sprintf("%s (%d tokens)", agentID, agentResp.TokenCount)
		if agentResp.Completed {
			header += " ✓"
		} else {
			header += " ⏳"
		}

		b.WriteString(agentHeaderStyle.Render(header))
		b.WriteString("\n")

		// Agent content with same color
		contentStyle := lipgloss.NewStyle().
			Foreground(agentColor)

		b.WriteString(contentStyle.Render(agentResp.FullContent))
		b.WriteString("\n\n")
	}

	return b.String()
}

func (m InteractiveModel) renderMessages() string {
	var b strings.Builder

	// Show last or all messages depending on history toggle
	start := 0
	if !m.showHistory && len(m.messages) > 0 {
		start = len(m.messages) - 1
	}
	for i := start; i < len(m.messages); i++ {
		msg := m.messages[i]

		// User message
		userStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true)

		b.WriteString(userStyle.Render(fmt.Sprintf("You [%s]:", msg.Timestamp.Format("15:04:05"))))
		b.WriteString("\n")
		b.WriteString(msg.Text)
		b.WriteString("\n\n")

		// Response
		if msg.Streaming {
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("8")).
				Italic(true).
				Render("⏳ Streaming response from agents..."))
			b.WriteString("\n\n")
		} else if msg.RawSSE != "" {
			// Render based on view mode
			if m.viewMode == ViewModeRaw {
				b.WriteString(m.renderRawView(msg))
			} else {
				b.WriteString(m.renderParsedView(msg))
			}
		}
	}

	return b.String()
}

// =============================================================================
// PANE RENDERERS
// =============================================================================

// buildFlowNodes extracts FlowNode state from parsed events
func (m InteractiveModel) buildFlowNodes(evts []*events.Event) map[string]*FlowNode {
	nodes := map[string]*FlowNode{}
	for _, e := range evts {
		if s := e.FlowStepStart; s != nil {
			n := nodes[s.Step]
			if n == nil {
				n = &FlowNode{Step: s.Step}
			}
			n.Enabled = s.Enabled
			n.InProgress = true
			if s.Step == "agent_exec" && s.NonWizardCount > 0 {
				n.AgentCount = s.NonWizardCount
			}
			nodes[s.Step] = n
		}
		if s := e.FlowStepEnd; s != nil {
			n := nodes[s.Step]
			if n == nil {
				n = &FlowNode{Step: s.Step}
			}
			n.Enabled = s.Enabled
			n.InProgress = false
			if s.AgentCount > 0 {
				n.AgentCount = s.AgentCount
			}
			if s.DurationMs > 0 {
				n.DurationMs = s.DurationMs
			}
			n.PromptRef = s.PromptRef
			n.RoutingMode = s.RoutingMode
			n.RouteTaken = s.RouteTaken
			n.RouteReason = s.RouteReason
			n.RouteAgents = s.RouteAgents
			if s.FilteredCount > 0 {
				n.FilteredCount = s.FilteredCount
			}
			nodes[s.Step] = n
		}
	}
	return nodes
}

// statusIcon returns the status indicator for a flow step
func statusIcon(n *FlowNode) string {
	if !n.Enabled {
		return "–"
	}
	if n.InProgress {
		return "…"
	}
	if n.DurationMs > 0 {
		return "✓"
	}
	return "…"
}

// renderFlowPane renders the Flow pane (F1) showing step rows
func (m InteractiveModel) renderFlowPane() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	b.WriteString(headerStyle.Render("Flow Steps"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("─────────────────────────────────────────"))
	b.WriteString("\n\n")

	// Select message for Flow view (latest by default, or navigated turn)
	if len(m.messages) == 0 {
		b.WriteString(dimStyle.Render("No messages yet. Send a message to see flow steps."))
		return b.String()
	}
	turn := m.flowTurnIndex
	if turn < 0 || turn >= len(m.messages) {
		turn = len(m.messages) - 1
	}
	msg := m.messages[turn]
	if len(msg.Events) == 0 {
		b.WriteString(dimStyle.Render("No flow events in latest message."))
		return b.String()
	}

	nodes := m.buildFlowNodes(msg.Events)
	steps := []string{"routing", "discovery", "agent_exec", "filter", "synthesis", "wizard"}

	// Header detail: show turn index for context
	b.WriteString(dimStyle.Render(fmt.Sprintf("Turn %d of %d  ([ ] to navigate)", turn+1, len(m.messages))))
	b.WriteString("\n\n")

	for i, step := range steps {
		n := nodes[step]

		// Build the line
		var line strings.Builder

		// Status icon
		if n != nil {
			line.WriteString(statusIcon(n))
		} else {
			line.WriteString(" ")
		}
		line.WriteString(" ")

		// Step name
		line.WriteString(fmt.Sprintf("%-12s", step))

		// Metrics (if available)
		if n != nil {
			// Duration
			if n.DurationMs > 0 {
				line.WriteString(fmt.Sprintf(" [%dms]", n.DurationMs))
			}

			// Agent count for agent_exec
			if step == "agent_exec" && n.AgentCount > 0 {
				line.WriteString(fmt.Sprintf(" agents:%d", n.AgentCount))
			}

			// Filtered count for filter
			if step == "filter" && n.FilteredCount > 0 {
				line.WriteString(fmt.Sprintf(" filtered:%d", n.FilteredCount))
			}

			// Routing info
			if step == "routing" && n.RouteTaken != "" {
				line.WriteString(fmt.Sprintf(" → %s", n.RouteTaken))
				if len(n.RouteAgents) > 0 {
					line.WriteString(fmt.Sprintf(" [%s]", strings.Join(n.RouteAgents, ", ")))
				}
			}

			// Wizard inline metrics (tokens, tokens/sec)
			if step == "wizard" && msg.AgentResponses != nil {
				if wizardResp := msg.AgentResponses["wizard"]; wizardResp != nil {
					line.WriteString(fmt.Sprintf(" tokens:%d", wizardResp.TokenCount))
					if wizardResp.DurationMs > 0 && wizardResp.TokenCount > 0 {
						tokensPerSec := float64(wizardResp.TokenCount) * 1000.0 / float64(wizardResp.DurationMs)
						line.WriteString(fmt.Sprintf(" %.1ftok/s", tokensPerSec))
					}
				}
			}
		}

		// Render with selection highlight
		lineStr := line.String()
		if i == m.selectedStepIndex {
			b.WriteString(selectedStyle.Render("> " + lineStr))
		} else {
			b.WriteString("  " + lineStr)
		}
		b.WriteString("\n")

		// Expanded details (if expanded and node exists)
		if m.flowExpanded[step] && n != nil {
			// For agent_exec, show agent list for this turn
			if step == "agent_exec" {
				agents := m.deriveAgentsFromEvents(msg.Events)
				if len(agents) == 0 && msg.RawSSE != "" {
					if parsed, err := parseSSEStream(msg.RawSSE); err == nil {
						agents = m.deriveAgentsFromEvents(parsed)
					}
				}
				if len(agents) > 0 {
					b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).PaddingLeft(4).Render(
						fmt.Sprintf("agents: [%s]", strings.Join(agents, ", ")),
					))
					b.WriteString("\n")
				}
			}
			// Pass wizard response for wizard step metrics
			var wizardResp *AgentResponse
			if step == "wizard" && msg.AgentResponses != nil {
				wizardResp = msg.AgentResponses["wizard"]
			}
			b.WriteString(m.renderFlowNodeDetails(n, wizardResp))
		}
	}

	return b.String()
}

// renderFlowAllPane renders a continuous flow across all messages (turns)
func (m InteractiveModel) renderFlowAllPane() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	b.WriteString(headerStyle.Render("Flow (All Turns)"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("─────────────────────────────────────────"))
	b.WriteString("\n\n")

	if len(m.messages) == 0 {
		b.WriteString(dimStyle.Render("No messages yet. Send a message to see flow steps."))
		return b.String()
	}

	for i, msg := range m.messages {
		// Per-turn header with timestamp and user message
		ts := msg.Timestamp.Format("15:04:05")
		preview := msg.Text
		if len(preview) > 80 {
			preview = preview[:80] + "…"
		}
		b.WriteString(lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("Turn %d • %s", i+1, ts)))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("You: %s\n", preview))

		// Build nodes for this turn
		evts := msg.Events
		if len(evts) == 0 && msg.RawSSE != "" {
			if parsed, err := parseSSEStream(msg.RawSSE); err == nil {
				evts = parsed
			}
		}
		nodes := m.buildFlowNodes(evts)

		// Agent list: prefer routing.route_agents; else derive from AgentStreamStart
		var agents []string
		if rn := nodes["routing"]; rn != nil && len(rn.RouteAgents) > 0 {
			agents = append(agents, rn.RouteAgents...)
		}
		if len(agents) == 0 {
			// derive from events
			uniq := map[string]struct{}{}
			for _, e := range evts {
				if e.Type == events.AgentStreamStart && e.AgentStreamStart != nil {
					aid := e.AgentStreamStart.AgentID
					if aid != "" && aid != "wizard" {
						uniq[aid] = struct{}{}
					}
				}
			}
			for aid := range uniq {
				agents = append(agents, aid)
			}
			sort.Strings(agents)
		}
		if len(agents) > 0 {
			b.WriteString(fmt.Sprintf("Agents: [%s]\n", strings.Join(agents, ", ")))
		}

		// Step rows
		steps := []string{"routing", "discovery", "agent_exec", "filter", "synthesis", "wizard"}
		for _, step := range steps {
			n := nodes[step]
			if n == nil {
				continue
			}

			var line strings.Builder
			line.WriteString(statusIcon(n))
			line.WriteString(" ")
			line.WriteString(fmt.Sprintf("%-12s", step))
			if n.DurationMs > 0 {
				line.WriteString(fmt.Sprintf(" [%dms]", n.DurationMs))
			}
			if step == "agent_exec" && n.AgentCount > 0 {
				line.WriteString(fmt.Sprintf(" agents:%d", n.AgentCount))
			}
			if step == "filter" && n.FilteredCount > 0 {
				line.WriteString(fmt.Sprintf(" filtered:%d", n.FilteredCount))
			}
			if step == "routing" && n.RouteTaken != "" {
				line.WriteString(fmt.Sprintf(" → %s", n.RouteTaken))
				if len(n.RouteAgents) > 0 {
					line.WriteString(fmt.Sprintf(" [%s]", strings.Join(n.RouteAgents, ", ")))
				}
			}
			// Wizard inline metrics (tokens, tokens/sec)
			if step == "wizard" && msg.AgentResponses != nil {
				if wizardResp := msg.AgentResponses["wizard"]; wizardResp != nil {
					line.WriteString(fmt.Sprintf(" tokens:%d", wizardResp.TokenCount))
					if wizardResp.DurationMs > 0 && wizardResp.TokenCount > 0 {
						tokensPerSec := float64(wizardResp.TokenCount) * 1000.0 / float64(wizardResp.DurationMs)
						line.WriteString(fmt.Sprintf(" %.1ftok/s", tokensPerSec))
					}
				}
			}
			b.WriteString("  " + line.String() + "\n")

			// Expanded details across all turns for the selected step
			if m.flowExpanded[step] {
				if step == "agent_exec" {
					agents := m.deriveAgentsFromEvents(evts)
					if len(agents) > 0 {
						b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).PaddingLeft(4).Render(
							fmt.Sprintf("agents: [%s]", strings.Join(agents, ", ")),
						))
						b.WriteString("\n")
					}
				}
				// Pass wizard response for wizard step metrics
				var wizardResp *AgentResponse
				if step == "wizard" && msg.AgentResponses != nil {
					wizardResp = msg.AgentResponses["wizard"]
				}
				b.WriteString(m.renderFlowNodeDetails(n, wizardResp))
			}
		}

		// Spacer between turns
		if i < len(m.messages)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// deriveAgentsFromEvents returns a sorted list of non-wizard agents that streamed in this turn
func (m InteractiveModel) deriveAgentsFromEvents(evts []*events.Event) []string {
	uniq := map[string]struct{}{}
	for _, e := range evts {
		if e.Type == events.AgentStreamStart && e.AgentStreamStart != nil {
			aid := e.AgentStreamStart.AgentID
			if aid != "" && aid != "wizard" {
				uniq[aid] = struct{}{}
			}
		}
	}
	if len(uniq) == 0 {
		return nil
	}
	agents := make([]string, 0, len(uniq))
	for aid := range uniq {
		agents = append(agents, aid)
	}
	sort.Strings(agents)
	return agents
}

// renderFlowNodeDetails renders expanded details for a flow node
// wizardResp is optional and only used when step is "wizard" to show metrics
func (m InteractiveModel) renderFlowNodeDetails(n *FlowNode, wizardResp *AgentResponse) string {
	var b strings.Builder
	detailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).PaddingLeft(4)
	wizardStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("13")).PaddingLeft(4)
	wrote := false

	// Basic fields always useful
	b.WriteString(detailStyle.Render(fmt.Sprintf("enabled: %v", n.Enabled)))
	b.WriteString("\n")
	wrote = true
	if n.DurationMs > 0 {
		b.WriteString(detailStyle.Render(fmt.Sprintf("duration_ms: %d", n.DurationMs)))
		b.WriteString("\n")
	}
	if n.AgentCount > 0 {
		b.WriteString(detailStyle.Render(fmt.Sprintf("agent_count: %d", n.AgentCount)))
		b.WriteString("\n")
	}

	// Wizard-specific metrics (only for wizard step)
	if n.Step == "wizard" && wizardResp != nil {
		b.WriteString(wizardStyle.Render("── Wizard Metrics ──"))
		b.WriteString("\n")
		b.WriteString(wizardStyle.Render(fmt.Sprintf("token_count: %d", wizardResp.TokenCount)))
		b.WriteString("\n")
		if wizardResp.FirstTokenMs > 0 {
			b.WriteString(wizardStyle.Render(fmt.Sprintf("first_token_ms: %d", wizardResp.FirstTokenMs)))
			b.WriteString("\n")
		}
		if wizardResp.DurationMs > 0 {
			b.WriteString(wizardStyle.Render(fmt.Sprintf("duration_ms: %d", wizardResp.DurationMs)))
			b.WriteString("\n")
			// Calculate tokens/sec (cleaner formula: tokens * 1000 / ms)
			if wizardResp.TokenCount > 0 {
				tokensPerSec := float64(wizardResp.TokenCount) * 1000.0 / float64(wizardResp.DurationMs)
				b.WriteString(wizardStyle.Render(fmt.Sprintf("tokens/sec: %.1f", tokensPerSec)))
				b.WriteString("\n")
			}
		}
		// Show plan_id from synthesis step prompt_ref if available
		if n.PromptRef != nil {
			if planID, ok := n.PromptRef["synthesis_plan_id"]; ok {
				b.WriteString(wizardStyle.Render(fmt.Sprintf("plan_id: %v", planID)))
				b.WriteString("\n")
			}
		}
		wrote = true
	}

	if n.RoutingMode != "" {
		b.WriteString(detailStyle.Render(fmt.Sprintf("mode: %s", n.RoutingMode)))
		b.WriteString("\n")
		wrote = true
	}
	if n.RouteReason != "" {
		b.WriteString(detailStyle.Render(fmt.Sprintf("reason: %s", n.RouteReason)))
		b.WriteString("\n")
		wrote = true
	}
	if m.showPromptRef && n.PromptRef != nil && len(n.PromptRef) > 0 {
		for k, v := range n.PromptRef {
			b.WriteString(detailStyle.Render(fmt.Sprintf("%s: %v", k, v)))
			b.WriteString("\n")
		}
		wrote = true
	}

	if !wrote {
		b.WriteString(detailStyle.Render("No details. Press 'p' to show YAML refs."))
		b.WriteString("\n")
	}

	return b.String()
}

// renderAppPane renders the App pane (F2) showing wizard output and agent summaries
func (m InteractiveModel) renderAppPane() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	focusStyle := lipgloss.NewStyle().Bold(true).Underline(true)

	b.WriteString(headerStyle.Render("App Output"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("─────────────────────────────────────────"))
	b.WriteString("\n\n")

	if len(m.messages) == 0 {
		b.WriteString(dimStyle.Render("No messages yet."))
		return b.String()
	}

	msg := m.messages[len(m.messages)-1]

	// Render based on focus: wizard first if focused, agents first if focused
	if m.appFocus == AppFocusWizard {
		b.WriteString(m.renderSynthesisSection(msg))
		b.WriteString(m.renderWizardSection(msg, focusStyle))
		b.WriteString(m.renderAgentSummaries(msg))
	} else {
		b.WriteString(m.renderAgentSummaries(msg))
		b.WriteString(m.renderSynthesisSection(msg))
		b.WriteString(m.renderWizardSection(msg, lipgloss.NewStyle()))
	}

	return b.String()
}

// renderSynthesisSection renders the synthesis output (separate from wizard)
func (m InteractiveModel) renderSynthesisSection(msg Message) string {
	var b strings.Builder

	// Look for synthesis_detail events in the message
	var synthesisContent string
	var synthesisPlanID string
	var synthesisMethod string
	var sourcesCombined int

	for _, evt := range msg.Events {
		if evt.Type == events.SynthesisDetail && evt.SynthesisDetail != nil {
			synthesisContent = evt.SynthesisDetail.SynthesisFull
			synthesisPlanID = evt.SynthesisDetail.PlanID
			synthesisMethod = evt.SynthesisDetail.SynthesisMethod
			sourcesCombined = evt.SynthesisDetail.SourcesCombined
			break
		}
	}

	if synthesisContent == "" {
		return ""
	}

	synthesisStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Italic(true)

	b.WriteString(synthesisStyle.Render("Synthesis Output"))
	b.WriteString("\n")

	// Show synthesis metadata
	if synthesisPlanID != "" || synthesisMethod != "" {
		var meta []string
		if synthesisPlanID != "" {
			meta = append(meta, fmt.Sprintf("plan: %s", synthesisPlanID))
		}
		if synthesisMethod != "" {
			meta = append(meta, fmt.Sprintf("method: %s", synthesisMethod))
		}
		if sourcesCombined > 0 {
			meta = append(meta, fmt.Sprintf("sources: %d", sourcesCombined))
		}
		b.WriteString(metaStyle.Render("[Synthesis] " + strings.Join(meta, ", ")))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(synthesisContent)
	b.WriteString("\n\n")
	return b.String()
}

// renderWizardSection renders the wizard output section
func (m InteractiveModel) renderWizardSection(msg Message, titleStyle lipgloss.Style) string {
	var b strings.Builder
	wizard, ok := msg.AgentResponses["wizard"]
	if !ok || wizard.FullContent == "" {
		return ""
	}

	wizardStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true)
	if titleStyle.GetUnderline() {
		b.WriteString(titleStyle.Foreground(lipgloss.Color("13")).Render("Wizard Output"))
	} else {
		b.WriteString(wizardStyle.Render("Wizard Output"))
	}
	b.WriteString("\n")

	// Show wizard debug summary above content
	b.WriteString(m.renderWizardDebugSummary(wizard))
	b.WriteString("\n")

	b.WriteString(wizard.FullContent)
	b.WriteString("\n\n")
	return b.String()
}

// renderWizardDebugSummary renders a compact one-line wizard metrics summary
func (m InteractiveModel) renderWizardDebugSummary(wizard *AgentResponse) string {
	if wizard == nil {
		return ""
	}

	summaryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Italic(true)

	var parts []string

	// Token count
	parts = append(parts, fmt.Sprintf("tokens: %d", wizard.TokenCount))

	// Tokens per second (only if we have duration)
	if wizard.DurationMs > 0 && wizard.TokenCount > 0 {
		tokensPerSec := float64(wizard.TokenCount) * 1000.0 / float64(wizard.DurationMs)
		parts = append(parts, fmt.Sprintf("rate: %.1f tok/s", tokensPerSec))
	}

	// First token latency
	if wizard.FirstTokenMs > 0 {
		parts = append(parts, fmt.Sprintf("first-token: %dms", wizard.FirstTokenMs))
	}

	// Status indicator
	if wizard.Completed {
		parts = append(parts, "✓")
	} else {
		parts = append(parts, "⏳")
	}

	return summaryStyle.Render("[Wizard] " + strings.Join(parts, ", "))
}

// renderAgentSummaries renders the agent summaries section with collapse/expand
func (m InteractiveModel) renderAgentSummaries(msg Message) string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))

	// Sort agents for stable display (excluding wizard)
	agentIDs := make([]string, 0, len(msg.AgentResponses))
	for id := range msg.AgentResponses {
		if id != "wizard" {
			agentIDs = append(agentIDs, id)
		}
	}
	sort.Strings(agentIDs)

	if len(agentIDs) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render(fmt.Sprintf("Agents (%d)", len(agentIDs))))
	b.WriteString("\n")

	for _, agentID := range agentIDs {
		resp := msg.AgentResponses[agentID]
		if resp == nil || resp.FullContent == "" {
			continue
		}

		agentColor := m.getAgentColor(agentID)
		agentStyle := lipgloss.NewStyle().Foreground(agentColor).Bold(true)

		// Collapse/expand indicator
		collapsed := m.agentCollapsed[agentID]
		caret := "▼"
		if collapsed {
			caret = "▶"
		}

		// Status indicator
		status := ""
		if resp.Completed {
			status = " ✓"
		}

		header := fmt.Sprintf("%s %s (%d tokens)%s", caret, agentID, resp.TokenCount, status)
		b.WriteString(agentStyle.Render(header))
		b.WriteString("\n")

		// Show content only if not collapsed
		if !collapsed {
			b.WriteString(lipgloss.NewStyle().Foreground(agentColor).Render(resp.FullContent))
			b.WriteString("\n\n")
		}
	}

	return b.String()
}

// lineDiff computes a simple line-based diff between two strings
func lineDiff(a, b string) string {
	al := strings.Split(a, "\n")
	bl := strings.Split(b, "\n")

	// Build sets for comparison
	aSet := make(map[string]struct{})
	for _, s := range al {
		aSet[s] = struct{}{}
	}
	bSet := make(map[string]struct{})
	for _, s := range bl {
		bSet[s] = struct{}{}
	}

	var out []string

	// Lines in new but not in old (added)
	for _, s := range bl {
		if _, ok := aSet[s]; !ok {
			out = append(out, "+ "+s)
		} else {
			out = append(out, "  "+s)
		}
	}

	// Lines in old but not in new (removed)
	for _, s := range al {
		if _, ok := bSet[s]; !ok {
			out = append(out, "- "+s)
		}
	}

	return strings.Join(out, "\n")
}

// renderYAMLPane renders the YAML pane (F3) showing config snapshot
func (m InteractiveModel) renderYAMLPane() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))   // green
	removeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")) // red

	b.WriteString(headerStyle.Render("YAML Configuration"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("─────────────────────────────────────────"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("R: reload prompts | y: take snapshot"))
	b.WriteString("\n\n")

	if m.yamlReloadStatus != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render(m.yamlReloadStatus))
		b.WriteString("\n\n")
	}

	// Show diff if available
	if m.yamlDiff != "" {
		b.WriteString(headerStyle.Render("Snapshot Diff"))
		b.WriteString("\n")
		// Color-code diff lines
		for _, line := range strings.Split(m.yamlDiff, "\n") {
			if strings.HasPrefix(line, "+ ") {
				b.WriteString(addStyle.Render(line))
			} else if strings.HasPrefix(line, "- ") {
				b.WriteString(removeStyle.Render(line))
			} else {
				b.WriteString(line)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if m.yamlSnapshot == "" {
		b.WriteString(dimStyle.Render("No snapshot. Press 'y' to capture current config."))
	} else {
		b.WriteString(headerStyle.Render("Current Snapshot"))
		b.WriteString("\n")
		b.WriteString(m.yamlSnapshot)
	}

	return b.String()
}

// renderTimelinePane renders the Timeline pane (F4) showing agent execution timeline
func (m InteractiveModel) renderTimelinePane() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	b.WriteString(headerStyle.Render("Timeline"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("─────────────────────────────────────────"))
	b.WriteString("\n\n")

	if len(m.messages) == 0 {
		b.WriteString(dimStyle.Render("No messages yet."))
		return b.String()
	}

	msg := m.messages[len(m.messages)-1]

	// Show stage durations from flow events
	nodes := m.buildFlowNodes(msg.Events)
	stages := []string{"routing", "discovery", "agent_exec", "filter", "synthesis", "wizard"}

	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Stages"))
	b.WriteString("\n")

	totalDuration := 0
	for _, stage := range stages {
		if n := nodes[stage]; n != nil && n.DurationMs > 0 {
			totalDuration += n.DurationMs
		}
	}

	for _, stage := range stages {
		n := nodes[stage]
		if n == nil {
			continue
		}

		// Calculate bar width proportional to duration
		barWidth := 0
		if totalDuration > 0 && n.DurationMs > 0 {
			barWidth = (n.DurationMs * 40) / totalDuration
			if barWidth < 1 {
				barWidth = 1
			}
		}

		bar := strings.Repeat("█", barWidth)

		b.WriteString(fmt.Sprintf("%-12s %s %dms\n", stage, bar, n.DurationMs))
	}

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Total: %dms\n", totalDuration))

	// Show agent timeline bars
	if len(msg.AgentResponses) > 0 {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Agents"))
		b.WriteString("\n")

		// Calculate max tokens for bar scaling
		maxTokens := 0
		for _, resp := range msg.AgentResponses {
			if resp.TokenCount > maxTokens {
				maxTokens = resp.TokenCount
			}
		}

		// Sort agents for stable display
		agentIDs := make([]string, 0, len(msg.AgentResponses))
		for id := range msg.AgentResponses {
			agentIDs = append(agentIDs, id)
		}
		sort.Strings(agentIDs)

		for _, agentID := range agentIDs {
			resp := msg.AgentResponses[agentID]
			status := "⏳"
			if resp.Completed {
				status = "✓"
			}

			// Calculate bar width proportional to tokens
			barWidth := 0
			if maxTokens > 0 && resp.TokenCount > 0 {
				barWidth = (resp.TokenCount * 30) / maxTokens
				if barWidth < 1 {
					barWidth = 1
				}
			}

			bar := strings.Repeat("▓", barWidth)
			agentColor := m.getAgentColor(agentID)
			barStyled := lipgloss.NewStyle().Foreground(agentColor).Render(bar)

			b.WriteString(fmt.Sprintf("%s %-15s %s %d tokens\n", status, agentID, barStyled, resp.TokenCount))
		}
	}

	return b.String()
}

// isEventVisible checks if an event should be visible based on current filter settings
func (m InteractiveModel) isEventVisible(evt *events.Event) bool {
	// Check if this is a wizard-related event
	isWizardEvent := evt.Type == events.WizardStreamStart ||
		evt.Type == events.WizardContent ||
		evt.Type == events.WizardStreamComplete
	// synthesis flow_step events are also relevant for wizard-only view
	isSynthesisFlowEvent := (evt.Type == events.FlowStepStart || evt.Type == events.FlowStepEnd) &&
		((evt.FlowStepStart != nil && (evt.FlowStepStart.Step == "synthesis" || evt.FlowStepStart.Step == "wizard")) ||
			(evt.FlowStepEnd != nil && (evt.FlowStepEnd.Step == "synthesis" || evt.FlowStepEnd.Step == "wizard")))

	// Skip non-wizard events if wizard-only filter is on
	if m.eventsWizardOnly && !isWizardEvent && !isSynthesisFlowEvent {
		return false
	}

	// Skip token events if showTokens is false
	isTokenEvent := evt.Type == events.AgentContent || evt.Type == events.WizardContent
	if isTokenEvent && !m.showTokens {
		return false
	}

	return true
}

// countVisibleEvents counts events that are visible after filtering
func (m InteractiveModel) countVisibleEvents() int {
	if len(m.messages) == 0 {
		return 0
	}
	msg := m.messages[len(m.messages)-1]
	count := 0
	for _, evt := range msg.Events {
		if m.isEventVisible(evt) {
			count++
		}
	}
	return count
}

// renderEventsPane renders the Events pane (F5) showing SSE events
func (m InteractiveModel) renderEventsPane() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	wizardStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true)

	viewLabel := "PARSED"
	if m.viewMode == ViewModeRaw {
		viewLabel = "RAW"
	}
	tokensLabel := "OFF"
	if m.showTokens {
		tokensLabel = "ON"
	}
	wizardOnlyLabel := "OFF"
	if m.eventsWizardOnly {
		wizardOnlyLabel = "ON"
	}

	b.WriteString(headerStyle.Render("Events"))
	b.WriteString(fmt.Sprintf(" (View: %s)", viewLabel))
	if m.viewMode == ViewModeParsed {
		b.WriteString(fmt.Sprintf(" (Tokens: %s)", tokensLabel))
		b.WriteString(fmt.Sprintf(" (Wizard: %s)", wizardOnlyLabel))
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("─────────────────────────────────────────"))
	b.WriteString("\n")
	// Hints
	if m.viewMode == ViewModeParsed {
		b.WriteString(dimStyle.Render("Ctrl+T: RAW view, t: tokens, W: wizard-only"))
	} else {
		b.WriteString(dimStyle.Render("Ctrl+T: PARSED view"))
	}
	b.WriteString("\n")

	// Show wizard metrics summary when in parsed mode
	if m.viewMode == ViewModeParsed && len(m.messages) > 0 {
		msg := m.messages[len(m.messages)-1]
		if msg.AgentResponses != nil {
			if wizardResp := msg.AgentResponses["wizard"]; wizardResp != nil {
				var metricsLine strings.Builder
				metricsLine.WriteString(fmt.Sprintf("🧙 Wizard: %d tokens", wizardResp.TokenCount))
				if wizardResp.DurationMs > 0 && wizardResp.TokenCount > 0 {
					tokensPerSec := float64(wizardResp.TokenCount) * 1000.0 / float64(wizardResp.DurationMs)
					metricsLine.WriteString(fmt.Sprintf(" | %.1f tok/s", tokensPerSec))
				}
				if wizardResp.FirstTokenMs > 0 {
					metricsLine.WriteString(fmt.Sprintf(" | first: %dms", wizardResp.FirstTokenMs))
				}
				if !wizardResp.Completed {
					metricsLine.WriteString(" ⏳")
				} else {
					metricsLine.WriteString(" ✓")
				}
				b.WriteString(wizardStyle.Render(metricsLine.String()))
				b.WriteString("\n")
			}
		}
	}
	b.WriteString("\n")

	if len(m.messages) == 0 {
		b.WriteString(dimStyle.Render("No messages yet."))
		return b.String()
	}

	msg := m.messages[len(m.messages)-1]

	// RAW view: pretty-print the raw SSE of the last message
	if m.viewMode == ViewModeRaw {
		if msg.RawSSE == "" {
			b.WriteString(dimStyle.Render("No raw SSE captured for latest message."))
			return b.String()
		}
		b.WriteString(m.renderRawView(msg))
		return b.String()
	}

	// PARSED view
	if len(msg.Events) == 0 {
		b.WriteString(dimStyle.Render("No events in latest message."))
		return b.String()
	}

	// Styles for event rendering
	eventStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	tokenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	wizardEventStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	selectedStyle := lipgloss.NewStyle().Background(lipgloss.Color("8")).Foreground(lipgloss.Color("15"))
	expandedContentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7")).PaddingLeft(4)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)

	// Track visible event index (for selection/expansion which counts visible events only)
	visibleIdx := 0

	for _, evt := range msg.Events {
		// Skip events that don't pass the filter
		if !m.isEventVisible(evt) {
			continue
		}

		// Check if this is a wizard-related event (for styling)
		isWizardEvent := evt.Type == events.WizardStreamStart ||
			evt.Type == events.WizardContent ||
			evt.Type == events.WizardStreamComplete
		isSynthesisFlowEvent := (evt.Type == events.FlowStepStart || evt.Type == events.FlowStepEnd) &&
			((evt.FlowStepStart != nil && (evt.FlowStepStart.Step == "synthesis" || evt.FlowStepStart.Step == "wizard")) ||
				(evt.FlowStepEnd != nil && (evt.FlowStepEnd.Step == "synthesis" || evt.FlowStepEnd.Step == "wizard")))
		isTokenEvent := evt.Type == events.AgentContent || evt.Type == events.WizardContent

		// All events are expandable - show raw JSON when expanded
		isExpandable := true

		isExpanded := m.eventExpanded[visibleIdx]
		isSelected := visibleIdx == m.selectedEventIdx

		// Selection cursor and expand/collapse icon
		var prefix string
		if isSelected {
			prefix = "→ "
		} else {
			prefix = "  "
		}
		if isExpandable {
			if isExpanded {
				prefix += "▼ "
			} else {
				prefix += "▶ "
			}
		} else {
			prefix += "  "
		}

		// Format event
		line := fmt.Sprintf("%s[%s]", prefix, evt.Type)

		// Add relevant details (compact summary on main line)
		switch evt.Type {
		case events.FlowStepStart:
			if evt.FlowStepStart != nil {
				line += fmt.Sprintf(" step=%s enabled=%v", evt.FlowStepStart.Step, evt.FlowStepStart.Enabled)
			}
		case events.FlowStepEnd:
			if evt.FlowStepEnd != nil {
				line += fmt.Sprintf(" step=%s duration=%dms", evt.FlowStepEnd.Step, evt.FlowStepEnd.DurationMs)
			}
		case events.AgentStreamStart:
			if evt.AgentStreamStart != nil {
				line += fmt.Sprintf(" agent=%s", evt.AgentStreamStart.AgentID)
			}
		case events.AgentStreamComplete:
			if evt.AgentStreamComplete != nil {
				line += fmt.Sprintf(" agent=%s tokens=%d", evt.AgentStreamComplete.AgentID, evt.AgentStreamComplete.TokenCount)
			}
		case events.AgentContent:
			if evt.AgentContent != nil {
				content := evt.AgentContent.Content
				if len(content) > 30 {
					content = content[:30] + "..."
				}
				line += fmt.Sprintf(" agent=%s content=%q", evt.AgentContent.AgentID, content)
			}
		case events.WizardContent:
			if evt.WizardContent != nil {
				content := evt.WizardContent.Content
				if len(content) > 30 {
					content = content[:30] + "..."
				}
				line += fmt.Sprintf(" content=%q", content)
			}
		// New debug event types - show compact summary
		case events.FilterDetail:
			if evt.FilterDetail != nil {
				line += fmt.Sprintf(" agent=%s thinking=%d filtered=%d", evt.FilterDetail.AgentID, evt.FilterDetail.OriginalLength, evt.FilterDetail.FilteredLength)
			}
		case events.PerspectiveDetail:
			if evt.PerspectiveDetail != nil {
				line += fmt.Sprintf(" agent=%s relevance=%.2f", evt.PerspectiveDetail.AgentID, evt.PerspectiveDetail.RelevanceScore)
			}
		case events.SynthesisDetail:
			if evt.SynthesisDetail != nil {
				line += fmt.Sprintf(" plan=%s method=%s sources=%d", evt.SynthesisDetail.PlanID, evt.SynthesisDetail.SynthesisMethod, evt.SynthesisDetail.SourcesCombined)
			}
		case events.AgentMetadata:
			if evt.AgentMetadata != nil {
				line += fmt.Sprintf(" agent=%s model=%s tokens=%d latency=%dms", evt.AgentMetadata.AgentID, evt.AgentMetadata.Model, evt.AgentMetadata.TotalTokens, evt.AgentMetadata.LatencyMs)
			}
		case events.PromptInfo:
			if evt.PromptInfo != nil {
				line += fmt.Sprintf(" agent=%s file=%s len=%d", evt.PromptInfo.AgentID, evt.PromptInfo.PromptFile, evt.PromptInfo.PromptLength)
			}
		case events.PromptFull:
			if evt.PromptFull != nil {
				line += fmt.Sprintf(" agent=%s len=%d", evt.PromptFull.AgentID, len(evt.PromptFull.SystemPrompt))
			}
		case events.FlowConfig:
			if evt.FlowConfig != nil {
				line += fmt.Sprintf(" flow=%s agents=%d stages=%d", evt.FlowConfig.FlowID, evt.FlowConfig.AgentCount, len(evt.FlowConfig.StagesOrder))
			}
		case events.FlowStepDetail:
			if evt.FlowStepDetail != nil {
				line += fmt.Sprintf(" step=%s", evt.FlowStepDetail.Step)
			}
		}

		// Apply appropriate styling
		var styledLine string
		if isSelected {
			styledLine = selectedStyle.Render(line)
		} else if isTokenEvent && !isWizardEvent {
			styledLine = tokenStyle.Render(line)
		} else if isWizardEvent || isSynthesisFlowEvent {
			styledLine = wizardEventStyle.Render(line)
		} else {
			styledLine = eventStyle.Render(line)
		}
		b.WriteString(styledLine)
		b.WriteString("\n")

		// Render expanded content if expanded
		if isExpandable && isExpanded {
			var expandedContent strings.Builder
			switch evt.Type {
			case events.FilterDetail:
				if evt.FilterDetail != nil {
					expandedContent.WriteString(labelStyle.Render("Thinking (full):"))
					expandedContent.WriteString("\n")
					expandedContent.WriteString(expandedContentStyle.Render(evt.FilterDetail.ThinkingFull))
					expandedContent.WriteString("\n")
					expandedContent.WriteString(labelStyle.Render("Filtered Response:"))
					expandedContent.WriteString("\n")
					expandedContent.WriteString(expandedContentStyle.Render(evt.FilterDetail.FilteredResponse))
				}
			case events.PerspectiveDetail:
				if evt.PerspectiveDetail != nil {
					expandedContent.WriteString(labelStyle.Render("Perspective (full):"))
					expandedContent.WriteString("\n")
					expandedContent.WriteString(expandedContentStyle.Render(evt.PerspectiveDetail.PerspectiveFull))
					expandedContent.WriteString("\n")
					expandedContent.WriteString(labelStyle.Render("Summary:"))
					expandedContent.WriteString("\n")
					expandedContent.WriteString(expandedContentStyle.Render(evt.PerspectiveDetail.Summary))
					if len(evt.PerspectiveDetail.KeyInsights) > 0 {
						expandedContent.WriteString("\n")
						expandedContent.WriteString(labelStyle.Render("Key Insights:"))
						expandedContent.WriteString("\n")
						for _, insight := range evt.PerspectiveDetail.KeyInsights {
							expandedContent.WriteString(expandedContentStyle.Render("• " + insight))
							expandedContent.WriteString("\n")
						}
					}
				}
			case events.SynthesisDetail:
				if evt.SynthesisDetail != nil {
					expandedContent.WriteString(labelStyle.Render("Synthesis (full):"))
					expandedContent.WriteString("\n")
					expandedContent.WriteString(expandedContentStyle.Render(evt.SynthesisDetail.SynthesisFull))
				}
			case events.AgentMetadata:
				if evt.AgentMetadata != nil {
					expandedContent.WriteString(labelStyle.Render("Model:"))
					expandedContent.WriteString(" " + evt.AgentMetadata.Model + "\n")
					expandedContent.WriteString(labelStyle.Render("Total Tokens:"))
					expandedContent.WriteString(fmt.Sprintf(" %d\n", evt.AgentMetadata.TotalTokens))
					expandedContent.WriteString(labelStyle.Render("Response Length:"))
					expandedContent.WriteString(fmt.Sprintf(" %d chars\n", evt.AgentMetadata.ResponseLength))
					expandedContent.WriteString(labelStyle.Render("Latency:"))
					expandedContent.WriteString(fmt.Sprintf(" %dms\n", evt.AgentMetadata.LatencyMs))
				}
			case events.PromptInfo:
				if evt.PromptInfo != nil {
					expandedContent.WriteString(labelStyle.Render("Prompt File:"))
					expandedContent.WriteString(" " + evt.PromptInfo.PromptFile + "\n")
					expandedContent.WriteString(labelStyle.Render("Snippet:"))
					expandedContent.WriteString("\n")
					expandedContent.WriteString(expandedContentStyle.Render(evt.PromptInfo.PromptSnippet))
				}
			case events.PromptFull:
				if evt.PromptFull != nil {
					expandedContent.WriteString(labelStyle.Render("System Prompt (full):"))
					expandedContent.WriteString("\n")
					expandedContent.WriteString(expandedContentStyle.Render(evt.PromptFull.SystemPrompt))
				}
			case events.FlowConfig:
				if evt.FlowConfig != nil {
					expandedContent.WriteString(labelStyle.Render("Flow File:"))
					expandedContent.WriteString(" " + evt.FlowConfig.FlowFile + "\n")
					expandedContent.WriteString(labelStyle.Render("Stages Order:"))
					expandedContent.WriteString(" " + strings.Join(evt.FlowConfig.StagesOrder, " → ") + "\n")
					expandedContent.WriteString(labelStyle.Render("Routing Mode:"))
					expandedContent.WriteString(" " + evt.FlowConfig.RoutingMode + "\n")
					expandedContent.WriteString(labelStyle.Render("Agent Count:"))
					expandedContent.WriteString(fmt.Sprintf(" %d\n", evt.FlowConfig.AgentCount))
				}
			case events.FlowStepDetail:
				if evt.FlowStepDetail != nil {
					if evt.FlowStepDetail.SynthesisPreview != "" {
						expandedContent.WriteString(labelStyle.Render("Synthesis Preview:"))
						expandedContent.WriteString("\n")
						expandedContent.WriteString(expandedContentStyle.Render(evt.FlowStepDetail.SynthesisPreview))
						expandedContent.WriteString("\n")
					}
					if evt.FlowStepDetail.SynthesisFull != "" {
						expandedContent.WriteString(labelStyle.Render("Synthesis Full:"))
						expandedContent.WriteString("\n")
						expandedContent.WriteString(expandedContentStyle.Render(evt.FlowStepDetail.SynthesisFull))
						expandedContent.WriteString("\n")
					}
					if evt.FlowStepDetail.ThinkingPreview != "" {
						expandedContent.WriteString(labelStyle.Render("Thinking Preview:"))
						expandedContent.WriteString("\n")
						expandedContent.WriteString(expandedContentStyle.Render(evt.FlowStepDetail.ThinkingPreview))
						expandedContent.WriteString("\n")
					}
					if len(evt.FlowStepDetail.Perspectives) > 0 {
						expandedContent.WriteString(labelStyle.Render("Perspectives:"))
						expandedContent.WriteString("\n")
						for _, p := range evt.FlowStepDetail.Perspectives {
							expandedContent.WriteString(expandedContentStyle.Render(fmt.Sprintf("• %s: %s", p.AgentID, p.Summary)))
							expandedContent.WriteString("\n")
						}
					}
				}
			case events.WizardStreamComplete:
				// Show full wizard response from AgentResponses
				if wizard := msg.AgentResponses["wizard"]; wizard != nil && wizard.FullContent != "" {
					expandedContent.WriteString(labelStyle.Render("Wizard Response:"))
					expandedContent.WriteString("\n")
					expandedContent.WriteString(expandedContentStyle.Render(wizard.FullContent))
					expandedContent.WriteString("\n")
					if wizard.TokenCount > 0 {
						expandedContent.WriteString(labelStyle.Render("Tokens:"))
						expandedContent.WriteString(fmt.Sprintf(" %d\n", wizard.TokenCount))
					}
				}
			default:
				// For any event type, show the raw JSON
				if len(evt.Raw) > 0 {
					expandedContent.WriteString(labelStyle.Render("Raw Event Data:"))
					expandedContent.WriteString("\n")
					// Pretty-print the JSON
					var prettyJSON bytes.Buffer
					if err := json.Indent(&prettyJSON, evt.Raw, "", "  "); err == nil {
						expandedContent.WriteString(expandedContentStyle.Render(prettyJSON.String()))
					} else {
						expandedContent.WriteString(expandedContentStyle.Render(string(evt.Raw)))
					}
				}
			}
			if expandedContent.Len() > 0 {
				b.WriteString(expandedContent.String())
				b.WriteString("\n")
			}
		}

		// Increment visible index for next iteration
		visibleIdx++
	}

	return b.String()
}

// clipLeft removes the first x columns (approx bytes) from each line for basic horizontal scrolling
func (m InteractiveModel) clipLeft(s string, off int) string {
	if off <= 0 {
		return s
	}
	var out strings.Builder
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if off >= len(line) {
			// If we clip more than line length, write empty
			// Note: we do not try to be rune-precise for performance; fallback to byte slicing
			// For better unicode handling, clip by runes below
			out.WriteString("")
		} else {
			// Clip by runes to avoid cutting multibyte characters
			out.WriteString(clipRunesLeft(line, off))
		}
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// wrapToWidth wraps long lines to the given width (approx runes)
func (m InteractiveModel) wrapToWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out strings.Builder
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line == "" {
			// Preserve blank lines
			//
		} else {
			// Wrap line by rune count
			runes := []rune(line)
			for start := 0; start < len(runes); start += width {
				end := start + width
				if end > len(runes) {
					end = len(runes)
				}
				out.WriteString(string(runes[start:end]))
				if end < len(runes) {
					out.WriteByte('\n')
				}
			}
		}
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// clipRunesLeft clips n columns (by rune) from the left; if n exceeds line length, returns empty
func clipRunesLeft(line string, n int) string {
	if n <= 0 {
		return line
	}
	// Fast path for ASCII
	if utf8.RuneCountInString(line) == len(line) {
		if n >= len(line) {
			return ""
		}
		return line[n:]
	}
	// Unicode-aware path
	i := 0
	for idx := range line {
		if i == n {
			return line[idx:]
		}
		i++
	}
	return ""
}

// renderEventExpandedPlainText returns the human-readable expanded content for an event
// This is the plain-text version of what the TUI shows when a node is expanded
// Used for Ctrl+S export to file
func (m InteractiveModel) renderEventExpandedPlainText(evt *events.Event, msg *Message) string {
	var b strings.Builder

	switch evt.Type {
	case events.FilterDetail:
		if evt.FilterDetail != nil {
			b.WriteString("Thinking (full):\n")
			b.WriteString(evt.FilterDetail.ThinkingFull)
			b.WriteString("\n\nFiltered Response:\n")
			b.WriteString(evt.FilterDetail.FilteredResponse)
		}
	case events.PerspectiveDetail:
		if evt.PerspectiveDetail != nil {
			b.WriteString("Perspective (full):\n")
			b.WriteString(evt.PerspectiveDetail.PerspectiveFull)
			b.WriteString("\n\nSummary:\n")
			b.WriteString(evt.PerspectiveDetail.Summary)
			if len(evt.PerspectiveDetail.KeyInsights) > 0 {
				b.WriteString("\n\nKey Insights:\n")
				for _, insight := range evt.PerspectiveDetail.KeyInsights {
					b.WriteString("• " + insight + "\n")
				}
			}
		}
	case events.SynthesisDetail:
		if evt.SynthesisDetail != nil {
			b.WriteString("Plan ID: ")
			b.WriteString(evt.SynthesisDetail.PlanID)
			b.WriteString("\nMethod: ")
			b.WriteString(evt.SynthesisDetail.SynthesisMethod)
			b.WriteString(fmt.Sprintf("\nSources Combined: %d", evt.SynthesisDetail.SourcesCombined))
			b.WriteString("\n\nSynthesis (full):\n")
			b.WriteString(evt.SynthesisDetail.SynthesisFull)
		}
	case events.AgentMetadata:
		if evt.AgentMetadata != nil {
			b.WriteString("Model: " + evt.AgentMetadata.Model + "\n")
			b.WriteString(fmt.Sprintf("Total Tokens: %d\n", evt.AgentMetadata.TotalTokens))
			b.WriteString(fmt.Sprintf("Response Length: %d chars\n", evt.AgentMetadata.ResponseLength))
			b.WriteString(fmt.Sprintf("Latency: %dms\n", evt.AgentMetadata.LatencyMs))
		}
	case events.PromptInfo:
		if evt.PromptInfo != nil {
			b.WriteString("Prompt File: " + evt.PromptInfo.PromptFile + "\n")
			b.WriteString(fmt.Sprintf("Prompt Length: %d chars\n", evt.PromptInfo.PromptLength))
			b.WriteString("\nSnippet:\n")
			b.WriteString(evt.PromptInfo.PromptSnippet)
		}
	case events.PromptFull:
		if evt.PromptFull != nil {
			b.WriteString("Agent: " + evt.PromptFull.AgentID + "\n")
			b.WriteString("\nSystem Prompt (full):\n")
			b.WriteString(evt.PromptFull.SystemPrompt)
		}
	case events.FlowConfig:
		if evt.FlowConfig != nil {
			b.WriteString("Flow ID: " + evt.FlowConfig.FlowID + "\n")
			b.WriteString("Flow File: " + evt.FlowConfig.FlowFile + "\n")
			b.WriteString("Stages Order: " + strings.Join(evt.FlowConfig.StagesOrder, " → ") + "\n")
			b.WriteString("Routing Mode: " + evt.FlowConfig.RoutingMode + "\n")
			b.WriteString(fmt.Sprintf("Agent Count: %d\n", evt.FlowConfig.AgentCount))
			b.WriteString(fmt.Sprintf("Non-Wizard Count: %d\n", evt.FlowConfig.NonWizardCount))
			if len(evt.FlowConfig.StagesEnabled) > 0 {
				b.WriteString("Stages Enabled:\n")
				for stage, enabled := range evt.FlowConfig.StagesEnabled {
					b.WriteString(fmt.Sprintf("  %s: %v\n", stage, enabled))
				}
			}
		}
	case events.FlowStepStart:
		if evt.FlowStepStart != nil {
			b.WriteString("Step: " + evt.FlowStepStart.Step + "\n")
			b.WriteString(fmt.Sprintf("Enabled: %v\n", evt.FlowStepStart.Enabled))
			if evt.FlowStepStart.AgentCount > 0 {
				b.WriteString(fmt.Sprintf("Agent Count: %d\n", evt.FlowStepStart.AgentCount))
			}
			if evt.FlowStepStart.NonWizardCount > 0 {
				b.WriteString(fmt.Sprintf("Non-Wizard Count: %d\n", evt.FlowStepStart.NonWizardCount))
			}
		}
	case events.FlowStepEnd:
		if evt.FlowStepEnd != nil {
			b.WriteString("Step: " + evt.FlowStepEnd.Step + "\n")
			b.WriteString(fmt.Sprintf("Enabled: %v\n", evt.FlowStepEnd.Enabled))
			if evt.FlowStepEnd.DurationMs > 0 {
				b.WriteString(fmt.Sprintf("Duration: %dms\n", evt.FlowStepEnd.DurationMs))
			}
			if evt.FlowStepEnd.RoutingMode != "" {
				b.WriteString("Routing Mode: " + evt.FlowStepEnd.RoutingMode + "\n")
			}
			if evt.FlowStepEnd.RouteTaken != "" {
				b.WriteString("Route Taken: " + evt.FlowStepEnd.RouteTaken + "\n")
			}
			if evt.FlowStepEnd.RouteReason != "" {
				b.WriteString("Route Reason: " + evt.FlowStepEnd.RouteReason + "\n")
			}
			if len(evt.FlowStepEnd.RouteAgents) > 0 {
				b.WriteString("Route Agents: " + strings.Join(evt.FlowStepEnd.RouteAgents, ", ") + "\n")
			}
		}
	case events.FlowStepDetail:
		if evt.FlowStepDetail != nil {
			b.WriteString("Step: " + evt.FlowStepDetail.Step + "\n")
			if evt.FlowStepDetail.PlanID != "" {
				b.WriteString("Plan ID: " + evt.FlowStepDetail.PlanID + "\n")
			}
			if evt.FlowStepDetail.SynthesisPreview != "" {
				b.WriteString("\nSynthesis Preview:\n")
				b.WriteString(evt.FlowStepDetail.SynthesisPreview)
				b.WriteString("\n")
			}
			if evt.FlowStepDetail.SynthesisFull != "" {
				b.WriteString("\nSynthesis Full:\n")
				b.WriteString(evt.FlowStepDetail.SynthesisFull)
				b.WriteString("\n")
			}
			if evt.FlowStepDetail.ThinkingPreview != "" {
				b.WriteString("\nThinking Preview:\n")
				b.WriteString(evt.FlowStepDetail.ThinkingPreview)
				b.WriteString("\n")
			}
			if len(evt.FlowStepDetail.Perspectives) > 0 {
				b.WriteString("\nPerspectives:\n")
				for _, p := range evt.FlowStepDetail.Perspectives {
					b.WriteString(fmt.Sprintf("• %s: %s\n", p.AgentID, p.Summary))
				}
			}
			if len(evt.FlowStepDetail.Agents) > 0 {
				b.WriteString("\nAgents: " + strings.Join(evt.FlowStepDetail.Agents, ", ") + "\n")
			}
		}
	case events.WizardStreamStart:
		if evt.WizardStreamStart != nil {
			b.WriteString("Message ID: " + evt.WizardStreamStart.MessageID + "\n")
		}
	case events.WizardStreamComplete:
		// Show full wizard response from AgentResponses
		if msg != nil {
			if wizard := msg.AgentResponses["wizard"]; wizard != nil && wizard.FullContent != "" {
				b.WriteString("Wizard Response:\n")
				b.WriteString(wizard.FullContent)
				b.WriteString("\n")
				if wizard.TokenCount > 0 {
					b.WriteString(fmt.Sprintf("\nTokens: %d\n", wizard.TokenCount))
				}
			}
		}
	case events.AgentStreamStart:
		if evt.AgentStreamStart != nil {
			b.WriteString("Agent: " + evt.AgentStreamStart.AgentID + "\n")
			b.WriteString("Message ID: " + evt.AgentStreamStart.MessageID + "\n")
		}
	case events.AgentStreamComplete:
		if evt.AgentStreamComplete != nil {
			b.WriteString("Agent: " + evt.AgentStreamComplete.AgentID + "\n")
			b.WriteString(fmt.Sprintf("Token Count: %d\n", evt.AgentStreamComplete.TokenCount))
			// Show full agent response from AgentResponses
			if msg != nil {
				if agent := msg.AgentResponses[evt.AgentStreamComplete.AgentID]; agent != nil && agent.FullContent != "" {
					b.WriteString("\nFull Response:\n")
					b.WriteString(agent.FullContent)
				}
			}
		}
	case events.SessionStart:
		if evt.SessionStart != nil {
			b.WriteString("Session ID: " + evt.SessionStart.SessionID + "\n")
			b.WriteString("Message ID: " + evt.SessionStart.MessageID + "\n")
			if evt.SessionStart.FlowID != "" {
				b.WriteString("Flow ID: " + evt.SessionStart.FlowID + "\n")
			}
		}
	case events.SessionComplete:
		if evt.SessionComplete != nil {
			b.WriteString("Session ID: " + evt.SessionComplete.SessionID + "\n")
		}
	case events.Error:
		if evt.Error != nil {
			b.WriteString("Error Type: " + string(evt.Error.ErrorType) + "\n")
			b.WriteString("Message: " + evt.Error.Message + "\n")
			if evt.Error.Details != "" {
				b.WriteString("Details: " + evt.Error.Details + "\n")
			}
			if evt.Error.AgentID != "" {
				b.WriteString("Agent: " + evt.Error.AgentID + "\n")
			}
		}
	default:
		// For any unknown event type, show the raw JSON (pretty-printed)
		if len(evt.Raw) > 0 {
			b.WriteString("Raw Event Data:\n")
			var prettyJSON bytes.Buffer
			if err := json.Indent(&prettyJSON, evt.Raw, "", "  "); err == nil {
				b.WriteString(prettyJSON.String())
			} else {
				b.WriteString(string(evt.Raw))
			}
		}
	}
	return b.String()
}

// urlQueryEscape safely escapes a message for URL query use
func urlQueryEscape(s string) string { return neturl.QueryEscape(s) }

// startStreamingCmd initiates the streaming request and hands off to chunk reader
func (m InteractiveModel) startStreamingCmd(message string, index int) tea.Cmd {
	return func() tea.Msg {
		url := m.cfg.StreamEndpointURL()
		fmt.Fprintf(os.Stderr, "DEBUG: Calling URL: %s\n", url)
		fmt.Fprintf(os.Stderr, "DEBUG: BaseURL=%q Endpoint=%q SessionID=%q\n",
			m.cfg.Backend.BaseURL, m.cfg.Backend.StreamEndpoint, m.cfg.Session.ID)

		requestBody := map[string]interface{}{"message": message, "stream": true}
		// Pass through stream debug level if set (enables flow_step_detail synthesis output)
		if lvl := m.cfg.Debug.Level; lvl != "" {
			requestBody["debug"] = lvl
		}
		jsonData, err := json.Marshal(requestBody)
		if err != nil {
			return streamErrorMsg{err: fmt.Errorf("failed to marshal request: %w", err)}
		}

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return streamErrorMsg{err: fmt.Errorf("failed to create request: %w", err)}
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", m.apiKey))
		req.Header.Set("Accept", "text/event-stream")

		clientHTTP := &http.Client{} // no timeout; SSE is long-lived
		ctx, cancel := context.WithCancel(context.Background())
		req = req.WithContext(ctx)

		resp, err := clientHTTP.Do(req)
		if err != nil {
			cancel() // Cancel context on error to avoid leak
			return streamErrorMsg{err: fmt.Errorf("failed to send request: %w", err)}
		}

		if resp.StatusCode == http.StatusMethodNotAllowed {
			_ = resp.Body.Close()
			getURL := fmt.Sprintf("%s?message=%s", url, urlQueryEscape(message))
			req, err = http.NewRequest("GET", getURL, nil)
			if err != nil {
				cancel() // Cancel context on error to avoid leak
				return streamErrorMsg{err: fmt.Errorf("failed to create GET request: %w", err)}
			}
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", m.apiKey))
			req.Header.Set("Accept", "text/event-stream")
			req = req.WithContext(ctx) // Ensure GET request also uses the cancellable context
			resp, err = clientHTTP.Do(req)
			if err != nil {
				cancel() // Cancel context on error to avoid leak
				return streamErrorMsg{err: fmt.Errorf("failed to send GET request: %w", err)}
			}
		}
		if resp.StatusCode != http.StatusOK {
			cancel() // Cancel context on error to avoid leak
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return streamErrorMsg{err: fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))}
		}
		return streamStartMsg{index: index, body: resp.Body, cancel: cancel}
	}
}

// readStreamChunkCmd reads the next chunk from the active stream and returns a chunk message
func (m InteractiveModel) readStreamChunkCmd() tea.Cmd {
	// capture the current ReadCloser
	r := m.streamBody
	idx := m.streamIndex
	return func() tea.Msg {
		if r == nil {
			return streamChunkMsg{index: idx, eof: true}
		}
		buf := make([]byte, 4096)
		n, err := r.Read(buf)
		if err != nil {
			if err == io.EOF {
				return streamChunkMsg{index: idx, chunk: buf[:n], eof: true}
			}
			return streamChunkMsg{index: idx, err: err}
		}
		return streamChunkMsg{index: idx, chunk: buf[:n]}
	}
}

// Message types
type streamCompleteMsg struct {
	index          int
	rawSSE         string
	events         []*events.Event
	agentResponses map[string]*AgentResponse
}

type streamErrorMsg struct {
	err error
}

// streamStartMsg signals beginning of incremental streaming
type streamStartMsg struct {
	index  int
	body   io.ReadCloser
	cancel context.CancelFunc
}

// streamChunkMsg carries data chunk from the streaming response
type streamChunkMsg struct {
	index int
	chunk []byte
	eof   bool
	err   error
}

// newSessionCmd generates a fresh session id, sets it, calls setup, and resets state
func (m InteractiveModel) newSessionCmd() tea.Cmd {
	return func() tea.Msg {
		// Generate new session id
		newID := fmt.Sprintf("debug-session-%s", time.Now().Format("20060102-150405"))
		m.cfg.Session.ID = newID

		// Recreate logger for new session
		if m.slog != nil {
			_ = m.slog.Close()
		}
		legacy := &config.Config{
			BackendURL:       m.cfg.Backend.BaseURL,
			APIKey:           m.cfg.APIKey,
			SessionID:        m.cfg.Session.ID,
			LogDir:           m.cfg.LogDir,
			EnableColors:     m.cfg.EnableColors,
			MaxAgentsVisible: m.cfg.MaxAgentsVisible,
		}
		if l, err := dblogger.NewStructuredLogger(legacy); err == nil {
			m.slog = l
		}

		// Call session setup (auto create)
		setupClient := clientapi.NewSessionSetupClient(m.cfg, m.cfg.APIKey)
		if resp, err := setupClient.CreateOrGetSession(); err == nil {
			// Ensure we track the actual backend session id
			m.cfg.Session.ID = resp.SessionID
		}

		// Reset UI state (clear all prior content and counters)
		m.messages = make([]Message, 0)
		m.err = nil
		m.flowTurnIndex = 0
		m.flowContinuous = true
		m.selectedStepIndex = 0
		m.flowExpanded = make(map[string]bool)
		m.showTokens = false
		m.eventsWizardOnly = false
		m.appFocus = AppFocusAgents
		m.viewport.SetContent("")
		m.contentDirty = true
		m.refreshViewportContent()
		return nil
	}
}
