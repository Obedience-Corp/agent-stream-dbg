package client

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/lancekrogers/stream-debugger/internal/bridge"
	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/transport"
)

// Client streams parsed events from the configured transport, applying the
// selected dialect (via internal/bridge) to every frame.
type Client struct {
	config *config.EnhancedConfig
	parser *bridge.Parser

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
	return &Client{
		config:  cfg,
		parser:  bridge.NewParser(cfg.Dialect.File),
		eventCh: make(chan *events.Event, 100),
		errCh:   make(chan error, 10),
	}
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
	tr, err := newTransport(c.config, c.parser, vars)
	if err != nil {
		return err
	}
	if err := tr.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect %s transport: %w", tr.Name(), err)
	}

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
		if len(allowed) > 0 && !slices.Contains(allowed, frame.Name) {
			continue
		}
		event, _ := c.parser.Parse(frame.Name, frame.Data)
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
