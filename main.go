package main

import (
	"context"
	"fmt"
	"os"

	"github.com/saltyorg/sb-go/cmd"
	"github.com/saltyorg/sb-go/internal/signals"
	"github.com/saltyorg/sb-go/internal/ubuntu"
	"github.com/saltyorg/sb-go/internal/utils"
)

func main() {
	if os.Geteuid() != 0 {
		// Relaunch as root - use context.Background() to allow unlimited time
		// The sudo subprocess may take a long time for operations like package installation
		exitCode, err := utils.RelaunchAsRoot(context.Background())
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
	if err := cmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Exit with appropriate code if shutdown was triggered
	if sigManager.IsShutdown() {
		os.Exit(sigManager.ExitCode())
	}

	// Exit successfully if we got here
	os.Exit(0)
}
