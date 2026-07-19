package anim

// AgentGlyph is the status spinner for an agent card.
// active && !done → rotating quad; done → check; idle → dim bullet.
func AgentGlyph(active, done bool, frame int, s Styles, reduced bool) string {
	if done {
		return s.Live.Render("✓")
	}
	if !active {
		return s.Muted.Render("●")
	}
	if reduced {
		return s.Agent.Render("◉")
	}
	spin := []string{"◐", "◓", "◑", "◒"}
	return s.Agent.Render(spin[frame%len(spin)])
}

// AggregatorGlyph mirrors AgentGlyph with aggregator accent.
func AggregatorGlyph(active, done bool, frame int, s Styles, reduced bool) string {
	if done {
		return s.Agg.Render("✓")
	}
	if !active {
		return s.Muted.Render("●")
	}
	if reduced {
		return s.Agg.Render("◉")
	}
	spin := []string{"◈", "◆", "◇", "◆"}
	return s.Agg.Render(spin[frame%len(spin)])
}
