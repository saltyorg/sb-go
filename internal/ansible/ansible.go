package ansible

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/saltyorg/sb-go/internal/cache"
	"github.com/saltyorg/sb-go/internal/constants"
	sbErrors "github.com/saltyorg/sb-go/internal/errors"
	"github.com/saltyorg/sb-go/internal/executor"
	"github.com/saltyorg/sb-go/internal/git"
	"github.com/saltyorg/sb-go/internal/logging"
)

// RunAnsiblePlaybook executes an Ansible playbook using the specified binary and arguments.
// It constructs the command based on the provided playbook path, extra arguments, and repository directory.
// If verbose is true, the command output is streamed directly to the console; otherwise, output is captured for error reporting.
// On error, it returns a detailed error message including the exit code and, if available, the captured stderr.
// The function uses the provided context for cancellation support, allowing graceful interruption via signals.
func RunAnsiblePlaybook(ctx context.Context, repoPath, playbookPath, ansibleBinaryPath string, extraArgs []string, verbose bool) error {
	command := []string{ansibleBinaryPath, playbookPath, "--become"}
	command = append(command, extraArgs...)

	if verbose {
		fmt.Println("Executing Ansible playbook with command:", strings.Join(command, " "))
	}

	// Use the appropriate output mode based on verbosity
	var outputMode executor.OutputMode
	if verbose {
		outputMode = executor.OutputModeInteractive
	} else {
		outputMode = executor.OutputModeCapture
	}

	result, err := executor.Run(ctx, command[0],
		executor.WithArgs(command[1:]...),
		executor.WithWorkingDir(repoPath),
		executor.WithOutputMode(outputMode),
		executor.WithInheritEnv())

	if err != nil {
		// Check if the error is due to context cancellation (signal interruption)
		if sbErrors.HandleInterruptError(err) {
			return fmt.Errorf("playbook execution interrupted by user")
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() < 0 {
				if sbErrors.HandleInterruptError(err) {
					return fmt.Errorf("playbook execution interrupted by user")
				}
			}
			if !verbose && len(result.Stderr) > 0 {
				return fmt.Errorf("playbook %s run failed, scroll up to the failed task to review.\nExit code: %d\nStderr:\n%s", playbookPath, exitErr.ExitCode(), string(result.Stderr))
			}
			return fmt.Errorf("playbook %s run failed, scroll up to the failed task to review.\nExit code: %d", playbookPath, exitErr.ExitCode())
		}
		if !verbose && len(result.Stderr) > 0 {
			return fmt.Errorf("playbook %s run failed: %w\nStderr:\n%s", playbookPath, err, string(result.Stderr))
		}
		return fmt.Errorf("playbook %s run failed: %w", playbookPath, err)
	}

	if verbose {
		fmt.Printf("\nPlaybook %s executed successfully.\n", playbookPath)
	}

	return nil
}

// PrepareAnsibleListTags configures the command for listing tags from an Ansible playbook
// and returns a parser function to extract the tags from the command output.
// It builds the command using repoPath, playbookPath, and extraSkipTags. Additionally, if a cache is provided,
// the function checks whether cached tags can be used by comparing the repository's commit hash.
// If the repoPath corresponds to a specific known path (i.e., saltbox_mod), a fixed command configuration is used.
// The function returns command args (or nil if cached tags are available), a function to parse the command output,
// the commit hash (if cache was valid, empty string otherwise), and an error if any configuration or cache retrieval fails.
// The context parameter allows for cancellation of the command execution.
// The verbosity parameter controls debug output (0 = no debug, >0 = debug).
func PrepareAnsibleListTags(ctx context.Context, repoPath, playbookPath, extraSkipTags string, cache *cache.Cache, verbosity int) ([]string, func(string) ([]string, error), string, error) {
	// parseOutput extracts tags from the ansible-playbook output using a regular expression.
	parseOutput := func(output string) ([]string, error) {
		re := regexp.MustCompile(`TASK TAGS:\s*\[(.*?)]`)
		match := re.FindStringSubmatch(output)
		if len(match) < 2 {
			return nil, fmt.Errorf("error: 'TASK TAGS:' not found in the ansible-playbook output. Please make sure '%s' is formatted correctly", playbookPath)
		}

		taskTags := strings.TrimSpace(match[1])
		if taskTags == "" {
			return []string{}, nil
		}

		tags := strings.Split(taskTags, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}

		return tags, nil
	}

	// Check if the cache is available and valid by comparing the stored commit hash with the current one.
	repoCache, ok := cache.GetRepoCache(repoPath)
	if ok {
		logging.Debug(verbosity, "PrepareAnsibleListTags: Cache found for %s", repoPath)
		if commit, commitOK := repoCache["commit"].(string); commitOK {
			currentCommit, err := git.GetGitCommitHash(ctx, repoPath)
			if err != nil {
				return nil, nil, "", err
			}

			logging.Debug(verbosity, "PrepareAnsibleListTags: Cached commit: %s, Current commit: %s", commit, currentCommit)

			if commit == currentCommit {
				// Cached tags are valid; create a parser function that returns them directly.
				cachedTags, tagsOk := repoCache["tags"].([]any)
				if tagsOk {
					logging.Debug(verbosity, "PrepareAnsibleListTags: Using cached tags (%d tags)", len(cachedTags))
					stringTags := make([]string, len(cachedTags))
					for i, tag := range cachedTags {
						if strTag, ok := tag.(string); ok {
							stringTags[i] = strTag
						} else {
							return nil, nil, "", fmt.Errorf("cached tags are not strings")
						}
					}
					return nil, func(string) ([]string, error) { return stringTags, nil }, currentCommit, nil
				} else {
					logging.Debug(verbosity, "PrepareAnsibleListTags: Cached tags not in expected format")
				}
			} else {
				logging.Debug(verbosity, "PrepareAnsibleListTags: Commit hash mismatch, will rebuild cache")
			}
		} else {
			logging.Debug(verbosity, "PrepareAnsibleListTags: No valid commit in cache")
		}
	} else {
		logging.Debug(verbosity, "PrepareAnsibleListTags: No cache found for %s", repoPath)
	}

	// No valid cache found; build the command args to list tags.
	args := []string{playbookPath, "--become", "--list-tags", fmt.Sprintf("--skip-tags=always,%s", extraSkipTags)}
	return args, parseOutput, "", nil
}

// RunAndCacheAnsibleTags runs the ansible-playbook command to list available tags,
// parses the output, and updates the cache with the results.
// The function first attempts to use cached tags (if the current commit hash matches the cache).
// If cached tags are used, it updates the cache (to ensure consistency) and returns false.
// If a fresh command is executed, it caches the new tags along with the current commit hash and returns true.
// The boolean return value indicates whether the cache was rebuilt (true) or if cached tags were used (false).
// The context parameter allows for cancellation of the command execution.
// The verbosity parameter controls debug output (0 = no debug, >0 = debug).
func RunAndCacheAnsibleTags(ctx context.Context, repoPath, playbookPath, extraSkipTags string, cache *cache.Cache, verbosity int) (bool, error) {
	args, tagParser, cachedCommit, err := PrepareAnsibleListTags(ctx, repoPath, playbookPath, extraSkipTags, cache, verbosity)
	if err != nil {
		return false, err
	}

	if args == nil && tagParser != nil {
		// Cached tags are available; retrieve and update the cache.
		// cachedCommit already contains the current commit hash from PrepareAnsibleListTags.
		logging.Debug(verbosity, "RunAndCacheAnsibleTags: Using cached tags for %s", repoPath)
		tags, err := tagParser("")
		if err != nil {
			return false, err
		}

		repoCache := map[string]any{
			"commit": cachedCommit,
			"tags":   tags,
		}
		if err := cache.SetRepoCache(repoPath, repoCache); err != nil {
			return false, fmt.Errorf("failed to save cache: %w", err)
		}
		logging.Debug(verbosity, "RunAndCacheAnsibleTags: Cache updated with %d tags", len(tags))
		return false, nil // Cache was used, not rebuilt
	}

	if args != nil {
		// Use the executor interface to run the command
		logging.Debug(verbosity, "RunAndCacheAnsibleTags: Running ansible-playbook for %s", repoPath)
		logging.Debug(verbosity, "RunAndCacheAnsibleTags: Command: %s %v", constants.AnsiblePlaybookBinaryPath, args)
		output, err := defaultExecutor.ExecuteContext(ctx, repoPath, constants.AnsiblePlaybookBinaryPath, args...)
		if err != nil {
			// Check if it's a user interrupt
			if sbErrors.HandleInterruptError(err) {
				return true, fmt.Errorf("command interrupted by user")
			}
			logging.Debug(verbosity, "RunAndCacheAnsibleTags: ansible-playbook failed with error: %v", err)
			logging.Debug(verbosity, "RunAndCacheAnsibleTags: Command output:\n%s", string(output))
			return true, fmt.Errorf("ansible-playbook failed: %w\nOutput: %s", err, string(output))
		}

		logging.Debug(verbosity, "RunAndCacheAnsibleTags: Raw ansible output length: %d bytes", len(output))
		logging.Trace(verbosity, "RunAndCacheAnsibleTags: Raw output:\n%s", string(output))

		tags, err := tagParser(string(output))
		if err != nil {
			logging.Debug(verbosity, "RunAndCacheAnsibleTags: Failed to parse tags: %v", err)
			return true, err
		}

		logging.Debug(verbosity, "RunAndCacheAnsibleTags: Parsed %d tags from ansible output", len(tags))

		currentCommit, err := git.GetGitCommitHash(ctx, repoPath)
		if err != nil {
			logging.Debug(verbosity, "RunAndCacheAnsibleTags: Failed to get git commit hash: %v", err)
			return true, err
		}

		logging.Debug(verbosity, "RunAndCacheAnsibleTags: Current commit hash: %s", currentCommit)

		repoCache := map[string]any{
			"commit": currentCommit,
			"tags":   tags,
		}
		if err := cache.SetRepoCache(repoPath, repoCache); err != nil {
			return true, fmt.Errorf("failed to save cache: %w", err)
		}

		logging.Debug(verbosity, "RunAndCacheAnsibleTags: Cache rebuilt with %d tags, commit: %s", len(tags), currentCommit)
		return true, nil // Cache was rebuilt with new tag information
	}

	return false, nil // Should not reach here but return false by default
}

// RunAnsibleListTags executes the ansible-playbook command to list tags for the specified playbook,
// then parses and returns the list of tags.
// This function does not support using cached tags; it always runs a fresh command.
// An error is returned if command execution or output parsing fails.
// The context parameter allows for cancellation of the command execution.
// The verbosity parameter controls debug output (0 = no debug, >0 = debug).
func RunAnsibleListTags(ctx context.Context, repoPath, playbookPath, extraSkipTags string, cache *cache.Cache, verbosity int) ([]string, error) {
	args, tagParser, _, err := PrepareAnsibleListTags(ctx, repoPath, playbookPath, extraSkipTags, cache, verbosity)
	if err != nil {
		return nil, err
	}

	if args == nil {
		return nil, fmt.Errorf("RunAnsibleListTags should not use cache")
	}

	// Use the executor interface to run the command
	logging.Debug(verbosity, "RunAnsibleListTags: Running ansible-playbook for %s", repoPath)
	logging.Debug(verbosity, "RunAnsibleListTags: Command: %s %v", constants.AnsiblePlaybookBinaryPath, args)
	output, err := defaultExecutor.ExecuteContext(ctx, repoPath, constants.AnsiblePlaybookBinaryPath, args...)
	if err != nil {
		// Check if it's a user interrupt
		if sbErrors.HandleInterruptError(err) {
			return nil, fmt.Errorf("command interrupted by user")
		}
		logging.Debug(verbosity, "RunAnsibleListTags: ansible-playbook failed with error: %v", err)
		logging.Debug(verbosity, "RunAnsibleListTags: Command output:\n%s", string(output))
		return nil, fmt.Errorf("ansible-playbook failed: %w\nOutput: %s", err, string(output))
	}

	logging.Debug(verbosity, "RunAnsibleListTags: Raw ansible output length: %d bytes", len(output))
	logging.Trace(verbosity, "RunAnsibleListTags: Raw output:\n%s", string(output))

	tags, err := tagParser(string(output))
	if err != nil {
		logging.Debug(verbosity, "RunAnsibleListTags: Failed to parse tags: %v", err)
		return nil, err
	}

	logging.Debug(verbosity, "RunAnsibleListTags: Parsed %d tags from ansible output", len(tags))

	return tags, nil
}
