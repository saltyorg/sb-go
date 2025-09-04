package cmd

import (
	"os"

	"github.com/saltyorg/sb-go/internal/validate"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "validate-config",
	Short: "Validate Saltbox configuration files",
	Long:  `Validate Saltbox configuration files`,
	Run: func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")
		err := validate.AllSaltboxConfigs(verbose)
		if err != nil {
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
}
