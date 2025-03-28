package cmd

import (
	"bytes"
	"fmt"
	"github.com/saltyorg/sb-go/utils"
	"github.com/saltyorg/sb-go/venv"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// reinstallPythonCmd represents the reinstallPython command
var reinstallPythonCmd = &cobra.Command{
	Use:   "reinstall-python",
	Short: "Reinstall the deadsnakes Python version used by Saltbox and related Ansible virtual environment",
	Long:  `Reinstall the deadsnakes Python version used by Saltbox and related Ansible virtual environment`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleReinstallPython()
	},
}

func init() {
	rootCmd.AddCommand(reinstallPythonCmd)
}

func handleReinstallPython() error {
	release, err := detectOSRelease()
	if err != nil {
		return fmt.Errorf("error detecting OS release: %w", err)
	}

	if release == "focal" || release == "jammy" {
		fmt.Println("Removing Python 3.12 packages and recreating Ansible venv.")
		if err := removePython(); err != nil {
			return fmt.Errorf("error removing Python packages: %w", err)
		}

		saltboxUser, err := utils.GetSaltboxUser()
		if err != nil {
			return fmt.Errorf("error getting saltbox user: %w", err)
		}

		if err := venv.ManageAnsibleVenv(true, saltboxUser); err != nil {
			return fmt.Errorf("error managing Ansible venv: %w", err)
		}
	} else {
		fmt.Println("This command is only for Ubuntu 20.04 and 22.04")
	}
	return nil
}

func detectOSRelease() (string, error) {
	cmd := exec.Command("lsb_release", "-cs")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error running lsb_release: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func removePython() error {
	packages := []string{
		"libpython3.12-minimal",
		"python3.12-minimal",
		"libpython3.12",
		"libpython3.12-dev",
		"python3.12",
		"python3.12-dev",
		"python3.12-venv",
		"libpython3.12-stdlib",
	}

	cmd := exec.Command("apt", append([]string{"remove", "-y"}, packages...)...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error removing Python packages: %s, stderr: %s", err, stderr.String())
	}

	return nil
}
