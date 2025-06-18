package motd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/config"
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
func GetSabnzbdInfo() string {
	configPath := "/srv/git/saltbox/motd.yml"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if Verbose {
			fmt.Printf("DEBUG: Config file %s does not exist\n", configPath)
		}
		return ""
	}

	if Verbose {
		fmt.Printf("DEBUG: Loading cfg from %s for SABnzbd\n", configPath)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if Verbose {
			fmt.Printf("DEBUG: Error loading cfg: %v\n", err)
		}
		return ""
	}

	sabnzbdInstances := cfg.Sabnzbd
	if len(sabnzbdInstances) == 0 {
		return ""
	}

	var queueInfos []SabnzbdInfo
	for _, instance := range sabnzbdInstances {
		if instance.URL == "" || instance.APIKey == "" {
			continue
		}

		info, err := getSabnzbdQueueInfo(instance)
		if err != nil {
			if Verbose {
				fmt.Printf("DEBUG: Error getting SABnzbd info for %s, hiding entry: %v\n", instance.Name, err)
			}
			continue // Skip this instance on error
		}
		queueInfos = append(queueInfos, info)
	}

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
	defer resp.Body.Close()

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
		appNameColored := GreenStyle.Render(paddedName)

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
