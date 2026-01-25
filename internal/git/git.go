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

	cloneArgs := []string{"clone", "--depth", "1", "-b", branch, repoURL, destPath}

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

// FetchAndReset performs a git fetch and reset to a specified branch.
// The context parameter allows for cancellation of git operations.
// The repoName parameter is used to identify the repository in user prompts.
func FetchAndReset(ctx context.Context, repoPath, defaultBranch, user string, customCommands [][]string, branchReset *bool, repoName string) error {
	// Get the current branch name
	result, err := executor.Run(ctx, "git",
		executor.WithArgs("rev-parse", "--abbrev-ref", "HEAD"),
		executor.WithWorkingDir(repoPath))
	if err != nil {
		fmt.Printf("Error: failed to get current branch: %s\n", string(result.Combined))
		return fmt.Errorf("failed to get current branch: %w", err)
	}
	currentBranch := strings.TrimSpace(string(result.Combined))

	var branch string
	// Determine if a reset to default_branch is needed
	if currentBranch != defaultBranch {
		if err := spinners.RunInfoSpinner(fmt.Sprintf("%s: Currently on branch '%s'", repoName, currentBranch)); err != nil {
			return err
		}

		if branchReset == nil {
			// No flag specified - prompt user if TTY, otherwise stay on current branch
			if !tty.IsInteractive() {
				// No TTY: default to keeping current branch (conservative approach)
				if err := spinners.RunInfoSpinner(fmt.Sprintf("%s: Updating the current branch '%s' (no TTY detected)", repoName, currentBranch)); err != nil {
					return err
				}
				branch = currentBranch
			} else {
				// TTY available: prompt user
				reader := bufio.NewReader(os.Stdin)
				fmt.Printf("%s: Do you want to reset to the '%s' branch? (y/n): ", repoName, defaultBranch)
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(strings.ToLower(input))

				if input != "y" {
					if err := spinners.RunInfoSpinner(fmt.Sprintf("%s: Updating the current branch '%s'", repoName, currentBranch)); err != nil {
						return err
					}
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
			if err := spinners.RunInfoSpinner(fmt.Sprintf("%s: Updating the current branch '%s'", repoName, currentBranch)); err != nil {
				return err
			}
			branch = currentBranch
		}
	} else {
		branch = defaultBranch
	}

	// Commands to fetch and reset
	commands := [][]string{
		{"git", "fetch", "--quiet"},
		{"git", "clean", "--quiet", "-df"},
		{"git", "reset", "--quiet", "--hard", "@{u}"},
		{"git", "checkout", "--quiet", branch},
		{"git", "clean", "--quiet", "-df"},
		{"git", "reset", "--quiet", "--hard", "@{u}"},
		{"git", "submodule", "update", "--init", "--recursive"},
		{"chown", "-R", fmt.Sprintf("%s:%s", user, user), repoPath},
	}

	for _, command := range commands {
		result, err := executor.Run(ctx, command[0],
			executor.WithArgs(command[1:]...),
			executor.WithWorkingDir(repoPath))
		if err != nil {
			fmt.Printf("Error: failed to execute command %v: %s\n", command, string(result.Combined))
			return fmt.Errorf("failed to execute command %v: %w", command, err)
		}
	}

	// Custom commands
	for _, command := range customCommands {
		result, err := executor.Run(ctx, command[0],
			executor.WithArgs(command[1:]...),
			executor.WithWorkingDir(repoPath))
		if err != nil {
			fmt.Printf("Error: failed to execute custom command %v: %s\n", command, string(result.Combined))
			return fmt.Errorf("failed to execute custom command %v: %w", command, err)
		}
	}

	if err := spinners.RunInfoSpinner(fmt.Sprintf("%s: Repository at %s (%s) has been updated", repoName, repoPath, branch)); err != nil {
		return err
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
