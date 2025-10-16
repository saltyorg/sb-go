package cmd

import (
	"fmt"

	"github.com/saltyorg/sb-go/internal/fact"
	"github.com/saltyorg/sb-go/internal/setup"
	"github.com/saltyorg/sb-go/internal/spinners"

	"github.com/spf13/cobra"
)

// ghaCmd represents the gha command
var ghaCmd = &cobra.Command{
	Use:    "gha",
	Short:  "Install GHA dependencies",
	Long:   `Install GHA dependencies`,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		// Set verbose mode for spinners
		spinners.SetVerboseMode(true)

		// Perform initial setup tasks
		fmt.Println("Starting initial setup...")
		if err := setup.InitialSetup(ctx, true); err != nil {
			return fmt.Errorf("error during initial setup: %w", err)
		}
		fmt.Println("Initial setup completed successfully")

		// Configure the locale
		fmt.Println("Starting locale configuration...")
		if err := setup.ConfigureLocale(ctx); err != nil {
			return fmt.Errorf("error configuring locale: %w", err)
		}
		fmt.Println("Locale configuration completed successfully")

		// Setup Python venv
		fmt.Println("Starting Python venv setup...")
		if err := setup.PythonVenv(ctx, true); err != nil {
			return fmt.Errorf("error setting up Python venv: %w", err)
		}
		fmt.Println("Python venv setup completed successfully")

		// Install pip3 Dependencies
		fmt.Println("Starting pip dependencies installation...")
		if err := setup.InstallPipDependencies(ctx, true); err != nil {
			return fmt.Errorf("error installing pip dependencies: %w", err)
		}
		fmt.Println("Pip dependencies installation completed successfully")

		fmt.Println("Starting saltbox.fact download and installation...")
		if err := fact.DownloadAndInstallSaltboxFact(false, true); err != nil {
			return fmt.Errorf("error downloading and installing saltbox.fact: %w", err)
		}
		fmt.Println("Saltbox.fact download and installation completed successfully")

		fmt.Println("Starting default config files copy...")
		if err := setup.CopyDefaultConfigFiles(ctx); err != nil {
			return fmt.Errorf("error copying default configuration files: %w", err)
		}
		fmt.Println("Default config files copy completed successfully")

		fmt.Println("GHA setup completed successfully!")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(ghaCmd)
}
