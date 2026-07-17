package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeYAMLConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadConfigFile_StrictUnknownFields(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantErrSub []string
	}{
		{
			name: "typo'd top-level key",
			yaml: `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "bearer"
      token_env: "API_KEY"
sesion:
  id_env: "SESSION_ID"
`,
			wantErrSub: []string{"sesion"},
		},
		{
			name: "typo'd nested key",
			yaml: `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "bearer"
      typ: "bearer"
      token_env: "API_KEY"
`,
			wantErrSub: []string{"typ"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv("API_KEY", "test-key")
			defer func() { _ = os.Unsetenv("API_KEY") }()

			path := writeYAMLConfig(t, tt.yaml)
			_, err := LoadConfigFile(path)
			if err == nil {
				t.Fatalf("expected error for unknown key, got nil")
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("expected error to name the config file %q, got: %v", path, err)
			}
			for _, sub := range tt.wantErrSub {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("expected error to mention offending key %q, got: %v", sub, err)
				}
			}
		})
	}
}

func TestLoadConfigFile_MalformedYAML(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "bad indentation",
			yaml: `
transport:
  base_url: "http://localhost:8080"
 stream_endpoint:
    url: "/api/stream"
`,
		},
		{
			name: "unterminated flow sequence",
			yaml: `
transport:
  base_url: "http://localhost:8080"
session:
  default_agents: [a, b
`,
		},
		{
			name: "not a mapping at all",
			yaml: `- just
- a
- list
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeYAMLConfig(t, tt.yaml)
			_, err := LoadConfigFile(path)
			if err == nil {
				t.Fatalf("expected error for malformed YAML, got nil")
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("expected error to name the config file %q, got: %v", path, err)
			}
		})
	}
}

func TestLoadConfigFile_ValidConfigLoads(t *testing.T) {
	_ = os.Setenv("API_KEY", "test-key")
	defer func() { _ = os.Unsetenv("API_KEY") }()

	path := writeYAMLConfig(t, `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    method: "POST"
    auth:
      type: "bearer"
      token_env: "API_KEY"
session:
  id_env: "SESSION_ID"
`)

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error loading valid config: %v", err)
	}
	if cfg.Transport.BaseURL != "http://localhost:8080" {
		t.Errorf("expected base_url to load, got %q", cfg.Transport.BaseURL)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("expected APIKey from env, got %q", cfg.APIKey)
	}
}

func TestLoadConfigFile_SSETransport_DefaultsToNoAuth(t *testing.T) {
	t.Setenv("API_KEY", "")
	path := writeYAMLConfig(t, `
transport:
  type: sse
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
`)

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error loading minimal config: %v", err)
	}
	if cfg.Transport.Auth.Type != "none" {
		t.Fatalf("expected auth type none, got %q", cfg.Transport.Auth.Type)
	}
	if name, value, ok := cfg.Transport.Auth.Header(); ok {
		t.Fatalf("expected no auth header, got %s=%s", name, value)
	}
	if cfg.APIKey != "" {
		t.Fatalf("expected no ambient API_KEY resolution, got %q", cfg.APIKey)
	}
}

func TestLoadConfigFile_DialectFileReference(t *testing.T) {
	_ = os.Setenv("API_KEY", "test-key")
	defer func() { _ = os.Unsetenv("API_KEY") }()

	path := writeYAMLConfig(t, `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "bearer"
      token_env: "API_KEY"
dialect:
  file: "dialects/reference.yaml"
`)

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Dialect.File != "dialects/reference.yaml" {
		t.Errorf("expected dialect.file to be stored, got %q", cfg.Dialect.File)
	}
}

func TestLoadConfigFile_VarsResolution(t *testing.T) {
	_ = os.Setenv("API_KEY", "test-key")
	defer func() { _ = os.Unsetenv("API_KEY") }()

	path := writeYAMLConfig(t, `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "bearer"
      token_env: "API_KEY"
vars:
  region: "us-west"
  tier: "premium"
`)

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Vars["region"] != "us-west" || cfg.Vars["tier"] != "premium" {
		t.Errorf("expected vars to be passed through, got %+v", cfg.Vars)
	}
}

func TestLoadConfigFile_GRPCTransport(t *testing.T) {
	_ = os.Setenv("GRPC_TOKEN", "grpc-secret")
	defer func() { _ = os.Unsetenv("GRPC_TOKEN") }()

	path := writeYAMLConfig(t, `
transport:
  type: grpc
  target: "localhost:50051"
  method: "/agent.v1.AgentService/StreamSession"
  discriminator: field:type
  discriminator_field: type
  plaintext: true
  descriptor_set: "testdata/agent.binpb"
  proto_file: "testdata/agent.proto"
  proto_import_path: ["testdata/proto", "testdata/vendor"]
  auth:
    type: metadata
    header_name: authorization
    token_env: GRPC_TOKEN
`)

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Transport.Type != "grpc" {
		t.Errorf("expected transport.type 'grpc', got %q", cfg.Transport.Type)
	}
	if cfg.Transport.Target != "localhost:50051" {
		t.Errorf("expected target 'localhost:50051', got %q", cfg.Transport.Target)
	}
	if cfg.Transport.GRPCMethod != "/agent.v1.AgentService/StreamSession" {
		t.Errorf("expected method to be stored, got %q", cfg.Transport.GRPCMethod)
	}
	if cfg.Transport.Discriminator != "field:type" {
		t.Errorf("expected discriminator 'field:type', got %q", cfg.Transport.Discriminator)
	}
	if cfg.Transport.DiscriminatorField != "type" {
		t.Errorf("expected discriminator_field 'type', got %q", cfg.Transport.DiscriminatorField)
	}
	if !cfg.Transport.Plaintext {
		t.Error("expected plaintext true")
	}
	if cfg.Transport.DescriptorSet != "testdata/agent.binpb" {
		t.Errorf("expected descriptor_set to be stored, got %q", cfg.Transport.DescriptorSet)
	}
	if cfg.Transport.ProtoFile != "testdata/agent.proto" {
		t.Errorf("expected proto_file to be stored, got %q", cfg.Transport.ProtoFile)
	}
	if want := []string{"testdata/proto", "testdata/vendor"}; !slices.Equal(cfg.Transport.ProtoImportPath, want) {
		t.Errorf("expected proto_import_path %v, got %v", want, cfg.Transport.ProtoImportPath)
	}
	key, value, ok := cfg.Transport.Auth.Metadata()
	if !ok {
		t.Fatal("expected Metadata() ok=true")
	}
	if key != "authorization" || value != "grpc-secret" {
		t.Errorf("expected metadata authorization=grpc-secret, got %s=%s", key, value)
	}
}

func TestLoadConfigFile_GRPCFieldTypeRequiresDiscriminatorField(t *testing.T) {
	path := writeYAMLConfig(t, `
transport:
  type: grpc
  target: "localhost:50051"
  method: "/agent.v1.AgentService/StreamSession"
  discriminator: field:type
  plaintext: true
`)

	_, err := LoadConfigFile(path)
	if err == nil {
		t.Fatal("expected error when field:type lacks discriminator_field")
	}
	if !strings.Contains(err.Error(), "discriminator_field") {
		t.Fatalf("expected discriminator_field error, got: %v", err)
	}
}

func TestLoadConfigFile_GRPCTransport_DefaultsToNoAuth(t *testing.T) {
	path := writeYAMLConfig(t, `
transport:
  type: grpc
  target: "localhost:50051"
  plaintext: true
`)

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, _, ok := cfg.Transport.Auth.Metadata(); ok {
		t.Error("expected no metadata auth when auth: block is omitted")
	}
}

func TestLoadConfigFile_GRPCTransport_PreserveFieldNamesOmittedIsNil(t *testing.T) {
	path := writeYAMLConfig(t, `
transport:
  type: grpc
  target: "localhost:50051"
  plaintext: true
`)

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Transport.PreserveFieldNames != nil {
		t.Errorf("expected PreserveFieldNames nil when preserve_field_names is omitted, got %v", *cfg.Transport.PreserveFieldNames)
	}
}

func TestLoadConfigFile_GRPCTransport_PreserveFieldNamesFalse(t *testing.T) {
	path := writeYAMLConfig(t, `
transport:
  type: grpc
  target: "localhost:50051"
  plaintext: true
  preserve_field_names: false
`)

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Transport.PreserveFieldNames == nil || *cfg.Transport.PreserveFieldNames != false {
		t.Errorf("expected PreserveFieldNames pointing to false, got %v", cfg.Transport.PreserveFieldNames)
	}
}

func TestLoadConfigFile_GRPCTransport_MetadataAuthRequiresHeaderName(t *testing.T) {
	_ = os.Setenv("GRPC_TOKEN", "grpc-secret")
	defer func() { _ = os.Unsetenv("GRPC_TOKEN") }()

	path := writeYAMLConfig(t, `
transport:
  type: grpc
  target: "localhost:50051"
  plaintext: true
  auth:
    type: metadata
    token_env: GRPC_TOKEN
`)

	_, err := LoadConfigFile(path)
	if err == nil {
		t.Fatal("expected error for metadata auth missing header_name")
	}
	if !strings.Contains(err.Error(), "header_name") {
		t.Errorf("expected error to mention header_name, got: %v", err)
	}
}

func TestLoadConfigFile_UnknownTransportType(t *testing.T) {
	path := writeYAMLConfig(t, `
transport:
  type: carrier_pigeon
`)

	_, err := LoadConfigFile(path)
	if err == nil {
		t.Fatal("expected error for unknown transport.type")
	}
	if !strings.Contains(err.Error(), "carrier_pigeon") {
		t.Errorf("expected error to name the bad type, got: %v", err)
	}
}

func TestAuthConfig_Metadata(t *testing.T) {
	tests := []struct {
		name string
		auth AuthConfig
		ok   bool
	}{
		{"metadata with key and token", AuthConfig{Type: "metadata", HeaderName: "authorization", Token: "t"}, true},
		{"metadata missing header name", AuthConfig{Type: "metadata", Token: "t"}, false},
		{"metadata missing token", AuthConfig{Type: "metadata", HeaderName: "authorization"}, false},
		{"bearer type never produces metadata", AuthConfig{Type: "bearer", Token: "t"}, false},
		{"none", AuthConfig{Type: "none"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, ok := tt.auth.Metadata()
			if ok != tt.ok {
				t.Errorf("expected ok=%v, got %v", tt.ok, ok)
			}
		})
	}
}
