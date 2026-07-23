package client

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/config"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/tracectx"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport/sse"
)

// Client streams parsed events from the configured transport, applying the
// selected dialect (via internal/bridge) to every frame.
type Client struct {
	config *config.EnhancedConfig
	parser *bridge.Parser
	corr   *tracectx.Correlator

	eventCh chan *events.Event
	errCh   chan error

	mu        sync.Mutex
	transport transport.Transport

	// wg tracks every readLoop goroutine ever started, across reconnects
	// (Connect may be called more than once on the same client — see
	// visualizer.Model.reconnect). Close waits on it before closing the
	// channels, so a send can never race a close by construction.
	wg sync.WaitGroup
}

// NewSSEClient creates a new SSE client.
// SSEClient is retained as a compatibility name while the client supports
// every configured stream transport.
type SSEClient = Client

// NewClient creates a client for the configured transport type.
func NewClient(cfg *config.EnhancedConfig) *Client {
	corr, err := NewCorrelatorFromConfig(cfg)
	if err != nil {
		// Config was already validated at load/CLI; fall back to observe-only
		// so tests constructing bare EnhancedConfig never panic. Callers that
		// need hard failure use NewCorrelatorFromConfig directly.
		corr = tracectx.New(tracectx.Options{Enabled: true, Mode: tracectx.ModeObserve})
	}
	return NewClientWithCorrelator(cfg, corr)
}

// NewClientWithCorrelator creates a client with an explicit correlator
// (may be nil to disable).
func NewClientWithCorrelator(cfg *config.EnhancedConfig, corr *tracectx.Correlator) *Client {
	return &Client{
		config:  cfg,
		parser:  bridge.NewParser(cfg.Dialect.File),
		corr:    corr,
		eventCh: make(chan *events.Event, 100),
		errCh:   make(chan error, 10),
	}
}

// Correlator returns the session correlator (may be nil).
func (c *Client) Correlator() *tracectx.Correlator {
	if c == nil {
		return nil
	}
	return c.corr
}

// NewCorrelatorFromConfig builds a correlator from EnhancedConfig.Correlation.
// Inbound observation is always enabled (design D1). --otel-propagate maps to
// ModeGenerate so a root is minted when nothing inbound exists (D4).
// Returns an error when Correlation.Traceparent is non-empty but invalid —
// silent force+generate is unsafe for join workflows.
func NewCorrelatorFromConfig(cfg *config.EnhancedConfig) (*tracectx.Correlator, error) {
	mode := tracectx.ModeObserve
	var forced tracectx.Context
	if cfg != nil {
		cc := cfg.Correlation
		if cc.Generate || cc.Propagate {
			// Propagate implies generate-if-missing for outbound inject.
			mode = tracectx.ModeGenerate
		}
		if tp := strings.TrimSpace(cc.Traceparent); tp != "" {
			parsed, err := tracectx.ParseTraceparent(tp)
			if err != nil {
				return nil, fmt.Errorf("invalid --otel-traceparent / TRACEPARENT %q: %w", tp, err)
			}
			parsed.Source = "forced"
			forced = parsed
		}
	}
	return tracectx.New(tracectx.Options{Enabled: true, Mode: mode, Forced: forced}), nil
}

// NewSSEClient retains the original constructor name for callers that have
// not yet migrated to the transport-neutral client name.
func NewSSEClient(cfg *config.EnhancedConfig) *SSEClient { return NewClient(cfg) }

// Connect renders the dialect's send: template and starts streaming
// events from the result. Safe to call again on the same client (e.g. to
// reconnect with a new debug level) — the previous transport, if any, is
// closed first, but Events()/Errors() keep delivering across the switch.
func (c *Client) Connect(ctx context.Context, message string) error {
	vars := config.InterpolationVarsFromConfig(c.config)
	vars.Message = message
	tr, err := newTransport(c.config, c.parser, vars, c.corr)
	if err != nil {
		return err
	}
	if err := tr.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect %s transport: %w", tr.Name(), err)
	}
	observeTransportTrace(c.corr, tr)

	c.mu.Lock()
	previous := c.transport
	c.transport = tr
	c.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}

	c.wg.Add(1)
	go c.readLoop(tr)
	return nil
}

// observeTransportTrace joins inbound W3C context from transport side channels.
func observeTransportTrace(corr *tracectx.Correlator, tr transport.Transport) {
	if corr == nil || tr == nil {
		return
	}
	if st, ok := tr.(*sse.Transport); ok {
		corr.ObserveHeaders(st.ResponseHeaders())
	}
}

// readLoop forwards frames from one transport connection to Events()/
// Errors() until that transport's stream ends. It never closes the
// channels itself — Close does, once every readLoop has exited.
func (c *Client) readLoop(tr transport.Transport) {
	defer c.wg.Done()

	allowed := c.config.Events.Types
	for frame := range tr.Frames() {
		if frame.Err != nil {
			c.errCh <- fmt.Errorf("malformed frame (name=%q, raw=%q): %w", frame.Name, frame.Raw, frame.Err)
			continue
		}
		// Observe gRPC trailers before event-type allowlist so filtered
		// grpc_status frames still join session correlation.
		if c.corr != nil && frame.Name == "grpc_status" && len(frame.Data) > 0 {
			var status struct {
				Trailers map[string]string `json:"trailers"`
			}
			if json.Unmarshal(frame.Data, &status) == nil && len(status.Trailers) > 0 {
				c.corr.ObserveMap(status.Trailers)
			}
		}
		if len(allowed) > 0 && !slices.Contains(allowed, frame.Name) {
			continue
		}
		event, _ := c.parser.Parse(frame.Name, frame.Data)
		if event != nil && c.corr != nil {
			c.corr.StampEvent(event)
		}
		c.eventCh <- event
	}
}

// Events returns the channel for receiving parsed events.
func (c *Client) Events() <-chan *events.Event {
	return c.eventCh
}

// Errors returns the channel for receiving errors.
func (c *Client) Errors() <-chan error {
	return c.errCh
}

// Close shuts down the current transport and waits for every readLoop
// goroutine started by this client to exit before closing Events()/
// Errors() — so a send can never race the close.
func (c *Client) Close() {
	c.mu.Lock()
	tr := c.transport
	c.mu.Unlock()
	if tr != nil {
		_ = tr.Close()
	}
	c.wg.Wait()
	close(c.eventCh)
	close(c.errCh)
}
