package saltbox

import (
	"context"
	"debug/elf"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/executor"
	"github.com/saltyorg/sb-go/layout"
	"github.com/saltyorg/sb-go/release"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/Masterminds/semver/v3"
)

// retryWithBackoff executes a function with exponential backoff retry logic
func retryWithBackoff(ctx context.Context, operation func() error, maxRetries int, baseDelay time.Duration) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate delay with exponential backoff (2^attempt * baseDelay)
			delay := min(time.Duration(1<<uint(attempt-1))*baseDelay,
				// Cap maximum delay at 30 seconds
				30*time.Second)
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
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

	binary, err := elf.Open(filePath)
	if err != nil {
		return fmt.Errorf("file is not a valid ELF binary: %w", err)
	}
	defer func() { _ = binary.Close() }()
	if binary.Class != elf.ELFCLASS64 || binary.Machine != elf.EM_X86_64 {
		return fmt.Errorf("file is not an x86-64 ELF binary")
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
func checkIfUpdateNeeded(ctx context.Context, task *terminal.Task, targetPath, latestVersion string, alwaysUpdate bool) (bool, string, error) {
	if alwaysUpdate {
		task.Info("Reinstall forced.")
		return true, "", nil
	}

	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		task.Info("saltbox.fact not found. Proceeding with update.")
		return true, "", nil
	} else if err != nil {
		return false, "", fmt.Errorf("error checking for existing saltbox.fact: %w", err)
	}

	currentVersion, err := getCurrentFactVersion(ctx, targetPath)
	if err != nil {
		task.Warning(fmt.Sprintf("%v. Proceeding with update.", err))
		return true, "", nil
	}

	currentSemVer, err := semver.NewVersion(strings.TrimPrefix(currentVersion, "v"))
	if err != nil {
		task.Warning(fmt.Sprintf("Failed to parse current version: %v. Updating...", err))
		return true, currentVersion, nil
	}

	latestSemVer, err := semver.NewVersion(strings.TrimPrefix(latestVersion, "v"))
	if err != nil {
		task.Warning(fmt.Sprintf("Failed to parse latest version: %v. Updating...", err))
		return true, currentVersion, nil
	}

	if currentSemVer.Compare(latestSemVer) >= 0 {
		task.Info(fmt.Sprintf("saltbox.fact is up to date (version %s)", currentVersion))
		return false, currentVersion, nil
	}

	return true, currentVersion, nil
}

type latestReleaseInfo struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		ID                 int64  `json:"id"`
		Name               string `json:"name"`
		Size               int64  `json:"size"`
		Digest             string `json:"digest"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// fetchLatestReleaseInfoFromURL fetches the latest release metadata from a single URL.
func fetchLatestReleaseInfoFromURL(ctx context.Context, client *http.Client, apiURL string) (string, release.Asset, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", release.Asset{}, fmt.Errorf("error creating latest release request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", release.Asset{}, fmt.Errorf("error fetching latest release info: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != http.StatusOK {
		return "", release.Asset{}, release.HTTPStatus(response.StatusCode)
	}

	var latestRelease latestReleaseInfo
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&latestRelease); err != nil {
		return "", release.Asset{}, release.InvalidResponse("returned invalid JSON", err)
	}
	if strings.TrimSpace(latestRelease.TagName) == "" {
		return "", release.Asset{}, release.InvalidResponse("response is missing tag_name", nil)
	}

	// Find the saltbox-facts asset and get its size.
	for _, asset := range latestRelease.Assets {
		if asset.Name == "saltbox-facts" {
			validated, err := release.NewAsset(asset.ID, asset.Name, asset.BrowserDownloadURL, asset.Size, asset.Digest)
			if err != nil {
				return "", release.Asset{}, release.InvalidResponse(err.Error(), err)
			}
			return latestRelease.TagName, validated, nil
		}
	}

	return "", release.Asset{}, release.InvalidResponse("response is missing the saltbox-facts asset", nil)
}

// fetchLatestReleaseInfo fetches latest release info through SVM first, then falls back to direct GitHub API.
func fetchLatestReleaseInfo(ctx context.Context, task *terminal.Task, proxyURL, githubURL string, verbose bool) (string, release.Asset, error) {
	var latestVersion string
	var selectedAsset release.Asset
	var fallbackNotified bool

	err := task.Run(ctx, terminal.TaskSpec{Running: "Fetching latest saltbox.fact release info"}, func(taskCtx context.Context, _ *terminal.Task) error {
		return retryWithBackoff(taskCtx, func() error {
			client := release.HTTPClient(30 * time.Second)

			version, asset, proxyErr := fetchLatestReleaseInfoFromURL(taskCtx, client, proxyURL)
			if proxyErr == nil {
				latestVersion = version
				selectedAsset = asset
				return nil
			}

			if !fallbackNotified {
				if verbose {
					fmt.Printf("SVM proxy unavailable or unusable (%v); falling back to direct GitHub API\n", proxyErr)
				} else {
					task.Warning(fmt.Sprintf("SVM proxy %s; trying GitHub directly", release.Describe(proxyErr)))
				}
				fallbackNotified = true
			}

			version, asset, githubErr := fetchLatestReleaseInfoFromURL(taskCtx, client, githubURL)
			if githubErr != nil {
				return fmt.Errorf("proxy request failed: %w; fallback GitHub API request failed: %w", proxyErr, githubErr)
			}

			latestVersion = version
			selectedAsset = asset
			if verbose {
				fmt.Println("Direct GitHub API fallback succeeded")
			} else {
				task.Info("GitHub fallback succeeded")
			}
			return nil
		}, 3, 1*time.Second) // 3 retries with 1-second base delay
	})

	return latestVersion, selectedAsset, err
}

// DownloadAndInstallSaltboxFact downloads and installs the latest saltbox.fact file.
func DownloadAndInstallSaltboxFact(
	ctx context.Context,
	task *terminal.Task,
	alwaysUpdate bool,
	verbose bool,
) error {
	return downloadAndInstallSaltboxFact(ctx, task, alwaysUpdate, verbose)
}

func downloadAndInstallSaltboxFact(ctx context.Context, task *terminal.Task, alwaysUpdate bool, verbose bool) error {
	targetPath := layout.SaltboxFactPath
	githubURL := "https://api.github.com/repos/saltyorg/ansible-facts/releases/latest"
	proxyURL := fmt.Sprintf("%s?url=%s", layout.SVMVersionProxyURL, githubURL)

	// Fetch the latest release info from GitHub with retry logic
	latestVersion, asset, err := fetchLatestReleaseInfo(ctx, task, proxyURL, githubURL, verbose)
	if err != nil {
		return err
	}

	// Check if we need to update
	needsUpdate, currentVersion, err := checkIfUpdateNeeded(ctx, task, targetPath, latestVersion, alwaysUpdate)
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

		if err := task.Run(ctx, terminal.TaskSpec{Running: taskMessage}, func(ctx context.Context, downloadTask *terminal.Task) error {
			return retryWithBackoff(ctx, func() error {
				staged, err := release.DownloadVerified(ctx, release.HTTPClient(30*time.Second), asset, 32<<20, filepath.Dir(targetPath), ".saltbox.fact-*")
				if err != nil {
					return fmt.Errorf("error downloading saltbox.fact: %w", err)
				}
				defer func() { _ = os.Remove(staged) }()
				if err := downloadTask.Run(ctx, terminal.TaskSpec{Running: "Validating downloaded saltbox.fact"}, func(context.Context, *terminal.Task) error {
					return validateBinary(staged, asset.Size, verbose)
				}); err != nil {
					return fmt.Errorf("downloaded binary validation failed: %w", err)
				}
				return release.Activate(staged, targetPath, 0755)
			}, 3, 2*time.Second)
		}); err != nil {
			return err
		}

		if alwaysUpdate {
			task.Info(fmt.Sprintf("saltbox.fact reinstalled successfully (version %s)", latestVersion))
		} else if currentVersion != "" {
			task.Info(fmt.Sprintf("saltbox.fact updated successfully: %s → %s", currentVersion, latestVersion))
		} else {
			task.Info(fmt.Sprintf("saltbox.fact installed successfully (version %s)", latestVersion))
		}
	}

	return nil
}
