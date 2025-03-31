package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/saltyorg/sb-go/motd" // Adjust the import path as needed
	"github.com/spf13/cobra"
)

// Display flags and options
var (
	showDistribution   bool
	showKernel         bool
	showUptime         bool
	showCpuAverages    bool
	showMemory         bool
	showDisk           bool
	showLastLogin      bool
	showSessions       bool
	showProcesses      bool
	showAptStatus      bool
	showRebootRequired bool
	showDocker         bool // New Docker containers flag
	showAll            bool
	bannerTitle        string
	bannerType         string
	bannerFont         string
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
			showDistribution = true
			showKernel = true
			showUptime = true
			showCpuAverages = true
			showMemory = true
			showDisk = true
			showLastLogin = true
			showSessions = true
			showProcesses = true
			showAptStatus = true
			showRebootRequired = true
			showDocker = true
		}

		// Check if at least one flag is enabled
		if !showDistribution && !showKernel && !showUptime && !showCpuAverages &&
			!showMemory && !showDisk && !showLastLogin && !showSessions && !showProcesses &&
			!showAptStatus && !showRebootRequired && !showDocker {
			fmt.Println("Error: No information selected to display.")
			fmt.Println("Please use at least one of the following flags:")
			fmt.Println("  --distro     Show distribution information")
			fmt.Println("  --kernel     Show kernel information")
			fmt.Println("  --uptime     Show uptime information")
			fmt.Println("  --cpu        Show CPU load averages")
			fmt.Println("  --memory     Show memory usage")
			fmt.Println("  --disk       Show disk usage for all partitions")
			fmt.Println("  --login      Show last login information")
			fmt.Println("  --sessions   Show active user sessions")
			fmt.Println("  --processes  Show process count")
			fmt.Println("  --apt        Show apt package status")
			fmt.Println("  --reboot     Show if reboot is required")
			fmt.Println("  --docker     Show Docker container information")
			fmt.Println("  --all        Show all information")
			os.Exit(1)
		}

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
				fmt.Println("Available types:")

				// Print available types in columns
				const numColumns = 4
				for i, bType := range motd.AvailableBannerTypes {
					if i%numColumns == 0 {
						fmt.Println()
					}
					fmt.Printf("  %-16s", bType)
				}
				fmt.Println("\n")
				os.Exit(1)
			}
		}

		// Validate font if specified
		if bannerFont != "" && !motd.IsValidFont(bannerFont) {
			fmt.Println("Error: Invalid font specified:", bannerFont)
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
			fmt.Println("\n")
			os.Exit(1)
		}

		displayMotd()
	},
}

func displayMotd() {
	// Set white color style for keys
	whiteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39")) // ANSI bright white

	// Display banner if title is provided
	if bannerTitle != "" {
		banner := motd.GenerateBanner(bannerTitle, bannerFont, bannerType)
		fmt.Println(banner)
	}

	// Set up info sources with appropriate timeouts and display order
	sources := []motd.InfoSource{
		{Key: "Distribution:", Provider: motd.GetDistributionWithContext, Timeout: 2 * time.Second, Order: 1},
		{Key: "Kernel:", Provider: motd.GetKernelWithContext, Timeout: 1 * time.Second, Order: 2},
		{Key: "Uptime:", Provider: motd.GetUptimeWithContext, Timeout: 1 * time.Second, Order: 3},
		{Key: "Load Averages:", Provider: motd.GetCpuAveragesWithContext, Timeout: 1 * time.Second, Order: 4},
		{Key: "Last login:", Provider: motd.GetLastLoginWithContext, Timeout: 3 * time.Second, Order: 6},
		{Key: "User Sessions:", Provider: motd.GetUserSessionsWithContext, Timeout: 1 * time.Second, Order: 7},
		{Key: "Processes:", Provider: motd.GetProcessCountWithContext, Timeout: 2 * time.Second, Order: 8},
		{Key: "Package Status:", Provider: motd.GetAptStatusWithContext, Timeout: 5 * time.Second, Order: 9},
		{Key: "Reboot Status:", Provider: motd.GetRebootRequiredWithContext, Timeout: 2 * time.Second, Order: 10},
	}

	// Filter sources based on enabled flags
	var activeSources []motd.InfoSource
	flags := map[string]bool{
		"Distribution:":   showDistribution,
		"Kernel:":         showKernel,
		"Uptime:":         showUptime,
		"Load Averages:":  showCpuAverages,
		"Last login:":     showLastLogin,
		"User Sessions:":  showSessions,
		"Processes:":      showProcesses,
		"Package Status:": showAptStatus,
		"Reboot Status:":  showRebootRequired,
	}

	for _, source := range sources {
		if enabled, exists := flags[source.Key]; exists && enabled {
			activeSources = append(activeSources, source)
		}
	}

	// Get system information in parallel
	results := motd.GetSystemInfo(activeSources)

	// Calculate spacing for display
	maxKeyLen := 0
	for _, result := range results {
		if len(result.Key) > maxKeyLen {
			maxKeyLen = len(result.Key)
		}
	}

	// Check for maxKeyLen to ensure consistent spacing
	if showDocker && len("Docker:") > maxKeyLen {
		maxKeyLen = len("Docker:")
	}
	if showMemory && len("Memory Usage:") > maxKeyLen {
		maxKeyLen = len("Memory Usage:")
	}
	if showDisk && len("Disk Usage:") > maxKeyLen {
		maxKeyLen = len("Disk Usage:")
	}

	// Add additional spacing (2 spaces)
	spacing := maxKeyLen + 2

	// Display results with consistently styled keys
	for _, result := range results {
		// Style the key in white and add proper spacing
		styledKey := whiteStyle.Render(result.Key)
		paddingLength := spacing - len(result.Key)
		padding := strings.Repeat(" ", paddingLength)

		fmt.Printf("%s%s%s\n", styledKey, padding, result.Value)
	}

	// Handle Docker containers display (multiline)
	if showDocker {
		// Set up a separate source for Docker info
		dockerSource := motd.InfoSource{
			Key:      "Docker:",
			Provider: motd.GetDockerInfoWithContext,
			Timeout:  5 * time.Second,
			Order:    0,
		}

		// Get Docker info
		multilineResult := motd.GetMultilineSystemInfo(dockerSource)

		if len(multilineResult.Values) > 0 {
			// First line is the summary
			if len(multilineResult.Values) == 1 {
				// Single line result (likely an error or "no containers")
				styledKey := whiteStyle.Render(multilineResult.Key)
				paddingLength := spacing - len(multilineResult.Key)
				padding := strings.Repeat(" ", paddingLength)
				fmt.Printf("%s%s%s\n", styledKey, padding, multilineResult.Values[0])
			} else {
				// Multi-line result
				// Print the first line (summary) with the styled label
				styledKey := whiteStyle.Render(multilineResult.Key)
				paddingLength := spacing - len(multilineResult.Key)
				padding := strings.Repeat(" ", paddingLength)
				fmt.Printf("%s%s%s\n", styledKey, padding, multilineResult.Values[0])

				// Print container details with consistent indentation
				containerPadding := strings.Repeat(" ", spacing)
				for i := 1; i < len(multilineResult.Values); i++ {
					fmt.Printf("%s%s\n", containerPadding, multilineResult.Values[i])
				}
			}
		} else {
			styledKey := whiteStyle.Render("Docker:")
			paddingLength := spacing - len("Docker:")
			padding := strings.Repeat(" ", paddingLength)
			fmt.Printf("%s%s%s\n", styledKey, padding, "No container information available")
		}
	}

	// Handle memory usage with bar (special case)
	if showMemory {
		// Get memory info with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		memoryUsage := motd.GetMemoryInfoParallel(ctx)

		if len(memoryUsage) == 1 && memoryUsage[0] == "Not available" {
			styledKey := whiteStyle.Render("Memory Usage:")
			paddingLength := spacing - len("Memory Usage:")
			padding := strings.Repeat(" ", paddingLength)
			fmt.Printf("%s%s%s\n", styledKey, padding, "Not available")
		} else if len(memoryUsage) >= 2 {
			// First line contains the memory stats - print with the styled "Memory Usage:" key
			styledKey := whiteStyle.Render("Memory Usage:")
			paddingLength := spacing - len("Memory Usage:")
			padding := strings.Repeat(" ", paddingLength)
			fmt.Printf("%s%s%s\n", styledKey, padding, memoryUsage[0])

			// Print the bar with proper spacing
			fmt.Printf("%s%s\n", strings.Repeat(" ", spacing), memoryUsage[1])
		} else {
			styledKey := whiteStyle.Render("Memory Usage:")
			paddingLength := spacing - len("Memory Usage:")
			padding := strings.Repeat(" ", paddingLength)
			fmt.Printf("%s%s%s\n", styledKey, padding, "Memory information unavailable")
		}
	}

	// Handle disk usage separately (special case)
	if showDisk {
		// Get disk info with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		diskUsage := motd.GetDiskInfoParallel(ctx)

		if len(diskUsage) == 1 && diskUsage[0] == "Not available" {
			styledKey := whiteStyle.Render("Disk Usage:")
			paddingLength := spacing - len("Disk Usage:")
			padding := strings.Repeat(" ", paddingLength)
			fmt.Printf("%s%s%s\n", styledKey, padding, "Not available")
		} else if len(diskUsage) > 0 {
			// First line contains the first partition info - print with the styled "Disk Usage:" key
			styledKey := whiteStyle.Render("Disk Usage:")
			paddingLength := spacing - len("Disk Usage:")
			padding := strings.Repeat(" ", paddingLength)
			fmt.Printf("%s%s%s\n", styledKey, padding, diskUsage[0])

			// Print the bar for the first partition
			if len(diskUsage) > 1 {
				fmt.Printf("%s%s\n", strings.Repeat(" ", spacing), diskUsage[1])
			}

			// Print the rest of the partitions (2 lines per partition)
			for i := 2; i < len(diskUsage); i += 2 {
				// Print partition info line with proper spacing
				fmt.Printf("%s%s\n", strings.Repeat(" ", spacing), diskUsage[i])

				// Print bar line if available
				if i+1 < len(diskUsage) {
					fmt.Printf("%s%s\n", strings.Repeat(" ", spacing), diskUsage[i+1])
				}
			}
		} else {
			styledKey := whiteStyle.Render("Disk Usage:")
			paddingLength := spacing - len("Disk Usage:")
			padding := strings.Repeat(" ", paddingLength)
			fmt.Printf("%s%s%s\n", styledKey, padding, "No disk information available")
		}
	}
}

func init() {
	rootCmd.AddCommand(motdCmd)

	// Define flags for enabling/disabling components (all default to false - opt-in)
	motdCmd.Flags().BoolVar(&showDistribution, "distro", false, "Show distribution information")
	motdCmd.Flags().BoolVar(&showKernel, "kernel", false, "Show kernel information")
	motdCmd.Flags().BoolVar(&showUptime, "uptime", false, "Show uptime information")
	motdCmd.Flags().BoolVar(&showCpuAverages, "cpu", false, "Show CPU load averages")
	motdCmd.Flags().BoolVar(&showMemory, "memory", false, "Show memory usage")
	motdCmd.Flags().BoolVar(&showDisk, "disk", false, "Show disk usage for all partitions")
	motdCmd.Flags().BoolVar(&showLastLogin, "login", false, "Show last login information")
	motdCmd.Flags().BoolVar(&showSessions, "sessions", false, "Show active user sessions")
	motdCmd.Flags().BoolVar(&showProcesses, "processes", false, "Show process count")
	motdCmd.Flags().BoolVar(&showAptStatus, "apt", false, "Show apt package status")
	motdCmd.Flags().BoolVar(&showRebootRequired, "reboot", false, "Show if reboot is required")
	motdCmd.Flags().BoolVar(&showDocker, "docker", false, "Show Docker container information")

	// Add a flag to show all information
	motdCmd.Flags().BoolVar(&showAll, "all", false, "Show all information")

	// Add banner options
	motdCmd.Flags().StringVar(&bannerTitle, "title", "Saltbox", "Text to display in the banner")
	motdCmd.Flags().StringVar(&bannerType, "type", "dog", "Banner type for boxes (use 'none' for no box)")
	motdCmd.Flags().StringVar(&bannerFont, "font", "ivrit", "Font for toilet")
}
