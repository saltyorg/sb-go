package cmd

import (
	"github.com/saltyorg/sb-go/internal/validate"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "validate-config",
	Short: "Validate Saltbox configuration files",
	Long:  `Validate Saltbox configuration files`,
	RunE: func(cmd *cobra.Command, args []string) error {
		verbose, _ := cmd.Flags().GetBool("verbose")
		if err := validate.AllSaltboxConfigs(verbose); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
}
