package cmd

import (
	"context"
	"fmt"

	"github.com/saltyorg/sb-go/internal/fact"
	"github.com/saltyorg/sb-go/internal/setup"
	"github.com/saltyorg/sb-go/internal/spinners"
	"github.com/saltyorg/sb-go/internal/venv"

	"github.com/spf13/cobra"
)

// ghaCmd represents the gha command
var ghaCmd = &cobra.Command{
	Use:    "gha",
	Short:  "Install GHA dependencies",
	Long:   `Install GHA dependencies`,
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		runner := spinners.NewRunner(spinners.RunnerOptions{Verbose: true})
		return runner.Run(ctx, spinners.TaskSpec{
			Running: "Installing GHA dependencies",
			Success: "GHA dependencies installed",
		}, func(ctx context.Context, task *spinners.Task) error {

			// Perform initial setup tasks
			if err := setup.InitialSetup(ctx, task, true); err != nil {
				return fmt.Errorf("error during initial setup: %w", err)
			}

			// Configure the locale
			if err := setup.ConfigureLocale(ctx, task); err != nil {
				return fmt.Errorf("error configuring locale: %w", err)
			}

			if err := venv.Reconcile(ctx, task, venv.Options{Verbose: true}); err != nil {
				return fmt.Errorf("error reconciling Python environment: %w", err)
			}

			if err := task.Run(ctx, spinners.TaskSpec{
				Running:      "Checking saltbox.fact",
				Success:      "saltbox.fact is ready",
				Failure:      "saltbox.fact update",
				ChildDisplay: spinners.CollapseChildTasks,
			}, func(ctx context.Context, factTask *spinners.Task) error {
				return fact.DownloadAndInstallSaltboxFact(ctx, factTask, false, true)
			}); err != nil {
				return fmt.Errorf("error downloading and installing saltbox.fact: %w", err)
			}

			if err := setup.CopyDefaultConfigFiles(ctx, task); err != nil {
				return fmt.Errorf("error copying default configuration files: %w", err)
			}

			if err := setup.InitializeGitHooks(ctx, task); err != nil {
				return fmt.Errorf("error initializing Git hooks: %w", err)
			}

			return nil
		})
	},
}

func init() {
	rootCmd.AddCommand(ghaCmd)
}
