package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/lancekrogers/stream-debugger/internal/config"
)

// SessionSetupClient handles automatic session creation/setup
type SessionSetupClient struct {
	config *config.EnhancedConfig
	apiKey string
}

// SessionSetupRequest is the request body for creating a debug session
type SessionSetupRequest struct {
	SessionID     string   `json:"session_id,omitempty"`
	Agents        []string `json:"agents"`
	ReuseExisting bool     `json:"reuse_existing"`
}

// SessionSetupResponse is the response from the debug session endpoint
type SessionSetupResponse struct {
	Success      bool     `json:"success"`
	SessionID    string   `json:"session_id"`
	ActiveAgents []string `json:"active_agents"`
	Created      bool     `json:"created"`
	Message      string   `json:"message"`
	Error        string   `json:"error,omitempty"`
}

// NewSessionSetupClient creates a new session setup client
func NewSessionSetupClient(cfg *config.EnhancedConfig, apiKey string) *SessionSetupClient {
	return &SessionSetupClient{
		config: cfg,
		apiKey: apiKey,
	}
}

// CreateOrGetSession creates or retrieves a debug session with default agents
func (c *SessionSetupClient) CreateOrGetSession() (*SessionSetupResponse, error) {
	// Build the setup endpoint URL
	url := fmt.Sprintf("%s%s",
		c.config.Backend.BaseURL,
		c.config.Session.SetupEndpoint,
	)

	// Prepare request
	request := SessionSetupRequest{
		SessionID:     c.config.Session.ID,
		Agents:        c.config.Session.DefaultAgents,
		ReuseExisting: true,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("session setup failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result SessionSetupResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("session setup failed: %s", result.Error)
	}

	return &result, nil
}
