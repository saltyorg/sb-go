package cmd

import (
	"context"
	"fmt"

	"github.com/saltyorg/sb-go/internal/spinners"
	"github.com/saltyorg/sb-go/internal/utils"
	"github.com/saltyorg/sb-go/internal/venv"

	"github.com/spf13/cobra"
)

// reinstallPythonCmd represents the reinstallPython command
var reinstallPythonCmd = &cobra.Command{
	Use:   "reinstall-python",
	Short: "Reinstall the Python version used by Saltbox and related Ansible virtual environment using uv",
	Long:  `Reinstall the Python version used by Saltbox and related Ansible virtual environment using uv`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		verbose, _ := cmd.Flags().GetBool("verbose")
		return handleReinstallPython(ctx, verbose)
	},
}

func init() {
	rootCmd.AddCommand(reinstallPythonCmd)
	reinstallPythonCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
}

func handleReinstallPython(ctx context.Context, verbose bool) error {
	runner := spinners.NewRunner(spinners.RunnerOptions{Verbose: verbose})
	return runner.Run(ctx, reinstallPythonTaskSpec(), func(ctx context.Context, task *spinners.Task) error {
		return reinstallPython(ctx, task, verbose)
	})
}

func reinstallPythonTaskSpec() spinners.TaskSpec {
	return spinners.TaskSpec{
		Running:      "Reinstalling Saltbox Python and Ansible environment",
		Success:      "Saltbox Python and Ansible environment reinstalled",
		Failure:      "Python and Ansible environment reinstall",
		ChildDisplay: spinners.RetainChildTasks,
	}
}

func reinstallPython(ctx context.Context, task *spinners.Task, verbose bool) error {
	// Get saltbox user
	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	if err := task.Run(ctx, reinstallPythonVenvTaskSpec(), func(ctx context.Context, venvTask *spinners.Task) error {
		return venv.Reconcile(ctx, venvTask, venv.Options{
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

func reinstallPythonVenvTaskSpec() spinners.TaskSpec {
	return spinners.TaskSpec{
		Running:      "Recreating Ansible virtual environment",
		Success:      "Ansible virtual environment recreated",
		Failure:      "Ansible virtual environment",
		ChildDisplay: spinners.RetainChildTasks,
	}
}
