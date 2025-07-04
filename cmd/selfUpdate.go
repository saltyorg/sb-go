package cmd

import (
	"bufio"
	"fmt"
	"github.com/saltyorg/sb-go/spinners"
	"os"
	"strings"

	"github.com/blang/semver"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
	"github.com/saltyorg/sb-go/runtime"
	"github.com/spf13/cobra"
)

// Debug flag to enable verbose output
var debug bool

// Auto-accept flag to skip confirmation
var autoAccept bool

// selfUpdateCmd represents the selfUpdate command
var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Update Saltbox CLI",
	Long:  `Update Saltbox CLI`,
	Run: func(cmd *cobra.Command, args []string) {
		doSelfUpdate(autoAccept, debug, "")
	},
}

func init() {
	rootCmd.AddCommand(selfUpdateCmd)

	// Add debug flag
	selfUpdateCmd.Flags().BoolVarP(&debug, "verbose", "v", false, "Enable verbose debug output")
	// Add auto-accept flag
	selfUpdateCmd.Flags().BoolVarP(&autoAccept, "yes", "y", false, "Automatically accept update without confirmation")
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

func doSelfUpdate(autoUpdate bool, verbose bool, optionalMessage string) {
	if verbose {
		fmt.Println("Debug: Starting self-update process")
		fmt.Printf("Debug: Current version: %s\n", runtime.Version)
		fmt.Printf("Debug: Current git commit: %s\n", runtime.GitCommit)
		fmt.Printf("Debug: Looking for updates in repository: saltyorg/sb-go\n")
		fmt.Printf("Debug: Auto-update mode: %t\n", autoUpdate)

		// Enable detailed logging in the selfupdate package if possible
		//selfupdate.EnableLog()
	}

	v := semver.MustParse(runtime.Version)

	if verbose {
		fmt.Printf("Debug: Parsed semver version: %s\n", v.String())
		fmt.Println("Debug: Checking for latest release from GitHub")
	}

	// First check if an update is available without applying it
	latest, found, err := selfupdate.DetectLatest("saltyorg/sb-go")
	if err != nil {
		if verbose {
			fmt.Printf("Debug: Error checking for updates: %v\n", err)
		}
		fmt.Println("Error checking for updates:", err)
		os.Exit(1)
		return
	}

	if !found || latest.Version.Equals(v) {
		if verbose {
			fmt.Println("Debug: No update available - current version is the latest")
		}
		_ = spinners.RunInfoSpinner(fmt.Sprintf("Current binary is the latest version: %s", runtime.Version))
		return
	}

	// An update is available
	_ = spinners.RunInfoSpinner(fmt.Sprintf("New sb CLI version available: %s (current: %s)", latest.Version, v))

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
	result, err := selfupdate.UpdateSelf(v, "saltyorg/sb-go")
	if err != nil {
		if verbose {
			fmt.Printf("Debug: Update failed with error: %v\n", err)
		}
		fmt.Println("Binary update failed:", err)
		os.Exit(1)
		return
	}

	if verbose {
		fmt.Printf("Debug: Update successful - previous version: %s, new version: %s\n", v, result.Version)
	}
	_ = spinners.RunInfoSpinner(fmt.Sprintf("Successfully updated sb CLI to version: %s", result.Version))

	// Print optional message if provided
	if optionalMessage != "" {
		fmt.Println()
		_ = spinners.RunInfoSpinner(optionalMessage)
	}
	fmt.Println("")
	os.Exit(0)
}
