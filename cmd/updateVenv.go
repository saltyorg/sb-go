package cmd

import (
	"context"
	"fmt"

	"github.com/saltyorg/sb-go/host"
	"github.com/saltyorg/sb-go/python"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/spf13/cobra"
)

func newUpdateVenvCommand() *cobra.Command {
	var verbose bool
	updateVenvCmd := &cobra.Command{
		Use:    "update-venv",
		Short:  "Update the Ansible virtual environment from the current Saltbox checkout",
		Long:   `Update the Ansible virtual environment from the current Saltbox checkout without updating Git repositories`,
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
	saltboxUser, err := host.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: verbose})
	return runner.Run(ctx, updateVenvTaskSpec(), func(ctx context.Context, task *terminal.Task) error {
		if err := python.ManageAnsibleVenv(ctx, task, false, saltboxUser, verbose); err != nil {
			return fmt.Errorf("error managing Ansible venv: %w", err)
		}
		return nil
	})
}

func updateVenvTaskSpec() terminal.TaskSpec {
	return terminal.TaskSpec{
		Running:      "Checking Ansible virtual environment for updates",
		Success:      "Ansible virtual environment is ready",
		Failure:      "Ansible virtual environment update check",
		ChildDisplay: terminal.RetainChildTasks,
	}
}
