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
    "strings"
    "time"

    "github.com/charmbracelet/bubbles/textarea"
    "github.com/charmbracelet/bubbles/viewport"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
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
    }
}

func (m InteractiveModel) Init() tea.Cmd {
	return textarea.Blink
}

// refreshViewportContent updates the viewport content based on current state
func (m *InteractiveModel) refreshViewportContent() {
    var viewportContent string
    if len(m.messages) > 0 {
        raw := m.renderMessages()
        if m.wrap {
            viewportContent = m.wrapToWidth(raw, m.viewport.Width)
        } else if m.xOffset > 0 {
            viewportContent = m.clipLeft(raw, m.xOffset)
        } else {
            viewportContent = raw
        }
    } else {
        viewportContent = lipgloss.NewStyle().
            Foreground(lipgloss.Color("8")).
            Render("💬 Type a message below to start chatting...")
    }
    m.viewport.SetContent(viewportContent)
    m.contentDirty = false
}

func (m InteractiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
    case tea.KeyMsg:
        // Global ctrl bindings
        if msg.Type == tea.KeyCtrlC { return m, tea.Quit }
        if msg.Type == tea.KeyCtrlT {
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
                m.viewport.LineDown(1); m.follow = false; return m, nil
            case "k":
                m.viewport.LineUp(1); m.follow = false; return m, nil
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

    // Refresh viewport content if dirty
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

func (m InteractiveModel) View() string {
	var b strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("6")).
		Padding(0, 1)

	b.WriteString(headerStyle.Render("🚀 Stream Debugger - Interactive Mode"))
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

	// Status line with view mode indicator
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Italic(true)

	viewModeStr := "RAW"
	if m.viewMode == ViewModeParsed {
		viewModeStr = "PARSED"
	}

    mode := "INSERT"
    if !m.insertMode { mode = "NORMAL" }
    status := fmt.Sprintf(
        "Mode:%s Msgs:%d Sess:%s View:%s Follow:%v Hist:%v Wrap:%v X:%d | Ctrl+T view, Ctrl+F follow, H hist, w wrap, ←/→/h/l horiz, ↑/↓/PgUp/PgDn/j/k scroll, G bottom, g top, i insert, Esc normal, Ctrl+C quit",
        mode, len(m.messages), m.cfg.Session.ID, viewModeStr, m.follow, m.showHistory, m.wrap, m.xOffset,
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

func (m InteractiveModel) sendMessage(message string, index int) tea.Cmd {
    return func() tea.Msg {
        // Build request
        url := m.cfg.StreamEndpointURL()

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
        
