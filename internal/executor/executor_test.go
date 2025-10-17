package executor

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestExecuteSimple(t *testing.T) {
	ctx := context.Background()
	executor := NewExecutor()

	result, err := executor.ExecuteSimple(ctx, "echo", "hello", "world")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	output := strings.TrimSpace(string(result.Combined))
	if output != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", output)
	}
}

func TestExecuteWithWorkingDir(t *testing.T) {
	ctx := context.Background()

	result, err := Run(ctx, "pwd", WithWorkingDir("/tmp"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	output := strings.TrimSpace(string(result.Combined))
	if output != "/tmp" {
		t.Errorf("expected '/tmp', got '%s'", output)
	}
}

func TestExecuteWithEnvironment(t *testing.T) {
	ctx := context.Background()

	result, err := Run(ctx, "sh",
		WithArgs("-c", "echo $TEST_VAR"),
		WithInheritEnv("TEST_VAR=test_value"),
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	output := strings.TrimSpace(string(result.Combined))
	if output != "test_value" {
		t.Errorf("expected 'test_value', got '%s'", output)
	}
}

func TestExecuteWithContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := Run(ctx, "sleep", WithArgs("10"))
	if err == nil {
		t.Fatal("expected context deadline error, got nil")
	}

	// Context cancellation can result in either DeadlineExceeded or a signal: killed error
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("expected context.DeadlineExceeded or signal: killed, got %v", err)
	}
}

func TestExecuteCapture(t *testing.T) {
	ctx := context.Background()

	result, err := Run(ctx, "sh",
		WithArgs("-c", "echo stdout && echo stderr >&2"),
		WithOutputMode(OutputModeCapture),
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	stdout := strings.TrimSpace(string(result.Stdout))
	stderr := strings.TrimSpace(string(result.Stderr))

	if stdout != "stdout" {
		t.Errorf("expected stdout 'stdout', got '%s'", stdout)
	}
	if stderr != "stderr" {
		t.Errorf("expected stderr 'stderr', got '%s'", stderr)
	}
}

func TestExecuteDiscard(t *testing.T) {
	ctx := context.Background()

	result, err := Run(ctx, "sh",
		WithArgs("-c", "echo stdout && echo stderr >&2 && exit 1"),
		WithOutputMode(OutputModeDiscard),
	)
	if err == nil {
		t.Fatal("expected error for exit code 1, got nil")
	}

	// Stdout should be empty (discarded)
	if len(result.Stdout) > 0 {
		t.Errorf("expected empty stdout, got %d bytes", len(result.Stdout))
	}

	// Stderr should be captured
	stderr := strings.TrimSpace(string(result.Stderr))
	if stderr != "stderr" {
		t.Errorf("expected stderr 'stderr', got '%s'", stderr)
	}

	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", result.ExitCode)
	}
}

func TestExecuteWithCustomStdin(t *testing.T) {
	ctx := context.Background()
	stdin := bytes.NewBufferString("input data\n")

	result, err := Run(ctx, "cat",
		WithOutputMode(OutputModeCombined),
		WithStdin(stdin),
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	output := strings.TrimSpace(string(result.Combined))
	if output != "input data" {
		t.Errorf("expected 'input data', got '%s'", output)
	}
}

func TestExecuteNonExistentCommand(t *testing.T) {
	ctx := context.Background()

	result, err := Run(ctx, "nonexistent_command_xyz")
	if err == nil {
		t.Fatal("expected error for non-existent command, got nil")
	}

	if result.ExitCode != -1 {
		t.Errorf("expected exit code -1 for command not found, got %d", result.ExitCode)
	}
}

func TestExecuteWithExitCode(t *testing.T) {
	ctx := context.Background()

	result, err := Run(ctx, "sh", WithArgs("-c", "exit 42"))
	if err == nil {
		t.Fatal("expected error for non-zero exit code, got nil")
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Errorf("expected ExitError, got %T", err)
	}

	if result.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", result.ExitCode)
	}
}

func TestRunVerbose(t *testing.T) {
	ctx := context.Background()

	// Test verbose mode (should not error)
	err := RunVerbose(ctx, "echo", []string{"test"}, true)
	if err != nil {
		t.Errorf("expected no error in verbose mode, got %v", err)
	}

	// Test non-verbose mode (should not error)
	err = RunVerbose(ctx, "echo", []string{"test"}, false)
	if err != nil {
		t.Errorf("expected no error in non-verbose mode, got %v", err)
	}

	// Test error in non-verbose mode (should include stderr)
	err = RunVerbose(ctx, "sh", []string{"-c", "echo error >&2 && exit 1"}, false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "Stderr:") {
		t.Errorf("expected error to contain 'Stderr:', got %v", err)
	}
}

func TestFormatError(t *testing.T) {
	result := &Result{
		ExitCode: 1,
		Stderr:   []byte("error message"),
		Error:    errors.New("command failed"),
	}

	err := result.FormatError("test command")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "test command") {
		t.Errorf("expected error to contain 'test command', got %s", errStr)
	}
	if !strings.Contains(errStr, "exit code: 1") {
		t.Errorf("expected error to contain 'exit code: 1', got %s", errStr)
	}
	if !strings.Contains(errStr, "error message") {
		t.Errorf("expected error to contain 'error message', got %s", errStr)
	}
}

func TestResultString(t *testing.T) {
	result := &Result{
		ExitCode: 0,
		Stdout:   []byte("stdout data"),
		Stderr:   []byte("stderr data"),
		Error:    nil,
	}

	str := result.String()
	if !strings.Contains(str, "ExitCode: 0") {
		t.Errorf("expected string to contain 'ExitCode: 0', got %s", str)
	}
	if !strings.Contains(str, "Stdout: 11 bytes") {
		t.Errorf("expected string to contain 'Stdout: 11 bytes', got %s", str)
	}
	if !strings.Contains(str, "Stderr: 11 bytes") {
		t.Errorf("expected string to contain 'Stderr: 11 bytes', got %s", str)
	}
}

func TestConfigValidation(t *testing.T) {
	executor := NewExecutor()

	// Test missing context
	result, err := executor.Execute(&Config{
		Command: "echo",
	})
	if err == nil {
		t.Error("expected error for missing context, got nil")
	}
	if result != nil {
		t.Error("expected nil result for invalid config")
	}

	// Test missing command
	result, err = executor.Execute(&Config{
		Context: context.Background(),
	})
	if err == nil {
		t.Error("expected error for missing command, got nil")
	}
	if result != nil {
		t.Error("expected nil result for invalid config")
	}
}

func TestWithOptions(t *testing.T) {
	ctx := context.Background()
	config := &Config{
		Context:    ctx,
		Command:    "test",
		OutputMode: OutputModeCapture,
	}

	// Test WithArgs
	WithArgs("arg1", "arg2")(config)
	if len(config.Args) != 2 || config.Args[0] != "arg1" || config.Args[1] != "arg2" {
		t.Errorf("WithArgs failed, got %v", config.Args)
	}

	// Test WithWorkingDir
	WithWorkingDir("/tmp")(config)
	if config.WorkingDir != "/tmp" {
		t.Errorf("WithWorkingDir failed, got %s", config.WorkingDir)
	}

	// Test WithOutputMode
	WithOutputMode(OutputModeStream)(config)
	if config.OutputMode != OutputModeStream {
		t.Errorf("WithOutputMode failed, got %v", config.OutputMode)
	}

	// Test WithInheritEnv
	WithInheritEnv("TEST=value")(config)
	if len(config.Env) == 0 {
		t.Error("WithInheritEnv failed, env is empty")
	}
	found := slices.Contains(config.Env, "TEST=value")
	if !found {
		t.Error("WithInheritEnv failed, TEST=value not found in env")
	}
}
