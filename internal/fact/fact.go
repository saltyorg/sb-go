package fact

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/spinners"

	"github.com/Masterminds/semver/v3"
)

// retryWithBackoff executes a function with exponential backoff retry logic
func retryWithBackoff(operation func() error, maxRetries int, baseDelay time.Duration) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate delay with exponential backoff (2^attempt * baseDelay)
			delay := time.Duration(1<<uint(attempt-1)) * baseDelay
			if delay > 30*time.Second {
				delay = 30 * time.Second // Cap maximum delay at 30 seconds
			}
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
	defer file.Close()

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

// DownloadAndInstallSaltboxFact downloads and installs the latest saltbox.fact file.
func DownloadAndInstallSaltboxFact(alwaysUpdate bool, verbose bool) error {
	downloadURL := "https://github.com/saltyorg/ansible-facts/releases/latest/download/saltbox-facts"
	targetPath := "/srv/git/saltbox/ansible_facts.d/saltbox.fact"
	apiURL := "https://svm.saltbox.dev/version?url=https://api.github.com/repos/saltyorg/ansible-facts/releases/latest"

	var latestVersion string
	var expectedSize int64

	// Fetch the latest release info from GitHub with retry logic
	if err := spinners.RunTaskWithSpinner("Fetching latest saltbox.fact release info", func() error {
		return retryWithBackoff(func() error {
			response, err := http.Get(apiURL)
			if err != nil {
				return fmt.Errorf("error fetching latest release info: %w", err)
			}
			defer func() {
				if err := response.Body.Close(); err != nil {
					fmt.Println("Error closing response body:", err)
				}
			}()

			if response.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status code: %d", response.StatusCode)
			}

			var latestRelease struct {
				TagName string `json:"tag_name"`
				Assets  []struct {
					Name string `json:"name"`
					Size int64  `json:"size"`
				} `json:"assets"`
			}
			if err := json.NewDecoder(response.Body).Decode(&latestRelease); err != nil {
				return fmt.Errorf("failed to parse release info: %w", err)
			}
			latestVersion = latestRelease.TagName

			// Find the saltbox-facts asset and get its size
			for _, asset := range latestRelease.Assets {
				if asset.Name == "saltbox-facts" {
					expectedSize = asset.Size
					break
				}
			}
			if expectedSize == 0 {
				return fmt.Errorf("saltbox-facts asset not found in release")
			}

			return nil
		}, 3, 1*time.Second) // 3 retries with 1-second base delay
	}); err != nil {
		return err
	}

	// Check if we need to update
	needsUpdate := alwaysUpdate
	if !needsUpdate {
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			if err := spinners.RunInfoSpinner("saltbox.fact not found. Proceeding with update."); err != nil {
				return err
			}
			needsUpdate = true
		} else if err != nil {
			return fmt.Errorf("error checking for existing saltbox.fact: %w", err)
		} else {
			// Run the existing saltbox.fact and parse its output
			cmd := exec.Command(targetPath)
			output, err := cmd.CombinedOutput()
			if err != nil {
				if err := spinners.RunWarningSpinner("Failed to run current saltbox.fact. Proceeding with update."); err != nil {
					return err
				}
				needsUpdate = true
			} else {
				var currentData map[string]interface{}
				if err = json.Unmarshal(output, &currentData); err != nil {
					if err := spinners.RunWarningSpinner("Failed to parse current saltbox.fact output. Proceeding with update."); err != nil {
						return err
					}
					needsUpdate = true
				} else {
					currentVersion, ok := currentData["saltbox_facts_version"].(string)
					if !ok {
						if err := spinners.RunWarningSpinner("Current saltbox.fact doesn't have version info. Updating..."); err != nil {
							return err
						}
						needsUpdate = true
					} else {
						currentSemVer, err := semver.NewVersion(strings.TrimPrefix(currentVersion, "v"))
						if err != nil {
							if err := spinners.RunWarningSpinner(fmt.Sprintf("Failed to parse current version: %v. Updating...", err)); err != nil {
								return err
							}
							needsUpdate = true
						} else {
							latestSemVer, err := semver.NewVersion(strings.TrimPrefix(latestVersion, "v"))
							if err != nil {
								if err := spinners.RunWarningSpinner(fmt.Sprintf("Failed to parse latest version: %v. Updating...", err)); err != nil {
									return err
								}
								needsUpdate = true
							} else {
								if currentSemVer.Compare(latestSemVer) >= 0 {
									if err := spinners.RunInfoSpinner(fmt.Sprintf("saltbox.fact is up to date (version %s)", currentVersion)); err != nil {
										return err
									}
									return nil
								}
								if err := spinners.RunInfoSpinner(fmt.Sprintf("New version available. Updating from %s to %s", currentVersion, latestVersion)); err != nil {
									return err
								}
								needsUpdate = true
							}
						}
					}
				}
			}
		}
	} else {
		if err := spinners.RunInfoSpinner("Reinstall forced."); err != nil {
			return err
		}
	}

	if //goland:noinspection GoDfaConstantCondition
	needsUpdate {
		// Download and install saltbox.fact with spinner
		taskMessage := fmt.Sprintf("Updating saltbox.fact to version %s", latestVersion)
		if alwaysUpdate {
			taskMessage = fmt.Sprintf("Reinstalling saltbox.fact with version %s", latestVersion)
		}

		if err := spinners.RunTaskWithSpinner(taskMessage, func() error {
			return retryWithBackoff(func() error {
				response, err := http.Get(downloadURL)
				if err != nil {
					return fmt.Errorf("error downloading saltbox.fact: %w", err)
				}
				defer func() {
					if err := response.Body.Close(); err != nil {
						fmt.Println("Error closing response body:", err)
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
						fmt.Println("Error closing file:", err)
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
				if err := validateBinary(targetPath, expectedSize, verbose); err != nil {
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

		if err := spinners.RunInfoSpinner(fmt.Sprintf("Successfully updated saltbox.fact to version %s at %s", latestVersion, targetPath)); err != nil {
			return err
		}
	}

	return nil
}
