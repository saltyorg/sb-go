package cmd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/spinners"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start Docker containers managed by Saltbox",
	Long:  `Start Docker containers managed by Saltbox in dependency order.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		verbose, _ := cmd.Flags().GetBool("verbose")
		spinners.SetVerboseMode(verbose)
		return spinners.RunTaskWithSpinnerCustomContext(ctx, spinners.SpinnerOptions{
			TaskName:         "Starting Docker containers",
			StopMessage:      "Docker containers started",
			StopFailMessage:  "Docker container start",
			CollapseChildren: true,
		}, func() error {
			return runDockerStart(ctx, verbose)
		})
	},
}

func runDockerStart(ctx context.Context, verbose bool) error {
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
		StopFailMessage: "Docker controller service check",
	}

	if err := spinners.RunTaskWithSpinnerCustomContext(ctx, opts, serviceCheckTask); err != nil {
		return fmt.Errorf("error: %v", err)
	}

	// Create a start container task
	startContainersTask := func() error {
		client := &http.Client{Timeout: 10 * time.Second}
		jobResp, err := requestDockerJob(ctx, constants.DockerControllerAPIURL+"/start", nil, client)
		if err != nil {
			return fmt.Errorf("failed to start containers: %w", err)
		}

		if verbose {
			_ = spinners.RunInfoSpinner(fmt.Sprintf("Starting containers. Job ID: %s", jobResp.JobID))
		}

		// Wait for job completion
		var success bool
		if err := spinners.RunTaskWithSpinnerContext(ctx, "Waiting for Docker start job", func() error {
			var err error
			success, err = waitForJobCompletion(ctx, jobResp.JobID)
			return err
		}); err != nil {
			return fmt.Errorf("error while starting containers: %w", err)
		}

		if !success {
			return fmt.Errorf("failed to start containers")
		}

		return nil
	}

	// Run spinner for starting containers
	startOpts := spinners.SpinnerOptions{
		TaskName:        "Requesting Docker container start",
		StopMessage:     "Docker start job completed",
		StopFailMessage: "Docker start job",
	}

	if err := spinners.RunTaskWithSpinnerCustomContext(ctx, startOpts, startContainersTask); err != nil {
		return fmt.Errorf("error: %v", err)
	}

	return nil
}

func init() {
	// Add verbose flag
	startCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")
}
