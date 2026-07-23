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
		runner := spinners.NewRunner(spinners.RunnerOptions{Verbose: verbose})
		return runDockerRestart(ctx, runner, verbose, ignoreContainers, spinners.CollapseChildTasks)
	},
}

func runDockerRestart(
	ctx context.Context,
	runner *spinners.Runner,
	verbose bool,
	ignoreContainers []string,
	childDisplay spinners.ChildDisplay,
) error {
	return runner.Run(ctx, spinners.TaskSpec{
		Running:      "Restarting Docker containers",
		Success:      "Docker containers restarted",
		Failure:      "Docker container restart",
		ChildDisplay: childDisplay,
	}, func(ctx context.Context, task *spinners.Task) error {
		return performDockerRestart(ctx, task, verbose, ignoreContainers, childDisplay)
	})
}

func performDockerRestart(
	ctx context.Context,
	task *spinners.Task,
	verbose bool,
	ignoreContainers []string,
	childDisplay spinners.ChildDisplay,
) error {
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
	if err := task.Run(ctx, spinners.TaskSpec{
		Running: "Checking Docker controller service",
		Success: "Docker controller service ready",
		Failure: "Docker controller service check",
	}, func(context.Context, *spinners.Task) error {
		return serviceCheckTask()
	}); err != nil {
		return fmt.Errorf("error: %v", err)
	}

	// Create a stop containers task
	stopContainersTask := func(stopTask *spinners.Task) error {
		if verbose && len(ignoreContainers) > 0 {
			stopTask.Info(fmt.Sprintf("Ignoring containers: %s", strings.Join(ignoreContainers, ", ")))
		}

		client := &http.Client{Timeout: 10 * time.Second}
		var stopJobResp JobResponse
		if err := stopTask.Run(ctx, spinners.TaskSpec{Running: "Requesting Docker stop job"}, func(context.Context, *spinners.Task) error {
			var err error
			stopJobResp, err = requestDockerJob(ctx, constants.DockerControllerAPIURL+"/stop", ignoreContainers, client)
			return err
		}); err != nil {
			return fmt.Errorf("failed to stop containers: %w", err)
		}

		if verbose {
			stopTask.Info(fmt.Sprintf("Stopping containers. Job ID: %s", stopJobResp.JobID))
		}

		// Display polling while it is active. The parent stop task collapses
		// this child on success and retains it when it fails.
		var success bool
		if err := stopTask.Run(ctx, spinners.TaskSpec{Running: "Waiting for Docker stop job"}, func(context.Context, *spinners.Task) error {
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
	startContainersTask := func(startTask *spinners.Task) error {
		client := &http.Client{Timeout: 10 * time.Second}
		var startJobResp JobResponse
		if err := startTask.Run(ctx, spinners.TaskSpec{Running: "Requesting Docker start job"}, func(context.Context, *spinners.Task) error {
			var err error
			startJobResp, err = requestDockerJob(ctx, constants.DockerControllerAPIURL+"/start", nil, client)
			return err
		}); err != nil {
			return fmt.Errorf("failed to start containers: %w", err)
		}

		if verbose {
			startTask.Info(fmt.Sprintf("Starting containers. Job ID: %s", startJobResp.JobID))
		}

		// Display polling while it is active. The parent start task collapses
		// this child on success and retains it when it fails.
		var success bool
		if err := startTask.Run(ctx, spinners.TaskSpec{Running: "Waiting for Docker start job"}, func(context.Context, *spinners.Task) error {
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
	stopSpec := spinners.TaskSpec{
		Running:      "Stopping Docker containers",
		Success:      "Stopped Docker containers",
		Failure:      "Stop Docker containers",
		ChildDisplay: childDisplay,
	}
	startSpec := spinners.TaskSpec{
		Running:      "Starting Docker containers",
		Success:      "Started Docker containers",
		Failure:      "Start Docker containers",
		ChildDisplay: childDisplay,
	}

	if err := task.Run(ctx, stopSpec, func(_ context.Context, stopTask *spinners.Task) error {
		return stopContainersTask(stopTask)
	}); err != nil {
		return fmt.Errorf("error: %v", err)
	}
	if err := task.Run(ctx, startSpec, func(_ context.Context, startTask *spinners.Task) error {
		return startContainersTask(startTask)
	}); err != nil {
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
