package cmd

import (
	"fmt"

	"github.com/saltyorg/sb-go/buildinfo"

	"github.com/spf13/cobra"
)

func newVersionCommand(info buildinfo.Info) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print Saltbox CLI version",
		Long:  `Print Saltbox CLI version`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "Saltbox CLI version: %s (commit: %s)\n", info.Version, info.GitCommit)
			return nil
		},
	}
}

func addVersionCommand(rootCmd *cobra.Command, info buildinfo.Info) {
	versionCmd := newVersionCommand(info)
	rootCmd.AddCommand(versionCmd)
}
