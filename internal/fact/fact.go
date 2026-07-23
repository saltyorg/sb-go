package fact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/executor"
	"github.com/saltyorg/sb-go/internal/releaseproxy"
	"github.com/saltyorg/sb-go/internal/spinners"

	"github.com/Masterminds/semver/v3"
)

// retryWithBackoff executes a function with exponential backoff retry logic
func retryWithBackoff(operation func() error, maxRetries int, baseDelay time.Duration) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate delay with exponential backoff (2^attempt * baseDelay)
			delay := min(time.Duration(1<<uint(attempt-1))*baseDelay,
				// Cap maximum delay at 30 seconds
				30*time.Second)
			time.Sleep(delay)
		}

		if err := operation(); err != nil {
			lastErr = err
			if attempt < maxRetries {
				continue // Try again
			}
		} else {
			return nil // Success
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", maxRetries+1, lastErr)
}

// validateBinary performs validation checks on the downloaded Ubuntu x86_64 binary
func validateBinary(filePath string, expectedSize int64, verbose bool) error {
	if verbose {
		fmt.Printf("Validating downloaded binary: %s\n", filePath)
	}

	// Check if file exists and get info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("cannot stat file: %w", err)
	}

	// Check file size matches what GitHub API reported
	actualSize := fileInfo.Size()
	if verbose {
		fmt.Printf("File size check: expected %d bytes, actual %d bytes\n", expectedSize, actualSize)
	}
	if actualSize != expectedSize {
		return fmt.Errorf("file size mismatch: expected %d bytes, got %d bytes", expectedSize, actualSize)
	}

	// Read first 4 bytes to check for ELF header (Ubuntu x86_64 binary)
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file for validation: %w", err)
	}
	defer func() { _ = file.Close() }()

	header := make([]byte, 4)
	if _, err := file.Read(header); err != nil {
		return fmt.Errorf("cannot read file header: %w", err)
	}

	if verbose {
		fmt.Printf("Binary header check: [0x%02x, %s] ", header[0], string(header[1:4]))
	}

	// Check for ELF magic number (0x7F followed by "ELF")
	if len(header) < 4 || header[0] != 0x7F || string(header[1:4]) != "ELF" {
		if verbose {
			fmt.Println("- Invalid ELF header")
		}
		return fmt.Errorf("file is not a valid ELF binary (expected for Ubuntu x86_64)")
	}

	if verbose {
		fmt.Println("- Valid ELF binary")
		fmt.Println("Binary validation passed")
	}

	return nil
}

// getCurrentFactVersion runs the existing saltbox.fact and extracts its version
func getCurrentFactVersion(ctx context.Context, targetPath string) (string, error) {
	// Use context with timeout for executing the binary
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result, err := executor.Run(ctx, targetPath,
		executor.WithOutputMode(executor.OutputModeCombined),
	)
	if err != nil {
		return "", fmt.Errorf("failed to run saltbox.fact: %w", err)
	}

	output := result.Combined

	var currentData map[string]any
	if err = json.Unmarshal(output, &currentData); err != nil {
		return "", fmt.Errorf("failed to parse output: %w", err)
	}

	currentVersion, ok := currentData["saltbox_facts_version"].(string)
	if !ok {
		return "", fmt.Errorf("no version info found")
	}

	return currentVersion, nil
}

// checkIfUpdateNeeded determines if saltbox.fact needs to be updated
func checkIfUpdateNeeded(ctx context.Context, task *spinners.Task, targetPath, latestVersion string, alwaysUpdate bool) (bool, error) {
	if alwaysUpdate {
		task.Info("Reinstall forced.")
		return true, nil
	}

	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		task.Info("saltbox.fact not found. Proceeding with update.")
		return true, nil
	} else if err != nil {
		return false, fmt.Errorf("error checking for existing saltbox.fact: %w", err)
	}

	currentVersion, err := getCurrentFactVersion(ctx, targetPath)
	if err != nil {
		task.Warning(fmt.Sprintf("%v. Proceeding with update.", err))
		return true, nil
	}

	currentSemVer, err := semver.NewVersion(strings.TrimPrefix(currentVersion, "v"))
	if err != nil {
		task.Warning(fmt.Sprintf("Failed to parse current version: %v. Updating...", err))
		return true, nil
	}

	latestSemVer, err := semver.NewVersion(strings.TrimPrefix(latestVersion, "v"))
	if err != nil {
		task.Warning(fmt.Sprintf("Failed to parse latest version: %v. Updating...", err))
		return true, nil
	}

	if currentSemVer.Compare(latestSemVer) >= 0 {
		task.Info(fmt.Sprintf("saltbox.fact is up to date (version %s)", currentVersion))
		return false, nil
	}

	task.Info(fmt.Sprintf("saltbox.fact update available: %s → %s", currentVersion, latestVersion))
	return true, nil
}

type latestReleaseInfo struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

// fetchLatestReleaseInfoFromURL fetches the latest release metadata from a single URL.
func fetchLatestReleaseInfoFromURL(ctx context.Context, client *http.Client, apiURL string) (string, int64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", 0, fmt.Errorf("error creating latest release request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", 0, fmt.Errorf("error fetching latest release info: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != http.StatusOK {
		return "", 0, releaseproxy.HTTPStatus(response.StatusCode)
	}

	var latestRelease latestReleaseInfo
	if err := json.NewDecoder(response.Body).Decode(&latestRelease); err != nil {
		return "", 0, releaseproxy.InvalidResponse("returned invalid JSON", err)
	}
	if strings.TrimSpace(latestRelease.TagName) == "" {
		return "", 0, releaseproxy.InvalidResponse("response is missing tag_name", nil)
	}

	// Find the saltbox-facts asset and get its size.
	for _, asset := range latestRelease.Assets {
		if asset.Name == "saltbox-facts" {
			if asset.Size <= 0 {
				return "", 0, releaseproxy.InvalidResponse(
					fmt.Sprintf("saltbox-facts asset has invalid size %d", asset.Size),
					nil,
				)
			}
			return latestRelease.TagName, asset.Size, nil
		}
	}

	return "", 0, releaseproxy.InvalidResponse("response is missing the saltbox-facts asset", nil)
}

// fetchLatestReleaseInfo fetches latest release info through SVM first, then falls back to direct GitHub API.
func fetchLatestReleaseInfo(ctx context.Context, task *spinners.Task, proxyURL, githubURL string, verbose bool) (string, int64, error) {
	var latestVersion string
	var expectedSize int64
	var fallbackNotified bool

	err := task.Run(ctx, spinners.TaskSpec{Running: "Fetching latest saltbox.fact release info"}, func(taskCtx context.Context, _ *spinners.Task) error {
		return retryWithBackoff(func() error {
			client := &http.Client{
				Timeout: 30 * time.Second,
			}

			version, size, proxyErr := fetchLatestReleaseInfoFromURL(taskCtx, client, proxyURL)
			if proxyErr == nil {
				latestVersion = version
				expectedSize = size
				return nil
			}

			if !fallbackNotified {
				if verbose {
					fmt.Printf("SVM proxy unavailable or unusable (%v); falling back to direct GitHub API\n", proxyErr)
				} else {
					task.Warning(fmt.Sprintf("SVM proxy %s; trying GitHub directly", releaseproxy.Describe(proxyErr)))
				}
				fallbackNotified = true
			}

			version, size, githubErr := fetchLatestReleaseInfoFromURL(taskCtx, client, githubURL)
			if githubErr != nil {
				return fmt.Errorf("proxy request failed: %w; fallback GitHub API request failed: %w", proxyErr, githubErr)
			}

			latestVersion = version
			expectedSize = size
			if verbose {
				fmt.Println("Direct GitHub API fallback succeeded")
			} else {
				task.Info("GitHub fallback succeeded")
			}
			return nil
		}, 3, 1*time.Second) // 3 retries with 1-second base delay
	})

	return latestVersion, expectedSize, err
}

// DownloadAndInstallSaltboxFact downloads and installs the latest saltbox.fact file.
func DownloadAndInstallSaltboxFact(
	ctx context.Context,
	task *spinners.Task,
	alwaysUpdate bool,
	verbose bool,
) error {
	return downloadAndInstallSaltboxFact(ctx, task, alwaysUpdate, verbose)
}

func downloadAndInstallSaltboxFact(ctx context.Context, task *spinners.Task, alwaysUpdate bool, verbose bool) error {
	downloadURL := "https://github.com/saltyorg/ansible-facts/releases/latest/download/saltbox-facts"
	targetPath := "/srv/git/saltbox/ansible_facts.d/saltbox.fact"
	githubURL := "https://api.github.com/repos/saltyorg/ansible-facts/releases/latest"
	proxyURL := fmt.Sprintf("%s?url=%s", constants.SVMVersionProxyURL, githubURL)

	// Fetch the latest release info from GitHub with retry logic
	latestVersion, expectedSize, err := fetchLatestReleaseInfo(ctx, task, proxyURL, githubURL, verbose)
	if err != nil {
		return err
	}

	// Check if we need to update
	needsUpdate, err := checkIfUpdateNeeded(ctx, task, targetPath, latestVersion, alwaysUpdate)
	if err != nil {
		return err
	}

	if //goland:noinspection GoDfaConstantCondition
	needsUpdate {
		// Download and install saltbox.fact with spinner
		taskMessage := fmt.Sprintf("Updating saltbox.fact to version %s", latestVersion)
		if alwaysUpdate {
			taskMessage = fmt.Sprintf("Reinstalling saltbox.fact with version %s", latestVersion)
		}

		if err := task.Run(ctx, spinners.TaskSpec{Running: taskMessage}, func(ctx context.Context, downloadTask *spinners.Task) error {
			return retryWithBackoff(func() error {
				client := &http.Client{
					Timeout: 30 * time.Second,
				}
				response, err := client.Get(downloadURL)
				if err != nil {
					return fmt.Errorf("error downloading saltbox.fact: %w", err)
				}
				defer func() {
					if err := response.Body.Close(); err != nil {
						downloadTask.Warning(fmt.Sprintf("Error closing response body: %v", err))
					}
				}()

				if response.StatusCode != http.StatusOK {
					return fmt.Errorf("unexpected status code: %d", response.StatusCode)
				}

				// Ensure the directory exists
				if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
					return fmt.Errorf("error creating directory: %w", err)
				}

				// Write the content to the file
				file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
				if err != nil {
					return fmt.Errorf("error opening file: %w", err)
				}
				defer func() {
					if err := file.Close(); err != nil {
						downloadTask.Warning(fmt.Sprintf("Error closing file: %v", err))
					}
				}()

				_, err = io.Copy(file, response.Body)
				if err != nil {
					return fmt.Errorf("error writing file: %w", err)
				}

				// Make the file executable
				err = os.Chmod(targetPath, 0755)
				if err != nil {
					return fmt.Errorf("error setting file permissions: %w", err)
				}

				// Validate the downloaded binary
				if err := downloadTask.Run(ctx, spinners.TaskSpec{Running: "Validating downloaded saltbox.fact"}, func(context.Context, *spinners.Task) error {
					return validateBinary(targetPath, expectedSize, verbose)
				}); err != nil {
					// Clean up the invalid file
					if removeErr := os.Remove(targetPath); removeErr != nil {
						return fmt.Errorf("downloaded binary validation failed (%w) and cleanup failed (%v)", err, removeErr)
					}
					return fmt.Errorf("downloaded binary validation failed: %w", err)
				}

				return nil
			}, 3, 2*time.Second) // 3 retries with 2-second base delay for downloads
		}); err != nil {
			return err
		}

	}

	return nil
}
