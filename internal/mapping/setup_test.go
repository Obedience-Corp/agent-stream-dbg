package mapping

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunSetup_NilSpecIsNoOp(t *testing.T) {
	sessionID, err := RunSetup(context.Background(), nil, InterpolationVars{}, nil, nil)
	if err != nil {
		t.Fatalf("expected no error for nil setup spec, got: %v", err)
	}
	if sessionID != "" {
		t.Errorf("expected empty session ID for nil setup spec, got %q", sessionID)
	}
}

func TestRunSetup_HappyPath(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success": true, "session_id": "srv-generated-session"}`))
	}))
	defer srv.Close()

	spec := &SetupSpec{
		Request: SetupRequest{
			Method: "POST",
			URL:    "{base_url}/api/v3/debug/session",
			Body:   `{"session_id": "{session_id}", "agents": {agents}, "reuse_existing": true}`,
		},
		Response: SetupResponse{
			RequirePath:   "success",
			RequireEquals: true,
			SessionIDPath: "session_id",
		},
	}
	vars := InterpolationVars{
		BaseURL:   srv.URL,
		SessionID: "client-session",
		Agents:    []string{"a1", "a2"},
	}

	sessionID, err := RunSetup(context.Background(), spec, vars, nil, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessionID != "srv-generated-session" {
		t.Errorf("expected extracted session_id 'srv-generated-session', got %q", sessionID)
	}
	if gotMethod != "POST" {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/api/v3/debug/session" {
		t.Errorf("expected path /api/v3/debug/session, got %s", gotPath)
	}
	if !strings.Contains(gotBody, `"session_id": "client-session"`) {
		t.Errorf("expected interpolated session_id in body, got %s", gotBody)
	}
	if !strings.Contains(gotBody, `"agents": ["a1","a2"]`) {
		t.Errorf("expected agents rendered as JSON array in body, got %s", gotBody)
	}
}

func TestRunSetup_RequireFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success": false, "error": "unauthorized"}`))
	}))
	defer srv.Close()

	spec := &SetupSpec{
		Request: SetupRequest{Method: "POST", URL: "{base_url}/setup"},
		Response: SetupResponse{
			RequirePath:   "success",
			RequireEquals: true,
			SessionIDPath: "session_id",
		},
	}
	vars := InterpolationVars{BaseURL: srv.URL}

	_, err := RunSetup(context.Background(), spec, vars, nil, srv.Client())
	if err == nil {
		t.Fatal("expected error when require check fails, got nil")
	}
	if !strings.Contains(err.Error(), "setup:") {
		t.Errorf("expected error to name the failed step ('setup:'), got: %v", err)
	}
	if !strings.Contains(err.Error(), "require") {
		t.Errorf("expected error to mention the require check, got: %v", err)
	}
}

func TestRunSetup_NetworkError(t *testing.T) {
	// A closed server: connections to it fail immediately.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	spec := &SetupSpec{
		Request: SetupRequest{Method: "POST", URL: "{base_url}/setup"},
	}
	vars := InterpolationVars{BaseURL: srv.URL}

	_, err := RunSetup(context.Background(), spec, vars, nil, srv.Client())
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	if !strings.Contains(err.Error(), "setup: request failed") {
		t.Errorf("expected error to name the failed step ('setup: request failed'), got: %v", err)
	}
}

func TestRunSetup_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	spec := &SetupSpec{
		Request: SetupRequest{Method: "POST", URL: "{base_url}/setup"},
	}
	vars := InterpolationVars{BaseURL: srv.URL}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := RunSetup(ctx, spec, vars, nil, srv.Client())
	if err == nil {
		t.Fatal("expected context deadline error, got nil")
	}
	if !strings.Contains(err.Error(), "setup: request failed") {
		t.Errorf("expected wrapped 'setup: request failed' error, got: %v", err)
	}
}

func TestRunSetup_HeadersAreApplied(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success": true}`))
	}))
	defer srv.Close()

	spec := &SetupSpec{
		Request:  SetupRequest{Method: "POST", URL: "{base_url}/setup"},
		Response: SetupResponse{RequirePath: "success", RequireEquals: true},
	}
	vars := InterpolationVars{BaseURL: srv.URL}
	headers := map[string]string{"Authorization": "Bearer test-token"}

	if _, err := RunSetup(context.Background(), spec, vars, headers, srv.Client()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("expected Authorization header to reach the server, got %q", gotAuth)
	}
}

func TestRunSetup_UnknownPlaceholderErrorsBeforeNetworkCall(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	spec := &SetupSpec{
		Request: SetupRequest{Method: "POST", URL: "{base_url}/setup", Body: `{"deployment": "{deployment_id}"}`},
	}
	vars := InterpolationVars{BaseURL: srv.URL}

	_, err := RunSetup(context.Background(), spec, vars, nil, srv.Client())
	if err == nil {
		t.Fatal("expected error for unknown placeholder, got nil")
	}
	if called {
		t.Error("expected no network call when body template fails to render")
	}
}
