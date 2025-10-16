// Package executor provides a unified interface for executing external commands with flexible
// output handling, context-based cancellation, and comprehensive error reporting.
//
// The executor package consolidates all command execution patterns across the sb-go codebase
// into a single, testable interface. It supports multiple output modes (capture, stream,
// discard, interactive), custom environment variables, working directory configuration,
// timeout support via context, and memory-efficient output handling.
//
// # Quick Start
//
// Simple command execution with combined output:
//
//	ctx := context.Background()
//	result, err := executor.Run(ctx, "echo", executor.WithArgs("hello", "world"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(string(result.Combined)) // "hello world"
//
// # Output Modes
//
// The package supports five output modes to handle different execution scenarios:
//
// OutputModeCombined (default): Captures combined stdout/stderr, ideal for most command execution
// where you need to retrieve the output.
//
//	result, err := executor.Run(ctx, "git", executor.WithArgs("status"))
//	fmt.Println(string(result.Combined))
//
// OutputModeCapture: Captures stdout and stderr separately, useful when you need to distinguish
// between normal output and errors.
//
//	result, err := executor.Run(ctx, "command",
//	    executor.WithOutputMode(executor.OutputModeCapture))
//	fmt.Println("stdout:", string(result.Stdout))
//	fmt.Println("stderr:", string(result.Stderr))
//
// OutputModeStream: Streams output directly to the terminal, ideal for long-running commands
// where you want real-time feedback.
//
//	result, err := executor.Run(ctx, "apt-get",
//	    executor.WithArgs("update"),
//	    executor.WithOutputMode(executor.OutputModeStream))
//
// OutputModeDiscard: Discards stdout while capturing stderr, perfect for commands running behind
// spinners where you only need error output.
//
//	result, err := executor.Run(ctx, "command",
//	    executor.WithOutputMode(executor.OutputModeDiscard))
//	// result.Stderr contains any error output
//
// OutputModeInteractive: Passes stdin/stdout/stderr through for interactive commands like
// ansible-playbook or interactive prompts.
//
//	result, err := executor.Run(ctx, "ansible-playbook",
//	    executor.WithArgs("playbook.yml"),
//	    executor.WithOutputMode(executor.OutputModeInteractive))
//
// # Common Patterns
//
// Verbose/Silent Pattern: A common pattern where verbose mode streams to console and
// non-verbose mode captures stderr for error reporting:
//
//	// Verbose: streams output to terminal
//	// Silent: discards stdout, captures stderr
//	err := executor.RunVerbose(ctx, "apt-get", []string{"update"}, verbose)
//
// Environment Variables: Inherit current environment and add custom variables:
//
//	result, err := executor.Run(ctx, "command",
//	    executor.WithInheritEnv("UV_PYTHON_INSTALL_DIR=/srv/python", "DEBUG=1"))
//
// Working Directory: Execute commands in a specific directory:
//
//	result, err := executor.Run(ctx, "git",
//	    executor.WithArgs("status"),
//	    executor.WithWorkingDir("/path/to/repo"))
//
// Timeout: Use context for automatic timeout:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	result, err := executor.Run(ctx, "long-running-command")
//
// # Error Handling
//
// The package provides comprehensive error handling with exit codes:
//
//	result, err := executor.Run(ctx, "command")
//	if err != nil {
//	    // Basic error handling
//	    log.Printf("Command failed with exit code %d: %v", result.ExitCode, err)
//
//	    // Detailed error with stderr output
//	    detailedErr := result.FormatError("installing packages")
//	    return detailedErr
//	}
//
// # Testing
//
// The package provides a MockExecutor for easy testing:
//
//	mock := executor.NewMockExecutor()
//	mock.WithMockResult(&executor.Result{
//	    ExitCode: 0,
//	    Combined: []byte("success"),
//	}, nil)
//
//	result, err := mock.Execute(&executor.Config{
//	    Context: context.Background(),
//	    Command: "test",
//	})
//
// # Performance Considerations
//
// - Use OutputModeDiscard instead of capturing output when working with spinners
// - Use OutputModeStream for long-running commands to avoid memory buildup
// - Executor instances are stateless and safe for concurrent use
// - Context cancellation is immediate and doesn't wait for command completion
//
// # Thread Safety
//
// The executor is safe for concurrent use. Each Execute call creates a new command instance,
// so multiple goroutines can safely use the same executor instance.
package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// OutputMode defines how command output should be handled during execution.
// Different output modes optimize for different use cases, from capturing output
// for processing to streaming real-time feedback to the terminal.
type OutputMode int

const (
	// OutputModeCapture captures stdout and stderr separately into memory buffers.
	// Use this when you need to process standard output and error streams independently.
	//
	// Example:
	//	result, _ := executor.Run(ctx, "command",
	//	    executor.WithOutputMode(executor.OutputModeCapture))
	//	fmt.Println("Output:", string(result.Stdout))
	//	fmt.Println("Errors:", string(result.Stderr))
	OutputModeCapture OutputMode = iota

	// OutputModeCombined captures combined stdout and stderr into a single buffer.
	// This is the default mode and works well for most command execution scenarios
	// where you need to retrieve and process the output. Similar to exec.CombinedOutput().
	//
	// Example:
	//	result, _ := executor.Run(ctx, "git", executor.WithArgs("status"))
	//	output := string(result.Combined)
	OutputModeCombined

	// OutputModeStream streams output directly to os.Stdout and os.Stderr in real-time.
	// Use this for long-running commands where you want users to see progress as it happens.
	// No output is captured in memory, making this memory-efficient for commands with
	// large output.
	//
	// Example:
	//	result, _ := executor.Run(ctx, "apt-get", executor.WithArgs("update"),
	//	    executor.WithOutputMode(executor.OutputModeStream))
	OutputModeStream

	// OutputModeDiscard discards stdout but captures stderr into a memory buffer.
	// Perfect for commands running behind progress spinners where you only need error
	// output for diagnostics. This prevents output from interfering with spinner display
	// while preserving error messages for troubleshooting.
	//
	// Example:
	//	spinner.Start("Installing packages...")
	//	result, _ := executor.Run(ctx, "command",
	//	    executor.WithOutputMode(executor.OutputModeDiscard))
	//	spinner.Stop()
	//	if result.Error != nil {
	//	    fmt.Println(string(result.Stderr))
	//	}
	OutputModeDiscard

	// OutputModeInteractive passes stdin, stdout, and stderr through to the terminal,
	// enabling full interactivity with preserved TTY properties (colors, terminal width).
	// Use this for commands that require user input or provide interactive interfaces,
	// such as text editors, interactive prompts, or tools like ansible-playbook.
	// Note: Output is NOT captured in this mode to preserve terminal characteristics.
	//
	// Example:
	//	result, _ := executor.Run(ctx, "ansible-playbook",
	//	    executor.WithArgs("playbook.yml", "--ask-become-pass"),
	//	    executor.WithOutputMode(executor.OutputModeInteractive))
	OutputModeInteractive
)

// Result contains the result of a command execution, including captured output,
// exit code, and any error that occurred.
//
// Output capturing behavior by mode:
//   - OutputModeCapture: Captures both stdout and stderr separately
//   - OutputModeCombined: Captures combined stdout+stderr
//   - OutputModeStream: Displays to terminal AND captures both stdout and stderr
//   - OutputModeDiscard: Discards stdout (not captured), but captures stderr for errors
//   - OutputModeInteractive: Passes through to terminal, NO capturing (preserves TTY properties)
//
// Stderr is always captured (except for OutputModeInteractive or when custom stderr
// writer is provided) to ensure error messages can include diagnostic information.
type Result struct {
	// Stdout contains captured standard output.
	// Empty for OutputModeDiscard (stdout discarded) and OutputModeInteractive (not captured).
	// Populated for all other modes.
	Stdout []byte

	// Stderr contains captured standard error output.
	// Always populated except for OutputModeInteractive (not captured).
	// Critical for error reporting and diagnostics.
	Stderr []byte

	// Combined contains combined output:
	//   - OutputModeCombined: The actual interleaved stdout+stderr stream
	//   - Other modes: stdout followed by stderr (for convenience)
	//   - OutputModeDiscard: Only stderr (since stdout was discarded)
	Combined []byte

	// ExitCode is the exit code returned by the command. A value of 0 indicates success.
	// Positive values indicate the command-specific error code. A value of -1 indicates
	// the command could not be started (e.g., command not found).
	ExitCode int

	// Error is the original error returned by the command execution, if any.
	// This will be nil if the command completed successfully (exit code 0).
	// For non-zero exit codes, this will typically be an *exec.ExitError.
	Error error
}

// Config contains all configuration options for command execution.
// Use the With* option functions to populate this struct rather than
// creating it directly, as the option functions provide validation and
// sensible defaults.
type Config struct {
	// Context for cancellation and timeout support (required).
	// The command will be terminated if the context is cancelled.
	Context context.Context

	// Command is the name or path of the command to execute (required).
	// This can be a binary name (resolved via PATH) or an absolute path.
	Command string

	// Args contains the command-line arguments to pass to the command.
	// Do not include the command name itself in Args.
	Args []string

	// WorkingDir sets the working directory for the command.
	// If empty, the command runs in the caller's current directory.
	WorkingDir string

	// Env contains environment variables for the command in the form "KEY=value".
	// If nil, the command inherits the current process's environment.
	// Use WithEnv to replace all environment variables, or WithInheritEnv
	// to inherit and add/override specific variables.
	Env []string

	// OutputMode determines how output is handled (capture, stream, discard, etc.).
	// Defaults to OutputModeCombined if not specified.
	OutputMode OutputMode

	// Stdin provides custom standard input for the command.
	// Commonly used for piping data to commands or in interactive mode.
	Stdin io.Reader

	// Stdout provides a custom writer for standard output.
	// If set, this overrides the OutputMode's stdout handling.
	Stdout io.Writer

	// Stderr provides a custom writer for standard error.
	// If set, this overrides the OutputMode's stderr handling.
	Stderr io.Writer
}

// Option is a functional option for configuring command execution.
// Options are applied in the order they are provided to Run or Execute.
// Later options can override earlier ones.
//
// Example:
//
//	result, err := executor.Run(ctx, "command",
//	    executor.WithArgs("arg1", "arg2"),
//	    executor.WithWorkingDir("/tmp"),
//	    executor.WithOutputMode(executor.OutputModeStream))
type Option func(*Config)

// WithContext sets the context for command execution.
// The context is used for cancellation and timeout support.
// If the context is cancelled, the command will be terminated.
//
// This option is typically not needed when using Run(), as it accepts
// a context parameter. Use this when configuring a Config directly for Execute().
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	result, err := executor.Run(ctx, "command")
func WithContext(ctx context.Context) Option {
	return func(c *Config) {
		c.Context = ctx
	}
}

// WithArgs sets the command-line arguments for the command.
// Arguments should not include the command name itself.
// Variable arguments make it easy to build argument lists.
//
// Example:
//
//	result, err := executor.Run(ctx, "git",
//	    executor.WithArgs("commit", "-m", "Initial commit"))
//
//	// Or with a slice:
//	args := []string{"commit", "-m", "Initial commit"}
//	result, err := executor.Run(ctx, "git", executor.WithArgs(args...))
func WithArgs(args ...string) Option {
	return func(c *Config) {
		c.Args = args
	}
}

// WithWorkingDir sets the working directory for command execution.
// If not set, the command runs in the caller's current directory.
// This is particularly useful for version control operations or
// when working with tools that expect to be run in specific directories.
//
// Example:
//
//	result, err := executor.Run(ctx, "git",
//	    executor.WithArgs("status"),
//	    executor.WithWorkingDir("/path/to/repository"))
func WithWorkingDir(dir string) Option {
	return func(c *Config) {
		c.WorkingDir = dir
	}
}

// WithEnv replaces all environment variables with the provided set.
// Each string should be in the form "KEY=value".
// The command will not inherit any environment variables from the current process.
// Use WithInheritEnv if you want to inherit the current environment and add/override
// specific variables.
//
// Example:
//
//	result, err := executor.Run(ctx, "command",
//	    executor.WithEnv([]string{
//	        "PATH=/usr/bin:/bin",
//	        "HOME=/root",
//	        "DEBUG=1",
//	    }))
func WithEnv(env []string) Option {
	return func(c *Config) {
		c.Env = env
	}
}

// WithInheritEnv inherits the current process's environment and adds or overrides
// specific environment variables. Each additional variable should be in the form "KEY=value".
// This is the most common way to set environment variables, as it preserves PATH and
// other system variables while allowing customization.
//
// Example:
//
//	result, err := executor.Run(ctx, "uv",
//	    executor.WithArgs("python", "install", "3.11"),
//	    executor.WithInheritEnv(
//	        "UV_PYTHON_INSTALL_DIR=/srv/python",
//	        "DEBUG=1",
//	    ))
func WithInheritEnv(additionalEnv ...string) Option {
	return func(c *Config) {
		c.Env = append(os.Environ(), additionalEnv...)
	}
}

// WithOutputMode sets how command output should be handled.
// See the OutputMode type documentation for details on each mode.
// The default mode is OutputModeCombined if not specified.
//
// Example:
//
//	// Stream output for long-running commands
//	result, err := executor.Run(ctx, "apt-get",
//	    executor.WithArgs("update"),
//	    executor.WithOutputMode(executor.OutputModeStream))
//
//	// Discard output when using spinners
//	result, err := executor.Run(ctx, "command",
//	    executor.WithOutputMode(executor.OutputModeDiscard))
func WithOutputMode(mode OutputMode) Option {
	return func(c *Config) {
		c.OutputMode = mode
	}
}

// WithStdin provides custom standard input for the command.
// This is useful for piping data to commands or providing programmatic input.
//
// Example:
//
//	input := bytes.NewBufferString("line1\nline2\nline3\n")
//	result, err := executor.Run(ctx, "grep",
//	    executor.WithArgs("line2"),
//	    executor.WithStdin(input))
func WithStdin(stdin io.Reader) Option {
	return func(c *Config) {
		c.Stdin = stdin
	}
}

// WithStdout provides a custom writer for standard output.
// If set, this overrides the OutputMode's stdout handling, giving you full control.
// This is useful when you need to write output to a specific destination like a file
// or custom buffer while still benefiting from the executor's other features.
//
// Example:
//
//	var buf bytes.Buffer
//	result, err := executor.Run(ctx, "command",
//	    executor.WithStdout(&buf))
//	// buf now contains the command's stdout
func WithStdout(stdout io.Writer) Option {
	return func(c *Config) {
		c.Stdout = stdout
	}
}

// WithStderr provides a custom writer for standard error.
// If set, this overrides the OutputMode's stderr handling, giving you full control.
// This is useful when you need to write error output to a specific destination.
//
// Example:
//
//	var errBuf bytes.Buffer
//	result, err := executor.Run(ctx, "command",
//	    executor.WithStderr(&errBuf))
//	// errBuf now contains the command's stderr
func WithStderr(stderr io.Writer) Option {
	return func(c *Config) {
		c.Stderr = stderr
	}
}

// Executor defines the interface for executing commands.
// This interface allows for easy mocking in tests by providing a simple
// contract that both production and test implementations can satisfy.
//
// The interface is designed to be minimal yet flexible, supporting both
// fully-configured execution via Execute() and simple cases via ExecuteSimple().
type Executor interface {
	// Execute runs a command with the given configuration and returns the result.
	// This is the primary method that provides full control over execution.
	//
	// Parameters:
	//   config - Complete configuration for command execution including context,
	//            command name, arguments, environment, and output handling.
	//
	// Returns:
	//   result - Contains captured output, exit code, and any error.
	//   error  - Non-nil if the command failed to execute or returned non-zero exit code.
	//
	// Example:
	//	executor := executor.NewExecutor()
	//	result, err := executor.Execute(&executor.Config{
	//	    Context: ctx,
	//	    Command: "git",
	//	    Args: []string{"status"},
	//	    OutputMode: executor.OutputModeCombined,
	//	})
	Execute(config *Config) (*Result, error)

	// ExecuteSimple is a convenience method for simple command execution with
	// combined output. This is equivalent to calling Execute with OutputModeCombined.
	//
	// Parameters:
	//   ctx     - Context for cancellation and timeout.
	//   command - The command to execute.
	//   args    - Variable arguments to pass to the command.
	//
	// Returns:
	//   result - Contains combined output, exit code, and any error.
	//   error  - Non-nil if the command failed.
	//
	// Example:
	//	executor := executor.NewExecutor()
	//	result, err := executor.ExecuteSimple(ctx, "echo", "hello", "world")
	ExecuteSimple(ctx context.Context, command string, args ...string) (*Result, error)
}

// DefaultExecutor is the production implementation of the Executor interface
// using the standard library's os/exec package. It is stateless and safe for
// concurrent use across multiple goroutines.
type DefaultExecutor struct{}

// NewExecutor creates a new default executor instance.
// The returned executor is safe for concurrent use and can be reused
// across multiple command executions.
//
// Example:
//
//	executor := executor.NewExecutor()
//	result1, _ := executor.Execute(config1)
//	result2, _ := executor.Execute(config2) // Safe to reuse
func NewExecutor() Executor {
	return &DefaultExecutor{}
}

// Execute runs a command with the given configuration.
// This is the main execution method that handles all output modes, environment
// configuration, and I/O redirection.
//
// The method validates that Context and Command are provided, then configures
// the command according to the Config settings. Output handling varies based on
// OutputMode, and custom I/O writers override the default mode behavior.
//
// Parameters:
//
//	config - Configuration specifying how to execute the command. Context and Command
//	         are required; other fields are optional and have sensible defaults.
//
// Returns:
//
//	result - Always returned (even on error) containing exit code and any captured output.
//	error  - Non-nil if the command failed to start or returned a non-zero exit code.
//	         The error is also stored in result.Error for convenience.
//
// Error Behavior:
//   - Returns error if config.Context is nil
//   - Returns error if config.Command is empty
//   - Returns error if command fails to start (e.g., command not found)
//   - Returns error if command exits with non-zero code
//
// Thread Safety:
//
//	Safe for concurrent use. Each call creates a new command instance.
func (e *DefaultExecutor) Execute(config *Config) (*Result, error) {
	if config.Context == nil {
		return nil, fmt.Errorf("context is required")
	}
	if config.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	cmd := exec.CommandContext(config.Context, config.Command, config.Args...)

	// Set working directory if provided
	if config.WorkingDir != "" {
		cmd.Dir = config.WorkingDir
	}

	// Set environment if provided
	if config.Env != nil {
		cmd.Env = config.Env
	}

	result := &Result{}

	// Always capture stdout and stderr internally, regardless of output mode
	var stdoutBuf, stderrBuf bytes.Buffer

	// Apply custom IO if provided (overrides OutputMode)
	if config.Stdin != nil {
		cmd.Stdin = config.Stdin
	}

	// Configure output handling based on mode
	// The mode controls what's displayed, but we always capture internally
	switch config.OutputMode {
	case OutputModeCapture:
		// Capture separately, no screen output
		if config.Stdout != nil {
			cmd.Stdout = io.MultiWriter(&stdoutBuf, config.Stdout)
		} else {
			cmd.Stdout = &stdoutBuf
		}
		if config.Stderr != nil {
			cmd.Stderr = io.MultiWriter(&stderrBuf, config.Stderr)
		} else {
			cmd.Stderr = &stderrBuf
		}

	case OutputModeCombined:
		// Capture combined output, no screen output
		if config.Stdout != nil || config.Stderr != nil {
			// Custom IO provided - capture separately
			if config.Stdout != nil {
				cmd.Stdout = io.MultiWriter(&stdoutBuf, config.Stdout)
			} else {
				cmd.Stdout = &stdoutBuf
			}
			if config.Stderr != nil {
				cmd.Stderr = io.MultiWriter(&stderrBuf, config.Stderr)
			} else {
				cmd.Stderr = &stderrBuf
			}
		} else {
			// Use CombinedOutput for efficiency when no custom IO
			output, err := cmd.CombinedOutput()
			result.Combined = output
			result.Stdout = output // Backward compatibility
			result.Stderr = output // Make stderr available too
			result.Error = err
			// Extract exit code early for CombinedOutput path
			if err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					result.ExitCode = exitErr.ExitCode()
				} else {
					result.ExitCode = -1
				}
			} else {
				result.ExitCode = 0
			}
			return result, result.Error
		}

	case OutputModeStream:
		// Stream to screen AND capture
		if config.Stdout != nil {
			cmd.Stdout = io.MultiWriter(&stdoutBuf, config.Stdout)
		} else {
			cmd.Stdout = io.MultiWriter(&stdoutBuf, os.Stdout)
		}
		if config.Stderr != nil {
			cmd.Stderr = io.MultiWriter(&stderrBuf, config.Stderr)
		} else {
			cmd.Stderr = io.MultiWriter(&stderrBuf, os.Stderr)
		}

	case OutputModeDiscard:
		// Discard stdout (don't capture), but capture stderr
		if config.Stdout != nil {
			cmd.Stdout = config.Stdout
		} else {
			cmd.Stdout = io.Discard
		}
		if config.Stderr != nil {
			cmd.Stderr = io.MultiWriter(&stderrBuf, config.Stderr)
		} else {
			cmd.Stderr = &stderrBuf
		}
		// Note: stdoutBuf will remain empty for this mode

	case OutputModeInteractive:
		// Pass through to terminal directly - DO NOT capture
		// This preserves TTY properties like colors and terminal width detection
		if config.Stdin == nil {
			cmd.Stdin = os.Stdin
		}
		if config.Stdout != nil {
			cmd.Stdout = config.Stdout
		} else {
			cmd.Stdout = os.Stdout
		}
		if config.Stderr != nil {
			cmd.Stderr = config.Stderr
		} else {
			cmd.Stderr = os.Stderr
		}
		// Note: stdoutBuf and stderrBuf remain empty for this mode
	}

	// Run the command
	result.Error = cmd.Run()

	// Always populate result fields with captured data
	result.Stdout = stdoutBuf.Bytes()
	result.Stderr = stderrBuf.Bytes()

	// For modes that don't use CombinedOutput, populate Combined field
	if config.OutputMode != OutputModeCombined {
		// Combine stdout and stderr for convenience
		result.Combined = append(result.Stdout, result.Stderr...)
	}

	// Extract exit code
	if result.Error != nil {
		var exitErr *exec.ExitError
		if errors.As(result.Error, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			// Non-exit error (e.g., command not found)
			result.ExitCode = -1
		}
	} else {
		result.ExitCode = 0
	}

	return result, result.Error
}

// ExecuteSimple is a convenience method for simple command execution with combined output.
// This method is ideal for straightforward command execution where you just need to run
// a command and get the output back.
//
// The method uses OutputModeCombined, which captures both stdout and stderr together,
// similar to exec.CombinedOutput() but with context support and better error handling.
//
// Parameters:
//
//	ctx     - Context for cancellation and timeout support.
//	command - The command name or path to execute.
//	args    - Variable arguments to pass to the command.
//
// Returns:
//
//	result - Contains combined output in result.Combined and result.Stdout, exit code,
//	         and any error that occurred.
//	error  - Non-nil if the command failed.
//
// Example:
//
//	executor := executor.NewExecutor()
//	result, err := executor.ExecuteSimple(ctx, "git", "status")
//	if err != nil {
//	    log.Printf("Command failed: %v", err)
//	}
//	fmt.Println(string(result.Combined))
func (e *DefaultExecutor) ExecuteSimple(ctx context.Context, command string, args ...string) (*Result, error) {
	config := &Config{
		Context:    ctx,
		Command:    command,
		Args:       args,
		OutputMode: OutputModeCombined,
	}
	return e.Execute(config)
}

// Run is a package-level convenience function for executing commands with options.
// This is the primary high-level function for command execution in this package,
// combining simplicity with flexibility through functional options.
//
// Run creates an executor, applies all options to build the configuration, and
// executes the command. The default output mode is OutputModeCombined unless
// overridden with WithOutputMode.
//
// Parameters:
//
//	ctx     - Context for cancellation and timeout support.
//	command - The command name or path to execute.
//	options - Zero or more functional options to configure execution.
//
// Returns:
//
//	result - Contains captured output (depending on mode), exit code, and any error.
//	error  - Non-nil if the command failed.
//
// Examples:
//
//	// Simple command with combined output
//	result, err := executor.Run(ctx, "echo", executor.WithArgs("hello"))
//
//	// Command with environment variables
//	result, err := executor.Run(ctx, "uv",
//	    executor.WithArgs("python", "install", "3.11"),
//	    executor.WithInheritEnv("UV_PYTHON_INSTALL_DIR=/srv/python"))
//
//	// Command with working directory and streaming output
//	result, err := executor.Run(ctx, "git",
//	    executor.WithArgs("status"),
//	    executor.WithWorkingDir("/path/to/repo"),
//	    executor.WithOutputMode(executor.OutputModeStream))
func Run(ctx context.Context, command string, options ...Option) (*Result, error) {
	config := &Config{
		Context:    ctx,
		Command:    command,
		OutputMode: OutputModeCombined, // Default mode
	}

	for _, opt := range options {
		opt(config)
	}

	executor := NewExecutor()
	return executor.Execute(config)
}

// RunVerbose is a convenience function that implements the verbose/silent pattern
// commonly used throughout the sb-go codebase.
//
// When verbose is true, output streams directly to the terminal (OutputModeStream).
// When verbose is false, stdout is discarded and only stderr is captured (OutputModeDiscard),
// which is then included in error messages for troubleshooting.
//
// This pattern is perfect for commands that run behind progress indicators or spinners
// in silent mode, while providing full output visibility in verbose mode.
//
// Parameters:
//
//	ctx     - Context for cancellation and timeout support.
//	command - The command name or path to execute.
//	args    - Command arguments as a slice.
//	verbose - If true, streams output to terminal. If false, discards stdout and captures stderr.
//	options - Additional functional options to customize execution.
//
// Returns:
//
//	error - Non-nil if the command failed. In non-verbose mode, the error includes
//	        captured stderr output for diagnostics.
//
// Examples:
//
//	// Verbose mode: user sees all output
//	err := executor.RunVerbose(ctx, "apt-get", []string{"update"}, true)
//
//	// Silent mode: stdout discarded, stderr captured for errors
//	err := executor.RunVerbose(ctx, "apt-get", []string{"update"}, false)
//
//	// With additional options
//	err := executor.RunVerbose(ctx, "apt-get", []string{"install", "-y", "git"}, verbose,
//	    executor.WithInheritEnv("DEBIAN_FRONTEND=noninteractive"))
func RunVerbose(ctx context.Context, command string, args []string, verbose bool, options ...Option) error {
	config := &Config{
		Context: ctx,
		Command: command,
		Args:    args,
	}

	// Apply verbose mode
	if verbose {
		config.OutputMode = OutputModeStream
	} else {
		config.OutputMode = OutputModeDiscard
	}

	// Apply additional options (can override verbose mode if needed)
	for _, opt := range options {
		opt(config)
	}

	executor := NewExecutor()
	result, err := executor.Execute(config)

	if err != nil {
		// Format error with stderr if available (now captured in both modes)
		if len(result.Stderr) > 0 {
			return fmt.Errorf("command failed: %w\nStderr:\n%s", err, string(result.Stderr))
		}
		return fmt.Errorf("command failed: %w", err)
	}

	return nil
}

// FormatError creates a detailed, user-friendly error message from the Result.
// This method is useful for providing comprehensive error information that includes
// the command description, exit code, and relevant output for troubleshooting.
//
// The method intelligently chooses between stderr (if available) or combined output
// to include in the error message, preferring stderr as it typically contains the
// most relevant error information.
//
// Parameters:
//
//	commandDescription - Human-readable description of what the command was doing
//	                     (e.g., "installing packages", "updating repository").
//	                     Can be empty if not needed.
//
// Returns:
//
//	error - A formatted error with context, or nil if r.Error is nil.
//	        The returned error wraps the original error using %w for error chain support.
//
// Examples:
//
//	result, err := executor.Run(ctx, "apt-get", executor.WithArgs("install", "nonexistent"))
//	if err != nil {
//	    // Create detailed error with context
//	    return result.FormatError("installing packages")
//	    // Error message includes: description, exit code, and stderr output
//	}
//
//	result, err := executor.Run(ctx, "git", executor.WithArgs("clone", "..."))
//	if err != nil {
//	    return result.FormatError("cloning repository")
//	}
func (r *Result) FormatError(commandDescription string) error {
	if r.Error == nil {
		return nil
	}

	var parts []string
	if commandDescription != "" {
		parts = append(parts, fmt.Sprintf("command failed: %s", commandDescription))
	}

	if r.ExitCode >= 0 {
		parts = append(parts, fmt.Sprintf("exit code: %d", r.ExitCode))
	}

	if len(r.Stderr) > 0 {
		parts = append(parts, fmt.Sprintf("stderr:\n%s", string(r.Stderr)))
	} else if len(r.Combined) > 0 {
		parts = append(parts, fmt.Sprintf("output:\n%s", string(r.Combined)))
	}

	if len(parts) > 0 {
		return fmt.Errorf("%s: %w", strings.Join(parts, ", "), r.Error)
	}

	return r.Error
}

// String returns a string representation of the Result for debugging and logging.
// The output includes the exit code and the size of each populated output buffer.
// This is useful for debugging without printing potentially large output buffers.
//
// The format is: "ExitCode: N, Stdout: X bytes, Stderr: Y bytes, Combined: Z bytes, Error: <error>"
//
// Example Output:
//
//	"ExitCode: 0, Combined: 1024 bytes"
//	"ExitCode: 1, Stderr: 512 bytes, Error: exit status 1"
//	"ExitCode: 0, Stdout: 2048 bytes, Stderr: 128 bytes"
//
// Usage:
//
//	result, _ := executor.Run(ctx, "command")
//	log.Printf("Command result: %s", result.String())
func (r *Result) String() string {
	var parts []string
	parts = append(parts, fmt.Sprintf("ExitCode: %d", r.ExitCode))
	if len(r.Stdout) > 0 {
		parts = append(parts, fmt.Sprintf("Stdout: %d bytes", len(r.Stdout)))
	}
	if len(r.Stderr) > 0 {
		parts = append(parts, fmt.Sprintf("Stderr: %d bytes", len(r.Stderr)))
	}
	if len(r.Combined) > 0 {
		parts = append(parts, fmt.Sprintf("Combined: %d bytes", len(r.Combined)))
	}
	if r.Error != nil {
		parts = append(parts, fmt.Sprintf("Error: %v", r.Error))
	}
	return strings.Join(parts, ", ")
}
