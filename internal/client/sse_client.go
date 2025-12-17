package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/r3labs/sse/v2"
)

// SSEClient handles Server-Sent Events connection to the backend
type SSEClient struct {
	config  *config.Config
	client  *sse.Client
	parser  *events.Parser
	eventCh chan *events.Event
	errCh   chan error
}

// NewSSEClient creates a new SSE client
func NewSSEClient(cfg *config.Config) *SSEClient {
	client := sse.NewClient(cfg.StreamEndpoint())

	// Set up authentication header
	client.Headers = map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", cfg.APIKey),
		"Accept":        "text/event-stream",
	}

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
	endpoint := c.config.StreamEndpoint()
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}

    q := u.Query()
    q.Set("message", message)
    if c.config.DebugLevel != "" {
        q.Set("debug", c.config.DebugLevel)
    }
    u.RawQuery = q.Encode()

	// Update client URL
	c.client = sse.NewClient(u.String())
	c.client.Headers = map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", c.config.APIKey),
		"Accept":        "text/event-stream",
	}

	// Subscribe to all event types
    eventTypes := []string{
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

// SendMessage sends a message to the backend (for testing)
func (c *SSEClient) SendMessage(ctx context.Context, message string) error {
	endpoint := fmt.Sprintf("%s/api/v3/sessions/%s/stream", c.config.BackendURL, c.config.SessionID)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Set("message", message)
	req.URL.RawQuery = q.Encode()

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.config.APIKey))
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{
		Timeout: 120 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
