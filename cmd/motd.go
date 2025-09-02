package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/motd"
	"github.com/spf13/cobra"
)

// Display flags and options
var (
	showAll              bool
	showAptStatus        bool
	showCPU              bool
	showCpuAverages      bool
	showDisk             bool
	showDistribution     bool
	showDocker           bool
	showEmby             bool
	showGPU              bool
	showJellyfin         bool
	showKernel           bool
	showLastLogin        bool
	showMemory           bool
	showNzbget           bool
	showPlex             bool
	showProcesses        bool
	showQbittorrent      bool
	showQueues           bool
	showRebootRequired   bool
	showRtorrent         bool
	showSabnzbd          bool
	showSessions         bool
	showTraefik          bool
	showUptime           bool
	bannerFile           string
	bannerFileToiletArgs string
	bannerFont           string
	bannerTitle          string
	bannerType           string
	verbosity            int
)

// motdCmd represents the motd command
var motdCmd = &cobra.Command{
	Use:   "motd",
	Short: "Display system information",
	Long: `Displays system information including Ubuntu distribution version,
kernel version, system uptime, CPU load, memory usage, disk usage,
last login, user sessions, process information, and system update status based on flags provided.`,
	Run: func(cmd *cobra.Command, args []string) {
		// If --all flag is used, enable everything
		if showAll {
			showAptStatus = true
			showCPU = true
			showCpuAverages = true
			showDisk = true
			showDistribution = true
			showDocker = true
			showEmby = true
			showGPU = true
			showJellyfin = true
			showKernel = true
			showLastLogin = true
			showMemory = true
			showNzbget = true
			showPlex = true
			showProcesses = true
			showQbittorrent = true
			showQueues = true
			showRebootRequired = true
			showRtorrent = true
			showSabnzbd = true
			showSessions = true
			showTraefik = true
			showUptime = true
		}

		// Check if at least one flag is enabled
		if !showAptStatus && !showCPU && !showCpuAverages && !showDisk && !showDistribution &&
			!showDocker && !showEmby && !showGPU && !showJellyfin && !showKernel && !showLastLogin &&
			!showMemory && !showNzbget && !showPlex && !showProcesses && !showQbittorrent &&
			!showQueues && !showRebootRequired && !showRtorrent && !showSabnzbd && !showSessions &&
			!showTraefik && !showUptime {
			fmt.Println("Error: No information selected to display.")
			fmt.Println("Please use at least one of the following flags:")
			fmt.Println("  --all          Show all information")
			fmt.Println("  --apt          Show apt package status")
			fmt.Println("  --cpu          Show CPU load averages")
			fmt.Println("  --cpu-info     Show CPU model and core count")
			fmt.Println("  --disk         Show disk usage for all partitions")
			fmt.Println("  --distro       Show distribution information")
			fmt.Println("  --docker       Show Docker container information")
			fmt.Println("  --emby         Show Emby streaming information")
			fmt.Println("  --gpu          Show GPU information")
			fmt.Println("  --jellyfin     Show Jellyfin streaming information")
			fmt.Println("  --kernel       Show kernel information")
			fmt.Println("  --login        Show last login information")
			fmt.Println("  --memory       Show memory usage")
			fmt.Println("  --nzbget       Show NZBGet queue information")
			fmt.Println("  --plex         Show Plex streaming information")
			fmt.Println("  --processes    Show process count")
			fmt.Println("  --qbittorrent  Show qBittorrent queue information")
			fmt.Println("  --queues       Show download queue information from Sonarr, Radarr, etc.")
			fmt.Println("  --reboot       Show if reboot is required")
			fmt.Println("  --rtorrent     Show rTorrent queue information")
			fmt.Println("  --sabnzbd      Show Sabnzbd queue information")
			fmt.Println("  --sessions     Show active user sessions")
			fmt.Println("  --traefik      Show Traefik router status information")
			fmt.Println("  --uptime       Show uptime information")
			os.Exit(1)
		}

		motd.Verbose = verbosity > 0

		// Validate banner type if specified
		if bannerType != "" && bannerType != "none" {
			validType := false
			for _, bType := range motd.AvailableBannerTypes {
				if bannerType == bType {
					validType = true
					break
				}
			}

			if !validType {
				fmt.Println("Error: Invalid banner type specified:", bannerType)
				fmt.Println()
				fmt.Println("Available types:")

				// Print available types in columns
				const numColumns = 4
				for i, bType := range motd.AvailableBannerTypes {
					if i%numColumns == 0 {
						fmt.Println()
					}
					fmt.Printf("  %-16s", bType)
				}
				fmt.Println()
				fmt.Println()
				os.Exit(1)
			}
		}

		// Validate font if specified
		if bannerFont != "" && !motd.IsValidFont(bannerFont) {
			fmt.Println("Error: Invalid font specified:", bannerFont)
			fmt.Println()
			fmt.Println("Available fonts (from /usr/share/figlet):")

			// Print available fonts in columns
			fonts := motd.ListAvailableFonts()
			const numColumns = 4
			for i, font := range fonts {
				if i%numColumns == 0 {
					fmt.Println()
				}
				fmt.Printf("  %-16s", font)
			}
			fmt.Println()
			fmt.Println()
			os.Exit(1)
		}

		displayMotd()
	},
}

func displayMotd() {
	// Display a banner from a file if provided. This takes precedence.
	if bannerFile != "" {
		content, err := os.ReadFile(bannerFile)
		if err != nil {
			fmt.Printf("Error: could not read banner file '%s': %v\n", bannerFile, err)
			os.Exit(1)
		}

		var banner string
		// If toilet args are provided, process the file content through toilet.
		if bannerFileToiletArgs != "" {
			banner = motd.GenerateBannerFromFile(string(content), bannerFileToiletArgs)
		} else {
			// Otherwise, just use the raw file content.
			banner = string(content)
		}
		fmt.Println(banner)

	} else if bannerTitle != "" {
		// Otherwise, generate banner if title is provided
		banner := motd.GenerateBanner(bannerTitle, bannerFont, bannerType)
		fmt.Println(banner)
	}

	// Set up info sources with appropriate timeouts and display order
	sources := []motd.InfoSource{
		{Key: "Distribution:", Provider: motd.GetDistributionWithContext, Timeout: 2 * time.Second, Order: 1},
		{Key: "Kernel:", Provider: motd.GetKernelWithContext, Timeout: 1 * time.Second, Order: 2},
		{Key: "Uptime:", Provider: motd.GetUptimeWithContext, Timeout: 1 * time.Second, Order: 3},
		{Key: "Load Averages:", Provider: motd.GetCpuAveragesWithContext, Timeout: 1 * time.Second, Order: 4},
		{Key: "Processes:", Provider: motd.GetProcessCountWithContext, Timeout: 2 * time.Second, Order: 5},
		{Key: "CPU:", Provider: motd.GetCpuInfoWithContext, Timeout: 2 * time.Second, Order: 6},
		{Key: "GPU:", Provider: motd.GetGpuInfoWithContext, Timeout: 3 * time.Second, Order: 7},
		{Key: "Memory Usage:", Provider: motd.GetMemoryInfoWithContext, Timeout: 2 * time.Second, Order: 8},
		{Key: "Package Status:", Provider: motd.GetAptStatusWithContext, Timeout: 5 * time.Second, Order: 9},
		{Key: "Reboot Status:", Provider: motd.GetRebootRequiredWithContext, Timeout: 2 * time.Second, Order: 10},
		{Key: "User Sessions:", Provider: motd.GetUserSessionsWithContext, Timeout: 1 * time.Second, Order: 11},
		{Key: "Last login:", Provider: motd.GetLastLoginWithContext, Timeout: 3 * time.Second, Order: 12},
		{Key: "Disk Usage:", Provider: motd.GetDiskInfoWithContext, Timeout: 3 * time.Second, Order: 13},
		{Key: "Docker:", Provider: motd.GetDockerInfoWithContext, Timeout: 5 * time.Second, Order: 14},
		{Key: "Traefik:", Provider: motd.GetTraefikInfoWithContext, Timeout: 10 * time.Second, Order: 15},
		{Key: "Download Queues:", Provider: motd.GetQueueInfoWithContext, Timeout: 10 * time.Second, Order: 16},
		{Key: "SABnzbd:", Provider: motd.GetSabnzbdInfoWithContext, Timeout: 10 * time.Second, Order: 17},
		{Key: "NZBGet:", Provider: motd.GetNzbgetInfoWithContext, Timeout: 10 * time.Second, Order: 18},
		{Key: "qBittorrent:", Provider: motd.GetQbittorrentInfoWithContext, Timeout: 10 * time.Second, Order: 19},
		{Key: "rTorrent:", Provider: motd.GetRtorrentInfoWithContext, Timeout: 10 * time.Second, Order: 20},
		{Key: "Plex:", Provider: motd.GetPlexInfoWithContext, Timeout: 10 * time.Second, Order: 21},
		{Key: "Emby:", Provider: motd.GetEmbyInfoWithContext, Timeout: 10 * time.Second, Order: 22},
		{Key: "Jellyfin:", Provider: motd.GetJellyfinInfoWithContext, Timeout: 10 * time.Second, Order: 23},
	}

	// Filter sources based on enabled flags
	var activeSources []motd.InfoSource
	flags := map[string]bool{
		"Distribution:":    showDistribution,
		"Kernel:":          showKernel,
		"Uptime:":          showUptime,
		"Load Averages:":   showCpuAverages,
		"Processes:":       showProcesses,
		"CPU:":             showCPU,
		"GPU:":             showGPU,
		"Memory Usage:":    showMemory,
		"Package Status:":  showAptStatus,
		"Reboot Status:":   showRebootRequired,
		"User Sessions:":   showSessions,
		"Last login:":      showLastLogin,
		"Disk Usage:":      showDisk,
		"Docker:":          showDocker,
		"Download Queues:": showQueues,
		"SABnzbd:":         showSabnzbd,
		"NZBGet:":          showNzbget,
		"qBittorrent:":     showQbittorrent,
		"rTorrent:":        showRtorrent,
		"Plex:":            showPlex,
		"Emby:":            showEmby,
		"Jellyfin:":        showJellyfin,
		"Traefik:":         showTraefik,
	}

	// Simply use all enabled sources
	for _, source := range sources {
		if enabled, exists := flags[source.Key]; exists && enabled {
			activeSources = append(activeSources, source)
		}
	}

	// Get system information in parallel
	results := motd.GetSystemInfo(activeSources)

	// Filter out any results with empty values
	var filteredResults []motd.Result
	for _, result := range results {
		if result.Value != "" {
			filteredResults = append(filteredResults, result)
		}
	}

	// Calculate spacing for display
	maxKeyLen := 0
	for _, result := range filteredResults {
		if len(result.Key) > maxKeyLen {
			maxKeyLen = len(result.Key)
		}
	}

	// Add additional spacing (2 spaces)
	spacing := maxKeyLen + 2

	// Display results with consistently styled keys
	for _, result := range filteredResults {
		// Apply key style and add proper spacing
		styledKey := motd.KeyStyle.Render(result.Key)
		paddingLength := spacing - len(result.Key)
		padding := strings.Repeat(" ", paddingLength)

		// Split the value by line breaks to support multi-line values
		lines := strings.Split(result.Value, "\n")

		// Print the first line with the key
		fmt.Printf("%s%s%s\n", styledKey, padding, lines[0])

		// Print any remaining lines with consistent padding
		if len(lines) > 1 {
			for i := 1; i < len(lines); i++ {
				padding := strings.Repeat(" ", spacing)
				fmt.Printf("%s%s\n", padding, lines[i])
			}
		}
	}

	fmt.Println()
}

func init() {
	rootCmd.AddCommand(motdCmd)

	// Define flags for enabling/disabling components (all default to false - opt-in)
	motdCmd.Flags().BoolVar(&showAll, "all", false, "Show all information")
	motdCmd.Flags().BoolVar(&showAptStatus, "apt", false, "Show apt package status")
	motdCmd.Flags().BoolVar(&showCpuAverages, "cpu", false, "Show CPU load averages")
	motdCmd.Flags().BoolVar(&showCPU, "cpu-info", false, "Show CPU model and core count information")
	motdCmd.Flags().BoolVar(&showDisk, "disk", false, "Show disk usage for all partitions")
	motdCmd.Flags().BoolVar(&showDistribution, "distro", false, "Show distribution information")
	motdCmd.Flags().BoolVar(&showDocker, "docker", false, "Show Docker container information")
	motdCmd.Flags().BoolVar(&showEmby, "emby", false, "Show Emby streaming information")
	motdCmd.Flags().BoolVar(&showGPU, "gpu", false, "Show GPU information")
	motdCmd.Flags().BoolVar(&showJellyfin, "jellyfin", false, "Show Jellyfin streaming information")
	motdCmd.Flags().BoolVar(&showKernel, "kernel", false, "Show kernel information")
	motdCmd.Flags().BoolVar(&showLastLogin, "login", false, "Show last login information")
	motdCmd.Flags().BoolVar(&showMemory, "memory", false, "Show memory usage")
	motdCmd.Flags().BoolVar(&showNzbget, "nzbget", false, "Show NZBGet queue information")
	motdCmd.Flags().BoolVar(&showPlex, "plex", false, "Show Plex streaming information")
	motdCmd.Flags().BoolVar(&showProcesses, "processes", false, "Show process count")
	motdCmd.Flags().BoolVar(&showQbittorrent, "qbittorrent", false, "Show qBittorrent queue information")
	motdCmd.Flags().BoolVar(&showQueues, "queues", false, "Show download queue information from Sonarr, Radarr, etc.")
	motdCmd.Flags().BoolVar(&showRebootRequired, "reboot", false, "Show if reboot is required")
	motdCmd.Flags().BoolVar(&showRtorrent, "rtorrent", false, "Show rTorrent queue information")
	motdCmd.Flags().BoolVar(&showSabnzbd, "sabnzbd", false, "Show SABnzbd queue information")
	motdCmd.Flags().BoolVar(&showSessions, "sessions", false, "Show active user sessions")
	motdCmd.Flags().BoolVar(&showTraefik, "traefik", false, "Show Traefik router status information")
	motdCmd.Flags().BoolVar(&showUptime, "uptime", false, "Show uptime information")

	// Add verbosity flag
	motdCmd.Flags().CountVarP(&verbosity, "verbose", "v", "Increase verbosity level (can be used multiple times, e.g. -vvv)")

	// Add banner options
	motdCmd.Flags().StringVar(&bannerTitle, "title", "Saltbox", "Text to display in the banner")
	motdCmd.Flags().StringVar(&bannerType, "type", "peek", "Banner type for boxes (use 'none' to omit box)")
	motdCmd.Flags().StringVar(&bannerFont, "font", "ivrit", "Font for toilet cli")
	motdCmd.Flags().StringVar(&bannerFile, "banner-file", "", "Path to a file containing a custom banner to display")
	motdCmd.Flags().StringVar(&bannerFileToiletArgs, "banner-file-toilet", "", "A string of arguments for toilet when using --banner-file")
}
