package cmd

import (
	"fmt"

	"github.com/saltyorg/sb-go/internal/runtime"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print Saltbox CLI version",
	Long:  `Print Saltbox CLI version`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Saltbox CLI version: %s (commit: %s)\n", runtime.Version, runtime.GitCommit)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
