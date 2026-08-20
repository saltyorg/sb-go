package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/saltyorg/sb-go/buildinfo"

	"github.com/spf13/cobra"
)

type versionOutput struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	UVVersion string `json:"uv_version"`
}

func newVersionCommand(info buildinfo.Info) *cobra.Command {
	var outputJSON bool
	command := &cobra.Command{
		Use:   "version",
		Short: "Print Saltbox CLI version",
		Long:  `Print Saltbox CLI version`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if outputJSON {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(versionOutput{
					Version:   info.Version,
					GitCommit: info.GitCommit,
					UVVersion: info.UVVersion,
				}); err != nil {
					return fmt.Errorf("write version JSON: %w", err)
				}
				return nil
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Saltbox CLI version: %s (commit: %s)\n", info.Version, info.GitCommit); err != nil {
				return fmt.Errorf("write version: %w", err)
			}
			return nil
		},
	}
	command.Flags().BoolVar(&outputJSON, "json", false, "Print machine-readable version information")
	return command
}

func addVersionCommand(rootCmd *cobra.Command, info buildinfo.Info) {
	versionCmd := newVersionCommand(info)
	rootCmd.AddCommand(versionCmd)
}
