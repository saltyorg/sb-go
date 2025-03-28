package cmd

import (
	"fmt"
	saltboxfact "github.com/saltyorg/sb-go/fact"
	"os"

	"github.com/spf13/cobra"
)

// reinstallFactsCmd represents the reinstallFacts command
var reinstallFactsCmd = &cobra.Command{
	Use:   "reinstall-facts",
	Short: "Reinstall the Rust saltbox.fact file",
	Long:  `Reinstall the Rust saltbox.fact file`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := saltboxfact.DownloadAndInstallSaltboxFact(true); err != nil {
			fmt.Println("Error reinstalling saltbox.fact:", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(reinstallFactsCmd)
}
