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

// EmbyStreamInfo contains detailed information about Emby streams
type EmbyStreamInfo struct {
	Name          string
	ActiveStreams int
	DirectPlay    int
	DirectStream  int
	Transcode     int
}

// EmbySessionInfo represents a single session from the Emby API
type EmbySessionInfo struct {
	PlayState struct {
		PlayMethod string `json:"PlayMethod"`
		IsPaused   bool   `json:"IsPaused"`
	} `json:"PlayState"`
	NowPlayingItem  map[string]any `json:"NowPlayingItem,omitempty"`
	TranscodingInfo struct {
		IsVideoDirect bool `json:"IsVideoDirect"`
		IsAudioDirect bool `json:"IsAudioDirect"`
	} `json:"TranscodingInfo"`
	ID string `json:"Id"`
}

// GetEmbyInfo fetches and formats Emby streaming information
func GetEmbyInfo(verbose bool) string {
	configPath := constants.SaltboxMOTDConfigPath
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if verbose {
			fmt.Printf("DEBUG: Config file %s does not exist\n", configPath)
		}
		return ""
	}

	if verbose {
		fmt.Printf("DEBUG: Loading cfg from %s for Emby\n", configPath)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if verbose {
			fmt.Printf("DEBUG: Error loading cfg: %v\n", err)
		}
		return ""
	}

	embyInstances := cfg.Emby
	if len(embyInstances) == 0 {
		return ""
	}

	// Create a wait group and mutex for async processing
	var wg sync.WaitGroup
	var mu sync.Mutex
	var streamInfos []EmbyStreamInfo

	// Process each Emby instance concurrently
	for i, instance := range embyInstances {
		if !instance.IsEnabled() {
			if verbose {
				fmt.Printf("DEBUG: Skipping Emby instance %d because it is disabled\n", i)
			}
			continue
		}
		if instance.URL == "" || instance.Token == "" {
			if verbose {
				fmt.Printf("DEBUG: Skipping Emby instance %d due to missing URL or token\n", i)
			}
			continue
		}

		wg.Add(1)
		go func(idx int, inst config.EmbyInstance) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					if verbose {
						fmt.Fprintf(os.Stderr, "PANIC in Emby stream info fetch (instance %d): %v\n", idx, r)
					}
				}
			}()

			if verbose {
				fmt.Printf("DEBUG: Processing Emby instance %d: %s, URL: %s\n", idx, inst.Name, inst.URL)
			}

			info, err := getEmbyStreamInfo(inst)
			if err != nil {
				if verbose {
					fmt.Printf("DEBUG: Error getting Emby stream info for %s, hiding entry: %v\n", inst.Name, err)
				}
				return
			}

			if verbose {
				fmt.Printf("DEBUG: Successfully retrieved Emby stream info for instance %d: %d active streams\n", idx, info.ActiveStreams)
			}

			mu.Lock()
			streamInfos = append(streamInfos, info)
			mu.Unlock()
		}(i, instance)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	if len(streamInfos) == 0 {
		return ""
	}

	return formatEmbyOutput(streamInfos)
}

// getEmbyStreamInfo fetches streaming information from a single Emby server
func getEmbyStreamInfo(instance config.EmbyInstance) (EmbyStreamInfo, error) {
	result := EmbyStreamInfo{
		Name: instance.Name,
	}
	if result.Name == "" {
		result.Name = "Emby"
	}

	timeout := 1 * time.Second
	if instance.Timeout > 0 {
		timeout = time.Duration(instance.Timeout) * time.Second
	}

	client := &http.Client{Timeout: timeout}
	url := fmt.Sprintf("%s/emby/Sessions?api_key=%s", strings.TrimSuffix(instance.URL, "/"), instance.Token)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return result, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return result, fmt.Errorf("failed to connect to Emby: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("emby API returned status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, fmt.Errorf("failed to read response body: %w", err)
	}

	var sessions []EmbySessionInfo
	if err := json.Unmarshal(body, &sessions); err != nil {
		return result, fmt.Errorf("failed to parse Emby response: %w", err)
	}

	for _, session := range sessions {
		// Only count sessions that are actively playing (not paused and have a playing item)
		if session.NowPlayingItem == nil || session.PlayState.IsPaused {
			continue
		}

		result.ActiveStreams++

		switch strings.ToLower(session.PlayState.PlayMethod) {
		case "directplay":
			result.DirectPlay++
		case "directstream":
			result.DirectStream++
		case "transcode":
			result.Transcode++
		}
	}

	return result, nil
}

// formatEmbyOutput formats the Emby streaming information for display
func formatEmbyOutput(infos []EmbyStreamInfo) string {
	var output strings.Builder

	if len(infos) == 1 {
		info := infos[0]
		if info.ActiveStreams == 0 {
			return "No active streams"
		}
		return formatStreamSummary(info)
	}

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

		if info.ActiveStreams == 0 {
			output.WriteString(fmt.Sprintf("%sNo active streams", appNameColored))
			continue
		}
		output.WriteString(fmt.Sprintf("%s%s", appNameColored, formatStreamSummary(info)))
	}

	return output.String()
}

// formatStreamSummary is a helper to format the stream count details
func formatStreamSummary(info EmbyStreamInfo) string {
	streamOrStreams := "stream"
	if info.ActiveStreams != 1 {
		streamOrStreams = "streams"
	}

	var streamTypes []string
	if info.DirectPlay > 0 {
		streamTypes = append(streamTypes, fmt.Sprintf("%s direct play", ValueStyle.Render(fmt.Sprintf("%d", info.DirectPlay))))
	}
	if info.DirectStream > 0 {
		streamTypes = append(streamTypes, fmt.Sprintf("%s direct stream", ValueStyle.Render(fmt.Sprintf("%d", info.DirectStream))))
	}
	if info.Transcode > 0 {
		streamTypes = append(streamTypes, fmt.Sprintf("%s transcode", ValueStyle.Render(fmt.Sprintf("%d", info.Transcode))))
	}

	summary := fmt.Sprintf("%s active %s", ValueStyle.Render(fmt.Sprintf("%d", info.ActiveStreams)), streamOrStreams)
	if len(streamTypes) > 0 {
		summary += fmt.Sprintf(" (%s)", strings.Join(streamTypes, ", "))
	}

	return summary
}
