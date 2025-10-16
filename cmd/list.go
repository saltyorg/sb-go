package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/saltyorg/sb-go/internal/ansible"
	"github.com/saltyorg/sb-go/internal/cache"
	"github.com/saltyorg/sb-go/internal/constants"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available Saltbox, Sandbox or Saltbox-mod tags",
	Long:  `List available Saltbox, Sandbox or Saltbox-mod tags`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		verbosity, _ := cmd.Flags().GetCount("verbose")
		return handleList(ctx, verbosity)
	},
}

var includeMod bool

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().BoolVarP(&includeMod, "include-mod", "m", false, "Include Saltbox-mod tags")
	listCmd.Flags().CountP("verbose", "v", "Increase verbosity level (can be used multiple times, e.g. -vvv)")
}

func handleList(ctx context.Context, verbosity int) error {
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
	}{
		{constants.SaltboxRepoPath, constants.SaltboxPlaybookPath(), "", "Saltbox tags:"},
		{constants.SandboxRepoPath, constants.SandboxPlaybookPath(), "sanity_check", "\nSandbox tags (prepend sandbox-):"},
	}

	if includeMod {
		if _, err := os.Stat(constants.SaltboxModRepoPath); !os.IsNotExist(err) {
			repoInfo = append(repoInfo, struct {
				RepoPath      string
				PlaybookPath  string
				ExtraSkipTags string
				BaseTitle     string
			}{constants.SaltboxModRepoPath, constants.SaltboxModPlaybookPath(), "sanity_check", "\nSaltbox_mod tags (prepend mod-):"})
		} else {
			fmt.Println("Saltbox-mod directory not found, skipping.  Ensure Saltbox-mod is installed.")
		}
	}

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
