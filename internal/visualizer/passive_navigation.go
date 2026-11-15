package visualizer

// Keep row selection within the evidence being displayed. Events are projected
// one bounded page at a time; raw detail selection retains the absolute index.
const passiveEventPage = 512

func (m *ObserverModel) movePassiveSelection(delta int) {
	m.followLatest = false
	count := 0
	switch {
	case m.selectingScope:
		count = len(m.store.groups())
	case m.pane == PaneFlow:
		count = len(m.presentation().flowRows())
	case m.pane == PaneApp:
		count = len(m.presentation().messages[0].AgentResponses)
	case m.pane == PaneTimeline:
		count = len(m.IntervalReport().Intervals)
	case m.pane == PaneEvents:
		count = len(m.selectedRecords())
	default:
		m.scroll = max(0, m.scroll+delta)
		return
	}
	oldPage := m.eventRow / passiveEventPage
	m.eventRow = max(0, min(max(0, count-1), m.eventRow+delta))
	m.scroll = max(0, m.scroll+delta)
	if m.pane == PaneEvents && oldPage != m.eventRow/passiveEventPage {
		m.scroll = 0
	}
	if m.pane == PaneTimeline {
		m.scroll = max(0, m.eventRow-max(1, m.height-18)+1)
	}
}

func (m *ObserverModel) selectedPaneInterval() *inspectedInterval {
	report := m.IntervalReport()
	if m.pane == PaneFlow {
		rows := m.presentation().flowRows()
		if len(rows) == 0 {
			return nil
		}
		row := rows[min(m.eventRow, len(rows)-1)]
		if row.child != nil {
			return row.child
		}
		for _, interval := range report.Intervals {
			if interval.Stage == row.stage && interval.ParentStage == "" {
				return interval
			}
		}
		return nil
	}
	// Match the domain-separated order drawn by the Timeline pane.
	row := 0
	for _, domain := range report.Domains {
		for _, interval := range domain.Intervals {
			if row == m.eventRow {
				return interval
			}
			row++
		}
	}
	return nil
}
