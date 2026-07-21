package home

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/Obedience-Corp/agent-stream-dbg/dialects"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/config"
)

const (
	screenHome = iota
	screenWizardTemplate
	screenWizardConnect
	screenWizardDialect
	screenWizardSave
	screenOffline
	screenConfirmDelete
)

type model struct {
	width, height int
	screen        int
	cursor        int
	entries       []config.ConfigEntry
	notice        string
	status        string
	opts          Options
	result        Result
	quitting      bool

	// wizard
	wizardTemplate int
	wizardDialect  int
	wizardFields   []textinput.Model
	wizardFocus    int
	nameInput      textinput.Model
	saveChoice     int // 0 open, 1 home
	dialectNames   []string

	// delete
	deleteIndex int

	// offline
	offlineDemos []OfflineDemo
}

func newModel(opts Options) model {
	m := model{
		screen:       screenHome,
		opts:         opts,
		notice:       opts.Notice,
		nameInput:    textinput.New(),
		dialectNames: dialects.Available(),
		offlineDemos: OfflineDemos(),
	}
	m.nameInput.Placeholder = "my-backend"
	m.nameInput.CharLimit = 64
	m.nameInput.Width = 40
	m.nameInput.Prompt = ""
	if len(m.dialectNames) == 0 {
		m.dialectNames = []string{"brainyard", "acp", "openai"}
	}
	m.reload()
	if len(m.entries) == 0 && m.notice == "" {
		m.notice = "Welcome — create a configuration or try an offline demo."
	}
	return m
}

func (m *model) reload() {
	m.entries = config.ListConfigs(m.opts.Cwd, m.opts.UserConfigDir)
	if m.cursor >= len(m.homeItems()) {
		m.cursor = 0
	}
}

func (m model) homeItems() []string {
	items := make([]string, 0, len(m.entries)+2)
	for range m.entries {
		items = append(items, "cfg")
	}
	items = append(items, "action:new", "action:offline")
	return items
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func templateKind(i int) config.TemplateKind {
	switch i {
	case 1:
		return config.TemplateGRPC
	case 2:
		return config.TemplateACP
	case 3:
		return config.TemplateReplay
	default:
		return config.TemplateSSE
	}
}

func newConnectFields(kind config.TemplateKind) []textinput.Model {
	mk := func(ph, val string) textinput.Model {
		t := textinput.New()
		t.Placeholder = ph
		t.SetValue(val)
		t.CharLimit = 500
		t.Width = 48
		t.Prompt = ""
		return t
	}
	switch kind {
	case config.TemplateGRPC:
		return []textinput.Model{
			mk("localhost:50051", "localhost:50051"),
			mk("/package.Service/Method", ""),
			mk("TOKEN_ENV (optional)", ""),
		}
	case config.TemplateACP:
		return []textinput.Model{
			mk("agent command", "grok"),
			mk("args (space-separated, optional)", "agent acp"),
		}
	case config.TemplateReplay:
		return []textinput.Model{
			mk("path to jsonl fixture", ""),
		}
	default:
		return []textinput.Model{
			mk("https://api.example.com", ""),
			mk("/v1/stream", "/v1/stream"),
			mk("API_KEY (env name, optional)", ""),
		}
	}
}

func connectLabels(kind config.TemplateKind) []string {
	switch kind {
	case config.TemplateGRPC:
		return []string{"Target", "Method", "Auth env"}
	case config.TemplateACP:
		return []string{"Command", "Args"}
	case config.TemplateReplay:
		return []string{"Fixture"}
	default:
		return []string{"Base URL", "Endpoint", "Auth env"}
	}
}

func (m model) exitOpen(path string, openPanel bool, notice string) (tea.Model, tea.Cmd) {
	m.result = Result{
		Action:          ActionOpen,
		Path:            path,
		OpenConfigPanel: openPanel,
		Notice:          notice,
	}
	m.quitting = true
	return m, tea.Quit
}

func (m model) exitQuit() (tea.Model, tea.Cmd) {
	m.result = Result{Action: ActionQuit}
	m.quitting = true
	return m, tea.Quit
}

func shortPath(path string) string {
	return config.DisplayPath(path, "")
}
