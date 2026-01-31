package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/saltyorg/sb-go/internal/ansible"
	"github.com/saltyorg/sb-go/internal/cache"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/git"
	"github.com/saltyorg/sb-go/internal/logging"
	"github.com/saltyorg/sb-go/internal/styles"
	"github.com/saltyorg/sb-go/internal/utils"

	"github.com/agnivade/levenshtein"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/charmtone"
	"github.com/spf13/cobra"
)

// suggestionType represents the type of suggestion being made
type suggestionType int

const (
	suggestionExactMatch suggestionType = iota // Exact match in other repo
	suggestionTypo                             // Likely typo in same repo
	suggestionTypoOther                        // Likely typo in other repo
	suggestionNotFound                         // Not found anywhere
)

// suggestion represents a tag validation issue with a proposed solution
type suggestion struct {
	inputTag    string
	suggestTag  string
	currentRepo string
	targetRepo  string
	sType       suggestionType
}

var forceDiskFull bool

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
			normalStyle := lipgloss.NewStyle()
			return fmt.Errorf("%s", normalStyle.Render("no tags provided"))
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
		// Silence help usage output once initial flags have been validated
		cmd.SilenceUsage = true

		return handleInstall(cmd, tags, extraVars, skipTags, extraArgs, verbosity, noCache)
	},
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Initialize cache
		cacheInstance, err := cache.NewCache()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		// Check if cache is populated
		if !isCachePopulated(cacheInstance) {
			// Try to auto-generate cache - at least one must succeed for completion to work
			ctx := context.Background()

			// Try Saltbox first
			_, saltboxErr := ansible.RunAndCacheAnsibleTags(ctx, constants.SaltboxRepoPath, constants.SaltboxPlaybookPath(), "", cacheInstance, 0)
			saltboxSuccess := saltboxErr == nil

			// Try Sandbox
			_, sandboxErr := ansible.RunAndCacheAnsibleTags(ctx, constants.SandboxRepoPath, constants.SandboxPlaybookPath(), "", cacheInstance, 0)
			sandboxSuccess := sandboxErr == nil

			// If both failed, abort completion
			if !saltboxSuccess && !sandboxSuccess {
				return nil, cobra.ShellCompDirectiveError
			}
		}

		// Retrieve and return all tags
		allTags := getCompletionTags(cacheInstance)
		return allTags, cobra.ShellCompDirectiveNoFileComp
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
	installCmd.Flags().StringArrayP("extra-vars", "e", []string{}, "Extra variables to pass to Ansible")
	installCmd.Flags().StringSliceP("skip-tags", "s", []string{}, "Tags to skip during Ansible playbook execution")
	installCmd.Flags().CountP("verbose", "v", "Increase verbosity level (can be used multiple times, e.g. -vvv)")
	installCmd.Flags().Bool("no-cache", false, "Skip cache validation and always perform tag checks")
	installCmd.Flags().BoolVar(&forceDiskFull, "force-disk-full", false, "Force disk space failure (debug)")
	_ = installCmd.Flags().MarkHidden("force-disk-full")
}

func handleInstall(cmd *cobra.Command, tags []string, extraVars []string, skipTags []string, extraArgs []string, verbosity int, noCache bool) error {
	ctx := cmd.Context()
	var saltboxTags []string
	var sandboxTags []string
	var saltboxModTags []string

	appDataPath := filepath.Dir(constants.SandboxRepoPath)

	if forceDiskFull {
		return utils.DiskSpaceError(appDataPath, 100.0, 0)
	}

	if err := utils.CheckDiskSpace([]string{"/", appDataPath}, verbosity); err != nil {
		return err
	}

	cacheInstance, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("error creating cache: %w", err)
	}

	for _, tag := range tags {
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

		logging.Debug(verbosity, "needsCacheUpdate: %t", needsCacheUpdate)

		if needsCacheUpdate {
			fmt.Println("INFO: Cache missing or incomplete, updating the cache")
		}
	} else {
		logging.Debug(verbosity, "Cache validation skipped due to --no-cache flag")
	}

	if !noCache {
		var allSuggestions []suggestion

		if len(saltboxTags) > 0 {
			suggestions, err := validateAndSuggest(ctx, constants.SaltboxRepoPath, saltboxTags, "", "sandbox-", cacheInstance, verbosity)
			if err != nil {
				return err
			}
			allSuggestions = append(allSuggestions, suggestions...)
		}

		if len(sandboxTags) > 0 {
			suggestions, err := validateAndSuggest(ctx, constants.SandboxRepoPath, sandboxTags, "sandbox-", "", cacheInstance, verbosity)
			if err != nil {
				return err
			}
			allSuggestions = append(allSuggestions, suggestions...)
		}

		if len(allSuggestions) > 0 {
			return fmt.Errorf("%s", formatSuggestions(allSuggestions))
		}
	}

	logging.Debug(verbosity, "No suggestions needed, continuing")

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

	err := ansible.RunAnsiblePlaybook(ctx, repoPath, playbookPath, ansibleBinaryPath, allArgs, true) // Always use true for verbose
	if err != nil {
		handleInterruptError(err)
		return err
	}
	return nil
}

// formatSuggestions builds a formatted string with all suggestions
func formatSuggestions(suggestions []suggestion) string {
	// Define styles
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.ColorYellow))
	inputStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.ColorBrightRed)).Bold(true)
	suggestStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.ColorBrightGreen)).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(charmtone.Cheeky.Hex())) // Tag:, Try:, Did you mean:
	normalStyle := lipgloss.NewStyle()                                                   // Regular text

	var result strings.Builder
	result.WriteString(warningStyle.Render("Tag validation found some issues:") + "\n\n")

	for _, s := range suggestions {
		switch s.sType {
		case suggestionExactMatch:
			// Exact match in other repo - this is the most helpful suggestion
			result.WriteString(fmt.Sprintf("%s %s %s\n",
				labelStyle.Render("Tag:"),
				inputStyle.Render(s.inputTag),
				normalStyle.Render("not present in "+s.currentRepo)))
			result.WriteString(fmt.Sprintf("%s %s %s\n",
				labelStyle.Render("Try:"),
				suggestStyle.Render(s.suggestTag),
				normalStyle.Render("(from "+s.targetRepo+")")))

		case suggestionTypo:
			// Likely typo in same repo
			result.WriteString(fmt.Sprintf("%s %s %s\n",
				labelStyle.Render("Tag:"),
				inputStyle.Render(s.inputTag),
				normalStyle.Render("not present in "+s.currentRepo)))
			result.WriteString(fmt.Sprintf("%s %s\n",
				labelStyle.Render("Did you mean:"),
				suggestStyle.Render(s.suggestTag)))

		case suggestionTypoOther:
			// Likely typo in other repo
			result.WriteString(fmt.Sprintf("%s %s %s\n",
				labelStyle.Render("Tag:"),
				inputStyle.Render(s.inputTag),
				normalStyle.Render("not present in "+s.currentRepo)))
			result.WriteString(fmt.Sprintf("%s %s %s\n",
				labelStyle.Render("Did you mean:"),
				suggestStyle.Render(s.suggestTag),
				normalStyle.Render("(from "+s.targetRepo+")")))

		case suggestionNotFound:
			// Not found anywhere
			infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.ColorLightBlue))
			result.WriteString(fmt.Sprintf("%s %s %s\n",
				labelStyle.Render("Tag:"),
				inputStyle.Render(s.inputTag),
				normalStyle.Render("not present in Saltbox or Sandbox")))
			result.WriteString(fmt.Sprintf("%s %s %s\n",
				labelStyle.Render("Add:"),
				infoStyle.Render("--no-cache"),
				normalStyle.Render("if developing your own role")))
		}

		// Add blank line between suggestions
		result.WriteString("\n")
	}

	return strings.TrimSuffix(result.String(), "\n")
}

func validateAndSuggest(ctx context.Context, repoPath string, providedTags []string, currentPrefix, otherPrefix string, cacheInstance *cache.Cache, verbosity int) ([]suggestion, error) {
	var suggestions []suggestion

	// Ensure the cache exists and is populated.
	validTags := getValidTags(ctx, repoPath, cacheInstance, verbosity)

	logging.Debug(verbosity, "Valid tags for %s (after getValidTags): %v", repoPath, validTags)
	if repoPath == constants.SaltboxRepoPath && len(validTags) == 0 {
		return nil, fmt.Errorf("saltbox install appears broken: tags cache missing or empty")
	}

	otherRepoPath := constants.SandboxRepoPath
	if repoPath == constants.SandboxRepoPath {
		otherRepoPath = constants.SaltboxRepoPath
	}
	otherValidTags := getValidTags(ctx, otherRepoPath, cacheInstance, verbosity)
	logging.Debug(verbosity, "Valid tags for other repo %s (after getValidTags): %v", otherRepoPath, otherValidTags)
	if otherRepoPath == constants.SaltboxRepoPath && len(otherValidTags) == 0 {
		return nil, fmt.Errorf("saltbox install appears broken: tags cache missing or empty")
	}

	repoName := "Saltbox"
	otherRepoName := "Sandbox"
	if repoPath == constants.SandboxRepoPath {
		repoName = "Sandbox"
		otherRepoName = "Saltbox"
	}

	for _, providedTag := range providedTags {
		logging.Debug(verbosity, "Checking tag: %s%s", currentPrefix, providedTag)
		found := slices.Contains(validTags, providedTag)
		if found {
			logging.Debug(verbosity, "Exact match found for %s%s", currentPrefix, providedTag)
			continue // Tag is valid, no suggestion needed
		}

		// 2. Check for an exact match in the *other* repository (the strongest suggestion)
		if slices.Contains(otherValidTags, providedTag) {
			suggestions = append(suggestions, suggestion{
				inputTag:    currentPrefix + providedTag,
				suggestTag:  otherPrefix + providedTag,
				currentRepo: repoName,
				targetRepo:  otherRepoName,
				sType:       suggestionExactMatch,
			})
			logging.Debug(verbosity, "Exact match found in other repo for %s%s, suggesting %s%s", currentPrefix, providedTag, otherPrefix, providedTag)
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
			logging.Debug(verbosity, "Distance between '%s' and '%s': %d", providedTag, validTag, distance)
			if distance < bestDistance && distance <= 2 { // Threshold of 2
				bestDistance = distance
				bestMatch = validTag
			}
		}

		if bestMatch != "" {
			suggestions = append(suggestions, suggestion{
				inputTag:    currentPrefix + providedTag,
				suggestTag:  currentPrefix + bestMatch,
				currentRepo: repoName,
				targetRepo:  repoName,
				sType:       suggestionTypo,
			})
			logging.Debug(verbosity, "Suggesting '%s%s' for '%s%s'", currentPrefix, bestMatch, currentPrefix, providedTag)
			continue
		}

		// 4. Find close matches in the *other* repository.
		bestMatchOther := ""
		bestDistanceOther := 9999
		for _, otherValidTag := range otherValidTags {
			distance := levenshtein.ComputeDistance(providedTag, otherValidTag)
			logging.Debug(verbosity, "Distance between '%s' and '%s' in other repo: %d", providedTag, otherValidTag, distance)
			if distance < bestDistanceOther && distance <= 2 {
				bestDistanceOther = distance
				bestMatchOther = otherValidTag
			}
		}
		if bestMatchOther != "" {
			suggestions = append(suggestions, suggestion{
				inputTag:    currentPrefix + providedTag,
				suggestTag:  otherPrefix + bestMatchOther,
				currentRepo: repoName,
				targetRepo:  otherRepoName,
				sType:       suggestionTypoOther,
			})
			logging.Debug(verbosity, "Suggesting '%s%s' for '%s%s' from other repo", otherPrefix, bestMatchOther, currentPrefix, providedTag)
			continue
		}

		// 5. No close match found, provide the generic error message.
		suggestions = append(suggestions, suggestion{
			inputTag:    currentPrefix + providedTag,
			suggestTag:  "",
			currentRepo: repoName,
			targetRepo:  "",
			sType:       suggestionNotFound,
		})
		logging.Debug(verbosity, "No match found for '%s%s'", currentPrefix, providedTag)
	}

	// Sort suggestions by input tag alphabetically
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].inputTag < suggestions[j].inputTag
	})
	return suggestions, nil
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
	// Also verify that the commit hash matches the current repository state.
	repoCache, ok := cacheInstance.GetRepoCache(repoPath)
	if ok {
		logging.Debug(verbosity, "Cache found for %s", repoPath)
		// Check if commit hash matches
		if cachedCommit, commitOK := repoCache["commit"].(string); commitOK {
			currentCommit, err := git.GetGitCommitHash(ctx, repoPath)
			if err == nil && cachedCommit == currentCommit {
				// Commit matches, check if tags exist
				cachedTagsInterface, ok := repoCache["tags"]
				if ok {
					logging.Debug(verbosity, "'tags' key found in cache for %s with matching commit", repoPath)
					cachedTags, ok := cachedTagsInterface.([]any)
					if ok {
						logging.Debug(verbosity, "Cache is valid type for: %s", repoPath)
						cachedTagsStrings := make([]string, 0, len(cachedTags))
						for _, tag := range cachedTags {
							if strTag, ok := tag.(string); ok {
								cachedTagsStrings = append(cachedTagsStrings, strTag)
							}
						}
						// If we got here, the cache is valid. Return
						logging.Debug(verbosity, "Cache valid. Returning %v", cachedTagsStrings)
						return cachedTagsStrings
					} else {
						logging.Debug(verbosity, "Cache is invalid type for %s", repoPath)
					}
				} else {
					logging.Debug(verbosity, "'tags' key NOT found in cache for %s", repoPath)
				}
			} else {
				if err != nil {
					logging.Debug(verbosity, "Error getting current commit for %s: %v", repoPath, err)
				} else {
					logging.Debug(verbosity, "Commit mismatch for %s (cached: %s, current: %s)", repoPath, cachedCommit, currentCommit)
				}
			}
		} else {
			logging.Debug(verbosity, "No valid commit hash in cache for %s", repoPath)
		}
	} else {
		logging.Debug(verbosity, "Cache NOT found for %s", repoPath)
	}

	// Attempt to update/populate the cache if not valid.
	logging.Debug(verbosity, "Attempting to update/populate cache for %s", repoPath)
	_, err := ansible.RunAndCacheAnsibleTags(ctx, repoPath, playbookPath, "", cacheInstance, verbosity) // Use empty string for extraSkipTags
	if err != nil {
		handleInterruptError(err)
		logging.Debug(verbosity, "Error updating cache for %s: %v", repoPath, err)
	}

	// Retrieve again
	repoCache, ok = cacheInstance.GetRepoCache(repoPath)

	// If *still* not ok, then return empty.
	if !ok {
		logging.Debug(verbosity, "Cache still not ok after update for %s", repoPath)
		return []string{}
	}
	cachedTagsInterface, ok := repoCache["tags"]
	if !ok {
		logging.Debug(verbosity, "'tags' key missing after update for %s", repoPath)
		return []string{}
	}

	// Handle both []string (freshly cached) and []any (loaded from JSON)
	switch tags := cachedTagsInterface.(type) {
	case []string:
		logging.Debug(verbosity, "Returning tags after update ([]string): %v", tags)
		return tags
	case []any:
		cachedTagsStrings := make([]string, 0, len(tags))
		for _, tag := range tags {
			if strTag, ok := tag.(string); ok {
				cachedTagsStrings = append(cachedTagsStrings, strTag)
			}
		}
		logging.Debug(verbosity, "Returning tags after update ([]any): %v", cachedTagsStrings)
		return cachedTagsStrings
	default:
		logging.Debug(verbosity, "Cache is invalid type after update for %s. Expected []string or []any, got %T", repoPath, cachedTagsInterface)
		return []string{}
	}
}

// Helper function to check cache existence and validity (DRY principle)
func cacheExistsAndIsValid(repoPath string, cacheInstance *cache.Cache, verbosity int) bool {
	repoCache, ok := cacheInstance.GetRepoCache(repoPath)
	if !ok {
		logging.Debug(verbosity, "cacheExistsAndIsValid: Cache not found for %s", repoPath)
		return false
	}
	logging.Debug(verbosity, "cacheExistsAndIsValid: Cache found for %s", repoPath)

	cachedTagsInterface, ok := repoCache["tags"]
	if !ok {
		logging.Debug(verbosity, "cacheExistsAndIsValid: 'tags' key not found for %s", repoPath)
		return false
	}
	logging.Debug(verbosity, "cacheExistsAndIsValid: 'tags' key found for %s", repoPath)

	// Check if cachedTagsInterface is a slice of interfaces (which is how JSON arrays are typically unmarshalled)
	cachedTagsSlice, ok := cachedTagsInterface.([]any)
	if ok {
		if len(cachedTagsSlice) == 0 {
			logging.Debug(verbosity, "cacheExistsAndIsValid: 'tags' array is empty for %s", repoPath)
			return false
		}

		logging.Debug(verbosity, "cacheExistsAndIsValid: 'tags' is a non-empty list for %s", repoPath)
		return true
	}

	logging.Debug(verbosity, "cacheExistsAndIsValid: 'tags' is not a []interface{} for %s (type: %T)", repoPath, cachedTagsInterface)
	return false
}

// isCachePopulated checks if the cache has valid tags for at least one repository
func isCachePopulated(cacheInstance *cache.Cache) bool {
	// Check Saltbox cache using existing validation function
	if cacheExistsAndIsValid(constants.SaltboxRepoPath, cacheInstance, 0) {
		return true
	}

	// Check Sandbox cache using existing validation function
	if cacheExistsAndIsValid(constants.SandboxRepoPath, cacheInstance, 0) {
		return true
	}

	return false
}

// getCompletionTags retrieves and formats all tags from cache for shell completion
func getCompletionTags(cacheInstance *cache.Cache) []string {
	var allTags []string

	// Get Saltbox tags (returned as-is)
	saltboxCache, ok := cacheInstance.GetRepoCache(constants.SaltboxRepoPath)
	if ok {
		if cachedTags, tagsOK := saltboxCache["tags"].([]any); tagsOK {
			for _, tag := range cachedTags {
				if strTag, ok := tag.(string); ok {
					allTags = append(allTags, strTag)
				}
			}
		}
	}

	// Get Sandbox tags (prefixed with "sandbox-")
	sandboxCache, ok := cacheInstance.GetRepoCache(constants.SandboxRepoPath)
	if ok {
		if cachedTags, tagsOK := sandboxCache["tags"].([]any); tagsOK {
			for _, tag := range cachedTags {
				if strTag, ok := tag.(string); ok {
					allTags = append(allTags, "sandbox-"+strTag)
				}
			}
		}
	}

	return allTags
}
