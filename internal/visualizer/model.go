package visualizer

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/config"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	dblogger "github.com/Obedience-Corp/agent-stream-dbg/internal/logger"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
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
	streamTransport transport.Transport
	streamIndex     int
	streamContext   context.Context
	sessionReady    bool

	// Flow pane state
	flowTurnIndex  int  // which message index is displayed in Flow (default latest)
	flowContinuous bool // when true, render all turns continuously

	// Events pane state (expandable nodes)
	eventExpanded    map[int]bool // event index -> expanded state
	selectedEventIdx int          // cursor position in events list

	// Save status message
	saveStatus string

	// Config panel state and the source file used by Ctrl+S in the panel.
	configPanel configPanel
	configPath  string

	// dialectFlow resolves stage order (and, later, lane roles) from the
	// loaded dialect's flow: spec, or derives them live from observed
	// events when it declared none.
	dialectFlow flowState
	parser      *bridge.Parser

	// animFrame advances on animTickMsg for LED / flow-chase glyph cycles only.
	animFrame int
	// energy is driven by KindContent token arrivals (not a free-running wiggle).
	energy energyState
}

type Message struct {
	Text      string
	Timestamp time.Time
	Streaming bool

	// TransportName identifies the wire transport that produced this turn.
	// It lets the raw pane render protocol-specific wire data without making
	// the event model transport-specific.
	TransportName string

	// RawSSE stores complete raw SSE (no truncation) for the legacy SSE view.
	RawSSE string

	// Frames stores lossless captured frames for the latest transport. SSE also
	// retains RawSSE for compatibility; structured transports use Data for the
	// decoded JSON representation and Raw for the original payload (protobuf
	// wire bytes for gRPC).
	Frames []CapturedFrame

	// Store parsed events
	Events []*events.Event

	// Store per-agent responses
	AgentResponses map[string]*AgentResponse
}

// CapturedFrame is the UI's lossless record of one transport frame. The
// transport package remains unaware of presentation concerns; this type only
// exists so the TUI can show decoded data and wire bytes side by side.
type CapturedFrame struct {
	Name      string
	Data      []byte
	Raw       []byte
	Timestamp time.Time
	Err       string
}

// NewInteractiveModel creates a new interactive TUI model
func NewInteractiveModel(cfg *config.EnhancedConfig) InteractiveModel {
	return NewInteractiveModelWithContext(cfg, context.TODO())
}

// NewInteractiveModelWithContext creates an interactive model whose network
// commands share the Bubble Tea program's lifecycle context.
func NewInteractiveModelWithContext(cfg *config.EnhancedConfig, ctx context.Context) InteractiveModel {
	return NewInteractiveModelWithContextAndConfigPath(cfg, ctx, "")
}

// NewInteractiveModelWithContextAndConfigPath creates an interactive model
// that can save panel edits back to the config file used to launch it.
func NewInteractiveModelWithContextAndConfigPath(cfg *config.EnhancedConfig, ctx context.Context, configPath string) InteractiveModel {
	return NewInteractiveModelWithContextAndConfigPathAndOpenConfig(cfg, ctx, configPath, false)
}

// NewInteractiveModelWithContextAndConfigPathAndOpenConfig creates an
// interactive model and optionally opens the configuration panel immediately.
// The startup variant is used when the CLI has created a starter config.
func NewInteractiveModelWithContextAndConfigPathAndOpenConfig(cfg *config.EnhancedConfig, ctx context.Context, configPath string, openConfigPanel bool) InteractiveModel {
	if ctx == nil {
		ctx = context.TODO()
	}
	parser := bridge.NewParser(cfg.Dialect.File)
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
		dialectFlow:      newFlowStateWithParser(parser),
		parser:           parser,
		streamContext:    ctx,
		// Normal interactive startup performs auto-setup before constructing
		// the model. A first-run model defers it until the first send, after
		// the user has filled and saved the configuration panel.
		sessionReady: !cfg.Session.AutoSetup || !openConfigPanel,
		configPath:   configPath,
		energy:       newEnergyState(),
	}
	// Set an initial placeholder so the viewport isn't blank before first refresh
	m.viewport.SetContent(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(
		"Press 1–5 to switch panes • i to type • Ctrl+T toggles RAW/PARSED in Events",
	))
	if openConfigPanel {
		m.configPanel = newConfigPanel(cfg)
		m.configPanel.open = true
		m.configPanel.notice = "First run: enter your backend details, then press Ctrl+S to save."
		m.textarea.Blur()
	}
	return m
}

func (m InteractiveModel) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, animTickCmd())
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
