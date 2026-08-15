package cmd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/saltyorg/sb-go/layout"
	"github.com/saltyorg/sb-go/terminal"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

func newDockerStartCommand() *cobra.Command {
	var verbose bool
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start Docker containers managed by Saltbox",
		Long:  `Start Docker containers managed by Saltbox in dependency order.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: verbose})
			return runDockerStart(ctx, runner, verbose, terminal.CollapseChildTasks)
		},
	}
	startCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	return startCmd
}

func runDockerStart(
	ctx context.Context,
	runner *terminal.Runner,
	verbose bool,
	childDisplay terminal.ChildDisplay,
) error {
	return runner.Run(ctx, terminal.TaskSpec{
		Running:      "Starting Docker containers",
		Success:      "Docker containers started",
		Failure:      "Docker container start",
		ChildDisplay: childDisplay,
	}, func(ctx context.Context, task *terminal.Task) error {
		return performDockerStart(ctx, task, verbose)
	})
}

func performDockerStart(ctx context.Context, task *terminal.Task, verbose bool) error {
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
	if err := task.Run(ctx, terminal.TaskSpec{
		Running: "Checking Docker controller service",
		Success: "Docker controller service ready",
		Failure: "Docker controller service check",
	}, func(context.Context, *terminal.Task) error {
		return serviceCheckTask()
	}); err != nil {
		return fmt.Errorf("error: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	var jobResp JobResponse
	if err := task.Run(ctx, terminal.TaskSpec{Running: "Requesting Docker start job"}, func(context.Context, *terminal.Task) error {
		var err error
		jobResp, err = requestDockerJob(ctx, layout.DockerControllerAPIURL+"/start", nil, client)
		return err
	}); err != nil {
		return fmt.Errorf("failed to start containers: %w", err)
	}

	if verbose {
		task.Info(fmt.Sprintf("Starting containers. Job ID: %s", jobResp.JobID))
	}

	var success bool
	if err := task.Run(ctx, terminal.TaskSpec{Running: "Waiting for Docker start job"}, func(context.Context, *terminal.Task) error {
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
