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
		return handleReinstallVenv()
	},
}

func init() {
	rootCmd.AddCommand(reinstallVenvCmd)
}

func handleReinstallVenv() error {
	if err := spinners.RunInfoSpinner("Reinstalling Ansible virtual environment."); err != nil {
		return err
	}

	saltboxUser, err := utils.GetSaltboxUser()
	if err != nil {
		return fmt.Errorf("error getting saltbox user: %w", err)
	}

	if err := venv.ManageAnsibleVenv(true, saltboxUser); err != nil {
		return fmt.Errorf("error managing Ansible venv: %w", err)
	}

	return nil
}
