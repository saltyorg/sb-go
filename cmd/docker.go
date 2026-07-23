package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/executor"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

// Define lipgloss styles for colored terminal output.
var (
	redStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("160"))
	yellowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	greenStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("40"))
)

// JobResponse represents the JSON response containing a job identifier.
type JobResponse struct {
	JobID string `json:"job_id"`
}

// StatusResponse represents the JSON response that includes the current job status.
type StatusResponse struct {
	Status string `json:"status"`
}

// Job status constants to avoid using magic strings.
const (
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
	JobStatusPending   = "pending"
	JobStatusRunning   = "running"

	dockerAPIResponseLimit = 1 << 20
	dockerJobPollInterval  = 5 * time.Second
	dockerJobMaxPolls      = 60
)

// dockerCmd is the primary command for managing Docker containers in Saltbox.
var dockerCmd = &cobra.Command{
	Use:                "docker",
	Short:              "Manage Docker containers managed by Saltbox",
	Long:               `Manage Docker containers managed by Saltbox`,
	DisableFlagParsing: false,
	Args:               cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// If args are provided, it means an unknown subcommand was used
		if len(args) > 0 {
			normalStyle := lipgloss.NewStyle()
			return fmt.Errorf("%s", normalStyle.Render(fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())))
		}
		// No args - show help
		return cmd.Help()
	},
}

// isServiceExistAndRunning checks whether the Docker controller service file exists
// and whether the service is currently active.
func isServiceExistAndRunning(ctx context.Context) (bool, bool, error) {
	// Verify the existence of the service file.
	_, err := os.Stat(constants.DockerControllerServiceFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// The service file does not exist.
			return false, false, nil
		}
		return false, false, err
	}

	// Use a context with timeout to execute the systemctl command.
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Run "systemctl is-active" to determine if the service is running.
	result, err := executor.Run(cmdCtx, "systemctl",
		executor.WithArgs("is-active", "saltbox_managed_docker_controller.service"),
		executor.WithOutputMode(executor.OutputModeCapture),
	)
	if err != nil {
		// The service exists but is not running.
		return true, false, nil
	}

	status := strings.TrimSpace(string(result.Stdout))
	return true, status == "active", nil
}

// getJobStatus sends an HTTP GET request to the given URL, reads the JSON response,
// and returns the parsed StatusResponse.
func getJobStatus(ctx context.Context, url string, client *http.Client) (StatusResponse, error) {
	var statusResp StatusResponse

	// Create a new HTTP request with context.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return statusResp, err
	}

	// Execute the HTTP request.
	resp, err := client.Do(req)
	if err != nil {
		return statusResp, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Ensure a successful HTTP response.
	if resp.StatusCode != http.StatusOK {
		return statusResp, fmt.Errorf("job status check failed with status code: %d", resp.StatusCode)
	}

	decoder := json.NewDecoder(io.LimitReader(resp.Body, dockerAPIResponseLimit))
	if err := decoder.Decode(&statusResp); err != nil {
		return statusResp, fmt.Errorf("decode job status response: %w", err)
	}
	if statusResp.Status == "" {
		return statusResp, fmt.Errorf("job status response is missing status")
	}

	return statusResp, nil
}

func requestDockerJob(ctx context.Context, endpoint string, ignoreContainers []string, client *http.Client) (JobResponse, error) {
	var jobResp JobResponse

	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return jobResp, fmt.Errorf("parse Docker controller URL: %w", err)
	}
	query := requestURL.Query()
	for _, container := range ignoreContainers {
		if container != "" {
			query.Add("ignore", container)
		}
	}
	requestURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), nil)
	if err != nil {
		return jobResp, fmt.Errorf("create Docker controller request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return jobResp, fmt.Errorf("send Docker controller request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return jobResp, fmt.Errorf("Docker controller request failed with status code: %d", resp.StatusCode)
	}

	decoder := json.NewDecoder(io.LimitReader(resp.Body, dockerAPIResponseLimit))
	if err := decoder.Decode(&jobResp); err != nil {
		return jobResp, fmt.Errorf("decode Docker controller response: %w", err)
	}
	if strings.TrimSpace(jobResp.JobID) == "" {
		return jobResp, fmt.Errorf("Docker controller response is missing job ID")
	}
	return jobResp, nil
}

// waitForJobCompletion polls the job status endpoint until the job is completed,
// has failed, or a timeout occurs.
func waitForJobCompletion(ctx context.Context, jobID string) (bool, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	return waitForJobCompletionWithClient(ctx, jobID, constants.DockerControllerAPIURL, client, dockerJobPollInterval, dockerJobMaxPolls)
}

func waitForJobCompletionWithClient(
	ctx context.Context,
	jobID, baseURL string,
	client *http.Client,
	pollInterval time.Duration,
	maxPolls int,
) (bool, error) {
	if strings.TrimSpace(jobID) == "" {
		return false, fmt.Errorf("job ID is empty")
	}
	statusURL := fmt.Sprintf("%s/job_status/%s", strings.TrimRight(baseURL, "/"), url.PathEscape(jobID))

	// Poll the job status endpoint.
	for attempt := range maxPolls {
		statusResp, err := getJobStatus(ctx, statusURL, client)
		if err != nil {
			return false, err
		}

		switch statusResp.Status {
		case JobStatusCompleted:
			return true, nil
		case JobStatusFailed:
			return false, fmt.Errorf("job failed")
		case JobStatusPending, JobStatusRunning:
			if attempt == maxPolls-1 {
				break
			}
			timer := time.NewTimer(pollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return false, ctx.Err()
			case <-timer.C:
			}
		default:
			return false, fmt.Errorf("unknown job status: %s", statusResp.Status)
		}
	}

	return false, fmt.Errorf("timeout waiting for job completion")
}

func containerDisplayName(id string, names []string) string {
	if len(names) > 0 {
		if name := strings.TrimPrefix(names[0], "/"); name != "" {
			return name
		}
	}
	return shortContainerID(id)
}

// init registers the docker command and its associated subcommands.
func init() {
	rootCmd.AddCommand(dockerCmd)

	// Register subcommands for Docker management.
	dockerCmd.AddCommand(startCmd)
	dockerCmd.AddCommand(stopCmd)
	dockerCmd.AddCommand(restartCmd)
	dockerCmd.AddCommand(psCmd)
}
