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

	if err := spinners.RunInfoSpinner("Switching Sandbox repository branch..."); err != nil {
		return err
	}

	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return err
	}

	if err := git.EnsureRemoteFetchAllBranches(ctx, constants.SandboxRepoPath); err != nil {
		return err
	}

	err = git.FetchAndReset(ctx, constants.SandboxRepoPath, branchName, saltboxUser, nil, nil, "Sandbox")
	if err != nil {
		return err
	}

	cacheInstance, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("error creating cache: %w", err)
	}

	if err := spinners.RunTaskWithSpinnerContext(ctx, "Updating Sandbox tags cache", func() error {
		_, err := ansible.RunAndCacheAnsibleTags(ctx, constants.SandboxRepoPath, constants.SandboxPlaybookPath(), "", cacheInstance, 0)
		return err
	}); err != nil {
		return err
	}

	if err := spinners.RunInfoSpinner(fmt.Sprintf("Sandbox repository branch switched to %s.", branchName)); err != nil {
		return err
	}
	return nil
}
