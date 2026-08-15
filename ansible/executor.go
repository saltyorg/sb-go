package ansible

import (
	"context"
	"io"

	"github.com/saltyorg/sb-go/executor"
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

// NewCommandExecutor returns the production Ansible command executor.
func NewCommandExecutor() CommandExecutor {
	return &RealCommandExecutor{executor: executor.NewExecutor()}
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

type executorContextKey struct{}

// WithExecutor returns a context whose Ansible operations use exec.
func WithExecutor(ctx context.Context, exec CommandExecutor) context.Context {
	if exec == nil {
		return ctx
	}
	return context.WithValue(ctx, executorContextKey{}, exec)
}

func executorFromContext(ctx context.Context) CommandExecutor {
	if exec, ok := ctx.Value(executorContextKey{}).(CommandExecutor); ok {
		return exec
	}
	return NewCommandExecutor()
}
