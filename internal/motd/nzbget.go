package motd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/saltyorg/sb-go/internal/config"
	"github.com/saltyorg/sb-go/internal/constants"
)

// NzbgetInfo holds processed information for an NZBGet instance
type NzbgetInfo struct {
	Name          string
	IsPaused      bool
	DownloadSpeed float64 // in B/s
	QueueCount    int
	RemainingSize int64 // in Bytes
	TotalSize     int64 // in Bytes
	Error         error
}

// jsonRPCRequest defines the structure for a JSON-RPC request
type jsonRPCRequest struct {
	Method string `json:"method"`
	Params []any  `json:"params"`
	ID     int    `json:"id"`
}

// statusResponse defines the structure for the result of the "status" method
type statusResponse struct {
	Result struct {
		ServerPaused bool    `json:"ServerPaused"`
		DownloadRate float64 `json:"DownloadRate"`
	} `json:"result"`
}

// listGroupsResponse defines the structure for the result of the "listgroups" method
type listGroupsResponse struct {
	Result []struct {
		RemainingSize int64  `json:"RemainingSizeMB"` // This is in MB
		FileSize      int64  `json:"FileSizeMB"`      // This is in MB
		Status        string `json:"Status"`
	} `json:"result"`
}

// GetNzbgetInfo fetches and formats NZBGet queue information
func GetNzbgetInfo(ctx context.Context, verbose bool) string {
	configPath := constants.SaltboxMOTDConfigPath
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if verbose {
			fmt.Printf("DEBUG: Config file %s does not exist\n", configPath)
		}
		return ""
	}

	if verbose {
		fmt.Printf("DEBUG: Loading cfg from %s for NZBGet\n", configPath)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if verbose {
			fmt.Printf("DEBUG: Error loading cfg: %v\n", err)
		}
		return ""
	}

	// Check if NZBGet section exists and is enabled
	if cfg.Nzbget == nil || !cfg.Nzbget.IsEnabled() || len(cfg.Nzbget.Instances) == 0 {
		return ""
	}

	nzbgetInstances := cfg.Nzbget.Instances

	// Create a wait group and mutex for async processing
	var wg sync.WaitGroup
	var mu sync.Mutex
	var queueInfos []NzbgetInfo

	// Process each NZBGet instance concurrently
	for i, instance := range nzbgetInstances {
		if !instance.IsEnabled() {
			if verbose {
				fmt.Printf("DEBUG: Skipping NZBGet instance %d because it is disabled\n", i)
			}
			continue
		}
		if instance.URL == "" || instance.User == "" || instance.Password == "" {
			if verbose {
				fmt.Printf("DEBUG: Skipping NZBGet instance %d due to missing URL, user, or password\n", i)
			}
			continue
		}

		wg.Add(1)
		go func(idx int, inst config.UserPassAppInstance) {
			defer wg.Done()
			instanceName := providerInstanceName(inst.Name, "NZBGet")
			defer func() {
				if r := recover(); r != nil {
					if verbose {
						fmt.Fprintf(os.Stderr, "PANIC in NZBGet queue info fetch (instance %d): %v\n", idx, r)
					}
					mu.Lock()
					queueInfos = append(queueInfos, NzbgetInfo{Name: instanceName, Error: fmt.Errorf("panic: %v", r)})
					mu.Unlock()
				}
			}()

			if verbose {
				fmt.Printf("DEBUG: Processing NZBGet instance %d: %s, URL: %s\n", idx, inst.Name, inst.URL)
			}

			info, err := getNzbgetQueueInfo(ctx, inst)
			if err != nil {
				if verbose {
					fmt.Printf("DEBUG: Error getting NZBGet info for %s, recording error: %v\n", inst.Name, err)
				}
				mu.Lock()
				queueInfos = append(queueInfos, NzbgetInfo{Name: instanceName, Error: err})
				mu.Unlock()
				return
			}

			if verbose {
				fmt.Printf("DEBUG: Successfully retrieved NZBGet info for instance %d: %d items in queue\n", idx, info.QueueCount)
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

	return formatNzbgetOutput(queueInfos)
}

// callNzbgetAPI is a helper to perform JSON-RPC calls
func callNzbgetAPI(ctx context.Context, instance config.UserPassAppInstance, method string, target any) error {
	timeout := 1 * time.Second
	if instance.Timeout > 0 {
		timeout = time.Duration(instance.Timeout) * time.Second
	}

	client := &http.Client{Timeout: timeout}

	parsedURL, err := url.Parse(strings.TrimSuffix(instance.URL, "/"))
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	parsedURL.User = url.UserPassword(instance.User, instance.Password)
	apiURL := fmt.Sprintf("%s/jsonrpc", parsedURL.String())

	jsonReq, err := json.Marshal(jsonRPCRequest{Method: method, Params: []any{}, ID: 1})
	if err != nil {
		return fmt.Errorf("failed to create JSON request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonReq))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to NZBGet: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("NZBGet API returned status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	return json.Unmarshal(body, target)
}

// getNzbgetQueueInfo fetches queue information from a single NZBGet server
func getNzbgetQueueInfo(ctx context.Context, instance config.UserPassAppInstance) (NzbgetInfo, error) {
	result := NzbgetInfo{Name: instance.Name}
	if result.Name == "" {
		result.Name = "NZBGet"
	}

	// Get status
	var status statusResponse
	if err := callNzbgetAPI(ctx, instance, "status", &status); err != nil {
		return result, err
	}
	result.IsPaused = status.Result.ServerPaused
	result.DownloadSpeed = status.Result.DownloadRate

	// Get queue details
	var queue listGroupsResponse
	if err := callNzbgetAPI(ctx, instance, "listgroups", &queue); err != nil {
		return result, err
	}

	result.QueueCount = len(queue.Result)
	var totalSize int64
	var remainingSize int64
	pausedCount := 0

	for _, item := range queue.Result {
		totalSize += item.FileSize
		remainingSize += item.RemainingSize
		if item.Status == "PAUSED" {
			pausedCount++
		}
	}

	// If the server isn't paused but all items in the queue are, consider it paused.
	if !result.IsPaused && result.QueueCount > 0 && pausedCount == result.QueueCount {
		result.IsPaused = true
	}

	result.TotalSize = totalSize * 1024 * 1024         // Convert MB to Bytes
	result.RemainingSize = remainingSize * 1024 * 1024 // Convert MB to Bytes

	return result, nil
}

// formatNzbgetOutput formats the NZBGet information for display
func formatNzbgetOutput(infos []NzbgetInfo) string {
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})

	if len(infos) == 1 {
		return formatNzbgetSummary(infos[0])
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

		summary := formatNzbgetSummary(info)
		output.WriteString(fmt.Sprintf("%s%s", appNameColored, summary))
	}

	return output.String()
}

// formatNzbgetSummary is a helper to format the summary for a single instance
func formatNzbgetSummary(info NzbgetInfo) string {
	if info.Error != nil {
		return ErrorStyle.Render(formatProviderError(info.Error))
	}

	if info.QueueCount == 0 {
		return "Queue is empty"
	}

	count := ValueStyle.Render(fmt.Sprintf("%d", info.QueueCount))
	size := ValueStyle.Render(formatBytes(info.TotalSize))
	sizeLeft := ValueStyle.Render(formatBytes(info.RemainingSize))
	itemOrItems := "item"
	if info.QueueCount != 1 {
		itemOrItems = "items"
	}

	queueSummary := fmt.Sprintf("%s %s in queue (%s remaining / %s total)", count, itemOrItems, sizeLeft, size)

	if info.IsPaused {
		return fmt.Sprintf("Paused, %s", queueSummary)
	}

	speed := ValueStyle.Render(fmt.Sprintf("%s/s", formatBytes(int64(info.DownloadSpeed))))
	return fmt.Sprintf("Downloading at %s, %s", speed, queueSummary)
}
