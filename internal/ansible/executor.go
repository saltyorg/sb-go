package ansible

import (
	"context"
	"io"

	"github.com/saltyorg/sb-go/internal/executor"
)

// CommandExecutor is an interface for executing commands
// This allows for easy mocking in tests
type CommandExecutor interface {
	ExecuteContext(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
	ExecuteWithIO(ctx context.Context, dir string, name string, args []string, stdout, stderr, stdin any) error
}

// RealCommandExecutor implements CommandExecutor using the unified executor
type RealCommandExecutor struct {
	executor executor.Executor
}

// ExecuteContext executes a command and returns the combined output
func (e *RealCommandExecutor) ExecuteContext(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
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

// ExecuteWithIO executes a command with custom IO streams
func (e *RealCommandExecutor) ExecuteWithIO(ctx context.Context, dir string, name string, args []string, stdout, stderr, stdin any) error {
	config := &executor.Config{
		Context:    ctx,
		Command:    name,
		Args:       args,
		WorkingDir: dir,
		OutputMode: executor.OutputModeCapture,
	}

	if stdout != nil {
		if w, ok := stdout.(io.Writer); ok {
			config.Stdout = w
		}
	}
	if stderr != nil {
		if w, ok := stderr.(io.Writer); ok {
			config.Stderr = w
		}
	}
	if stdin != nil {
		if r, ok := stdin.(io.Reader); ok {
			config.Stdin = r
		}
	}

	_, err := e.executor.Execute(config)
	return err
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
		if w, ok := stdout.(io.Writer); ok {
			w.Write([]byte("mock output"))
		}
	}
	return nil
}

// defaultExecutor is the default executor used by the package
var defaultExecutor CommandExecutor = &RealCommandExecutor{
	executor: executor.NewExecutor(),
}

// SetExecutor sets the command executor (useful for testing)
func SetExecutor(executor CommandExecutor) {
	defaultExecutor = executor
}

// GetExecutor returns the current command executor
func GetExecutor() CommandExecutor {
	return defaultExecutor
}
