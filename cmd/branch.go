package cmd

import (
	"fmt"

	"github.com/saltyorg/sb-go/internal/ansible"
	"github.com/saltyorg/sb-go/internal/cache"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/fact"
	"github.com/saltyorg/sb-go/internal/git"
	"github.com/saltyorg/sb-go/internal/utils"

	"github.com/spf13/cobra"
)

// branchCmd represents the branch command
var branchCmd = &cobra.Command{
	Use:   "branch [branch_name]",
	Short: "Change the branch used by Saltbox",
	Long:  `Change the branch used by Saltbox`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		branchName := args[0]
		return changeBranch(branchName)
	},
}

func init() {
	rootCmd.AddCommand(branchCmd)
}

func changeBranch(branchName string) error {
	fmt.Println("Switching Saltbox repository branch...")

	customCommands := [][]string{
		{"cp", fmt.Sprintf("%s/defaults/ansible.cfg.default", constants.SaltboxRepoPath), fmt.Sprintf("%s/ansible.cfg", constants.SaltboxRepoPath)},
	}

	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return err
	}

	err = git.FetchAndReset(constants.SaltboxRepoPath, branchName, saltboxUser, customCommands)
	if err != nil {
		return err
	}

	// Always update saltbox.fact during branch change
	if err := fact.DownloadAndInstallSaltboxFact(false, false); err != nil {
		return err
	}

	fmt.Println("Updating Saltbox tags cache.")
	cacheInstance, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("error creating cache: %w", err)
	}

	_, err = ansible.RunAndCacheAnsibleTags(constants.SaltboxRepoPath, constants.SaltboxPlaybookPath(), "", cacheInstance)
	if err != nil {
		return err
	}

	fmt.Printf("Saltbox repository branch switched to %s.\n", branchName)
	return nil
}
