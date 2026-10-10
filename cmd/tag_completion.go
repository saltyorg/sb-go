package cmd

import (
	"slices"
	"strings"
	"unicode"

	"github.com/saltyorg/sb-go/ansible"
	"github.com/saltyorg/sb-go/layout"
	"github.com/spf13/cobra"
)

// Completion reads the last published cache. Discovery and cache refresh belong
// to normal install, list, and update operations, so pressing Tab runs no playbook.
func loadCompletionTags() ([]string, error) {
	return loadCompletionTagsFromFile(layout.SaltboxCacheFile)
}

func loadCompletionTagsFromFile(path string) ([]string, error) {
	cache, err := ansible.NewCacheWithFile(path)
	if err != nil {
		return nil, err
	}
	return getCompletionTags(cache), nil
}

func completeInstallTags(tags, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	const directive = cobra.ShellCompDirectiveNoFileComp
	if strings.ContainsFunc(toComplete, unicode.IsControl) {
		return nil, directive
	}
	// Fish can pass an unparsed opening quote. Returning it in a candidate
	// inserts a literal quote into the argument, so leave this token unchanged.
	if strings.HasPrefix(toComplete, "'") || strings.HasPrefix(toComplete, "\"") {
		return nil, directive
	}

	selected := make(map[string]bool)
	for _, arg := range args {
		for tag := range strings.SplitSeq(arg, ",") {
			selected[strings.TrimSpace(tag)] = true
		}
	}

	prefix, suffix := "", toComplete
	if comma := strings.LastIndexByte(toComplete, ','); comma >= 0 {
		prefix, suffix = toComplete[:comma+1], toComplete[comma+1:]
		for tag := range strings.SplitSeq(toComplete[:comma], ",") {
			selected[strings.TrimSpace(tag)] = true
		}
	}
	// The install command trims tag edges. Keep typed whitespace in the token
	// while matching the remaining tag, including quoted comma-separated input.
	trimmedSuffix := strings.TrimLeftFunc(suffix, unicode.IsSpace)
	prefix += suffix[:len(suffix)-len(trimmedSuffix)]
	suffix = trimmedSuffix

	var suggestions []string
	for _, rawTag := range tags {
		tag := completionTag(rawTag)
		if tag == "" {
			continue
		}
		if selected[tag] || !strings.HasPrefix(tag, suffix) {
			continue
		}
		selected[tag] = true
		suggestions = append(suggestions, prefix+tag)
	}
	slices.Sort(suggestions)
	return suggestions, directive
}

func completionTag(rawTag string) string {
	if strings.ContainsRune(rawTag, ',') || strings.ContainsFunc(rawTag, unicode.IsControl) {
		return ""
	}
	return strings.TrimSpace(rawTag)
}

// getCompletionTags preserves the command's repository routing. Mod tags are
// intentionally not offered, as they were not part of its cached completion set.
func getCompletionTags(cache *ansible.Cache) []string {
	var tags []string
	if repo, ok := cache.GetRepoCache(layout.SaltboxRepoPath); ok {
		for _, tag := range cachedTagStrings(repo["tags"]) {
			if tag = completionTag(tag); tag != "" {
				tags = append(tags, tag)
			}
		}
	}
	if repo, ok := cache.GetRepoCache(layout.Current().SandboxRepoPath); ok {
		for _, tag := range cachedTagStrings(repo["tags"]) {
			if tag = completionTag(tag); tag != "" {
				tags = append(tags, "sandbox-"+tag)
			}
		}
	}
	return tags
}

func cachedTagStrings(value any) []string {
	switch tags := value.(type) {
	case []string:
		return slices.Clone(tags)
	case []any:
		result := make([]string, 0, len(tags))
		for _, tag := range tags {
			if tag, ok := tag.(string); ok {
				result = append(result, tag)
			}
		}
		return result
	default:
		return nil
	}
}
