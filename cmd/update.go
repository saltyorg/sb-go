package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/saltyorg/sb-go/internal/announcements"
	"github.com/saltyorg/sb-go/internal/ansible"
	"github.com/saltyorg/sb-go/internal/apt"
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

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update Saltbox & Sandbox",
	Long:  `Update Saltbox & Sandbox`,
	Args:  cobra.NoArgs,
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
	updateCmd.MarkFlagsMutuallyExclusive("keep-branch", "reset-branch")
}

func handleUpdate(ctx context.Context, verbose bool, branchReset *bool, skipSelfUpdate bool) error {
	// Check if running in an interactive terminal
	if !tty.IsInteractive() {
		normalStyle := lipgloss.NewStyle()
		return fmt.Errorf("%s", normalStyle.Render("update command requires an interactive terminal (TTY not available)"))
	}

	appDataPath := filepath.Dir(constants.SandboxRepoPath)
	pathsToCheck := []string{"/", appDataPath, "/srv"}
	verbosity := 0
	if verbose {
		verbosity = 1
	}
	if err := utils.CheckDiskSpace(pathsToCheck, verbosity); err != nil {
		return err
	}

	runner := spinners.NewRunner(spinners.RunnerOptions{Verbose: verbose})

	if !skipSelfUpdate {
		updated, err := doSelfUpdate(ctx, runner, true, verbose, "Re-run the update command to update Saltbox", false)
		if err != nil {
			return fmt.Errorf("error during self-update: %w", err)
		}
		if updated {
			// Exit after successful update so the new version can be run
			os.Exit(0)
		}
	}

	// Update apt cache
	if err := runner.Run(ctx, spinners.TaskSpec{Running: "Updating apt package cache"}, func(ctx context.Context, task *spinners.Task) error {
		return task.RunStreaming(ctx, spinners.TaskSpec{Running: "Refreshing apt package lists"}, func(taskCtx context.Context) error {
			updateCache := apt.UpdatePackageLists(taskCtx, verbose)
			return updateCache()
		})
	}); err != nil {
		return fmt.Errorf("error updating apt cache: %w", err)
	}

	// Load announcement files before updates
	saltboxAnnouncementsBefore, sandboxAnnouncementsBefore, err := announcements.LoadAllAnnouncementFiles()
	if err != nil {
		return fmt.Errorf("error loading announcements before update: %w", err)
	}

	// Update repositories
	if err := updateSaltbox(ctx, runner, verbose, branchReset); err != nil {
		return fmt.Errorf("error updating Saltbox: %w", err)
	}
	if err := updateSandbox(ctx, runner, branchReset); err != nil {
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
	if err := announcements.DisplayAnnouncements(runner, announcementDiffs); err != nil {
		return fmt.Errorf("error displaying announcements: %w", err)
	}

	// Prompt for migration approvals and execute
	migrationRequests, err := announcements.PromptForMigrations(announcementDiffs)
	if err != nil {
		return fmt.Errorf("error prompting for migrations: %w", err)
	}

	// Execute migration requests with context
	if err := announcements.ExecuteMigrations(ctx, runner, migrationRequests); err != nil {
		return fmt.Errorf("error executing migrations: %w", err)
	}

	// Validate Saltbox configuration after announcements and migrations
	if err := validateSaltboxConfig(ctx, runner, verbose); err != nil {
		return fmt.Errorf("error validating Saltbox configuration: %w", err)
	}

	// Regenerate shell completions if they're installed
	regenerateInstalledCompletions()

	return nil
}

// validateSaltboxConfig validates the Saltbox configuration.
func validateSaltboxConfig(ctx context.Context, runner *spinners.Runner, verbose bool) error {
	err := runner.Run(ctx, spinners.TaskSpec{
		Running: "Validating Saltbox configuration",
	}, func(ctx context.Context, task *spinners.Task) error {
		return validate.AllSaltboxConfigs(ctx, task, verbose)
	})
	if err != nil {
		return fmt.Errorf("error validating configs: %w", err)
	}

	return nil
}

// updateSaltbox updates the Saltbox repository and configuration.
func updateSaltbox(ctx context.Context, runner *spinners.Runner, verbose bool, branchReset *bool) error {
	if err := requireDirectory(constants.SaltboxRepoPath); err != nil {
		return err
	}
	branch, err := git.ResolveUpdateBranch(ctx, runner, constants.SaltboxRepoPath, "master", branchReset, "Saltbox")
	if err != nil {
		return err
	}
	return runner.Run(ctx, spinners.TaskSpec{
		Running: "Updating Saltbox",
		Success: "Saltbox updated",
		Failure: "Saltbox update",
	}, func(ctx context.Context, task *spinners.Task) error {
		return updateSaltboxComponents(ctx, task, verbose, branch)
	})
}

func updateSaltboxComponents(ctx context.Context, task *spinners.Task, verbose bool, branch string) error {
	// Check if Saltbox repo exists
	if err := requireDirectory(constants.SaltboxRepoPath); err != nil {
		return err
	}

	// Get Saltbox user
	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	// Clean up old deadsnakes packages on Ubuntu 20.04 and 22.04
	if err := task.RunStreaming(ctx, spinners.TaskSpec{Running: "Checking for old deadsnakes Python packages"}, func(taskCtx context.Context) error {
		cleaned, err := python.CleanupDeadsnakesIfNeeded(taskCtx, verbose)
		if err != nil {
			return err
		}
		if cleaned && verbose {
			fmt.Println("Removed old deadsnakes Python packages")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error cleaning up deadsnakes packages: %w", err)
	}

	// Ensure uv is installed
	if err := task.RunStreaming(ctx, spinners.TaskSpec{Running: "Ensuring uv is installed"}, func(taskCtx context.Context) error {
		return uv.DownloadAndInstallUV(taskCtx, verbose)
	}); err != nil {
		return fmt.Errorf("error installing uv: %w", err)
	}

	// Ensure Python install directory exists
	if err := task.Run(ctx, spinners.TaskSpec{Running: "Ensuring Python install directory exists"}, func(context.Context, *spinners.Task) error {
		return os.MkdirAll(constants.PythonInstallDir, 0755)
	}); err != nil {
		return fmt.Errorf("error creating Python install directory: %w", err)
	}

	// Ensure Python is installed via uv
	if err := task.RunStreaming(ctx, spinners.TaskSpec{Running: fmt.Sprintf("Ensuring Python %s is installed", constants.AnsibleVenvPythonVersion)}, func(taskCtx context.Context) error {
		return uv.InstallPython(taskCtx, constants.AnsibleVenvPythonVersion, verbose)
	}); err != nil {
		return fmt.Errorf("error installing Python %s: %w", constants.AnsibleVenvPythonVersion, err)
	}

	// Get old commit hash
	oldCommitHash, err := git.GetGitCommitHash(ctx, constants.SaltboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting old commit hash: %w", err)
	}

	// Fetch and reset git repo - this function already has internal spinners
	if err := task.Run(ctx, spinners.TaskSpec{
		Running:      "Updating Saltbox repository",
		Success:      fmt.Sprintf("Saltbox repository updated (%s)", branch),
		Failure:      "Saltbox repository update",
		ChildDisplay: spinners.CollapseChildTasks,
	}, func(ctx context.Context, gitTask *spinners.Task) error {
		return git.FetchAndResetBranch(ctx, gitTask, constants.SaltboxRepoPath, branch, saltboxUser, nil, "Saltbox")
	}); err != nil {
		return fmt.Errorf("error fetching and resetting git: %w", err)
	}

	// Manage Ansible venv - this function already has internal spinners
	if err := task.Run(ctx, spinners.TaskSpec{
		Running:      "Preparing Ansible virtual environment",
		Success:      "Ansible virtual environment ready",
		Failure:      "Ansible virtual environment",
		ChildDisplay: spinners.CollapseChildTasks,
	}, func(ctx context.Context, venvTask *spinners.Task) error {
		return venv.ManageAnsibleVenv(ctx, venvTask, false, saltboxUser, verbose)
	}); err != nil {
		return fmt.Errorf("error managing Ansible venv: %w", err)
	}

	// Download and install Saltbox fact - this function already has internal spinners
	if err := task.Run(ctx, spinners.TaskSpec{
		Running:      "Checking saltbox.fact",
		Success:      "saltbox.fact is ready",
		Failure:      "saltbox.fact update",
		ChildDisplay: spinners.CollapseChildTasks,
	}, func(ctx context.Context, factTask *spinners.Task) error {
		return fact.DownloadAndInstallSaltboxFact(ctx, factTask, false, verbose)
	}); err != nil {
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
	saltboxTagsExist := saltboxCacheExists && saltboxCache["tags"] != nil

	if oldCommitHash != newCommitHash || !saltboxCacheExists || !saltboxTagsExist {
		if err := task.Run(ctx, spinners.TaskSpec{Running: "Updating Saltbox tags cache"}, func(context.Context, *spinners.Task) error {
			if _, err := ansible.RunAndCacheAnsibleTags(ctx, constants.SaltboxRepoPath, constants.SaltboxPlaybookPath(), "", ansibleCache, 0); err != nil {
				handleInterruptError(err)
				return fmt.Errorf("error running and caching ansible tags: %w", err)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

// updateSandbox updates the Sandbox repository and configuration.
func updateSandbox(ctx context.Context, runner *spinners.Runner, branchReset *bool) error {
	if err := requireDirectory(constants.SandboxRepoPath); err != nil {
		return err
	}
	branch, err := git.ResolveUpdateBranch(ctx, runner, constants.SandboxRepoPath, "master", branchReset, "Sandbox")
	if err != nil {
		return err
	}
	return runner.Run(ctx, spinners.TaskSpec{
		Running: "Updating Sandbox",
		Success: "Sandbox updated",
		Failure: "Sandbox update",
	}, func(ctx context.Context, task *spinners.Task) error {
		return updateSandboxComponents(ctx, task, branch)
	})
}

func updateSandboxComponents(ctx context.Context, task *spinners.Task, branch string) error {
	// Check if Sandbox repo exists
	if err := requireDirectory(constants.SandboxRepoPath); err != nil {
		return err
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
	if err := task.Run(ctx, spinners.TaskSpec{
		Running:      "Updating Sandbox repository",
		Success:      fmt.Sprintf("Sandbox repository updated (%s)", branch),
		Failure:      "Sandbox repository update",
		ChildDisplay: spinners.CollapseChildTasks,
	}, func(ctx context.Context, gitTask *spinners.Task) error {
		return git.FetchAndResetBranch(ctx, gitTask, constants.SandboxRepoPath, branch, saltboxUser, nil, "Sandbox")
	}); err != nil {
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
	sandboxTagsExist := sandboxCacheExists && sandboxCache["tags"] != nil

	if oldCommitHash != newCommitHash || !sandboxCacheExists || !sandboxTagsExist {
		if err := task.Run(ctx, spinners.TaskSpec{Running: "Updating Sandbox tags cache"}, func(context.Context, *spinners.Task) error {
			if _, err := ansible.RunAndCacheAnsibleTags(ctx, constants.SandboxRepoPath, constants.SandboxPlaybookPath(), "", ansibleCache, 0); err != nil {
				handleInterruptError(err)
				return fmt.Errorf("error running and caching ansible tags: %w", err)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

func requireDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect required directory %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s exists but is not a directory", path)
	}
	return nil
}

// regenerateInstalledCompletions auto-installs or regenerates shell completion files
func regenerateInstalledCompletions() {
	// Install/regenerate completion for all names (binary + symlinks)
	for _, cmdName := range getAllBinaryNames() {
		bashPath := fmt.Sprintf("/etc/bash_completion.d/%s", cmdName)
		_ = InstallOrRegenerateCompletion("bash", bashPath, func(path string) error {
			return generateStaticBashCompletion(path, cmdName)
		})

		// Only install or regenerate zsh completion if zsh is installed
		if isZshInstalled() {
			zshPath := fmt.Sprintf("/usr/share/zsh/vendor-completions/_%s", cmdName)
			_ = InstallOrRegenerateCompletion("zsh", zshPath, func(path string) error {
				return generateStaticZshCompletion(path, cmdName)
			})
		}
	}

	// Silent execution - errors are ignored
	// To enable verbose output in the future, uncomment the print statements in InstallOrRegenerateCompletion()
}
