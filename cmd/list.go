package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/saltyorg/sb-go/ansible"
	"github.com/saltyorg/sb-go/layout"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/agnivade/levenshtein"
	aquatable "github.com/aquasecurity/table"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// listCmd represents the list command
func newListCommand() *cobra.Command {
	opts := struct {
		includeMod bool
		verbosity  int
	}{}
	listCmd := &cobra.Command{
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
			var query string
			if len(args) > 0 {
				query = args[0]
			}

			return handleList(ctx, opts.verbosity, query, opts.includeMod)
		},
	}
	listCmd.Flags().BoolVarP(&opts.includeMod, "include-mod", "m", false, "Include Saltbox-mod tags")
	listCmd.Flags().CountVarP(&opts.verbosity, "verbose", "v", "Increase verbosity level (can be used multiple times, e.g. -vvv)")
	return listCmd
}

func addListCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newListCommand())
}

// tagResult holds a tag with its metadata for search results
type tagResult struct {
	tag      string
	prefix   string
	repoName string
	distance int
}

func handleList(ctx context.Context, verbosity int, query string, includeMod bool) error {
	cacheInstance, err := ansible.NewCache()
	if err != nil {
		return fmt.Errorf("error creating cache: %w", err)
	}

	terminal.Debug(verbosity, "Cache instance created successfully")

	repoInfo := []struct {
		RepoPath      string
		PlaybookPath  string
		ExtraSkipTags string
		BaseTitle     string
		Prefix        string
		RepoName      string
	}{
		{layout.SaltboxRepoPath, layout.SaltboxPlaybookPath(), "", "Saltbox tags:", "", "Saltbox"},
		{layout.Current().SandboxRepoPath, layout.SandboxPlaybookPath(), "sanity_check", "\nSandbox tags (prepend sandbox-):", "sandbox-", "Sandbox"},
	}

	if includeMod {
		if _, err := os.Stat(layout.Current().SaltboxModRepoPath); err == nil {
			repoInfo = append(repoInfo, struct {
				RepoPath      string
				PlaybookPath  string
				ExtraSkipTags string
				BaseTitle     string
				Prefix        string
				RepoName      string
			}{layout.Current().SaltboxModRepoPath, layout.SaltboxModPlaybookPath(), "sanity_check", "\nSaltbox_mod tags (prepend mod-):", "mod-", "Saltbox-mod"})
		} else if errors.Is(err, os.ErrNotExist) {
			fmt.Println("Saltbox-mod directory not found, skipping.  Ensure Saltbox-mod is installed.")
		} else {
			return fmt.Errorf("inspect Saltbox-mod directory: %w", err)
		}
	}

	// If search query provided, collect all tags first
	if query != "" {
		return handleSearch(ctx, query, repoInfo, cacheInstance, verbosity)
	}

	// Normal list mode - display by repository
	var errs []error
	for _, info := range repoInfo {
		var tags []string // Declare tags here
		cacheStatus := "" // Default to empty string

		terminal.Debug(verbosity, "Processing repository: %s", info.RepoPath)
		terminal.Debug(verbosity, "Playbook path: %s", info.PlaybookPath)
		terminal.Debug(verbosity, "Extra skip tags: %s", info.ExtraSkipTags)

		if info.RepoPath == layout.Current().SaltboxModRepoPath {
			// Always run ansible list tags for saltbox_mod
			terminal.Debug(verbosity, "Running ansible list tags for saltbox_mod (no cache)")
			tags, err = ansible.RunAnsibleListTags(ctx, info.RepoPath, info.PlaybookPath, info.ExtraSkipTags, cacheInstance, verbosity)
			if err != nil {
				handleInterruptError(ctx, err)
				errs = append(errs, fmt.Errorf("error running ansible list tags for %s: %w", info.RepoPath, err))
				continue
			}
			terminal.Debug(verbosity, "Retrieved %d tags from ansible", len(tags))
		} else {
			// Use cache for other repositories
			terminal.Debug(verbosity, "Attempting to use cache for %s", info.RepoPath)
			cacheRebuilt, err := ansible.RunAndCacheAnsibleTags(ctx, info.RepoPath, info.PlaybookPath, info.ExtraSkipTags, cacheInstance, verbosity)
			if err != nil {
				handleInterruptError(ctx, err)
				errs = append(errs, fmt.Errorf("error running and caching ansible tags for %s: %w", info.RepoPath, err))
				continue
			}
			terminal.Debug(verbosity, "Cache rebuilt: %t", cacheRebuilt)

			repoCache, cacheFound := cacheInstance.GetRepoCache(info.RepoPath)
			terminal.Debug(verbosity, "Cache found for %s: %t", info.RepoPath, cacheFound)
			if cacheFound {
				terminal.Debug(verbosity, "Cache contents: %+v", repoCache)
			}

			tagsInterface, ok := repoCache["tags"]
			if !ok {
				errs = append(errs, fmt.Errorf("tags not found in cache for %s", info.RepoPath))
				continue
			}

			terminal.Debug(verbosity, "Tags interface type: %T", tagsInterface)

			tags = make([]string, 0)
			switch v := tagsInterface.(type) {
			case []any:
				terminal.Debug(verbosity, "Processing []any with %d elements", len(v))
				for i, tag := range v {
					if strTag, ok := tag.(string); ok {
						tags = append(tags, strTag)
					} else {
						errs = append(errs, fmt.Errorf("non-string tag in cache for %s at index %d", info.RepoPath, i))
					}
				}
			case []string:
				terminal.Debug(verbosity, "Processing []string with %d elements", len(v))
				tags = v
			default:
				errs = append(errs, fmt.Errorf("unexpected tags type in cache for %s: %T", info.RepoPath, tagsInterface))
				continue
			}

			terminal.Debug(verbosity, "Successfully extracted %d tags from cache", len(tags))

			if !cacheRebuilt && repoCache != nil {
				cacheStatus = " (cached)"
				terminal.Debug(verbosity, "Using cached tags (not rebuilt)")
			}
		}

		terminal.Debug(verbosity, "Final tag count for %s: %d", info.RepoPath, len(tags))

		fmt.Printf("%s%s\n\n", info.BaseTitle, cacheStatus)
		printInColumns(tags, 2)
	}

	// Return accumulated errors if any repos failed
	if len(errs) > 0 {
		return fmt.Errorf("errors occurred while listing tags: %w", errors.Join(errs...))
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
}, cacheInstance *ansible.Cache, verbosity int) error {
	queryLower := strings.ToLower(query)
	var allResults []tagResult
	var errs []error

	// Collect tags from all repositories
	for _, info := range repoInfo {
		var tags []string

		terminal.Debug(verbosity, "Processing repository: %s", info.RepoPath)

		if info.RepoPath == layout.Current().SaltboxModRepoPath {
			// Always run ansible list tags for saltbox_mod
			var err error
			tags, err = ansible.RunAnsibleListTags(ctx, info.RepoPath, info.PlaybookPath, info.ExtraSkipTags, cacheInstance, verbosity)
			if err != nil {
				handleInterruptError(ctx, err)
				errs = append(errs, fmt.Errorf("error running ansible list tags for %s: %w", info.RepoPath, err))
				continue
			}
		} else {
			// Use cache for other repositories
			_, err := ansible.RunAndCacheAnsibleTags(ctx, info.RepoPath, info.PlaybookPath, info.ExtraSkipTags, cacheInstance, verbosity)
			if err != nil {
				handleInterruptError(ctx, err)
				errs = append(errs, fmt.Errorf("error running and caching ansible tags for %s: %w", info.RepoPath, err))
				continue
			}

			repoCache, cacheFound := cacheInstance.GetRepoCache(info.RepoPath)
			if !cacheFound {
				errs = append(errs, fmt.Errorf("cache not found for %s", info.RepoPath))
				continue
			}

			tagsInterface, ok := repoCache["tags"]
			if !ok {
				errs = append(errs, fmt.Errorf("tags not found in cache for %s", info.RepoPath))
				continue
			}

			tags = make([]string, 0)
			switch v := tagsInterface.(type) {
			case []any:
				for i, tag := range v {
					if strTag, ok := tag.(string); ok {
						tags = append(tags, strTag)
					} else {
						errs = append(errs, fmt.Errorf("non-string tag in cache for %s at index %d", info.RepoPath, i))
					}
				}
			case []string:
				tags = v
			default:
				errs = append(errs, fmt.Errorf("unexpected tags type in cache for %s: %T", info.RepoPath, tagsInterface))
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
		if len(errs) > 0 {
			return fmt.Errorf("errors occurred while searching tags: %w", errors.Join(errs...))
		}
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
	t := terminal.New(os.Stdout)

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

	// Return accumulated errors if any repos failed
	if len(errs) > 0 {
		return fmt.Errorf("errors occurred while searching tags: %w", errors.Join(errs...))
	}
	return nil
}
