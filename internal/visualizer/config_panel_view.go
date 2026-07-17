package visualizer

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m InteractiveModel) renderConfigPanel() string {
	width := m.width - 6
	if width < 64 {
		width = 64
	}
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Width(20)
	focusedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true).Width(20)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(width)

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")).Render("Configuration"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Edit runtime settings. Enter applies and reconnects the latest message."))
	b.WriteString("\n\n")
	if m.configPanel.notice != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true).Render(m.configPanel.notice))
		b.WriteString("\n\n")
	}
	transportType := strings.ToLower(strings.TrimSpace(m.configPanel.fields[configFieldTransport].input.Value()))
	b.WriteString(dimStyle.Render(configTransportHint(transportType)))
	b.WriteString("\n\n")
	for i, field := range m.configPanel.fields {
		if !field.active {
			continue
		}
		label := labelStyle.Render(field.label)
		if i == m.configPanel.focused {
			label = focusedStyle.Render("▸ " + field.label)
		}
		b.WriteString(label)
		b.WriteString(field.input.View())
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Tab/Shift+Tab: move  Enter: Apply & Reconnect  Ctrl+S: Save  Esc: Cancel"))
	b.WriteString("\n")
	if m.configPath == "" {
		b.WriteString(dimStyle.Render("Save: unavailable (interactive mode was started without a config path)"))
	} else {
		b.WriteString(dimStyle.Render(fmt.Sprintf("Save path: %s", m.configPath)))
	}
	if m.configPanel.status != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Render(m.configPanel.status))
	}
	return panelStyle.Render(b.String())
}

func configTransportHint(transportType string) string {
	switch transportType {
	case "grpc":
		return "gRPC selected: fill the gRPC target. SSE fields are hidden and preserved."
	case "replay":
		return "Replay selected: fill the replay file path. Network fields are hidden."
	case "sse":
		return "SSE selected: fill Base URL + endpoint. Auth env is optional (for example API_KEY). gRPC target is not used."
	default:
		return "Choose sse, grpc, or replay; only the selected transport's fields are used."
	}
}
