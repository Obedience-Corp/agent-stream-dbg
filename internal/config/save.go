package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SaveConfigFile updates the editable runtime settings in an existing YAML
// config while preserving unrelated keys such as auth environment mappings.
// Resolved secrets from EnhancedConfig are never serialized.
func SaveConfigFile(path string, cfg *EnhancedConfig) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("config path is empty")
	}
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse config file: %w", err)
	}
	if root == nil {
		root = make(map[string]any)
	}

	transport := ensureMap(root, "transport")
	transport["type"] = cfg.Transport.Type
	transport["base_url"] = cfg.Transport.BaseURL
	transport["target"] = cfg.Transport.Target
	transport["discriminator"] = cfg.Transport.Discriminator
	transport["discriminator_field"] = cfg.Transport.DiscriminatorField

	streamEndpoint := ensureMap(transport, "stream_endpoint")
	streamEndpoint["url"] = cfg.Transport.StreamEndpoint
	if cfg.Transport.Auth.TokenEnv != "" {
		authParent := streamEndpoint
		if cfg.Transport.Type == "grpc" || cfg.Transport.Type == "replay" {
			authParent = transport
		}
		auth := ensureMap(authParent, "auth")
		auth["type"] = cfg.Transport.Auth.Type
		auth["token_env"] = cfg.Transport.Auth.TokenEnv
	} else if cfg.Transport.Type == "sse" && cfg.Transport.Auth.Type == "none" {
		auth := ensureMap(streamEndpoint, "auth")
		auth["type"] = cfg.Transport.Auth.Type
		delete(auth, "token_env")
	}

	dialect := ensureMap(root, "dialect")
	dialect["file"] = cfg.Dialect.File
	session := ensureMap(root, "session")
	session["auto_setup"] = cfg.Session.AutoSetup
	session["default_agents"] = copyStrings(cfg.Session.DefaultAgents)
	root["vars"] = copyVars(cfg.Vars)

	updated, err := yaml.Marshal(root)
	if err != nil {
		return fmt.Errorf("marshal config file: %w", err)
	}

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".stream-debugger-config-*.yaml")
	if err != nil {
		return fmt.Errorf("create temporary config file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set config file permissions: %w", err)
	}
	if _, err := tmp.Write(bytes.TrimSpace(updated)); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary config file: %w", err)
	}
	if _, err := tmp.Write([]byte("\n")); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("finish temporary config file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary config file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace config file: %w", err)
	}
	return nil
}

func ensureMap(parent map[string]any, key string) map[string]any {
	if value, ok := parent[key].(map[string]any); ok {
		return value
	}
	value := make(map[string]any)
	parent[key] = value
	return value
}

func copyVars(vars map[string]string) map[string]string {
	if len(vars) == 0 {
		return map[string]string{}
	}
	copy := make(map[string]string, len(vars))
	for key, value := range vars {
		copy[key] = value
	}
	return copy
}

func copyStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}
