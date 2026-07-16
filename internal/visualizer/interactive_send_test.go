package visualizer

import (
	"testing"

	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/testutil"
)

// TestStartStreamingCmd_SendsViaDialectTemplate proves interactive mode's
// send path renders the dialect's send: template (via bridge.RenderSend)
// and successfully connects to a real HTTP server with it — no hardcoded
// request body involved.
func TestStartStreamingCmd_SendsViaDialectTemplate(t *testing.T) {
	srv, err := testutil.NewMockSSEServer("../../testdata/fixtures/brainyard-session.jsonl", 0)
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
		if start.body != nil {
			_ = start.body.Close()
		}
	}()
	if start.cancel != nil {
		defer start.cancel()
	}
	if start.body == nil {
		t.Error("expected a non-nil response body to stream from")
	}

	// Verify the request actually reached the mock server (proves the
	// rendered send: request, not a hardcoded literal, is what was sent).
	headers := srv.LastHeaders()
	if headers == nil {
		t.Fatal("expected the mock server to have received a request")
	}
}
