package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/git"
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
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		verbose, _ := cmd.Flags().GetBool("verbose")
		branch, _ := cmd.Flags().GetString("branch")

		// Set verbose mode for spinners
		spinners.SetVerboseMode(verbose)

		// Check if Saltbox installation was already installed and prompt for confirmation.
		if info, err := os.Stat(constants.SaltboxRepoPath); err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s exists but is not a directory", constants.SaltboxRepoPath)
			}
			reader := bufio.NewReader(os.Stdin)
			fmt.Printf("\n%s folder already exists. Continuing may reset your installation. Are you sure you want to continue? (yes/no): ", constants.SaltboxRepoPath)
			response, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("read setup confirmation: %w", err)
			}
			response = strings.TrimSpace(strings.ToLower(response))

			if response != "yes" && response != "y" {
				fmt.Println("Setup aborted by user.")
				return nil
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect existing Saltbox installation: %w", err)
		}

		selectedBranch := branch
		if _, err := os.Stat(constants.SaltboxRepoPath + "/.git"); err == nil {
			selectedBranch, err = git.ResolveUpdateBranch(ctx, constants.SaltboxRepoPath, branch, nil, "Saltbox")
			if err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect existing Saltbox Git repository: %w", err)
		}

		return spinners.RunTaskWithSpinnerCustomContext(ctx, spinners.SpinnerOptions{
			TaskName:         "Installing Saltbox",
			StopMessage:      "Saltbox installation completed",
			StopFailMessage:  "Saltbox installation",
			CollapseChildren: true,
		}, func() error {
			return runSetup(ctx, verbose, selectedBranch)
		})
	},
}

func runSetup(ctx context.Context, verbose bool, branch string) error {
	if err := runSetupPhase(ctx, "Checking system compatibility", func() error {
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
		return nil
	}); err != nil {
		return err
	}

	if err := runSetupPhase(ctx, "Installing system prerequisites", func() error {
		if err := setup.InitialSetup(ctx, verbose); err != nil {
			return fmt.Errorf("error during initial setup: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := runSetupPhase(ctx, "Configuring system locale", func() error {
		if err := setup.ConfigureLocale(ctx); err != nil {
			return fmt.Errorf("error configuring locale: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := runSetupPhase(ctx, "Installing Python runtime", func() error {
		if err := setup.PythonVenv(ctx, verbose); err != nil {
			return fmt.Errorf("error setting up Python venv: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := runSetupPhase(ctx, "Preparing Saltbox repository", func() error {
		if err := setup.SaltboxRepo(ctx, verbose, branch); err != nil {
			return fmt.Errorf("error setting up Saltbox repository: %w", err)
		}
		if err := setup.InitializeGitHooks(ctx); err != nil {
			return fmt.Errorf("error initializing Git hooks: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := runSetupPhase(ctx, "Installing Ansible dependencies", func() error {
		if err := setup.InstallPipDependencies(ctx, verbose); err != nil {
			return fmt.Errorf("error installing pip dependencies: %w", err)
		}
		if err := setup.CopyRequiredBinaries(ctx); err != nil {
			return fmt.Errorf("error copying binaries: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func runSetupPhase(ctx context.Context, name string, task spinners.TaskFunc) error {
	return spinners.RunTaskWithSpinnerCustomContext(ctx, spinners.SpinnerOptions{
		TaskName:        name,
		StopMessage:     name + " completed",
		StopFailMessage: name,
	}, task)
}

func init() {
	rootCmd.AddCommand(setupCmd)
	setupCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	setupCmd.PersistentFlags().StringP("branch", "b", "master", "Branch to use for Saltbox repository")
}
