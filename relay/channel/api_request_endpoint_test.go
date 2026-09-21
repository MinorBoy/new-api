package channel

import "testing"

func TestUpstreamRequestPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "removes query", raw: "https://provider.example/v1/chat/completions?api-version=2026", want: "/v1/chat/completions"},
		{name: "keeps nested path", raw: "https://provider.example/api/v3/contents/generations/tasks", want: "/api/v3/contents/generations/tasks"},
		{name: "fallback removes query", raw: "/v1/responses?foo=bar", want: "/v1/responses"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := upstreamRequestPath(tt.raw); got != tt.want {
				t.Fatalf("upstreamRequestPath(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
