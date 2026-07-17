package visualizer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	dblogger "github.com/lancekrogers/stream-debugger/internal/logger"
)

// urlQueryEscape safely escapes a message for URL query use
func urlQueryEscape(s string) string { return neturl.QueryEscape(s) }

// startStreamingCmd initiates the streaming request and hands off to chunk reader
func (m InteractiveModel) startStreamingCmd(message string, index int) tea.Cmd {
	return func() tea.Msg {
		vars := config.InterpolationVarsFromConfig(m.cfg)
		vars.Message = message
		method, sendURL, sendBody, err := m.parser.RenderSend(vars)
		if err != nil {
			return streamErrorMsg{err: fmt.Errorf("failed to render send request: %w", err)}
		}
		// Pass through stream debug level if set (enables flow_step_detail synthesis output)
		if lvl := m.cfg.Debug.Level; lvl != "" {
			sendURL += "&debug=" + urlQueryEscape(lvl)
		}

		var bodyReader io.Reader
		if sendBody != nil {
			bodyReader = bytes.NewReader(sendBody)
		}
		req, err := http.NewRequest(method, sendURL, bodyReader)
		if err != nil {
			return streamErrorMsg{err: fmt.Errorf("failed to create request: %w", err)}
		}
		if sendBody != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range m.cfg.Transport.ResolvedHeaders() {
			req.Header.Set(k, v)
		}
		req.Header.Set("Accept", "text/event-stream")

		clientHTTP := &http.Client{} // no timeout; SSE is long-lived
		ctx, cancel := context.WithCancel(context.Background())
		req = req.WithContext(ctx)

		resp, err := clientHTTP.Do(req)
		if err != nil {
			cancel() // Cancel context on error to avoid leak
			return streamErrorMsg{err: fmt.Errorf("failed to send request: %w", err)}
		}
		if resp.StatusCode != http.StatusOK {
			cancel() // Cancel context on error to avoid leak
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return streamErrorMsg{err: fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))}
		}
		return streamStartMsg{index: index, body: resp.Body, cancel: cancel}
	}
}

// readStreamChunkCmd reads the next chunk from the active stream and returns a chunk message
func (m InteractiveModel) readStreamChunkCmd() tea.Cmd {
	// capture the current ReadCloser
	r := m.streamBody
	idx := m.streamIndex
	return func() tea.Msg {
		if r == nil {
			return streamChunkMsg{index: idx, eof: true}
		}
		buf := make([]byte, 4096)
		n, err := r.Read(buf)
		if err != nil {
			if err == io.EOF {
				return streamChunkMsg{index: idx, chunk: buf[:n], eof: true}
			}
			return streamChunkMsg{index: idx, err: err}
		}
		return streamChunkMsg{index: idx, chunk: buf[:n]}
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
	body   io.ReadCloser
	cancel context.CancelFunc
}

// streamChunkMsg carries data chunk from the streaming response
type streamChunkMsg struct {
	index int
	chunk []byte
	eof   bool
	err   error
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
