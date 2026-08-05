package cmd

import (
	"github.com/saltyorg/sb-go/internal/errors"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sb",
	Short: "Saltbox CLI",
	Long:  `Saltbox CLI`,
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true, // removes cmd - we use custom completion installation
	},
}

// GetRootCommand returns the root command for use with fang.Execute
func GetRootCommand() *cobra.Command {
	return rootCmd
}

func init() {
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true}) // -h/--help flags are sufficient
}

// handleInterruptError checks if the error is from a user interrupt and triggers shutdown.
// Returns true if it was an interrupt error and shutdown was initiated.
func handleInterruptError(err error) {
	errors.HandleInterruptError(err)
}
