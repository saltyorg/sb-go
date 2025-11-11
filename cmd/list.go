package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/agnivade/levenshtein"
	aquatable "github.com/aquasecurity/table"
	"github.com/saltyorg/sb-go/internal/ansible"
	"github.com/saltyorg/sb-go/internal/cache"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/table"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list [query]",
	Short: "List available Saltbox, Sandbox or Saltbox-mod tags",
	Long: `List available Saltbox, Sandbox or Saltbox-mod tags

Without arguments, displays all available tags.
With a query argument, performs fuzzy search across all tags.

Examples:
  sb list                # List all tags
  sb list plex           # Search for tags matching "plex"
  sb list arr            # Search for tags matching "arr"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		verbosity, _ := cmd.Flags().GetCount("verbose")

		var query string
		if len(args) > 0 {
			query = args[0]
		}

		return handleList(ctx, verbosity, query)
	},
}

var includeMod bool

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().BoolVarP(&includeMod, "include-mod", "m", false, "Include Saltbox-mod tags")
	listCmd.Flags().CountP("verbose", "v", "Increase verbosity level (can be used multiple times, e.g. -vvv)")
}

// tagResult holds a tag with its metadata for search results
type tagResult struct {
	tag      string
	prefix   string
	repoName string
	distance int
}

func handleList(ctx context.Context, verbosity int, query string) error {
	cacheInstance, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("error creating cache: %w", err)
	}

	if verbosity > 0 {
		fmt.Println("DEBUG: Cache instance created successfully")
	}

	repoInfo := []struct {
		RepoPath      string
		PlaybookPath  string
		ExtraSkipTags string
		BaseTitle     string
		Prefix        string
		RepoName      string
	}{
		{constants.SaltboxRepoPath, constants.SaltboxPlaybookPath(), "", "Saltbox tags:", "", "Saltbox"},
		{constants.SandboxRepoPath, constants.SandboxPlaybookPath(), "sanity_check", "\nSandbox tags (prepend sandbox-):", "sandbox-", "Sandbox"},
	}

	if includeMod {
		if _, err := os.Stat(constants.SaltboxModRepoPath); !os.IsNotExist(err) {
			repoInfo = append(repoInfo, struct {
				RepoPath      string
				PlaybookPath  string
				ExtraSkipTags string
				BaseTitle     string
				Prefix        string
				RepoName      string
			}{constants.SaltboxModRepoPath, constants.SaltboxModPlaybookPath(), "sanity_check", "\nSaltbox_mod tags (prepend mod-):", "mod-", "Saltbox-mod"})
		} else {
			fmt.Println("Saltbox-mod directory not found, skipping.  Ensure Saltbox-mod is installed.")
		}
	}

	// If search query provided, collect all tags first
	if query != "" {
		return handleSearch(ctx, query, repoInfo, cacheInstance, verbosity)
	}

	// Normal list mode - display by repository
	for _, info := range repoInfo {
		var tags []string // Declare tags here
		cacheStatus := "" // Default to empty string

		if verbosity > 0 {
			fmt.Printf("DEBUG: Processing repository: %s\n", info.RepoPath)
			fmt.Printf("DEBUG: Playbook path: %s\n", info.PlaybookPath)
			fmt.Printf("DEBUG: Extra skip tags: %s\n", info.ExtraSkipTags)
		}

		if info.RepoPath == constants.SaltboxModRepoPath {
			// Always run ansible list tags for saltbox_mod
			if verbosity > 0 {
				fmt.Printf("DEBUG: Running ansible list tags for saltbox_mod (no cache)\n")
			}
			tags, err = ansible.RunAnsibleListTags(ctx, info.RepoPath, info.PlaybookPath, info.ExtraSkipTags, cacheInstance, verbosity)
			if err != nil {
				handleInterruptError(err)
				fmt.Printf("Error running ansible list tags for %s: %v\n", info.RepoPath, err)
				continue
			}
			if verbosity > 0 {
				fmt.Printf("DEBUG: Retrieved %d tags from ansible\n", len(tags))
			}
		} else {
			// Use cache for other repositories
			if verbosity > 0 {
				fmt.Printf("DEBUG: Attempting to use cache for %s\n", info.RepoPath)
			}
			cacheRebuilt, err := ansible.RunAndCacheAnsibleTags(ctx, info.RepoPath, info.PlaybookPath, info.ExtraSkipTags, cacheInstance, verbosity)
			if err != nil {
				handleInterruptError(err)
				fmt.Printf("Error running and caching ansible tags for %s: %v\n", info.RepoPath, err)
				continue
			}
			if verbosity > 0 {
				fmt.Printf("DEBUG: Cache rebuilt: %t\n", cacheRebuilt)
			}

			repoCache, cacheFound := cacheInstance.GetRepoCache(info.RepoPath)
			if verbosity > 0 {
				fmt.Printf("DEBUG: Cache found for %s: %t\n", info.RepoPath, cacheFound)
				if cacheFound {
					fmt.Printf("DEBUG: Cache contents: %+v\n", repoCache)
				}
			}

			tagsInterface, ok := repoCache["tags"]
			if !ok {
				fmt.Printf("Error: Tags not found in cache for %s. RepoCache: %+v\n", info.RepoPath, repoCache)
				continue
			}

			if verbosity > 0 {
				fmt.Printf("DEBUG: Tags interface type: %T\n", tagsInterface)
			}

			tags = make([]string, 0)
			switch v := tagsInterface.(type) {
			case []any:
				if verbosity > 0 {
					fmt.Printf("DEBUG: Processing []any with %d elements\n", len(v))
				}
				for i, tag := range v {
					if strTag, ok := tag.(string); ok {
						tags = append(tags, strTag)
					} else {
						fmt.Printf("Error: Non-string tag found in cache for %s at index %d. Tag: %+v (type: %T)\n", info.RepoPath, i, tag, tag)
						continue
					}
				}
			case []string:
				if verbosity > 0 {
					fmt.Printf("DEBUG: Processing []string with %d elements\n", len(v))
				}
				tags = v
			default:
				fmt.Printf("Error: Unexpected type for tags in cache for %s. Type: %T\n", info.RepoPath, tagsInterface)
				continue
			}

			if verbosity > 0 {
				fmt.Printf("DEBUG: Successfully extracted %d tags from cache\n", len(tags))
			}

			if !cacheRebuilt && repoCache != nil {
				cacheStatus = " (cached)"
				if verbosity > 0 {
					fmt.Printf("DEBUG: Using cached tags (not rebuilt)\n")
				}
			}
		}

		if verbosity > 0 {
			fmt.Printf("DEBUG: Final tag count for %s: %d\n", info.RepoPath, len(tags))
		}

		fmt.Printf("%s%s\n\n", info.BaseTitle, cacheStatus)
		printInColumns(tags, 2)
	}
	return nil
}

func getConsoleWidth(defaultWidth int) int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return defaultWidth
	}
	return width
}

func printInColumns(tags []string, padding int) {
	if len(tags) == 0 {
		return
	}

	consoleWidth := getConsoleWidth(80)
	maxTagLength := 0
	for _, tag := range tags {
		if len(tag) > maxTagLength {
			maxTagLength = len(tag)
		}
	}
	maxTagLength += padding

	numColumns := max(consoleWidth/maxTagLength, 1)
	numRows := (len(tags) + numColumns - 1) / numColumns

	for row := range numRows {
		for col := range numColumns {
			idx := row + col*numRows
			if idx < len(tags) {
				fmt.Printf("%-*s", maxTagLength, tags[idx])
			}
		}
		fmt.Println()
	}
}

func handleSearch(ctx context.Context, query string, repoInfo []struct {
	RepoPath      string
	PlaybookPath  string
	ExtraSkipTags string
	BaseTitle     string
	Prefix        string
	RepoName      string
}, cacheInstance *cache.Cache, verbosity int) error {
	queryLower := strings.ToLower(query)
	var allResults []tagResult

	// Collect tags from all repositories
	for _, info := range repoInfo {
		var tags []string

		if verbosity > 0 {
			fmt.Printf("DEBUG: Processing repository: %s\n", info.RepoPath)
		}

		if info.RepoPath == constants.SaltboxModRepoPath {
			// Always run ansible list tags for saltbox_mod
			var err error
			tags, err = ansible.RunAnsibleListTags(ctx, info.RepoPath, info.PlaybookPath, info.ExtraSkipTags, cacheInstance, verbosity)
			if err != nil {
				handleInterruptError(err)
				fmt.Printf("Error running ansible list tags for %s: %v\n", info.RepoPath, err)
				continue
			}
		} else {
			// Use cache for other repositories
			_, err := ansible.RunAndCacheAnsibleTags(ctx, info.RepoPath, info.PlaybookPath, info.ExtraSkipTags, cacheInstance, verbosity)
			if err != nil {
				handleInterruptError(err)
				fmt.Printf("Error running and caching ansible tags for %s: %v\n", info.RepoPath, err)
				continue
			}

			repoCache, cacheFound := cacheInstance.GetRepoCache(info.RepoPath)
			if !cacheFound {
				continue
			}

			tagsInterface, ok := repoCache["tags"]
			if !ok {
				continue
			}

			tags = make([]string, 0)
			switch v := tagsInterface.(type) {
			case []any:
				for _, tag := range v {
					if strTag, ok := tag.(string); ok {
						tags = append(tags, strTag)
					}
				}
			case []string:
				tags = v
			}
		}

		// Search within tags
		for _, tag := range tags {
			tagLower := strings.ToLower(tag)

			// Check for substring match (more lenient than exact match)
			if strings.Contains(tagLower, queryLower) {
				allResults = append(allResults, tagResult{
					tag:      tag,
					prefix:   info.Prefix,
					repoName: info.RepoName,
					distance: 0, // Exact substring match
				})
				continue
			}

			// Calculate Levenshtein distance for fuzzy matching
			distance := levenshtein.ComputeDistance(queryLower, tagLower)

			// Include tags with distance <= 2 (same threshold as install command)
			if distance <= 2 {
				allResults = append(allResults, tagResult{
					tag:      tag,
					prefix:   info.Prefix,
					repoName: info.RepoName,
					distance: distance,
				})
			}
		}
	}

	if len(allResults) == 0 {
		fmt.Printf("No tags found matching '%s'\n", query)
		return nil
	}

	// Sort results: exact/substring matches first, then by distance, then by repo, then alphabetically
	sort.Slice(allResults, func(i, j int) bool {
		if allResults[i].distance != allResults[j].distance {
			return allResults[i].distance < allResults[j].distance
		}
		if allResults[i].repoName != allResults[j].repoName {
			// Saltbox first, then Sandbox, then Saltbox-mod
			repoOrder := map[string]int{"Saltbox": 0, "Sandbox": 1, "Saltbox-mod": 2}
			return repoOrder[allResults[i].repoName] < repoOrder[allResults[j].repoName]
		}
		return allResults[i].tag < allResults[j].tag
	})

	// Display results in table format
	fmt.Printf("Found %d matching tag(s) for '%s':\n\n", len(allResults), query)

	// Group results by repository
	resultsByRepo := make(map[string][]tagResult)
	for _, result := range allResults {
		resultsByRepo[result.repoName] = append(resultsByRepo[result.repoName], result)
	}

	// Define section order and check which sections have results
	type section struct {
		name    string
		prefix  string
		results []tagResult
	}

	sections := []section{
		{"Saltbox", "", resultsByRepo["Saltbox"]},
		{"Sandbox", "sandbox-", resultsByRepo["Sandbox"]},
		{"Saltbox-mod", "mod-", resultsByRepo["Saltbox-mod"]},
	}

	// Filter out empty sections
	var nonEmptySections []section
	for _, s := range sections {
		if len(s.results) > 0 {
			nonEmptySections = append(nonEmptySections, s)
		}
	}

	if len(nonEmptySections) == 0 {
		return nil
	}

	// Create a single table with all repositories
	t := table.New(os.Stdout)

	// First section becomes the table header
	t.SetHeaders(nonEmptySections[0].name)
	t.SetHeaderColSpans(0, 2)
	t.SetHeaderStyle(aquatable.StyleBold)
	t.SetAlignment(aquatable.AlignLeft, aquatable.AlignLeft)
	t.SetBorders(true)
	t.SetRowLines(true)
	t.SetDividers(aquatable.UnicodeRoundedDividers)
	t.SetLineStyle(aquatable.StyleBlue)
	t.SetPadding(1)

	rowIndex := 0

	// Add first section rows (no section header since it's the table header)
	for _, result := range nonEmptySections[0].results {
		usage := fmt.Sprintf("sb install %s%s", nonEmptySections[0].prefix, result.tag)
		t.AddRow(result.tag, usage)
		rowIndex++
	}

	// Add remaining sections with section headers
	for _, s := range nonEmptySections[1:] {
		// Add section header as a bold centered row with colspan
		sectionHeader := fmt.Sprintf("\033[1m%s\033[0m", s.name)
		t.AddRow(sectionHeader)
		t.SetColSpans(rowIndex, 2)
		rowIndex++

		for _, result := range s.results {
			usage := fmt.Sprintf("sb install %s%s", s.prefix, result.tag)
			t.AddRow(result.tag, usage)
			rowIndex++
		}
	}

	t.Render()
	fmt.Println()

	return nil
}
