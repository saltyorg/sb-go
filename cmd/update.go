package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/saltyorg/sb-go/ansible"
	"github.com/saltyorg/sb-go/buildinfo"
	"github.com/saltyorg/sb-go/git"
	"github.com/saltyorg/sb-go/host"
	"github.com/saltyorg/sb-go/layout"
	"github.com/saltyorg/sb-go/python"
	"github.com/saltyorg/sb-go/saltbox"
	"github.com/saltyorg/sb-go/signals"
	"github.com/saltyorg/sb-go/terminal"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

func newUpdateCommand() *cobra.Command {
	opts := struct {
		verbose, keepBranch, resetBranch, skipSelfUpdate bool
	}{}
	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Update Saltbox & Sandbox",
		Long:  `Update Saltbox & Sandbox`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			var branchReset *bool
			if opts.keepBranch {
				falseVal := false
				branchReset = &falseVal
			} else if opts.resetBranch {
				trueVal := true
				branchReset = &trueVal
			}

			return handleUpdate(ctx, cmd.Root(), opts.verbose, branchReset, opts.skipSelfUpdate)
		},
	}
	updateCmd.PersistentFlags().BoolVarP(&opts.verbose, "verbose", "v", false, "Enable verbose output")
	updateCmd.PersistentFlags().BoolVar(&opts.keepBranch, "keep-branch", false, "Skip branch reset prompt and stay on current branch")
	updateCmd.PersistentFlags().BoolVar(&opts.resetBranch, "reset-branch", false, "Skip branch reset prompt and reset to default branch")
	updateCmd.PersistentFlags().BoolVar(&opts.skipSelfUpdate, "skip-self-update", false, "Skip CLI self-update check")
	updateCmd.MarkFlagsMutuallyExclusive("keep-branch", "reset-branch")
	return updateCmd
}

func addUpdateCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newUpdateCommand())
}

func handleUpdate(ctx context.Context, rootCmd *cobra.Command, verbose bool, branchReset *bool, skipSelfUpdate bool) error {
	// Check if running in an interactive terminal
	if !terminal.IsInteractive() {
		normalStyle := lipgloss.NewStyle()
		return fmt.Errorf("%s", normalStyle.Render("update command requires an interactive terminal (TTY not available)"))
	}

	appDataPath := filepath.Dir(layout.Current().SandboxRepoPath)
	pathsToCheck := []string{"/", appDataPath, "/srv"}
	verbosity := 0
	if verbose {
		verbosity = 1
	}
	if err := host.CheckDiskSpace(pathsToCheck, verbosity); err != nil {
		return err
	}

	runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: verbose})

	if !skipSelfUpdate {
		updated, err := doSelfUpdate(ctx, runner, buildinfo.Current(), true, verbose, "Re-run the update command to update Saltbox", false)
		if err != nil {
			return fmt.Errorf("error during self-update: %w", err)
		}
		if updated {
			// Request a clean process exit after successful replacement so the
			// current invocation cannot continue with the old in-memory code.
			signals.Shutdown(ctx, 0)
			return nil
		}
	}

	// Update apt cache
	if err := runner.Run(ctx, terminal.TaskSpec{Running: "Updating apt package cache"}, func(ctx context.Context, task *terminal.Task) error {
		return task.RunStreaming(ctx, terminal.TaskSpec{Running: "Refreshing apt package lists"}, func(taskCtx context.Context) error {
			updateCache := host.UpdatePackageLists(taskCtx, verbose)
			return updateCache()
		})
	}); err != nil {
		return fmt.Errorf("error updating apt cache: %w", err)
	}

	// Load announcement files before updates
	saltboxAnnouncementsBefore, sandboxAnnouncementsBefore, err := saltbox.LoadAllAnnouncementFiles()
	if err != nil {
		return fmt.Errorf("error loading announcements before update: %w", err)
	}

	// Update repositories
	if err := updateSaltbox(ctx, runner, verbose, branchReset); err != nil {
		return fmt.Errorf("error updating Saltbox: %w", err)
	}
	if err := runner.Run(ctx, terminal.TaskSpec{
		Running: "Securing Saltbox configuration permissions",
		Success: "Saltbox configuration permissions secured",
		Failure: "Saltbox configuration permission migration",
	}, func(context.Context, *terminal.Task) error {
		return saltbox.SecureExistingConfigFiles()
	}); err != nil {
		return fmt.Errorf("error securing Saltbox configuration permissions: %w", err)
	}
	if err := updateSandbox(ctx, runner, branchReset); err != nil {
		return fmt.Errorf("error updating Sandbox: %w", err)
	}

	// Load announcement files after updates
	saltboxAnnouncementsAfter, sandboxAnnouncementsAfter, err := saltbox.LoadAllAnnouncementFiles()
	if err != nil {
		return fmt.Errorf("error loading announcements after update: %w", err)
	}

	// Check for new announcements in both repositories
	announcementDiffs := saltbox.CheckForNewAnnouncementsAllRepos(saltboxAnnouncementsBefore, saltboxAnnouncementsAfter, sandboxAnnouncementsBefore, sandboxAnnouncementsAfter)

	// Display new announcements
	if err := saltbox.DisplayAnnouncements(runner, announcementDiffs); err != nil {
		return fmt.Errorf("error displaying announcements: %w", err)
	}

	// Prompt for migration approvals and execute
	migrationRequests, err := saltbox.PromptForMigrations(announcementDiffs)
	if err != nil {
		return fmt.Errorf("error prompting for migrations: %w", err)
	}

	// Execute migration requests with context
	if err := saltbox.ExecuteMigrations(ctx, runner, migrationRequests); err != nil {
		return fmt.Errorf("error executing migrations: %w", err)
	}

	// Validate Saltbox configuration after announcements and migrations
	if err := validateSaltboxConfig(ctx, runner, verbose); err != nil {
		return fmt.Errorf("error validating Saltbox configuration: %w", err)
	}

	// Regenerate shell completions if they're installed
	regenerateInstalledCompletions(rootCmd)

	return nil
}

// validateSaltboxConfig validates the Saltbox configuration.
func validateSaltboxConfig(ctx context.Context, runner *terminal.Runner, verbose bool) error {
	err := runner.Run(ctx, terminal.TaskSpec{
		Running: "Validating Saltbox configuration",
	}, func(ctx context.Context, task *terminal.Task) error {
		return saltbox.AllSaltboxConfigs(ctx, task, verbose)
	})
	if err != nil {
		return fmt.Errorf("error validating configs: %w", err)
	}

	return nil
}

// updateSaltbox updates the Saltbox repository and configuration.
func updateSaltbox(ctx context.Context, runner *terminal.Runner, verbose bool, branchReset *bool) error {
	if err := requireDirectory(layout.SaltboxRepoPath); err != nil {
		return err
	}
	branch, err := git.ResolveUpdateBranch(ctx, runner, layout.SaltboxRepoPath, "master", branchReset, "Saltbox")
	if err != nil {
		return err
	}
	return runner.Run(ctx, terminal.TaskSpec{
		Running: "Updating Saltbox",
		Success: "Saltbox updated",
		Failure: "Saltbox update",
	}, func(ctx context.Context, task *terminal.Task) error {
		return updateSaltboxComponents(ctx, task, verbose, branch)
	})
}

func updateSaltboxComponents(ctx context.Context, task *terminal.Task, verbose bool, branch string) error {
	// Check if Saltbox repo exists
	if err := requireDirectory(layout.SaltboxRepoPath); err != nil {
		return err
	}

	// Get Saltbox user
	saltboxUser, err := host.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	// Get old commit hash
	oldCommitHash, err := git.GetGitCommitHash(ctx, layout.SaltboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting old commit hash: %w", err)
	}

	// Fetch and reset git repo - this function already has internal spinners
	if err := task.Run(ctx, terminal.TaskSpec{
		Running:      "Updating Saltbox repository",
		Success:      fmt.Sprintf("Saltbox repository updated (%s)", branch),
		Failure:      "Saltbox repository update",
		ChildDisplay: terminal.CollapseChildTasks,
	}, func(ctx context.Context, gitTask *terminal.Task) error {
		return git.FetchAndResetBranch(ctx, gitTask, layout.SaltboxRepoPath, branch, saltboxUser, nil, "Saltbox")
	}); err != nil {
		return fmt.Errorf("error fetching and resetting git: %w", err)
	}

	if err := task.Run(ctx, terminal.TaskSpec{Running: "Checking for old deadsnakes Python packages"}, func(taskCtx context.Context, _ *terminal.Task) error {
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

	// Manage Ansible venv - this function already has internal spinners
	if err := task.Run(ctx, terminal.TaskSpec{
		Running:      "Preparing Ansible virtual environment",
		Success:      "Ansible virtual environment ready",
		Failure:      "Ansible virtual environment",
		ChildDisplay: terminal.CollapseChildTasks,
	}, func(ctx context.Context, venvTask *terminal.Task) error {
		return python.ManageAnsibleVenv(ctx, venvTask, false, saltboxUser, verbose)
	}); err != nil {
		return fmt.Errorf("error managing Ansible venv: %w", err)
	}

	// Download and install Saltbox fact - this function already has internal spinners
	if err := task.Run(ctx, terminal.TaskSpec{
		Running:      "Checking saltbox.fact",
		Success:      "saltbox.fact is ready",
		Failure:      "saltbox.fact update",
		ChildDisplay: terminal.CollapseChildTasks,
	}, func(ctx context.Context, factTask *terminal.Task) error {
		return saltbox.DownloadAndInstallSaltboxFact(ctx, factTask, false, verbose)
	}); err != nil {
		return fmt.Errorf("error downloading and installing saltbox fact: %w", err)
	}

	// Get commit hash after fetch and reset
	newCommitHash, err := git.GetGitCommitHash(ctx, layout.SaltboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting new commit hash: %w", err)
	}

	// Update tags cache if commit hash changed or cache is missing
	ansibleCache, err := ansible.NewCache()
	if err != nil {
		return fmt.Errorf("error creating cache: %w", err)
	}

	saltboxCache, saltboxCacheExists := ansibleCache.GetRepoCache(layout.SaltboxRepoPath)
	saltboxTagsExist := saltboxCacheExists && saltboxCache["tags"] != nil

	if oldCommitHash != newCommitHash || !saltboxCacheExists || !saltboxTagsExist {
		if err := task.Run(ctx, terminal.TaskSpec{Running: "Updating Saltbox tags cache"}, func(context.Context, *terminal.Task) error {
			if _, err := ansible.RunAndCacheAnsibleTags(ctx, layout.SaltboxRepoPath, layout.SaltboxPlaybookPath(), "", ansibleCache, 0); err != nil {
				handleInterruptError(ctx, err)
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
func updateSandbox(ctx context.Context, runner *terminal.Runner, branchReset *bool) error {
	if err := requireDirectory(layout.Current().SandboxRepoPath); err != nil {
		return err
	}
	branch, err := git.ResolveUpdateBranch(ctx, runner, layout.Current().SandboxRepoPath, "master", branchReset, "Sandbox")
	if err != nil {
		return err
	}
	return runner.Run(ctx, terminal.TaskSpec{
		Running: "Updating Sandbox",
		Success: "Sandbox updated",
		Failure: "Sandbox update",
	}, func(ctx context.Context, task *terminal.Task) error {
		return updateSandboxComponents(ctx, task, branch)
	})
}

func updateSandboxComponents(ctx context.Context, task *terminal.Task, branch string) error {
	// Check if Sandbox repo exists
	if err := requireDirectory(layout.Current().SandboxRepoPath); err != nil {
		return err
	}

	// Get Saltbox user
	saltboxUser, err := host.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	// Get old commit hash
	oldCommitHash, err := git.GetGitCommitHash(ctx, layout.Current().SandboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting old commit hash: %w", err)
	}

	// Fetch and reset git repo - this function already has internal spinners
	if err := task.Run(ctx, terminal.TaskSpec{
		Running:      "Updating Sandbox repository",
		Success:      fmt.Sprintf("Sandbox repository updated (%s)", branch),
		Failure:      "Sandbox repository update",
		ChildDisplay: terminal.CollapseChildTasks,
	}, func(ctx context.Context, gitTask *terminal.Task) error {
		return git.FetchAndResetBranch(ctx, gitTask, layout.Current().SandboxRepoPath, branch, saltboxUser, nil, "Sandbox")
	}); err != nil {
		return fmt.Errorf("error fetching and resetting git: %w", err)
	}

	// Get commit hash after fetch and reset
	newCommitHash, err := git.GetGitCommitHash(ctx, layout.Current().SandboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting new commit hash: %w", err)
	}

	// Update tags cache if commit hash changed or cache is missing
	ansibleCache, err := ansible.NewCache()
	if err != nil {
		return fmt.Errorf("error creating cache: %w", err)
	}

	sandboxCache, sandboxCacheExists := ansibleCache.GetRepoCache(layout.Current().SandboxRepoPath)
	sandboxTagsExist := sandboxCacheExists && sandboxCache["tags"] != nil

	if oldCommitHash != newCommitHash || !sandboxCacheExists || !sandboxTagsExist {
		if err := task.Run(ctx, terminal.TaskSpec{Running: "Updating Sandbox tags cache"}, func(context.Context, *terminal.Task) error {
			if _, err := ansible.RunAndCacheAnsibleTags(ctx, layout.Current().SandboxRepoPath, layout.SandboxPlaybookPath(), "", ansibleCache, 0); err != nil {
				handleInterruptError(ctx, err)
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
func regenerateInstalledCompletions(rootCmd *cobra.Command) {
	// Install/regenerate completion for all names (binary + symlinks)
	for _, cmdName := range getAllBinaryNames() {
		bashPath := fmt.Sprintf("/etc/bash_completion.d/%s", cmdName)
		_ = InstallOrRegenerateCompletion(bashPath, func(path string) error {
			return generateStaticBashCompletion(rootCmd, path, cmdName)
		})

		// Only install or regenerate zsh completion if zsh is installed
		if isZshInstalled() {
			zshPath := fmt.Sprintf("/usr/share/zsh/vendor-completions/_%s", cmdName)
			_ = InstallOrRegenerateCompletion(zshPath, func(path string) error {
				return generateStaticZshCompletion(rootCmd, path, cmdName)
			})
		}
	}

	// Silent execution - errors are ignored
}
