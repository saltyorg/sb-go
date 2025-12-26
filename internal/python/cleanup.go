package python

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saltyorg/sb-go/internal/apt"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/executor"
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
	result, err := executor.Run(ctx, "dpkg-query",
		executor.WithArgs("--show", "--showformat=${Status}", pkgName))
	if err != nil {
		// Package is not installed or command failed
		return false, nil
	}

	// Check if the package is actually installed
	status := string(result.Combined)
	return strings.Contains(status, "install ok installed"), nil
}

// RemoveDeadsnakesRepositories scans /etc/apt/sources.list.d/ for files containing
// deadsnakes repository entries, removes those files, and runs apt update if any files were removed.
func RemoveDeadsnakesRepositories(ctx context.Context, verbose bool) (bool, error) {
	sourcesDir := "/etc/apt/sources.list.d/"

	// Read all files in the sources.list.d directory
	entries, err := os.ReadDir(sourcesDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Directory doesn't exist, nothing to clean up
			return false, nil
		}
		return false, fmt.Errorf("error reading %s: %w", sourcesDir, err)
	}

	var removedFiles []string

	// Check each file for deadsnakes entries
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filePath := filepath.Join(sourcesDir, entry.Name())

		// Read file content
		content, err := os.ReadFile(filePath)
		if err != nil {
			// Skip files we can't read
			if verbose {
				fmt.Printf("Warning: could not read %s: %v\n", filePath, err)
			}
			continue
		}

		// Check if the file contains deadsnakes references
		contentStr := string(content)
		if strings.Contains(contentStr, "deadsnakes") {
			if verbose {
				fmt.Printf("Found deadsnakes repository in: %s\n", filePath)
			}

			// Remove the file
			if err := os.Remove(filePath); err != nil {
				return false, fmt.Errorf("error removing %s: %w", filePath, err)
			}

			removedFiles = append(removedFiles, entry.Name())
			if verbose {
				fmt.Printf("Removed: %s\n", filePath)
			}
		}
	}

	// If we removed any files, run apt update
	if len(removedFiles) > 0 {
		if verbose {
			fmt.Printf("Removed %d deadsnakes repository file(s), running apt update...\n", len(removedFiles))
		}

		updateFunc := apt.UpdatePackageLists(ctx, verbose)
		if err := updateFunc(); err != nil {
			return true, fmt.Errorf("error running apt update after removing repository files: %w", err)
		}

		return true, nil
	}

	return false, nil
}

// RemoveDeadsnakesPackages removes deadsnakes Python packages if they exist.
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

	// Wait for apt lock to be available
	if err := apt.WaitForAptLock(ctx, verbose); err != nil {
		return fmt.Errorf("failed waiting for apt lock: %w", err)
	}

	// --- Step 1: Remove the main installed packages ---
	args := append([]string{"remove", "-y"}, installedPackages...)

	if verbose {
		fmt.Printf("Running command: apt-get %s\n", strings.Join(args, " "))
	}

	if err := executor.RunVerbose(ctx, "apt-get", args, verbose,
		executor.WithInheritEnv("DEBIAN_FRONTEND=noninteractive")); err != nil {
		return fmt.Errorf("error removing Python packages: %w", err)
	}

	// --- Step 2: Run apt autoremove to clean up dependencies ---
	if verbose {
		fmt.Println("Running command: apt-get autoremove -y")
	}

	if err := executor.RunVerbose(ctx, "apt-get", []string{"autoremove", "-y"}, verbose,
		executor.WithInheritEnv("DEBIAN_FRONTEND=noninteractive")); err != nil {
		return fmt.Errorf("error running apt autoremove: %w", err)
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

// CleanupDeadsnakesIfNeeded checks if cleanup is needed and performs it.
// This function removes both deadsnakes packages and their apt repository files.
func CleanupDeadsnakesIfNeeded(ctx context.Context, verbose bool) (bool, error) {
	shouldCleanup, err := ShouldCleanupDeadsnakes()
	if err != nil {
		return false, fmt.Errorf("error checking if cleanup is needed: %w", err)
	}

	if !shouldCleanup {
		return false, nil
	}

	var cleanedUp bool

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

	// Perform package cleanup if packages are installed
	if len(installedPackages) > 0 {
		if err := RemoveDeadsnakesPackages(ctx, pythonVersion, verbose); err != nil {
			return false, fmt.Errorf("error removing deadsnakes packages: %w", err)
		}
		cleanedUp = true
	}

	// Always check for and remove deadsnakes repository files
	reposRemoved, err := RemoveDeadsnakesRepositories(ctx, verbose)
	if err != nil {
		return cleanedUp, fmt.Errorf("error removing deadsnakes repositories: %w", err)
	}

	if reposRemoved {
		cleanedUp = true
	}

	return cleanedUp, nil
}
