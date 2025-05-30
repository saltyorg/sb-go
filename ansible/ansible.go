package ansible

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/saltyorg/sb-go/cache"
	"github.com/saltyorg/sb-go/git"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// RunAnsiblePlaybook executes an Ansible playbook using the specified binary and arguments.
// It constructs the command based on the provided playbook path, extra arguments, and repository directory.
// If verbose is true, the command output is streamed directly to the console; otherwise, output is captured for error reporting.
// On error, it returns a detailed error message including the exit code and, if available, the captured stderr.
func RunAnsiblePlaybook(repoPath, playbookPath, ansibleBinaryPath string, extraArgs []string, verbose bool) error {
	command := []string{ansibleBinaryPath, playbookPath, "--become"}
	command = append(command, extraArgs...)

	if verbose {
		fmt.Println("Executing Ansible playbook with command:", strings.Join(command, " "))
	}

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = repoPath

	var stdoutBuf, stderrBuf bytes.Buffer

	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stdin = os.Stdin
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
				return fmt.Errorf("\nError: Playbook %s run failed, scroll up to the failed task to review.\nExit code: %d\nStderr:\n%s", playbookPath, exitErr.ExitCode(), stderrBuf.String())
			}
			return fmt.Errorf("\nError: Playbook %s run failed, scroll up to the failed task to review.\nExit code: %d", playbookPath, exitErr.ExitCode())
		}
		if !verbose {
			return fmt.Errorf("\nError: Playbook %s run failed: %w\nStderr:\n%s", playbookPath, err, stderrBuf.String())
		}
		return fmt.Errorf("\nError: Playbook %s run failed: %w", playbookPath, err)
	}

	if verbose {
		fmt.Printf("\nPlaybook %s executed successfully.\n", playbookPath)
	}

	return nil
}

// PrepareAnsibleListTags configures the command for listing tags from an Ansible playbook,
// and returns a parser function to extract the tags from the command output.
// It builds the command using repoPath, playbookPath, and extraSkipTags. Additionally, if a cache is provided,
// the function checks whether cached tags can be used by comparing the repository's commit hash.
// If the repoPath corresponds to a specific known path (i.e. saltbox_mod), a fixed command configuration is used.
// The function returns an exec.Cmd (or nil if cached tags are available), a function to parse the command output,
// and an error if any configuration or cache retrieval fails.
func PrepareAnsibleListTags(repoPath, playbookPath, extraSkipTags string, cache *cache.Cache) (*exec.Cmd, func(string) ([]string, error), error) {
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

	// If repoPath matches the specific saltbox_mod repository, use a predetermined command configuration.
	if repoPath == "/opt/saltbox_mod" { // Assuming this is the saltbox_mod repo path
		cmd := exec.Command("/usr/local/bin/ansible-playbook", playbookPath, "--become", "--list-tags", fmt.Sprintf("--skip-tags=always,%s", extraSkipTags))
		cmd.Dir = repoPath
		return cmd, parseOutput, nil
	}

	// Check if cache is available and valid by comparing the stored commit hash with the current one.
	repoCache, ok := cache.GetRepoCache(repoPath)
	if ok {
		if commit, commitOK := repoCache["commit"].(string); commitOK {
			currentCommit, err := git.GetGitCommitHash(repoPath)
			if err != nil {
				return nil, nil, err
			}

			if commit == currentCommit {
				// Cached tags are valid; create a parser function that returns them directly.
				cachedTags, tagsOk := repoCache["tags"].([]interface{})
				if tagsOk {
					stringTags := make([]string, len(cachedTags))
					for i, tag := range cachedTags {
						if strTag, ok := tag.(string); ok {
							stringTags[i] = strTag
						} else {
							return nil, nil, fmt.Errorf("cached tags are not strings")
						}
					}
					return nil, func(string) ([]string, error) { return stringTags, nil }, nil
				}
			}
		}
	}

	// No valid cache found; build the command to list tags.
	cmd := exec.Command("/usr/local/bin/ansible-playbook", playbookPath, "--become", "--list-tags", fmt.Sprintf("--skip-tags=always,%s", extraSkipTags))
	cmd.Dir = repoPath
	return cmd, parseOutput, nil
}

// RunAndCacheAnsibleTags runs the ansible-playbook command to list available tags,
// parses the output, and updates the cache with the results.
// The function first attempts to use cached tags (if the current commit hash matches the cache).
// If cached tags are used, it updates the cache (to ensure consistency) and returns false.
// If a fresh command is executed, it caches the new tags along with the current commit hash and returns true.
// The boolean return value indicates whether the cache was rebuilt (true) or if cached tags were used (false).
func RunAndCacheAnsibleTags(repoPath, playbookPath, extraSkipTags string, cache *cache.Cache) (bool, error) {
	cmd, tagParser, err := PrepareAnsibleListTags(repoPath, playbookPath, extraSkipTags, cache)
	if err != nil {
		return false, err
	}

	if cmd == nil && tagParser != nil {
		// Cached tags are available; retrieve and update the cache.
		tags, err := tagParser("")
		if err != nil {
			return false, err
		}
		currentCommit, err := git.GetGitCommitHash(repoPath)
		if err != nil {
			return false, err
		}

		repoCache := map[string]interface{}{
			"commit": currentCommit,
			"tags":   tags,
		}
		cache.SetRepoCache(repoPath, repoCache)
		return false, nil // Cache was used, not rebuilt
	}

	if cmd != nil {
		var out bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			return true, fmt.Errorf("ansible-playbook failed: %s, stderr: %s", err, stderr.String())
		}

		tags, err := tagParser(out.String())
		if err != nil {
			return true, err
		}

		currentCommit, err := git.GetGitCommitHash(repoPath)
		if err != nil {
			return true, err
		}

		repoCache := map[string]interface{}{
			"commit": currentCommit,
			"tags":   tags,
		}
		cache.SetRepoCache(repoPath, repoCache)

		return true, nil // Cache was rebuilt with new tag information
	}

	return false, nil // Should not reach here, but return false by default
}

// RunAnsibleListTags executes the ansible-playbook command to list tags for the specified playbook,
// then parses and returns the list of tags.
// This function does not support using cached tags; it always runs a fresh command.
// An error is returned if command execution or output parsing fails.
func RunAnsibleListTags(repoPath, playbookPath, extraSkipTags string, cache *cache.Cache) ([]string, error) {
	cmd, tagParser, err := PrepareAnsibleListTags(repoPath, playbookPath, extraSkipTags, cache)
	if err != nil {
		return nil, err
	}

	if cmd == nil {
		return nil, fmt.Errorf("RunAnsibleListTags should not use cache")
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("ansible-playbook failed: %s, stderr: %s", err, stderr.String())
	}

	tags, err := tagParser(out.String())
	if err != nil {
		return nil, err
	}

	return tags, nil
}
