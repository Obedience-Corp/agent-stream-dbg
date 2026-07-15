package mapping

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

// SetupSpec declares a dialect's optional pre-stream handshake, e.g. a
// session-creation POST some systems require before streaming. When a
// dialect declares no setup: block, Setup is nil and RunSetup is a no-op.
type SetupSpec struct {
	Request  SetupRequest
	Response SetupResponse
}

// SetupRequest is the handshake's outgoing request, with interpolated
// method/url/body.
type SetupRequest struct {
	Method string
	URL    string
	Body   string
}

// SetupResponse declares how to validate and read the handshake's response.
type SetupResponse struct {
	RequirePath   string // gjson path that must equal RequireEquals; empty = no check
	RequireEquals any
	SessionIDPath string // gjson path to the session ID; empty = none extracted
}

// setupYAML is the raw YAML shape of a setup: block, strictly decoded.
type setupYAML struct {
	Request struct {
		Method string `yaml:"method,omitempty"`
		URL    string `yaml:"url"`
		Body   string `yaml:"body,omitempty"`
	} `yaml:"request"`
	Response struct {
		Require struct {
			Path   string `yaml:"path"`
			Equals any    `yaml:"equals"`
		} `yaml:"require,omitempty"`
		SessionID string `yaml:"session_id,omitempty"`
	} `yaml:"response,omitempty"`
}

func (s setupYAML) compile() SetupSpec {
	method := s.Request.Method
	if method == "" {
		method = "POST"
	}
	return SetupSpec{
		Request: SetupRequest{
			Method: method,
			URL:    s.Request.URL,
			Body:   s.Request.Body,
		},
		Response: SetupResponse{
			RequirePath:   s.Response.Require.Path,
			RequireEquals: s.Response.Require.Equals,
			SessionIDPath: s.Response.SessionID,
		},
	}
}

// RunSetup executes a dialect's setup: handshake (if declared) and returns
// the session ID extracted per response.session_id. If spec is nil (the
// dialect declares no setup: block), RunSetup is a no-op returning ("", nil)
// — watch-only dialects simply skip this step. httpClient defaults to
// http.DefaultClient if nil. headers (typically config.Transport.
// ResolvedHeaders()) carries auth and custom headers — the setup handshake
// is authenticated exactly like every other backend request.
func RunSetup(ctx context.Context, spec *SetupSpec, vars InterpolationVars, headers map[string]string, httpClient *http.Client) (string, error) {
	if spec == nil {
		return "", nil
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	url, err := Interpolate(spec.Request.URL, vars)
	if err != nil {
		return "", fmt.Errorf("setup: render url: %w", err)
	}

	var bodyReader io.Reader
	if spec.Request.Body != "" {
		body, err := InterpolateJSON(spec.Request.Body, vars)
		if err != nil {
			return "", fmt.Errorf("setup: render body: %w", err)
		}
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, spec.Request.Method, url, bodyReader)
	if err != nil {
		return "", fmt.Errorf("setup: build request: %w", err)
	}
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("setup: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("setup: read response: %w", err)
	}

	if spec.Response.RequirePath != "" {
		result := gjson.GetBytes(respBody, spec.Response.RequirePath)
		if !result.Exists() || result.String() != fmt.Sprintf("%v", spec.Response.RequireEquals) {
			return "", fmt.Errorf("setup: response did not satisfy require (%s == %v): %s",
				spec.Response.RequirePath, spec.Response.RequireEquals, respBody)
		}
	}

	sessionID := ""
	if spec.Response.SessionIDPath != "" {
		sessionID = gjson.GetBytes(respBody, spec.Response.SessionIDPath).String()
	}
	return sessionID, nil
}
