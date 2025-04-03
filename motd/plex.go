package motd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// PlexInstance represents a single Plex server instance configuration
type PlexInstance struct {
	Name  string `yaml:"name"`  // Optional friendly name
	URL   string `yaml:"url"`   // Base URL for the Plex server
	Token string `yaml:"token"` // X-Plex-Token for authentication
}

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
func GetPlexInfo() string {
	// Check if the configuration file exists
	configPath := "/srv/git/saltbox/motd.yml"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if Verbose {
			fmt.Printf("DEBUG: Config file %s does not exist\n", configPath)
		}
		// If config does not exist, return empty string so this section won't be displayed
		return ""
	}

	if Verbose {
		fmt.Printf("DEBUG: Loading config from %s for Plex\n", configPath)
	}

	// Load the configuration
	config, err := LoadConfig(configPath)
	if err != nil {
		if Verbose {
			fmt.Printf("DEBUG: Error loading config: %v\n", err)
		}
		// If there's an error loading the config, return empty string to skip this section
		return ""
	}

	// Check if Plex section exists in the config
	plexInstances := config.Plex
	if len(plexInstances) == 0 {
		if Verbose {
			fmt.Printf("DEBUG: No Plex instances found in config\n")
		}
		return ""
	}

	if Verbose {
		fmt.Printf("DEBUG: Found %d Plex instance(s) in config\n", len(plexInstances))
	}

	var streamInfos []PlexStreamInfo
	// Process each Plex instance
	for _, instance := range plexInstances {
		if instance.URL == "" || instance.Token == "" {
			if Verbose {
				fmt.Printf("DEBUG: Skipping Plex instance %s due to missing URL or token\n", instance.Name)
			}
			continue
		}

		info, err := getPlexStreamInfo(instance)
		if err != nil {
			if Verbose {
				fmt.Printf("DEBUG: Error getting Plex stream info for %s: %v\n", instance.Name, err)
			}
			info.Error = err
		}

		streamInfos = append(streamInfos, info)
	}

	if len(streamInfos) == 0 {
		if Verbose {
			fmt.Println("DEBUG: No valid Plex information found")
		}
		return ""
	}

	return formatPlexOutput(streamInfos)
}

// getPlexStreamInfo fetches streaming information from a single Plex server
func getPlexStreamInfo(instance PlexInstance) (PlexStreamInfo, error) {
	result := PlexStreamInfo{
		Name: instance.Name,
	}

	if instance.Name == "" {
		result.Name = "Plex"
	}

	// Ensure URL ends with a slash
	serverURL := instance.URL
	if !strings.HasSuffix(serverURL, "/") {
		serverURL += "/"
	}

	// Build the sessions endpoint URL with proper path joining
	baseURL, err := url.Parse(serverURL)
	if err != nil {
		return result, fmt.Errorf("invalid URL: %w", err)
	}

	sessionsPath, err := url.Parse("status/sessions")
	if err != nil {
		return result, fmt.Errorf("invalid path: %w", err)
	}

	sessionURL := baseURL.ResolveReference(sessionsPath).String()

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
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
		return result, fmt.Errorf("Plex API returned status code %d", resp.StatusCode)
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

// formatPlexOutput formats the Plex streaming information for display
func formatPlexOutput(infos []PlexStreamInfo) string {
	var output strings.Builder

	// If there's only one Plex instance, we can omit the name for cleaner output
	if len(infos) == 1 {
		info := infos[0]

		// If there was an error fetching data
		if info.Error != nil {
			output.WriteString(RedStyle.Render(fmt.Sprintf("Error: %v", info.Error)))
			return output.String()
		}

		// No active streams
		if info.ActiveStreams == 0 {
			output.WriteString("No active streams")
			return output.String()
		}

		// Format stream counts
		streamOrStreams := "stream"
		if info.ActiveStreams != 1 {
			streamOrStreams = "streams"
		}

		// Build stream type breakdown
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

		// Format the summary line
		activeCount := ValueStyle.Render(fmt.Sprintf("%d", info.ActiveStreams))
		summary := fmt.Sprintf("%s active %s", activeCount, streamOrStreams)

		if len(streamTypes) > 0 {
			summary += fmt.Sprintf(" (%s)", strings.Join(streamTypes, ", "))
		}

		output.WriteString(summary)
		return output.String()
	}

	// Multiple Plex instances - show names for each
	// Find the length of the longest name
	maxNameLen := 0
	for _, info := range infos {
		if len(info.Name) > maxNameLen {
			maxNameLen = len(info.Name)
		}
	}

	for i, info := range infos {
		// Add a newline between servers
		if i > 0 {
			output.WriteString("\n")
		}

		// Align the server names
		namePadding := maxNameLen - len(info.Name)
		paddedName := fmt.Sprintf("%s:%s", info.Name, strings.Repeat(" ", namePadding+1))

		appNameColored := GreenStyle.Render(paddedName)

		// If there was an error fetching data
		if info.Error != nil {
			output.WriteString(fmt.Sprintf("%s%s", appNameColored, RedStyle.Render(fmt.Sprintf("Error: %v", info.Error))))
			continue
		}

		// No active streams
		if info.ActiveStreams == 0 {
			output.WriteString(fmt.Sprintf("%sNo active streams", appNameColored))
			continue
		}

		// Format stream counts
		streamOrStreams := "stream"
		if info.ActiveStreams != 1 {
			streamOrStreams = "streams"
		}

		// Build stream type breakdown
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

		// Format the summary line
		activeCount := ValueStyle.Render(fmt.Sprintf("%d", info.ActiveStreams))
		summary := fmt.Sprintf("%s active %s", activeCount, streamOrStreams)

		if len(streamTypes) > 0 {
			summary += fmt.Sprintf(" (%s)", strings.Join(streamTypes, ", "))
		}

		output.WriteString(fmt.Sprintf("%s%s", appNameColored, summary))
	}

	return output.String()
}
