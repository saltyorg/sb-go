package cmd

import (
	"context"
	"fmt"

	"github.com/saltyorg/sb-go/internal/ansible"
	"github.com/saltyorg/sb-go/internal/cache"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/git"
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
	fmt.Println("Switching Sandbox repository branch...")

	customCommands := [][]string{
		{"cp", fmt.Sprintf("%s/defaults/ansible.cfg.default", constants.SandboxRepoPath), fmt.Sprintf("%s/ansible.cfg", constants.SandboxRepoPath)},
	}

	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return err
	}

	err = git.FetchAndReset(ctx, constants.SandboxRepoPath, branchName, saltboxUser, customCommands, nil, "Sandbox")
	if err != nil {
		return err
	}

	fmt.Println("Updating Sandbox tags cache.")
	cacheInstance, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("error creating cache: %w", err)
	}

	_, err = ansible.RunAndCacheAnsibleTags(ctx, constants.SandboxRepoPath, constants.SandboxPlaybookPath(), "", cacheInstance, 0)
	if err != nil {
		return err
	}

	fmt.Printf("Sandbox repository branch switched to %s.\n", branchName)
	return nil
}
