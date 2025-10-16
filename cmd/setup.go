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
	RunE: func(cmd *cobra.Command, args []string) error {
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
				return nil
			}
		}

		if err := spinners.RunTaskWithSpinnerContext(ctx, "Checking Ubuntu version", func() error {
			return utils.CheckUbuntuSupport()
		}); err != nil {
			return err
		}

		if err := spinners.RunTaskWithSpinnerContext(ctx, "Checking CPU architecture", func() error {
			return utils.CheckArchitecture(ctx)
		}); err != nil {
			return err
		}

		if err := spinners.RunTaskWithSpinnerContext(ctx, "Checking for LXC container", func() error {
			return utils.CheckLXC(ctx)
		}); err != nil {
			return err
		}

		if err := spinners.RunTaskWithSpinnerContext(ctx, "Checking for desktop environment", func() error {
			return utils.CheckDesktopEnvironment(ctx)
		}); err != nil {
			return err
		}

		// Perform initial setup tasks
		if err := setup.InitialSetup(ctx, verbose); err != nil {
			return fmt.Errorf("error during initial setup: %w", err)
		}

		// Configure the locale
		if err := setup.ConfigureLocale(ctx); err != nil {
			return fmt.Errorf("error configuring locale: %w", err)
		}

		// Setup Python venv
		if err := setup.PythonVenv(ctx, verbose); err != nil {
			return fmt.Errorf("error setting up Python venv: %w", err)
		}

		// Setup Saltbox Repo
		if err := setup.SaltboxRepo(ctx, verbose, branch); err != nil {
			return fmt.Errorf("error setting up Saltbox repository: %w", err)
		}

		// Install pip3 Dependencies
		if err := setup.InstallPipDependencies(ctx, verbose); err != nil {
			return fmt.Errorf("error installing pip dependencies: %w", err)
		}

		// Copy ansible* files to /usr/local/bin
		if err := setup.CopyRequiredBinaries(ctx); err != nil {
			return fmt.Errorf("error copying binaries: %w", err)
		}

		if verbose {
			fmt.Println("Initial setup tasks completed")
		} else {
			if err := spinners.RunInfoSpinner("Initial setup tasks completed"); err != nil {
				return err
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
	setupCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	setupCmd.PersistentFlags().StringP("branch", "b", "master", "Branch to use for Saltbox repository")
}
