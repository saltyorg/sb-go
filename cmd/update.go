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
	"github.com/saltyorg/sb-go/internal/tty"
	"github.com/saltyorg/sb-go/internal/utils"
	"github.com/saltyorg/sb-go/internal/uv"
	"github.com/saltyorg/sb-go/internal/validate"
	"github.com/saltyorg/sb-go/internal/venv"

	"github.com/charmbracelet/lipgloss"
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
		skipSelfUpdate, _ := cmd.Flags().GetBool("skip-self-update")

		var branchReset *bool
		if keepBranch {
			falseVal := false
			branchReset = &falseVal
		} else if resetBranch {
			trueVal := true
			branchReset = &trueVal
		}

		return handleUpdate(ctx, verbose, branchReset, skipSelfUpdate)
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	updateCmd.PersistentFlags().Bool("keep-branch", false, "Skip branch reset prompt and stay on current branch")
	updateCmd.PersistentFlags().Bool("reset-branch", false, "Skip branch reset prompt and reset to default branch")
	updateCmd.PersistentFlags().Bool("skip-self-update", false, "Skip CLI self-update check")
}

func handleUpdate(ctx context.Context, verbose bool, branchReset *bool, skipSelfUpdate bool) error {
	// Check if running in an interactive terminal
	if !tty.IsInteractive() {
		normalStyle := lipgloss.NewStyle()
		return fmt.Errorf("%s", normalStyle.Render("update command requires an interactive terminal (TTY not available)"))
	}

	// Set verbose mode for spinners
	spinners.SetVerboseMode(verbose)

	if !skipSelfUpdate {
		updated, err := doSelfUpdate(true, verbose, "Re-run the update command to update Saltbox", false)
		if err != nil {
			return fmt.Errorf("error during self-update: %w", err)
		}
		if updated {
			// Exit after successful update so the new version can be run
			os.Exit(0)
		}
	}

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

	// Regenerate shell completions if they're installed
	regenerateInstalledCompletions()

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
		normalStyle := lipgloss.NewStyle()
		return fmt.Errorf("%s", normalStyle.Render("error: SB_REPO_PATH does not exist or is not a directory"))
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

	// Ensure uv is installed
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Ensuring uv is installed", func() error {
		return uv.DownloadAndInstallUV(ctx, verbose)
	}); err != nil {
		return fmt.Errorf("error installing uv: %w", err)
	}

	// Ensure Python install directory exists
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Ensuring Python install directory exists", func() error {
		return os.MkdirAll(constants.PythonInstallDir, 0755)
	}); err != nil {
		return fmt.Errorf("error creating Python install directory: %w", err)
	}

	// Ensure Python is installed via uv
	if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Ensuring Python %s is installed", constants.AnsibleVenvPythonVersion), func() error {
		return uv.InstallPython(ctx, constants.AnsibleVenvPythonVersion, verbose)
	}); err != nil {
		return fmt.Errorf("error installing Python %s: %w", constants.AnsibleVenvPythonVersion, err)
	}

	// Get old commit hash
	oldCommitHash, err := git.GetGitCommitHash(ctx, constants.SaltboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting old commit hash: %w", err)
	}

	// Fetch and reset git repo - this function already has internal spinners
	if err := git.FetchAndReset(ctx, constants.SaltboxRepoPath, "master", saltboxUser, nil, branchReset, "Saltbox"); err != nil {
		return fmt.Errorf("error fetching and resetting git: %w", err)
	}

	// Manage Ansible venv - this function already has internal spinners
	if err := venv.ManageAnsibleVenv(ctx, false, saltboxUser, verbose); err != nil {
		return fmt.Errorf("error managing Ansible venv: %w", err)
	}

	// Download and install Saltbox fact - this function already has internal spinners
	if err := fact.DownloadAndInstallSaltboxFact(false, verbose); err != nil {
		return fmt.Errorf("error downloading and installing saltbox fact: %w", err)
	}

	// Get commit hash after fetch and reset
	newCommitHash, err := git.GetGitCommitHash(ctx, constants.SaltboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting new commit hash: %w", err)
	}

	// Update tags cache if commit hash changed or cache is missing
	ansibleCache, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("error creating cache: %w", err)
	}

	saltboxCache, saltboxCacheExists := ansibleCache.GetRepoCache(constants.SaltboxRepoPath)
	_, saltboxTagsExist := saltboxCache["tags"]

	if oldCommitHash != newCommitHash || !saltboxCacheExists || !saltboxTagsExist {
		if err := spinners.RunInfoSpinner("Updating Saltbox tags cache."); err != nil {
			return err
		}
		if _, err := ansible.RunAndCacheAnsibleTags(ctx, constants.SaltboxRepoPath, constants.SaltboxPlaybookPath(), "", ansibleCache, 0); err != nil {
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
		normalStyle := lipgloss.NewStyle()
		return fmt.Errorf("%s", normalStyle.Render(fmt.Sprintf("error: %s does not exist or is not a directory", constants.SandboxRepoPath)))
	}

	// Get Saltbox user
	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	// Get old commit hash
	oldCommitHash, err := git.GetGitCommitHash(ctx, constants.SandboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting old commit hash: %w", err)
	}

	// Fetch and reset git repo - this function already has internal spinners
	if err := git.FetchAndReset(ctx, constants.SandboxRepoPath, "master", saltboxUser, nil, branchReset, "Sandbox"); err != nil {
		return fmt.Errorf("error fetching and resetting git: %w", err)
	}

	// Get commit hash after fetch and reset
	newCommitHash, err := git.GetGitCommitHash(ctx, constants.SandboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting new commit hash: %w", err)
	}

	// Update tags cache if commit hash changed or cache is missing
	ansibleCache, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("error creating cache: %w", err)
	}

	sandboxCache, sandboxCacheExists := ansibleCache.GetRepoCache(constants.SandboxRepoPath)
	_, sandboxTagsExist := sandboxCache["tags"]

	if oldCommitHash != newCommitHash || !sandboxCacheExists || !sandboxTagsExist {
		if err := spinners.RunInfoSpinner("Updating Sandbox tags cache."); err != nil {
			return err
		}
		if _, err := ansible.RunAndCacheAnsibleTags(ctx, constants.SandboxRepoPath, constants.SandboxPlaybookPath(), "", ansibleCache, 0); err != nil {
			handleInterruptError(err)
			return fmt.Errorf("error running and caching ansible tags: %w", err)
		}
	}

	// Final success message
	return spinners.RunInfoSpinner("Sandbox Update Completed")
}

// regenerateInstalledCompletions auto-installs or regenerates shell completion files
func regenerateInstalledCompletions() {
	bashPath, zshPath := getCompletionPaths()
	cmdName := getBinaryName()

	// Always install or regenerate bash completion for current binary name
	_ = InstallOrRegenerateCompletion("bash", bashPath, func(path string) error {
		return generateStaticBashCompletion(path, cmdName)
	})

	// Only install or regenerate zsh completion if zsh is installed
	if isZshInstalled() {
		_ = InstallOrRegenerateCompletion("zsh", zshPath, func(path string) error {
			return generateStaticZshCompletion(path, cmdName)
		})
	}

	// Silent execution - errors are ignored
	// To enable verbose output in the future, uncomment the print statements in InstallOrRegenerateCompletion()
}
