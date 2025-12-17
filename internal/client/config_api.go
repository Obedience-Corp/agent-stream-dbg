package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/lancekrogers/stream-debugger/internal/config"
)

// ConfigAPIClient handles flow config and reload API calls
type ConfigAPIClient struct {
	config *config.EnhancedConfig
	apiKey string
}

// NewConfigAPIClient creates a new config API client
func NewConfigAPIClient(cfg *config.EnhancedConfig, apiKey string) *ConfigAPIClient {
	return &ConfigAPIClient{
		config: cfg,
		apiKey: apiKey,
	}
}

// GetFlowVizConfig fetches the current flow visualization config
func (c *ConfigAPIClient) GetFlowVizConfig() ([]byte, int, error) {
	url := fmt.Sprintf("%s/api/flow-viz-config", c.config.Backend.BaseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	return body, resp.StatusCode, nil
}

// ReloadPrompts triggers a prompt reload and returns validation results
func (c *ConfigAPIClient) ReloadPrompts() ([]byte, int, error) {
	url := fmt.Sprintf("%s/api/reload-prompts", c.config.Backend.BaseURL)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte("{}")))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	return body, resp.StatusCode, nil
}

// PrettyJSON formats JSON with indentation
func PrettyJSON(data []byte) string {
	var out bytes.Buffer
	if err := json.Indent(&out, data, "", "  "); err == nil {
		return out.String()
	}
	return string(data)
}
