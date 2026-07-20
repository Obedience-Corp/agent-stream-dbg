package home

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/Obedience-Corp/stream-debugger/internal/config"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	labelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Width(14)
	chipReady   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	chipNeed    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	chipErr     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	panelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(1, 2)
	noticeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)

func (m model) View() string {
	if m.quitting {
		return ""
	}
	width := m.width - 6
	if width < 64 {
		width = 64
	}
	panel := panelStyle.Width(width)

	var body string
	switch m.screen {
	case screenHome:
		body = m.viewHome()
	case screenWizardTemplate:
		body = m.viewWizardTemplate()
	case screenWizardConnect:
		body = m.viewWizardConnect()
	case screenWizardDialect:
		body = m.viewWizardDialect()
	case screenWizardSave:
		body = m.viewWizardSave()
	case screenOffline:
		body = m.viewOffline()
	case screenConfirmDelete:
		body = m.viewConfirmDelete()
	default:
		body = "unknown screen"
	}
	return panel.Render(body)
}

func (m model) viewHome() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Stream Debugger"))
	b.WriteString("\n")
	if len(m.entries) == 0 {
		b.WriteString(dimStyle.Render("Welcome"))
	} else {
		b.WriteString(dimStyle.Render("Configurations"))
	}
	b.WriteString("\n\n")
	if m.notice != "" {
		b.WriteString(noticeStyle.Render(m.notice))
		b.WriteString("\n\n")
	}
	if len(m.entries) == 0 {
		b.WriteString(dimStyle.Render("No configurations yet. Create one to connect to an"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("SSE, gRPC, or ACP stream — or try an offline demo."))
		b.WriteString("\n\n")
	}

	items := m.homeItems()
	for i, item := range items {
		cursor := "  "
		style := dimStyle
		if i == m.cursor {
			cursor = "▸ "
			style = cursorStyle
		}
		if item == "cfg" {
			entry := m.entries[i]
			chip := chipReady.Render(string(entry.Status))
			switch entry.Status {
			case config.EntryNeedsSetup, config.EntryAuthNeeded:
				chip = chipNeed.Render(string(entry.Status))
			case config.EntryError:
				chip = chipErr.Render(string(entry.Status))
			}
			dialect := entry.Dialect
			if len(dialect) > 14 {
				dialect = dialect[:11] + "…"
			}
			line := fmt.Sprintf("%s%-18s  %-6s  %-12s  %s", cursor, entry.Name, entry.Transport, dialect, chip)
			b.WriteString(style.Render(line))
			b.WriteString("\n")
			if i == m.cursor {
				b.WriteString(dimStyle.Render("    " + config.DisplayPath(entry.Path, m.opts.Cwd)))
				if entry.Status == config.EntryAuthNeeded && entry.Err != "" {
					b.WriteString("\n")
					b.WriteString(dimStyle.Render("    " + entry.Err + " — export the env var or press e to edit"))
				}
				b.WriteString("\n")
			}
			continue
		}
		if i == len(m.entries) && len(m.entries) > 0 {
			b.WriteString(dimStyle.Render("────────────────────────────────────────"))
			b.WriteString("\n")
		}
		label := item
		switch item {
		case "action:new":
			label = "+ New configuration"
		case "action:offline":
			label = "◎ Offline demos"
		}
		b.WriteString(style.Render(cursor + label))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("↑↓ navigate  enter open  e edit  n new  d delete  q quit"))
	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(statusStyle.Render(m.status))
	}
	return b.String()
}

func (m model) viewWizardTemplate() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("New configuration (1/4)"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Choose a starting template"))
	b.WriteString("\n\n")
	templates := []string{
		"SSE HTTP streaming",
		"gRPC (reflection-friendly)",
		"ACP stdio agent",
		"Replay recorded JSONL",
	}
	for i, t := range templates {
		cursor := "  "
		style := dimStyle
		if i == m.wizardTemplate {
			cursor = "▸ "
			style = cursorStyle
		}
		b.WriteString(style.Render(cursor + t))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("enter next  esc cancel"))
	return b.String()
}

func (m model) viewWizardConnect() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("New configuration (2/4)"))
	b.WriteString("\n")
	kind := templateKind(m.wizardTemplate)
	b.WriteString(dimStyle.Render("Connection · transport: " + string(kind)))
	b.WriteString("\n\n")
	labels := connectLabels(kind)
	for i, f := range m.wizardFields {
		lab := labels[i]
		label := labelStyle.Render(lab)
		if i == m.wizardFocus {
			label = cursorStyle.Width(14).Render("▸ " + lab)
		}
		b.WriteString(label)
		b.WriteString(f.View())
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("tab fields  enter next  esc back"))
	return b.String()
}

func (m model) viewWizardDialect() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("New configuration (3/4)"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Dialect — what the bytes mean"))
	b.WriteString("\n\n")
	for i, name := range m.dialectNames {
		cursor := "  "
		style := dimStyle
		if i == m.wizardDialect {
			cursor = "▸ "
			style = cursorStyle
		}
		b.WriteString(style.Render(cursor + name))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("enter next  esc back"))
	return b.String()
}

func (m model) viewWizardSave() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("New configuration (4/4)"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Name and save location"))
	b.WriteString("\n\n")
	b.WriteString(labelStyle.Render("Name"))
	b.WriteString(m.nameInput.View())
	b.WriteString("\n")
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		name = "my-backend"
	}
	dir := m.opts.UserConfigDir
	if dir == "" {
		dir = m.opts.Cwd
	}
	path := filepath.Join(dir, config.SanitizeConfigName(name)+".yaml")
	b.WriteString(dimStyle.Render("File  " + config.DisplayPath(path, m.opts.Cwd)))
	b.WriteString("\n\n")
	choices := []string{"Save and open session", "Save and return to home"}
	for i, c := range choices {
		cursor := "  "
		style := dimStyle
		if !m.nameInput.Focused() && i == m.saveChoice {
			cursor = "▸ "
			style = cursorStyle
		}
		b.WriteString(style.Render(cursor + c))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("tab focus actions  enter confirm  esc back"))
	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(statusStyle.Render(m.status))
	}
	return b.String()
}

func (m model) viewOffline() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Offline demos"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("No network required — bundled fixtures"))
	b.WriteString("\n\n")
	for i, d := range m.offlineDemos {
		cursor := "  "
		style := dimStyle
		if i == m.cursor {
			cursor = "▸ "
			style = cursorStyle
		}
		b.WriteString(style.Render(cursor + d.Title))
		b.WriteString("\n")
		if i == m.cursor {
			b.WriteString(dimStyle.Render("    dialect: " + d.Dialect))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("enter run  esc back"))
	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(statusStyle.Render(m.status))
	}
	return b.String()
}

func (m model) viewConfirmDelete() string {
	name := m.entries[m.deleteIndex].Name
	var b strings.Builder
	b.WriteString(titleStyle.Render("Delete configuration"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Delete %q?\n", name))
	b.WriteString(dimStyle.Render(shortPath(m.entries[m.deleteIndex].Path)))
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("y / enter confirm  n / esc cancel"))
	return b.String()
}
