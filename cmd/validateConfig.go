package cmd

import (
	"github.com/saltyorg/sb-go/validate"
	"github.com/spf13/cobra"
)

var (
	verbose bool
)

var configCmd = &cobra.Command{
	Use:   "validate-config",
	Short: "Validate Saltbox configuration files",
	Long:  `Validate Saltbox configuration files`,
	Run: func(cmd *cobra.Command, args []string) {
		validate.ValidateAllConfigs(verbose)
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	// Add the -v flag as a persistent flag to the config command.
	configCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}
