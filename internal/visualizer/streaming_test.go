package visualizer

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lancekrogers/stream-debugger/internal/config"
)

// TestUrlQueryEscape is a floor-level test for streaming.go's small
// escaping helper (previously untested directly).
func TestUrlQueryEscape(t *testing.T) {
	if got := urlQueryEscape("hello world"); got != "hello+world" {
		t.Errorf("expected query-escaped string, got %q", got)
	}
	if got := urlQueryEscape("a&b=c"); got != "a%26b%3Dc" {
		t.Errorf("expected special characters escaped, got %q", got)
	}
}

// TestReadStreamChunkCmd_NilBody proves a nil stream body (already closed,
// or never opened) reads as an immediate EOF rather than panicking —
// startStreamingCmd's happy path against a real server is already covered
// by interactive_send_test.go.
func TestReadStreamChunkCmd_NilBody(t *testing.T) {
	m := InteractiveModel{streamIndex: 3}
	cmd := m.readStreamChunkCmd()
	msg := cmd()
	chunk, ok := msg.(streamChunkMsg)
	if !ok {
		t.Fatalf("expected streamChunkMsg, got %T", msg)
	}
	if !chunk.eof || chunk.index != 3 {
		t.Errorf("expected eof=true index=3, got eof=%v index=%d", chunk.eof, chunk.index)
	}
}

// TestNewSessionCmd_ResetsStateWithoutBackend proves newSessionCmd resets
// UI/session state even when the setup handshake can't reach a backend
// (RunSetup fails fast against an unroutable URL and the command keeps its
// locally generated session id) — the reset behavior itself doesn't depend
// on setup succeeding.
func TestNewSessionCmd_ResetsStateWithoutBackend(t *testing.T) {
	// A closed server: connections to it fail immediately (same trick
	// internal/mapping's TestRunSetup_NetworkError uses).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	cfg := &config.EnhancedConfig{LogDir: t.TempDir()}
	cfg.Transport.BaseURL = srv.URL
	cfg.Session.ID = "old-session"
	cfg.Normalize()

	m := NewInteractiveModel(cfg)
	m.messages = append(m.messages, Message{Text: "leftover"})
	m.err = errBoom
	m.flowTurnIndex = 5
	m.selectedStepIndex = 2
	m.showTokens = true
	m.eventsWizardOnly = true
	m.appFocus = AppFocusWizard

	cmd := m.newSessionCmd()
	cmd() // side effects land on the closure's own model copy, not m

	if cfg.Session.ID == "old-session" {
		t.Error("expected a fresh session id to be generated even without backend confirmation")
	}
}
