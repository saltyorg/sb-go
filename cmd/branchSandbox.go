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
	spinners.SetVerboseMode(false)

	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return err
	}

	if err := git.EnsureRemoteFetchAllBranches(ctx, constants.SandboxRepoPath); err != nil {
		return err
	}

	selectedBranch, err := git.ResolveUpdateBranch(ctx, constants.SandboxRepoPath, branchName, nil, "Sandbox")
	if err != nil {
		return err
	}

	return spinners.RunTaskWithSpinnerCustomContext(ctx, spinners.SpinnerOptions{
		TaskName:         fmt.Sprintf("Switching Sandbox repository to %s", selectedBranch),
		StopMessage:      fmt.Sprintf("Sandbox repository switched to %s", selectedBranch),
		StopFailMessage:  "Sandbox branch switch",
		CollapseChildren: true,
	}, func() error {
		if err := git.FetchAndResetBranch(ctx, constants.SandboxRepoPath, selectedBranch, saltboxUser, nil, "Sandbox"); err != nil {
			return err
		}

		cacheInstance, err := cache.NewCache()
		if err != nil {
			return fmt.Errorf("error creating cache: %w", err)
		}

		return spinners.RunTaskWithSpinnerContext(ctx, "Updating Sandbox tags cache", func() error {
			_, err := ansible.RunAndCacheAnsibleTags(ctx, constants.SandboxRepoPath, constants.SandboxPlaybookPath(), "", cacheInstance, 0)
			return err
		})
	})
}
