package cmd

import (
	"fmt"
	"strings"

	"github.com/saltyorg/sb-go/ansible"
	"github.com/saltyorg/sb-go/cache"
	"github.com/saltyorg/sb-go/constants"
	"github.com/saltyorg/sb-go/fact"
	"github.com/saltyorg/sb-go/git"
	"github.com/saltyorg/sb-go/spinners"
	"github.com/saltyorg/sb-go/utils"

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
	if err := fact.DownloadAndInstallSaltboxFact(false); err != nil {
		return err
	}

	// Run Settings role with specified tags and skip-tags
	tags := []string{"settings"}
	skipTags := []string{"sanity-check", "pre-tasks"}
	if err := runAnsiblePlaybook(constants.SaltboxRepoPath, constants.SaltboxPlaybookPath(), constants.AnsiblePlaybookBinaryPath, tags, skipTags); err != nil {
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

	fmt.Printf("Saltbox repository branch switched to %s and settings updated.\n", branchName)
	return nil
}

func runAnsiblePlaybook(repoPath, playbookPath, ansibleBinaryPath string, tags, skipTags []string) error {
	tagsArg := strings.Join(tags, ",")
	skipTagsArg := strings.Join(skipTags, ",")

	allArgs := []string{"--tags", tagsArg, "--skip-tags", skipTagsArg}

	if err := spinners.RunTaskWithSpinner("Running Ansible Playbook to update settings.yml", func() error {
		return ansible.RunAnsiblePlaybook(repoPath, playbookPath, ansibleBinaryPath, allArgs, true)
	}); err != nil {
		return fmt.Errorf("error running ansible playbook: %w", err)
	}
	return nil
}
