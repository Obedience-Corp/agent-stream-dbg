package visualizer

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lancekrogers/stream-debugger/internal/bridge"
	"github.com/lancekrogers/stream-debugger/internal/config"
)

type configField int

const (
	configFieldDialect configField = iota
	configFieldTransport
	configFieldBaseURL
	configFieldStreamEndpoint
	configFieldGRPCTarget
	configFieldVars
)

type configFieldState struct {
	label string
	input textinput.Model
}

type configPanel struct {
	open    bool
	focused int
	fields  []configFieldState
	notice  string
	status  string
}

func newConfigPanel(cfg *config.EnhancedConfig) configPanel {
	values := []string{"", "sse", "", "", "", ""}
	if cfg != nil {
		transportType := cfg.Transport.Type
		if transportType == "" {
			transportType = "sse"
		}
		values = []string{
			cfg.Dialect.File,
			transportType,
			cfg.Transport.BaseURL,
			cfg.Transport.StreamEndpoint,
			cfg.Transport.Target,
			formatConfigVars(cfg.Vars),
		}
	}
	labels := []string{"Dialect", "Transport", "Base URL", "SSE endpoint", "gRPC target", "Vars"}
	placeholders := []string{
		"embedded name or path to dialect YAML",
		"sse, grpc, or replay",
		"https://api.example.com",
		"/v1/stream",
		"localhost:50051",
		"key=value,other=value",
	}
	fields := make([]configFieldState, len(labels))
	for i, label := range labels {
		input := textinput.New()
		input.Prompt = ""
		input.Placeholder = placeholders[i]
		input.CharLimit = 2000
		input.Width = 64
		input.SetValue(values[i])
		fields[i] = configFieldState{label: label, input: input}
	}
	fields[0].input.Focus()
	return configPanel{focused: 0, fields: fields}
}

func (m *InteractiveModel) openConfigPanel() tea.Cmd {
	m.configPanel = newConfigPanel(m.cfg)
	m.configPanel.open = true
	m.insertMode = false
	m.textarea.Blur()
	m.contentDirty = true
	return m.configPanel.fields[0].input.Focus()
}

func (m *InteractiveModel) closeConfigPanel() {
	if len(m.configPanel.fields) > 0 {
		m.configPanel.fields[m.configPanel.focused].input.Blur()
	}
	m.configPanel.open = false
	m.configPanel.status = ""
	m.contentDirty = true
}

func (m InteractiveModel) handleConfigKeyMsg(msg tea.KeyMsg) (InteractiveModel, tea.Cmd, bool) {
	if msg.Type == tea.KeyCtrlC {
		m.closeActiveStream()
		return m, tea.Quit, true
	}
	if msg.Type == tea.KeyEsc {
		m.closeConfigPanel()
		return m, nil, true
	}
	if len(m.configPanel.fields) == 0 {
		return m, nil, true
	}

	switch msg.Type {
	case tea.KeyTab:
		return m, m.focusConfigField(1), true
	case tea.KeyShiftTab:
		return m, m.focusConfigField(-1), true
	case tea.KeyEnter:
		cmd, err := m.applyConfigPanel(true)
		if err != nil {
			m.configPanel.status = err.Error()
			return m, nil, true
		}
		return m, cmd, true
	case tea.KeyCtrlS:
		if strings.TrimSpace(m.configPath) == "" {
			m.configPanel.status = "Save unavailable: no config path was provided"
			return m, nil, true
		}
		if _, err := m.applyConfigPanel(false); err != nil {
			m.configPanel.status = err.Error()
			return m, nil, true
		}
		if err := config.SaveConfigFile(m.configPath, m.cfg); err != nil {
			m.saveStatus = fmt.Sprintf("Config save failed: %v", err)
		} else {
			m.saveStatus = fmt.Sprintf("Config saved to %s", filepath.Base(m.configPath))
		}
		return m, nil, true
	}

	var cmd tea.Cmd
	m.configPanel.fields[m.configPanel.focused].input, cmd = m.configPanel.fields[m.configPanel.focused].input.Update(msg)
	return m, cmd, true
}

func (m *InteractiveModel) focusConfigField(delta int) tea.Cmd {
	if len(m.configPanel.fields) == 0 {
		return nil
	}
	m.configPanel.fields[m.configPanel.focused].input.Blur()
	m.configPanel.focused = (m.configPanel.focused + delta) % len(m.configPanel.fields)
	if m.configPanel.focused < 0 {
		m.configPanel.focused += len(m.configPanel.fields)
	}
	return m.configPanel.fields[m.configPanel.focused].input.Focus()
}

func (m *InteractiveModel) applyConfigPanel(reconnect bool) (tea.Cmd, error) {
	if m.cfg == nil {
		return nil, fmt.Errorf("cannot apply configuration: config is nil")
	}
	values := make([]string, len(m.configPanel.fields))
	for i := range m.configPanel.fields {
		values[i] = strings.TrimSpace(m.configPanel.fields[i].input.Value())
	}
	if len(values) != 6 {
		return nil, fmt.Errorf("configuration panel is incomplete")
	}
	transportType := strings.ToLower(values[configFieldTransport])
	if transportType == "" {
		transportType = "sse"
	}
	if transportType != "sse" && transportType != "grpc" && transportType != "replay" {
		return nil, fmt.Errorf("unsupported transport %q (use sse, grpc, or replay)", transportType)
	}
	vars, err := parseConfigVars(values[configFieldVars])
	if err != nil {
		return nil, err
	}

	candidate := *m.cfg
	candidate.Transport = m.cfg.Transport
	candidate.Transport.Type = transportType
	candidate.Transport.BaseURL = values[configFieldBaseURL]
	candidate.Transport.StreamEndpoint = values[configFieldStreamEndpoint]
	candidate.Transport.Target = values[configFieldGRPCTarget]
	candidate.Dialect = m.cfg.Dialect
	candidate.Dialect.File = values[configFieldDialect]
	candidate.Vars = vars
	candidateParser, err := bridge.NewParserFor(candidate.Dialect.File)
	if err != nil {
		return nil, fmt.Errorf("cannot load dialect %q: %w", candidate.Dialect.File, err)
	}
	candidate.Normalize()

	m.closeActiveStream()
	m.cfg = &candidate
	m.parser = candidateParser
	m.dialectFlow = newFlowStateWithParser(candidateParser)
	m.err = nil
	m.configPanel.open = false
	m.configPanel.status = ""
	m.saveStatus = fmt.Sprintf("Applied %s transport", transportType)
	m.contentDirty = true
	if reconnect {
		return m.reconnectLatestMessage(), nil
	}
	return nil, nil
}

func (m *InteractiveModel) reconnectLatestMessage() tea.Cmd {
	if len(m.messages) == 0 {
		return nil
	}
	index := len(m.messages) - 1
	message := strings.TrimSpace(m.messages[index].Text)
	if message == "" {
		return nil
	}
	m.messages[index].RawSSE = ""
	m.messages[index].Events = nil
	m.messages[index].AgentResponses = make(map[string]*AgentResponse)
	m.messages[index].Streaming = true
	m.streamIndex = index
	m.flowTurnIndex = index
	m.selectedEventIdx = 0
	m.streaming = true
	m.contentDirty = true
	return m.startStreamingCmd(message, index)
}

func parseConfigVars(raw string) (map[string]string, error) {
	vars := make(map[string]string)
	if strings.TrimSpace(raw) == "" {
		return vars, nil
	}
	for _, part := range strings.Split(raw, ",") {
		pair := strings.SplitN(part, "=", 2)
		if len(pair) != 2 || strings.TrimSpace(pair[0]) == "" {
			return nil, fmt.Errorf("invalid var %q (use key=value, comma-separated)", part)
		}
		vars[strings.TrimSpace(pair[0])] = strings.TrimSpace(pair[1])
	}
	return vars, nil
}

func formatConfigVars(vars map[string]string) string {
	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+vars[key])
	}
	return strings.Join(parts, ",")
}
