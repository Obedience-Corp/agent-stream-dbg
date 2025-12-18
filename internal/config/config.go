package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the stream debugger
type Config struct {
	// Backend API Configuration
	BackendURL string
	APIKey     string

	// Session Configuration
	SessionID string

	// Logging Configuration
	LogDir   string
	LogLevel string

	// Display Configuration
	EnableColors     bool
	MaxAgentsVisible int
	// Debug level passed to backend stream endpoint: "", "verbose", or "full"
	DebugLevel string
}

// Load reads configuration from environment and .env file
func Load() (*Config, error) {
	// Load .env file if it exists (optional)
	_ = godotenv.Load()

	cfg := &Config{
		BackendURL:       getEnv("BACKEND_URL", "http://localhost:5003"),
		APIKey:           getEnv("API_KEY", ""),
		SessionID:        getEnv("SESSION_ID", "test-session-001"),
		LogDir:           getEnv("LOG_DIR", "./logs"),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		EnableColors:     getEnvBool("ENABLE_COLORS", true),
		MaxAgentsVisible: getEnvInt("MAX_AGENTS_VISIBLE", 5),
		DebugLevel:       getEnv("STREAM_DEBUG_LEVEL", ""),
	}

	// Validate required fields
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API_KEY is required but not set in environment or .env file")
	}

	// Ensure log directories exist
	if err := cfg.createLogDirs(); err != nil {
		return nil, fmt.Errorf("failed to create log directories: %w", err)
	}

	return cfg, nil
}

// createLogDirs ensures all log directories exist
func (c *Config) createLogDirs() error {
	dirs := []string{
		filepath.Join(c.LogDir),
		filepath.Join(c.LogDir, "by-event-type"),
		filepath.Join(c.LogDir, "by-agent"),
		filepath.Join(c.LogDir, "by-session"),
		filepath.Join(c.LogDir, "api-calls"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// StreamEndpoint returns the full URL for the streaming endpoint
func (c *Config) StreamEndpoint() string {
	return fmt.Sprintf("%s/api/v3/sessions/%s/stream", c.BackendURL, c.SessionID)
}

// getEnv retrieves an environment variable with a default fallback
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvBool retrieves a boolean environment variable with a default fallback
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		b, err := strconv.ParseBool(value)
		if err == nil {
			return b
		}
	}
	return defaultValue
}

// getEnvInt retrieves an integer environment variable with a default fallback
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		i, err := strconv.Atoi(value)
		if err == nil {
			return i
		}
	}
	return defaultValue
}
