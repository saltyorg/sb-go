package cmd

import (
	"fmt"
	"os"

	"github.com/saltyorg/sb-go/internal/fact"
	"github.com/saltyorg/sb-go/internal/spinners"

	"github.com/spf13/cobra"
)

// reinstallFactsCmd represents the reinstallFacts command
var reinstallFactsCmd = &cobra.Command{
	Use:   "reinstall-facts",
	Short: "Reinstall the Rust saltbox.fact file",
	Long:  `Reinstall the Rust saltbox.fact file`,
	Run: func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")

		// Set verbose mode for spinners
		spinners.SetVerboseMode(verbose)

		if err := fact.DownloadAndInstallSaltboxFact(true); err != nil {
			fmt.Println("Error reinstalling saltbox.fact:", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(reinstallFactsCmd)
	reinstallFactsCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
}
