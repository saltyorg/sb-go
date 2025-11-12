package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/saltyorg/sb-go/cmd"
	"github.com/saltyorg/sb-go/internal/signals"
	"github.com/saltyorg/sb-go/internal/ubuntu"
	"github.com/saltyorg/sb-go/internal/utils"
)

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
			os.Setenv("COLORTERM", "truecolor")
			lipgloss.SetColorProfile(termenv.TrueColor)
		case "ansi256":
			os.Setenv("COLORTERM", "256color")
			lipgloss.SetColorProfile(termenv.ANSI256)
		case "ansi":
			lipgloss.SetColorProfile(termenv.ANSI)
		case "ascii":
			lipgloss.SetColorProfile(termenv.Ascii)
		default:
			// Invalid value, use default (truecolor)
			os.Setenv("COLORTERM", "truecolor")
			lipgloss.SetColorProfile(termenv.TrueColor)
		}
	} else {
		// Default to truecolor if not set
		os.Setenv("COLORTERM", "truecolor")
		lipgloss.SetColorProfile(termenv.TrueColor)
	}

	// Initialize global signal manager and get context for the application
	sigManager := signals.GetGlobalManager()
	ctx := sigManager.Context()

	// Execute commands with fang for enhanced CLI UX
	// Fang provides styled help, formatted errors, and improved presentation
	if err := fang.Execute(ctx, cmd.GetRootCommand()); err != nil {
		os.Exit(1)
	}

	// Exit with appropriate code if shutdown was triggered
	if sigManager.IsShutdown() {
		os.Exit(sigManager.ExitCode())
	}

	// Exit successfully if we got here
	os.Exit(0)
}
