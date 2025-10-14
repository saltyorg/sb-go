package python

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/ubuntu"
)

// DeadsnakesPackages returns the list of deadsnakes Python packages to remove
func DeadsnakesPackages(pythonVersion string) []string {
	return []string{
		fmt.Sprintf("python%s", pythonVersion),
		fmt.Sprintf("python%s-dev", pythonVersion),
		fmt.Sprintf("python%s-distutils", pythonVersion),
		fmt.Sprintf("python%s-venv", pythonVersion),
		// Additional packages that might be installed
		fmt.Sprintf("libpython%s", pythonVersion),
		fmt.Sprintf("libpython%s-dev", pythonVersion),
		fmt.Sprintf("libpython%s-minimal", pythonVersion),
		fmt.Sprintf("libpython%s-stdlib", pythonVersion),
		fmt.Sprintf("python%s-minimal", pythonVersion),
	}
}

// IsPackageInstalled checks if a package is installed using dpkg-query
func IsPackageInstalled(ctx context.Context, pkgName string) (bool, error) {
	cmd := exec.CommandContext(ctx, "dpkg-query", "--show", "--showformat=${Status}", pkgName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Package is not installed or command failed
		return false, nil
	}

	// Check if the package is actually installed
	status := string(output)
	return strings.Contains(status, "install ok installed"), nil
}

// RemoveDeadsnakesPackages removes deadsnakes Python packages if they exist
func RemoveDeadsnakesPackages(ctx context.Context, pythonVersion string, verbose bool) error {
	packages := DeadsnakesPackages(pythonVersion)

	// Check which packages are installed
	var installedPackages []string
	for _, pkg := range packages {
		isInstalled, err := IsPackageInstalled(ctx, pkg)
		if err != nil {
			return fmt.Errorf("error checking if %s is installed: %w", pkg, err)
		}
		if isInstalled {
			installedPackages = append(installedPackages, pkg)
		}
	}

	// If no packages are installed, return early
	if len(installedPackages) == 0 {
		return nil
	}

	// Remove installed packages
	args := append([]string{"remove", "-y"}, installedPackages...)
	cmd := exec.CommandContext(ctx, "apt", args...)

	if verbose {
		fmt.Printf("Removing packages: %s\n", strings.Join(installedPackages, ", "))
		// Don't attach stdout/stderr in verbose mode for cleaner spinner output
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error removing Python packages: %w", err)
	}

	// Run apt autoremove to clean up dependencies
	autoremoveCmd := exec.CommandContext(ctx, "apt", "autoremove", "-y")
	if err := autoremoveCmd.Run(); err != nil {
		// Don't fail on autoremove error, just log it
		if verbose {
			fmt.Printf("Warning: apt autoremove failed: %v\n", err)
		}
	}

	return nil
}

// ShouldCleanupDeadsnakes checks if the system should clean up deadsnakes packages
// Returns true if the system is Ubuntu 20.04 or 22.04
func ShouldCleanupDeadsnakes() (bool, error) {
	osRelease, err := ubuntu.ParseOSRelease("/etc/os-release")
	if err != nil {
		return false, fmt.Errorf("error parsing OS release: %w", err)
	}

	versionID, ok := osRelease["VERSION_ID"]
	if !ok {
		return false, nil
	}

	// Only clean up on Ubuntu 20.04 (focal) and 22.04 (jammy)
	return versionID == "20.04" || versionID == "22.04", nil
}

// CleanupDeadsnakesIfNeeded checks if cleanup is needed and performs it
func CleanupDeadsnakesIfNeeded(ctx context.Context, verbose bool) (bool, error) {
	shouldCleanup, err := ShouldCleanupDeadsnakes()
	if err != nil {
		return false, fmt.Errorf("error checking if cleanup is needed: %w", err)
	}

	if !shouldCleanup {
		return false, nil
	}

	// Check if any deadsnakes packages are installed
	pythonVersion := constants.AnsibleVenvPythonVersion
	packages := DeadsnakesPackages(pythonVersion)

	var installedPackages []string
	for _, pkg := range packages {
		isInstalled, err := IsPackageInstalled(ctx, pkg)
		if err != nil {
			// Don't fail on check error, just skip
			continue
		}
		if isInstalled {
			installedPackages = append(installedPackages, pkg)
		}
	}

	// If no packages found, no cleanup needed
	if len(installedPackages) == 0 {
		return false, nil
	}

	// Perform cleanup
	if err := RemoveDeadsnakesPackages(ctx, pythonVersion, verbose); err != nil {
		return false, fmt.Errorf("error removing deadsnakes packages: %w", err)
	}

	return true, nil
}
