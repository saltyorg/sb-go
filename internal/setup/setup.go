package setup

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/apt"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/fact"
	"github.com/saltyorg/sb-go/internal/git"
	"github.com/saltyorg/sb-go/internal/spinners"
	"github.com/saltyorg/sb-go/internal/uv"
)

// InitialSetup performs the initial setup tasks.
// The context parameter allows for cancellation of long-running operations.
func InitialSetup(ctx context.Context, verbose bool) {
	// Update apt cache
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Updating apt package cache", func() error {
		updateCache := apt.UpdatePackageLists(ctx, verbose)
		return updateCache()
	}); err != nil {
		fmt.Println("Error updating apt cache:", err)
		os.Exit(1)
	}

	// Install git and curl
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Installing git and curl", func() error {
		installGitCurl := apt.InstallPackage(ctx, []string{"git", "curl"}, verbose)
		return installGitCurl()
	}); err != nil {
		fmt.Println("Error installing git and curl:", err)
		os.Exit(1)
	}

	// Create /srv/git directory
	dir := constants.SaltboxGitPath
	if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Creating directory %s", dir), func() error {
		return os.MkdirAll(dir, 0755)
	}); err != nil {
		fmt.Printf("Error creating %s: %v\n", dir, err)
		os.Exit(1)
	}

	// Create /srv/ansible directory
	dir = constants.AnsibleVenvPath
	if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Creating directory %s", dir), func() error {
		return os.MkdirAll(dir, 0755)
	}); err != nil {
		fmt.Printf("Error creating %s: %v\n", dir, err)
		os.Exit(1)
	}

	// Install software-properties-common and apt-transport-https
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Installing software-properties-common and apt-transport-https", func() error {
		installPropsTransport := apt.InstallPackage(ctx, []string{"software-properties-common", "apt-transport-https"}, verbose)
		return installPropsTransport()
	}); err != nil {
		fmt.Println("Error installing software-properties-common and apt-transport-https:", err)
		os.Exit(1)
	}

	// Add apt repos
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Adding apt repositories", func() error {
		return apt.AddAptRepositories(ctx)
	}); err != nil {
		fmt.Println("Error adding apt repositories:", err)
		os.Exit(1)
	}

	// Update apt cache again after adding repositories
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Updating apt package cache again", func() error {
		updateCacheAgain := apt.UpdatePackageLists(ctx, verbose)
		return updateCacheAgain()
	}); err != nil {
		fmt.Println("Error updating apt cache:", err)
		os.Exit(1)
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
		fmt.Println("Error installing additional packages:", err)
		os.Exit(1)
	}
}

// ConfigureLocale attempts to set the system-wide locale to "en_US.UTF-8".
// The context parameter allows for cancellation of long-running operations.
func ConfigureLocale(ctx context.Context) {
	targetLocale := "en_US.UTF-8"

	// Check if the locale is already installed.
	localeInstalled := false
	cmdLocaleCheck := exec.CommandContext(ctx, "locale", "-a")
	outputLocaleCheck, _ := cmdLocaleCheck.CombinedOutput() // Ignore error; presence check.
	if strings.Contains(string(outputLocaleCheck), targetLocale) {
		localeInstalled = true
	}

	// Generate locale if not already installed.
	if !localeInstalled {
		if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Generating locale %s", targetLocale), func() error {
			cmdLocaleGen := exec.CommandContext(ctx, "locale-gen", targetLocale)
			return cmdLocaleGen.Run()
		}); err != nil {
			fmt.Println("Error generating locale:", err)
			os.Exit(1)
		}
	}

	// Use update-locale to set both LANG and LC_ALL system-wide locale variables
	if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Setting system-wide locale (LC_ALL and LANG) to %s", targetLocale), func() error {
		cmdUpdateLocale := exec.CommandContext(ctx, "update-locale", "LC_ALL="+targetLocale, "LANG="+targetLocale)
		return cmdUpdateLocale.Run()
	}); err != nil {
		// Don't exit here; try dpkg-reconfigure as a fallback. Log with an info spinner.
		_ = spinners.RunInfoSpinner(fmt.Sprintf("Attempting to set locale with update-locale failed: %v", err))
	}

	// Check /etc/default/locale (more reliable than the `locale` command)
	localeFileContent, err := os.ReadFile("/etc/default/locale")
	if err != nil && !os.IsNotExist(err) {
		// Use a warning spinner for file read errors (but don't exit).
		_ = spinners.RunWarningSpinner(fmt.Sprintf("Error reading /etc/default/locale: %v", err))
	}

	// Check for both LC_ALL and LANG settings
	lcAllSet := strings.Contains(string(localeFileContent), "LC_ALL="+targetLocale)
	langSet := strings.Contains(string(localeFileContent), "LANG="+targetLocale)

	if !lcAllSet || !langSet {
		// Use a spinner for dpkg-reconfigure.
		if err := spinners.RunTaskWithSpinnerContext(ctx, "Locale not set correctly, reconfiguring locales...", func() error {
			cmdReconfigureLocales := exec.CommandContext(ctx, "dpkg-reconfigure", "locales")
			return cmdReconfigureLocales.Run()
		}); err != nil {
			fmt.Println("Error reconfiguring locales:", err)
			os.Exit(1)
		}

		// Read /etc/default/locale *again* after reconfiguring
		localeFileContent, err = os.ReadFile("/etc/default/locale")
		if err != nil && !os.IsNotExist(err) {
			_ = spinners.RunWarningSpinner(fmt.Sprintf("Error reading /etc/default/locale after reconfigure: %v", err))
		}

		// Check again for both variables
		lcAllSet = strings.Contains(string(localeFileContent), "LC_ALL="+targetLocale)
		langSet = strings.Contains(string(localeFileContent), "LANG="+targetLocale)
	}

	if !lcAllSet || !langSet {
		if !lcAllSet {
			_ = spinners.RunWarningSpinner("Warning: LC_ALL not set correctly in /etc/default/locale")
		}
		if !langSet {
			_ = spinners.RunWarningSpinner("Warning: LANG not set correctly in /etc/default/locale")
		}
	} else {
		// Use an info spinner to be consistent with other successful steps
		_ = spinners.RunInfoSpinner(fmt.Sprintf("Locales LC_ALL and LANG both set to %s", targetLocale))
	}
}

// PythonVenv installs Python using uv and creates the Ansible venv.
// The context parameter allows for cancellation of long-running operations.
func PythonVenv(ctx context.Context, verbose bool) {
	_ = spinners.RunInfoSpinner("Installing Python 3.12 using uv")

	// Download and install uv
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Downloading and installing uv", func() error {
		return uv.DownloadAndInstallUV(ctx, verbose)
	}); err != nil {
		fmt.Println("Error installing uv:", err)
		os.Exit(1)
	}

	// Create /srv/python directory
	pythonDir := constants.PythonInstallDir
	if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Creating directory %s", pythonDir), func() error {
		return os.MkdirAll(pythonDir, 0755)
	}); err != nil {
		fmt.Printf("Error creating %s: %v\n", pythonDir, err)
		os.Exit(1)
	}

	// Install Python 3.12 using uv
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Installing Python 3.12 using uv", func() error {
		return uv.InstallPython(ctx, constants.AnsibleVenvPythonVersion, verbose)
	}); err != nil {
		fmt.Println("Error installing Python 3.12:", err)
		os.Exit(1)
	}

	// Create venv using uv
	venvPath := filepath.Join(constants.AnsibleVenvPath, "venv")
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Creating venv", func() error {
		return uv.CreateVenv(ctx, venvPath, constants.AnsibleVenvPythonVersion, verbose)
	}); err != nil {
		fmt.Println("Error creating venv:", err)
		os.Exit(1)
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
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

// SaltboxRepo checks out the master branch of the Saltbox GitHub repository.
// Resets the existing git repository folder if present.
// Runs submodule update.
// The context parameter allows for cancellation of long-running operations.
func SaltboxRepo(ctx context.Context, verbose bool, branch string) {
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
			fmt.Printf("Error cloning Saltbox repository: %v\n", err)
			os.Exit(1)
		}

		// Run submodule update after cloning.
		if err := spinners.RunTaskWithSpinnerContext(ctx, "Updating git submodules", func() error {
			submoduleCmd := exec.CommandContext(ctx, "git", "submodule", "update", "--init", "--recursive")
			submoduleCmd.Dir = saltboxPath
			return submoduleCmd.Run()
		}); err != nil {
			fmt.Printf("Error running git submodule update: %v\n", err)
			os.Exit(1)
		}

	} else if err != nil {
		// Handle errors other than "not exists" (e.g., permissions).
		fmt.Printf("Error checking for Saltbox directory: %v\n", err)
		os.Exit(1)

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
					cmd := exec.CommandContext(ctx, command[0], command[1:]...)
					cmd.Dir = saltboxPath
					if err := cmd.Run(); err != nil {
						return fmt.Errorf("error running command %v: %w", command, err) // Wrap the error
					}
				}
				return nil
			}); err != nil {
				fmt.Println(err) // Error is already formatted nicely
				os.Exit(1)
			}

		} else if err != nil {
			// Handle errors other than "not exists" (e.g., permissions).
			fmt.Printf("Error checking for .git directory: %v\n", err)
			os.Exit(1)
		} else {
			// It's a git repo, fetch and reset
			if err := spinners.RunTaskWithSpinnerContext(ctx, "Updating existing Saltbox repository", func() error {
				return git.FetchAndReset(ctx, saltboxPath, branch, "root", nil, nil) // Assuming root user
			}); err != nil {
				fmt.Printf("Error updating Saltbox repository: %v\n", err)
				os.Exit(1)
			}
		}
	}

	// These functions already have internal spinners
	if err := fact.DownloadAndInstallSaltboxFact(false, verbose); err != nil {
		fmt.Printf("Error downloading and installing saltbox.fact: %v\n", err)
		os.Exit(1)
	}

	if err := CopyDefaultConfigFiles(ctx); err != nil {
		fmt.Printf("Error copying default configuration files: %v\n", err)
		os.Exit(1)
	}
}

// InstallPipDependencies installs pip dependencies in the Ansible virtual environment.
// The context parameter allows for cancellation of long-running operations.
func InstallPipDependencies(ctx context.Context, verbose bool) {
	venvPythonPath := constants.AnsibleVenvPythonPath()
	python3Cmd := []string{venvPythonPath, "-m", "pip", "install", "--timeout=360", "--no-cache-dir", "--disable-pip-version-check", "--upgrade"}

	// Install pip, setuptools, and wheel
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Installing pip, setuptools, and wheel", func() error {
		installBaseDeps := append(python3Cmd, "pip", "setuptools", "wheel")
		cmdInstallBase := exec.CommandContext(ctx, installBaseDeps[0], installBaseDeps[1:]...)
		if verbose {
			fmt.Println("Running command:", installBaseDeps)
			cmdInstallBase.Stdout = os.Stdout
			cmdInstallBase.Stderr = os.Stderr
		}
		return cmdInstallBase.Run()
	}); err != nil {
		fmt.Println("Error installing pip, setuptools, and wheel:", err)
		os.Exit(1)
	}

	// Install requirements from requirements-saltbox.txt
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Installing requirements from requirements-saltbox.txt", func() error {
		requirementsPath := filepath.Join(constants.SaltboxRepoPath, "requirements", "requirements-saltbox.txt")
		installRequirements := append(python3Cmd, "--requirement", requirementsPath)
		cmdInstallReq := exec.CommandContext(ctx, installRequirements[0], installRequirements[1:]...)
		if verbose {
			fmt.Println("Running command:", installRequirements)
			cmdInstallReq.Stdout = os.Stdout
			cmdInstallReq.Stderr = os.Stderr
		}
		return cmdInstallReq.Run()
	}); err != nil {
		fmt.Println("Error installing requirements from requirements-saltbox.txt:", err)
		os.Exit(1)
	}
}

// CopyRequiredBinaries copies select binaries from the virtual environment to /usr/local/bin.
func CopyRequiredBinaries(ctx context.Context) {
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Copying required binaries to /usr/local/bin", func() error {
		venvBinDir := filepath.Join(constants.AnsibleVenvPath, "venv", "bin")
		destDir := "/usr/local/bin"
		files, err := os.ReadDir(venvBinDir)
		if err != nil {
			return fmt.Errorf("error reading virtual environment bin directory: %w", err) // Wrap error
		}

		for _, file := range files {
			fileName := file.Name()
			// Check for ansible, certbot, or apprise binaries
			if strings.HasPrefix(fileName, "ansible") || strings.HasPrefix(fileName, "certbot") || strings.HasPrefix(fileName, "apprise") {
				srcPath := filepath.Join(venvBinDir, fileName)
				destPath := filepath.Join(destDir, fileName)

				// Open source file
				srcFile, err := os.Open(srcPath)
				if err != nil {
					return fmt.Errorf("error opening source file %s: %w", srcPath, err)
				}
				defer srcFile.Close()

				// Create destination file
				destFile, err := os.Create(destPath)
				if err != nil {
					return fmt.Errorf("error creating destination file %s: %w", destPath, err)
				}
				defer destFile.Close()

				// Set permissions on destination file
				if err := os.Chmod(destPath, 0755); err != nil {
					return fmt.Errorf("error setting permissions on destination file %s: %w", destPath, err)
				}

				// Copy contents
				_, err = io.Copy(destFile, srcFile)

				if err != nil {
					return fmt.Errorf("error copying file %s to %s: %w", srcPath, destPath, err)
				}
			}
		}
		return nil
	}); err != nil {
		fmt.Println("Error copying required binaries:", err)
		os.Exit(1)
	}
}

// CopyDefaultConfigFiles copies default config files into the Saltbox folder.
func CopyDefaultConfigFiles(ctx context.Context) error {
	saltboxPath := constants.SaltboxRepoPath
	defaultsDir := filepath.Join(saltboxPath, "defaults")
	files, err := filepath.Glob(filepath.Join(defaultsDir, "*.default"))
	if err != nil {
		return fmt.Errorf("error listing default config files: %w", err)
	}

	processFile := func(srcPath, destPath, baseName, destName string) error {
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
		if _, err := io.Copy(destFile, srcFile); err != nil {
			return fmt.Errorf("error copying file %s to %s: %w", srcPath, destPath, err)
		}
		return nil
	}

	for _, srcPath := range files {
		baseName := filepath.Base(srcPath)
		destName := strings.TrimSuffix(baseName, ".default")
		destPath := filepath.Join(saltboxPath, destName)

		// Check if the destination file already exists.
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			// Destination file doesn't exist, proceed with copying.
			if err := spinners.RunTaskWithSpinnerContext(ctx, fmt.Sprintf("Copying %s", baseName), func() error {
				return processFile(srcPath, destPath, baseName, destName)
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
