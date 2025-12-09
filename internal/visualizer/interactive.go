package visualizer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "sort"
    "unicode/utf8"
    neturl "net/url"
    "strings"
    "time"

    "github.com/charmbracelet/bubbles/textarea"
    "github.com/charmbracelet/bubbles/viewport"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
    "github.com/lancekrogers/stream-debugger/internal/client"
    "github.com/lancekrogers/stream-debugger/internal/config"
    dblogger "github.com/lancekrogers/stream-debugger/internal/logger"
    "github.com/lancekrogers/stream-debugger/internal/events"
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

    // App pane state
    appFocus       AppFocus
    agentCollapsed map[string]bool // per-agent collapsed state

    // YAML pane state
    yamlSnapshot     string
    yamlPrevSnapshot string
    yamlReloadStatus string
    yamlDiff         string
    configClient     *client.ConfigAPIClient
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
	vp.HighPerformanceRendering = false

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

    return InteractiveModel{
        cfg:      cfg,
        apiKey:   apiKey,
        textarea: ta,
        viewport: vp,
        viewMode: ViewModeRaw, // Default to raw view
        messages:    make([]Message, 0),
        follow:      true,
        showHistory: true,
        wrap:        false,
        xOffset:     0,
        insertMode:  false,
        slog:        slog,
        contentDirty: true,
        // Pane defaults
        activePane:        PaneFlow,
        selectedStepIndex: 0,
        flowExpanded:      make(map[string]bool),
        showTokens:        false,
        showPromptRef:     false,
        // App pane defaults
        appFocus:       AppFocusWizard,
        agentCollapsed: make(map[string]bool),
        // YAML pane
        configClient: client.NewConfigAPIClient(cfg, apiKey),
    }
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
        viewportContent = m.renderFlowPane()
    case PaneApp:
        viewportContent = m.renderAppPane()
    case PaneYAML:
        viewportContent = m.renderYAMLPane()
    case PaneTimeline:
        viewportContent = m.renderTimelinePane()
    case PaneEvents:
        viewportContent = m.renderEventsPane()
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
    // Always refresh viewport content on return if content became dirty,
    // even when we return early inside key handlers.
    defer func() {
        if m.contentDirty {
            m.refreshViewportContent()
        }
    }()

	switch msg := msg.(type) {
    case tea.KeyMsg:
        // Global ctrl bindings
        if msg.Type == tea.KeyCtrlC { return m, tea.Quit }
        if msg.Type == tea.KeyCtrlT {
            // Toggle view mode (used by Events pane to switch RAW↔PARSED views)
            if m.viewMode == ViewModeRaw { m.viewMode = ViewModeParsed } else { m.viewMode = ViewModeRaw }
            m.contentDirty = true
            return m, nil
        }
        if msg.Type == tea.KeyCtrlF {
            m.follow = !m.follow
            if m.follow { m.viewport.GotoBottom() }
            return m, nil
        }
        if msg.Type == tea.KeyEsc {
            if m.insertMode { m.insertMode = false; m.textarea.Blur() }
            return m, nil
        }
        if msg.Type == tea.KeyEnter {
            if m.insertMode && !m.streaming {
                message := m.textarea.Value()
                if strings.TrimSpace(message) != "" {
                    m.messages = append(m.messages, Message{ Text: message, Timestamp: time.Now(), Streaming: true })
                    m.textarea.Reset()
                    m.streaming = true
                    m.contentDirty = true
                    return m, m.sendMessage(message, len(m.messages)-1)
                }
            }
            return m, nil
        }

        // Insert toggle
        if msg.String() == "i" && !m.insertMode { m.insertMode = true; m.textarea.Focus(); return m, nil }

        // Pane switching (works in normal mode only)
        if !m.insertMode {
            switch msg.String() {
            case "1", "f1":
                m.activePane = PaneFlow
                m.contentDirty = true
                return m, nil
            case "2", "f2":
                m.activePane = PaneApp
                m.contentDirty = true
                return m, nil
            case "3", "f3":
                m.activePane = PaneYAML
                m.contentDirty = true
                return m, nil
            case "4", "f4":
                m.activePane = PaneTimeline
                m.contentDirty = true
                return m, nil
            case "5", "f5":
                m.activePane = PaneEvents
                m.contentDirty = true
                return m, nil
            }
        }

        // Normal-mode navigation only when not inserting
        if !m.insertMode {
            switch msg.Type {
            case tea.KeyUp:
                m.viewport.LineUp(1); m.follow = false; return m, nil
            case tea.KeyDown:
                m.viewport.LineDown(1); m.follow = false; return m, nil
            case tea.KeyPgUp:
                m.viewport.HalfViewUp(); m.follow = false; return m, nil
            case tea.KeyPgDown:
                m.viewport.HalfViewDown(); m.follow = false; return m, nil
            case tea.KeyHome:
                m.viewport.GotoTop(); m.follow = false; return m, nil
            case tea.KeyEnd:
                m.viewport.GotoBottom(); m.follow = true; return m, nil
            case tea.KeyLeft:
                if m.xOffset > 0 { m.xOffset--; m.contentDirty = true }; return m, nil
            case tea.KeyRight:
                m.xOffset++; m.contentDirty = true; return m, nil
            }
            switch msg.String() {
            case "j":
                // Flow pane: move selection down; others: scroll viewport
                if m.activePane == PaneFlow {
                    if m.selectedStepIndex < 5 { // 6 steps: 0-5
                        m.selectedStepIndex++
                        m.contentDirty = true
                    }
                } else {
                    m.viewport.LineDown(1); m.follow = false
                }
                return m, nil
            case "k":
                // Flow pane: move selection up; others: scroll viewport
                if m.activePane == PaneFlow {
                    if m.selectedStepIndex > 0 {
                        m.selectedStepIndex--
                        m.contentDirty = true
                    }
                } else {
                    m.viewport.LineUp(1); m.follow = false
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
                    }
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
                    }
                }
                return m, nil
            case "p":
                // Flow pane: toggle prompt ref display
                if m.activePane == PaneFlow {
                    m.showPromptRef = !m.showPromptRef
                    m.contentDirty = true
                }
                return m, nil
            case "t":
                // Events pane: toggle token visibility
                if m.activePane == PaneEvents {
                    m.showTokens = !m.showTokens
                    m.contentDirty = true
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
                        m.yamlSnapshot = client.PrettyJSON(data)
                        m.yamlReloadStatus = fmt.Sprintf("Snapshot taken at %s", time.Now().Format("15:04:05"))
                        // Compute diff if we have a previous snapshot
                        if m.yamlPrevSnapshot != "" {
                            m.yamlDiff = lineDiff(m.yamlPrevSnapshot, m.yamlSnapshot)
                        } else {
                            m.yamlDiff = ""
                        }
                    }
                    m.contentDirty = true
                }
                return m, nil
            case "R":
                // YAML pane: reload prompts
                if m.activePane == PaneYAML && m.configClient != nil {
                    data, status, err := m.configClient.ReloadPrompts()
                    if err != nil {
                        m.yamlReloadStatus = fmt.Sprintf("Reload error: %v", err)
                    } else {
                        m.yamlReloadStatus = fmt.Sprintf("HTTP %d\n%s", status, client.PrettyJSON(data))
                    }
                    m.contentDirty = true
                }
                return m, nil
            case "g":
                m.viewport.GotoTop(); m.follow = false; return m, nil
            case "G":
                m.viewport.GotoBottom(); m.follow = true; return m, nil
            case " ":
                m.viewport.HalfViewDown(); m.follow = false; return m, nil
            case "b":
                m.viewport.HalfViewUp(); m.follow = false; return m, nil
            case "H":
                m.showHistory = !m.showHistory; m.contentDirty = true; if !m.showHistory { m.follow = true; m.viewport.GotoBottom() }; return m, nil
            case "w":
                m.wrap = !m.wrap; m.contentDirty = true; if m.wrap { m.xOffset = 0 }; return m, nil
            case "h":
                if m.xOffset > 0 { m.xOffset--; m.contentDirty = true }; return m, nil
            case "l":
                m.xOffset++; m.contentDirty = true; return m, nil
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

	case streamErrorMsg:
		m.err = msg.err
		if len(m.messages) > 0 {
			m.messages[len(m.messages)-1].Streaming = false
		}
		m.streaming = false
		m.contentDirty = true
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
    paneNames := []string{"Flow", "App", "YAML", "Timeline", "Events"}
    var tabs strings.Builder
    for i, name := range paneNames {
        if Pane(i) == m.activePane {
            tabs.WriteString(paneStyle.Render(fmt.Sprintf("[%d]%s", i+1, name)))
        } else {
            tabs.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fmt.Sprintf(" %d:%s ", i+1, name)))
        }
    }

	b.WriteString(headerStyle.Render("🚀 Stream Debugger"))
    b.WriteString(" ")
    b.WriteString(tabs.String())
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
    if !m.insertMode { mode = "NORMAL" }

    // Pane-specific status hints
    var paneHints string
    switch m.activePane {
    case PaneFlow:
        paneHints = "j/k:select Enter:expand p:prompt"
    case PaneEvents:
        tokensStr := "OFF"
        if m.showTokens { tokensStr = "ON" }
        paneHints = fmt.Sprintf("t:tokens(%s)", tokensStr)
    case PaneYAML:
        paneHints = "R:reload y:snapshot"
    default:
        paneHints = ""
    }

    status := fmt.Sprintf(
        "Mode:%s Pane:%s Msgs:%d | 1-5:panes %s Ctrl+T:raw/parsed i:insert Esc:normal Ctrl+C:quit",
        mode, m.currentPaneName(), len(m.messages), paneHints,
    )
	b.WriteString(statusStyle.Render(status))

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

// buildAgentResponses builds agent response map from parsed events
func buildAgentResponses(events []*events.Event) map[string]*AgentResponse {
	responses := make(map[string]*AgentResponse)

	for _, event := range events {
		agentID := event.GetAgentID()
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

    // Render each agent's response (deterministic order)
    // Collect and sort agent IDs for stable rendering to avoid flicker
    keys := make([]string, 0, len(msg.AgentResponses))
    for k := range msg.AgentResponses { keys = append(keys, k) }
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

    // Get latest message events
    if len(m.messages) == 0 {
        b.WriteString(dimStyle.Render("No messages yet. Send a message to see flow steps."))
        return b.String()
    }

    msg := m.messages[len(m.messages)-1]
    if len(msg.Events) == 0 {
        b.WriteString(dimStyle.Render("No flow events in latest message."))
        return b.String()
    }

    nodes := m.buildFlowNodes(msg.Events)
    steps := []string{"routing", "discovery", "agent_exec", "filter", "synthesis", "wizard"}

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
            b.WriteString(m.renderFlowNodeDetails(n))
        }
    }

    return b.String()
}

// renderFlowNodeDetails renders expanded details for a flow node
func (m InteractiveModel) renderFlowNodeDetails(n *FlowNode) string {
    var b strings.Builder
    detailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).PaddingLeft(4)

    if n.RoutingMode != "" {
        b.WriteString(detailStyle.Render(fmt.Sprintf("mode: %s", n.RoutingMode)))
        b.WriteString("\n")
    }
    if n.RouteReason != "" {
        b.WriteString(detailStyle.Render(fmt.Sprintf("reason: %s", n.RouteReason)))
        b.WriteString("\n")
    }
    if m.showPromptRef && n.PromptRef != nil && len(n.PromptRef) > 0 {
        for k, v := range n.PromptRef {
            b.WriteString(detailStyle.Render(fmt.Sprintf("%s: %v", k, v)))
            b.WriteString("\n")
        }
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
        b.WriteString(m.renderWizardSection(msg, focusStyle))
        b.WriteString(m.renderAgentSummaries(msg))
    } else {
        b.WriteString(m.renderAgentSummaries(msg))
        b.WriteString(m.renderWizardSection(msg, lipgloss.NewStyle()))
    }

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
    b.WriteString(wizard.FullContent)
    b.WriteString("\n\n")
    return b.String()
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
    addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // green
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

// renderEventsPane renders the Events pane (F5) showing SSE events
func (m InteractiveModel) renderEventsPane() string {
    var b strings.Builder

    headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
    dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

    viewLabel := "PARSED"
    if m.viewMode == ViewModeRaw {
        viewLabel = "RAW"
    }
    tokensLabel := "OFF"
    if m.showTokens { tokensLabel = "ON" }

    b.WriteString(headerStyle.Render("Events"))
    b.WriteString(fmt.Sprintf(" (View: %s)", viewLabel))
    if m.viewMode == ViewModeParsed {
        b.WriteString(fmt.Sprintf(" (Tokens: %s)", tokensLabel))
    }
    b.WriteString("\n")
    b.WriteString(dimStyle.Render("─────────────────────────────────────────"))
    b.WriteString("\n")
    // Hints
    if m.viewMode == ViewModeParsed {
        b.WriteString(dimStyle.Render("Ctrl+T: RAW view, t: toggle token events"))
    } else {
        b.WriteString(dimStyle.Render("Ctrl+T: PARSED view"))
    }
    b.WriteString("\n\n")

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

    // Filter events based on showTokens flag
    eventStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
    tokenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

    for _, evt := range msg.Events {
        // Skip token events if showTokens is false
        isTokenEvent := evt.Type == events.AgentContent || evt.Type == events.WizardContent
        if isTokenEvent && !m.showTokens {
            continue
        }

        // Format event
        line := fmt.Sprintf("[%s]", evt.Type)

        // Add relevant details
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
        }

        if isTokenEvent {
            b.WriteString(tokenStyle.Render(line))
        } else {
            b.WriteString(eventStyle.Render(line))
        }
        b.WriteString("\n")
    }

    return b.String()
}

// clipLeft removes the first x columns (approx bytes) from each line for basic horizontal scrolling
func (m InteractiveModel) clipLeft(s string, off int) string {
    if off <= 0 { return s }
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
        if i < len(lines)-1 { out.WriteByte('\n') }
    }
    return out.String()
}

// wrapToWidth wraps long lines to the given width (approx runes)
func (m InteractiveModel) wrapToWidth(s string, width int) string {
    if width <= 0 { return s }
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
                if end > len(runes) { end = len(runes) }
                out.WriteString(string(runes[start:end]))
                if end < len(runes) { out.WriteByte('\n') }
            }
        }
        if i < len(lines)-1 { out.WriteByte('\n') }
    }
    return out.String()
}

// clipRunesLeft clips n columns (by rune) from the left; if n exceeds line length, returns empty
func clipRunesLeft(line string, n int) string {
    if n <= 0 { return line }
    // Fast path for ASCII
    if utf8.RuneCountInString(line) == len(line) {
        if n >= len(line) { return "" }
        return line[n:]
    }
    // Unicode-aware path
    i := 0
    for idx := range line {
        if i == n { return line[idx:] }
        i++
    }
    return ""
}

// urlQueryEscape safely escapes a message for URL query use
func urlQueryEscape(s string) string { return neturl.QueryEscape(s) }

func (m InteractiveModel) sendMessage(message string, index int) tea.Cmd {
    return func() tea.Msg {
        // Build request
        url := m.cfg.StreamEndpointURL()

        // Debug: Log the URL being called
        fmt.Fprintf(os.Stderr, "DEBUG: Calling URL: %s\n", url)
        fmt.Fprintf(os.Stderr, "DEBUG: BaseURL=%q Endpoint=%q SessionID=%q\n",
            m.cfg.Backend.BaseURL, m.cfg.Backend.StreamEndpoint, m.cfg.Session.ID)

		requestBody := map[string]interface{}{
			"message": message,
			"stream":  true,
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

        // Prepare logging file for raw SSE
        logDir := m.cfg.LogDir
        bySessionDir := filepath.Join(logDir, "by-session")
        _ = os.MkdirAll(bySessionDir, 0o755)
        ts := time.Now().Format("20060102_150405")
        ssePath := filepath.Join(bySessionDir, fmt.Sprintf("interactive_%s_%s.sse", m.cfg.Session.ID, ts))
        sseFile, _ := os.OpenFile(ssePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
        defer func(){ if sseFile != nil { _ = sseFile.Close() } }()

        // Send request
        client := &http.Client{
            Timeout: 60 * time.Second,
        }

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		req = req.WithContext(ctx)

        resp, err := client.Do(req)
        if err != nil {
            return streamErrorMsg{err: fmt.Errorf("failed to send request: %w", err)}
        }
        defer resp.Body.Close()

        if resp.StatusCode == http.StatusMethodNotAllowed {
            // Fallback to GET with query parameter if POST not allowed
            resp.Body.Close()
            getURL := fmt.Sprintf("%s?message=%s", url, urlQueryEscape(message))
            req, err = http.NewRequest("GET", getURL, nil)
            if err != nil { return streamErrorMsg{err: fmt.Errorf("failed to create GET request: %w", err)} }
            req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", m.apiKey))
            req.Header.Set("Accept", "text/event-stream")
            resp, err = client.Do(req)
            if err != nil { return streamErrorMsg{err: fmt.Errorf("failed to send GET request: %w", err)} }
            defer resp.Body.Close()
        }
        if resp.StatusCode != http.StatusOK {
            body, _ := io.ReadAll(resp.Body)
            return streamErrorMsg{err: fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))}
        }

        // Read streaming response (complete raw SSE) and append to log as it arrives
        var rawSSE strings.Builder
        buf := make([]byte, 4096)

        for {
            n, err := resp.Body.Read(buf)
            if n > 0 {
                rawSSE.Write(buf[:n])
                if sseFile != nil {
                    _, _ = sseFile.Write(buf[:n])
                }
            }
            if err != nil {
                if err != io.EOF {
                    return streamErrorMsg{err: err}
                }
                break
            }
        }

        // Parse SSE stream into events
        rawString := rawSSE.String()
        parsedEvents, err := parseSSEStream(rawString)
        if err != nil {
            // If parsing fails, still show raw SSE
            parsedEvents = []*events.Event{}
        }

        // Build agent responses from events
        agentResponses := buildAgentResponses(parsedEvents)

        // Log parsed events to structured logger if available
        if m.slog != nil {
            for _, ev := range parsedEvents {
                _ = m.slog.LogEvent(ev)
            }
        }

		return streamCompleteMsg{
			index:          index,
			rawSSE:         rawString,
			events:         parsedEvents,
			agentResponses: agentResponses,
		}
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
        
