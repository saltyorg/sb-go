package cmd

import (
	"os"

	"github.com/saltyorg/sb-go/internal/validate2"

	"github.com/spf13/cobra"
)

var config2Cmd = &cobra.Command{
	Use:   "validate-config2",
	Short: "Validate Saltbox configuration files using YAML schemas",
	Long:  `Validate Saltbox configuration files using YAML schemas with custom validators`,
	Run: func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")
		err := validate2.AllSaltboxConfigs(verbose)
		if err != nil {
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(config2Cmd)
	config2Cmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
}