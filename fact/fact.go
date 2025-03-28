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

	"github.com/Masterminds/semver/v3"
	"github.com/saltyorg/sb-go/spinners"
)

// DownloadAndInstallSaltboxFact downloads and installs the latest saltbox.fact file.
func DownloadAndInstallSaltboxFact(alwaysUpdate bool) error {
	downloadURL := "https://github.com/saltyorg/ansible-facts/releases/latest/download/saltbox-facts"
	targetPath := "/srv/git/saltbox/ansible_facts.d/saltbox.fact"
	apiURL := "https://api.github.com/repos/saltyorg/ansible-facts/releases/latest"

	var latestVersion string

	// Fetch the latest release info from GitHub with spinner
	err := spinners.RunTaskWithSpinner("Fetching latest saltbox.fact release info", func() error {
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
		}
		err = json.NewDecoder(response.Body).Decode(&latestRelease)
		if err != nil {
			return fmt.Errorf("error decoding release info: %w", err)
		}
		latestVersion = latestRelease.TagName
		return nil
	})
	if err != nil {
		return err
	}

	if _, err := os.Stat(targetPath); err == nil && !alwaysUpdate {
		// Run the existing saltbox.fact and parse its output
		cmd := exec.Command(targetPath)
		output, err := cmd.CombinedOutput()
		if err == nil {
			var currentData map[string]interface{}
			err = json.Unmarshal(output, &currentData)
			if err == nil {
				currentVersion, ok := currentData["saltbox_facts_version"].(string)
				if !ok {
					if err := spinners.RunWarningSpinner("Current saltbox.fact doesn't have version info. Updating..."); err != nil {
						return err
					}
				} else {
					currentSemVer, err := semver.NewVersion(strings.TrimPrefix(currentVersion, "v"))
					if err != nil {
						if err := spinners.RunWarningSpinner(fmt.Sprintf("Failed to parse current version: %v", err)); err != nil {
							return err
						}
					} else {
						latestSemVer, err := semver.NewVersion(strings.TrimPrefix(latestVersion, "v"))
						if err != nil {
							if err := spinners.RunWarningSpinner(fmt.Sprintf("Failed to parse latest version: %v", err)); err != nil {
								return err
							}
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
						}
					}

				}
			} else {
				if err := spinners.RunWarningSpinner("Failed to run current saltbox.fact. Proceeding with update."); err != nil {
					return err
				}
			}
		} else {
			if err := spinners.RunWarningSpinner("Failed to parse current saltbox.fact output. Proceeding with update."); err != nil {
				return err
			}
		}
	} else {
		if alwaysUpdate {
			if err := spinners.RunInfoSpinner("Reinstall forced."); err != nil {
				return err
			}
		} else {
			if err := spinners.RunInfoSpinner("saltbox.fact not found. Proceeding with update."); err != nil {
				return err
			}
		}
	}

	// Download and install saltbox.fact with spinner
	taskMessage := fmt.Sprintf("Updating saltbox.fact to version %s", latestVersion)
	if alwaysUpdate {
		taskMessage = fmt.Sprintf("Reinstalling saltbox.fact with version %s", latestVersion)
	}

	err = spinners.RunTaskWithSpinner(taskMessage, func() error {
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
		return nil
	})
	if err != nil {
		return err
	}

	if err := spinners.RunInfoSpinner(fmt.Sprintf("Successfully updated saltbox.fact to version %s at %s", latestVersion, targetPath)); err != nil {
		return err
	}

	return nil
}
