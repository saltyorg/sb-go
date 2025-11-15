package errors

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/saltyorg/sb-go/internal/signals"
)

// IsInterruptError checks if an error is due to user interrupt (Ctrl+C).
// It detects context cancellation and signal-based termination.
func IsInterruptError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled) ||
		strings.Contains(err.Error(), "signal: killed") ||
		strings.Contains(err.Error(), "signal: interrupt")
}

// HandleInterruptError checks if the error is from a user interrupt and triggers shutdown via signal manager.
// Returns true if it was an interrupt error and shutdown was initiated.
func HandleInterruptError(err error) bool {
	if IsInterruptError(err) {
		sigManager := signals.GetGlobalManager()
		sigManager.Shutdown(130) // Standard exit code for SIGINT (128 + 2)
		return true
	}
	return false
}

// ExitWithError prints an error message and triggers shutdown via signal manager with exit code 1.
// This should be used instead of direct os.Exit(1) calls to ensure proper cleanup.
func ExitWithError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	sigManager := signals.GetGlobalManager()
	sigManager.Shutdown(1)
}
