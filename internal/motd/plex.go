package motd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/saltyorg/sb-go/internal/config"
	"github.com/saltyorg/sb-go/internal/constants"
)

// PlexStreamInfo contains information about Plex streams
type PlexStreamInfo struct {
	Name          string
	ActiveStreams int
	DirectPlay    int
	Transcode     int
	DirectStream  int
	OtherStream   int
	Error         error
}

// PlexSessions represents the response from the Plex sessions endpoint
type PlexSessions struct {
	MediaContainer struct {
		Size    int           `json:"size"`
		Streams []PlexSession `json:"Metadata"`
	} `json:"MediaContainer"`
}

// PlexSession represents a single Plex streaming session
type PlexSession struct {
	Session struct {
		ID string `json:"id"`
	} `json:"Session"`
	Media []struct {
		VideoCodec      string `json:"videoCodec"`
		AudioCodec      string `json:"audioCodec"`
		Container       string `json:"container"`
		AudioChannels   int    `json:"audioChannels"`
		AudioProfile    string `json:"audioProfile"`
		VideoResolution string `json:"videoResolution"`
		VideoProfile    string `json:"videoProfile"`
		Selected        bool   `json:"selected"`
		Decision        string `json:"decision"`
		Part            []struct {
			Decision string `json:"decision"`
		} `json:"Part"`
	} `json:"Media"`
	TranscodeSession struct {
		VideoDecision string `json:"videoDecision"`
		AudioDecision string `json:"audioDecision"`
	} `json:"TranscodeSession"`
	User struct {
		Title string `json:"title"`
	} `json:"User"`
	Player struct {
		State     string `json:"state"`
		Product   string `json:"product"`
		Platform  string `json:"platform"`
		LocalAddr string `json:"localAddress"`
	} `json:"Player"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

// GetPlexInfo fetches and formats Plex streaming information
func GetPlexInfo(verbose bool) string {
	// Check if the configuration file exists
	configPath := constants.SaltboxMOTDConfigPath
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if verbose {
			fmt.Printf("DEBUG: Config file %s does not exist\n", configPath)
		}
		// If config does not exist, return empty string so this section won't be displayed
		return ""
	}

	if verbose {
		fmt.Printf("DEBUG: Loading config from %s for Plex\n", configPath)
	}

	// Load the configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if verbose {
			fmt.Printf("DEBUG: Error loading config: %v\n", err)
		}
		// If there's an error loading the config, return an empty string to skip this section
		return ""
	}

	// Check if a Plex section exists in the config
	plexInstances := cfg.Plex
	if len(plexInstances) == 0 {
		return ""
	}

	if verbose {
		fmt.Printf("DEBUG: Found %d Plex instance(s) in config\n", len(plexInstances))
	}

	// Create channels and wait group for async processing
	var wg sync.WaitGroup
	var mu sync.Mutex
	var streamInfos []PlexStreamInfo

	// Process each Plex instance concurrently
	for i, instance := range plexInstances {
		if !instance.IsEnabled() {
			if verbose {
				fmt.Printf("DEBUG: Skipping Plex instance %s because it is disabled\n", instance.Name)
			}
			continue
		}
		if instance.URL == "" || instance.Token == "" {
			if verbose {
				fmt.Printf("DEBUG: Skipping Plex instance %s due to missing URL or token\n", instance.Name)
			}
			continue
		}

		wg.Add(1)
		go func(idx int, inst config.PlexInstance) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					if verbose {
						fmt.Fprintf(os.Stderr, "PANIC in Plex stream info fetch (instance %d): %v\n", idx, r)
					}
				}
			}()

			if verbose {
				fmt.Printf("DEBUG: Processing Plex instance %d: %s, URL: %s\n", idx, inst.Name, inst.URL)
			}

			info, err := getPlexStreamInfo(inst)
			if err != nil {
				if verbose {
					fmt.Printf("DEBUG: Error getting Plex stream info for %s, hiding entry: %v\n", inst.Name, err)
				}
				return
			}

			if verbose {
				fmt.Printf("DEBUG: Successfully retrieved Plex stream info for instance %d: %d active streams\n", idx, info.ActiveStreams)
			}

			mu.Lock()
			streamInfos = append(streamInfos, info)
			mu.Unlock()
		}(i, instance)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	if len(streamInfos) == 0 {
		if verbose {
			fmt.Println("DEBUG: No valid Plex information found")
		}
		return ""
	}

	return formatPlexOutput(streamInfos)
}

// getPlexStreamInfo fetches streaming information from a single Plex server
func getPlexStreamInfo(instance config.PlexInstance) (PlexStreamInfo, error) {
	result := PlexStreamInfo{
		Name: instance.Name,
	}

	if instance.Name == "" {
		result.Name = "Plex"
	}

	// Ensure the URL ends with a slash
	serverURL := instance.URL
	if !strings.HasSuffix(serverURL, "/") {
		serverURL += "/"
	}

	// Build the sessions endpoint URL with the proper path joining
	baseURL, err := url.Parse(serverURL)
	if err != nil {
		return result, fmt.Errorf("invalid URL: %w", err)
	}

	sessionsPath, err := url.Parse("status/sessions")
	if err != nil {
		return result, fmt.Errorf("invalid path: %w", err)
	}

	sessionURL := baseURL.ResolveReference(sessionsPath).String()

	// Set timeout, defaulting to 1 second
	timeout := 1 * time.Second
	if instance.Timeout > 0 {
		timeout = time.Duration(instance.Timeout) * time.Second
	}

	// Create HTTP client with custom timeout
	client := &http.Client{
		Timeout: timeout,
	}

	// Create request
	req, err := http.NewRequest("GET", sessionURL, nil)
	if err != nil {
		return result, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Add("X-Plex-Token", instance.Token)
	req.Header.Add("Accept", "application/json")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return result, fmt.Errorf("failed to connect to Plex: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("plex API returned status code %d", resp.StatusCode)
	}

	// Read and parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, fmt.Errorf("failed to read response: %w", err)
	}

	var sessions PlexSessions
	if err := json.Unmarshal(body, &sessions); err != nil {
		return result, fmt.Errorf("failed to parse Plex response: %w", err)
	}

	// Count total active streams
	result.ActiveStreams = sessions.MediaContainer.Size

	// Count stream types
	for _, stream := range sessions.MediaContainer.Streams {
		// Check TranscodeSession or Media.decision for the type of playback
		if stream.TranscodeSession.VideoDecision == "transcode" || stream.TranscodeSession.AudioDecision == "transcode" {
			result.Transcode++
		} else if len(stream.Media) > 0 {
			// Check media's decision - could be directplay or directstream
			if len(stream.Media[0].Part) > 0 && stream.Media[0].Part[0].Decision == "directplay" {
				result.DirectPlay++
			} else if stream.Media[0].Decision == "directplay" {
				result.DirectPlay++
			} else if len(stream.Media[0].Part) > 0 && stream.Media[0].Part[0].Decision == "directstream" {
				result.DirectStream++
			} else if stream.Media[0].Decision == "directstream" {
				result.DirectStream++
			} else {
				// Some other type of stream or not specified
				result.OtherStream++
			}
		} else {
			// Can't determine stream type
			result.OtherStream++
		}
	}

	return result, nil
}

// buildPlexStreamTypeBreakdown creates the stream type breakdown string
func buildPlexStreamTypeBreakdown(info PlexStreamInfo) []string {
	var streamTypes []string

	if info.DirectPlay > 0 {
		count := ValueStyle.Render(fmt.Sprintf("%d", info.DirectPlay))
		streamTypes = append(streamTypes, fmt.Sprintf("%s direct play", count))
	}

	if info.Transcode > 0 {
		count := ValueStyle.Render(fmt.Sprintf("%d", info.Transcode))
		streamTypes = append(streamTypes, fmt.Sprintf("%s transcode", count))
	}

	if info.DirectStream > 0 {
		count := ValueStyle.Render(fmt.Sprintf("%d", info.DirectStream))
		streamTypes = append(streamTypes, fmt.Sprintf("%s direct stream", count))
	}

	if info.OtherStream > 0 {
		count := ValueStyle.Render(fmt.Sprintf("%d", info.OtherStream))
		streamTypes = append(streamTypes, fmt.Sprintf("%s other", count))
	}

	return streamTypes
}

// formatPlexStreamSummary formats a single Plex stream info into a summary string
func formatPlexStreamSummary(info PlexStreamInfo) string {
	if info.ActiveStreams == 0 {
		return "No active streams"
	}

	streamOrStreams := "stream"
	if info.ActiveStreams != 1 {
		streamOrStreams = "streams"
	}

	streamTypes := buildPlexStreamTypeBreakdown(info)
	activeCount := ValueStyle.Render(fmt.Sprintf("%d", info.ActiveStreams))
	summary := fmt.Sprintf("%s active %s", activeCount, streamOrStreams)

	if len(streamTypes) > 0 {
		summary += fmt.Sprintf(" (%s)", strings.Join(streamTypes, ", "))
	}

	return summary
}

// formatPlexOutput formats the Plex streaming information for display
func formatPlexOutput(infos []PlexStreamInfo) string {
	var output strings.Builder

	// If there's only one Plex instance, we can omit the name for cleaner output
	if len(infos) == 1 {
		output.WriteString(formatPlexStreamSummary(infos[0]))
		return output.String()
	}

	// Multiple Plex instances - show names for each
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
		appNameColored := SuccessStyle.Render(paddedName)

		summary := formatPlexStreamSummary(info)
		output.WriteString(fmt.Sprintf("%s%s", appNameColored, summary))
	}

	return output.String()
}
