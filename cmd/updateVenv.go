package cmd

import (
	"context"
	"fmt"

	"github.com/saltyorg/sb-go/internal/spinners"
	"github.com/saltyorg/sb-go/internal/utils"
	"github.com/saltyorg/sb-go/internal/venv"

	"github.com/spf13/cobra"
)

var updateVenvCmd = &cobra.Command{
	Use:    "update-venv",
	Short:  "Update the Ansible virtual environment from the current Saltbox checkout",
	Long:   `Update the Ansible virtual environment from the current Saltbox checkout without updating Git repositories`,
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		verbose, _ := cmd.Flags().GetBool("verbose")
		return handleUpdateVenv(cmd.Context(), verbose)
	},
}

func init() {
	rootCmd.AddCommand(updateVenvCmd)
	updateVenvCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
}

func handleUpdateVenv(ctx context.Context, verbose bool) error {
	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	runner := spinners.NewRunner(spinners.RunnerOptions{Verbose: verbose})
	return runner.Run(ctx, updateVenvTaskSpec(), func(ctx context.Context, task *spinners.Task) error {
		if err := venv.ManageAnsibleVenv(ctx, task, false, saltboxUser, verbose); err != nil {
			return fmt.Errorf("error managing Ansible venv: %w", err)
		}
		return nil
	})
}

func updateVenvTaskSpec() spinners.TaskSpec {
	return spinners.TaskSpec{
		Running:      "Checking Ansible virtual environment for updates",
		Success:      "Ansible virtual environment is ready",
		Failure:      "Ansible virtual environment update check",
		ChildDisplay: spinners.RetainChildTasks,
	}
}
