package cmd

import (
	"context"
	"fmt"

	"github.com/saltyorg/sb-go/internal/fact"
	"github.com/saltyorg/sb-go/internal/spinners"

	"github.com/spf13/cobra"
)

// reinstallFactsCmd represents the reinstallFacts command
var reinstallFactsCmd = &cobra.Command{
	Use:   "reinstall-facts",
	Short: "Reinstall the Rust saltbox.fact file",
	Long:  `Reinstall the Rust saltbox.fact file`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		verbose, _ := cmd.Flags().GetBool("verbose")

		runner := spinners.NewRunner(spinners.RunnerOptions{Verbose: verbose})
		return runner.Run(cmd.Context(), spinners.TaskSpec{
			Running: "Reinstalling saltbox.fact",
		}, func(ctx context.Context, task *spinners.Task) error {
			if err := fact.DownloadAndInstallSaltboxFact(ctx, task, true, verbose); err != nil {
				return fmt.Errorf("error reinstalling saltbox.fact: %w", err)
			}
			return nil
		})
	},
}

func init() {
	rootCmd.AddCommand(reinstallFactsCmd)
	reinstallFactsCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
}
