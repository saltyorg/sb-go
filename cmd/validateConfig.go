package cmd

import (
	"context"

	"github.com/saltyorg/sb-go/saltbox"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	var verbose bool
	configCmd := &cobra.Command{
		Use:   "validate-config",
		Short: "Validate Saltbox configuration files",
		Long:  `Validate Saltbox configuration files`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: verbose})
			return runner.Run(cmd.Context(), terminal.TaskSpec{
				Running:      "Validating Saltbox configuration",
				Success:      "Saltbox configuration validated",
				ChildDisplay: terminal.RetainChildTasks,
			}, func(ctx context.Context, task *terminal.Task) error {
				return saltbox.AllSaltboxConfigs(ctx, task, verbose)
			})
		},
	}
	configCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	return configCmd
}

func addConfigCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newConfigCommand())
}
