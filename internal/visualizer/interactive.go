package visualizer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lancekrogers/stream-debugger/internal/config"
)

// InteractiveModel represents the interactive TUI state
type InteractiveModel struct {
	cfg      *config.EnhancedConfig
	apiKey   string
	textarea textarea.Model

	// Message history
	messages  []Message
	streaming bool

	// UI state
	width  int
	height int
	err    error
}

type Message struct {
	Text      string
	Timestamp time.Time
	Response  string
	Streaming bool
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

	return InteractiveModel{
		cfg:      cfg,
		apiKey:   apiKey,
		textarea: ta,
		messages: make([]Message, 0),
	}
}

func (m InteractiveModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m InteractiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			if !m.streaming {
				// Send the message
				message := m.textarea.Value()
				if strings.TrimSpace(message) != "" {
					m.messages = append(m.messages, Message{
						Text:      message,
						Timestamp: time.Now(),
						Streaming: true,
					})
					m.textarea.Reset()
					m.streaming = true

					// Start streaming
					return m, m.sendMessage(message, len(m.messages)-1)
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msg.Width - 4)

	case streamCompleteMsg:
		if msg.index < len(m.messages) {
			m.messages[msg.index].Response = msg.response
			m.messages[msg.index].Streaming = false
		}
		m.streaming = false

	case streamErrorMsg:
		m.err = msg.err
		if len(m.messages) > 0 {
			m.messages[len(m.messages)-1].Streaming = false
		}
		m.streaming = false
	}

	// Update textarea
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

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

	// Message history
	if len(m.messages) > 0 {
		b.WriteString(m.renderMessages())
		b.WriteString("\n")
	} else {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Render("💬 Type a message below to start chatting..."))
		b.WriteString("\n\n")
	}

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

	// Status line
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Italic(true)

	status := fmt.Sprintf("Messages: %d | Session: %s | Press Ctrl+C to quit",
		len(m.messages), m.cfg.Session.ID)
	b.WriteString(statusStyle.Render(status))

	return b.String()
}

func (m InteractiveModel) renderMessages() string {
	var b strings.Builder

	// Show last 3 messages
	start := 0
	if len(m.messages) > 3 {
		start = len(m.messages) - 3
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
		} else if msg.Response != "" {
			responseStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("13")).
				Bold(true)

			b.WriteString(responseStyle.Render("Agents:"))
			b.WriteString("\n")

			// Truncate response if too long
			response := msg.Response
			if len(response) > 500 {
				response = response[:500] + "..."
			}
			b.WriteString(response)
			b.WriteString("\n\n")
		}
	}

	return b.String()
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

		// Read streaming response
		var responseText strings.Builder
		buf := make([]byte, 4096)

		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				responseText.Write(buf[:n])
			}
			if err != nil {
				if err != io.EOF {
					return streamErrorMsg{err: err}
				}
				break
			}
		}

		return streamCompleteMsg{
			index:    index,
			response: responseText.String(),
		}
	}
}

// Message types
type streamCompleteMsg struct {
	index    int
	response string
}

type streamErrorMsg struct {
	err error
}
