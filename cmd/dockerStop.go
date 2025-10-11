package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/spinners"

	"github.com/spf13/cobra"
)

// stopCmd represents the stop command
var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop Docker containers managed by Saltbox",
	Long:  `Stop Docker containers managed by Saltbox in dependency order.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()
		verbose, _ := cmd.Flags().GetBool("verbose")
		ignoreContainers, _ := cmd.Flags().GetStringSlice("ignore")

		serviceCheckTask := func() error {
			exists, running, err := isServiceExistAndRunning(ctx)
			if err != nil {
				return fmt.Errorf("error checking service status: %v", err)
			}

			if !exists {
				return fmt.Errorf("the Docker controller service does not exist")
			}

			if !running {
				return fmt.Errorf("the Docker controller service is not running")
			}
			return nil
		}

		// Check service with spinner
		opts := spinners.SpinnerOptions{
			TaskName:        "Checking Docker controller service",
			StopMessage:     "Docker controller service ready",
			StopFailMessage: "Docker controller service check failed",
		}

		if err := spinners.RunTaskWithSpinnerCustomContext(ctx, opts, serviceCheckTask); err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		// Create a stop container task
		stopContainersTask := func() error {
			// Build query parameters
			url := fmt.Sprintf("%s/stop", constants.DockerControllerAPIURL)
			if len(ignoreContainers) > 0 {
				url += "?"
				for i, container := range ignoreContainers {
					if i > 0 {
						url += "&"
					}
					url += fmt.Sprintf("ignore=%s", container)
				}

				if verbose {
					_ = spinners.RunInfoSpinner(fmt.Sprintf("Ignoring containers: %s", strings.Join(ignoreContainers, ", ")))
				}
			}

			// Call the API to stop containers
			resp, err := http.Post(url, "application/json", nil)
			if err != nil {
				return fmt.Errorf("failed to stop containers: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("failed to stop containers (status code: %d)", resp.StatusCode)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read response: %v", err)
			}

			var jobResp JobResponse
			if err := json.Unmarshal(body, &jobResp); err != nil {
				return fmt.Errorf("failed to parse response: %v", err)
			}

			if verbose {
				_ = spinners.RunInfoSpinner(fmt.Sprintf("Stopping containers. Job ID: %s", jobResp.JobID))
			}

			// Wait for job completion
			success, err := waitForJobCompletion(jobResp.JobID)
			if err != nil {
				return fmt.Errorf("error while stopping containers: %v", err)
			}

			if !success {
				return fmt.Errorf("failed to stop containers")
			}

			return nil
		}

		// Run spinner for stopping containers
		stopOpts := spinners.SpinnerOptions{
			TaskName:        "Stopping Docker containers",
			StopMessage:     "Stopped Docker containers",
			StopFailMessage: "Failed to stop Docker containers",
		}

		if err := spinners.RunTaskWithSpinnerCustomContext(ctx, stopOpts, stopContainersTask); err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		if verbose {
			_ = spinners.RunInfoSpinner("Containers stopped successfully")
		}
	},
}

func init() {
	// Add verbose flag
	stopCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")

	// Add ignore flag
	stopCmd.Flags().StringSlice("ignore", []string{}, "Containers to ignore during stop operation (can be specified multiple times)")
}
