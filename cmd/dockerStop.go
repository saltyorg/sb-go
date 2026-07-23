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
		runner := spinners.NewRunner(spinners.RunnerOptions{Verbose: verbose})
		return runDockerStop(ctx, runner, verbose, ignoreContainers, spinners.CollapseChildTasks)
	},
}

func runDockerStop(
	ctx context.Context,
	runner *spinners.Runner,
	verbose bool,
	ignoreContainers []string,
	childDisplay spinners.ChildDisplay,
) error {
	return runner.Run(ctx, spinners.TaskSpec{
		Running:      "Stopping Docker containers",
		Success:      "Docker containers stopped",
		Failure:      "Docker container stop",
		ChildDisplay: childDisplay,
	}, func(ctx context.Context, task *spinners.Task) error {
		return performDockerStop(ctx, task, verbose, ignoreContainers)
	})
}

func performDockerStop(ctx context.Context, task *spinners.Task, verbose bool, ignoreContainers []string) error {
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

	if verbose && len(ignoreContainers) > 0 {
		task.Info(fmt.Sprintf("Ignoring containers: %s", strings.Join(ignoreContainers, ", ")))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	var jobResp JobResponse
	if err := task.Run(ctx, spinners.TaskSpec{Running: "Requesting Docker stop job"}, func(context.Context, *spinners.Task) error {
		var err error
		jobResp, err = requestDockerJob(ctx, constants.DockerControllerAPIURL+"/stop", ignoreContainers, client)
		return err
	}); err != nil {
		return fmt.Errorf("failed to stop containers: %w", err)
	}

	if verbose {
		task.Info(fmt.Sprintf("Stopping containers. Job ID: %s", jobResp.JobID))
	}

	var success bool
	if err := task.Run(ctx, spinners.TaskSpec{Running: "Waiting for Docker stop job"}, func(context.Context, *spinners.Task) error {
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
