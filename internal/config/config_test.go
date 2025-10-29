package config

import (
	"os"
	"testing"
)

func TestLoad_WithDefaults(t *testing.T) {
	// Set required env var
	os.Setenv("API_KEY", "test-key")
	defer os.Unsetenv("API_KEY")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check defaults
	if cfg.BackendURL != "http://localhost:5003" {
		t.Errorf("Expected default backend URL, got %s", cfg.BackendURL)
	}

	if cfg.APIKey != "test-key" {
		t.Errorf("Expected API key 'test-key', got '%s'", cfg.APIKey)
	}

	if cfg.LogLevel != "info" {
		t.Errorf("Expected default log level 'info', got '%s'", cfg.LogLevel)
	}

	if !cfg.EnableColors {
		t.Error("Expected colors enabled by default")
	}

	if cfg.MaxAgentsVisible != 5 {
		t.Errorf("Expected max agents 5, got %d", cfg.MaxAgentsVisible)
	}
}

func TestLoad_MissingAPIKey(t *testing.T) {
	os.Unsetenv("API_KEY")

	_, err := Load()
	if err == nil {
		t.Fatal("Expected error for missing API_KEY, got nil")
	}
}

func TestLoad_CustomValues(t *testing.T) {
	os.Setenv("API_KEY", "custom-key")
	os.Setenv("BACKEND_URL", "http://custom:8080")
	os.Setenv("SESSION_ID", "custom-session")
	os.Setenv("LOG_DIR", "/tmp/logs")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("ENABLE_COLORS", "false")
	os.Setenv("MAX_AGENTS_VISIBLE", "10")

	defer func() {
		os.Unsetenv("API_KEY")
		os.Unsetenv("BACKEND_URL")
		os.Unsetenv("SESSION_ID")
		os.Unsetenv("LOG_DIR")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("ENABLE_COLORS")
		os.Unsetenv("MAX_AGENTS_VISIBLE")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.BackendURL != "http://custom:8080" {
		t.Errorf("Expected custom backend URL, got %s", cfg.BackendURL)
	}

	if cfg.SessionID != "custom-session" {
		t.Errorf("Expected custom session ID, got %s", cfg.SessionID)
	}

	if cfg.LogDir != "/tmp/logs" {
		t.Errorf("Expected custom log dir, got %s", cfg.LogDir)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("Expected log level 'debug', got '%s'", cfg.LogLevel)
	}

	if cfg.EnableColors {
		t.Error("Expected colors disabled")
	}

	if cfg.MaxAgentsVisible != 10 {
		t.Errorf("Expected max agents 10, got %d", cfg.MaxAgentsVisible)
	}
}

func TestStreamEndpoint(t *testing.T) {
	os.Setenv("API_KEY", "test-key")
	defer os.Unsetenv("API_KEY")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	expected := "http://localhost:5003/api/v3/sessions/test-session-001/stream"
	endpoint := cfg.StreamEndpoint()

	if endpoint != expected {
		t.Errorf("Expected endpoint %s, got %s", expected, endpoint)
	}
}

func TestGetEnv(t *testing.T) {
	os.Setenv("TEST_VAR", "test-value")
	defer os.Unsetenv("TEST_VAR")

	value := getEnv("TEST_VAR", "default")
	if value != "test-value" {
		t.Errorf("Expected 'test-value', got '%s'", value)
	}

	value = getEnv("NON_EXISTENT", "default")
	if value != "default" {
		t.Errorf("Expected 'default', got '%s'", value)
	}
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"1", true},
		{"false", false},
		{"False", false},
		{"FALSE", false},
		{"0", false},
		{"invalid", true}, // Falls back to default
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			os.Setenv("TEST_BOOL", tt.value)
			defer os.Unsetenv("TEST_BOOL")

			result := getEnvBool("TEST_BOOL", true)
			if result != tt.expected {
				t.Errorf("For value '%s', expected %v, got %v", tt.value, tt.expected, result)
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		value    string
		expected int
	}{
		{"42", 42},
		{"0", 0},
		{"-5", -5},
		{"invalid", 10}, // Falls back to default
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			os.Setenv("TEST_INT", tt.value)
			defer os.Unsetenv("TEST_INT")

			result := getEnvInt("TEST_INT", 10)
			if result != tt.expected {
				t.Errorf("For value '%s', expected %d, got %d", tt.value, tt.expected, result)
			}
		})
	}
}
