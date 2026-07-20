package visualizer

import (
	"strings"
	"testing"

	"github.com/Obedience-Corp/stream-debugger/internal/config"
	"github.com/Obedience-Corp/stream-debugger/internal/events"
)

// stripANSI removes lipgloss's ANSI escape codes so rendered output can
// be checked for plain-text substrings.
func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TestRenderHandoffEdge_ThreeModesAreDistinguishable is this task's
// explicit Done-When: spawn/handoff/delegate must render distinguishably
// from one another, not just all fall back to the same generic edge.
func TestRenderHandoffEdge_ThreeModesAreDistinguishable(t *testing.T) {
	base := func(mode string) *events.Event {
		return &events.Event{
			Kind:   events.KindHandoff,
			Fields: map[string]any{"from": "router", "to": "worker_a", "mode": mode},
		}
	}

	spawn := stripANSI(renderHandoffEdge(base("spawn")))
	handoff := stripANSI(renderHandoffEdge(base("handoff")))
	delegate := stripANSI(renderHandoffEdge(base("delegate")))

	if spawn == handoff || spawn == delegate || handoff == delegate {
		t.Fatalf("expected all three modes to render distinctly, got:\nspawn=%q\nhandoff=%q\ndelegate=%q", spawn, handoff, delegate)
	}

	// Each must still name the same from/to lanes.
	for name, out := range map[string]string{"spawn": spawn, "handoff": handoff, "delegate": delegate} {
		if !strings.Contains(out, "router") || !strings.Contains(out, "worker_a") {
			t.Errorf("mode %s: expected both lane names present, got %q", name, out)
		}
	}

	// spawn implies a NEW lane branching off — distinct tree-like marker.
	if !strings.Contains(spawn, "spawn") {
		t.Errorf("expected spawn's rendering to mention 'spawn', got %q", spawn)
	}
	// delegate implies from resumes — distinct return marker, absent from
	// the other two modes.
	if !strings.Contains(delegate, "return") {
		t.Errorf("expected delegate's rendering to mention 'return' (the call/return distinction from a plain handoff), got %q", delegate)
	}
	if strings.Contains(handoff, "return") || strings.Contains(spawn, "return") {
		t.Error("expected only delegate to mention 'return' — handoff and spawn must not imply resumption")
	}
}

// TestRenderHandoffEdge_UnrecognizedModeFallsBackGracefully proves a
// malformed/missing mode doesn't panic or render blank — a debugger
// showing an approximate edge beats showing nothing.
func TestRenderHandoffEdge_UnrecognizedModeFallsBackGracefully(t *testing.T) {
	evt := &events.Event{Kind: events.KindHandoff, Fields: map[string]any{"from": "a", "to": "b", "mode": "something-unexpected"}}
	out := stripANSI(renderHandoffEdge(evt))
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") {
		t.Errorf("expected the fallback rendering to still show both lanes, got %q", out)
	}
}

// TestEventsPane_HandoffRendersAsEdgeNotContent is this task's other
// explicit Done-When: a Kind: handoff event must never be treated as
// row-content for either the from or to lane (i.e. never populate
// AgentResponses for either), and must render via the dedicated edge
// path in the Events pane rather than falling through to generic
// per-event-name content rendering.
func TestEventsPane_HandoffRendersAsEdgeNotContent(t *testing.T) {
	m := NewInteractiveModel(&config.EnhancedConfig{})
	m.messages = append(m.messages, Message{})
	m.streamIndex = 0

	m.applyParsedEvent(&events.Event{
		Kind:   events.KindHandoff,
		Name:   "handoff",
		Fields: map[string]any{"from": "router", "to": "worker_a", "mode": "handoff"},
	})

	if ar := m.messages[0].AgentResponses["router"]; ar != nil {
		t.Errorf("expected the handoff's 'from' lane to have no AgentResponse row, got %+v", ar)
	}
	if ar := m.messages[0].AgentResponses["worker_a"]; ar != nil {
		t.Errorf("expected the handoff's 'to' lane to have no AgentResponse row, got %+v", ar)
	}
	if len(m.messages[0].Events) != 1 {
		t.Fatalf("expected the handoff event to still be recorded in Events, got %d", len(m.messages[0].Events))
	}

	out := stripANSI(m.renderEventsPane())
	if !strings.Contains(out, "router") || !strings.Contains(out, "worker_a") {
		t.Errorf("expected the Events pane to show the handoff edge (from/to lane names), got:\n%s", out)
	}
}
