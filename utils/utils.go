package utils

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/saltyorg/sb-go/constants"
	"github.com/saltyorg/sb-go/ubuntu"
	"gopkg.in/yaml.v3"
)

// RelaunchAsRoot relaunches the current process with sudo.
func RelaunchAsRoot() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	args := os.Args[1:] // Exclude the program name itself.

	relaunchCmd := exec.Command("sudo", append([]string{executable}, args...)...)

	relaunchCmd.Stdout = os.Stdout
	relaunchCmd.Stderr = os.Stderr
	relaunchCmd.Stdin = os.Stdin

	if err := relaunchCmd.Run(); err != nil {
		return fmt.Errorf("failed to execute sudo: %w", err)
	}

	return nil
}

// GetSaltboxUser retrieves the Saltbox user from accounts.yml.
func GetSaltboxUser() (string, error) {
	data, err := os.ReadFile(constants.SaltboxAccountsPath)
	if err != nil {
		return "", fmt.Errorf("failed to read accounts.yml: %w", err)
	}

	var accounts map[string]interface{}
	err = yaml.Unmarshal(data, &accounts)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal accounts.yml: %w", err)
	}

	user, ok := accounts["user"].(map[string]interface{})
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
	for _, v := range supportedVersions {
		if versionID == v {
			return nil // Supported version found
		}
	}
	return fmt.Errorf("warning: Could not determine specific support level after successful OS check")
}

// CheckArchitecture checks if the CPU architecture is supported.
func CheckArchitecture() error {
	cmd := exec.Command("uname", "-m")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Return the error but include the output for debugging.  No longer continuing.
		return fmt.Errorf("error getting architecture: %v, output: %s", err, strings.TrimSpace(string(output)))
	}

	arch := strings.TrimSpace(string(output))
	x8664regex := regexp.MustCompile(`(x86_64)$`)

	if x8664regex.MatchString(arch) {
		return nil // Supported architecture
	} else {
		return fmt.Errorf("UNSUPPORTED CPU Architecture - Install cancelled: %s is not supported. Supported: x86_64", arch)
	}
}

// CheckLXC checks if the system is running inside an LXC container.
func CheckLXC() error {
	cmd := exec.Command("systemd-detect-virt", "-c")
	output, err := cmd.CombinedOutput()

	// systemd-detect-virt returns "none" when *not* in a container, and an exit code of 0
	// If there is an error running the command, err != nil, *but* the output *might* also
	// be "none". We only want to return an error if the command itself failed to run,
	// not if it successfully ran and detected "none"
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			// If it's an ExitError, and the output *isn't* none, then we have a problem
			if strings.TrimSpace(string(output)) != "none" {
				return fmt.Errorf("could not detect virtualization using systemd-detect-virt: %v, output: %s", err, strings.TrimSpace(string(output)))
			} else {
				// If the output is "none", even if there was an exit error, we treat it like not being in a container
				return nil
			}
		}
		// If it's not an ExitError (some other error), we have a real issue
		return fmt.Errorf("could not detect virtualization using systemd-detect-virt: %v, output: %s", err, strings.TrimSpace(string(output)))
	}

	// If the command succeeds, check the output
	virtType := strings.ToLower(strings.TrimSpace(string(output)))
	if virtType == "lxc" {
		return fmt.Errorf("UNSUPPORTED VIRTUALIZATION - Install cancelled: Running in an LXC container is not supported")
	}

	return nil // No error: not running in LXC
}

// CheckDesktopEnvironment checks if a desktop environment is installed.
func CheckDesktopEnvironment() error {
	cmd := exec.Command("dpkg", "-l", "ubuntu-desktop")
	err := cmd.Run()
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
			return fmt.Errorf("dpkg command failed with unexpected exit code: %d, error: %w", exitError.ExitCode(), err)
		}
	}
	return fmt.Errorf("unexpected error checking for desktop environment: %w", err)
}
