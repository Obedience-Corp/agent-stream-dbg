package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/r3labs/sse/v2"
)

// defaultEventTypes is used when the config does not specify events.types.
var defaultEventTypes = []string{
	"session_start",
	"session_complete",
	"agent_stream_start",
	"agent_content",
	"agent_stream_complete",
	"wizard_stream_start",
	"wizard_content",
	"wizard_stream_complete",
	"error",
	"flow_step_start",
	"flow_step_end",
	"flow_step_detail",
}

// SSEClient handles Server-Sent Events connection to the backend
type SSEClient struct {
	config  *config.EnhancedConfig
	client  *sse.Client
	parser  *events.Parser
	eventCh chan *events.Event
	errCh   chan error
}

// NewSSEClient creates a new SSE client
func NewSSEClient(cfg *config.EnhancedConfig) *SSEClient {
	client := sse.NewClient(cfg.StreamEndpointURL())

	// Set up authentication and custom headers from config
	client.Headers = cfg.Backend.ResolvedHeaders()
	client.Headers["Accept"] = "text/event-stream"

	return &SSEClient{
		config:  cfg,
		client:  client,
		parser:  events.NewParser(),
		eventCh: make(chan *events.Event, 100),
		errCh:   make(chan error, 10),
	}
}

// Connect establishes SSE connection and starts streaming events
func (c *SSEClient) Connect(ctx context.Context, message string) error {
	// Add message as query parameter
	endpoint := c.config.StreamEndpointURL()
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}

	q := u.Query()
	q.Set("message", message)
	if c.config.Debug.Level != "" {
		q.Set("debug", c.config.Debug.Level)
	}
	u.RawQuery = q.Encode()

	// Update client URL
	c.client = sse.NewClient(u.String())
	c.client.Headers = c.config.Backend.ResolvedHeaders()
	c.client.Headers["Accept"] = "text/event-stream"

	eventTypes := c.config.Events.Types
	if len(eventTypes) == 0 {
		eventTypes = defaultEventTypes
	}

	for _, eventType := range eventTypes {
		c.subscribeToEvent(ctx, eventType)
	}

	return nil
}

// subscribeToEvent subscribes to a specific SSE event type
func (c *SSEClient) subscribeToEvent(ctx context.Context, eventType string) {
	go func() {
		err := c.client.SubscribeWithContext(ctx, eventType, func(msg *sse.Event) {
			// Parse the event
			event, parseErr := c.parser.Parse(string(msg.Event), msg.Data)
			if parseErr != nil {
				c.errCh <- fmt.Errorf("failed to parse event %s: %w", eventType, parseErr)
				return
			}

			// Send to event channel
			select {
			case c.eventCh <- event:
			case <-ctx.Done():
				return
			}
		})

		if err != nil && ctx.Err() == nil {
			c.errCh <- fmt.Errorf("subscription error for %s: %w", eventType, err)
		}
	}()
}

// Events returns the channel for receiving parsed events
func (c *SSEClient) Events() <-chan *events.Event {
	return c.eventCh
}

// Errors returns the channel for receiving errors
func (c *SSEClient) Errors() <-chan error {
	return c.errCh
}

// Close closes the SSE connection
func (c *SSEClient) Close() {
	close(c.eventCh)
	close(c.errCh)
}
