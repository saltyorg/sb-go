package motd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractTraefikRouterError(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		want     string
		contains string
	}{
		{
			name: "empty raw message",
			raw:  "",
			want: "",
		},
		{
			name: "null raw message",
			raw:  "null",
			want: "",
		},
		{
			name: "string error",
			raw:  `"router is invalid"`,
			want: "router is invalid",
		},
		{
			name: "array error",
			raw:  `["first error","second error"]`,
			want: "first error",
		},
		{
			name: "object with message",
			raw:  `{"message":"bad route"}`,
			want: "bad route",
		},
		{
			name:     "object without message",
			raw:      `{"foo":"bar"}`,
			contains: `"foo":"bar"`,
		},
		{
			name: "number fallback",
			raw:  `123`,
			want: "123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTraefikRouterError(json.RawMessage(tt.raw))
			if tt.contains != "" {
				if !strings.Contains(got, tt.contains) {
					t.Fatalf("extractTraefikRouterError() = %q, expected to contain %q", got, tt.contains)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("extractTraefikRouterError() = %q, want %q", got, tt.want)
			}
		})
	}
}
