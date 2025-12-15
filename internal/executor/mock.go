package executor

import (
	"context"
	"fmt"
	"strings"
)

// MockExecutor is a mock implementation of Executor for testing
type MockExecutor struct {
	// ExecuteFunc is called when Execute is invoked
	ExecuteFunc func(config *Config) (*Result, error)
	// ExecuteSimpleFunc is called when ExecuteSimple is invoked
	ExecuteSimpleFunc func(ctx context.Context, command string, args ...string) (*Result, error)
	// Calls tracks all Execute calls for verification
	Calls []MockCall
}

// MockCall represents a single call to Execute
type MockCall struct {
	Config *Config
	Result *Result
	Error  error
}

// NewMockExecutor creates a new mock executor
func NewMockExecutor() *MockExecutor {
	return &MockExecutor{
		Calls: make([]MockCall, 0),
	}
}

// Execute mocks command execution
func (m *MockExecutor) Execute(config *Config) (*Result, error) {
	var result *Result
	var err error

	if m.ExecuteFunc != nil {
		result, err = m.ExecuteFunc(config)
	} else {
		// Default mock behavior: return success
		result = &Result{
			ExitCode: 0,
			Combined: []byte("mock output"),
		}
		err = nil
	}

	// Track the call
	m.Calls = append(m.Calls, MockCall{
		Config: config,
		Result: result,
		Error:  err,
	})

	return result, err
}

// ExecuteSimple mocks simple command execution
func (m *MockExecutor) ExecuteSimple(ctx context.Context, command string, args ...string) (*Result, error) {
	if m.ExecuteSimpleFunc != nil {
		return m.ExecuteSimpleFunc(ctx, command, args...)
	}

	config := &Config{
		Context:    ctx,
		Command:    command,
		Args:       args,
		OutputMode: OutputModeCombined,
	}
	return m.Execute(config)
}

// Reset clears all tracked calls
func (m *MockExecutor) Reset() {
	m.Calls = make([]MockCall, 0)
	m.ExecuteFunc = nil
	m.ExecuteSimpleFunc = nil
}

// CallCount returns the number of times Execute was called
func (m *MockExecutor) CallCount() int {
	return len(m.Calls)
}

// LastCall returns the last call to Execute, or nil if no calls were made
func (m *MockExecutor) LastCall() *MockCall {
	if len(m.Calls) == 0 {
		return nil
	}
	return &m.Calls[len(m.Calls)-1]
}

// GetCall returns the call at the specified index, or nil if out of bounds
func (m *MockExecutor) GetCall(index int) *MockCall {
	if index < 0 || index >= len(m.Calls) {
		return nil
	}
	return &m.Calls[index]
}

// VerifyCommandCalled checks if a command with the given name was called
func (m *MockExecutor) VerifyCommandCalled(command string) bool {
	for _, call := range m.Calls {
		if call.Config.Command == command {
			return true
		}
	}
	return false
}

// VerifyCommandWithArgs checks if a command with specific args was called
func (m *MockExecutor) VerifyCommandWithArgs(command string, args ...string) bool {
	for _, call := range m.Calls {
		if call.Config.Command != command {
			continue
		}
		if len(call.Config.Args) != len(args) {
			continue
		}
		match := true
		for i, arg := range args {
			if call.Config.Args[i] != arg {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// WithMockResult is a helper to set up a mock that returns a specific result
func (m *MockExecutor) WithMockResult(result *Result, err error) *MockExecutor {
	m.ExecuteFunc = func(config *Config) (*Result, error) {
		return result, err
	}
	return m
}

// WithMockResultForCommand sets up a mock that returns specific results for specific commands
func (m *MockExecutor) WithMockResultForCommand(command string, result *Result, err error) *MockExecutor {
	m.ExecuteFunc = func(config *Config) (*Result, error) {
		if config.Command == command {
			return result, err
		}
		// Default behavior for other commands
		return &Result{
			ExitCode: 0,
			Combined: []byte("mock output"),
		}, nil
	}
	return m
}

// String returns a string representation of all calls for debugging
func (m *MockExecutor) String() string {
	if len(m.Calls) == 0 {
		return "MockExecutor: no calls"
	}
	var output strings.Builder
	output.WriteString(fmt.Sprintf("MockExecutor: %d calls\n", len(m.Calls)))
	for i, call := range m.Calls {
		output.WriteString(fmt.Sprintf("  Call %d: %s %v (working dir: %s, exit code: %d)\n",
			i, call.Config.Command, call.Config.Args, call.Config.WorkingDir, call.Result.ExitCode))
	}
	return output.String()
}
