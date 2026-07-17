package visualizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lancekrogers/stream-debugger/internal/config"
)

func TestConfigVarsRoundTripIsDeterministic(t *testing.T) {
	raw := formatConfigVars(map[string]string{"tier": "premium", "region": "us-west"})
	if raw != "region=us-west,tier=premium" {
		t.Fatalf("unexpected formatted vars %q", raw)
	}
	vars, err := parseConfigVars(raw)
	if err != nil {
		t.Fatalf("parseConfigVars: %v", err)
	}
	if vars["region"] != "us-west" || vars["tier"] != "premium" {
		t.Errorf("unexpected parsed vars %#v", vars)
	}
}

func TestConfigPanel_ApplyReconnectUpdatesRuntimeConfig(t *testing.T) {
	cfg := &config.EnhancedConfig{LogDir: t.TempDir()}
	m := NewInteractiveModel(cfg)

	updated, _, handled := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if !handled || !updated.configPanel.open {
		t.Fatalf("expected c to open config panel; handled=%v open=%v", handled, updated.configPanel.open)
	}
	setConfigField(&updated, configFieldDialect, "openai")
	setConfigField(&updated, configFieldTransport, "sse")
	setConfigField(&updated, configFieldBaseURL, "https://example.test")
	setConfigField(&updated, configFieldStreamEndpoint, "/stream")
	setConfigField(&updated, configFieldVars, "region=us-west,tier=premium")
	updated.messages = append(updated.messages, Message{Text: "hello"})

	updated, cmd, handled := updated.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("expected Enter to apply config")
	}
	if cmd == nil {
		t.Fatal("expected Apply & Reconnect to schedule the latest message")
	}
	if updated.configPanel.open {
		t.Error("expected config panel to close after applying")
	}
	if got := updated.cfg.Dialect.File; got != "openai" {
		t.Errorf("expected dialect update, got %q", got)
	}
	if got := updated.cfg.Transport.BaseURL; got != "https://example.test" {
		t.Errorf("expected base URL update, got %q", got)
	}
	if got := updated.cfg.Vars["region"]; got != "us-west" {
		t.Errorf("expected vars update, got %q", got)
	}
	if !updated.streaming || !updated.messages[0].Streaming {
		t.Error("expected Apply & Reconnect to mark the latest message streaming")
	}
}

func TestConfigPanel_InvalidVarsStayOpen(t *testing.T) {
	m := NewInteractiveModel(&config.EnhancedConfig{LogDir: t.TempDir()})
	updated, _, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	setConfigField(&updated, configFieldVars, "not-a-pair")

	updated, cmd, handled := updated.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled || cmd != nil {
		t.Fatalf("expected invalid config to be handled without a command; handled=%v cmd=%v", handled, cmd)
	}
	if !updated.configPanel.open {
		t.Error("expected invalid config to keep panel open")
	}
	if !strings.Contains(updated.configPanel.status, "key=value") {
		t.Errorf("expected actionable var error, got %q", updated.configPanel.status)
	}
}

func TestConfigPanel_SaveUsesLaunchConfigPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := "transport:\n  type: sse\n  base_url: https://old.example\n  stream_endpoint:\n    url: /old\ndialect:\n  file: openai\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg := &config.EnhancedConfig{LogDir: t.TempDir()}
	m := NewInteractiveModelWithContextAndConfigPath(cfg, nil, path)
	updated, _, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	setConfigField(&updated, configFieldBaseURL, "https://new.example")

	updated, cmd, handled := updated.handleKeyMsg(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !handled || cmd != nil {
		t.Fatalf("expected Ctrl+S to save synchronously; handled=%v cmd=%v", handled, cmd)
	}
	if updated.configPanel.open {
		t.Error("expected config panel to close after saving")
	}
	if !strings.Contains(updated.saveStatus, "Config saved") {
		t.Errorf("expected save status, got %q", updated.saveStatus)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if !strings.Contains(string(saved), "https://new.example") {
		t.Errorf("saved config did not contain updated URL:\n%s", saved)
	}
}

func setConfigField(m *InteractiveModel, field configField, value string) {
	m.configPanel.fields[field].input.SetValue(value)
}
