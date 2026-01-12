package motd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/executor"
)

// ExecCommand executes a command and returns its output as a string
func ExecCommand(ctx context.Context, name string, args ...string) string {
	// Add timeout to context if not already set
	ctx, cancel := applyTimeout(ctx, 5*time.Second)
	if cancel != nil {
		defer cancel()
	}

	result, err := executor.Run(ctx, name,
		executor.WithArgs(args...),
		executor.WithOutputMode(executor.OutputModeCapture),
	)
	if err != nil {
		return "Not available"
	}
	return strings.TrimSpace(string(result.Stdout))
}

func applyTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= timeout {
		return ctx, nil
	}
	return context.WithTimeout(ctx, timeout)
}

// formatBytes converts bytes to a human-readable string (KB, MB, GB, etc.)
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
