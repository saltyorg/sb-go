package main

import (
	"fmt"
	"os"

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

	// Initialize global signal manager and get context for the application
	sigManager := signals.GetGlobalManager()
	ctx := sigManager.Context()

	// Execute commands with context
	// Cobra will handle printing errors and exiting with code 1
	if err := cmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}

	// Exit with appropriate code if shutdown was triggered
	if sigManager.IsShutdown() {
		os.Exit(sigManager.ExitCode())
	}

	// Exit successfully if we got here
	os.Exit(0)
}
