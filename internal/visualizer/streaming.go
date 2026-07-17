package visualizer

import (
	"context"
	"fmt"
	neturl "net/url"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lancekrogers/stream-debugger/internal/client"
	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	dblogger "github.com/lancekrogers/stream-debugger/internal/logger"
	"github.com/lancekrogers/stream-debugger/internal/transport"
)

// urlQueryEscape safely escapes a message for URL query use
func urlQueryEscape(s string) string { return neturl.QueryEscape(s) }

// startStreamingCmd initiates the shared configured transport and hands off to
// the frame reader. The dialect send template is rendered by client.NewTransport;
// this model only adapts raw transport frames into Bubble Tea messages.
func (m InteractiveModel) startStreamingCmd(message string, index int) tea.Cmd {
	return func() tea.Msg {
		tr, err := client.NewTransport(m.cfg, m.parser, message)
		if err != nil {
			return streamErrorMsg{err: fmt.Errorf("failed to build stream transport: %w", err)}
		}
		if err := tr.Connect(context.Background()); err != nil {
			_ = tr.Close()
			return streamErrorMsg{err: fmt.Errorf("failed to connect stream transport: %w", err)}
		}
		return streamStartMsg{index: index, stream: tr}
	}
}

// readStreamFrameCmd reads one complete raw frame from the active transport.
// The transport owns wire parsing and lifecycle; the TUI only schedules one
// frame at a time so its existing tea.Cmd update pattern remains intact.
func (m InteractiveModel) readStreamFrameCmd() tea.Cmd {
	tr := m.streamTransport
	idx := m.streamIndex
	return func() tea.Msg {
		if tr == nil {
			return streamFrameMsg{index: idx, eof: true}
		}
		frame, ok := <-tr.Frames()
		if !ok {
			return streamFrameMsg{index: idx, eof: true}
		}
		return streamFrameMsg{index: idx, frame: frame}
	}
}

// closeActiveStream is safe to call from normal completion, error handling,
// and the quit path. Transport.Close guarantees the frame channel is closed
// and its reader has stopped before returning.
func (m *InteractiveModel) closeActiveStream() {
	if m.streamTransport != nil {
		_ = m.streamTransport.Close()
		m.streamTransport = nil
	}
}

// Message types
type streamCompleteMsg struct {
	index          int
	rawSSE         string
	events         []*events.Event
	agentResponses map[string]*AgentResponse
}

type streamErrorMsg struct {
	err error
}

// streamStartMsg signals beginning of incremental streaming
type streamStartMsg struct {
	index  int
	stream transport.Transport
}

// streamFrameMsg carries one transport frame from the streaming response.
type streamFrameMsg struct {
	index int
	frame transport.Frame
	eof   bool
}

// newSessionCmd generates a fresh session id, sets it, calls setup, and resets state
func (m InteractiveModel) newSessionCmd() tea.Cmd {
	return func() tea.Msg {
		// Generate new session id
		newID := fmt.Sprintf("debug-session-%s", time.Now().Format("20060102-150405"))
		m.cfg.Session.ID = newID

		// Recreate logger for new session
		if m.slog != nil {
			_ = m.slog.Close()
		}
		if l, err := dblogger.NewStructuredLogger(m.cfg); err == nil {
			m.slog = l
		}

		// Call session setup (auto create)
		vars := config.InterpolationVarsFromConfig(m.cfg)
		if sessionID, err := m.parser.RunSetup(context.Background(), vars, m.cfg.Transport.ResolvedHeaders(), nil); err == nil {
			// Ensure we track the actual backend session id
			m.cfg.Session.ID = sessionID
		}

		// Reset UI state (clear all prior content and counters)
		m.messages = make([]Message, 0)
		m.err = nil
		m.flowTurnIndex = 0
		m.flowContinuous = true
		m.selectedStepIndex = 0
		m.flowExpanded = make(map[string]bool)
		m.showTokens = false
		m.eventsAggregatorOnly = false
		m.appFocus = AppFocusAgents
		m.viewport.SetContent("")
		m.contentDirty = true
		m.refreshViewportContent()
		return nil
	}
}
