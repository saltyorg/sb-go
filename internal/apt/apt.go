package apt

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/executor"
)

// InstallPackage returns a function that installs one or more apt packages using "apt-get install".
// When executed, the returned function builds a command that includes:
// - "sudo apt-get install -y" to install packages non-interactively.
// - The provided package names appended individually.
// The function sets the environment variable "DEBIAN_FRONTEND=noninteractive" to suppress interactive prompts.
// When verbose is true, all output is streamed to console. When false, stdout is discarded but stderr
// is captured for error reporting, avoiding conflicts with spinner frameworks.
// In case of an error, the returned function provides a detailed error message including the exit code and stderr output.
// The context parameter allows for cancellation of the installation process.
func InstallPackage(ctx context.Context, packages []string, verbose bool) func() error {
	return func() error {
		// Build the command arguments starting with "apt-get install -y"
		args := append([]string{"apt-get", "install", "-y"}, packages...)

		// Run the command with the unified executor
		err := executor.RunVerbose(ctx, "sudo", args, verbose,
			executor.WithInheritEnv("DEBIAN_FRONTEND=noninteractive"))

		// Handle command execution errors.
		if err != nil {
			packageList := strings.Join(packages, ", ")
			return fmt.Errorf("failed to install packages '%s': %w", packageList, err)
		}

		// On a successful installation, print a success message if verbose.
		if verbose {
			packageList := strings.Join(packages, ", ")
			fmt.Printf("Packages '%s' installed successfully.\n", packageList)
		}

		return nil
	}
}

// UpdatePackageLists returns a function that updates the system's apt package lists.
// When executed, it runs the "sudo apt-get update" command with the non-interactive environment.
// The verbose flag determines whether the command output is streamed to the console or discarded.
// If the command fails, a detailed error message is returned, including the exit code.
// The context parameter allows for cancellation of the update process.
//
// This function implements retry logic with exponential backoff to handle transient mirror sync
// failures that can occur during CI runs. If apt-get update fails and the error message indicates
// a mirror sync issue (e.g., "Mirror sync in progress?", "File has unexpected size"), it will
// retry up to 3 times with delays of 5s, 10s, and 20s between attempts.
func UpdatePackageLists(ctx context.Context, verbose bool) func() error {
	return func() error {
		const maxRetries = 3
		const initialDelay = 5 * time.Second

		var lastErr error
		delay := initialDelay

		for attempt := 1; attempt <= maxRetries; attempt++ {
			// Run the command with the unified executor
			err := executor.RunVerbose(ctx, "sudo", []string{"apt-get", "update"}, verbose,
				executor.WithInheritEnv("DEBIAN_FRONTEND=noninteractive"))

			// Success - return immediately
			if err == nil {
				if verbose {
					fmt.Println("Package lists updated successfully.")
				}
				return nil
			}

			// Save the error for potential retry
			lastErr = err

			// Check if this is a transient mirror sync error worth retrying
			errStr := err.Error()
			isMirrorSyncError := strings.Contains(errStr, "Mirror sync in progress") ||
				strings.Contains(errStr, "File has unexpected size") ||
				strings.Contains(errStr, "Hashes of expected file")

			// If it's not a mirror sync error, or we've exhausted retries, fail immediately
			if !isMirrorSyncError || attempt == maxRetries {
				break
			}

			// Log retry attempt if verbose
			if verbose {
				fmt.Printf("apt-get update failed (attempt %d/%d), retrying in %v due to transient mirror sync error...\n",
					attempt, maxRetries, delay)
			}

			// Wait before retrying
			select {
			case <-ctx.Done():
				return fmt.Errorf("failed to update package lists: context cancelled during retry: %w", ctx.Err())
			case <-time.After(delay):
				// Double the delay for next attempt (exponential backoff: 5s -> 10s -> 20s)
				delay *= 2
			}
		}

		// All retries exhausted or non-retryable error
		return fmt.Errorf("failed to update package lists: %w", lastErr)
	}
}

// AddAptRepositories configures the system's apt repositories based on the Ubuntu release codename.
// It first retrieves the current Ubuntu codename using "lsb_release -sc".
// Then it resets the repository configuration by removing and recreating the "/etc/apt/sources.list.d/" directory.
// Depending on the codename (e.g., matching "jammy" or "noble"), it adds a predefined list of repository entries
// to the main sources file ("/etc/apt/sources.list") using the helper function addRepo.
// If the release codename is unsupported or any step fails, an error is returned.
// The context parameter is used for external command execution but not for local file I/O
// operations, as Go's standard library does not provide context-aware file operations.
//
//goland:noinspection HttpUrlsUsage
func AddAptRepositories(ctx context.Context) error {
	// Get the Ubuntu release codename.
	result, err := executor.Run(ctx, "lsb_release",
		executor.WithArgs("-sc"))
	if err != nil {
		return fmt.Errorf("error getting Ubuntu release codename: %w", err)
	}
	release := strings.TrimSpace(string(result.Combined))

	sourcesFile := "/etc/apt/sources.list"

	// Define regex patterns to identify specific Ubuntu releases.
	jammyRegex := regexp.MustCompile(`(jammy)$`)
	nobleRegex := regexp.MustCompile(`(noble)$`)

	// Remove repository configuration files, but preserve ubuntu.sources on Noble
	sourcesDir := "/etc/apt/sources.list.d/"
	entries, err := os.ReadDir(sourcesDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error reading %s: %w", sourcesDir, err)
	}

	for _, entry := range entries {
		filePath := filepath.Join(sourcesDir, entry.Name())
		// On Noble, skip deleting ubuntu.sources
		if nobleRegex.MatchString(release) && entry.Name() == "ubuntu.sources" {
			continue
		}
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("error removing %s: %w", filePath, err)
		}
	}

	// Based on the release codename, select and add the appropriate repository lines.
	if jammyRegex.MatchString(release) {
		repos := []string{
			"deb http://archive.ubuntu.com/ubuntu/ " + release + " main",
			"deb http://archive.ubuntu.com/ubuntu/ " + release + " universe",
			"deb http://archive.ubuntu.com/ubuntu/ " + release + " restricted",
			"deb http://archive.ubuntu.com/ubuntu/ " + release + " multiverse",
		}
		for _, repo := range repos {
			if err := addRepo(repo, sourcesFile); err != nil {
				return err
			}
		}
	} else if nobleRegex.MatchString(release) {
		repos := []string{
			"deb http://archive.ubuntu.com/ubuntu/ " + release + " main restricted universe multiverse",
			"deb http://archive.ubuntu.com/ubuntu/ " + release + "-updates main restricted universe multiverse",
			"deb http://archive.ubuntu.com/ubuntu/ " + release + "-backports main restricted universe multiverse",
			"deb http://security.ubuntu.com/ubuntu " + release + "-security main restricted universe multiverse",
		}
		for _, repo := range repos {
			if err := addRepo(repo, sourcesFile); err != nil {
				return err
			}
		}
	} else {
		return fmt.Errorf("unsupported Ubuntu release: %s", release)
	}

	return nil
}

// addRepo is a helper function that appends a repository line to the specified sources file.
// It opens the file in append mode (creating it if necessary), writes the repository line followed by a newline,
// and then flushes the write buffer. It returns an error if any file operation fails.
func addRepo(repoLine, sourcesFile string) error {
	file, err := os.OpenFile(sourcesFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("error opening %s: %w", sourcesFile, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	_, err = writer.WriteString(repoLine + "\n")
	if err != nil {
		return fmt.Errorf("error writing to %s: %w", sourcesFile, err)
	}
	return writer.Flush()
}

// AddPPA returns a function that adds a Personal Package Archive (PPA) to the system using "add-apt-repository".
// When executed, the returned function constructs the command "sudo add-apt-repository <ppa> --yes"
// and runs it with non-interactive settings. The verbose flag controls whether command output is streamed
// directly to the console or discarded.
// If the command fails, an error is returned with details including the exit code.
// The context parameter allows for cancellation of the operation.
func AddPPA(ctx context.Context, ppa string, verbose bool) func() error {
	return func() error {
		// Run the command with the unified executor
		err := executor.RunVerbose(ctx, "sudo", []string{"add-apt-repository", ppa, "--yes"}, verbose,
			executor.WithInheritEnv("DEBIAN_FRONTEND=noninteractive"))

		// Handle errors during PPA addition.
		if err != nil {
			return fmt.Errorf("failed to add PPA '%s': %w", ppa, err)
		}

		// Print a success message if in verbose mode.
		if verbose {
			fmt.Printf("PPA '%s' added successfully.\n", ppa)
		}

		return nil
	}
}
