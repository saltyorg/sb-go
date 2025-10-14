package apt

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// InstallPackage returns a function that installs one or more apt packages using "apt-get install".
// When executed, the returned function builds a command that includes:
// - "sudo apt-get install -y" to install packages non-interactively.
// - The provided package names appended individually.
// The function sets the environment variable "DEBIAN_FRONTEND=noninteractive" to suppress interactive prompts.
// Depending on the verbose flag, command output is either streamed directly to the console or captured in buffers.
// In case of an error, the returned function provides a detailed error message including the exit code and,
// if not in verbose mode, the captured stderr output.
// The context parameter allows for cancellation of the installation process.
func InstallPackage(ctx context.Context, packages []string, verbose bool) func() error {
	return func() error {
		// Build the command arguments starting with "sudo apt-get install -y"
		command := []string{"sudo", "apt-get", "install", "-y"}
		// Append each package name individually to the command.
		command = append(command, packages...) // The ... is crucial here!

		// Create the command to run with context for cancellation support.
		cmd := exec.CommandContext(ctx, command[0], command[1:]...)
		// Set the environment to non-interactive mode.
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")

		var stdoutBuf, stderrBuf bytes.Buffer

		// Configure output based on a verbose flag.
		if verbose {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		} else {
			cmd.Stdout = &stdoutBuf
			cmd.Stderr = &stderrBuf
		}

		err := cmd.Run()

		// Handle command execution errors.
		if err != nil {
			var exitErr *exec.ExitError
			// Create a comma-separated string of package names for error messaging.
			packageList := strings.Join(packages, ", ")
			if errors.As(err, &exitErr) {
				if !verbose {
					return fmt.Errorf("failed to install packages '%s'.\nExit code: %d\nStderr:\n%s",
						packageList, exitErr.ExitCode(), stderrBuf.String())
				}
				return fmt.Errorf("failed to install packages '%s'.\nExit code: %d",
					packageList, exitErr.ExitCode())
			}
			if !verbose {
				return fmt.Errorf("failed to install packages '%s': %w\nStderr:\n%s",
					packageList, err, stderrBuf.String())
			}
			return fmt.Errorf("failed to install packages '%s': %w",
				packageList, err)
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
// The verbose flag determines whether the command output is streamed to the console or captured in buffers.
// If the command fails, a detailed error message is returned, including the exit code and (when not verbose)
// the stderr output.
// The context parameter allows for cancellation of the update process.
func UpdatePackageLists(ctx context.Context, verbose bool) func() error {
	return func() error {
		// Build the command to update package lists.
		command := []string{"sudo", "apt-get", "update"}
		cmd := exec.CommandContext(ctx, command[0], command[1:]...)
		// Set non-interactive mode.
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")

		var stdoutBuf, stderrBuf bytes.Buffer

		// Configure output based on a verbose flag.
		if verbose {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		} else {
			cmd.Stdout = &stdoutBuf
			cmd.Stderr = &stderrBuf
		}

		err := cmd.Run()

		// Handle errors from the update command.
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				if !verbose {
					return fmt.Errorf("failed to update package lists.\nExit code: %d\nStderr:\n%s", exitErr.ExitCode(), stderrBuf.String())
				}
				return fmt.Errorf("failed to update package lists.\nExit code: %d", exitErr.ExitCode())
			}
			if !verbose {
				return fmt.Errorf("failed to update package lists: %w\nStderr:\n%s", err, stderrBuf.String())
			}
			return fmt.Errorf("failed to update package lists: %w", err)
		}

		// Notify success if verbose.
		if verbose {
			fmt.Println("Package lists updated successfully.")
		}

		return nil
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
	cmd := exec.CommandContext(ctx, "lsb_release", "-sc")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error getting Ubuntu release codename: %w", err)
	}
	release := strings.TrimSpace(string(output))

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
// directly to the console or captured in buffers for error reporting.
// If the command fails, an error is returned with details including the exit code and (when not verbose)
// the captured stderr output.
// The context parameter allows for cancellation of the operation.
func AddPPA(ctx context.Context, ppa string, verbose bool) func() error {
	return func() error {
		// Build the command for adding the PPA.
		command := []string{"sudo", "add-apt-repository", ppa, "--yes"}
		cmd := exec.CommandContext(ctx, command[0], command[1:]...)
		// Set non-interactive mode.
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")

		var stdoutBuf, stderrBuf bytes.Buffer
		// Configure command output based on the verbose flag.
		if verbose {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		} else {
			cmd.Stdout = &stdoutBuf
			cmd.Stderr = &stderrBuf
		}

		err := cmd.Run()
		// Handle errors during PPA addition.
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				if !verbose {
					return fmt.Errorf("failed to add PPA '%s'.\nExit code: %d\nStderr:\n%s", ppa, exitErr.ExitCode(), stderrBuf.String())
				}
				return fmt.Errorf("failed to add PPA '%s'.\nExit code: %d", ppa, exitErr.ExitCode())
			}
			if !verbose {
				return fmt.Errorf("failed to add PPA '%s': %w\nStderr:\n%s", ppa, err, stderrBuf.String())
			}
			return fmt.Errorf("failed to add PPA '%s': %w", ppa, err)
		}

		// Print a success message if in verbose mode.
		if verbose {
			fmt.Printf("PPA '%s' added successfully.\n", ppa)
		}

		return nil
	}
}
