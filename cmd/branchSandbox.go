package cmd

import (
	"context"
	"fmt"

	"github.com/saltyorg/sb-go/internal/ansible"
	"github.com/saltyorg/sb-go/internal/cache"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/git"
	"github.com/saltyorg/sb-go/internal/spinners"
	"github.com/saltyorg/sb-go/internal/utils"

	"github.com/spf13/cobra"
)

var branchSandboxCmd = &cobra.Command{
	Use:   "branch-sandbox [branch_name]",
	Short: "Change the branch used by Sandbox",
	Long:  `Change the branch used by Sandbox`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		branchName := args[0]
		return changeSandboxBranch(ctx, branchName)
	},
}

func init() {
	rootCmd.AddCommand(branchSandboxCmd)
}

func changeSandboxBranch(ctx context.Context, branchName string) error {
	runner := spinners.NewRunner(spinners.RunnerOptions{})

	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return err
	}

	if err := git.EnsureRemoteFetchAllBranches(ctx, constants.SandboxRepoPath); err != nil {
		return err
	}

	selectedBranch, err := git.ResolveUpdateBranch(ctx, runner, constants.SandboxRepoPath, branchName, nil, "Sandbox")
	if err != nil {
		return err
	}

	return runner.Run(ctx, spinners.TaskSpec{
		Running: fmt.Sprintf("Switching Sandbox repository to %s", selectedBranch),
		Success: fmt.Sprintf("Sandbox repository switched to %s", selectedBranch),
		Failure: "Sandbox branch switch",
	}, func(ctx context.Context, task *spinners.Task) error {
		if err := task.Run(ctx, spinners.TaskSpec{
			Running:      "Updating Sandbox repository",
			Success:      fmt.Sprintf("Sandbox repository updated (%s)", selectedBranch),
			Failure:      "Sandbox repository update",
			ChildDisplay: spinners.CollapseChildTasks,
		}, func(ctx context.Context, gitTask *spinners.Task) error {
			return git.FetchAndResetBranch(ctx, gitTask, constants.SandboxRepoPath, selectedBranch, saltboxUser, nil, "Sandbox")
		}); err != nil {
			return err
		}

		cacheInstance, err := cache.NewCache()
		if err != nil {
			return fmt.Errorf("error creating cache: %w", err)
		}

		return task.Run(ctx, spinners.TaskSpec{Running: "Updating Sandbox tags cache"}, func(context.Context, *spinners.Task) error {
			_, err := ansible.RunAndCacheAnsibleTags(ctx, constants.SandboxRepoPath, constants.SandboxPlaybookPath(), "", cacheInstance, 0)
			return err
		})
	})
}
