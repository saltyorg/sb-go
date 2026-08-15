package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/saltyorg/sb-go/buildinfo"
	"github.com/saltyorg/sb-go/layout"
	"github.com/saltyorg/sb-go/selfupdate"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/spf13/cobra"
)

func newSelfUpdateCommand(info buildinfo.Info) *cobra.Command {
	var verbose, autoAccept, forceUpdate bool
	selfUpdateCmd := &cobra.Command{
		Use:    "self-update",
		Hidden: true,
		Short:  "Update Saltbox CLI",
		Long:   `Update Saltbox CLI`,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			runner := terminal.NewRunner(terminal.RunnerOptions{
				Verbose: verbose,
				Output:  cmd.ErrOrStderr(),
			})
			if info.DisableSelfUpdate && !forceUpdate {
				if verbose {
					runner.Info("Debug: Self-update is disabled in this build")
					runner.Info("Debug: Use --force-update to override this restriction")
				} else {
					runner.Warning("Self-update is disabled in this build")
					runner.Info("Use --force-update to override this restriction")
				}
				return nil
			}
			confirm := func(prompt string) (bool, error) {
				return promptForConfirmation(cmd.InOrStdin(), cmd.OutOrStdout(), prompt)
			}
			_, err := runSelfUpdate(cmd.Context(), runner, info, autoAccept, verbose, "", forceUpdate, confirm)
			return err
		},
	}
	selfUpdateCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose debug output")
	selfUpdateCmd.Flags().BoolVarP(&autoAccept, "yes", "y", false, "Automatically accept update without confirmation")
	if info.DisableSelfUpdate {
		selfUpdateCmd.Flags().BoolVar(&forceUpdate, "force-update", false, "Force update even when self-update is disabled")
	}
	return selfUpdateCmd
}

func addSelfUpdateCommand(rootCmd *cobra.Command, info buildinfo.Info) {
	rootCmd.AddCommand(newSelfUpdateCommand(info))
}

func promptForConfirmation(input io.Reader, output io.Writer, prompt string) (bool, error) {
	if _, err := fmt.Fprintf(output, "%s [y/n]: ", prompt); err != nil {
		return false, fmt.Errorf("write confirmation prompt: %w", err)
	}
	response, err := bufio.NewReader(input).ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("read confirmation input: %w", err)
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes", nil
}

func runSelfUpdate(
	ctx context.Context,
	runner *terminal.Runner,
	info buildinfo.Info,
	autoUpdate bool,
	verbose bool,
	optionalMessage string,
	force bool,
	confirm func(string) (bool, error),
) (bool, error) {
	source, err := selfupdate.NewSource(layout.SVMVersionProxyURL, verbose, runner)
	if err != nil {
		return false, fmt.Errorf("create release source: %w", err)
	}
	return selfupdate.Run(ctx, source, runner, selfupdate.Options{
		BuildInfo:       info,
		AutoAccept:      autoUpdate,
		OptionalMessage: optionalMessage,
		Force:           force,
		Confirm:         confirm,
	})
}

func doSelfUpdate(ctx context.Context, runner *terminal.Runner, info buildinfo.Info, autoUpdate bool, verbose bool, optionalMessage string, force bool) (bool, error) {
	confirm := func(prompt string) (bool, error) {
		return promptForConfirmation(os.Stdin, os.Stdout, prompt)
	}
	return runSelfUpdate(ctx, runner, info, autoUpdate, verbose, optionalMessage, force, confirm)
}
