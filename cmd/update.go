package cmd

import (
	"fmt"
	"os"

	"github.com/saltyorg/sb-go/ansible"
	"github.com/saltyorg/sb-go/cache"
	"github.com/saltyorg/sb-go/constants"
	"github.com/saltyorg/sb-go/fact"
	"github.com/saltyorg/sb-go/git"
	"github.com/saltyorg/sb-go/spinners"
	"github.com/saltyorg/sb-go/utils"
	"github.com/saltyorg/sb-go/validate"
	"github.com/saltyorg/sb-go/venv"
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
	updateCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}

func handleUpdate() error {
	// Set verbose mode for spinners
	spinners.SetVerboseMode(verbose)

	doSelfUpdate(true, verbose, "Re-run your previous update command")
	if err := updateSaltbox(verbose); err != nil {
		return fmt.Errorf("error updating Saltbox: %w", err)
	}
	if err := updateSandbox(verbose); err != nil {
		return fmt.Errorf("error updating Sandbox: %w", err)
	}
	return nil
}

// updateSaltbox updates the Saltbox repository and configuration.
func updateSaltbox(verbose bool) error {
	// Check if Saltbox repo exists
	if _, err := os.Stat(constants.SaltboxRepoPath); os.IsNotExist(err) {
		return fmt.Errorf("error: SB_REPO_PATH does not exist or is not a directory")
	}

	if err := spinners.RunInfoSpinner("Validating Saltbox configuration"); err != nil {
		return err
	}

	// Validate Saltbox configuration
	err := validate.AllSaltboxConfigs(verbose)
	if err != nil {
		fmt.Println("Saltbox update cancelled")
		return fmt.Errorf("error validating configs: %w", err)
	}

	// Get Saltbox user
	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	// Manage Ansible venv - this function already has internal spinners
	if err := venv.ManageAnsibleVenv(false, saltboxUser, verbose); err != nil {
		return fmt.Errorf("error managing Ansible venv: %w", err)
	}

	// Set up custom commands for git
	customCommands := [][]string{
		{
			"cp",
			fmt.Sprintf("%s/defaults/ansible.cfg.default", constants.SaltboxRepoPath),
			fmt.Sprintf("%s/ansible.cfg", constants.SaltboxRepoPath),
		},
	}

	// Get old commit hash
	oldCommitHash, err := git.GetGitCommitHash(constants.SaltboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting old commit hash: %w", err)
	}

	// Fetch and reset git repo - this function already has internal spinners
	if err := git.FetchAndReset(constants.SaltboxRepoPath, "master", saltboxUser, customCommands); err != nil {
		return fmt.Errorf("error fetching and resetting git: %w", err)
	}

	// Download and install Saltbox fact - this function already has internal spinners
	if err := fact.DownloadAndInstallSaltboxFact(false); err != nil {
		return fmt.Errorf("error downloading and installing saltbox fact: %w", err)
	}

	// Get commit hash after fetch and reset
	newCommitHash, err := git.GetGitCommitHash(constants.SaltboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting new commit hash: %w", err)
	}

	// Update tags cache if commit hash changed
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

	// Final success message
	return spinners.RunInfoSpinner("Saltbox Update Completed")
}

// updateSandbox updates the Sandbox repository and configuration.
func updateSandbox(verbose bool) error {
	// Check if Sandbox repo exists
	if _, err := os.Stat(constants.SandboxRepoPath); os.IsNotExist(err) {
		return fmt.Errorf("error: %s does not exist or is not a directory", constants.SandboxRepoPath)
	}

	// Get Saltbox user
	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	// Set up custom commands for git
	customCommands := [][]string{
		{
			"cp",
			fmt.Sprintf("%s/defaults/ansible.cfg.default", constants.SandboxRepoPath),
			fmt.Sprintf("%s/ansible.cfg", constants.SandboxRepoPath),
		},
	}

	// Get old commit hash
	oldCommitHash, err := git.GetGitCommitHash(constants.SandboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting old commit hash: %w", err)
	}

	// Fetch and reset git repo - this function already has internal spinners
	if err := git.FetchAndReset(constants.SandboxRepoPath, "master", saltboxUser, customCommands); err != nil {
		return fmt.Errorf("error fetching and resetting git: %w", err)
	}

	// Run Ansible playbook to upgrade configuration files
	tags := []string{"--tags", "settings"}
	skipTags := []string{"--skip-tags", "sanity-check,pre-tasks"}
	ansibleArgs := append(tags, skipTags...)

	if err := spinners.RunTaskWithSpinner("Running Ansible Playbook to upgrade configuration files", func() error {
		return ansible.RunAnsiblePlaybook(constants.SandboxRepoPath, constants.SandboxPlaybookPath(), constants.AnsiblePlaybookBinaryPath, ansibleArgs, verbose)
	}); err != nil {
		return fmt.Errorf("error running ansible playbook: %w", err)
	}

	// Get commit hash after fetch and reset
	newCommitHash, err := git.GetGitCommitHash(constants.SandboxRepoPath)
	if err != nil {
		return fmt.Errorf("error getting new commit hash: %w", err)
	}

	// Update tags cache if commit hash changed
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

	// Final success message
	return spinners.RunInfoSpinner("Sandbox Update Completed")
}
