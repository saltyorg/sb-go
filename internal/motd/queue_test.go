package motd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTimeoutFromContext(t *testing.T) {
	t.Run("uses fallback when no configuration and no deadline", func(t *testing.T) {
		got := timeoutFromContext(context.Background(), 0, 1*time.Second)
		if got != 1*time.Second {
			t.Fatalf("timeoutFromContext() = %v, want %v", got, 1*time.Second)
		}
	})

	t.Run("uses configured timeout when no deadline", func(t *testing.T) {
		got := timeoutFromContext(context.Background(), 5, 1*time.Second)
		if got != 5*time.Second {
			t.Fatalf("timeoutFromContext() = %v, want %v", got, 5*time.Second)
		}
	})

	t.Run("clamps to context deadline when shorter", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		got := timeoutFromContext(ctx, 5, 1*time.Second)
		if got <= 0 {
			t.Fatalf("timeoutFromContext() = %v, expected positive duration", got)
		}
		if got > 250*time.Millisecond {
			t.Fatalf("timeoutFromContext() = %v, expected clamped near context deadline", got)
		}
	})

	t.Run("returns minimum timeout when context is already expired", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
		defer cancel()

		got := timeoutFromContext(ctx, 5, 1*time.Second)
		if got != 1*time.Millisecond {
			t.Fatalf("timeoutFromContext() = %v, want %v", got, 1*time.Millisecond)
		}
	})
}

func TestFormatProviderError(t *testing.T) {
	got := formatProviderError(errors.New("dial tcp timeout\nextra details"))
	want := "Error: dial tcp timeout"
	if got != want {
		t.Fatalf("formatProviderError() = %q, want %q", got, want)
	}
}

func TestFormatDetailedQueueOutputIncludesInstanceErrors(t *testing.T) {
	out := formatDetailedQueueOutput([]QueueInfo{
		{
			Name:  "Sonarr",
			Items: []QueueItem{{Status: "Downloading"}, {Status: "Queued"}},
		},
		{
			Name:  "Radarr",
			Error: errors.New("request timeout"),
		},
	}, false)

	if !strings.Contains(out, "Sonarr:") {
		t.Fatalf("expected Sonarr output, got %q", out)
	}
	if !strings.Contains(out, "Radarr:") {
		t.Fatalf("expected Radarr output, got %q", out)
	}
	if !strings.Contains(out, "Error: request timeout") {
		t.Fatalf("expected explicit instance error output, got %q", out)
	}
}
