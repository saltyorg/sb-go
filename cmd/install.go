package cmd

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/saltyorg/sb-go/internal/ansible"
	"github.com/saltyorg/sb-go/internal/cache"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/utils"

	"github.com/agnivade/levenshtein"
	"github.com/spf13/cobra"
)

// installCmd represents the install command
var installCmd = &cobra.Command{
	Use:   "install [tags]",
	Short: "Runs Ansible playbooks with specified tags",
	Long:  `Runs Ansible playbooks with specified tags`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if err := utils.CheckLXC(ctx); err != nil {
			return err
		}

		joined := strings.Join(args, ",")
		rawTags := strings.Split(joined, ",")

		var tags []string
		for _, t := range rawTags {
			tag := strings.TrimSpace(t)
			if tag != "" {
				tags = append(tags, tag)
			}
		}

		if len(tags) == 0 {
			return fmt.Errorf("no tags provided")
		}

		verbosity, _ := cmd.Flags().GetCount("verbose")
		skipTags, _ := cmd.Flags().GetStringSlice("skip-tags")
		extraVars, _ := cmd.Flags().GetStringArray("extra-vars")
		noCache, _ := cmd.Flags().GetBool("no-cache")

		var extraArgs []string
		if verbosity > 0 {
			vFlag := "-" + strings.Repeat("v", verbosity)
			extraArgs = append(extraArgs, vFlag)
		}

		return handleInstall(cmd, tags, extraVars, skipTags, extraArgs, verbosity, noCache)
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
	installCmd.Flags().StringArrayP("extra-vars", "e", []string{}, "Extra variables to pass to Ansible")
	installCmd.Flags().StringSliceP("skip-tags", "s", []string{}, "Tags to skip during Ansible playbook execution")
	installCmd.Flags().CountP("verbose", "v", "Increase verbosity level (can be used multiple times, e.g. -vvv)")
	installCmd.Flags().Bool("no-cache", false, "Skip cache validation and always perform tag checks")
}

func handleInstall(cmd *cobra.Command, tags []string, extraVars []string, skipTags []string, extraArgs []string, verbosity int, noCache bool) error {
	ctx := cmd.Context()
	var saltboxTags []string
	var sandboxTags []string
	var saltboxModTags []string

	cacheInstance, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("error creating cache: %w", err)
	}

	var parsedTags []string
	for _, arg := range tags {
		parsedTags = append(parsedTags, strings.TrimSpace(arg))
	}

	for _, tag := range parsedTags {
		if after, ok := strings.CutPrefix(tag, "mod-"); ok {
			saltboxModTags = append(saltboxModTags, after)
		} else if after, ok := strings.CutPrefix(tag, "sandbox-"); ok {
			sandboxTags = append(sandboxTags, after)
		} else {
			saltboxTags = append(saltboxTags, tag)
		}
	}

	needsCacheUpdate := false
	if !noCache {
		// Check if either cache is missing or has an empty 'tags' list
		saltboxCacheValid := cacheExistsAndIsValid(constants.SaltboxRepoPath, cacheInstance, verbosity)
		sandboxCacheValid := cacheExistsAndIsValid(constants.SandboxRepoPath, cacheInstance, verbosity)

		if !saltboxCacheValid || !sandboxCacheValid {
			needsCacheUpdate = true
		}

		if verbosity > 0 {
			fmt.Printf("DEBUG: needsCacheUpdate: %t\n", needsCacheUpdate)
		}

		if needsCacheUpdate {
			fmt.Println("INFO: Cache missing or incomplete, updating the cache")
		}
	} else if verbosity > 0 {
		fmt.Println("DEBUG: Cache validation skipped due to --no-cache flag")
	}

	if !noCache {
		var allSuggestions []string

		if len(saltboxTags) > 0 {
			allSuggestions = append(allSuggestions, validateAndSuggest(ctx, constants.SaltboxRepoPath, saltboxTags, "", "sandbox-", cacheInstance, verbosity)...)
		}

		if len(sandboxTags) > 0 {
			allSuggestions = append(allSuggestions, validateAndSuggest(ctx, constants.SandboxRepoPath, sandboxTags, "sandbox-", "", cacheInstance, verbosity)...)
		}

		if len(allSuggestions) > 0 {
			fmt.Println("----------------------------------------")
			fmt.Println("The following issues were found with the provided tags:")
			for i, suggestion := range allSuggestions {
				fmt.Printf("%d. %s\n", i+1, suggestion)
			}
			fmt.Println("----------------------------------------")
			return fmt.Errorf("invalid tags provided, see suggestions above")
		}
	}

	if verbosity > 0 {
		fmt.Println("DEBUG: No suggestions needed, continuing")
	}

	ansibleBinaryPath := constants.AnsiblePlaybookBinaryPath

	if len(saltboxTags) > 0 {
		if err := runPlaybook(ctx, constants.SaltboxRepoPath, constants.SaltboxPlaybookPath(), saltboxTags, ansibleBinaryPath, extraVars, skipTags, extraArgs); err != nil {
			return err
		}
	}

	if len(saltboxModTags) > 0 {
		if err := runPlaybook(ctx, constants.SaltboxModRepoPath, constants.SaltboxModPlaybookPath(), saltboxModTags, ansibleBinaryPath, extraVars, skipTags, extraArgs); err != nil {
			return err
		}
	}

	if len(sandboxTags) > 0 {
		if err := runPlaybook(ctx, constants.SandboxRepoPath, constants.SandboxPlaybookPath(), sandboxTags, ansibleBinaryPath, extraVars, skipTags, extraArgs); err != nil {
			return err
		}
	}

	if len(parsedTags) == 0 {
		return fmt.Errorf("no valid tags were provided for installation")
	}

	return nil
}

func runPlaybook(ctx context.Context, repoPath, playbookPath string, tags []string, ansibleBinaryPath string, extraVars []string, skipTags []string, extraArgs []string) error {
	tagsArg := strings.Join(tags, ",")
	allArgs := []string{"--tags", tagsArg}

	for _, extraVar := range extraVars {
		allArgs = append(allArgs, "--extra-vars", extraVar)
	}

	if len(skipTags) > 0 {
		allArgs = append(allArgs, "--skip-tags", strings.Join(skipTags, ","))
	}

	allArgs = append(allArgs, extraArgs...)

	err := ansible.RunAnsiblePlaybook(ctx, repoPath, playbookPath, ansibleBinaryPath, allArgs, true)
	if err != nil {
		handleInterruptError(err)
		return fmt.Errorf("error running playbook: %w", err)
	}
	return nil
}

func validateAndSuggest(ctx context.Context, repoPath string, providedTags []string, currentPrefix, otherPrefix string, cacheInstance *cache.Cache, verbosity int) []string {
	var suggestions []string

	// Ensure the cache exists and is populated.
	validTags := getValidTags(ctx, repoPath, cacheInstance, verbosity)

	if verbosity > 0 {
		fmt.Printf("DEBUG: Valid tags for %s (after getValidTags): %v\n", repoPath, validTags)
	}

	otherRepoPath := constants.SandboxRepoPath
	if repoPath == constants.SandboxRepoPath {
		otherRepoPath = constants.SaltboxRepoPath
	}
	otherValidTags := getValidTags(ctx, otherRepoPath, cacheInstance, verbosity)
	if verbosity > 0 {
		fmt.Printf("DEBUG: Valid tags for other repo %s (after getValidTags): %v\n", otherRepoPath, otherValidTags)
	}

	repoName := "Saltbox"
	otherRepoName := "Sandbox"
	if repoPath == constants.SandboxRepoPath {
		repoName = "Sandbox"
		otherRepoName = "Saltbox"
	}

	for _, providedTag := range providedTags {
		if verbosity > 0 {
			fmt.Printf("DEBUG: Checking tag: %s%s\n", currentPrefix, providedTag)
		}
		found := slices.Contains(validTags, providedTag)
		if found {
			if verbosity > 0 {
				fmt.Printf("DEBUG: Exact match found for %s%s\n", currentPrefix, providedTag)
			}
			continue // Tag is valid, no suggestion needed
		}

		// 2. Check for an exact match in the *other* repository (the strongest suggestion)
		if slices.Contains(otherValidTags, providedTag) {
			suggestions = append(suggestions, fmt.Sprintf("'%s%s' doesn't exist in %s, but '%s%s' exists in %s. Use that instead.", currentPrefix, providedTag, repoName, otherPrefix, providedTag, otherRepoName))
			if verbosity > 0 {
				fmt.Printf("DEBUG: Exact match found in other repo for %s%s, suggesting %s%s\n", currentPrefix, providedTag, otherPrefix, providedTag)
			}
			found = true
		}
		if found {
			continue
		}

		// 3. Find close matches (Levenshtein distance) within the *current* repository
		bestMatch := ""
		bestDistance := 9999 // Initialize with a large distance

		for _, validTag := range validTags {
			distance := levenshtein.ComputeDistance(providedTag, validTag)
			if verbosity > 0 {
				fmt.Printf("DEBUG: Distance between '%s' and '%s': %d\n", providedTag, validTag, distance)
			}
			if distance < bestDistance && distance <= 2 { // Threshold of 2
				bestDistance = distance
				bestMatch = validTag
			}
		}

		if bestMatch != "" {
			suggestions = append(suggestions, fmt.Sprintf("'%s%s' doesn't exist in %s. Did you mean '%s%s'?", currentPrefix, providedTag, repoName, currentPrefix, bestMatch))
			if verbosity > 0 {
				fmt.Printf("DEBUG: Suggesting '%s%s' for '%s%s'\n", currentPrefix, bestMatch, currentPrefix, providedTag)
			}
			continue
		}

		// 4. Find close matches in the *other* repository.
		bestMatchOther := ""
		bestDistanceOther := 9999
		for _, otherValidTag := range otherValidTags {
			distance := levenshtein.ComputeDistance(providedTag, otherValidTag)
			if verbosity > 0 {
				fmt.Printf("DEBUG: Distance between '%s' and '%s' in other repo: %d\n", providedTag, otherValidTag, distance)
			}
			if distance < bestDistanceOther && distance <= 2 {
				bestDistanceOther = distance
				bestMatchOther = otherValidTag
			}
		}
		if bestMatchOther != "" {
			suggestions = append(suggestions, fmt.Sprintf("'%s%s' doesn't exist in %s. Did you mean '%s%s' (from %s)?", currentPrefix, providedTag, repoName, otherPrefix, bestMatchOther, otherRepoName))
			if verbosity > 0 {
				fmt.Printf("DEBUG: Suggesting '%s%s' for '%s%s' from other repo\n", otherPrefix, bestMatchOther, currentPrefix, providedTag)
			}
			continue
		}

		// 5. No close match found, provide the generic error message.
		suggestions = append(suggestions, fmt.Sprintf("'%s%s' doesn't exist in Saltbox nor Sandbox. Use '--no-cache' if developing your own role.", currentPrefix, providedTag))
		if verbosity > 0 {
			fmt.Printf("DEBUG: No match found for '%s%s'\n", currentPrefix, providedTag)
		}
	}

	sort.Strings(suggestions) // Sort suggestions alphabetically
	return suggestions
}

// getValidTags retrieves valid tags from the cache, handling potential errors, and updates the cache if needed
func getValidTags(ctx context.Context, repoPath string, cacheInstance *cache.Cache, verbosity int) []string {
	playbookPath := ""
	switch repoPath {
	case constants.SaltboxRepoPath:
		playbookPath = constants.SaltboxPlaybookPath()
	case constants.SandboxRepoPath:
		playbookPath = constants.SandboxPlaybookPath()
	default:
		return []string{} // Unknown repo path, return empty slice
	}

	// Check if the cache exists and is *complete* *before* attempting to update.
	repoCache, ok := cacheInstance.GetRepoCache(repoPath)
	if ok {
		if verbosity > 0 {
			fmt.Printf("DEBUG: Cache found for %s\n", repoPath)
		}
		cachedTagsInterface, ok := repoCache["tags"]
		if ok {
			if verbosity > 0 {
				fmt.Printf("DEBUG: 'tags' key found in cache for %s\n", repoPath)
			}
			cachedTags, ok := cachedTagsInterface.([]any)
			if ok {
				if verbosity > 0 {
					fmt.Printf("DEBUG: Cache is valid type for: %s\n", repoPath)
				}
				cachedTagsStrings := make([]string, 0, len(cachedTags))
				for _, tag := range cachedTags {
					if strTag, ok := tag.(string); ok {
						cachedTagsStrings = append(cachedTagsStrings, strTag)
					}
				}
				// If we got here, the cache is valid. Return
				if verbosity > 0 {
					fmt.Printf("DEBUG: Cache valid. Returning %v\n", cachedTagsStrings)
				}
				return cachedTagsStrings
			} else if verbosity > 0 {
				fmt.Printf("DEBUG: Cache is invalid type for %s\n", repoPath)
			}
		} else if verbosity > 0 {
			fmt.Printf("DEBUG: 'tags' key NOT found in cache for %s\n", repoPath)
		}
	} else if verbosity > 0 {
		fmt.Printf("DEBUG: Cache NOT found for %s\n", repoPath)
	}

	// Attempt to update/populate the cache if not valid.
	if verbosity > 0 {
		fmt.Printf("DEBUG: Attempting to update/populate cache for %s\n", repoPath)
	}
	_, err := ansible.RunAndCacheAnsibleTags(ctx, repoPath, playbookPath, "", cacheInstance) // Use empty string for extraSkipTags
	if err != nil {
		handleInterruptError(err)
		if verbosity > 0 {
			fmt.Printf("DEBUG: Error updating cache for %s: %v\n", repoPath, err)
		}
	}

	// Retrieve again
	repoCache, ok = cacheInstance.GetRepoCache(repoPath)

	// If *still* not ok, then return empty.
	if !ok {
		if verbosity > 0 {
			fmt.Printf("DEBUG: Cache still not ok after update for %s\n", repoPath)
		}
		return []string{}
	}
	cachedTagsInterface, ok := repoCache["tags"]
	if !ok {
		if verbosity > 0 {
			fmt.Printf("DEBUG: 'tags' key missing after update for %s\n", repoPath)
		}
		return []string{}
	}
	cachedTags, ok := cachedTagsInterface.([]string) // Cast to []string directly
	if !ok {
		if verbosity > 0 {
			fmt.Printf("DEBUG: Cache is invalid type after update for %s.  Expected []string, got %T\n", repoPath, cachedTagsInterface)
		}
		return []string{}
	}

	cachedTagsStrings := make([]string, 0, len(cachedTags)) // Pre-allocate for efficiency
	for _, tag := range cachedTags {
		cachedTagsStrings = append(cachedTagsStrings, tag)

	}
	if verbosity > 0 {
		fmt.Printf("DEBUG: Returning tags after update: %v\n", cachedTagsStrings)
	}
	return cachedTagsStrings
}

// Helper function to check cache existence and validity (DRY principle)
func cacheExistsAndIsValid(repoPath string, cacheInstance *cache.Cache, verbosity int) bool {
	repoCache, ok := cacheInstance.GetRepoCache(repoPath)
	if !ok {
		if verbosity > 0 {
			fmt.Printf("DEBUG: cacheExistsAndIsValid: Cache not found for %s\n", repoPath)
		}
		return false
	}
	if verbosity > 0 {
		fmt.Printf("DEBUG: cacheExistsAndIsValid: Cache found for %s\n", repoPath)
	}

	cachedTagsInterface, ok := repoCache["tags"]
	if !ok {
		if verbosity > 0 {
			fmt.Printf("DEBUG: cacheExistsAndIsValid: 'tags' key not found for %s\n", repoPath)
		}
		return false
	}
	if verbosity > 0 {
		fmt.Printf("DEBUG: cacheExistsAndIsValid: 'tags' key found for %s\n", repoPath)
	}

	// Check if cachedTagsInterface is a slice of interfaces (which is how JSON arrays are typically unmarshalled)
	cachedTagsSlice, ok := cachedTagsInterface.([]any)
	if ok {
		if len(cachedTagsSlice) == 0 {
			if verbosity > 0 {
				fmt.Printf("DEBUG: cacheExistsAndIsValid: 'tags' array is empty for %s\n", repoPath)
			}
			return false
		}

		if verbosity > 0 {
			fmt.Printf("DEBUG: cacheExistsAndIsValid: 'tags' is a non-empty list for %s\n", repoPath)
		}
		return true
	}

	if verbosity > 0 {
		fmt.Printf("DEBUG: cacheExistsAndIsValid: 'tags' is not a []interface{} for %s (type: %T)\n", repoPath, cachedTagsInterface)
	}
	return false
}
