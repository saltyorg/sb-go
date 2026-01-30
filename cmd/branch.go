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
	spinners.SetVerboseMode(false)

	if err := spinners.RunInfoSpinner("Switching Saltbox repository branch..."); err != nil {
		return err
	}

	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return err
	}

	if err := git.EnsureRemoteFetchAllBranches(ctx, constants.SaltboxRepoPath); err != nil {
		return err
	}

	err = git.FetchAndReset(ctx, constants.SaltboxRepoPath, branchName, saltboxUser, nil, nil, "Saltbox")
	if err != nil {
		return err
	}

	// Always update saltbox.fact during branch change
	if err := fact.DownloadAndInstallSaltboxFact(false, false); err != nil {
		return err
	}

	// Manage Ansible venv - this function already has internal spinners
	if err := venv.ManageAnsibleVenv(ctx, false, saltboxUser, false); err != nil {
		return fmt.Errorf("error managing Ansible venv: %w", err)
	}

	cacheInstance, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("error creating cache: %w", err)
	}

	if err := spinners.RunTaskWithSpinnerContext(ctx, "Updating Saltbox tags cache", func() error {
		_, err := ansible.RunAndCacheAnsibleTags(ctx, constants.SaltboxRepoPath, constants.SaltboxPlaybookPath(), "", cacheInstance, 0)
		return err
	}); err != nil {
		return err
	}

	if err := spinners.RunInfoSpinner(fmt.Sprintf("Saltbox repository branch switched to %s.", branchName)); err != nil {
		return err
	}
	return nil
}
