package cmd

import (
	"fmt"

	"github.com/saltyorg/sb-go/spinners"
	"github.com/saltyorg/sb-go/utils"
	"github.com/saltyorg/sb-go/venv"
	"github.com/spf13/cobra"
)

// reinstallVenvCmd represents the reinstall-venv command
var reinstallVenvCmd = &cobra.Command{
	Use:   "reinstall-venv",
	Short: "Reinstall the Ansible virtual environment",
	Long:  `Reinstall the Ansible virtual environment`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleReinstallVenv(verbose)
	},
}

func init() {
	rootCmd.AddCommand(reinstallVenvCmd)
	// Add the -v flag as a persistent flag to the config command.
	reinstallVenvCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}

// handleReinstallVenv handles the reinstallation of the Ansible virtual environment.
func handleReinstallVenv(verbose bool) error {
	// Set verbose mode for spinners
	spinners.SetVerboseMode(verbose)

	// Display initial message
	if err := spinners.RunInfoSpinner("Reinstalling Ansible virtual environment"); err != nil {
		return err
	}

	// Get Saltbox user
	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	// Manage Ansible venv with force recreate flag set to true
	// This function already has internal spinners
	if err := venv.ManageAnsibleVenv(true, saltboxUser, verbose); err != nil {
		return fmt.Errorf("error managing Ansible venv: %w", err)
	}

	// Success message
	return spinners.RunInfoSpinner("Ansible Virtual Environment reinstalled successfully")
}
