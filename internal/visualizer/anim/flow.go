package anim

// FlowGlyph is the status icon for a flow stage row.
// inProgress → chase spinner; done → check; disabled → dash; pending → dim.
func FlowGlyph(enabled, inProgress, done bool, frame int, s Styles, reduced bool) string {
	if !enabled {
		return s.Muted.Render("–")
	}
	if done && !inProgress {
		return s.Live.Render("✓")
	}
	if inProgress {
		if reduced {
			return s.Accent.Render("…")
		}
		spin := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		return s.Accent.Render(spin[frame%len(spin)])
	}
	return s.Muted.Render("·")
}

// FlowChase is a short in-progress trail appended next to a hot stage name.
func FlowChase(frame int, s Styles, reduced bool) string {
	if reduced {
		return s.Muted.Render("···")
	}
	// Sliding highlight across three dots.
	slots := []string{
		s.Accent.Render("●") + s.Muted.Render("··"),
		s.Muted.Render("·") + s.Accent.Render("●") + s.Muted.Render("·"),
		s.Muted.Render("··") + s.Accent.Render("●"),
		s.Muted.Render("·") + s.Accent.Render("●") + s.Muted.Render("·"),
	}
	return slots[frame%len(slots)]
}
