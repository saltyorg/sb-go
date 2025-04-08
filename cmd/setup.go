package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/saltyorg/sb-go/constants"
	"github.com/saltyorg/sb-go/setup"
	"github.com/saltyorg/sb-go/spinners"
	"github.com/saltyorg/sb-go/utils"
	"github.com/spf13/cobra"
)

// setupCmd represents the setup command
var setupCmd = &cobra.Command{
	Use:    "setup",
	Short:  "Install Saltbox and its dependencies",
	Long:   `Install Saltbox and its dependencies`,
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		// Set verbose mode for spinners
		spinners.SetVerboseMode(verbose)

		// Check for existing Saltbox installation and prompt for confirmation.
		if _, err := os.Stat(constants.SaltboxRepoPath); err == nil {
			reader := bufio.NewReader(os.Stdin)
			fmt.Printf("\n%s folder already exists. Continuing may reset your installation. Are you sure you want to continue? (yes/no): ", constants.SaltboxRepoPath) // Use constant here
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))

			if response != "yes" && response != "y" {
				fmt.Println("Setup aborted by user.")
				os.Exit(0)
			}
		}

		if err := spinners.RunTaskWithSpinner("Checking for existing Cloudbox installation", func() error {
			return utils.CheckCloudboxInstalled()
		}); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if err := spinners.RunTaskWithSpinner("Checking Ubuntu version", func() error {
			return utils.CheckUbuntuSupport()
		}); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if err := spinners.RunTaskWithSpinner("Checking CPU architecture", func() error {
			return utils.CheckArchitecture()
		}); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if err := spinners.RunTaskWithSpinner("Checking for LXC container", func() error {
			return utils.CheckLXC()
		}); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if err := spinners.RunTaskWithSpinner("Checking for desktop environment", func() error {
			return utils.CheckDesktopEnvironment()
		}); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Perform initial setup tasks (moved to setup package)
		setup.InitialSetup(verbose)

		// Configure the locale (moved to setup package)
		setup.ConfigureLocale()

		// Setup Python venv (moved to setup package)
		setup.PythonVenv(verbose)

		// Setup Saltbox Repo (moved to setup package)
		setup.SaltboxRepo(verbose)

		// Install pip3 Dependencies
		setup.InstallPipDependencies()

		// Copy ansible* files to /usr/local/bin
		setup.CopyAnsibleBinaries()

		if verbose {
			fmt.Println("Initial setup tasks completed")
		} else {
			if err := spinners.RunInfoSpinner("Initial setup tasks completed"); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
	// Add the -v flag as a persistent flag to the config command.
	setupCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}
