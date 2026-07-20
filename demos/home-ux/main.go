// Command home-ux is a design prototype for the agent-stream-dbg launch hub.
// It is not the product binary. See workflow/design/agent-stream-dbg-home-tui/.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Screens
const (
	screenHome = iota
	screenWizardTemplate
	screenWizardConnect
	screenWizardDialect
	screenWizardSave
	screenEditor
	screenSessionMock
	screenOffline
	screenConfirmDelete
)

type configRow struct {
	Name      string
	Path      string
	Transport string
	Dialect   string
	Status    string // ready | needs setup | error
	Source    string // user | cwd
}

type model struct {
	width, height int
	screen        int
	cursor        int
	configs       []configRow
	// footer actions on home when empty vs list: rows are configs + actions
	notice string
	status string

	// wizard
	wizardTemplate int
	wizardDialect  int
	wizardFields   []textinput.Model
	wizardFocus    int
	nameInput      textinput.Model
	saveChoice     int // 0 open, 1 home

	// editor
	editIndex  int
	editFields []textinput.Model
	editFocus  int
	editDirty  bool

	// delete
	deleteIndex int

	// mock session
	sessionName string

	// seed mode from env for VHS
	configDir string
	quitting  bool
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Width(14)
	chipReady    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	chipNeed     = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	chipErr      = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	panelStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(1, 2)
	noticeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	sessionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
)

func main() {
	configDir := os.Getenv("SD_HOME_UX_CONFIG_DIR")
	if configDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "user config dir: %v\n", err)
			os.Exit(1)
		}
		configDir = filepath.Join(base, "agent-stream-dbg-home-ux")
	}
	_ = os.MkdirAll(configDir, 0o700)

	m := newModel(configDir)
	if seed := os.Getenv("SD_HOME_UX_SEED"); seed == "multi" {
		m.seedMulti()
	}

	// No alt-screen: VHS/gif capture sees the UI. Product can use alt-screen.
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func newModel(configDir string) model {
	m := model{
		screen:    screenHome,
		configDir: configDir,
		nameInput: textinput.New(),
	}
	m.nameInput.Placeholder = "my-backend"
	m.nameInput.CharLimit = 64
	m.nameInput.Width = 40
	m.wizardFields = newConnectFields("sse")
	m.loadFromDisk()
	if len(m.configs) == 0 {
		m.notice = "Welcome — create a configuration or try an offline demo."
	}
	return m
}

func (m *model) seedMulti() {
	m.configs = []configRow{
		{Name: "brainyard-local", Path: filepath.Join(m.configDir, "brainyard-local.yaml"), Transport: "sse", Dialect: "brainyard", Status: "ready", Source: "user"},
		{Name: "obey-campaign", Path: filepath.Join(m.configDir, "obey-campaign.yaml"), Transport: "acp", Dialect: "acp", Status: "ready", Source: "user"},
		{Name: "starter", Path: filepath.Join(m.configDir, "starter.yaml"), Transport: "sse", Dialect: "—", Status: "needs setup", Source: "user"},
	}
	for _, c := range m.configs {
		_ = os.WriteFile(c.Path, []byte("# prototype seed\n"), 0o600)
	}
	m.notice = "Select a configuration and press enter to open."
	m.cursor = 0
}

func (m *model) loadFromDisk() {
	entries, err := os.ReadDir(m.configDir)
	if err != nil {
		return
	}
	var rows []configRow
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
		path := filepath.Join(m.configDir, name)
		// Prototype: status from filename markers or simple parse
		status := "ready"
		transport := "sse"
		dialect := "—"
		if strings.Contains(base, "starter") || strings.Contains(base, "incomplete") {
			status = "needs setup"
		}
		if b, err := os.ReadFile(path); err == nil {
			s := string(b)
			if strings.Contains(s, "type: acp") || strings.Contains(s, "type: \"acp\"") {
				transport = "acp"
			}
			if strings.Contains(s, "type: grpc") {
				transport = "grpc"
			}
			if strings.Contains(s, "type: replay") {
				transport = "replay"
			}
			if i := strings.Index(s, "file:"); i >= 0 {
				line := s[i:]
				if nl := strings.Index(line, "\n"); nl > 0 {
					line = line[:nl]
				}
				line = strings.TrimSpace(strings.TrimPrefix(line, "file:"))
				line = strings.Trim(line, " \"'")
				if line != "" {
					dialect = line
				}
			}
			if strings.Contains(s, "base_url: \"\"") || strings.Contains(s, "base_url: ''") {
				if transport == "sse" {
					status = "needs setup"
				}
			}
		}
		rows = append(rows, configRow{
			Name: base, Path: path, Transport: transport, Dialect: dialect, Status: status, Source: "user",
		})
	}
	m.configs = rows
}

func homeItems(m model) []string {
	// For empty: New, Offline, Quit-as-last is via q
	// For list: each config + New + Offline
	var items []string
	for _, c := range m.configs {
		items = append(items, "cfg:"+c.Name)
	}
	items = append(items, "action:new", "action:offline")
	return items
}

func newConnectFields(transport string) []textinput.Model {
	mk := func(ph, val string) textinput.Model {
		t := textinput.New()
		t.Placeholder = ph
		t.SetValue(val)
		t.CharLimit = 200
		t.Width = 48
		t.Prompt = ""
		return t
	}
	switch transport {
	case "grpc":
		return []textinput.Model{
			mk("localhost:50051", "localhost:50051"),
			mk("plaintext or tls", "plaintext"),
			mk("/package.Service/Method", ""),
		}
	case "acp":
		return []textinput.Model{
			mk("agent command", "demo-acp-agent"),
			mk("args (optional)", ""),
		}
	case "replay":
		return []textinput.Model{
			mk("path to jsonl", "logs/by-session/session.jsonl"),
		}
	default: // sse
		return []textinput.Model{
			mk("https://api.example.com", ""),
			mk("/v1/stream", "/v1/stream"),
			mk("API_KEY (env name)", ""),
		}
	}
}

func templateName(i int) string {
	names := []string{"sse", "grpc", "acp", "replay", "blank"}
	if i < 0 || i >= len(names) {
		return "sse"
	}
	return names[i]
}

func dialectName(i int) string {
	names := []string{"acp", "brainyard", "openai", "custom"}
	if i < 0 || i >= len(names) {
		return "acp"
	}
	return names[i]
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

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
		case screenEditor:
			return m.updateEditor(msg)
		case screenSessionMock:
			if msg.String() == "q" || msg.String() == "esc" || msg.String() == "ctrl+c" {
				m.screen = screenHome
				m.notice = "Returned from session · " + m.sessionName
				m.status = ""
				return m, nil
			}
			return m, nil
		case screenOffline:
			return m.updateOffline(msg)
		case screenConfirmDelete:
			return m.updateConfirmDelete(msg)
		}
	}
	return m, nil
}

func (m model) updateHome(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := homeItems(m)
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
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
		if m.cursor < len(m.configs) {
			return m.openEditor(m.cursor)
		}
	case "d":
		if m.cursor < len(m.configs) {
			m.deleteIndex = m.cursor
			m.screen = screenConfirmDelete
		}
	case "o", "enter":
		item := items[m.cursor]
		switch {
		case strings.HasPrefix(item, "action:new"):
			return m.startWizard()
		case strings.HasPrefix(item, "action:offline"):
			m.screen = screenOffline
			m.cursor = 0
			return m, nil
		default:
			idx := m.cursor
			if idx >= len(m.configs) {
				return m, nil
			}
			cfg := m.configs[idx]
			if cfg.Status == "needs setup" || cfg.Status == "error" {
				return m.openEditor(idx)
			}
			m.sessionName = cfg.Name
			m.screen = screenSessionMock
			m.notice = ""
			return m, nil
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

func (m model) openEditor(idx int) (tea.Model, tea.Cmd) {
	m.editIndex = idx
	cfg := m.configs[idx]
	m.editFields = newConnectFields(cfg.Transport)
	// prefill lightly
	if cfg.Transport == "sse" && len(m.editFields) > 0 {
		// leave empty for needs setup demo
	}
	m.editFocus = 0
	m.editDirty = false
	m.screen = screenEditor
	m.editFields[0].Focus()
	return m, textinput.Blink
}

func (m model) updateWizardTemplate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenHome
		return m, nil
	case "up", "k":
		if m.wizardTemplate > 0 {
			m.wizardTemplate--
		}
	case "down", "j":
		if m.wizardTemplate < 4 {
			m.wizardTemplate++
		}
	case "enter":
		t := templateName(m.wizardTemplate)
		if t == "blank" {
			t = "sse"
		}
		m.wizardFields = newConnectFields(t)
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
		m.wizardDialect = 0
		return m, nil
	}
	var cmd tea.Cmd
	m.wizardFields[m.wizardFocus], cmd = m.wizardFields[m.wizardFocus].Update(msg)
	return m, cmd
}

func (m model) updateWizardDialect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		if m.wizardDialect < 3 {
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
		name := strings.TrimSpace(m.nameInput.Value())
		if name == "" {
			name = "my-backend"
		}
		name = sanitizeName(name)
		path := filepath.Join(m.configDir, name+".yaml")
		transport := templateName(m.wizardTemplate)
		if transport == "blank" {
			transport = "sse"
		}
		dialect := dialectName(m.wizardDialect)
		body := fmt.Sprintf("# created by home-ux prototype\ntransport:\n  type: %s\n  base_url: %q\ndialect:\n  file: %q\n",
			transport, firstField(m.wizardFields), dialect)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			m.status = "save failed: " + err.Error()
			return m, nil
		}
		m.loadFromDisk()
		// select new row
		for i, c := range m.configs {
			if c.Name == name {
				m.cursor = i
				break
			}
		}
		if m.saveChoice == 0 {
			m.sessionName = name
			m.screen = screenSessionMock
			m.notice = ""
			m.status = ""
			return m, nil
		}
		m.screen = screenHome
		m.notice = "Saved " + name + ".yaml"
		m.status = ""
		return m, nil
	}
	if m.nameInput.Focused() {
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func firstField(fields []textinput.Model) string {
	if len(fields) == 0 {
		return ""
	}
	return fields[0].Value()
}

func sanitizeName(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if r == ' ' {
			b.WriteByte('-')
		}
	}
	out := b.String()
	if out == "" {
		return "config"
	}
	return out
}

func (m model) updateEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenHome
		return m, nil
	case "ctrl+s":
		cfg := m.configs[m.editIndex]
		// mark ready by writing non-empty base
		body := fmt.Sprintf("transport:\n  type: %s\n  base_url: %q\ndialect:\n  file: %q\n",
			cfg.Transport, firstField(m.editFields), cfg.Dialect)
		if cfg.Dialect == "—" || cfg.Dialect == "" {
			body = fmt.Sprintf("transport:\n  type: %s\n  base_url: %q\ndialect:\n  file: \"brainyard\"\n",
				cfg.Transport, firstField(m.editFields))
		}
		_ = os.WriteFile(cfg.Path, []byte(body), 0o600)
		m.loadFromDisk()
		m.editDirty = false
		m.notice = "Saved " + cfg.Name
		m.screen = screenHome
		return m, nil
	case "tab", "down":
		m.editFields[m.editFocus].Blur()
		m.editFocus = (m.editFocus + 1) % len(m.editFields)
		m.editFields[m.editFocus].Focus()
		return m, textinput.Blink
	case "shift+tab", "up":
		m.editFields[m.editFocus].Blur()
		m.editFocus = (m.editFocus - 1 + len(m.editFields)) % len(m.editFields)
		m.editFields[m.editFocus].Focus()
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.editFields[m.editFocus], cmd = m.editFields[m.editFocus].Update(msg)
	m.editDirty = true
	return m, cmd
}

func (m model) updateOffline(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.screen = screenHome
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 1 {
			m.cursor++
		}
	case "enter":
		names := []string{"multi-agent-timeline", "acp-session"}
		m.sessionName = "demo:" + names[m.cursor]
		m.screen = screenSessionMock
	}
	return m, nil
}

func (m model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "n":
		m.screen = screenHome
	case "y", "enter":
		if m.deleteIndex >= 0 && m.deleteIndex < len(m.configs) {
			_ = os.Remove(m.configs[m.deleteIndex].Path)
			m.loadFromDisk()
			if m.cursor >= len(homeItems(m)) {
				m.cursor = 0
			}
			m.notice = "Deleted configuration"
		}
		m.screen = screenHome
	}
	return m, nil
}

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
	case screenEditor:
		body = m.viewEditor()
	case screenSessionMock:
		body = m.viewSessionMock()
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
	b.WriteString(titleStyle.Render("agent-stream-dbg"))
	b.WriteString("\n")
	if len(m.configs) == 0 {
		b.WriteString(dimStyle.Render("Welcome"))
	} else {
		b.WriteString(dimStyle.Render("Configurations"))
	}
	b.WriteString("\n\n")
	if m.notice != "" {
		b.WriteString(noticeStyle.Render(m.notice))
		b.WriteString("\n\n")
	}
	if len(m.configs) == 0 {
		b.WriteString(dimStyle.Render("No configurations yet. Create one to connect to an"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("SSE, gRPC, or ACP stream — or try an offline demo."))
		b.WriteString("\n\n")
	}

	items := homeItems(m)
	for i, item := range items {
		cursor := "  "
		style := dimStyle
		if i == m.cursor {
			cursor = "▸ "
			style = cursorStyle
		}
		if strings.HasPrefix(item, "cfg:") {
			cfg := m.configs[i]
			chip := chipReady.Render(cfg.Status)
			if cfg.Status == "needs setup" {
				chip = chipNeed.Render(cfg.Status)
			} else if cfg.Status == "error" {
				chip = chipErr.Render(cfg.Status)
			}
			line := fmt.Sprintf("%s%-18s  %-6s  %-12s  %s", cursor, cfg.Name, cfg.Transport, cfg.Dialect, chip)
			b.WriteString(style.Render(line))
			b.WriteString("\n")
			if i == m.cursor {
				b.WriteString(dimStyle.Render("    " + cfg.Path))
				b.WriteString("\n")
			}
			continue
		}
		label := item
		switch item {
		case "action:new":
			label = "+ New configuration"
		case "action:offline":
			label = "◎ Offline demos"
		}
		// separator before actions once
		if i == len(m.configs) && len(m.configs) > 0 {
			b.WriteString(dimStyle.Render("────────────────────────────────────────"))
			b.WriteString("\n")
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
		"Blank",
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
	t := templateName(m.wizardTemplate)
	if t == "blank" {
		t = "sse"
	}
	b.WriteString(dimStyle.Render("Connection · transport: " + t))
	b.WriteString("\n\n")
	labels := connectLabels(t)
	for i, f := range m.wizardFields {
		label := labelStyle.Render(labels[i])
		if i == m.wizardFocus {
			label = cursorStyle.Width(14).Render("▸ " + labels[i])
		}
		b.WriteString(label)
		b.WriteString(f.View())
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("tab fields  enter next  esc back"))
	return b.String()
}

func connectLabels(transport string) []string {
	switch transport {
	case "grpc":
		return []string{"Target", "Security", "Method"}
	case "acp":
		return []string{"Command", "Args"}
	case "replay":
		return []string{"Fixture"}
	default:
		return []string{"Base URL", "Endpoint", "Auth env"}
	}
}

func (m model) viewWizardDialect() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("New configuration (3/4)"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Dialect — what the bytes mean"))
	b.WriteString("\n\n")
	opts := []string{
		"acp            Agent Client Protocol",
		"brainyard      multi-agent SSE (embedded)",
		"openai         chat completions stream",
		"path…          load a YAML dialect file",
	}
	for i, o := range opts {
		cursor := "  "
		style := dimStyle
		if i == m.wizardDialect {
			cursor = "▸ "
			style = cursorStyle
		}
		b.WriteString(style.Render(cursor + o))
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
	path := filepath.Join(m.configDir, sanitizeName(name)+".yaml")
	b.WriteString(dimStyle.Render("File  " + path))
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
	return b.String()
}

func (m model) viewEditor() string {
	var b strings.Builder
	cfg := m.configs[m.editIndex]
	title := "Edit · " + cfg.Name
	if m.editDirty {
		title += " •"
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(cfg.Path))
	b.WriteString("\n\n")
	labels := connectLabels(cfg.Transport)
	if len(labels) == 0 {
		labels = connectLabels("sse")
	}
	for i, f := range m.editFields {
		lab := labels[i]
		label := labelStyle.Render(lab)
		if i == m.editFocus {
			label = cursorStyle.Width(14).Render("▸ " + lab)
		}
		b.WriteString(label)
		b.WriteString(f.View())
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("ctrl+s save  esc cancel"))
	return b.String()
}

func (m model) viewSessionMock() string {
	var b strings.Builder
	b.WriteString(sessionStyle.Render("Interactive session (prototype)"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Opened configuration: %s\n", m.sessionName))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("In the real product this is the existing multi-pane TUI."))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Quit returns to the home hub instead of exiting the process."))
	b.WriteString("\n\n")
	b.WriteString(noticeStyle.Render("esc / q  →  back to home"))
	return b.String()
}

func (m model) viewOffline() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Offline demos"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("No network required"))
	b.WriteString("\n\n")
	opts := []string{
		"Multi-agent timeline fixture",
		"ACP session fixture",
	}
	for i, o := range opts {
		cursor := "  "
		style := dimStyle
		if i == m.cursor {
			cursor = "▸ "
			style = cursorStyle
		}
		b.WriteString(style.Render(cursor + o))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("enter run  esc back"))
	return b.String()
}

func (m model) viewConfirmDelete() string {
	name := m.configs[m.deleteIndex].Name
	var b strings.Builder
	b.WriteString(titleStyle.Render("Delete configuration"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Delete %q?\n", name))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("y / enter confirm  n / esc cancel"))
	return b.String()
}
