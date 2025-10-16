package utils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/executor"
	"github.com/saltyorg/sb-go/internal/ubuntu"

	"gopkg.in/yaml.v3"
)

// RelaunchAsRoot relaunches the current process with sudo and returns the exit code.
// Returns the exit code from the sudo subprocess and an error if execution failed.
// The caller should exit with the returned exit code.
func RelaunchAsRoot(ctx context.Context) (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 1, fmt.Errorf("failed to get executable path: %w", err)
	}

	args := os.Args[1:] // Exclude the program name itself.
	sudoArgs := append([]string{executable}, args...)

	result, err := executor.Run(ctx, "sudo",
		executor.WithArgs(sudoArgs...),
		executor.WithOutputMode(executor.OutputModeInteractive),
	)

	// Return the exit code from the sudo subprocess, regardless of whether there was an error
	// This ensures the parent process exits with the same code as the child process
	return result.ExitCode, err
}

// GetSaltboxUser retrieves the Saltbox user from accounts.yml.
func GetSaltboxUser() (string, error) {
	data, err := os.ReadFile(constants.SaltboxAccountsPath)
	if err != nil {
		return "", fmt.Errorf("failed to read accounts.yml: %w", err)
	}

	var accounts map[string]any
	err = yaml.Unmarshal(data, &accounts)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal accounts.yml: %w", err)
	}

	user, ok := accounts["user"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("user section not found in accounts.yml")
	}

	userName, ok := user["name"].(string)
	if !ok {
		return "", fmt.Errorf("user.name not found in accounts.yml")
	}

	return userName, nil
}

// CheckUbuntuSupport checks if the current Ubuntu version is supported.
func CheckUbuntuSupport() error {
	supportedVersions := constants.GetSupportedUbuntuReleases()
	if err := ubuntu.CheckSupport(supportedVersions); err != nil {
		return fmt.Errorf("UNSUPPORTED OS - Install cancelled: %w. Supported OS versions: %s", err, strings.Join(supportedVersions, ", "))
	}

	osRelease, _ := ubuntu.ParseOSRelease("/etc/os-release")
	versionID := osRelease["VERSION_ID"]
	if slices.Contains(supportedVersions, versionID) {
		return nil // Supported version found
	}
	return fmt.Errorf("warning: Could not determine specific support level after successful OS check")
}

// CheckArchitecture checks if the CPU architecture is supported.
func CheckArchitecture(ctx context.Context) error {
	// Create a context with timeout for the command
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	result, err := executor.Run(cmdCtx, "uname",
		executor.WithArgs("-m"),
		executor.WithOutputMode(executor.OutputModeCombined),
	)
	if err != nil {
		// Return the error but include the output for debugging.  No longer continuing.
		return fmt.Errorf("error getting architecture: %v, output: %s", err, strings.TrimSpace(string(result.Combined)))
	}

	arch := strings.TrimSpace(string(result.Combined))
	x8664regex := regexp.MustCompile(`(x86_64)$`)

	if x8664regex.MatchString(arch) {
		return nil // Supported architecture
	} else {
		return fmt.Errorf("UNSUPPORTED CPU Architecture - Install cancelled: %s is not supported. Supported: x86_64", arch)
	}
}

// CheckLXC checks if the system is running inside an LXC container.
func CheckLXC(ctx context.Context) error {
	// Create a context with timeout for the command
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	result, err := executor.Run(cmdCtx, "systemd-detect-virt",
		executor.WithArgs("-c"),
		executor.WithOutputMode(executor.OutputModeCombined),
	)

	// systemd-detect-virt returns "none" when *not* in a container, and an exit code of 0
	// If there is an error running the command, err != nil, *but* the output *might* also
	// be "none". We only want to return an error if the command itself failed to run,
	// not if it successfully ran and detected "none"
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			// If it's an ExitError, and the output *isn't* none, then we have a problem
			if strings.TrimSpace(string(result.Combined)) != "none" {
				return fmt.Errorf("could not detect virtualization using systemd-detect-virt: %v, output: %s", err, strings.TrimSpace(string(result.Combined)))
			} else {
				// If the output is "none", even if there was an exit error, we treat it like not being in a container
				return nil
			}
		}
		// If it's not an ExitError (some other error), we have a real issue
		return fmt.Errorf("could not detect virtualization using systemd-detect-virt: %v, output: %s", err, strings.TrimSpace(string(result.Combined)))
	}

	// If the command succeeds, check the output
	virtType := strings.ToLower(strings.TrimSpace(string(result.Combined)))
	if virtType == "lxc" {
		return fmt.Errorf("UNSUPPORTED VIRTUALIZATION - Install cancelled: Running in an LXC container is not supported")
	}

	return nil // No error: not running in LXC
}

// CheckDesktopEnvironment checks if a desktop environment is installed.
func CheckDesktopEnvironment(ctx context.Context) error {
	// Create a context with timeout for the command
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	result, err := executor.Run(cmdCtx, "dpkg",
		executor.WithArgs("-l", "ubuntu-desktop"),
		executor.WithOutputMode(executor.OutputModeDiscard),
	)
	if err == nil {
		return fmt.Errorf("UNSUPPORTED DESKTOP INSTALL - Install cancelled: Only Ubuntu Server is supported")
	}
	// If there is an error other than the package being missing
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		// non-zero exit code means package not installed, which is what we want
		if exitError.ExitCode() == 1 {
			return nil
		} else {
			return fmt.Errorf("dpkg command failed with unexpected exit code: %d, error: %w", result.ExitCode, err)
		}
	}
	return fmt.Errorf("unexpected error checking for desktop environment: %w", err)
}
