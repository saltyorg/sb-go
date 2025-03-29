package cmd

import (
	"fmt"
	"os"

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
		doSelfUpdate()
	},
}

func init() {
	rootCmd.AddCommand(selfUpdateCmd)

	// Add debug flag
	selfUpdateCmd.Flags().BoolVarP(&debug, "verbose", "v", false, "Enable verbose debug output")
}

func doSelfUpdate() {
	if debug {
		fmt.Println("Debug: Starting self-update process")
		fmt.Printf("Debug: Current version: %s\n", runtime.Version)
		fmt.Printf("Debug: Current git commit: %s\n", runtime.GitCommit)
		fmt.Printf("Debug: Looking for updates in repository: saltyorg/sb-go\n")

		// Enable detailed logging in the selfupdate package if possible
		selfupdate.EnableLog()
	}

	v := semver.MustParse(runtime.Version)

	if debug {
		fmt.Printf("Debug: Parsed semver version: %s\n", v.String())
		fmt.Println("Debug: Checking for latest release from GitHub")
	}

	latest, err := selfupdate.UpdateSelf(v, "saltyorg/sb-go")

	if err != nil {
		if debug {
			fmt.Printf("Debug: Update failed with error: %v\n", err)
		}
		fmt.Println("Binary update failed:", err)
		os.Exit(1)
		return
	}

	if latest.Version.Equals(v) {
		if debug {
			fmt.Println("Debug: No update available - current version is the latest")
		}
		fmt.Println("Current binary is the latest version:", runtime.Version)
	} else {
		if debug {
			fmt.Printf("Debug: Update successful - previous version: %s, new version: %s\n", v, latest.Version)
		}
		fmt.Println("Successfully updated to version:", latest.Version)
	}
}
