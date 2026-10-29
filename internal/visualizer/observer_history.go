package visualizer

import (
	"context"
	"errors"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"strings"
)

type observerSelection struct {
	Key, Runtime, Turn, Label string
	Offset, LastOffset        int64
	StartMS                   *float64
	RootSeen, Legacy          bool
}
type historyRequest struct {
	Generation                                 uint64
	Path                                       string
	Parser                                     *bridge.Parser
	Anchor                                     observerSelection
	Direction                                  int // -1 prior, +1 next, 0 reload selected scope
	Legacy                                     bool
	CaptureID, Schema, Runtime, Session, Epoch string
	MinSize                                    int64
	Identity                                   os.FileInfo
	Prefix                                     []byte
	Index                                      []sourceIndexEntry
	OwnBytes                                   func(int)
}
type observerHistoryResult struct {
	Generation uint64
	Page       *observerStore
	Anchor     observerSelection
	Err        error
}

func sourceTurnRecord(record *observedRecord) bool {
	return record.Event.StringField("turn_id") != "" || strings.HasPrefix(record.Scope, "legacy-local/")
}
func selectionFromRecords(store *observerStore, key string) (observerSelection, bool) {
	result := observerSelection{Key: key}
	found := false
	for _, record := range store.records {
		if record.Scope != key {
			continue
		}
		e := record.Event
		if !found {
			found = true
			result.Offset = record.Offset
			result.Runtime = strings.Clone(boundedIdentity(e.StringField("runtime_id")))
			result.Turn = strings.Clone(boundedIdentity(e.StringField("turn_id")))
			result.Label = result.Turn
			if result.Label == "" {
				result.Label = key
			}
			result.Legacy = strings.HasPrefix(key, "legacy-local/")
		}
		result.Offset = min(result.Offset, record.Offset)
		result.LastOffset = max(result.LastOffset, record.Offset)
		if result.StartMS == nil {
			if value, ok := finiteNumber(e.Fields["turn_start_ms"]); ok {
				result.StartMS = &value
			}
		}
		if e.StringField("span_id") == result.Turn && result.Turn != "" && e.StringField("span_state") == "start" {
			result.RootSeen = true
		}
	}
	return result, found
}
func (m *ObserverModel) selectScope(key string, store *observerStore) {
	m.selected = key
	anchor, ok := selectionFromRecords(store, key)
	if !ok {
		return
	}
	if m.selectionAnchor.Key == key {
		anchor.Offset = min(anchor.Offset, m.selectionAnchor.Offset)
		anchor.LastOffset = max(anchor.LastOffset, m.selectionAnchor.LastOffset)
		anchor.RootSeen = anchor.RootSeen || m.selectionAnchor.RootSeen
		if anchor.StartMS == nil {
			anchor.StartMS = m.selectionAnchor.StartMS
		}
	}
	m.selectionAnchor = anchor
}
func neighborRecord(anchor observerSelection, record *observedRecord, direction int) bool {
	if !sourceTurnRecord(record) || record.Scope == anchor.Key {
		return false
	}
	runtime := boundedIdentity(record.Event.StringField("runtime_id"))
	if anchor.Runtime != "" && runtime != "" && runtime != anchor.Runtime {
		return false
	}
	if anchor.StartMS != nil {
		if value, ok := finiteNumber(record.Event.Fields["turn_start_ms"]); ok {
			if value != *anchor.StartMS {
				return direction < 0 && value < *anchor.StartMS || direction > 0 && value > *anchor.StartMS
			}
			return direction < 0 && record.Offset < anchor.Offset || direction > 0 && record.Offset > anchor.Offset
		}
	}
	return direction < 0 && record.Offset < anchor.Offset || direction > 0 && record.Offset > anchor.LastOffset
}
func betterNeighbor(current *observedRecord, candidate *observedRecord, direction int) bool {
	if current == nil {
		return true
	}
	a, aOK := finiteNumber(current.Event.Fields["turn_start_ms"])
	b, bOK := finiteNumber(candidate.Event.Fields["turn_start_ms"])
	if aOK && bOK {
		if a != b {
			return direction < 0 && b > a || direction > 0 && b < a
		}
		return direction < 0 && candidate.Offset > current.Offset || direction > 0 && candidate.Offset < current.Offset
	}
	return direction < 0 && candidate.Offset > current.Offset || direction > 0 && candidate.Offset < current.Offset
}
func findRetainedNeighbor(anchor observerSelection, stores []*observerStore, direction int) (string, *observerStore) {
	var chosen *observedRecord
	var owner *observerStore
	for _, store := range stores {
		if store == nil {
			continue
		}
		visited := make(map[string]bool)
		for _, record := range store.records {
			if visited[record.Scope] {
				continue
			}
			visited[record.Scope] = true
			if neighborRecord(anchor, record, direction) && betterNeighbor(chosen, record, direction) {
				chosen = record
				owner = store
			}
		}
	}
	if chosen == nil {
		return "", nil
	}
	return chosen.Scope, owner
}
func (m *ObserverModel) followLatestSelection() {
	m.followLatest = true
	m.page = nil
	m.details = false
	m.historyPending = nil
	m.historyGeneration++
	if m.historyCancel != nil {
		m.historyCancel()
	}
	groups := m.store.groups()
	if len(groups) > 0 {
		m.selectScope(latestObservedGroup(groups, m.runtimeID), m.store)
	}
	m.scroll = 0
	m.eventRow = 0
}
func (m *ObserverModel) navigateTurn(direction int) tea.Cmd {
	m.followLatest = false
	m.details = false
	if m.selectionAnchor.Key != m.selected {
		if m.page != nil {
			m.selectScope(m.selected, m.page)
		}
		if m.selectionAnchor.Key != m.selected {
			m.selectScope(m.selected, m.store)
		}
	}
	anchor := m.selectionAnchor
	key, owner := findRetainedNeighbor(anchor, []*observerStore{m.page, m.store}, direction)
	earliest := int64(0)
	if len(m.store.records) > 0 {
		earliest = m.store.records[0].Offset
	}
	needsDisk := m.store.evicted && (anchor.Offset < earliest || key == "")
	if key != "" {
		candidate, _ := selectionFromRecords(owner, key)
		if m.store.evicted && !candidate.RootSeen && !candidate.Legacy {
			needsDisk = true
		}
	}
	if key != "" && !needsDisk {
		m.cancelHistorySelection()
		m.selectScope(key, owner)
		m.scroll = 0
		m.eventRow = 0
		return nil
	}
	if !m.store.evicted && key == "" {
		m.status = "turn boundary reached"
		return nil
	}
	return m.requestHistory(direction)
}
func (m *ObserverModel) requestHistory(direction int) tea.Cmd {
	if m.source.Name() == "sse" {
		m.status = "Live history is limited to retained events; reconnect for recent source history"
		return nil
	}
	m.followLatest = false
	m.details = false
	m.page = nil
	m.historyGeneration++
	request := historyRequest{Generation: m.historyGeneration, Path: m.path, Parser: m.parser, Anchor: m.selectionAnchor, Direction: direction, Legacy: m.store.legacy, CaptureID: m.captureID, Schema: m.traceSchema, Runtime: m.runtimeID, Session: m.sessionID, Epoch: m.conversationEpoch, MinSize: m.store.maxSourceEnd, Index: append([]sourceIndexEntry(nil), m.store.index...)}
	if source, ok := m.source.(interface{ Snapshot() (os.FileInfo, []byte) }); ok {
		request.Identity, request.Prefix = source.Snapshot()
	}
	request.OwnBytes = func(bytes int) {
		m.historyInFlight.Store(int64(bytes))
		for {
			peak := m.historyPeak.Load()
			if int64(bytes) <= peak || m.historyPeak.CompareAndSwap(peak, int64(bytes)) {
				break
			}
		}
	}
	if m.historyActive {
		if m.historyCancel != nil {
			m.historyCancel()
		}
		m.historyPending = &request
		m.status = "canceling previous history load"
		return nil
	}
	return m.startHistory(request)
}
func (m *ObserverModel) startHistory(request historyRequest) tea.Cmd {
	ctx, cancel := context.WithCancel(m.ctx)
	m.historyCancel = cancel
	m.historyActive = true
	m.status = "loading source history"
	loader := m.historyLoad
	if loader == nil {
		loader = loadObserverHistory
	}
	return func() tea.Msg {
		page, anchor, err := loader(ctx, request)
		if ctx.Err() != nil {
			page = nil
			err = ctx.Err()
		}
		return &observerHistoryResult{request.Generation, page, anchor, err}
	}
}
func (m *ObserverModel) historyResult(message *observerHistoryResult) tea.Cmd {
	m.historyActive = false
	m.historyInFlight.Store(0)
	page := message.Page
	message.Page = nil
	if m.historyPending != nil {
		request := *m.historyPending
		m.historyPending = nil
		page = nil
		return m.startHistory(request)
	}
	if message.Generation != m.historyGeneration {
		return nil
	}
	if message.Err != nil {
		if !errors.Is(message.Err, context.Canceled) {
			m.err = message.Err
			m.store.issue(message.Err.Error())
			m.status = "history unavailable"
		}
		return nil
	}
	if page == nil {
		m.status = "turn boundary reached"
		return nil
	}
	m.page = page
	m.selected = message.Anchor.Key
	m.selectionAnchor = message.Anchor
	m.scroll = 0
	m.eventRow = 0
	m.status = "source history loaded; inspection open"
	if m.ingesting {
		m.status = "source history loaded; live ingestion continues"
	}
	return nil
}
func (m *ObserverModel) cancelHistorySelection() {
	m.page = nil
	m.historyPending = nil
	m.historyGeneration++
	if m.historyCancel != nil {
		m.historyCancel()
	}
}
