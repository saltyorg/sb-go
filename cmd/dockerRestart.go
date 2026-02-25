package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/spinners"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

// restartCmd represents the restart command
var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart Docker containers managed by Saltbox",
	Long:  `Restart Docker containers managed by Saltbox in dependency order.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		verbose, _ := cmd.Flags().GetBool("verbose")
		ignoreContainers, _ := cmd.Flags().GetStringSlice("ignore")

		serviceCheckTask := func() error {
			exists, running, err := isServiceExistAndRunning(ctx)
			if err != nil {
				return fmt.Errorf("error checking service status: %v", err)
			}

			if !exists {
				normalStyle := lipgloss.NewStyle()
				return fmt.Errorf("%s", normalStyle.Render("the Docker controller service does not exist"))
			}

			if !running {
				normalStyle := lipgloss.NewStyle()
				return fmt.Errorf("%s", normalStyle.Render("the Docker controller service is not running"))
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
			return fmt.Errorf("error: %v", err)
		}

		// Create a stop containers task
		stopContainersTask := func() error {
			// Build query parameters
			var stopURL strings.Builder
			stopURL.WriteString(fmt.Sprintf("%s/stop", constants.DockerControllerAPIURL))
			if len(ignoreContainers) > 0 {
				stopURL.WriteString("?")
				for i, container := range ignoreContainers {
					if i > 0 {
						stopURL.WriteString("&")
					}
					stopURL.WriteString(fmt.Sprintf("ignore=%s", container))
				}

				if verbose {
					_ = spinners.RunInfoSpinner(fmt.Sprintf("Ignoring containers: %s", strings.Join(ignoreContainers, ", ")))
				}
			}

			resp, err := http.Post(stopURL.String(), "application/json", nil)
			if err != nil {
				return fmt.Errorf("failed to stop containers: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			// Check status code before reading body to prevent memory exhaustion attacks
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("failed to stop containers (status code: %d)", resp.StatusCode)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read response: %v", err)
			}

			var stopJobResp JobResponse
			if err := json.Unmarshal(body, &stopJobResp); err != nil {
				return fmt.Errorf("failed to parse response: %v", err)
			}

			if verbose {
				_ = spinners.RunInfoSpinner(fmt.Sprintf("Stopping containers. Job ID: %s", stopJobResp.JobID))
			}

			// Wait for the stop job to complete
			success, err := waitForJobCompletion(stopJobResp.JobID)
			if err != nil {
				return fmt.Errorf("error while stopping containers: %v", err)
			}

			if !success {
				return fmt.Errorf("failed to stop containers")
			}

			return nil
		}

		// Create a start containers task
		startContainersTask := func() error {
			// Now start containers
			startResp, err := http.Post(fmt.Sprintf("%s/start", constants.DockerControllerAPIURL), "application/json", nil)
			if err != nil {
				return fmt.Errorf("failed to start containers: %v", err)
			}
			defer func() { _ = startResp.Body.Close() }()

			// Check status code before reading body to prevent memory exhaustion attacks
			if startResp.StatusCode != http.StatusOK {
				return fmt.Errorf("failed to start containers (status code: %d)", startResp.StatusCode)
			}

			startBody, err := io.ReadAll(startResp.Body)
			if err != nil {
				return fmt.Errorf("failed to read response: %v", err)
			}

			var startJobResp JobResponse
			if err := json.Unmarshal(startBody, &startJobResp); err != nil {
				return fmt.Errorf("failed to parse response: %v", err)
			}

			if verbose {
				_ = spinners.RunInfoSpinner(fmt.Sprintf("Starting containers. Job ID: %s", startJobResp.JobID))
			}

			// Wait for the start job to complete
			success, err := waitForJobCompletion(startJobResp.JobID)
			if err != nil {
				return fmt.Errorf("error while starting containers: %v", err)
			}

			if !success {
				return fmt.Errorf("failed to start containers")
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
			return fmt.Errorf("error: %v", err)
		}

		// Run spinner for starting containers
		startOpts := spinners.SpinnerOptions{
			TaskName:        "Starting Docker containers",
			StopMessage:     "Started Docker containers",
			StopFailMessage: "Failed to start Docker containers",
		}

		if err := spinners.RunTaskWithSpinnerCustomContext(ctx, startOpts, startContainersTask); err != nil {
			return fmt.Errorf("error: %v", err)
		}

		if verbose {
			_ = spinners.RunInfoSpinner("Containers restarted successfully")
		}
		return nil
	},
}

func init() {
	// Add verbose flag
	restartCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")

	// Add ignore flag
	restartCmd.Flags().StringSlice("ignore", []string{}, "Containers to ignore during restart operation (can be specified multiple times)")
}
