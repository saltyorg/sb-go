package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/saltyorg/sb-go/executor"
	"github.com/saltyorg/sb-go/terminal"
)

const gitHTTP11Config = "http.version=HTTP/1.1"

// RunRemoteCommand executes a Git operation that may contact a remote. When
// GitHub unexpectedly requests credentials, it retries once over HTTP/1.1.
// If task is non-nil, the retry is shown as its child task.
func RunRemoteCommand(
	ctx context.Context,
	task *terminal.Task,
	repositoryName, workingDir string,
	outputMode executor.OutputMode,
	args ...string,
) (*executor.Result, error) {
	result, err := runGit(ctx, workingDir, outputMode, args)
	if err == nil {
		return result, nil
	}
	if !isGitHubAuthenticationFailure(result) {
		return result, formatGitCommandError(args, result, err)
	}

	retryArgs := make([]string, 0, len(args)+2)
	retryArgs = append(retryArgs, "-c", gitHTTP11Config)
	retryArgs = append(retryArgs, args...)
	retryResult, retryErr := runGitHTTP11Fallback(
		ctx,
		task,
		repositoryName,
		gitRemoteOperation(args),
		workingDir,
		outputMode,
		retryArgs,
	)
	if retryErr == nil {
		return retryResult, nil
	}
	if !isGitHubAuthenticationFailure(retryResult) {
		return retryResult, formatGitCommandError(retryArgs, retryResult, retryErr)
	}

	return retryResult, &publicGitHubAuthenticationError{
		repositoryName: repositoryName,
		details:        resultOutput(retryResult),
		cause:          retryErr,
	}
}

func runGitHTTP11Fallback(
	ctx context.Context,
	task *terminal.Task,
	repositoryName, operation, workingDir string,
	outputMode executor.OutputMode,
	args []string,
) (*executor.Result, error) {
	if task == nil {
		return runGit(ctx, workingDir, outputMode, args)
	}

	var result *executor.Result
	err := task.RunStreaming(ctx, terminal.TaskSpec{
		Running: fmt.Sprintf("Retrying %s Git %s over HTTP/1.1", repositoryName, operation),
		Success: fmt.Sprintf("Retried %s Git %s over HTTP/1.1", repositoryName, operation),
		Failure: fmt.Sprintf("%s Git %s retry over HTTP/1.1", repositoryName, operation),
	}, func(taskCtx context.Context, _ *terminal.Task) error {
		var err error
		result, err = runGit(taskCtx, workingDir, outputMode, args)
		return err
	})
	return result, err
}

func gitRemoteOperation(args []string) string {
	if len(args) == 0 {
		return "operation"
	}
	if args[0] == "submodule" && len(args) > 1 {
		return strings.Join(args[:2], " ")
	}
	return args[0]
}

func runGit(
	ctx context.Context,
	workingDir string,
	outputMode executor.OutputMode,
	args []string,
) (*executor.Result, error) {
	return executor.Run(ctx, "git",
		executor.WithArgs(args...),
		executor.WithWorkingDir(workingDir),
		executor.WithOutputMode(outputMode))
}

func isGitHubAuthenticationFailure(result *executor.Result) bool {
	output := resultOutput(result)
	return strings.Contains(output, "could not read ") &&
		strings.Contains(output, "for 'https://github.com") &&
		strings.Contains(output, "terminal prompts disabled")
}

func formatGitCommandError(args []string, result *executor.Result, err error) error {
	command := append([]string{"git"}, args...)
	if output := resultOutput(result); output != "" {
		return fmt.Errorf("failed to execute command %v: %w\n%s", command, err, output)
	}
	return fmt.Errorf("failed to execute command %v: %w", command, err)
}

func resultOutput(result *executor.Result) string {
	if result == nil {
		return ""
	}
	return strings.TrimSpace(string(result.Combined))
}

type publicGitHubAuthenticationError struct {
	repositoryName string
	details        string
	cause          error
}

func (e *publicGitHubAuthenticationError) Error() string {
	return fmt.Sprintf(
		"GitHub unexpectedly requested authentication for the public %s repository.\n\n"+
			"The public %s repository does not require credentials. sb stopped instead of prompting for a username. "+
			"A retry using HTTP/1.1 also failed. This is likely a temporary GitHub-specific problem; try again in a few minutes.\n\n"+
			"Git details:\n%s",
		e.repositoryName,
		e.repositoryName,
		e.details,
	)
}

func (e *publicGitHubAuthenticationError) Unwrap() error {
	return e.cause
}
