package visualizer

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
	filetransport "github.com/Obedience-Corp/agent-stream-dbg/internal/transport/file"
	tea "github.com/charmbracelet/bubbletea"
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
func historyOpen(ctx context.Context, request historyRequest) (*os.File, error) {
	f, err := os.Open(request.Path)
	if err != nil {
		return nil, fmt.Errorf("open source history: %w", err)
	}
	fail := func(err error) (*os.File, error) { _ = f.Close(); return nil, err }
	info, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	minimum := request.MinSize
	if request.Identity != nil {
		minimum = max(minimum, request.Identity.Size())
		if !os.SameFile(request.Identity, info) {
			return fail(filetransport.ErrSourceChanged)
		}
	}
	if !info.Mode().IsRegular() || info.Size() < minimum {
		return fail(filetransport.ErrSourceChanged)
	}
	if len(request.Prefix) > 0 {
		prefix := make([]byte, len(request.Prefix))
		if _, err := f.ReadAt(prefix, 0); err != nil || !bytes.Equal(prefix, request.Prefix) {
			return fail(filetransport.ErrSourceChanged)
		}
	}
	reader := bufio.NewReaderSize(f, 64<<10)
	header, _, err := historyLine(ctx, reader, true)
	if err != nil {
		return fail(fmt.Errorf("source header unavailable: %w", err))
	}
	if len(header) > filetransport.MaxLineBytes {
		return fail(fmt.Errorf("source header exceeds bounded line size"))
	}
	event, err := request.Parser.ParseBounded("", header, 4<<20)
	if err != nil {
		return fail(err)
	}
	for _, check := range [][2]string{{"capture_id", request.CaptureID}, {"trace_schema", request.Schema}, {"runtime_id", request.Runtime}, {"session_id", request.Session}, {"conversation_epoch", request.Epoch}} {
		if event.StringField(check[0]) != check[1] {
			return fail(filetransport.ErrSourceChanged)
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	return f, nil
}
func historyScan(ctx context.Context, request historyRequest, start int64, visit func(*events.Event, transport.Frame, string) bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := historyOpen(ctx, request)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return err
	}
	reader := bufio.NewReaderSize(f, 64<<10)
	offset := start
	state := newObserverStore()
	state.legacy = request.Legacy
	for {
		raw, size, readErr := historyLine(ctx, reader)
		lineOffset := offset
		offset += size
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			if errors.Is(readErr, errHistoryOversize) {
				continue
			}
			return readErr
		}
		event, parseErr := request.Parser.ParseBounded("", bytes.TrimSpace(raw), 4<<20)
		if parseErr != nil {
			continue
		}
		if capture := event.StringField("capture_id"); capture != "" && request.CaptureID != "" && capture != request.CaptureID {
			return filetransport.ErrSourceChanged
		}
		scope := state.scopeFor(event)
		frame := transport.Frame{Data: bytes.TrimSpace(raw), Raw: raw, Offset: lineOffset, Size: size}
		if !visit(event, frame, scope) {
			break
		}
	}
	current, err := os.Stat(request.Path)
	if err != nil {
		return filetransport.ErrSourceChanged
	}
	info, err := f.Stat()
	if err != nil || !os.SameFile(info, current) || current.Size() < request.MinSize {
		return filetransport.ErrSourceChanged
	}
	if request.Identity != nil && !os.SameFile(request.Identity, current) {
		return filetransport.ErrSourceChanged
	}
	// Reopen/revalidate after reading too: a same-inode header replacement
	// while the visitor was running must not publish mixed evidence.
	verified, verifyErr := historyOpen(ctx, request)
	if verifyErr != nil {
		return verifyErr
	}
	_ = verified.Close()
	return ctx.Err()
}
func loadObserverHistory(ctx context.Context, request historyRequest) (*observerStore, observerSelection, error) {
	target := request.Anchor
	payloadStart := int64(0)
	if request.Direction != 0 {
		// Sparse windows may contain delayed observations of much older turns.
		// Scan the complete source to establish the nearest source-clock neighbor;
		// the index is only a seek hint, never proof of predecessor coverage.
		var chosen *observedRecord
		// Only equal-clock comparisons need first-occurrence lookup. Cache a
		// bounded subset; cache exhaustion falls back to a cancellable prefix
		// scan rather than imposing a maximum number of turns in the artifact.
		firstOffsets := make(map[string]int64)
		var discoveryErr error
		err := historyScan(ctx, request, 0, func(event *events.Event, frame transport.Frame, scope string) bool {
			if event.StringField("turn_id") == "" && !strings.HasPrefix(scope, "legacy-local/") {
				return true
			}
			if scope == request.Anchor.Key {
				return true
			}
			if chosen != nil && chosen.Scope == scope {
				return true
			}
			first := frame.Offset
			value, known := finiteNumber(event.Fields["turn_start_ms"])
			tied := !known || request.Anchor.StartMS == nil || value == *request.Anchor.StartMS
			if chosen != nil {
				other, ok := finiteNumber(chosen.Event.Fields["turn_start_ms"])
				tied = tied || known && ok && value == other
			}
			if tied {
				cached, ok := firstOffsets[scope]
				if !ok {
					lookupErr := historyScan(ctx, request, 0, func(_ *events.Event, frame transport.Frame, candidate string) bool {
						if candidate == scope {
							cached = frame.Offset
							return false
						}
						return true
					})
					if lookupErr != nil {
						discoveryErr = lookupErr
						return false
					}
					if len(firstOffsets) < 128 {
						firstOffsets[scope] = cached
					}
				}
				first = cached
			}
			record := &observedRecord{Event: event, Offset: first, Scope: scope}
			if neighborRecord(request.Anchor, record, request.Direction) && betterNeighbor(chosen, record, request.Direction) {
				chosen = &observedRecord{Scope: scope, Offset: first, Event: &events.Event{Fields: map[string]any{
					"runtime_id":    strings.Clone(boundedIdentity(event.StringField("runtime_id"))),
					"turn_id":       strings.Clone(boundedIdentity(event.StringField("turn_id"))),
					"turn_start_ms": nil,
				}}}
				if known {
					chosen.Event.Fields["turn_start_ms"] = value
				}
			}
			return true
		})
		if err != nil {
			return nil, observerSelection{}, err
		}
		if discoveryErr != nil {
			return nil, observerSelection{}, discoveryErr
		}
		found := chosen != nil
		if found {
			target = observerSelection{Key: chosen.Scope, Offset: chosen.Offset}
			if !request.Legacy {
				for _, entry := range request.Index {
					if entry.Offset <= chosen.Offset && entry.Offset > payloadStart {
						payloadStart = entry.Offset
					}
				}
			}
		}
		if !found {
			return nil, observerSelection{}, nil
		}
	}
	// A filtered scan from a proven sparse predecessor preserves every delayed
	// observation for this immutable scope; the index never defines turn order. A long single turn still obeys the same page cap.
	page := newObserverStore()
	page.legacy = request.Legacy
	err := historyScan(ctx, request, payloadStart, func(event *events.Event, frame transport.Frame, scope string) bool {
		if scope == target.Key {
			page.add(event, frame, false, scope)
			if request.OwnBytes != nil {
				request.OwnBytes(page.bytes)
			}
		}
		return true
	})
	if err != nil {
		return nil, observerSelection{}, err
	}
	anchor, found := selectionFromRecords(page, target.Key)
	if found {
		anchor.Offset = min(anchor.Offset, target.Offset)
	}
	if !found {
		return nil, observerSelection{}, nil
	}
	return page, anchor, nil
}

var errHistoryOversize = errors.New("history record exceeds bounded line size")

// historyLine counts exact bytes, retains CRLF, and never interprets an
// unterminated tail as a complete record. Oversized rows are drained to newline.
func historyLine(ctx context.Context, reader *bufio.Reader, rejectOversize ...bool) ([]byte, int64, error) {
	var raw []byte
	var size int64
	oversized := false
	for {
		if err := ctx.Err(); err != nil {
			return nil, size, err
		}
		fragment, err := reader.ReadSlice('\n')
		size += int64(len(fragment))
		if !oversized {
			if len(raw)+len(fragment) > filetransport.MaxLineBytes {
				oversized = true
				raw = nil
				if len(rejectOversize) > 0 && rejectOversize[0] {
					return nil, size, errHistoryOversize
				}
			} else {
				raw = append(raw, fragment...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil {
			return raw, size, err
		}
		if oversized {
			return nil, size, errHistoryOversize
		}
		return raw, size, nil
	}
}

// Immediate user selections supersede any queued asynchronous page result.
func (m *ObserverModel) cancelHistorySelection() {
	m.page = nil
	m.historyPending = nil
	m.historyGeneration++
	if m.historyCancel != nil {
		m.historyCancel()
	}
}
