package utils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/executor"
	"github.com/saltyorg/sb-go/internal/logging"
	"github.com/saltyorg/sb-go/internal/ubuntu"

	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"
)

const (
	// diskSpaceMinFreeBytes is the minimum free space we require on critical filesystems.
	diskSpaceMinFreeBytes uint64 = 2 * 1024 * 1024 * 1024 // 2 GiB
)

type diskUsage struct {
	path           string
	totalBytes     uint64
	availableBytes uint64
	usedPercent    float64
	fsid           string
}

var statfsFunc = unix.Statfs

// RelaunchAsRoot relaunches the current process with sudo and returns the exit code.
// Returns the exit code from the sudo subprocess and an error if execution failed.
// The caller should exit with the returned exit code.
func RelaunchAsRoot() (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 1, fmt.Errorf("failed to get executable path: %w", err)
	}

	args := os.Args[1:] // Exclude the program name itself
	cmd := exec.Command("sudo", append([]string{executable}, args...)...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err = cmd.Run()
	if err != nil {
		// Check if it's an ExitError (non-zero exit code)
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Return the exit code without treating it as an error
			return exitErr.ExitCode(), nil
		}
		// Non-exit error (e.g., sudo not found)
		return 1, fmt.Errorf("failed to execute sudo: %w", err)
	}

	return 0, nil
}

// GetSaltboxUser retrieves the Saltbox user from accounts.yml.
func GetSaltboxUser() (string, error) {
	data, err := os.ReadFile(constants.SaltboxAccountsConfigPath)
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
	return nil
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

// CheckDiskSpace verifies that required paths have enough free space before continuing.
func CheckDiskSpace(paths []string, verbosity int) error {
	if len(paths) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	seenFS := make(map[string]string)

	for _, path := range paths {
		resolvedPath := nearestExistingPath(path)
		if _, ok := seen[resolvedPath]; ok {
			continue
		}
		seen[resolvedPath] = struct{}{}

		usage, err := getDiskUsage(resolvedPath)
		if err != nil {
			return fmt.Errorf("unable to check disk usage for %s: %w", resolvedPath, err)
		}

		if usage.fsid != "" {
			if existing, ok := seenFS[usage.fsid]; ok {
				logging.Debug(verbosity, "Skipping %s (same filesystem as %s)", resolvedPath, existing)
				continue
			}
			seenFS[usage.fsid] = resolvedPath
		}

		logging.Debug(verbosity, "Disk usage for %s: total=%s, available=%s, used=%.1f%%",
			usage.path, formatBytes(usage.totalBytes), formatBytes(usage.availableBytes), usage.usedPercent)

		if usage.availableBytes < diskSpaceMinFreeBytes {
			return diskSpaceError(usage.path, usage.usedPercent, usage.availableBytes)
		}
	}

	return nil
}

func getDiskUsage(path string) (diskUsage, error) {
	var stat unix.Statfs_t
	if err := statfsFunc(path, &stat); err != nil {
		return diskUsage{}, err
	}

	if stat.Blocks == 0 {
		return diskUsage{}, fmt.Errorf("filesystem stats for %s reported zero blocks", path)
	}

	blockSize := uint64(stat.Bsize)
	totalBytes := uint64(stat.Blocks) * blockSize
	availableBytes := uint64(stat.Bavail) * blockSize
	usedPercent := (float64(stat.Blocks-stat.Bavail) / float64(stat.Blocks)) * 100

	return diskUsage{
		path:           path,
		totalBytes:     totalBytes,
		availableBytes: availableBytes,
		usedPercent:    usedPercent,
		fsid:           fsidKey(stat),
	}, nil
}

// nearestExistingPath walks up the directory tree until it finds an existing path.
func nearestExistingPath(path string) string {
	current := filepath.Clean(path)
	for {
		if _, err := os.Stat(current); err == nil {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			return current
		}
		current = parent
	}
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div := uint64(unit)
	exp := 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func diskSpaceError(path string, usedPercent float64, availableBytes uint64) error {
	return fmt.Errorf("INSUFFICIENT DISK SPACE - Install cancelled: %s is %.1f%% full (%s free). Free up space on %s before continuing.",
		path, usedPercent, formatBytes(availableBytes), path)
}

// DiskSpaceError exposes the standard disk space error format for callers that need to force a failure.
func DiskSpaceError(path string, usedPercent float64, availableBytes uint64) error {
	return diskSpaceError(path, usedPercent, availableBytes)
}

func fsidKey(stat unix.Statfs_t) string {
	return fmt.Sprintf("%d:%d", stat.Fsid.Val[0], stat.Fsid.Val[1])
}
