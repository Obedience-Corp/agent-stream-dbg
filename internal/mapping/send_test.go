package mapping

import (
	"net/url"
	"strings"
	"testing"
)

func TestRenderSend_NilSpecErrorsClearly(t *testing.T) {
	_, _, _, err := RenderSend(nil, InterpolationVars{})
	if err == nil {
		t.Fatal("expected error for watch-only dialect (no send: block), got nil")
	}
	if !strings.Contains(err.Error(), "watch-only") {
		t.Errorf("expected clear 'watch-only' message, got: %v", err)
	}
}

func TestRenderSend_GETStyleNoBody(t *testing.T) {
	spec := &SendSpec{
		Method: "GET",
		URL:    "{base_url}/api/v3/sessions/{session_id}/stream?message={message}",
	}
	vars := InterpolationVars{
		BaseURL:   "http://localhost:5003",
		SessionID: "s1",
		Message:   "hello there",
	}

	method, renderedURL, body, err := RenderSend(spec, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if method != "GET" {
		t.Errorf("expected GET, got %s", method)
	}
	if body != nil {
		t.Errorf("expected nil body for GET-style send, got %s", body)
	}

	want := "http://localhost:5003/api/v3/sessions/s1/stream?message=hello+there"
	if renderedURL != want {
		t.Errorf("expected %q, got %q", want, renderedURL)
	}

	// And the query string must actually parse back to the original message.
	parsed, err := url.Parse(renderedURL)
	if err != nil {
		t.Fatalf("rendered URL does not parse: %v", err)
	}
	if got := parsed.Query().Get("message"); got != vars.Message {
		t.Errorf("expected message to round-trip through the URL, got %q want %q", got, vars.Message)
	}
}

func TestRenderSend_POSTStyleWithBody(t *testing.T) {
	spec := &SendSpec{
		Method: "POST",
		URL:    "{base_url}/api/v3/sessions/{session_id}/stream",
		Body:   `{"message": "{message}", "stream": true}`,
	}
	vars := InterpolationVars{
		BaseURL:   "http://localhost:5003",
		SessionID: "s1",
		Message:   `has "quotes" and \backslash`,
	}

	method, renderedURL, body, err := RenderSend(spec, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if method != "POST" {
		t.Errorf("expected POST, got %s", method)
	}
	if renderedURL != "http://localhost:5003/api/v3/sessions/s1/stream" {
		t.Errorf("unexpected URL: %s", renderedURL)
	}
	if !strings.Contains(string(body), `"stream": true`) {
		t.Errorf("expected rendered body to contain sibling key, got %s", body)
	}
}

func TestRenderSend_MessageCannotInjectQueryParam(t *testing.T) {
	spec := &SendSpec{
		Method: "GET",
		URL:    "{base_url}/stream?message={message}",
	}
	// An attempt to smuggle a second query parameter via the message.
	vars := InterpolationVars{
		BaseURL: "http://localhost:5003",
		Message: "hello&admin=true&debug=full",
	}

	_, renderedURL, _, err := RenderSend(spec, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed, err := url.Parse(renderedURL)
	if err != nil {
		t.Fatalf("rendered URL does not parse: %v", err)
	}
	q := parsed.Query()
	if q.Get("message") != vars.Message {
		t.Errorf("expected message to round-trip exactly, got %q", q.Get("message"))
	}
	if q.Get("admin") != "" || q.Get("debug") != "" {
		t.Errorf("expected no injected query params, got admin=%q debug=%q", q.Get("admin"), q.Get("debug"))
	}
}

func TestRenderSend_UnknownPlaceholderErrors(t *testing.T) {
	spec := &SendSpec{
		Method: "GET",
		URL:    "{base_url}/stream?deployment={deployment_id}",
	}
	_, _, _, err := RenderSend(spec, InterpolationVars{BaseURL: "http://x"})
	if err == nil {
		t.Fatal("expected error for unknown placeholder, got nil")
	}
}

func TestRenderSend_MalformedBodyTemplateErrors(t *testing.T) {
	spec := &SendSpec{
		Method: "POST",
		URL:    "{base_url}/stream",
		Body:   `{"message": {message}, not valid json`,
	}
	_, _, _, err := RenderSend(spec, InterpolationVars{BaseURL: "http://x", Message: "hi"})
	if err == nil {
		t.Fatal("expected error for malformed body template, got nil")
	}
}
