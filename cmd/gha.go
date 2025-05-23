package cmd

import (
	"fmt"
	"os"

	"github.com/saltyorg/sb-go/fact"
	"github.com/saltyorg/sb-go/setup"
	"github.com/saltyorg/sb-go/spinners"
	"github.com/spf13/cobra"
)

// ghaCmd represents the gha command
var ghaCmd = &cobra.Command{
	Use:    "gha",
	Short:  "Install GHA dependencies",
	Long:   `Install GHA dependencies`,
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		// Set verbose mode for spinners
		spinners.SetVerboseMode(true)

		// Perform initial setup tasks (moved to setup package)
		setup.InitialSetup(true)

		// Configure the locale (moved to setup package)
		setup.ConfigureLocale()

		// Setup Python venv (moved to setup package)
		setup.PythonVenv(true)

		if err := fact.DownloadAndInstallSaltboxFact(false); err != nil {
			fmt.Printf("Error downloading and installing saltbox.fact: %v\n", err)
			os.Exit(1)
		}
		if err := setup.CopyDefaultConfigFiles(); err != nil {
			fmt.Printf("Error copying default configuration files: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(ghaCmd)
}
