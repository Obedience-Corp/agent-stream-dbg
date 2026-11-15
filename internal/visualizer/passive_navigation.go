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

// selectionIdentity names the selected Flow or Timeline row by what it shows,
// so a paused selection survives rows being added above it.
type selectionIdentity struct{ stage, interval string }

func (m *ObserverModel) selectedIdentity() (selectionIdentity, bool) {
	if m.selectingScope {
		return selectionIdentity{}, false
	}
	switch m.pane {
	case PaneFlow:
		rows := m.presentation().flowRows()
		if len(rows) == 0 {
			return selectionIdentity{}, false
		}
		row := rows[min(m.eventRow, len(rows)-1)]
		id := selectionIdentity{stage: row.stage}
		if row.child != nil {
			id.interval = intervalIdentity(row.child.Runtime, row.child.ID)
		}
		return id, true
	case PaneTimeline:
		if interval := m.selectedPaneInterval(); interval != nil {
			return selectionIdentity{interval: intervalIdentity(interval.Runtime, interval.ID)}, true
		}
	}
	return selectionIdentity{}, false
}

// restoreSelection moves the row index back onto the remembered row. A request
// row that is gone falls back to its stage; anything else keeps its index.
func (m *ObserverModel) restoreSelection(id selectionIdentity) {
	switch m.pane {
	case PaneFlow:
		stageRow := -1
		for index, row := range m.presentation().flowRows() {
			if row.stage != id.stage {
				continue
			}
			if row.child == nil {
				stageRow = index
				if id.interval == "" {
					break
				}
			} else if intervalIdentity(row.child.Runtime, row.child.ID) == id.interval {
				m.eventRow = index
				return
			}
		}
		if stageRow >= 0 {
			m.eventRow = stageRow
		}
	case PaneTimeline:
		row := 0
		for _, domain := range m.IntervalReport().Domains {
			for _, interval := range domain.Intervals {
				if intervalIdentity(interval.Runtime, interval.ID) == id.interval {
					m.eventRow = row
					m.scroll = max(0, m.eventRow-max(1, m.height-18)+1)
					return
				}
				row++
			}
		}
	}
}
