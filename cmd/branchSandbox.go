package cmd

import (
	"fmt"
	"strings"

	"github.com/saltyorg/sb-go/ansible"
	"github.com/saltyorg/sb-go/cache"
	"github.com/saltyorg/sb-go/constants"
	"github.com/saltyorg/sb-go/git"
	"github.com/saltyorg/sb-go/utils"
	"github.com/spf13/cobra"
)

var branchSandboxCmd = &cobra.Command{
	Use:   "branch-sandbox [branch_name]",
	Short: "Change the branch used by Sandbox",
	Long:  `Change the branch used by Sandbox`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		branchName := args[0]
		return changeSandboxBranch(branchName)
	},
}

func init() {
	rootCmd.AddCommand(branchSandboxCmd)
}

func changeSandboxBranch(branchName string) error {
	fmt.Println("Switching Sandbox repository branch...")

	customCommands := [][]string{
		{"cp", fmt.Sprintf("%s/defaults/ansible.cfg.default", constants.SandboxRepoPath), fmt.Sprintf("%s/ansible.cfg", constants.SandboxRepoPath)},
	}

	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return err
	}

	err = git.FetchAndReset(constants.SandboxRepoPath, branchName, saltboxUser, customCommands)
	if err != nil {
		return err
	}

	// Run Settings role with specified tags and skip-tags
	tags := []string{"settings"}
	skipTags := []string{"sanity-check", "pre-tasks"}
	if err := runSandboxAnsiblePlaybook(constants.SandboxRepoPath, constants.SandboxPlaybookPath(), constants.AnsiblePlaybookBinaryPath, tags, skipTags); err != nil {
		return err
	}

	fmt.Println("Updating Sandbox tags cache.")
	cacheInstance, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("error creating cache: %w", err)
	}

	_, err = ansible.RunAndCacheAnsibleTags(constants.SandboxRepoPath, constants.SandboxPlaybookPath(), "", cacheInstance)
	if err != nil {
		return err
	}

	fmt.Printf("Sandbox repository branch switched to %s and settings updated.\n", branchName)
	return nil
}

func runSandboxAnsiblePlaybook(repoPath, playbookPath, ansibleBinaryPath string, tags, skipTags []string) error {
	tagsArg := strings.Join(tags, ",")
	skipTagsArg := strings.Join(skipTags, ",")

	allArgs := []string{"--tags", tagsArg, "--skip-tags", skipTagsArg}

	err := ansible.RunAnsiblePlaybook(repoPath, playbookPath, ansibleBinaryPath, allArgs, true)
	if err != nil {
		return fmt.Errorf("error running playbook: %w", err)
	}
	return nil
}
