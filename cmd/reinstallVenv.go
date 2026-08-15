package cmd

import (
	"context"
	"fmt"

	"github.com/saltyorg/sb-go/host"
	"github.com/saltyorg/sb-go/python"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/spf13/cobra"
)

// reinstallVenvCmd represents the reinstall-venv command
func newReinstallVenvCommand() *cobra.Command {
	var verbose bool
	reinstallVenvCmd := &cobra.Command{
		Use:   "reinstall-venv",
		Short: "Reinstall the Ansible virtual environment",
		Long:  `Reinstall the Ansible virtual environment`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			return handleReinstallVenv(ctx, verbose)
		},
	}
	reinstallVenvCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	return reinstallVenvCmd
}

func addReinstallVenvCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newReinstallVenvCommand())
}

// handleReinstallVenv handles the reinstallation of the Ansible virtual environment.
func handleReinstallVenv(ctx context.Context, verbose bool) error {
	runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: verbose})

	// Get Saltbox user
	saltboxUser, err := host.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	// Manage Ansible venv with the force recreate flag set to true
	// This function already has internal spinners
	return runner.Run(ctx, terminal.TaskSpec{
		Running:      "Reinstalling Ansible virtual environment",
		Success:      "Ansible virtual environment reinstalled",
		ChildDisplay: terminal.RetainChildTasks,
	}, func(ctx context.Context, task *terminal.Task) error {
		if err := python.ManageAnsibleVenv(ctx, task, true, saltboxUser, verbose); err != nil {
			return fmt.Errorf("error managing Ansible venv: %w", err)
		}
		return nil
	})
}
