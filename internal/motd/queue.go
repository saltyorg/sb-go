package motd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/saltyorg/sb-go/internal/config"
	"github.com/saltyorg/sb-go/internal/constants"

	"golift.io/starr"
	"golift.io/starr/lidarr"
	"golift.io/starr/radarr"
	"golift.io/starr/readarr"
	"golift.io/starr/sonarr"
)

// QueueItem represents an individual item in the queue with its status
type QueueItem struct {
	Status string
}

// QueueInfo represents queue information for an app instance
type QueueInfo struct {
	Name  string
	Items []QueueItem
	Error error
}

// GetQueueInfo fetches queue information from configured applications
func GetQueueInfo(verbose bool) string {
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
		fmt.Printf("DEBUG: Loading config from %s\n", configPath)
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

	if verbose {
		fmt.Printf("DEBUG: Loaded config - Sonarr: %d instances, Radarr: %d instances, Lidarr: %d instances, Readarr: %d instances\n",
			len(cfg.Sonarr), len(cfg.Radarr), len(cfg.Lidarr), len(cfg.Readarr))
	}

	// Create a wait group to fetch all queues concurrently
	var wg sync.WaitGroup

	// Use a mutex to protect the shared slice
	var mu sync.Mutex
	var allQueues []QueueInfo

	// Fetch Sonarr queues concurrently
	for i, instance := range cfg.Sonarr {
		if !instance.IsEnabled() {
			if verbose {
				fmt.Printf("DEBUG: Skipping Sonarr instance %d because it is disabled\n", i)
			}
			continue
		}
		if instance.URL == "" || instance.APIKey == "" {
			if verbose {
				fmt.Printf("DEBUG: Skipping Sonarr instance %d due to missing URL or API key\n", i)
			}
			continue
		}

		wg.Add(1)
		go func(idx int, inst config.AppInstance) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					if verbose {
						fmt.Fprintf(os.Stderr, "PANIC in Sonarr queue fetch (instance %d): %v\n", idx, r)
					}
				}
			}()

			if verbose {
				fmt.Printf("DEBUG: Processing Sonarr instance %d: %s, URL: %s\n", idx, inst.Name, inst.URL)
			}

			queue, err := getSonarrQueueDetailed(inst, verbose)
			if err != nil {
				if verbose {
					fmt.Printf("DEBUG: Error getting detailed Sonarr queue for instance %d, hiding entry: %v\n", idx, err)
				}
				return
			}

			if verbose {
				fmt.Printf("DEBUG: Successfully retrieved detailed Sonarr queue for instance %d: %d items\n", idx, len(queue.Items))
			}

			mu.Lock()
			allQueues = append(allQueues, queue)
			mu.Unlock()
		}(i, instance)
	}

	// Fetch Radarr queues concurrently
	for i, instance := range cfg.Radarr {
		if !instance.IsEnabled() {
			if verbose {
				fmt.Printf("DEBUG: Skipping Radarr instance %d because it is disabled\n", i)
			}
			continue
		}
		if instance.URL == "" || instance.APIKey == "" {
			if verbose {
				fmt.Printf("DEBUG: Skipping Radarr instance %d due to missing URL or API key\n", i)
			}
			continue
		}

		wg.Add(1)
		go func(idx int, inst config.AppInstance) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					if verbose {
						fmt.Fprintf(os.Stderr, "PANIC in Radarr queue fetch (instance %d): %v\n", idx, r)
					}
				}
			}()

			if verbose {
				fmt.Printf("DEBUG: Processing Radarr instance %d: %s, URL: %s\n", idx, inst.Name, inst.URL)
			}

			queue, err := getRadarrQueueDetailed(inst, verbose)
			if err != nil {
				if verbose {
					fmt.Printf("DEBUG: Error getting detailed Radarr queue for instance %d, hiding entry: %v\n", idx, err)
				}
				return
			}

			if verbose {
				fmt.Printf("DEBUG: Successfully retrieved detailed Radarr queue for instance %d: %d items\n", idx, len(queue.Items))
			}

			mu.Lock()
			allQueues = append(allQueues, queue)
			mu.Unlock()
		}(i, instance)
	}

	// Fetch Lidarr queues concurrently
	for i, instance := range cfg.Lidarr {
		if !instance.IsEnabled() {
			if verbose {
				fmt.Printf("DEBUG: Skipping Lidarr instance %d because it is disabled\n", i)
			}
			continue
		}
		if instance.URL == "" || instance.APIKey == "" {
			if verbose {
				fmt.Printf("DEBUG: Skipping Lidarr instance %d due to missing URL or API key\n", i)
			}
			continue
		}

		wg.Add(1)
		go func(idx int, inst config.AppInstance) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					if verbose {
						fmt.Fprintf(os.Stderr, "PANIC in Lidarr queue fetch (instance %d): %v\n", idx, r)
					}
				}
			}()

			if verbose {
				fmt.Printf("DEBUG: Processing Lidarr instance %d: %s, URL: %s\n", idx, inst.Name, inst.URL)
			}

			queue, err := getLidarrQueueDetailed(inst, verbose)
			if err != nil {
				if verbose {
					fmt.Printf("DEBUG: Error getting detailed Lidarr queue for instance %d, hiding entry: %v\n", idx, err)
				}
				return
			}

			if verbose {
				fmt.Printf("DEBUG: Successfully retrieved detailed Lidarr queue for instance %d: %d items\n", idx, len(queue.Items))
			}

			mu.Lock()
			allQueues = append(allQueues, queue)
			mu.Unlock()
		}(i, instance)
	}

	// Fetch Readarr queues concurrently
	for i, instance := range cfg.Readarr {
		if !instance.IsEnabled() {
			if verbose {
				fmt.Printf("DEBUG: Skipping Readarr instance %d because it is disabled\n", i)
			}
			continue
		}
		if instance.URL == "" || instance.APIKey == "" {
			if verbose {
				fmt.Printf("DEBUG: Skipping Readarr instance %d due to missing URL or API key\n", i)
			}
			continue
		}

		wg.Add(1)
		go func(idx int, inst config.AppInstance) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					if verbose {
						fmt.Fprintf(os.Stderr, "PANIC in Readarr queue fetch (instance %d): %v\n", idx, r)
					}
				}
			}()

			if verbose {
				fmt.Printf("DEBUG: Processing Readarr instance %d: %s, URL: %s\n", idx, inst.Name, inst.URL)
			}

			queue, err := getReadarrQueueDetailed(inst, verbose)
			if err != nil {
				if verbose {
					fmt.Printf("DEBUG: Error getting detailed Readarr queue for instance %d, hiding entry: %v\n", idx, err)
				}
				return
			}

			if verbose {
				fmt.Printf("DEBUG: Successfully retrieved detailed Readarr queue for instance %d: %d items\n", idx, len(queue.Items))
			}

			mu.Lock()
			allQueues = append(allQueues, queue)
			mu.Unlock()
		}(i, instance)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	if verbose {
		fmt.Printf("DEBUG: All queue fetching completed, got %d valid queues\n", len(allQueues))
	}

	// If no valid queues were found
	if len(allQueues) == 0 {
		if verbose {
			fmt.Println("DEBUG: No valid queues found, returning empty string")
		}
		return ""
	}

	if verbose {
		fmt.Printf("DEBUG: Found %d valid queues, formatting output\n", len(allQueues))
	}

	// Sort queues by app name
	sort.Slice(allQueues, func(i, j int) bool {
		return allQueues[i].Name < allQueues[j].Name
	})

	// Format the output
	return formatDetailedQueueOutput(allQueues, verbose)
}

// getSonarrQueueDetailed gets the detailed queue for a Sonarr instance
func getSonarrQueueDetailed(instance config.AppInstance, verbose bool) (QueueInfo, error) {
	if verbose {
		fmt.Printf("DEBUG: Creating Sonarr client for %s (%s)\n", instance.Name, instance.URL)
	}

	// Set timeout, defaulting to 1 second
	timeout := 1 * time.Second
	if instance.Timeout > 0 {
		timeout = time.Duration(instance.Timeout) * time.Second
	}

	// Create a starr.Config with a custom timeout
	c := starr.New(instance.APIKey, instance.URL, timeout)
	client := sonarr.New(c)

	if verbose {
		fmt.Printf("DEBUG: Fetching Sonarr queue for %s\n", instance.Name)
	}

	// GetQueue(records, perPage) - 0 records means get all, 100 perPage for internal pagination
	queue, err := client.GetQueue(0, 100)
	if err != nil {
		if verbose {
			fmt.Printf("DEBUG: Error fetching Sonarr queue: %v\n", err)
		}
		return QueueInfo{}, err
	}

	// Check for nil queue to prevent dereference
	if queue == nil {
		if verbose {
			fmt.Printf("DEBUG: Received nil queue from Sonarr API\n")
		}
		return QueueInfo{Name: instance.Name, Items: []QueueItem{}}, nil
	}

	if verbose {
		fmt.Printf("DEBUG: Received Sonarr queue with %d total records\n", len(queue.Records))
	}

	info := QueueInfo{
		Name:  instance.Name,
		Items: make([]QueueItem, len(queue.Records)),
	}
	for i, record := range queue.Records {
		info.Items[i] = QueueItem{Status: record.Status}
	}

	return info, nil
}

// getRadarrQueueDetailed gets the detailed queue for a Radarr instance
func getRadarrQueueDetailed(instance config.AppInstance, verbose bool) (QueueInfo, error) {
	if verbose {
		fmt.Printf("DEBUG: Creating Radarr client for %s (%s)\n", instance.Name, instance.URL)
	}

	timeout := 1 * time.Second
	if instance.Timeout > 0 {
		timeout = time.Duration(instance.Timeout) * time.Second
	}

	c := starr.New(instance.APIKey, instance.URL, timeout)
	client := radarr.New(c)

	if verbose {
		fmt.Printf("DEBUG: Fetching Radarr queue for %s\n", instance.Name)
	}

	// GetQueue(records, perPage) - 0 records means get all, 100 perPage for internal pagination
	queue, err := client.GetQueue(0, 100)
	if err != nil {
		if verbose {
			fmt.Printf("DEBUG: Error fetching Radarr queue: %v\n", err)
		}
		return QueueInfo{}, err
	}

	// Check for nil queue to prevent dereference
	if queue == nil {
		if verbose {
			fmt.Printf("DEBUG: Received nil queue from Radarr API\n")
		}
		return QueueInfo{Name: instance.Name, Items: []QueueItem{}}, nil
	}

	if verbose {
		fmt.Printf("DEBUG: Received Radarr queue with %d total records\n", len(queue.Records))
	}

	info := QueueInfo{
		Name:  instance.Name,
		Items: make([]QueueItem, len(queue.Records)),
	}
	for i, record := range queue.Records {
		info.Items[i] = QueueItem{Status: record.Status}
	}

	return info, nil
}

// getLidarrQueueDetailed gets the detailed queue for a Lidarr instance
func getLidarrQueueDetailed(instance config.AppInstance, verbose bool) (QueueInfo, error) {
	if verbose {
		fmt.Printf("DEBUG: Creating Lidarr client for %s (%s)\n", instance.Name, instance.URL)
	}

	timeout := 1 * time.Second
	if instance.Timeout > 0 {
		timeout = time.Duration(instance.Timeout) * time.Second
	}

	c := starr.New(instance.APIKey, instance.URL, timeout)
	client := lidarr.New(c)

	if verbose {
		fmt.Printf("DEBUG: Fetching Lidarr queue for %s\n", instance.Name)
	}

	// GetQueue(records, perPage) - 0 records means get all, 100 perPage for internal pagination
	queue, err := client.GetQueue(0, 100)
	if err != nil {
		if verbose {
			fmt.Printf("DEBUG: Error fetching Lidarr queue: %v\n", err)
		}
		return QueueInfo{}, err
	}

	// Check for nil queue to prevent dereference
	if queue == nil {
		if verbose {
			fmt.Printf("DEBUG: Received nil queue from Lidarr API\n")
		}
		return QueueInfo{Name: instance.Name, Items: []QueueItem{}}, nil
	}

	if verbose {
		fmt.Printf("DEBUG: Received Lidarr queue with %d total records\n", len(queue.Records))
	}

	info := QueueInfo{
		Name:  instance.Name,
		Items: make([]QueueItem, len(queue.Records)),
	}
	for i, record := range queue.Records {
		info.Items[i] = QueueItem{Status: record.Status}
	}

	return info, nil
}

// getReadarrQueueDetailed gets the detailed queue for a Readarr instance
func getReadarrQueueDetailed(instance config.AppInstance, verbose bool) (QueueInfo, error) {
	if verbose {
		fmt.Printf("DEBUG: Creating Readarr client for %s (%s)\n", instance.Name, instance.URL)
	}

	timeout := 1 * time.Second
	if instance.Timeout > 0 {
		timeout = time.Duration(instance.Timeout) * time.Second
	}

	c := starr.New(instance.APIKey, instance.URL, timeout)
	client := readarr.New(c)

	if verbose {
		fmt.Printf("DEBUG: Fetching Readarr queue for %s\n", instance.Name)
	}

	// GetQueue(records, perPage) - 0 records means get all, 100 perPage for internal pagination
	queue, err := client.GetQueue(0, 100)
	if err != nil {
		if verbose {
			fmt.Printf("DEBUG: Error fetching Readarr queue: %v\n", err)
		}
		return QueueInfo{}, err
	}

	// Check for nil queue to prevent dereference
	if queue == nil {
		if verbose {
			fmt.Printf("DEBUG: Received nil queue from Readarr API\n")
		}
		return QueueInfo{Name: instance.Name, Items: []QueueItem{}}, nil
	}

	if verbose {
		fmt.Printf("DEBUG: Received Readarr queue with %d total records\n", len(queue.Records))
	}

	info := QueueInfo{
		Name:  instance.Name,
		Items: make([]QueueItem, len(queue.Records)),
	}
	for i, record := range queue.Records {
		info.Items[i] = QueueItem{Status: record.Status}
	}

	return info, nil
}

// formatDetailedQueueOutput formats the detailed queue information for display
func formatDetailedQueueOutput(queues []QueueInfo, verbose bool) string {
	if len(queues) == 0 {
		if verbose {
			fmt.Println("DEBUG: No queues to format")
		}
		return ""
	}

	var output strings.Builder

	// Find the length of the longest name
	maxNameLen := 0
	for _, queue := range queues {
		if len(queue.Name) > maxNameLen {
			maxNameLen = len(queue.Name)
		}
	}

	for i, queue := range queues {
		// Add a newline between apps
		if i > 0 {
			output.WriteString("\n")
		}

		appName := queue.Name
		if appName == "" {
			appName = "Unknown App"
		}

		if verbose {
			fmt.Printf("DEBUG: Formatting detailed output for %s with %d items\n", appName, len(queue.Items))
		}

		statusCounts := make(map[string]int)
		for _, item := range queue.Items {
			statusCounts[item.Status]++
		}

		var statusParts []string
		totalItems := len(queue.Items)

		// Sort statuses for consistent output
		var statuses []string
		for status := range statusCounts {
			statuses = append(statuses, status)
		}
		sort.Strings(statuses)

		for _, status := range statuses {
			count := statusCounts[status]
			statusParts = append(statusParts, fmt.Sprintf("%s %s", ValueStyle.Render(fmt.Sprintf("%d", count)), strings.ToLower(status)))
		}

		var queueSummary string
		itemOrItems := "items"
		if totalItems == 1 {
			itemOrItems = "item"
		}
		queueSummary = fmt.Sprintf("%s %s in queue", ValueStyle.Render(fmt.Sprintf("%d", totalItems)), itemOrItems)
		if len(statusParts) > 0 {
			queueSummary += fmt.Sprintf(", %s", strings.Join(statusParts, ", "))
		}

		// Align the queue summary text
		namePadding := maxNameLen - len(appName)
		paddedName := fmt.Sprintf("%s:%s", appName, strings.Repeat(" ", namePadding+1))

		appNameColored := AppNameStyle.Render(paddedName)
		output.WriteString(fmt.Sprintf("%s%s", appNameColored, queueSummary))
	}

	if verbose {
		fmt.Println("DEBUG: Formatted detailed output ready to return")
	}

	return output.String()
}
