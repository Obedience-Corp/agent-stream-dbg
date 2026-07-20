package visualizer

import (
	"fmt"
	neturl "net/url"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lancekrogers/stream-debugger/internal/client"
	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/transport"
)

// urlQueryEscape safely escapes a message for URL query use
func urlQueryEscape(s string) string { return neturl.QueryEscape(s) }

// startStreamingCmd initiates the shared configured transport and hands off to
// the frame reader. The dialect send template is rendered by client.NewTransport;
// this model only adapts raw transport frames into Bubble Tea messages.
//
// Sessionful transports (ACP) are reused across turns: the first message
// Connects, later messages call Prompt on the same process/session.
func (m InteractiveModel) startStreamingCmd(message string, index int) tea.Cmd {
	existing := m.streamTransport
	return func() tea.Msg {
		// Reuse a live session transport (e.g. ACP stdio) for multi-turn chat.
		if existing != nil {
			if st, ok := existing.(transport.SessionTransport); ok && st.Alive() {
				if err := st.Prompt(m.streamContext, message); err != nil {
					_ = existing.Close()
					return streamErrorMsg{err: fmt.Errorf("failed to prompt session transport: %w", err)}
				}
				return streamStartMsg{index: index, stream: existing, sessionReady: false, reused: true}
			}
			// Stale or non-session transport: drop before opening a new one.
			_ = existing.Close()
		}

		setupCompleted := false
		if m.cfg.Session.AutoSetup && !m.sessionReady {
			vars := config.InterpolationVarsFromConfig(m.cfg)
			sessionID, err := m.parser.RunSetup(m.streamContext, vars, m.cfg.Transport.ResolvedHeaders(), nil)
			if err != nil {
				return streamErrorMsg{err: fmt.Errorf("failed to setup session: %w", err)}
			}
			if sessionID != "" {
				m.cfg.Session.ID = sessionID
			}
			setupCompleted = true
		}
		tr, err := client.NewTransport(m.cfg, m.parser, message)
		if err != nil {
			return streamErrorMsg{err: fmt.Errorf("failed to build stream transport: %w", err)}
		}
		if err := tr.Connect(m.streamContext); err != nil {
			_ = tr.Close()
			return streamErrorMsg{err: fmt.Errorf("failed to connect stream transport: %w", err)}
		}
		return streamStartMsg{index: index, stream: tr, sessionReady: setupCompleted}
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
	index        int
	stream       transport.Transport
	sessionReady bool
	// reused is true when an existing SessionTransport was prompted again
	// instead of Connect-ing a new process.
	reused bool
}

// turnEnded reports whether a decoded event marks the end of a user turn
// for a sessionful transport (keep process; stop "streaming" UI state).
func turnEnded(evt *events.Event) bool {
	if evt == nil {
		return false
	}
	return evt.Kind == events.KindStreamEnd || evt.Kind == events.KindSessionEnd
}

// streamFrameMsg carries one transport frame from the streaming response.
type streamFrameMsg struct {
	index int
	frame transport.Frame
	eof   bool
}

// newSessionCmd generates a fresh session id and performs setup. The returned
// message lets Update apply both the reset and any setup error to the live
// model instead of mutating a command's value-receiver copy.
func (m InteractiveModel) newSessionCmd() tea.Cmd {
	return func() tea.Msg {
		// Generate new session id
		newID := fmt.Sprintf("debug-session-%s", time.Now().Format("20060102-150405"))
		m.cfg.Session.ID = newID

		// Call session setup (auto create)
		vars := config.InterpolationVarsFromConfig(m.cfg)
		sessionID, err := m.parser.RunSetup(m.streamContext, vars, m.cfg.Transport.ResolvedHeaders(), nil)
		if err != nil {
			return sessionSetupMsg{
				sessionID: newID,
				err:       fmt.Errorf("session setup failed: %w", err),
			}
		}
		return sessionSetupMsg{sessionID: sessionID}
	}
}

type sessionSetupMsg struct {
	sessionID string
	err       error
}
