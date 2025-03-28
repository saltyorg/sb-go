package venv

import (
	"fmt"
	"github.com/saltyorg/sb-go/constants"
	"github.com/saltyorg/sb-go/spinners"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ManageAnsibleVenv manages the Ansible virtual environment.
func ManageAnsibleVenv(forceRecreate bool, saltboxUser string) error {
	ansibleVenvPath := constants.AnsibleVenvPath
	venvPythonPath := constants.AnsibleVenvPythonPath()
	pythonMissing := false

	// Check Python version
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
	if err := spinners.RunTaskWithSpinner("Detecting OS release", func() error {
		var err error
		release, err = detectOSRelease()
		return err
	}); err != nil {
		return fmt.Errorf("error detecting OS release: %w", err)
	}

	if recreate {
		// Remove existing venv
		if err := spinners.RunTaskWithSpinner("Removing existing venv", func() error {
			return removeExistingVenv(ansibleVenvPath)
		}); err != nil {
			return fmt.Errorf("error removing existing venv: %w", err)
		}
	}

	if _, err := os.Stat(ansibleVenvPath); os.IsNotExist(err) {
		// Create venv
		if err := spinners.RunTaskWithSpinner("Creating virtual environment", func() error {
			return createVirtualEnv(ansibleVenvPath, release)
		}); err != nil {
			return fmt.Errorf("error creating virtual environment: %w", err)
		}
	}

	// Upgrade pip
	if err := spinners.RunTaskWithSpinner("Upgrading pip", func() error {
		return upgradePip(ansibleVenvPath)
	}); err != nil {
		return fmt.Errorf("error upgrading pip: %w", err)
	}

	// Install requirements
	if err := spinners.RunTaskWithSpinner("Installing pip requirements", func() error {
		return installRequirements(ansibleVenvPath)
	}); err != nil {
		return fmt.Errorf("error installing pip requirements: %w", err)
	}

	// Copy binaries
	if err := spinners.RunTaskWithSpinner("Copying binaries", func() error {
		return copyBinaries(ansibleVenvPath)
	}); err != nil {
		return fmt.Errorf("error copying binaries: %w", err)
	}

	// Set ownership
	if err := spinners.RunTaskWithSpinner("Setting ownership", func() error {
		return setOwnership(ansibleVenvPath, saltboxUser)
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
func detectOSRelease() (string, error) {
	cmd := exec.Command("lsb_release", "-cs")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error running lsb_release: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// removeExistingVenv removes the existing virtual environment.
func removeExistingVenv(ansibleVenvPath string) error {
	cmd := exec.Command("rm", "-rf", ansibleVenvPath)
	return cmd.Run()
}

// createVirtualEnv creates the virtual environment.
func createVirtualEnv(ansibleVenvPath, release string) error {
	env := os.Environ()
	env = append(env, "DEBIAN_FRONTEND=noninteractive")
	pythonCmd := fmt.Sprintf("python%s", constants.AnsibleVenvPythonVersion)

	if release == "focal" || release == "jammy" {
		if err := runCommand([]string{"add-apt-repository", "ppa:deadsnakes/ppa", "--yes"}, env); err != nil {
			return fmt.Errorf("error adding python ppa: %w", err)
		}
		if err := runCommand([]string{"apt-get", "update"}, env); err != nil {
			return fmt.Errorf("error running apt update: %w", err)
		}
		if err := runCommand([]string{"apt-get", "install", fmt.Sprintf("python%s", constants.AnsibleVenvPythonVersion), fmt.Sprintf("python%s-dev", constants.AnsibleVenvPythonVersion), fmt.Sprintf("python%s-venv", constants.AnsibleVenvPythonVersion), "-y"}, env); err != nil {
			return fmt.Errorf("error installing python: %w", err)
		}
		if err := runCommand([]string{pythonCmd, "-m", "ensurepip"}, env); err != nil {
			return fmt.Errorf("error ensuring pip: %w", err)
		}
	}
	if err := os.MkdirAll(ansibleVenvPath, 0755); err != nil {
		return fmt.Errorf("error creating venv dir: %w", err)
	}

	cmd := exec.Command(pythonCmd, "-m", "venv", "venv")
	cmd.Dir = ansibleVenvPath
	return cmd.Run()
}

// upgradePip upgrades pip, setuptools, and wheel.
func upgradePip(ansibleVenvPath string) error {
	pythonPath := filepath.Join(ansibleVenvPath, "venv", "bin", fmt.Sprintf("python%s", constants.AnsibleVenvPythonVersion))
	cmd := exec.Command(pythonPath, "-m", "pip", "install", "--no-cache-dir", "--disable-pip-version-check", "--upgrade", "pip", "setuptools", "wheel")
	return cmd.Run()
}

// installRequirements installs the requirements.
func installRequirements(ansibleVenvPath string) error {
	pythonPath := filepath.Join(ansibleVenvPath, "venv", "bin", fmt.Sprintf("python%s", constants.AnsibleVenvPythonVersion))
	cmd := exec.Command(pythonPath, "-m", "pip", "install", "--no-cache-dir", "--disable-pip-version-check", "--upgrade", "--requirement", constants.AnsibleRequirementsPath)
	return cmd.Run()
}

// copyBinaries copies the binaries.
func copyBinaries(ansibleVenvPath string) error {
	binaries := []string{"ansible*", "certbot", "apprise"}
	for _, binary := range binaries {
		src := filepath.Join(ansibleVenvPath, "venv", "bin", binary)
		dst := "/usr/local/bin/"

		cmd := exec.Command("sh", "-c", fmt.Sprintf("cp %s %s", src, dst))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error copying %s: %w", binary, err)
		}
	}
	return nil
}

// setOwnership sets the ownership.
func setOwnership(ansibleVenvPath, saltboxUser string) error {
	cmd := exec.Command("chown", "-R", fmt.Sprintf("%s:%s", saltboxUser, saltboxUser), ansibleVenvPath)
	return cmd.Run()
}

// runCommand runs a command with the given environment.
func runCommand(command []string, env []string) error {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = env
	return cmd.Run()
}
