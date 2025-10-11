package motd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ExecCommand executes a command and returns its output as a string
func ExecCommand(name string, args ...string) string {
	// Use context with timeout for command execution
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.Output()
	if err != nil {
		return "Not available"
	}
	return strings.TrimSpace(string(output))
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
