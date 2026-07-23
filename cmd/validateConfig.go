package cmd

import (
	"context"

	"github.com/saltyorg/sb-go/internal/spinners"
	"github.com/saltyorg/sb-go/internal/validate"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "validate-config",
	Short: "Validate Saltbox configuration files",
	Long:  `Validate Saltbox configuration files`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		verbose, _ := cmd.Flags().GetBool("verbose")
		runner := spinners.NewRunner(spinners.RunnerOptions{Verbose: verbose})
		return runner.Run(cmd.Context(), spinners.TaskSpec{
			Running:      "Validating Saltbox configuration",
			Success:      "Saltbox configuration validated",
			ChildDisplay: spinners.RetainChildTasks,
		}, func(ctx context.Context, task *spinners.Task) error {
			return validate.AllSaltboxConfigs(ctx, task, verbose)
		})
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
}
