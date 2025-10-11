package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/setup"
	"github.com/saltyorg/sb-go/internal/spinners"
	"github.com/saltyorg/sb-go/internal/utils"

	"github.com/spf13/cobra"
)

// setupCmd represents the setup command
var setupCmd = &cobra.Command{
	Use:    "setup",
	Short:  "Install Saltbox and its dependencies",
	Long:   `Install Saltbox and its dependencies`,
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()
		verbose, _ := cmd.Flags().GetBool("verbose")
		branch, _ := cmd.Flags().GetString("branch")

		// Set verbose mode for spinners
		spinners.SetVerboseMode(verbose)

		// Check if Saltbox installation was already installed and prompt for confirmation.
		if _, err := os.Stat(constants.SaltboxRepoPath); err == nil {
			reader := bufio.NewReader(os.Stdin)
			fmt.Printf("\n%s folder already exists. Continuing may reset your installation. Are you sure you want to continue? (yes/no): ", constants.SaltboxRepoPath)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))

			if response != "yes" && response != "y" {
				fmt.Println("Setup aborted by user.")
				os.Exit(0)
			}
		}

		if err := spinners.RunTaskWithSpinnerContext(ctx, "Checking Ubuntu version", func() error {
			return utils.CheckUbuntuSupport()
		}); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if err := spinners.RunTaskWithSpinnerContext(ctx, "Checking CPU architecture", func() error {
			return utils.CheckArchitecture(ctx)
		}); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if err := spinners.RunTaskWithSpinnerContext(ctx, "Checking for LXC container", func() error {
			return utils.CheckLXC(ctx)
		}); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if err := spinners.RunTaskWithSpinnerContext(ctx, "Checking for desktop environment", func() error {
			return utils.CheckDesktopEnvironment(ctx)
		}); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Perform initial setup tasks
		setup.InitialSetup(ctx, verbose)

		// Configure the locale
		setup.ConfigureLocale(ctx)

		// Setup Python venv
		setup.PythonVenv(ctx, verbose)

		// Setup Saltbox Repo
		setup.SaltboxRepo(ctx, verbose, branch)

		// Install pip3 Dependencies
		setup.InstallPipDependencies(ctx, verbose)

		// Copy ansible* files to /usr/local/bin
		setup.CopyRequiredBinaries(ctx)

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
	setupCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	setupCmd.PersistentFlags().StringP("branch", "b", "master", "Branch to use for Saltbox repository")
}
