package client

import (
	"testing"

	"github.com/lancekrogers/stream-debugger/internal/mapping"
)

func TestWithDebugQueryPreservesAndSetsQueryValues(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{name: "without existing query", base: "https://example.test/stream", want: "https://example.test/stream?debug=verbose"},
		{name: "with existing query", base: "https://example.test/stream?message=hello&debug=old", want: "https://example.test/stream?debug=verbose&message=hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := withDebugQuery(tt.base, "verbose")
			if err != nil {
				t.Fatalf("withDebugQuery: %v", err)
			}
			if got != tt.want {
				t.Errorf("withDebugQuery(%q): got %q, want %q", tt.base, got, tt.want)
			}
		})
	}
}

func TestInterpolateGRPCRequest_RendersNestedTurnValues(t *testing.T) {
	vars := mapping.InterpolationVars{
		SessionID: "session-1",
		Message:   `hello "agent"`,
		Vars:      map[string]string{"campaign": "camp-1"},
	}
	request, err := interpolateGRPCRequest(map[string]any{
		"session_id": "{session_id}",
		"message":    "{message}",
		"nested": map[string]any{
			"campaign_id": "{campaign}",
		},
		"flags": []any{"{session_id}", true},
	}, vars)
	if err != nil {
		t.Fatalf("interpolateGRPCRequest: %v", err)
	}
	if request["session_id"] != "session-1" || request["message"] != `hello "agent"` {
		t.Fatalf("unexpected rendered request: %#v", request)
	}
	nested := request["nested"].(map[string]any)
	if nested["campaign_id"] != "camp-1" {
		t.Errorf("expected nested campaign ID, got %#v", nested["campaign_id"])
	}
	flags := request["flags"].([]any)
	if flags[0] != "session-1" || flags[1] != true {
		t.Errorf("expected rendered list, got %#v", flags)
	}
}

func TestInterpolateGRPCRequest_RejectsUnknownPlaceholder(t *testing.T) {
	_, err := interpolateGRPCRequest(map[string]any{"message": "{missing}"}, mapping.InterpolationVars{})
	if err == nil {
		t.Fatal("expected unknown placeholder to fail before dialing")
	}
}
