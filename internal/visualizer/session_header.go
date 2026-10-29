package visualizer

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Shared by prompt-driven sessions and passive diagnostic sessions.
func sessionHeader(title, session string, pane Pane) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")).Padding(0, 1)
	selected := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14")).Background(lipgloss.Color("8")).Padding(0, 1)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	var tabs strings.Builder
	for i, name := range []string{"Flow", "App", "Timeline", "Events", "Messages"} {
		if Pane(i) == pane {
			tabs.WriteString(selected.Render(fmt.Sprintf("[%d]%s", i+1, name)))
		} else {
			tabs.WriteString(dim.Render(fmt.Sprintf(" %d:%s ", i+1, name)))
		}
	}
	return titleStyle.Render(title) + " " + tabs.String() + "  " + dim.Render("(session: "+session+")")
}
