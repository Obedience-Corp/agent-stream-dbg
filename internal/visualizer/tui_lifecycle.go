package visualizer

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// waitForEvent waits for the next SSE event.
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

// waitForError waits for errors from the SSE client.
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

// tickCmd sends periodic tick messages for UI updates.
func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// agentColorOverrides returns the config-declared agent_colors map.
func (m *Model) agentColorOverrides() map[string]string {
	if m.config == nil || m.config.Display == nil {
		return nil
	}
	return m.config.Display.AgentColors
}
