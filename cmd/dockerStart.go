package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/spinners"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start Docker containers managed by Saltbox",
	Long:  `Start Docker containers managed by Saltbox in dependency order.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		verbose, _ := cmd.Flags().GetBool("verbose")

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

		// Create a start container task
		startContainersTask := func() error {
			// Call the API to start containers
			resp, err := http.Post(fmt.Sprintf("%s/start", constants.DockerControllerAPIURL), "application/json", nil)
			if err != nil {
				return fmt.Errorf("failed to start containers: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("failed to start containers (status code: %d)", resp.StatusCode)
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
				_ = spinners.RunInfoSpinner(fmt.Sprintf("Starting containers. Job ID: %s", jobResp.JobID))
			}

			// Wait for job completion
			success, err := waitForJobCompletion(jobResp.JobID)
			if err != nil {
				return fmt.Errorf("error while starting containers: %v", err)
			}

			if !success {
				return fmt.Errorf("failed to start containers")
			}

			return nil
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
			_ = spinners.RunInfoSpinner("Containers started successfully")
		}
		return nil
	},
}

func init() {
	// Add verbose flag
	startCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")
}
