package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/saltyorg/sb-go/internal/errors"
	"github.com/saltyorg/sb-go/internal/executor"

	"github.com/spf13/cobra"
)

// benchCmd represents the bench command
var benchCmd = &cobra.Command{
	Use:   "bench",
	Short: "Runs bench.sh benchmark",
	Long:  `Runs bench.sh benchmark`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if err := runBenchmark(ctx); err != nil {
			return err
		}
		return nil
	},
}

func runBenchmark(ctx context.Context) error {
	// Create a variable to track our temporary file
	var tempFileName string

	// Create a cleanup function
	cleanup := func() {
		if tempFileName != "" {
			if err := os.Remove(tempFileName); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to remove temporary file %s: %v\n", tempFileName, err)
			}
		}
	}

	// Ensure cleanup happens when the function returns normally
	defer cleanup()

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create request with context for cancellation support
	req, err := http.NewRequestWithContext(ctx, "GET", "https://bench.sh", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set User-Agent to emulate curl
	req.Header.Set("User-Agent", "curl/8.5.0")

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download bench.sh: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			if err != nil {
				err = fmt.Errorf("%w; failed to close response body: %v", err, closeErr)
			} else {
				err = fmt.Errorf("failed to close response body: %v", closeErr)
			}
		}
	}()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received non-OK response: %d", resp.StatusCode)
	}

	// Create a temporary file to store the script
	tmpFile, err := os.CreateTemp("", "bench-*.sh")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}

	// Store the filename for cleanup
	tempFileName = tmpFile.Name()

	// Copy the response body to the temp file
	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write response to file: %w", err)
	}

	// Close the file to ensure all data is written
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Run the command using unified executor
	_, err = executor.Run(ctx, "bash",
		executor.WithArgs(tempFileName),
		executor.WithOutputMode(executor.OutputModeStream),
	)

	if err != nil {
		if errors.HandleInterruptError(err) {
			return fmt.Errorf("benchmark execution interrupted by user")
		}
		return err
	}

	return nil
}

func init() {
	rootCmd.AddCommand(benchCmd)
}
