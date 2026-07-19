package visualizer

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() {
	// Package init runs before the Bubble Tea program starts so package-level
	// and first-frame styles never render under a monochrome detection.
	forceTUIColorProfile()
}

// forceTUIColorProfile ensures lipgloss emits real color sequences even when
// the host terminal (or a recorder like VHS/ttyd) reports a weak capability,
// and when the parent agent shell exported NO_COLOR=1.
//
// CLICOLOR_FORCE / FORCE_COLOR / STREAM_DEBUGGER_COLOR_PROFILE override NO_COLOR
// so demos and CI can still paint.
func forceTUIColorProfile() {
	force := envTruthy("CLICOLOR_FORCE") ||
		envTruthy("FORCE_COLOR") ||
		os.Getenv("STREAM_DEBUGGER_COLOR_PROFILE") != ""
	if os.Getenv("NO_COLOR") != "" && !force {
		return
	}
	// Never block on OSC background queries under bare PTYs.
	lipgloss.SetHasDarkBackground(true)

	switch strings.ToLower(strings.TrimSpace(os.Getenv("STREAM_DEBUGGER_COLOR_PROFILE"))) {
	case "truecolor", "24bit", "true":
		lipgloss.SetColorProfile(termenv.TrueColor)
		return
	case "ansi256", "256":
		lipgloss.SetColorProfile(termenv.ANSI256)
		return
	case "ansi", "16":
		lipgloss.SetColorProfile(termenv.ANSI)
		return
	case "ascii", "mono":
		lipgloss.SetColorProfile(termenv.Ascii)
		return
	}
	if force ||
		os.Getenv("COLORTERM") == "truecolor" ||
		os.Getenv("COLORTERM") == "24bit" {
		lipgloss.SetColorProfile(termenv.TrueColor)
		return
	}
	if lipgloss.ColorProfile() == termenv.Ascii {
		lipgloss.SetColorProfile(termenv.TrueColor)
	}
}

func envTruthy(key string) bool {
	v := strings.TrimSpace(os.Getenv(key))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}
