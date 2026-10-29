package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/config"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/home"
)

// runHome is the default bare-launch experience: list/create configs, open
// sessions, and return to the hub when a session ends.
// corrFlags come from global CLI flags and env (correlationFromCLI).
func runHome(dialectOverride string, corrFlags ...correlationFlags) error {
	corr := correlationFlags{}
	if len(corrFlags) > 0 {
		corr = corrFlags[0]
	} else {
		// Env-only fallback when called without CLI context.
		if v := strings.TrimSpace(os.Getenv("OTEL_PROPAGATE")); v == "1" || strings.EqualFold(v, "true") {
			corr.propagate = true
		}
		corr.traceparent = strings.TrimSpace(os.Getenv("TRACEPARENT"))
	}
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

	return runHomeWithOptions(home.Options{Cwd: cwd, UserConfigDir: userConfigDir}, dialectOverride, corr)
}

func runHomeWithOptions(opts home.Options, dialectOverride string, corr correlationFlags) error {
	for {
		result, err := home.Run(opts)
		if err != nil {
			return err
		}
		opts.Notice = result.Notice
		switch result.Action {
		case home.ActionQuit:
			return nil
		case home.ActionLive:
			opts.StartLive, opts.StartCaptures, opts.CapturePath = true, false, result.Path
			opts.Notice = ""
			if err := runLiveVoice(result.Path); err != nil {
				opts.Notice = "Could not attach: " + err.Error()
			}
		case home.ActionObserve:
			opts.StartLive = false
			opts.StartCaptures, opts.CapturePath = true, result.Path
			opts.Notice = ""
			if err := runObserve(result.Path, "obey-voice", result.Follow); err != nil {
				opts.Notice = "Could not open recording: " + err.Error()
			}
		case home.ActionOpen:
			opts.StartCaptures, opts.StartLive = false, false
			if err := runInteractiveWithOptions(result.Path, dialectOverride, result.OpenConfigPanel, corr); err != nil {
				// Return to home with the error so a bad setup is recoverable.
				opts.Notice = "Session error: " + err.Error()
				fmt.Fprintf(os.Stderr, "⚠️  %s\n", opts.Notice)
				continue
			}
			if opts.Notice == "" {
				opts.Notice = "Returned from session · " + filepath.Base(result.Path)
			}
		default:
			return nil
		}
	}
}
