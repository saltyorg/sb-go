package motd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/config"
	"github.com/saltyorg/sb-go/constants"
	jellyfin "github.com/sj14/jellyfin-go/api"
)

// JellyfinStreamInfo contains detailed information about Jellyfin streams
type JellyfinStreamInfo struct {
	Name          string
	ActiveStreams int
	DirectPlay    int
	DirectStream  int
	Remux         int
	Transcode     int
}

// GetJellyfinInfo fetches and formats Jellyfin streaming information
func GetJellyfinInfo() string {
	configPath := constants.SaltboxMOTDPath
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if Verbose {
			fmt.Printf("DEBUG: Config file %s does not exist\n", configPath)
		}
		return ""
	}

	if Verbose {
		fmt.Printf("DEBUG: Loading cfg from %s for Jellyfin\n", configPath)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if Verbose {
			fmt.Printf("DEBUG: Error loading cfg: %v\n", err)
		}
		return ""
	}

	jellyfinInstances := cfg.Jellyfin
	if len(jellyfinInstances) == 0 {
		return ""
	}

	var streamInfos []JellyfinStreamInfo
	for _, instance := range jellyfinInstances {
		if instance.URL == "" || instance.Token == "" {
			continue
		}

		info, err := getJellyfinStreamInfo(instance)
		if err != nil {
			if Verbose {
				fmt.Printf("DEBUG: Error getting Jellyfin stream info for %s, hiding entry: %v\n", instance.Name, err)
			}
			continue // Skip this instance on error
		}
		streamInfos = append(streamInfos, info)
	}

	if len(streamInfos) == 0 {
		return ""
	}

	return formatJellyfinOutput(streamInfos)
}

// getJellyfinStreamInfo fetches streaming information from a single Jellyfin server
func getJellyfinStreamInfo(instance config.JellyfinInstance) (JellyfinStreamInfo, error) {
	result := JellyfinStreamInfo{
		Name: instance.Name,
	}

	if result.Name == "" {
		result.Name = "Jellyfin"
	}

	timeout := 1 * time.Second
	if instance.Timeout > 0 {
		timeout = time.Duration(instance.Timeout) * time.Second
	}

	// Configure the API client
	apiConfig := &jellyfin.Configuration{
		Servers:       jellyfin.ServerConfigurations{{URL: instance.URL}},
		DefaultHeader: map[string]string{"Authorization": fmt.Sprintf(`MediaBrowser Token="%s"`, instance.Token)},
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
	}
	client := jellyfin.NewAPIClient(apiConfig)

	// Fetch active sessions
	sessions, resp, err := client.SessionAPI.GetSessions(context.Background()).Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return result, fmt.Errorf("failed to get sessions: %w", err)
	}

	for _, session := range sessions {
		if !session.HasNowPlayingItem() {
			continue
		}
		result.ActiveStreams++

		// Determine stream type
		if !session.HasTranscodingInfo() {
			result.DirectPlay++
		} else {
			transcodeInfo := session.GetTranscodingInfo()
			isVideoDirect := transcodeInfo.GetIsVideoDirect()
			isAudioDirect := transcodeInfo.GetIsAudioDirect()

			if !isVideoDirect {
				result.Transcode++ // Video is being transcoded
			} else if !isAudioDirect {
				result.DirectStream++ // Video is direct, audio is not
			} else {
				// Both audio and video are direct, but TranscodingInfo exists,
				// which implies a container change (Remux).
				result.Remux++
			}
		}
	}
	return result, nil
}

// formatJellyfinOutput formats the Jellyfin streaming information for display
func formatJellyfinOutput(infos []JellyfinStreamInfo) string {
	var output strings.Builder

	// If there's only one instance, omit the name for cleaner output
	if len(infos) == 1 {
		info := infos[0]

		if info.ActiveStreams == 0 {
			output.WriteString("No active streams")
			return output.String()
		}

		streamOrStreams := "stream"
		if info.ActiveStreams != 1 {
			streamOrStreams = "streams"
		}

		var streamTypes []string
		if info.DirectPlay > 0 {
			count := ValueStyle.Render(fmt.Sprintf("%d", info.DirectPlay))
			streamTypes = append(streamTypes, fmt.Sprintf("%s direct play", count))
		}
		if info.Remux > 0 {
			count := ValueStyle.Render(fmt.Sprintf("%d", info.Remux))
			streamTypes = append(streamTypes, fmt.Sprintf("%s remux", count))
		}
		if info.DirectStream > 0 {
			count := ValueStyle.Render(fmt.Sprintf("%d", info.DirectStream))
			streamTypes = append(streamTypes, fmt.Sprintf("%s direct stream", count))
		}
		if info.Transcode > 0 {
			count := ValueStyle.Render(fmt.Sprintf("%d", info.Transcode))
			streamTypes = append(streamTypes, fmt.Sprintf("%s transcode", count))
		}

		activeCount := ValueStyle.Render(fmt.Sprintf("%d", info.ActiveStreams))
		summary := fmt.Sprintf("%s active %s", activeCount, streamOrStreams)

		if len(streamTypes) > 0 {
			summary += fmt.Sprintf(" (%s)", strings.Join(streamTypes, ", "))
		}

		output.WriteString(summary)
		return output.String()
	}

	// Multiple instances - show names for each
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

		if info.ActiveStreams == 0 {
			output.WriteString(fmt.Sprintf("%sNo active streams", appNameColored))
			continue
		}

		streamOrStreams := "stream"
		if info.ActiveStreams != 1 {
			streamOrStreams = "streams"
		}

		var streamTypes []string
		if info.DirectPlay > 0 {
			count := ValueStyle.Render(fmt.Sprintf("%d", info.DirectPlay))
			streamTypes = append(streamTypes, fmt.Sprintf("%s direct play", count))
		}
		if info.Remux > 0 {
			count := ValueStyle.Render(fmt.Sprintf("%d", info.Remux))
			streamTypes = append(streamTypes, fmt.Sprintf("%s remux", count))
		}
		if info.DirectStream > 0 {
			count := ValueStyle.Render(fmt.Sprintf("%d", info.DirectStream))
			streamTypes = append(streamTypes, fmt.Sprintf("%s direct stream", count))
		}
		if info.Transcode > 0 {
			count := ValueStyle.Render(fmt.Sprintf("%d", info.Transcode))
			streamTypes = append(streamTypes, fmt.Sprintf("%s transcode", count))
		}

		activeCount := ValueStyle.Render(fmt.Sprintf("%d", info.ActiveStreams))
		summary := fmt.Sprintf("%s active %s", activeCount, streamOrStreams)

		if len(streamTypes) > 0 {
			summary += fmt.Sprintf(" (%s)", strings.Join(streamTypes, ", "))
		}

		output.WriteString(fmt.Sprintf("%s%s", appNameColored, summary))
	}

	return output.String()
}
