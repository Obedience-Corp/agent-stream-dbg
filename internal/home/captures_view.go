package home

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Metadata is external text; keep it on one row and out of terminal controls.
func captureLabel(s string, width int) string {
	s = strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return ' '
		}
		return r
	}, s)
	return ansi.Truncate(s, max(1, width), "…")
}

func (m model) viewCaptures() string {
	var out strings.Builder
	b := m.captures
	if b.live {
		return m.viewLive()
	}
	width := max(25, m.width-12)
	line := func(s string) { out.WriteString(s); out.WriteByte('\n') }
	line(titleStyle.Render("Obey Voice recordings"))
	line(dimStyle.Render("Newest first · refreshes automatically · opens in Timeline"))
	line(dimStyle.Render(captureLabel(b.root, width)))
	line("")
	if m.notice != "" {
		line(chipNeed.Render(captureLabel(m.notice, width)))
	}
	if b.preferenceErr != "" {
		line(chipNeed.Render("Folder preference unreadable; press c to choose it again."))
	}
	if b.err != "" {
		line(statusStyle.Render(captureLabel(b.err, width)))
	}
	if len(b.listing.Entries) == 0 {
		if b.loading {
			line("Looking for recordings…")
		} else if b.listing.Missing {
			line("The captures folder does not exist yet.")
		} else {
			line("No recordings found here yet.")
		}
		line("")
		line("In your Obey Voice conversation, type /record talk.")
		line("This records events, transcript, and conversation audio.")
		line("The recording will appear here; select it and press Enter.")
		line("Use /record off in Obey Voice when you're finished.")
		line("")
		line(dimStyle.Render("Using a custom install? Press c to choose its captures folder."))
	} else {
		rows := max(1, m.height-17)
		start := max(0, b.cursor-rows+1)
		end := min(len(b.listing.Entries), start+rows)
		for i := start; i < end; i++ {
			entry := b.listing.Entries[i]
			stamp := "time unknown"
			if !entry.StartedAt.IsZero() {
				stamp = entry.StartedAt.Local().Format("Jan 02 15:04:05")
			}
			session := entry.SessionID
			if session == "" {
				session = filepath.Base(filepath.Dir(entry.Path))
			}
			status := entry.Status
			if status == "sealed" {
				status = "saved"
			}
			if status == "" {
				status = "unknown"
			}
			prefix, style := "  ", dimStyle
			if i == b.cursor {
				prefix, style = "▸ ", cursorStyle
			}
			row := fmt.Sprintf("%s%s  %-10s %-4s  %s", prefix, stamp, status, entry.Mode, session)
			line(style.Render(captureLabel(row, width)))
		}
		line(dimStyle.Render(fmt.Sprintf("%d–%d of %d recordings", start+1, end, len(b.listing.Entries))))
		line("")
		if b.cursor >= 0 && b.cursor < len(b.listing.Entries) {
			entry := b.listing.Entries[b.cursor]
			line(dimStyle.Render(captureLabel(filepath.Base(filepath.Dir(entry.Path)), width)))
			if entry.Follow() {
				line("Enter follows new events. Open recordings may be idle or abandoned.")
			} else {
				line("Enter inspects this saved recording.")
			}
		} else {
			line(chipNeed.Render("Selected recording disappeared; use ↑↓ to select another."))
		}
	}
	if b.listing.Skipped > 0 {
		line(chipNeed.Render(fmt.Sprintf("%d folders skipped: missing, unreadable, or invalid capture files.", b.listing.Skipped)))
	}
	line("")
	line(dimStyle.Render("↑↓ select  enter open  c folder  r refresh  esc home  Ctrl+C quit"))
	return out.String()
}

func (m model) viewCaptureFolder() string {
	if m.captures.live {
		return titleStyle.Render("Choose Obey Voice config folder") + "\n\nPaste the config directory used by your running Obey Voice.\nIts debug/runtimes folder is discovered automatically.\n\n" + m.captures.folder.View() + "\n\n" + dimStyle.Render("enter remember folder  esc cancel") + "\n" + statusStyle.Render(captureLabel(m.status, max(25, m.width-12)))
	}
	var out strings.Builder
	out.WriteString(titleStyle.Render("Choose Obey Voice captures folder") + "\n\n")
	out.WriteString("Paste the folder containing your recording bundles.\n")
	out.WriteString("It is usually the captures folder inside your Obey Voice config directory.\n\n")
	out.WriteString(m.captures.folder.View() + "\n\n")
	out.WriteString(dimStyle.Render("Remembered for future launches · enter save  esc cancel"))
	if m.status != "" {
		out.WriteString("\n\n" + statusStyle.Render(captureLabel(m.status, max(25, m.width-12))))
	}
	return out.String()
}
