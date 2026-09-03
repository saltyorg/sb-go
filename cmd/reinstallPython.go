package cmd

import (
	"context"
	"fmt"

	"github.com/saltyorg/sb-go/host"
	"github.com/saltyorg/sb-go/python"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/spf13/cobra"
)

// reinstallPythonCmd represents the reinstallPython command
func newReinstallPythonCommand() *cobra.Command {
	var verbose bool
	reinstallPythonCmd := &cobra.Command{
		Use:   "reinstall-python",
		Short: "Reinstall the Python version used by Saltbox and related Ansible virtual environment using uv",
		Long:  `Reinstall the Python version used by Saltbox and related Ansible virtual environment using uv`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			return handleReinstallPython(ctx, verbose)
		},
	}
	reinstallPythonCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	return reinstallPythonCmd
}

func addReinstallPythonCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newReinstallPythonCommand())
}

func handleReinstallPython(ctx context.Context, verbose bool) error {
	runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: verbose})
	return runner.Run(ctx, terminal.TaskSpec{
		Running: "Reinstalling Saltbox Python and Ansible environment",
		Success: "Saltbox Python and Ansible environment reinstalled",
		Failure: "Python and Ansible environment reinstall",
	}, func(ctx context.Context, task *terminal.Task) error {
		return reinstallPython(ctx, task, verbose)
	})
}

func reinstallPython(ctx context.Context, task *terminal.Task, verbose bool) error {
	// Get saltbox user
	saltboxUser, err := host.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	if err := task.Run(ctx, terminal.TaskSpec{
		Running: "Recreating Ansible virtual environment",
		Success: "Ansible virtual environment recreated",
		Failure: "Ansible virtual environment",
	}, func(ctx context.Context, venvTask *terminal.Task) error {
		return python.Reconcile(ctx, venvTask, python.Options{
			ForceVenv:   true,
			ForcePython: true,
			SaltboxUser: saltboxUser,
			Verbose:     verbose,
		})
	}); err != nil {
		return fmt.Errorf("error managing Ansible venv: %w", err)
	}

	return nil
}
