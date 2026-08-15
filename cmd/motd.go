package cmd

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

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
	showSystemd          bool
	showTraefik          bool
	showUptime           bool
	shareMode            bool
	generateConfig       bool
	bannerFile           string
	bannerFileToiletArgs string
	bannerFont           string
	bannerFontExplicit   bool
	bannerTitle          string
	bannerType           string
	verbosity            int
}

// motdCmd represents the motd command
func newMOTDCommand() *cobra.Command {
	config := &motdConfig{}
	motdCmd := &cobra.Command{
		Use:   "motd",
		Short: "Display system information",
		Long: `Displays system information including Ubuntu distribution version,
kernel version, system uptime, CPU load, memory usage, disk usage,
last login, user sessions, process information, and system update status based on flags provided.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			config.bannerFontExplicit = cmd.Flags().Changed("font")
			return runMotdCommand(cmd.Context(), config)
		},
	}
	motdCmd.Flags().BoolVar(&config.showAll, "all", false, "Show all information")
	motdCmd.Flags().BoolVar(&config.showAptStatus, "apt", false, "Show apt package status")
	motdCmd.Flags().BoolVar(&config.showCpuAverages, "cpu", false, "Show CPU load averages")
	motdCmd.Flags().BoolVar(&config.showCPU, "cpu-info", false, "Show CPU model and core count information")
	motdCmd.Flags().BoolVar(&config.showDisk, "disk", false, "Show disk usage for all partitions")
	motdCmd.Flags().BoolVar(&config.showDistribution, "distro", false, "Show distribution information")
	motdCmd.Flags().BoolVar(&config.showDocker, "docker", false, "Show Docker container information")
	motdCmd.Flags().BoolVar(&config.showEmby, "emby", false, "Show Emby streaming information")
	motdCmd.Flags().BoolVar(&config.showGPU, "gpu", false, "Show GPU information")
	motdCmd.Flags().BoolVar(&config.showJellyfin, "jellyfin", false, "Show Jellyfin streaming information")
	motdCmd.Flags().BoolVar(&config.showKernel, "kernel", false, "Show kernel information")
	motdCmd.Flags().BoolVar(&config.showLastLogin, "login", false, "Show last login information")
	motdCmd.Flags().BoolVar(&config.showMemory, "memory", false, "Show memory usage")
	motdCmd.Flags().BoolVar(&config.showNzbget, "nzbget", false, "Show NZBGet queue information")
	motdCmd.Flags().BoolVar(&config.showPlex, "plex", false, "Show Plex streaming information")
	motdCmd.Flags().BoolVar(&config.showProcesses, "processes", false, "Show process count")
	motdCmd.Flags().BoolVar(&config.showQbittorrent, "qbittorrent", false, "Show qBittorrent queue information")
	motdCmd.Flags().BoolVar(&config.showQueues, "queues", false, "Show download queue information from Sonarr, Radarr, etc.")
	motdCmd.Flags().BoolVar(&config.showRebootRequired, "reboot", false, "Show if reboot is required")
	motdCmd.Flags().BoolVar(&config.showRtorrent, "rtorrent", false, "Show rTorrent queue information")
	motdCmd.Flags().BoolVar(&config.showSabnzbd, "sabnzbd", false, "Show SABnzbd queue information")
	motdCmd.Flags().BoolVar(&config.showSessions, "sessions", false, "Show active user sessions")
	motdCmd.Flags().BoolVar(&config.showSystemd, "systemd", false, "Show systemd services status")
	motdCmd.Flags().BoolVar(&config.showTraefik, "traefik", false, "Show Traefik router status information")
	motdCmd.Flags().BoolVar(&config.showUptime, "uptime", false, "Show uptime information")
	motdCmd.Flags().CountVarP(&config.verbosity, "verbose", "v", "Increase verbosity level (can be used multiple times, e.g. -vvv)")
	motdCmd.Flags().BoolVar(&config.shareMode, "share", false, "Obscure sensitive information like IP addresses for sharing screenshots")
	motdCmd.Flags().BoolVar(&config.generateConfig, "generate-config", false, "Print an example MOTD configuration file to stdout")
	motdCmd.Flags().StringVar(&config.bannerTitle, "title", "Saltbox", "Text to display in the banner")
	motdCmd.Flags().StringVar(&config.bannerType, "type", "peek", "Banner type for boxes (use 'none' to omit box)")
	motdCmd.Flags().StringVar(&config.bannerFont, "font", "ivrit", "Font for toilet cli")
	motdCmd.Flags().StringVar(&config.bannerFile, "banner-file", "", "Path to a file containing a custom banner to display")
	motdCmd.Flags().StringVar(&config.bannerFileToiletArgs, "banner-file-toilet", "", "A string of arguments for toilet when using --banner-file")
	return motdCmd
}

// runMotdCommand handles the main logic for the motd command
func runMotdCommand(ctx context.Context, mcfg *motdConfig) error {
	// Handle --generate-config flag
	if mcfg.generateConfig {
		config, err := motd.GenerateExampleConfig()
		if err != nil {
			return fmt.Errorf("failed to generate example config: %w", err)
		}
		fmt.Print(config)
		return nil
	}

	// Initialize custom colors from config if available
	motd.InitializeColors()

	// If --all flag is used, enable everything
	if mcfg.showAll {
		mcfg.showAptStatus = true
		mcfg.showCPU = true
		mcfg.showCpuAverages = true
		mcfg.showDisk = true
		mcfg.showDistribution = true
		mcfg.showDocker = true
		mcfg.showEmby = true
		mcfg.showGPU = true
		mcfg.showJellyfin = true
		mcfg.showKernel = true
		mcfg.showLastLogin = true
		mcfg.showMemory = true
		mcfg.showNzbget = true
		mcfg.showPlex = true
		mcfg.showProcesses = true
		mcfg.showQbittorrent = true
		mcfg.showQueues = true
		mcfg.showRebootRequired = true
		mcfg.showRtorrent = true
		mcfg.showSabnzbd = true
		mcfg.showSessions = true
		mcfg.showSystemd = true
		mcfg.showTraefik = true
		mcfg.showUptime = true
	}

	// Check if at least one flag is enabled
	if !mcfg.showAptStatus && !mcfg.showCPU && !mcfg.showCpuAverages && !mcfg.showDisk && !mcfg.showDistribution &&
		!mcfg.showDocker && !mcfg.showEmby && !mcfg.showGPU && !mcfg.showJellyfin && !mcfg.showKernel && !mcfg.showLastLogin &&
		!mcfg.showMemory && !mcfg.showNzbget && !mcfg.showPlex && !mcfg.showProcesses && !mcfg.showQbittorrent &&
		!mcfg.showQueues && !mcfg.showRebootRequired && !mcfg.showRtorrent && !mcfg.showSabnzbd && !mcfg.showSessions &&
		!mcfg.showSystemd && !mcfg.showTraefik && !mcfg.showUptime {
		return fmt.Errorf("no information selected to display (use --all or specific flags)")
	}

	shouldRenderBanner := mcfg.bannerFile == "" && mcfg.bannerTitle != ""

	// Validate banner type if specified and banner will be rendered.
	if shouldRenderBanner && mcfg.bannerType != "" && mcfg.bannerType != "none" {
		validType := slices.Contains(motd.AvailableBannerTypes, mcfg.bannerType)

		if !validType {
			var availableTypes strings.Builder
			availableTypes.WriteString("\nAvailable types:\n")

			// Print available types in columns
			const numColumns = 4
			for i, bType := range motd.AvailableBannerTypes {
				if i%numColumns == 0 {
					availableTypes.WriteString("\n")
				}
				availableTypes.WriteString(fmt.Sprintf("  %-16s", bType))
			}
			availableTypes.WriteString("\n")

			return fmt.Errorf("invalid banner type specified: %s%s", mcfg.bannerType, availableTypes.String())
		}
	}

	// Validate font if specified and banner will be rendered.
	if shouldRenderBanner && mcfg.bannerFont != "" && mcfg.bannerFontExplicit && !motd.IsValidFont(mcfg.bannerFont) {
		var availableFonts strings.Builder
		availableFonts.WriteString("\nAvailable fonts (from /usr/share/figlet):\n")

		// Print available fonts in columns
		fonts := motd.ListAvailableFonts()
		const numColumns = 4
		for i, font := range fonts {
			if i%numColumns == 0 {
				availableFonts.WriteString("\n")
			}
			availableFonts.WriteString(fmt.Sprintf("  %-16s", font))
		}
		availableFonts.WriteString("\n")

		return fmt.Errorf("invalid font specified: %s%s", mcfg.bannerFont, availableFonts.String())
	}

	return displayMotd(ctx, mcfg, mcfg.verbosity > 0)
}

func displayMotd(ctx context.Context, config *motdConfig, verbose bool) error {
	ctx = motd.WithShareMode(ctx, config.shareMode)

	// Display a banner from a file if provided. This takes precedence.
	if config.bannerFile != "" {
		content, err := os.ReadFile(config.bannerFile)
		if err != nil {
			return fmt.Errorf("could not read banner file '%s': %w", config.bannerFile, err)
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

	// Set up info sources with display order
	sources := []motd.InfoSource{
		{Key: "Distribution:", Provider: motd.GetDistributionWithContext, Order: 1},
		{Key: "Kernel:", Provider: motd.GetKernelWithContext, Order: 2},
		{Key: "Uptime:", Provider: motd.GetUptimeWithContext, Order: 3},
		{Key: "Load Averages:", Provider: motd.GetCpuAveragesWithContext, Order: 4},
		{Key: "Processes:", Provider: motd.GetProcessCountWithContext, Order: 5},
		{Key: "CPU:", Provider: motd.GetCpuInfoWithContext, Order: 6},
		{Key: "GPU:", Provider: motd.GetGpuInfoWithContext, Order: 7},
		{Key: "Memory Usage:", Provider: motd.GetMemoryInfoWithContext, Order: 8},
		{Key: "Package Status:", Provider: motd.GetAptStatusWithContext, Order: 9},
		{Key: "Reboot Status:", Provider: motd.GetRebootRequiredWithContext, Order: 10},
		{Key: "User Sessions:", Provider: motd.GetUserSessionsWithContext, Order: 11},
		{Key: "Last login:", Provider: motd.GetLastLoginWithContext, Order: 12},
		{Key: "Disk Usage:", Provider: motd.GetDiskInfoWithContext, Order: 13},
		{Key: "Services:", Provider: motd.GetSystemdServicesInfoWithContext, Order: 14},
		{Key: "Docker:", Provider: motd.GetDockerInfoWithContext, Order: 15},
		{Key: "Traefik:", Provider: motd.GetTraefikInfoWithContext, Order: 16},
		{Key: "Download Queues:", Provider: motd.GetQueueInfoWithContext, Order: 17},
		{Key: "SABnzbd:", Provider: motd.GetSabnzbdInfoWithContext, Order: 18},
		{Key: "NZBGet:", Provider: motd.GetNzbgetInfoWithContext, Order: 19},
		{Key: "qBittorrent:", Provider: motd.GetQbittorrentInfoWithContext, Order: 20},
		{Key: "rTorrent:", Provider: motd.GetRtorrentInfoWithContext, Order: 21},
		{Key: "Plex:", Provider: motd.GetPlexInfoWithContext, Order: 22},
		{Key: "Emby:", Provider: motd.GetEmbyInfoWithContext, Order: 23},
		{Key: "Jellyfin:", Provider: motd.GetJellyfinInfoWithContext, Order: 24},
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
		"Services:":        config.showSystemd,
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
	results := motd.GetSystemInfo(ctx, activeSources, verbose)

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

	return nil
}

func addMOTDCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newMOTDCommand())
}
