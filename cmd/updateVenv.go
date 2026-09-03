package cmd

import (
	"context"
	"fmt"

	"github.com/saltyorg/sb-go/host"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/spf13/cobra"
)

func newUpdateVenvCommand() *cobra.Command {
	var verbose bool
	updateVenvCmd := &cobra.Command{
		Use:    "update-venv",
		Short:  "Update the Ansible virtual environment and saltbox.fact",
		Long:   `Update the Ansible virtual environment from the current Saltbox checkout and refresh saltbox.fact without updating Git repositories`,
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleUpdateVenv(cmd.Context(), verbose)
		},
	}
	updateVenvCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	return updateVenvCmd
}

func addUpdateVenvCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newUpdateVenvCommand())
}

func handleUpdateVenv(ctx context.Context, verbose bool) error {
	runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: verbose})
	return handleUpdateVenvWith(ctx, runner, verbose, host.GetSaltboxUser, reconcileSaltboxRuntime)
}

func handleUpdateVenvWith(
	ctx context.Context,
	runner *terminal.Runner,
	verbose bool,
	getSaltboxUser func() (string, error),
	reconcileRuntime func(context.Context, *terminal.Task, string, bool) error,
) error {
	saltboxUser, err := getSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	return runner.Run(ctx, updateVenvTaskSpec(), func(ctx context.Context, task *terminal.Task) error {
		return reconcileRuntime(ctx, task, saltboxUser, verbose)
	})
}

func updateVenvTaskSpec() terminal.TaskSpec {
	return terminal.TaskSpec{
		Running: "Checking Ansible virtual environment and saltbox.fact for updates",
		Success: "Ansible virtual environment and saltbox.fact are ready",
		Failure: "Saltbox runtime dependency update check",
	}
}
