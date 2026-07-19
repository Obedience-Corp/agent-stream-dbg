package visualizer

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestForceTUIColorProfileOverridesNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("STREAM_DEBUGGER_COLOR_PROFILE", "truecolor")
	forceTUIColorProfile()
	if lipgloss.ColorProfile() == termenv.Ascii {
		t.Fatal("expected non-ascii color profile when CLICOLOR_FORCE overrides NO_COLOR")
	}
}

func TestForceTUIColorProfileExplicitANSI(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("STREAM_DEBUGGER_COLOR_PROFILE", "ansi")
	forceTUIColorProfile()
	if lipgloss.ColorProfile() != termenv.ANSI {
		t.Fatalf("profile = %v, want ANSI", lipgloss.ColorProfile())
	}
}
