package release

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type retryPolicy struct {
	maxAttempts   int
	baseDelay     time.Duration
	maxHeaderWait time.Duration
	wait          func(context.Context, time.Duration) error
	now           func() time.Time
}

// GetWithRetry performs a release metadata GET with bounded retries for transient failures.
// The caller owns the response body, including a final non-success response after retries are exhausted.
func GetWithRetry(ctx context.Context, client *http.Client, rawURL string) (*http.Response, error) {
	return getWithRetry(ctx, client, rawURL, defaultRetryPolicy())
}

func defaultRetryPolicy() retryPolicy {
	return retryPolicy{
		maxAttempts:   4,
		baseDelay:     time.Second,
		maxHeaderWait: time.Minute,
		wait:          waitForRetry,
		now:           time.Now,
	}
}

func normalizeRetryPolicy(policy retryPolicy) retryPolicy {
	if policy.maxAttempts < 1 {
		policy.maxAttempts = 1
	}
	if policy.wait == nil {
		policy.wait = waitForRetry
	}
	if policy.now == nil {
		policy.now = time.Now
	}
	return policy
}

func getWithRetry(ctx context.Context, client *http.Client, rawURL string, policy retryPolicy) (*http.Response, error) {
	if client == nil {
		return nil, fmt.Errorf("release metadata HTTP client is required")
	}
	policy = normalizeRetryPolicy(policy)

	for attempt := 1; attempt <= policy.maxAttempts; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, fmt.Errorf("create release metadata request: %w", err)
		}
		response, err := client.Do(request)
		if err == nil && !isRetryableHTTPStatus(response.StatusCode) {
			return response, nil
		}
		if attempt == policy.maxAttempts {
			return response, err
		}
		if err != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		delay, retry := retryDelay(response, attempt, policy)
		if !retry {
			status := "transient failure"
			if response != nil {
				status = response.Status
				_ = response.Body.Close()
			}
			return nil, fmt.Errorf("release metadata retry wait %s exceeds %s limit after %s", delay, policy.maxHeaderWait, status)
		}
		if response != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
			_ = response.Body.Close()
		}

		if err := policy.wait(ctx, delay); err != nil {
			return nil, err
		}
	}

	return nil, fmt.Errorf("release metadata request exhausted retries")
}

func retryDelay(response *http.Response, attempt int, policy retryPolicy) (time.Duration, bool) {
	delay := time.Duration(1<<uint(attempt-1)) * policy.baseDelay
	if response == nil {
		return delay, true
	}
	retryAfter := strings.TrimSpace(response.Header.Get("Retry-After"))
	seconds, err := strconv.ParseInt(retryAfter, 10, 64)
	if err == nil && seconds >= 0 {
		delay = time.Duration(seconds) * time.Second
	} else if retryAt, dateErr := http.ParseTime(retryAfter); dateErr == nil {
		untilRetry := retryAt.Sub(policy.now())
		if untilRetry > 0 {
			delay = untilRetry
		}
	} else if strings.TrimSpace(response.Header.Get("X-RateLimit-Remaining")) == "0" {
		reset, resetErr := strconv.ParseInt(strings.TrimSpace(response.Header.Get("X-RateLimit-Reset")), 10, 64)
		if resetErr == nil {
			untilReset := time.Unix(reset, 0).Sub(policy.now())
			if untilReset > 0 {
				delay = untilReset
			}
		}
	}
	if policy.maxHeaderWait > 0 && delay > policy.maxHeaderWait {
		return delay, false
	}
	return delay, true
}

func isRetryableHTTPStatus(statusCode int) bool {
	return statusCode == http.StatusForbidden ||
		statusCode == http.StatusTooManyRequests ||
		(statusCode >= http.StatusInternalServerError && statusCode <= 599)
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
