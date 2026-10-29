package visualizer

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
	tea "github.com/charmbracelet/bubbletea"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type observerConnected struct{ err error }
type observerFrame struct {
	frame transport.Frame
	ok    bool
}

// ObserverModel is the passive mode of the visualizer. It has no prompt,
// session setup, logger, control client, or source write capability.
type ObserverModel struct {
	animationStarted                                              bool
	flowExpanded                                                  map[string]bool
	agentCollapsed                                                map[string]bool
	selectingScope                                                bool
	animFrame                                                     int
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
	return &ObserverModel{source: tr, parser: parser, path: path, following: follow, ctx: ctx, cancel: cancel, store: newObserverStore(), followLatest: true, pane: PaneFlow, width: 100, height: 30, status: "attaching", manifestCaptureID: manifestCaptureID, intervalsEnabled: false, flowExpanded: make(map[string]bool), agentCollapsed: make(map[string]bool)}
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
	case animTickMsg:
		m.animFrame++
		return m, animTickCmd()
	case *observerHistoryResult:
		return m, m.historyResult(msg)
	case tea.WindowSizeMsg:
		m.width = max(30, msg.Width)
		m.height = max(12, msg.Height)
		if !m.animationStarted {
			m.animationStarted = true
			return m, animTickCmd()
		}
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
				if m.source.Name() == "sse" {
					m.status = "Disconnected · conversation ended or runtime changed"
				}
			}
			return m, nil
		}
		frame := msg.frame
		if m.source.Name() == "sse" {
			frame.Offset = m.store.maxSourceEnd
			frame.Size = int64(max(1, len(frame.Raw)))
		}
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
			m.health = fmt.Sprintf("lost:%v omitted:%v scope-excluded:%v errors:%v written:%v synced:%v", observerNumber(health["dropped"]), observerNumber(health["skipped"]), observerNumber(health["scope_excluded"]), observerNumber(health["write_errors"]), observerNumber(health["last_written_seq"]), observerNumber(health["last_synced_seq"]))
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
			m.eventRow = 0
			m.details = false
		case "1":
			m.pane = PaneFlow
			m.scroll = 0
			m.eventRow = 0
		case "2":
			m.pane = PaneApp
			m.scroll = 0
			m.eventRow = 0
		case "3":
			m.pane = PaneTimeline
			m.scroll = 0
			m.eventRow = 0
		case "4":
			m.pane = PaneEvents
			m.scroll = 0
			m.eventRow = 0
		case "5":
			m.pane = PaneMessages
			m.scroll = 0
			m.eventRow = 0
		case "up", "k":
			m.movePassiveSelection(-1)
		case "down", "j":
			m.movePassiveSelection(1)
		case "pgup":
			m.movePassiveSelection(-max(1, m.height-10))
		case "pgdown":
			m.movePassiveSelection(max(1, m.height-10))
		case "s":
			m.selectingScope = !m.selectingScope
			m.eventRow = 0
			m.scroll = 0
		case "d":
			m.followLatest = false
			m.details = true
			m.openObserverDetails()
		case "enter":
			m.followLatest = false
			if !m.selectingScope && (m.pane == PaneApp || m.pane == PaneFlow) {
				m.togglePassiveRow()
				break
			}
			if m.selectingScope {
				groups := m.store.groups()
				if len(groups) > 0 {
					m.cancelHistorySelection()
					m.selectScope(groups[min(m.eventRow, len(groups)-1)].Key, m.store)
					m.followLatest = false
					m.selectingScope = false
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
