package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/saltyorg/sb-go/constants"
	"github.com/saltyorg/sb-go/spinners"
	"github.com/spf13/cobra"
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start Docker containers managed by Saltbox",
	Long:  `Start Docker containers managed by Saltbox in dependency order.`,
	Run: func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")

		serviceCheckTask := func() error {
			exists, running, err := isServiceExistAndRunning()
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
			Color:           "blue",
			StopColor:       "green",
			StopFailColor:   "red",
			StopMessage:     "Docker controller service ready",
			StopFailMessage: "Docker controller service check failed",
		}

		if err := spinners.RunTaskWithSpinnerCustom(opts, serviceCheckTask); err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		// Create start container task
		startContainersTask := func() error {
			// Call the API to start containers
			resp, err := http.Post(fmt.Sprintf("%s/start", constants.DockerControllerAPIURL), "application/json", nil)
			if err != nil {
				return fmt.Errorf("failed to start containers: %v", err)
			}
			defer resp.Body.Close()

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
				fmt.Printf("Starting containers. Job ID: %s\n", jobResp.JobID)
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
			Color:           "yellow",
			StopColor:       "green",
			StopFailColor:   "red",
			StopMessage:     "Started Docker containers",
			StopFailMessage: "Failed to start Docker containers",
		}

		if err := spinners.RunTaskWithSpinnerCustom(startOpts, startContainersTask); err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		if verbose {
			fmt.Println("Containers started successfully.")
		}
	},
}

func init() {
	// Add verbose flag
	startCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")
}
