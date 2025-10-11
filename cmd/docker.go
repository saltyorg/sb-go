package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/constants"

	"github.com/charmbracelet/lipgloss"
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
)

// dockerCmd is the primary command for managing Docker containers in Saltbox.
var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Manage Docker containers managed by Saltbox",
	Long:  `Manage Docker containers managed by Saltbox`,
	Run: func(cmd *cobra.Command, args []string) {
		// Show help if no subcommand is provided.
		cmd.Help()
	},
}

// isServiceExistAndRunning checks whether the Docker controller service file exists
// and whether the service is currently active.
func isServiceExistAndRunning() (bool, bool, error) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run "systemctl is-active" to determine if the service is running.
	cmd := exec.CommandContext(ctx, "systemctl", "is-active", "saltbox_managed_docker_controller.service")
	output, err := cmd.Output()
	if err != nil {
		// The service exists but is not running.
		return true, false, nil
	}

	status := strings.TrimSpace(string(output))
	return true, status == "active", nil
}

// getJobStatus sends an HTTP GET request to the given URL, reads the JSON response,
// and returns the parsed StatusResponse.
func getJobStatus(url string, client *http.Client) (StatusResponse, error) {
	var statusResp StatusResponse

	// Create a new HTTP request with context.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return statusResp, err
	}

	// Execute the HTTP request.
	resp, err := client.Do(req)
	if err != nil {
		return statusResp, err
	}
	defer resp.Body.Close()

	// Ensure a successful HTTP response.
	if resp.StatusCode != http.StatusOK {
		return statusResp, fmt.Errorf("job status check failed with status code: %d", resp.StatusCode)
	}

	// Read and unmarshal the response body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return statusResp, err
	}

	if err := json.Unmarshal(body, &statusResp); err != nil {
		return statusResp, err
	}

	return statusResp, nil
}

// waitForJobCompletion polls the job status endpoint until the job is completed,
// has failed, or a timeout occurs.
func waitForJobCompletion(jobID string) (bool, error) {
	maxRetries := 60 // Polling for 5 minutes with 5-second intervals.
	url := fmt.Sprintf("%s/job_status/%s", constants.DockerControllerAPIURL, jobID)
	client := &http.Client{Timeout: 10 * time.Second}

	// Poll the job status endpoint.
	for range maxRetries {
		statusResp, err := getJobStatus(url, client)
		if err != nil {
			return false, err
		}

		switch statusResp.Status {
		case JobStatusCompleted:
			return true, nil
		case JobStatusFailed:
			return false, fmt.Errorf("job failed")
		case JobStatusPending, JobStatusRunning:
			// Job is still in progress; wait before retrying.
			time.Sleep(5 * time.Second)
		default:
			return false, fmt.Errorf("unknown job status: %s", statusResp.Status)
		}
	}

	return false, fmt.Errorf("timeout waiting for job completion")
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
