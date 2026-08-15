package cmd

import (
	"fmt"

	"github.com/saltyorg/sb-go/ansible"
	"github.com/saltyorg/sb-go/layout"

	"github.com/spf13/cobra"
)

// diagCmd represents the diag command
func newDiagCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "diag",
		Short: "Runs Saltbox diagnostics role",
		Long:  `Runs Saltbox diagnostics role`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleDiag(cmd)
		},
	}
}

func addDiagCommand(rootCmd *cobra.Command) {
	diagCmd := newDiagCommand()
	rootCmd.AddCommand(diagCmd)
}

func handleDiag(cmd *cobra.Command) error {
	ctx := cmd.Context()
	tags := []string{"--tags", "diag"}
	err := ansible.RunAnsiblePlaybook(
		ctx,
		layout.SaltboxRepoPath,
		layout.SaltboxPlaybookPath(),
		layout.AnsiblePlaybookBinaryPath,
		tags,
		true,
	)
	if err != nil {
		return fmt.Errorf("error running diagnostic role: %w", err)
	}
	return nil
}
