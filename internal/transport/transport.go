// Package transport defines the Frame contract every wire transport
// (SSE, replay, and eventually gRPC) emits into the dialect engine. A
// transport knows nothing about agents, kinds, or dialects — it only
// ever yields raw frames.
package transport

import (
	"context"
	"time"
)

// Frame is one unit off the wire, pre-interpretation. Raw is never
// normalized and Err is a field, not a return: a malformed frame is a
// finding a debugger must surface, not an error a client library should
// swallow.
type Frame struct {
	Name      string    // SSE event name; gRPC oneof case; "" if undiscriminated
	Data      []byte    // JSON (SSE) or protojson-rendered (gRPC)
	Raw       []byte    // exact bytes as received — never normalized
	Timestamp time.Time // local receipt time, always set
	Err       error     // malformed frame — surfaced, not dropped
}

// Transport yields raw frames from one wire shape. Implementations must
// never silently reconnect or drop malformed input — see Frame.Err.
// Connect's context governs dialing and initial stream setup only. After
// Connect returns successfully, the stream lifetime is governed by Close;
// implementations derive an internal stream context so a short-lived
// dialing timeout cannot terminate an otherwise healthy stream.
type Transport interface {
	Name() string
	Connect(ctx context.Context) error
	Frames() <-chan Frame
	// Send pushes an engine-rendered payload from the dialect's send
	// template. Transports that fix their request at Connect time (e.g.
	// SSE) return a clear error rather than silently no-op'ing.
	Send(ctx context.Context, payload []byte) error
	Close() error
}
