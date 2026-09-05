package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/saltyorg/sb-go/facts"
	"github.com/saltyorg/sb-go/factui"
	"github.com/saltyorg/sb-go/layout"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newFactCommand() *cobra.Command {
	return newFactEditorCommand(facts.OpenSession, factui.Run)
}

// Keep the persistence and runner boundaries injectable while always checking
// the actual Cobra streams for terminal descriptors before opening a session.
func newFactEditorCommand(
	open func(string) (*facts.Session, error),
	run func(context.Context, factui.Session, io.Reader, io.Writer) error,
) *cobra.Command {
	return &cobra.Command{
		Use:   "fact",
		Short: "Manage Saltbox configuration facts",
		Long: "Interactively browse and edit existing Saltbox configuration facts.\n" +
			"Changes are staged for review before applying. Requires an interactive terminal.\n" +
			"Mouse: click to select, use the wheel to navigate, and right-click for actions. Hold Shift while dragging for native terminal text selection.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			input, output := cmd.InOrStdin(), cmd.OutOrStdout()
			if !factTerminal(input) || !factTerminal(output) {
				return fmt.Errorf("sb fact requires an interactive terminal for input and output")
			}
			session, err := open(layout.Current().SaltboxFactsPath)
			if err != nil {
				return fmt.Errorf("open fact editor: %w", err)
			}
			return run(cmd.Context(), session, input, output)
		},
	}
}

func factTerminal(stream any) bool {
	file, ok := stream.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(file.Fd()))
}

func addFactCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newFactCommand())
}
