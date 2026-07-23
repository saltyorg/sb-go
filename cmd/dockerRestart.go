package cmd

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

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
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		verbose, _ := cmd.Flags().GetBool("verbose")
		ignoreContainers, _ := cmd.Flags().GetStringSlice("ignore")
		spinners.SetVerboseMode(verbose)
		return spinners.RunTaskWithSpinnerCustomContext(ctx, spinners.SpinnerOptions{
			TaskName:         "Restarting Docker containers",
			StopMessage:      "Docker containers restarted",
			StopFailMessage:  "Docker container restart",
			CollapseChildren: true,
		}, func() error {
			return runDockerRestart(ctx, verbose, ignoreContainers)
		})
	},
}

func runDockerRestart(ctx context.Context, verbose bool, ignoreContainers []string) error {
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

	// Create a stop containers task
	stopContainersTask := func() error {
		if verbose && len(ignoreContainers) > 0 {
			_ = spinners.RunInfoSpinner(fmt.Sprintf("Ignoring containers: %s", strings.Join(ignoreContainers, ", ")))
		}

		client := &http.Client{Timeout: 10 * time.Second}
		stopJobResp, err := requestDockerJob(ctx, constants.DockerControllerAPIURL+"/stop", ignoreContainers, client)
		if err != nil {
			return fmt.Errorf("failed to stop containers: %w", err)
		}

		if verbose {
			_ = spinners.RunInfoSpinner(fmt.Sprintf("Stopping containers. Job ID: %s", stopJobResp.JobID))
		}

		// Wait for the stop job to complete
		var success bool
		if err := spinners.RunTaskWithSpinnerContext(ctx, "Waiting for Docker stop job", func() error {
			var err error
			success, err = waitForJobCompletion(ctx, stopJobResp.JobID)
			return err
		}); err != nil {
			return fmt.Errorf("error while stopping containers: %w", err)
		}

		if !success {
			return fmt.Errorf("failed to stop containers")
		}

		return nil
	}

	// Create a start containers task
	startContainersTask := func() error {
		client := &http.Client{Timeout: 10 * time.Second}
		startJobResp, err := requestDockerJob(ctx, constants.DockerControllerAPIURL+"/start", nil, client)
		if err != nil {
			return fmt.Errorf("failed to start containers: %w", err)
		}

		if verbose {
			_ = spinners.RunInfoSpinner(fmt.Sprintf("Starting containers. Job ID: %s", startJobResp.JobID))
		}

		// Wait for the start job to complete
		var success bool
		if err := spinners.RunTaskWithSpinnerContext(ctx, "Waiting for Docker start job", func() error {
			var err error
			success, err = waitForJobCompletion(ctx, startJobResp.JobID)
			return err
		}); err != nil {
			return fmt.Errorf("error while starting containers: %w", err)
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
		StopFailMessage: "Stop Docker containers",
	}

	startOpts := spinners.SpinnerOptions{
		TaskName:        "Starting Docker containers",
		StopMessage:     "Started Docker containers",
		StopFailMessage: "Start Docker containers",
	}

	if err := spinners.RunTaskWithSpinnerCustomContext(ctx, stopOpts, stopContainersTask); err != nil {
		return fmt.Errorf("error: %v", err)
	}
	if err := spinners.RunTaskWithSpinnerCustomContext(ctx, startOpts, startContainersTask); err != nil {
		return fmt.Errorf("error: %v", err)
	}
	return nil
}

func init() {
	// Add verbose flag
	restartCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")

	// Add ignore flag
	restartCmd.Flags().StringSlice("ignore", []string{}, "Containers to ignore during restart operation (can be specified multiple times)")
}
