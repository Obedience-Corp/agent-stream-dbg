// Package bridge is the stable call-site facade over the real Brainyard
// dialect (dialects/brainyard.yaml) for internal/client, internal/visualizer,
// and cmd/stream-debugger. It knows no event types itself —
// internal/mapping.Engine does the work, driven entirely by that YAML file;
// this package only loads it once and exposes a convenient API.
package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/mapping"
)

// dialectPath is resolved relative to this source file's location rather
// than the process's working directory, so it finds dialects/brainyard.yaml
// whether invoked via `go test` (cwd = this package's directory), `just
// build && ./bin/...` from the repo root, or a `go install`ed binary run
// from the source checkout that built it.
var dialectPath = func() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "dialects", "brainyard.yaml")
}()

// engine is loaded once, on first use rather than at import time, so
// unrelated commands (--help, version) aren't affected by a missing
// dialect file. A load failure panics: a missing or corrupt
// dialects/brainyard.yaml is a deployment error, not a runtime data
// condition — the alternative (silently returning errors from Parse) would
// reintroduce the exact "unknown events vanish" bug this phase fixed.
var engine = sync.OnceValue(func() *mapping.Engine {
	e, err := mapping.LoadFile(dialectPath)
	if err != nil {
		panic("bridge: failed to load " + dialectPath + ": " + err.Error())
	}
	return e
})

// Parser decodes wire frames into the generic events.Event core via the
// Brainyard dialect.
type Parser struct{}

// NewParser creates a new event parser.
func NewParser() *Parser {
	return &Parser{}
}

// Parse converts raw wire data into the generic Event, given its event name.
// It never errors — an unrecognized event name or malformed body decodes to
// a Kind=Unknown event rather than being dropped (passthrough is
// non-negotiable for a debugger).
func (p *Parser) Parse(eventType string, data []byte) (*events.Event, error) {
	return engine().Decode(eventType, data), nil
}

// ParseRaw parses event data without a separately-known event name, reading
// the "type" field from the payload itself if present. Like Parse, it never
// errors.
func (p *Parser) ParseRaw(data []byte) (*events.Event, error) {
	return engine().Decode(rawEventType(data), data), nil
}

// rawEventType extracts the "type" field from a payload with no separate
// SSE event name (e.g. a JSONL log line), for use as the dispatch name.
func rawEventType(data []byte) string {
	var probe struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(data, &probe)
	return probe.Type
}

// RenderSend renders the dialect's send: block into a transport-ready
// request (method, URL, body), given the run-config values a session
// needs. Stable facade over mapping.RenderSend for internal/client and
// internal/visualizer.
func RenderSend(vars mapping.InterpolationVars) (method, url string, body []byte, err error) {
	return mapping.RenderSend(engine().Send, vars)
}

// RunSetup executes the dialect's setup: handshake (if any) and returns
// the extracted session ID. Stable facade over mapping.RunSetup. headers
// (typically config.Transport.ResolvedHeaders()) authenticates the
// handshake exactly like every other backend request.
func RunSetup(ctx context.Context, vars mapping.InterpolationVars, headers map[string]string, httpClient *http.Client) (string, error) {
	return mapping.RunSetup(ctx, engine().Setup, vars, headers, httpClient)
}
