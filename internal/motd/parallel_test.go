package motd

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestGetSystemInfoEnforcesProviderTimeout(t *testing.T) {
	blocked := make(chan struct{})
	t.Cleanup(func() { close(blocked) })

	sources := []InfoSource{{
		Key: "Blocked:",
		Provider: func(context.Context, bool) string {
			<-blocked
			return "late result"
		},
		Order: 1,
	}}

	start := time.Now()
	results := getSystemInfo(context.Background(), sources, false, 20*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed >= time.Second {
		t.Fatalf("GetSystemInfo did not enforce provider timeout; returned after %v", elapsed)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if !strings.Contains(results[0].Value, "provider timed out") {
		t.Fatalf("expected timeout result, got %q", results[0].Value)
	}
}

func TestGetSystemInfoRecoversProviderPanic(t *testing.T) {
	sources := []InfoSource{{
		Key: "Panicking:",
		Provider: func(context.Context, bool) string {
			panic("boom")
		},
		Order: 1,
	}}

	results := getSystemInfo(context.Background(), sources, false, time.Second)

	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if !strings.Contains(results[0].Value, "panic occurred (boom)") {
		t.Fatalf("expected panic result, got %q", results[0].Value)
	}
}
