package signals

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

// TestIsInterruptError tests the IsInterruptError function with various error types
func TestIsInterruptError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "context.Canceled error",
			err:      context.Canceled,
			expected: true,
		},
		{
			name:     "wrapped context.Canceled error",
			err:      fmt.Errorf("operation failed: %w", context.Canceled),
			expected: true,
		},
		{
			name:     "signal killed error",
			err:      errors.New("signal: killed"),
			expected: true,
		},
		{
			name:     "signal interrupt error",
			err:      errors.New("signal: interrupt"),
			expected: true,
		},
		{
			name:     "error containing signal: killed",
			err:      errors.New("command failed with: signal: killed"),
			expected: true,
		},
		{
			name:     "error containing signal: interrupt",
			err:      errors.New("process terminated: signal: interrupt"),
			expected: true,
		},
		{
			name:     "regular error",
			err:      errors.New("regular error"),
			expected: false,
		},
		{
			name:     "EOF error",
			err:      fmt.Errorf("unexpected EOF"),
			expected: false,
		},
		{
			name:     "timeout error",
			err:      context.DeadlineExceeded,
			expected: false,
		},
		{
			name:     "custom error type",
			err:      &customError{msg: "custom error"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsInterruptError(tt.err)
			if result != tt.expected {
				t.Errorf("IsInterruptError(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

// customError is a custom error type for testing
type customError struct {
	msg string
}

func (e *customError) Error() string {
	return e.msg
}

// TestIsInterruptError_EdgeCases tests edge cases for IsInterruptError
func TestIsInterruptError_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "error with 'signal:' but not interrupt or killed",
			err:      errors.New("signal: term"),
			expected: false,
		},
		{
			name:     "error with 'interrupt' but not signal",
			err:      errors.New("connection interrupt detected"),
			expected: false,
		},
		{
			name:     "error with 'killed' but not signal",
			err:      errors.New("process killed by user"),
			expected: false,
		},
		{
			name:     "case sensitive signal: interrupt",
			err:      errors.New("SIGNAL: INTERRUPT"),
			expected: false,
		},
		{
			name:     "multiple wrapped errors with context.Canceled",
			err:      fmt.Errorf("outer: %w", fmt.Errorf("middle: %w", context.Canceled)),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsInterruptError(tt.err)
			if result != tt.expected {
				t.Errorf("IsInterruptError(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

// TestHandleInterruptError tests the HandleInterruptError function
// Note: This test cannot fully test the actual signal manager interaction
// because it uses the global signal manager. In a real-world scenario,
// you would need to refactor the code to allow dependency injection.
func TestHandleInterruptError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedReturn bool
	}{
		{
			name:           "context.Canceled error",
			err:            context.Canceled,
			expectedReturn: true,
		},
		{
			name:           "signal interrupt error",
			err:            errors.New("signal: interrupt"),
			expectedReturn: true,
		},
		{
			name:           "signal killed error",
			err:            errors.New("signal: killed"),
			expectedReturn: true,
		},
		{
			name:           "regular error",
			err:            errors.New("regular error"),
			expectedReturn: false,
		},
		{
			name:           "nil error",
			err:            nil,
			expectedReturn: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HandleInterruptError(context.Background(), tt.err)

			if result != tt.expectedReturn {
				t.Errorf("HandleInterruptError(%v) = %v, expected %v", tt.err, result, tt.expectedReturn)
			}
		})
	}
}

// TestExitWithError tests the ExitWithError function
// Note: This test cannot fully test the actual signal manager interaction
// because it uses the global signal manager. In a real-world scenario,
// TestIsInterruptError_Concurrency tests IsInterruptError with concurrent access
func TestIsInterruptError_Concurrency(t *testing.T) {
	// Test that IsInterruptError is safe for concurrent use
	errors := []error{
		nil,
		context.Canceled,
		errors.New("signal: interrupt"),
		errors.New("signal: killed"),
		errors.New("regular error"),
	}

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := errors[idx%len(errors)]
			_ = IsInterruptError(err)
		}(i)
	}

	wg.Wait()
}

// TestHandleInterruptError_NonInterruptErrors tests that HandleInterruptError
// doesn't trigger shutdown for non-interrupt errors
func TestHandleInterruptError_NonInterruptErrors(t *testing.T) {
	nonInterruptErrors := []error{
		errors.New("regular error"),
		fmt.Errorf("wrapped error: %w", errors.New("base error")),
		context.DeadlineExceeded,
		&customError{msg: "custom"},
		fmt.Errorf("operation failed"),
	}

	for _, err := range nonInterruptErrors {
		t.Run(fmt.Sprintf("error_%v", err), func(t *testing.T) {
			result := HandleInterruptError(context.Background(), err)

			if result {
				t.Errorf("HandleInterruptError(%v) returned true, expected false", err)
			}
		})
	}
}

func TestHandleInterruptErrorRequestsShutdown(t *testing.T) {
	requested := 0
	ctx := WithShutdown(context.Background(), func(code int) {
		requested = code
	})

	if !HandleInterruptError(ctx, context.Canceled) {
		t.Fatal("HandleInterruptError() did not classify cancellation as an interrupt")
	}
	if requested != 130 {
		t.Fatalf("shutdown code = %d, want 130", requested)
	}
}
