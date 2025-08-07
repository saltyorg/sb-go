package cmd

import (
	"github.com/saltyorg/sb-go/validate"
	"github.com/spf13/cobra"
	"os"
)

var (
	verbose bool
)

var configCmd = &cobra.Command{
	Use:   "validate-config",
	Short: "Validate Saltbox configuration files",
	Long:  `Validate Saltbox configuration files`,
	Run: func(cmd *cobra.Command, args []string) {
		err := validate.AllSaltboxConfigs(verbose)
		if err != nil {
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}
