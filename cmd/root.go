package cmd

import (
	"context"

	"github.com/saltyorg/sb-go/ansible"
	"github.com/saltyorg/sb-go/buildinfo"
	gitops "github.com/saltyorg/sb-go/git"
	"github.com/saltyorg/sb-go/signals"

	"github.com/spf13/cobra"
)

// Dependencies contains process-scoped command dependencies.
type Dependencies struct {
	AnsibleExecutor ansible.CommandExecutor
	GitExecutor     gitops.CommandExecutor
	Shutdown        func(int)
	BuildInfo       buildinfo.Info
}

// NewRootCommand builds a fresh command tree for each invocation.
func NewRootCommand(deps Dependencies) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "sb",
		Short: "Saltbox CLI",
		Long:  `Saltbox CLI`,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ctx := ansible.WithExecutor(cmd.Context(), deps.AnsibleExecutor)
			ctx = gitops.WithExecutor(ctx, deps.GitExecutor)
			ctx = signals.WithShutdown(ctx, deps.Shutdown)
			cmd.SetContext(ctx)
		},
	}
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})

	addAnnouncementsCommand(rootCmd)
	addBenchCommand(rootCmd)
	addBranchCommand(rootCmd)
	addBranchSandboxCommand(rootCmd)
	addCompletionCommand(rootCmd)
	addConfigCommand(rootCmd)
	addDiagCommand(rootCmd)
	addDockerCommand(rootCmd)
	addEditCommand(rootCmd)
	addFactCommand(rootCmd)
	addGHACommand(rootCmd)
	addInstallCommand(rootCmd)
	addListCommand(rootCmd)
	addLogsCommand(rootCmd)
	addMOTDCommand(rootCmd)
	addReinstallFactsCommand(rootCmd)
	addReinstallPythonCommand(rootCmd)
	addReinstallVenvCommand(rootCmd)
	addRestoreCommand(rootCmd)
	addSelfUpdateCommand(rootCmd, deps.BuildInfo)
	addSetupCommand(rootCmd)
	addUpdateCommand(rootCmd)
	addUpdateVenvCommand(rootCmd)
	addVersionCommand(rootCmd, deps.BuildInfo)

	return rootCmd
}

// handleInterruptError requests shutdown when err represents a user interrupt.
func handleInterruptError(ctx context.Context, err error) {
	signals.HandleInterruptError(ctx, err)
}
