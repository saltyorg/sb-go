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
		runner := spinners.NewRunner(spinners.RunnerOptions{Verbose: verbose})

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
			selectedBranch, err = git.ResolveUpdateBranch(ctx, runner, constants.SaltboxRepoPath, branch, nil, "Saltbox")
			if err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect existing Saltbox Git repository: %w", err)
		}

		return runner.Run(ctx, spinners.TaskSpec{
			Running: "Installing Saltbox",
			Success: "Saltbox installation completed",
			Failure: "Saltbox installation",
		}, func(ctx context.Context, task *spinners.Task) error {
			return runSetup(ctx, task, verbose, selectedBranch)
		})
	},
}

func runSetup(ctx context.Context, task *spinners.Task, verbose bool, branch string) error {
	if err := runSetupPhase(ctx, task, "Checking system compatibility", func(ctx context.Context, phase *spinners.Task) error {
		if err := phase.Run(ctx, spinners.TaskSpec{Running: "Checking Ubuntu version"}, func(context.Context, *spinners.Task) error {
			return utils.CheckUbuntuSupport()
		}); err != nil {
			return err
		}

		if err := phase.Run(ctx, spinners.TaskSpec{Running: "Checking CPU architecture"}, func(context.Context, *spinners.Task) error {
			return utils.CheckArchitecture(ctx)
		}); err != nil {
			return err
		}

		if err := phase.Run(ctx, spinners.TaskSpec{Running: "Checking for LXC container"}, func(context.Context, *spinners.Task) error {
			return utils.CheckLXC(ctx)
		}); err != nil {
			return err
		}

		if err := phase.Run(ctx, spinners.TaskSpec{Running: "Checking for desktop environment"}, func(context.Context, *spinners.Task) error {
			return utils.CheckDesktopEnvironment(ctx)
		}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	if err := runSetupPhase(ctx, task, "Installing system prerequisites", func(ctx context.Context, phase *spinners.Task) error {
		if err := setup.InitialSetup(ctx, phase, verbose); err != nil {
			return fmt.Errorf("error during initial setup: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := runSetupPhase(ctx, task, "Configuring system locale", func(ctx context.Context, phase *spinners.Task) error {
		if err := setup.ConfigureLocale(ctx, phase); err != nil {
			return fmt.Errorf("error configuring locale: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := runSetupPhase(ctx, task, "Installing Python runtime", func(ctx context.Context, phase *spinners.Task) error {
		if err := setup.PythonVenv(ctx, phase, verbose); err != nil {
			return fmt.Errorf("error setting up Python venv: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := runSetupPhase(ctx, task, "Preparing Saltbox repository", func(ctx context.Context, phase *spinners.Task) error {
		if err := setup.SaltboxRepo(ctx, phase, verbose, branch); err != nil {
			return fmt.Errorf("error setting up Saltbox repository: %w", err)
		}
		if err := setup.InitializeGitHooks(ctx, phase); err != nil {
			return fmt.Errorf("error initializing Git hooks: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := runSetupPhase(ctx, task, "Installing Ansible dependencies", func(ctx context.Context, phase *spinners.Task) error {
		if err := setup.InstallPipDependencies(ctx, phase, verbose); err != nil {
			return fmt.Errorf("error installing pip dependencies: %w", err)
		}
		if err := setup.CopyRequiredBinaries(ctx, phase); err != nil {
			return fmt.Errorf("error copying binaries: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func runSetupPhase(
	ctx context.Context,
	parent *spinners.Task,
	name string,
	fn func(context.Context, *spinners.Task) error,
) error {
	return parent.Run(ctx, spinners.TaskSpec{
		Running: name,
		Success: name + " completed",
		Failure: name,
	}, fn)
}

func init() {
	rootCmd.AddCommand(setupCmd)
	setupCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	setupCmd.PersistentFlags().StringP("branch", "b", "master", "Branch to use for Saltbox repository")
}
