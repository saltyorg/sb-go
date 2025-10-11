package venv

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/saltyorg/sb-go/internal/apt"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/spinners"
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
		pythonMissing, err = checkPythonVersion(ansibleVenvPath, venvPythonPath)
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

		// Detect OS release
		fmt.Println("Detecting OS release...")
		var release string
		release, err = detectOSRelease(ctx)
		if err != nil {
			return fmt.Errorf("error detecting OS release: %w", err)
		}
		fmt.Printf("Detected OS Release: %s\n", release)

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
			if err := createVirtualEnv(ctx, ansibleVenvPath, release, verbose); err != nil {
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
		if err := spinners.RunTaskWithSpinner("Checking Python version", func() error {
			var err error
			pythonMissing, err = checkPythonVersion(ansibleVenvPath, venvPythonPath)
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

		// Detect OS release
		var release string
		if err := spinners.RunTaskWithSpinnerContext(ctx, "Detecting OS release", func() error {
			var err error
			release, err = detectOSRelease(ctx)
			return err
		}); err != nil {
			return fmt.Errorf("error detecting OS release: %w", err)
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
				return createVirtualEnv(ctx, ansibleVenvPath, release, verbose)
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
func checkPythonVersion(ansibleVenvPath, venvPythonPath string) (bool, error) {
	if _, err := os.Stat(filepath.Join(ansibleVenvPath, "venv", "bin")); err == nil {
		if _, err := os.Stat(venvPythonPath); os.IsNotExist(err) {
			return true, nil
		} else if err != nil {
			return false, fmt.Errorf("error checking python version: %w", err)
		}
	}
	return false, nil
}

// detectOSRelease detects the OS release.
func detectOSRelease(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "lsb_release", "-cs")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error running lsb_release: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// removeExistingVenv removes the existing virtual environment.
func removeExistingVenv(ctx context.Context, ansibleVenvPath string) error {
	cmd := exec.CommandContext(ctx, "rm", "-rf", ansibleVenvPath)
	return cmd.Run()
}

// createVirtualEnv creates the virtual environment.
func createVirtualEnv(ctx context.Context, ansibleVenvPath, release string, verbose bool) error {
	env := os.Environ()
	env = append(env, "DEBIAN_FRONTEND=noninteractive")
	pythonCmd := fmt.Sprintf("python%s", constants.AnsibleVenvPythonVersion)

	if release == "focal" || release == "jammy" {
		if err := runCommand(ctx, []string{"add-apt-repository", "ppa:deadsnakes/ppa", "--yes"}, env, verbose); err != nil {
			return fmt.Errorf("error adding python ppa: %w", err)
		}
		if err := runCommand(ctx, []string{"apt-get", "update"}, env, verbose); err != nil {
			return fmt.Errorf("error running apt update: %w", err)
		}
		if err := runCommand(ctx, []string{"apt-get", "install", fmt.Sprintf("python%s", constants.AnsibleVenvPythonVersion), fmt.Sprintf("python%s-dev", constants.AnsibleVenvPythonVersion), fmt.Sprintf("python%s-venv", constants.AnsibleVenvPythonVersion), "-y"}, env, verbose); err != nil {
			return fmt.Errorf("error installing python: %w", err)
		}
		if err := runCommand(ctx, []string{pythonCmd, "-m", "ensurepip"}, env, verbose); err != nil {
			return fmt.Errorf("error ensuring pip: %w", err)
		}
	}
	if err := os.MkdirAll(ansibleVenvPath, 0755); err != nil {
		return fmt.Errorf("error creating venv dir: %w", err)
	}

	cmd := exec.CommandContext(ctx, pythonCmd, "-m", "venv", "venv")
	cmd.Dir = ansibleVenvPath
	return cmd.Run()
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

// copyBinaries copies the binaries.
func copyBinaries(ctx context.Context, ansibleVenvPath string, verbose bool) error {
	binaries := []string{"ansible*", "certbot", "apprise"}
	env := os.Environ()

	for _, binary := range binaries {
		src := filepath.Join(ansibleVenvPath, "venv", "bin", binary)
		dst := "/usr/local/bin/"

		command := []string{"sh", "-c", fmt.Sprintf("cp %s %s", src, dst)}

		if err := runCommand(ctx, command, env, verbose); err != nil {
			return fmt.Errorf("error copying %s: %w", binary, err)
		}
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
	}
	return cmd.Run()
}
