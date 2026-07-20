package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/config"
)

// resolveExplicitConfig validates an explicit --config path. It never creates files.
func resolveExplicitConfig(explicit string) (string, error) {
	if strings.TrimSpace(explicit) == "" {
		return "", fmt.Errorf("config path is empty")
	}
	if _, err := os.Stat(explicit); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("config file %q not found; run agent-stream-dbg without --config to open the home hub", explicit)
		}
		return "", fmt.Errorf("check config file %q: %w", explicit, err)
	}
	return explicit, nil
}

// resolveInteractiveConfig is kept for tests that still expect first-match
// discovery without auto-create. Prefer the home hub for bare launch.
func resolveInteractiveConfig(explicit string) (path string, created bool, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false, fmt.Errorf("find current directory: %w", err)
	}
	userConfigDir, err := config.UserConfigDir()
	if err != nil {
		return "", false, fmt.Errorf("find user config directory: %w", err)
	}
	return resolveInteractiveConfigAt(explicit, cwd, userConfigDir)
}

func resolveInteractiveConfigAt(explicit, cwd, userConfigDir string) (path string, created bool, err error) {
	if strings.TrimSpace(explicit) != "" {
		p, err := resolveExplicitConfig(explicit)
		return p, false, err
	}

	entries := config.ListConfigs(cwd, userConfigDir)
	if len(entries) == 0 {
		return "", false, fmt.Errorf("no config found; run agent-stream-dbg (no flags) to open the home hub and create one")
	}
	// Prefer first ready entry, else first listed.
	for _, e := range entries {
		if e.Status == config.EntryReady {
			return e.Path, false, nil
		}
	}
	return entries[0].Path, false, nil
}
