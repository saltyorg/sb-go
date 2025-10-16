package cmd

import (
	"fmt"

	"github.com/saltyorg/sb-go/internal/fact"
	"github.com/saltyorg/sb-go/internal/spinners"

	"github.com/spf13/cobra"
)

// reinstallFactsCmd represents the reinstallFacts command
var reinstallFactsCmd = &cobra.Command{
	Use:   "reinstall-facts",
	Short: "Reinstall the Rust saltbox.fact file",
	Long:  `Reinstall the Rust saltbox.fact file`,
	RunE: func(cmd *cobra.Command, args []string) error {
		verbose, _ := cmd.Flags().GetBool("verbose")

		// Set verbose mode for spinners
		spinners.SetVerboseMode(verbose)

		if err := fact.DownloadAndInstallSaltboxFact(true, verbose); err != nil {
			return fmt.Errorf("error reinstalling saltbox.fact: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(reinstallFactsCmd)
	reinstallFactsCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
}
