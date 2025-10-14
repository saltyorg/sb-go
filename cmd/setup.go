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
				return
			}
		}

		if err := spinners.RunTaskWithSpinnerContext(ctx, "Checking Ubuntu version", func() error {
			return utils.CheckUbuntuSupport()
		}); err != nil {
			fmt.Println(err)
			return
		}

		if err := spinners.RunTaskWithSpinnerContext(ctx, "Checking CPU architecture", func() error {
			return utils.CheckArchitecture(ctx)
		}); err != nil {
			fmt.Println(err)
			return
		}

		if err := spinners.RunTaskWithSpinnerContext(ctx, "Checking for LXC container", func() error {
			return utils.CheckLXC(ctx)
		}); err != nil {
			fmt.Println(err)
			return
		}

		if err := spinners.RunTaskWithSpinnerContext(ctx, "Checking for desktop environment", func() error {
			return utils.CheckDesktopEnvironment(ctx)
		}); err != nil {
			fmt.Println(err)
			return
		}

		// Perform initial setup tasks
		if err := setup.InitialSetup(ctx, verbose); err != nil {
			fmt.Printf("Error during initial setup: %v\n", err)
			os.Exit(1)
		}

		// Configure the locale
		if err := setup.ConfigureLocale(ctx); err != nil {
			fmt.Printf("Error configuring locale: %v\n", err)
			os.Exit(1)
		}

		// Setup Python venv
		if err := setup.PythonVenv(ctx, verbose); err != nil {
			fmt.Printf("Error setting up Python venv: %v\n", err)
			os.Exit(1)
		}

		// Setup Saltbox Repo
		if err := setup.SaltboxRepo(ctx, verbose, branch); err != nil {
			fmt.Printf("Error setting up Saltbox repository: %v\n", err)
			os.Exit(1)
		}

		// Install pip3 Dependencies
		if err := setup.InstallPipDependencies(ctx, verbose); err != nil {
			fmt.Printf("Error installing pip dependencies: %v\n", err)
			os.Exit(1)
		}

		// Copy ansible* files to /usr/local/bin
		if err := setup.CopyRequiredBinaries(ctx); err != nil {
			fmt.Printf("Error copying binaries: %v\n", err)
			os.Exit(1)
		}

		if verbose {
			fmt.Println("Initial setup tasks completed")
		} else {
			if err := spinners.RunInfoSpinner("Initial setup tasks completed"); err != nil {
				fmt.Println(err)
				return
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
	setupCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	setupCmd.PersistentFlags().StringP("branch", "b", "master", "Branch to use for Saltbox repository")
}
