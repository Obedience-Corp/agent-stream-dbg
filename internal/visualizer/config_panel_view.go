package visualizer

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/Obedience-Corp/stream-debugger/internal/client"
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
	if m.configPanel.grpcMethodPickerOpen {
		b.WriteString(renderGRPCMethodPicker(m.configPanel.grpcMethods, m.configPanel.grpcMethodPickerIndex, dimStyle))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("↑/↓ or j/k: move  Enter: select  Esc: back"))
		return panelStyle.Render(b.String())
	}
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
	footer := "Tab/Shift+Tab: move  Enter: Apply & Reconnect  Ctrl+S: Save  Esc: Cancel"
	if transportType == "grpc" {
		footer = "Tab/Shift+Tab: move  Ctrl+G: Discover RPCs  Enter: Apply & Reconnect  Ctrl+S: Save  Esc: Cancel"
	}
	b.WriteString(dimStyle.Render(footer))
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
		return "gRPC selected: enter a target, security, and optional metadata auth, then press Ctrl+G to discover streaming RPCs. Reflection fills the method, request JSON, and a sensible discriminator; manual fields remain available for reflection-disabled servers."
	case "replay":
		return "Replay selected: fill the replay file path. Network fields are hidden."
	case "sse":
		return "SSE selected: fill Base URL + endpoint. Auth env is optional (for example API_KEY). gRPC target is not used."
	default:
		return "Choose sse, grpc, replay, or acp; only the selected transport's fields are used."
	}
}

func renderGRPCMethodPicker(methods []client.GRPCStreamingMethod, selected int, dimStyle lipgloss.Style) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).Render("Streaming RPCs"))
	b.WriteString("\n")
	if len(methods) == 0 {
		return b.String() + dimStyle.Render("No streaming RPCs found")
	}

	start := selected - 6
	if start < 0 {
		start = 0
	}
	end := start + 13
	if end > len(methods) {
		end = len(methods)
		start = end - 13
		if start < 0 {
			start = 0
		}
	}
	for i := start; i < end; i++ {
		method := methods[i]
		style := dimStyle
		marker := "  "
		if i == selected {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
			marker = "▸ "
		}
		availability := ""
		if !method.Supported {
			availability = " · unavailable"
		}
		b.WriteString(style.Render(marker + method.Path + " · " + method.Shape + availability))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("    " + method.InputType + " → " + method.OutputType))
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}
