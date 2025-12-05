package motd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/saltyorg/sb-go/internal/config"
	"github.com/saltyorg/sb-go/internal/constants"
)

// SabnzbdInfo holds the processed information for an SABnzbd instance
type SabnzbdInfo struct {
	Name       string
	Status     string
	Speed      string
	QueueCount int
	QueueSize  string
	QueueLeft  string
	Error      error
}

// SabnzbdAPIResponse is the top-level structure of the SABnzbd API response
type SabnzbdAPIResponse struct {
	Queue SabnzbdQueue `json:"queue"`
}

// SabnzbdQueue contains the details of the download queue
type SabnzbdQueue struct {
	Status         string `json:"status"`
	Speed          string `json:"speed"`
	Size           string `json:"size"`
	SizeLeft       string `json:"sizeleft"`
	NoOfSlotsTotal int    `json:"noofslots_total"`
}

// GetSabnzbdInfo fetches and formats SABnzbd queue information
func GetSabnzbdInfo(verbose bool) string {
	configPath := constants.SaltboxMOTDConfigPath
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if verbose {
			fmt.Printf("DEBUG: Config file %s does not exist\n", configPath)
		}
		return ""
	}

	if verbose {
		fmt.Printf("DEBUG: Loading cfg from %s for SABnzbd\n", configPath)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if verbose {
			fmt.Printf("DEBUG: Error loading cfg: %v\n", err)
		}
		return ""
	}

	// Check if SABnzbd section exists and is enabled
	if cfg.Sabnzbd == nil || !cfg.Sabnzbd.IsEnabled() || len(cfg.Sabnzbd.Instances) == 0 {
		return ""
	}

	sabnzbdInstances := cfg.Sabnzbd.Instances

	// Create a wait group and mutex for async processing
	var wg sync.WaitGroup
	var mu sync.Mutex
	var queueInfos []SabnzbdInfo

	// Process each SABnzbd instance concurrently
	for i, instance := range sabnzbdInstances {
		if !instance.IsEnabled() {
			if verbose {
				fmt.Printf("DEBUG: Skipping SABnzbd instance %d because it is disabled\n", i)
			}
			continue
		}
		if instance.URL == "" || instance.APIKey == "" {
			if verbose {
				fmt.Printf("DEBUG: Skipping SABnzbd instance %d due to missing URL or API key\n", i)
			}
			continue
		}

		wg.Add(1)
		go func(idx int, inst config.AppInstance) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					if verbose {
						fmt.Fprintf(os.Stderr, "PANIC in SABnzbd queue info fetch (instance %d): %v\n", idx, r)
					}
				}
			}()

			if verbose {
				fmt.Printf("DEBUG: Processing SABnzbd instance %d: %s, URL: %s\n", idx, inst.Name, inst.URL)
			}

			info, err := getSabnzbdQueueInfo(inst)
			if err != nil {
				if verbose {
					fmt.Printf("DEBUG: Error getting SABnzbd info for %s, hiding entry: %v\n", inst.Name, err)
				}
				return
			}

			if verbose {
				fmt.Printf("DEBUG: Successfully retrieved SABnzbd info for instance %d: %d items in queue\n", idx, info.QueueCount)
			}

			mu.Lock()
			queueInfos = append(queueInfos, info)
			mu.Unlock()
		}(i, instance)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	if len(queueInfos) == 0 {
		return ""
	}

	return formatSabnzbdOutput(queueInfos)
}

// getSabnzbdQueueInfo fetches queue information from a single SABnzbd server
func getSabnzbdQueueInfo(instance config.AppInstance) (SabnzbdInfo, error) {
	result := SabnzbdInfo{
		Name: instance.Name,
	}
	if result.Name == "" {
		result.Name = "SABnzbd"
	}

	timeout := 1 * time.Second
	if instance.Timeout > 0 {
		timeout = time.Duration(instance.Timeout) * time.Second
	}

	client := &http.Client{Timeout: timeout}
	url := fmt.Sprintf("%s/api?mode=queue&output=json&apikey=%s", strings.TrimSuffix(instance.URL, "/"), instance.APIKey)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return result, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return result, fmt.Errorf("failed to connect to SABnzbd: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("SABnzbd API returned status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, fmt.Errorf("failed to read response body: %w", err)
	}

	var apiResponse SabnzbdAPIResponse
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return result, fmt.Errorf("failed to parse SABnzbd response: %w", err)
	}

	result.Status = apiResponse.Queue.Status
	result.Speed = apiResponse.Queue.Speed
	result.QueueCount = apiResponse.Queue.NoOfSlotsTotal
	result.QueueSize = apiResponse.Queue.Size
	result.QueueLeft = apiResponse.Queue.SizeLeft

	return result, nil
}

// formatSabnzbdOutput formats the SABnzbd information for display
func formatSabnzbdOutput(infos []SabnzbdInfo) string {
	if len(infos) == 1 {
		return formatSabnzbdSummary(infos[0])
	}

	var output strings.Builder
	maxNameLen := 0
	for _, info := range infos {
		if len(info.Name) > maxNameLen {
			maxNameLen = len(info.Name)
		}
	}

	for i, info := range infos {
		if i > 0 {
			output.WriteString("\n")
		}
		namePadding := maxNameLen - len(info.Name)
		paddedName := fmt.Sprintf("%s:%s", info.Name, strings.Repeat(" ", namePadding+1))
		appNameColored := AppNameStyle.Render(paddedName)

		summary := formatSabnzbdSummary(info)
		output.WriteString(fmt.Sprintf("%s%s", appNameColored, summary))
	}

	return output.String()
}

// formatSabnzbdSummary is a helper to format the summary for a single instance
func formatSabnzbdSummary(info SabnzbdInfo) string {
	if info.QueueCount == 0 {
		return "Queue is empty"
	}

	count := ValueStyle.Render(fmt.Sprintf("%d", info.QueueCount))
	size := ValueStyle.Render(info.QueueSize)
	sizeLeft := ValueStyle.Render(info.QueueLeft)
	itemOrItems := "item"
	if info.QueueCount != 1 {
		itemOrItems = "items"
	}

	queueSummary := fmt.Sprintf("%s %s in queue (%s remaining / %s total)", count, itemOrItems, sizeLeft, size)

	if strings.ToLower(info.Status) == "paused" {
		return fmt.Sprintf("Paused, %s", queueSummary)
	}

	speed := ValueStyle.Render(fmt.Sprintf("%s/s", info.Speed))
	return fmt.Sprintf("Downloading at %s, %s", speed, queueSummary)
}
