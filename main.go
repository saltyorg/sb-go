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
		if err := utils.RelaunchAsRoot(); err != nil {
			//fmt.Println("Error relaunching:", err)
			os.Exit(1)
		}
		os.Exit(0)
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
	cmd.ExecuteContext(ctx)

	// Exit with appropriate code if shutdown was triggered
	if sigManager.IsShutdown() {
		os.Exit(sigManager.ExitCode())
	}
}
