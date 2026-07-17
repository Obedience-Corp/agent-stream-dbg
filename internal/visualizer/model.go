package visualizer

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	PaneTimeline
	PaneEvents
	PaneMessages
)

// AppFocus represents the focused section in App pane
type AppFocus int

const (
	AppFocusAggregator AppFocus = iota
	AppFocusAgents
)

// FlowLane is a dialect-declared or observed lane in the flow projection.
// Role is semantic metadata supplied by flow.lanes, not a wire-name guess.
type FlowLane struct {
	SourceID string
	Role     events.Role
	Label    string
}

// FlowStage is a stage in the flow projection. A stage role is resolved from
// the matching lane source when the dialect declares one.
type FlowStage struct {
	Name string
	Role events.Role
}

// FlowModel is the renderer-facing flow projection shared by both TUI models.
type FlowModel struct {
	Lanes  []FlowLane
	Stages []FlowStage
}

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
	Role          events.Role
	ContentChunks []string
	FullContent   string
	TokenCount    int
	StartTime     time.Time
	EndTime       time.Time
	Completed     bool
	// Aggregator-role-only metrics (only populated for the lane whose
	// role resolves to aggregator, per the dialect's flow: config)
	FirstTokenMs int64 // Time from stream_start to first content event (ms)
	DurationMs   int64 // Total duration from stream_start to stream_complete (ms)
}

// InteractiveModel represents the interactive TUI state
type InteractiveModel struct {
	cfg      *config.EnhancedConfig
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
	activePane           Pane
	selectedStepIndex    int
	flowExpanded         map[string]bool // track expanded/collapsed state per step
	showTokens           bool            // Events pane: show token events
	showPromptRef        bool            // Flow pane: show prompt references
	eventsAggregatorOnly bool            // Events pane: show only aggregator events when true

	// App pane state
	appFocus       AppFocus
	agentCollapsed map[string]bool // per-agent collapsed state

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

	// dialectFlow resolves stage order (and, later, lane roles) from the
	// loaded dialect's flow: spec, or derives them live from observed
	// events when it declared none.
	dialectFlow flowState
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
func NewInteractiveModel(cfg *config.EnhancedConfig) InteractiveModel {
	ta := textarea.New()
	ta.Placeholder = "Type your message and press Enter to send (Ctrl+C to quit)..."
	ta.Focus()
	ta.CharLimit = 1000
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	// Initialize viewport for scrolling
	vp := viewport.New(80, 20)

	// Initialize structured logger
	var slog *dblogger.StructuredLogger
	if l, err := dblogger.NewStructuredLogger(cfg); err == nil {
		slog = l
	}

	m := InteractiveModel{
		cfg:          cfg,
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
		// Events pane expandable nodes
		eventExpanded:    make(map[int]bool),
		selectedEventIdx: 0,
		dialectFlow:      newFlowState(),
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
