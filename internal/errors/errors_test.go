package errors

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

// mockSignalManager is a mock implementation of the signal manager for testing
type mockSignalManager struct {
	shutdownCalled bool
	shutdownCode   int
	mu             sync.Mutex
}

func (m *mockSignalManager) Shutdown(code int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownCalled = true
	m.shutdownCode = code
}

func (m *mockSignalManager) WasShutdownCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shutdownCalled
}

func (m *mockSignalManager) GetShutdownCode() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shutdownCode
}

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
			// Capture stderr output
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			result := HandleInterruptError(tt.err)

			// Restore stderr
			w.Close()
			os.Stderr = oldStderr

			// Read captured output
			buf := make([]byte, 1024)
			n, _ := r.Read(buf)
			output := string(buf[:n])

			if result != tt.expectedReturn {
				t.Errorf("HandleInterruptError(%v) = %v, expected %v", tt.err, result, tt.expectedReturn)
			}

			if tt.expectedReturn {
				// Verify that the interrupt message was printed
				if !strings.Contains(output, "Command interrupted by user") {
					t.Errorf("Expected interrupt message in output, got: %q", output)
				}
			}
		})
	}
}

// TestExitWithError tests the ExitWithError function
// Note: This test cannot fully test the actual signal manager interaction
// because it uses the global signal manager. In a real-world scenario,
// you would need to refactor the code to allow dependency injection.
func TestExitWithError(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		args     []any
		expected string
	}{
		{
			name:     "simple error message",
			format:   "error occurred",
			args:     []any{},
			expected: "error occurred\n",
		},
		{
			name:     "formatted error message",
			format:   "error: %s",
			args:     []any{"file not found"},
			expected: "error: file not found\n",
		},
		{
			name:     "multiple format args",
			format:   "error at line %d: %s",
			args:     []any{42, "syntax error"},
			expected: "error at line 42: syntax error\n",
		},
		{
			name:     "error with path",
			format:   "failed to open file: %s",
			args:     []any{"/path/to/file.txt"},
			expected: "failed to open file: /path/to/file.txt\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stderr output
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			ExitWithError(tt.format, tt.args...)

			// Restore stderr
			w.Close()
			os.Stderr = oldStderr

			// Read captured output
			buf := make([]byte, 1024)
			n, _ := r.Read(buf)
			output := string(buf[:n])

			if output != tt.expected {
				t.Errorf("ExitWithError() output = %q, expected %q", output, tt.expected)
			}
		})
	}
}

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
			// Capture stderr to prevent test output pollution
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			result := HandleInterruptError(err)

			w.Close()
			os.Stderr = oldStderr
			r.Close()

			if result {
				t.Errorf("HandleInterruptError(%v) returned true, expected false", err)
			}
		})
	}
}
