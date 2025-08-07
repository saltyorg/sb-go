package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// benchCmd represents the bench command
var benchCmd = &cobra.Command{
	Use:   "bench",
	Short: "Runs bench.sh benchmark",
	Long:  `Runs bench.sh benchmark`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runBenchmark(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "An error occurred while executing the benchmark: %v\n", err)
			os.Exit(1)
		}
	},
}

func runBenchmark() error {
	// Create a variable to track our temporary file
	var tempFileName string

	// Create a cleanup function
	cleanup := func() {
		if tempFileName != "" {
			if err := os.Remove(tempFileName); err != nil {
				fmt.Printf("Failed to remove temporary file %s: %v\n", tempFileName, err)
			}
		}
	}

	// Ensure cleanup happens when the function returns normally
	defer cleanup()

	// Create HTTP client
	client := &http.Client{}

	// Create request
	req, err := http.NewRequest("GET", "https://bench.sh", nil)
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

	// Create command
	command := exec.Command("bash", tempFileName)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	// Set up signal handling
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	// Flag to track if we've been interrupted
	interrupted := false

	// Start the command
	if err := command.Start(); err != nil {
		return fmt.Errorf("failed to start bench.sh: %w", err)
	}

	// Handle interrupt in a goroutine
	go func() {
		<-signalChan

		interrupted = true

		if signalErr := command.Process.Signal(os.Interrupt); signalErr != nil {
			fmt.Println("Error sending interrupt signal:", signalErr)
		}
	}()

	// Wait for the command to complete
	err = command.Wait()

	// Custom handling for interruption
	if interrupted {
		fmt.Println("\nScript was interrupted by the user.")
		return nil
	}

	if err != nil {
		return fmt.Errorf("error executing bench.sh: %w", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(benchCmd)
}
