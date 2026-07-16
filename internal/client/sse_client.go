package client

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"sync"

	"github.com/lancekrogers/stream-debugger/internal/bridge"
	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/mapping"
	"github.com/lancekrogers/stream-debugger/internal/transport/sse"
)

// SSEClient streams parsed events from the backend over the stdlib SSE
// transport, applying the dialect (via internal/bridge) to every frame.
type SSEClient struct {
	config *config.EnhancedConfig
	parser *bridge.Parser

	eventCh chan *events.Event
	errCh   chan error

	mu        sync.Mutex
	transport *sse.Transport

	// wg tracks every readLoop goroutine ever started, across reconnects
	// (Connect may be called more than once on the same client — see
	// visualizer.Model.reconnect). Close waits on it before closing the
	// channels, so a send can never race a close by construction.
	wg sync.WaitGroup
}

// NewSSEClient creates a new SSE client.
func NewSSEClient(cfg *config.EnhancedConfig) *SSEClient {
	return &SSEClient{
		config:  cfg,
		parser:  bridge.NewParser(),
		eventCh: make(chan *events.Event, 100),
		errCh:   make(chan error, 10),
	}
}

// Connect renders the dialect's send: template and starts streaming
// events from the result. Safe to call again on the same client (e.g. to
// reconnect with a new debug level) — the previous transport, if any, is
// closed first, but Events()/Errors() keep delivering across the switch.
func (c *SSEClient) Connect(ctx context.Context, message string) error {
	vars := mapping.InterpolationVars{
		BaseURL:   c.config.Transport.BaseURL,
		SessionID: c.config.Session.ID,
		Message:   message,
	}
	method, renderedURL, body, err := bridge.RenderSend(vars)
	if err != nil {
		return fmt.Errorf("failed to render send request: %w", err)
	}
	if method != "GET" || body != nil {
		return fmt.Errorf("stream mode can only execute a GET-style send (no body) today; dialect declared %s with a body — needs a POST-capable transport", method)
	}

	if c.config.Debug.Level != "" {
		u, err := url.Parse(renderedURL)
		if err != nil {
			return fmt.Errorf("invalid endpoint URL: %w", err)
		}
		q := u.Query()
		q.Set("debug", c.config.Debug.Level)
		u.RawQuery = q.Encode()
		renderedURL = u.String()
	}

	headers := c.config.Transport.ResolvedHeaders()
	headers["Accept"] = "text/event-stream"

	tr := sse.New(method, renderedURL, nil, headers)
	if err := tr.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
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
func (c *SSEClient) readLoop(tr *sse.Transport) {
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
func (c *SSEClient) Events() <-chan *events.Event {
	return c.eventCh
}

// Errors returns the channel for receiving errors.
func (c *SSEClient) Errors() <-chan error {
	return c.errCh
}

// Close shuts down the current transport and waits for every readLoop
// goroutine started by this client to exit before closing Events()/
// Errors() — so a send can never race the close.
func (c *SSEClient) Close() {
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
