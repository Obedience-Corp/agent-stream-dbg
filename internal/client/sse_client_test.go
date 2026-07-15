package client

import (
	"context"
	"testing"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/testutil"
)

// TestSSEClient_EndToEndAgainstMockServer drives the real SSE client against
// the mock SSE server replaying the recorded demo fixture, proving the two
// integrate correctly offline (no backend, key, or network required).
func TestSSEClient_EndToEndAgainstMockServer(t *testing.T) {
	srv, err := testutil.NewMockSSEServer("../../testdata/fixtures/brainyard-session.jsonl", 0)
	if err != nil {
		t.Fatalf("NewMockSSEServer: %v", err)
	}
	defer srv.Close()

	cfg := &config.EnhancedConfig{
		Backend: config.BackendConfig{
			BaseURL:        srv.URL(),
			StreamEndpoint: "",
		},
		APIKey:  "test-key",
		Session: config.SessionConfig{ID: "e2e-session"},
	}
	cfg.Normalize()
	c := NewSSEClient(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Connect(ctx, "hello"); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// 12 concurrent per-event-type connections race independently, so collect
	// until the stream has been quiet for a bit rather than stopping on the
	// first session_complete seen (which offers no ordering guarantee).
	seenTypes := map[events.EventType]bool{}
	seenAgents := map[string]bool{}
	idle := time.NewTimer(300 * time.Millisecond)
	defer idle.Stop()

collect:
	for {
		select {
		case evt, ok := <-c.Events():
			if !ok {
				break collect
			}
			seenTypes[evt.Type] = true
			if agentID := evt.GetAgentID(); agentID != "" {
				seenAgents[agentID] = true
			}
			if !idle.Stop() {
				<-idle.C
			}
			idle.Reset(300 * time.Millisecond)
		case err := <-c.Errors():
			t.Fatalf("unexpected client error: %v", err)
		case <-idle.C:
			break collect
		case <-ctx.Done():
			t.Fatalf("timed out collecting events; saw types: %v", seenTypes)
		}
	}

	wantTypes := []events.EventType{
		events.SessionStart,
		events.AgentContent,
		events.WizardContent,
		events.Error,
		events.SessionComplete,
	}
	for _, wt := range wantTypes {
		if !seenTypes[wt] {
			t.Errorf("expected to see event type %q, did not", wt)
		}
	}

	wantAgents := []string{"sam_harris", "eckhart_tolle"}
	for _, wa := range wantAgents {
		if !seenAgents[wa] {
			t.Errorf("expected to see agent %q, did not", wa)
		}
	}
}
