package ansible

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/saltyorg/sb-go/internal/cache"
	"github.com/saltyorg/sb-go/internal/constants"
	sbErrors "github.com/saltyorg/sb-go/internal/errors"
	"github.com/saltyorg/sb-go/internal/git"
)

// TagParser is a function type that parses output and extracts tags
type TagParser func(string) ([]string, error)

// parseTagsFromOutput extracts tags from ansible-playbook output
func parseTagsFromOutput(output string) ([]string, error) {
	re := regexp.MustCompile(`TASK TAGS:\s*\[(.*?)]`)
	match := re.FindStringSubmatch(output)
	if len(match) < 2 {
		return nil, fmt.Errorf("error: 'TASK TAGS:' not found in the ansible-playbook output")
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

// buildAnsibleCommand constructs the ansible-playbook command arguments
func buildAnsibleCommand(playbookPath string, extraArgs []string) []string {
	command := []string{playbookPath, "--become"}
	command = append(command, extraArgs...)
	return command
}

// buildListTagsCommand constructs the command for listing tags
func buildListTagsCommand(playbookPath, extraSkipTags string) []string {
	return []string{
		playbookPath,
		"--become",
		"--list-tags",
		fmt.Sprintf("--skip-tags=always,%s", extraSkipTags),
	}
}

// checkCachedTags checks if valid cached tags exist for the given repository
func checkCachedTags(ctx context.Context, repoPath string, cache *cache.Cache) ([]string, bool) {
	repoCache, ok := cache.GetRepoCache(repoPath)
	if !ok {
		return nil, false
	}

	commitIface, commitOK := repoCache["commit"]
	if !commitOK {
		return nil, false
	}

	commit, ok := commitIface.(string)
	if !ok {
		return nil, false
	}

	currentCommit, err := git.GetGitCommitHash(ctx, repoPath)
	if err != nil {
		return nil, false
	}

	if commit != currentCommit {
		return nil, false
	}

	cachedTags, tagsOk := repoCache["tags"].([]any)
	if !tagsOk {
		return nil, false
	}

	stringTags := make([]string, len(cachedTags))
	for i, tag := range cachedTags {
		if strTag, ok := tag.(string); ok {
			stringTags[i] = strTag
		} else {
			return nil, false
		}
	}

	return stringTags, true
}

// cacheTagsWithCommit caches the tags along with the current commit hash
func cacheTagsWithCommit(ctx context.Context, repoPath string, tags []string, cache *cache.Cache) error {
	currentCommit, err := git.GetGitCommitHash(ctx, repoPath)
	if err != nil {
		return err
	}

	repoCache := map[string]any{
		"commit": currentCommit,
		"tags":   tags,
	}
	cache.SetRepoCache(repoPath, repoCache)

	return nil
}

// formatPlaybookError formats an error message for playbook execution failures
func formatPlaybookError(playbookPath string, err error, stderrBuf *bytes.Buffer, verbose bool) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// Check if the exit code indicates the process was killed by a signal
		if exitErr.ExitCode() < 0 {
			if sbErrors.HandleInterruptError(err) {
				return fmt.Errorf("playbook execution interrupted by user")
			}
		}
		if !verbose && stderrBuf != nil {
			return fmt.Errorf("Playbook %s run failed, scroll up to the failed task to review.\nExit code: %d\nStderr:\n%s",
				playbookPath, exitErr.ExitCode(), stderrBuf.String())
		}
		return fmt.Errorf("Playbook %s run failed, scroll up to the failed task to review.\nExit code: %d",
			playbookPath, exitErr.ExitCode())
	}

	if !verbose && stderrBuf != nil {
		return fmt.Errorf("Playbook %s run failed: %w\nStderr:\n%s", playbookPath, err, stderrBuf.String())
	}
	return fmt.Errorf("Playbook %s run failed: %w", playbookPath, err)
}

// isSaltboxModRepo checks if the repository path is the SaltboxMod repository
func isSaltboxModRepo(repoPath string) bool {
	return repoPath == constants.SaltboxModRepoPath
}

// createTagParserFunc creates a parser function that returns cached tags
func createTagParserFunc(tags []string) TagParser {
	return func(string) ([]string, error) {
		return tags, nil
	}
}
