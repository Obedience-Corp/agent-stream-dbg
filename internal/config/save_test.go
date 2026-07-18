package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveConfigFile_UpdatesRuntimeFieldsAndPreservesAuthConfig(t *testing.T) {
	t.Setenv("API_KEY", "test-key")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	original := `transport:
  type: sse
  base_url: https://old.example
  stream_endpoint:
    url: /old-stream
    auth:
      type: bearer
      token_env: API_KEY
dialect:
  file: dialects/reference.yaml
vars:
  old: value
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := &EnhancedConfig{
		Transport: TransportConfig{
			Type:           "sse",
			BaseURL:        "https://new.example",
			StreamEndpoint: "/stream",
		},
		Dialect: DialectConfig{File: "dialects/openai.yaml"},
		Vars:    map[string]string{"region": "us-west", "tier": "premium"},
	}
	if err := SaveConfigFile(path, cfg); err != nil {
		t.Fatalf("SaveConfigFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	saved := string(data)
	for _, want := range []string{
		"https://new.example",
		"/stream",
		"dialects/openai.yaml",
		"region: us-west",
		"token_env: API_KEY",
	} {
		if !strings.Contains(saved, want) {
			t.Errorf("saved config missing %q:\n%s", want, saved)
		}
	}
	if strings.Contains(saved, "test-key") {
		t.Error("saved config leaked the resolved API key")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat saved config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("expected permissions 0600 to be preserved, got %o", got)
	}
}

func TestSaveConfigFile_RequiresPathAndConfig(t *testing.T) {
	if err := SaveConfigFile("", &EnhancedConfig{}); err == nil {
		t.Error("expected empty path to fail")
	}
	if err := SaveConfigFile(filepath.Join(t.TempDir(), "config.yaml"), nil); err == nil {
		t.Error("expected nil config to fail")
	}
}

func TestSaveConfigFile_PersistsGRPCRequestAndMethod(t *testing.T) {
	t.Setenv("OBEY_GRPC_TOKEN", "test-metadata-token")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("transport:\n  type: grpc\ndialect:\n  file: obey\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := &EnhancedConfig{
		Transport: TransportConfig{
			Type:          "grpc",
			Target:        "unix:///tmp/obey.sock",
			GRPCMethod:    "/local.v1.LocalDaemonService/WatchCampaignState",
			Request:       map[string]any{"include_initial_snapshot": true},
			Discriminator: "oneof",
			Plaintext:     true,
			Auth: AuthConfig{
				Type:       "metadata",
				HeaderName: "authorization",
				TokenEnv:   "OBEY_GRPC_TOKEN",
				Token:      "test-metadata-token",
			},
		},
		Dialect: DialectConfig{File: "obey"},
	}
	if err := SaveConfigFile(path, cfg); err != nil {
		t.Fatalf("SaveConfigFile: %v", err)
	}

	saved, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if saved.Transport.GRPCMethod != cfg.Transport.GRPCMethod {
		t.Fatalf("saved method = %q, want %q", saved.Transport.GRPCMethod, cfg.Transport.GRPCMethod)
	}
	if got, ok := saved.Transport.Request["include_initial_snapshot"].(bool); !ok || !got {
		t.Fatalf("saved request = %#v, want include_initial_snapshot=true", saved.Transport.Request)
	}
	if !saved.Transport.Plaintext {
		t.Fatal("saved plaintext setting was lost")
	}
	key, value, ok := saved.Transport.Auth.Metadata()
	if !ok || key != "authorization" || value != "test-metadata-token" {
		t.Fatalf("saved gRPC metadata auth = %q=%q, ok=%v", key, value, ok)
	}
}
