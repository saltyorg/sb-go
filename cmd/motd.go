package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/motd"

	"github.com/spf13/cobra"
)

// motdConfig holds the configuration for the motd command
type motdConfig struct {
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
}

// motdCmd represents the motd command
var motdCmd = &cobra.Command{
	Use:   "motd",
	Short: "Display system information",
	Long: `Displays system information including Ubuntu distribution version,
kernel version, system uptime, CPU load, memory usage, disk usage,
last login, user sessions, process information, and system update status based on flags provided.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Get flag values and create config
		config := &motdConfig{}
		config.showAll, _ = cmd.Flags().GetBool("all")
		config.showAptStatus, _ = cmd.Flags().GetBool("apt")
		config.showCPU, _ = cmd.Flags().GetBool("cpu-info")
		config.showCpuAverages, _ = cmd.Flags().GetBool("cpu")
		config.showDisk, _ = cmd.Flags().GetBool("disk")
		config.showDistribution, _ = cmd.Flags().GetBool("distro")
		config.showDocker, _ = cmd.Flags().GetBool("docker")
		config.showEmby, _ = cmd.Flags().GetBool("emby")
		config.showGPU, _ = cmd.Flags().GetBool("gpu")
		config.showJellyfin, _ = cmd.Flags().GetBool("jellyfin")
		config.showKernel, _ = cmd.Flags().GetBool("kernel")
		config.showLastLogin, _ = cmd.Flags().GetBool("login")
		config.showMemory, _ = cmd.Flags().GetBool("memory")
		config.showNzbget, _ = cmd.Flags().GetBool("nzbget")
		config.showPlex, _ = cmd.Flags().GetBool("plex")
		config.showProcesses, _ = cmd.Flags().GetBool("processes")
		config.showQbittorrent, _ = cmd.Flags().GetBool("qbittorrent")
		config.showQueues, _ = cmd.Flags().GetBool("queues")
		config.showRebootRequired, _ = cmd.Flags().GetBool("reboot")
		config.showRtorrent, _ = cmd.Flags().GetBool("rtorrent")
		config.showSabnzbd, _ = cmd.Flags().GetBool("sabnzbd")
		config.showSessions, _ = cmd.Flags().GetBool("sessions")
		config.showTraefik, _ = cmd.Flags().GetBool("traefik")
		config.showUptime, _ = cmd.Flags().GetBool("uptime")
		config.bannerFile, _ = cmd.Flags().GetString("banner-file")
		config.bannerFileToiletArgs, _ = cmd.Flags().GetString("banner-file-toilet")
		config.bannerFont, _ = cmd.Flags().GetString("font")
		config.bannerTitle, _ = cmd.Flags().GetString("title")
		config.bannerType, _ = cmd.Flags().GetString("type")
		config.verbosity, _ = cmd.Flags().GetCount("verbose")

		runMotdCommand(config)
	},
}

// runMotdCommand handles the main logic for the motd command
func runMotdCommand(config *motdConfig) {
	// If --all flag is used, enable everything
	if config.showAll {
		config.showAptStatus = true
		config.showCPU = true
		config.showCpuAverages = true
		config.showDisk = true
		config.showDistribution = true
		config.showDocker = true
		config.showEmby = true
		config.showGPU = true
		config.showJellyfin = true
		config.showKernel = true
		config.showLastLogin = true
		config.showMemory = true
		config.showNzbget = true
		config.showPlex = true
		config.showProcesses = true
		config.showQbittorrent = true
		config.showQueues = true
		config.showRebootRequired = true
		config.showRtorrent = true
		config.showSabnzbd = true
		config.showSessions = true
		config.showTraefik = true
		config.showUptime = true
	}

	// Check if at least one flag is enabled
	if !config.showAptStatus && !config.showCPU && !config.showCpuAverages && !config.showDisk && !config.showDistribution &&
		!config.showDocker && !config.showEmby && !config.showGPU && !config.showJellyfin && !config.showKernel && !config.showLastLogin &&
		!config.showMemory && !config.showNzbget && !config.showPlex && !config.showProcesses && !config.showQbittorrent &&
		!config.showQueues && !config.showRebootRequired && !config.showRtorrent && !config.showSabnzbd && !config.showSessions &&
		!config.showTraefik && !config.showUptime {
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

	motd.Verbose = config.verbosity > 0

	// Validate banner type if specified
	if config.bannerType != "" && config.bannerType != "none" {
		validType := false
		for _, bType := range motd.AvailableBannerTypes {
			if config.bannerType == bType {
				validType = true
				break
			}
		}

		if !validType {
			fmt.Println("Error: Invalid banner type specified:", config.bannerType)
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
	if config.bannerFont != "" && !motd.IsValidFont(config.bannerFont) {
		fmt.Println("Error: Invalid font specified:", config.bannerFont)
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

	displayMotd(config)
}

func displayMotd(config *motdConfig) {
	// Display a banner from a file if provided. This takes precedence.
	if config.bannerFile != "" {
		content, err := os.ReadFile(config.bannerFile)
		if err != nil {
			fmt.Printf("Error: could not read banner file '%s': %v\n", config.bannerFile, err)
			os.Exit(1)
		}

		var banner string
		// If toilet args are provided, process the file content through toilet.
		if config.bannerFileToiletArgs != "" {
			banner = motd.GenerateBannerFromFile(string(content), config.bannerFileToiletArgs)
		} else {
			// Otherwise, just use the raw file content.
			banner = string(content)
		}
		fmt.Println(banner)

	} else if config.bannerTitle != "" {
		// Otherwise, generate banner if title is provided
		banner := motd.GenerateBanner(config.bannerTitle, config.bannerFont, config.bannerType)
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
		"Distribution:":    config.showDistribution,
		"Kernel:":          config.showKernel,
		"Uptime:":          config.showUptime,
		"Load Averages:":   config.showCpuAverages,
		"Processes:":       config.showProcesses,
		"CPU:":             config.showCPU,
		"GPU:":             config.showGPU,
		"Memory Usage:":    config.showMemory,
		"Package Status:":  config.showAptStatus,
		"Reboot Status:":   config.showRebootRequired,
		"User Sessions:":   config.showSessions,
		"Last login:":      config.showLastLogin,
		"Disk Usage:":      config.showDisk,
		"Docker:":          config.showDocker,
		"Download Queues:": config.showQueues,
		"SABnzbd:":         config.showSabnzbd,
		"NZBGet:":          config.showNzbget,
		"qBittorrent:":     config.showQbittorrent,
		"rTorrent:":        config.showRtorrent,
		"Plex:":            config.showPlex,
		"Emby:":            config.showEmby,
		"Jellyfin:":        config.showJellyfin,
		"Traefik:":         config.showTraefik,
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
	motdCmd.Flags().Bool("all", false, "Show all information")
	motdCmd.Flags().Bool("apt", false, "Show apt package status")
	motdCmd.Flags().Bool("cpu", false, "Show CPU load averages")
	motdCmd.Flags().Bool("cpu-info", false, "Show CPU model and core count information")
	motdCmd.Flags().Bool("disk", false, "Show disk usage for all partitions")
	motdCmd.Flags().Bool("distro", false, "Show distribution information")
	motdCmd.Flags().Bool("docker", false, "Show Docker container information")
	motdCmd.Flags().Bool("emby", false, "Show Emby streaming information")
	motdCmd.Flags().Bool("gpu", false, "Show GPU information")
	motdCmd.Flags().Bool("jellyfin", false, "Show Jellyfin streaming information")
	motdCmd.Flags().Bool("kernel", false, "Show kernel information")
	motdCmd.Flags().Bool("login", false, "Show last login information")
	motdCmd.Flags().Bool("memory", false, "Show memory usage")
	motdCmd.Flags().Bool("nzbget", false, "Show NZBGet queue information")
	motdCmd.Flags().Bool("plex", false, "Show Plex streaming information")
	motdCmd.Flags().Bool("processes", false, "Show process count")
	motdCmd.Flags().Bool("qbittorrent", false, "Show qBittorrent queue information")
	motdCmd.Flags().Bool("queues", false, "Show download queue information from Sonarr, Radarr, etc.")
	motdCmd.Flags().Bool("reboot", false, "Show if reboot is required")
	motdCmd.Flags().Bool("rtorrent", false, "Show rTorrent queue information")
	motdCmd.Flags().Bool("sabnzbd", false, "Show SABnzbd queue information")
	motdCmd.Flags().Bool("sessions", false, "Show active user sessions")
	motdCmd.Flags().Bool("traefik", false, "Show Traefik router status information")
	motdCmd.Flags().Bool("uptime", false, "Show uptime information")

	// Add verbosity flag
	motdCmd.Flags().CountP("verbose", "v", "Increase verbosity level (can be used multiple times, e.g. -vvv)")

	// Add banner options
	motdCmd.Flags().String("title", "Saltbox", "Text to display in the banner")
	motdCmd.Flags().String("type", "peek", "Banner type for boxes (use 'none' to omit box)")
	motdCmd.Flags().String("font", "ivrit", "Font for toilet cli")
	motdCmd.Flags().String("banner-file", "", "Path to a file containing a custom banner to display")
	motdCmd.Flags().String("banner-file-toilet", "", "A string of arguments for toilet when using --banner-file")
}
