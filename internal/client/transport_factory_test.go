package client

import "testing"

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
