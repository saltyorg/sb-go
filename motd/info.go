package motd

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// GetDistribution returns the Ubuntu distribution version with codename
func GetDistribution() string {
	distroInfo := ExecCommand("lsb_release", "-ds")
	codename := ExecCommand("lsb_release", "-cs")

	if codename != "" && distroInfo != "Not available" {
		distroInfo = distroInfo + " (" + codename + ")"
	}
	return distroInfo
}

// GetKernel returns the kernel version
func GetKernel() string {
	return ExecCommand("uname", "-r")
}

// GetUptime returns the system uptime
func GetUptime() string {
	uptimeInfo := ExecCommand("uptime", "-p")
	if uptimeInfo != "Not available" {
		// Remove "up " prefix from uptime
		uptimeInfo = strings.TrimPrefix(uptimeInfo, "up ")
	}
	return uptimeInfo
}

// GetCpuAverages returns the system load averages with styled output
func GetCpuAverages() string {
	// Try to read from /proc/loadavg first
	content, err := os.ReadFile("/proc/loadavg")
	if err == nil {
		fields := strings.Fields(string(content))
		if len(fields) >= 3 {
			return fmt.Sprintf("%s: %s | %s: %s | %s: %s",
				DefaultStyle.Render("1 min"), GreenStyle.Render(fields[0]),
				DefaultStyle.Render("5 min"), GreenStyle.Render(fields[1]),
				DefaultStyle.Render("15 min"), GreenStyle.Render(fields[2]),
			)
		}
	}

	// Fallback to uptime command if /proc/loadavg can't be read
	loadInfo := ExecCommand("uptime")
	if loadInfo != "Not available" {
		// Extract load averages from uptime output
		// Typical output: "... load average: 0.00, 0.01, 0.05"
		if idx := strings.Index(loadInfo, "load average:"); idx != -1 {
			loadPart := loadInfo[idx+14:]
			loads := strings.Split(loadPart, ", ")
			if len(loads) >= 3 {
				return fmt.Sprintf("%s: %s | %s: %s | %s: %s",
					DefaultStyle.Render("1 min"), GreenStyle.Render(strings.TrimSpace(loads[0])),
					DefaultStyle.Render("5 min"), GreenStyle.Render(strings.TrimSpace(loads[1])),
					DefaultStyle.Render("15 min"), GreenStyle.Render(strings.TrimSpace(loads[2])),
				)
			}
		}
	}

	return DefaultStyle.Render("Not available")
}

// GetMemoryUsage returns the system memory usage with a visual bar
func GetMemoryUsage() []string {
	// Constants for memory usage bar
	const (
		maxUsageThreshold = 90 // Percentage at which memory usage is considered high
		barWidth          = 50 // Width of the usage bar in characters
	)

	// Try to read from /proc/meminfo first
	content, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return []string{DefaultStyle.Render("Not available")}
	}

	// Parse the meminfo content
	memInfo := make(map[string]uint64)
	lines := strings.Split(string(content), "\n")

	re := regexp.MustCompile(`^(\S+):\s+(\d+)`)
	for _, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) == 3 {
			key := matches[1]
			valueStr := matches[2]
			value, err := strconv.ParseUint(valueStr, 10, 64)
			if err == nil {
				memInfo[key] = value
			}
		}
	}

	// Check if we have the required fields
	if memTotal, ok := memInfo["MemTotal"]; ok {
		var memAvailable uint64
		if avail, ok := memInfo["MemAvailable"]; ok {
			// MemAvailable is more accurate for modern kernels
			memAvailable = avail
		} else if free, ok := memInfo["MemFree"]; ok {
			// Fallback to MemFree for older kernels
			memAvailable = free
		} else {
			return []string{DefaultStyle.Render("Not available")}
		}

		memUsed := memTotal - memAvailable

		// Calculate usage percentage
		usagePercent := int((float64(memUsed) / float64(memTotal)) * 100)

		// Always format total memory in GB
		totalGB := float64(memTotal) / 1024.0 / 1024.0
		totalFormatted := fmt.Sprintf("%3.0fG", totalGB) // Right-aligned, 3 digit integer + G

		// Calculate the bar
		usedWidth := (usagePercent * barWidth) / 100
		unusedWidth := barWidth - usedWidth

		// Choose color based on usage threshold
		var usedBarStyle lipgloss.Style
		if usagePercent >= maxUsageThreshold {
			usedBarStyle = RedStyle
		} else {
			usedBarStyle = GreenStyle
		}

		// Create the usage bar
		usedBar := strings.Repeat("=", usedWidth)
		unusedBar := strings.Repeat("=", unusedWidth)

		// Style the bars
		styledUsedBar := usedBarStyle.Render(usedBar)
		styledUnusedBar := DimStyle.Render(unusedBar)

		// Create the complete bar
		completeBar := fmt.Sprintf("[%s%s]", styledUsedBar, styledUnusedBar)

		// Format the info line - use empty string for mountPoint to match disk usage format
		infoLine := fmt.Sprintf("%-31s%3d%% used out of %3s", "", usagePercent, totalFormatted)
		infoLine = DefaultStyle.Render(infoLine) // Apply default style

		return []string{infoLine, completeBar}
	}

	// Fallback to free command if parsing /proc/meminfo failed
	freeOutput := ExecCommand("free", "-m") // Get output in MB
	if freeOutput != "Not available" {
		lines := strings.Split(freeOutput, "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 4 {
				if total, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
					if used, err := strconv.ParseUint(fields[2], 10, 64); err == nil {
						// Calculate usage percentage
						usagePercent := int((float64(used) / float64(total)) * 100)

						// Always format total memory in GB
						totalGB := float64(total) / 1024.0
						totalFormatted := fmt.Sprintf("%3.0fG", totalGB) // Right-aligned, 3 digit integer + G

						// Calculate the bar
						usedWidth := (usagePercent * barWidth) / 100
						unusedWidth := barWidth - usedWidth

						// Choose color based on usage threshold
						var usedBarStyle lipgloss.Style
						if usagePercent >= maxUsageThreshold {
							usedBarStyle = RedStyle
						} else {
							usedBarStyle = GreenStyle
						}

						// Create the usage bar
						usedBar := strings.Repeat("=", usedWidth)
						unusedBar := strings.Repeat("=", unusedWidth)

						// Style the bars
						styledUsedBar := usedBarStyle.Render(usedBar)
						styledUnusedBar := DimStyle.Render(unusedBar)

						// Create the complete bar
						completeBar := fmt.Sprintf("[%s%s]", styledUsedBar, styledUnusedBar)

						// Format the info line - use empty string for mountPoint to match disk usage format
						infoLine := fmt.Sprintf("%-31s%3d%% used out of %3s", "", usagePercent, totalFormatted)
						infoLine = DefaultStyle.Render(infoLine) // Apply default style

						return []string{infoLine, completeBar}
					}
				}
			}
		}
	}

	return []string{DefaultStyle.Render("Not available")}
}

// GetDiskUsage returns the disk usage for all real partitions with visual bars
func GetDiskUsage() []string {
	// Constants for disk usage bar
	const (
		maxUsageThreshold = 90 // Percentage at which disk usage is considered high
		barWidth          = 50 // Width of the usage bar in characters
	)

	// Style definitions using lipgloss
	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("40")).Bold(true) // ANSI 16 green
	redStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("160")).Bold(true)  // ANSI 16 red
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	// Run df command to get disk usage with the proper exclusions
	dfOutput := ExecCommand("df", "-H", "-x", "tmpfs", "-x", "overlay", "-x", "fuse.mergerfs", "-x", "fuse.rclone",
		"--output=target,pcent,size")
	if dfOutput == "Not available" {
		return []string{"Not available"}
	}

	// Process df output
	lines := strings.Split(dfOutput, "\n")
	if len(lines) <= 1 { // If there's only one line (the header), then no valid partitions
		return []string{"No valid disk partitions found"}
	}

	// Skip the header line
	lines = lines[1:]
	var results []string
	var partitions []struct {
		mountPoint   string
		usagePercent int
		size         string
		formattedBar string
	}

	// Process each partition
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		// Skip specific mount points
		mountPoint := fields[0]
		if strings.HasPrefix(mountPoint, "/dev") ||
			strings.HasPrefix(mountPoint, "/sys") ||
			strings.HasPrefix(mountPoint, "/proc") ||
			strings.HasPrefix(mountPoint, "/run") {
			continue
		}

		// Get percentage (remove the '%' character)
		usagePercentStr := strings.TrimSuffix(fields[1], "%")
		usagePercent, err := strconv.Atoi(usagePercentStr)
		if err != nil {
			continue
		}

		// Get size
		size := fields[2]

		// Calculate the bar
		usedWidth := (usagePercent * barWidth) / 100
		unusedWidth := barWidth - usedWidth

		// Choose color based on usage threshold
		var usedBarStyle lipgloss.Style
		if usagePercent >= maxUsageThreshold {
			usedBarStyle = redStyle
		} else {
			usedBarStyle = greenStyle
		}

		// Create the usage bar
		usedBar := strings.Repeat("=", usedWidth)
		unusedBar := strings.Repeat("=", unusedWidth)

		// Style the bars
		styledUsedBar := usedBarStyle.Render(usedBar)
		styledUnusedBar := dimStyle.Render(unusedBar)

		// Create the complete bar
		completeBar := fmt.Sprintf("[%s%s]", styledUsedBar, styledUnusedBar)

		// Add to partitions slice
		partitions = append(partitions, struct {
			mountPoint   string
			usagePercent int
			size         string
			formattedBar string
		}{
			mountPoint:   mountPoint,
			usagePercent: usagePercent,
			size:         size,
			formattedBar: completeBar,
		})
	}

	if len(partitions) == 0 {
		return []string{"No valid disk partitions found"}
	}

	// Format the results
	for i, p := range partitions {
		// For the first partition, format the info line
		if i == 0 {
			// Will be combined with the key in the display function
			infoLine := fmt.Sprintf("%-31s%3d%% used out of %4s", p.mountPoint, p.usagePercent, p.size)
			results = append(results, infoLine)
		} else {
			// For subsequent partitions, add proper spacing for alignment
			infoLine := fmt.Sprintf("%-31s%3d%% used out of %4s", p.mountPoint, p.usagePercent, p.size)
			results = append(results, infoLine)
		}
		// Add the bar line
		results = append(results, p.formattedBar)
	}

	return results
}

// GetLastLogin returns the last login information
func GetLastLogin() string {
	// Try last command to get the most recent login
	lastOutput := ExecCommand("last", "-1")
	if lastOutput != "Not available" && lastOutput != "" {
		// First check if "still logged in" is present
		stillLoggedIn := strings.Contains(lastOutput, "still logged in")

		fields := strings.Fields(lastOutput)
		if len(fields) >= 5 {
			user := fields[0]

			// Extract from IP
			fromIP := fields[2]

			// Initialize variables for the time components
			var day, month, date, time, logoutTime, duration string

			// Find the start of the date/time information
			timeIndex := 3 // Default position for date/time components

			// Check if the IP address actually contains an IP (sometimes it might not)
			if !strings.Contains(fromIP, ".") && !strings.Contains(fromIP, ":") {
				timeIndex = 2
				fromIP = "local"
			}

			// Extract date and time if we have enough fields
			if len(fields) >= timeIndex+4 {
				day = fields[timeIndex]
				month = fields[timeIndex+1]
				date = fields[timeIndex+2]
				time = fields[timeIndex+3]

				// For entries that are still logged in
				if stillLoggedIn {
					return fmt.Sprintf("%s at %s %s %s %s (still logged in) from %s",
						user, day, month, date, time, fromIP)
				}

				// For entries that have logged out, find the logout time and duration
				dashIndex := -1
				for i, field := range fields {
					if field == "-" {
						dashIndex = i
						break
					}
				}

				if dashIndex != -1 && dashIndex+1 < len(fields) {
					logoutTime = fields[dashIndex+1]

					// Find the duration which is in parentheses
					for i, field := range fields {
						if strings.HasPrefix(field, "(") {
							// Start building duration from this field
							duration = strings.TrimPrefix(field, "(")

							// May need to join with the next field if it's a multipart duration
							if !strings.HasSuffix(duration, ")") && i+1 < len(fields) {
								duration += " " + strings.TrimSuffix(fields[i+1], ")")
							} else {
								duration = strings.TrimSuffix(duration, ")")
							}
							break
						}
					}

					return fmt.Sprintf("%s at %s %s %s %s until %s (%s) from %s",
						user, day, month, date, time, logoutTime, duration, fromIP)
				}

				// If we couldn't parse the logout info but have login info
				return fmt.Sprintf("%s at %s %s %s %s from %s",
					user, day, month, date, time, fromIP)
			}
		}
	}

	// Fallback methods similar to before...
	lastlogOutput := ExecCommand("lastlog", "-u", "root")
	if lastlogOutput != "Not available" && lastlogOutput != "" {
		lines := strings.Split(lastlogOutput, "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 4 {
				user := fields[0]
				loginInfo := strings.Join(fields[3:], " ")
				return fmt.Sprintf("%s %s", user, loginInfo)
			}
		}
	}

	// Additional fallback with the who command
	whoOutput := ExecCommand("who", "-u")
	if whoOutput != "Not available" && whoOutput != "" {
		lines := strings.Split(whoOutput, "\n")
		if len(lines) > 0 {
			fields := strings.Fields(lines[0])
			if len(fields) >= 5 {
				user := fields[0]
				month := fields[2]
				day := fields[3]
				time := fields[4]

				return fmt.Sprintf("%s at %s %s %s (still logged in)", user, month, day, time)
			}
		}
	}

	return "No recent logins"
}

// GetUserSessions returns the number of active user sessions
func GetUserSessions() string {
	// Use the 'who' command to get user sessions
	whoOutput := ExecCommand("who")
	if whoOutput == "Not available" || whoOutput == "" {
		return "No active sessions"
	}

	lines := strings.Split(whoOutput, "\n")
	count := len(lines)

	if count == 1 {
		return "1 active session"
	} else {
		return fmt.Sprintf("%d active sessions", count)
	}
}

// GetProcessCount returns the number of running processes
func GetProcessCount() string {
	// Method 1: Count directories in /proc that are numeric
	entries, err := os.ReadDir("/proc")
	if err == nil {
		count := 0
		for _, entry := range entries {
			if entry.IsDir() {
				// Check if the directory name is a number (process ID)
				if _, err := strconv.Atoi(entry.Name()); err == nil {
					count++
				}
			}
		}
		return fmt.Sprintf("%d running processes", count)
	}

	// Method 2: Use ps command (fallback)
	psOutput := ExecCommand("ps", "ax")
	if psOutput != "Not available" {
		lines := strings.Split(psOutput, "\n")
		// Subtract 1 for the header line
		count := len(lines) - 1
		if count <= 0 {
			count = 0
		}
		return fmt.Sprintf("%d running processes", count)
	}

	return "Not available"
}

// GetAptStatus returns the apt package status
func GetAptStatus() string {
	// Check the updates-available file, which is updated by the daily apt update
	updatesFile := "/var/lib/update-notifier/updates-available"
	data, err := os.ReadFile(updatesFile)

	if err == nil && len(data) > 0 {
		content := string(data)
		lines := strings.Split(content, "\n")

		// Look specifically for the line with "updates can be applied immediately"
		// that doesn't mention ESM
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, "updates can be applied immediately") &&
				!strings.Contains(trimmed, "ESM") &&
				!strings.Contains(trimmed, "esm") {
				// Extract just the main update count message, removing any instruction text
				if idx := strings.Index(trimmed, "."); idx != -1 {
					return trimmed[:idx+1] // Include the period
				}
				return trimmed
			}
		}

		// If we found no updates but the file exists, check if it explicitly says system is up to date
		for _, line := range lines {
			if strings.Contains(line, "up to date") || strings.Contains(line, "Up to date") {
				return "System is up to date"
			}
		}
	}

	// Fallback to apt-check command
	output := ExecCommand("/usr/lib/update-notifier/apt-check", "--human-readable", "--no-esm-messages")
	if output != "Not available" && output != "" {
		lines := strings.Split(output, "\n")

		// First look for the regular update line
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, "packages can be updated") &&
				!strings.Contains(trimmed, "ESM") &&
				!strings.Contains(trimmed, "esm") {
				return trimmed
			}
		}

		// If no specific update line found, use any non-ESM line that mentions updates
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" &&
				!strings.Contains(trimmed, "ESM") &&
				!strings.Contains(trimmed, "esm") &&
				(strings.Contains(trimmed, "update") || strings.Contains(trimmed, "package")) {
				return trimmed
			}
		}
	}

	// Additional fallback using apt list
	output = ExecCommand("apt", "list", "--upgradable")
	if output != "Not available" && output != "" {
		lines := strings.Split(output, "\n")
		if len(lines) > 1 {
			updateCount := 0
			for _, line := range lines {
				if strings.Contains(line, "/") && !strings.Contains(line, "Listing") {
					updateCount++
				}
			}

			if updateCount > 0 {
				return fmt.Sprintf("%d updates can be applied immediately", updateCount)
			}
		}
	}

	// If we can't determine the status, return a neutral message
	return "Update status unknown"
}

// GetRebootRequired checks if a system reboot is required
func GetRebootRequired() string {
	// Method 1: Go native implementation
	// Check if the reboot-required file exists
	rebootFile := "/var/run/reboot-required"
	_, err := os.Stat(rebootFile)
	if err == nil {
		// File exists, a reboot is required

		// Try to get the specific packages requiring reboot
		pkgFile := "/var/run/reboot-required.pkgs"
		pkgData, err := os.ReadFile(pkgFile)
		if err == nil && len(pkgData) > 0 {
			pkgs := strings.Split(string(pkgData), "\n")
			// Filter out empty lines
			var validPkgs []string
			for _, pkg := range pkgs {
				if pkg != "" {
					validPkgs = append(validPkgs, pkg)
				}
			}

			if len(validPkgs) > 0 {
				if len(validPkgs) == 1 {
					return fmt.Sprintf("Reboot required (package: %s)", validPkgs[0])
				} else {
					return fmt.Sprintf("Reboot required (%d packages)", len(validPkgs))
				}
			}
		}

		// If we couldn't get package details, just return that a reboot is required
		return "Reboot required"
	}

	// Method 2: Fallback to the update-motd script
	output := ExecCommand("/usr/lib/update-notifier/update-motd-reboot-required")
	if output != "Not available" && output != "" {
		return strings.TrimSpace(output)
	}

	// If no reboot is required or we can't determine
	return "No reboot required"
}

// GetCpuInfo returns information about the CPU model and core count
func GetCpuInfo() string {
	// Try to read from /proc/cpuinfo
	content, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return DefaultStyle.Render("Not available")
	}

	// Parse the cpuinfo content
	cpuInfo := string(content)
	lines := strings.Split(cpuInfo, "\n")

	modelName := ""
	cpuCores := 0
	physicalIds := make(map[string]bool)

	for _, line := range lines {
		// Extract CPU model name
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) >= 2 {
				modelName = strings.TrimSpace(parts[1])
			}
		}

		// Count physical cores by looking at "processor" entries
		if strings.HasPrefix(line, "processor") {
			cpuCores++
		}

		// Track physical IDs to count actual CPUs
		if strings.HasPrefix(line, "physical id") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) >= 2 {
				physicalId := strings.TrimSpace(parts[1])
				physicalIds[physicalId] = true
			}
		}
	}

	// Calculate number of physical CPUs
	numPhysicalCPUs := len(physicalIds)
	if numPhysicalCPUs == 0 {
		numPhysicalCPUs = 1 // Default to 1 if we can't determine
	}

	// Calculate threads per core (logical cores / physical cores)
	threadsPerCore := cpuCores
	if numPhysicalCPUs > 0 && cpuCores > 0 {
		threadsPerCore = cpuCores / numPhysicalCPUs
	}

	// Format the result
	if modelName != "" {
		// Use colored values for the numbers
		cpuCoresStr := ValueStyle.Render(fmt.Sprintf("%d", cpuCores))
		if numPhysicalCPUs > 1 || threadsPerCore > 1 {
			// Show more detailed info if we have multiple physical CPUs or threads
			return fmt.Sprintf("%s (%s cores, %s CPUs)",
				DefaultStyle.Render(modelName),
				cpuCoresStr,
				ValueStyle.Render(fmt.Sprintf("%d", numPhysicalCPUs)))
		} else {
			// Simple output for single CPU
			return fmt.Sprintf("%s (%s cores)",
				DefaultStyle.Render(modelName),
				cpuCoresStr)
		}
	}

	// Fallback if we couldn't parse model name but have core count
	if cpuCores > 0 {
		return fmt.Sprintf("%s cores", ValueStyle.Render(fmt.Sprintf("%d", cpuCores)))
	}

	// Fallback to lscpu if we couldn't parse /proc/cpuinfo
	lscpuOutput := ExecCommand("lscpu")
	if lscpuOutput != "Not available" {
		lines := strings.Split(lscpuOutput, "\n")
		modelLine := ""
		coresLine := ""

		for _, line := range lines {
			if strings.HasPrefix(line, "Model name:") {
				modelLine = line
			}
			if strings.HasPrefix(line, "CPU(s):") {
				coresLine = line
			}
		}

		if modelLine != "" && coresLine != "" {
			modelParts := strings.SplitN(modelLine, ":", 2)
			coresParts := strings.SplitN(coresLine, ":", 2)

			if len(modelParts) >= 2 && len(coresParts) >= 2 {
				model := strings.TrimSpace(modelParts[1])
				cores := strings.TrimSpace(coresParts[1])

				return fmt.Sprintf("%s (%s cores)",
					DefaultStyle.Render(model),
					ValueStyle.Render(cores))
			}
		}
	}

	return DefaultStyle.Render("Not available")
}

// GetMemoryInfo returns the system memory usage with a visual bar
func GetMemoryInfo() string {
	// Constants for memory usage bar
	const (
		maxUsageThreshold = 90 // Percentage at which memory usage is considered high
		barWidth          = 50 // Width of the usage bar in characters
	)

	// Try to read from /proc/meminfo first
	content, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return DefaultStyle.Render("Not available")
	}

	// Parse the meminfo content
	memInfo := make(map[string]uint64)
	lines := strings.Split(string(content), "\n")

	re := regexp.MustCompile(`^(\S+):\s+(\d+)`)
	for _, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) == 3 {
			key := matches[1]
			valueStr := matches[2]
			value, err := strconv.ParseUint(valueStr, 10, 64)
			if err == nil {
				memInfo[key] = value
			}
		}
	}

	// Check if we have the required fields
	if memTotal, ok := memInfo["MemTotal"]; ok {
		var memAvailable uint64
		if avail, ok := memInfo["MemAvailable"]; ok {
			// MemAvailable is more accurate for modern kernels
			memAvailable = avail
		} else if free, ok := memInfo["MemFree"]; ok {
			// Fallback to MemFree for older kernels
			memAvailable = free
		} else {
			return DefaultStyle.Render("Not available")
		}

		memUsed := memTotal - memAvailable

		// Calculate usage percentage
		usagePercent := int((float64(memUsed) / float64(memTotal)) * 100)

		// Format total memory in GB
		totalGB := float64(memTotal) / 1024.0 / 1024.0
		totalFormatted := fmt.Sprintf("%dG", int(totalGB))

		// Calculate the bar
		usedWidth := (usagePercent * barWidth) / 100
		unusedWidth := barWidth - usedWidth

		// Choose color based on usage threshold
		var usedBarStyle lipgloss.Style
		if usagePercent >= maxUsageThreshold {
			usedBarStyle = RedStyle
		} else {
			usedBarStyle = GreenStyle
		}

		// Create the usage bar
		usedBar := strings.Repeat("=", usedWidth)
		unusedBar := strings.Repeat("=", unusedWidth)

		// Style the bars
		styledUsedBar := usedBarStyle.Render(usedBar)
		styledUnusedBar := DimStyle.Render(unusedBar)

		// Create the complete bar
		completeBar := fmt.Sprintf("[%s%s]", styledUsedBar, styledUnusedBar)

		// Format the info line
		infoLine := fmt.Sprintf("%-30s%3d%% used out of %4s", "", usagePercent, totalFormatted)

		// Return the formatted memory info string
		return DefaultStyle.Render(infoLine) + "\n" + completeBar
	}

	return DefaultStyle.Render("Not available")
}

// GetDockerInfo returns information about Docker containers with their status
func GetDockerInfo() string {
	var output strings.Builder

	// Check if Docker service is running
	statusOutput := ExecCommand("systemctl", "is-active", "docker")
	if statusOutput != "active" {
		// Check if Docker is installed but not running
		installedCheck := ExecCommand("which", "docker")
		if installedCheck != "Not available" {
			return DefaultStyle.Render("Docker is installed but not running")
		}
		return DefaultStyle.Render("Docker is not installed or not detected")
	}

	// Get container list with detailed format
	containerOutput := ExecCommand("docker", "ps", "-a", "--format", "{{.Names}}|{{.Status}}|{{.State}}")
	if containerOutput == "Not available" || containerOutput == "" {
		return DefaultStyle.Render("Docker is running but no containers found")
	}

	containerLines := strings.Split(containerOutput, "\n")
	if len(containerLines) == 0 || (len(containerLines) == 1 && containerLines[0] == "") {
		return DefaultStyle.Render("Docker is running but no containers found")
	}

	// Process container statuses
	var problemContainers []string
	runningCount := 0
	totalCount := 0

	// Sort containers alphabetically by name
	sort.Strings(containerLines)

	for _, line := range containerLines {
		if line == "" {
			continue
		}

		totalCount++

		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}

		name := parts[0]
		statusText := parts[1]
		stateText := parts[2]

		// Determine container state based on the state field directly
		isProblematic := false
		status := ""

		// Use the State field (3rd column) to determine the basic state
		switch stateText {
		case "running":
			runningCount++
			// Check for health status
			if strings.Contains(statusText, "unhealthy") {
				isProblematic = true
				status = "running (unhealthy)"
			} else if strings.Contains(statusText, "healthy") {
				status = "running (healthy)"
			} else {
				status = "running"
			}
		case "restarting":
			isProblematic = true
			status = "restarting"
		case "exited":
			// Extract exit code for stopped containers
			exitCodeMatch := regexp.MustCompile(`Exited \((\d+)\)`).FindStringSubmatch(statusText)
			if len(exitCodeMatch) > 1 && exitCodeMatch[1] != "0" {
				isProblematic = true
				status = fmt.Sprintf("stopped (error: %s)", exitCodeMatch[1])
			} else {
				status = "stopped"
				isProblematic = true // Consider stopped containers as problematic
			}
		case "created":
			isProblematic = true
			status = "created"
		case "paused":
			isProblematic = true
			status = "paused"
		case "dead":
			isProblematic = true
			status = "dead"
		default:
			// For any other state, consider it problematic
			isProblematic = true
			status = stateText // Use the raw state
		}

		// Only add problematic containers to the result
		if isProblematic {
			// Use DefaultStyle for the container name and the specific status style
			formattedLine := fmt.Sprintf("%s: %s", DefaultStyle.Render(name), RedStyle.Render(status))
			problemContainers = append(problemContainers, formattedLine)
		}
	}

	// Create a simple summary line - always show total and running
	if len(problemContainers) > 0 {
		output.WriteString(DefaultStyle.Render(fmt.Sprintf("%d containers (%d running, %d need attention)",
			totalCount, runningCount, len(problemContainers))))
	} else {
		output.WriteString(DefaultStyle.Render(fmt.Sprintf("%d containers (%d running)",
			totalCount, runningCount)))
	}

	// If there are problematic containers, add them to the output
	if len(problemContainers) > 0 {
		for _, container := range problemContainers {
			output.WriteString(fmt.Sprintf("\n%s", container))
		}
	}

	return output.String()
}

// GetDiskInfo returns the disk usage for all real partitions with visual bars
func GetDiskInfo() string {
	var output strings.Builder

	// Constants for disk usage bar
	const (
		maxUsageThreshold = 90 // Percentage at which disk usage is considered high
		barWidth          = 50 // Width of the usage bar in characters
	)

	// Run df command to get disk usage with the proper exclusions
	dfOutput := ExecCommand("df", "-H", "-x", "tmpfs", "-x", "overlay", "-x", "fuse.mergerfs", "-x", "fuse.rclone",
		"--output=target,pcent,size")
	if dfOutput == "Not available" {
		return DefaultStyle.Render("Not available")
	}

	// Process df output
	lines := strings.Split(dfOutput, "\n")
	if len(lines) <= 1 { // If there's only one line (the header), then no valid partitions
		return DefaultStyle.Render("No valid disk partitions found")
	}

	// Skip the header line
	lines = lines[1:]
	var partitions []struct {
		mountPoint   string
		usagePercent int
		size         string
		formattedBar string
	}

	// Process each partition
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		// Skip specific mount points
		mountPoint := fields[0]
		if strings.HasPrefix(mountPoint, "/dev") ||
			strings.HasPrefix(mountPoint, "/sys") ||
			strings.HasPrefix(mountPoint, "/proc") ||
			strings.HasPrefix(mountPoint, "/run") {
			continue
		}

		// Get percentage (remove the '%' character)
		usagePercentStr := strings.TrimSuffix(fields[1], "%")
		usagePercent, err := strconv.Atoi(usagePercentStr)
		if err != nil {
			continue
		}

		// Get size
		size := fields[2]

		// Calculate the bar
		usedWidth := (usagePercent * barWidth) / 100
		unusedWidth := barWidth - usedWidth

		// Choose color based on usage threshold
		var usedBarStyle lipgloss.Style
		if usagePercent >= maxUsageThreshold {
			usedBarStyle = RedStyle
		} else {
			usedBarStyle = GreenStyle
		}

		// Create the usage bar
		usedBar := strings.Repeat("=", usedWidth)
		unusedBar := strings.Repeat("=", unusedWidth)

		// Style the bars
		styledUsedBar := usedBarStyle.Render(usedBar)
		styledUnusedBar := DimStyle.Render(unusedBar)

		// Create the complete bar
		completeBar := fmt.Sprintf("[%s%s]", styledUsedBar, styledUnusedBar)

		// Add to partitions slice
		partitions = append(partitions, struct {
			mountPoint   string
			usagePercent int
			size         string
			formattedBar string
		}{
			mountPoint:   mountPoint,
			usagePercent: usagePercent,
			size:         size,
			formattedBar: completeBar,
		})
	}

	if len(partitions) == 0 {
		return DefaultStyle.Render("No valid disk partitions found")
	}

	// Format the results
	for i, p := range partitions {
		// For the first partition, add it directly to the output
		if i == 0 {
			// Format using original format with wide fixed spacing and mountpoint
			infoLine := fmt.Sprintf("%-30s%3d%% used out of %4s", p.mountPoint, p.usagePercent, p.size)
			output.WriteString(DefaultStyle.Render(infoLine))
			output.WriteString(fmt.Sprintf("\n%s", p.formattedBar))
		} else {
			// For subsequent partitions, add line breaks before
			infoLine := fmt.Sprintf("%-30s%3d%% used out of %4s", p.mountPoint, p.usagePercent, p.size)
			output.WriteString(fmt.Sprintf("\n%s", DefaultStyle.Render(infoLine)))
			output.WriteString(fmt.Sprintf("\n%s", p.formattedBar))
		}
	}

	return output.String()
}

// GetEmptyLine returns an empty line
func GetEmptyLine() string {
	return " " // Using a space rather than completely empty string for better visibility
}
