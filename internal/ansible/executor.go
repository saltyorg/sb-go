package ansible

import (
	"context"
	"os/exec"
)

// CommandExecutor is an interface for executing commands
// This allows for easy mocking in tests
type CommandExecutor interface {
	ExecuteContext(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
	ExecuteWithIO(ctx context.Context, dir string, name string, args []string, stdout, stderr, stdin any) error
}

// RealCommandExecutor implements CommandExecutor using actual exec.Command
type RealCommandExecutor struct{}

// ExecuteContext executes a command and returns the combined output
func (e *RealCommandExecutor) ExecuteContext(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.CombinedOutput()
}

// ExecuteWithIO executes a command with custom IO streams
func (e *RealCommandExecutor) ExecuteWithIO(ctx context.Context, dir string, name string, args []string, stdout, stderr, stdin any) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
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
	ExecuteContextFunc func(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
	ExecuteWithIOFunc  func(ctx context.Context, dir string, name string, args []string, stdout, stderr, stdin any) error
}

// ExecuteContext mock implementation
func (m *MockCommandExecutor) ExecuteContext(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	if m.ExecuteContextFunc != nil {
		return m.ExecuteContextFunc(ctx, dir, name, args...)
	}
	return []byte{}, nil
}

// ExecuteWithIO mock implementation
func (m *MockCommandExecutor) ExecuteWithIO(ctx context.Context, dir string, name string, args []string, stdout, stderr, stdin any) error {
	if m.ExecuteWithIOFunc != nil {
		return m.ExecuteWithIOFunc(ctx, dir, name, args, stdout, stderr, stdin)
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
