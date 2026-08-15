package cmd

import (
	"context"
	"fmt"

	"github.com/saltyorg/sb-go/saltbox"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/spf13/cobra"
)

// reinstallFactsCmd represents the reinstallFacts command
func newReinstallFactsCommand() *cobra.Command {
	var verbose bool
	reinstallFactsCmd := &cobra.Command{
		Use:   "reinstall-facts",
		Short: "Reinstall the Rust saltbox.fact file",
		Long:  `Reinstall the Rust saltbox.fact file`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: verbose})
			return runner.Run(cmd.Context(), terminal.TaskSpec{
				Running: "Reinstalling saltbox.fact",
			}, func(ctx context.Context, task *terminal.Task) error {
				if err := saltbox.DownloadAndInstallSaltboxFact(ctx, task, true, verbose); err != nil {
					return fmt.Errorf("error reinstalling saltbox.fact: %w", err)
				}
				return nil
			})
		},
	}
	reinstallFactsCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	return reinstallFactsCmd
}

func addReinstallFactsCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newReinstallFactsCommand())
}
