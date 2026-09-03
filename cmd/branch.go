package cmd

import (
	"context"
	"fmt"

	"github.com/saltyorg/sb-go/ansible"
	"github.com/saltyorg/sb-go/git"
	"github.com/saltyorg/sb-go/host"
	"github.com/saltyorg/sb-go/layout"
	"github.com/saltyorg/sb-go/python"
	"github.com/saltyorg/sb-go/saltbox"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/spf13/cobra"
)

// branchCmd represents the branch command
func newBranchCommand() *cobra.Command {
	return &cobra.Command{
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
}

func addBranchCommand(rootCmd *cobra.Command) {
	branchCmd := newBranchCommand()
	rootCmd.AddCommand(branchCmd)
}

func changeBranch(ctx context.Context, branchName string) error {
	runner := terminal.NewRunner(terminal.RunnerOptions{})

	saltboxUser, err := host.GetSaltboxUser()
	if err != nil {
		return err
	}

	if err := git.EnsureRemoteFetchAllBranches(ctx, layout.SaltboxRepoPath); err != nil {
		return err
	}

	selectedBranch, err := git.ResolveUpdateBranch(ctx, runner, layout.SaltboxRepoPath, branchName, nil, "Saltbox")
	if err != nil {
		return err
	}

	return runner.Run(ctx, terminal.TaskSpec{
		Running: fmt.Sprintf("Switching Saltbox repository to %s", selectedBranch),
		Success: fmt.Sprintf("Saltbox repository switched to %s", selectedBranch),
		Failure: "Saltbox branch switch",
	}, func(ctx context.Context, task *terminal.Task) error {
		if err := task.Run(ctx, terminal.TaskSpec{
			Running: "Updating Saltbox repository",
			Success: fmt.Sprintf("Saltbox repository updated (%s)", selectedBranch),
			Failure: "Saltbox repository update",
		}, func(ctx context.Context, gitTask *terminal.Task) error {
			return git.FetchAndResetBranch(ctx, gitTask, layout.SaltboxRepoPath, selectedBranch, saltboxUser, nil, "Saltbox")
		}); err != nil {
			return err
		}

		if err := task.Run(ctx, terminal.TaskSpec{
			Running: "Checking saltbox.fact",
			Success: "saltbox.fact is ready",
			Failure: "saltbox.fact update",
		}, func(ctx context.Context, factTask *terminal.Task) error {
			return saltbox.DownloadAndInstallSaltboxFact(ctx, factTask, false, false)
		}); err != nil {
			return err
		}

		if err := task.Run(ctx, terminal.TaskSpec{
			Running: "Preparing Ansible virtual environment",
			Success: "Ansible virtual environment ready",
			Failure: "Ansible virtual environment",
		}, func(ctx context.Context, venvTask *terminal.Task) error {
			return python.ManageAnsibleVenv(ctx, venvTask, false, saltboxUser, false)
		}); err != nil {
			return fmt.Errorf("error managing Ansible venv: %w", err)
		}

		cacheInstance, err := ansible.NewCache()
		if err != nil {
			return fmt.Errorf("error creating cache: %w", err)
		}

		return task.Run(ctx, terminal.TaskSpec{Running: "Updating Saltbox tags cache"}, func(context.Context, *terminal.Task) error {
			_, err := ansible.RunAndCacheAnsibleTags(ctx, layout.SaltboxRepoPath, layout.SaltboxPlaybookPath(), "", cacheInstance, 0)
			return err
		})
	})
}
