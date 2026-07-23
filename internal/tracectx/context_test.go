package tracectx

import (
	"net/http"
	"strings"
	"testing"
)

func TestParseTraceparent_OK(t *testing.T) {
	const raw = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	ctx, err := ParseTraceparent(raw)
	if err != nil {
		t.Fatalf("ParseTraceparent: %v", err)
	}
	if ctx.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace_id=%q", ctx.TraceID)
	}
	if ctx.SpanID != "00f067aa0ba902b7" {
		t.Errorf("span_id=%q", ctx.SpanID)
	}
	if got := ctx.Traceparent(); !strings.HasPrefix(got, "00-4bf92f3577b34da6a3ce929d0e0e4736-") {
		t.Errorf("Traceparent()=%q", got)
	}
}

func TestParseTraceparent_RejectsMalformed(t *testing.T) {
	cases := []string{
		"",
		"not-a-header",
		"00-short-00f067aa0ba902b7-01",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-short-01",
		"00-" + strings.Repeat("0", 32) + "-00f067aa0ba902b7-01",
		"ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}
	for _, c := range cases {
		if _, err := ParseTraceparent(c); err == nil {
			t.Errorf("expected error for %q", c)
		}
	}
}

func TestFromHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	h.Set("Tracestate", "rojo=00f067aa0ba902b7")
	ctx, ok := FromHeaders(h)
	if !ok {
		t.Fatal("expected ok")
	}
	if ctx.Source != "header" {
		t.Errorf("source=%q", ctx.Source)
	}
	if ctx.TraceState != "rojo=00f067aa0ba902b7" {
		t.Errorf("tracestate=%q", ctx.TraceState)
	}
}

func TestFromMap_TraceIDFallback(t *testing.T) {
	ctx, ok := FromMap(map[string]string{"trace-id": "4bf92f3577b34da6a3ce929d0e0e4736"})
	if !ok {
		t.Fatal("expected ok")
	}
	if ctx.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace_id=%q", ctx.TraceID)
	}
}

func TestFromFields_Payload(t *testing.T) {
	ctx, ok := FromFields(map[string]any{
		"trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
		"span_id":  "00f067aa0ba902b7",
	})
	if !ok {
		t.Fatal("expected ok")
	}
	if ctx.Source != "payload" {
		t.Errorf("source=%q", ctx.Source)
	}
}

func TestGenerate_Valid(t *testing.T) {
	ctx, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if !ctx.Valid() {
		t.Fatal("expected valid generated context")
	}
	if ctx.Source != "generated" {
		t.Errorf("source=%q", ctx.Source)
	}
}

func TestShortTraceID(t *testing.T) {
	ctx := Context{TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"}
	if got := ctx.ShortTraceID(); got != "4bf9…4736" {
		t.Errorf("ShortTraceID=%q", got)
	}
}
