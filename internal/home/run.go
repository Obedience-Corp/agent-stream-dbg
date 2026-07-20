package home

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Obedience-Corp/stream-debugger/internal/config"
)

// Action is what the CLI should do after the home hub exits.
type Action int

const (
	ActionQuit Action = iota
	ActionOpen
)

// Result is returned when the home hub TUI exits.
type Result struct {
	Action         Action
	Path           string
	OpenConfigPanel bool
	Notice         string
}

// Options configure the home hub.
type Options struct {
	Cwd           string
	UserConfigDir string
	Notice        string
}

// Run launches the home hub TUI and blocks until the user quits or opens a session.
func Run(opts Options) (Result, error) {
	m := newModel(opts)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return Result{}, fmt.Errorf("home TUI: %w", err)
	}
	out, ok := final.(model)
	if !ok {
		return Result{Action: ActionQuit}, nil
	}
	return out.result, nil
}

// ReloadEntries re-scans config locations (used by tests and after external edits).
func ReloadEntries(cwd, userConfigDir string) []config.ConfigEntry {
	return config.ListConfigs(cwd, userConfigDir)
}
