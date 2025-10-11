package ansible

import (
	"bytes"
	"context"
	"os/exec"
)

// CommandExecutor is an interface for executing commands
// This allows for easy mocking in tests
type CommandExecutor interface {
	ExecuteContext(ctx context.Context, name string, args ...string) ([]byte, error)
	ExecuteWithIO(ctx context.Context, name string, args []string, stdout, stderr, stdin interface{}) error
}

// RealCommandExecutor implements CommandExecutor using actual exec.Command
type RealCommandExecutor struct{}

// ExecuteContext executes a command and returns the combined output
func (e *RealCommandExecutor) ExecuteContext(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// ExecuteWithIO executes a command with custom IO streams
func (e *RealCommandExecutor) ExecuteWithIO(ctx context.Context, name string, args []string, stdout, stderr, stdin interface{}) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdout != nil {
		if w, ok := stdout.(interface{ Write([]byte) (int, error) }); ok {
			cmd.Stdout = w
		}
	}
	if stderr != nil {
		if w, ok := stderr.(interface{ Write([]byte) (int, error) }); ok {
			cmd.Stderr = w
		}
	}
	if stdin != nil {
		if r, ok := stdin.(interface{ Read([]byte) (int, error) }); ok {
			cmd.Stdin = r
		}
	}
	return cmd.Run()
}

// MockCommandExecutor is a mock implementation for testing
type MockCommandExecutor struct {
	ExecuteContextFunc func(ctx context.Context, name string, args ...string) ([]byte, error)
	ExecuteWithIOFunc  func(ctx context.Context, name string, args []string, stdout, stderr, stdin interface{}) error
}

// ExecuteContext mock implementation
func (m *MockCommandExecutor) ExecuteContext(ctx context.Context, name string, args ...string) ([]byte, error) {
	if m.ExecuteContextFunc != nil {
		return m.ExecuteContextFunc(ctx, name, args...)
	}
	return []byte{}, nil
}

// ExecuteWithIO mock implementation
func (m *MockCommandExecutor) ExecuteWithIO(ctx context.Context, name string, args []string, stdout, stderr, stdin interface{}) error {
	if m.ExecuteWithIOFunc != nil {
		return m.ExecuteWithIOFunc(ctx, name, args, stdout, stderr, stdin)
	}
	// Write some mock output if stdout is provided
	if stdout != nil {
		if w, ok := stdout.(interface{ Write([]byte) (int, error) }); ok {
			w.Write([]byte("mock output"))
		}
	}
	return nil
}

// defaultExecutor is the default executor used by the package
var defaultExecutor CommandExecutor = &RealCommandExecutor{}

// SetExecutor sets the command executor (useful for testing)
func SetExecutor(executor CommandExecutor) {
	defaultExecutor = executor
}

// GetExecutor returns the current command executor
func GetExecutor() CommandExecutor {
	return defaultExecutor
}

// createCommand is a helper that creates an exec.Cmd
// This is kept for internal use where we need the actual *exec.Cmd
func createCommand(ctx context.Context, name string, args []string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

// executeCommand executes a command with provided context and returns output buffers
func executeCommand(ctx context.Context, cmd *exec.Cmd, verbose bool) (*bytes.Buffer, *bytes.Buffer, error) {
	var stdoutBuf, stderrBuf bytes.Buffer

	if !verbose {
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
	}

	err := cmd.Run()
	return &stdoutBuf, &stderrBuf, err
}
