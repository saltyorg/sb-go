package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/saltyorg/sb-go/internal/runtime"
	"github.com/saltyorg/sb-go/internal/spinners"

	"github.com/Masterminds/semver/v3"
	"github.com/creativeprojects/go-selfupdate"
	"github.com/spf13/cobra"
)

// Debug flag to enable verbose output
var debug bool

// Auto-accept flag to skip confirmation
var autoAccept bool

// Force update flag to bypass DisableSelfUpdate build flag
var forceUpdate bool

// selfUpdateCmd represents the selfUpdate command
var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Update Saltbox CLI",
	Long:  `Update Saltbox CLI`,
	Run: func(cmd *cobra.Command, args []string) {
		// Check if self-update is disabled at build time (unless the force flag is used)
		if runtime.DisableSelfUpdate == "true" && !forceUpdate {
			_ = spinners.RunWarningSpinner("Self-update is disabled in this build")
			if runtime.DisableSelfUpdate == "true" {
				_ = spinners.RunInfoSpinner("Use --force-update to override this restriction")
			}
			return
		}
		doSelfUpdate(autoAccept, debug, "", forceUpdate)
	},
}

func init() {
	rootCmd.AddCommand(selfUpdateCmd)
	selfUpdateCmd.Flags().BoolVarP(&debug, "verbose", "v", false, "Enable verbose debug output")
	selfUpdateCmd.Flags().BoolVarP(&autoAccept, "yes", "y", false, "Automatically accept update without confirmation")

	// Only add a force-update flag if self-update is disabled at build time
	if runtime.DisableSelfUpdate == "true" {
		selfUpdateCmd.Flags().BoolVar(&forceUpdate, "force-update", false, "Force update even when self-update is disabled")
	}
}

// promptForConfirmation asks the user for confirmation (y/n)
func promptForConfirmation(prompt string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [y/n]: ", prompt)

	response, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading input:", err)
		return false
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

func doSelfUpdate(autoUpdate bool, verbose bool, optionalMessage string, force bool) {
	// Check if self-update is disabled at build time (unless force is true)
	if runtime.DisableSelfUpdate == "true" && !force {
		if verbose {
			fmt.Println("Debug: Self-update is disabled (build flag)")
		} else {
			_ = spinners.RunWarningSpinner("Self-update is disabled in this build")
		}
		return
	}

	// Log if force update is being used
	if force && runtime.DisableSelfUpdate == "true" {
		if verbose {
			fmt.Println("Debug: Force update flag is active, bypassing DisableSelfUpdate build flag")
		} else {
			_ = spinners.RunInfoSpinner("Forcing self-update despite build configuration")
		}
	}

	if verbose {
		fmt.Println("Debug: Starting self-update process")
		fmt.Printf("Debug: Current version: %s\n", runtime.Version)
		fmt.Printf("Debug: Current git commit: %s\n", runtime.GitCommit)
		fmt.Printf("Debug: Looking for updates in repository: saltyorg/sb-go\n")
		fmt.Printf("Debug: Auto-update mode: %t\n", autoUpdate)
		//selfupdate.EnableLog()
	}

	v := semver.MustParse(runtime.Version)

	if verbose {
		fmt.Printf("Debug: Parsed semver version: %s\n", v.String())
		fmt.Println("Debug: Checking for latest release from GitHub")
	}

	// First, check if an update is available without applying it
	updater, err := selfupdate.NewUpdater(selfupdate.Config{})
	if err != nil {
		if verbose {
			fmt.Printf("Debug: Error creating updater: %v\n", err)
		}
		fmt.Println("Error creating updater:", err)
		os.Exit(1)
		return
	}

	latest, found, err := updater.DetectLatest(context.Background(), selfupdate.ParseSlug("saltyorg/sb-go"))
	if err != nil {
		if verbose {
			fmt.Printf("Debug: Error checking for updates: %v\n", err)
		}
		fmt.Println("Error checking for updates:", err)
		os.Exit(1)
		return
	}

	if !found || latest.Version() == v.String() {
		if verbose {
			fmt.Println("Debug: No update available - current version is the latest")
		}
		_ = spinners.RunInfoSpinner(fmt.Sprintf("Current binary is the latest version: %s", runtime.Version))
		return
	}

	// An update is available
	_ = spinners.RunInfoSpinner(fmt.Sprintf("New sb CLI version available: %s (current: %s)", latest.Version(), v))

	// If autoUpdate is false, ask for confirmation
	if !autoUpdate {
		if !promptForConfirmation("Do you want to update") {
			_ = spinners.RunWarningSpinner("Update of sb CLI cancelled")
			fmt.Println()
			return
		}
	} else if verbose {
		fmt.Println("Debug: Auto-update enabled, proceeding without confirmation")
	}

	// User confirmed or auto-update enabled, proceed with update
	exe, err := os.Executable()
	if err != nil {
		if verbose {
			fmt.Printf("Debug: Error getting executable path: %v\n", err)
		}
		fmt.Println("Error getting executable path:", err)
		os.Exit(1)
		return
	}

	err = updater.UpdateTo(context.Background(), latest, exe)
	if err != nil {
		if verbose {
			fmt.Printf("Debug: Update failed with error: %v\n", err)
		}
		fmt.Println("Binary update failed:", err)
		os.Exit(1)
		return
	}

	if verbose {
		fmt.Printf("Debug: Update successful - previous version: %s, new version: %s\n", v, latest.Version())
	}
	_ = spinners.RunInfoSpinner(fmt.Sprintf("Successfully updated sb CLI to version: %s", latest.Version()))

	// Print an optional message if provided
	if optionalMessage != "" {
		_ = spinners.RunWarningSpinner(optionalMessage)
	}
	fmt.Println("")
	os.Exit(0)
}
