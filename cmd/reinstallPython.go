package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/saltyorg/sb-go/spinners"
	"github.com/saltyorg/sb-go/utils"
	"github.com/saltyorg/sb-go/venv"

	"github.com/spf13/cobra"
)

// reinstallPythonCmd represents the reinstallPython command
var reinstallPythonCmd = &cobra.Command{
	Use:   "reinstall-python",
	Short: "Reinstall the deadsnakes Python version used by Saltbox and related Ansible virtual environment",
	Long:  `Reinstall the deadsnakes Python version used by Saltbox and related Ansible virtual environment`,
	RunE: func(cmd *cobra.Command, args []string) error {
		verbose, _ := cmd.Flags().GetBool("verbose")
		return handleReinstallPython(verbose)
	},
}

func init() {
	rootCmd.AddCommand(reinstallPythonCmd)
	reinstallPythonCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
}

func handleReinstallPython(verbose bool) error {
	// Set verbose mode for spinners
	spinners.SetVerboseMode(verbose)

	release, err := detectOSRelease()
	if err != nil {
		return fmt.Errorf("error detecting OS release: %w", err)
	}

	if release == "focal" || release == "jammy" {
		_ = spinners.RunInfoSpinner("Removing Python 3.12 packages and recreating Ansible venv.")
		if err := removePython(verbose); err != nil {
			return fmt.Errorf("error removing Python packages: %w", err)
		}

		saltboxUser, err := utils.GetSaltboxUser()
		if err != nil {
			return fmt.Errorf("error getting saltbox user: %w", err)
		}

		if err := venv.ManageAnsibleVenv(true, saltboxUser, verbose); err != nil {
			return fmt.Errorf("error managing Ansible venv: %w", err)
		}
	} else {
		fmt.Println("This command is only for Ubuntu 20.04 and 22.04")
		os.Exit(1)
	}
	return nil
}

func detectOSRelease() (string, error) {
	cmd := exec.Command("lsb_release", "-cs")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error running lsb_release: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func removePython(verbose bool) error {
	packages := []string{
		"libpython3.12-minimal",
		"python3.12-minimal",
		"libpython3.12",
		"libpython3.12-dev",
		"python3.12",
		"python3.12-dev",
		"python3.12-venv",
		"libpython3.12-stdlib",
	}

	// Check if packages are installed first
	var missingPackages []string
	for _, pkg := range packages {
		isInstalled, err := isPkgInstalled(pkg)
		if err != nil {
			return fmt.Errorf("error checking if %s is installed: %w", pkg, err)
		}
		if !isInstalled {
			missingPackages = append(missingPackages, pkg)
		}
	}

	if len(missingPackages) == len(packages) {
		// No packages to remove, but we'll continue execution
		_ = spinners.RunInfoSpinner("None of the Python packages are installed, skipping removal step")
		return nil
	}

	if verbose {
		if len(missingPackages) > 0 {
			_ = spinners.RunInfoSpinner(fmt.Sprintf("Note: The following packages are not installed: %s", strings.Join(missingPackages, ", ")))
			_ = spinners.RunInfoSpinner("Proceeding with removal of installed packages...")
		}
	}

	// Filter out missing packages from the removal list
	var packagesToRemove []string
	for _, pkg := range packages {
		if !contains(missingPackages, pkg) {
			packagesToRemove = append(packagesToRemove, pkg)
		}
	}

	cmd := exec.Command("apt", append([]string{"remove", "-y"}, packagesToRemove...)...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error removing Python packages: %s, stderr: %s", err, stderr.String())
	}

	return nil
}

// isPkgInstalled checks if a package is installed using dpkg-query
func isPkgInstalled(pkgName string) (bool, error) {
	// Use dpkg-query to check if the package is installed
	cmd := exec.Command("dpkg-query", "--show", "--showformat='${Status}'", pkgName)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Package is not installed or command failed
		return false, nil // This is expected for not-installed packages
	}

	// Check if the package is actually installed
	status := stdout.String()
	return strings.Contains(status, "install ok installed"), nil
}

// contains checks if a string is in a slice
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
