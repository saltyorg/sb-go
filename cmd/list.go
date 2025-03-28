package cmd

import (
	"fmt"
	"os"

	"github.com/saltyorg/sb-go/ansible"
	"github.com/saltyorg/sb-go/cache"
	"github.com/saltyorg/sb-go/constants"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available Saltbox, Sandbox or Saltbox-mod tags",
	Long:  `List available Saltbox, Sandbox or Saltbox-mod tags`,
	Run: func(cmd *cobra.Command, args []string) {
		handleList()
	},
}

var includeMod bool

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().BoolVarP(&includeMod, "include-mod", "m", false, "Include Saltbox-mod tags")
}

func handleList() {
	cacheInstance, err := cache.NewCache()
	if err != nil {
		fmt.Printf("Error creating cache: %v\n", err)
		os.Exit(1)
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

		if info.RepoPath == constants.SaltboxModRepoPath {
			// Always run ansible list tags for saltbox_mod
			tags, err = ansible.RunAnsibleListTags(info.RepoPath, info.PlaybookPath, info.ExtraSkipTags, cacheInstance)
			if err != nil {
				fmt.Printf("Error running ansible list tags for %s: %v\n", info.RepoPath, err)
				continue
			}
		} else {
			// Use cache for other repositories
			cacheRebuilt, err := ansible.RunAndCacheAnsibleTags(info.RepoPath, info.PlaybookPath, info.ExtraSkipTags, cacheInstance)
			if err != nil {
				fmt.Printf("Error running and caching ansible tags for %s: %v\n", info.RepoPath, err)
				continue
			}

			repoCache, _ := cacheInstance.GetRepoCache(info.RepoPath)

			tagsInterface, ok := repoCache["tags"]
			if !ok {
				fmt.Printf("Error: Tags not found in cache for %s. RepoCache: %+v\n", info.RepoPath, repoCache)
				continue
			}

			tags = make([]string, 0)
			switch v := tagsInterface.(type) {
			case []interface{}:
				for _, tag := range v {
					if strTag, ok := tag.(string); ok {
						tags = append(tags, strTag)
					} else {
						fmt.Printf("Error: Non-string tag found in cache for %s. Tag: %+v\n", info.RepoPath, tag)
						continue
					}
				}
			case []string:
				tags = v
			default:
				fmt.Printf("Error: Unexpected type for tags in cache for %s. Type: %T\n", info.RepoPath, tagsInterface)
				continue
			}

			if !cacheRebuilt && repoCache != nil {
				cacheStatus = " (cached)"
			}
		}

		fmt.Printf("%s%s\n\n", info.BaseTitle, cacheStatus)
		printInColumns(tags, 2)
	}
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

	numColumns := consoleWidth / maxTagLength
	if numColumns < 1 {
		numColumns = 1
	}
	numRows := (len(tags) + numColumns - 1) / numColumns

	for row := 0; row < numRows; row++ {
		for col := 0; col < numColumns; col++ {
			idx := row + col*numRows
			if idx < len(tags) {
				fmt.Printf("%-*s", maxTagLength, tags[idx])
			}
		}
		fmt.Println()
	}
}
