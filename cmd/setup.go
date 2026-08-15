package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/saltyorg/sb-go/git"
	"github.com/saltyorg/sb-go/host"
	"github.com/saltyorg/sb-go/layout"
	"github.com/saltyorg/sb-go/python"
	"github.com/saltyorg/sb-go/saltbox"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/spf13/cobra"
)

// setupCmd represents the setup command
func newSetupCommand() *cobra.Command {
	opts := struct {
		verbose bool
		branch  string
		force   bool
	}{}
	setupCmd := &cobra.Command{
		Use:    "setup",
		Short:  "Install Saltbox and its dependencies",
		Long:   `Install Saltbox and its dependencies`,
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: opts.verbose})

			if err := checkSetupAllowed(layout.SaltboxRepoPath, opts.force); err != nil {
				return err
			}

			selectedBranch := opts.branch
			if _, err := os.Stat(layout.SaltboxRepoPath + "/.git"); err == nil {
				selectedBranch, err = git.ResolveUpdateBranch(ctx, runner, layout.SaltboxRepoPath, opts.branch, nil, "Saltbox")
				if err != nil {
					return err
				}
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("inspect existing Saltbox Git repository: %w", err)
			}

			return runner.Run(ctx, terminal.TaskSpec{
				Running: "Installing Saltbox",
				Success: "Saltbox installation completed",
				Failure: "Saltbox installation",
			}, func(ctx context.Context, task *terminal.Task) error {
				return runSetup(ctx, task, opts.verbose, selectedBranch)
			})
		},
	}
	setupCmd.PersistentFlags().BoolVarP(&opts.verbose, "verbose", "v", false, "Enable verbose output")
	setupCmd.PersistentFlags().StringVarP(&opts.branch, "branch", "b", "master", "Branch to use for Saltbox repository")
	setupCmd.PersistentFlags().BoolVar(&opts.force, "force", false, "Allow setup to run when an existing Saltbox installation is present")
	return setupCmd
}

func checkSetupAllowed(repoPath string, force bool) error {
	info, err := os.Lstat(repoPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect existing Saltbox installation: %w", err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s exists but is a symlink", repoPath)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s exists but is not a directory", repoPath)
	}
	if !force {
		return fmt.Errorf("Saltbox setup already exists at %s; setup is intended for fresh installations only (use --force to run it anyway)", repoPath)
	}

	return nil
}

func runSetup(ctx context.Context, task *terminal.Task, verbose bool, branch string) error {
	if err := runSetupPhase(ctx, task, "Checking system compatibility", func(ctx context.Context, phase *terminal.Task) error {
		if err := phase.Run(ctx, terminal.TaskSpec{Running: "Checking Ubuntu version"}, func(context.Context, *terminal.Task) error {
			return host.CheckUbuntuSupport()
		}); err != nil {
			return err
		}

		if err := phase.Run(ctx, terminal.TaskSpec{Running: "Checking CPU architecture"}, func(context.Context, *terminal.Task) error {
			return host.CheckArchitecture(ctx)
		}); err != nil {
			return err
		}

		if err := phase.Run(ctx, terminal.TaskSpec{Running: "Checking for LXC container"}, func(context.Context, *terminal.Task) error {
			return host.CheckLXC(ctx)
		}); err != nil {
			return err
		}

		if err := phase.Run(ctx, terminal.TaskSpec{Running: "Checking for desktop environment"}, func(context.Context, *terminal.Task) error {
			return host.CheckDesktopEnvironment(ctx)
		}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	if err := runSetupPhase(ctx, task, "Installing system prerequisites", func(ctx context.Context, phase *terminal.Task) error {
		if err := saltbox.InitialSetup(ctx, phase, verbose); err != nil {
			return fmt.Errorf("error during initial setup: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := runSetupPhase(ctx, task, "Configuring system locale", func(ctx context.Context, phase *terminal.Task) error {
		if err := saltbox.ConfigureLocale(ctx, phase); err != nil {
			return fmt.Errorf("error configuring locale: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := runSetupPhase(ctx, task, "Preparing Saltbox repository", func(ctx context.Context, phase *terminal.Task) error {
		if err := saltbox.SaltboxRepo(ctx, phase, verbose, branch); err != nil {
			return fmt.Errorf("error setting up Saltbox repository: %w", err)
		}
		if err := saltbox.InitializeGitHooks(ctx, phase); err != nil {
			return fmt.Errorf("error initializing Git hooks: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := runSetupPhase(ctx, task, "Installing Python runtime", func(ctx context.Context, phase *terminal.Task) error {
		saltboxUser, err := host.GetSaltboxUser()
		if err != nil {
			return fmt.Errorf("get Saltbox user: %w", err)
		}
		return python.Reconcile(ctx, phase, python.Options{SaltboxUser: saltboxUser, Verbose: verbose})
	}); err != nil {
		return err
	}
	return nil
}

func runSetupPhase(
	ctx context.Context,
	parent *terminal.Task,
	name string,
	fn func(context.Context, *terminal.Task) error,
) error {
	return parent.Run(ctx, terminal.TaskSpec{
		Running: name,
		Success: name + " completed",
		Failure: name,
	}, fn)
}

func addSetupCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newSetupCommand())
}
