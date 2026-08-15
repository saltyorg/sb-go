package cmd

import (
	"context"
	"fmt"

	"github.com/saltyorg/sb-go/ansible"
	"github.com/saltyorg/sb-go/git"
	"github.com/saltyorg/sb-go/host"
	"github.com/saltyorg/sb-go/layout"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/spf13/cobra"
)

func newBranchSandboxCommand() *cobra.Command {
	return &cobra.Command{
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
}

func addBranchSandboxCommand(rootCmd *cobra.Command) {
	branchSandboxCmd := newBranchSandboxCommand()
	rootCmd.AddCommand(branchSandboxCmd)
}

func changeSandboxBranch(ctx context.Context, branchName string) error {
	runner := terminal.NewRunner(terminal.RunnerOptions{})

	saltboxUser, err := host.GetSaltboxUser()
	if err != nil {
		return err
	}

	if err := git.EnsureRemoteFetchAllBranches(ctx, layout.Current().SandboxRepoPath); err != nil {
		return err
	}

	selectedBranch, err := git.ResolveUpdateBranch(ctx, runner, layout.Current().SandboxRepoPath, branchName, nil, "Sandbox")
	if err != nil {
		return err
	}

	return runner.Run(ctx, terminal.TaskSpec{
		Running: fmt.Sprintf("Switching Sandbox repository to %s", selectedBranch),
		Success: fmt.Sprintf("Sandbox repository switched to %s", selectedBranch),
		Failure: "Sandbox branch switch",
	}, func(ctx context.Context, task *terminal.Task) error {
		if err := task.Run(ctx, terminal.TaskSpec{
			Running:      "Updating Sandbox repository",
			Success:      fmt.Sprintf("Sandbox repository updated (%s)", selectedBranch),
			Failure:      "Sandbox repository update",
			ChildDisplay: terminal.CollapseChildTasks,
		}, func(ctx context.Context, gitTask *terminal.Task) error {
			return git.FetchAndResetBranch(ctx, gitTask, layout.Current().SandboxRepoPath, selectedBranch, saltboxUser, nil, "Sandbox")
		}); err != nil {
			return err
		}

		cacheInstance, err := ansible.NewCache()
		if err != nil {
			return fmt.Errorf("error creating cache: %w", err)
		}

		return task.Run(ctx, terminal.TaskSpec{Running: "Updating Sandbox tags cache"}, func(context.Context, *terminal.Task) error {
			_, err := ansible.RunAndCacheAnsibleTags(ctx, layout.Current().SandboxRepoPath, layout.SandboxPlaybookPath(), "", cacheInstance, 0)
			return err
		})
	})
}
