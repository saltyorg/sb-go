package cmd

import (
	"context"
	"fmt"

	"github.com/saltyorg/sb-go/internal/ansible"
	"github.com/saltyorg/sb-go/internal/cache"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/fact"
	"github.com/saltyorg/sb-go/internal/git"
	"github.com/saltyorg/sb-go/internal/spinners"
	"github.com/saltyorg/sb-go/internal/utils"
	"github.com/saltyorg/sb-go/internal/venv"

	"github.com/spf13/cobra"
)

// branchCmd represents the branch command
var branchCmd = &cobra.Command{
	Use:   "branch [branch_name]",
	Short: "Change the branch used by Saltbox",
	Long:  `Change the branch used by Saltbox`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		branchName := args[0]
		return changeBranch(ctx, branchName)
	},
}

func init() {
	rootCmd.AddCommand(branchCmd)
}

func changeBranch(ctx context.Context, branchName string) error {
	runner := spinners.NewRunner(spinners.RunnerOptions{})

	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return err
	}

	if err := git.EnsureRemoteFetchAllBranches(ctx, constants.SaltboxRepoPath); err != nil {
		return err
	}

	selectedBranch, err := git.ResolveUpdateBranch(ctx, runner, constants.SaltboxRepoPath, branchName, nil, "Saltbox")
	if err != nil {
		return err
	}

	return runner.Run(ctx, spinners.TaskSpec{
		Running: fmt.Sprintf("Switching Saltbox repository to %s", selectedBranch),
		Success: fmt.Sprintf("Saltbox repository switched to %s", selectedBranch),
		Failure: "Saltbox branch switch",
	}, func(ctx context.Context, task *spinners.Task) error {
		if err := task.Run(ctx, spinners.TaskSpec{
			Running:      "Updating Saltbox repository",
			Success:      fmt.Sprintf("Saltbox repository updated (%s)", selectedBranch),
			Failure:      "Saltbox repository update",
			ChildDisplay: spinners.CollapseChildTasks,
		}, func(ctx context.Context, gitTask *spinners.Task) error {
			return git.FetchAndResetBranch(ctx, gitTask, constants.SaltboxRepoPath, selectedBranch, saltboxUser, nil, "Saltbox")
		}); err != nil {
			return err
		}

		if err := task.Run(ctx, spinners.TaskSpec{
			Running:      "Checking saltbox.fact",
			Success:      "saltbox.fact is ready",
			Failure:      "saltbox.fact update",
			ChildDisplay: spinners.CollapseChildTasks,
		}, func(ctx context.Context, factTask *spinners.Task) error {
			return fact.DownloadAndInstallSaltboxFact(ctx, factTask, false, false)
		}); err != nil {
			return err
		}

		if err := task.Run(ctx, spinners.TaskSpec{
			Running:      "Preparing Ansible virtual environment",
			Success:      "Ansible virtual environment ready",
			Failure:      "Ansible virtual environment",
			ChildDisplay: spinners.CollapseChildTasks,
		}, func(ctx context.Context, venvTask *spinners.Task) error {
			return venv.ManageAnsibleVenv(ctx, venvTask, false, saltboxUser, false)
		}); err != nil {
			return fmt.Errorf("error managing Ansible venv: %w", err)
		}

		cacheInstance, err := cache.NewCache()
		if err != nil {
			return fmt.Errorf("error creating cache: %w", err)
		}

		return task.Run(ctx, spinners.TaskSpec{Running: "Updating Saltbox tags cache"}, func(context.Context, *spinners.Task) error {
			_, err := ansible.RunAndCacheAnsibleTags(ctx, constants.SaltboxRepoPath, constants.SaltboxPlaybookPath(), "", cacheInstance, 0)
			return err
		})
	})
}
