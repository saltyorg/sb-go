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
		runner := spinners.NewRunner(spinners.RunnerOptions{Verbose: verbose})
		return runDockerStart(ctx, runner, verbose, spinners.CollapseChildTasks)
	},
}

func runDockerStart(
	ctx context.Context,
	runner *spinners.Runner,
	verbose bool,
	childDisplay spinners.ChildDisplay,
) error {
	return runner.Run(ctx, spinners.TaskSpec{
		Running:      "Starting Docker containers",
		Success:      "Docker containers started",
		Failure:      "Docker container start",
		ChildDisplay: childDisplay,
	}, func(ctx context.Context, task *spinners.Task) error {
		return performDockerStart(ctx, task, verbose)
	})
}

func performDockerStart(ctx context.Context, task *spinners.Task, verbose bool) error {
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

	client := &http.Client{Timeout: 10 * time.Second}
	var jobResp JobResponse
	if err := task.Run(ctx, spinners.TaskSpec{Running: "Requesting Docker start job"}, func(context.Context, *spinners.Task) error {
		var err error
		jobResp, err = requestDockerJob(ctx, constants.DockerControllerAPIURL+"/start", nil, client)
		return err
	}); err != nil {
		return fmt.Errorf("failed to start containers: %w", err)
	}

	if verbose {
		task.Info(fmt.Sprintf("Starting containers. Job ID: %s", jobResp.JobID))
	}

	var success bool
	if err := task.Run(ctx, spinners.TaskSpec{Running: "Waiting for Docker start job"}, func(context.Context, *spinners.Task) error {
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

func init() {
	// Add verbose flag
	startCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")
}
