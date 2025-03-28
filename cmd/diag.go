package cmd

import (
	"fmt"
	"github.com/saltyorg/sb-go/ansible"
	"github.com/saltyorg/sb-go/constants"

	"github.com/spf13/cobra"
)

// diagCmd represents the diag command
var diagCmd = &cobra.Command{
	Use:   "diag",
	Short: "Runs Saltbox diagnostics role",
	Long:  `Runs Saltbox diagnostics role`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleDiag()
	},
}

func init() {
	rootCmd.AddCommand(diagCmd)
}

func handleDiag() error {
	tags := []string{"--tags", "diag"}
	err := ansible.RunAnsiblePlaybook(
		constants.SaltboxRepoPath,
		constants.SaltboxPlaybookPath(),
		constants.AnsiblePlaybookBinaryPath,
		tags,
		false,
	)
	if err != nil {
		return fmt.Errorf("error running diagnostic role: %w", err)
	}
	return nil
}
