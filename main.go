package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/saltyorg/sb-go/cmd"
	"github.com/saltyorg/sb-go/internal/signals"
	"github.com/saltyorg/sb-go/internal/ubuntu"
	"github.com/saltyorg/sb-go/internal/utils"

	"github.com/charmbracelet/fang"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// customErrorHandler handles error formatting with proper line break support.
// Unlike the default handler, this respects \n characters in error messages
// and renders each line separately for better readability.
func customErrorHandler(w io.Writer, styles fang.Styles, err error) {
	// Print error header (already styled by Fang)
	fmt.Fprintf(w, "%s\n", styles.ErrorHeader.String())

	// Split error message by line breaks
	errorText := err.Error()
	lines := strings.SplitSeq(errorText, "\n")

	// Print each line of the error message with proper styling
	// ErrorText style includes margin, width constraints, and transforms
	// We unset both width and transform to prevent wrapping and preserve custom formatting
	for line := range lines {
		if line != "" {
			// All lines rendered without transform to preserve custom formatting
			// This allows error messages with embedded lipgloss styles and ANSI codes
			// to display correctly without Fang's titleFirstWord transform interfering
			lineStyle := styles.ErrorText.UnsetTransform().UnsetWidth()
			fmt.Fprintf(w, "%s\n", lineStyle.Render(line))
		} else {
			fmt.Fprintf(w, "\n") // Preserve blank lines
		}
	}

	// Add trailing newline if error doesn't end with one
	if !strings.HasSuffix(errorText, "\n") {
		fmt.Fprintf(w, "\n")
	}
}

func main() {
	if os.Geteuid() != 0 {
		// Relaunch as root with sudo
		exitCode, err := utils.RelaunchAsRoot()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error relaunching as root: %v\n", err)
		}
		// Exit with the same code as the sudo subprocess
		os.Exit(exitCode)
	}

	supportedVersions := []string{"20.04", "22.04", "24.04"}

	if err := ubuntu.CheckSupport(supportedVersions); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Force truecolor for consistent styling across all commands (process-local only)
	// Can be overridden by setting SB_COLOR_PROFILE environment variable
	// Valid values: truecolor, ansi256, ansi, ascii
	if colorProfile := os.Getenv("SB_COLOR_PROFILE"); colorProfile != "" {
		switch colorProfile {
		case "truecolor":
			_ = os.Setenv("COLORTERM", "truecolor")
			lipgloss.SetColorProfile(termenv.TrueColor)
		case "ansi256":
			_ = os.Setenv("COLORTERM", "256color")
			lipgloss.SetColorProfile(termenv.ANSI256)
		case "ansi":
			lipgloss.SetColorProfile(termenv.ANSI)
		case "ascii":
			lipgloss.SetColorProfile(termenv.Ascii)
		default:
			// Invalid value, use default (truecolor)
			_ = os.Setenv("COLORTERM", "truecolor")
			lipgloss.SetColorProfile(termenv.TrueColor)
		}
	} else {
		// Default to truecolor if not set
		_ = os.Setenv("COLORTERM", "truecolor")
		lipgloss.SetColorProfile(termenv.TrueColor)
	}

	// Initialize global signal manager and get context for the application
	sigManager := signals.GetGlobalManager()
	ctx := sigManager.Context()

	// Execute commands with fang for enhanced CLI UX
	// Fang provides styled help, formatted errors, and improved presentation
	if err := fang.Execute(ctx, cmd.GetRootCommand(),
		fang.WithErrorHandler(customErrorHandler),
	); err != nil {
		os.Exit(1)
	}

	// Exit with appropriate code if shutdown was triggered
	if sigManager.IsShutdown() {
		os.Exit(sigManager.ExitCode())
	}

	// Exit successfully if we got here
	os.Exit(0)
}
