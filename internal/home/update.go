package home

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/Obedience-Corp/stream-debugger/internal/config"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		if m.quitting {
			return m, tea.Quit
		}
		switch m.screen {
		case screenHome:
			return m.updateHome(msg)
		case screenWizardTemplate:
			return m.updateWizardTemplate(msg)
		case screenWizardConnect:
			return m.updateWizardConnect(msg)
		case screenWizardDialect:
			return m.updateWizardDialect(msg)
		case screenWizardSave:
			return m.updateWizardSave(msg)
		case screenOffline:
			return m.updateOffline(msg)
		case screenConfirmDelete:
			return m.updateConfirmDelete(msg)
		}
	}
	return m, nil
}

func (m model) updateHome(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.homeItems()
	switch msg.String() {
	case "ctrl+c", "q":
		return m.exitQuit()
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(items)-1 {
			m.cursor++
		}
	case "n":
		return m.startWizard()
	case "e":
		if m.cursor < len(m.entries) {
			return m.exitOpen(m.entries[m.cursor].Path, true, "")
		}
	case "d":
		if m.cursor < len(m.entries) {
			m.deleteIndex = m.cursor
			m.screen = screenConfirmDelete
		}
	case "o", "enter":
		if len(items) == 0 {
			return m, nil
		}
		item := items[m.cursor]
		switch item {
		case "action:new":
			return m.startWizard()
		case "action:offline":
			m.screen = screenOffline
			m.cursor = 0
			return m, nil
		default:
			if m.cursor >= len(m.entries) {
				return m, nil
			}
			entry := m.entries[m.cursor]
			if entry.Status == config.EntryError {
				m.status = entry.Err
				return m, nil
			}
			openPanel := entry.Status == config.EntryNeedsSetup
			return m.exitOpen(entry.Path, openPanel, "")
		}
	}
	return m, nil
}

func (m model) startWizard() (tea.Model, tea.Cmd) {
	m.screen = screenWizardTemplate
	m.wizardTemplate = 0
	m.wizardDialect = 0
	m.wizardFocus = 0
	m.saveChoice = 0
	m.status = ""
	m.notice = ""
	return m, nil
}

func (m model) updateWizardTemplate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenHome
		m.reload()
		return m, nil
	case "up", "k":
		if m.wizardTemplate > 0 {
			m.wizardTemplate--
		}
	case "down", "j":
		if m.wizardTemplate < 3 {
			m.wizardTemplate++
		}
	case "enter":
		kind := templateKind(m.wizardTemplate)
		m.wizardFields = newConnectFields(kind)
		m.wizardFocus = 0
		m.wizardFields[0].Focus()
		m.screen = screenWizardConnect
		return m, textinput.Blink
	}
	return m, nil
}

func (m model) updateWizardConnect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenWizardTemplate
		return m, nil
	case "tab", "down":
		m.wizardFields[m.wizardFocus].Blur()
		m.wizardFocus = (m.wizardFocus + 1) % len(m.wizardFields)
		m.wizardFields[m.wizardFocus].Focus()
		return m, textinput.Blink
	case "shift+tab", "up":
		m.wizardFields[m.wizardFocus].Blur()
		m.wizardFocus = (m.wizardFocus - 1 + len(m.wizardFields)) % len(m.wizardFields)
		m.wizardFields[m.wizardFocus].Focus()
		return m, textinput.Blink
	case "enter":
		m.screen = screenWizardDialect
		// Prefer acp/brainyard defaults by name
		m.wizardDialect = 0
		for i, name := range m.dialectNames {
			if name == "brainyard" {
				m.wizardDialect = i
				break
			}
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.wizardFields[m.wizardFocus], cmd = m.wizardFields[m.wizardFocus].Update(msg)
	return m, cmd
}

func (m model) updateWizardDialect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	max := len(m.dialectNames) - 1
	if max < 0 {
		max = 0
	}
	switch msg.String() {
	case "esc":
		m.screen = screenWizardConnect
		m.wizardFields[m.wizardFocus].Focus()
		return m, textinput.Blink
	case "up", "k":
		if m.wizardDialect > 0 {
			m.wizardDialect--
		}
	case "down", "j":
		if m.wizardDialect < max {
			m.wizardDialect++
		}
	case "enter":
		m.screen = screenWizardSave
		m.nameInput.SetValue("")
		m.nameInput.Focus()
		m.saveChoice = 0
		return m, textinput.Blink
	}
	return m, nil
}

func (m model) updateWizardSave(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenWizardDialect
		return m, nil
	case "tab", "down", "j":
		if m.nameInput.Focused() {
			m.nameInput.Blur()
			m.saveChoice = 0
			return m, nil
		}
		m.saveChoice = (m.saveChoice + 1) % 2
		return m, nil
	case "up", "k", "shift+tab":
		if !m.nameInput.Focused() && m.saveChoice == 0 {
			m.nameInput.Focus()
			return m, textinput.Blink
		}
		if m.saveChoice > 0 {
			m.saveChoice--
		}
		return m, nil
	case "enter":
		return m.finishWizard()
	}
	if m.nameInput.Focused() {
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) finishWizard() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		name = "my-backend"
	}
	base := config.SanitizeConfigName(name)
	dir := m.opts.UserConfigDir
	if strings.TrimSpace(dir) == "" {
		dir = m.opts.Cwd
	}
	path := filepath.Join(dir, base+".yaml")

	kind := templateKind(m.wizardTemplate)
	dialect := "brainyard"
	if m.wizardDialect >= 0 && m.wizardDialect < len(m.dialectNames) {
		dialect = m.dialectNames[m.wizardDialect]
	}

	var yaml string
	switch kind {
	case config.TemplateGRPC:
		yaml = config.NewConfigYAML(kind, dialect, "", "", fieldVal(m.wizardFields, 2), fieldVal(m.wizardFields, 0), fieldVal(m.wizardFields, 1), "", "")
	case config.TemplateACP:
		yaml = config.NewConfigYAML(kind, dialect, "", "", "", "", "", fieldVal(m.wizardFields, 0), "")
		if args := strings.TrimSpace(fieldVal(m.wizardFields, 1)); args != "" {
			parts := strings.Fields(args)
			yaml = strings.Replace(yaml, "  args: []\n", "  args: ["+quoteYAMLList(parts)+"]\n", 1)
		}
	case config.TemplateReplay:
		yaml = config.NewConfigYAML(kind, dialect, "", "", "", "", "", "", fieldVal(m.wizardFields, 0))
	default:
		yaml = config.NewConfigYAML(kind, dialect, fieldVal(m.wizardFields, 0), fieldVal(m.wizardFields, 1), fieldVal(m.wizardFields, 2), "", "", "", "")
	}

	if err := config.WriteNewConfig(path, yaml); err != nil {
		m.status = err.Error()
		return m, nil
	}

	// Determine if ready
	cfg, err := config.LoadConfigFile(path)
	openPanel := err != nil || !config.ConfigReady(cfg)

	if m.saveChoice == 0 {
		return m.exitOpen(path, openPanel, "Created "+base+".yaml")
	}
	m.reload()
	for i, e := range m.entries {
		if e.Path == path {
			m.cursor = i
			break
		}
	}
	m.screen = screenHome
	m.notice = "Saved " + base + ".yaml"
	m.status = ""
	return m, nil
}

func fieldVal(fields []textinput.Model, i int) string {
	if i < 0 || i >= len(fields) {
		return ""
	}
	return strings.TrimSpace(fields[i].Value())
}

func quoteYAMLList(parts []string) string {
	quoted := make([]string, len(parts))
	for i, p := range parts {
		quoted[i] = fmt.Sprintf("%q", p)
	}
	return strings.Join(quoted, ", ")
}

func (m model) updateOffline(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	max := len(m.offlineDemos) - 1
	if max < 0 {
		max = 0
	}
	switch msg.String() {
	case "esc", "q":
		m.screen = screenHome
		m.cursor = len(m.entries) + 1 // offline action
		if m.cursor >= len(m.homeItems()) {
			m.cursor = 0
		}
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < max {
			m.cursor++
		}
	case "enter":
		if m.cursor < 0 || m.cursor >= len(m.offlineDemos) {
			return m, nil
		}
		demo := m.offlineDemos[m.cursor]
		dir := filepath.Join(m.opts.UserConfigDir, "demos")
		if strings.TrimSpace(m.opts.UserConfigDir) == "" {
			dir = filepath.Join(os.TempDir(), "stream-debugger-demos")
		}
		path, err := MaterializeDemo(dir, demo)
		if err != nil {
			m.status = err.Error()
			return m, nil
		}
		return m.exitOpen(path, false, "Offline demo: "+demo.Title)
	}
	return m, nil
}

func (m model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "n":
		m.screen = screenHome
	case "y", "enter":
		if m.deleteIndex >= 0 && m.deleteIndex < len(m.entries) {
			path := m.entries[m.deleteIndex].Path
			if err := os.Remove(path); err != nil {
				m.status = "delete failed: " + err.Error()
			} else {
				m.notice = "Deleted " + filepath.Base(path)
				m.status = ""
			}
			m.reload()
		}
		m.screen = screenHome
	}
	return m, nil
}
