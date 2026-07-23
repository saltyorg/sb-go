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

// stopCmd represents the stop command
var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop Docker containers managed by Saltbox",
	Long:  `Stop Docker containers managed by Saltbox in dependency order.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		verbose, _ := cmd.Flags().GetBool("verbose")
		ignoreContainers, _ := cmd.Flags().GetStringSlice("ignore")
		spinners.SetVerboseMode(verbose)
		return spinners.RunTaskWithSpinnerCustomContext(ctx, spinners.SpinnerOptions{
			TaskName:         "Stopping Docker containers",
			StopMessage:      "Docker containers stopped",
			StopFailMessage:  "Docker container stop",
			CollapseChildren: true,
		}, func() error {
			return runDockerStop(ctx, verbose, ignoreContainers)
		})
	},
}

func runDockerStop(ctx context.Context, verbose bool, ignoreContainers []string) error {
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

	if verbose && len(ignoreContainers) > 0 {
		_ = spinners.RunInfoSpinner(fmt.Sprintf("Ignoring containers: %s", strings.Join(ignoreContainers, ", ")))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	var jobResp JobResponse
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Requesting Docker stop job", func() error {
		var err error
		jobResp, err = requestDockerJob(ctx, constants.DockerControllerAPIURL+"/stop", ignoreContainers, client)
		return err
	}); err != nil {
		return fmt.Errorf("failed to stop containers: %w", err)
	}

	if verbose {
		_ = spinners.RunInfoSpinner(fmt.Sprintf("Stopping containers. Job ID: %s", jobResp.JobID))
	}

	var success bool
	if err := spinners.RunTaskWithSpinnerContext(ctx, "Waiting for Docker stop job", func() error {
		var err error
		success, err = waitForJobCompletion(ctx, jobResp.JobID)
		return err
	}); err != nil {
		return fmt.Errorf("error while stopping containers: %w", err)
	}
	if !success {
		return fmt.Errorf("failed to stop containers")
	}

	return nil
}

func init() {
	// Add verbose flag
	stopCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")

	// Add ignore flag
	stopCmd.Flags().StringSlice("ignore", []string{}, "Containers to ignore during stop operation (can be specified multiple times)")
}
