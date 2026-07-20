package transport

import "context"

// SessionTransport is an optional extension for transports that keep a live
// session across multiple user turns (e.g. ACP stdio). Callers type-assert
// after Connect and use Prompt for subsequent messages instead of building a
// new Transport.
//
// Frames() stays open until the session dies; turn boundaries are observed
// from decoded events (e.g. stream_end), not from channel close.
type SessionTransport interface {
	Transport
	// Prompt starts a new user turn on the existing session.
	Prompt(ctx context.Context, message string) error
	// Alive reports whether Prompt may still be called.
	Alive() bool
}
