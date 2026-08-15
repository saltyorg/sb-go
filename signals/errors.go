package signals

import (
	"context"
	"errors"
	"strings"
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

// HandleInterruptError reports whether an error represents user interruption
// and requests the conventional SIGINT shutdown code through ctx.
func HandleInterruptError(ctx context.Context, err error) bool {
	if !IsInterruptError(err) {
		return false
	}
	Shutdown(ctx, 130)
	return true
}
