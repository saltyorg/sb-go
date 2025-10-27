package cmd

import (
	"context"
	"fmt"
	"os"
	"slices"

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
	// Set verbose mode for spinners
	spinners.SetVerboseMode(verbose)

	_ = spinners.RunInfoSpinner(fmt.Sprintf("Reinstalling Python %s using uv and recreating Ansible venv", constants.AnsibleVenvPythonVersion))

	// Ensure uv is installed
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Ensuring uv is installed", func() error {
		return uv.DownloadAndInstallUV(ctx, verbose)
	}); err != nil {
		return fmt.Errorf("error installing uv: %w", err)
	}

	// Create /srv/python directory if it doesn't exist
	if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Creating directory %s", constants.PythonInstallDir), func() error {
		return os.MkdirAll(constants.PythonInstallDir, 0755)
	}); err != nil {
		return fmt.Errorf("error creating python install dir: %w", err)
	}

	// Check if Python is already installed
	pythonInstalled := false
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Checking for installed Python versions", func() error {
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
		if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Uninstalling existing Python %s", constants.AnsibleVenvPythonVersion), func() error {
			return uv.UninstallPython(ctx, constants.AnsibleVenvPythonVersion, verbose)
		}); err != nil {
			return fmt.Errorf("error uninstalling Python: %w", err)
		}
	}

	// Install Python using uv
	if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Installing Python %s using uv", constants.AnsibleVenvPythonVersion), func() error {
		return uv.InstallPython(ctx, constants.AnsibleVenvPythonVersion, verbose)
	}); err != nil {
		return fmt.Errorf("error installing Python: %w", err)
	}

	// Get saltbox user
	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	// Recreate Ansible venv
	if err := venv.ManageAnsibleVenv(ctx, true, saltboxUser, verbose); err != nil {
		return fmt.Errorf("error managing Ansible venv: %w", err)
	}

	return nil
}
