package cmd

import (
	"fmt"
	"os"

	saltboxfact "github.com/saltyorg/sb-go/fact"
	"github.com/saltyorg/sb-go/spinners"
	"github.com/spf13/cobra"
)

// reinstallFactsCmd represents the reinstallFacts command
var reinstallFactsCmd = &cobra.Command{
	Use:   "reinstall-facts",
	Short: "Reinstall the Rust saltbox.fact file",
	Long:  `Reinstall the Rust saltbox.fact file`,
	Run: func(cmd *cobra.Command, args []string) {
		// Set verbose mode for spinners
		spinners.SetVerboseMode(verbose)

		if err := saltboxfact.DownloadAndInstallSaltboxFact(true); err != nil {
			fmt.Println("Error reinstalling saltbox.fact:", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(reinstallFactsCmd)
	// Add the -v flag as a persistent flag to the config command.
	reinstallFactsCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}
