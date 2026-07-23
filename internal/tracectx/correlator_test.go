package tracectx

import (
	"net/http"
	"strings"
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

func TestCorrelator_Outbound_MintsLocalSpanNotRemote(t *testing.T) {
	const remote = "00f067aa0ba902b7"
	c := New(Options{Enabled: true, Mode: ModeGenerate})
	c.ObserveHeaders(http.Header{
		"Traceparent": []string{"00-4bf92f3577b34da6a3ce929d0e0e4736-" + remote + "-00"},
	})
	tp1, ok := c.OutboundTraceparent()
	if !ok || tp1 == "" {
		t.Fatal("expected outbound traceparent")
	}
	parts := strings.Split(tp1, "-")
	if len(parts) != 4 {
		t.Fatalf("bad tp %q", tp1)
	}
	if parts[1] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace_id=%q", parts[1])
	}
	if parts[2] == remote {
		t.Error("outbound must mint a local span, not reuse remote parent span")
	}
	if parts[3] != "00" {
		t.Errorf("flags should preserve inbound 00, got %q", parts[3])
	}
	// Stable across calls.
	tp2, ok := c.OutboundTraceparent()
	if !ok || tp2 != tp1 {
		t.Errorf("outbound should be stable: %q vs %q", tp1, tp2)
	}
	cur := c.Current()
	if cur.RemoteSpan != remote {
		t.Errorf("RemoteSpan=%q want %q", cur.RemoteSpan, remote)
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

func TestCorrelator_InjectMetadata_NoClobber(t *testing.T) {
	c := New(Options{Enabled: true, Mode: ModeGenerate})
	m := map[string]string{"Traceparent": "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01"}
	c.InjectMetadata(m)
	if m["Traceparent"] != "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01" {
		t.Errorf("clobbered metadata: %v", m)
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

func TestCorrelator_ForcedSeedsSession(t *testing.T) {
	forced, err := ParseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if err != nil {
		t.Fatal(err)
	}
	forced.Source = "forced"
	c := New(Options{Enabled: true, Mode: ModeGenerate, Forced: forced})
	if c.Current().TraceID != forced.TraceID {
		t.Fatal("forced not applied")
	}
	tp, ok := c.OutboundTraceparent()
	if !ok {
		t.Fatal("expected outbound")
	}
	if strings.Contains(tp, "00f067aa0ba902b7") {
		t.Error("should not reuse forced remote span as local")
	}
}
