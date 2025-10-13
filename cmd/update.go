package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/saltyorg/sb-go/internal/announcements"
	"github.com/saltyorg/sb-go/internal/ansible"
	"github.com/saltyorg/sb-go/internal/cache"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/fact"
	"github.com/saltyorg/sb-go/internal/git"
	"github.com/saltyorg/sb-go/internal/python"
	"github.com/saltyorg/sb-go/internal/spinners"
	"github.com/saltyorg/sb-go/internal/utils"
	"github.com/saltyorg/sb-go/internal/validate"
	"github.com/saltyorg/sb-go/internal/venv"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update Saltbox & Sandbox",
	Long:  `Update Saltbox & Sandbox`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		verbose, _ := cmd.Flags().GetBool("verbose")
		keepBranch, _ := cmd.Flags().GetBool("keep-branch")
		resetBranch, _ := cmd.Flags().GetBool("reset-branch")

		var branchReset *bool
		if keepBranch {
			falseVal := false
			branchReset = &falseVal
		} else if resetBranch {
			trueVal := true
			branchReset = &trueVal
		}

		return handleUpdate(ctx, verbose, branchReset)
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	updateCmd.PersistentFlags().Bool("keep-branch", false, "Skip branch reset prompt and stay on current branch")
	updateCmd.PersistentFlags().Bool("reset-branch", false, "Skip branch reset prompt and reset to default branch")
}

func handleUpdate(ctx context.Context, verbose bool, branchReset *bool) error {
	// Set verbose mode for spinners
	spinners.SetVerboseMode(verbose)

	doSelfUpdate(true, verbose, "Re-run the update command to update Saltbox")

	// Load announcement files before updates
	saltboxAnnouncementsBefore, sandboxAnnouncementsBefore, err := announcements.LoadAllAnnouncementFiles()
	if err != nil {
		return fmt.Errorf("error loading announcements before update: %w", err)
	}

	// Update repositories
	if err := updateSaltbox(ctx, verbose, branchReset); err != nil {
		return fmt.Errorf("error updating Saltbox: %w", err)
	}
	if err := updateSandbox(ctx, branchReset); err != nil {
		return fmt.Errorf("error updating Sandbox: %w", err)
	}

	// Load announcement files after updates
	saltboxAnnouncementsAfter, sandboxAnnouncementsAfter, err := announcements.LoadAllAnnouncementFiles()
	if err != nil {
		return fmt.Errorf("error loading announcements after update: %w", err)
	}

	// Check for new announcements in both repositories
	announcementDiffs := announcements.CheckForNewAnnouncementsAllRepos(saltboxAnnouncementsBefore, saltboxAnnouncementsAfter, sandboxAnnouncementsBefore, sandboxAnnouncementsAfter)

	// Display new announcements
	if err := announcements.DisplayAnnouncements(announcementDiffs); err != nil {
		return fmt.Errorf("error displaying announcements: %w", err)
	}

	// Prompt for migration approvals and execute
	migrationRequests, err := announcements.PromptForMigrations(announcementDiffs)
	if err != nil {
		return fmt.Errorf("error prompting for migrations: %w", err)
	}

	// Execute migration requests with context
	if err := announcements.ExecuteMigrations(ctx, migrationRequests); err != nil {
		return fmt.Errorf("error executing migrations: %w", err)
	}

	// Validate Saltbox configuration after announcements and migrations
	if err := validateSaltboxConfig(verbose); err != nil {
		return fmt.Errorf("error validating Saltbox configuration: %w", err)
	}

	return nil
}

// validateSaltboxConfig validates the Saltbox configuration.
func validateSaltboxConfig(verbose bool) error {
	if err := spinners.RunInfoSpinner("Validating Saltbox configuration"); err != nil {
		return err
	}

	// Validate Saltbox configuration
	err := validate.AllSaltboxConfigs(verbose)
	if err != nil {
		return fmt.Errorf("error validating configs: %w", err)
	}

	return nil
}

// updateSaltbox updates the Saltbox repository and configuration.
func updateSaltbox(ctx context.Context, verbose bool, branchReset *bool) error {
	// Check if Saltbox repo exists
	if _, err := os.Stat(constants.SaltboxRepoPath); os.IsNotExist(err) {
		return fmt.Errorf("error: SB_REPO_PATH does not exist or is not a directory")
	}

	// Get Saltbox user
	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	// Clean up old deadsnakes packages on Ubuntu 20.04 and 22.04
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Checking for old deadsnakes Python packages", func() error {
		cleaned, err := python.CleanupDeadsnakesIfNeeded(ctx, verbose)
		if err != nil {
			return err
		}
		if cleaned && verbose {
			fmt.Println("Removed old deadsnakes Python packages")
		}
		return nil
	}); err != nil {
		// Don't fail the update if cleanup fails, just log a warning
		_ = spinners.RunWarningSpinner(fmt.Sprintf("Warning: Failed to clean up deadsnakes packages: %v", err))
	}

	// Manage Ansible venv - this function already has internal spinners
	if err := venv.ManageAnsibleVenv(ctx, false, saltboxUser, verbose); err != nil {
		return fmt.Errorf("error managing Ansible venv: %w", err)
	}

	// Set up custom commands for git
	customCommands := [][]string{
		{
			"cp",
			fmt.Sprintf("%s/defaults/ansible.cfg.default", constants.SaltboxRepoPath),
			fmt.Sprintf("%s/ansible.cfg", constants.SaltboxRepoPath),
		},
	}

	// Get old commit hash
	oldCommitHash, err := git.GetGitCommitHash(constants.SaltboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting old commit hash: %w", err)
	}

	// Fetch and reset git repo - this function already has internal spinners
	if err := git.FetchAndReset(ctx, constants.SaltboxRepoPath, "master", saltboxUser, customCommands, branchReset); err != nil {
		return fmt.Errorf("error fetching and resetting git: %w", err)
	}

	// Download and install Saltbox fact - this function already has internal spinners
	if err := fact.DownloadAndInstallSaltboxFact(false, verbose); err != nil {
		return fmt.Errorf("error downloading and installing saltbox fact: %w", err)
	}

	// Get commit hash after fetch and reset
	newCommitHash, err := git.GetGitCommitHash(constants.SaltboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting new commit hash: %w", err)
	}

	// Update tags cache if commit hash changed
	if oldCommitHash != newCommitHash {
		if err := spinners.RunInfoSpinner("Saltbox Commit Hash changed, updating tags cache."); err != nil {
			return err
		}
		ansibleCache, err := cache.NewCache()
		if err != nil {
			return fmt.Errorf("error creating cache: %w", err)
		}
		if _, err := ansible.RunAndCacheAnsibleTags(ctx, constants.SaltboxRepoPath, constants.SaltboxPlaybookPath(), "", ansibleCache); err != nil {
			handleInterruptError(err)
			return fmt.Errorf("error running and caching ansible tags: %w", err)
		}
	}

	// Final success message
	return spinners.RunInfoSpinner("Saltbox Update Completed")
}

// updateSandbox updates the Sandbox repository and configuration.
func updateSandbox(ctx context.Context, branchReset *bool) error {
	// Check if Sandbox repo exists
	if _, err := os.Stat(constants.SandboxRepoPath); os.IsNotExist(err) {
		return fmt.Errorf("error: %s does not exist or is not a directory", constants.SandboxRepoPath)
	}

	// Get Saltbox user
	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	// Set up custom commands for git
	customCommands := [][]string{
		{
			"cp",
			fmt.Sprintf("%s/defaults/ansible.cfg.default", constants.SandboxRepoPath),
			fmt.Sprintf("%s/ansible.cfg", constants.SandboxRepoPath),
		},
	}

	// Get old commit hash
	oldCommitHash, err := git.GetGitCommitHash(constants.SandboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting old commit hash: %w", err)
	}

	// Fetch and reset git repo - this function already has internal spinners
	if err := git.FetchAndReset(ctx, constants.SandboxRepoPath, "master", saltboxUser, customCommands, branchReset); err != nil {
		return fmt.Errorf("error fetching and resetting git: %w", err)
	}

	// Get commit hash after fetch and reset
	newCommitHash, err := git.GetGitCommitHash(constants.SandboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting new commit hash: %w", err)
	}

	// Update tags cache if commit hash changed
	if oldCommitHash != newCommitHash {
		if err := spinners.RunInfoSpinner("Sandbox Commit Hash changed, updating tags cache."); err != nil {
			return err
		}
		ansibleCache, err := cache.NewCache()
		if err != nil {
			return fmt.Errorf("error creating cache: %w", err)
		}
		if _, err := ansible.RunAndCacheAnsibleTags(ctx, constants.SandboxRepoPath, constants.SandboxPlaybookPath(), "", ansibleCache); err != nil {
			handleInterruptError(err)
			return fmt.Errorf("error running and caching ansible tags: %w", err)
		}
	}

	// Final success message
	return spinners.RunInfoSpinner("Sandbox Update Completed")
}
