package motd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/saltydk/go-rtorrent"
	"github.com/saltyorg/sb-go/config"
	"github.com/saltyorg/sb-go/constants"
)

// rtorrentInfo holds the processed information for an rTorrent instance.
type rtorrentInfo struct {
	Name             string
	DownloadingCount int
	SeedingCount     int
	StoppedCount     int
	ErrorCount       int
	DownloadSpeed    int
	UploadSpeed      int
	Error            error
}

// GetRtorrentInfo fetches and formats rTorrent information.
func GetRtorrentInfo() string {
	configPath := constants.SaltboxMOTDPath
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if Verbose {
			fmt.Printf("DEBUG: Config file %s does not exist\n", configPath)
		}
		return ""
	}

	if Verbose {
		fmt.Printf("DEBUG: Loading cfg from %s for rTorrent\n", configPath)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if Verbose {
			fmt.Printf("DEBUG: Error loading cfg: %v\n", err)
		}
		return ""
	}

	rtorrentInstances := cfg.Rtorrent
	if len(rtorrentInstances) == 0 {
		return ""
	}

	// Create a wait group and mutex for async processing
	var wg sync.WaitGroup
	var mu sync.Mutex
	var queueInfos []rtorrentInfo

	// Process each rTorrent instance concurrently
	for i, instance := range rtorrentInstances {
		if instance.URL == "" {
			if Verbose {
				fmt.Printf("DEBUG: Skipping rTorrent instance %d due to missing URL\n", i)
			}
			continue
		}

		wg.Add(1)
		go func(idx int, inst config.UserPassAppInstance) {
			defer wg.Done()

			if Verbose {
				fmt.Printf("DEBUG: Processing rTorrent instance %d: %s, URL: %s\n", idx, inst.Name, inst.URL)
			}

			info, err := getRtorrentStats(inst)
			if err != nil {
				if Verbose {
					fmt.Printf("DEBUG: Error getting rTorrent info for %s, hiding entry: %v\n", inst.Name, err)
				}
				return
			}

			if Verbose {
				fmt.Printf("DEBUG: Successfully retrieved rTorrent info for instance %d: %d downloading, %d seeding\n", idx, info.DownloadingCount, info.SeedingCount)
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

	return formatRtorrentOutput(queueInfos)
}

// getRtorrentStats fetches statistics from an rTorrent instance.
func getRtorrentStats(instance config.UserPassAppInstance) (rtorrentInfo, error) {
	result := rtorrentInfo{Name: instance.Name}
	if result.Name == "" {
		result.Name = "rTorrent"
	}

	// Configure the client
	clientCfg := rtorrent.Config{
		Addr:      instance.URL,
		BasicUser: instance.User,
		BasicPass: instance.Password,
	}

	client := rtorrent.NewClient(clientCfg)

	// Correctly apply the timeout by creating a custom http.Client
	if instance.Timeout > 0 {
		httpClient := &http.Client{
			Timeout: time.Duration(instance.Timeout) * time.Second,
		}
		// Use the WithHTTPClient method to apply the custom client
		client = client.WithHTTPClient(httpClient)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Fetch all torrents
	torrents, err := client.GetTorrents(ctx, rtorrent.ViewMain)
	if err != nil {
		return result, fmt.Errorf("could not get torrents: %w", err)
	}

	// Get global speeds
	downRate, err := client.DownRate(ctx)
	if err != nil {
		return result, fmt.Errorf("could not get download rate: %w", err)
	}
	result.DownloadSpeed = downRate

	upRate, err := client.UpRate(ctx)
	if err != nil {
		return result, fmt.Errorf("could not get upload rate: %w", err)
	}
	result.UploadSpeed = upRate

	// Categorize torrents by state
	for _, t := range torrents {
		// If the torrent has a non-empty message, consider it an error
		if t.Message != "" {
			result.ErrorCount++
			continue // Skip to the next torrent
		}

		isActive, err := client.IsActive(ctx, t)
		if err != nil {
			// Skip torrent if we can't determine its state
			continue
		}

		if isActive {
			if t.Completed {
				result.SeedingCount++
			} else {
				result.DownloadingCount++
			}
		} else {
			result.StoppedCount++
		}
	}

	return result, nil
}

// formatRtorrentOutput formats the rTorrent information for display.
func formatRtorrentOutput(infos []rtorrentInfo) string {
	if len(infos) == 1 {
		return formatRtorrentSummary(infos[0])
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

		summary := formatRtorrentSummary(info)
		output.WriteString(fmt.Sprintf("%s%s", appNameColored, summary))
	}

	return output.String()
}

// formatRtorrentSummary is a helper to format the summary for a single instance.
func formatRtorrentSummary(info rtorrentInfo) string {
	if info.DownloadingCount == 0 && info.SeedingCount == 0 && info.StoppedCount == 0 && info.ErrorCount == 0 {
		return "No torrents present"
	}

	dlSpeed := ValueStyle.Render(formatSpeed(int64(info.DownloadSpeed)))
	upSpeed := ValueStyle.Render(formatSpeed(int64(info.UploadSpeed)))

	downloading := ValueStyle.Render(fmt.Sprintf("%d", info.DownloadingCount))
	seeding := ValueStyle.Render(fmt.Sprintf("%d", info.SeedingCount))
	stopped := ValueStyle.Render(fmt.Sprintf("%d", info.StoppedCount))
	errored := ValueStyle.Render(fmt.Sprintf("%d", info.ErrorCount))

	var parts []string
	parts = append(parts, fmt.Sprintf("↓%s ↑%s", dlSpeed, upSpeed))
	parts = append(parts, fmt.Sprintf("%s Downloading", downloading))
	parts = append(parts, fmt.Sprintf("%s Seeding", seeding))

	if info.StoppedCount > 0 {
		parts = append(parts, RedStyle.Render(fmt.Sprintf("%s Stopped", stopped)))
	}

	if info.ErrorCount > 0 {
		parts = append(parts, RedStyle.Render(fmt.Sprintf("%s Error", errored)))
	}

	return strings.Join(parts, " | ")
}
