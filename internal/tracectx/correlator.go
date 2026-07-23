package tracectx

import (
	"net/http"
	"sync"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
)

// Mode controls outbound behavior. Inbound observation is always on when
// Enabled is true (design D1).
type Mode int

const (
	// ModeObserve only joins inbound context (default).
	ModeObserve Mode = iota
	// ModePropagate injects outbound traceparent when a context exists.
	ModePropagate
	// ModeGenerate is ModePropagate plus generating a root when missing.
	ModeGenerate
)

// Options configure a Correlator.
type Options struct {
	Enabled bool
	Mode    Mode
	// Forced, if Valid, seeds the session context (e.g. --otel-traceparent).
	// Forced is applied only in New — not via Observe.
	Forced Context
}

// Correlator holds session-scoped trace context for join, stamp, and inject.
type Correlator struct {
	mu      sync.Mutex
	enabled bool
	mode    Mode
	current Context
	// localSpanMinted is true once OutboundTraceparent has minted a local
	// span under the session trace (so inject is stable across calls).
	localSpanMinted bool
}

// New builds a correlator. When opts.Enabled is false, all methods no-op.
func New(opts Options) *Correlator {
	c := &Correlator{
		enabled: opts.Enabled,
		mode:    opts.Mode,
	}
	if opts.Forced.Valid() {
		forced := opts.Forced
		if forced.Source == "" {
			forced.Source = "forced"
		}
		if forced.Flags == "" {
			forced.Flags = "01"
		}
		// Forced parent is remote identity; outbound will mint a local child span.
		forced.RemoteSpan = forced.SpanID
		c.current = forced
	}
	return c
}

// Enabled reports whether correlation is active.
func (c *Correlator) Enabled() bool {
	if c == nil {
		return false
	}
	return c.enabled
}

// Mode returns the outbound mode.
func (c *Correlator) Mode() Mode {
	if c == nil {
		return ModeObserve
	}
	return c.mode
}

// Current returns a copy of the active session context.
func (c *Correlator) Current() Context {
	if c == nil {
		return Context{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

// Observe merges an inbound context into the session.
// First valid TraceID wins for session chrome. Forced is only set in New,
// never via Observe.
func (c *Correlator) Observe(in Context) {
	if c == nil || !c.enabled || !in.Valid() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.current.Valid() {
		// Observed remote identity; keep SpanID as remote parent until outbound mints.
		if in.SpanID != "" {
			in.RemoteSpan = in.SpanID
		}
		c.current = in
		c.localSpanMinted = false
		return
	}
	// Keep first session trace_id; refresh remote parent / metadata only when
	// same trace. Do not overwrite a minted local span with remote.
	if c.current.TraceID == in.TraceID {
		if in.SpanID != "" {
			c.current.RemoteSpan = in.SpanID
			if !c.localSpanMinted {
				c.current.SpanID = in.SpanID
			}
		}
		if in.Flags != "" && !c.localSpanMinted {
			c.current.Flags = in.Flags
		}
		if in.TraceState != "" {
			c.current.TraceState = in.TraceState
		}
		if in.RawParent != "" {
			c.current.RawParent = in.RawParent
		}
		if in.Source != "" && c.current.Source != "forced" && c.current.Source != "generated" {
			c.current.Source = in.Source
		}
	}
}

// ObserveHeaders extracts and observes W3C headers.
func (c *Correlator) ObserveHeaders(h http.Header) {
	if ctx, ok := FromHeaders(h); ok {
		c.Observe(ctx)
	}
}

// ObserveMap extracts and observes trailer/metadata maps.
func (c *Correlator) ObserveMap(m map[string]string) {
	if ctx, ok := FromMap(m, "trailer"); ok {
		c.Observe(ctx)
	}
}

// ObserveEventFields extracts and observes decoded Fields.
func (c *Correlator) ObserveEventFields(fields map[string]any) {
	if ctx, ok := FromFields(fields); ok {
		c.Observe(ctx)
	}
}

// Changed reports whether session TraceID/SpanID differ from prev.
func (c *Correlator) Changed(prev Context) bool {
	if c == nil {
		return false
	}
	cur := c.Current()
	return cur.TraceID != prev.TraceID || cur.SpanID != prev.SpanID
}

// OutboundTraceparent returns a header value when propagation is enabled.
// Always mints and stores a stable local span under the session TraceID so
// the debugger request is a child of the observed remote parent, not a
// reuse of the remote span id.
func (c *Correlator) OutboundTraceparent() (string, bool) {
	if c == nil || !c.enabled {
		return "", false
	}
	if c.mode != ModePropagate && c.mode != ModeGenerate {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.current.Valid() {
		if c.mode != ModeGenerate {
			return "", false
		}
		gen, err := Generate()
		if err != nil {
			return "", false
		}
		c.current = gen
		c.localSpanMinted = true
	} else if !c.localSpanMinted {
		// Mint a local child span under the session (possibly remote) trace.
		if c.current.SpanID != "" && c.current.RemoteSpan == "" {
			c.current.RemoteSpan = c.current.SpanID
		}
		sid, err := randomHex(8)
		if err != nil {
			return "", false
		}
		c.current.SpanID = sid
		if c.current.Flags == "" {
			// Preserve inbound flags when known; default sampled only when absent.
			c.current.Flags = "01"
		}
		c.localSpanMinted = true
	}
	tp := c.current.Traceparent()
	if tp == "" {
		return "", false
	}
	return tp, true
}

// OutboundTracestate returns tracestate when present and propagating.
func (c *Correlator) OutboundTracestate() (string, bool) {
	if c == nil || !c.enabled {
		return "", false
	}
	if c.mode != ModePropagate && c.mode != ModeGenerate {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current.TraceState == "" {
		return "", false
	}
	return c.current.TraceState, true
}

// StampEvent observes payload fields when present, then stamps Fields.
// Inherits session trace_id when the event has none (design D3); span_id only
// when event-specific. Cheap no-op when there is nothing new to join and the
// event already carries (or does not need) a session trace_id.
func (c *Correlator) StampEvent(ev *events.Event) {
	if c == nil || !c.enabled || ev == nil {
		return
	}

	// Observe only when payload likely carries correlation keys (hot path).
	if fieldsLikelyHaveTrace(ev.Fields) {
		c.ObserveEventFields(ev.Fields)
		if trailers, ok := ev.Fields["trailers"].(map[string]any); ok {
			flat := make(map[string]string, len(trailers))
			for k, v := range trailers {
				if s, ok := v.(string); ok {
					flat[k] = s
				}
			}
			c.ObserveMap(flat)
		} else if trailers, ok := ev.Fields["trailers"].(map[string]string); ok {
			c.ObserveMap(trailers)
		}
	}

	c.mu.Lock()
	cur := c.current
	c.mu.Unlock()

	if !cur.Valid() && !fieldsLikelyHaveTrace(ev.Fields) {
		return
	}

	eventCtx, hasEvent := FromFields(ev.Fields)
	if ev.Fields == nil {
		ev.Fields = make(map[string]any)
	}
	if hasEvent && eventCtx.Valid() {
		ev.Fields[FieldTraceID] = eventCtx.TraceID
		if eventCtx.SpanID != "" {
			ev.Fields[FieldSpanID] = eventCtx.SpanID
		}
		if eventCtx.Source != "" {
			ev.Fields[FieldTraceSource] = eventCtx.Source
		}
		return
	}
	if cur.Valid() {
		// Inherit trace_id only.
		if _, ok := ev.Fields[FieldTraceID]; !ok {
			ev.Fields[FieldTraceID] = cur.TraceID
		}
		if cur.Source != "" {
			if _, ok := ev.Fields[FieldTraceSource]; !ok {
				ev.Fields[FieldTraceSource] = cur.Source
			}
		}
	}
}

// InjectHTTPHeaders sets traceparent/tracestate on h when propagation is on
// and the header is not already present (case-insensitive).
func (c *Correlator) InjectHTTPHeaders(h map[string]string) {
	if h == nil {
		return
	}
	if _, exists := headerLookup(h, "traceparent"); exists {
		return
	}
	tp, ok := c.OutboundTraceparent()
	if !ok {
		return
	}
	h["traceparent"] = tp
	if ts, ok := c.OutboundTracestate(); ok {
		if _, exists := headerLookup(h, "tracestate"); !exists {
			h["tracestate"] = ts
		}
	}
}

// InjectMetadata sets traceparent/tracestate on a metadata map with the same
// no-clobber rules as InjectHTTPHeaders (shared inject policy for SSE/gRPC).
func (c *Correlator) InjectMetadata(m map[string]string) {
	c.InjectHTTPHeaders(m)
}

func headerLookup(h map[string]string, name string) (string, bool) {
	for k, v := range h {
		if http.CanonicalHeaderKey(k) == http.CanonicalHeaderKey(name) {
			return v, true
		}
	}
	return "", false
}
