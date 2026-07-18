package visualizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lancekrogers/stream-debugger/internal/client"
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

func TestConfigAgentsRoundTripIsDeterministic(t *testing.T) {
	got := formatConfigAgents([]string{" sam_harris ", "eckhart_tolle", "wizard"})
	if got != "sam_harris,eckhart_tolle,wizard" {
		t.Fatalf("unexpected formatted agents %q", got)
	}
	agents, err := parseConfigAgents(got)
	if err != nil {
		t.Fatalf("parseConfigAgents: %v", err)
	}
	if strings.Join(agents, ",") != got {
		t.Errorf("unexpected parsed agents %#v", agents)
	}
}

func TestConfigPanelTransportFieldsAreScoped(t *testing.T) {
	sse := newConfigPanel(&config.EnhancedConfig{Transport: config.TransportConfig{Type: "sse"}})
	if !sse.fields[configFieldBaseURL].active || !sse.fields[configFieldStreamEndpoint].active {
		t.Fatal("expected SSE fields to be active for SSE transport")
	}
	if sse.fields[configFieldGRPCTarget].active {
		t.Fatal("expected gRPC target to be hidden for SSE transport")
	}

	sse.fields[configFieldTransport].input.SetValue("grpc")
	sse.refreshFieldVisibility()
	if !sse.fields[configFieldGRPCTarget].active {
		t.Fatal("expected gRPC target to be active for gRPC transport")
	}
	for _, field := range []configField{configFieldGRPCSecurity, configFieldGRPCAuthKey, configFieldGRPCMethod, configFieldGRPCRequest, configFieldGRPCDiscriminator, configFieldGRPCDiscriminatorField} {
		if !sse.fields[field].active {
			t.Fatalf("expected gRPC field %d to be active for gRPC transport", field)
		}
	}
	if sse.fields[configFieldBaseURL].active || sse.fields[configFieldStreamEndpoint].active {
		t.Fatal("expected SSE fields to be hidden for gRPC transport")
	}

	sse.fields[configFieldTransport].input.SetValue("replay")
	sse.refreshFieldVisibility()
	if !sse.fields[configFieldBaseURL].active || sse.fields[configFieldStreamEndpoint].active || sse.fields[configFieldGRPCTarget].active {
		t.Fatal("expected replay to show only the replay file field")
	}
}

func TestConfigPanelTypingGRPCReplacesStarterSSEDefault(t *testing.T) {
	m := NewInteractiveModel(&config.EnhancedConfig{Transport: config.TransportConfig{Type: "sse"}})
	m.openConfigPanel()
	m.configPanel.focused = int(configFieldTransport)
	m.configPanel.fields[configFieldTransport].input.Focus()
	if _, _, handled := m.handleConfigKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")}); !handled {
		t.Fatal("expected transport typing to be handled")
	}
	if got := m.configPanel.fields[configFieldTransport].input.Value(); got != "g" {
		t.Fatalf("transport value = %q, want starter default replaced by g", got)
	}
}

func TestConfigRequestRoundTrip(t *testing.T) {
	request, err := parseConfigRequest(`{"include_initial_snapshot":true,"campaign_id":"demo"}`)
	if err != nil {
		t.Fatalf("parseConfigRequest: %v", err)
	}
	if got, ok := request["include_initial_snapshot"].(bool); !ok || !got {
		t.Fatalf("unexpected request %#v", request)
	}
	formatted := formatConfigRequest(request)
	if formatted != `{"campaign_id":"demo","include_initial_snapshot":true}` {
		t.Fatalf("unexpected deterministic JSON %q", formatted)
	}
}

func TestConfigPanelAuthEnvConfiguresBearerWithoutStoringSecretInConfig(t *testing.T) {
	t.Setenv("STREAM_DEBUGGER_TEST_API_KEY", "test-secret")
	m := NewInteractiveModel(&config.EnhancedConfig{LogDir: t.TempDir()})
	m.openConfigPanel()
	setConfigField(&m, configFieldDialect, "brainyard")
	setConfigField(&m, configFieldAuthEnv, "STREAM_DEBUGGER_TEST_API_KEY")
	setConfigField(&m, configFieldAgents, "sam_harris,eckhart_tolle,wizard")

	if _, err := m.applyConfigPanel(false); err != nil {
		t.Fatalf("applyConfigPanel: %v", err)
	}
	if got := m.cfg.Transport.Auth.Type; got != "bearer" {
		t.Fatalf("auth type = %q, want bearer", got)
	}
	if got := m.cfg.Transport.Auth.TokenEnv; got != "STREAM_DEBUGGER_TEST_API_KEY" {
		t.Fatalf("token env = %q, want STREAM_DEBUGGER_TEST_API_KEY", got)
	}
	if got := m.cfg.Transport.Auth.Token; got != "test-secret" {
		t.Fatalf("resolved token = %q, want test-secret", got)
	}
	if got := strings.Join(m.cfg.Session.DefaultAgents, ","); got != "sam_harris,eckhart_tolle,wizard" {
		t.Fatalf("default agents = %q", got)
	}
}

func TestConfigPanelSavePersistsAuthReferenceNotSecret(t *testing.T) {
	t.Setenv("STREAM_DEBUGGER_TEST_API_KEY", "test-secret")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := "transport:\n  type: sse\n  stream_endpoint:\n    auth:\n      type: none\ndialect:\n  file: brainyard\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	m := NewInteractiveModelWithContextAndConfigPath(&config.EnhancedConfig{LogDir: t.TempDir()}, nil, path)
	m.openConfigPanel()
	setConfigField(&m, configFieldAuthEnv, "STREAM_DEBUGGER_TEST_API_KEY")
	setConfigField(&m, configFieldAgents, "sam_harris,eckhart_tolle,wizard")
	if _, _, handled := m.handleConfigKeyMsg(tea.KeyMsg{Type: tea.KeyCtrlS}); !handled {
		t.Fatal("expected Ctrl+S to be handled")
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	text := string(saved)
	if !strings.Contains(text, "type: bearer") || !strings.Contains(text, "token_env: STREAM_DEBUGGER_TEST_API_KEY") {
		t.Fatalf("saved config did not contain the auth reference:\n%s", text)
	}
	if !strings.Contains(text, "- sam_harris") || !strings.Contains(text, "- wizard") {
		t.Fatalf("saved config did not contain default agents:\n%s", text)
	}
	if strings.Contains(text, "test-secret") {
		t.Fatal("saved config contained the resolved secret")
	}
}

func TestConfigPanelFocusSkipsInactiveTransportFields(t *testing.T) {
	m := NewInteractiveModel(&config.EnhancedConfig{Transport: config.TransportConfig{Type: "sse"}})
	m.openConfigPanel()
	m.configPanel.focused = int(configFieldStreamEndpoint)
	m.focusConfigField(1)
	if m.configPanel.focused != int(configFieldAgents) {
		t.Fatalf("focus moved to field %d, want Agents", m.configPanel.focused)
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

func TestConfigPanel_ApplyGRPCRequest(t *testing.T) {
	t.Setenv("STREAM_DEBUGGER_TEST_GRPC_TOKEN", "test-token")
	m := NewInteractiveModel(&config.EnhancedConfig{LogDir: t.TempDir()})
	m.openConfigPanel()
	setConfigField(&m, configFieldDialect, "obey")
	setConfigField(&m, configFieldTransport, "grpc")
	setConfigField(&m, configFieldAuthEnv, "STREAM_DEBUGGER_TEST_GRPC_TOKEN")
	setConfigField(&m, configFieldGRPCTarget, "unix:///tmp/obey.sock")
	setConfigField(&m, configFieldGRPCSecurity, "plaintext")
	setConfigField(&m, configFieldGRPCAuthKey, "x-obey-token")
	setConfigField(&m, configFieldGRPCMethod, "/local.v1.LocalDaemonService/WatchCampaignState")
	setConfigField(&m, configFieldGRPCRequest, `{"include_initial_snapshot":true}`)
	setConfigField(&m, configFieldGRPCDiscriminator, "oneof")

	if _, err := m.applyConfigPanel(false); err != nil {
		t.Fatalf("applyConfigPanel: %v", err)
	}
	if m.cfg.Transport.GRPCMethod != "/local.v1.LocalDaemonService/WatchCampaignState" {
		t.Fatalf("unexpected gRPC method %q", m.cfg.Transport.GRPCMethod)
	}
	if !m.cfg.Transport.Plaintext {
		t.Fatal("expected plaintext gRPC security")
	}
	if got := m.cfg.Transport.Auth.HeaderName; got != "x-obey-token" {
		t.Fatalf("metadata key = %q, want x-obey-token", got)
	}
	if got, ok := m.cfg.Transport.Request["include_initial_snapshot"].(bool); !ok || !got {
		t.Fatalf("unexpected gRPC request %#v", m.cfg.Transport.Request)
	}
}

func TestConfigPanelGRPCPickerFillsMethodRequestAndDiscriminator(t *testing.T) {
	m := NewInteractiveModel(&config.EnhancedConfig{LogDir: t.TempDir()})
	m.openConfigPanel()
	setConfigField(&m, configFieldTransport, "grpc")
	setConfigField(&m, configFieldGRPCTarget, "127.0.0.1:50051")

	updated, _ := m.Update(grpcMethodsMsg{methods: []client.GRPCStreamingMethod{
		{
			Path:          "/agent.v1.Agent/Watch",
			Shape:         "server-streaming",
			InputType:     "agent.v1.WatchRequest",
			OutputType:    "agent.v1.Event",
			RequestJSON:   `{"session_id":""}`,
			Discriminator: "message_type",
			Supported:     true,
		}}})
	m = updated.(InteractiveModel)
	if !m.configPanel.grpcMethodPickerOpen {
		t.Fatal("expected gRPC method picker to open after discovery")
	}

	m, _, handled := m.handleConfigKeyMsg(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("expected method picker Enter to be handled")
	}
	if m.configPanel.grpcMethodPickerOpen {
		t.Fatal("expected picker to close after selecting a method")
	}
	if got := m.configPanel.fields[configFieldGRPCMethod].input.Value(); got != "/agent.v1.Agent/Watch" {
		t.Errorf("method field = %q", got)
	}
	if got := m.configPanel.fields[configFieldGRPCRequest].input.Value(); got != `{"session_id":""}` {
		t.Errorf("request field = %q", got)
	}
	if got := m.configPanel.fields[configFieldGRPCDiscriminator].input.Value(); got != "message_type" {
		t.Errorf("discriminator field = %q", got)
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
