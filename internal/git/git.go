package git

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/saltyorg/sb-go/internal/spinners"
)

// CloneRepository clones a Git repository to a specified path and branch.
// The verbose flag controls whether stdout and stderr are directly outputted.
func CloneRepository(repoURL, destPath, branch string, verbose bool) error {
	if _, err := os.Stat(destPath); !os.IsNotExist(err) {
		return fmt.Errorf("destination path '%s' already exists", destPath)
	}

	cloneArgs := []string{"clone", "--depth", "1", "-b", branch, repoURL, destPath}
	cmd := exec.Command("git", cloneArgs...)

	var stdoutBuf, stderrBuf bytes.Buffer

	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
	}

	err := cmd.Run()

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if !verbose {
				return fmt.Errorf("failed to clone repository '%s' (branch: '%s') to '%s'.\nExit code: %d\nStderr:\n%s",
					repoURL, branch, destPath, exitErr.ExitCode(), stderrBuf.String())
			}
			return fmt.Errorf("failed to clone repository '%s' (branch: '%s') to '%s'.\nExit code: %d",
				repoURL, branch, destPath, exitErr.ExitCode())
		}
		if !verbose {
			return fmt.Errorf("failed to clone repository '%s' (branch: '%s') to '%s': %w\nStderr:\n%s",
				repoURL, branch, destPath, err, stderrBuf.String())
		}
		return fmt.Errorf("failed to clone repository '%s' (branch: '%s') to '%s': %w",
			repoURL, branch, destPath, err)
	}

	if verbose {
		fmt.Printf("Repository '%s' (branch: '%s') cloned successfully to '%s'\n", repoURL, branch, destPath)
	}

	return nil
}

// FetchAndReset performs a git fetch and reset to a specified branch.
func FetchAndReset(repoPath, defaultBranch, user string, customCommands [][]string) error {
	// Get the current branch name
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error: failed to get current branch: %s\n", string(output))
		return fmt.Errorf("failed to get current branch: %w", err)
	}
	currentBranch := strings.TrimSpace(string(output))

	var branch string
	// Determine if a reset to default_branch is needed
	if currentBranch != defaultBranch {
		if err := spinners.RunInfoSpinner(fmt.Sprintf("Currently on branch '%s'", currentBranch)); err != nil {
			return err
		}
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("Do you want to reset to the '%s' branch? (y/n): ", defaultBranch)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		if input != "y" {
			if err := spinners.RunInfoSpinner(fmt.Sprintf("Updating the current branch '%s'", currentBranch)); err != nil {
				return err
			}
			branch = currentBranch
		} else {
			branch = defaultBranch
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
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Dir = repoPath
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error: failed to execute command %v: %s\n", command, string(output))
			return fmt.Errorf("failed to execute command %v: %w", command, err)
		}
	}

	// Custom commands
	if customCommands != nil {
		for _, command := range customCommands {
			cmd := exec.Command(command[0], command[1:]...)
			cmd.Dir = repoPath
			output, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf("Error: failed to execute custom command %v: %s\n", command, string(output))
				return fmt.Errorf("failed to execute custom command %v: %w", command, err)
			}
		}
	}

	if err := spinners.RunInfoSpinner(fmt.Sprintf("Repository at %s (%s) has been updated", repoPath, branch)); err != nil {
		return err
	}
	return nil
}

// GetGitCommitHash returns the current Git commit hash of the repository.
func GetGitCommitHash(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()

	if err != nil {
		if _, statErr := os.Stat(repoPath); statErr != nil {
			return "", fmt.Errorf("the folder '%s' does not exist. This indicates an incomplete install", repoPath)
		}

		return "", fmt.Errorf("error occurred while trying to get the git commit hash: %s", stderr.String())
	}

	return strings.TrimSpace(out.String()), nil
}
