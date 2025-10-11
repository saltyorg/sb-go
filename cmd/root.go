package cmd

import (
	"context"
	"os"

	"github.com/saltyorg/sb-go/internal/errors"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sb",
	Short: "Saltbox CLI",
	Long:  `Saltbox CLI`,
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true, // hides cmd
		// DisableDefaultCmd: true, // removes cmd
	},
}

// ExecuteContext adds all child commands to the root command and sets flags appropriately.
// It accepts a context that will be available to all commands via cmd.Context() for cancellation and timeouts.
// This is called by main.main() and only needs to happen once to the rootCmd.
func ExecuteContext(ctx context.Context) {
	err := rootCmd.ExecuteContext(ctx)
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.github.com/saltyorg/sb-go.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// handleInterruptError checks if the error is from a user interrupt and triggers shutdown.
// Returns true if it was an interrupt error and shutdown was initiated.
func handleInterruptError(err error) {
	errors.HandleInterruptError(err)
}
