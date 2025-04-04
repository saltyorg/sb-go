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
	// Add the -v flag as a persistent flag to the config command.
	updateCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}

func handleUpdate() error {
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
	if verbose {
		fmt.Println("--- Updating Saltbox (Verbose) ---")

		fmt.Println("Checking Saltbox repository path...")
		if _, err := os.Stat(constants.SaltboxRepoPath); os.IsNotExist(err) {
			return fmt.Errorf("error: SB_REPO_PATH does not exist or is not a directory")
		}

		fmt.Println("Getting Saltbox user...")
		saltboxUser, err := utils.GetSaltboxUser()
		if err != nil {
			return fmt.Errorf("error getting saltbox user: %w", err)
		}
		fmt.Printf("Saltbox user: %s\n", saltboxUser)

		fmt.Println("Managing Ansible venv...")
		if err := venv.ManageAnsibleVenv(false, saltboxUser, verbose); err != nil {
			return fmt.Errorf("error managing Ansible venv: %w", err)
		}

		customCommands := [][]string{
			{
				"cp",
				fmt.Sprintf("%s/defaults/ansible.cfg.default", constants.SaltboxRepoPath),
				fmt.Sprintf("%s/ansible.cfg", constants.SaltboxRepoPath),
			},
		}

		fmt.Println("Getting old Git commit hash...")
		oldCommitHash, err := git.GetGitCommitHash(constants.SaltboxRepoPath)
		if err != nil {
			return fmt.Errorf("error getting old commit hash: %w", err)
		}
		fmt.Printf("Old commit hash: %s\n", oldCommitHash)

		fmt.Println("Fetching and resetting Git repository...")
		if err := git.FetchAndReset(constants.SaltboxRepoPath, "master", saltboxUser, customCommands); err != nil {
			return fmt.Errorf("error fetching and resetting git: %w", err)
		}

		fmt.Println("Downloading and installing Saltbox fact...")
		if err := fact.DownloadAndInstallSaltboxFact(false, verbose); err != nil {
			return fmt.Errorf("error downloading and installing saltbox fact: %w", err)
		}

		tags := []string{"--tags", "settings"}
		skipTags := []string{"--skip-tags", "sanity-check,pre-tasks"}

		ansibleArgs := append(tags, skipTags...)

		fmt.Println("Running Ansible Playbook to upgrade configuration files...")
		if err := ansible.RunAnsiblePlaybook(constants.SaltboxRepoPath, constants.SaltboxPlaybookPath(), constants.AnsiblePlaybookBinaryPath, ansibleArgs, verbose); err != nil {
			return fmt.Errorf("error running ansible playbook: %w", err)
		}

		fmt.Println("Getting new Git commit hash...")
		newCommitHash, err := git.GetGitCommitHash(constants.SaltboxRepoPath)
		if err != nil {
			return fmt.Errorf("error getting new commit hash: %w", err)
		}
		fmt.Printf("New commit hash: %s\n", newCommitHash)

		if oldCommitHash != newCommitHash {
			fmt.Println("Saltbox Commit Hash changed, updating tags cache...")
			ansibleCache, err := cache.NewCache()
			if err != nil {
				return fmt.Errorf("error creating cache: %w", err)
			}
			if _, err := ansible.RunAndCacheAnsibleTags(constants.SaltboxRepoPath, constants.SaltboxPlaybookPath(), "", ansibleCache); err != nil {
				return fmt.Errorf("error running and caching ansible tags: %w", err)
			}
		}

		fmt.Println("--- Saltbox Update Completed (Verbose) ---")

	} else {
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

		if err := venv.ManageAnsibleVenv(false, saltboxUser, verbose); err != nil {
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

		if err := fact.DownloadAndInstallSaltboxFact(false, verbose); err != nil {
			return fmt.Errorf("error downloading and installing saltbox fact: %w", err)
		}

		tags := []string{"--tags", "settings"}
		skipTags := []string{"--skip-tags", "sanity-check,pre-tasks"}

		ansibleArgs := append(tags, skipTags...)

		if err := spinners.RunTaskWithSpinner("Running Ansible Playbook to upgrade configuration files", func() error {
			return ansible.RunAnsiblePlaybook(constants.SaltboxRepoPath, constants.SaltboxPlaybookPath(), constants.AnsiblePlaybookBinaryPath, ansibleArgs, verbose)
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
	}
	return nil
}

// updateSandbox updates the Sandbox repository and configuration.
func updateSandbox(verbose bool) error {
	if verbose {
		fmt.Println("--- Updating Sandbox (Verbose) ---")

		fmt.Println("Checking Sandbox repository path...")
		if _, err := os.Stat(constants.SandboxRepoPath); os.IsNotExist(err) {
			return fmt.Errorf("error: %s does not exist or is not a directory", constants.SandboxRepoPath)
		}

		fmt.Println("Getting Saltbox user...")
		saltboxUser, err := utils.GetSaltboxUser()
		if err != nil {
			return fmt.Errorf("error getting saltbox user: %w", err)
		}
		fmt.Printf("Saltbox user: %s\n", saltboxUser)

		customCommands := [][]string{
			{
				"cp",
				fmt.Sprintf("%s/defaults/ansible.cfg.default", constants.SandboxRepoPath),
				fmt.Sprintf("%s/ansible.cfg", constants.SandboxRepoPath),
			},
		}

		fmt.Println("Getting old Git commit hash...")
		oldCommitHash, err := git.GetGitCommitHash(constants.SandboxRepoPath)
		if err != nil {
			return fmt.Errorf("error getting old commit hash: %w", err)
		}
		fmt.Printf("Old commit hash: %s\n", oldCommitHash)

		fmt.Println("Fetching and resetting Git repository...")
		if err := git.FetchAndReset(constants.SandboxRepoPath, "master", saltboxUser, customCommands); err != nil {
			return fmt.Errorf("error fetching and resetting git: %w", err)
		}

		tags := []string{"--tags", "settings"}
		skipTags := []string{"--skip-tags", "sanity-check,pre-tasks"}

		ansibleArgs := append(tags, skipTags...)

		fmt.Println("Running Ansible Playbook to upgrade configuration files...")
		if err := ansible.RunAnsiblePlaybook(constants.SandboxRepoPath, constants.SandboxPlaybookPath(), constants.AnsiblePlaybookBinaryPath, ansibleArgs, verbose); err != nil {
			return fmt.Errorf("error running ansible playbook: %w", err)
		}

		fmt.Println("Getting new Git commit hash...")
		newCommitHash, err := git.GetGitCommitHash(constants.SandboxRepoPath)
		if err != nil {
			return fmt.Errorf("error getting new commit hash: %w", err)
		}
		fmt.Printf("New commit hash: %s\n", newCommitHash)

		if oldCommitHash != newCommitHash {
			fmt.Println("Sandbox Commit Hash changed, updating tags cache...")
			ansibleCache, err := cache.NewCache()
			if err != nil {
				return fmt.Errorf("error creating cache: %w", err)
			}
			if _, err := ansible.RunAndCacheAnsibleTags(constants.SandboxRepoPath, constants.SandboxPlaybookPath(), "", ansibleCache); err != nil {
				return fmt.Errorf("error running and caching ansible tags: %w", err)
			}
		}

		fmt.Println("--- Sandbox Update Completed (Verbose) ---")

	} else {
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
			return ansible.RunAnsiblePlaybook(constants.SandboxRepoPath, constants.SandboxPlaybookPath(), constants.AnsiblePlaybookBinaryPath, ansibleArgs, verbose)
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
	}
	return nil
}
