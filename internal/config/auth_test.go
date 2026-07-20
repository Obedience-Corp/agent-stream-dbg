package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadConfigFile_AuthErrors(t *testing.T) {
	tests := []struct {
		name           string
		yaml           string
		env            map[string]string
		wantLoadErrSub string // empty = load must succeed
		wantAuthErrSub string // non-empty = ValidateAuth must mention this
	}{
		{
			name: "bearer without token loads but ValidateAuth fails",
			yaml: `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "bearer"
      token_env: "MISSING_TOKEN"
dialect:
  file: openai
`,
			wantAuthErrSub: ".env",
		},
		{
			name: "basic without username/password loads but ValidateAuth fails",
			yaml: `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "basic"
      username_env: "BASIC_USER"
      password_env: "BASIC_PASS"
dialect:
  file: openai
`,
			wantAuthErrSub: ".env",
		},
		{
			name: "unknown auth type",
			yaml: `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "oauth2"
`,
			wantLoadErrSub: "unknown auth.type",
		},
		{
			name: "unknown transport type",
			yaml: `
transport:
  type: "carrier_pigeon"
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "none"
`,
			wantLoadErrSub: "unknown transport.type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				_ = os.Setenv(k, v)
			}
			defer func() {
				for k := range tt.env {
					_ = os.Unsetenv(k)
				}
			}()

			path := writeYAMLConfig(t, tt.yaml)
			cfg, err := LoadConfigFile(path)
			if tt.wantLoadErrSub != "" {
				if err == nil {
					t.Fatalf("expected load error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantLoadErrSub) {
					t.Errorf("expected load error containing %q, got: %v", tt.wantLoadErrSub, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfigFile: %v", err)
			}
			authErr := cfg.ValidateAuth()
			if tt.wantAuthErrSub == "" {
				if authErr != nil {
					t.Fatalf("ValidateAuth: %v", authErr)
				}
				return
			}
			if authErr == nil {
				t.Fatal("expected ValidateAuth error, got nil")
			}
			if !strings.Contains(authErr.Error(), tt.wantAuthErrSub) {
				t.Errorf("expected ValidateAuth containing %q, got: %v", tt.wantAuthErrSub, authErr)
			}
		})
	}
}

func TestLoadConfigFile_AuthStrategies(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		env        map[string]string
		wantHeader string
		wantValue  string
	}{
		{
			name: "bearer (explicit opt-in)",
			yaml: `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "bearer"
      token_env: "API_KEY"
`,
			env:        map[string]string{"API_KEY": "tok-123"},
			wantHeader: "Authorization",
			wantValue:  "Bearer tok-123",
		},
		{
			name: "api_key with custom header name",
			yaml: `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "api_key"
      header_name: "X-API-Key"
      token_env: "API_KEY"
`,
			env:        map[string]string{"API_KEY": "tok-456"},
			wantHeader: "X-API-Key",
			wantValue:  "tok-456",
		},
		{
			name: "api_key defaults header name to X-API-Key",
			yaml: `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "api_key"
      token_env: "API_KEY"
`,
			env:        map[string]string{"API_KEY": "tok-789"},
			wantHeader: "X-API-Key",
			wantValue:  "tok-789",
		},
		{
			name: "basic auth",
			yaml: `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "basic"
      username_env: "BASIC_USER"
      password_env: "BASIC_PASS"
`,
			env:        map[string]string{"BASIC_USER": "alice", "BASIC_PASS": "secret"},
			wantHeader: "Authorization",
			wantValue:  "Basic YWxpY2U6c2VjcmV0", // base64("alice:secret")
		},
		{
			name: "none sets no auth header",
			yaml: `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "none"
`,
			wantHeader: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				_ = os.Setenv(k, v)
			}
			defer func() {
				for k := range tt.env {
					_ = os.Unsetenv(k)
				}
			}()

			path := writeYAMLConfig(t, tt.yaml)
			cfg, err := LoadConfigFile(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			name, value, ok := cfg.Transport.Auth.Header()
			if tt.wantHeader == "" {
				if ok {
					t.Errorf("expected no auth header, got %s=%s", name, value)
				}
				return
			}
			if !ok {
				t.Fatalf("expected auth header %s, got none", tt.wantHeader)
			}
			if name != tt.wantHeader {
				t.Errorf("expected header name %q, got %q", tt.wantHeader, name)
			}
			if value != tt.wantValue {
				t.Errorf("expected header value %q, got %q", tt.wantValue, value)
			}
		})
	}
}

func TestLoadConfigFile_BackendHeadersImplemented(t *testing.T) {
	_ = os.Setenv("API_KEY", "tok")
	defer func() { _ = os.Unsetenv("API_KEY") }()

	path := writeYAMLConfig(t, `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    headers:
      X-Custom-Header: "custom-value"
    auth:
      type: "bearer"
      token_env: "API_KEY"
`)

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	headers := cfg.Transport.ResolvedHeaders()
	if headers["X-Custom-Header"] != "custom-value" {
		t.Errorf("expected custom header to be present, got: %v", headers)
	}
	if headers["Authorization"] != "Bearer tok" {
		t.Errorf("expected auth header alongside custom header, got: %v", headers)
	}
}

func TestLoadConfigFile_LoggingDimensions(t *testing.T) {
	_ = os.Setenv("API_KEY", "tok")
	defer func() { _ = os.Unsetenv("API_KEY") }()

	t.Run("unspecified defaults all dimensions on", func(t *testing.T) {
		path := writeYAMLConfig(t, `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "bearer"
      token_env: "API_KEY"
`)
		cfg, err := LoadConfigFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		d := cfg.Logging.Dimensions
		if !d.ByEventType || !d.ByAgent || !d.BySession || !d.APICalls {
			t.Errorf("expected all dimensions on by default, got %+v", d)
		}
	})

	t.Run("explicit values respected", func(t *testing.T) {
		path := writeYAMLConfig(t, `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "bearer"
      token_env: "API_KEY"
logging:
  dimensions:
    by_event_type: true
    by_agent: false
    by_session: false
    api_calls: false
`)
		cfg, err := LoadConfigFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		d := cfg.Logging.Dimensions
		if !d.ByEventType || d.ByAgent || d.BySession || d.APICalls {
			t.Errorf("expected only by_event_type on, got %+v", d)
		}
	})
}

func TestLoadConfigFile_SessionIDDirect(t *testing.T) {
	_ = os.Setenv("API_KEY", "tok")
	defer func() { _ = os.Unsetenv("API_KEY") }()

	t.Run("session.id used directly", func(t *testing.T) {
		path := writeYAMLConfig(t, `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "bearer"
      token_env: "API_KEY"
session:
  id: "static-session-id"
`)
		cfg, err := LoadConfigFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Session.ID != "static-session-id" {
			t.Errorf("expected session.id to be used directly, got %q", cfg.Session.ID)
		}
	})

	t.Run("id_env overrides static id when set", func(t *testing.T) {
		_ = os.Setenv("MY_SESSION_ID", "env-session-id")
		defer func() { _ = os.Unsetenv("MY_SESSION_ID") }()

		path := writeYAMLConfig(t, `
transport:
  base_url: "http://localhost:8080"
  stream_endpoint:
    url: "/api/stream"
    auth:
      type: "bearer"
      token_env: "API_KEY"
session:
  id: "static-session-id"
  id_env: "MY_SESSION_ID"
`)
		cfg, err := LoadConfigFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Session.ID != "env-session-id" {
			t.Errorf("expected id_env to override static id, got %q", cfg.Session.ID)
		}
	})
}
