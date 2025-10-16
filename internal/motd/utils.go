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
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
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
