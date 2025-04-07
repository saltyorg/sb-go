package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/blang/semver"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
	"github.com/saltyorg/sb-go/runtime"
	"github.com/spf13/cobra"
)

// Debug flag to enable verbose output
var debug bool

// selfUpdateCmd represents the selfUpdate command
var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Update Saltbox CLI",
	Long:  `Update Saltbox CLI`,
	Run: func(cmd *cobra.Command, args []string) {
		// When called from command, pass along the debug flag value
		doSelfUpdate(false, debug)
	},
}

func init() {
	rootCmd.AddCommand(selfUpdateCmd)

	// Add debug flag
	selfUpdateCmd.Flags().BoolVarP(&debug, "verbose", "v", false, "Enable verbose debug output")
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

func doSelfUpdate(autoUpdate bool, verbose bool) {
	if verbose {
		fmt.Println("Debug: Starting self-update process")
		fmt.Printf("Debug: Current version: %s\n", runtime.Version)
		fmt.Printf("Debug: Current git commit: %s\n", runtime.GitCommit)
		fmt.Printf("Debug: Looking for updates in repository: saltyorg/sb-go\n")

		// Enable detailed logging in the selfupdate package if possible
		selfupdate.EnableLog()
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
		fmt.Println("Current binary is the latest version:", runtime.Version)
		return
	}

	// An update is available
	fmt.Printf("New sb CLI version available: %s (current: %s)\n", latest.Version, v)

	// If autoUpdate is false, ask for confirmation
	if !autoUpdate {
		if !promptForConfirmation("Do you want to update") {
			fmt.Println("Update cancelled")
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
	fmt.Println("Successfully updated sb CLI to version:", result.Version)
	fmt.Println()
}
