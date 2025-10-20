package cmd

import (
	"fmt"

	"github.com/saltyorg/sb-go/internal/announcements"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/spinners"

	"github.com/spf13/cobra"
)

var announcementsCmd = &cobra.Command{
	Use:    "announcements",
	Short:  "Display announcements from announcement files",
	Long:   `Display announcements by comparing before and after announcements.yml files`,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		verbose, _ := cmd.Flags().GetBool("verbose")
		beforePath, _ := cmd.Flags().GetString("before")
		afterPath, _ := cmd.Flags().GetString("after")
		repo, _ := cmd.Flags().GetString("repo")

		return handleAnnouncements(verbose, beforePath, afterPath, repo)
	},
}

func init() {
	rootCmd.AddCommand(announcementsCmd)
	announcementsCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	announcementsCmd.PersistentFlags().StringP("before", "b", "", "Path to the before announcements.yml file")
	announcementsCmd.PersistentFlags().StringP("after", "a", "", "Path to the after announcements.yml file")
	announcementsCmd.PersistentFlags().StringP("repo", "r", "Saltbox", "Repository name (Saltbox or Sandbox)")

	// Mark required flags
	announcementsCmd.MarkPersistentFlagRequired("before")
	announcementsCmd.MarkPersistentFlagRequired("after")
}

func handleAnnouncements(verbose bool, beforePath, afterPath, repo string) error {
	// Set verbose mode for spinners
	spinners.SetVerboseMode(verbose)

	// Validate repository name
	if repo != "Saltbox" && repo != "Sandbox" {
		return fmt.Errorf("invalid repository name '%s', must be 'Saltbox' or 'Sandbox'", repo)
	}

	// Load the before announcement file
	beforeFile, err := announcements.LoadSingleAnnouncementFile(beforePath)
	if err != nil {
		return fmt.Errorf("error loading before announcement file '%s': %w", beforePath, err)
	}

	// Load the after announcement file
	afterFile, err := announcements.LoadSingleAnnouncementFile(afterPath)
	if err != nil {
		return fmt.Errorf("error loading after announcement file '%s': %w", afterPath, err)
	}

	// Determine repo path based on repo name
	var repoPath string
	if repo == "Saltbox" {
		repoPath = constants.SaltboxRepoPath
	} else {
		repoPath = constants.SandboxRepoPath
	}

	// Check for new announcements
	diff := announcements.CheckSingleRepoAnnouncements(repo, repoPath, beforeFile, afterFile)

	// Create announcement diffs array for consistency with existing functions
	announcementDiffs := []*announcements.AnnouncementDiff{diff}

	// Display new announcements
	if err := announcements.DisplayAnnouncements(announcementDiffs); err != nil {
		return fmt.Errorf("error displaying announcements: %w", err)
	}

	// Prompt for migration approvals
	migrationRequests, err := announcements.PromptForMigrations(announcementDiffs)
	if err != nil {
		return fmt.Errorf("error prompting for migrations: %w", err)
	}

	// Display what migrations would be executed (simulation only)
	if len(migrationRequests) > 0 {
		for _, migration := range migrationRequests {
			msg := fmt.Sprintf("Would execute migration '%s' for %s repository", migration.Tag, migration.RepoName)
			if err := spinners.RunInfoSpinner(msg); err != nil {
				return err
			}
		}
	}

	return nil
}
