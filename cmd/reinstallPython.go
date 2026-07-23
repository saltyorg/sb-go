package cmd

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/saltyorg/sb-go/internal/apt"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/spinners"
	"github.com/saltyorg/sb-go/internal/utils"
	"github.com/saltyorg/sb-go/internal/uv"
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
		Running:      fmt.Sprintf("Reinstalling Python %s and Ansible environment", constants.AnsibleVenvPythonVersion),
		Success:      fmt.Sprintf("Python %s and Ansible environment reinstalled", constants.AnsibleVenvPythonVersion),
		Failure:      "Python and Ansible environment reinstall",
		ChildDisplay: spinners.RetainChildTasks,
	}
}

func reinstallPython(ctx context.Context, task *spinners.Task, verbose bool) error {
	// Update apt cache
	if err := task.RunStreaming(ctx, spinners.TaskSpec{Running: "Updating apt package cache"}, func(taskCtx context.Context) error {
		updateCache := apt.UpdatePackageLists(taskCtx, verbose)
		return updateCache()
	}); err != nil {
		return fmt.Errorf("error updating apt cache: %w", err)
	}

	// Ensure uv is installed
	if err := task.RunStreaming(ctx, spinners.TaskSpec{Running: "Ensuring uv is installed"}, func(taskCtx context.Context) error {
		return uv.DownloadAndInstallUV(taskCtx, verbose)
	}); err != nil {
		return fmt.Errorf("error installing uv: %w", err)
	}

	// Create /srv/python directory if it doesn't exist
	if err := task.Run(ctx, spinners.TaskSpec{Running: fmt.Sprintf("Creating directory %s", constants.PythonInstallDir)}, func(context.Context, *spinners.Task) error {
		return os.MkdirAll(constants.PythonInstallDir, 0755)
	}); err != nil {
		return fmt.Errorf("error creating python install dir: %w", err)
	}

	// Check if Python is already installed
	pythonInstalled := false
	if err := task.Run(ctx, spinners.TaskSpec{Running: "Checking for installed Python versions"}, func(context.Context, *spinners.Task) error {
		versions, err := uv.ListInstalledPythons(ctx)
		if err == nil {
			if slices.Contains(versions, constants.AnsibleVenvPythonVersion) {
				pythonInstalled = true
			}
		}
		return nil // Don't fail if listing fails, we'll try to install anyway
	}); err != nil {
		return err
	}

	// Uninstall existing Python if installed
	if pythonInstalled {
		if err := task.RunStreaming(ctx, spinners.TaskSpec{Running: fmt.Sprintf("Uninstalling existing Python %s", constants.AnsibleVenvPythonVersion)}, func(taskCtx context.Context) error {
			return uv.UninstallPython(taskCtx, constants.AnsibleVenvPythonVersion, verbose)
		}); err != nil {
			return fmt.Errorf("error uninstalling Python: %w", err)
		}
	}

	// Install Python using uv
	if err := task.RunStreaming(ctx, spinners.TaskSpec{Running: fmt.Sprintf("Installing Python %s using uv", constants.AnsibleVenvPythonVersion)}, func(taskCtx context.Context) error {
		return uv.InstallPython(taskCtx, constants.AnsibleVenvPythonVersion, verbose)
	}); err != nil {
		return fmt.Errorf("error installing Python: %w", err)
	}

	// Get saltbox user
	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	// Recreate Ansible venv
	if err := task.Run(ctx, reinstallPythonVenvTaskSpec(), func(ctx context.Context, venvTask *spinners.Task) error {
		return venv.ManageAnsibleVenv(ctx, venvTask, true, saltboxUser, verbose)
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
