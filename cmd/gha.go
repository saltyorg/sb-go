package cmd

import (
	"fmt"
	"github.com/saltyorg/sb-go/fact"
	"github.com/saltyorg/sb-go/setup"
	"github.com/spf13/cobra"
	"os"
)

// ghaCmd represents the gha command
var ghaCmd = &cobra.Command{
	Use:    "gha",
	Short:  "Install GHA dependencies",
	Long:   `Install GHA dependencies`,
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		// Perform initial setup tasks (moved to setup package)
		setup.InitialSetup(verbose)

		// Configure the locale (moved to setup package)
		setup.ConfigureLocale(verbose)

		// Setup Python venv (moved to setup package)
		setup.PythonVenv(verbose)

		if err := fact.DownloadAndInstallSaltboxFact(false, verbose); err != nil {
			fmt.Printf("Error downloading and installing saltbox.fact: %v\n", err)
			os.Exit(1)
		}
		if err := setup.CopyDefaultConfigFiles(verbose); err != nil {
			fmt.Printf("Error copying default configuration files: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(ghaCmd)
	// Add the -v flag as a persistent flag to the config command.
	ghaCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}
