// Package tracectx implements W3C Trace Context parsing and session-level
// correlation for agent-stream-dbg. It deliberately does not depend on the
// OpenTelemetry SDK: correlation is a join layer, not always-on tracing.
package tracectx

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

// Field keys stamped onto events.Event.Fields and structured logs.
const (
	FieldTraceID     = "trace_id"
	FieldSpanID      = "span_id"
	FieldTraceSource = "trace_source"
)

// Context is a parsed W3C Trace Context (or a partial vendor-shaped ID set).
type Context struct {
	TraceID    string // 32 lowercase hex
	SpanID     string // 16 lowercase hex (parent/span when known)
	TraceState string // raw tracestate if present
	RawParent  string // original traceparent when known
	Source     string // header | trailer | metadata | payload | generated | forced
}

// Valid reports whether TraceID is present and well-formed.
func (c Context) Valid() bool {
	return isTraceID(c.TraceID)
}

// Empty reports whether no identity is set.
func (c Context) Empty() bool {
	return c.TraceID == "" && c.SpanID == "" && c.RawParent == ""
}

// Traceparent rebuilds a W3C traceparent header value.
// Returns "" if TraceID is invalid. Generates a random span id when SpanID
// is missing so the header remains well-formed for outbound injection.
func (c Context) Traceparent() string {
	if !isTraceID(c.TraceID) {
		return ""
	}
	span := c.SpanID
	if !isSpanID(span) {
		var err error
		span, err = randomHex(8)
		if err != nil {
			return ""
		}
	}
	return fmt.Sprintf("00-%s-%s-01", c.TraceID, span)
}

// ShortTraceID returns a truncated display form (prefix…suffix).
func (c Context) ShortTraceID() string {
	if !c.Valid() {
		return ""
	}
	if len(c.TraceID) < 12 {
		return c.TraceID
	}
	return c.TraceID[:4] + "…" + c.TraceID[len(c.TraceID)-4:]
}

// ParseTraceparent parses a W3C traceparent header value.
// Malformed input returns an empty Context and a non-nil error; callers
// treating correlation as best-effort should ignore the error.
func ParseTraceparent(s string) (Context, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Context{}, fmt.Errorf("tracectx: empty traceparent")
	}
	parts := strings.Split(s, "-")
	if len(parts) != 4 {
		return Context{}, fmt.Errorf("tracectx: traceparent must have 4 parts")
	}
	version, traceID, spanID, flags := parts[0], parts[1], parts[2], parts[3]
	if version != "00" && version != "ff" {
		// Accept only known versions; ff is forbidden by the spec for sending
		// but we still refuse to treat it as a valid observed parent.
		if version == "ff" {
			return Context{}, fmt.Errorf("tracectx: forbidden traceparent version ff")
		}
	}
	if version != "00" {
		return Context{}, fmt.Errorf("tracectx: unsupported traceparent version %q", version)
	}
	if !isTraceID(traceID) {
		return Context{}, fmt.Errorf("tracectx: invalid trace_id")
	}
	if !isSpanID(spanID) {
		return Context{}, fmt.Errorf("tracectx: invalid span_id")
	}
	if len(flags) != 2 || !isHex(flags) {
		return Context{}, fmt.Errorf("tracectx: invalid flags")
	}
	// All-zero trace and span ids are invalid per W3C.
	if traceID == strings.Repeat("0", 32) || spanID == strings.Repeat("0", 16) {
		return Context{}, fmt.Errorf("tracectx: zero identity")
	}
	return Context{
		TraceID:   strings.ToLower(traceID),
		SpanID:    strings.ToLower(spanID),
		RawParent: s,
	}, nil
}

// FromHeaders extracts W3C context from HTTP headers (case-insensitive).
func FromHeaders(h http.Header) (Context, bool) {
	if h == nil {
		return Context{}, false
	}
	tp := h.Get("Traceparent")
	if tp == "" {
		tp = h.Get("traceparent")
	}
	if tp == "" {
		return Context{}, false
	}
	ctx, err := ParseTraceparent(tp)
	if err != nil {
		return Context{}, false
	}
	ctx.Source = "header"
	if ts := h.Get("Tracestate"); ts != "" {
		ctx.TraceState = ts
	} else if ts := h.Get("tracestate"); ts != "" {
		ctx.TraceState = ts
	}
	return ctx, true
}

// FromMap extracts context from a flat string map (gRPC trailers/metadata,
// or flattened header maps). Prefers traceparent; falls back to known keys.
func FromMap(m map[string]string) (Context, bool) {
	if m == nil {
		return Context{}, false
	}
	// Case-insensitive lookup helpers.
	get := func(keys ...string) string {
		for _, want := range keys {
			for k, v := range m {
				if strings.EqualFold(k, want) && strings.TrimSpace(v) != "" {
					// Multi-value trailers may be comma-joined; take first segment for W3C.
					return strings.TrimSpace(strings.Split(v, ",")[0])
				}
			}
		}
		return ""
	}
	if tp := get("traceparent"); tp != "" {
		ctx, err := ParseTraceparent(tp)
		if err == nil {
			ctx.Source = "trailer"
			if ts := get("tracestate"); ts != "" {
				ctx.TraceState = ts
			}
			return ctx, true
		}
	}
	if tid := get("trace_id", "traceId", "trace-id"); tid != "" {
		tid = strings.ToLower(strings.ReplaceAll(tid, "-", ""))
		if isTraceID(tid) && tid != strings.Repeat("0", 32) {
			ctx := Context{TraceID: tid, Source: "trailer"}
			if sid := get("span_id", "spanId", "span-id", "parent_span_id", "parentSpanId"); sid != "" {
				sid = strings.ToLower(sid)
				if isSpanID(sid) {
					ctx.SpanID = sid
				}
			}
			return ctx, true
		}
	}
	return Context{}, false
}

// FromFields extracts context from decoded event Fields (payload fallback).
func FromFields(fields map[string]any) (Context, bool) {
	if fields == nil {
		return Context{}, false
	}
	str := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := fields[k]; ok {
				switch t := v.(type) {
				case string:
					if s := strings.TrimSpace(t); s != "" {
						return s
					}
				}
			}
		}
		return ""
	}
	if tp := str("traceparent"); tp != "" {
		ctx, err := ParseTraceparent(tp)
		if err == nil {
			ctx.Source = "payload"
			return ctx, true
		}
	}
	if tid := str("trace_id", "traceId", "trace-id"); tid != "" {
		tid = strings.ToLower(strings.ReplaceAll(tid, "-", ""))
		if isTraceID(tid) && tid != strings.Repeat("0", 32) {
			ctx := Context{TraceID: tid, Source: "payload"}
			if sid := str("span_id", "spanId", "span-id", "parent_span_id", "parentSpanId"); sid != "" {
				sid = strings.ToLower(sid)
				if isSpanID(sid) {
					ctx.SpanID = sid
				}
			}
			return ctx, true
		}
	}
	return Context{}, false
}

// Generate creates a new root context (random trace + span ids).
func Generate() (Context, error) {
	tid, err := randomHex(16)
	if err != nil {
		return Context{}, err
	}
	sid, err := randomHex(8)
	if err != nil {
		return Context{}, err
	}
	return Context{
		TraceID: tid,
		SpanID:  sid,
		Source:  "generated",
	}, nil
}

func isTraceID(s string) bool {
	return len(s) == 32 && isHex(s)
}

func isSpanID(s string) bool {
	return len(s) == 16 && isHex(s)
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func randomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("tracectx: random: %w", err)
	}
	return hex.EncodeToString(b), nil
}
