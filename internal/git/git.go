package git

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/saltyorg/sb-go/internal/executor"
	"github.com/saltyorg/sb-go/internal/spinners"
	"github.com/saltyorg/sb-go/internal/tty"
)

// CloneRepository clones a Git repository to a specified path and branch.
// The verbose flag controls whether stdout and stderr are directly outputted.
// The context parameter allows for cancellation of the clone operation.
func CloneRepository(ctx context.Context, repoURL, destPath, branch string, verbose bool) error {
	if _, err := os.Stat(destPath); !os.IsNotExist(err) {
		return fmt.Errorf("destination path '%s' already exists", destPath)
	}

	cloneArgs := []string{"clone", "--progress", "--depth", "1", "-b", branch, repoURL, destPath}

	// Use executor to handle verbose/non-verbose output
	var mode executor.OutputMode
	if verbose {
		mode = executor.OutputModeStream
	} else {
		mode = executor.OutputModeCapture
	}

	result, err := executor.Run(ctx, "git",
		executor.WithArgs(cloneArgs...),
		executor.WithOutputMode(mode))

	if err != nil {
		if !verbose && len(result.Stderr) > 0 {
			return fmt.Errorf("failed to clone repository '%s' (branch: '%s') to '%s' (exit code %d)\nStderr:\n%s",
				repoURL, branch, destPath, result.ExitCode, string(result.Stderr))
		}
		return fmt.Errorf("failed to clone repository '%s' (branch: '%s') to '%s' (exit code %d): %w",
			repoURL, branch, destPath, result.ExitCode, err)
	}

	if verbose {
		fmt.Printf("Repository '%s' (branch: '%s') cloned successfully to '%s'\n", repoURL, branch, destPath)
	}

	return nil
}

// EnsureRemoteFetchAllBranches makes sure remote.origin.fetch includes all branches.
func EnsureRemoteFetchAllBranches(ctx context.Context, repoPath string) error {
	const fetchSpec = "+refs/heads/*:refs/remotes/origin/*"

	result, err := executor.Run(ctx, "git",
		executor.WithArgs("config", "--get-all", "remote.origin.fetch"),
		executor.WithWorkingDir(repoPath))
	fetchConfig := strings.TrimSpace(string(result.Combined))
	if err != nil && fetchConfig != "" {
		fmt.Printf("Error: failed to read remote.origin.fetch: %s\n", string(result.Combined))
		return fmt.Errorf("failed to read remote.origin.fetch: %w", err)
	}

	if fetchConfig != "" {
		for line := range strings.SplitSeq(fetchConfig, "\n") {
			if strings.Contains(strings.TrimSpace(line), "refs/heads/*:refs/remotes/origin/*") {
				return nil
			}
		}
	}

	result, err = executor.Run(ctx, "git",
		executor.WithArgs("config", "--add", "remote.origin.fetch", fetchSpec),
		executor.WithWorkingDir(repoPath))
	if err != nil {
		fmt.Printf("Error: failed to update remote.origin.fetch: %s\n", string(result.Combined))
		return fmt.Errorf("failed to update remote.origin.fetch: %w", err)
	}

	return nil
}

// ResolveUpdateBranch determines the branch to update and performs any required
// interactive prompt before a terminal renderer is started.
func ResolveUpdateBranch(
	ctx context.Context,
	runner *spinners.Runner,
	repoPath, defaultBranch string,
	branchReset *bool,
	repoName string,
) (string, error) {
	// Get the current branch name
	result, err := executor.Run(ctx, "git",
		executor.WithArgs("rev-parse", "--abbrev-ref", "HEAD"),
		executor.WithWorkingDir(repoPath))
	if err != nil {
		fmt.Printf("Error: failed to get current branch: %s\n", string(result.Combined))
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	currentBranch := strings.TrimSpace(string(result.Combined))

	var branch string
	// Determine if a reset to default_branch is needed
	if currentBranch != defaultBranch {
		runner.Info(fmt.Sprintf("%s: Currently on branch '%s'", repoName, currentBranch))

		if branchReset == nil {
			// No flag specified - prompt user if TTY, otherwise stay on current branch
			if !tty.IsInteractive() {
				// No TTY: default to keeping current branch (conservative approach)
				runner.Info(fmt.Sprintf("%s: Updating the current branch '%s' (no TTY detected)", repoName, currentBranch))
				branch = currentBranch
			} else {
				// TTY available: prompt user
				reader := bufio.NewReader(os.Stdin)
				fmt.Printf("%s: Do you want to reset to the '%s' branch? (y/n): ", repoName, defaultBranch)
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(strings.ToLower(input))

				if input != "y" {
					runner.Info(fmt.Sprintf("%s: Updating the current branch '%s'", repoName, currentBranch))
					branch = currentBranch
				} else {
					branch = defaultBranch
				}
			}
		} else if *branchReset {
			// --reset-branch flag: reset to default branch
			branch = defaultBranch
		} else {
			// --keep-branch flag: stay on current branch
			runner.Info(fmt.Sprintf("%s: Updating the current branch '%s'", repoName, currentBranch))
			branch = currentBranch
		}
	} else {
		branch = defaultBranch
	}
	return branch, nil
}

// FetchAndResetBranch updates a repository after branch selection has already
// been resolved.
func FetchAndResetBranch(
	ctx context.Context,
	parent *spinners.Task,
	repoPath, branch, user string,
	customCommands [][]string,
	repoName string,
) error {
	fetchCommands := [][]string{
		{"git", "fetch", "--progress"},
	}
	resetCommands := [][]string{
		{"git", "clean", "--quiet", "-df"},
		{"git", "reset", "--quiet", "--hard", "@{u}"},
		{"git", "checkout", "--quiet", branch},
		{"git", "clean", "--quiet", "-df"},
		{"git", "reset", "--quiet", "--hard", "@{u}"},
	}
	submoduleCommands := [][]string{
		{"git", "submodule", "update", "--progress", "--init", "--recursive"},
	}
	ownershipCommands := [][]string{
		{"chown", "-R", fmt.Sprintf("%s:%s", user, user), repoPath},
	}

	runCommands := func(commandCtx context.Context, commands [][]string) error {
		for _, command := range commands {
			result, err := executor.Run(commandCtx, command[0],
				executor.WithArgs(command[1:]...),
				executor.WithWorkingDir(repoPath))
			if err != nil {
				return fmt.Errorf("failed to execute command %v: %w\n%s", command, err, string(result.Combined))
			}
		}
		return nil
	}

	steps := []struct {
		name     string
		commands [][]string
	}{
		{name: "Fetching repository changes", commands: fetchCommands},
		{name: fmt.Sprintf("Resetting repository to %s", branch), commands: resetCommands},
		{name: "Updating git submodules", commands: submoduleCommands},
		{name: "Setting repository ownership", commands: ownershipCommands},
	}
	for _, step := range steps {
		if err := parent.RunStreaming(ctx, spinners.TaskSpec{Running: step.name}, func(taskCtx context.Context) error {
			return runCommands(taskCtx, step.commands)
		}); err != nil {
			return err
		}
	}

	if len(customCommands) > 0 {
		if err := parent.RunStreaming(ctx, spinners.TaskSpec{Running: "Running repository update hooks"}, func(taskCtx context.Context) error {
			return runCommands(taskCtx, customCommands)
		}); err != nil {
			return err
		}
	}
	return nil
}

// GetGitCommitHash returns the current Git commit hash of the repository.
func GetGitCommitHash(ctx context.Context, repoPath string) (string, error) {
	output, err := defaultExecutor.ExecuteCommand(ctx, repoPath, "git", BuildRevParseArgs()...)

	if err != nil {
		if _, statErr := os.Stat(repoPath); statErr != nil {
			return "", fmt.Errorf("the folder '%s' does not exist. This indicates an incomplete install", repoPath)
		}

		return "", fmt.Errorf("error occurred while trying to get the git commit hash: %s", string(output))
	}

	return ParseCommitHash(output), nil
}
