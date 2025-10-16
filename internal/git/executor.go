package git

import (
	"context"

	"github.com/saltyorg/sb-go/internal/executor"
)

// CommandExecutor defines an interface for executing git commands.
// This allows for easy mocking in tests.
type CommandExecutor interface {
	// ExecuteCommand executes a command and returns the combined output and error
	ExecuteCommand(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
}

// DefaultCommandExecutor is the production implementation of CommandExecutor
// It now uses the unified executor package
type DefaultCommandExecutor struct {
	executor executor.Executor
}

// ExecuteCommand executes a command using the unified executor
func (e *DefaultCommandExecutor) ExecuteCommand(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	result, err := e.executor.Execute(&executor.Config{
		Context:    ctx,
		Command:    name,
		Args:       args,
		WorkingDir: dir,
		OutputMode: executor.OutputModeCombined,
	})
	if err != nil {
		return result.Combined, err
	}
	return result.Combined, nil
}

// defaultExecutor is the global executor instance used by package functions
var defaultExecutor CommandExecutor = &DefaultCommandExecutor{
	executor: executor.NewExecutor(),
}

// SetExecutor allows replacing the default executor (primarily for testing)
func SetExecutor(exec CommandExecutor) {
	defaultExecutor = exec
}

// GetExecutor returns the current executor
func GetExecutor() CommandExecutor {
	return defaultExecutor
}

// BuildCloneArgs constructs git clone command arguments
func BuildCloneArgs(repoURL, destPath, branch string) []string {
	return []string{"clone", "--depth", "1", "-b", branch, repoURL, destPath}
}

// BuildRevParseArgs constructs git rev-parse command arguments
func BuildRevParseArgs() []string {
	return []string{"rev-parse", "HEAD"}
}

// BuildRevParseBranchArgs constructs git rev-parse command arguments for branch name
func BuildRevParseBranchArgs() []string {
	return []string{"rev-parse", "--abbrev-ref", "HEAD"}
}

// ParseCommitHash extracts and trims the commit hash from git output
func ParseCommitHash(output []byte) string {
	// Trim whitespace from output
	result := string(output)
	return trimSpace(result)
}

// ParseBranchName extracts and trims the branch name from git output
func ParseBranchName(output []byte) string {
	result := string(output)
	return trimSpace(result)
}

// trimSpace is a simple string trimming function
func trimSpace(s string) string {
	start := 0
	end := len(s)

	// Trim leading whitespace
	for start < end && isSpace(s[start]) {
		start++
	}

	// Trim trailing whitespace
	for end > start && isSpace(s[end-1]) {
		end--
	}

	return s[start:end]
}

// isSpace checks if a byte is a whitespace character
func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
