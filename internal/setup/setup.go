package setup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/apt"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/executor"
	"github.com/saltyorg/sb-go/internal/fact"
	"github.com/saltyorg/sb-go/internal/git"
	"github.com/saltyorg/sb-go/internal/spinners"
	"github.com/saltyorg/sb-go/internal/uv"
)

// InitialSetup performs the initial setup tasks.
// The context parameter allows for cancellation of long-running operations.
func InitialSetup(ctx context.Context, verbose bool) error {
	// Update apt cache
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Updating apt package cache", func() error {
		updateCache := apt.UpdatePackageLists(ctx, verbose)
		return updateCache()
	}); err != nil {
		return fmt.Errorf("error updating apt cache: %w", err)
	}

	// Install git and curl
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Installing git and curl", func() error {
		installGitCurl := apt.InstallPackage(ctx, []string{"git", "curl"}, verbose)
		return installGitCurl()
	}); err != nil {
		return fmt.Errorf("error installing git and curl: %w", err)
	}

	// Create /srv/git directory
	dir := constants.SaltboxGitPath
	if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Creating directory %s", dir), func() error {
		return os.MkdirAll(dir, 0755)
	}); err != nil {
		return fmt.Errorf("error creating %s: %w", dir, err)
	}

	// Create /srv/ansible directory
	dir = constants.AnsibleVenvPath
	if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Creating directory %s", dir), func() error {
		return os.MkdirAll(dir, 0755)
	}); err != nil {
		return fmt.Errorf("error creating %s: %w", dir, err)
	}

	// Install software-properties-common and apt-transport-https
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Installing software-properties-common and apt-transport-https", func() error {
		installPropsTransport := apt.InstallPackage(ctx, []string{"software-properties-common", "apt-transport-https"}, verbose)
		return installPropsTransport()
	}); err != nil {
		return fmt.Errorf("error installing software-properties-common and apt-transport-https: %w", err)
	}

	// Add apt repos
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Adding apt repositories", func() error {
		return apt.AddAptRepositories(ctx)
	}); err != nil {
		return fmt.Errorf("error adding apt repositories: %w", err)
	}

	// Update apt cache again after adding repositories
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Updating apt package cache again", func() error {
		updateCacheAgain := apt.UpdatePackageLists(ctx, verbose)
		return updateCacheAgain()
	}); err != nil {
		return fmt.Errorf("error updating apt cache: %w", err)
	}

	// Install additional required packages.
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Installing additional required packages", func() error {
		packages := []string{
			"locales", "nano", "wget", "jq", "file", "gpg-agent", "libpq-dev",
			"build-essential", "libssl-dev", "libffi-dev", "python3-dev",
			"python3-testresources", "python3-apt", "python3-venv", "python3-pip",
		}
		installPackages := apt.InstallPackage(ctx, packages, verbose)
		return installPackages()
	}); err != nil {
		return fmt.Errorf("error installing additional packages: %w", err)
	}
	return nil
}

// ConfigureLocale attempts to set the system-wide locale to "en_US.UTF-8" and returns an error on failure.
func ConfigureLocale(ctx context.Context) error {
	targetLocale := "en_US.UTF-8"

	// Check if the locale is already installed.
	localeInstalled := false
	resultLocaleCheck, _ := executor.Run(ctx, "locale",
		executor.WithArgs("-a"),
		executor.WithOutputMode(executor.OutputModeCombined),
	)
	if strings.Contains(string(resultLocaleCheck.Combined), targetLocale) {
		localeInstalled = true
	}

	// Generate locale if not already installed.
	if !localeInstalled {
		if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Generating locale %s", targetLocale), func() error {
			result, err := executor.Run(ctx, "locale-gen",
				executor.WithArgs(targetLocale),
				executor.WithOutputMode(executor.OutputModeCombined),
			)
			if err != nil {
				return fmt.Errorf("%w: %s", err, string(result.Combined))
			}
			return nil
		}); err != nil {
			// Return the error instead of exiting.
			return fmt.Errorf("error generating locale: %w", err)
		}
	}

	// Use update-locale to set both LANG and LC_ALL system-wide locale variables
	if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Setting system-wide locale (LC_ALL and LANG) to %s", targetLocale), func() error {
		result, err := executor.Run(ctx, "update-locale",
			executor.WithArgs("LC_ALL="+targetLocale, "LANG="+targetLocale),
			executor.WithOutputMode(executor.OutputModeCombined),
		)
		if err != nil {
			return fmt.Errorf("%w: %s", err, string(result.Combined))
		}
		return nil
	}); err != nil {
		// Don't treat this as fatal; just log it and let dpkg-reconfigure try to fix it.
		_ = spinners.RunInfoSpinner(fmt.Sprintf("update-locale failed, attempting fallback: %v", err))
	}

	// Check /etc/default/locale (more reliable than the `locale` command)
	localeFileContent, err := os.ReadFile("/etc/default/locale")
	if err != nil && !os.IsNotExist(err) {
		_ = spinners.RunWarningSpinner(fmt.Sprintf("Warning: could not read /etc/default/locale: %v", err))
	}

	// Check for both LC_ALL and LANG settings
	lcAllSet := strings.Contains(string(localeFileContent), "LC_ALL="+targetLocale)
	langSet := strings.Contains(string(localeFileContent), "LANG="+targetLocale)

	if !lcAllSet || !langSet {
		// Use a spinner for dpkg-reconfigure.
		if err := spinners.RunTaskWithSpinnerContext(ctx, "Locale not set correctly, reconfiguring locales...", func() error {
			result, err := executor.Run(ctx, "dpkg-reconfigure",
				executor.WithArgs("locales"),
				executor.WithInheritEnv("DEBIAN_FRONTEND=noninteractive"),
				executor.WithOutputMode(executor.OutputModeCombined),
			)
			if err != nil {
				return fmt.Errorf("%w: %s", err, string(result.Combined))
			}
			return nil
		}); err != nil {
			// Return the error instead of exiting.
			return fmt.Errorf("error reconfiguring locales: %w", err)
		}

		// Read /etc/default/locale *again* after reconfiguring
		localeFileContent, err = os.ReadFile("/etc/default/locale")
		if err != nil && !os.IsNotExist(err) {
			_ = spinners.RunWarningSpinner(fmt.Sprintf("Warning: could not read /etc/default/locale after reconfigure: %v", err))
		}

		// Check again for both variables
		lcAllSet = strings.Contains(string(localeFileContent), "LC_ALL="+targetLocale)
		langSet = strings.Contains(string(localeFileContent), "LANG="+targetLocale)
	}

	if !lcAllSet || !langSet {
		if !lcAllSet {
			_ = spinners.RunWarningSpinner("Warning: LC_ALL was not set correctly in /etc/default/locale. This might cause issues.")
		}
		if !langSet {
			_ = spinners.RunWarningSpinner("Warning: LANG was not set correctly in /etc/default/locale. This might cause issues.")
		}
	} else {
		_ = spinners.RunInfoSpinner(fmt.Sprintf("Locales LC_ALL and LANG successfully set to %s", targetLocale))
	}

	return nil
}

// PythonVenv installs Python using uv and creates the Ansible venv.
// The context parameter allows for cancellation of long-running operations.
func PythonVenv(ctx context.Context, verbose bool) error {
	_ = spinners.RunInfoSpinner(fmt.Sprintf("Installing Python %s using uv", constants.AnsibleVenvPythonVersion))

	// Download and install uv
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Downloading and installing uv", func() error {
		return uv.DownloadAndInstallUV(ctx, verbose)
	}); err != nil {
		return fmt.Errorf("error installing uv: %w", err)
	}

	// Create /srv/python directory
	pythonDir := constants.PythonInstallDir
	if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Creating directory %s", pythonDir), func() error {
		return os.MkdirAll(pythonDir, 0755)
	}); err != nil {
		return fmt.Errorf("error creating %s: %w", pythonDir, err)
	}

	// Install Python using uv
	if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Installing Python %s using uv", constants.AnsibleVenvPythonVersion), func() error {
		return uv.InstallPython(ctx, constants.AnsibleVenvPythonVersion, verbose)
	}); err != nil {
		return fmt.Errorf("error installing Python %s: %w", constants.AnsibleVenvPythonVersion, err)
	}

	// Create venv using uv
	venvPath := filepath.Join(constants.AnsibleVenvPath, "venv")
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Creating venv", func() error {
		return uv.CreateVenv(ctx, venvPath, constants.AnsibleVenvPythonVersion, verbose)
	}); err != nil {
		return fmt.Errorf("error creating venv: %w", err)
	}

	// --- Check for venv Python and wait ---
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Checking for venv Python", func() error {
		venvPythonPath := constants.AnsibleVenvPythonPath()
		maxWait := 10 * time.Second
		startTime := time.Now()

		for time.Since(startTime) < maxWait {
			if _, err := os.Stat(venvPythonPath); err == nil {
				return nil // File exists, exit loop
			}
			time.Sleep(1 * time.Second)
		}

		return fmt.Errorf("virtual environment Python still not found after waiting")
	}); err != nil {
		return fmt.Errorf("error checking for venv Python: %w", err)
	}
	return nil
}

// SaltboxRepo checks out the master branch of the Saltbox GitHub repository.
// Resets the existing git repository folder if present.
// Runs submodule update.
// The context parameter allows for cancellation of long-running operations.
func SaltboxRepo(ctx context.Context, verbose bool, branch string) error {
	saltboxPath := constants.SaltboxRepoPath
	saltboxRepoURL := constants.SaltboxRepoURL
	if branch == "" {
		branch = "master" // Default to master if not specified
	}

	// Check if the Saltbox directory exists.
	_, err := os.Stat(saltboxPath)
	if os.IsNotExist(err) {
		// Clone the repository if it doesn't exist.
		if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Cloning Saltbox repository to %s (branch: %s)", saltboxPath, branch), func() error {
			return git.CloneRepository(ctx, saltboxRepoURL, saltboxPath, branch, verbose)
		}); err != nil {
			return fmt.Errorf("error cloning Saltbox repository: %w", err)
		}

		// Run submodule update after cloning.
		if err := spinners.RunTaskWithSpinnerContext(ctx, "Updating git submodules", func() error {
			_, err := executor.Run(ctx, "git",
				executor.WithArgs("submodule", "update", "--init", "--recursive"),
				executor.WithWorkingDir(saltboxPath),
				executor.WithOutputMode(executor.OutputModeDiscard),
			)
			return err
		}); err != nil {
			return fmt.Errorf("error running git submodule update: %w", err)
		}

	} else if err != nil {
		// Handle errors other than "not exists" (e.g., permissions).
		return fmt.Errorf("error checking for Saltbox directory: %w", err)

	} else {
		// The directory exists. Check if it's a git repo.
		gitDirPath := filepath.Join(saltboxPath, ".git")
		_, err := os.Stat(gitDirPath)

		if os.IsNotExist(err) {
			// Not a git repo, initialize, fetch, and set up.
			_ = spinners.RunInfoSpinner("Saltbox directory exists but is not a git repository. Initializing...")

			initCmds := [][]string{
				{"git", "init"},
				{"git", "remote", "add", "origin", saltboxRepoURL},
				{"git", "fetch", "--all", "--prune"},
				{"git", "branch", branch, "origin/" + branch},
				{"git", "reset", "--hard", "origin/" + branch},
				{"git", "submodule", "update", "--init", "--recursive"},
			}

			// Wrap the entire loop in a spinner
			if err := spinners.RunTaskWithSpinnerContext(ctx, "Initializing Git repository", func() error {
				for _, command := range initCmds {
					_, err := executor.Run(ctx, command[0],
						executor.WithArgs(command[1:]...),
						executor.WithWorkingDir(saltboxPath),
						executor.WithOutputMode(executor.OutputModeDiscard),
					)
					if err != nil {
						return fmt.Errorf("error running command %v: %w", command, err) // Wrap the error
					}
				}
				return nil
			}); err != nil {
				return err // Error is already formatted nicely
			}

		} else if err != nil {
			// Handle errors other than "not exists" (e.g., permissions).
			return fmt.Errorf("error checking for .git directory: %w", err)
		} else {
			// It's a git repo, fetch and reset
			if err := spinners.RunTaskWithSpinnerContext(ctx, "Updating existing Saltbox repository", func() error {
				return git.FetchAndReset(ctx, saltboxPath, branch, "root", nil, nil, "Saltbox") // Assuming root user
			}); err != nil {
				return fmt.Errorf("error updating Saltbox repository: %w", err)
			}
		}
	}

	// These functions already have internal spinners
	if err := fact.DownloadAndInstallSaltboxFact(false, verbose); err != nil {
		return fmt.Errorf("error downloading and installing saltbox.fact: %w", err)
	}

	if err := CopyDefaultConfigFiles(ctx); err != nil {
		return fmt.Errorf("error copying default configuration files: %w", err)
	}

	return nil
}

// InstallPipDependencies installs pip dependencies in the Ansible virtual environment.
// The context parameter allows for cancellation of long-running operations.
func InstallPipDependencies(ctx context.Context, verbose bool) error {
	venvPythonPath := constants.AnsibleVenvPythonPath()
	python3Cmd := []string{venvPythonPath, "-m", "pip", "install", "--timeout=360", "--no-cache-dir", "--disable-pip-version-check", "--upgrade"}

	// Install pip, setuptools, and wheel
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Installing pip, setuptools, and wheel", func() error {
		installBaseDeps := append(python3Cmd, "pip", "setuptools", "wheel")
		if verbose {
			fmt.Println("Running command:", installBaseDeps)
			_, err := executor.Run(ctx, installBaseDeps[0],
				executor.WithArgs(installBaseDeps[1:]...),
				executor.WithOutputMode(executor.OutputModeStream),
			)
			return err
		}
		// Capture output to prevent terminal interference
		result, err := executor.Run(ctx, installBaseDeps[0],
			executor.WithArgs(installBaseDeps[1:]...),
			executor.WithOutputMode(executor.OutputModeCombined),
		)
		if err != nil {
			return fmt.Errorf("command failed: %w\nOutput:\n%s", err, string(result.Combined))
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error installing pip, setuptools, and wheel: %w", err)
	}

	// Install requirements from requirements-saltbox.txt
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Installing requirements from requirements-saltbox.txt", func() error {
		requirementsPath := filepath.Join(constants.SaltboxRepoPath, "requirements", "requirements-saltbox.txt")
		installRequirements := append(python3Cmd, "--requirement", requirementsPath)
		if verbose {
			fmt.Println("Running command:", installRequirements)
			_, err := executor.Run(ctx, installRequirements[0],
				executor.WithArgs(installRequirements[1:]...),
				executor.WithOutputMode(executor.OutputModeStream),
			)
			return err
		}
		// Capture output to prevent terminal interference
		result, err := executor.Run(ctx, installRequirements[0],
			executor.WithArgs(installRequirements[1:]...),
			executor.WithOutputMode(executor.OutputModeCombined),
		)
		if err != nil {
			return fmt.Errorf("command failed: %w\nOutput:\n%s", err, string(result.Combined))
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error installing requirements from requirements-saltbox.txt: %w", err)
	}

	return nil
}

// copyBinaryFile copies a single binary file from src to dest with proper permissions
func copyBinaryFile(srcPath, destPath string) error {
	// Get file info from the source to read its permissions.
	sourceFileInfo, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("could not stat source file: %w", err)
	}

	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("error opening source file %s: %w", srcPath, err)
	}
	defer srcFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("error creating destination file %s: %w", destPath, err)
	}
	defer destFile.Close()

	// Copy contents
	if _, err := io.Copy(destFile, srcFile); err != nil {
		return fmt.Errorf("error copying file: %w", err)
	}

	// Apply the original file's permissions to the new file.
	err = os.Chmod(destPath, sourceFileInfo.Mode())
	if err != nil {
		return fmt.Errorf("could not set permissions on destination file: %w", err)
	}

	return nil
}

// CopyRequiredBinaries copies select binaries from the virtual environment to /usr/local/bin.
func CopyRequiredBinaries(ctx context.Context) error {
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Copying required binaries to /usr/local/bin", func() error {
		venvBinDir := filepath.Join(constants.AnsibleVenvPath, "venv", "bin")
		destDir := "/usr/local/bin"
		files, err := os.ReadDir(venvBinDir)
		if err != nil {
			return fmt.Errorf("error reading virtual environment bin directory: %w", err)
		}

		for _, file := range files {
			fileName := file.Name()
			// Check for ansible, certbot, or apprise binaries
			if strings.HasPrefix(fileName, "ansible") || strings.HasPrefix(fileName, "certbot") || strings.HasPrefix(fileName, "apprise") {
				srcPath := filepath.Join(venvBinDir, fileName)
				destPath := filepath.Join(destDir, fileName)

				if err := copyBinaryFile(srcPath, destPath); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error copying required binaries: %w", err)
	}
	return nil
}

// copyConfigFile copies a single config file from src to dest with proper permissions
func copyConfigFile(srcPath, destPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("error opening source file %s: %w", srcPath, err)
	}
	defer srcFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("error creating destination file %s: %w", destPath, err)
	}
	defer destFile.Close()

	// Set Permissions
	if err := os.Chmod(destPath, 0755); err != nil {
		return fmt.Errorf("error setting permissions on destination file %s: %w", destPath, err)
	}

	// Copy contents
	if _, err := io.Copy(destFile, srcFile); err != nil {
		return fmt.Errorf("error copying file %s to %s: %w", srcPath, destPath, err)
	}

	return nil
}

// CopyDefaultConfigFiles copies default config files into the Saltbox folder.
func CopyDefaultConfigFiles(ctx context.Context) error {
	saltboxPath := constants.SaltboxRepoPath
	defaultsDir := filepath.Join(saltboxPath, "defaults")
	files, err := filepath.Glob(filepath.Join(defaultsDir, "*.default"))
	if err != nil {
		return fmt.Errorf("error listing default config files: %w", err)
	}

	for _, srcPath := range files {
		baseName := filepath.Base(srcPath)
		destName := strings.TrimSuffix(baseName, ".default")
		destPath := filepath.Join(saltboxPath, destName)

		// Check if the destination file already exists.
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			// Destination file doesn't exist, proceed with copying.
			if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Copying %s", baseName), func() error {
				return copyConfigFile(srcPath, destPath)
			}); err != nil {
				return err
			}
		} else if err != nil {
			// os.Stat error other than IsNotExist
			return fmt.Errorf("could not check file %s: %w", destPath, err)
		}
	}

	return nil
}
