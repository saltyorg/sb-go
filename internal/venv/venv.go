package venv

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/saltyorg/sb-go/internal/apt"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/spinners"
	"github.com/saltyorg/sb-go/internal/uv"
)

// ManageAnsibleVenv manages the Ansible virtual environment.
// The context parameter allows for cancellation of long-running operations.
func ManageAnsibleVenv(ctx context.Context, forceRecreate bool, saltboxUser string, verbose bool) error {
	ansibleVenvPath := constants.AnsibleVenvPath
	venvPythonPath := constants.AnsibleVenvPythonPath()
	pythonMissing := false

	if verbose {
		fmt.Println("--- Managing Ansible Virtual Environment (Verbose) ---")
		fmt.Printf("Force Recreate: %t, Saltbox User: %s\n", forceRecreate, saltboxUser)

		// Check the Python version
		fmt.Println("Checking Python version...")
		var err error
		pythonMissing, err = checkPythonVersion(ctx, ansibleVenvPath, venvPythonPath)
		if err != nil {
			return fmt.Errorf("error checking python version: %w", err)
		}
		fmt.Printf("Python Missing: %t\n", pythonMissing)

		recreate := forceRecreate || pythonMissing
		fmt.Printf("Recreate Venv: %t\n", recreate)

		if forceRecreate {
			fmt.Println("Recreate flag set, forcing recreation of Ansible venv")
		} else if pythonMissing {
			fmt.Println("Python 3.12 not detected in venv, recreation required")
		}

		if recreate {
			fmt.Println("Recreating Ansible venv...")
		} else {
			fmt.Println("Updating Ansible venv...")
		}

		if recreate {
			// Remove existing venv
			fmt.Println("Removing existing venv...")
			if err := removeExistingVenv(ctx, ansibleVenvPath); err != nil {
				return fmt.Errorf("error removing existing venv: %w", err)
			}
		}

		if _, err := os.Stat(ansibleVenvPath); os.IsNotExist(err) {
			// Create venv
			fmt.Println("Creating virtual environment...")
			if err := createVirtualEnv(ctx, ansibleVenvPath, verbose); err != nil {
				return fmt.Errorf("error creating virtual environment: %w", err)
			}
		}

		// Upgrade pip
		fmt.Println("Upgrading pip...")
		if err := upgradePip(ctx, ansibleVenvPath, verbose); err != nil {
			return fmt.Errorf("error upgrading pip: %w", err)
		}

		// Install libpq-dev dependency
		fmt.Println("Installing libpq-dev...")
		if err := apt.InstallPackage(ctx, []string{"libpq-dev"}, verbose)(); err != nil {
			return fmt.Errorf("error installing libpq-dev: %w", err)
		}

		// Install requirements
		fmt.Println("Installing pip requirements...")
		if err := installRequirements(ctx, ansibleVenvPath, verbose); err != nil {
			return fmt.Errorf("error installing pip requirements: %w", err)
		}

		// Copy binaries
		fmt.Println("Copying binaries...")
		if err := copyBinaries(ctx, ansibleVenvPath, verbose); err != nil {
			return fmt.Errorf("error copying binaries: %w", err)
		}

		// Set ownership
		fmt.Printf("Setting ownership to user: %s...\n", saltboxUser)
		if err := setOwnership(ctx, ansibleVenvPath, saltboxUser, verbose); err != nil {
			return fmt.Errorf("error setting ownership: %w", err)
		}

		if recreate {
			fmt.Println("Done recreating Ansible venv")
		} else {
			fmt.Println("Done updating Ansible venv")
		}

		fmt.Println("--- Ansible Virtual Environment Management (Verbose) Complete ---")

	} else {
		// Check the Python version
		if err := spinners.RunTaskWithSpinnerContext(ctx, "Checking Python version", func() error {
			var err error
			pythonMissing, err = checkPythonVersion(ctx, ansibleVenvPath, venvPythonPath)
			return err
		}); err != nil {
			return fmt.Errorf("error checking python version: %w", err)
		}

		recreate := forceRecreate || pythonMissing

		if forceRecreate {
			if err := spinners.RunInfoSpinner("Recreate flag set, forcing recreation of Ansible venv"); err != nil {
				return err
			}
		} else if pythonMissing {
			if err := spinners.RunWarningSpinner("Python 3.12 not detected in venv, recreation required"); err != nil {
				return err
			}
		}

		if recreate {
			if err := spinners.RunInfoSpinner("Recreating Ansible venv"); err != nil {
				return err
			}
		} else {
			if err := spinners.RunInfoSpinner("Updating Ansible venv"); err != nil {
				return err
			}
		}

		if recreate {
			// Remove existing venv
			if err := spinners.RunTaskWithSpinnerContext(ctx, "Removing existing venv", func() error {
				return removeExistingVenv(ctx, ansibleVenvPath)
			}); err != nil {
				return fmt.Errorf("error removing existing venv: %w", err)
			}
		}

		if _, err := os.Stat(ansibleVenvPath); os.IsNotExist(err) {
			// Create venv
			if err := spinners.RunTaskWithSpinnerContext(ctx, "Creating virtual environment", func() error {
				return createVirtualEnv(ctx, ansibleVenvPath, verbose)
			}); err != nil {
				return fmt.Errorf("error creating virtual environment: %w", err)
			}
		}

		// Upgrade pip
		if err := spinners.RunTaskWithSpinnerContext(ctx, "Upgrading pip", func() error {
			return upgradePip(ctx, ansibleVenvPath, verbose)
		}); err != nil {
			return fmt.Errorf("error upgrading pip: %w", err)
		}

		// Install libpq-dev dependency
		if err := spinners.RunTaskWithSpinnerContext(ctx, "Installing libpq-dev", func() error {
			return apt.InstallPackage(ctx, []string{"libpq-dev"}, verbose)()
		}); err != nil {
			return fmt.Errorf("error installing libpq-dev: %w", err)
		}

		// Install requirements
		if err := spinners.RunTaskWithSpinnerContext(ctx, "Installing pip requirements", func() error {
			return installRequirements(ctx, ansibleVenvPath, verbose)
		}); err != nil {
			return fmt.Errorf("error installing pip requirements: %w", err)
		}

		// Copy binaries
		if err := spinners.RunTaskWithSpinnerContext(ctx, "Copying binaries", func() error {
			return copyBinaries(ctx, ansibleVenvPath, verbose)
		}); err != nil {
			return fmt.Errorf("error copying binaries: %w", err)
		}

		// Set ownership
		if err := spinners.RunTaskWithSpinnerContext(ctx, "Setting ownership", func() error {
			return setOwnership(ctx, ansibleVenvPath, saltboxUser, verbose)
		}); err != nil {
			return fmt.Errorf("error setting ownership: %w", err)
		}

		if recreate {
			if err := spinners.RunInfoSpinner("Done recreating Ansible venv"); err != nil {
				return err
			}
		} else {
			if err := spinners.RunInfoSpinner("Done updating Ansible venv"); err != nil {
				return err
			}
		}
	}

	return nil
}

// checkPythonVersion checks if the Python version is correct.
// Returns true if Python is missing or pointing to wrong version (needs recreation).
func checkPythonVersion(ctx context.Context, ansibleVenvPath, venvPythonPath string) (bool, error) {
	// Check if venv bin directory exists
	if _, err := os.Stat(filepath.Join(ansibleVenvPath, "venv", "bin")); err != nil {
		// Venv doesn't exist, needs creation
		return false, nil
	}

	// Check if Python binary exists at expected path
	if _, err := os.Stat(venvPythonPath); os.IsNotExist(err) {
		// Python binary missing, needs recreation
		return true, nil
	} else if err != nil {
		return false, fmt.Errorf("error checking python path: %w", err)
	}

	// Python binary exists, now verify it's the correct version from uv
	// Run python --version to get the actual version
	cmd := exec.CommandContext(ctx, venvPythonPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Can't run Python, needs recreation
		return true, nil
	}

	// Parse version output (format: "Python 3.12.x")
	versionStr := strings.TrimSpace(string(output))
	if !strings.HasPrefix(versionStr, "Python "+constants.AnsibleVenvPythonVersion) {
		// Wrong Python version, needs recreation
		return true, nil
	}

	// Check if Python is from uv installation by checking if it resolves to /srv/python
	// Follow symlinks to find the real path
	realPath, err := filepath.EvalSymlinks(venvPythonPath)
	if err != nil {
		// Can't resolve symlink, needs recreation
		return true, nil
	}

	// If Python is from uv, it should be in /srv/python directory
	if !strings.HasPrefix(realPath, constants.PythonInstallDir) {
		// Not from uv installation (probably deadsnakes), needs recreation
		return true, nil
	}

	// Final check: verify Python can actually import modules (not just run --version)
	// This catches broken venv that have correct symlinks but broken Python paths
	cmd = exec.CommandContext(ctx, venvPythonPath, "-c", "import encodings, sys; sys.exit(0)")
	if err := cmd.Run(); err != nil {
		// Python can't import basic modules, venv is broken, needs recreation
		return true, nil
	}

	// All checks passed, venv is good
	return false, nil
}

// removeExistingVenv removes the existing virtual environment.
func removeExistingVenv(ctx context.Context, ansibleVenvPath string) error {
	cmd := exec.CommandContext(ctx, "rm", "-rf", ansibleVenvPath)
	return cmd.Run()
}

// createVirtualEnv creates the virtual environment using uv.
func createVirtualEnv(ctx context.Context, ansibleVenvPath string, verbose bool) error {
	// Ensure uv is installed
	if err := uv.DownloadAndInstallUV(ctx, verbose); err != nil {
		return fmt.Errorf("error installing uv: %w", err)
	}

	// Create /srv/python directory if it doesn't exist
	if err := os.MkdirAll(constants.PythonInstallDir, 0755); err != nil {
		return fmt.Errorf("error creating python install dir: %w", err)
	}

	// Install Python using uv
	if err := uv.InstallPython(ctx, constants.AnsibleVenvPythonVersion, verbose); err != nil {
		return fmt.Errorf("error installing python: %w", err)
	}

	// Create the venv directory
	if err := os.MkdirAll(ansibleVenvPath, 0755); err != nil {
		return fmt.Errorf("error creating venv dir: %w", err)
	}

	// Create venv using uv
	venvPath := filepath.Join(ansibleVenvPath, "venv")
	if err := uv.CreateVenv(ctx, venvPath, constants.AnsibleVenvPythonVersion, verbose); err != nil {
		return fmt.Errorf("error creating venv: %w", err)
	}

	return nil
}

// upgradePip upgrades pip, setuptools, and wheel.
func upgradePip(ctx context.Context, ansibleVenvPath string, verbose bool) error {
	pythonPath := filepath.Join(ansibleVenvPath, "venv", "bin", fmt.Sprintf("python%s", constants.AnsibleVenvPythonVersion))
	command := []string{pythonPath, "-m", "pip", "install", "--no-cache-dir", "--disable-pip-version-check", "--upgrade", "pip", "setuptools", "wheel"}
	env := os.Environ() // Inherit current environment

	return runCommand(ctx, command, env, verbose)
}

// installRequirements installs the requirements.
func installRequirements(ctx context.Context, ansibleVenvPath string, verbose bool) error {
	pythonPath := filepath.Join(ansibleVenvPath, "venv", "bin", fmt.Sprintf("python%s", constants.AnsibleVenvPythonVersion))
	command := []string{pythonPath, "-m", "pip", "install", "--no-cache-dir", "--disable-pip-version-check", "--upgrade", "--requirement", constants.AnsibleRequirementsPath}
	env := os.Environ()

	return runCommand(ctx, command, env, verbose)
}

// copyBinaries copies the binaries in a robust and error-checked way.
func copyBinaries(ctx context.Context, ansibleVenvPath string, verbose bool) error {
	venvBinDir := filepath.Join(ansibleVenvPath, "venv", "bin")
	destDir := "/usr/local/bin/"

	// A list of patterns to find. "ansible*" is a glob pattern.
	patterns := []string{"ansible*", "certbot", "apprise"}
	var sourcesToCopy []string

	// --- Step 1: Find all the files that match the patterns ---
	for _, pattern := range patterns {
		// Construct the full path for the pattern
		fullPattern := filepath.Join(venvBinDir, pattern)

		// Use filepath.Glob to find all matching files.
		// This is safer than relying on shell expansion.
		matches, err := filepath.Glob(fullPattern)
		if err != nil {
			return fmt.Errorf("error finding files for pattern %s: %w", pattern, err)
		}

		// If a pattern (like "certbot") matches no files, Glob returns an empty slice.
		// We should treat this as an error.
		if len(matches) == 0 {
			// Provide a specific error if a required binary is missing.
			return fmt.Errorf("required binary not found in venv: %s", pattern)
		}

		sourcesToCopy = append(sourcesToCopy, matches...)
	}

	// --- Step 2: Copy each file individually ---
	for _, srcPath := range sourcesToCopy {
		// Get the base filename (e.g., "ansible-playbook")
		fileName := filepath.Base(srcPath)
		destPath := filepath.Join(destDir, fileName)

		if verbose {
			fmt.Printf("Copying %s to %s\n", srcPath, destPath)
		}

		// Perform the copy using Go's native functions.
		if err := copyFile(srcPath, destPath); err != nil {
			return fmt.Errorf("failed to copy %s: %w", fileName, err)
		}
	}

	return nil
}

// copyFile is a helper function to copy a single file and apply the source's permissions.
func copyFile(src, dst string) error {
	// Get file info from the source to read its permissions.
	sourceFileInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("could not stat source file: %w", err)
	}

	// Open the source file for reading.
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("could not open source file: %w", err)
	}
	defer sourceFile.Close()

	// Create the destination file.
	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("could not create destination file: %w", err)
	}
	defer destFile.Close()

	// Copy the contents.
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("could not copy file contents: %w", err)
	}

	// Apply the original file's permissions to the new file.
	err = os.Chmod(dst, sourceFileInfo.Mode())
	if err != nil {
		return fmt.Errorf("could not set permissions on destination file: %w", err)
	}

	return nil
}

// setOwnership sets the ownership.
func setOwnership(ctx context.Context, ansibleVenvPath, saltboxUser string, verbose bool) error {
	command := []string{"chown", "-R", fmt.Sprintf("%s:%s", saltboxUser, saltboxUser), ansibleVenvPath}
	env := os.Environ()

	return runCommand(ctx, command, env, verbose)
}

// runCommand runs a command with the given environment.
func runCommand(ctx context.Context, command []string, env []string, verbose bool) error {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Env = env

	if verbose {
		fmt.Println("Running command:", command)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// For non-verbose mode, capture the output to prevent the child process
	// from interfering with the terminal. This is the fix.
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Provide the captured output for better error diagnosis.
		return fmt.Errorf("command failed: %w\nOutput:\n%s", err, string(output))
	}
	return nil
}
