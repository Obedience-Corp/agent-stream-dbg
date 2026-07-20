package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// EntryStatus describes whether a listed run config is usable.
type EntryStatus string

const (
	EntryReady      EntryStatus = "ready"
	EntryNeedsSetup EntryStatus = "needs setup"
	EntryError      EntryStatus = "error"
)

// ConfigEntry is one run-config file discovered for the launch home hub.
type ConfigEntry struct {
	Name      string
	Path      string
	Transport string
	Dialect   string
	Status    EntryStatus
	Source    string // "cwd" or "user"
	Err       string // set when Status == EntryError
}

// interactiveConfigNames are the well-known basenames scanned in the cwd.
var interactiveConfigNames = []string{
	"stream-debugger.yaml",
	"stream-debugger.yml",
	"config.yaml",
	"config.yml",
}

// ListConfigs returns run configs from the working directory and the user
// config directory. It never creates files.
func ListConfigs(cwd, userConfigDir string) []ConfigEntry {
	seen := make(map[string]struct{})
	var out []ConfigEntry

	add := func(path, source string) {
		if path == "" {
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		if _, ok := seen[abs]; ok {
			return
		}
		seen[abs] = struct{}{}
		out = append(out, inspectConfig(path, source))
	}

	if strings.TrimSpace(cwd) != "" {
		for _, name := range interactiveConfigNames {
			candidate := filepath.Join(cwd, name)
			if fileExists(candidate) {
				add(candidate, "cwd")
			}
		}
		// Optional project convention: configs/*.yaml
		configsDir := filepath.Join(cwd, "configs")
		if entries, err := os.ReadDir(configsDir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !isYAMLName(e.Name()) {
					continue
				}
				add(filepath.Join(configsDir, e.Name()), "cwd")
			}
		}
	}

	if strings.TrimSpace(userConfigDir) != "" {
		if entries, err := os.ReadDir(userConfigDir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !isYAMLName(e.Name()) {
					continue
				}
				add(filepath.Join(userConfigDir, e.Name()), "user")
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		// ready first, then needs setup, then error; name within group
		ri, rj := statusRank(out[i].Status), statusRank(out[j].Status)
		if ri != rj {
			return ri < rj
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func statusRank(s EntryStatus) int {
	switch s {
	case EntryReady:
		return 0
	case EntryNeedsSetup:
		return 1
	default:
		return 2
	}
}

func isYAMLName(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func inspectConfig(path, source string) ConfigEntry {
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))

	entry := ConfigEntry{
		Name:   name,
		Path:   path,
		Source: source,
	}

	cfg, err := LoadConfigFile(path)
	if err != nil {
		entry.Status = EntryError
		entry.Err = err.Error()
		entry.Transport = "?"
		entry.Dialect = "—"
		return entry
	}

	entry.Transport = cfg.Transport.Type
	if entry.Transport == "" {
		entry.Transport = "sse"
	}
	entry.Dialect = cfg.Dialect.File
	if entry.Dialect == "" {
		entry.Dialect = "—"
	}
	if ConfigReady(cfg) {
		entry.Status = EntryReady
	} else {
		entry.Status = EntryNeedsSetup
	}
	return entry
}

// ConfigReady reports whether cfg has the minimum fields to open a session.
func ConfigReady(cfg *EnhancedConfig) bool {
	if cfg == nil {
		return false
	}
	if strings.TrimSpace(cfg.Dialect.File) == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Transport.Type)) {
	case "grpc":
		return strings.TrimSpace(cfg.Transport.Target) != "" && strings.TrimSpace(cfg.Transport.GRPCMethod) != ""
	case "acp":
		return strings.TrimSpace(cfg.Transport.Command) != ""
	case "replay":
		return strings.TrimSpace(cfg.Transport.BaseURL) != ""
	default: // sse
		return strings.TrimSpace(cfg.Transport.BaseURL) != "" && strings.TrimSpace(cfg.Transport.StreamEndpoint) != ""
	}
}

// UserConfigDir returns ~/.config/stream-debugger (or platform equivalent).
func UserConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(base, "stream-debugger"), nil
}

// WriteNewConfig writes a new private run-config file. path must not already exist.
func WriteNewConfig(path, contents string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("config already exists: %s", path)
		}
		return fmt.Errorf("create config: %w", err)
	}
	if _, err := file.WriteString(contents); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write config: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close config: %w", err)
	}
	return nil
}

// SanitizeConfigName turns a display name into a safe YAML basename (no extension).
func SanitizeConfigName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	out := b.String()
	if out == "" {
		return "config"
	}
	return out
}
