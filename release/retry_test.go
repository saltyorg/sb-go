package release

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetWithRetryRetriesTransientStatus(t *testing.T) {
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 4 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	var delays []time.Duration
	response, err := getWithRetry(t.Context(), server.Client(), server.URL, retryPolicy{
		maxAttempts:   4,
		baseDelay:     time.Second,
		maxHeaderWait: time.Minute,
		wait: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
		now: time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("response body = %q, want %q", body, "ok")
	}
	if attempts != 4 {
		t.Fatalf("request attempts = %d, want 4", attempts)
	}
	wantDelays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	if len(delays) != len(wantDelays) {
		t.Fatalf("retry delays = %v, want %v", delays, wantDelays)
	}
	for i := range wantDelays {
		if delays[i] != wantDelays[i] {
			t.Fatalf("retry delays = %v, want %v", delays, wantDelays)
		}
	}
}

func TestGetWithRetryHonorsRetryAfter(t *testing.T) {
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "12")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	var delays []time.Duration
	response, err := getWithRetry(t.Context(), server.Client(), server.URL, retryPolicy{
		maxAttempts:   4,
		baseDelay:     time.Second,
		maxHeaderWait: time.Minute,
		wait: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
		now: time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if attempts != 2 {
		t.Fatalf("request attempts = %d, want 2", attempts)
	}
	if len(delays) != 1 || delays[0] != 12*time.Second {
		t.Fatalf("retry delays = %v, want [12s]", delays)
	}
}

func TestGetWithRetryHonorsRetryAfterHTTPDate(t *testing.T) {
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "Sun, 30 Aug 2026 12:00:20 GMT")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	var delays []time.Duration
	response, err := getWithRetry(t.Context(), server.Client(), server.URL, retryPolicy{
		maxAttempts:   4,
		baseDelay:     time.Second,
		maxHeaderWait: time.Minute,
		wait: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
		now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if attempts != 2 {
		t.Fatalf("request attempts = %d, want 2", attempts)
	}
	if len(delays) != 1 || delays[0] != 20*time.Second {
		t.Fatalf("retry delays = %v, want [20s]", delays)
	}
}

func TestGetWithRetryHonorsGitHubRateLimitReset(t *testing.T) {
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", "1788091220")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	var delays []time.Duration
	response, err := getWithRetry(t.Context(), server.Client(), server.URL, retryPolicy{
		maxAttempts:   4,
		baseDelay:     time.Second,
		maxHeaderWait: time.Minute,
		wait: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
		now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if attempts != 2 {
		t.Fatalf("request attempts = %d, want 2", attempts)
	}
	if len(delays) != 1 || delays[0] != 20*time.Second {
		t.Fatalf("retry delays = %v, want [20s]", delays)
	}
}

func TestGetWithRetryDoesNotWaitPastBound(t *testing.T) {
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1788091320")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	var delays []time.Duration
	response, err := getWithRetry(t.Context(), server.Client(), server.URL, retryPolicy{
		maxAttempts:   4,
		baseDelay:     time.Second,
		maxHeaderWait: time.Minute,
		wait: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
		now: func() time.Time { return now },
	})
	if err == nil {
		if response != nil {
			_ = response.Body.Close()
		}
		t.Fatal("getWithRetry() returned nil error for an excessive retry wait")
	}
	if response != nil {
		_ = response.Body.Close()
		t.Fatalf("getWithRetry() response = %v, want nil on excessive retry wait", response.Status)
	}
	for _, want := range []string{"2m0s", "1m0s"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("getWithRetry() error = %q, want %q", err, want)
		}
	}
	if attempts != 1 {
		t.Fatalf("request attempts = %d, want 1", attempts)
	}
	if len(delays) != 0 {
		t.Fatalf("retry delays = %v, want none", delays)
	}
}

func TestGetWithRetryStopsWhenContextCanceledDuringBackoff(t *testing.T) {
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	response, err := getWithRetry(t.Context(), server.Client(), server.URL, retryPolicy{
		maxAttempts:   4,
		baseDelay:     time.Second,
		maxHeaderWait: time.Minute,
		wait: func(context.Context, time.Duration) error {
			return context.Canceled
		},
		now: time.Now,
	})
	if response != nil {
		_ = response.Body.Close()
		t.Fatalf("getWithRetry() response = %v, want nil", response.Status)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("getWithRetry() error = %v, want context cancellation", err)
	}
	if attempts != 1 {
		t.Fatalf("request attempts = %d, want 1", attempts)
	}
}

func TestGetWithRetryDoesNotRetryNonHTTPStatus(t *testing.T) {
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(600)
	}))
	defer server.Close()

	var delays []time.Duration
	response, err := getWithRetry(t.Context(), server.Client(), server.URL, retryPolicy{
		maxAttempts:   4,
		baseDelay:     time.Second,
		maxHeaderWait: time.Minute,
		wait: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
		now: time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != 600 {
		t.Fatalf("response status = %d, want 600", response.StatusCode)
	}
	if attempts != 1 {
		t.Fatalf("request attempts = %d, want 1", attempts)
	}
	if len(delays) != 0 {
		t.Fatalf("retry delays = %v, want none", delays)
	}
}
