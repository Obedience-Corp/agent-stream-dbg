package config

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// YAMLConfig represents the full YAML configuration structure
type YAMLConfig struct {
	Backend struct {
		BaseURL        string `yaml:"base_url"`
		StreamEndpoint struct {
			URL           string `yaml:"url"`
			Method        string `yaml:"method"`
			MessageFormat struct {
				Type         string `yaml:"type"`
				BodyTemplate string `yaml:"body_template"`
			} `yaml:"message_format"`
			Headers map[string]string `yaml:"headers"`
			Auth    struct {
				Type     string `yaml:"type"`
				TokenEnv string `yaml:"token_env"`
			} `yaml:"auth"`
		} `yaml:"stream_endpoint"`
	} `yaml:"backend"`

	Session struct {
		IDEnv         string   `yaml:"id_env"`
		AutoSetup     bool     `yaml:"auto_setup"`
		DefaultAgents []string `yaml:"default_agents"`
		SetupEndpoint string   `yaml:"setup_endpoint"`
	} `yaml:"session"`

	Events struct {
		Types []string `yaml:"types"`
	} `yaml:"events"`

	Logging struct {
		Dir        string `yaml:"dir"`
		Dimensions struct {
			ByEventType bool `yaml:"by_event_type"`
			ByAgent     bool `yaml:"by_agent"`
			BySession   bool `yaml:"by_session"`
			APICalls    bool `yaml:"api_calls"`
		} `yaml:"dimensions"`
	} `yaml:"logging"`

	Display struct {
		Colors           bool              `yaml:"colors"`
		MaxAgentsVisible int               `yaml:"max_agents_visible"`
		AgentColors      map[string]string `yaml:"agent_colors"`
	} `yaml:"display"`
}

// LoadConfigFile loads configuration from a YAML file and merges with environment
func LoadConfigFile(configPath string) (*EnhancedConfig, error) {
	// Read YAML file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML strictly: unknown keys error at load instead of being silently dropped.
	var yamlCfg YAMLConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&yamlCfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config %s: %w", configPath, err)
	}

	// Load environment variables (for secrets like API_KEY)
	_ = godotenv.Load()

	// Get API key from environment
	apiKey := os.Getenv(yamlCfg.Backend.StreamEndpoint.Auth.TokenEnv)
	if apiKey == "" {
		apiKey = os.Getenv("API_KEY") // Fallback
	}

	if apiKey == "" {
		return nil, fmt.Errorf("%s environment variable not set", yamlCfg.Backend.StreamEndpoint.Auth.TokenEnv)
	}

	// Get session ID from environment or use default
	sessionID := os.Getenv(yamlCfg.Session.IDEnv)
	if sessionID == "" {
		sessionID = getEnv("SESSION_ID", "debug-session-001")
	}

	// Build enhanced config
	cfg := &EnhancedConfig{
		Backend: BackendConfig{
			BaseURL:        yamlCfg.Backend.BaseURL,
			StreamEndpoint: yamlCfg.Backend.StreamEndpoint.URL,
			Method:         yamlCfg.Backend.StreamEndpoint.Method,
			Headers:        yamlCfg.Backend.StreamEndpoint.Headers,
		},
		Session: SessionConfig{
			ID:            sessionID,
			AutoSetup:     yamlCfg.Session.AutoSetup,
			DefaultAgents: yamlCfg.Session.DefaultAgents,
			SetupEndpoint: yamlCfg.Session.SetupEndpoint,
		},
		Display: &DisplayConfig{
			Colors:           yamlCfg.Display.Colors,
			MaxAgentsVisible: yamlCfg.Display.MaxAgentsVisible,
			AgentColors:      yamlCfg.Display.AgentColors,
		},
		APIKey:           apiKey,
		LogDir:           yamlCfg.Logging.Dir,
		EnableColors:     yamlCfg.Display.Colors,
		MaxAgentsVisible: yamlCfg.Display.MaxAgentsVisible,
	}

	// Normalize defaults for endpoints if missing
	cfg.Normalize()

	// Ensure log directories exist
	if err := cfg.createLogDirs(); err != nil {
		return nil, fmt.Errorf("failed to create log directories: %w", err)
	}

	return cfg, nil
}

// EnhancedConfig is the unified configuration structure
type EnhancedConfig struct {
	Backend BackendConfig
	Session SessionConfig
	Display *DisplayConfig
	APIKey  string
	Debug   DebugConfig

	// Logging
	LogDir string

	// Display (deprecated - use Display field instead)
	EnableColors     bool
	MaxAgentsVisible int
}

// DisplayConfig holds display configuration
type DisplayConfig struct {
	Colors           bool
	MaxAgentsVisible int
	AgentColors      map[string]string
}

// DebugConfig holds debug settings for streaming (e.g., synthesis visibility)
type DebugConfig struct {
	// Level can be "" (off), "verbose", or "full"
	Level string `yaml:"level"`
}

// BackendConfig holds backend API configuration
type BackendConfig struct {
	BaseURL        string
	StreamEndpoint string
	Method         string
	Headers        map[string]string
}

// SessionConfig holds session configuration
type SessionConfig struct {
	ID            string
	AutoSetup     bool
	DefaultAgents []string
	SetupEndpoint string
}

// StreamEndpointURL returns the full streaming endpoint URL
func (c *EnhancedConfig) StreamEndpointURL() string {
	// Replace {session_id} placeholder
	endpoint := c.Backend.StreamEndpoint
	sessionID := c.Session.ID

	// Simple string replacement for {session_id}
	endpoint = strings.Replace(endpoint, "{session_id}", sessionID, 1)

	return fmt.Sprintf("%s%s", c.Backend.BaseURL, endpoint)
}

// Normalize fills in sensible defaults for missing endpoints
func (c *EnhancedConfig) Normalize() {
	if c.Backend.BaseURL == "" {
		c.Backend.BaseURL = "http://localhost:5003"
	}
	if c.Session.SetupEndpoint == "" {
		c.Session.SetupEndpoint = "/api/v3/debug/session"
	}
	if c.Backend.StreamEndpoint == "" {
		c.Backend.StreamEndpoint = "/api/v3/sessions/{session_id}/stream"
	}
	if c.LogDir == "" {
		c.LogDir = "./logs"
	}
}

// createLogDirs ensures all log directories exist
func (c *EnhancedConfig) createLogDirs() error {
	dirs := []string{
		c.LogDir,
		fmt.Sprintf("%s/by-event-type", c.LogDir),
		fmt.Sprintf("%s/by-agent", c.LogDir),
		fmt.Sprintf("%s/by-session", c.LogDir),
		fmt.Sprintf("%s/api-calls", c.LogDir),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}
