package git

import (
	"bytes"
	"context"
	"os/exec"
)

// CommandExecutor defines an interface for executing git commands.
// This allows for easy mocking in tests.
type CommandExecutor interface {
	// ExecuteCommand executes a command and returns the combined output and error
	ExecuteCommand(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
}

// DefaultCommandExecutor is the production implementation of CommandExecutor
type DefaultCommandExecutor struct{}

// ExecuteCommand executes a command using exec.CommandContext
func (e *DefaultCommandExecutor) ExecuteCommand(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// defaultExecutor is the global executor instance used by package functions
var defaultExecutor CommandExecutor = &DefaultCommandExecutor{}

// SetExecutor allows replacing the default executor (primarily for testing)
func SetExecutor(executor CommandExecutor) {
	defaultExecutor = executor
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
	// Use bytes package for efficiency
	return string(bytes.TrimSpace(output))
}

// ParseBranchName extracts and trims the branch name from git output
func ParseBranchName(output []byte) string {
	return string(bytes.TrimSpace(output))
}
