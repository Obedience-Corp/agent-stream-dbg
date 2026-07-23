package tracectx

import (
	"net/http"
	"testing"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
)

func TestCorrelator_ObserveAndStamp_InheritTraceID(t *testing.T) {
	c := New(Options{Enabled: true, Mode: ModeObserve})
	c.ObserveHeaders(http.Header{
		"Traceparent": []string{"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
	})
	if !c.Current().Valid() {
		t.Fatal("expected session context")
	}
	ev := &events.Event{Name: "agent_content", Fields: map[string]any{}}
	c.StampEvent(ev)
	if ev.Fields[FieldTraceID] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("fields=%v", ev.Fields)
	}
	if _, ok := ev.Fields[FieldSpanID]; ok {
		t.Error("should not inherit foreign span_id onto content events")
	}
}

func TestCorrelator_Outbound_Generate(t *testing.T) {
	c := New(Options{Enabled: true, Mode: ModeGenerate})
	tp, ok := c.OutboundTraceparent()
	if !ok || tp == "" {
		t.Fatal("expected generated outbound traceparent")
	}
	if !c.Current().Valid() {
		t.Fatal("expected session context after generate")
	}
}

func TestCorrelator_Outbound_ObserveOnly(t *testing.T) {
	c := New(Options{Enabled: true, Mode: ModeObserve})
	if _, ok := c.OutboundTraceparent(); ok {
		t.Fatal("observe mode must not inject")
	}
}

func TestCorrelator_InjectHTTPHeaders_NoClobber(t *testing.T) {
	c := New(Options{Enabled: true, Mode: ModeGenerate})
	h := map[string]string{"traceparent": "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01"}
	c.InjectHTTPHeaders(h)
	if h["traceparent"] != "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01" {
		t.Errorf("clobbered existing header: %q", h["traceparent"])
	}
}

func TestCorrelator_StampEvent_PayloadWins(t *testing.T) {
	c := New(Options{Enabled: true})
	c.Observe(Context{TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", SpanID: "00f067aa0ba902b7", Source: "header"})
	ev := &events.Event{Fields: map[string]any{
		"trace_id": "cccccccccccccccccccccccccccccccc",
		"span_id":  "dddddddddddddddd",
	}}
	c.StampEvent(ev)
	if ev.Fields[FieldTraceID] != "cccccccccccccccccccccccccccccccc" {
		t.Errorf("expected payload trace_id, got %v", ev.Fields[FieldTraceID])
	}
	if ev.Fields[FieldSpanID] != "dddddddddddddddd" {
		t.Errorf("expected payload span_id, got %v", ev.Fields[FieldSpanID])
	}
}

func TestCorrelator_DisabledNoop(t *testing.T) {
	c := New(Options{Enabled: false, Mode: ModeGenerate})
	c.ObserveHeaders(http.Header{
		"Traceparent": []string{"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
	})
	if c.Current().Valid() {
		t.Fatal("disabled correlator must not observe")
	}
}
