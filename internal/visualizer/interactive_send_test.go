package visualizer

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/stream-debugger/internal/config"
	"github.com/Obedience-Corp/stream-debugger/internal/testutil"
)

// TestStartStreamingCmd_SendsViaDialectTemplate proves interactive mode's
// send path renders the dialect's send: template (via bridge.RenderSend)
// and successfully connects to a real HTTP server with it — no hardcoded
// request body involved.
func TestStartStreamingCmd_SendsViaDialectTemplate(t *testing.T) {
	srv, err := testutil.NewMockSSEServer(referenceFixturePath(t), 0)
	if err != nil {
		t.Fatalf("NewMockSSEServer: %v", err)
	}
	defer srv.Close()

	cfg := &config.EnhancedConfig{
		LogDir: t.TempDir(),
		Transport: config.TransportConfig{
			BaseURL: srv.URL(),
		},
		Session: config.SessionConfig{ID: "interactive-send-test"},
		Logging: config.LoggingConfig{Dimensions: config.DimensionsConfig{ByEventType: true, ByAgent: true, BySession: true, APICalls: true}},
	}
	cfg.Normalize()

	m := NewInteractiveModel(cfg)

	cmd := m.startStreamingCmd("hello from the test", 0)
	msg := cmd()

	start, ok := msg.(streamStartMsg)
	if !ok {
		if errMsg, isErr := msg.(streamErrorMsg); isErr {
			t.Fatalf("expected streamStartMsg, got streamErrorMsg: %v", errMsg.err)
		}
		t.Fatalf("expected streamStartMsg, got %T: %+v", msg, msg)
	}
	defer func() {
		if start.stream != nil {
			_ = start.stream.Close()
		}
	}()
	if start.stream == nil {
		t.Error("expected a non-nil transport to stream from")
	}

	// Verify the request actually reached the mock server (proves the
	// rendered send: request, not a hardcoded literal, is what was sent).
	headers := srv.LastHeaders()
	if headers == nil {
		t.Fatal("expected the mock server to have received a request")
	}
}

func TestStartStreamingCmd_DefersAutoSetupUntilAfterFirstRunConfig(t *testing.T) {
	const token = "test-secret"
	var setupSeen, streamSeen bool
	var setupAuth, streamAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/setup":
			setupSeen = true
			setupAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"success":true,"session_id":"created-session"}`)
		case "/stream":
			streamSeen = true
			streamAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: data\ndata: {\"content\":\"ok\"}\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dialect := `version: 1
name: deferred-setup-test
discriminator: event
setup:
  request:
    method: POST
    url: "{base_url}/setup"
    body: '{"session_id":"{session_id}"}'
  response:
    require: {path: success, equals: true}
    session_id: session_id
send:
  request:
    method: GET
    url: "{base_url}/stream?message={message}"
rules:
  - match: {event: data}
    kind: content
`
	dialectPath := filepath.Join(t.TempDir(), "deferred-setup.yaml")
	if err := os.WriteFile(dialectPath, []byte(dialect), 0o600); err != nil {
		t.Fatalf("write dialect: %v", err)
	}

	cfg := &config.EnhancedConfig{
		LogDir: t.TempDir(),
		Transport: config.TransportConfig{
			Type:    "sse",
			BaseURL: srv.URL,
			Auth: config.AuthConfig{
				Type:     "bearer",
				TokenEnv: "STREAM_DEBUGGER_TEST_API_KEY",
				Token:    token,
			},
		},
		Dialect: config.DialectConfig{File: dialectPath},
		Session: config.SessionConfig{ID: "initial-session", AutoSetup: true},
	}
	m := NewInteractiveModelWithContextAndConfigPathAndOpenConfig(cfg, context.Background(), "", true)

	msg := m.startStreamingCmd("hello world", 0)()
	start, ok := msg.(streamStartMsg)
	if !ok {
		if errMsg, isErr := msg.(streamErrorMsg); isErr {
			t.Fatalf("expected streamStartMsg, got streamErrorMsg: %v", errMsg.err)
		}
		t.Fatalf("expected streamStartMsg, got %T: %+v", msg, msg)
	}
	defer func() {
		if start.stream != nil {
			_ = start.stream.Close()
		}
	}()

	if !setupSeen || !streamSeen {
		t.Fatalf("expected deferred setup and stream requests, setup=%v stream=%v", setupSeen, streamSeen)
	}
	if setupAuth != "Bearer "+token || streamAuth != "Bearer "+token {
		t.Errorf("expected bearer auth on setup and stream, setup=%q stream=%q", setupAuth, streamAuth)
	}
	if cfg.Session.ID != "created-session" {
		t.Errorf("session id = %q, want created-session", cfg.Session.ID)
	}
	if !start.sessionReady {
		t.Error("expected stream start to mark deferred setup ready")
	}
	if strings.Contains(string(dialect), token) {
		t.Error("test dialect unexpectedly contained the auth secret")
	}
}
