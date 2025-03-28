package cmd

import (
	"fmt"
	"github.com/saltyorg/sb-go/ansible"
	"github.com/saltyorg/sb-go/cache"
	"github.com/saltyorg/sb-go/constants"
	"github.com/saltyorg/sb-go/fact"
	"github.com/saltyorg/sb-go/git"
	"github.com/saltyorg/sb-go/spinners"
	"github.com/saltyorg/sb-go/utils"
	"github.com/saltyorg/sb-go/venv"
	"os"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update Saltbox & Sandbox",
	Long:  `Update Saltbox & Sandbox`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleUpdate()
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func handleUpdate() error {
	if err := updateSaltbox(); err != nil {
		return fmt.Errorf("error updating Saltbox: %w", err)
	}
	if err := updateSandbox(); err != nil {
		return fmt.Errorf("error updating Sandbox: %w", err)
	}
	return nil
}

func updateSaltbox() error {
	if err := spinners.RunInfoSpinner("Updating Saltbox"); err != nil {
		return err
	}

	if _, err := os.Stat(constants.SaltboxRepoPath); os.IsNotExist(err) {
		return fmt.Errorf("error: SB_REPO_PATH does not exist or is not a directory")
	}

	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	if err := venv.ManageAnsibleVenv(false, saltboxUser); err != nil {
		return fmt.Errorf("error managing Ansible venv: %w", err)
	}

	customCommands := [][]string{
		{
			"cp",
			fmt.Sprintf("%s/defaults/ansible.cfg.default", constants.SaltboxRepoPath),
			fmt.Sprintf("%s/ansible.cfg", constants.SaltboxRepoPath),
		},
	}

	oldCommitHash, err := git.GetGitCommitHash(constants.SaltboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting old commit hash: %w", err)
	}

	if err := git.FetchAndReset(constants.SaltboxRepoPath, "master", saltboxUser, customCommands); err != nil {
		return fmt.Errorf("error fetching and resetting git: %w", err)
	}

	if err := fact.DownloadAndInstallSaltboxFact(false); err != nil {
		return fmt.Errorf("error downloading and installing saltbox fact: %w", err)
	}

	tags := []string{"--tags", "settings"}
	skipTags := []string{"--skip-tags", "sanity-check,pre-tasks"}

	ansibleArgs := append(tags, skipTags...)

	if err := spinners.RunTaskWithSpinner("Running Ansible Playbook to upgrade configuration files", func() error {
		return ansible.RunAnsiblePlaybook(constants.SaltboxRepoPath, constants.SaltboxPlaybookPath(), constants.AnsiblePlaybookBinaryPath, ansibleArgs, true)
	}); err != nil {
		return fmt.Errorf("error running ansible playbook: %w", err)
	}

	newCommitHash, err := git.GetGitCommitHash(constants.SaltboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting new commit hash: %w", err)
	}

	if oldCommitHash != newCommitHash {
		if err := spinners.RunInfoSpinner("Saltbox Commit Hash changed, updating tags cache."); err != nil {
			return err
		}
		ansibleCache, err := cache.NewCache()
		if err != nil {
			return fmt.Errorf("error creating cache: %w", err)
		}
		if _, err := ansible.RunAndCacheAnsibleTags(constants.SaltboxRepoPath, constants.SaltboxPlaybookPath(), "", ansibleCache); err != nil {
			return fmt.Errorf("error running and caching ansible tags: %w", err)
		}
	}

	if err := spinners.RunInfoSpinner("Saltbox Update Completed"); err != nil {
		return err
	}

	return nil
}

func updateSandbox() error {
	if err := spinners.RunInfoSpinner("Updating Sandbox"); err != nil {
		return err
	}

	if _, err := os.Stat(constants.SandboxRepoPath); os.IsNotExist(err) {
		return fmt.Errorf("error: %s does not exist or is not a directory", constants.SandboxRepoPath)
	}

	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	customCommands := [][]string{
		{
			"cp",
			fmt.Sprintf("%s/defaults/ansible.cfg.default", constants.SandboxRepoPath),
			fmt.Sprintf("%s/ansible.cfg", constants.SandboxRepoPath),
		},
	}

	oldCommitHash, err := git.GetGitCommitHash(constants.SandboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting old commit hash: %w", err)
	}

	if err := git.FetchAndReset(constants.SandboxRepoPath, "master", saltboxUser, customCommands); err != nil {
		return fmt.Errorf("error fetching and resetting git: %w", err)
	}

	tags := []string{"--tags", "settings"}
	skipTags := []string{"--skip-tags", "sanity-check,pre-tasks"}

	ansibleArgs := append(tags, skipTags...)

	if err := spinners.RunTaskWithSpinner("Running Ansible Playbook to upgrade configuration files", func() error {
		return ansible.RunAnsiblePlaybook(constants.SandboxRepoPath, constants.SandboxPlaybookPath(), constants.AnsiblePlaybookBinaryPath, ansibleArgs, true)
	}); err != nil {
		return fmt.Errorf("error running ansible playbook: %w", err)
	}

	newCommitHash, err := git.GetGitCommitHash(constants.SandboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting new commit hash: %w", err)
	}

	if oldCommitHash != newCommitHash {
		if err := spinners.RunInfoSpinner("Sandbox Commit Hash changed, updating tags cache."); err != nil {
			return err
		}
		ansibleCache, err := cache.NewCache()
		if err != nil {
			return fmt.Errorf("error creating cache: %w", err)
		}
		if _, err := ansible.RunAndCacheAnsibleTags(constants.SandboxRepoPath, constants.SandboxPlaybookPath(), "", ansibleCache); err != nil {
			return fmt.Errorf("error running and caching ansible tags: %w", err)
		}
	}

	if err := spinners.RunInfoSpinner("Sandbox Update Completed"); err != nil {
		return err
	}

	return nil
}
