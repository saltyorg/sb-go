package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/saltyorg/sb-go/host"
	"github.com/saltyorg/sb-go/layout"
	"github.com/saltyorg/sb-go/python"
	"github.com/saltyorg/sb-go/saltbox"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/spf13/cobra"
)

// ghaCmd represents the gha command
func newGHACommand() *cobra.Command {
	return &cobra.Command{
		Use:    "gha",
		Short:  "Install GHA dependencies",
		Long:   `Install GHA dependencies`,
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: true})
			return runner.Run(ctx, terminal.TaskSpec{
				Running: "Installing GHA dependencies",
				Success: "GHA dependencies installed",
			}, func(ctx context.Context, task *terminal.Task) error {

				// Perform initial setup tasks
				if err := saltbox.InitialSetup(ctx, task, true); err != nil {
					return fmt.Errorf("error during initial setup: %w", err)
				}

				// Configure the locale
				if err := saltbox.ConfigureLocale(ctx, task); err != nil {
					return fmt.Errorf("error configuring locale: %w", err)
				}

				if err := python.Reconcile(ctx, task, python.Options{Verbose: true}); err != nil {
					return fmt.Errorf("error reconciling Python environment: %w", err)
				}

				if err := task.Run(ctx, terminal.TaskSpec{
					Running: "Checking saltbox.fact",
					Success: "saltbox.fact is ready",
					Failure: "saltbox.fact update",
				}, func(ctx context.Context, factTask *terminal.Task) error {
					return saltbox.DownloadAndInstallSaltboxFact(ctx, factTask, false, true)
				}); err != nil {
					return fmt.Errorf("error downloading and installing saltbox.fact: %w", err)
				}

				if err := saltbox.CopyDefaultConfigFiles(ctx, task); err != nil {
					return fmt.Errorf("error copying default configuration files: %w", err)
				}
				configOwner := strings.TrimSpace(os.Getenv("SUDO_USER"))
				if err := host.EnsureForExistingUser(configOwner,
					layout.SaltboxAccountsConfigPath,
					layout.SaltboxAdvancedSettingsConfigPath,
					layout.SaltboxBackupConfigPath,
					layout.SaltboxHetznerVLANConfigPath,
					layout.SaltboxProvidersConfigPath,
					layout.SaltboxSettingsConfigPath,
				); err != nil {
					return fmt.Errorf("error assigning GHA configuration ownership: %w", err)
				}

				if err := saltbox.InitializeGitHooks(ctx, task); err != nil {
					return fmt.Errorf("error initializing Git hooks: %w", err)
				}

				return nil
			})
		},
	}
}

func addGHACommand(rootCmd *cobra.Command) {
	ghaCmd := newGHACommand()
	rootCmd.AddCommand(ghaCmd)
}
