package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Obedience-Corp/stream-debugger/internal/config"
)

var interactiveConfigNames = []string{
	"stream-debugger.yaml",
	"stream-debugger.yml",
	"config.yaml",
	"config.yml",
}

// resolveInteractiveConfig returns an explicit or discovered config path. If
// no config exists, it creates a private starter config so the interactive TUI
// can guide the user through setup on the first launch.
func resolveInteractiveConfig(explicit string) (path string, created bool, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false, fmt.Errorf("find current directory: %w", err)
	}
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", false, fmt.Errorf("find user config directory: %w", err)
	}
	return resolveInteractiveConfigAt(explicit, cwd, filepath.Join(userConfigDir, "stream-debugger"))
}

func resolveInteractiveConfigAt(explicit, cwd, userConfigDir string) (path string, created bool, err error) {
	if strings.TrimSpace(explicit) != "" {
		if _, err := os.Stat(explicit); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", false, fmt.Errorf("config file %q not found; run stream-debugger without --config to create a starter config", explicit)
			}
			return "", false, fmt.Errorf("check config file %q: %w", explicit, err)
		}
		return explicit, false, nil
	}

	for _, directory := range []string{cwd, userConfigDir} {
		if strings.TrimSpace(directory) == "" {
			continue
		}
		for _, name := range interactiveConfigNames {
			candidate := filepath.Join(directory, name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, false, nil
			} else if !errors.Is(err, fs.ErrNotExist) {
				return "", false, fmt.Errorf("check config file %q: %w", candidate, err)
			}
		}
	}

	if strings.TrimSpace(userConfigDir) == "" {
		return "", false, fmt.Errorf("no config found and user config directory is unavailable")
	}
	starterPath := filepath.Join(userConfigDir, "config.yaml")
	if err := config.WriteStarterConfig(starterPath); err != nil {
		return "", false, fmt.Errorf("no config found; create starter config: %w", err)
	}
	return starterPath, true, nil
}
