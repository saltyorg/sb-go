package cmd

import (
	"fmt"

	"github.com/saltyorg/sb-go/runtime"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print Saltbox CLI version",
	Long:  `Print Saltbox CLI version`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Saltbox CLI version: %s (commit: %s)\n", runtime.Version, runtime.GitCommit)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
