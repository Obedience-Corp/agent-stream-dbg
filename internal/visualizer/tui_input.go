package visualizer

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// handleKeyPress handles keyboard input.
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
		m.config.Debug.Level = "verbose"
		return m, m.reconnect()
	case "F2":
		m.config.Debug.Level = "full"
		return m, m.reconnect()
	case "F3":
		m.config.Debug.Level = ""
		return m, m.reconnect()
	}
	return m, nil
}

// reconnect restarts the SSE connection with current debug level.
func (m *Model) reconnect() tea.Cmd {
	return func() tea.Msg {
		if m.cancel != nil {
			m.cancel()
		}
		m.ctx, m.cancel = context.WithCancel(m.parent)
		m.agents = make(map[string]*AgentState)
		m.aggregatorState = &AggregatorState{BufferedTokens: make(map[int]string)}
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
		if err := m.client.Connect(m.ctx, m.message); err != nil {
			return errorMsg{err: fmt.Errorf("reconnect failed: %w", err)}
		}
		return nil
	}
}
