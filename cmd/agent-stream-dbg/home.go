package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/config"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/home"
)

// runHome is the default bare-launch experience: list/create configs, open
// sessions, and return to the hub when a session ends.
// Correlation flags come from process env only on this path (no CLI context).
func runHome(dialectOverride string) error {
	corr := correlationFlags{}
	if v := os.Getenv("OTEL_PROPAGATE"); v == "1" || v == "true" || v == "TRUE" {
		corr.propagate = true
	}
	corr.traceparent = os.Getenv("TRACEPARENT")
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("find current directory: %w", err)
	}
	userConfigDir, err := config.UserConfigDir()
	if err != nil {
		return fmt.Errorf("find user config directory: %w", err)
	}
	if err := os.MkdirAll(userConfigDir, 0o700); err != nil {
		return fmt.Errorf("create user config directory: %w", err)
	}

	notice := ""
	for {
		result, err := home.Run(home.Options{
			Cwd:           cwd,
			UserConfigDir: userConfigDir,
			Notice:        notice,
		})
		if err != nil {
			return err
		}
		notice = result.Notice
		switch result.Action {
		case home.ActionQuit:
			return nil
		case home.ActionOpen:
			if err := runInteractiveWithOptions(result.Path, dialectOverride, result.OpenConfigPanel, corr); err != nil {
				// Return to home with the error so a bad setup is recoverable.
				notice = "Session error: " + err.Error()
				fmt.Fprintf(os.Stderr, "⚠️  %s\n", notice)
				continue
			}
			if notice == "" {
				notice = "Returned from session · " + filepath.Base(result.Path)
			}
		default:
			return nil
		}
	}
}
