package visualizer

import (
	"github.com/charmbracelet/lipgloss"
	"strings"
	"testing"
)

func TestHistoryWideCellClippingBoundedWork(t *testing.T) {
	for _, text := range []string{strings.Repeat("漢字🙂", 10000), strings.Repeat("\u0301", 100000) + "tail", strings.Repeat("\x00", 100000) + "tail"} {
		clipped := observerClip(text, 20)
		if lipgloss.Width(clipped) > 20 || len(clipped) > 20*16+8 {
			t.Fatalf("unbounded terminal row: cells=%d bytes=%d", lipgloss.Width(clipped), len(clipped))
		}
	}
}
