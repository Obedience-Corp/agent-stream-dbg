// Package bridge is the stable call-site facade over a selected dialect for
// internal/client, internal/visualizer, and cmd/agent-stream-dbg. It knows no
// event types itself — internal/mapping.Engine does the work, driven entirely
// by YAML data; this package only loads the chosen source and exposes a
// convenient API.
package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Obedience-Corp/agent-stream-dbg/dialects"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/mapping"
)

// Parser decodes wire frames into the generic events.Event core via one
// selected dialect. Loading errors are retained and returned by Parse rather
// than causing a panic during package initialization or first use.
type Parser struct {
	engine *mapping.Engine
	err    error
}

// LoadDialect resolves source in this order: an explicit filesystem path,
// then an embedded dialect name. Empty source selects dialects.DefaultName.
// Errors include the source and, for missing embedded names, the available
// embedded dialects.
func LoadDialect(source string) (*mapping.Engine, error) {
	data, label, err := readDialect(source)
	if err != nil {
		return nil, err
	}
	engine, err := mapping.Load(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse dialect %s: %w", label, err)
	}
	return engine, nil
}

func readDialect(source string) ([]byte, string, error) {
	requested := strings.TrimSpace(source)
	if requested == "" {
		requested = dialects.DefaultName
	}
	if isExplicitPath(requested) {
		data, err := os.ReadFile(requested)
		if err != nil {
			return nil, requested, fmt.Errorf("failed to read dialect file %s: %w", requested, err)
		}
		return data, requested, nil
	}
	data, err := dialects.Load(requested)
	if err != nil {
		return nil, requested, err
	}
	return data, "embedded " + requested, nil
}

func isExplicitPath(source string) bool {
	return filepath.IsAbs(source) || strings.HasPrefix(source, ".") || strings.ContainsAny(source, `/\\`)
}

// NewParser creates a parser for source. With no source it uses the embedded
// default dialect. A parser created for an invalid source returns that load
// error from Parse; use NewParserFor when construction-time error handling is
// preferred.
func NewParser(source ...string) *Parser {
	requested := ""
	if len(source) > 0 {
		requested = source[0]
	}
	engine, err := LoadDialect(requested)
	return &Parser{engine: engine, err: err}
}

// NewParserFor creates a parser and returns a load error immediately.
func NewParserFor(source string) (*Parser, error) {
	engine, err := LoadDialect(source)
	if err != nil {
		return nil, err
	}
	return &Parser{engine: engine}, nil
}

func (p *Parser) getEngine() (*mapping.Engine, error) {
	if p == nil {
		return nil, fmt.Errorf("bridge: nil parser")
	}
	if p.err != nil {
		return nil, p.err
	}
	if p.engine == nil {
		return nil, fmt.Errorf("bridge: parser has no dialect engine")
	}
	return p.engine, nil
}

// Engine returns the compiled dialect or its loading error.
func (p *Parser) Engine() (*mapping.Engine, error) {
	return p.getEngine()
}

// Parse converts raw wire data into the generic Event, given its event name.
// An unrecognized event name or malformed body decodes to a Kind=Unknown
// event rather than being dropped; a dialect load error is returned.
func (p *Parser) Parse(eventType string, data []byte) (*events.Event, error) {
	engine, err := p.getEngine()
	if err != nil {
		return nil, err
	}
	return engine.Decode(eventType, data), nil
}

// ParseRaw parses event data without a separately-known event name, reading
// the "type" field from the payload itself if present. Like Parse, it never
// errors for an unknown or malformed event body.
func (p *Parser) ParseRaw(data []byte) (*events.Event, error) {
	return p.Parse(rawEventType(data), data)
}

// Flow returns the selected dialect's flow projection, or nil when loading
// failed or the dialect declares no flow block.
func (p *Parser) Flow() *mapping.FlowSpec {
	engine, err := p.getEngine()
	if err != nil {
		return nil
	}
	return engine.Flow
}

// RenderSend renders the selected dialect's send: block.
func (p *Parser) RenderSend(vars mapping.InterpolationVars) (method, url string, body []byte, err error) {
	engine, err := p.getEngine()
	if err != nil {
		return "", "", nil, err
	}
	return mapping.RenderSend(engine.Send, vars)
}

// RunSetup executes the selected dialect's setup: handshake.
func (p *Parser) RunSetup(ctx context.Context, vars mapping.InterpolationVars, headers map[string]string, httpClient *http.Client) (string, error) {
	engine, err := p.getEngine()
	if err != nil {
		return "", err
	}
	return mapping.RunSetup(ctx, engine.Setup, vars, headers, httpClient)
}

// defaultParser preserves the original no-argument bridge facade while using
// the embedded loader. A missing or bad dialect is retained as an error
// instead of causing a panic.
var defaultParser = sync.OnceValue(func() *Parser { return NewParser() })

// RenderSend renders the embedded default dialect's send: block.
func RenderSend(vars mapping.InterpolationVars) (method, url string, body []byte, err error) {
	return defaultParser().RenderSend(vars)
}

// RunSetup executes the embedded default dialect's setup: handshake.
func RunSetup(ctx context.Context, vars mapping.InterpolationVars, headers map[string]string, httpClient *http.Client) (string, error) {
	return defaultParser().RunSetup(ctx, vars, headers, httpClient)
}

// Flow returns the embedded default dialect's flow projection.
func Flow() *mapping.FlowSpec {
	return defaultParser().Flow()
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
