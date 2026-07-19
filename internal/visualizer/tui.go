package visualizer

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/lancekrogers/stream-debugger/internal/bridge"
	"github.com/lancekrogers/stream-debugger/internal/client"
	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/logger"
)

// Model represents the TUI application state.
type Model struct {
	config  *config.EnhancedConfig
	client  *client.SSEClient
	logger  *logger.StructuredLogger
	parent  context.Context
	ctx     context.Context
	cancel  context.CancelFunc
	message string

	// State
	agents          map[string]*AgentState
	aggregatorState *AggregatorState
	sessionActive   bool
	errorCount      int

	// UI State
	width  int
	height int
	paused bool

	// Stats
	startTime   time.Time
	totalTokens int
	totalEvents int
	// animFrame advances on tickMsg for energy strip / agent pulses.
	animFrame int

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
	parser      *bridge.Parser
}

// AgentState tracks the state of a single agent.
type AgentState struct {
	ID             string
	Role           events.Role
	Active         bool
	Content        strings.Builder
	TokenCount     int
	Sequence       int
	BufferedTokens map[int]string // sequence -> token
	StartTime      time.Time
	EndTime        time.Time
	LastUpdate     time.Time
}

// AggregatorState tracks the aggregator-role lane's synthesis state.
type AggregatorState struct {
	Role           events.Role
	Active         bool
	Content        strings.Builder
	TokenCount     int
	Sequence       int
	BufferedTokens map[int]string
	StartTime      time.Time
	EndTime        time.Time
}

// FlowStepStatus tracks the state of a flow step.
type FlowStepStatus struct {
	Step        string
	Role        events.Role
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

// PromptInfoState tracks prompt info for an agent.
type PromptInfoState struct {
	AgentID       string
	PromptFile    string
	PromptSnippet string
	PromptLength  int
	SystemPrompt  string // Only populated when debug=full
}

// FlowConfigState tracks flow configuration in debug mode.
type FlowConfigState struct {
	FlowID             string
	FlowFile           string
	StagesOrder        []string
	StagesEnabled      map[string]bool
	RoutingMode        string
	AgentCount         int
	NonAggregatorCount int
}

// eventMsg wraps an SSE event for bubbletea.
type eventMsg struct {
	event *events.Event
}

// errorMsg wraps an error for bubbletea.
type errorMsg struct {
	err error
}

// tickMsg is sent periodically for UI updates.
type tickMsg time.Time

// NewModel creates a new TUI model.
func NewModel(cfg *config.EnhancedConfig, sseClient *client.SSEClient, structuredLogger *logger.StructuredLogger, message string) *Model {
	return NewModelWithContext(cfg, sseClient, structuredLogger, message, context.TODO())
}

// NewModelWithContext creates the legacy event-pane model with a parent
// lifecycle context supplied by its Bubble Tea program.
func NewModelWithContext(cfg *config.EnhancedConfig, sseClient *client.SSEClient, structuredLogger *logger.StructuredLogger, message string, parent context.Context) *Model {
	if parent == nil {
		parent = context.TODO()
	}
	ctx, cancel := context.WithCancel(parent)
	parser := bridge.NewParser(cfg.Dialect.File)

	return &Model{
		config:          cfg,
		client:          sseClient,
		logger:          structuredLogger,
		parent:          parent,
		ctx:             ctx,
		cancel:          cancel,
		message:         message,
		agents:          make(map[string]*AgentState),
		aggregatorState: &AggregatorState{BufferedTokens: make(map[int]string)},
		sessionActive:   false,
		startTime:       time.Now(),
		flow:            make(map[string]*FlowStepStatus),
		promptInfo:      make(map[string]*PromptInfoState),
		dialectFlow:     newFlowStateWithParser(parser),
		parser:          parser,
	}
}

// Init initializes the bubbletea application.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.waitForEvent(), m.waitForError(), m.tickCmd())
}

// Update handles messages and updates the model.
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
		m.animFrame++
		return m, m.tickCmd()
	}
	return m, nil
}
