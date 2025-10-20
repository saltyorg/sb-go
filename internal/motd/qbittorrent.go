package motd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/saltyorg/sb-go/internal/config"
	"github.com/saltyorg/sb-go/internal/constants"

	"github.com/autobrr/go-qbittorrent"
)

// qbittorrentInfo holds the processed information for a qBittorrent instance.
type qbittorrentInfo struct {
	Name             string
	DownloadingCount int
	SeedingCount     int
	ErrorCount       int
	StoppedCount     int
	DownloadSpeed    int64
	UploadSpeed      int64
	Error            error
}

// GetQbittorrentInfo fetches and formats qBittorrent information.
func GetQbittorrentInfo(verbose bool) string {
	configPath := constants.SaltboxMOTDConfigPath
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if verbose {
			fmt.Printf("DEBUG: Config file %s does not exist\n", configPath)
		}
		return ""
	}

	if verbose {
		fmt.Printf("DEBUG: Loading cfg from %s for qBittorrent\n", configPath)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if verbose {
			fmt.Printf("DEBUG: Error loading cfg: %v\n", err)
		}
		return ""
	}

	qbittorrentInstances := cfg.Qbittorrent
	if len(qbittorrentInstances) == 0 {
		return ""
	}

	// Create a wait group and mutex for async processing
	var wg sync.WaitGroup
	var mu sync.Mutex
	var queueInfos []qbittorrentInfo

	// Process each qBittorrent instance concurrently
	for i, instance := range qbittorrentInstances {
		if instance.URL == "" || instance.User == "" || instance.Password == "" {
			if verbose {
				fmt.Printf("DEBUG: Skipping qBittorrent instance %d due to missing URL, user, or password\n", i)
			}
			continue
		}

		wg.Add(1)
		go func(idx int, inst config.UserPassAppInstance) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					if verbose {
						fmt.Fprintf(os.Stderr, "PANIC in qBittorrent stats fetch (instance %d): %v\n", idx, r)
					}
				}
			}()

			if verbose {
				fmt.Printf("DEBUG: Processing qBittorrent instance %d: %s, URL: %s\n", idx, inst.Name, inst.URL)
			}

			info, err := getQbittorrentStats(inst)
			if err != nil {
				if verbose {
					fmt.Printf("DEBUG: Error getting qBittorrent info for %s, hiding entry: %v\n", inst.Name, err)
				}
				return
			}

			if verbose {
				fmt.Printf("DEBUG: Successfully retrieved qBittorrent info for instance %d: %d downloading, %d seeding\n", idx, info.DownloadingCount, info.SeedingCount)
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

	return formatQbittorrentOutput(queueInfos)
}

// getQbittorrentStats fetches statistics using the sync endpoint.
func getQbittorrentStats(instance config.UserPassAppInstance) (qbittorrentInfo, error) {
	result := qbittorrentInfo{Name: instance.Name}
	if result.Name == "" {
		result.Name = "qBittorrent"
	}

	// Configure the client
	clientCfg := qbittorrent.Config{
		Host:     instance.URL,
		Username: instance.User,
		Password: instance.Password,
		Timeout:  instance.Timeout,
	}
	client := qbittorrent.NewClient(clientCfg)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Login
	if err := client.LoginCtx(ctx); err != nil {
		return result, fmt.Errorf("failed to login to qbittorrent: %w", err)
	}

	// Fetch all data
	mainData, err := client.SyncMainDataCtx(ctx, 0) // rid=0 to get a full update
	if err != nil {
		return result, fmt.Errorf("could not sync main data: %w", err)
	}

	// Populate speeds from the server state
	result.DownloadSpeed = mainData.ServerState.DlInfoSpeed
	result.UploadSpeed = mainData.ServerState.UpInfoSpeed

	// Categorize torrents by iterating through the map from the sync data
	for _, t := range mainData.Torrents {
		switch t.State {
		// Downloading States
		case qbittorrent.TorrentStateDownloading, qbittorrent.TorrentStateForcedDl, qbittorrent.TorrentStateMetaDl, qbittorrent.TorrentStateCheckingDl, qbittorrent.TorrentStateQueuedDl, qbittorrent.TorrentStateStalledDl:
			result.DownloadingCount++

		// Seeding States
		case qbittorrent.TorrentStateUploading, qbittorrent.TorrentStateForcedUp, qbittorrent.TorrentStateCheckingUp, qbittorrent.TorrentStateQueuedUp, qbittorrent.TorrentStateStalledUp:
			result.SeedingCount++

		// Stopped/Paused States
		case qbittorrent.TorrentStatePausedDl, qbittorrent.TorrentStatePausedUp, qbittorrent.TorrentStateStoppedUp, qbittorrent.TorrentStateStoppedDl:
			result.StoppedCount++

		// Error States
		case qbittorrent.TorrentStateError, qbittorrent.TorrentStateMissingFiles:
			result.ErrorCount++
		}
	}

	return result, nil
}

// formatQbittorrentOutput formats the qBittorrent information for display.
func formatQbittorrentOutput(infos []qbittorrentInfo) string {
	if len(infos) == 1 {
		return formatQbittorrentSummary(infos[0])
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

		summary := formatQbittorrentSummary(info)
		output.WriteString(fmt.Sprintf("%s%s", appNameColored, summary))
	}

	return output.String()
}

// formatQbittorrentSummary is a helper to format the summary for a single instance.
func formatQbittorrentSummary(info qbittorrentInfo) string {
	// Check if there are any torrents in any category before proceeding.
	if info.DownloadingCount == 0 && info.SeedingCount == 0 && info.StoppedCount == 0 && info.ErrorCount == 0 {
		return "No torrents present"
	}

	dlSpeed := ValueStyle.Render(formatSpeed(info.DownloadSpeed))
	upSpeed := ValueStyle.Render(formatSpeed(info.UploadSpeed))

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

// formatSpeed converts bytes/s to a human-readable string (KB/s, MB/s, etc.).
func formatSpeed(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B/s", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB/s", float64(b)/float64(div), "KMGTPE"[exp])
}
