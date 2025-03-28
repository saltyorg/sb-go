package cmd

import (
	"fmt"
	"os"

	"github.com/blang/semver"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
	"github.com/saltyorg/sb-go/runtime"
	"github.com/spf13/cobra"
)

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
}

func doSelfUpdate() {
	v := semver.MustParse(runtime.Version)
	latest, err := selfupdate.UpdateSelf(v, "saltyorg/sb-go")
	if err != nil {
		fmt.Println("Binary update failed:", err)
		os.Exit(1)
		return
	}
	if latest.Version.Equals(v) {
		fmt.Println("Current binary is the latest version:", runtime.Version)
	} else {
		fmt.Println("Successfully updated to version:", latest.Version)
	}
}
