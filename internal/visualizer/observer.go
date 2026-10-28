package visualizer

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type observerConnected struct{ err error }
type observerFrame struct {
	frame transport.Frame
	ok    bool
}

// ObserverModel is the passive mode of the visualizer. It has no prompt,
// session setup, logger, control client, or source write capability.
type ObserverModel struct {
	selectionAnchor                                               observerSelection
	historyActive                                                 bool
	historyPending                                                *historyRequest
	historyCancel                                                 context.CancelFunc
	historyGeneration                                             uint64
	historyInFlight                                               atomic.Int64
	historyPeak                                                   atomic.Int64
	historyLoad                                                   func(context.Context, historyRequest) (*observerStore, observerSelection, error)
	traceSchema, conversationEpoch                                string
	detailRaw                                                     bool
	detailInterval                                                string
	detailRecordOffset                                            int64
	detailRawOffset, detailScroll, detailEvidence                 int
	source                                                        transport.Transport
	parser                                                        *bridge.Parser
	path                                                          string
	following                                                     bool
	ctx                                                           context.Context
	cancel                                                        context.CancelFunc
	store                                                         *observerStore
	page                                                          *observerStore
	selected                                                      string
	followLatest                                                  bool
	pane                                                          Pane
	width, height                                                 int
	scroll, eventRow                                              int
	details                                                       bool
	attached, ingesting, sourceEnded, sealed                      bool
	captureID, runtimeID, sessionID, health, schemaNotice, status string
	manifestCaptureID                                             string
	intervalsEnabled                                              bool
	err                                                           error
}

func NewObserverModel(ctx context.Context, tr transport.Transport, parser *bridge.Parser, path string, follow bool, manifestCaptureID string) *ObserverModel {
	ctx, cancel := context.WithCancel(ctx)
	return &ObserverModel{source: tr, parser: parser, path: path, following: follow, ctx: ctx, cancel: cancel, store: newObserverStore(), followLatest: true, pane: PaneTimeline, width: 100, height: 30, status: "attaching", manifestCaptureID: manifestCaptureID, intervalsEnabled: false}
}
func (m *ObserverModel) Init() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		return observerConnected{m.source.Connect(ctx)}
	}
}
func (m *ObserverModel) waitFrame() tea.Cmd {
	return func() tea.Msg {
		select {
		case frame, ok := <-m.source.Frames():
			return observerFrame{frame, ok}
		case <-m.ctx.Done():
			return nil
		}
	}
}
func (m *ObserverModel) closeSource() tea.Cmd {
	return func() tea.Msg { _ = m.source.Close(); return observerFrame{ok: false} }
}
func (m *ObserverModel) Close() error { m.cancel(); return m.source.Close() }
func (m *ObserverModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case *observerHistoryResult:
		return m, m.historyResult(msg)
	case tea.WindowSizeMsg:
		m.width = max(30, msg.Width)
		m.height = max(12, msg.Height)
	case observerConnected:
		if msg.err != nil {
			m.err = msg.err
			m.status = "attach failed"
			return m, nil
		}
		m.attached = true
		m.ingesting = true
		m.status = "reading"
		return m, m.waitFrame()
	case observerFrame:
		if !msg.ok {
			m.ingesting = false
			if !m.sourceEnded {
				m.status = "EOF — unsealed capture"
			}
			return m, nil
		}
		frame := msg.frame
		event, err := m.parser.ParseBounded(frame.Name, frame.Data, 4<<20)
		if err != nil {
			m.err = err
			return m, m.closeSource()
		}
		if frame.Err != nil {
			event = &events.Event{Name: "frame_error", Kind: events.KindError, Timestamp: frame.Timestamp, Fields: map[string]any{"frame_error": frame.Err.Error()}}
			m.store.issue(frame.Err.Error())
		}
		if event.Kind == events.KindSessionStart {
			m.sessionID = boundedIdentity(event.StringField("session_id"))
			m.runtimeID = boundedIdentity(event.StringField("runtime_id"))
			schema := event.StringField("trace_schema")
			m.traceSchema = schema
			m.conversationEpoch = event.StringField("conversation_epoch")
			m.schemaNotice = ""
			m.intervalsEnabled = true
			if schema == "" {
				m.store.legacy = true
				m.schemaNotice = "Legacy capture: response correlation and timing origin incomplete"
			} else if expected, family := event.IntField("supported_trace_major"), event.StringField("supported_trace_family"); (expected > 0 && traceMajor(schema) != expected) || (family != "" && traceFamily(schema) != family) {
				m.intervalsEnabled = false
				m.schemaNotice = "Unsupported trace schema family/version; raw inspection available"
			}
		}
		capture := boundedIdentity(event.StringField("capture_id"))
		if capture != "" {
			if m.captureID != "" && m.captureID != capture {
				m.err = fmt.Errorf("Source changed; reopen explicitly (capture identity)")
				m.status = m.err.Error()
				m.sourceEnded = true
				return m, m.closeSource()
			}
			m.captureID = capture
			if m.manifestCaptureID != "" && capture != m.manifestCaptureID {
				m.err = fmt.Errorf("capture header/manifest identity mismatch")
				m.sourceEnded = true
				return m, m.closeSource()
			}
		}
		if health := event.MapField("capture_health"); health != nil {
			m.health = fmt.Sprintf("lost:%v omitted:%v errors:%v written:%v synced:%v", observerNumber(health["dropped"]), observerNumber(health["skipped"]), observerNumber(health["write_errors"]), observerNumber(health["last_written_seq"]), observerNumber(health["last_synced_seq"]))
		}
		m.store.add(event, frame, true)
		if frame.Terminal && frame.Err != nil {
			m.err = frame.Err
			m.sourceEnded = true
			m.status = frame.Err.Error()
			return m, m.closeSource()
		}
		groups := m.store.groups()
		if m.followLatest && len(groups) > 0 {
			m.selectScope(latestObservedGroup(groups, m.runtimeID), m.store)
			m.scroll = 0
			m.eventRow = 0
		}
		if event.Kind == events.KindSessionEnd {
			m.sourceEnded = true
			m.sealed = event.StringField("status") == "sealed"
			if m.sealed {
				m.status = "sealed — inspection open"
			} else {
				m.status = "source end — " + observerClip(event.StringField("status"), 64)
			}
			if finite, ok := m.source.(interface{ FinishFollow() }); ok {
				finite.FinishFollow()
				return m, m.waitFrame()
			}
			return m, m.closeSource()
		}
		return m, m.waitFrame()
	case tea.KeyMsg:
		if m.details {
			switch msg.String() {
			case "r":
				m.detailRaw = true
				m.detailScroll = 0
				m.detailRawOffset = 0
				return m, nil
			case "p":
				m.detailRaw = false
				m.detailScroll = 0
				return m, nil
			case "esc":
				m.details = false
				return m, nil
			case "up", "k":
				m.detailScroll = max(0, m.detailScroll-1)
				return m, nil
			case "down", "j":
				m.detailScroll++
				return m, nil
			case "left", "h":
				m.detailEvidence = max(0, m.detailEvidence-1)
				m.detailScroll = 0
				m.detailRawOffset = 0
				return m, nil
			case "right", "l":
				m.detailEvidence = min(7, m.detailEvidence+1)
				m.detailScroll = 0
				m.detailRawOffset = 0
				return m, nil
			case "pgup":
				if m.detailRaw {
					m.detailRawOffset = max(0, m.detailRawOffset-detailChunkBytes)
					m.detailScroll = 0
				} else {
					m.detailScroll = max(0, m.detailScroll-m.height/2)
				}
				return m, nil
			case "pgdown":
				if m.detailRaw {
					m.detailRawOffset += detailChunkBytes
					m.detailScroll = 0
				} else {
					m.detailScroll += m.height / 2
				}
				return m, nil
			}
		}
		switch msg.String() {
		case "q", "ctrl+c":
			m.cancel()
			return m, tea.Sequence(m.closeSource(), tea.Quit)
		case "ctrl+r":
			return m, m.requestHistory(0)
		case "ctrl+f":
			m.followLatestSelection()
		case "[", "]":
			direction := -1
			if msg.String() == "]" {
				direction = 1
			}
			return m, m.navigateTurn(direction)
		case " ":
			if m.followLatest {
				m.followLatest = false
			} else {
				m.followLatestSelection()
			}
		case "tab":
			m.pane = (m.pane + 1) % 5
			m.scroll = 0
			m.details = false
		case "1":
			m.pane = PaneFlow
			m.scroll = 0
		case "2":
			m.pane = PaneApp
			m.scroll = 0
		case "3":
			m.pane = PaneTimeline
			m.scroll = 0
		case "4":
			m.pane = PaneEvents
			m.scroll = 0
		case "5":
			m.pane = PaneMessages
			m.scroll = 0
		case "up", "k":
			m.followLatest = false
			m.scroll = max(0, m.scroll-1)
			m.eventRow = max(0, m.eventRow-1)
		case "down", "j":
			m.followLatest = false
			m.scroll++
			m.eventRow++
		case "pgup":
			m.followLatest = false
			m.scroll = max(0, m.scroll-max(1, m.height-10))
		case "pgdown":
			m.followLatest = false
			m.scroll += max(1, m.height-10)
		case "enter":
			if m.pane == PaneApp || m.pane == PaneFlow {
				groups := m.store.groups()
				if len(groups) > 0 {
					m.cancelHistorySelection()
					m.selectScope(groups[min(m.eventRow, len(groups)-1)].Key, m.store)
					m.followLatest = false
					m.scroll = 0
				}
				break
			}
			m.details = !m.details
			if m.details {
				m.openObserverDetails()
			}
		}
	}
	return m, nil
}
func traceMajor(schema string) int {
	at := strings.LastIndex(schema, "/v")
	if at < 0 {
		return -1
	}
	version := strings.SplitN(schema[at+2:], ".", 2)[0]
	n, err := strconv.Atoi(version)
	if err != nil {
		return -1
	}
	return n
}
func observerClean(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || unicode.IsPrint(r) {
			return r
		}
		return -1
	}, text)
}
func observerClip(text string, width int) string {
	if width < 2 {
		return ""
	}
	var out strings.Builder
	out.Grow(min(len(text), width*4))
	truncated := false
	// Bound traversal too: zero-width/control runs must not scan an entire
	// retained message just to render a single terminal row.
	for index, r := range text {
		if index >= width*16 {
			truncated = true
			break
		}
		if r == '\n' || r == '\t' {
			r = ' '
		}
		if !unicode.IsPrint(r) {
			continue
		}
		out.WriteRune(r)
	}
	clean := out.String()
	if truncated || lipgloss.Width(clean) > width {
		return lipgloss.NewStyle().MaxWidth(width-1).Render(clean) + "…"
	}
	return clean
}

func (m *ObserverModel) selectedRecords() []*observedRecord {
	if m.page != nil {
		return m.page.selected(m.selected)
	}
	return m.store.selected(m.selected)
}
func (m *ObserverModel) View() string {
	width := min(240, max(30, m.width))
	height := min(100, max(12, m.height))
	cyan := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	lines := []string{cyan.Render(observerClip("agent-stream-dbg · observe · "+m.sourceLifecycle()+" · "+m.status, width)), observerClip(m.path, width), observerClip(fmt.Sprintf("capture %s · runtime %s · session %s", m.captureID, m.runtimeID, m.sessionID), width)}
	if m.schemaNotice != "" {
		lines = append(lines, observerClip(m.schemaNotice, width))
	}
	if m.err != nil {
		lines = append(lines, observerClip(m.err.Error(), width))
	}
	if m.health != "" {
		lines = append(lines, observerClip(m.health, width))
	}
	if len(m.store.issues) > 0 {
		lines = append(lines, observerClip("! "+m.store.issues[len(m.store.issues)-1], width))
	}
	groups := m.store.groups()
	labelGroups := groups
	if m.page != nil {
		labelGroups = append(labelGroups, m.page.groups()...)
	}
	label := m.selected
	for _, g := range labelGroups {
		if g.Key == m.selected {
			label = fmt.Sprintf("%s · response:%s · persona:%s", g.Label, g.Response, g.Persona)
		}
	}
	follow := "selection paused"
	if m.followLatest {
		follow = "following latest"
	}
	lines = append(lines, observerClip(label+" · "+follow, width))
	paneNames := []string{"1 Flow", "2 App", "3 Timeline", "4 Events", "5 Messages"}
	for i := range paneNames {
		if Pane(i) == m.pane {
			paneNames[i] = "[" + paneNames[i] + "]"
		}
	}
	lines = append(lines, strings.Join(paneNames, "  "), strings.Repeat("─", width))
	records := m.selectedRecords()
	windowed := false
	availableRows := max(1, height-len(lines)-3)
	body := make([]string, 0)
	switch {
	case m.details:
		windowed = true
		body = m.renderDetails(width, availableRows)

	case m.pane == PaneFlow || m.pane == PaneApp:
		for _, g := range groups {
			mark := " "
			if g.Key == m.selected {
				mark = ">"
			}
			body = append(body, fmt.Sprintf("%s %s · %d events · response %s", mark, g.Label, g.Count, g.Response))
		}
	case m.pane == PaneTimeline:
		windowed = true
		if !m.intervalsEnabled {
			body = []string{"Interval interpretation disabled; Events and raw details remain available"}
			break
		}
		body = renderObserverIntervals(m.IntervalReport(), width, m.scroll, availableRows)

	case m.pane == PaneMessages:
		windowed = true
		filtered := make([]*observedRecord, 0, len(records))
		for _, r := range records {
			if r.Event.Content != "" {
				filtered = append(filtered, r)
			}
		}
		start := min(m.scroll, max(0, len(filtered)-availableRows))
		for _, r := range filtered[start:min(len(filtered), start+availableRows)] {
			body = append(body, observerClip(r.Event.Name, 64)+": "+observerClip(r.Event.Content, width))
		}
	default:
		windowed = true
		start := min(m.scroll, max(0, len(records)-availableRows))
		for i, r := range records[start:min(len(records), start+availableRows)] {
			i += start
			mark := " "
			if i == m.eventRow {
				mark = ">"
			}
			body = append(body, fmt.Sprintf("%s #%d %s · %s %s", mark, r.Event.Seq, observerSourceTime(r.Event, false), observerClip(r.Event.Name, 64), observerClip(r.Event.Content, width)))
			if r.Issue != "" {
				body = append(body, "  ! "+r.Issue)
			}
		}
	}
	if len(body) == 0 {
		body = append(body, "Waiting for source evidence; ingestion requires no prompt")
	}
	if m.store.evicted {
		body = append(body, "Earlier history on disk")
	}
	for _, issue := range m.store.issues {
		body = append(body, "! "+issue)
	}
	available := max(1, height-len(lines)-3)
	start := min(m.scroll, max(0, len(body)-available))
	if windowed {
		start = 0
	}
	for _, line := range body[start:min(len(body), start+available)] {
		lines = append(lines, observerClip(line, width))
	}
	for len(lines) < height-2 {
		lines = append(lines, "")
	}
	pageCount, pageBytes := 0, 0
	if m.page != nil {
		pageCount = len(m.page.records)
		pageBytes = m.page.bytes
	}
	lines = append(lines, dim.Render(observerClip(fmt.Sprintf("%d live + %d page events · %d retained bytes · read-only", len(m.store.records), pageCount, m.store.bytes+pageBytes), width)), dim.Render(observerClip("[ ] turn  Ctrl+R reload  Ctrl+F latest  Space pause/follow  Tab panes  Enter details  q quit", width)))
	return strings.Join(lines, "\n")
}

// ManifestCaptureID reads only the bounded identity needed to validate a bundle.
func ManifestCaptureID(data []byte) (string, error) {
	if len(data) > 64<<10 {
		return "", fmt.Errorf("manifest exceeds64KiB")
	}
	var v struct {
		CaptureID string `json:"capture_id"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return "", fmt.Errorf("invalid capture manifest: %w", err)
	}
	if len(v.CaptureID) > 256 {
		return "", fmt.Errorf("manifest capture identity exceeds256bytes")
	}
	return v.CaptureID, nil
}

func finiteNumber(value any) (float64, bool) {
	n, ok := value.(float64)
	return n, ok && !math.IsNaN(n) && !math.IsInf(n, 0)
}

func observerNumber(value any) string {
	if n, ok := finiteNumber(value); ok {
		return strconv.FormatFloat(n, 'f', 3, 64)
	}
	return "unavailable"
}

func traceFamily(schema string) string {
	at := strings.LastIndex(schema, "/v")
	if at < 0 {
		return ""
	}
	return schema[:at]
}

func observerSourceTime(event *events.Event, full bool) string {
	if !event.SourceTimestampKnown {
		return "source time unavailable"
	}
	if full {
		return event.Timestamp.Format(time.RFC3339Nano)
	}
	return event.Timestamp.Format("15:04:05.000")
}

// Source lifecycle is independent of transient navigation/loading notices.
func (m *ObserverModel) sourceLifecycle() string {
	if m.sealed {
		return "sealed"
	}
	if m.sourceEnded {
		return "unsealed / stopped"
	}
	if !m.attached {
		return "attaching"
	}
	if !m.ingesting {
		return "EOF / unsealed"
	}
	if m.following {
		return "following source"
	}
	return "reading saved source"
}
