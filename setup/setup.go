package setup

import (
	"fmt"
	"github.com/saltyorg/sb-go/fact"
	"github.com/saltyorg/sb-go/git"
	"github.com/saltyorg/sb-go/spinners"
	"github.com/saltyorg/sb-go/ubuntu"
	"github.com/saltyorg/sb-go/utils"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/apt"
	"github.com/saltyorg/sb-go/constants"
)

// InitialSetup performs the initial setup tasks.
func InitialSetup(verbose bool) {
	if verbose {
		fmt.Println("--- Running initial setup with verbose output ---")

		fmt.Println("Updating apt package cache...")
		if err := apt.UpdatePackageLists(verbose)(); err != nil {
			fmt.Println("Error updating apt cache:", err)
			os.Exit(1)
		}

		fmt.Println("Installing git and curl...")
		if err := apt.InstallPackage([]string{"git", "curl"}, verbose)(); err != nil {
			fmt.Println("Error installing git and curl:", err)
			os.Exit(1)
		}

		dir := constants.SaltboxGitPath
		fmt.Printf("Creating directory %s...\n", dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("Error creating %s: %v\n", dir, err)
			os.Exit(1)
		}

		dir = constants.AnsibleVenvPath
		fmt.Printf("Creating directory %s...\n", dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("Error creating %s: %v\n", dir, err)
			os.Exit(1)
		}

		fmt.Println("Installing software-properties-common and apt-transport-https...")
		if err := apt.InstallPackage([]string{"software-properties-common", "apt-transport-https"}, verbose)(); err != nil {
			fmt.Println("Error installing software-properties-common and apt-transport-https:", err)
			os.Exit(1)
		}

		fmt.Println("Adding apt repositories...")
		if err := apt.AddAptRepositories(); err != nil {
			fmt.Println("Error adding apt repositories:", err)
			os.Exit(1)
		}

		fmt.Println("Updating apt package cache again...")
		if err := apt.UpdatePackageLists(verbose)(); err != nil {
			fmt.Println("Error updating apt cache:", err)
			os.Exit(1)
		}

		fmt.Println("Installing additional required packages...")
		packages := []string{
			"locales", "nano", "wget", "jq", "file", "gpg-agent",
			"build-essential", "libssl-dev", "libffi-dev", "python3-dev",
			"python3-testresources", "python3-apt", "python3-venv",
		}
		if err := apt.InstallPackage(packages, verbose)(); err != nil {
			fmt.Println("Error installing additional packages:", err)
			os.Exit(1)
		}

		fmt.Println("--- Initial setup with verbose output complete ---")

	} else {
		// Update apt cache
		if err := spinners.RunTaskWithSpinner("Updating apt package cache", func() error {
			updateCache := apt.UpdatePackageLists(verbose)
			return updateCache()
		}); err != nil {
			fmt.Println("Error updating apt cache:", err)
			os.Exit(1)
		}

		// Install git and curl
		if err := spinners.RunTaskWithSpinner("Installing git and curl", func() error {
			installGitCurl := apt.InstallPackage([]string{"git", "curl"}, verbose)
			return installGitCurl()
		}); err != nil {
			fmt.Println("Error installing git and curl:", err)
			os.Exit(1)
		}

		// Create /srv/git directory
		dir := constants.SaltboxGitPath
		if err := spinners.RunTaskWithSpinner(fmt.Sprintf("Creating directory %s", dir), func() error {
			return os.MkdirAll(dir, 0755)
		}); err != nil {
			fmt.Printf("Error creating %s: %v\n", dir, err)
			os.Exit(1)
		}

		// Create /srv/ansible directory
		dir = constants.AnsibleVenvPath
		if err := spinners.RunTaskWithSpinner(fmt.Sprintf("Creating directory %s", dir), func() error {
			return os.MkdirAll(dir, 0755)
		}); err != nil {
			fmt.Printf("Error creating %s: %v\n", dir, err)
			os.Exit(1)
		}

		// Install software-properties-common and apt-transport-https
		if err := spinners.RunTaskWithSpinner("Installing software-properties-common and apt-transport-https", func() error {
			installPropsTransport := apt.InstallPackage([]string{"software-properties-common", "apt-transport-https"}, verbose)
			return installPropsTransport()
		}); err != nil {
			fmt.Println("Error installing software-properties-common and apt-transport-https:", err)
			os.Exit(1)
		}

		// Add apt repos
		if err := spinners.RunTaskWithSpinner("Adding apt repositories", func() error {
			return apt.AddAptRepositories()
		}); err != nil {
			fmt.Println("Error adding apt repositories:", err)
			os.Exit(1)
		}

		// Update apt cache again after adding repositories
		if err := spinners.RunTaskWithSpinner("Updating apt package cache again", func() error {
			updateCacheAgain := apt.UpdatePackageLists(verbose)
			return updateCacheAgain()
		}); err != nil {
			fmt.Println("Error updating apt cache:", err)
			os.Exit(1)
		}

		// Install additional required packages.
		if err := spinners.RunTaskWithSpinner("Installing additional required packages", func() error {
			packages := []string{
				"locales", "nano", "wget", "jq", "file", "gpg-agent",
				"build-essential", "libssl-dev", "libffi-dev", "python3-dev",
				"python3-testresources", "python3-apt", "python3-venv",
			}
			installPackages := apt.InstallPackage(packages, verbose)
			return installPackages()
		}); err != nil {
			fmt.Println("Error installing additional packages:", err)
			os.Exit(1)
		}
	}
}

// ConfigureLocale attempts to set the system-wide locale to "en_US.UTF-8".
// If verbose is true, the output of the locale-gen and update-locale commands will be printed to stdout/stderr.
func ConfigureLocale(verbose bool) {
	targetLocale := "en_US.UTF-8"

	// Check if the locale is already installed.
	localeInstalled := false
	cmdLocaleCheck := exec.Command("locale", "-a")
	outputLocaleCheck, _ := cmdLocaleCheck.CombinedOutput() // Ignore error; presence check.
	if strings.Contains(string(outputLocaleCheck), targetLocale) {
		localeInstalled = true
	}

	if verbose {
		fmt.Printf("--- Configuring Locale to %s (Verbose) ---\n", targetLocale)

		if !localeInstalled {
			fmt.Printf("Generating locale %s...\n", targetLocale)
			cmdLocaleGen := exec.Command("locale-gen", targetLocale)
			cmdLocaleGen.Stdout = os.Stdout
			cmdLocaleGen.Stderr = os.Stderr
			if err := cmdLocaleGen.Run(); err != nil {
				fmt.Println("Error generating locale:", err)
				os.Exit(1)
			}
		} else {
			fmt.Printf("Locale %s is already installed.\n", targetLocale)
		}

		fmt.Printf("Setting system-wide locale to %s using update-locale...\n", targetLocale)
		cmdUpdateLocale := exec.Command("update-locale", "LC_ALL="+targetLocale)
		cmdUpdateLocale.Stdout = os.Stdout
		cmdUpdateLocale.Stderr = os.Stderr
		if err := cmdUpdateLocale.Run(); err != nil {
			fmt.Printf("Attempting to set locale with update-locale failed: %v\n", err)
		}

		// Check /etc/default/locale (more reliable than `locale` command)
		localeFileContent, err := os.ReadFile("/etc/default/locale")
		if err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: Error reading /etc/default/locale: %v\n", err)
		}

		if !strings.Contains(string(localeFileContent), "LC_ALL="+targetLocale) {
			fmt.Println("Locale not set correctly, reconfiguring locales...")
			cmdReconfigureLocales := exec.Command("dpkg-reconfigure", "locales")
			cmdReconfigureLocales.Stdout = os.Stdout
			cmdReconfigureLocales.Stderr = os.Stderr
			if err := cmdReconfigureLocales.Run(); err != nil {
				fmt.Println("Error reconfiguring locales:", err)
				os.Exit(1)
			}

			// Read /etc/default/locale *again* after reconfiguring
			localeFileContent, err = os.ReadFile("/etc/default/locale")
			if err != nil && !os.IsNotExist(err) {
				fmt.Printf("Warning: Error reading /etc/default/locale after reconfigure: %v\n", err)
			}
		}

		if !strings.Contains(string(localeFileContent), "LC_ALL="+targetLocale) {
			fmt.Printf("Warning: Locale still not set correctly in /etc/default/locale\n")
		} else {
			fmt.Printf("Locale set to %s\n", targetLocale)
		}

		fmt.Println("--- Locale Configuration (Verbose) Complete ---")

	} else {
		// Generate locale if not already installed.
		if !localeInstalled {
			if err := spinners.RunTaskWithSpinner(fmt.Sprintf("Generating locale %s", targetLocale), func() error {
				cmdLocaleGen := exec.Command("locale-gen", targetLocale)
				return cmdLocaleGen.Run()
			}); err != nil {
				fmt.Println("Error generating locale:", err)
				os.Exit(1)
			}
		}

		// Use update-locale to set the system-wide locale.
		if err := spinners.RunTaskWithSpinner(fmt.Sprintf("Setting system-wide locale to %s", targetLocale), func() error {
			cmdUpdateLocale := exec.Command("update-locale", "LC_ALL="+targetLocale)
			return cmdUpdateLocale.Run()
		}); err != nil {
			// Don't exit here; try dpkg-reconfigure as a fallback.  Log with an info spinner.
			_ = spinners.RunInfoSpinner(fmt.Sprintf("Attempting to set locale with update-locale failed: %v", err))
		}

		// Check /etc/default/locale (more reliable than `locale` command)
		localeFileContent, err := os.ReadFile("/etc/default/locale")
		if err != nil && !os.IsNotExist(err) {
			// Use a warning spinner for file read errors (but don't exit).
			_ = spinners.RunWarningSpinner(fmt.Sprintf("Error reading /etc/default/locale: %v", err))
		}

		if !strings.Contains(string(localeFileContent), "LC_ALL="+targetLocale) {
			// Use a spinner for dpkg-reconfigure.
			if err := spinners.RunTaskWithSpinner("Locale not set correctly, reconfiguring locales...", func() error {
				cmdReconfigureLocales := exec.Command("dpkg-reconfigure", "locales")
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
		}

		if !strings.Contains(string(localeFileContent), "LC_ALL="+targetLocale) {
			_ = spinners.RunWarningSpinner(fmt.Sprintf("Warning: Locale still not set correctly in /etc/default/locale"))
		} else {
			//Use an info spinner, to be consistent with other successful steps
			_ = spinners.RunInfoSpinner(fmt.Sprintf("Locale set to %s", targetLocale))
		}
	}
}

// PythonVenv installs Python from deadsnakes, if required, and creates the Ansible venv.
func PythonVenv(verbose bool) {
	osRelease, _ := ubuntu.ParseOSRelease("/etc/os-release")
	versionCodename := osRelease["VERSION_CODENAME"]

	jammyRegex := regexp.MustCompile(`^jammy$`)
	nobleRegex := regexp.MustCompile(`^noble$`)

	if verbose {
		fmt.Println("--- Setting up Python Virtual Environment (Verbose) ---")
		fmt.Printf("Detected Ubuntu codename: %s\n", versionCodename)

		if jammyRegex.MatchString(versionCodename) {
			fmt.Println("Ubuntu Jammy (or similar) detected, deploying venv with Python 3.12.")

			// Add deadsnakes PPA
			fmt.Println("Adding deadsnakes PPA...")
			if err := apt.AddPPA("ppa:deadsnakes/ppa", verbose)(); err != nil {
				fmt.Println("Error adding deadsnakes PPA", err)
				os.Exit(1)
			}

			// Install Python 3.12 and venv
			fmt.Println("Installing Python 3.12 and venv...")
			if err := apt.InstallPackage([]string{"python3.12", "python3.12-dev", "python3.12-venv"}, verbose)(); err != nil {
				fmt.Println("Error installing Python 3.12:", err)
				os.Exit(1)
			}

			// Ensure pip and setuptools are present in system python
			fmt.Println("Ensuring pip is present...")
			ensurePip := exec.Command("python3.12", "-m", "ensurepip")
			ensurePip.Dir = constants.AnsibleVenvPath //Set working directory
			ensurePip.Stdout = os.Stdout
			ensurePip.Stderr = os.Stderr
			if err := ensurePip.Run(); err != nil {
				fmt.Printf("Warning: Ensuring pip failed: %v\n", err)
			}

			// Create venv
			fmt.Println("Creating venv...")
			createVenv := exec.Command("python3.12", "-m", "venv", "venv")
			createVenv.Dir = constants.AnsibleVenvPath
			createVenv.Stdout = os.Stdout
			createVenv.Stderr = os.Stderr
			if err := createVenv.Run(); err != nil {
				fmt.Println("Error creating venv:", err)
				os.Exit(1)
			}

			// Install pip using get-pip.py (for Jammy)
			fmt.Println("Installing pip using get-pip.py...")
			if err := utils.InstallPip(verbose); err != nil {
				fmt.Println("Error installing pip", err) //Consistent error handling
				os.Exit(1)
			}

		} else if nobleRegex.MatchString(versionCodename) {
			fmt.Println("Ubuntu Noble (or similar) detected, deploying venv with Python 3.12.")

			// Install python3-pip (needed for Noble)
			fmt.Println("Installing python3-pip...")
			if err := apt.InstallPackage([]string{"python3-pip"}, verbose)(); err != nil {
				fmt.Println("Error installing python3-pip:", err)
				os.Exit(1)
			}

			// Create venv (using system python3)
			fmt.Println("Creating venv...")
			createVenv := exec.Command("python3", "-m", "venv", "venv")
			createVenv.Dir = constants.AnsibleVenvPath
			createVenv.Stdout = os.Stdout
			createVenv.Stderr = os.Stderr
			if err := createVenv.Run(); err != nil {
				fmt.Println("Error creating venv:", err)
				os.Exit(1)
			}
		}

		// --- Check for venv Python and wait ---
		fmt.Println("Checking for venv Python...")
		venvPythonPath := constants.AnsibleVenvPythonPath()
		maxWait := 10 * time.Second
		startTime := time.Now()

		for time.Since(startTime) < maxWait {
			if _, err := os.Stat(venvPythonPath); err == nil {
				fmt.Printf("Venv Python found at: %s\n", venvPythonPath)
				goto venvCheckComplete // Exit loop
			}
			fmt.Print(".")
			time.Sleep(1 * time.Second)
		}

		fmt.Printf("\nError: Virtual environment Python still not found after waiting %s\n", maxWait)
		os.Exit(1)

	venvCheckComplete:
		fmt.Println("\n--- Python Virtual Environment Setup (Verbose) Complete ---")

	} else {
		osRelease, _ := ubuntu.ParseOSRelease("/etc/os-release")
		versionCodename := osRelease["VERSION_CODENAME"]

		jammyRegex := regexp.MustCompile(`^jammy$`)
		nobleRegex := regexp.MustCompile(`^noble$`)

		if jammyRegex.MatchString(versionCodename) {
			_ = spinners.RunInfoSpinner("Ubuntu Jammy (or similar) detected, deploying venv with Python 3.12.")

			// Add deadsnakes PPA
			if err := spinners.RunTaskWithSpinner("Adding deadsnakes PPA", func() error {
				addPPA := apt.AddPPA("ppa:deadsnakes/ppa", verbose)
				return addPPA()
			}); err != nil {
				fmt.Println("Error adding deadsnakes PPA", err)
				os.Exit(1)
			}

			// Install Python 3.12 and venv
			if err := spinners.RunTaskWithSpinner("Installing Python 3.12 and venv", func() error {
				installPython := apt.InstallPackage([]string{"python3.12", "python3.12-dev", "python3.12-venv"}, verbose)
				return installPython()
			}); err != nil {
				fmt.Println("Error installing Python 3.12:", err)
				os.Exit(1)
			}

			// Ensure pip and setuptools are present in system python
			if err := spinners.RunTaskWithSpinner("Ensuring pip is present", func() error {
				ensurePip := exec.Command("python3.12", "-m", "ensurepip")
				ensurePip.Dir = constants.AnsibleVenvPath //Set working directory
				return ensurePip.Run()
			}); err != nil {
				//Don't exit here
				_ = spinners.RunWarningSpinner(fmt.Sprintf("Warning: Ensuring pip failed: %v", err))
			}

			// Create venv
			if err := spinners.RunTaskWithSpinner("Creating venv", func() error {
				createVenv := exec.Command("python3.12", "-m", "venv", "venv")
				createVenv.Dir = constants.AnsibleVenvPath
				return createVenv.Run()
			}); err != nil {
				fmt.Println("Error creating venv:", err)
				os.Exit(1)
			}

			// Install pip using get-pip.py (for Jammy)
			if err := spinners.RunTaskWithSpinner("Installing pip", func() error {
				return utils.InstallPip(verbose)
			}); err != nil {
				fmt.Println("Error installing pip", err) //Consistent error handling
				os.Exit(1)
			}

		} else if nobleRegex.MatchString(versionCodename) {
			_ = spinners.RunInfoSpinner("Ubuntu Noble (or similar) detected, deploying venv with Python 3.12.")

			// Install python3-pip (needed for Noble)
			if err := spinners.RunTaskWithSpinner("Installing python3-pip", func() error {
				installPipPackage := apt.InstallPackage([]string{"python3-pip"}, verbose)
				return installPipPackage()
			}); err != nil {
				fmt.Println("Error installing python3-pip:", err)
				os.Exit(1)
			}

			// Create venv (using system python3)
			if err := spinners.RunTaskWithSpinner("Creating venv", func() error {
				createVenv := exec.Command("python3", "-m", "venv", "venv")
				createVenv.Dir = constants.AnsibleVenvPath
				return createVenv.Run()
			}); err != nil {
				fmt.Println("Error creating venv:", err)
				os.Exit(1)
			}
		}

		// --- Check for venv Python and wait ---
		if err := spinners.RunTaskWithSpinner("Checking for venv Python", func() error {
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
}

// SaltboxRepo checks out the master branch of the Saltbox GitHub repository.
// Resets existing git repository folder if present.
// Runs submodule update.
func SaltboxRepo(verbose bool) {
	saltboxPath := constants.SaltboxRepoPath
	saltboxRepoURL := constants.SaltboxRepoURL
	branch := "master" // Or get this from a flag/config

	if verbose {
		fmt.Printf("--- Managing Saltbox Repository at %s (Branch: %s) (Verbose) ---\n", saltboxPath, branch)

		// Check if the Saltbox directory exists.
		_, err := os.Stat(saltboxPath)
		if os.IsNotExist(err) {
			// Clone the repository if it doesn't exist.
			fmt.Printf("Cloning Saltbox repository to %s (branch: %s)...\n", saltboxPath, branch)
			if err := git.CloneRepository(saltboxRepoURL, saltboxPath, branch, verbose); err != nil {
				fmt.Printf("Error cloning Saltbox repository: %v\n", err)
				os.Exit(1)
			}

			// Run submodule update after cloning.
			fmt.Println("Updating git submodules...")
			submoduleCmd := exec.Command("git", "submodule", "update", "--init", "--recursive")
			submoduleCmd.Dir = saltboxPath
			submoduleCmd.Stdout = os.Stdout
			submoduleCmd.Stderr = os.Stderr
			if err := submoduleCmd.Run(); err != nil {
				fmt.Printf("Error running git submodule update: %v\n", err)
				os.Exit(1)
			}

		} else if err != nil {
			// Handle errors other than "not exists" (e.g., permissions).
			fmt.Printf("Error checking for Saltbox directory: %v\n", err)
			os.Exit(1)

		} else {
			// The directory exists.  Check if it's a git repo.
			gitDirPath := filepath.Join(saltboxPath, ".git")
			_, err := os.Stat(gitDirPath)

			if os.IsNotExist(err) {
				// Not a git repo, initialize, fetch, and set up.
				fmt.Println("Saltbox directory exists but is not a git repository. Initializing...")

				initCmds := [][]string{
					{"git", "init"},
					{"git", "remote", "add", "origin", saltboxRepoURL},
					{"git", "fetch", "--all", "--prune"},
					{"git", "branch", branch, "origin/" + branch},
					{"git", "reset", "--hard", "origin/" + branch},
					{"git", "submodule", "update", "--init", "--recursive"},
				}

				fmt.Println("Initializing Git repository...")
				for _, command := range initCmds {
					cmd := exec.Command(command[0], command[1:]...)
					cmd.Dir = saltboxPath
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					fmt.Printf("Running: %s %s\n", command[0], command[1:])
					if err := cmd.Run(); err != nil {
						fmt.Errorf("error running command %v: %w", command, err) // Wrap the error
						os.Exit(1)
					}
				}

			} else if err != nil {
				// Handle errors other than "not exists" (e.g., permissions).
				fmt.Printf("Error checking for .git directory: %v\n", err)
				os.Exit(1)
			} else {
				// It's a git repo, fetch and reset
				fmt.Println("Updating existing Saltbox repository...")
				if err := git.FetchAndReset(saltboxPath, branch, "root", nil); err != nil {
					fmt.Printf("Error updating Saltbox repository: %v\n", err)
					os.Exit(1)
				}
			}
		}
		fmt.Println("Downloading and installing saltbox.fact...")
		if err := fact.DownloadAndInstallSaltboxFact(false, verbose); err != nil { // Assuming fact function doesn't need verbose here
			fmt.Printf("Error downloading and installing saltbox.fact: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Copying default configuration files...")
		if err := CopyDefaultConfigFiles(verbose); err != nil {
			fmt.Printf("Error copying default configuration files: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("--- Saltbox Repository Management (Verbose) Complete ---")

	} else {
		saltboxPath := constants.SaltboxRepoPath
		saltboxRepoURL := constants.SaltboxRepoURL
		branch := "master" // Or get this from a flag/config

		// Check if the Saltbox directory exists.  No spinner for the os.Stat itself.
		_, err := os.Stat(saltboxPath)
		if os.IsNotExist(err) {
			// Clone the repository if it doesn't exist.  Use a spinner.
			if err := spinners.RunTaskWithSpinner(fmt.Sprintf("Cloning Saltbox repository to %s (branch: %s)", saltboxPath, branch), func() error {
				return git.CloneRepository(saltboxRepoURL, saltboxPath, branch, verbose)
			}); err != nil {
				fmt.Printf("Error cloning Saltbox repository: %v\n", err)
				os.Exit(1)
			}

			// Run submodule update after cloning. Use spinner.
			if err := spinners.RunTaskWithSpinner("Updating git submodules", func() error {
				submoduleCmd := exec.Command("git", "submodule", "update", "--init", "--recursive")
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
			// The directory exists.  Check if it's a git repo.
			gitDirPath := filepath.Join(saltboxPath, ".git")
			_, err := os.Stat(gitDirPath)

			if os.IsNotExist(err) {
				// Not a git repo, initialize, fetch, and set up.
				_ = spinners.RunInfoSpinner("Saltbox directory exists but is not a git repository.  Initializing...")

				initCmds := [][]string{
					{"git", "init"},
					{"git", "remote", "add", "origin", saltboxRepoURL},
					{"git", "fetch", "--all", "--prune"},
					{"git", "branch", branch, "origin/" + branch},
					{"git", "reset", "--hard", "origin/" + branch},
					{"git", "submodule", "update", "--init", "--recursive"},
				}

				// Wrap the entire loop in a spinner
				if err := spinners.RunTaskWithSpinner("Initializing Git repository", func() error {
					for _, command := range initCmds {
						cmd := exec.Command(command[0], command[1:]...)
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
				if err := spinners.RunTaskWithSpinner("Updating existing Saltbox repository", func() error {
					return git.FetchAndReset(saltboxPath, branch, "root", nil) // Assuming root user
				}); err != nil {
					fmt.Printf("Error updating Saltbox repository: %v\n", err)
					os.Exit(1)
				}
			}
		}
		if err := fact.DownloadAndInstallSaltboxFact(false, verbose); err != nil {
			fmt.Printf("Error downloading and installing saltbox.fact: %v\n", err)
			os.Exit(1)
		}
		if err := CopyDefaultConfigFiles(verbose); err != nil {
			fmt.Printf("Error copying default configuration files: %v\n", err)
			os.Exit(1)
		}
	}
}

// InstallPipDependencies installs pip dependencies in the Ansible virtual environment.
func InstallPipDependencies(verbose bool) {
	venvPythonPath := constants.AnsibleVenvPythonPath()
	python3Cmd := []string{venvPythonPath, "-m", "pip", "install", "--timeout=360", "--no-cache-dir", "--disable-pip-version-check", "--upgrade"}

	if verbose {
		fmt.Println("--- Installing Pip Dependencies (Verbose) ---")

		// Install pip, setuptools, and wheel
		fmt.Println("Installing pip, setuptools, and wheel...")
		installBaseDeps := append(python3Cmd, "pip", "setuptools", "wheel")
		cmdInstallBase := exec.Command(installBaseDeps[0], installBaseDeps[1:]...)
		cmdInstallBase.Stdout = os.Stdout
		cmdInstallBase.Stderr = os.Stderr
		if err := cmdInstallBase.Run(); err != nil {
			fmt.Println("Error installing pip, setuptools, and wheel:", err)
			os.Exit(1)
		}

		// Install requirements from requirements-saltbox.txt
		fmt.Println("Installing requirements from requirements-saltbox.txt...")
		requirementsPath := filepath.Join(constants.SaltboxRepoPath, "requirements", "requirements-saltbox.txt")
		installRequirements := append(python3Cmd, "--requirement", requirementsPath)
		cmdInstallReq := exec.Command(installRequirements[0], installRequirements[1:]...)
		cmdInstallReq.Stdout = os.Stdout
		cmdInstallReq.Stderr = os.Stderr
		if err := cmdInstallReq.Run(); err != nil {
			fmt.Println("Error installing requirements from requirements-saltbox.txt:", err)
			os.Exit(1)
		}

		fmt.Println("--- Pip Dependencies Installation (Verbose) Complete ---")

	} else {
		// Install pip, setuptools, and wheel
		if err := spinners.RunTaskWithSpinner("Installing pip, setuptools, and wheel", func() error {
			installBaseDeps := append(python3Cmd, "pip", "setuptools", "wheel")
			cmdInstallBase := exec.Command(installBaseDeps[0], installBaseDeps[1:]...)
			return cmdInstallBase.Run()
		}); err != nil {
			fmt.Println("Error installing pip, setuptools, and wheel:", err)
			os.Exit(1)
		}

		// Install requirements from requirements-saltbox.txt
		if err := spinners.RunTaskWithSpinner("Installing requirements from requirements-saltbox.txt", func() error {
			requirementsPath := filepath.Join(constants.SaltboxRepoPath, "requirements", "requirements-saltbox.txt")
			installRequirements := append(python3Cmd, "--requirement", requirementsPath)
			cmdInstallReq := exec.Command(installRequirements[0], installRequirements[1:]...)
			return cmdInstallReq.Run()
		}); err != nil {
			fmt.Println("Error installing requirements from requirements-saltbox.txt:", err)
			os.Exit(1)
		}
	}
}

// CopyAnsibleBinaries copies Ansible binaries from the virtual environment to /usr/local/bin.
func CopyAnsibleBinaries(verbose bool) {
	if verbose {
		fmt.Println("--- Copying Ansible Binaries to /usr/local/bin (Verbose) ---")

		ansibleBinDir := filepath.Join(constants.AnsibleVenvPath, "venv", "bin")
		destDir := "/usr/local/bin"
		fmt.Printf("Source directory: %s\n", ansibleBinDir)
		fmt.Printf("Destination directory: %s\n", destDir)

		files, err := os.ReadDir(ansibleBinDir)
		if err != nil {
			fmt.Errorf("error reading Ansible bin directory: %w", err) // Wrap error
			return
		}

		for _, file := range files {
			if strings.HasPrefix(file.Name(), "ansible") {
				srcPath := filepath.Join(ansibleBinDir, file.Name())
				destPath := filepath.Join(destDir, file.Name())
				fmt.Printf("Copying %s to %s...\n", srcPath, destPath)

				// Open source file
				srcFile, err := os.Open(srcPath)
				if err != nil {
					fmt.Errorf("error opening source file %s: %w", srcPath, err) // Wrap error
					continue
				}

				// Create destination file
				destFile, err := os.Create(destPath)
				if err != nil {
					srcFile.Close()                                                     // Close srcFile before exiting
					fmt.Errorf("error creating destination file %s: %w", destPath, err) // Wrap error
					continue
				}

				// Set permissions on destination file
				if err := os.Chmod(destPath, 0755); err != nil {
					srcFile.Close()
					destFile.Close()
					fmt.Errorf("error setting permissions on destination file %s: %w", destPath, err) //Wrap
					continue
				}

				// Copy contents
				_, err = io.Copy(destFile, srcFile)
				srcFile.Close()  // Close srcFile *after* the copy
				destFile.Close() //close destFile *after* copy

				if err != nil {
					fmt.Errorf("error copying file %s to %s: %w", srcPath, destPath, err) // Wrap error
					continue
				}
			}
		}
		fmt.Println("--- Ansible Binaries Copied (Verbose) ---")

	} else {
		if err := spinners.RunTaskWithSpinner("Copying Ansible binaries to /usr/local/bin", func() error {
			ansibleBinDir := filepath.Join(constants.AnsibleVenvPath, "venv", "bin")
			destDir := "/usr/local/bin"
			files, err := os.ReadDir(ansibleBinDir)
			if err != nil {
				return fmt.Errorf("error reading Ansible bin directory: %w", err) // Wrap error
			}

			for _, file := range files {
				if strings.HasPrefix(file.Name(), "ansible") {
					srcPath := filepath.Join(ansibleBinDir, file.Name())
					destPath := filepath.Join(destDir, file.Name())

					// Open source file
					srcFile, err := os.Open(srcPath)
					if err != nil {
						return fmt.Errorf("error opening source file %s: %w", srcPath, err) // Wrap error
					}

					// Create destination file
					destFile, err := os.Create(destPath)
					if err != nil {
						srcFile.Close()                                                            // Close srcFile before exiting
						return fmt.Errorf("error creating destination file %s: %w", destPath, err) // Wrap error
					}

					// Set permissions on destination file
					if err := os.Chmod(destPath, 0755); err != nil {
						srcFile.Close()
						destFile.Close()
						return fmt.Errorf("error setting permissions on destination file %s: %w", destPath, err) //Wrap
					}

					// Copy contents
					_, err = io.Copy(destFile, srcFile)
					srcFile.Close()  // Close srcFile *after* the copy
					destFile.Close() //close destFile *after* copy

					if err != nil {
						return fmt.Errorf("error copying file %s to %s: %w", srcPath, destPath, err) // Wrap error
					}
				}
			}
			return nil
		}); err != nil {
			fmt.Println("Error copying Ansible binaries:", err)
			os.Exit(1)

		}
	}
}

// CopyDefaultConfigFiles copies default config files into the Saltbox folder.
func CopyDefaultConfigFiles(verbose bool) error {
	saltboxPath := constants.SaltboxRepoPath
	defaultsDir := filepath.Join(saltboxPath, "defaults")
	files, err := filepath.Glob(filepath.Join(defaultsDir, "*.default"))
	if err != nil {
		return fmt.Errorf("error listing default config files: %w", err)
	}

	if verbose {
		fmt.Println("--- Copying Default Configuration Files (Verbose) ---")
		fmt.Printf("Defaults directory: %s\n", defaultsDir)
		fmt.Printf("Destination directory: %s\n", saltboxPath)

		for _, srcPath := range files {
			baseName := filepath.Base(srcPath)
			destName := strings.TrimSuffix(baseName, ".default")
			destPath := filepath.Join(saltboxPath, destName)

			fmt.Printf("Processing %s -> %s\n", baseName, destName)

			// Check if the destination file already exists.
			if _, err := os.Stat(destPath); os.IsNotExist(err) {
				// Destination file doesn't exist, proceed with copying.
				fmt.Printf("Copying %s...\n", baseName)
				srcFile, err := os.Open(srcPath)
				if err != nil {
					fmt.Errorf("error opening source file %s: %w", srcPath, err)
					continue
				}
				defer srcFile.Close()

				destFile, err := os.Create(destPath)
				if err != nil {
					fmt.Errorf("error creating destination file %s: %w", destPath, err)
					continue
				}
				defer destFile.Close()

				// Set Permissions
				if err := os.Chmod(destPath, 0755); err != nil {
					fmt.Errorf("error setting permissions on destination file %s: %w", destPath, err)
					continue
				}
				if _, err := io.Copy(destFile, srcFile); err != nil {
					fmt.Errorf("error copying file %s to %s: %w", srcPath, destPath, err)
					continue
				}
			} else if err != nil {
				// os.Stat error other than IsNotExist
				fmt.Errorf("could not check file %s: %w", destPath, err)
			} else {
				fmt.Printf("File %s already exists, skipping.\n", destName)
			}
		}
		fmt.Println("--- Default Configuration Files Copied (Verbose) ---")

	} else {
		for _, srcPath := range files {
			baseName := filepath.Base(srcPath)
			destName := strings.TrimSuffix(baseName, ".default")
			destPath := filepath.Join(saltboxPath, destName)

			// Check if the destination file already exists.
			if _, err := os.Stat(destPath); os.IsNotExist(err) {
				// Destination file doesn't exist, proceed with copying.
				if err := spinners.RunTaskWithSpinner(fmt.Sprintf("Copying %s", baseName), func() error {
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
						return fmt.Errorf("error setting permissions on destination file: %w", err)
					}
					if _, err := io.Copy(destFile, srcFile); err != nil {
						return fmt.Errorf("error copying file %s to %s: %w", srcPath, destPath, err)
					}
					return nil
				}); err != nil {
					return err // RunTaskWithSpinner already formats error, no need to wrap again
				}
			} else if err != nil {
				// os.Stat error other than IsNotExist
				return fmt.Errorf("could not check file %s: %w", destPath, err)
			}
		}
	}

	return nil
}
