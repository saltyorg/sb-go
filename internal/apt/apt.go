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
//
// Each attempt has a 2-minute timeout to prevent indefinite hangs on extremely slow mirrors.
// Timeout errors are treated as retryable, just like mirror sync errors.
func UpdatePackageLists(ctx context.Context, verbose bool) func() error {
	return func() error {
		const maxRetries = 3
		const initialDelay = 5 * time.Second
		const attemptTimeout = 2 * time.Minute

		var lastErr error
		delay := initialDelay

		for attempt := 1; attempt <= maxRetries; attempt++ {
			// Create a timeout context for this attempt
			attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)

			// Run the command with the unified executor
			err := executor.RunVerbose(attemptCtx, "sudo", []string{"apt-get", "update"}, verbose,
				executor.WithInheritEnv("DEBIAN_FRONTEND=noninteractive"))

			// Clean up the timeout context
			cancel()

			// Success - return immediately
			if err == nil {
				if verbose {
					fmt.Println("Package lists updated successfully.")
				}
				return nil
			}

			// Save the error for potential retry
			lastErr = err

			// Check if the attempt timed out
			isTimeout := attemptCtx.Err() == context.DeadlineExceeded

			// Check if this is a transient mirror sync error worth retrying
			errStr := err.Error()
			isMirrorSyncError := strings.Contains(errStr, "Mirror sync in progress") ||
				strings.Contains(errStr, "File has unexpected size") ||
				strings.Contains(errStr, "Hashes of expected file")

			// Determine if this error is retryable
			isRetryable := isTimeout || isMirrorSyncError

			// If it's not retryable, or we've exhausted retries, fail immediately
			if !isRetryable || attempt == maxRetries {
				break
			}

			// Log retry attempt if verbose
			if verbose {
				if isTimeout {
					fmt.Printf("apt-get update timed out after %v (attempt %d/%d), retrying in %v...\n",
						attemptTimeout, attempt, maxRetries, delay)
				} else {
					fmt.Printf("apt-get update failed (attempt %d/%d), retrying in %v due to transient mirror sync error...\n",
						attempt, maxRetries, delay)
				}
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
// The verbose flag controls whether informative messages about the configuration process are printed.
//
//goland:noinspection HttpUrlsUsage
func AddAptRepositories(ctx context.Context, verbose bool) error {
	// Get the Ubuntu release codename.
	if verbose {
		fmt.Println("Detecting Ubuntu release codename...")
	}
	result, err := executor.Run(ctx, "lsb_release",
		executor.WithArgs("-sc"))
	if err != nil {
		return fmt.Errorf("error getting Ubuntu release codename: %w", err)
	}
	release := strings.TrimSpace(string(result.Combined))
	if verbose {
		fmt.Printf("Detected Ubuntu release: %s\n", release)
	}

	sourcesFile := "/etc/apt/sources.list"

	// Define regex patterns to identify specific Ubuntu releases.
	jammyRegex := regexp.MustCompile(`(jammy)$`)
	nobleRegex := regexp.MustCompile(`(noble)$`)

	// Remove repository configuration files, but preserve ubuntu.sources on Noble
	sourcesDir := "/etc/apt/sources.list.d/"
	if verbose {
		fmt.Printf("Cleaning up existing repository configuration files in %s\n", sourcesDir)
	}
	entries, err := os.ReadDir(sourcesDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error reading %s: %w", sourcesDir, err)
	}

	removedCount := 0
	for _, entry := range entries {
		filePath := filepath.Join(sourcesDir, entry.Name())
		// On Noble, skip deleting ubuntu.sources
		if nobleRegex.MatchString(release) && entry.Name() == "ubuntu.sources" {
			if verbose {
				fmt.Printf("  Preserving %s (required for Noble)\n", entry.Name())
			}
			continue
		}
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("error removing %s: %w", filePath, err)
		}
		if verbose {
			fmt.Printf("  Removed %s\n", entry.Name())
		}
		removedCount++
	}
	if verbose {
		fmt.Printf("Removed %d repository configuration file(s)\n", removedCount)
	}

	// Based on the release codename, select and add the appropriate repository lines.
	if jammyRegex.MatchString(release) {
		if verbose {
			fmt.Printf("Configuring repositories for Ubuntu %s\n", release)
		}
		repos := []string{
			"deb http://archive.ubuntu.com/ubuntu/ " + release + " main",
			"deb http://archive.ubuntu.com/ubuntu/ " + release + " universe",
			"deb http://archive.ubuntu.com/ubuntu/ " + release + " restricted",
			"deb http://archive.ubuntu.com/ubuntu/ " + release + " multiverse",
		}
		for _, repo := range repos {
			if verbose {
				fmt.Printf("  Adding repository: %s\n", repo)
			}
			if err := addRepo(repo, sourcesFile); err != nil {
				return err
			}
		}
		if verbose {
			fmt.Printf("Successfully configured %d repositories in %s\n", len(repos), sourcesFile)
		}
	} else if nobleRegex.MatchString(release) {
		if verbose {
			fmt.Printf("Configuring repositories for Ubuntu %s (using DEB822 format)\n", release)
		}
		// On Noble, check if the existing ubuntu.sources uses the official archive
		ubuntuSourcesFile := filepath.Join(sourcesDir, "ubuntu.sources")
		if verbose {
			fmt.Printf("Checking existing mirror configuration in %s\n", ubuntuSourcesFile)
		}

		// Read and display the current ubuntu.sources content if verbose
		if verbose {
			if content, err := os.ReadFile(ubuntuSourcesFile); err == nil {
				fmt.Println("\nCurrent ubuntu.sources content:")
				fmt.Println("---")
				fmt.Print(string(content))
				fmt.Println("---")
				fmt.Println()
			} else if !os.IsNotExist(err) {
				fmt.Printf("Warning: Could not read %s: %v\n", ubuntuSourcesFile, err)
			}
		}

		usingArchive, err := isUsingArchiveMirror(ubuntuSourcesFile)
		if err != nil {
			return fmt.Errorf("error checking ubuntu.sources mirror configuration: %w", err)
		}

		// Only add ubuntu-archive.sources if NOT using the official archive mirror
		// (i.e., if using a custom mirror like corporate/regional mirrors)
		// This adds the official archives alongside the custom mirror
		if !usingArchive {
			if verbose {
				fmt.Println("Custom mirror detected, adding official Ubuntu archive as alternative source")
			}
			archiveSourcesFile := filepath.Join(sourcesDir, "ubuntu-archive.sources")

			// Create DEB822 format content for official Ubuntu archives
			deb822Content := buildNobleSourcesContent(release)

			if verbose {
				fmt.Println("\nWriting ubuntu-archive.sources with content:")
				fmt.Println("---")
				fmt.Print(deb822Content)
				fmt.Println("---")
				fmt.Println()
			}

			if err := writeDeb822Sources(archiveSourcesFile, deb822Content); err != nil {
				return fmt.Errorf("error writing ubuntu-archive.sources: %w", err)
			}
			if verbose {
				fmt.Printf("Created %s with official Ubuntu archive configuration\n", archiveSourcesFile)
			}
		} else {
			if verbose {
				fmt.Println("Already using official Ubuntu archive, no additional configuration needed")
			}
		}
		// If already using archive mirror, skip adding - ubuntu.sources already has what we need
	} else {
		return fmt.Errorf("unsupported Ubuntu release: %s", release)
	}

	return nil
}

// buildNobleSourcesContent generates DEB822 format content for Noble Ubuntu archives.
// It returns a properly formatted .sources file content string.
func buildNobleSourcesContent(release string) string {
	return fmt.Sprintf(
		"Types: deb\n"+
			"URIs: http://archive.ubuntu.com/ubuntu/\n"+
			"Suites: %s %s-updates %s-backports\n"+
			"Components: main restricted universe multiverse\n"+
			"Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg\n"+
			"\n"+
			"Types: deb\n"+
			"URIs: http://security.ubuntu.com/ubuntu/\n"+
			"Suites: %s-security\n"+
			"Components: main restricted universe multiverse\n"+
			"Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg\n",
		release, release, release, release)
}

// parseUbuntuSources parses a DEB822 format .sources file and extracts all URIs.
// It returns a slice of URIs found in the file, or an empty slice if the file doesn't exist
// or contains no URIs.
func parseUbuntuSources(sourcesFile string) ([]string, error) {
	content, err := os.ReadFile(sourcesFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("error reading %s: %w", sourcesFile, err)
	}

	var uris []string
	scanner := bufio.NewScanner(strings.NewReader(string(content)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Look for URIs: lines in DEB822 format
		if after, ok := strings.CutPrefix(line, "URIs:"); ok {
			// Extract the URI value after "URIs:"
			uriValue := strings.TrimSpace(after)
			if uriValue != "" {
				uris = append(uris, uriValue)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning %s: %w", sourcesFile, err)
	}

	return uris, nil
}

// isUsingArchiveMirror checks if the ubuntu.sources file is using official Ubuntu archive mirrors.
// Returns true if using archive.ubuntu.com or security.ubuntu.com, false for custom mirrors.
func isUsingArchiveMirror(sourcesFile string) (bool, error) {
	uris, err := parseUbuntuSources(sourcesFile)
	if err != nil {
		return false, err
	}

	// If no URIs found, consider it as not using archive (empty/missing file)
	if len(uris) == 0 {
		return false, nil
	}

	// Check if any URI uses the official archive endpoints
	for _, uri := range uris {
		if strings.Contains(uri, "archive.ubuntu.com") || strings.Contains(uri, "security.ubuntu.com") {
			return true, nil
		}
	}

	return false, nil
}

// writeDeb822Sources writes repository configuration in DEB822 format to a .sources file.
// It creates the file if it doesn't exist, or overwrites it if it does.
func writeDeb822Sources(sourcesFile, content string) error {
	file, err := os.OpenFile(sourcesFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("error opening %s for writing: %w", sourcesFile, err)
	}
	defer func() { _ = file.Close() }()

	writer := bufio.NewWriter(file)
	if _, err := writer.WriteString(content); err != nil {
		return fmt.Errorf("error writing to %s: %w", sourcesFile, err)
	}

	return writer.Flush()
}

// addRepo is a helper function that adds a repository line to the specified sources file
// only if it doesn't already exist. It reads the file, deduplicates all lines, adds the new
// line if missing, and writes the deduplicated content back. This both prevents and cleans up
// duplicate repository entries.
func addRepo(repoLine, sourcesFile string) error {
	// Read existing content
	existingContent, err := os.ReadFile(sourcesFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error reading %s: %w", sourcesFile, err)
	}

	// Use a map to track unique lines (preserves order via slice)
	seenLines := make(map[string]bool)
	var uniqueLines []string
	repoExists := false

	// Process existing lines
	if existingContent != nil {
		scanner := bufio.NewScanner(strings.NewReader(string(existingContent)))
		for scanner.Scan() {
			line := scanner.Text()
			trimmedLine := strings.TrimSpace(line)

			// Skip empty lines
			if trimmedLine == "" {
				continue
			}

			// Check if this is the repo we're trying to add
			if trimmedLine == repoLine {
				repoExists = true
			}

			// Add line only if we haven't seen it before
			if !seenLines[trimmedLine] {
				seenLines[trimmedLine] = true
				uniqueLines = append(uniqueLines, trimmedLine)
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error scanning %s: %w", sourcesFile, err)
		}
	}

	// Add the new repository line if it doesn't exist
	if !repoExists {
		uniqueLines = append(uniqueLines, repoLine)
	}

	// Write the deduplicated content back to the file
	file, err := os.OpenFile(sourcesFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("error opening %s for writing: %w", sourcesFile, err)
	}
	defer func() { _ = file.Close() }()

	writer := bufio.NewWriter(file)
	for _, line := range uniqueLines {
		if _, err := writer.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("error writing to %s: %w", sourcesFile, err)
		}
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
