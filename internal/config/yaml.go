package config

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"maps"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// YAMLConfig represents the full YAML configuration structure
type YAMLConfig struct {
	Transport struct {
		Type           string `yaml:"type"` // sse (only supported today; grpc/replay land in 006)
		BaseURL        string `yaml:"base_url"`
		StreamEndpoint struct {
			URL     string            `yaml:"url"`
			Method  string            `yaml:"method"`
			Headers map[string]string `yaml:"headers"`
			Auth    struct {
				Type        string `yaml:"type"`
				TokenEnv    string `yaml:"token_env"`
				HeaderName  string `yaml:"header_name"`
				UsernameEnv string `yaml:"username_env"`
				PasswordEnv string `yaml:"password_env"`
			} `yaml:"auth"`
		} `yaml:"stream_endpoint"`
	} `yaml:"transport"`

	// Dialect declares which mapping file interprets this system's frames.
	// Not yet loaded — the mapping engine lands in phase 004. Declared now so
	// run configs don't need a second migration when it does.
	Dialect struct {
		File string `yaml:"file"`
	} `yaml:"dialect"`

	// Vars are user-defined values available for interpolation in a
	// dialect's setup:/send: templates (phase 004).
	Vars map[string]string `yaml:"vars"`

	Session struct {
		ID            string   `yaml:"id"`
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

	transportType := yamlCfg.Transport.Type
	if transportType == "" {
		transportType = "sse"
	}
	if transportType != "sse" {
		return nil, fmt.Errorf("unknown transport.type %q (only %q is currently supported; grpc and replay arrive in later phases)", transportType, "sse")
	}

	// Load environment variables (for secrets like API_KEY)
	_ = godotenv.Load()

	// Resolve the token unconditionally: bearer/api_key require it, but other
	// clients (session auto-setup, config API) still read cfg.APIKey directly.
	tokenEnv := yamlCfg.Transport.StreamEndpoint.Auth.TokenEnv
	apiKey := os.Getenv(tokenEnv)
	if apiKey == "" {
		apiKey = os.Getenv("API_KEY") // Fallback
	}

	authType := yamlCfg.Transport.StreamEndpoint.Auth.Type
	if authType == "" {
		authType = "bearer"
	}

	auth := AuthConfig{
		Type:       authType,
		HeaderName: yamlCfg.Transport.StreamEndpoint.Auth.HeaderName,
		Token:      apiKey,
	}

	switch authType {
	case "bearer", "api_key":
		if apiKey == "" {
			envName := tokenEnv
			if envName == "" {
				envName = "API_KEY"
			}
			return nil, fmt.Errorf("%s environment variable not set", envName)
		}
	case "basic":
		auth.Username = os.Getenv(yamlCfg.Transport.StreamEndpoint.Auth.UsernameEnv)
		auth.Password = os.Getenv(yamlCfg.Transport.StreamEndpoint.Auth.PasswordEnv)
		if auth.Username == "" || auth.Password == "" {
			return nil, fmt.Errorf("basic auth requires %s and %s environment variables",
				yamlCfg.Transport.StreamEndpoint.Auth.UsernameEnv, yamlCfg.Transport.StreamEndpoint.Auth.PasswordEnv)
		}
	case "none":
		// No credentials required.
	default:
		return nil, fmt.Errorf("unknown auth.type %q (must be bearer, api_key, basic, or none)", authType)
	}

	// Get session ID: static session.id, overridden by session.id_env if set.
	sessionID := yamlCfg.Session.ID
	if yamlCfg.Session.IDEnv != "" {
		if v := os.Getenv(yamlCfg.Session.IDEnv); v != "" {
			sessionID = v
		}
	}
	if sessionID == "" {
		sessionID = getEnv("SESSION_ID", "debug-session-001")
	}

	// Build enhanced config
	cfg := &EnhancedConfig{
		Transport: TransportConfig{
			BaseURL:        yamlCfg.Transport.BaseURL,
			StreamEndpoint: yamlCfg.Transport.StreamEndpoint.URL,
			Method:         yamlCfg.Transport.StreamEndpoint.Method,
			Headers:        yamlCfg.Transport.StreamEndpoint.Headers,
			Auth:           auth,
		},
		Dialect: DialectConfig{File: yamlCfg.Dialect.File},
		Vars:    yamlCfg.Vars,
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
		Events: EventsConfig{Types: yamlCfg.Events.Types},
		Logging: LoggingConfig{
			Dimensions: DimensionsConfig{
				ByEventType: yamlCfg.Logging.Dimensions.ByEventType,
				ByAgent:     yamlCfg.Logging.Dimensions.ByAgent,
				BySession:   yamlCfg.Logging.Dimensions.BySession,
				APICalls:    yamlCfg.Logging.Dimensions.APICalls,
			},
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
	Transport TransportConfig
	Dialect   DialectConfig
	Vars      map[string]string
	Session   SessionConfig
	Display   *DisplayConfig
	Events    EventsConfig
	Logging   LoggingConfig
	APIKey    string
	Debug     DebugConfig

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

// TransportConfig holds transport-level connection configuration.
type TransportConfig struct {
	BaseURL        string
	StreamEndpoint string
	Method         string
	Headers        map[string]string
	Auth           AuthConfig
}

// ResolvedHeaders returns transport.headers merged with the resolved
// authentication header (if any), ready to attach to an outgoing request.
func (t TransportConfig) ResolvedHeaders() map[string]string {
	h := make(map[string]string, len(t.Headers)+1)
	maps.Copy(h, t.Headers)
	if name, value, ok := t.Auth.Header(); ok {
		h[name] = value
	}
	return h
}

// DialectConfig declares which mapping file interprets this system's
// frames. Not yet loaded by this phase — see YAMLConfig.Dialect.
type DialectConfig struct {
	File string
}

// AuthConfig holds resolved authentication settings for backend requests.
type AuthConfig struct {
	Type       string // bearer, api_key, basic, or none
	HeaderName string // custom header name for api_key (default X-API-Key)
	Token      string // resolved token for bearer/api_key
	Username   string // resolved username for basic
	Password   string // resolved password for basic
}

// Header returns the auth header name/value to set, or ok=false if no
// auth header should be attached (type "none", or credentials unresolved).
func (a AuthConfig) Header() (name, value string, ok bool) {
	switch a.Type {
	case "", "bearer":
		if a.Token == "" {
			return "", "", false
		}
		return "Authorization", "Bearer " + a.Token, true
	case "api_key":
		if a.Token == "" {
			return "", "", false
		}
		headerName := a.HeaderName
		if headerName == "" {
			headerName = "X-API-Key"
		}
		return headerName, a.Token, true
	case "basic":
		if a.Username == "" && a.Password == "" {
			return "", "", false
		}
		cred := base64.StdEncoding.EncodeToString([]byte(a.Username + ":" + a.Password))
		return "Authorization", "Basic " + cred, true
	default: // "none" or unrecognized
		return "", "", false
	}
}

// LoggingConfig holds logging behavior settings.
type LoggingConfig struct {
	Dimensions DimensionsConfig
}

// DimensionsConfig controls which log dimensions are written. When all four
// are false (the zero value — nothing specified in YAML), Normalize enables
// all of them, preserving the tool's always-log-everything default.
type DimensionsConfig struct {
	ByEventType bool
	ByAgent     bool
	BySession   bool
	APICalls    bool
}

// SessionConfig holds session configuration
type SessionConfig struct {
	ID            string
	AutoSetup     bool
	DefaultAgents []string
	SetupEndpoint string
}

// EventsConfig holds which SSE event types the client subscribes to.
// An empty Types means "track all events" (the client's built-in default set).
type EventsConfig struct {
	Types []string
}

// StreamEndpointURL returns the full streaming endpoint URL
func (c *EnhancedConfig) StreamEndpointURL() string {
	// Replace {session_id} placeholder
	endpoint := c.Transport.StreamEndpoint
	sessionID := c.Session.ID

	// Simple string replacement for {session_id}
	endpoint = strings.Replace(endpoint, "{session_id}", sessionID, 1)

	return fmt.Sprintf("%s%s", c.Transport.BaseURL, endpoint)
}

// Normalize fills in generic (non-system-specific) defaults. It no longer
// injects Brainyard's base URL or endpoint paths — those belong in a run
// config (or, from phase 004, the dialect's setup:/send: templates), not in
// library code that claims to be system-agnostic.
func (c *EnhancedConfig) Normalize() {
	if c.LogDir == "" {
		c.LogDir = "./logs"
	}
	d := &c.Logging.Dimensions
	if !d.ByEventType && !d.ByAgent && !d.BySession && !d.APICalls {
		d.ByEventType, d.ByAgent, d.BySession, d.APICalls = true, true, true, true
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
