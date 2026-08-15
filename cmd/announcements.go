package cmd

import (
	"fmt"

	"github.com/saltyorg/sb-go/layout"
	"github.com/saltyorg/sb-go/saltbox"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/spf13/cobra"
)

type announcementsOptions struct {
	verbose               bool
	beforePath, afterPath string
	repository            string
}

func newAnnouncementsCommand() *cobra.Command {
	var opts announcementsOptions
	announcementsCmd := &cobra.Command{
		Use:    "announcements",
		Short:  "Display announcements from announcement files",
		Long:   `Display announcements by comparing before and after announcements.yml files`,
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleAnnouncements(opts.verbose, opts.beforePath, opts.afterPath, opts.repository)
		},
	}
	announcementsCmd.PersistentFlags().BoolVarP(&opts.verbose, "verbose", "v", false, "Enable verbose output")
	announcementsCmd.PersistentFlags().StringVarP(&opts.beforePath, "before", "b", "", "Path to the before announcements.yml file")
	announcementsCmd.PersistentFlags().StringVarP(&opts.afterPath, "after", "a", "", "Path to the after announcements.yml file")
	announcementsCmd.PersistentFlags().StringVarP(&opts.repository, "repo", "r", "Saltbox", "Repository name (Saltbox or Sandbox)")
	if err := announcementsCmd.MarkPersistentFlagRequired("before"); err != nil {
		panic(fmt.Sprintf("failed to mark 'before' flag as required: %v", err))
	}
	if err := announcementsCmd.MarkPersistentFlagRequired("after"); err != nil {
		panic(fmt.Sprintf("failed to mark 'after' flag as required: %v", err))
	}
	return announcementsCmd
}

func addAnnouncementsCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newAnnouncementsCommand())
}

func handleAnnouncements(verbose bool, beforePath, afterPath, repo string) error {
	runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: verbose})

	// Validate repository name
	if repo != "Saltbox" && repo != "Sandbox" {
		return fmt.Errorf("invalid repository name '%s', must be 'Saltbox' or 'Sandbox'", repo)
	}

	// Load the before announcement file
	beforeFile, err := saltbox.LoadSingleAnnouncementFile(beforePath)
	if err != nil {
		return fmt.Errorf("error loading before announcement file '%s': %w", beforePath, err)
	}

	// Load the after announcement file
	afterFile, err := saltbox.LoadSingleAnnouncementFile(afterPath)
	if err != nil {
		return fmt.Errorf("error loading after announcement file '%s': %w", afterPath, err)
	}

	// Determine repo path based on repo name
	var repoPath string
	if repo == "Saltbox" {
		repoPath = layout.SaltboxRepoPath
	} else {
		repoPath = layout.Current().SandboxRepoPath
	}

	// Check for new announcements
	diff := saltbox.CheckSingleRepoAnnouncements(repo, repoPath, beforeFile, afterFile)

	// Create announcement diffs array for consistency with existing functions
	announcementDiffs := []*saltbox.AnnouncementDiff{diff}

	// Display new announcements
	if err := saltbox.DisplayAnnouncements(runner, announcementDiffs); err != nil {
		return fmt.Errorf("error displaying announcements: %w", err)
	}

	// Prompt for migration approvals
	migrationRequests, err := saltbox.PromptForMigrations(announcementDiffs)
	if err != nil {
		return fmt.Errorf("error prompting for migrations: %w", err)
	}

	// Display what migrations would be executed (simulation only)
	if len(migrationRequests) > 0 {
		for _, migration := range migrationRequests {
			msg := fmt.Sprintf("Would execute migration '%s' for %s repository", migration.Tag, migration.RepoName)
			runner.Info(msg)
		}
	}

	return nil
}
