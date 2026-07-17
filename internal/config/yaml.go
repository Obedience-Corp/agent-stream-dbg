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

// authYAML is the raw YAML shape of an auth: block, shared verbatim
// between transport.stream_endpoint.auth (SSE) and transport.auth (gRPC)
// — one auth vocabulary for both transports, per architecture.md's gRPC
// config example.
type authYAML struct {
	Type        string `yaml:"type"` // bearer | api_key | basic | metadata | none
	TokenEnv    string `yaml:"token_env"`
	HeaderName  string `yaml:"header_name"` // HTTP header name (SSE) or metadata key (gRPC)
	UsernameEnv string `yaml:"username_env"`
	PasswordEnv string `yaml:"password_env"`
}

// YAMLConfig represents the full YAML configuration structure
type YAMLConfig struct {
	Transport struct {
		Type           string `yaml:"type"` // sse | grpc (replay lands as a transport, not a config type, in phase 004)
		BaseURL        string `yaml:"base_url"`
		StreamEndpoint struct {
			URL     string            `yaml:"url"`
			Method  string            `yaml:"method"`
			Headers map[string]string `yaml:"headers"`
			Auth    authYAML          `yaml:"auth"`
		} `yaml:"stream_endpoint"`

		// gRPC-specific (type: grpc). Flat, matching architecture.md's
		// example shape (transport.target/method/discriminator/plaintext),
		// not nested like SSE's stream_endpoint — gRPC has no equivalent
		// concept to nest under.
		Target        string `yaml:"target"`
		Method        string `yaml:"method"`        // fully-qualified RPC method, e.g. /agent.v1.AgentService/StreamSession
		Discriminator string `yaml:"discriminator"` // oneof | field:type | message_type | none — resolved in sequence 03
		// DiscriminatorField is the proto field name used when
		// discriminator is field:type (e.g. "type"). Required for that
		// mode; ignored otherwise.
		DiscriminatorField string `yaml:"discriminator_field"`
		// PreserveFieldNames mirrors grpc.Config.PreserveFieldNames: nil
		// (the key omitted, the common case) means "unset" and resolves to
		// true — today's snake_case rendering — not Go's bool zero value,
		// so a run config that predates this field can't be silently
		// flipped to camelCase. See grpc.Config.PreserveFieldNames's doc
		// comment for the full reasoning.
		PreserveFieldNames *bool    `yaml:"preserve_field_names"`
		Plaintext          bool     `yaml:"plaintext"`
		DescriptorSet      string   `yaml:"descriptor_set"`    // path to a compiled FileDescriptorSet — fallback when the target has reflection disabled
		ProtoFile          string   `yaml:"proto_file"`        // path to a raw .proto — lowest-priority fallback, compiled at runtime
		ProtoImportPath    []string `yaml:"proto_import_path"` // import roots for ProtoFile and its own imports, mirroring protoc's -I/--proto_path
		Auth               authYAML `yaml:"auth"`
	} `yaml:"transport"`

	// Dialect declares which mapping file or embedded dialect name interprets
	// this system's frames.
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
	if transportType != "sse" && transportType != "grpc" && transportType != "replay" {
		return nil, fmt.Errorf("unknown transport.type %q (must be \"sse\", \"grpc\", or \"replay\")", transportType)
	}

	// Load environment variables (for secrets like API_KEY)
	_ = godotenv.Load()

	var transport TransportConfig
	var apiKey string
	switch transportType {
	case "grpc":
		auth, resolvedToken, err := resolveAuth(yamlCfg.Transport.Auth, "none")
		if err != nil {
			return nil, err
		}
		if yamlCfg.Transport.Discriminator == "field:type" && yamlCfg.Transport.DiscriminatorField == "" {
			return nil, fmt.Errorf("transport.discriminator_field is required when transport.discriminator is \"field:type\"")
		}
		apiKey = resolvedToken
		transport = TransportConfig{
			Type:               "grpc",
			Target:             yamlCfg.Transport.Target,
			GRPCMethod:         yamlCfg.Transport.Method,
			Discriminator:      yamlCfg.Transport.Discriminator,
			DiscriminatorField: yamlCfg.Transport.DiscriminatorField,
			PreserveFieldNames: yamlCfg.Transport.PreserveFieldNames,
			Plaintext:          yamlCfg.Transport.Plaintext,
			DescriptorSet:      yamlCfg.Transport.DescriptorSet,
			ProtoFile:          yamlCfg.Transport.ProtoFile,
			ProtoImportPath:    yamlCfg.Transport.ProtoImportPath,
			Auth:               auth,
		}
	case "replay":
		auth, resolvedToken, err := resolveAuth(yamlCfg.Transport.Auth, "none")
		if err != nil {
			return nil, err
		}
		apiKey = resolvedToken
		transport = TransportConfig{
			Type:           "replay",
			BaseURL:        yamlCfg.Transport.BaseURL,
			StreamEndpoint: yamlCfg.Transport.StreamEndpoint.URL,
			Auth:           auth,
		}
	default: // sse
		auth, resolvedToken, err := resolveAuth(yamlCfg.Transport.StreamEndpoint.Auth, "none")
		if err != nil {
			return nil, err
		}
		apiKey = resolvedToken
		transport = TransportConfig{
			Type:           "sse",
			BaseURL:        yamlCfg.Transport.BaseURL,
			StreamEndpoint: yamlCfg.Transport.StreamEndpoint.URL,
			Method:         yamlCfg.Transport.StreamEndpoint.Method,
			Headers:        yamlCfg.Transport.StreamEndpoint.Headers,
			Auth:           auth,
		}
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
		Transport: transport,
		Dialect:   DialectConfig{File: yamlCfg.Dialect.File},
		Vars:      yamlCfg.Vars,
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

// resolveAuth turns a raw auth: YAML block into a resolved AuthConfig,
// reading only the explicitly named *_env credential from the environment.
// defaultType is used when the block omits type: — all product transports
// pass "none", so credentials require an explicit auth.type opt-in. Returns the resolved
// token separately since callers outside the transport itself (session
// auto-setup) still read EnhancedConfig.APIKey directly, regardless of
// which auth type the primary transport ends up using.
func resolveAuth(y authYAML, defaultType string) (AuthConfig, string, error) {
	tokenEnv := y.TokenEnv
	var apiKey string
	if tokenEnv != "" {
		apiKey = os.Getenv(tokenEnv)
	}

	authType := y.Type
	if authType == "" {
		authType = defaultType
	}

	auth := AuthConfig{
		Type:       authType,
		HeaderName: y.HeaderName,
		Token:      apiKey,
	}

	switch authType {
	case "bearer", "api_key", "metadata":
		if apiKey == "" {
			if tokenEnv == "" {
				return AuthConfig{}, "", fmt.Errorf("auth.type %q requires token_env", authType)
			}
			return AuthConfig{}, "", fmt.Errorf("%s environment variable not set", tokenEnv)
		}
		if authType == "metadata" && auth.HeaderName == "" {
			return AuthConfig{}, "", fmt.Errorf("auth.type \"metadata\" requires header_name (the metadata key to attach)")
		}
	case "basic":
		auth.Username = os.Getenv(y.UsernameEnv)
		auth.Password = os.Getenv(y.PasswordEnv)
		if auth.Username == "" || auth.Password == "" {
			return AuthConfig{}, "", fmt.Errorf("basic auth requires %s and %s environment variables",
				y.UsernameEnv, y.PasswordEnv)
		}
	case "none":
		// No credentials required.
	default:
		return AuthConfig{}, "", fmt.Errorf("unknown auth.type %q (must be bearer, api_key, basic, metadata, or none)", authType)
	}

	return auth, apiKey, nil
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

// TransportConfig holds transport-level connection configuration for
// either transport type; which fields apply is determined by Type.
type TransportConfig struct {
	Type string // "sse" (default), "grpc", or "replay"

	// SSE fields.
	BaseURL        string
	StreamEndpoint string
	Method         string
	Headers        map[string]string

	// gRPC fields.
	Target             string
	GRPCMethod         string // fully-qualified RPC method, e.g. /agent.v1.AgentService/StreamSession
	Discriminator      string // oneof | field:type | message_type | none
	DiscriminatorField string // proto field name for discriminator field:type
	// PreserveFieldNames: nil (unset) = true = snake_case field names
	// (today's default for every existing dialect); false = lowerCamelCase,
	// for a dialect whose real wire protocol mandates it. See
	// grpc.Config.PreserveFieldNames.
	PreserveFieldNames *bool
	Plaintext          bool
	DescriptorSet      string   // path to a compiled FileDescriptorSet — fallback when reflection is disabled
	ProtoFile          string   // path to a raw .proto — lowest-priority fallback, compiled at runtime
	ProtoImportPath    []string // import roots for ProtoFile and its own imports

	// Shared: one auth vocabulary for both transports.
	Auth AuthConfig
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

// DialectConfig declares which mapping file or embedded dialect name
// interprets this system's frames.
type DialectConfig struct {
	File string
}

// AuthConfig holds resolved authentication settings for backend requests.
type AuthConfig struct {
	Type       string // bearer, api_key, basic, metadata, or none
	HeaderName string // custom header name for api_key (default X-API-Key), or the metadata key for type "metadata"
	Token      string // resolved token for bearer/api_key/metadata
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
	default: // "none", "metadata", or unrecognized
		return "", "", false
	}
}

// Metadata returns the gRPC metadata key/value to attach, or ok=false if
// no metadata should be attached (any type other than "metadata", or
// credentials unresolved). Distinct from Header: gRPC auth rides
// per-RPC metadata, not an HTTP header, and "metadata" is the one auth
// type SSE's Header never handles.
func (a AuthConfig) Metadata() (key, value string, ok bool) {
	if a.Type != "metadata" || a.Token == "" || a.HeaderName == "" {
		return "", "", false
	}
	return a.HeaderName, a.Token, true
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

// Normalize fills in generic (non-system-specific) defaults. It never
// injects a backend's base URL or endpoint paths — those belong in a run
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
